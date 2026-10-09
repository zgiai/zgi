package mcpclient

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"sync"
)

// LocalProcessLauncher starts a trusted, caller-selected executable. It does
// not inherit arbitrary API process secrets into the child.
type LocalProcessLauncher struct{}

func (LocalProcessLauncher) Launch(ctx context.Context, spec ProcessSpec) (Process, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cmd := exec.Command(spec.Command, spec.Args...)
	cmd.Dir = spec.WorkDir
	base := []string{"PATH", "HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "TEMP", "TMP", "SystemRoot"}
	for _, key := range base {
		if value := os.Getenv(key); value != "" {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	keys := make([]string, 0, len(spec.Env))
	for key := range spec.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		cmd.Env = append(cmd.Env, key+"="+spec.Env[key])
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdio stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("stdio stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
		stdout.Close()
		return nil, fmt.Errorf("stdio stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		stderr.Close()
		return nil, fmt.Errorf("start stdio process: %w", err)
	}
	p := &localProcess{cmd: cmd, stdin: stdin, stdout: stdout, stderr: stderr, done: make(chan struct{})}
	go func() { p.mu.Lock(); p.waitErr = cmd.Wait(); p.mu.Unlock(); close(p.done) }()
	if err := ctx.Err(); err != nil {
		_ = p.Terminate(context.Background())
		return nil, err
	}
	return p, nil
}

type localProcess struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	stderr  io.ReadCloser
	done    chan struct{}
	mu      sync.Mutex
	waitErr error
}

func (p *localProcess) Stdin() io.WriteCloser { return p.stdin }
func (p *localProcess) Stdout() io.ReadCloser { return p.stdout }
func (p *localProcess) Stderr() io.ReadCloser { return p.stderr }
func (p *localProcess) Wait(ctx context.Context) error {
	select {
	case <-p.done:
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.waitErr
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (p *localProcess) Terminate(ctx context.Context) error {
	select {
	case <-p.done:
		return p.Wait(ctx)
	default:
	}
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	return p.Wait(ctx)
}
