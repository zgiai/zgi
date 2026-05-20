//go:build linux

package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/config"
)

type linuxSecureBackend struct {
	rootfs     string
	bwrapBin   string
	allowShell bool
}

func newLinuxSecureBackend(cfg config.Config) (backend, error) {
	rootfs := strings.TrimSpace(cfg.SecureRootFS)
	if rootfs == "" {
		return nil, errors.New("linux-secure backend requires ZGI_SANDBOX_SECURE_ROOTFS")
	}
	if _, err := os.Stat(rootfs); err != nil {
		return nil, fmt.Errorf("stat secure rootfs: %w", err)
	}

	bwrapBin := strings.TrimSpace(cfg.BwrapBinary)
	if bwrapBin == "" {
		bwrapBin = "bwrap"
	}
	if _, err := exec.LookPath(bwrapBin); err != nil {
		return nil, fmt.Errorf("find bubblewrap binary: %w", err)
	}

	return &linuxSecureBackend{
		rootfs:     rootfs,
		bwrapBin:   bwrapBin,
		allowShell: true,
	}, nil
}

func (b *linuxSecureBackend) Name() string {
	return "linux-secure"
}

func (b *linuxSecureBackend) Run(parent context.Context, req Request, workDir string, ephemeral bool, timeout time.Duration, outputCap int) (Result, error) {
	spec, err := languageSpec(req.Language)
	if err != nil {
		return Result{}, err
	}

	runCtx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	root := workDir
	if root == "" {
		root, err = os.MkdirTemp("", "zgi-sandbox-*")
		if err != nil {
			return Result{}, err
		}
	}
	if ephemeral {
		defer os.RemoveAll(root)
	}

	scriptName := fmt.Sprintf(".zgi-run-%s-%s", token(), spec.filename)
	hostScriptPath := filepath.Join(root, scriptName)
	if err := os.WriteFile(hostScriptPath, []byte(buildContent(req.Preload, req.Code)), 0o600); err != nil {
		return Result{}, err
	}
	defer os.Remove(hostScriptPath)

	containerPath := containerScriptPath(root, scriptName)
	return b.exec(runCtx, root, spec.binary, spec.args(containerPath), req.EnableNetwork, outputCap)
}

func (b *linuxSecureBackend) ExecuteCommand(parent context.Context, workDir string, command string, args []string, timeout time.Duration, outputCap int) (CommandResult, error) {
	runCtx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	commandArgs := []string{}
	if len(args) > 0 {
		commandArgs = append(commandArgs, command)
		commandArgs = append(commandArgs, args...)
	} else if b.allowShell {
		commandArgs = []string{"/bin/sh", "-lc", command}
	} else {
		commandArgs = []string{command}
	}

	result, err := b.exec(runCtx, workDir, commandArgs[0], commandArgs[1:], false, outputCap)
	if err != nil {
		return CommandResult{}, err
	}
	return CommandResult{
		Stdout:     result.Stdout,
		Error:      result.Error,
		ExitCode:   result.ExitCode,
		DurationMS: result.DurationMS,
		Truncated:  result.Truncated,
		Command:    command,
		Args:       args,
	}, nil
}

func (b *linuxSecureBackend) exec(ctx context.Context, workDir string, binary string, args []string, enableNetwork bool, outputCap int) (Result, error) {
	bwrapArgs := []string{
		"--die-with-parent",
		"--new-session",
		"--clearenv",
		"--ro-bind", b.rootfs, "/",
		"--proc", "/proc",
		"--dev", "/dev",
		"--tmpfs", "/tmp",
		"--dir", "/tmp/workspace",
		"--bind", workDir, "/tmp/workspace",
		"--chdir", "/tmp/workspace",
		"--setenv", "HOME", "/tmp/workspace",
		"--setenv", "PATH", "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"--unshare-user",
		"--uid", "65534",
		"--gid", "65534",
		"--unshare-pid",
		"--unshare-ipc",
		"--unshare-uts",
		"--unshare-cgroup",
	}
	if !enableNetwork {
		bwrapArgs = append(bwrapArgs, "--unshare-net")
	}
	bwrapArgs = append(bwrapArgs, binary)
	bwrapArgs = append(bwrapArgs, args...)

	cmd := exec.CommandContext(ctx, b.bwrapBin, bwrapArgs...)
	stdout := newCappedBuffer(outputCap)
	stderr := newCappedBuffer(outputCap)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	started := time.Now()
	err := cmd.Run()
	duration := time.Since(started).Milliseconds()

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		switch {
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			exitCode = 124
			stderr.AppendLine("execution timed out")
		case errors.As(err, &exitErr):
			exitCode = exitErr.ExitCode()
		default:
			return Result{}, err
		}
	}

	return Result{
		Stdout:     stdout.String(),
		Error:      stderr.String(),
		ExitCode:   exitCode,
		DurationMS: duration,
		Truncated:  stdout.Truncated() || stderr.Truncated(),
	}, nil
}
