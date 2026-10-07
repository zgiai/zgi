package runtimeagent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateAPTArgs(t *testing.T) {
	for _, args := range [][]string{
		{"update"},
		{"install", "-y", "--no-install-recommends", "poppler-utils", "ffmpeg"},
		{"download", "libreoffice"},
	} {
		if err := validateAPTArgs(args); err != nil {
			t.Errorf("validateAPTArgs(%v): %v", args, err)
		}
	}
	for _, args := range [][]string{
		{"upgrade"},
		{"install"},
		{"install", "https://example.invalid/a.deb"},
		{"install", "git+https://example.invalid/repo"},
		{"install", "-o", "Dir=/"},
	} {
		if err := validateAPTArgs(args); err == nil {
			t.Errorf("validateAPTArgs(%v) accepted an unsafe request", args)
		}
	}
}

func TestPackageLedgerRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	ledger := filepath.Join(root, "ledger.json")
	outside := filepath.Join(root, "outside.json")
	if err := os.WriteFile(outside, []byte(`{"entries":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, ledger); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	backend := &Backend{packageLedgerPath: ledger}
	if err := backend.recordPackageLedger([]string{"install", "poppler-utils"}); err == nil {
		t.Fatal("package ledger symlink was accepted")
	}
}

func TestFailedPackageReplayIsNotSilentlyAcceptedAndCanRetry(t *testing.T) {
	ledger := filepath.Join(t.TempDir(), "ledger.json")
	if err := os.WriteFile(ledger, []byte(`{"entries":[{"args":["upgrade"]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := &Backend{packageLedgerPath: ledger}
	for attempt := 0; attempt < 2; attempt++ {
		if err := backend.replayPackageLedger(t.Context()); err == nil {
			t.Fatalf("invalid replay attempt %d reported success", attempt)
		}
	}
	if err := os.WriteFile(ledger, []byte(`{"entries":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := backend.replayPackageLedger(t.Context()); err != nil {
		t.Fatalf("retry repaired ledger: %v", err)
	}
}
