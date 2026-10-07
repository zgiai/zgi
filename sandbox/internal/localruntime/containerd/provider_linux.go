//go:build linux

// Package containerd provides the local Agent execution environment lifecycle
// on containerd. Each Binding receives a dedicated gVisor container and a
// retained host workspace. The Runtime Agent is reached only through a
// host-visible Unix socket.
package containerd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	runtimeoptions "github.com/containerd/containerd/api/types/runtimeoptions/v1"
	containerdclient "github.com/containerd/containerd/v2/client"
	corecontainers "github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/errdefs"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/egressproxy"
	"github.com/zgiai/zgi-sandbox/internal/localruntime"
	"github.com/zgiai/zgi-sandbox/internal/packagesource"
	"github.com/zgiai/zgi-sandbox/internal/runtimeagent"
)

const (
	defaultAddress      = "/run/containerd/containerd.sock"
	defaultNamespace    = "zgi-agent-runtime"
	defaultRuntimeName  = "io.containerd.runsc.v1"
	defaultAgentPath    = "/usr/local/bin/zgi-runtime-agent"
	defaultStartupLimit = 30 * time.Second
	defaultMemoryBytes  = 512 * 1024 * 1024
	defaultPIDs         = 128
	defaultCPUPeriod    = 100000
	defaultCPUQuota     = 100000
	runtimeSocketName   = "runtime.sock"
	runtimeTokenName    = "token"
	// The gVisor shim treats a container without this OCI annotation as a
	// subcontainer and cannot manage its standalone lifecycle correctly.
	gvisorContainerTypeAnnotation = "io.kubernetes.cri.container-type"
	gvisorSandboxContainerType    = "sandbox"
	// A sandbox-local UDS is invisible to the host even on a bind mount. Allow
	// creating host-visible sockets so the worker can reach the Runtime Agent;
	// do not allow opening existing host sockets.
	gvisorHostUDSAnnotation = "dev.gvisor.flag.host-uds"
	gvisorHostUDSCreate     = "create"
	// Keep more than one bridge available because HTTP clients such as pip
	// retain CONNECT tunnels with keep-alive while opening a second host.
	// The bound prevents an idle or malicious client from creating an
	// unbounded number of host-side bridge connections.
	maxEgressBridges = 8
)

var (
	safeIDPattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
	digestImagePattern = regexp.MustCompile(`@sha256:[a-f0-9]{64}$`)
)

type Profile struct {
	Image                     string               `json:"image"`
	RuntimeAgentPath          string               `json:"runtime_agent_path,omitempty"`
	DependencyPreparation     bool                 `json:"dependency_preparation,omitempty"`
	DependencyPreparationMode string               `json:"dependency_preparation_mode,omitempty"`
	DependencyInstallation    bool                 `json:"dependency_installation,omitempty"`
	PackageNetworkMode        string               `json:"package_network_mode,omitempty"`
	EgressAllowedHosts        []string             `json:"egress_allowed_hosts,omitempty"`
	PackageSources            packagesource.Config `json:"package_sources,omitempty"`
	PackageSourceCheck        bool                 `json:"package_source_check,omitempty"`
}

type Options struct {
	Address      string
	Namespace    string
	RootDir      string
	RuntimeName  string
	Profiles     map[string]Profile
	StartupLimit time.Duration
	MemoryBytes  int64
	PIDs         int64
	CPUPeriod    uint64
	CPUQuota     int64
	UserID       uint32
	GroupID      uint32
	NewToken     func() (string, error)
}

type Provider struct {
	client       *containerdclient.Client
	rootDir      string
	runscConfig  string
	runtimeName  string
	profiles     map[string]Profile
	startupLimit time.Duration
	memoryBytes  int64
	pids         int64
	cpuPeriod    uint64
	cpuQuota     int64
	userID       uint32
	groupID      uint32
	newToken     func() (string, error)
	bridgeMu     sync.Mutex
	bridges      map[string]context.CancelFunc
}

var _ localruntime.Provider = (*Provider)(nil)

