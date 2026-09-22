package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestCreateRuntimeWorkspacesAddsScopedCredentialsBindingsAndFiles(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s)ALTER TABLE public\.user_subjects.*CREATE TABLE IF NOT EXISTS public\.runtime_workspaces.*CREATE TABLE IF NOT EXISTS public\.runtime_external_files`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	builder := mschema.New(db)
	if err := upCreateRuntimeWorkspaces(builder); err != nil {
		t.Fatal(err)
	}
	statements := strings.Join(builder.Statements(), "\n")
	for _, required := range []string{
		"runtime_workspaces",
		"runtime_workspace_credentials",
		"runtime_workspace_credential_operations",
		"runtime_workspace_agents",
		"runtime_external_files",
		"runtime_workspace_api",
		"mode varchar(24) NOT NULL DEFAULT 'attached'",
		"UNIQUE (runtime_workspace_id, public_app_key)",
		"UNIQUE (runtime_workspace_id, created_by_account_id, idempotency_key_hash)",
		"idx_runtime_workspace_agents_listing",
		"idx_agent_published_versions_agent_workspace_active",
	} {
		if !strings.Contains(statements, required) {
			t.Fatalf("migration is missing %q:\n%s", required, statements)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateRuntimeWorkspacesRollbackRestoresSubjectConstraint(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	for _, table := range []string{"runtime_external_files", "runtime_workspace_agents", "runtime_workspace_credential_operations", "runtime_workspace_credentials", "runtime_workspaces"} {
		mock.ExpectExec(`DROP TABLE IF EXISTS "public"\."` + table + `"`).WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectExec(`(?s)ALTER TABLE public\.user_subjects.*DROP CONSTRAINT IF EXISTS ck_user_memory_subject_identity.*ADD CONSTRAINT ck_user_memory_subject_identity`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	builder := mschema.New(db).AllowDestructive()
	if err := downCreateRuntimeWorkspaces(builder); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
