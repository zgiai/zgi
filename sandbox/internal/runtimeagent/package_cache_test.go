package runtimeagent

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/packagesource"
)

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