func New(options Options) (*Provider, error) {
	options = defaultOptions(options)
	if err := validateOptions(options); err != nil {
		return nil, err
	}
	rootDir, err := prepareRoot(options.RootDir)
	if err != nil {
		return nil, err
	}
	client, err := containerdclient.New(options.Address, containerdclient.WithDefaultNamespace(options.Namespace))
	if err != nil {
		return nil, fmt.Errorf("connect containerd: %w", err)
	}
	return &Provider{
		client: client, rootDir: rootDir, runscConfig: filepath.Join(rootDir, "runsc-controlled.toml"), runtimeName: options.RuntimeName,
		profiles: cloneProfiles(options.Profiles), startupLimit: options.StartupLimit,
		memoryBytes: options.MemoryBytes, pids: options.PIDs,
		cpuPeriod: options.CPUPeriod, cpuQuota: options.CPUQuota,
		userID: options.UserID, groupID: options.GroupID, newToken: options.NewToken,
		bridges: make(map[string]context.CancelFunc),
	}, nil
}

func (p *Provider) Close() error {
	p.bridgeMu.Lock()
	for bindingID, cancel := range p.bridges {
		cancel()
		delete(p.bridges, bindingID)
	}
	p.bridgeMu.Unlock()
	return p.client.Close()
}

func (p *Provider) Create(ctx context.Context, spec agentexecution.CreateSpec) (agentexecution.Allocation, error) {
	return p.launch(ctx, spec)
}

func (p *Provider) Suspend(ctx context.Context, allocation agentexecution.Allocation) error {
	if err := validateAllocation(allocation); err != nil {
		return err
	}
	p.stopEgressBridge(allocation.BindingRef)
	if err := p.stopCompute(ctx, allocation.BindingRef); err != nil {
		return err
	}
	return errors.Join(
		os.RemoveAll(p.controlDir(allocation.BindingRef)),
		os.RemoveAll(p.secretDir(allocation.BindingRef)),
	)
}

func (p *Provider) Resume(ctx context.Context, spec agentexecution.CreateSpec, allocation agentexecution.Allocation) (agentexecution.Allocation, error) {
	if err := validateAllocation(allocation); err != nil {
		return agentexecution.Allocation{}, err
	}
	if allocation.BindingRef != containerID(spec.BindingID) || allocation.WorkspaceRef != spec.WorkspaceID {
		return agentexecution.Allocation{}, fmt.Errorf("%w: allocation does not match the binding", agentexecution.ErrInvalidRequest)
	}
	if info, err := os.Stat(p.workspaceDir(allocation.WorkspaceRef)); errors.Is(err, os.ErrNotExist) {
		return agentexecution.Allocation{}, agentexecution.ErrNotFound
	} else if err != nil {
		return agentexecution.Allocation{}, err
	} else if !info.IsDir() {
		return agentexecution.Allocation{}, fmt.Errorf("%w: retained workspace is not a directory", agentexecution.ErrBackendUnavailable)
	}
	return p.launch(ctx, spec)
}

func (p *Provider) Destroy(ctx context.Context, allocation agentexecution.Allocation) error {
	if err := validateAllocation(allocation); err != nil {
		return err
	}
	p.stopEgressBridge(allocation.BindingRef)
	if err := p.stopCompute(ctx, allocation.BindingRef); err != nil {
		return err
	}
	return errors.Join(
		os.RemoveAll(p.workspaceDir(allocation.WorkspaceRef)),
		os.RemoveAll(p.controlDir(allocation.BindingRef)),
		os.RemoveAll(p.secretDir(allocation.BindingRef)),
	)
}

func (p *Provider) Connect(_ context.Context, allocation agentexecution.Allocation) (agentexecution.ExecutionBackend, error) {
	if err := validateAllocation(allocation); err != nil {
		return nil, err
	}
	token, err := os.ReadFile(filepath.Join(p.secretDir(allocation.BindingRef), runtimeTokenName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, agentexecution.ErrBackendUnavailable
		}
		return nil, fmt.Errorf("read runtime agent token: %w", err)
	}
	return runtimeagent.NewUnixRemote(filepath.Join(p.controlDir(allocation.BindingRef), runtimeSocketName), string(token), 50*time.Millisecond)
}

