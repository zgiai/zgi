package migrations

import (
	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"strings"
	"testing"
)

func TestConnectorOnboardingMigrationPreservesLegacyDataAndScopesSecrets(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s)CREATE TABLE public.connector_onboarding_flows.*CREATE TABLE public.connector_onboarding_secrets.*CREATE TABLE public.connector_connection_provider_evidence`).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := upCreateConnectorOnboarding(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"FOREIGN KEY (flow_id, organization_id, owner_account_id)", "WHERE state IN ('starting', 'awaiting_authorization', 'verifying')", "expected_connection_revision", "expected_credential_version", "accepted_definition_digest", "ON DELETE CASCADE", "generation > 0", "credential_version > 0"} {
		if !strings.Contains(createConnectorOnboardingSQL, s) {
			t.Fatalf("missing guard %s", s)
		}
	}
	for _, s := range []string{"device_code ", "app_secret ", "tenant_access_token ", "UPDATE public.", "DELETE FROM", "agent_channel", "INSERT INTO"} {
		if strings.Contains(createConnectorOnboardingSQL, s) {
			t.Fatalf("unsafe migration %s", s)
		}
	}
}
