package runtimeagent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

func packageTestBackend(t *testing.T) *Backend {
	t.Helper()
	backend, err := New(Options{WorkspaceRoot: t.TempDir(), NetworkMode: "disabled"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := backend.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := backend.workspace.MkdirAll(sessionHomeDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	return backend
}

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
	backend := packageTestBackend(t)
	ledger := filepath.Join(backend.workspaceName, packageLedgerName)
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"entries":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, ledger); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := backend.recordPackageLedger([]string{"install", "poppler-utils"}); err == nil {
		t.Fatal("package ledger symlink was accepted")
	}
}

func TestFailedPackageReplayIsNotSilentlyAcceptedAndCanRetry(t *testing.T) {
	backend := packageTestBackend(t)
	ledger := filepath.Join(backend.workspaceName, packageLedgerName)
	if err := os.WriteFile(ledger, []byte(`{"entries":[{"args":["upgrade"]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
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

func TestPackageOutputBufferBoundsMemoryWhileDraining(t *testing.T) {
	buffer := packageOutputBuffer{maximum: 64}
	chunk := []byte(strings.Repeat("x", 1024))
	for range 1000 {
		if n, err := buffer.Write(chunk); n != len(chunk) || err != nil {
			t.Fatalf("write: n=%d error=%v", n, err)
		}
	}
	if buffer.buffer.Len() != 64 || !strings.HasSuffix(buffer.String(), "[output truncated]") {
		t.Fatalf("buffer length=%d truncated=%v", buffer.buffer.Len(), buffer.truncated)
	}
}

func TestPackageQueueWaitIsCancelableAndCloseJoinsOwner(t *testing.T) {
	backend := packageTestBackend(t)
	owner, release, err := backend.acquirePackageInstallation(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	waitCtx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := backend.acquirePackageInstallation(waitCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled waiter: %v", err)
	}
	closed := make(chan error, 1)
	go func() { closed <- backend.Close() }()
	select {
	case <-owner.Done():
	case <-time.After(time.Second):
		t.Fatal("backend close did not cancel package owner")
	}
	select {
	case <-closed:
		t.Fatal("backend closed workspace before installation released it")
	default:
	}
	release()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("backend close did not join installation")
	}
	if _, _, err := backend.acquirePackageInstallation(t.Context()); !errors.Is(err, agentexecution.ErrBackendUnavailable) {
		t.Fatalf("closed queue accepted new owner: %v", err)
	}
}

func TestPackageLedgerIsBoundedAtomicAndDeduplicated(t *testing.T) {
	backend := packageTestBackend(t)
	for range 2 {
		if err := backend.recordPackageLedger([]string{"install", "tree"}); err != nil {
			t.Fatal(err)
		}
	}
	ledger, err := backend.readPackageLedger()
	if err != nil || len(ledger.Entries) != 1 {
		t.Fatalf("ledger=%v error=%v", ledger, err)
	}
	if err := backend.workspace.WriteFile(packageLedgerName, []byte(strings.Repeat("x", maxPackageLedgerBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.readPackageLedger(); !errors.Is(err, agentexecution.ErrOutputLimit) {
		t.Fatalf("oversized ledger: %v", err)
	}
}

func TestPackageLedgerParentCannotEscapeWorkspace(t *testing.T) {
	backend := packageTestBackend(t)
	if err := backend.workspace.Remove(sessionHomeDirectory); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(backend.workspaceName, sessionHomeDirectory)); err != nil {
		t.Skipf("symlink: %v", err)
	}
	if err := backend.recordPackageLedger([]string{"install", "tree"}); err == nil {
		t.Fatal("ledger parent escaped workspace")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("outside directory mutated: entries=%v error=%v", entries, err)
	}
}
