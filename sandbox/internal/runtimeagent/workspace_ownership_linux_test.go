//go:build linux

package runtimeagent

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

func TestRootAgentHandsUploadedAndRestoredFilesToNonRootJob(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires a root Runtime Agent with non-root Jobs")
	}
	newBackend := func() *Backend {
		t.Helper()
		root := t.TempDir()
		if err := os.Chmod(filepath.Dir(root), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(root, 0o755); err != nil {
			t.Fatal(err)
		}
		backend, err := New(Options{WorkspaceRoot: root, JobUID: 65532, JobGID: 65532})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = backend.Close() })
		return backend
	}
	assertWritable := func(backend *Backend, jobID string) {
		t.Helper()
		job, err := backend.StartJob(t.Context(), agentexecution.Allocation{}, agentexecution.JobSpec{
			JobID: jobID, Command: "/bin/sh", Timeout: 5 * time.Second,
			Args: []string{"-c", "test \"$(id -u)\" = 65532 && test \"$(cat .zgi/skill-archives/pdf/input.txt)\" = test-input && printf job-result > .zgi/result.txt && printf updated > .zgi/skill-archives/pdf/input.txt"},
		}, func(agentexecution.OutputChunk) error { return nil })
		if err != nil {
			t.Fatal(err)
		}
		if result, err := job.Wait(t.Context()); err != nil || result.ExitCode != 0 {
			t.Fatalf("non-root file access = %+v, error=%v", result, err)
		}
	}
	source := newBackend()
	if _, err := source.UploadFile(t.Context(), agentexecution.Allocation{}, ".zgi/skill-archives/pdf/input.txt", bytes.NewBufferString("test-input"), 10); err != nil {
		t.Fatal(err)
	}
	archive, err := source.ExportWorkspace(t.Context(), agentexecution.Allocation{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(archive)
	_ = archive.Close()
	if err != nil {
		t.Fatal(err)
	}
	assertWritable(source, "uploaded-access")
	destination := newBackend()
	if err := destination.ImportWorkspace(t.Context(), agentexecution.Allocation{}, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	assertWritable(destination, "restored-access")
}
