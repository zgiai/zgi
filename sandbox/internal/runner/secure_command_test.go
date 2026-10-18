package runner

import (
	"reflect"
	"testing"
)

func TestWrapSecureCommandInCgroupPreservesArgumentsAndBwrapIndex(t *testing.T) {
	command, args, bwrapArgIndex := wrapSecureCommandInCgroup(
		"/bin/sh",
		"/sys/fs/cgroup/zgi-sandbox-executions/execution-1",
		"/usr/bin/prlimit",
		[]string{"--cpu=2", "--", "/usr/bin/bwrap", "--unshare-pid", "/bin/true"},
		3,
	)

	if command != "/bin/sh" {
		t.Fatalf("command=%q, want %q", command, "/bin/sh")
	}
	wantArgs := []string{
		"-c",
		secureCgroupLauncherScript,
		"zgi-cgroup-launch",
		"/sys/fs/cgroup/zgi-sandbox-executions/execution-1",
		"/usr/bin/prlimit",
		"--cpu=2",
		"--",
		"/usr/bin/bwrap",
		"--unshare-pid",
		"/bin/true",
	}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args:\n got: %#v\nwant: %#v", args, wantArgs)
	}
	if bwrapArgIndex != 8 {
		t.Fatalf("bwrap arg index=%d, want 8", bwrapArgIndex)
	}
}
