package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSeedDependencyProfileRuntimeCacheCreatesIsolatedWritableCopy(t *testing.T) {
	profile := t.TempDir()
	workspaceOne := t.TempDir()
	workspaceTwo := t.TempDir()
	source := filepath.Join(profile, "runtime-cache", "matplotlib")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "fontlist.json"), []byte("trusted"), 0o444); err != nil {
		t.Fatal(err)
	}

	for _, workspace := range []string{workspaceOne, workspaceTwo} {
		if err := seedDependencyProfileRuntimeCache(profile, workspace); err != nil {
			t.Fatalf("seed cache: %v", err)
		}
		copyPath := filepath.Join(workspace, ".zgi-runtime-cache", "matplotlib", "fontlist.json")
		content, err := os.ReadFile(copyPath)
		if err != nil {
			t.Fatalf("read seeded cache: %v", err)
		}
		if string(content) != "trusted" {
			t.Fatalf("seeded content = %q", content)
		}
	}

	copyOne := filepath.Join(workspaceOne, ".zgi-runtime-cache", "matplotlib", "fontlist.json")
	if err := os.WriteFile(copyOne, []byte("tenant-one"), 0o666); err != nil {
		t.Fatalf("workspace cache must be writable: %v", err)
	}
	copyTwo, err := os.ReadFile(filepath.Join(workspaceTwo, ".zgi-runtime-cache", "matplotlib", "fontlist.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(copyTwo) != "trusted" {
		t.Fatalf("workspace caches are not isolated: %q", copyTwo)
	}
	sourceContent, err := os.ReadFile(filepath.Join(source, "fontlist.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(sourceContent) != "trusted" {
		t.Fatalf("profile cache was modified: %q", sourceContent)
	}
}

func TestSeedDependencyProfileRuntimeCacheRejectsSymlink(t *testing.T) {
	profile := t.TempDir()
	workspace := t.TempDir()
	source := filepath.Join(profile, "runtime-cache", "matplotlib")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("outside", filepath.Join(source, "unsafe")); err != nil {
		t.Fatal(err)
	}
	if err := seedDependencyProfileRuntimeCache(profile, workspace); err == nil {
		t.Fatal("expected symlink rejection")
	}
}

func TestSeedDependencyProfileRuntimeCacheRejectsSymlinkedWorkspaceCacheRoot(t *testing.T) {
	profile := t.TempDir()
	workspace := t.TempDir()
	outside := t.TempDir()
	writeRuntimeCacheFixture(t, profile)
	if err := os.Symlink(outside, filepath.Join(workspace, runtimeCacheRootName)); err != nil {
		t.Fatal(err)
	}

	err := seedDependencyProfileRuntimeCache(profile, workspace)
	if err == nil {
		t.Fatal("expected workspace cache root symlink rejection")
	}
	if _, statErr := os.Stat(filepath.Join(outside, matplotlibRuntimeCacheName)); !os.IsNotExist(statErr) {
		t.Fatalf("outside cache path was modified: %v", statErr)
	}
}

func TestSeedDependencyProfileRuntimeCacheRejectsSymlinkedWorkspaceDestination(t *testing.T) {
	profile := t.TempDir()
	workspace := t.TempDir()
	outside := t.TempDir()
	writeRuntimeCacheFixture(t, profile)
	cacheRoot := filepath.Join(workspace, runtimeCacheRootName)
	if err := os.Mkdir(cacheRoot, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(cacheRoot, matplotlibRuntimeCacheName)); err != nil {
		t.Fatal(err)
	}

	err := seedDependencyProfileRuntimeCache(profile, workspace)
	if err == nil {
		t.Fatal("expected workspace cache destination symlink rejection")
	}
	entries, readErr := os.ReadDir(outside)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("outside directory contains %d entries, want 0", len(entries))
	}
}

func writeRuntimeCacheFixture(t *testing.T, profile string) {
	t.Helper()
	source := filepath.Join(profile, "runtime-cache", matplotlibRuntimeCacheName)
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "fontlist.json"), []byte("trusted"), 0o444); err != nil {
		t.Fatal(err)
	}
}
