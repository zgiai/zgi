package runtimeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

const languageLedgerName = sessionHomeDirectory + "/language-package-ledger.json"
const languageRecoveryMessage = "retained Python or Node dependency verification failed; restore the complete checkpoint with its original runtime profile before retrying; no automatic reinstall or latest-version fallback"

type languageRecoveryError struct{ cause error }

func (e *languageRecoveryError) Error() string { return languageRecoveryMessage }
func (e *languageRecoveryError) Unwrap() error { return e.cause }

type languagePackageLedger struct {
	Version     int                      `json:"version"`
	Environment packageLedgerEnvironment `json:"environment"`
	Packages    []languagePackage        `json:"packages"`
}

func decodeLanguageLedger(raw []byte, ledger *languagePackageLedger) error {
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("language package ledger must be a JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(ledger); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("language package ledger must contain one object")
	}
	return nil
}

func (b *Backend) readLanguageLedger() (languagePackageLedger, bool, error) {
	var ledger languagePackageLedger
	raw, err := readRegularWorkspaceFile(b.workspace, languageLedgerName, maxPackageLedgerBytes)
	if errors.Is(err, os.ErrNotExist) {
		return ledger, false, nil
	}
	if err != nil {
		return ledger, true, err
	}
	if err := decodeLanguageLedger(raw, &ledger); err != nil {
		return ledger, true, err
	}
	if ledger.Version != agentexecution.LanguagePackageLedgerVersion || ledger.Environment != b.packageLedgerEnvironment() {
		return ledger, true, errors.New("language package ledger version or environment mismatch")
	}
	if len(ledger.Packages) > maxLanguagePackages {
		return ledger, true, agentexecution.ErrOutputLimit
	}
	seen := make(map[string]bool, len(ledger.Packages))
	for _, pkg := range ledger.Packages {
		if !pkg.valid() || seen[pkg.Path] {
			return ledger, true, errors.New("invalid or duplicate language package lock")
		}
		seen[pkg.Path] = true
	}
	return ledger, true, nil
}

// The first command after reconstruction must see the same retained inventory.
// Thereafter ordinary Jobs may intentionally install, upgrade or remove packages;
// the next quiescent checkpoint records the resulting state, not stale pins.
func (b *Backend) verifyLanguageLedger(ctx context.Context) error {
	b.languageLedgerMu.Lock()
	defer b.languageLedgerMu.Unlock()
	return b.verifyLanguageLedgerLocked(ctx)
}

func (b *Backend) verifyLanguageLedgerLocked(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			err = &languageRecoveryError{cause: err}
		}
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	if b.languageLedgerVerified {
		return nil
	}
	ledger, exists, err := b.readLanguageLedger()
	if err != nil {
		return err
	}
	if exists {
		actual, err := b.installedLanguagePackages(ctx)
		if err != nil {
			return err
		}
		// Sort expected records: older encoders need not use our traversal order.
		slices.SortFunc(ledger.Packages, func(a, c languagePackage) int { return strings.Compare(a.Path, c.Path) })
		if !slices.Equal(ledger.Packages, actual) {
			return errors.New("retained language package inventory mismatch")
		}
	}
	b.languageLedgerVerified = true
	return nil
}

func (b *Backend) captureLanguageLedger(ctx context.Context) error {
	b.languageLedgerMu.Lock()
	defer b.languageLedgerMu.Unlock()
	// Do not let checkpointing bless a damaged imported environment before its
	// first execution. Historical checkpoints without a ledger remain readable.
	if err := b.verifyLanguageLedgerLocked(ctx); err != nil {
		return err
	}
	packages, err := b.installedLanguagePackages(ctx)
	if err != nil {
		return err
	}
	ledger := languagePackageLedger{Version: agentexecution.LanguagePackageLedgerVersion, Environment: b.packageLedgerEnvironment(), Packages: packages}
	raw, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		return err
	}
	if len(raw) >= maxPackageLedgerBytes {
		return agentexecution.ErrOutputLimit
	}
	if err := b.workspace.MkdirAll(sessionHomeDirectory, 0o700); err != nil {
		return err
	}
	tmp, err := temporaryName(languageLedgerName)
	if err != nil {
		return err
	}
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
	return b.workspace.Rename(tmp, languageLedgerName)
}
