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
const languageRecoveryMessage = "retained Python or Node dependency recovery failed; verify the original runtime profile and retained artifact integrity before retrying; no network or latest-version fallback"

type languageRecoveryError struct{ cause error }

func (e *languageRecoveryError) Error() string { return languageRecoveryMessage }
func (e *languageRecoveryError) Unwrap() error { return e.cause }

type languagePackageLedger struct {
	Version         int                      `json:"version"`
	Environment     packageLedgerEnvironment `json:"environment"`
	Packages        []languagePackage        `json:"packages"`
	Artifacts       []dependencyArtifact     `json:"artifacts,omitempty"`
	UncachedTargets []string                 `json:"uncached_targets,omitempty"`
	CacheKey        string                   `json:"runtime_cache_key,omitempty"`
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
	if (ledger.Version != 1 && ledger.Version != agentexecution.LanguagePackageLedgerVersion) || ledger.Environment != b.packageLedgerEnvironment() {
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
	if ledger.Version == 1 && (len(ledger.Artifacts) != 0 || len(ledger.UncachedTargets) != 0 || ledger.CacheKey != "") {
		return ledger, true, errors.New("historical ledger cannot contain dependency artifacts")
	}
	if ledger.Version == agentexecution.LanguagePackageLedgerVersion {
		if b.profileIdentity != "" && (len(ledger.CacheKey) != 64 || !lowercaseDigest.MatchString(ledger.CacheKey)) {
			return ledger, true, errors.New("dependency artifact ABI identity is missing or invalid")
		}
		if err := b.validateDependencyArtifacts(ledger); err != nil {
			return ledger, true, err
		}
	}
	return ledger, true, nil
}

// The first command after reconstruction must see the same retained inventory.
// Thereafter ordinary Jobs may intentionally install, upgrade or remove packages;
// the next quiescent checkpoint records the resulting state, not stale pins.
func (b *Backend) verifyLanguageLedger(ctx context.Context) error {
	ctx, release, err := b.acquirePackageInstallation(ctx)
	if err != nil {
		return &languageRecoveryError{cause: err}
	}
	defer release()
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
		if ledger.Version == agentexecution.LanguagePackageLedgerVersion {
			key, err := b.cacheNamespace(ctx)
			if err != nil {
				return err
			}
			if key != ledger.CacheKey {
				return errors.New("dependency artifact ABI or cache identity mismatch")
			}
			if err := b.restoreDependencyArtifacts(ctx, ledger.Artifacts); err != nil {
				return err
			}
		}
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
	ctx, release, err := b.acquirePackageInstallation(ctx)
	if err != nil {
		return err
	}
	defer release()
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
	previous, _, err := b.readLanguageLedger()
	if err != nil {
		return err
	}
	// Checkpointing can precede the first Job. The ABI/cache marker is stored
	// under HOME too, so create the confined directory before probing the cache.
	if err := b.workspace.MkdirAll(sessionHomeDirectory, 0o700); err != nil {
		return err
	}
	key, err := b.cacheNamespace(ctx)
	if err != nil {
		return err
	}
	artifacts, uncached, err := b.captureDependencyArtifacts(ctx, packages)
	committed := false
	defer func() {
		if !committed {
			b.removeUnreferencedDependencyArtifacts(artifacts, previous.Artifacts)
		}
	}()
	if err != nil {
		return err
	}
	ledger := languagePackageLedger{Version: agentexecution.LanguagePackageLedgerVersion, Environment: b.packageLedgerEnvironment(), Packages: packages}
	ledger.Artifacts = artifacts
	ledger.UncachedTargets = uncached
	ledger.CacheKey = key
	raw, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		return err
	}
	if len(raw) >= maxPackageLedgerBytes {
		return agentexecution.ErrOutputLimit
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
	if err := b.workspace.Rename(tmp, languageLedgerName); err != nil {
		return err
	}
	committed = true
	b.removeUnreferencedDependencyArtifacts(previous.Artifacts, artifacts)
	return nil
}

// Only exact previously-owned content-addressed files are removed, never a
// directory glob or another session's cache. Old durable checkpoints keep copies.
func (b *Backend) removeUnreferencedDependencyArtifacts(old, current []dependencyArtifact) {
	for _, artifact := range old {
		if !slices.ContainsFunc(current, func(value dependencyArtifact) bool { return value.SHA256 == artifact.SHA256 }) {
			_ = b.workspace.Remove(artifact.name())
		}
	}
}
