//go:build linux

package runtimeagent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

type runningProcess struct {
	cmd         *exec.Cmd
	cancel      context.CancelFunc
	done        chan struct{}
	pty         *os.File
	ptyMu       sync.Mutex
	cancelOnce  sync.Once
	canceled    atomic.Bool
	mu          sync.Mutex
	result      agentexecution.JobResult
	stopFailed  chan struct{}
	failureOnce sync.Once
	stopErr     error
	onUnsafe    func(error)
}

func supportsPTY() bool { return true }

func startProcess(parent context.Context, workDir string, spec agentexecution.JobSpec, jobUID, jobGID uint32, sink func(agentexecution.OutputStream, []byte) error, onUnsafe func(error), cleanup func(temporaryDisposition) error) (agentexecution.RunningJob, error) {
	var jobContext context.Context
	var cancel context.CancelFunc
	if spec.Timeout > 0 {
		jobContext, cancel = context.WithTimeout(parent, spec.Timeout)
	} else {
		jobContext, cancel = context.WithCancel(parent)
	}
	cmd := exec.CommandContext(jobContext, spec.Command, spec.Args...)
	if jobUID != 0 && os.Geteuid() == 0 {
		// E2B does not inherit the OCI noNewPrivileges setting used locally.
		// Set it in the child before executing untrusted code, without changing
		// the root Runtime Agent or the separately controlled APT installer.
		args := append([]string{"--no-new-privs", "--inh-caps=-all", "--ambient-caps=-all", "--", spec.Command}, spec.Args...)
		cmd = exec.CommandContext(jobContext, "/usr/bin/setpriv", args...)
	}
	cmd.Dir = workDir
	cmd.Env = mergedEnvironment(spec.Environment)
	process := &runningProcess{cmd: cmd, cancel: cancel, done: make(chan struct{}), stopFailed: make(chan struct{}), onUnsafe: onUnsafe}
	cmd.Cancel = func() error {
		err := process.kill()
		if err != nil {
			process.markTerminationFailure(err)
		}
		return err
	}
	cmd.WaitDelay = time.Second

	var ptyOutputDone chan struct{}
	if jobUID != 0 || jobGID != 0 {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Credential: &syscall.Credential{Uid: jobUID, Gid: jobGID}}
	}
	if spec.PTY {
		terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: spec.PTYColumns, Rows: spec.PTYRows})
		if err != nil {
			cancel()
			return nil, fmt.Errorf("start PTY job: %w", err)
		}
		process.pty = terminal
		ptyOutputDone = make(chan struct{})
		go func() {
			defer close(ptyOutputDone)
			pumpOutput(terminal, agentexecution.OutputStreamStdout, sink)
		}()
	} else {
		if cmd.SysProcAttr == nil {
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		}
		cmd.Stdout = &outputWriter{stream: agentexecution.OutputStreamStdout, sink: sink}
		cmd.Stderr = &outputWriter{stream: agentexecution.OutputStreamStderr, sink: sink}
		if err := cmd.Start(); err != nil {
			cancel()
			return nil, fmt.Errorf("start job: %w", err)
		}
	}

	go func() {
		waitErr := cmd.Wait()
		contextErr := jobContext.Err()
		process.ptyMu.Lock()
		terminal := process.pty
		process.pty = nil
		process.ptyMu.Unlock()
		if terminal != nil {
			select {
			case <-ptyOutputDone:
			case <-time.After(time.Second):
				// A descendant can hold the slave side open after cmd.Wait.
				// Bound the drain, then unblock the reader by closing the master.
				_ = terminal.Close()
				<-ptyOutputDone
			}
			_ = terminal.Close()
		}
		result := processResult(waitErr, contextErr, process.canceled.Load())
		// A detached background child must not keep writing while its temporary
		// directory is reclaimed. Fail closed instead of deleting live data.
		if err := ensureProcessGroupStopped(cmd.Process.Pid); err != nil {
			process.markTerminationFailure(errors.Join(err, cleanup(retainTemporary)))
		} else if err := cleanup(discardTemporary); err != nil {
			process.markTerminationFailure(fmt.Errorf("job temporary cleanup failed: %w", err))
		}
		process.mu.Lock()
		if process.stopErr != nil {
			result = agentexecution.JobResult{ExitCode: -1, Error: process.stopErr.Error(), TerminationFailed: true}
		}
		process.result = result
		process.mu.Unlock()
		cancel()
		close(process.done)
	}()
	return process, nil
}

func ensureProcessGroupStopped(group int) error {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		value, err := os.ReadFile("/proc/" + entry.Name() + "/stat")
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		// comm may itself contain spaces and parentheses.
		end := strings.LastIndexByte(string(value), ')')
		if end < 0 {
			return errors.New("invalid process status")
		}
		fields := strings.Fields(string(value[end+1:]))
		if len(fields) < 3 {
			return errors.New("incomplete process status")
		}
		pgrp, err := strconv.Atoi(fields[2])
		if err != nil {
			return err
		}
		if pgrp == group && fields[0] != "Z" && fields[0] != "X" {
			return errors.New("job process group is still running")
		}
	}
	return nil
}

type outputWriter struct {
	stream agentexecution.OutputStream
	sink   func(agentexecution.OutputStream, []byte) error
}

