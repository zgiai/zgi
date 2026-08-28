package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestCreateIntegrationOAuthRecoveryOutboxExecutesThroughSchemaBuilder(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)CREATE TABLE public.integration_oauth_recovery_operations.*CREATE INDEX idx_integration_oauth_recovery_client_impact").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := up20260723140000(mschema.New(db)); err != nil {
		t.Fatalf("up migration error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("migration database expectations: %v", err)
	}
}

func TestCreateIntegrationOAuthRecoveryOutboxSecurityContract(t *testing.T) {
	sql := compactSQL(createIntegrationOAuthRecoveryOutboxSQL)
	for _, expected := range []string{
		"payload jsonb NOT NULL",
		"payload ? 'encrypted_credentials'",
		"payload ? 'encrypted_client_credentials'",
		"status IN ('pending', 'processing', 'dead_letter')",
		"status = 'processing' AND lease_owner IS NOT NULL AND lease_until IS NOT NULL",
		"idx_integration_oauth_recovery_ready",
		"idx_integration_oauth_recovery_client_impact",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("durable OAuth recovery migration missing %q: %s", expected, sql)
		}
	}
	rollback := compactSQL(rollbackIntegrationOAuthRecoveryOutboxSQL)
	if !strings.Contains(rollback, "cannot remove durable OAuth recovery outbox while recovery operations exist") {
		t.Fatalf("durable OAuth recovery rollback is not fail-closed: %s", rollback)
	}
}
