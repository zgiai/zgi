package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestIntegrationConnectionsAndPoliciesMigrationExecutesThroughSchemaBuilder(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)CREATE TABLE public.integration_connections.*CREATE INDEX idx_integration_action_policies_org_integration").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := up20260721090000(mschema.New(db)); err != nil {
		t.Fatalf("up migration error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("migration database expectations: %v", err)
	}
}

func TestIntegrationConnectionsMigrationDefinesCredentialIsolationContract(t *testing.T) {
	sql := compactSQL(createIntegrationConnectionsAndPoliciesSQL)
	for _, want := range []string{
		"CREATE TABLE public.integration_connections",
		"organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE",
		"credential_source varchar(32) NOT NULL",
		"auth_type varchar(32) NOT NULL",
		"encrypted_credentials text",
		"config jsonb NOT NULL DEFAULT '{}'::jsonb",
		"granted_scopes jsonb NOT NULL DEFAULT '[]'::jsonb",
		"credential_version integer NOT NULL DEFAULT 1",
		"revision integer NOT NULL DEFAULT 1",
		"CONSTRAINT integration_connections_revision_positive CHECK (revision >= 1)",
		"CHECK (credential_source IN ('platform', 'organization'))",
		"CHECK (auth_type IN ('platform', 'api_key'))",
		"CHECK (status IN ('pending', 'active', 'invalid', 'disabled'))",
		"credential_source = 'platform' AND auth_type = 'platform' AND encrypted_credentials IS NULL",
		"credential_source = 'organization' AND auth_type = 'api_key' AND encrypted_credentials IS NOT NULL",
		"CHECK (NOT is_default OR status = 'active')",
		"CREATE UNIQUE INDEX idx_integration_connections_org_name_active ON public.integration_connections (organization_id, integration_id, lower(name)) WHERE deleted_at IS NULL",
		"CREATE UNIQUE INDEX idx_integration_connections_org_default_active ON public.integration_connections (organization_id, integration_id) WHERE is_default = true AND deleted_at IS NULL",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("integration connections SQL missing %q: %s", want, sql)
		}
	}
}

func TestIntegrationActionPoliciesMigrationCanOnlyTightenProviderPolicy(t *testing.T) {
	sql := compactSQL(createIntegrationConnectionsAndPoliciesSQL)
	for _, want := range []string{
		"CREATE TABLE public.integration_action_policies",
		"PRIMARY KEY (organization_id, integration_id, action_id)",
		"enabled boolean NOT NULL DEFAULT true",
		"approval_policy varchar(32) NOT NULL DEFAULT 'inherit'",
		"data_egress_allowed boolean NOT NULL DEFAULT true",
		"CHECK (approval_policy IN ('inherit', 'always_ask'))",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("integration action policies SQL missing %q: %s", want, sql)
		}
	}
	for _, forbidden := range []string{"effect varchar", "risk_level varchar", "external_destination varchar", "never_ask"} {
		if strings.Contains(strings.ToLower(sql), forbidden) {
			t.Fatalf("organization policy must not persist provider-governance override %q: %s", forbidden, sql)
		}
	}
}

func TestIntegrationConnectionsMigrationStoresNoPlaintextSecretColumn(t *testing.T) {
	sql := strings.ToLower(compactSQL(createIntegrationConnectionsAndPoliciesSQL))
	for _, forbidden := range []string{
		"api_key text",
		"access_token text",
		"refresh_token text",
		"client_secret text",
		"password text",
		"plaintext",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("integration connection schema must not persist %q: %s", forbidden, sql)
		}
	}
}
