package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootFSSelectorUsesDefaultWhenProfileRootDirIsUnset(t *testing.T) {
	defaultRoot := testRootFSDir(t, "default")

	selected, err := rootFSSelector{defaultRootFS: defaultRoot}.resolve("workflow-safe")
	if err != nil {
		t.Fatalf("resolve rootfs: %v", err)
	}
	if selected != defaultRoot {
		t.Fatalf("expected default rootfs, got %s", selected)
	}
}

func TestRootFSSelectorUsesDependencyProfileRootFS(t *testing.T) {
	defaultRoot := testRootFSDir(t, "default")
	profileRoot := t.TempDir()
	expected := filepath.Join(profileRoot, "workflow-safe")
	if err := os.Mkdir(expected, 0o755); err != nil {
		t.Fatalf("create profile rootfs: %v", err)
	}

	selected, err := rootFSSelector{
		defaultRootFS:       defaultRoot,
		dependencyRootFSDir: profileRoot,
	}.resolve("workflow-safe")
	if err != nil {
		t.Fatalf("resolve profile rootfs: %v", err)
	}
	if selected != expected {
		t.Fatalf("expected profile rootfs %s, got %s", expected, selected)
	}
}

func TestRootFSSelectorRejectsUnsafeDependencyProfileName(t *testing.T) {
	defaultRoot := testRootFSDir(t, "default")

	_, err := rootFSSelector{
		defaultRootFS:       defaultRoot,
		dependencyRootFSDir: t.TempDir(),
	}.resolve("../workflow-safe")
	if err == nil || !strings.Contains(err.Error(), "not safe") {
		t.Fatalf("expected unsafe profile rejection, got %v", err)
	}
}

func TestRootFSSelectorRejectsMissingDependencyProfileRootFS(t *testing.T) {
	defaultRoot := testRootFSDir(t, "default")

	_, err := rootFSSelector{
		defaultRootFS:       defaultRoot,
		dependencyRootFSDir: t.TempDir(),
	}.resolve("workflow-safe")
	if err == nil || !strings.Contains(err.Error(), "not usable") {
		t.Fatalf("expected missing profile rootfs rejection, got %v", err)
	}
}

func TestRootFSSelectorRejectsWorldWritableDependencyProfileRootFS(t *testing.T) {
	defaultRoot := testRootFSDir(t, "default")
	profileRoot := t.TempDir()
	unsafeRoot := filepath.Join(profileRoot, "workflow-safe")
	if err := os.Mkdir(unsafeRoot, 0o777); err != nil {
		t.Fatalf("create profile rootfs: %v", err)
	}
	if err := os.Chmod(unsafeRoot, 0o777); err != nil {
		t.Fatalf("chmod profile rootfs: %v", err)
	}

	_, err := rootFSSelector{
		defaultRootFS:       defaultRoot,
		dependencyRootFSDir: profileRoot,
	}.resolve("workflow-safe")
	if err == nil || !strings.Contains(err.Error(), "world-writable") {
		t.Fatalf("expected world-writable profile rootfs rejection, got %v", err)
	}
}

func testRootFSDir(t *testing.T, name string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatalf("create rootfs dir: %v", err)
	}
	return root
}
