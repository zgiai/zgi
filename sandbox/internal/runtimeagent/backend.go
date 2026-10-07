// Package runtimeagent implements the narrow execution protocol that runs
// inside an already isolated Linux environment. It does not create a security
// boundary by itself; the outer worker must provide gVisor, VM, or equivalent
// isolation and enforce the advertised network policy.
package runtimeagent

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/egressproxy"
	"github.com/zgiai/zgi-sandbox/internal/packagesource"
)

type Options struct {
	WorkspaceRoot string
	NetworkMode   string
	// EgressMode controls how the runtime obtains the package-only boundary.
	// bridge is used by local gVisor and connects the loopback HTTP proxy to a
	// host-side allowlist. direct is used by hosted providers: a root-owned
	// in-VM proxy applies the same HTTPS policy, while the template firewall
	// restricts Jobs to its loopback port and the provider enforces outer egress.
	EgressMode                string
	EgressProxyAddress        string
	EgressAllowedHosts        []string
	EgressSocketPath          string
	DependencyPreparation     bool
	DependencyPreparationMode string
	// DependencyInstallation enables the standard Skill package-manager
	// contract. It is only valid with package-install network mode, which is
	// bridged through the worker's fixed package-host allowlist.
	DependencyInstallation bool
	PackageNetworkMode     string
	PackageInstallTimeout  time.Duration
	MaxPackageOutputBytes  int64
	JobUID                 uint32
	JobGID                 uint32
	PackageSources         packagesource.Config
	APTConfigPath          string
	ProfileIdentity        string
	MaxUploadBytes         int64
	MaxDownloadBytes       int64
}

type Backend struct {
	workspace                 *os.Root
	workspaceName             string
	networkMode               string
	dependencyPreparation     bool
	dependencyPreparationMode string
	dependencyInstallation    bool
	packageNetworkMode        string
	packageNetworkEnforced    bool
	jobUID                    uint32
	jobGID                    uint32
	packageInstallTimeout     time.Duration
	maxPackageOutputBytes     int64
	packageInstallGate        chan struct{}
	packageInstallDone        chan struct{}
	packageInstalls           sync.WaitGroup
	packageReplayComplete     bool
	packageLedgerMu           sync.Mutex
	packageQueryPath          string // Empty uses the fixed in-sandbox dpkg-query binary.
	maxUploadBytes            int64
	maxDownloadBytes          int64
	egressProxyAddress        string
	egress                    *egressProxy
	mu                        sync.Mutex
	jobs                      map[string]trackedJob
	closed                    bool
	unsafe                    error
	packageSources            packagesource.Config
	aptConfigPath             string
	profileIdentity           string
	cacheMu                   sync.Mutex
	cacheKey                  string
}

const sessionHomeDirectory = ".zgi-runtime-home"

const DependencyPreparationModeOfflineBundled = "offline-bundled"
const DependencyPreparationModeControlledHTTP = "controlled-http"

const (
	EgressModeBridge = "bridge"
	EgressModeDirect = "direct"
)

type trackedJob struct {
	specHash string
	running  agentexecution.RunningJob
}

var _ agentexecution.ExecutionBackend = (*Backend)(nil)

