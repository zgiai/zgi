//go:build linux

package runtimeagent

import (
	"errors"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// startGroupWithBackgroundChild starts a group leader that exits at once and
// leaves a background child of its group running for the given duration.
func startGroupWithBackgroundChild(t *testing.T, childSeconds string) int {
	t.Helper()
	cmd := exec.Command("/bin/sh", "-c", "sleep "+childSeconds+" & exit 0")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	group := cmd.Process.Pid
	t.Cleanup(func() { _ = syscall.Kill(-group, syscall.SIGKILL) })
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	return group
}

func TestProcessGroupMembersExitingShortlyAfterTheLeaderAreStopped(t *testing.T) {
	group := startGroupWithBackgroundChild(t, "0.3")
	if err := ensureProcessGroupStopped(group); !errors.Is(err, errProcessGroupRunning) {
		t.Fatalf("immediate scan = %v; the member is still exiting", err)
	}
	if err := awaitProcessGroupStopped(group, 2*time.Second); err != nil {
		t.Fatalf("await = %v; want the group stopped once its last member exited", err)
	}
}

func TestProcessGroupMemberOutlivingTheGraceIsStillRunning(t *testing.T) {
	group := startGroupWithBackgroundChild(t, "30")
	started := time.Now()
	if err := awaitProcessGroupStopped(group, 200*time.Millisecond); !errors.Is(err, errProcessGroupRunning) {
		t.Fatalf("await = %v; want a detached child reported running", err)
	}
	if elapsed := time.Since(started); elapsed < 200*time.Millisecond {
		t.Fatalf("gave up after %v, before the grace ended", elapsed)
	}
}
