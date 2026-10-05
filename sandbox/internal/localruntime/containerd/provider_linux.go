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
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	containerdclient "github.com/containerd/containerd/v2/client"
	corecontainers "github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/errdefs"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/localruntime"
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
)

var (
	safeIDPattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
	digestImagePattern = regexp.MustCompile(`@sha256:[a-f0-9]{64}$`)
)

type Profile struct {
	Image                 string `json:"image"`
	RuntimeAgentPath      string `json:"runtime_agent_path,omitempty"`
	DependencyPreparation bool   `json:"dependency_preparation,omitempty"`
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
		client: client, rootDir: rootDir, runtimeName: options.RuntimeName,
		profiles: cloneProfiles(options.Profiles), startupLimit: options.StartupLimit,
		memoryBytes: options.MemoryBytes, pids: options.PIDs,
		cpuPeriod: options.CPUPeriod, cpuQuota: options.CPUQuota,
		userID: options.UserID, groupID: options.GroupID, newToken: options.NewToken,
	}, nil
}

func (p *Provider) Close() error {
	return p.client.Close()
}

func (p *Provider) Create(ctx context.Context, spec agentexecution.CreateSpec) (agentexecution.Allocation, error) {
	return p.launch(ctx, spec)
}

func (p *Provider) Suspend(ctx context.Context, allocation agentexecution.Allocation) error {
	if err := validateAllocation(allocation); err != nil {
		return err
	}
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
	if networkMode := strings.TrimSpace(spec.NetworkMode); networkMode != "" && networkMode != "disabled" {
		return agentexecution.Allocation{}, fmt.Errorf("%w: local gVisor runtime network mode %q is not available", agentexecution.ErrBackendUnavailable, networkMode)
	}
	allocation, profile, err := p.resolveCreate(spec)
	if err != nil {
		return allocation, err
	}
	startupContext, cancelStartup := context.WithTimeout(ctx, p.startupLimit)
	defer cancelStartup()
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
	container, err := p.client.NewContainer(startupContext, allocation.BindingRef,
		containerdclient.WithImage(image),
		containerdclient.WithNewSnapshot(allocation.BindingRef+"-rootfs", image),
		containerdclient.WithRuntime(p.runtimeName, nil),
		containerdclient.WithContainerLabels(map[string]string{
			"zgi.agent-runtime.managed": "true",
			"zgi.agent-runtime.binding": spec.BindingID,
			"zgi.agent-runtime.profile": spec.ProfileID,
		}),
		containerdclient.WithNewSpec(
			oci.WithImageConfig(image),
			oci.WithProcessArgs(profile.RuntimeAgentPath),
			oci.WithEnv([]string{
				"ZGI_RUNTIME_AGENT_TOKEN=" + token,
				"ZGI_RUNTIME_AGENT_WORKSPACE=/workspace",
				"ZGI_RUNTIME_AGENT_LISTEN=unix:///run/zgi-agent/" + runtimeSocketName,
				"ZGI_RUNTIME_AGENT_NETWORK_MODE=disabled",
				"ZGI_RUNTIME_AGENT_DEPENDENCY_PREPARATION_ENABLED=" + strconv.FormatBool(profile.DependencyPreparation),
			}),
			oci.WithRootFSReadonly(),
			oci.WithLinuxNamespace(specs.LinuxNamespace{Type: specs.NetworkNamespace}),
			oci.WithMounts([]specs.Mount{
				{Destination: "/workspace", Type: "bind", Source: workspaceDir, Options: []string{"rbind", "rw", "nosuid", "nodev"}},
				{Destination: "/run/zgi-agent", Type: "bind", Source: controlDir, Options: []string{"rbind", "rw", "nosuid", "nodev", "noexec"}},
				{Destination: "/tmp", Type: "tmpfs", Source: "tmpfs", Options: []string{"nosuid", "nodev", "mode=1777", "size=67108864"}},
			}),
			p.hardenSpec(),
		),
	)
	if err != nil {
		return allocation, fmt.Errorf("create gVisor container: %w", err)
	}
	task, err := container.NewTask(startupContext, cio.NullIO)
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
	if err := p.waitReady(startupContext, allocation); err != nil {
		cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		_ = p.stopCompute(cleanupContext, allocation.BindingRef)
		cancel()
		return allocation, err
	}
	return allocation, nil
}

func (p *Provider) waitReady(ctx context.Context, allocation agentexecution.Allocation) error {
	readyContext, cancel := context.WithTimeout(ctx, p.startupLimit)
	defer cancel()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	var lastError error
	for {
		connection, err := p.Connect(readyContext, allocation)
		if err == nil {
			_, err = connection.Capabilities(readyContext, allocation)
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
	return allocation, profile, nil
}

func (p *Provider) hardenSpec() oci.SpecOpts {
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
		spec.Process.User.UID = p.userID
		spec.Process.User.GID = p.groupID
		spec.Process.NoNewPrivileges = true
		spec.Process.Capabilities = &specs.LinuxCapabilities{}
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
