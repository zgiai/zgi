package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestRepointRuntimeWorkspaceForeignKeysUsesLogicalRuntimeID(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s)ALTER TABLE public\.runtime_workspaces.*DROP CONSTRAINT IF EXISTS uq_runtime_workspaces_runtime_id.*ADD CONSTRAINT uq_runtime_workspaces_runtime_id UNIQUE \(runtime_id\).*DO \$\$.*runtime_workspace_credentials.*runtime_workspace_credential_operations.*runtime_workspace_agents.*runtime_external_files.*REFERENCES public\.runtime_workspaces\(runtime_id\)`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := upRepointRuntimeWorkspaceForeignKeys(builder); err != nil {
		t.Fatal(err)
	}
	statements := strings.Join(builder.Statements(), "\n")
	for _, required := range []string{
		"UNIQUE (runtime_id)",
		"con.confrelid = 'public.runtime_workspaces'::regclass",
		"REFERENCES public.runtime_workspaces(runtime_id)",
	} {
		if !strings.Contains(statements, required) {
			t.Fatalf("Runtime Workspace foreign-key migration is missing %q", required)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRepointRuntimeWorkspaceForeignKeysRollbackRestoresResourceWorkspaceScope(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s)ALTER TABLE public\.runtime_workspace_credentials.*DROP CONSTRAINT IF EXISTS fk_runtime_workspace_credentials_runtime_workspace.*runtime_workspace_credential_operations.*runtime_workspace_agents.*runtime_external_files.*REFERENCES public\.runtime_workspaces\(workspace_id\).*CREATE UNIQUE INDEX IF NOT EXISTS uq_runtime_workspaces_runtime_id`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := downRepointRuntimeWorkspaceForeignKeys(mschema.New(db).AllowDestructive()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
