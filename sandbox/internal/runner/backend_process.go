package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type processBackend struct{}

func newProcessBackend() backend {
	return &processBackend{}
}

func (b *processBackend) Name() string {
	return "preview-process"
}

func (b *processBackend) Run(parent context.Context, req Request, workDir string, ephemeral bool, timeout time.Duration, stdoutLimit int, stderrLimit int) (Result, error) {
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

	scriptPath := filepath.Join(root, fmt.Sprintf(".zgi-run-%s-%s", token(), spec.filename))
	content := buildContent(req.Preload, req.Code)
	if err := os.WriteFile(scriptPath, []byte(content), 0o600); err != nil {
		return Result{}, err
	}
	defer os.Remove(scriptPath)

	cmd := exec.CommandContext(runCtx, spec.binary, spec.args(scriptPath)...)
	cmd.Dir = root
	if req.Stdin != "" {
		cmd.Stdin = strings.NewReader(req.Stdin)
	}

	stdout := newCappedBuffer(stdoutLimit)
	stderr := newCappedBuffer(stderrLimit)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	started := time.Now()
	err = runProcessGroup(runCtx, cmd)
	duration := time.Since(started).Milliseconds()

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		switch {
		case errors.Is(runCtx.Err(), context.DeadlineExceeded):
			exitCode = 124
			stderr.AppendLine("execution timed out")
		case errors.As(err, &exitErr):
			exitCode = exitErr.ExitCode()
		default:
			return Result{}, err
		}
	}

	return Result{
		Stdout:           stdout.String(),
		Error:            stderr.String(),
		ExitCode:         exitCode,
		DurationMS:       duration,
		NetworkRequested: req.EnableNetwork,
		Truncated:        stdout.Truncated() || stderr.Truncated(),
	}, nil
}

func (b *processBackend) ExecuteCommand(parent context.Context, spec CommandSpec) (CommandResult, error) {
	runCtx, cancel := context.WithTimeout(parent, spec.Timeout)
	defer cancel()

	var cmd *exec.Cmd
	if len(spec.Args) > 0 {
		cmd = exec.CommandContext(runCtx, spec.Command, spec.Args...)
	} else if spec.AllowShellForm {
		cmd = exec.CommandContext(runCtx, "/bin/sh", "-lc", spec.Command)
	} else {
		cmd = exec.CommandContext(runCtx, spec.Command)
	}
	cmd.Dir = spec.WorkDir
	if spec.Stdin != "" {
		cmd.Stdin = strings.NewReader(spec.Stdin)
	}
	cmd.Env = safeBaseEnv(os.Environ())
	for key, value := range spec.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}

	stdout := newCappedBuffer(spec.StdoutLimit)
	stderr := newCappedBuffer(spec.StderrLimit)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	started := time.Now()
	err := runProcessGroup(runCtx, cmd)
	duration := time.Since(started).Milliseconds()

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		switch {
		case errors.Is(runCtx.Err(), context.DeadlineExceeded):
			exitCode = 124
			stderr.AppendLine("command timed out")
		case errors.As(err, &exitErr):
			exitCode = exitErr.ExitCode()
		default:
			return CommandResult{}, err
		}
	}

	return CommandResult{
		Stdout:     stdout.String(),
		Error:      stderr.String(),
		ExitCode:   exitCode,
		DurationMS: duration,
		Truncated:  stdout.Truncated() || stderr.Truncated(),
		Command:    spec.Command,
		Args:       spec.Args,
	}, nil
}

func runProcessGroup(ctx context.Context, cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		killProcessGroup(cmd)
		<-done
		return ctx.Err()
	}
}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	_ = cmd.Process.Kill()
}
