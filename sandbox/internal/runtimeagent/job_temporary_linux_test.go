//go:build linux

package runtimeagent

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

func TestJobTemporaryTerminalPaths(t *testing.T) {
	for _, kind := range []string{"success", "failure", "cancel", "timeout", "close", "start-failure"} {
		t.Run(kind, func(t *testing.T) {
			b := newTestBackend(t)
			spec := agentexecution.JobSpec{JobID: kind, Command: "sh", Args: []string{"-ec", `test "$TMPDIR" = "$TEMP"; test "$TMPDIR" = "$TMP"; printf '%s' "$TMPDIR" > temp-path; truncate -s 80000000 "$TMPDIR/build"`}, Environment: map[string]string{"TMPDIR": "/tmp/untrusted"}, Timeout: time.Minute}
			switch kind {
			case "failure":
				spec.Args[1] += "; exit 7"
			case "cancel", "timeout", "close":
				spec.Args[1] += "; sleep 60 & wait"
			case "start-failure":
				spec.Command = "/missing/zgi-test-command"
			}
			if kind == "timeout" {
				spec.Timeout = 300 * time.Millisecond
			}
			job, err := b.StartJob(t.Context(), agentexecution.Allocation{}, spec, func(agentexecution.OutputChunk) error { return nil })
			if kind == "start-failure" {
				if err == nil {
					t.Fatal("missing command started")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				end := time.Now().Add(3 * time.Second)
				for {
					if _, err := b.workspace.Stat("temp-path"); err == nil {
						break
					}
					if time.Now().After(end) {
						t.Fatal("temporary probe did not start")
					}
					time.Sleep(5 * time.Millisecond)
				}
				if kind == "cancel" {
					if err := job.Cancel(t.Context()); err != nil {
						t.Fatal(err)
					}
				}
				if kind == "close" {
					if err := b.Close(); err != nil {
						t.Fatal(err)
					}
				}
				result, err := job.Wait(t.Context())
				if err != nil || result.TerminationFailed || (kind == "success" && result.ExitCode != 0) || (kind == "failure" && result.ExitCode != 7) || (kind == "cancel" && !result.Canceled) || (kind == "timeout" && result.Error != "job timed out") {
					t.Fatalf("terminal=%s result=%+v error=%v", kind, result, err)
				}
			}
			entries, err := os.ReadDir(filepath.Join(b.workspaceName, jobTemporaryDirectory))
			if err != nil || len(entries) != 0 {
				t.Fatalf("temporary residue=%v error=%v", entries, err)
			}
		})
	}
}

func TestJobTemporaryConcurrentAndIdempotent(t *testing.T) {
	b := newTestBackend(t)
	start := func(id string) agentexecution.RunningJob {
		t.Helper()
		job, err := b.StartJob(t.Context(), agentexecution.Allocation{}, agentexecution.JobSpec{JobID: id, Command: "sh", Args: []string{"-ec", `echo "$TMPDIR" > ` + id + `; sleep 60 & wait`}}, func(agentexecution.OutputChunk) error { return nil })
		if err != nil {
			t.Fatal(err)
		}
		return job
	}
	a, c := start("a"), start("c")
	if same := start("a"); same != a {
		t.Fatal("idempotent start created another owner")
	}
	entries, err := os.ReadDir(filepath.Join(b.workspaceName, jobTemporaryDirectory))
	if err != nil || len(entries) != 2 {
		t.Fatalf("concurrent directories=%v error=%v", entries, err)
	}
	if err := a.Cancel(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	entries, err = os.ReadDir(filepath.Join(b.workspaceName, jobTemporaryDirectory))
	if err != nil || len(entries) != 1 {
		t.Fatalf("canceled one Job removed another directory: %v error=%v", entries, err)
	}
	_ = c.Cancel(t.Context())
	_, _ = c.Wait(t.Context())
}

func TestJobTemporaryRecoveryAndCheckpoint(t *testing.T) {
	b := newTestBackend(t)
	stale := jobTemporaryDirectory + "/job-" + strings.Repeat("a", 32)
	if err := b.workspace.MkdirAll(stale, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := b.workspace.WriteFile(stale+"/interrupted-build", []byte("discard"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := b.workspace.WriteFile("business.txt", []byte("retain"), 0o600); err != nil {
		t.Fatal(err)
	}
	job, err := b.StartJob(t.Context(), agentexecution.Allocation{}, agentexecution.JobSpec{JobID: "recovered", Command: "true"}, func(agentexecution.OutputChunk) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := job.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := b.workspace.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale temporary data not reaped: %v", err)
	}
	archive, err := b.ExportWorkspace(t.Context(), agentexecution.Allocation{})
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	reader := tar.NewReader(archive)
	found := false
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if isJobTemporaryPath(header.Name) {
			t.Fatalf("temporary data in checkpoint: %s", header.Name)
		}
		found = found || header.Name == "business.txt"
	}
	if !found {
		t.Fatal("business output missing")
	}
	var raw bytes.Buffer
	writer := tar.NewWriter(&raw)
	_ = writer.WriteHeader(&tar.Header{Name: stale, Typeflag: tar.TypeDir, Mode: 0o700})
	_ = writer.Close()
	destination := newTestBackend(t)
	if err := destination.ImportWorkspace(context.Background(), agentexecution.Allocation{}, bytes.NewReader(raw.Bytes())); !errors.Is(err, agentexecution.ErrInvalidPath) {
		t.Fatalf("reserved checkpoint path accepted: %v", err)
	}
}

func TestJobTemporaryRejectsParentLinkAndDoesNotFollowChildLinks(t *testing.T) {
	b := newTestBackend(t)
	outside := t.TempDir()
	keep := filepath.Join(outside, "keep.txt")
	if err := os.WriteFile(keep, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := b.workspace.Symlink(outside, jobTemporaryDirectory); err != nil {
		t.Fatal(err)
	}
	_, err := b.StartJob(t.Context(), agentexecution.Allocation{}, agentexecution.JobSpec{JobID: "unsafe", Command: "true"}, func(agentexecution.OutputChunk) error { return nil })
	if err == nil {
		t.Fatal("temporary parent link accepted")
	}
	if err := b.workspace.Remove(jobTemporaryDirectory); err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	_, cleanup, err := b.prepareJobTemporaryDirectory()
	b.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := b.workspace.Open(jobTemporaryDirectory)
	names, _ := entries.ReadDir(-1)
	_ = entries.Close()
	if err := b.workspace.Symlink(outside, jobTemporaryDirectory+"/"+names[0].Name()+"/link"); err != nil {
		t.Fatal(err)
	}
	if err := cleanup(discardTemporary); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(keep); err != nil || string(data) != "keep" {
		t.Fatalf("outside file modified: %q error=%v", data, err)
	}
}

func TestJobTemporaryRetainsDataWhenTerminationIsUncertain(t *testing.T) {
	b := newTestBackend(t)
	b.mu.Lock()
	directory, release, err := b.prepareJobTemporaryDirectory()
	b.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err := release(retainTemporary); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(directory); err != nil {
		t.Fatalf("unsafe termination discarded temporary data: %v", err)
	}
}
