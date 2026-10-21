package migrations

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// Opt-in integration test. It never runs repository migrations against public:
// one reserved connection and a newly-created random schema own all fixtures.
func TestConnectorReversalProjectionPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_CONNECTOR_REVERSAL_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_CONNECTOR_REVERSAL_TEST_DSN to run isolated PostgreSQL trigger tests")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal("cannot open isolated PostgreSQL test connection")
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal("cannot connect to isolated PostgreSQL test database")
	}
	defer conn.Close()
	schemaName := "connector_reversal_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := pq.QuoteIdentifier(schemaName)
	if _, err := conn.ExecContext(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := conn.ExecContext(cleanup, "DROP SCHEMA "+quotedSchema+" CASCADE"); err != nil {
			t.Errorf("clean up owned test schema: %v", err)
		}
	}()
	if _, err := conn.ExecContext(ctx, "SET search_path TO "+quotedSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, reversalProjectionPostgresFixtureSchema); err != nil {
		t.Fatal(err)
	}
	for _, migrationSQL := range []string{hardenConnectorAgentBindingProjectionSQL, authorizeConnectorReversalProjectionSQL, guardConnectorMigrationSourcesSQL} {
		scopedSQL := strings.ReplaceAll(migrationSQL, "public.", quotedSchema+".")
		if _, err := conn.ExecContext(ctx, scopedSQL); err != nil {
			if diagnostic, ok := err.(*pq.Error); ok {
				position, _ := strconv.Atoi(diagnostic.Position)
				t.Fatalf("migration SQL failed: %s near %q", diagnostic.Message, scopedSQL[max(0, position-180):min(len(scopedSQL), position+180)])
			}
			t.Fatal(err)
		}
	}

	for _, surface := range []string{"agent", "workflow_draft", "workflow_published"} {
		for _, readiness := range []string{"ready", "unavailable", "reconnect_required"} {
			t.Run(surface+"/"+readiness, func(t *testing.T) {
				tx := seedReversalProjectionPostgres(t, ctx, conn, surface)
				defer tx.Rollback()
				setReversalProjectionReadiness(t, ctx, tx, readiness)
				setReversalProjectionIntent(t, ctx, tx, nil)
				if _, err := tx.ExecContext(ctx, `UPDATE agent_resource_bindings SET metadata = $1::jsonb, updated_at = CURRENT_TIMESTAMP WHERE id = $2`, reversalProjectionMetadata, reversalTestBinding); err != nil {
					t.Fatalf("authorized %s reversal failed: %v", readiness, err)
				}
				var metadata string
				var owner, mode, status string
				if err := tx.QueryRowContext(ctx, `SELECT metadata::text, authorized_by::text, access_mode FROM agent_resource_bindings WHERE id = $1`, reversalTestBinding).Scan(&metadata, &owner, &mode); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(metadata, "legacy_opaque") || strings.Contains(metadata, "approval") || !strings.Contains(metadata, "legacy_read_only") || owner != reversalTestOwner || mode != "read" {
					t.Fatal("reversal changed authority or restored retired metadata")
				}
				if err := tx.QueryRowContext(ctx, `SELECT status FROM integration_connections WHERE id = $1`, reversalTestConnection).Scan(&status); err != nil {
					t.Fatal(err)
				}
				if readiness != "ready" && status == "active" {
					t.Fatal("reversal temporarily enabled an unavailable connection")
				}
				// The same projection is idempotent even though the connection is not ready.
				if _, err := tx.ExecContext(ctx, `UPDATE agent_resource_bindings SET metadata = $1::jsonb WHERE id = $2`, reversalProjectionMetadata, reversalTestBinding); err != nil {
					t.Fatal(err)
				}
			})
		}
	}

	for _, surface := range []string{"workflow_draft", "workflow_published"} {
		t.Run(surface+"/distinct canonical and ledger IDs", func(t *testing.T) {
			tx := seedReversalProjectionPostgres(t, ctx, conn, surface)
			defer tx.Rollback()
			canonicalID := uuid.NewString()
			if _, err := tx.ExecContext(ctx, `UPDATE connector_target_bindings SET id=$1`, canonicalID); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.ExecContext(ctx, `UPDATE connector_migration_inventory SET restriction_snapshot=jsonb_set(restriction_snapshot, '{bindings}', '[]'::jsonb)`); err != nil {
				t.Fatal(err)
			}
			setReversalProjectionReadiness(t, ctx, tx, "unavailable")
			setReversalProjectionIntent(t, ctx, tx, map[string]any{"canonical_binding_id": canonicalID})
			if _, err := tx.ExecContext(ctx, `UPDATE agent_resource_bindings SET metadata=$1::jsonb WHERE id=$2`, reversalProjectionMetadata, reversalTestBinding); err != nil {
				t.Fatalf("distinct UUID reversal failed: %v", err)
			}
			var actualCanonical, actualLedger string
			if err := tx.QueryRowContext(ctx, `SELECT binding.id::text, ledger.id::text FROM connector_target_bindings binding CROSS JOIN agent_resource_bindings ledger`).Scan(&actualCanonical, &actualLedger); err != nil || actualCanonical != canonicalID || actualLedger != reversalTestBinding {
				t.Fatal("projection changed canonical or ledger identity")
			}
		})
	}

	for _, test := range []struct {
		name       string
		mutate     string
		intent     map[string]any
		metadata   string
		withIntent bool
	}{
		{name: "ordinary readonly metadata remains protected"},
		{name: "ordinary unavailable binding remains protected", mutate: `UPDATE integration_connections SET status='disabled'; UPDATE connector_connections SET state='unavailable'`},
		{name: "stale rollout", withIntent: true, intent: map[string]any{"rollout_revision": 3}},
		{name: "foreign connection", withIntent: true, intent: map[string]any{"connection_id": uuid.NewString()}},
		{name: "foreign binding", withIntent: true, intent: map[string]any{"binding_id": uuid.NewString()}},
		{name: "foreign canonical binding", withIntent: true, intent: map[string]any{"canonical_binding_id": uuid.NewString()}},
		{name: "renumbered baseline", withIntent: true, intent: map[string]any{"canonical_binding_id": "10000000-0000-0000-0000-000000000007"}, mutate: `UPDATE connector_target_bindings SET id='10000000-0000-0000-0000-000000000007'`},
		{name: "ambiguous canonical target", withIntent: true, mutate: `INSERT INTO connector_target_bindings SELECT (jsonb_populate_record(NULL::connector_target_bindings, to_jsonb(binding) || '{"id":"10000000-0000-0000-0000-000000000007"}'::jsonb)).* FROM connector_target_bindings binding`},
		{name: "ambiguous ledger target", withIntent: true, mutate: `INSERT INTO agent_resource_bindings SELECT (jsonb_populate_record(NULL::agent_resource_bindings, to_jsonb(ledger) || '{"id":"10000000-0000-0000-0000-000000000007"}'::jsonb)).* FROM agent_resource_bindings ledger`},
		{name: "wrong phase", withIntent: true, mutate: `UPDATE connector_provider_rollouts SET phase='connector'`},
		{name: "already materialized", withIntent: true, mutate: `UPDATE connector_provider_rollouts SET reversal_materialized=true`},
		{name: "in flight", withIntent: true, mutate: `UPDATE connector_provider_rollouts SET in_flight_connector_claims=1`},
		{name: "removed native", withIntent: true, mutate: `UPDATE connector_connections SET deleted_at=CURRENT_TIMESTAMP`},
		{name: "removed legacy", withIntent: true, mutate: `UPDATE integration_connections SET deleted_at=CURRENT_TIMESTAMP`},
		{name: "organization credential", withIntent: true, mutate: `UPDATE integration_connections SET credential_source='organization'`},
		{name: "disabled binding", withIntent: true, mutate: `UPDATE connector_target_bindings SET status='disabled'`},
		{name: "retired policy", withIntent: true, metadata: `{"connector_projection_compatibility":"legacy_read_only","allowed_action_ids":["github.issue.get"],"tool_name":"get_github_issue","approval":false}`},
		{name: "widened frozen allowlist", withIntent: true, metadata: `{"connector_projection_compatibility":"legacy_read_only","allowed_action_ids":["github.issue.get","github.issue.list"],"tool_name":"get_github_issue"}`},
		{name: "native marker", withIntent: true, metadata: `{"connector_projection_compatibility":"native","allowed_action_ids":["github.issue.get"],"tool_name":"get_github_issue"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx := seedReversalProjectionPostgres(t, ctx, conn, "agent")
			defer tx.Rollback()
			if test.mutate != "" {
				if _, err := tx.ExecContext(ctx, test.mutate); err != nil {
					t.Fatal(err)
				}
			}
			if test.withIntent {
				setReversalProjectionIntent(t, ctx, tx, test.intent)
			}
			metadata := test.metadata
			if metadata == "" {
				metadata = reversalProjectionMetadata
			}
			_, err := tx.ExecContext(ctx, `UPDATE agent_resource_bindings SET metadata = $1::jsonb WHERE id = $2`, metadata, reversalTestBinding)
			if err == nil {
				t.Fatal("unauthorized projection was accepted")
			}
			pgErr, ok := err.(*pq.Error)
			if !ok || (pgErr.Code != "23503" && pgErr.Code != "55000") {
				t.Fatalf("unexpected trigger failure: %v", err)
			}
		})
	}

	t.Run("source barrier composes with complete reversal projection", func(t *testing.T) {
		tx := seedReversalProjectionPostgres(t, ctx, conn, "agent")
		defer tx.Rollback()
		assertReversalSourceFlag(t, ctx, tx, false)
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(726914053)`); err != nil {
			t.Fatal(err)
		}
		setReversalProjectionReadiness(t, ctx, tx, "reconnect_required")
		assertReversalSourceFlag(t, ctx, tx, true)
		setReversalProjectionIntent(t, ctx, tx, nil)
		if _, err := tx.ExecContext(ctx, `UPDATE agent_resource_bindings SET metadata=$1::jsonb WHERE id=$2`, reversalProjectionMetadata, reversalTestBinding); err != nil {
			t.Fatal(err)
		}
		assertReversalSourceFlag(t, ctx, tx, true)
		if _, err := tx.ExecContext(ctx, `SELECT set_config('zgi.connector_reversal_projection', '', true)`); err != nil {
			t.Fatal(err)
		}
		// The materializer may clear only its own completed projection under the
		// same source barrier. It does not replace the immutable source digest.
		if _, err := tx.ExecContext(ctx, `UPDATE connector_migration_inventory SET source_changed=false WHERE connection_id=$1 AND organization_id=$2 AND owner_account_id=$3 AND connector_id='github' AND eligibility='backfilled' AND restriction_digest='restriction-test'`, reversalTestConnection, reversalTestOrganization, reversalTestOwner); err != nil {
			t.Fatal(err)
		}
		assertReversalSourceFlag(t, ctx, tx, false)
		if _, err := tx.ExecContext(ctx, `UPDATE integration_connections SET credential_version=credential_version+1 WHERE id=$1`, reversalTestConnection); err != nil {
			t.Fatal(err)
		}
		assertReversalSourceFlag(t, ctx, tx, true)
	})

	t.Run("native Workflow lifecycle does not dirty legacy source", func(t *testing.T) {
		tx := seedReversalProjectionPostgres(t, ctx, conn, "agent")
		defer tx.Rollback()
		canonicalID, ledgerID, workflowID := uuid.NewString(), uuid.NewString(), uuid.NewString()
		queries := []struct {
			sql  string
			args []any
		}{
			{`UPDATE connector_provider_rollouts SET phase='connector'`, nil},
			{`INSERT INTO connector_target_bindings VALUES ($1,$2,$3,$4,'github','workflow',$5,'new-node','active','native','{"allowed_operation_ids":["github.issue.get"]}')`, []any{canonicalID, reversalTestConnection, reversalTestOrganization, reversalTestOwner, workflowID}},
			{`INSERT INTO agent_resource_bindings (id,organization_id,workspace_id,agent_id,consumer_type,consumer_id,binding_instance_id,binding_scope,binding_type,resource_id,parent_resource_id,access_mode,authorized_by,metadata) VALUES ($1,$2,$2,$3,'workflow',$3,'new-node','draft','integration_connection',$4,'github','read',$5,'{"connector_projection_compatibility":"native","allowed_action_ids":["github.issue.get"]}')`, []any{ledgerID, reversalTestOrganization, workflowID, reversalTestConnection, reversalTestOwner}},
			{`INSERT INTO connector_mutation_journal VALUES ($1,$2,$3,'github',1,'binding_upserted',false,jsonb_build_object('binding_id',$4::text))`, []any{reversalTestConnection, reversalTestOrganization, reversalTestOwner, canonicalID}},
		}
		for _, query := range queries {
			if _, err := tx.ExecContext(ctx, query.sql, query.args...); err != nil {
				t.Fatal(err)
			}
		}
		assertReversalSourceFlag(t, ctx, tx, false)
		if _, err := tx.ExecContext(ctx, `UPDATE connector_target_bindings SET status='disabled' WHERE id=$1`, canonicalID); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO connector_mutation_journal VALUES ($1,$2,$3,'github',2,'binding_detached',true,jsonb_build_object('binding_id',$4::text))`, reversalTestConnection, reversalTestOrganization, reversalTestOwner, canonicalID); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM agent_resource_bindings WHERE id=$1`, ledgerID); err != nil {
			t.Fatal(err)
		}
		assertReversalSourceFlag(t, ctx, tx, false)
	})

	t.Run("intent cannot change identity and failure rolls back", func(t *testing.T) {
		tx := seedReversalProjectionPostgres(t, ctx, conn, "agent")
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		tx, err = conn.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		setReversalProjectionReadiness(t, ctx, tx, "unavailable")
		setReversalProjectionIntent(t, ctx, tx, nil)
		if _, err := tx.ExecContext(ctx, `UPDATE agent_resource_bindings SET metadata=$1::jsonb, authorized_by=$2 WHERE id=$3`, reversalProjectionMetadata, uuid.NewString(), reversalTestBinding); err == nil {
			t.Fatal("owner change accepted")
		}
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}
		var status string
		if err := conn.QueryRowContext(ctx, `SELECT status FROM integration_connections WHERE id=$1`, reversalTestConnection).Scan(&status); err != nil || status != "active" {
			t.Fatalf("failed projection did not roll back readiness: %q, %v", status, err)
		}
		var changed bool
		if err := conn.QueryRowContext(ctx, `SELECT source_changed FROM connector_migration_inventory WHERE connection_id=$1`, reversalTestConnection).Scan(&changed); err != nil || changed {
			t.Fatal("failed projection did not roll back source invalidation")
		}
	})
}

