//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package runner

import "os/exec"

func exitCodeFromExitError(err *exec.ExitError, _ *cappedBuffer) int {
	if err == nil {
		return 0
	}
	return err.ExitCode()
}