func (p *Provider) launch(ctx context.Context, spec agentexecution.CreateSpec) (agentexecution.Allocation, error) {
	networkMode := strings.TrimSpace(spec.NetworkMode)
	if networkMode != "" && networkMode != "disabled" && networkMode != "controlled-http" && networkMode != "package-install" {
		return agentexecution.Allocation{}, fmt.Errorf("%w: local gVisor runtime network mode %q is not available", agentexecution.ErrBackendUnavailable, networkMode)
	}
	allocation, profile, err := p.resolveCreate(spec)
	if err != nil {
		return allocation, err
	}
	startupContext, cancelStartup := context.WithTimeout(ctx, p.startupLimit)
	defer cancelStartup()
	p.stopEgressBridge(allocation.BindingRef)
	if err := p.stopCompute(startupContext, allocation.BindingRef); err != nil {
		return allocation, fmt.Errorf("remove stale local runtime compute: %w", err)
	}
	workspaceDir := p.workspaceDir(allocation.WorkspaceRef)
	controlDir := p.controlDir(allocation.BindingRef)
	secretDir := p.secretDir(allocation.BindingRef)
	if err := prepareOwnedDirectory(workspaceDir, p.userID, p.groupID, 0o750); err != nil {
		return allocation, fmt.Errorf("prepare runtime workspace: %w", err)
	}
	if err := os.RemoveAll(controlDir); err != nil {
		return allocation, fmt.Errorf("reset runtime control directory: %w", err)
	}
	if err := prepareOwnedDirectory(controlDir, p.userID, p.groupID, 0o750); err != nil {
		return allocation, fmt.Errorf("prepare runtime control directory: %w", err)
	}
	if err := os.MkdirAll(secretDir, 0o700); err != nil {
		return allocation, fmt.Errorf("prepare runtime secret directory: %w", err)
	}
	token, err := p.newToken()
	if err != nil {
		return allocation, fmt.Errorf("create runtime agent token: %w", err)
	}
	if err := writeSecret(filepath.Join(secretDir, runtimeTokenName), token); err != nil {
		return allocation, err
	}

	image, err := p.client.GetImage(startupContext, profile.Image)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return allocation, fmt.Errorf("%w: approved profile image is not preloaded", agentexecution.ErrBackendUnavailable)
		}
		return allocation, fmt.Errorf("get profile image: %w", err)
	}
	if networkMode == "controlled-http" || networkMode == "package-install" {
		if err := p.ensureControlledRunscConfig(); err != nil {
			return allocation, err
		}
	}
	jobUID, jobGID := uint32(0), uint32(0)
	if profile.DependencyInstallation {
		jobUID, jobGID = p.userID, p.groupID
	}
	container, err := p.client.NewContainer(startupContext, allocation.BindingRef,
		containerdclient.WithImage(image),
		containerdclient.WithNewSnapshot(allocation.BindingRef+"-rootfs", image),
		containerdclient.WithRuntime(p.runtimeName, runscRuntimeOptions(networkMode, p.runscConfig)),
		containerdclient.WithContainerLabels(map[string]string{
			"zgi.agent-runtime.managed": "true",
			"zgi.agent-runtime.binding": spec.BindingID,
			"zgi.agent-runtime.profile": spec.ProfileID,
		}),
		containerdclient.WithNewSpec(
			oci.WithImageConfig(image),
			oci.WithProcessArgs(profile.RuntimeAgentPath),
			oci.WithTTY,
			oci.WithEnv([]string{
				"ZGI_RUNTIME_AGENT_TOKEN=" + token,
				"ZGI_RUNTIME_AGENT_WORKSPACE=/workspace",
				"ZGI_RUNTIME_AGENT_LISTEN=unix:///run/zgi-agent/" + runtimeSocketName,
				"ZGI_RUNTIME_AGENT_NETWORK_MODE=" + fallback(networkMode, "disabled"),
				"ZGI_RUNTIME_AGENT_EGRESS_PROXY_ADDRESS=127.0.0.1:3128",
				"ZGI_RUNTIME_AGENT_EGRESS_SOCKET=/run/zgi-agent/egress.sock",
				"ZGI_RUNTIME_AGENT_DEPENDENCY_PREPARATION_ENABLED=" + strconv.FormatBool(profile.DependencyPreparation),
				"ZGI_RUNTIME_AGENT_DEPENDENCY_PREPARATION_MODE=" + strings.TrimSpace(profile.DependencyPreparationMode),
				"ZGI_RUNTIME_AGENT_DEPENDENCY_INSTALLATION_ENABLED=" + strconv.FormatBool(profile.DependencyInstallation),
				"ZGI_RUNTIME_AGENT_PACKAGE_NETWORK_MODE=" + strings.TrimSpace(profile.PackageNetworkMode),
				"ZGI_RUNTIME_AGENT_JOB_UID=" + strconv.FormatUint(uint64(jobUID), 10),
				"ZGI_RUNTIME_AGENT_JOB_GID=" + strconv.FormatUint(uint64(jobGID), 10),
				"ZGI_RUNTIME_AGENT_PACKAGE_LISTEN=unix:///run/zgi-agent/package.sock",
				"ZGI_RUNTIME_AGENT_PACKAGE_SOURCES_JSON=" + profile.PackageSources.JSON(),
				"ZGI_RUNTIME_AGENT_PACKAGE_SOURCE_CHECK=" + strconv.FormatBool(profile.PackageSourceCheck),
				"ZGI_RUNTIME_AGENT_PROFILE_IDENTITY=" + profile.Image,
			}),
			withRootFSReadonly(!profile.DependencyInstallation),
			oci.WithLinuxNamespace(specs.LinuxNamespace{Type: specs.NetworkNamespace}),
			oci.WithMounts([]specs.Mount{
				{Destination: "/workspace", Type: "bind", Source: workspaceDir, Options: []string{"rbind", "rw", "nosuid", "nodev"}},
				{Destination: "/run/zgi-agent", Type: "bind", Source: controlDir, Options: []string{"rbind", "rw", "nosuid", "nodev", "noexec"}},
				{Destination: "/tmp", Type: "tmpfs", Source: "tmpfs", Options: []string{"nosuid", "nodev", "mode=1777", "size=67108864"}},
			}),
			p.hardenSpec(profile.DependencyInstallation),
		),
	)
	if err != nil {
		return allocation, fmt.Errorf("create gVisor container: %w", err)
	}
	// The gVisor containerd shim requires a terminal-mode stdio configuration to
	// complete the task start handshake. Keep the streams detached from the host
	// while still providing FIFO paths; cio.NullIO leaves the shim in a
	// created/unknown state on valid workers.
	task, err := container.NewTask(startupContext, cio.NewCreator(
		cio.WithTerminal,
		cio.WithStreams(strings.NewReader(""), io.Discard, io.Discard),
	))
	if err != nil {
		cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		_ = container.Delete(cleanupContext, containerdclient.WithSnapshotCleanup)
		cancel()
		return allocation, fmt.Errorf("create gVisor task: %w", err)
	}
	if err := task.Start(startupContext); err != nil {
		cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		_, _ = task.Delete(cleanupContext, containerdclient.WithProcessKill)
		_ = container.Delete(cleanupContext, containerdclient.WithSnapshotCleanup)
		cancel()
		return allocation, fmt.Errorf("start gVisor task: %w", err)
	}
	if err := p.waitReady(startupContext, allocation, profile); err != nil {
		cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		_ = p.stopCompute(cleanupContext, allocation.BindingRef)
		cancel()
		return allocation, err
	}
	if networkMode == "controlled-http" || networkMode == "package-install" {
		if err := p.startEgressBridge(startupContext, allocation, profile); err != nil {
			cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			_ = p.stopCompute(cleanupContext, allocation.BindingRef)
			cancel()
			return allocation, err
		}
	}
	return allocation, nil
}

