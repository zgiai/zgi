//go:build linux

package runtimeagent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/egressproxy"
)

func TestBackendRunsJobStreamsOutputAndIsIdempotent(t *testing.T) {
	backend := newTestBackend(t)
	if err := os.Mkdir(filepath.Join(backend.workspaceName, "work"), 0o750); err != nil {
		t.Fatalf("create work dir: %v", err)
	}
	var outputMu sync.Mutex
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	sink := func(chunk agentexecution.OutputChunk) error {
		outputMu.Lock()
		defer outputMu.Unlock()
		if chunk.Stream == agentexecution.OutputStreamStdout {
			_, _ = stdout.Write(chunk.Data)
		} else {
			_, _ = stderr.Write(chunk.Data)
		}
		return nil
	}
	spec := agentexecution.JobSpec{
		JobID: "job-1", Command: "/bin/sh",
		Args:        []string{"-c", `printf '%s:%s' "$VALUE" "$PWD"; printf 'warning' >&2`},
		Environment: map[string]string{"VALUE": "ok"}, WorkDir: "work", Timeout: time.Second,
	}
	running, err := backend.StartJob(context.Background(), agentexecution.Allocation{}, spec, sink)
	if err != nil {
		t.Fatalf("start job: %v", err)
	}
	result, err := running.Wait(context.Background())
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("wait job: result=%+v error=%v", result, err)
	}
	if !strings.Contains(stdout.String(), "ok:") || stderr.String() != "warning" {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	same, err := backend.StartJob(context.Background(), agentexecution.Allocation{}, spec, sink)
	if err != nil || same != running {
		t.Fatalf("idempotent start: same=%v error=%v", same == running, err)
	}
	spec.Command = "/bin/false"
	if _, err := backend.StartJob(context.Background(), agentexecution.Allocation{}, spec, sink); !errors.Is(err, agentexecution.ErrIdempotencyConflict) {
		t.Fatalf("changed job error = %v, want idempotency conflict", err)
	}
}

