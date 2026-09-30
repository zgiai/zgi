package migrations

import (
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestConnectorFoundationMigrationExecutesAndRollbackIsGuarded(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)CREATE TABLE public.connector_definitions.*CREATE OR REPLACE FUNCTION public.enforce_agent_integration_connection_binding").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := up20260901090000(mschema.New(db)); err != nil {
		t.Fatalf("up migration error = %v", err)
	}

	mock.ExpectExec("(?s)cannot remove connector foundation while authoritative records exist.*DROP TABLE IF EXISTS public.connector_definitions").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := down20260901090000(mschema.New(db).AllowDestructive()); err != nil {
		t.Fatalf("down migration error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("migration database expectations: %v", err)
	}
}

func TestConnectorFoundationMigrationDefinesCanonicalRecords(t *testing.T) {
	sql := compactSQL(createConnectorFoundationSQL)
	for _, want := range []string{
		"CREATE TABLE public.connector_definitions",
		"connector_id varchar(128) PRIMARY KEY",
		"classification IN ('connector', 'data_source')",
		"status IN ('active', 'disabled', 'unavailable')",
		"jsonb_typeof(display) = 'object'",
		"jsonb_typeof(key_schema) = 'object'",
		"jsonb_typeof(capability_summary) = 'object'",
		"CREATE TABLE public.connector_connections",
		"owner_account_id uuid NOT NULL REFERENCES public.accounts(id)",
		"encrypted_credentials text NOT NULL",
		"credential_version >= 1",
		"revision >= 1",
		"state IN ('ready', 'reconnect_required', 'unavailable')",
		"jsonb_typeof(usable_operation_ids) = 'array'",
		"jsonb_typeof(unavailable_operations) = 'object'",
		"UNIQUE (id, organization_id, owner_account_id, connector_id)",
		"CREATE TABLE public.connector_key_setup_attempts",
		"idempotency_key_hash varchar(128) NOT NULL",
		"request_fingerprint varchar(128) NOT NULL",
		"CHECK (request_fingerprint ~ '^[0-9a-f]{64}$')",
		"binding_instance_id varchar(255)",
		"state IN ('verifying', 'ready', 'failed', 'cancelled')",
		"target_type IN ('chat', 'agent') AND target_id IS NOT NULL AND btrim(target_id) <> '' AND binding_instance_id IS NULL",
		"target_type = 'workflow' AND target_id IS NOT NULL AND btrim(target_id) <> '' AND binding_instance_id IS NOT NULL AND btrim(binding_instance_id) <> ''",
		"CREATE UNIQUE INDEX idx_connector_key_setups_owner_idempotency",
		"CREATE TABLE public.connector_target_bindings",
		"target_type IN ('chat', 'agent', 'workflow')",
		"status IN ('active', 'disabled', 'needs_replacement')",
		"jsonb_typeof(compatibility_state) = 'object'",
		"CREATE UNIQUE INDEX idx_connector_target_bindings_logical_key",
		"COALESCE(binding_instance_id, '')",
		"CREATE TABLE public.connector_execution_route_claims",
		"receipt_id uuid PRIMARY KEY",
		"UNIQUE (organization_id, logical_operation_id)",
		"path IN ('connector_native', 'legacy')",
		"CREATE INDEX idx_connector_execution_route_claims_connection",
		"CREATE TABLE public.connector_lifecycle_decisions",
		"status IN ('submitted', 'denied', 'blocked', 'cancelled', 'failed', 'successful', 'unknown')",
		"jsonb_typeof(details) = 'object'",
		"CREATE TABLE public.connector_operation_outcomes",
		"status IN ('successful', 'failed', 'unknown')",
		"CREATE UNIQUE INDEX idx_connector_operation_outcomes_logical_operation",
		"CREATE TABLE public.connector_legacy_identity_mappings",
		"cutover_state IN ('candidate', 'shadow', 'connector', 'reversed', 'blocked')",
		"CREATE TABLE public.connector_mutation_journal",
		"credential_version IS NULL OR credential_version >= 1",
		"tombstone boolean NOT NULL DEFAULT false",
		"protected_credential_ref",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("connector foundation migration missing %q: %s", want, sql)
		}
	}
}

func TestConnectorFoundationMigrationPreservesOwnershipAndLegacyBindingGuard(t *testing.T) {
	sql := compactSQL(createConnectorFoundationSQL)
	if count := strings.Count(sql, "binding_instance_id varchar(255)"); count != 5 {
		t.Fatalf("connector foundation binding_instance_id column count = %d, want setup, binding, route claim, decision, and outcome columns", count)
	}
	for _, want := range []string{
		"FOREIGN KEY (connection_id, organization_id, owner_account_id, connector_id) REFERENCES public.connector_connections(id, organization_id, owner_account_id, connector_id)",
		"CREATE OR REPLACE FUNCTION public.enforce_connector_connection_legacy_collision()",
		"legacy.credential_source <> 'account'",
		"CREATE OR REPLACE FUNCTION public.enforce_connector_legacy_personal_mapping()",
		"legacy.credential_source = 'account'",
		"legacy.owner_account_id = NEW.owner_account_id",
		"CREATE OR REPLACE FUNCTION public.enforce_agent_integration_connection_binding()",
		"FROM public.integration_connections AS connection",
		"connection.status = 'active'",
		"connection.expires_at IS NULL OR connection.expires_at > CURRENT_TIMESTAMP",
		"FROM public.connector_connections AS connection",
		"connection.owner_account_id = NEW.authorized_by",
		"connection.state = 'ready'",
		"definition.status = 'active'",
		"connection.deleted_at IS NULL",
		"decision.binding_instance_id IS NOT DISTINCT FROM NEW.binding_instance_id",
		"CREATE TRIGGER connector_execution_route_claims_immutable",
		"FOR SHARE",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("connector ownership/binding migration missing %q: %s", want, sql)
		}
	}
}

func TestConnectorFoundationMigrationStoresNoPlaintextSecretColumns(t *testing.T) {
	sql := strings.ToLower(createConnectorFoundationSQL)
	for _, forbidden := range []string{
		`(?m)[,(]\s*(key|api_key|token|access_token|refresh_token|password|secret|submitted_key|candidate_key)\s+(text|varchar|jsonb|bytea)\b`,
		`plaintext`,
		`submitted_values`,
		`candidate_credentials`,
		`request_payload`,
		`request_values`,
		`input_payload`,
	} {
		if regexp.MustCompile(forbidden).MatchString(sql) {
			t.Fatalf("connector foundation migration persists forbidden secret shape %q", forbidden)
		}
	}
	if !strings.Contains(sql, "encrypted_credentials text not null") {
		t.Fatal("connector credentials must be persisted only as an encrypted envelope")
	}
}
