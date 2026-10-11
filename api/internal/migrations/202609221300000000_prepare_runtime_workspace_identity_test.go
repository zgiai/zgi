package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestPrepareRuntimeWorkspaceIdentityPreservesAttachedIDs(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s)ALTER TABLE public\.runtime_workspaces.*ADD COLUMN IF NOT EXISTS runtime_id`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE public\.runtime_workspaces.*SET runtime_id = workspace_id`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)ALTER TABLE public\.runtime_workspaces.*ALTER COLUMN runtime_id SET DEFAULT.*CREATE UNIQUE INDEX IF NOT EXISTS uq_runtime_workspaces_runtime_id.*DROP CONSTRAINT IF EXISTS ck_runtime_workspace_mode.*ADD CONSTRAINT ck_runtime_workspace_mode CHECK \(mode IN \('attached', 'isolated'\)\)`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := upPrepareRuntimeWorkspaceIdentity(builder); err != nil {
		t.Fatal(err)
	}
	statements := strings.Join(builder.Statements(), "\n")
	for _, required := range []string{
		"runtime_id uuid",
		"mode IN ('attached', 'isolated')",
	} {
		if !strings.Contains(statements, required) {
			t.Fatalf("Runtime Workspace identity migration is missing %q", required)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareRuntimeWorkspaceIdentityRollbackRestoresAttachedMode(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s)ALTER TABLE public\.runtime_workspaces.*DROP CONSTRAINT IF EXISTS ck_runtime_workspace_mode.*ADD CONSTRAINT ck_runtime_workspace_mode CHECK \(mode IN \('attached'\)\).*DROP INDEX IF EXISTS public\.uq_runtime_workspaces_runtime_id.*DROP COLUMN IF EXISTS runtime_id`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := downPrepareRuntimeWorkspaceIdentity(mschema.New(db).AllowDestructive()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
