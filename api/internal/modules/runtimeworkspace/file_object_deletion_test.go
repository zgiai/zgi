package runtimeworkspace

import (
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/pkg/storage"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type deferredDeletionStorage struct {
	deleted []string
	err     error
	// absent makes Exists report the object as already gone.
	absent bool
}

func (s *deferredDeletionStorage) Save(string, []byte) error                { return nil }
func (s *deferredDeletionStorage) Load(string) ([]byte, error)              { return nil, nil }
func (s *deferredDeletionStorage) LoadStream(string) (<-chan []byte, error) { return nil, nil }
func (s *deferredDeletionStorage) Download(string, string) error            { return nil }
func (s *deferredDeletionStorage) Exists(string) (bool, error)              { return !s.absent, nil }
func (s *deferredDeletionStorage) Delete(key string) error {
	s.deleted = append(s.deleted, key)
	return s.err
}
func (s *deferredDeletionStorage) List(string) ([]storage.FileInfo, error) { return nil, nil }

func newFileObjectDeletionTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return db, mock
}

func expectDeferredDeletionClaim(mock sqlmock.Sqlmock, id uuid.UUID, key string, attemptCount int, now time.Time) {
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT \* FROM "file_object_deletions".*next_attempt_at <= \$1.*ORDER BY next_attempt_at ASC, created_at ASC.*LIMIT \$2 FOR UPDATE SKIP LOCKED`).
		WithArgs(now, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "object_key", "attempt_count", "next_attempt_at", "last_error", "created_at", "updated_at"}).
			AddRow(id, key, attemptCount, now.Add(-time.Hour), "", now.Add(-time.Hour), now.Add(-time.Hour)))
	mock.ExpectExec(`UPDATE "file_object_deletions" SET "attempt_count"=attempt_count \+ 1,"next_attempt_at"=\$1,"updated_at"=\$2 WHERE id = \$3`).
		WithArgs(now.Add(fileObjectDeletionLease), now, id).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func expectNoDueDeferredDeletion(mock sqlmock.Sqlmock, now time.Time) {
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT \* FROM "file_object_deletions".*next_attempt_at <= \$1.*ORDER BY next_attempt_at ASC, created_at ASC.*LIMIT \$2 FOR UPDATE SKIP LOCKED`).
		WithArgs(now, 1).WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectCommit()
}

func TestFileObjectDeletionWorkerDeletesDueObject(t *testing.T) {
	db, mock := newFileObjectDeletionTestDB(t)
	now := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
	id := uuid.New()
	key := "runtime/workspace/report.pdf"
	expectDeferredDeletionClaim(mock, id, key, 0, now)
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "file_object_deletions" WHERE id = \$1`).
		WithArgs(id).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	expectNoDueDeferredDeletion(mock, now)

	store := &deferredDeletionStorage{}
	worker := NewFileObjectDeletionWorker(db, store)
	worker.now = func() time.Time { return now }
	if err := worker.RunOnce(t.Context()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(store.deleted) != 1 || store.deleted[0] != key {
		t.Fatalf("deleted keys = %#v, want %q", store.deleted, key)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFileObjectDeletionWorkerAcknowledgesProviderNotFound(t *testing.T) {
	db, mock := newFileObjectDeletionTestDB(t)
	now := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
	id := uuid.New()
	key := "runtime/workspace/already-gone.pdf"
	expectDeferredDeletionClaim(mock, id, key, 0, now)
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "file_object_deletions" WHERE id = \$1`).
		WithArgs(id).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	expectNoDueDeferredDeletion(mock, now)

	store := &deferredDeletionStorage{err: errors.New("qiniu: no such file or directory (612)"), absent: true}
	worker := NewFileObjectDeletionWorker(db, store)
	worker.now = func() time.Time { return now }
	if err := worker.RunOnce(t.Context()); err != nil {
		t.Fatalf("RunOnce() error = %v, want an already-absent object to count as deleted", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFileObjectDeletionWorkerReschedulesStorageFailure(t *testing.T) {
	db, mock := newFileObjectDeletionTestDB(t)
	now := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
	id := uuid.New()
	expectDeferredDeletionClaim(mock, id, "runtime/workspace/report.pdf", 1, now)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "file_object_deletions" SET "last_error"=\$1,"next_attempt_at"=\$2,"updated_at"=\$3 WHERE id = \$4`).
		WithArgs("storage unavailable", now.Add(fileObjectDeletionRetryBase*2), now, id).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	expectNoDueDeferredDeletion(mock, now)

	store := &deferredDeletionStorage{err: errors.New("storage unavailable")}
	worker := NewFileObjectDeletionWorker(db, store)
	worker.now = func() time.Time { return now }
	if err := worker.RunOnce(t.Context()); err == nil || !errors.Is(err, store.err) {
		t.Fatalf("RunOnce() error = %v, want storage error", err)
	}
	if len(store.deleted) != 1 {
		t.Fatalf("delete calls = %d, want 1", len(store.deleted))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFileObjectDeletionWorkerLeavesFutureObjectsUntouched(t *testing.T) {
	db, mock := newFileObjectDeletionTestDB(t)
	now := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT \* FROM "file_object_deletions".*next_attempt_at <= \$1.*ORDER BY next_attempt_at ASC, created_at ASC.*LIMIT \$2 FOR UPDATE SKIP LOCKED`).
		WithArgs(now, 1).WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectCommit()

	store := &deferredDeletionStorage{}
	worker := NewFileObjectDeletionWorker(db, store)
	worker.now = func() time.Time { return now }
	if err := worker.RunOnce(t.Context()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(store.deleted) != 0 {
		t.Fatalf("deleted keys = %#v, want none", store.deleted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFileObjectDeletionRetryDelayCaps(t *testing.T) {
	if got := fileObjectDeletionRetryDelay(1); got != fileObjectDeletionRetryBase {
		t.Fatalf("retry delay for first attempt = %v, want %v", got, fileObjectDeletionRetryBase)
	}
	if got := fileObjectDeletionRetryDelay(20); got != fileObjectDeletionRetryMax {
		t.Fatalf("retry delay cap = %v, want %v", got, fileObjectDeletionRetryMax)
	}
}