func assertReversalSourceFlag(t *testing.T, ctx context.Context, tx *sql.Tx, expected bool) {
	t.Helper()
	var changed bool
	var sourceDigest string
	if err := tx.QueryRowContext(ctx, `SELECT source_changed, source_digest FROM connector_migration_inventory WHERE connection_id=$1`, reversalTestConnection).Scan(&changed, &sourceDigest); err != nil || changed != expected || sourceDigest != strings.Repeat("a", 64) {
		t.Fatalf("source flag = %v, want %v; immutable digest preserved = %v; error = %v", changed, expected, sourceDigest == strings.Repeat("a", 64), err)
	}
}

const (
	reversalTestConnection     = "10000000-0000-0000-0000-000000000001"
	reversalTestOwner          = "10000000-0000-0000-0000-000000000002"
	reversalTestOrganization   = "10000000-0000-0000-0000-000000000003"
	reversalTestBinding        = "10000000-0000-0000-0000-000000000004"
	reversalTestTarget         = "10000000-0000-0000-0000-000000000005"
	reversalProjectionMetadata = `{"connector_projection_compatibility":"legacy_read_only","allowed_action_ids":["github.issue.get"],"tool_name":"get_github_issue"}`
)

func setReversalProjectionIntent(t *testing.T, ctx context.Context, tx *sql.Tx, override map[string]any) {
	t.Helper()
	intent := map[string]any{"connection_id": reversalTestConnection, "binding_id": reversalTestBinding, "canonical_binding_id": reversalTestBinding, "connector_id": "github", "rollout_revision": 4}
	for key, value := range override {
		intent[key] = value
	}
	encoded, err := json.Marshal(intent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `SELECT set_config('zgi.connector_reversal_projection', $1, true)`, string(encoded)); err != nil {
		t.Fatal(err)
	}
}

