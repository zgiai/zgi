package runner

import (
	"fmt"
	"os/exec"
	"syscall"
)

func exitCodeFromExitError(err *exec.ExitError, stderr *cappedBuffer) int {
	if err == nil {
		return 0
	}
	status, ok := err.Sys().(syscall.WaitStatus)
	if ok && status.Signaled() {
		signal := status.Signal()
		if stderr != nil {
			stderr.AppendLine(fmt.Sprintf("process terminated by signal: %s", signal.String()))
		}
		return 128 + int(signal)
	}
	return err.ExitCode()
}
