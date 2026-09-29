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
	cmd        *exec.Cmd
	cancel     context.CancelFunc
	done       chan struct{}
	pty        *os.File
	ptyMu      sync.Mutex
	cancelOnce sync.Once
	canceled   atomic.Bool
	mu         sync.Mutex
	result     agentexecution.JobResult
}

func supportsPTY() bool { return true }

func startProcess(parent context.Context, workDir string, spec agentexecution.JobSpec, sink func(agentexecution.OutputStream, []byte) error) (agentexecution.RunningJob, error) {
	jobContext, cancel := context.WithCancel(parent)
	if spec.Timeout > 0 {
		jobContext, cancel = context.WithTimeout(parent, spec.Timeout)
	}
	cmd := exec.CommandContext(jobContext, spec.Command, spec.Args...)
	cmd.Dir = workDir
	cmd.Env = mergedEnvironment(spec.Environment)
	process := &runningProcess{cmd: cmd, cancel: cancel, done: make(chan struct{})}
	cmd.Cancel = func() error {
		process.kill()
		return nil
	}
	cmd.WaitDelay = time.Second

	var readers sync.WaitGroup
	if spec.PTY {
		terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: spec.PTYColumns, Rows: spec.PTYRows})
		if err != nil {
			cancel()
			return nil, fmt.Errorf("start PTY job: %w", err)
		}
		process.pty = terminal
		readers.Add(1)
		go func() {
			defer readers.Done()
			pumpOutput(terminal, agentexecution.OutputStreamStdout, sink)
		}()
	} else {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
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
		if terminal != nil {
			_ = terminal.Close()
		}
		process.ptyMu.Unlock()
		readers.Wait()
		result := processResult(waitErr, contextErr, process.canceled.Load())
		process.mu.Lock()
		process.result = result
		process.mu.Unlock()
		cancel()
		close(process.done)
	}()
	return process, nil
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
	case <-ctx.Done():
		_ = p.Cancel(context.Background())
		<-p.done
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
		terminateDescendants(p.cmd.Process.Pid)
		select {
		case <-p.done:
			return
		case <-time.After(250 * time.Millisecond):
		}
		p.cancel()
		p.kill()
	})
	return nil
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

func (p *runningProcess) kill() {
	if p.cmd == nil || p.cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
	_ = p.cmd.Process.Kill()
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

func terminateDescendants(parentPID int) {
	children := processChildren(parentPID)
	for _, childPID := range children {
		terminateDescendants(childPID)
		_ = syscall.Kill(childPID, syscall.SIGKILL)
	}
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
