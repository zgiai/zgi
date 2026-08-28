package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAddIntegrationProviderDiagnosticsExecutesThroughSchemaBuilder(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)ALTER TABLE public.integration_executions.*ALTER TABLE public.integration_connection_health_events").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := up20260728173000(mschema.New(db)); err != nil {
		t.Fatalf("up migration error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("migration database expectations: %v", err)
	}
}

func TestAddIntegrationProviderDiagnosticsContract(t *testing.T) {
	sql := compactSQL(addIntegrationProviderDiagnosticsSQL)
	for _, expected := range []string{
		"ALTER TABLE public.integration_executions",
		"provider_error_code varchar(128)",
		"provider_http_status integer",
		"retry_after_at timestamptz",
		"CHECK (provider_http_status IS NULL OR provider_http_status BETWEEN 100 AND 599)",
		"ALTER TABLE public.integration_connection_health_events",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("provider diagnostics migration missing %q: %s", expected, sql)
		}
	}
	for _, forbidden := range []string{
		"provider_message",
		"response_body",
		"request_body",
		"response_headers",
	} {
		if strings.Contains(strings.ToLower(sql), forbidden) {
			t.Fatalf("provider diagnostics migration must not persist %q: %s", forbidden, sql)
		}
	}
}

func TestRollbackIntegrationProviderDiagnosticsIsFailClosed(t *testing.T) {
	sql := compactSQL(rollbackIntegrationProviderDiagnosticsSQL)
	if !strings.Contains(sql, "cannot remove integration provider diagnostics while diagnostic history exists") {
		t.Fatalf("provider diagnostics rollback is not fail-closed: %s", sql)
	}
	if strings.Contains(sql, "DROP COLUMN") {
		t.Fatalf("provider diagnostics rollback guard must run independently before schema removal: %s", sql)
	}
}

func TestRollbackIntegrationProviderDiagnosticsRunsGuardBeforeSchemaRemoval(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)DO.*cannot remove integration provider diagnostics while diagnostic history exists").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("(?s)ALTER TABLE public.integration_executions.*DROP CONSTRAINT").
		WillReturnResult(sqlmock.NewResult(0, 0))
	for _, column := range []string{
		`"provider_error_code"`,
		`"retry_after_at"`,
		`"provider_http_status"`,
		`"provider_error_code"`,
	} {
		mock.ExpectExec(`ALTER TABLE "public"\."[^"]+" DROP COLUMN ` + column).
			WillReturnResult(sqlmock.NewResult(0, 0))
	}
	if err := down20260728173000(mschema.New(db).AllowDestructive()); err != nil {
		t.Fatalf("down migration error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("migration database expectations: %v", err)
	}
}
