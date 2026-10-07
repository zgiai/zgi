//go:build linux

package runtimeagent

import (
	"errors"
	"os"
	"path"
	"syscall"
	"testing"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

func TestLanguageMetadataRefusesLinksAndFIFO(t *testing.T) {
	for _, kind := range []string{"symlink", "fifo", "missing"} {
		t.Run(kind, func(t *testing.T) {
			b := packageTestBackend(t)
			if err := b.workspace.MkdirAll(path.Dir(testNodeMetadata), 0o750); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "symlink":
				writeLanguageFixture(t, b, "metadata.json", `{"name":"bufferutil","version":"4.0.9"}`)
				if err := b.workspace.Symlink("../../metadata.json", testNodeMetadata); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err := syscall.Mkfifo(b.workspaceName+"/"+testNodeMetadata, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := b.installedLanguagePackages(t.Context()); err == nil {
				t.Fatalf("unsafe %s metadata accepted", kind)
			}
		})
	}
}

func TestLanguageInventorySupportsConfinedNodePackageLinks(t *testing.T) {
	b := packageTestBackend(t)
	writeLanguageFixture(t, b, "node_modules/.pnpm/bufferutil@4.0.9/node_modules/bufferutil/package.json", `{"name":"bufferutil","version":"4.0.9"}`)
	if err := b.workspace.Symlink(".pnpm/bufferutil@4.0.9/node_modules/bufferutil", "node_modules/bufferutil"); err != nil {
		t.Fatal(err)
	}
	packages, err := b.installedLanguagePackages(t.Context())
	if err != nil || len(packages) != 2 {
		t.Fatalf("linked inventory=%+v err=%v", packages, err)
	}
	if err := b.workspace.Remove("node_modules/bufferutil"); err != nil {
		t.Fatal(err)
	}
	if err := b.workspace.Symlink("/etc", "node_modules/bufferutil"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.installedLanguagePackages(t.Context()); err == nil {
		t.Fatal("escaping linked package accepted")
	}
}

func TestLanguageLedgerBoundsAndMissingPackageBlockUserCommand(t *testing.T) {
	b := packageTestBackend(t)
	b.dependencyInstallation = true
	writeLanguageFixture(t, b, testPythonMetadata, "Name: ujson\nVersion: 5.10.0\n")
	if err := b.captureLanguageLedger(t.Context()); err != nil {
		t.Fatal(err)
	}
	b.languageLedgerVerified = false
	if err := b.workspace.Remove(testPythonMetadata); err != nil {
		t.Fatal(err)
	}
	job, err := b.StartJob(t.Context(), agentexecution.Allocation{}, agentexecution.JobSpec{JobID: "missing-package", Command: "touch", Args: []string{"must-not-run"}}, func(agentexecution.OutputChunk) error { return nil })
	var recovery *languageRecoveryError
	if job != nil || !errors.As(err, &recovery) {
		t.Fatalf("missing metadata allowed execution: %v %v", job, err)
	}
	if _, err := b.workspace.Stat("must-not-run"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("command ran")
	}
	if err := b.workspace.WriteFile(languageLedgerName, make([]byte, maxPackageLedgerBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.readLanguageLedger(); !errors.Is(err, agentexecution.ErrOutputLimit) {
		t.Fatalf("oversized ledger accepted: %v", err)
	}
}
