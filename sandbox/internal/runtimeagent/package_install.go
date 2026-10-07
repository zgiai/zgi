package runtimeagent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

// PackageInstallResult is deliberately small and is only used by the
// in-sandbox apt facade. Python and Node package managers remain ordinary
// user-space commands executed by StartJob.
type PackageInstallResult struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	Error    string `json:"error,omitempty"`
}

var aptPackageName = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]{0,127}$`)

const (
	defaultPackageInstallTimeout = 10 * time.Minute
	defaultPackageOutputBytes    = 256 * 1024
	maxPackageLedgerBytes        = 1024 * 1024
	maxPackageLedgerEntries      = 1000
	packageLedgerName            = sessionHomeDirectory + "/package-install-ledger.json"
)

// Each backend owns one cancelable installation queue. Close prevents new
// owners, cancels the active operation, and joins it before closing os.Root.
func (b *Backend) acquirePackageInstallation(ctx context.Context) (context.Context, func(), error) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, nil, agentexecution.ErrBackendUnavailable
	}
	if b.packageInstallGate == nil {
		b.packageInstallGate = make(chan struct{}, 1)
		b.packageInstallDone = make(chan struct{})
	}
	gate, closed := b.packageInstallGate, b.packageInstallDone
	b.mu.Unlock()
	select {
	case gate <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-closed:
		return nil, nil, agentexecution.ErrBackendUnavailable
	}
	b.mu.Lock()
	if b.closed || ctx.Err() != nil {
		b.mu.Unlock()
		<-gate
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		return nil, nil, agentexecution.ErrBackendUnavailable
	}
	b.packageInstalls.Add(1)
	b.mu.Unlock()
	operationCtx, cancel := context.WithCancel(ctx)
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		select {
		case <-closed:
			cancel()
		case <-operationCtx.Done():
		}
	}()
	return operationCtx, func() {
		cancel()
		<-watchDone
		<-gate
		b.packageInstalls.Done()
	}, nil
}

// InstallSystemPackages runs the real apt binary as the Runtime Agent's root
// process. The Job itself never receives root or a writable control socket;
// the wrapper exposes only this validated command subset.
func (b *Backend) InstallSystemPackages(ctx context.Context, args []string) (PackageInstallResult, error) {
	if err := b.executionReady(); err != nil {
		return PackageInstallResult{}, err
	}
	if !b.dependencyInstallation || b.networkMode != "package-install" {
		return PackageInstallResult{}, fmt.Errorf("%w: system package installation is not enabled", agentexecution.ErrBackendUnavailable)
	}
	if err := validateAPTArgs(args); err != nil {
		return PackageInstallResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return PackageInstallResult{}, err
	}
	ctx, release, err := b.acquirePackageInstallation(ctx)
	if err != nil {
		return PackageInstallResult{}, err
	}
	defer release()
	// A retained ledger is untrusted. Reject invalid identity or commands before
	// allowing a new privileged installation to mutate this environment.
	if _, err := b.readPackageLedger(); err != nil {
		return PackageInstallResult{}, &packageRecoveryError{cause: err}
	}
	result := b.runAPTWithMetadataRetry(ctx, args)
	if result.ExitCode == 0 && (args[0] == "install" || args[0] == "download") {
		if err := b.recordPackageLedger(ctx, args); err != nil {
			return result, &packageRecoveryError{cause: fmt.Errorf("installation completed but its ledger could not be recorded: %w", err)}
		}
	}
	return result, nil
}

func (b *Backend) aptConfig() string {
	if b.aptConfigPath != "" {
		return b.aptConfigPath
	}
	return "/etc/zgi/apt.conf"
}

// Minimal runtime images can omit APT lists. Retry only the pre-install
// missing-package case, not a partially executed package configuration.
func (b *Backend) runAPTWithMetadataRetry(ctx context.Context, args []string) PackageInstallResult {
	result := b.runAPT(ctx, args)
	if len(args) == 0 || (args[0] != "install" && args[0] != "download") || result.ExitCode != 100 || !strings.Contains(result.Stderr, "Unable to locate package") {
		return result
	}
	update := b.runAPT(ctx, []string{"update"})
	if update.ExitCode != 0 {
		update.Stderr = limitPackageOutput(result.Stderr + "\nAPT metadata refresh failed:\n" + update.Stderr)
		return update
	}
	result = b.runAPT(ctx, args)
	result.Stdout = limitPackageOutput("APT metadata refreshed before retry:\n" + update.Stdout + "\n" + result.Stdout)
	return result
}

func (b *Backend) runAPT(ctx context.Context, args []string) PackageInstallResult {
	timeout := b.packageInstallTimeout
	if timeout <= 0 {
		timeout = defaultPackageInstallTimeout
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	aptPath := strings.TrimSpace(os.Getenv("ZGI_RUNTIME_AGENT_APT_PATH"))
	if aptPath == "" {
		aptPath = "/usr/lib/zgi/apt-get.real"
	}
	command := exec.CommandContext(commandCtx, aptPath, args...)
	configurePackageProcess(command)
	command.Env = append(filteredEnvironment(os.Environ(), proxyEnvironmentKeys...),
		"APT_CONFIG="+b.aptConfig(),
		"DEBIAN_FRONTEND=noninteractive",
		"APT_LISTBUGS_FRONTEND=none",
		"APT_LISTCHANGES_FRONTEND=none",
	)
	if b.egressProxyAddress != "" {
		command.Env = append(command.Env,
			"HTTP_PROXY=http://"+b.egressProxyAddress,
			"HTTPS_PROXY=http://"+b.egressProxyAddress,
			"ALL_PROXY=http://"+b.egressProxyAddress,
			"http_proxy=http://"+b.egressProxyAddress,
			"https_proxy=http://"+b.egressProxyAddress,
			"all_proxy=http://"+b.egressProxyAddress,
		)
	} else {
		command.Env = append(command.Env,
			"HTTP_PROXY=", "HTTPS_PROXY=", "ALL_PROXY=",
			"http_proxy=", "https_proxy=", "all_proxy=", "NO_PROXY=*", "no_proxy=*",
		)
	}
	maximum := b.maxPackageOutputBytes
	if maximum <= 0 {
		maximum = defaultPackageOutputBytes
	}
	stdout, stderr := packageOutputBuffer{maximum: maximum}, packageOutputBuffer{maximum: maximum}
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	result := PackageInstallResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if commandCtx.Err() != nil {
		result.ExitCode = -1
		if errors.Is(commandCtx.Err(), context.DeadlineExceeded) {
			result.Error = "package installation timed out"
		} else {
			result.Error = "package installation canceled"
		}
		return result
	}
	if err == nil {
		return result
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		result.Error = "apt-get exited with a non-zero status"
		return result
	}
	result.ExitCode = -1
	result.Error = err.Error()
	return result
}

var proxyEnvironmentKeys = []string{
	"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY",
	"http_proxy", "https_proxy", "all_proxy", "no_proxy",
}

func filteredEnvironment(values []string, keys ...string) []string {
	blocked := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		blocked[key] = struct{}{}
	}
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		key, _, ok := strings.Cut(value, "=")
		if ok {
			if _, found := blocked[key]; found {
				continue
			}
		}
		filtered = append(filtered, value)
	}
	return filtered
}

// Truncate while draining, not after an unbounded bytes.Buffer has filled.
// exec.Cmd owns one copying goroutine per stream; String is read after Wait.
type packageOutputBuffer struct {
	buffer    bytes.Buffer
	maximum   int64
	truncated bool
}

func (b *packageOutputBuffer) Write(value []byte) (int, error) {
	remaining := max(int64(0), b.maximum-int64(b.buffer.Len()))
	keep := min(int64(len(value)), remaining)
	_, _ = b.buffer.Write(value[:keep])
	if keep < int64(len(value)) {
		b.truncated = true
	}
	return len(value), nil
}

func (b *packageOutputBuffer) String() string {
	if b.truncated {
		return b.buffer.String() + "\n[output truncated]"
	}
	return b.buffer.String()
}

func validateAPTArgs(args []string) error {
	if len(args) > 128 {
		return fmt.Errorf("%w: too many apt arguments", agentexecution.ErrInvalidRequest)
	}
	if len(args) == 0 || (args[0] != "update" && args[0] != "install" && args[0] != "download") {
		return fmt.Errorf("%w: only apt update, install, or download is permitted", agentexecution.ErrInvalidRequest)
	}
	command := args[0]
	seenPackage := false
	for _, arg := range args[1:] {
		switch arg {
		case "-y", "--yes", "--no-install-recommends":
			continue
		}
		if strings.HasPrefix(arg, "-") || strings.ContainsAny(arg, ":/@") || !aptPackageName.MatchString(arg) {
			return fmt.Errorf("%w: apt argument %q is not allowed", agentexecution.ErrInvalidRequest, arg)
		}
		seenPackage = true
	}
	if (command == "install" || command == "download") && !seenPackage {
		return fmt.Errorf("%w: apt %s requires at least one package", agentexecution.ErrInvalidRequest, command)
	}
	return nil
}

func limitPackageOutput(value string) string {
	const limit = 256 * 1024
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "\n[output truncated]"
}
