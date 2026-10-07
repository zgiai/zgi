//go:build linux

package runtimeagent

import (
	"errors"
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
		return errors.Join(terminateDescendants(command.Process.Pid),
			terminationSignalError(syscall.Kill(-command.Process.Pid, syscall.SIGKILL)),
			terminationSignalError(command.Process.Kill()))
	}
}