func New(options Options) (*Backend, error) {
	if err := options.PackageSources.Validate(nil); err != nil {
		return nil, err
	}
	if options.PackageInstallTimeout < 0 || options.MaxPackageOutputBytes < 0 {
		return nil, fmt.Errorf("%w: package installation limits cannot be negative", agentexecution.ErrInvalidRequest)
	}
	options.WorkspaceRoot = strings.TrimSpace(options.WorkspaceRoot)
	if options.WorkspaceRoot == "" {
		return nil, fmt.Errorf("%w: workspace root is required", agentexecution.ErrInvalidRequest)
	}
	if err := os.MkdirAll(options.WorkspaceRoot, 0o750); err != nil {
		return nil, fmt.Errorf("prepare runtime workspace: %w", err)
	}
	root, err := os.OpenRoot(options.WorkspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("open runtime workspace: %w", err)
	}
	if options.MaxUploadBytes <= 0 {
		options.MaxUploadBytes = 64 * 1024 * 1024
	}
	if options.MaxDownloadBytes <= 0 {
		options.MaxDownloadBytes = 64 * 1024 * 1024
	}
	mode := strings.TrimSpace(options.NetworkMode)
	if mode == "" {
		mode = "unknown"
	}
	dependencyMode := strings.ToLower(strings.TrimSpace(options.DependencyPreparationMode))
	if options.DependencyPreparation {
		if dependencyMode == "" {
			dependencyMode = DependencyPreparationModeOfflineBundled
		}
		if dependencyMode != DependencyPreparationModeOfflineBundled && dependencyMode != DependencyPreparationModeControlledHTTP {
			return nil, fmt.Errorf("%w: dependency preparation mode %q is not available", agentexecution.ErrBackendUnavailable, dependencyMode)
		}
	} else if dependencyMode != "" {
		return nil, fmt.Errorf("%w: dependency preparation mode requires dependency preparation", agentexecution.ErrInvalidRequest)
	}
	packageMode := strings.ToLower(strings.TrimSpace(options.PackageNetworkMode))
	if packageMode == "" && options.DependencyInstallation {
		packageMode = "package-install"
	}
	if packageMode != "" && packageMode != "package-install" {
		return nil, fmt.Errorf("%w: package network mode %q is not available", agentexecution.ErrBackendUnavailable, packageMode)
	}
	if options.DependencyInstallation && packageMode != "package-install" {
		return nil, fmt.Errorf("%w: dependency installation requires package-install network mode", agentexecution.ErrBackendUnavailable)
	}
	if packageMode == "package-install" && !options.DependencyInstallation {
		return nil, fmt.Errorf("%w: package-install network mode requires dependency installation", agentexecution.ErrBackendUnavailable)
	}
	if packageMode == "package-install" && mode != "package-install" {
		return nil, fmt.Errorf("%w: package-install network mode must be the runtime network mode", agentexecution.ErrBackendUnavailable)
	}
	egressMode := strings.ToLower(strings.TrimSpace(options.EgressMode))
	if egressMode == "" {
		egressMode = EgressModeBridge
	}
	if egressMode != EgressModeBridge && egressMode != EgressModeDirect {
		return nil, fmt.Errorf("%w: egress mode %q is not available", agentexecution.ErrBackendUnavailable, egressMode)
	}
	if egressMode == EgressModeDirect && mode != "package-install" {
		return nil, fmt.Errorf("%w: direct egress mode requires package-install network mode", agentexecution.ErrBackendUnavailable)
	}
	if dependencyMode == DependencyPreparationModeControlledHTTP && mode != "controlled-http" {
		return nil, fmt.Errorf("%w: controlled-http dependency preparation requires controlled-http network mode", agentexecution.ErrBackendUnavailable)
	}
	var proxy *egressProxy
	proxyAddress := strings.TrimSpace(options.EgressProxyAddress)
	if mode == "controlled-http" || (mode == "package-install" && egressMode == EgressModeBridge) {
		if proxyAddress == "" {
			proxyAddress = defaultEgressProxyAddress
		}
		socketPath := strings.TrimSpace(options.EgressSocketPath)
		if socketPath == "" {
			socketPath = defaultEgressSocketPath
		}
		proxy, err = newEgressProxy(proxyAddress, socketPath)
		if err != nil {
			_ = root.Close()
			return nil, err
		}
		proxyAddress = proxy.proxyListener.Addr().String()
	} else if mode == "package-install" && egressMode == EgressModeDirect {
		if proxyAddress == "" {
			proxyAddress = defaultEgressProxyAddress
		}
		hosts := options.EgressAllowedHosts
		if len(hosts) == 0 {
			hosts = []string{"pypi.org", "files.pythonhosted.org", "registry.npmjs.org", "deb.debian.org", "security.debian.org"}
		}
		proxy, err = newDirectPackageProxy(proxyAddress, egressproxy.Policy{AllowedHosts: hosts})
		if err != nil {
			_ = root.Close()
			return nil, err
		}
		proxyAddress = proxy.proxyListener.Addr().String()
	} else if dependencyMode == DependencyPreparationModeControlledHTTP {
		_ = root.Close()
		return nil, fmt.Errorf("%w: controlled-http dependency preparation requires controlled-http network mode", agentexecution.ErrBackendUnavailable)
	} else {
		proxyAddress = ""
	}
	return &Backend{
		workspace: root, workspaceName: root.Name(), networkMode: mode,
		dependencyPreparation: options.DependencyPreparation, dependencyPreparationMode: dependencyMode,
		dependencyInstallation: options.DependencyInstallation, packageNetworkMode: packageMode,
		packageNetworkEnforced: packageMode == "package-install" && egressMode == EgressModeDirect,
		jobUID:                 options.JobUID, jobGID: options.JobGID,
		packageInstallTimeout: options.PackageInstallTimeout,
		maxPackageOutputBytes: options.MaxPackageOutputBytes,
		maxUploadBytes:        options.MaxUploadBytes, maxDownloadBytes: options.MaxDownloadBytes,
		egressProxyAddress: proxyAddress, egress: proxy,
		jobs:           make(map[string]trackedJob),
		packageSources: options.PackageSources.Resolved(), aptConfigPath: options.APTConfigPath, profileIdentity: options.ProfileIdentity,
	}, nil
}

