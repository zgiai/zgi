package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestCreateIntegrationOAuthFlowsAndClientsMigrationExecutesThroughSchemaBuilder(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)CREATE TABLE public.integration_oauth_client_configs.*CREATE INDEX idx_integration_oauth_states_flow").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := up20260723100000(mschema.New(db)); err != nil {
		t.Fatalf("up migration error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("migration database expectations: %v", err)
	}
}

func TestCreateIntegrationOAuthFlowsAndClientsMigrationSecurityContract(t *testing.T) {
	sql := compactSQL(createIntegrationOAuthFlowsAndClientsSQL)
	for _, expected := range []string{
		"encrypted_credentials text NOT NULL",
		"flow_digest varchar(64) NOT NULL UNIQUE",
		"encrypted_flow_token text NOT NULL",
		"CHECK (flow_digest ~ '^[0-9a-f]{64}$')",
		"CHECK (credential_source IN ('organization', 'account'))",
		"CHECK (intent IN ('connect', 'reconnect', 'scope_upgrade'))",
		"CHECK (status IN ('pending', 'succeeded', 'failed', 'cancelled', 'expired'))",
		"ADD COLUMN flow_id uuid REFERENCES public.integration_oauth_flows(id) ON DELETE CASCADE",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("OAuth flow/client migration missing %q: %s", expected, sql)
		}
	}
	rollback := compactSQL(rollbackIntegrationOAuthFlowsAndClientsSQL)
	if !strings.Contains(rollback, "cannot roll back OAuth flow/client schema while OAuth data exists") {
		t.Fatalf("OAuth rollback is not fail-closed: %s", rollback)
	}
}
