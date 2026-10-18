//go:build linux

package runtimeagent

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

func TestTerminationErrorsDistinguishExitedFromPermissionDenied(t *testing.T) {
	for _, err := range []error{nil, syscall.ESRCH, os.ErrProcessDone} {
		if got := terminationSignalError(err); got != nil {
			t.Fatalf("normalize %v: %v", err, got)
		}
	}
	if err := terminationSignalError(syscall.EPERM); !errors.Is(err, syscall.EPERM) {
		t.Fatalf("permission error was discarded: %v", err)
	}
}

func TestTerminationFailureReturnsWithoutWaitingAndQuarantinesExecution(t *testing.T) {
	backend := newTestBackend(t)
	p := &runningProcess{done: make(chan struct{}), stopFailed: make(chan struct{}), onUnsafe: backend.quarantine}
	p.markTerminationFailure(syscall.EPERM)
	p.markTerminationFailure(syscall.ESRCH)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	result, err := p.Wait(ctx)
	if !errors.Is(err, syscall.EPERM) || !result.TerminationFailed || result.Canceled {
		t.Fatalf("failure result=%+v error=%v", result, err)
	}
	_, err = backend.StartJob(ctx, agentexecution.Allocation{}, agentexecution.JobSpec{JobID: "unsafe", Command: "true"}, func(agentexecution.OutputChunk) error { return nil })
	if !errors.Is(err, agentexecution.ErrBackendUnavailable) {
		t.Fatalf("unsafe environment accepted job: %v", err)
	}
	// File transfer is deliberately unaffected by execution quarantine.
	if err := backend.workspace.WriteFile("preserved.txt", []byte("retained"), 0o600); err != nil {
		t.Fatal(err)
	}
	reader, _, err := backend.DownloadFile(ctx, agentexecution.Allocation{}, "preserved.txt")
	if err != nil {
		t.Fatalf("quarantine disabled file retrieval: %v", err)
	}
	_ = reader.Close()
}
