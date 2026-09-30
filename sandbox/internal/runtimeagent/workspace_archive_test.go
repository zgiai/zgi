package runtimeagent

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

func TestWorkspaceArchiveRoundTrip(t *testing.T) {
	sourceDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(sourceDir, "input"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "input", "first.csv"), []byte("a,b\n1,2\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "result.json"), []byte(`{"sum":3}`), 0o640); err != nil {
		t.Fatal(err)
	}
	source, err := New(Options{WorkspaceRoot: sourceDir})
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	stream, err := source.ExportWorkspace(context.Background(), agentexecution.Allocation{})
	if err != nil {
		t.Fatal(err)
	}
	archive, err := io.ReadAll(stream)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	destinationDir := t.TempDir()
	destination, err := New(Options{WorkspaceRoot: destinationDir})
	if err != nil {
		t.Fatal(err)
	}
	defer destination.Close()
	if err := destination.ImportWorkspace(context.Background(), agentexecution.Allocation{}, bytes.NewReader(archive)); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{"input/first.csv": "a,b\n1,2\n", "result.json": `{"sum":3}`} {
		got, err := os.ReadFile(filepath.Join(destinationDir, filepath.FromSlash(path)))
		if err != nil || string(got) != want {
			t.Fatalf("restored %s = %q, %v", path, got, err)
		}
	}
	if err := destination.ImportWorkspace(context.Background(), agentexecution.Allocation{}, bytes.NewReader(archive)); !errors.Is(err, agentexecution.ErrInvalidTransition) {
		t.Fatalf("second import error = %v", err)
	}
}

func TestWorkspaceArchiveRejectsUnsafeEntries(t *testing.T) {
	for _, test := range []struct {
		name  string
		entry tar.Header
	}{
		{"escape", tar.Header{Name: "../outside", Typeflag: tar.TypeReg, Size: 1}},
		{"absolute", tar.Header{Name: "/outside", Typeflag: tar.TypeReg, Size: 1}},
		{"symlink", tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "../outside"}},
		{"device", tar.Header{Name: "device", Typeflag: tar.TypeChar}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var buffer bytes.Buffer
			writer := tar.NewWriter(&buffer)
			if err := writer.WriteHeader(&test.entry); err != nil {
				t.Fatal(err)
			}
			if test.entry.Size != 0 {
				_, _ = writer.Write([]byte("x"))
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			backend, err := New(Options{WorkspaceRoot: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			defer backend.Close()
			if err := backend.ImportWorkspace(context.Background(), agentexecution.Allocation{}, &buffer); !errors.Is(err, agentexecution.ErrInvalidPath) {
				t.Fatalf("import error = %v", err)
			}
		})
	}
}

func TestWorkspaceArchiveRejectsSourceSymlink(t *testing.T) {
	workspace := t.TempDir()
	if err := os.Symlink("target", filepath.Join(workspace, "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	backend, err := New(Options{WorkspaceRoot: workspace})
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	reader, err := backend.ExportWorkspace(context.Background(), agentexecution.Allocation{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(io.Discard, reader)
	_ = reader.Close()
	if !errors.Is(err, agentexecution.ErrInvalidPath) {
		t.Fatalf("export error = %v", err)
	}
}

func TestWorkspaceArchiveRejectsOversizedEntryBeforeReadingPayload(t *testing.T) {
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	if err := writer.WriteHeader(&tar.Header{
		Name: "huge.bin", Typeflag: tar.TypeReg, Size: maxWorkspaceArchiveBytes + 1,
	}); err != nil {
		t.Fatal(err)
	}
	backend, err := New(Options{WorkspaceRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	if err := backend.ImportWorkspace(context.Background(), agentexecution.Allocation{}, &buffer); !errors.Is(err, agentexecution.ErrOutputLimit) {
		t.Fatalf("oversized entry error = %v", err)
	}
}