// runscRuntimeOptions keeps the controlled HTTP profile on gVisor's isolated
// netstack with loopback available. A fresh OCI network namespace has no
// configured link, so the default runsc network mode cannot even bind the
// loopback proxy address. The "none" mode intentionally provides only the
// in-sandbox loopback; outbound traffic still has to cross the host bridge
// through the runtime-agent UDS.
func runscRuntimeOptions(networkMode, configPath string) interface{} {
	if networkMode != "controlled-http" && networkMode != "package-install" {
		return nil
	}
	return &runtimeoptions.Options{
		TypeUrl:    "io.containerd.runsc.v1.options",
		ConfigPath: configPath,
	}
}

func (p *Provider) ensureControlledRunscConfig() error {
	info, err := os.Lstat(p.runscConfig)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return errors.New("controlled gVisor runtime config cannot be a symbolic link")
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect controlled gVisor runtime config: %w", err)
	}
	file, err := os.OpenFile(p.runscConfig, os.O_CREATE|os.O_TRUNC|os.O_WRONLY|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return fmt.Errorf("create controlled gVisor runtime config: %w", err)
	}
	if _, err := file.WriteString("[runsc_config]\nnetwork = \"none\"\n"); err != nil {
		_ = file.Close()
		return fmt.Errorf("write controlled gVisor runtime config: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync controlled gVisor runtime config: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close controlled gVisor runtime config: %w", err)
	}
	return nil
}

