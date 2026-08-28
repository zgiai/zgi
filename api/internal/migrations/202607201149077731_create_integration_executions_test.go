package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestIntegrationExecutionsMigrationExecutesThroughSchemaBuilder(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)CREATE TABLE public.integration_executions.*CREATE INDEX idx_integration_executions_provider_request").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := up202607201149077731(mschema.New(db)); err != nil {
		t.Fatalf("up migration error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("migration database expectations: %v", err)
	}
}

func TestIntegrationExecutionsMigrationDefinesAuditContract(t *testing.T) {
	sql := compactSQL(createIntegrationExecutionsSQL)
	for _, want := range []string{
		"CREATE TABLE public.integration_executions",
		"id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4()",
		"organization_id uuid NOT NULL",
		"workspace_id uuid",
		"account_id uuid",
		"app_id uuid",
		"conversation_id uuid",
		"message_id uuid",
		"connection_id uuid",
		"integration_id varchar(64) NOT NULL",
		"driver_id varchar(64) NOT NULL",
		"action_id varchar(128) NOT NULL",
		"invoke_from varchar(32) NOT NULL",
		"status varchar(32) NOT NULL",
		"provider_request_id varchar(128)",
		"duration_ms bigint NOT NULL DEFAULT 0",
		"cost_usd numeric(20,8)",
		"input_hmac varchar(64)",
		"result_count integer NOT NULL DEFAULT 0",
		"attempt_count integer NOT NULL DEFAULT 0",
		"error_code varchar(64)",
		"created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP",
		"updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP",
		"CHECK (duration_ms >= 0)",
		"CHECK (cost_usd IS NULL OR cost_usd >= 0)",
		"CHECK (input_hmac IS NULL OR char_length(input_hmac) = 64)",
		"CHECK (result_count >= 0)",
		"CHECK (attempt_count >= 0)",
		"CREATE INDEX idx_integration_executions_org_created ON public.integration_executions (organization_id, created_at)",
		"CREATE INDEX idx_integration_executions_conversation_created ON public.integration_executions (conversation_id, created_at)",
		"CREATE INDEX idx_integration_executions_provider_request ON public.integration_executions (provider_request_id)",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("integration executions SQL missing %q: %s", want, sql)
		}
	}
}

func TestIntegrationExecutionsMigrationDoesNotPersistSensitivePayloads(t *testing.T) {
	sql := strings.ToLower(compactSQL(createIntegrationExecutionsSQL))
	for _, forbidden := range []string{
		"raw_input",
		"raw_output",
		"request_body",
		"response_body",
		"query_text",
		"api_key",
		"credentials",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("integration executions SQL must not persist %q: %s", forbidden, sql)
		}
	}
}
