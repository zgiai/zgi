package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestConnectorMigrationSourceBarrierCoversPhantomsAndInvalidatesEvidence(t *testing.T) {
	for _, fragment := range []string{
		"source_digest", "source_changed", "sha256(convert_to(",
		"pg_advisory_xact_lock(726914053)",
		"BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE", "FOR EACH STATEMENT",
		"AFTER INSERT OR UPDATE OR DELETE", "FOR EACH ROW",
		"integration_connections", "integration_action_policies", "integration_connection_grants",
		"agent_resource_bindings", "aichat_integration_preferences",
		"source_changed = true",
	} {
		if !strings.Contains(guardConnectorMigrationSourcesSQL, fragment) {
			t.Errorf("source barrier migration lacks %q", fragment)
		}
	}
	invalidation := strings.Split(guardConnectorMigrationSourcesSQL, "CREATE FUNCTION public.invalidate_connector_legacy_source_row()")[1]
	if strings.Contains(invalidation, "r.phase") || strings.Contains(invalidation, "rollout.phase") {
		t.Error("legacy source invalidation must not exempt connector or reversing phases")
	}
	for _, fragment := range []string{"connector_native_host_projection_owned", "binding.connection_id = native.id", "rollout.phase = 'connector'", "ORDER BY journal.sequence_number DESC LIMIT 1", "binding.id::text = item->>'id'"} {
		if !strings.Contains(guardConnectorMigrationSourcesSQL, fragment) {
			t.Errorf("canonical projection exception lacks %q", fragment)
		}
	}
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)ALTER TABLE public.connector_migration_inventory.*connector_legacy_source_digest.*connector_source_invalidation").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := up20260902170000(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
