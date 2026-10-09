//go:build linux

package runtimeagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"syscall"
)

func prepareJobFileOwnership(file *os.File, uid, gid uint32) error {
	if uid == 0 && gid == 0 || os.Geteuid() != 0 {
		return nil
	}
	return file.Chown(int(uid), int(gid))
}

// Open through os.Root before changing ownership so a Job-controlled link
// cannot redirect this privileged operation outside the workspace.
func prepareWorkspaceDirectoryOwnership(root *os.Root, directory string, uid, gid uint32) error {
	if uid == 0 && gid == 0 || os.Geteuid() != 0 {
		return nil
	}
	for name := directory; name != "."; name = path.Dir(name) {
		file, err := root.Open(name)
		if err != nil {
			return err
		}
		info, statErr := file.Stat()
		if statErr != nil || !info.IsDir() {
			_ = file.Close()
			return errors.New("workspace parent must be a directory")
		}
		err = errors.Join(prepareJobFileOwnership(file, uid, gid), file.Close())
		if err != nil {
			return err
		}
	}
	return nil
}

// prepareSessionPythonEnvironment creates the writable per-session virtual
// environment used by standard Skills. The parent is under the retained
// workspace, so a Home/Workspace checkpoint keeps installed wheels available
// after an environment is recreated. System site packages are visible so a
// pre-built Profile remains useful without copying its packages into every
// session venv.
func prepareSessionPythonEnvironment(ctx context.Context, home string, uid, gid uint32) error {
	venv := filepath.Join(home, ".venv")
	info, err := os.Lstat(venv)
	if err == nil {
		if !info.IsDir() {
			return errors.New("session Python virtual environment is not a directory")
		}
		python := filepath.Join(venv, "bin", "python")
		// A venv created without --copies may expose the interpreter as a
		// symlink to the profile runtime, so follow this path when checking
		// completeness. The venv directory itself is still checked with Lstat.
		pythonInfo, statErr := os.Stat(python)
		if statErr == nil && pythonInfo.Mode().IsRegular() {
			return nil
		}
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
		// A failed or interrupted venv creation can leave a directory that
		// passes the parent check but cannot run Python. Remove that incomplete
		// state while the caller holds the per-backend preparation lock.
		if removeErr := os.RemoveAll(venv); removeErr != nil {
			return fmt.Errorf("remove incomplete session Python virtual environment: %w", removeErr)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return err
	}
	python := strings.TrimSpace(os.Getenv("ZGI_RUNTIME_AGENT_PYTHON"))
	if python == "" {
		python = "python3"
	}
	command := exec.CommandContext(ctx, python, "-m", "venv", "--copies", "--system-site-packages", venv)
	if uid != 0 && os.Geteuid() == 0 {
		// The Home directory is Job-controlled. Never run Python preparation
		// or recursively chown its contents as root: a concurrent Job could
		// swap a file for a link between inspection and mutation.
		command = exec.CommandContext(ctx, "/usr/bin/setpriv", "--no-new-privs", "--inh-caps=-all", "--ambient-caps=-all", "--", python,
			"-m", "venv", "--copies", "--system-site-packages", venv)
		command.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uid, Gid: gid}}
	}
	output, runErr := command.CombinedOutput()
	if runErr != nil {
		_ = os.RemoveAll(venv)
		return fmt.Errorf("create virtual environment: %w: %s", runErr, limitPackageOutput(string(output)))
	}
	return nil
}