func (p *Provider) waitReady(ctx context.Context, allocation agentexecution.Allocation, profile Profile) error {
	readyContext, cancel := context.WithTimeout(ctx, p.startupLimit)
	defer cancel()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	var lastError error
	for {
		connection, err := p.Connect(readyContext, allocation)
		if err == nil {
			var capabilities agentexecution.RuntimeCapabilities
			capabilities, err = connection.Capabilities(readyContext, allocation)
			if err == nil && profile.DependencyInstallation {
				return validatePackageLedgerCapability(capabilities)
			}
		}
		if err == nil {
			return nil
		}
		lastError = err
		select {
		case <-readyContext.Done():
			return fmt.Errorf("runtime agent did not become ready: %w", errors.Join(readyContext.Err(), lastError))
		case <-ticker.C:
		}
	}
}

func validatePackageLedgerCapability(capabilities agentexecution.RuntimeCapabilities) error {
	if capabilities.SystemPackageLedgerVersion != agentexecution.SystemPackageLedgerVersion {
		return fmt.Errorf("%w: local runtime image lacks fixed-version system package recovery; rebuild the package image", agentexecution.ErrBackendUnavailable)
	}
	return nil
}

func (p *Provider) startEgressBridge(ctx context.Context, allocation agentexecution.Allocation, profile Profile) error {
	policy := egressproxy.Policy{AllowedHosts: append([]string(nil), profile.EgressAllowedHosts...)}
	if err := policy.Validate(); err != nil {
		return fmt.Errorf("%w: %v", agentexecution.ErrInvalidRequest, err)
	}
	bridgeContext, cancel := context.WithCancel(context.Background())
	p.bridgeMu.Lock()
	if previous := p.bridges[allocation.BindingRef]; previous != nil {
		previous()
	}
	p.bridges[allocation.BindingRef] = cancel
	p.bridgeMu.Unlock()
	socketPath := filepath.Join(p.controlDir(allocation.BindingRef), "egress.sock")
	go func() {
		bridgeSlots := make(chan struct{}, maxEgressBridges)
		defer func() {
			p.bridgeMu.Lock()
			if current := p.bridges[allocation.BindingRef]; current != nil {
				delete(p.bridges, allocation.BindingRef)
			}
			p.bridgeMu.Unlock()
		}()
		for {
			select {
			case bridgeSlots <- struct{}{}:
			case <-bridgeContext.Done():
				return
			}
			if bridgeContext.Err() != nil {
				<-bridgeSlots
				return
			}
			connection, err := (&net.Dialer{Timeout: time.Second}).DialContext(bridgeContext, "unix", socketPath)
			if err != nil {
				<-bridgeSlots
				select {
				case <-bridgeContext.Done():
					return
				case <-time.After(100 * time.Millisecond):
					continue
				}
			}
			go func(connection net.Conn) {
				defer func() { <-bridgeSlots }()
				_ = egressproxy.ServeConnection(bridgeContext, connection, policy)
			}(connection)
		}
	}()
	return nil
}

func (p *Provider) stopEgressBridge(bindingRef string) {
	p.bridgeMu.Lock()
	if cancel := p.bridges[bindingRef]; cancel != nil {
		cancel()
		delete(p.bridges, bindingRef)
	}
	p.bridgeMu.Unlock()
}

func (p *Provider) stopCompute(ctx context.Context, bindingRef string) error {
	container, err := p.client.LoadContainer(ctx, bindingRef)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("load local runtime container: %w", err)
	}
	task, err := container.Task(ctx, nil)
	if err == nil {
		status, statusErr := task.Status(ctx)
		if statusErr == nil && status.Status == containerdclient.Paused {
			_ = task.Resume(ctx)
		}
		wait, waitErr := task.Wait(ctx)
		var killErr error
		if statusErr != nil || status.Status != containerdclient.Stopped {
			killErr = task.Kill(ctx, syscall.SIGKILL, containerdclient.WithKillAll)
			if errdefs.IsNotFound(killErr) {
				killErr = nil
			}
		}
		if waitErr == nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-wait:
			}
		}
		_, deleteErr := task.Delete(ctx)
		if errdefs.IsNotFound(deleteErr) {
			deleteErr = nil
		}
		if err := errors.Join(killErr, deleteErr); err != nil {
			return fmt.Errorf("delete local runtime task: %w", err)
		}
	} else if !errdefs.IsNotFound(err) {
		return fmt.Errorf("load local runtime task: %w", err)
	}
	if err := container.Delete(ctx, containerdclient.WithSnapshotCleanup); err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("delete local runtime container: %w", err)
	}
	return nil
}

