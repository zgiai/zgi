package runtimeagent

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/packagesource"
)

func TestUnusedPackageEnvironmentCheckpointInitializesHome(t *testing.T) {
	// Use controlled ABI-probe executables, not a session interpreter or a
	// machine-dependent Node installation, so this regression never skips.
	bin := t.TempDir()
	for _, name := range []string{"python3", "node"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nprintf 'test-abi\\n'\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	source := packageTestBackend(t)
	if err := source.workspace.Remove(sessionHomeDirectory); err != nil {
		t.Fatal(err)
	}
	source.profileIdentity = "profile@sha256:blank-checkpoint"
	source.dependencyInstallation = true
	archive, err := source.ExportWorkspace(t.Context(), agentexecution.Allocation{})
	if err != nil {
		t.Fatalf("checkpoint before first Job: %v", err)
	}
	raw, err := io.ReadAll(archive)
	if closeErr := archive.Close(); err != nil || closeErr != nil {
		t.Fatalf("read checkpoint: %v, close: %v", err, closeErr)
	}
	if len(source.jobs) != 0 {
		t.Fatal("checkpoint unexpectedly started a Job")
	}
	ledger, exists, err := source.readLanguageLedger()
	if err != nil || !exists || len(ledger.Packages) != 0 || len(ledger.CacheKey) != 64 {
		t.Fatalf("blank ledger = %+v, exists=%v, err=%v", ledger, exists, err)
	}
	destination := newTestBackend(t)
	if err := destination.ImportWorkspace(t.Context(), agentexecution.Allocation{}, bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	destination.profileIdentity = source.profileIdentity
	if err := destination.verifyLanguageLedger(t.Context()); err != nil {
		t.Fatalf("restore unused environment: %v", err)
	}
}

func TestPackageCacheNamespaceSurvivesCheckpointAndRejectsProfileMismatch(t *testing.T) {
	if _, err := os.Stat("/usr/bin/node"); err != nil {
		t.Skip("requires Node ABI probe")
	}
	source := packageTestBackend(t)
	source.profileIdentity = "profile@sha256:abc"
	key, err := source.cacheNamespace(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 64 {
		t.Fatalf("cache key = %q", key)
	}
	name := sessionHomeDirectory + "/.cache/pip/" + key + "/wheel"
	if err := source.workspace.MkdirAll(sessionHomeDirectory+"/.cache/pip/"+key, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := source.workspace.WriteFile(name, []byte("cached-wheel"), 0o600); err != nil {
		t.Fatal(err)
	}
	archive, err := source.ExportWorkspace(t.Context(), agentexecution.Allocation{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(archive)
	_ = archive.Close()
	if err != nil {
		t.Fatal(err)
	}
	restored := newTestBackend(t)
	if err := restored.ImportWorkspace(t.Context(), agentexecution.Allocation{}, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	restored.profileIdentity = source.profileIdentity
	got, err := restored.cacheNamespace(t.Context())
	if err != nil || got != key {
		t.Fatalf("restored key=%q err=%v", got, err)
	}
	if data, err := restored.workspace.ReadFile(name); err != nil || string(data) != "cached-wheel" {
		t.Fatalf("restored cache=%q err=%v", data, err)
	}
	restored.cacheKey = ""
	restored.packageSources = packagesource.Config{PyPI: "https://mirror.example/simple"}.Resolved()
	other, err := restored.cacheNamespace(t.Context())
	if err != nil || other == key {
		t.Fatalf("changed source cache=%q err=%v", other, err)
	}
	restored.cacheKey = ""
	restored.profileIdentity = "different-profile"
	if _, err := restored.cacheNamespace(t.Context()); err == nil {
		t.Fatal("mismatched retained ABI/profile accepted")
	}
}
