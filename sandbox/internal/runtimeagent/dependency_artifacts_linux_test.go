//go:build linux

package runtimeagent

import (
	"encoding/json"
	"errors"
	"os"
	"path"
	"strings"
	"syscall"
	"testing"
)

func TestDependencyArtifactsKeepWorkspaceLinksUncached(t *testing.T) {
	b := packageTestBackend(t)
	writeLanguageFixture(t, b, "project/package.json", `{"name":"workspace-package","version":"1.0.0"}`)
	if err := b.workspace.MkdirAll("node_modules", 0o750); err != nil {
		t.Fatal(err)
	}
	if err := b.workspace.Symlink("../project", "node_modules/workspace-package"); err != nil {
		t.Fatal(err)
	}
	if err := b.captureLanguageLedger(t.Context()); err != nil {
		t.Fatal(err)
	}
	ledger, _, err := b.readLanguageLedger()
	if err != nil || len(ledger.Artifacts) != 0 || len(ledger.UncachedTargets) != 1 || ledger.UncachedTargets[0] != "node_modules" {
		t.Fatalf("linked ledger=%+v err=%v", ledger, err)
	}
	b.languageLedgerVerified = false
	if err := b.verifyLanguageLedger(t.Context()); err != nil {
		t.Fatalf("legacy linked workspace changed: %v", err)
	}
}

func TestDependencyRecoveryABIConflictPrecedesMutation(t *testing.T) {
	b := dependencyFixture(t)
	ledger, _, err := b.readLanguageLedger()
	if err != nil {
		t.Fatal(err)
	}
	b.profileIdentity = "approved-profile"
	ledger.Environment = b.packageLedgerEnvironment()
	ledger.CacheKey = strings.Repeat("1", 64)
	b.cacheKey = strings.Repeat("2", 64)
	raw, err := json.Marshal(ledger)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.workspace.WriteFile(languageLedgerName, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := b.workspace.RemoveAll("node_modules"); err != nil {
		t.Fatal(err)
	}
	b.languageLedgerVerified = false
	if err := b.verifyLanguageLedger(t.Context()); err == nil {
		t.Fatal("ABI conflict accepted")
	}
	if _, err := b.workspace.Stat("node_modules"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("ABI conflict mutated dependencies")
	}
}

func TestDependencyArtifactRefusesFIFOAndSymlink(t *testing.T) {
	for _, kind := range []string{"fifo", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			b := dependencyFixture(t)
			ledger, _, err := b.readLanguageLedger()
			if err != nil {
				t.Fatal(err)
			}
			artifact := ledger.Artifacts[0]
			if err := b.workspace.Remove(artifact.name()); err != nil {
				t.Fatal(err)
			}
			if kind == "fifo" {
				if err := syscall.Mkfifo(b.workspaceName+"/"+artifact.name(), 0o600); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := b.workspace.Symlink("../../untrusted", artifact.name()); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := b.openDependencyArtifact(t.Context(), artifact); err == nil {
				t.Fatalf("unsafe %s artifact accepted", kind)
			}
		})
	}
}

func TestDependencyRecoveryPreservesNonRootOwnership(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root runtime identity")
	}
	b := dependencyFixture(t)
	b.jobUID, b.jobGID = 65532, 65532
	ledger, _, err := b.readLanguageLedger()
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range ledger.Artifacts {
		if err := b.workspace.RemoveAll(artifact.Target); err != nil {
			t.Fatal(err)
		}
	}
	b.languageLedgerVerified = false
	if err := b.verifyLanguageLedger(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{sessionHomeDirectory + "/.venv", path.Dir(testNodeMetadata), testNodeMetadata} {
		info, err := b.workspace.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != 65532 || stat.Gid != 65532 || info.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
			t.Fatalf("restored owner/mode %s: %+v %v", name, stat, info.Mode())
		}
	}
}

func TestDependencyRecoveryRejectsUnsafeTemporaryTrees(t *testing.T) {
	for _, kind := range []string{"stage", "backup"} {
		t.Run(kind, func(t *testing.T) {
			b := dependencyFixture(t)
			ledger, _, err := b.readLanguageLedger()
			if err != nil {
				t.Fatal(err)
			}
			stage, backup := dependencyRecoveryNames(ledger.Artifacts[1])
			name := stage
			if kind == "backup" {
				name = backup
			}
			writeLanguageFixture(t, b, "business/result.txt", "keep")
			if err := b.workspace.Symlink("../../business", name); err != nil {
				t.Fatal(err)
			}
			b.languageLedgerVerified = false
			if err := b.verifyLanguageLedger(t.Context()); err == nil {
				t.Fatal("unsafe temporary tree accepted")
			}
			if raw, err := b.workspace.ReadFile("business/result.txt"); err != nil || string(raw) != "keep" {
				t.Fatalf("unrelated result changed: %s %v", raw, err)
			}
		})
	}
}