func (b *Backend) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	if b.packageInstallDone != nil {
		close(b.packageInstallDone)
	}
	jobs := make([]agentexecution.RunningJob, 0, len(b.jobs))
	for _, job := range b.jobs {
		jobs = append(jobs, job.running)
	}
	b.mu.Unlock()
	for _, job := range jobs {
		_ = job.Cancel(context.Background())
		waitContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, _ = job.Wait(waitContext)
		cancel()
	}
	if b.egress != nil {
		_ = b.egress.Close()
	}
	b.packageInstalls.Wait()
	return b.workspace.Close()
}

func (b *Backend) Capabilities(context.Context, agentexecution.Allocation) (agentexecution.RuntimeCapabilities, error) {
	ledgerVersion := 0
	if b.dependencyInstallation {
		ledgerVersion = packageLedgerVersion
	}
	return agentexecution.RuntimeCapabilities{
		SystemPackageLedgerVersion: ledgerVersion,
		Shell:                      true, PTY: supportsPTY(), FileTransfer: true, DependencyPreparation: b.dependencyPreparation, DependencyPreparationMode: b.dependencyPreparationMode,
		DependencyInstallation: b.dependencyInstallation, PackageNetworkMode: b.packageNetworkMode,
		PackageNetworkEnforced: b.packageNetworkEnforced,
		NetworkMode:            b.networkMode, EgressProxyAddress: b.egressProxyAddress,
		MaxUploadBytes: b.maxUploadBytes, MaxDownloadBytes: b.maxDownloadBytes,
		PackageSourceDigest: b.packageSources.Digest(),
	}, nil
}

