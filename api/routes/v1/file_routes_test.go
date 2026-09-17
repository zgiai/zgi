package v1

import (
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	pkgscheduler "github.com/zgiai/zgi/api/pkg/scheduler"
	"github.com/zgiai/zgi/api/pkg/storage"
)

// stubFileObjectStorage only needs to be non-nil: the registration branch hands it to the
// processor and never calls it.
type stubFileObjectStorage struct {
	storage.Storage
}

type recordingFileObjectDeletionRegistrar struct {
	registered int
	taskType   string
	interval   time.Duration
	err        error
}

func (r *recordingFileObjectDeletionRegistrar) RegisterTask(task pkgscheduler.ScheduledTask, _ pkgscheduler.TaskHandler) error {
	if r.err != nil {
		return r.err
	}
	r.registered++
	r.taskType = task.TaskType()
	r.interval = task.Interval()
	return nil
}

func TestRegisterFileObjectDeletionRetrySchedulesTheSweep(t *testing.T) {
	registrar := &recordingFileObjectDeletionRegistrar{}

	registerFileObjectDeletionRetry(registrar, &gorm.DB{}, stubFileObjectStorage{})

	if registrar.registered != 1 {
		t.Fatalf("registered tasks = %d, want 1", registrar.registered)
	}
	// Pinned as a literal because the task type is the wire contract the scheduler stores.
	if got, want := registrar.taskType, "files:object_deletion_retry"; got != want {
		t.Fatalf("task type = %q, want %q", got, want)
	}
	if registrar.interval <= 0 {
		t.Fatalf("interval = %v, want a positive sweep interval", registrar.interval)
	}
}

// The retry worker is a safety net, so a scheduler that refuses the task must not stop the
// API from booting: the pending rows stay durable and a restart retries the registration.
func TestRegisterFileObjectDeletionRetrySurvivesRegistrationFailure(t *testing.T) {
	registrar := &recordingFileObjectDeletionRegistrar{err: errors.New("scheduler unavailable")}

	registerFileObjectDeletionRetry(registrar, &gorm.DB{}, stubFileObjectStorage{})

	if registrar.registered != 0 {
		t.Fatalf("registered tasks = %d, want 0", registrar.registered)
	}
}

func TestRegisterFileObjectDeletionRetrySkipsIncompleteWiring(t *testing.T) {
	recording := &recordingFileObjectDeletionRegistrar{}
	registerFileObjectDeletionRetry(recording, &gorm.DB{}, stubFileObjectStorage{})
	if recording.registered != 1 {
		t.Fatalf("registered tasks = %d, want 1 for a fully wired call", recording.registered)
	}

	skipped := &recordingFileObjectDeletionRegistrar{}
	registerFileObjectDeletionRetry(nil, &gorm.DB{}, stubFileObjectStorage{})
	registerFileObjectDeletionRetry(skipped, nil, stubFileObjectStorage{})
	registerFileObjectDeletionRetry(skipped, &gorm.DB{}, nil)

	if skipped.registered != 0 {
		t.Fatalf("registered tasks = %d, want 0 without scheduler, DB, or storage", skipped.registered)
	}
}