func TestBackendAdvertisesDependencyPreparationOnlyWhenEnabled(t *testing.T) {
	backend, err := New(Options{WorkspaceRoot: t.TempDir(), NetworkMode: "disabled"})
	if err != nil {
		t.Fatalf("new disabled backend: %v", err)
	}
	caps, err := backend.Capabilities(context.Background(), agentexecution.Allocation{})
	if err != nil || caps.DependencyPreparation || caps.EgressProxyAddress != "" {
		t.Fatalf("disabled capabilities = %+v, error=%v", caps, err)
	}
	_ = backend.Close()

	backend, err = New(Options{WorkspaceRoot: t.TempDir(), NetworkMode: "disabled", DependencyPreparation: true})
	if err != nil {
		t.Fatalf("new enabled backend: %v", err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	caps, err = backend.Capabilities(context.Background(), agentexecution.Allocation{})
	if err != nil || !caps.DependencyPreparation || caps.DependencyPreparationMode != DependencyPreparationModeOfflineBundled || caps.NetworkMode != "disabled" {
		t.Fatalf("enabled capabilities = %+v, error=%v", caps, err)
	}
}

func TestBackendRejectsUnknownDependencyPreparationMode(t *testing.T) {
	backend, err := New(Options{WorkspaceRoot: t.TempDir(), NetworkMode: "disabled", DependencyPreparation: true, DependencyPreparationMode: "controlled-http"})
	if backend != nil || !errors.Is(err, agentexecution.ErrBackendUnavailable) {
		t.Fatalf("unknown dependency mode result = backend=%v error=%v", backend, err)
	}
}

func TestControlledHTTPProxyUsesHostVisibleBridge(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "egress.sock")
	backend, err := New(Options{WorkspaceRoot: t.TempDir(), NetworkMode: "controlled-http", EgressProxyAddress: "127.0.0.1:0", EgressSocketPath: socketPath})
	if err != nil {
		t.Fatalf("new controlled backend: %v", err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	caps, err := backend.Capabilities(context.Background(), agentexecution.Allocation{})
	if err != nil || caps.NetworkMode != "controlled-http" || caps.EgressProxyAddress == "" {
		t.Fatalf("controlled capabilities = %+v, error=%v", caps, err)
	}
	bridgeReady := make(chan error, 1)
	go func() {
		connection, dialErr := net.Dial("unix", socketPath)
		if dialErr != nil {
			bridgeReady <- dialErr
			return
		}
		defer connection.Close()
		frame, readErr := egressproxy.ReadFrame(connection)
		if readErr != nil {
			bridgeReady <- readErr
			return
		}
		if frame.Type != egressproxy.FrameTypeConnect || frame.Target != "packages.example:443" {
			bridgeReady <- fmt.Errorf("unexpected bridge frame: %+v", frame)
			return
		}
		if writeErr := egressproxy.WriteFrame(connection, egressproxy.Frame{Type: egressproxy.FrameTypeResult, OK: true}); writeErr != nil {
			bridgeReady <- writeErr
			return
		}
		buffer := make([]byte, 4)
		if _, readErr := io.ReadFull(connection, buffer); readErr != nil {
			bridgeReady <- readErr
			return
		}
		_, writeErr := connection.Write([]byte("pong"))
		bridgeReady <- writeErr
	}()
	proxy, err := net.Dial("tcp", backend.egressProxyAddress)
	if err != nil {
		t.Fatalf("dial loopback proxy: %v", err)
	}
	defer proxy.Close()
	if _, err := io.WriteString(proxy, "CONNECT packages.example:443 HTTP/1.1\r\nHost: packages.example:443\r\n\r\n"); err != nil {
		t.Fatalf("write CONNECT: %v", err)
	}
	response := make([]byte, 64)
	count, err := proxy.Read(response)
	if err != nil || !strings.Contains(string(response[:count]), "200 Connection Established") {
		t.Fatalf("proxy response=%q error=%v", response[:count], err)
	}
	if _, err := proxy.Write([]byte("ping")); err != nil {
		t.Fatalf("write tunnel: %v", err)
	}
	pong := make([]byte, 4)
	if _, err := io.ReadFull(proxy, pong); err != nil || string(pong) != "pong" {
		t.Fatalf("tunnel response=%q error=%v", pong, err)
	}
	if err := <-bridgeReady; err != nil {
		t.Fatalf("bridge: %v", err)
	}
}

func TestControlledJobForcesLoopbackProxyEnvironment(t *testing.T) {
	backend, err := New(Options{WorkspaceRoot: t.TempDir(), NetworkMode: "controlled-http", EgressProxyAddress: "127.0.0.1:0", EgressSocketPath: filepath.Join(t.TempDir(), "egress.sock")})
	if err != nil {
		t.Fatalf("new controlled backend: %v", err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	var output bytes.Buffer
	running, err := backend.StartJob(context.Background(), agentexecution.Allocation{}, agentexecution.JobSpec{
		JobID: "job-controlled-env", Command: "/bin/sh",
		Args:        []string{"-c", `printf '%s|%s|%s|%s' "$HTTP_PROXY" "$HTTPS_PROXY" "$ALL_PROXY" "$NO_PROXY"`},
		Environment: map[string]string{"HTTP_PROXY": "http://bypass.invalid:8080", "NO_PROXY": "*"}, Timeout: time.Second,
	}, func(chunk agentexecution.OutputChunk) error {
		_, _ = output.Write(chunk.Data)
		return nil
	})
	if err != nil {
		t.Fatalf("start controlled job: %v", err)
	}
	result, err := running.Wait(context.Background())
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("controlled job result=%+v error=%v", result, err)
	}
	parts := strings.Split(output.String(), "|")
	if len(parts) != 4 || parts[0] != "http://"+backend.egressProxyAddress || parts[1] != parts[0] || parts[2] != parts[0] || parts[3] != "127.0.0.1,localhost" {
		t.Fatalf("controlled proxy environment = %q", output.String())
	}
}

func TestBackendDoesNotExposeRuntimeControlEnvironment(t *testing.T) {
	t.Setenv("ZGI_RUNTIME_AGENT_TOKEN", "host-secret")
	t.Setenv("ZGI_RUNTIME_AGENT_LISTEN", "tcp://control-only")
	backend := newTestBackend(t)
	var output bytes.Buffer
	running, err := backend.StartJob(context.Background(), agentexecution.Allocation{}, agentexecution.JobSpec{
		JobID: "job-secret-boundary", Command: "/bin/sh",
		Args: []string{"-c", `printf '%s:%s:%s' "${ZGI_RUNTIME_AGENT_TOKEN-unset}" "${ZGI_RUNTIME_AGENT_LISTEN-unset}" "${VISIBLE-unset}"`},
		Environment: map[string]string{
			"ZGI_RUNTIME_AGENT_TOKEN": "override-secret",
			"VISIBLE":                 "ok",
		},
		Timeout: time.Second,
	}, func(chunk agentexecution.OutputChunk) error {
		_, _ = output.Write(chunk.Data)
		return nil
	})
	if err != nil {
		t.Fatalf("start job: %v", err)
	}
	result, err := running.Wait(context.Background())
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("wait job: result=%+v error=%v", result, err)
	}
	if output.String() != "unset:unset:ok" {
		t.Fatalf("job environment = %q", output.String())
	}
}

func TestBackendCancelsProcessGroup(t *testing.T) {
	backend := newTestBackend(t)
	running, err := backend.StartJob(context.Background(), agentexecution.Allocation{}, agentexecution.JobSpec{
		JobID: "job-cancel", Command: "/bin/sh",
		Args: []string{"-c", `sleep 30 & echo $! > child.pid; wait`}, Timeout: time.Minute,
	}, func(agentexecution.OutputChunk) error { return nil })
	if err != nil {
		t.Fatalf("start job: %v", err)
	}
	childPID := waitForChildPID(t, filepath.Join(backend.workspaceName, "child.pid"))
	if err := running.Cancel(context.Background()); err != nil {
		t.Fatalf("cancel job: %v", err)
	}
	result, err := running.Wait(context.Background())
	if err != nil || !result.Canceled {
		t.Fatalf("wait canceled job: result=%+v error=%v", result, err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(childPID, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("child process %d survived process-group cancellation", childPID)
}

func TestBackendPTYAndTimeout(t *testing.T) {
	backend := newTestBackend(t)
	var output bytes.Buffer
	var outputMu sync.Mutex
	running, err := backend.StartJob(context.Background(), agentexecution.Allocation{}, agentexecution.JobSpec{
		JobID: "job-pty", Command: "/bin/sh", Args: []string{"-c", "printf 'pty-ok'"},
		PTY: true, PTYColumns: 80, PTYRows: 24, Timeout: time.Second,
	}, func(chunk agentexecution.OutputChunk) error {
		outputMu.Lock()
		defer outputMu.Unlock()
		_, _ = output.Write(chunk.Data)
		return nil
	})
	if err != nil {
		t.Fatalf("start PTY job: %v", err)
	}
	if err := running.ResizePTY(context.Background(), 100, 40); err != nil {
		t.Fatalf("resize PTY: %v", err)
	}
	result, err := running.Wait(context.Background())
	if err != nil || result.ExitCode != 0 || !strings.Contains(output.String(), "pty-ok") {
		t.Fatalf("PTY result=%+v output=%q error=%v", result, output.String(), err)
	}

	running, err = backend.StartJob(context.Background(), agentexecution.Allocation{}, agentexecution.JobSpec{
		JobID: "job-timeout", Command: "/bin/sh", Args: []string{"-c", "sleep 30"}, Timeout: 50 * time.Millisecond,
	}, func(agentexecution.OutputChunk) error { return nil })
	if err != nil {
		t.Fatalf("start timeout job: %v", err)
	}
	result, err = running.Wait(context.Background())
	if err != nil || result.Error != "job timed out" {
		t.Fatalf("timeout result=%+v error=%v", result, err)
	}
}

func TestBackendFileTransferStaysWithinWorkspace(t *testing.T) {
	backend := newTestBackend(t)
	content := []byte("artifact")
	info, err := backend.UploadFile(context.Background(), agentexecution.Allocation{}, "results/out.txt", bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("upload file: %v", err)
	}
	reader, downloaded, err := backend.DownloadFile(context.Background(), agentexecution.Allocation{}, info.Path)
	if err != nil {
		t.Fatalf("download file: %v", err)
	}
	value, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if readErr != nil || !bytes.Equal(value, content) || downloaded.Size != int64(len(content)) {
		t.Fatalf("download=%q info=%+v error=%v", value, downloaded, readErr)
	}

	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(backend.workspaceName, "escape")); err != nil {
		t.Fatalf("create escape symlink: %v", err)
	}
	if _, err := backend.UploadFile(context.Background(), agentexecution.Allocation{}, "escape/out.txt", bytes.NewReader(content), int64(len(content))); err == nil {
		t.Fatal("upload through escaping symlink succeeded")
	}
	if _, err := os.Stat(filepath.Join(outside, "out.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outside file exists or stat failed: %v", err)
	}
}

func newTestBackend(t *testing.T) *Backend {
	t.Helper()
	backend, err := New(Options{WorkspaceRoot: t.TempDir(), NetworkMode: "disabled"})
	if err != nil {
		t.Fatalf("new backend: %v", err)
	}
	t.Cleanup(func() {
		if err := backend.Close(); err != nil {
			t.Errorf("close backend: %v", err)
		}
	})
	return backend
}

func waitForChildPID(t *testing.T, filePath string) int {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		value, err := os.ReadFile(filePath)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(value)))
			if parseErr == nil && pid > 1 {
				return pid
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("child PID was not written")
	return 0
}
