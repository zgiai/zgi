package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestCloseIntegrationOAuthArtifactLifecycleMigrationExecutesThroughSchemaBuilder(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)ALTER TABLE public.integration_oauth_flows.*CREATE INDEX idx_integration_oauth_flows_actor_provider_created").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := up20260723110000(mschema.New(db)); err != nil {
		t.Fatalf("up migration error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("migration database expectations: %v", err)
	}
}

func TestCloseIntegrationOAuthArtifactLifecycleMigrationSecurityContract(t *testing.T) {
	sql := compactSQL(closeIntegrationOAuthArtifactLifecycleSQL)
	for _, expected := range []string{
		"(status = 'pending' AND char_length(encrypted_flow_token) > 3)",
		"(status <> 'pending' AND encrypted_flow_token = '')",
		"(status = 'consumed' AND encrypted_verifier = '')",
		") NOT VALID",
		"idx_integration_oauth_flows_actor_provider_created",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("OAuth artifact lifecycle migration missing %q: %s", expected, sql)
		}
	}
	rollback := compactSQL(rollbackIntegrationOAuthArtifactLifecycleSQL)
	if !strings.Contains(rollback, "cannot restore legacy OAuth secret constraints after temporary secrets were erased") {
		t.Fatalf("OAuth artifact lifecycle rollback is not fail-closed: %s", rollback)
	}
}