func (p *Provider) resolveCreate(spec agentexecution.CreateSpec) (agentexecution.Allocation, Profile, error) {
	allocation := agentexecution.Allocation{BindingRef: containerID(spec.BindingID), WorkspaceRef: spec.WorkspaceID}
	if !safeIDPattern.MatchString(spec.BindingID) || !safeIDPattern.MatchString(spec.WorkspaceID) || !safeIDPattern.MatchString(allocation.BindingRef) {
		return allocation, Profile{}, fmt.Errorf("%w: binding and workspace identifiers are invalid", agentexecution.ErrInvalidRequest)
	}
	profile, ok := p.profiles[spec.ProfileID]
	if !ok {
		return allocation, Profile{}, fmt.Errorf("%w: profile is not approved for the local runtime", agentexecution.ErrInvalidRequest)
	}
	if spec.HomeSnapshotID != "" {
		if !digestImagePattern.MatchString(spec.HomeSnapshotRef) {
			return allocation, Profile{}, fmt.Errorf("%w: approved Home Snapshot image must be pinned by digest", agentexecution.ErrInvalidRequest)
		}
		profile.Image = spec.HomeSnapshotRef
	}
	if strings.TrimSpace(spec.NetworkMode) == "controlled-http" || strings.TrimSpace(spec.NetworkMode) == "package-install" {
		if err := (egressproxy.Policy{AllowedHosts: profile.EgressAllowedHosts}).Validate(); err != nil {
			return allocation, profile, fmt.Errorf("%w: package/profile egress policy: %v", agentexecution.ErrInvalidRequest, err)
		}
	}
	if strings.TrimSpace(spec.NetworkMode) == "package-install" {
		if !profile.DependencyInstallation || strings.ToLower(strings.TrimSpace(profile.PackageNetworkMode)) != "package-install" {
			return allocation, profile, fmt.Errorf("%w: profile does not approve package installation", agentexecution.ErrBackendUnavailable)
		}
		if err := validatePackageSourceAllowlist(profile.EgressAllowedHosts, profile.PackageSources); err != nil {
			return allocation, profile, fmt.Errorf("%w: package source allowlist: %v", agentexecution.ErrInvalidRequest, err)
		}
	}
	return allocation, profile, nil
}

func (p *Provider) hardenSpec(runtimeAgentRoot ...bool) oci.SpecOpts {
	return func(_ context.Context, _ oci.Client, _ *corecontainers.Container, spec *oci.Spec) error {
		if spec.Process == nil {
			return errors.New("OCI process is missing")
		}
		if spec.Annotations == nil {
			spec.Annotations = make(map[string]string)
		}
		spec.Annotations[gvisorContainerTypeAnnotation] = gvisorSandboxContainerType
		spec.Annotations[gvisorHostUDSAnnotation] = gvisorHostUDSCreate
		spec.Process.Cwd = "/workspace"
		if len(runtimeAgentRoot) > 0 && runtimeAgentRoot[0] {
			spec.Process.User.UID = 0
			spec.Process.User.GID = 0
		} else {
			spec.Process.User.UID = p.userID
			spec.Process.User.GID = p.groupID
		}
		spec.Process.NoNewPrivileges = true
		spec.Process.Capabilities = &specs.LinuxCapabilities{}
		if len(runtimeAgentRoot) > 0 && runtimeAgentRoot[0] {
			// Only the Runtime Agent root process receives the small set of
			// capabilities needed by dpkg to write the package layer. Jobs are
			// started with UID/GID 65532 and do not inherit these after exec.
			capabilities := []string{"CAP_CHOWN", "CAP_DAC_OVERRIDE", "CAP_FOWNER", "CAP_SETGID", "CAP_SETUID", "CAP_KILL"}
			spec.Process.Capabilities = &specs.LinuxCapabilities{
				Bounding: capabilities, Effective: capabilities, Inheritable: capabilities,
				Permitted: capabilities, Ambient: capabilities,
			}
		}
		if spec.Linux == nil {
			spec.Linux = &specs.Linux{}
		}
		pids := p.pids
		spec.Linux.Resources = &specs.LinuxResources{
			Memory: &specs.LinuxMemory{Limit: &p.memoryBytes},
			Pids:   &specs.LinuxPids{Limit: &pids},
			CPU:    &specs.LinuxCPU{Period: &p.cpuPeriod, Quota: &p.cpuQuota},
		}
		return nil
	}
}

func withRootFSReadonly(readonly bool) oci.SpecOpts {
	if readonly {
		return oci.WithRootFSReadonly()
	}
	return func(context.Context, oci.Client, *corecontainers.Container, *oci.Spec) error { return nil }
}

