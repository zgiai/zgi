package service

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"github.com/zgiai/zgi/api/internal/modules/file_process/model"
)

func temporaryUploadAttemptRows(attempt model.FileUploadAttempt) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "owner_id", "file_id", "object_key", "status", "expires_at", "created_at", "updated_at"}).
		AddRow(attempt.ID, attempt.OwnerID, attempt.FileID, attempt.ObjectKey, attempt.Status, attempt.ExpiresAt, attempt.CreatedAt, attempt.UpdatedAt)
}

func temporaryUploadFileRows(file model.UploadFile) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "organization_id", "is_temporary", "key", "created_by", "used"}).
		AddRow(file.ID, file.OrganizationID, file.IsTemporary, file.Key, file.CreatedBy, file.Used)
}

func TestTemporaryUploadCleanupNeverDeletesANonTemporaryFile(t *testing.T) {
	db, mock := openFileObjectDeletionMockDB(t)
	now := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	attempt := model.FileUploadAttempt{
		ID: uuid.NewString(), OwnerID: uuid.NewString(), FileID: uuid.NewString(),
		ObjectKey: "upload_files/org/permanent.pdf", Status: model.FileUploadAttemptCompleted,
		ExpiresAt: now.Add(-time.Minute), CreatedAt: now.Add(-25 * time.Hour), UpdatedAt: now,
	}
	file := model.UploadFile{
		ID: attempt.FileID, OrganizationID: uuid.NewString(), IsTemporary: false,
		Key: attempt.ObjectKey, CreatedBy: attempt.OwnerID,
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT .* FROM "file_upload_attempts".*FOR UPDATE`).
		WillReturnRows(temporaryUploadAttemptRows(attempt))
	mock.ExpectQuery(`(?s)SELECT .* FROM "upload_files".*`).
		WillReturnRows(temporaryUploadFileRows(file))
	// The only delete is the malformed lifecycle row. There must be no file deletion and
	// no object-deletion handoff for a durable upload.
	mock.ExpectExec(`DELETE FROM "file_upload_attempts"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	processor := NewFileUploadAttemptCleanupProcessor(db)
	processor.now = func() time.Time { return now }
	removed, err := processor.processOne(context.Background(), attempt.ID)
	if err != nil {
		t.Fatalf("processOne: %v", err)
	}
	if !removed {
		t.Fatal("processOne removed = false, want the malformed attempt discarded")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTemporaryUploadCleanupEventuallyDropsClaimedAttemptMetadata(t *testing.T) {
	db, mock := openFileObjectDeletionMockDB(t)
	now := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	attempt := model.FileUploadAttempt{
		ID: uuid.NewString(), OwnerID: uuid.NewString(), FileID: uuid.NewString(),
		ObjectKey: "upload_files/temp/claimed.pdf", Status: model.FileUploadAttemptClaimed,
		ExpiresAt: now.Add(-time.Minute), CreatedAt: now.Add(-25 * time.Hour), UpdatedAt: now,
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT .* FROM "file_upload_attempts".*FOR UPDATE`).
		WillReturnRows(temporaryUploadAttemptRows(attempt))
	mock.ExpectExec(`DELETE FROM "file_upload_attempts"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	processor := NewFileUploadAttemptCleanupProcessor(db)
	processor.now = func() time.Time { return now }
	removed, err := processor.processOne(context.Background(), attempt.ID)
	if err != nil {
		t.Fatalf("processOne: %v", err)
	}
	if !removed {
		t.Fatal("processOne removed = false, want expired claimed metadata removed")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestClaimTemporaryFilesLocksTheFileBeforeMarkingTheAttemptClaimed(t *testing.T) {
	db, mock := openFileObjectDeletionMockDB(t)
	now := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	ownerID := uuid.NewString()
	fileID := uuid.NewString()
	attempt := model.FileUploadAttempt{
		ID: uuid.NewString(), OwnerID: ownerID, FileID: fileID,
		ObjectKey: "upload_files/temp/claim.pdf", Status: model.FileUploadAttemptCompleted,
		ExpiresAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now,
	}
	file := model.UploadFile{
		ID: fileID, OrganizationID: uuid.NewString(), IsTemporary: true,
		Key: attempt.ObjectKey, CreatedBy: ownerID, Used: false,
	}

	mock.ExpectQuery(`(?s)SELECT .* FROM "file_upload_attempts".*FOR UPDATE`).
		WillReturnRows(temporaryUploadAttemptRows(attempt))
	mock.ExpectQuery(`(?s)SELECT .* FROM "upload_files".*FOR UPDATE`).
		WillReturnRows(temporaryUploadFileRows(file))
	mock.ExpectExec(`UPDATE "upload_files"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "file_upload_attempts"`).WillReturnResult(sqlmock.NewResult(0, 1))

	processor := &fileService{}
	if err := processor.ClaimTemporaryFilesInTx(context.Background(), db, []string{fileID}, ownerID, uuid.NewString()); err != nil {
		t.Fatalf("ClaimTemporaryFilesInTx: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTemporaryUploadCleanupTaskUsesBoundedInterval(t *testing.T) {
	task := &FileUploadAttemptCleanupTask{}
	if got := task.TaskType(); got != temporaryUploadCleanupTaskType {
		t.Fatalf("TaskType = %q, want %q", got, temporaryUploadCleanupTaskType)
	}
	if task.Interval() <= 0 || task.Interval() >= temporaryUploadAttemptTTL {
		t.Fatalf("Interval = %v, want a positive interval below the attempt TTL", task.Interval())
	}
	if got := task.CronSpec(); got != "" {
		t.Fatalf("CronSpec = %q, want interval scheduling", got)
	}
}
