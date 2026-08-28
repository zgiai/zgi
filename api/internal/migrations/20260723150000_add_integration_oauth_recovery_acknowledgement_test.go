package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAddIntegrationOAuthRecoveryAcknowledgementExecutesThroughSchemaBuilder(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)ALTER TABLE public.integration_oauth_recovery_operations.*CREATE INDEX idx_integration_oauth_recovery_unresolved").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := up20260723150000(mschema.New(db)); err != nil {
		t.Fatalf("up migration error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("migration database expectations: %v", err)
	}
}

func TestAddIntegrationOAuthRecoveryAcknowledgementSecurityContract(t *testing.T) {
	sql := compactSQL(addIntegrationOAuthRecoveryAcknowledgementSQL)
	for _, expected := range []string{
		"acknowledged_at timestamptz",
		"acknowledged_by uuid",
		"status = 'dead_letter'",
		"resolution_code IN ('provider_access_removed', 'token_confirmed_expired')",
		"WHERE status = 'dead_letter' AND acknowledged_at IS NULL",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("OAuth recovery acknowledgement migration missing %q: %s", expected, sql)
		}
	}
	rollback := compactSQL(rollbackIntegrationOAuthRecoveryAcknowledgementSQL)
	if !strings.Contains(rollback, "cannot remove OAuth recovery acknowledgement history") {
		t.Fatalf("OAuth recovery acknowledgement rollback is not fail-closed: %s", rollback)
	}
}
