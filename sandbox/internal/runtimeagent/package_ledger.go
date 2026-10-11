package runtimeagent

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

type packageLedger struct {
	Version     int                      `json:"version,omitempty"`
	Environment packageLedgerEnvironment `json:"environment,omitempty"`
	Packages    []systemPackage          `json:"packages,omitempty"`
	Entries     []packageLedgerEntry     `json:"entries"`
}

type packageLedgerEntry struct {
	Args      []string  `json:"args"`
	Installed time.Time `json:"installed_at"`
}

const packageRecoveryMessage = "system package recovery failed; verify the retained ledger, original profile and sources, and locked package availability before retrying; no latest-version fallback"

// Preserve the detailed cause locally, but only a fixed safe diagnostic crosses
// the authenticated protocol. APT output and internal paths are not error text.
type packageRecoveryError struct{ cause error }

func (e *packageRecoveryError) Error() string {
	return fmt.Sprintf("%s: %v", packageRecoveryMessage, e.cause)
}
func (e *packageRecoveryError) Unwrap() error { return e.cause }

func (b *Backend) recordPackageLedger(ctx context.Context, args []string) error {
	b.packageLedgerMu.Lock()
	defer b.packageLedgerMu.Unlock()
	ledger, err := b.readPackageLedger()
	if err != nil {
		return err
	}
	if err := validateAPTArgs(args); err != nil {
		return err
	}
	if args[0] == "install" {
		packages, err := b.installedSystemPackages(ctx)
		if err != nil {
			return err
		}
		if len(packages) == 0 {
			return errors.New("successful system installation has no installed package inventory")
		}
		ledger.Packages = packages
	}
	// Do not silently upgrade an unlocked historical ledger on download alone.
	if ledger.Version != 0 || len(ledger.Entries) == 0 || args[0] == "install" {
		ledger.Version = packageLedgerVersion
		ledger.Environment = b.packageLedgerEnvironment()
	}
	found := false
	for _, entry := range ledger.Entries {
		if slices.Equal(entry.Args, args) {
			found = true
			break
		}
	}
	if !found && len(ledger.Entries) >= maxPackageLedgerEntries {
		return agentexecution.ErrOutputLimit
	}
	if !found {
		copyArgs := append([]string(nil), args...)
		ledger.Entries = append(ledger.Entries, packageLedgerEntry{Args: copyArgs, Installed: time.Now().UTC()})
	}
	return b.writePackageLedger(ledger)
}

func (b *Backend) writePackageLedger(ledger packageLedger) error {
	if err := b.validatePackageLedger(ledger); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		return err
	}
	if len(raw) >= maxPackageLedgerBytes {
		return agentexecution.ErrOutputLimit
	}
	tmp := sessionHomeDirectory + "/.package-ledger-" + rand.Text()
	file, err := b.workspace.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer b.workspace.Remove(tmp)
	if _, err := file.Write(append(raw, '\n')); err != nil {
		_ = file.Close()
		return err
	}
	if err := errors.Join(file.Sync(), file.Close()); err != nil {
		return err
	}
	return b.workspace.Rename(tmp, packageLedgerName)
}

func (b *Backend) replayPackageLedger(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			err = &packageRecoveryError{cause: err}
		}
	}()
	ctx, release, err := b.acquirePackageInstallation(ctx)
	if err != nil {
		return err
	}
	defer release()
	b.packageLedgerMu.Lock()
	defer b.packageLedgerMu.Unlock()
	if b.packageReplayComplete {
		return nil
	}
	ledger, err := b.readPackageLedger()
	if err != nil {
		return err
	}
	if ledger.Version == packageLedgerVersion {
		if err := b.restoreSystemPackageVersions(ctx, ledger.Packages); err != nil {
			return err
		}
	} else if len(ledger.Entries) != 0 {
		// Historical commands have no resolved versions. Replay once under the
		// original rules, then capture what actually exists; never invent pins.
		installed := false
		for _, entry := range ledger.Entries {
			if entry.Args[0] != "install" {
				continue // Download is not an installed dependency.
			}
			result := b.runAPTWithMetadataRetry(ctx, entry.Args)
			if result.ExitCode != 0 {
				return fmt.Errorf("replay legacy apt installation failed: %s: %s", result.Error, result.Stderr)
			}
			installed = true
		}
		if installed {
			ledger.Packages, err = b.installedSystemPackages(ctx)
			if err != nil {
				return err
			}
			if len(ledger.Packages) == 0 {
				return errors.New("legacy system installation has no installed package inventory")
			}
		}
		ledger.Version, ledger.Environment = packageLedgerVersion, b.packageLedgerEnvironment()
		if err := b.writePackageLedger(ledger); err != nil {
			return err
		}
	}
	// Failed or canceled replays remain retryable and cannot silently turn
	// into success on the next Job, as they would with sync.Once.
	b.packageReplayComplete = true
	return nil
}

func (b *Backend) readPackageLedger() (packageLedger, error) {
	var ledger packageLedger
	info, err := b.workspace.Lstat(packageLedgerName)
	if errors.Is(err, os.ErrNotExist) {
		return ledger, nil
	}
	if err != nil {
		return ledger, err
	}
	if !info.Mode().IsRegular() {
		return ledger, errors.New("package installation ledger must be a regular file")
	}
	file, err := openPackageLedgerFile(b.workspace)
	if err != nil {
		return ledger, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return ledger, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return ledger, errors.New("package installation ledger changed during open")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxPackageLedgerBytes+1))
	if err != nil {
		return ledger, err
	}
	if len(raw) > maxPackageLedgerBytes {
		return ledger, agentexecution.ErrOutputLimit
	}
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' {
		return ledger, errors.New("package installation ledger must be a JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&ledger); err != nil {
		return ledger, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return ledger, errors.New("package installation ledger must contain one JSON object")
	}
	return ledger, b.validatePackageLedger(ledger)
}
