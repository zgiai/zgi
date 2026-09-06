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