func defaultOptions(options Options) Options {
	options.Address = fallback(options.Address, defaultAddress)
	options.Namespace = fallback(options.Namespace, defaultNamespace)
	options.RuntimeName = fallback(options.RuntimeName, defaultRuntimeName)
	if options.StartupLimit <= 0 {
		options.StartupLimit = defaultStartupLimit
	}
	if options.MemoryBytes <= 0 {
		options.MemoryBytes = defaultMemoryBytes
	}
	if options.PIDs <= 0 {
		options.PIDs = defaultPIDs
	}
	if options.CPUPeriod == 0 {
		options.CPUPeriod = defaultCPUPeriod
	}
	if options.CPUQuota <= 0 {
		options.CPUQuota = defaultCPUQuota
	}
	if options.UserID == 0 {
		options.UserID = 65532
	}
	if options.GroupID == 0 {
		options.GroupID = 65532
	}
	if options.NewToken == nil {
		options.NewToken = randomToken
	}
	profiles := cloneProfiles(options.Profiles)
	for id, profile := range profiles {
		profile.RuntimeAgentPath = fallback(profile.RuntimeAgentPath, defaultAgentPath)
		profiles[id] = profile
	}
	options.Profiles = profiles
	return options
}

func validateOptions(options Options) error {
	if strings.TrimSpace(options.RootDir) == "" || !filepath.IsAbs(options.RootDir) {
		return fmt.Errorf("%w: local runtime root must be absolute", agentexecution.ErrInvalidRequest)
	}
	if len(options.Profiles) == 0 {
		return fmt.Errorf("%w: at least one approved local runtime profile is required", agentexecution.ErrInvalidRequest)
	}
	if options.RuntimeName != defaultRuntimeName {
		return fmt.Errorf("%w: local runtime must use %s", agentexecution.ErrInvalidRequest, defaultRuntimeName)
	}
	for id, profile := range options.Profiles {
		if !safeIDPattern.MatchString(id) || !digestImagePattern.MatchString(profile.Image) {
			return fmt.Errorf("%w: profile %q must use a valid ID and digest-pinned image", agentexecution.ErrInvalidRequest, id)
		}
		if !filepath.IsAbs(profile.RuntimeAgentPath) {
			return fmt.Errorf("%w: profile %q runtime agent path must be absolute", agentexecution.ErrInvalidRequest, id)
		}
		if len(profile.EgressAllowedHosts) > 0 {
			if err := (egressproxy.Policy{AllowedHosts: profile.EgressAllowedHosts}).Validate(); err != nil {
				return fmt.Errorf("%w: profile %q egress policy: %v", agentexecution.ErrInvalidRequest, id, err)
			}
		}
		mode := strings.ToLower(strings.TrimSpace(profile.DependencyPreparationMode))
		if mode != "" && mode != runtimeagent.DependencyPreparationModeOfflineBundled && mode != runtimeagent.DependencyPreparationModeControlledHTTP {
			return fmt.Errorf("%w: profile %q dependency preparation mode %q is not supported", agentexecution.ErrInvalidRequest, id, mode)
		}
		packageMode := strings.ToLower(strings.TrimSpace(profile.PackageNetworkMode))
		if packageMode != "" && packageMode != "package-install" {
			return fmt.Errorf("%w: profile %q package network mode %q is not supported", agentexecution.ErrInvalidRequest, id, packageMode)
		}
		if profile.DependencyInstallation && packageMode != "package-install" {
			return fmt.Errorf("%w: profile %q dependency installation requires package-install network mode", agentexecution.ErrInvalidRequest, id)
		}
		if packageMode == "package-install" && !profile.DependencyInstallation {
			return fmt.Errorf("%w: profile %q package-install mode requires dependency installation", agentexecution.ErrInvalidRequest, id)
		}
		if packageMode == "package-install" && len(profile.EgressAllowedHosts) == 0 {
			return fmt.Errorf("%w: profile %q package-install mode requires an explicit package-source allowlist", agentexecution.ErrInvalidRequest, id)
		}
		if packageMode == "package-install" {
			if err := profile.PackageSources.Validate(profile.EgressAllowedHosts); err != nil {
				return err
			}
			if err := validatePackageSourceAllowlist(profile.EgressAllowedHosts, profile.PackageSources); err != nil {
				return fmt.Errorf("%w: profile %q package source allowlist: %v", agentexecution.ErrInvalidRequest, id, err)
			}
		}
		if packageMode == "package-install" && mode == runtimeagent.DependencyPreparationModeControlledHTTP {
			return fmt.Errorf("%w: profile %q cannot combine package-install with legacy controlled-http dependency preparation", agentexecution.ErrInvalidRequest, id)
		}
	}
	return nil
}

