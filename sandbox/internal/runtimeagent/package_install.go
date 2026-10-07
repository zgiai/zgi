package runtimeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

type packageLedger struct {
	Entries []packageLedgerEntry `json:"entries"`
}

type packageLedgerEntry struct {
	Args      []string  `json:"args"`
	Installed time.Time `json:"installed_at"`
}

// InstallSystemPackages runs the real apt binary as the Runtime Agent's root
// process. The Job itself never receives root or a writable control socket;
// the wrapper exposes only this validated command subset.
func (b *Backend) InstallSystemPackages(ctx context.Context, args []string) (PackageInstallResult, error) {
	if !b.dependencyInstallation || b.networkMode != "package-install" {
		return PackageInstallResult{}, fmt.Errorf("%w: system package installation is not enabled", agentexecution.ErrBackendUnavailable)
	}
	if err := validateAPTArgs(args); err != nil {
		return PackageInstallResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return PackageInstallResult{}, err
	}
	result := b.runAPT(ctx, args)
	if result.ExitCode == 0 && (args[0] == "install" || args[0] == "download") {
		if err := b.recordPackageLedger(args); err != nil {
			return result, fmt.Errorf("record package installation ledger: %w", err)
		}
	}
	return result, nil
}

func (b *Backend) runAPT(ctx context.Context, args []string) PackageInstallResult {
	commandCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	aptPath := strings.TrimSpace(os.Getenv("ZGI_RUNTIME_AGENT_APT_PATH"))
	if aptPath == "" {
		aptPath = "/usr/lib/zgi/apt-get.real"
	}
	command := exec.CommandContext(commandCtx, aptPath, args...)
	command.Env = append(filteredEnvironment(os.Environ(), proxyEnvironmentKeys...),
		"APT_CONFIG=/etc/zgi/apt.conf",
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
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	result := PackageInstallResult{Stdout: limitPackageOutput(stdout.String()), Stderr: limitPackageOutput(stderr.String())}
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

func (b *Backend) recordPackageLedger(args []string) error {
	b.packageLedgerMu.Lock()
	defer b.packageLedgerMu.Unlock()
	ledger := packageLedger{}
	if info, err := os.Lstat(b.packageLedgerPath); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return errors.New("package installation ledger cannot be a symbolic link")
	}
	if raw, err := os.ReadFile(b.packageLedgerPath); err == nil {
		if err := json.Unmarshal(raw, &ledger); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	copyArgs := append([]string(nil), args...)
	ledger.Entries = append(ledger.Entries, packageLedgerEntry{Args: copyArgs, Installed: time.Now().UTC()})
	raw, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		return err
	}
	tmp := b.packageLedgerPath + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(raw, '\n')); err != nil {
		_ = file.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, b.packageLedgerPath)
}

func (b *Backend) replayPackageLedger(ctx context.Context) error {
	b.packageLedgerMu.Lock()
	defer b.packageLedgerMu.Unlock()
	if b.packageReplayComplete {
		return nil
	}
	if info, err := os.Lstat(b.packageLedgerPath); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return errors.New("package installation ledger cannot be a symbolic link")
	}
	raw, err := os.ReadFile(filepath.Clean(b.packageLedgerPath))
	if errors.Is(err, os.ErrNotExist) {
		b.packageReplayComplete = true
		return nil
	}
	if err != nil {
		return err
	}
	var ledger packageLedger
	if err := json.Unmarshal(raw, &ledger); err != nil {
		return err
	}
	for _, entry := range ledger.Entries {
		if err := validateAPTArgs(entry.Args); err != nil {
			return err
		}
		result := b.runAPT(ctx, entry.Args)
		if result.ExitCode != 0 {
			return fmt.Errorf("replay apt command %v failed: %s", entry.Args, result.Stderr)
		}
	}
	// Failed or canceled replays remain retryable and cannot silently turn
	// into success on the next Job, as they would with sync.Once.
	b.packageReplayComplete = true
	return nil
}

func validateAPTArgs(args []string) error {
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
