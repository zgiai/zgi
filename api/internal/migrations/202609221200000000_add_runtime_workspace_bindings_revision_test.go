package migrations

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAddRuntimeWorkspaceBindingsRevision(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s)ALTER TABLE public\.runtime_workspaces.*ADD COLUMN IF NOT EXISTS bindings_revision bigint NOT NULL DEFAULT 0`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := upAddRuntimeWorkspaceBindingsRevision(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAddRuntimeWorkspaceBindingsRevisionRollback(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s)ALTER TABLE public\.runtime_workspaces.*DROP COLUMN IF EXISTS bindings_revision`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := downAddRuntimeWorkspaceBindingsRevision(mschema.New(db).AllowDestructive()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
