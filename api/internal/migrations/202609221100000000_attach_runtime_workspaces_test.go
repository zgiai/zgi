package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAttachRuntimeWorkspacesMigratesModeAndCredentialOperations(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s)ALTER TABLE public\.runtime_workspaces.*ADD COLUMN IF NOT EXISTS mode`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE public\.runtime_workspaces SET mode = 'attached'`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)ALTER TABLE public\.runtime_workspaces.*ADD CONSTRAINT ck_runtime_workspace_mode.*CREATE TABLE IF NOT EXISTS public\.runtime_workspace_credential_operations`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	builder := mschema.New(db)
	if err := upAttachRuntimeWorkspaces(builder); err != nil {
		t.Fatal(err)
	}
	statements := strings.Join(builder.Statements(), "\n")
	for _, required := range []string{"mode IN ('attached')", "idempotency_key_hash", "request_fingerprint", "secret_envelope", "secret_expires_at", "uq_runtime_workspace_credential_operation_key"} {
		if !strings.Contains(statements, required) {
			t.Fatalf("attached Runtime Workspace migration is missing %q", required)
		}
	}
}

func TestAttachRuntimeWorkspacesRollbackRestoresLegacyColumn(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`DROP TABLE IF EXISTS "public"\."runtime_workspace_credential_operations"`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)ALTER TABLE public\.runtime_workspaces.*ADD COLUMN IF NOT EXISTS runtime_only.*DROP COLUMN IF EXISTS mode`).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := downAttachRuntimeWorkspaces(mschema.New(db).AllowDestructive()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
