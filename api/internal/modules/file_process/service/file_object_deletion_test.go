package service

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/zgiai/zgi/api/internal/modules/file_process/model"
	"github.com/zgiai/zgi/api/pkg/storage"
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

// fakeDeletionStorage answers the two calls the processor makes, so the tests pin the
// decision rather than a real backend.
type fakeDeletionStorage struct {
	storage.Storage

	deleted      []string
	deleteErr    error
	exists       bool
	existsErr    error
	existsChecks []string

	// beforeDelete runs before each delete, which lets a test cancel the sweep mid-batch.
	beforeDelete func(call int)
}

func (f *fakeDeletionStorage) Delete(key string) error {
	f.deleted = append(f.deleted, key)
	if f.beforeDelete != nil {
		f.beforeDelete(len(f.deleted))
	}
	return f.deleteErr
}

func (f *fakeDeletionStorage) Exists(key string) (bool, error) {
	f.existsChecks = append(f.existsChecks, key)
	return f.exists, f.existsErr
}

func openFileObjectDeletionMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:       sqlDB,
		DriverName: "postgres",
	}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	return db, mock
}

// The backends disagree about a missing key: S3 and OSS report success, while the local and
// OpenDAL backends fail with ENOENT. A row that can never clear would make the sweep
// retry forever, so an object that is really gone counts as removed.
func TestFileObjectDeletionProcessorClearsAnObjectThatIsAlreadyAbsent(t *testing.T) {
	db, mock := openFileObjectDeletionMockDB(t)
	backend := &fakeDeletionStorage{deleteErr: os.ErrNotExist, exists: false}
	mock.ExpectExec(`(?s)DELETE FROM "file_object_deletions".*`).WillReturnResult(sqlmock.NewResult(0, 1))

	processor := NewFileObjectDeletionProcessor(db, backend)
	entry := &model.FileObjectDeletion{ID: "pending-1", ObjectKey: "upload_files/org/gone.png"}

	if err := processor.process(context.Background(), entry); err != nil {
		t.Fatalf("process = %v, want nil for an absent object", err)
	}
	if len(backend.existsChecks) != 1 || backend.existsChecks[0] != entry.ObjectKey {
		t.Fatalf("Exists checks = %v, want one check of %q", backend.existsChecks, entry.ObjectKey)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFileObjectDeletionProcessorReschedulesWhileTheObjectRemains(t *testing.T) {
	tests := []struct {
		name    string
		backend *fakeDeletionStorage
	}{
		{
			name:    "the delete failed and the object is still there",
			backend: &fakeDeletionStorage{deleteErr: errors.New("storage unavailable"), exists: true},
		},
		{
			name:    "the backend cannot say whether the object is there",
			backend: &fakeDeletionStorage{deleteErr: errors.New("storage unavailable"), existsErr: errors.New("head object failed")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := openFileObjectDeletionMockDB(t)
			mock.ExpectExec(`(?s)UPDATE "file_object_deletions".*`).WillReturnResult(sqlmock.NewResult(0, 1))

			processor := NewFileObjectDeletionProcessor(db, tt.backend)
			err := processor.process(context.Background(), &model.FileObjectDeletion{ID: "pending-2", ObjectKey: "upload_files/org/file.png"})
			if err == nil {
				t.Fatal("process = nil, want the failed attempt reported")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// An empty key can never be removed, so it must not keep a row pending forever either.
func TestFileObjectDeletionProcessorDropsARowWithoutAnObjectKey(t *testing.T) {
	db, mock := openFileObjectDeletionMockDB(t)
	backend := &fakeDeletionStorage{}
	mock.ExpectExec(`(?s)DELETE FROM "file_object_deletions".*`).WillReturnResult(sqlmock.NewResult(0, 1))

	processor := NewFileObjectDeletionProcessor(db, backend)
	if err := processor.process(context.Background(), &model.FileObjectDeletion{ID: "pending-3"}); err != nil {
		t.Fatalf("process = %v, want nil", err)
	}
	if len(backend.deleted) != 0 || len(backend.existsChecks) != 0 {
		t.Fatalf("storage calls = %v/%v, want none for an empty key", backend.deleted, backend.existsChecks)
	}
}

func TestFileObjectDeletionProcessorCountsAnAbsentObjectAsRemoved(t *testing.T) {
	db, mock := openFileObjectDeletionMockDB(t)
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`(?s)SELECT .* FROM "file_object_deletions".*`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "object_key", "attempt_count", "next_attempt_at", "last_error", "created_at", "updated_at"}).
			AddRow("pending-4", "upload_files/org/gone.png", 0, now, "", now, now))
	mock.ExpectExec(`(?s)DELETE FROM "file_object_deletions".*`).WillReturnResult(sqlmock.NewResult(0, 1))

	processor := NewFileObjectDeletionProcessor(db, &fakeDeletionStorage{deleteErr: os.ErrNotExist, exists: false})
	removed, err := processor.ProcessPending(context.Background(), 10)
	if err != nil {
		t.Fatalf("ProcessPending = %v, want nil", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// A cancelled sweep must stop before touching more objects: a storage delete cannot be
// interrupted, so the remaining rows are better left for the next sweep.
func TestFileObjectDeletionProcessorStopsSweepingOnceTheContextIsCancelled(t *testing.T) {
	db, mock := openFileObjectDeletionMockDB(t)
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "object_key", "attempt_count", "next_attempt_at", "last_error", "created_at", "updated_at"}).
		AddRow("pending-5", "upload_files/org/first.png", 0, now, "", now, now).
		AddRow("pending-6", "upload_files/org/second.png", 0, now, "", now, now)
	// Only the first object is attempted; the second one waits for the next sweep. The
	// attempt that was already in flight cannot record itself either, because the cancelled
	// context rejects the write, so the row keeps its attempt count for the next sweep.
	mock.ExpectQuery(`(?s)SELECT .* FROM "file_object_deletions".*`).WillReturnRows(rows)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	backend := &fakeDeletionStorage{deleteErr: errors.New("storage unavailable"), exists: true}
	backend.beforeDelete = func(call int) {
		if call == 1 {
			// The deadline fires while the first object is being removed.
			cancel()
		}
	}

	processor := NewFileObjectDeletionProcessor(db, backend)
	if _, err := processor.ProcessPending(ctx, 10); err != nil {
		t.Fatalf("ProcessPending = %v, want nil", err)
	}
	if len(backend.deleted) != 1 {
		t.Fatalf("storage deletes = %v, want only the first object attempted", backend.deleted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