func (b *Backend) StartJob(ctx context.Context, _ agentexecution.Allocation, spec agentexecution.JobSpec, sink func(agentexecution.OutputChunk) error) (agentexecution.RunningJob, error) {
	if err := b.executionReady(); err != nil {
		return nil, err
	}
	if err := validateJobSpec(spec, sink); err != nil {
		return nil, err
	}
	// The root filesystem can remain read-only in gVisor while each session
	// still gets a writable HOME. Keeping it under /workspace makes the
	// existing authenticated checkpoint protocol retain it with work files.
	if err := b.workspace.MkdirAll(sessionHomeDirectory, 0o700); err != nil {
		return nil, fmt.Errorf("prepare session home: %w", err)
	}
	if b.dependencyInstallation {
		if err := b.replayPackageLedger(ctx); err != nil {
			return nil, fmt.Errorf("restore package installation ledger: %w", err)
		}
	}
	spec.Environment = cloneJobEnvironment(spec.Environment)
	home := filepath.Join(b.workspaceName, sessionHomeDirectory)
	spec.Environment["HOME"] = home
	if b.egress != nil && (b.networkMode == "controlled-http" || b.networkMode == "package-install") {
		// Force all supported HTTP clients through the loopback proxy. User
		// supplied proxy values are not allowed to bypass the worker policy.
		proxy := "http://" + b.egressProxyAddress
		spec.Environment["HTTP_PROXY"] = proxy
		spec.Environment["HTTPS_PROXY"] = proxy
		spec.Environment["ALL_PROXY"] = proxy
		spec.Environment["http_proxy"] = proxy
		spec.Environment["https_proxy"] = proxy
		spec.Environment["all_proxy"] = proxy
		spec.Environment["NO_PROXY"] = "127.0.0.1,localhost"
		spec.Environment["no_proxy"] = "127.0.0.1,localhost"
	} else if b.networkMode == "package-install" && b.packageNetworkEnforced {
		// The hosted provider enforces package-only egress outside the VM. Clear
		// caller-provided proxy variables so commands cannot redirect traffic to
		// an unapproved proxy inside the workspace.
		spec.Environment["HTTP_PROXY"] = ""
		spec.Environment["HTTPS_PROXY"] = ""
		spec.Environment["ALL_PROXY"] = ""
		spec.Environment["http_proxy"] = ""
		spec.Environment["https_proxy"] = ""
		spec.Environment["all_proxy"] = ""
		spec.Environment["NO_PROXY"] = "*"
		spec.Environment["no_proxy"] = "*"
	}
	if b.dependencyInstallation {
		// Keep package-manager state in the retained session workspace. The
		// runtime image remains immutable and ordinary commands still have no
		// network unless the package-install profile is selected.
		if err := prepareWorkspaceDirectoryOwnership(b.workspace, sessionHomeDirectory, b.jobUID, b.jobGID); err != nil {
			return nil, fmt.Errorf("prepare session home ownership: %w", err)
		}
		cacheKey, err := b.cacheNamespace(ctx)
		if err != nil {
			return nil, err
		}
		if err := prepareSessionPythonEnvironment(ctx, home, b.jobUID, b.jobGID); err != nil {
			return nil, fmt.Errorf("prepare session Python environment: %w", err)
		}
		venv := filepath.Join(home, ".venv")
		spec.Environment["VIRTUAL_ENV"] = venv
		spec.Environment["PIP_CONFIG_FILE"] = "/etc/zgi/pip.conf"
		spec.Environment["PIP_INDEX_URL"] = b.packageSources.PyPI
		spec.Environment["PIP_EXTRA_INDEX_URL"] = ""
		spec.Environment["PIP_TRUSTED_HOST"] = ""
		spec.Environment["NPM_CONFIG_REGISTRY"] = b.packageSources.NPM
		spec.Environment["npm_config_registry"] = b.packageSources.NPM
		// Standard runtime images ship matching Node headers in /usr/local.
		// Native builds must not fetch headers from an unapproved host.
		spec.Environment["NPM_CONFIG_NODEDIR"] = "/usr/local"
		spec.Environment["PIP_CACHE_DIR"] = filepath.Join(home, ".cache", "pip", cacheKey)
		spec.Environment["PIP_BREAK_SYSTEM_PACKAGES"] = "1"
		spec.Environment["NPM_CONFIG_CACHE"] = filepath.Join(home, ".cache", "npm", cacheKey)
		spec.Environment["npm_config_cache"] = spec.Environment["NPM_CONFIG_CACHE"]
		spec.Environment["NPM_CONFIG_GLOBAL"] = "false"
		spec.Environment["NPM_CONFIG_FUND"] = "false"
		spec.Environment["NPM_CONFIG_AUDIT"] = "false"
		basePath := strings.TrimSpace(spec.Environment["PATH"])
		if basePath == "" {
			basePath = strings.TrimSpace(os.Getenv("PATH"))
		}
		if basePath == "" {
			basePath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
		}
		spec.Environment["PATH"] = filepath.Join(venv, "bin") + string(os.PathListSeparator) + filepath.Join(home, ".local", "bin") + string(os.PathListSeparator) + filepath.Join(b.workspaceName, "node_modules", ".bin") + string(os.PathListSeparator) + basePath
	}
	workDir, err := b.resolveWorkDir(spec.WorkDir)
	if err != nil {
		return nil, err
	}
	hash, err := hashJobSpec(spec)
	if err != nil {
		return nil, err
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.unsafe != nil {
		return nil, agentexecution.ErrBackendUnavailable
	}
	if existing, ok := b.jobs[spec.JobID]; ok {
		if existing.specHash != hash {
			return nil, agentexecution.ErrIdempotencyConflict
		}
		return existing.running, nil
	}
	var outputMu sync.Mutex
	var sequence int64
	running, err := startProcess(ctx, workDir, spec, b.jobUID, b.jobGID, func(stream agentexecution.OutputStream, value []byte) error {
		outputMu.Lock()
		defer outputMu.Unlock()
		chunk := agentexecution.OutputChunk{
			JobID: spec.JobID, Sequence: sequence, Stream: stream,
			Data: append([]byte(nil), value...), CreatedAt: time.Now().UTC(),
		}
		sequence++
		return sink(chunk)
	}, b.quarantine)
	if err != nil {
		return nil, err
	}
	b.jobs[spec.JobID] = trackedJob{specHash: hash, running: running}
	return running, nil
}

func (b *Backend) executionReady() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.unsafe != nil {
		return fmt.Errorf("%w: execution environment requires recovery", agentexecution.ErrBackendUnavailable)
	}
	return nil
}