// validatePackageSourceAllowlist prevents a package-install Profile from
// silently becoming a general HTTPS egress Profile. Internal mirrors may use
// deployment-owned .internal/.local names; public hosts are limited to the
// registries and Debian mirrors used by the standard runtime.
func validatePackageSourceAllowlist(hosts []string, sources ...packagesource.Config) error {
	if err := (egressproxy.Policy{AllowedHosts: hosts}).Validate(); err != nil {
		return err
	}
	knownExact := map[string]struct{}{
		"pypi.org": {}, "registry.npmjs.org": {}, "registry.yarnpkg.com": {},
		"deb.debian.org": {}, "security.debian.org": {}, "snapshot.debian.org": {},
	}
	for _, source := range sources {
		if err := source.Validate(hosts); err != nil {
			return err
		}
		for _, host := range source.Hosts() {
			knownExact[strings.ToLower(host)] = struct{}{}
		}
	}
	for _, raw := range hosts {
		host := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(raw), "*."))
		known := strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".local")
		if _, ok := knownExact[host]; ok {
			known = true
		}
		if host == "pythonhosted.org" || strings.HasSuffix(host, ".pythonhosted.org") {
			known = true
		}
		if !known {
			return fmt.Errorf("host %q is not an approved package source", raw)
		}
	}
	return nil
}

func validateAllocation(allocation agentexecution.Allocation) error {
	if !safeIDPattern.MatchString(allocation.BindingRef) || !safeIDPattern.MatchString(allocation.WorkspaceRef) {
		return fmt.Errorf("%w: local runtime allocation is invalid", agentexecution.ErrInvalidRequest)
	}
	return nil
}

func prepareRoot(value string) (string, error) {
	if err := os.MkdirAll(value, 0o750); err != nil {
		return "", fmt.Errorf("prepare local runtime root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(value)
	if err != nil {
		return "", fmt.Errorf("resolve local runtime root: %w", err)
	}
	if !filepath.IsAbs(resolved) {
		return "", fmt.Errorf("%w: resolved local runtime root is not absolute", agentexecution.ErrInvalidRequest)
	}
	for _, child := range []string{"workspaces", "control", "secrets"} {
		childPath := filepath.Join(resolved, child)
		if info, err := os.Lstat(childPath); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("local runtime %s root cannot be a symbolic link", child)
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("inspect local runtime %s root: %w", child, err)
		}
		if err := os.MkdirAll(childPath, 0o750); err != nil {
			return "", fmt.Errorf("prepare local runtime %s root: %w", child, err)
		}
	}
	return resolved, nil
}

func prepareOwnedDirectory(path string, uid uint32, gid uint32, mode os.FileMode) error {
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return errors.New("runtime directory cannot be a symbolic link")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(path, mode); err != nil {
		return err
	}
	if err := os.Chmod(path, mode); err != nil {
		return err
	}
	return os.Chown(path, int(uid), int(gid))
}

func writeSecret(path string, value string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) {
		if removeErr := os.Remove(path); removeErr != nil {
			return fmt.Errorf("replace runtime agent token: %w", removeErr)
		}
		file, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	}
	if err != nil {
		return fmt.Errorf("create runtime agent token: %w", err)
	}
	if _, err := file.WriteString(value); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("write runtime agent token: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("sync runtime agent token: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("close runtime agent token: %w", err)
	}
	return nil
}

func randomToken() (string, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func cloneProfiles(source map[string]Profile) map[string]Profile {
	result := make(map[string]Profile, len(source))
	for id, profile := range source {
		profile.EgressAllowedHosts = append([]string(nil), profile.EgressAllowedHosts...)
		result[id] = profile
	}
	return result
}

func containerID(bindingID string) string {
	return "zgi-ar-" + bindingID
}

func (p *Provider) workspaceDir(ref string) string {
	return filepath.Join(p.rootDir, "workspaces", ref)
}
func (p *Provider) controlDir(ref string) string { return filepath.Join(p.rootDir, "control", ref) }
func (p *Provider) secretDir(ref string) string  { return filepath.Join(p.rootDir, "secrets", ref) }

func fallback(value string, fallbackValue string) string {
	if strings.TrimSpace(value) == "" {
		return fallbackValue
	}
	return strings.TrimSpace(value)
}
