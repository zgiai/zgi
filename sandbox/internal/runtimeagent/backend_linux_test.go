//go:build linux

package runtimeagent

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

func TestBackendRunsJobStreamsOutputAndIsIdempotent(t *testing.T) {
	backend := newTestBackend(t)
	if err := os.Mkdir(filepath.Join(backend.workspaceName, "work"), 0o750); err != nil {
		t.Fatalf("create work dir: %v", err)
	}
	var outputMu sync.Mutex
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	sink := func(stream agentexecution.OutputStream, value []byte) error {
		outputMu.Lock()
		defer outputMu.Unlock()
		if stream == agentexecution.OutputStreamStdout {
			_, _ = stdout.Write(value)
		} else {
			_, _ = stderr.Write(value)
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

func TestBackendCancelsProcessGroup(t *testing.T) {
	backend := newTestBackend(t)
	running, err := backend.StartJob(context.Background(), agentexecution.Allocation{}, agentexecution.JobSpec{
		JobID: "job-cancel", Command: "/bin/sh",
		Args: []string{"-c", `sleep 30 & echo $! > child.pid; wait`}, Timeout: time.Minute,
	}, func(agentexecution.OutputStream, []byte) error { return nil })
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
	}, func(_ agentexecution.OutputStream, value []byte) error {
		outputMu.Lock()
		defer outputMu.Unlock()
		_, _ = output.Write(value)
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
	}, func(agentexecution.OutputStream, []byte) error { return nil })
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
