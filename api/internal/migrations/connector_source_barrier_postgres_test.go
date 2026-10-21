package migrations

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// This opt-in integration test touches only its randomly named private schema.
// It never runs the application's public migration chain or provider calls.
func TestConnectorSourceBarrierPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_CONNECTOR_SOURCE_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_CONNECTOR_SOURCE_TEST_DSN for isolated PostgreSQL source-barrier validation")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal("open PostgreSQL test database")
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	schema := "connector_source_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := db.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		// Use a new connection because db.Close is deferred before Cleanup runs.
		cleanupDB, err := sql.Open("pgx", dsn)
		if err != nil {
			t.Error("open cleanup database")
			return
		}
		defer cleanupDB.Close()
		if _, err := cleanupDB.ExecContext(cleanup, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	ddl := `
CREATE TABLE SCHEMA.connector_migration_inventory(connection_id uuid PRIMARY KEY, organization_id uuid, connector_id text, provider_id text, eligibility text);
CREATE TABLE SCHEMA.connector_provider_rollouts(connector_id text PRIMARY KEY, phase text, reversal_materialized bool NOT NULL DEFAULT false);
CREATE TABLE SCHEMA.integration_connections(id uuid PRIMARY KEY, organization_id uuid, integration_id text, encrypted_credentials text, credential_version int, deleted_at timestamptz);
CREATE TABLE SCHEMA.integration_action_policies(organization_id uuid, integration_id text, action_id text, enabled bool);
CREATE TABLE SCHEMA.integration_connection_grants(id uuid PRIMARY KEY, organization_id uuid, connection_id uuid);
CREATE TABLE SCHEMA.agent_resource_bindings(id uuid PRIMARY KEY, organization_id uuid, binding_type text, resource_id text, parent_resource_id text, authorized_by uuid, agent_id uuid, consumer_type text, consumer_id uuid, binding_instance_id text NOT NULL DEFAULT '', binding_scope text, published_version_uuid uuid, metadata jsonb NOT NULL DEFAULT '{}', display_name text NOT NULL DEFAULT '');
CREATE TABLE SCHEMA.aichat_integration_preferences(id uuid PRIMARY KEY, organization_id uuid, integration_id text, selected_connection_ids jsonb);
CREATE TABLE SCHEMA.connector_connections(id uuid PRIMARY KEY, organization_id uuid, owner_account_id uuid, connector_id text, provider_id text);
CREATE TABLE SCHEMA.connector_target_bindings(id uuid PRIMARY KEY, organization_id uuid, owner_account_id uuid, connection_id uuid, connector_id text, target_type text, target_id text, binding_instance_id text, status text, compatibility text);
CREATE TABLE SCHEMA.connector_legacy_identity_mappings(connection_id uuid PRIMARY KEY, organization_id uuid, owner_account_id uuid, connector_id text, cutover_state text);
CREATE TABLE SCHEMA.connector_mutation_journal(connection_id uuid, organization_id uuid, owner_account_id uuid, connector_id text, sequence_number bigint, mutation_type text, tombstone bool, details jsonb);
`
	if _, err := db.ExecContext(ctx, strings.ReplaceAll(ddl, "SCHEMA.", schema+".")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, strings.ReplaceAll(guardConnectorMigrationSourcesSQL, "public.", schema+".")); err != nil {
		t.Fatal(err)
	}
	organizationID, connectionID := uuid.New(), uuid.New()
	if _, err := db.ExecContext(ctx, "INSERT INTO "+schema+".connector_provider_rollouts(connector_id,phase) VALUES ('github','legacy')"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO "+schema+".integration_connections VALUES ($1,$2,'github','protected-original',1,NULL)", connectionID, organizationID); err != nil {
		t.Fatal(err)
	}
	var original string
	if err := db.QueryRowContext(ctx, "SELECT "+schema+".connector_legacy_source_digest($1,$2)", organizationID, connectionID).Scan(&original); err != nil || len(original) != 64 {
		t.Fatalf("source fingerprint failed: %v", err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO "+schema+".connector_migration_inventory VALUES ($1,$2,'github','github','backfilled',$3,false)", connectionID, organizationID, original); err != nil {
		t.Fatal(err)
	}

	// A phantom grant writer must wait even though no grant row existed when
	// staging acquired the barrier. A second backend verifies real lock behavior.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(726914053)"); err != nil {
		t.Fatal(err)
	}
	writer, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.ExecContext(ctx, "SET LOCAL lock_timeout='150ms'"); err != nil {
		t.Fatal(err)
	}
	_, err = writer.ExecContext(ctx, "INSERT INTO "+schema+".integration_connection_grants VALUES ($1,$2,$3)", uuid.New(), organizationID, connectionID)
	if err == nil {
		t.Fatal("phantom writer bypassed source transaction barrier")
	}
	_ = writer.Rollback()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO "+schema+".integration_connection_grants VALUES ($1,$2,$3)", uuid.New(), organizationID, connectionID); err != nil {
		t.Fatal(err)
	}
	var changed bool
	if err := db.QueryRowContext(ctx, "SELECT source_changed FROM "+schema+".connector_migration_inventory").Scan(&changed); err != nil || !changed {
		t.Fatalf("phantom did not invalidate inventory: %v", err)
	}
	var latest string
	if err := db.QueryRowContext(ctx, "SELECT "+schema+".connector_legacy_source_digest($1,$2)", organizationID, connectionID).Scan(&latest); err != nil || latest == original {
		t.Fatalf("grant was absent from source fingerprint: %v", err)
	}

	// Credential replacement, denial-policy insertion, binding insertion,
	// preference insertion, and source removal are all protected inputs.
	for _, statement := range []string{
		"UPDATE " + schema + ".integration_connections SET encrypted_credentials='protected-rotated', credential_version=2",
		"INSERT INTO " + schema + ".integration_action_policies VALUES ('" + organizationID.String() + "','github','get',false)",
		"INSERT INTO " + schema + ".agent_resource_bindings(id,organization_id,binding_type,resource_id,parent_resource_id) VALUES ('" + uuid.NewString() + "','" + organizationID.String() + "','integration_connection','" + connectionID.String() + "','github')",
		"INSERT INTO " + schema + ".aichat_integration_preferences VALUES ('" + uuid.NewString() + "','" + organizationID.String() + "','github','[]')",
		"UPDATE " + schema + ".integration_connections SET deleted_at=now()",
	} {
		if _, err := db.ExecContext(ctx, "UPDATE "+schema+".connector_migration_inventory SET source_changed=false"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, "SELECT source_changed FROM "+schema+".connector_migration_inventory").Scan(&changed); err != nil || !changed {
			t.Fatalf("source mutation did not invalidate inventory: %v", err)
		}
	}
	var removed sql.NullString
	if err := db.QueryRowContext(ctx, "SELECT "+schema+".connector_legacy_source_digest($1,$2)", organizationID, connectionID).Scan(&removed); err != nil || removed.Valid {
		t.Fatalf("removed source fingerprint remains usable: %v", err)
	}
	// Native connector execution does not consult legacy inventory, but an old
	// source change must still invalidate any later reversal. Reversing ordinary
	// writes must also invalidate both sides of the materialization-ready gap.
	for _, rollout := range []struct {
		name         string
		phase        string
		materialized bool
	}{
		{name: "shadow", phase: "shadow"},
		{name: "canary", phase: "canary"},
		{name: "connector", phase: "connector"},
		{name: "reversing_frozen", phase: "reversing"},
		{name: "reversing_ready", phase: "reversing", materialized: true},
	} {
		t.Run(rollout.name, func(t *testing.T) {
			if _, err := db.ExecContext(ctx, "UPDATE "+schema+".connector_provider_rollouts SET phase=$1, reversal_materialized=$2", rollout.phase, rollout.materialized); err != nil {
				t.Fatal(err)
			}
			for _, mutation := range []struct {
				name string
				sql  string
			}{
				{name: "credential_rotation", sql: "UPDATE " + schema + ".integration_connections SET credential_version=credential_version+1"},
				{name: "policy_change", sql: "UPDATE " + schema + ".integration_action_policies SET enabled=NOT enabled"},
				{name: "truncate", sql: "TRUNCATE " + schema + ".integration_connection_grants"},
			} {
				t.Run(mutation.name, func(t *testing.T) {
					if _, err := db.ExecContext(ctx, "UPDATE "+schema+".connector_migration_inventory SET source_changed=false"); err != nil {
						t.Fatal(err)
					}
					if _, err := db.ExecContext(ctx, mutation.sql); err != nil {
						t.Fatal(err)
					}
					if err := db.QueryRowContext(ctx, "SELECT source_changed FROM "+schema+".connector_migration_inventory").Scan(&changed); err != nil || !changed {
						t.Fatalf("source change was not invalidated: %v", err)
					}
				})
			}
		})
	}
	testCanonicalHostProjectionSourceBoundary(t, ctx, db, schema, organizationID, connectionID)
}

