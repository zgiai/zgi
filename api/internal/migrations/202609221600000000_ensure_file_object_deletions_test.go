package migrations

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestEnsureFileObjectDeletionsIsIdempotent(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s)CREATE TABLE IF NOT EXISTS public\.file_object_deletions.*CREATE INDEX IF NOT EXISTS idx_file_object_deletions_ready`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := upEnsureFileObjectDeletions(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureFileObjectDeletionsRollbackKeepsSharedQueue(t *testing.T) {
	if err := downEnsureFileObjectDeletions(nil); err != nil {
		t.Fatal(err)
	}
}
