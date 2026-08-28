package migrations

import (
	"strings"
	"testing"
)

func TestIntegrationConnectionSetupCompletionMigrationContract(t *testing.T) {
	sql := compactSQL(addIntegrationConnectionSetupCompletionSQL)
	for _, expected := range []string{
		"ADD COLUMN setup_version integer NOT NULL DEFAULT 1",
		"ADD COLUMN setup_completed_at timestamptz",
		"ADD COLUMN setup_completed_by uuid",
		"CHECK (setup_version >= 1)",
		"connection.credential_source = 'account'",
		"FROM public.integration_connection_grants AS connection_grant",
		"jsonb_array_length(connection_grant.allowed_action_ids) > 0",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("connection setup migration missing %q: %s", expected, sql)
		}
	}
}

func TestIntegrationConnectionSetupCompletionBackfillIsConservative(t *testing.T) {
	sql := compactSQL(addIntegrationConnectionSetupCompletionSQL)
	for _, required := range []string{
		"connection.status = 'active'",
		"connection.auth_status = 'valid'",
		"connection_grant.organization_id = connection.organization_id",
		"connection_grant.connection_id = connection.id",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("connection setup backfill is not conservative; missing %q: %s", required, sql)
		}
	}
}
