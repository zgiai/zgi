//go:build linux

package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewProcessCgroupManagerRequiresDelegatedPidsController(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "cgroup.subtree_control"), []byte("cpu memory"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := newProcessCgroupManager(root, 8); err == nil {
		t.Fatal("expected missing pids controller to be rejected")
	}
}

func TestProcessCgroupManagerCreatesIsolatedLimit(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "cgroup.subtree_control"), []byte("cpu pids memory"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager, err := newProcessCgroupManager(root, 17)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	group, err := manager.Create()
	if err != nil {
		t.Fatalf("create execution cgroup: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(group.path, "pids.max"))
	if err != nil {
		t.Fatalf("read process limit: %v", err)
	}
	if got := string(data); got != "17" {
		t.Fatalf("process limit=%q, want %q", got, "17")
	}
}

func TestReadCgroupPopulated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cgroup.events")
	if err := os.WriteFile(path, []byte("populated 1\nfrozen 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	populated, err := readCgroupPopulated(path)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	if !populated {
		t.Fatal("expected populated cgroup")
	}
}
