package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestExtendAgentResourceBindingsIntegrationsMigrationExecutes(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)ALTER TABLE public.agent_resource_bindings.*CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_resource_bindings_integration_unique").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := upExtendAgentResourceBindingsIntegrations(mschema.New(db)); err != nil {
		t.Fatalf("up migration error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("migration database expectations: %v", err)
	}
}

func TestExtendAgentResourceBindingsIntegrationsMigrationContract(t *testing.T) {
	sql := compactSQL(extendAgentResourceBindingsIntegrationsSQL)
	for _, want := range []string{
		"ADD COLUMN IF NOT EXISTS metadata jsonb NOT NULL DEFAULT '{}'::jsonb",
		"'integration_connection'",
		"CREATE OR REPLACE FUNCTION public.enforce_agent_integration_connection_binding()",
		"connection.organization_id = NEW.organization_id",
		"connection.status = 'active'",
		"connection.deleted_at IS NULL",
		"connection.expires_at IS NULL OR connection.expires_at > CURRENT_TIMESTAMP",
		"FOR SHARE",
		"CREATE TRIGGER agent_resource_bindings_integration_connection_guard",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_resource_bindings_integration_unique",
		"COALESCE(published_version_uuid, '00000000-0000-0000-0000-000000000000'::uuid)",
		"parent_resource_id",
		"WHERE binding_type = 'integration_connection'",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("agent integration binding migration missing %q: %s", want, sql)
		}
	}
}