func (w outputWriter) Write(value []byte) (int, error) {
	copyValue := append([]byte(nil), value...)
	if err := w.sink(w.stream, copyValue); err != nil {
		return 0, err
	}
	return len(value), nil
}

func (p *runningProcess) Wait(ctx context.Context) (agentexecution.JobResult, error) {
	select {
	case <-p.done:
	case <-p.stopFailed:
		return p.terminationFailure()
	case <-ctx.Done():
		if err := p.Cancel(ctx); err != nil {
			return p.terminationFailure()
		}
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case <-p.done:
		case <-p.stopFailed:
			return p.terminationFailure()
		case <-timer.C:
			p.markTerminationFailure(errors.New("process did not exit after termination"))
			return p.terminationFailure()
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.result, nil
}

func (p *runningProcess) Cancel(context.Context) error {
	select {
	case <-p.done:
		return nil
	default:
	}
	p.cancelOnce.Do(func() {
		p.canceled.Store(true)
		if err := terminateDescendants(p.cmd.Process.Pid); err != nil {
			p.markTerminationFailure(err)
		}
		select {
		case <-p.done:
			return
		case <-time.After(250 * time.Millisecond):
		}
		p.cancel()
		if err := p.kill(); err != nil {
			p.markTerminationFailure(err)
		}
	})
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stopErr
}

func (p *runningProcess) ResizePTY(_ context.Context, columns uint16, rows uint16) error {
	p.ptyMu.Lock()
	defer p.ptyMu.Unlock()
	if p.pty == nil {
		return fmt.Errorf("%w: job was not started with a PTY", agentexecution.ErrInvalidTransition)
	}
	if columns == 0 || rows == 0 {
		return fmt.Errorf("%w: PTY dimensions are required", agentexecution.ErrInvalidRequest)
	}
	return pty.Setsize(p.pty, &pty.Winsize{Cols: columns, Rows: rows})
}

func (p *runningProcess) kill() error {
	if p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	return errors.Join(terminateDescendants(p.cmd.Process.Pid),
		terminationSignalError(syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)),
		terminationSignalError(p.cmd.Process.Kill()))
}

func terminationSignalError(err error) error {
	if errors.Is(err, syscall.ESRCH) || errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}

func (p *runningProcess) markTerminationFailure(err error) {
	p.failureOnce.Do(func() {
		p.mu.Lock()
		p.stopErr = fmt.Errorf("job termination failed: %w", err)
		p.mu.Unlock()
		if p.onUnsafe != nil {
			p.onUnsafe(err)
		}
		close(p.stopFailed)
	})
}

func (p *runningProcess) terminationFailure() (agentexecution.JobResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return agentexecution.JobResult{ExitCode: -1, Error: p.stopErr.Error(), TerminationFailed: true}, p.stopErr
}

func pumpOutput(reader io.Reader, stream agentexecution.OutputStream, sink func(agentexecution.OutputStream, []byte) error) {
	buffer := make([]byte, 32*1024)
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			_ = sink(stream, append([]byte(nil), buffer[:count]...))
		}
		if err != nil {
			return
		}
	}
}

func processResult(waitErr error, contextErr error, explicitlyCanceled bool) agentexecution.JobResult {
	if explicitlyCanceled || errors.Is(contextErr, context.Canceled) {
		return agentexecution.JobResult{ExitCode: -1, Canceled: true}
	}
	if errors.Is(contextErr, context.DeadlineExceeded) {
		return agentexecution.JobResult{ExitCode: -1, Error: "job timed out"}
	}
	if waitErr == nil {
		return agentexecution.JobResult{ExitCode: 0}
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		return agentexecution.JobResult{ExitCode: exitErr.ExitCode(), Error: waitErr.Error()}
	}
	return agentexecution.JobResult{ExitCode: -1, Error: waitErr.Error()}
}

func terminateDescendants(parentPID int) error {
	var result error
	children := processChildren(parentPID)
	for _, childPID := range children {
		result = errors.Join(result, terminateDescendants(childPID), terminationSignalError(syscall.Kill(childPID, syscall.SIGKILL)))
	}
	return result
}

func processChildren(parentPID int) []int {
	value, err := os.ReadFile(fmt.Sprintf("/proc/%d/task/%d/children", parentPID, parentPID))
	if err != nil {
		return nil
	}
	fields := strings.Fields(string(value))
	result := make([]int, 0, len(fields))
	for _, field := range fields {
		pid, err := strconv.Atoi(field)
		if err == nil && pid > 1 {
			result = append(result, pid)
		}
	}
	return result
}

func mergedEnvironment(overrides map[string]string) []string {
	values := make(map[string]string)
	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if ok && !isRuntimeControlEnvironment(key) {
			values[key] = value
		}
	}
	for key, value := range overrides {
		if !isRuntimeControlEnvironment(key) {
			values[key] = value
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result
}

// Runtime Agent configuration is a control-plane secret boundary. Commands
// execute as untrusted workload code and must not inherit, or override, the
// listener, bearer token, workspace root, or protocol limits.
func isRuntimeControlEnvironment(key string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(key)), "ZGI_RUNTIME_AGENT_")
}
