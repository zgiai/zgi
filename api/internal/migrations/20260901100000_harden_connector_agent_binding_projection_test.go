package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestHardenConnectorAgentBindingProjectionMigrationExecutes(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)CREATE OR REPLACE FUNCTION public.enforce_agent_integration_connection_binding.*CREATE TRIGGER agent_resource_bindings_legacy_projection_read_only_guard").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := up20260901100000(mschema.New(db)); err != nil {
		t.Fatalf("up migration error = %v", err)
	}

	mock.ExpectExec("(?s)DROP TRIGGER IF EXISTS agent_resource_bindings_legacy_projection_read_only_guard.*CREATE OR REPLACE FUNCTION public.enforce_agent_integration_connection_binding").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := down20260901100000(mschema.New(db).AllowDestructive()); err != nil {
		t.Fatalf("down migration error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("migration expectations: %v", err)
	}
}

func TestHardenConnectorAgentBindingProjectionGuardIsPersonalOnlyAndFailClosed(t *testing.T) {
	sql := compactSQL(hardenConnectorAgentBindingProjectionSQL)
	for _, want := range []string{
		"FROM public.integration_connections AS connection",
		"connection.organization_id = NEW.organization_id",
		"lower(connection.integration_id) = lower(btrim(NEW.parent_resource_id))",
		"connection.status = 'active'",
		"connection.deleted_at IS NULL",
		"connection.expires_at IS NULL OR connection.expires_at > CURRENT_TIMESTAMP",
		"FROM public.connector_connections AS connection",
		"connection.owner_account_id = NEW.authorized_by",
		"NEW.authorized_by IS NOT NULL",
		"connection.state = 'ready'",
		"connection.deleted_at IS NULL",
		"definition.status = 'active'",
		"NEW.metadata->>'connector_projection_compatibility' = 'native'",
		"BEFORE INSERT OR UPDATE OF organization_id, binding_type, resource_id, parent_resource_id, metadata, authorized_by",
		"OLD.metadata->>'connector_projection_compatibility' = 'legacy_read_only'",
		"NEW.resource_id IS DISTINCT FROM OLD.resource_id",
		"NEW.parent_resource_id IS DISTINCT FROM OLD.parent_resource_id",
		"NEW.access_mode IS DISTINCT FROM OLD.access_mode",
		"NEW.metadata IS DISTINCT FROM OLD.metadata",
		"legacy integration projection is read-only",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("hardened Connector Agent binding migration missing %q: %s", want, sql)
		}
	}
	for _, forbidden := range []string{
		"credential_source = 'organization'",
		"credential_source = 'shared'",
		"owner_account_id IS NULL",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("hardened guard widens legacy organization/shared access with %q", forbidden)
		}
	}
}
