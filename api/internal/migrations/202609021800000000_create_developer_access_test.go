package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestDeveloperAccessMigrationUsesSchemaBuilder(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)ALTER TABLE public.llm_organization_api_keys.*idx_llm_usage_bills_workspace_principal").WillReturnResult(sqlmock.NewResult(0, 0))
	if err := upCreateDeveloperAccess(mschema.New(db)); err != nil {
		t.Fatalf("up migration: %v", err)
	}
	mock.ExpectExec("(?s)DO \\$\\$.*cannot remove developer access schema while records exist.*DROP COLUMN IF EXISTS workspace_id").WillReturnResult(sqlmock.NewResult(0, 0))
	if err := downCreateDeveloperAccess(mschema.New(db).AllowDestructive()); err != nil {
		t.Fatalf("down migration: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeveloperAccessMigrationContract(t *testing.T) {
	sql := strings.ToLower(createDeveloperAccessSQL)
	for _, fragment := range []string{
		"alter column key drop not null",
		"create table public.llm_developer_access_policies",
		"create table public.llm_developer_access_requests",
		"create table public.llm_developer_access_grants",
		"principal_type in ('user', 'service_account')",
		"where status = 'pending' and deleted_at is null",
		"access_grant_id uuid",
		"auth_method varchar(32)",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
	if strings.Contains(sql, "api_key_ciphertext") {
		t.Fatal("developer keys must never persist new plaintext or ciphertext secrets")
	}
	if !strings.Contains(strings.ToLower(rollbackDeveloperAccessSQL), "alter column key set not null") {
		t.Fatal("rollback must restore the legacy key not-null contract")
	}
}
