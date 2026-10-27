package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestCreateOrgAuditLogsMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)CREATE TABLE IF NOT EXISTS public.org_audit_logs.*idx_org_audit_logs_org_created.*idx_org_audit_logs_org_actor.*idx_org_audit_logs_org_target.*idx_org_audit_logs_org_action").WillReturnResult(sqlmock.NewResult(0, 0))
	if err := up202610261000000000(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("DROP TABLE IF EXISTS public.org_audit_logs").WillReturnResult(sqlmock.NewResult(0, 0))
	if err := down202610261000000000(mschema.New(db).AllowDestructive()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"organization_id uuid NOT NULL",
		"actor_account_id uuid",
		"target_account_id uuid",
		"result IN ('success', 'denied', 'failed')",
		"jsonb_typeof(before) = 'object'",
		"jsonb_typeof(after) = 'object'",
		"request_id varchar(128)",
		"ip varchar(64)",
		"created_at timestamptz NOT NULL",
	} {
		if !strings.Contains(createOrgAuditLogsSQL, fragment) {
			t.Fatalf("missing %q", fragment)
		}
	}
}

func TestCreateOrgAuditLogsMigrationIsRegistered(t *testing.T) {
	for _, migration := range registeredMigrations() {
		if migration.ID == "202610261000000000_create_org_audit_logs" {
			if migration.Rollback == nil {
				t.Fatal("org audit log migration has no rollback")
			}
			return
		}
	}
	t.Fatal("org audit log migration is not registered")
}
