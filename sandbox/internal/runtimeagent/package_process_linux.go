//go:build linux

package runtimeagent

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

func openPackageLedgerFile(root *os.Root) (*os.File, error) {
	// Never follow a replaced ledger link or block on a Job-created FIFO.
	return root.OpenFile(packageLedgerName, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}

func configurePackageProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.WaitDelay = time.Second
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		terminateDescendants(command.Process.Pid)
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		return command.Process.Kill()
	}
}
