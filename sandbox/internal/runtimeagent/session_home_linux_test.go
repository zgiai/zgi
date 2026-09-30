//go:build linux

package runtimeagent

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

func TestSessionHomeSurvivesWorkspaceCheckpoint(t *testing.T) {
	sourceDir := t.TempDir()
	source, err := New(Options{WorkspaceRoot: sourceDir})
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	write, err := source.StartJob(context.Background(), agentexecution.Allocation{}, agentexecution.JobSpec{
		JobID: "write-home", Command: "/bin/sh",
		Args:        []string{"-c", `printf 'prepared-result' > "$HOME/package.txt"`},
		Environment: map[string]string{"HOME": "/tmp/unsafe-override"}, Timeout: 5 * time.Second,
	}, func(agentexecution.OutputChunk) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	result, err := write.Wait(context.Background())
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("write home = %+v, %v", result, err)
	}
	if data, err := os.ReadFile(filepath.Join(sourceDir, sessionHomeDirectory, "package.txt")); err != nil || string(data) != "prepared-result" {
		t.Fatalf("session home file = %q, %v", data, err)
	}
	archive, err := source.ExportWorkspace(context.Background(), agentexecution.Allocation{})
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(archive)
	if err := errors.Join(readErr, archive.Close()); err != nil {
		t.Fatal(err)
	}
	destination, err := New(Options{WorkspaceRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer destination.Close()
	if err := destination.ImportWorkspace(context.Background(), agentexecution.Allocation{}, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	read, err := destination.StartJob(context.Background(), agentexecution.Allocation{}, agentexecution.JobSpec{
		JobID: "read-home", Command: "/bin/sh", Args: []string{"-c", `cat "$HOME/package.txt"`}, Timeout: 5 * time.Second,
	}, func(chunk agentexecution.OutputChunk) error {
		_, err := output.Write(chunk.Data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err = read.Wait(context.Background())
	if err != nil || result.ExitCode != 0 || output.String() != "prepared-result" {
		t.Fatalf("restored home = %+v, output=%q, error=%v", result, output.String(), err)
	}
}