func testCanonicalHostProjectionSourceBoundary(t *testing.T, ctx context.Context, db *sql.DB, schema string, organizationID, connectionID uuid.UUID) {
	t.Helper()
	for _, test := range []struct {
		name    string
		marker  string
		prepare string
		change  string
		dirty   bool
	}{
		{name: "native_insert", marker: "native"},
		{name: "native_update", marker: "native", change: "UPDATE SCHEMA.agent_resource_bindings SET display_name='renamed' WHERE id=$1"},
		{name: "native_delete", marker: "native", change: "DELETE FROM SCHEMA.agent_resource_bindings WHERE id=$1"},
		{name: "native_to_legacy", marker: "native", change: "UPDATE SCHEMA.agent_resource_bindings SET metadata='{}' WHERE id=$1", dirty: true},
		{name: "marker_without_binding", marker: "native", prepare: "DELETE FROM SCHEMA.connector_target_bindings", dirty: true},
		{name: "wrong_owner", marker: "native", prepare: "UPDATE SCHEMA.connector_target_bindings SET owner_account_id='00000000-0000-0000-0000-000000000001'", dirty: true},
		{name: "wrong_target", marker: "native", prepare: "UPDATE SCHEMA.connector_target_bindings SET target_id='other-target'", dirty: true},
		{name: "wrong_node", marker: "native", prepare: "UPDATE SCHEMA.connector_target_bindings SET binding_instance_id='another-node'", dirty: true},
		{name: "wrong_mapping", marker: "native", prepare: "UPDATE SCHEMA.connector_legacy_identity_mappings SET cutover_state='shadow'", dirty: true},
		{name: "wrong_connection_without_detach", marker: "native", prepare: "UPDATE SCHEMA.connector_target_bindings SET connection_id='00000000-0000-0000-0000-000000000001'", change: "DELETE FROM SCHEMA.agent_resource_bindings WHERE id=$1", dirty: true},
		{name: "shadow_marker", marker: "native", prepare: "UPDATE SCHEMA.connector_provider_rollouts SET phase='shadow'", dirty: true},
		{name: "reversing_marker", marker: "native", prepare: "UPDATE SCHEMA.connector_provider_rollouts SET phase='reversing'", dirty: true},
		{name: "unmarked", dirty: true},
		{name: "unmarked_delete", change: "DELETE FROM SCHEMA.agent_resource_bindings WHERE id=$1", dirty: true},
		{name: "readonly", marker: "legacy_read_only", dirty: true},
		{name: "readonly_delete_without_tombstone", marker: "legacy_read_only", change: "DELETE FROM SCHEMA.agent_resource_bindings WHERE id=$1", dirty: true},
		{name: "readonly_delete_without_journal", marker: "legacy_read_only", prepare: "UPDATE SCHEMA.connector_target_bindings SET status='disabled'", change: "DELETE FROM SCHEMA.agent_resource_bindings WHERE id=$1", dirty: true},
		{name: "readonly_delete_after_reattach", marker: "legacy_read_only", prepare: "UPDATE SCHEMA.connector_target_bindings SET status='disabled'; INSERT INTO SCHEMA.connector_mutation_journal SELECT connection_id,organization_id,owner_account_id,connector_id,1,'binding_detached',true,jsonb_build_object('binding_id',id::text) FROM SCHEMA.connector_target_bindings; INSERT INTO SCHEMA.connector_mutation_journal SELECT connection_id,organization_id,owner_account_id,connector_id,2,'binding_upserted',false,jsonb_build_object('binding_id',id::text) FROM SCHEMA.connector_target_bindings", change: "DELETE FROM SCHEMA.agent_resource_bindings WHERE id=$1", dirty: true},
		{name: "readonly_native_detach", marker: "legacy_read_only", prepare: "UPDATE SCHEMA.connector_target_bindings SET status='disabled'; INSERT INTO SCHEMA.connector_mutation_journal SELECT connection_id,organization_id,owner_account_id,connector_id,1,'binding_detached',true,jsonb_build_object('binding_id',id::text) FROM SCHEMA.connector_target_bindings", change: "DELETE FROM SCHEMA.agent_resource_bindings WHERE id=$1"},
		{name: "readonly_replaced_connection", marker: "legacy_read_only", prepare: "INSERT INTO SCHEMA.connector_mutation_journal SELECT connection_id,organization_id,owner_account_id,connector_id,1,'binding_detached',true,jsonb_build_object('binding_id',id::text) FROM SCHEMA.connector_target_bindings; UPDATE SCHEMA.connector_target_bindings SET connection_id='00000000-0000-0000-0000-000000000001'", change: "DELETE FROM SCHEMA.agent_resource_bindings WHERE id=$1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			exec := func(query string, args ...any) {
				t.Helper()
				if _, err := tx.ExecContext(ctx, strings.ReplaceAll(query, "SCHEMA.", schema+"."), args...); err != nil {
					t.Fatal(err)
				}
			}
			ownerID, bindingID, hostID, targetID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
			if test.marker == "legacy_read_only" {
				// Migrated readonly baselines preserve the same binding UUID;
				// new native host projections have their own independent UUID.
				hostID = bindingID
			}
			exec("UPDATE SCHEMA.connector_provider_rollouts SET phase='connector', reversal_materialized=false")
			exec("INSERT INTO SCHEMA.connector_connections VALUES ($1,$2,$3,'github','github')", connectionID, organizationID, ownerID)
			exec("INSERT INTO SCHEMA.connector_legacy_identity_mappings VALUES ($1,$2,$3,'github','connector')", connectionID, organizationID, ownerID)
			exec("INSERT INTO SCHEMA.connector_target_bindings VALUES ($1,$2,$3,$4,'github','workflow',$5,'node-1','active','native')", bindingID, organizationID, ownerID, connectionID, targetID.String())
			if test.prepare != "" {
				exec(test.prepare)
			}
			exec("UPDATE SCHEMA.connector_migration_inventory SET source_changed=false")
			exec("INSERT INTO SCHEMA.agent_resource_bindings (id,organization_id,binding_type,resource_id,parent_resource_id,authorized_by,agent_id,consumer_type,consumer_id,binding_instance_id,binding_scope,metadata) VALUES ($1,$2,'integration_connection',$3,'github',$4,$5,'workflow',$5,'node-1','draft',jsonb_build_object('connector_projection_compatibility',$6::text))", hostID, organizationID, connectionID.String(), ownerID, targetID, test.marker)
			if test.change != "" {
				exec("UPDATE SCHEMA.connector_migration_inventory SET source_changed=false")
				exec(test.change, hostID)
			}
			var dirty bool
			if err := tx.QueryRowContext(ctx, "SELECT source_changed FROM "+schema+".connector_migration_inventory WHERE connection_id=$1", connectionID).Scan(&dirty); err != nil || dirty != test.dirty {
				t.Fatalf("source changed = %v, want %v: %v", dirty, test.dirty, err)
			}
			if !test.dirty {
				// This is the exact source gate at materializer entry. Native
				// projection lifecycle must not make a later reversal unavailable.
				exec("UPDATE SCHEMA.connector_provider_rollouts SET phase='reversing'")
				if err := tx.QueryRowContext(ctx, "SELECT source_changed FROM "+schema+".connector_migration_inventory WHERE connection_id=$1", connectionID).Scan(&dirty); err != nil || dirty {
					t.Fatalf("canonical host projection blocked reversal: %v", err)
				}
			}
		})
	}
}
