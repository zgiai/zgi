package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestRedactAcknowledgedOAuthRecoveryPayloadExecutesThroughSchemaBuilder(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)LOCK TABLE public.integration_oauth_recovery_operations.*ADD CONSTRAINT integration_oauth_recovery_payload_check").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := up20260723160000(mschema.New(db)); err != nil {
		t.Fatalf("up migration error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("migration database expectations: %v", err)
	}
}

func TestRestoreAcknowledgedOAuthRecoveryPayloadExecutesThroughSchemaBuilder(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)LOCK TABLE public.integration_oauth_recovery_operations.*ALTER COLUMN payload SET NOT NULL").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := down20260723160000(mschema.New(db).AllowDestructive()); err != nil {
		t.Fatalf("down migration error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("migration database expectations: %v", err)
	}
}

func TestRedactAcknowledgedOAuthRecoveryPayloadSecurityContract(t *testing.T) {
	sql := compactSQL(redactAcknowledgedOAuthRecoveryPayloadSQL)
	for _, expected := range []string{
		"LOCK TABLE public.integration_oauth_recovery_operations IN ACCESS EXCLUSIVE MODE",
		"ALTER COLUMN payload DROP NOT NULL",
		"SET payload = NULL WHERE acknowledged_at IS NOT NULL",
		"acknowledged_at IS NULL AND payload IS NOT NULL",
		"payload ? 'encrypted_credentials'",
		"payload ? 'encrypted_client_credentials'",
		"status = 'dead_letter' AND acknowledged_at IS NOT NULL AND payload IS NULL",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("OAuth recovery payload-redaction migration missing %q: %s", expected, sql)
		}
	}

	rollback := compactSQL(restoreAcknowledgedOAuthRecoveryPayloadSQL)
	for _, expected := range []string{
		"LOCK TABLE public.integration_oauth_recovery_operations IN ACCESS EXCLUSIVE MODE",
		"WHERE acknowledged_at IS NOT NULL OR payload IS NULL",
		"cannot restore OAuth recovery payload retention while redacted audit tombstones exist",
		"ALTER COLUMN payload SET NOT NULL",
	} {
		if !strings.Contains(rollback, expected) {
			t.Fatalf("OAuth recovery payload-redaction rollback missing %q: %s", expected, rollback)
		}
	}
	guardIndex := strings.Index(rollback, "cannot restore OAuth recovery payload retention")
	ddlIndex := strings.Index(rollback, "DROP CONSTRAINT integration_oauth_recovery_payload_check")
	if guardIndex < 0 || ddlIndex < 0 || guardIndex > ddlIndex {
		t.Fatalf("OAuth recovery payload-redaction rollback must fail before destructive DDL: %s", rollback)
	}
}
