package migrations

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestBindExecutionUploadsMigrationContract(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s)CREATE TABLE IF NOT EXISTS public\.agent_execution_uploads.*ck_agent_execution_upload_owner.*CREATE INDEX IF NOT EXISTS idx_agent_execution_upload_owner.*CREATE TRIGGER tr_execution_upload_owner`).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := upBindExecutionUploads(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBindExecutionUploadsPostgres(t *testing.T) {
	db := openExecutionFileTestPostgres(t)
	for range 2 {
		if err := upBindExecutionUploads(mschema.New(db)); err != nil {
			t.Fatal(err)
		}
	}
	owner := execution.UploadOwner{OrganizationID: uuid.NewString(), WorkspaceID: uuid.NewString(), ApplicationType: "agent",
		ApplicationID: uuid.NewString(), Entry: "webapp", SubjectKind: "account", SubjectID: uuid.NewString()}
	id := uuid.NewString()
	if err := execution.BindUpload(t.Context(), db, id, owner); err != nil {
		t.Fatal(err)
	}
	// Reapplying the migration preserves existing ownership and never adopts
	// historical generic uploads that have no execution binding.
	if err := upBindExecutionUploads(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	if err := execution.AuthorizeUpload(t.Context(), db, id, owner); err != nil {
		t.Fatal(err)
	}
	if err := execution.AuthorizeUpload(t.Context(), db, uuid.NewString(), owner); err == nil {
		t.Fatal("unbound upload adopted")
	}
	for _, field := range []string{"organization_id", "workspace_id", "application_id", "subject_id", "file_id"} {
		if err := db.Exec("UPDATE agent_execution_uploads SET "+field+" = ? WHERE file_id = ?", uuid.New(), id).Error; err == nil {
			t.Fatalf("ownership column changed: %s", field)
		}
	}
	for _, statement := range []string{
		`UPDATE agent_execution_uploads SET application_type = 'digital_human' WHERE file_id = ?`,
		`UPDATE agent_execution_uploads SET entry = 'external-api', subject_kind = 'api_end_user' WHERE file_id = ?`,
	} {
		if err := db.Exec(statement, id).Error; err == nil {
			t.Fatalf("ownership changed: %s", statement)
		}
	}
	invalid := owner
	invalid.Entry, invalid.SubjectKind = "webapp", "api_end_user"
	if err := db.Create(&execution.UploadBinding{FileID: uuid.New(), UploadOwner: invalid}).Error; err == nil {
		t.Fatal("invalid entry/user pair accepted by database")
	}
	// Separate connections racing to claim the same new upload cannot overwrite
	// ownership or both return success.
	start := make(chan struct{})
	results := make(chan error, 2)
	raceID := uuid.NewString()
	for range 2 {
		candidate := owner
		candidate.SubjectID = uuid.NewString()
		go func() {
			<-start
			results <- execution.BindUpload(t.Context(), db, raceID, candidate)
		}()
	}
	close(start)
	success := 0
	for range 2 {
		if <-results == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("concurrent upload claims succeeded=%d, want 1", success)
	}
}