func setReversalProjectionReadiness(t *testing.T, ctx context.Context, tx *sql.Tx, readiness string) {
	t.Helper()
	status := map[string]string{"ready": "active", "unavailable": "disabled", "reconnect_required": "invalid"}[readiness]
	if _, err := tx.ExecContext(ctx, `UPDATE connector_connections SET state=$1`, readiness); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE integration_connections SET status=$1`, status); err != nil {
		t.Fatal(err)
	}
}

func seedReversalProjectionPostgres(t *testing.T, ctx context.Context, conn *sql.Conn, surface string) *sql.Tx {
	t.Helper()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	exec := func(query string, args ...any) {
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
	}
	consumer, scope, instance, target := "agent", "draft", "", reversalTestTarget
	var published any
	if strings.HasPrefix(surface, "workflow") {
		consumer, instance = "workflow", "node-1"
	}
	if surface == "workflow_published" {
		scope, target = "published", "10000000-0000-0000-0000-000000000006"
		published = target
	}
	exec(`INSERT INTO connector_definitions VALUES ('github','active')`)
	exec(`INSERT INTO connector_connections VALUES ($1,$2,$3,'github','github','github-rest','personal_access_token','ready',NULL,3,'protected-test-envelope')`, reversalTestConnection, reversalTestOrganization, reversalTestOwner)
	exec(`INSERT INTO integration_connections VALUES ($1,$2,$3,'github','github-rest','personal_access_token','active',NULL,NULL,'account',3,'protected-test-envelope')`, reversalTestConnection, reversalTestOrganization, reversalTestOwner)
	exec(`INSERT INTO connector_provider_rollouts VALUES ('github','reversing',4,false,0,0)`)
	exec(`INSERT INTO connector_legacy_identity_mappings VALUES ($1,$2,$3,'github','github','github-rest','connector','restriction-test')`, reversalTestConnection, reversalTestOrganization, reversalTestOwner)
	policy, _ := json.Marshal(map[string]any{"allowed_operation_ids": []string{"github.issue.get", "github.issue.list"}, "bindings": []any{map[string]any{"binding_id": reversalTestBinding, "target_type": consumer, "target_id": target, "binding_instance_id": instance, "status": "active", "access_mode": "read", "audience": "owner_only", "allowed_operation_ids": []string{"github.issue.get"}}}})
	exec(`INSERT INTO connector_migration_inventory (connection_id,organization_id,owner_account_id,connector_id,eligibility,restriction_digest,restriction_snapshot,provider_id) VALUES ($1,$2,$3,'github','backfilled','restriction-test',$4::jsonb,'github')`, reversalTestConnection, reversalTestOrganization, reversalTestOwner, string(policy))
	exec(`INSERT INTO connector_target_bindings VALUES ($1,$2,$3,$4,'github',$5,$6,$7,'active','native','{"allowed_operation_ids":["github.issue.get"]}')`, reversalTestBinding, reversalTestConnection, reversalTestOrganization, reversalTestOwner, consumer, target, instance)
	exec(`INSERT INTO agent_resource_bindings (id,organization_id,workspace_id,agent_id,consumer_type,consumer_id,binding_instance_id,binding_scope,published_version_uuid,binding_type,resource_id,parent_resource_id,access_mode,authorized_by,metadata) VALUES ($1,$2,$2,$3,$4,$3,$5,$6,$7,'integration_connection',$8,'github','read',$9,'{"connector_projection_compatibility":"legacy_read_only","allowed_action_ids":["github.issue.get"],"tool_name":"get_github_issue","legacy_opaque_item":{"approval":true}}')`, reversalTestBinding, reversalTestOrganization, reversalTestTarget, consumer, instance, scope, published, reversalTestConnection, reversalTestOwner)
	exec(`UPDATE connector_migration_inventory SET source_digest=repeat('a',64), source_changed=false WHERE connection_id=$1`, reversalTestConnection)
	return tx
}

const reversalProjectionPostgresFixtureSchema = `
CREATE TABLE connector_definitions (connector_id text PRIMARY KEY, status text);
CREATE TABLE connector_connections (id uuid PRIMARY KEY, organization_id uuid, owner_account_id uuid, connector_id text, provider_id text, driver_id text, auth_method_id text, state text, deleted_at timestamptz, credential_version integer, encrypted_credentials text);
CREATE TABLE integration_connections (id uuid PRIMARY KEY, organization_id uuid, owner_account_id uuid, integration_id text, driver_id text, auth_method_id text, status text, deleted_at timestamptz, expires_at timestamptz, credential_source text, credential_version integer, encrypted_credentials text);
CREATE TABLE connector_provider_rollouts (connector_id text PRIMARY KEY, phase text, revision bigint, reversal_materialized boolean, in_flight_connector_claims integer, in_flight_reconnects integer);
CREATE TABLE connector_legacy_identity_mappings (connection_id uuid PRIMARY KEY, organization_id uuid, owner_account_id uuid, connector_id text, legacy_provider_id text, legacy_driver_id text, cutover_state text, restriction_digest text);
CREATE TABLE connector_migration_inventory (connection_id uuid PRIMARY KEY, organization_id uuid, owner_account_id uuid, connector_id text, eligibility text, restriction_digest text, restriction_snapshot jsonb, provider_id text);
CREATE TABLE connector_target_bindings (id uuid PRIMARY KEY, connection_id uuid, organization_id uuid, owner_account_id uuid, connector_id text, target_type text, target_id text, binding_instance_id text, status text, compatibility text, compatibility_state jsonb);
CREATE TABLE agent_resource_bindings (id uuid PRIMARY KEY, organization_id uuid, workspace_id uuid, agent_id uuid, consumer_type text, consumer_id uuid, binding_instance_id text, binding_scope text, published_version_uuid uuid, binding_type text, resource_id text, parent_resource_id text, access_mode text, authorized_by uuid, authorized_at timestamptz DEFAULT CURRENT_TIMESTAMP, metadata jsonb, display_name text DEFAULT '', created_at timestamptz DEFAULT CURRENT_TIMESTAMP, updated_at timestamptz DEFAULT CURRENT_TIMESTAMP);
CREATE TABLE connector_mutation_journal (connection_id uuid, organization_id uuid, owner_account_id uuid, connector_id text, sequence_number bigint, mutation_type text, tombstone boolean, details jsonb);
CREATE TABLE integration_action_policies (organization_id uuid, integration_id text, action_id text, enabled boolean);
CREATE TABLE integration_connection_grants (id uuid PRIMARY KEY, organization_id uuid, connection_id uuid);
CREATE TABLE aichat_integration_preferences (id uuid PRIMARY KEY, organization_id uuid, integration_id text, selected_connection_ids jsonb);
`
