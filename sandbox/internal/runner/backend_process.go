package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type processBackend struct{}

func newProcessBackend() backend {
	return &processBackend{}
}

func (b *processBackend) Name() string {
	return "preview-process"
}

func (b *processBackend) Run(parent context.Context, req Request, workDir string, ephemeral bool, timeout time.Duration, outputCap int) (Result, error) {
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

	stdout := newCappedBuffer(outputCap)
	stderr := newCappedBuffer(outputCap)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	started := time.Now()
	err = cmd.Run()
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

func (b *processBackend) ExecuteCommand(parent context.Context, workDir string, command string, args []string, timeout time.Duration, outputCap int) (CommandResult, error) {
	runCtx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	var cmd *exec.Cmd
	if len(args) > 0 {
		cmd = exec.CommandContext(runCtx, command, args...)
	} else {
		cmd = exec.CommandContext(runCtx, "/bin/sh", "-lc", command)
	}
	cmd.Dir = workDir

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
		Command:    command,
		Args:       args,
	}, nil
}