func (b *Backend) quarantine(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.unsafe == nil {
		b.unsafe = err
	}
}

func cloneJobEnvironment(values map[string]string) map[string]string {
	copyValues := make(map[string]string, len(values)+1)
	for key, value := range values {
		copyValues[key] = value
	}
	return copyValues
}

func validateJobSpec(spec agentexecution.JobSpec, sink func(agentexecution.OutputChunk) error) error {
	if strings.TrimSpace(spec.JobID) == "" || strings.TrimSpace(spec.Command) == "" || sink == nil {
		return fmt.Errorf("%w: job id, command, and output sink are required", agentexecution.ErrInvalidRequest)
	}
	if strings.ContainsRune(spec.Command, '\x00') {
		return fmt.Errorf("%w: command contains a null byte", agentexecution.ErrInvalidRequest)
	}
	for _, argument := range spec.Args {
		if strings.ContainsRune(argument, '\x00') {
			return fmt.Errorf("%w: argument contains a null byte", agentexecution.ErrInvalidRequest)
		}
	}
	for key, value := range spec.Environment {
		if strings.TrimSpace(key) == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("%w: environment contains an invalid entry", agentexecution.ErrInvalidRequest)
		}
	}
	if spec.PTY && (spec.PTYColumns == 0 || spec.PTYRows == 0) {
		return fmt.Errorf("%w: PTY dimensions are required", agentexecution.ErrInvalidRequest)
	}
	if spec.Timeout < 0 {
		return fmt.Errorf("%w: timeout cannot be negative", agentexecution.ErrInvalidRequest)
	}
	return nil
}

func (b *Backend) UploadFile(ctx context.Context, _ agentexecution.Allocation, filePath string, reader io.Reader, size int64) (agentexecution.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return agentexecution.FileInfo{}, err
	}
	clean, err := cleanRelativePath(filePath, false)
	if err != nil {
		return agentexecution.FileInfo{}, err
	}
	if reader == nil || size < 0 || size > b.maxUploadBytes {
		return agentexecution.FileInfo{}, agentexecution.ErrOutputLimit
	}
	parent := path.Dir(clean)
	if parent != "." {
		if err := b.workspace.MkdirAll(parent, 0o750); err != nil {
			return agentexecution.FileInfo{}, fmt.Errorf("prepare upload directory: %w", err)
		}
		if err := prepareWorkspaceDirectoryOwnership(b.workspace, parent, b.jobUID, b.jobGID); err != nil {
			return agentexecution.FileInfo{}, fmt.Errorf("prepare upload directory ownership: %w", err)
		}
	}
	temporary, err := temporaryName(clean)
	if err != nil {
		return agentexecution.FileInfo{}, err
	}
	file, err := b.workspace.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return agentexecution.FileInfo{}, fmt.Errorf("create upload: %w", err)
	}
	keep := false
	defer func() {
		_ = file.Close()
		if !keep {
			_ = b.workspace.Remove(temporary)
		}
	}()
	if err := prepareJobFileOwnership(file, b.jobUID, b.jobGID); err != nil {
		return agentexecution.FileInfo{}, fmt.Errorf("prepare upload ownership: %w", err)
	}
	written, copyErr := io.CopyN(file, reader, size)
	if copyErr != nil {
		return agentexecution.FileInfo{}, fmt.Errorf("write upload after %d bytes: %w", written, copyErr)
	}
	if err := file.Sync(); err != nil {
		return agentexecution.FileInfo{}, fmt.Errorf("sync upload: %w", err)
	}
	if err := file.Close(); err != nil {
		return agentexecution.FileInfo{}, fmt.Errorf("close upload: %w", err)
	}
	if err := b.workspace.Rename(temporary, clean); err != nil {
		return agentexecution.FileInfo{}, fmt.Errorf("commit upload: %w", err)
	}
	keep = true
	return b.fileInfo(clean)
}

