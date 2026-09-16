package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestConnectorConfirmationContinuationsMigrationExecutesAndGuardsRollback(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)CREATE UNIQUE INDEX uq_connector_execution_route_claims_owner_receipt.*CREATE TABLE public.connector_confirmation_continuations.*idx_connector_confirmation_continuations_workflow_run").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := up20260902120000(mschema.New(db)); err != nil {
		t.Fatalf("up migration error = %v", err)
	}

	mock.ExpectExec("(?s)cannot remove Connector confirmation continuations while records exist.*DROP INDEX IF EXISTS public.uq_connector_execution_route_claims_owner_receipt").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := down20260902120000(mschema.New(db).AllowDestructive()); err != nil {
		t.Fatalf("down migration error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestConnectorConfirmationContinuationSchemaIsOwnerScopedAndFailClosed(t *testing.T) {
	sql := compactSQL(createConnectorConfirmationContinuationsSQL)
	for _, expected := range []string{
		"route_receipt uuid PRIMARY KEY",
		"FOREIGN KEY (route_receipt, organization_id, owner_account_id)",
		"REFERENCES public.connector_execution_route_claims(receipt_id, organization_id, owner_account_id)",
		"origin_surface IN ('chat', 'agent', 'workflow')",
		"state IN ('pending', 'cancelled', 'completed', 'failed', 'unknown')",
		"state = 'pending' AND decision_id IS NULL AND outcome_id IS NULL",
		"state = 'cancelled' AND decision_id IS NOT NULL AND outcome_id IS NULL",
		"state = 'completed' AND decision_id IS NOT NULL AND outcome_id IS NOT NULL",
		"state IN ('failed', 'unknown') AND diagnostic_code IS NOT NULL",
		"idx_connector_confirmation_continuations_owner_state",
		"idx_connector_confirmation_continuations_conversation",
		"idx_connector_confirmation_continuations_agent_run",
		"idx_connector_confirmation_continuations_workflow_run",
	} {
		if !strings.Contains(sql, compactSQL(expected)) {
			t.Fatalf("migration SQL missing %q", expected)
		}
	}
}

func TestConnectorConfirmationContinuationSchemaCannotPersistPlaintextInputOrKeys(t *testing.T) {
	lower := strings.ToLower(createConnectorConfirmationContinuationsSQL)
	tableStart := strings.Index(lower, "create table public.connector_confirmation_continuations")
	tableEnd := strings.Index(lower[tableStart:], "create index idx_connector_confirmation_continuations_owner_state")
	if tableStart < 0 || tableEnd < 0 {
		t.Fatal("continuation table boundary missing")
	}
	table := lower[tableStart : tableStart+tableEnd]
	if !strings.Contains(table, "protected_payload_envelope text not null") {
		t.Fatal("continuation table is missing the protected envelope")
	}
	for _, forbiddenReference := range []string{
		"decision_id uuid references", "outcome_id uuid references",
	} {
		if strings.Contains(table, forbiddenReference) {
			t.Fatalf("continuation terminal ID has unsafe availability coupling %q", forbiddenReference)
		}
	}
	for _, forbidden := range []string{
		"plaintext_input", "provider_input", "request_input", "api_key", "key_value",
		"credential_value", "token_value", "secret_value", "raw_payload",
	} {
		if strings.Contains(table, forbidden) {
			t.Fatalf("continuation table contains forbidden secret-bearing column %q", forbidden)
		}
	}
}
