package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestConnectorReversalProjectionMigrationExecutes(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)CREATE OR REPLACE FUNCTION public.connector_reversal_projection_authorized.*enforce_legacy_agent_integration_projection_read_only").WillReturnResult(sqlmock.NewResult(0, 0))
	if err := up20260902160000(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("(?s)CREATE OR REPLACE FUNCTION public.enforce_agent_integration_connection_binding.*DROP FUNCTION IF EXISTS public.connector_reversal_projection_authorized").WillReturnResult(sqlmock.NewResult(0, 0))
	if err := down20260902160000(mschema.New(db).AllowDestructive()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestConnectorReversalProjectionAuthorizationIsNarrow(t *testing.T) {
	sql := compactSQL(authorizeConnectorReversalProjectionSQL)
	for _, expected := range []string{
		"current_setting('zgi.connector_reversal_projection', true)",
		"rollout.phase = 'reversing'", "rollout.revision::text = projection_context->>'rollout_revision'",
		"mapping.owner_account_id = new_row.authorized_by", "legacy.credential_source = 'account'",
		"binding.status = 'active'", "native.deleted_at IS NULL", "legacy.deleted_at IS NULL",
		"new_row.metadata->>'connector_projection_compatibility' = 'legacy_read_only'",
		"to_jsonb(old_row) - ARRAY['metadata', 'updated_at']",
		"provider_allowed @> allowed", "frozen_allowed @> allowed",
		"connection.state = 'ready'", "connection.status = 'active'",
		"OLD.metadata->>'connector_projection_compatibility' = 'legacy_read_only'",
		"legacy integration projection is read-only",
	} {
		if !strings.Contains(sql, compactSQL(expected)) {
			t.Fatalf("missing reversal safeguard %q", expected)
		}
	}
	for _, forbidden := range []string{"DISABLE TRIGGER", "session_replication_role", "SET status = 'active'"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("unsafe reversal bypass %q", forbidden)
		}
	}
}