func (b *Backend) DownloadFile(ctx context.Context, _ agentexecution.Allocation, filePath string) (io.ReadCloser, agentexecution.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, agentexecution.FileInfo{}, err
	}
	clean, err := cleanRelativePath(filePath, false)
	if err != nil {
		return nil, agentexecution.FileInfo{}, err
	}
	info, err := b.fileInfo(clean)
	if err != nil {
		return nil, agentexecution.FileInfo{}, err
	}
	if info.Size > b.maxDownloadBytes {
		return nil, agentexecution.FileInfo{}, agentexecution.ErrOutputLimit
	}
	file, err := b.workspace.Open(clean)
	if err != nil {
		return nil, agentexecution.FileInfo{}, fmt.Errorf("open download: %w", err)
	}
	return file, info, nil
}

func (b *Backend) fileInfo(filePath string) (agentexecution.FileInfo, error) {
	info, err := b.workspace.Stat(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return agentexecution.FileInfo{}, agentexecution.ErrNotFound
		}
		return agentexecution.FileInfo{}, err
	}
	if !info.Mode().IsRegular() {
		return agentexecution.FileInfo{}, fmt.Errorf("%w: workspace path is not a regular file", agentexecution.ErrInvalidPath)
	}
	return agentexecution.FileInfo{
		Path: filePath, Size: info.Size(), Mode: uint32(info.Mode().Perm()),
		ModifiedAt: info.ModTime().UTC(), ContentType: mime.TypeByExtension(path.Ext(filePath)),
	}, nil
}

func (b *Backend) resolveWorkDir(value string) (string, error) {
	if strings.TrimSpace(value) == "" || strings.TrimSpace(value) == "." {
		return b.workspaceName, nil
	}
	clean, err := cleanRelativePath(value, true)
	if err != nil {
		return "", err
	}
	info, err := b.workspace.Stat(clean)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w: work directory does not exist", agentexecution.ErrInvalidPath)
		}
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: work directory is not a directory", agentexecution.ErrInvalidPath)
	}
	return filepath.Join(b.workspaceName, filepath.FromSlash(clean)), nil
}

func cleanRelativePath(value string, allowDot bool) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || strings.ContainsRune(value, '\x00') || strings.HasPrefix(value, "/") || (len(value) >= 2 && value[1] == ':') {
		return "", agentexecution.ErrInvalidPath
	}
	clean := path.Clean(value)
	if clean == ".." || strings.HasPrefix(clean, "../") || (!allowDot && clean == ".") {
		return "", agentexecution.ErrInvalidPath
	}
	return clean, nil
}

func temporaryName(target string) (string, error) {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return path.Join(path.Dir(target), "."+path.Base(target)+".upload-"+hex.EncodeToString(value[:])), nil
}

func hashJobSpec(spec agentexecution.JobSpec) (string, error) {
	payload, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	var value [32]byte
	value = sha256Sum(payload)
	return hex.EncodeToString(value[:]), nil
}

func sha256Sum(value []byte) [32]byte {
	return sha256.Sum256(value)
}
