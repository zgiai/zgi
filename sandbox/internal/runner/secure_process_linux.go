//go:build linux

package runner

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

// runSecureProcess uses bubblewrap's private information FD to track its PID
// namespace init. In setuid mode bubblewrap's outer monitor can change host UID
// and become unsignalable by the server. Its namespace init retains the caller's
// host UID; terminating it also terminates every process in that PID namespace,
// including children which created a new session or process group.
func runSecureProcess(ctx context.Context, cmd *exec.Cmd, bwrapArgIndex int) error {
	reader, writer, err := os.Pipe()
	if err != nil {
		return err
	}
	defer reader.Close()
	defer writer.Close()
	fd := 3 + len(cmd.ExtraFiles)
	cmd.ExtraFiles = append(cmd.ExtraFiles, writer)
	// Resource-limit and cgroup launchers can precede bubblewrap. The caller
	// supplies its final argument index so the information option still lands
	// immediately after the bwrap binary.
	insert := bwrapArgIndex + 1
	args := append([]string(nil), cmd.Args[:insert]...)
	args = append(args, "--info-fd", strconv.Itoa(fd))
	cmd.Args = append(args, cmd.Args[insert:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	ready := make(chan struct{})
	var namespaceInit *os.Process
	go func() {
		defer close(ready)
		var info struct {
			ChildPID int `json:"child-pid"`
		}
		if json.NewDecoder(reader).Decode(&info) == nil && info.ChildPID > 1 {
			namespaceInit, _ = os.FindProcess(info.ChildPID)
			// Handle cancellation that arrives before bubblewrap emits the PID.
			if ctx.Err() != nil && namespaceInit != nil {
				_ = namespaceInit.Kill()
			}
		}
	}()
	defer func() {
		_ = reader.Close()
		<-ready
		if namespaceInit != nil {
			_ = namespaceInit.Release()
		}
	}()
	cmd.Cancel = func() error {
		select {
		case <-ready:
			if namespaceInit != nil {
				_ = namespaceInit.Kill()
			}
		case <-time.After(time.Second):
			// The reader above will kill an init reported after this deadline.
		}
		killProcessGroup(cmd)
		return nil
	}
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = writer.Close()
	err = cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
