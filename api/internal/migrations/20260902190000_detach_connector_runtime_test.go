package migrations

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

func TestConnectorDetachmentHasNoDestructiveHistoricalRewrite(t *testing.T) {
	sql := strings.ToLower(detachConnectorRuntimeSQL)
	for _, forbidden := range []string{"drop table", "delete from", "disable trigger all", " cascade", "update public.connector_lifecycle_decisions", "update public.connector_operation_outcomes"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("detachment may not rewrite history or cascade: %s", forbidden)
		}
	}
	for _, required := range []string{"drop trigger if exists connector_connections_legacy_collision_guard", "drop constraint if exists connector_reversal_baselines_legacy_fk", "connector_confirmation_continuations_native_claim_fk", "target_scope, target_version_id", "conversion_required", "connector_invocation_claims", "connector_unavailable_references"} {
		if !strings.Contains(sql, required) {
			t.Fatalf("missing native isolation contract: %s", required)
		}
	}
}

func TestConnectorDetachmentPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_CONNECTOR_DETACH_TEST_DSN")
	if dsn == "" {
		t.Skip("ZGI_CONNECTOR_DETACH_TEST_DSN is required for isolated PostgreSQL new-install and mixed-upgrade tests")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal("invalid PostgreSQL test configuration")
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	for _, mixed := range []bool{false, true} {
		name := "new_install"
		if mixed {
			name = "mixed_upgrade"
		}
		t.Run(name, func(t *testing.T) {
			schema := "connector_detach_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			conn, err := db.Conn(ctx)
			if err != nil {
				t.Fatal("cannot acquire PostgreSQL test connection")
			}
			defer conn.Close()
			if _, err = conn.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
				t.Fatal(err)
			}
			defer func() { _, _ = conn.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE") }()
			replace := func(sql string) string { return strings.ReplaceAll(sql, "public.", schema+".") }
			if _, err = conn.ExecContext(ctx, replace(detachmentFixture)); err != nil {
				t.Fatal(err)
			}
			if mixed {
				if _, err = conn.ExecContext(ctx, "INSERT INTO "+schema+".integration_connections(id) VALUES ('11111111-1111-4111-8111-111111111111')"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = conn.ExecContext(ctx, replace(detachConnectorRuntimeSQL)); err != nil {
				t.Fatal(err)
			}
			var state string
			if err = conn.QueryRowContext(ctx, "SELECT state FROM "+schema+".connector_schema_readiness WHERE id=1").Scan(&state); err != nil {
				t.Fatal(err)
			}
			want := "ready"
			if mixed {
				want = "conversion_required"
			}
			if state != want {
				t.Fatalf("readiness=%q want %q", state, want)
			}
			// Remove the predecessor sources after schema conversion. New connection,
			// binding, claim, continuation and audit writes must still succeed.
			for _, table := range []string{"integration_connections", "integration_action_policies", "integration_connection_grants", "aichat_integration_preferences", "connector_execution_route_claims", "connector_reversal_baselines", "connector_legacy_identity_mappings"} {
				if _, err = conn.ExecContext(ctx, "DROP TABLE "+schema+"."+table); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = conn.ExecContext(ctx, replace(detachmentNativeWrites)); err != nil {
				t.Fatal(err)
			}
			var terminal string
			if err = conn.QueryRowContext(ctx, "SELECT state FROM "+schema+".connector_invocation_claims").Scan(&terminal); err != nil || terminal != "unknown" {
				t.Fatalf("submitted unknown must remain a terminal barrier: %s %v", terminal, err)
			}
			for _, forbidden := range []string{"UPDATE " + schema + ".connector_invocation_claims SET state='submitted'", "DELETE FROM " + schema + ".connector_invocation_claims", "UPDATE " + schema + ".connector_invocation_claims SET target_scope='draft'"} {
				if _, err := conn.ExecContext(ctx, forbidden); err == nil {
					t.Fatal("permanent claim barrier was mutable")
				}
			}
			if _, err = conn.ExecContext(ctx, "ALTER TABLE "+schema+".connector_confirmation_continuations VALIDATE CONSTRAINT connector_confirmation_continuations_native_claim_fk"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

const detachmentFixture = `
CREATE TABLE public.integration_connections(id uuid PRIMARY KEY);
CREATE TABLE public.integration_action_policies(id uuid);
CREATE TABLE public.integration_connection_grants(id uuid);
CREATE TABLE public.aichat_integration_preferences(id uuid);
CREATE TABLE public.agent_resource_bindings(id uuid,binding_type text);
CREATE TABLE public.connector_connections(id uuid PRIMARY KEY);
CREATE TABLE public.connector_legacy_identity_mappings(id uuid);
CREATE TABLE public.connector_reversal_baselines(id uuid,connection_id uuid,
 CONSTRAINT connector_reversal_baselines_legacy_fk FOREIGN KEY(connection_id) REFERENCES public.integration_connections(id));
CREATE TABLE public.workflows(id uuid,version_uuid uuid,version text,graph text);
CREATE TABLE public.workflow_runs(id uuid,graph text);
CREATE TABLE public.agents_configs(id uuid,agent_mode text);
CREATE TABLE public.agent_published_versions(id uuid,config_snapshot jsonb);
CREATE TABLE public.chat_runtime_custom_skills(id uuid);
CREATE TABLE public.connector_target_bindings(id uuid PRIMARY KEY,organization_id uuid,owner_account_id uuid,target_type text,target_id text,connector_id text,binding_instance_id text);
CREATE UNIQUE INDEX idx_connector_target_bindings_logical_key ON public.connector_target_bindings(organization_id,owner_account_id,target_type,target_id,connector_id,COALESCE(binding_instance_id,''));
CREATE TABLE public.connector_execution_route_claims(receipt_id uuid,organization_id uuid,owner_account_id uuid,UNIQUE(receipt_id,organization_id,owner_account_id));
CREATE TABLE public.connector_confirmation_continuations(route_receipt uuid,organization_id uuid,owner_account_id uuid,
 CONSTRAINT connector_confirmation_continuations_route_owner_fk FOREIGN KEY(route_receipt,organization_id,owner_account_id) REFERENCES public.connector_execution_route_claims(receipt_id,organization_id,owner_account_id));
CREATE TABLE public.connector_lifecycle_decisions(id uuid PRIMARY KEY,organization_id uuid,owner_account_id uuid,acting_principal_id uuid,connector_id text,connection_id uuid,target_type text,target_id text,binding_instance_id text,operation_id text,logical_operation_id text,decision text,status text);
CREATE TABLE public.connector_operation_outcomes(id uuid PRIMARY KEY,decision_id uuid,organization_id uuid,owner_account_id uuid,acting_principal_id uuid,connector_id text,connection_id uuid,target_type text,target_id text,binding_instance_id text,operation_id text,logical_operation_id text,status text);
CREATE FUNCTION public.enforce_connector_operation_outcome() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$;
CREATE TRIGGER connector_operation_outcomes_submission_guard BEFORE INSERT OR UPDATE ON public.connector_operation_outcomes FOR EACH ROW EXECUTE FUNCTION public.enforce_connector_operation_outcome();
CREATE FUNCTION public.enforce_connector_connection_legacy_collision() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 PERFORM 1 FROM public.integration_connections; RETURN NEW; END $$;
CREATE TRIGGER connector_connections_legacy_collision_guard BEFORE INSERT ON public.connector_connections FOR EACH ROW EXECUTE FUNCTION public.enforce_connector_connection_legacy_collision();
`
const detachmentNativeWrites = `
INSERT INTO public.connector_connections(id,connection_constraint) VALUES('11111111-1111-4111-8111-111111111111','{}');
INSERT INTO public.connector_target_bindings(id,organization_id,owner_account_id,target_type,target_id,connector_id,target_scope)
 VALUES('22222222-2222-4222-8222-222222222222','33333333-3333-4333-8333-333333333333','44444444-4444-4444-8444-444444444444','chat','55555555-5555-4555-8555-555555555555','github','live');
INSERT INTO public.connector_invocation_claims(receipt_id,organization_id,owner_account_id,acting_principal_id,binding_id,connection_id,connector_id,target_type,target_id,target_scope,operation_id,logical_operation_id,correlation_id,input_digest,contract_digest,binding_revision,credential_version,state)
 VALUES('66666666-6666-4666-8666-666666666666','33333333-3333-4333-8333-333333333333','44444444-4444-4444-8444-444444444444','44444444-4444-4444-8444-444444444444','22222222-2222-4222-8222-222222222222','11111111-1111-4111-8111-111111111111','github','chat','55555555-5555-4555-8555-555555555555','live','github.issue.create','one-operation','correlation',repeat('a',64),repeat('b',64),1,1,'claimed');
INSERT INTO public.connector_confirmation_continuations(route_receipt,organization_id,owner_account_id)
 VALUES('66666666-6666-4666-8666-666666666666','33333333-3333-4333-8333-333333333333','44444444-4444-4444-8444-444444444444');
INSERT INTO public.connector_lifecycle_decisions(id,organization_id,owner_account_id,acting_principal_id,connector_id,connection_id,target_type,target_id,target_scope,target_version_id,binding_instance_id,operation_id,logical_operation_id,decision,status)
 SELECT receipt_id,organization_id,owner_account_id,acting_principal_id,connector_id,connection_id,target_type,target_id,target_scope,target_version_id,binding_instance_id,operation_id,logical_operation_id,'dispatch','submitted' FROM public.connector_invocation_claims;
INSERT INTO public.connector_operation_outcomes(id,decision_id,organization_id,owner_account_id,acting_principal_id,connector_id,connection_id,target_type,target_id,target_scope,target_version_id,binding_instance_id,operation_id,logical_operation_id,status)
 SELECT id,id,organization_id,owner_account_id,acting_principal_id,connector_id,connection_id,target_type,target_id,target_scope,target_version_id,binding_instance_id,operation_id,logical_operation_id,'unknown' FROM public.connector_lifecycle_decisions;
`
