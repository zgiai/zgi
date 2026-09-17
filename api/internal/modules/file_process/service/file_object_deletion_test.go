package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestFileObjectDeletionBackoffGrowsAndCaps(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 0, want: 30 * time.Second},
		{attempt: 1, want: 30 * time.Second},
		{attempt: 2, want: time.Minute},
		{attempt: 3, want: 2 * time.Minute},
		{attempt: 6, want: 16 * time.Minute},
		{attempt: 8, want: time.Hour},
		{attempt: 50, want: time.Hour},
	}

	for _, tt := range tests {
		if got := fileObjectDeletionBackoff(tt.attempt); got != tt.want {
			t.Fatalf("fileObjectDeletionBackoff(%d) = %v, want %v", tt.attempt, got, tt.want)
		}
	}
}

func TestFileObjectDeletionErrorTextIsBoundedForDiagnostics(t *testing.T) {
	if got := fileObjectDeletionErrorText(nil); got != "" {
		t.Fatalf("nil cause = %q, want an empty message", got)
	}

	cause := errors.New("storage unavailable")
	if got := fileObjectDeletionErrorText(cause); got != cause.Error() {
		t.Fatalf("short cause = %q, want %q", got, cause.Error())
	}

	long := errors.New(strings.Repeat("x", fileObjectDeletionMaxErrorLength+50))
	if got := fileObjectDeletionErrorText(long); len(got) != fileObjectDeletionMaxErrorLength {
		t.Fatalf("long cause length = %d, want %d", len(got), fileObjectDeletionMaxErrorLength)
	}
}

// The processor is a safety net, so a partially configured deployment must degrade to a
// no-op sweep instead of failing the scheduled task.
func TestFileObjectDeletionProcessorToleratesMissingDependencies(t *testing.T) {
	processor := &FileObjectDeletionProcessor{}

	removed, err := processor.ProcessPending(context.Background(), 10)
	if removed != 0 || err != nil {
		t.Fatalf("ProcessPending = (%d, %v), want (0, nil)", removed, err)
	}
	if err := processor.ProcessOne(context.Background(), "pending-id"); err != nil {
		t.Fatalf("ProcessOne = %v, want nil", err)
	}
}

func TestFileObjectDeletionTaskHandlerToleratesMissingProcessor(t *testing.T) {
	handler := &FileObjectDeletionTaskHandler{}
	if err := handler.Handle(context.Background(), nil); err != nil {
		t.Fatalf("Handle = %v, want nil", err)
	}
}

func TestFileObjectDeletionTaskSweepsOnAnInterval(t *testing.T) {
	task := &FileObjectDeletionTask{}

	if got := task.TaskType(); got != fileObjectDeletionTaskType {
		t.Fatalf("TaskType = %q, want %q", got, fileObjectDeletionTaskType)
	}
	if got := task.CronSpec(); got != "" {
		t.Fatalf("CronSpec = %q, want an interval task", got)
	}
	if task.Interval() <= 0 {
		t.Fatalf("Interval = %v, want a positive sweep interval", task.Interval())
	}
	if task.Payload() != nil {
		t.Fatalf("Payload = %v, want no payload", task.Payload())
	}
	if len(task.Options()) == 0 {
		t.Fatal("Options = empty, want the sweep bounded by retry and timeout options")
	}
}
