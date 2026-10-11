package main

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// testHistoricalMigrations models a database at the published historical
// checkpoint. The ledger is ordered, as loadMigrationHistory reads it.
var testHistoricalMigrations = []string{
	"20260520000000_initial_schema",
	"20260901090000_create_connector_foundation",
	"20260902150000_create_connector_reversal_baselines",
	historicalCheckpointID,
}

const testAdvancedCheckpointID = "20260903120000_messaging_connector_protocol"

func migrationRows(ids []string) []row {
	rows := make([]row, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, row{"id": id})
	}
	return rows
}

func testAdvancedHistory() []string {
	return append(append([]string(nil), testHistoricalMigrations...), testAdvancedCheckpointID)
}

func TestCheckpointChoiceDefaultsToTheExactHistoricalHead(t *testing.T) {
	if err := (checkpointChoice{}).validate(1, testHistoricalMigrations); err != nil {
		t.Fatalf("default historical checkpoint rejected: %v", err)
	}
	for _, tc := range []struct {
		name    string
		version int
		history []string
	}{
		{"advanced head", 1, testAdvancedHistory()},
		{"schema2", 2, testHistoricalMigrations},
		{"empty ledger", 1, nil},
		{"finalizer marker", 1, append(testAdvancedHistory(), finalizeMigrationID)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := (checkpointChoice{}).validate(tc.version, tc.history); err == nil {
				t.Fatal("default mode accepted a non-historical checkpoint")
			}
		})
	}
}

func TestAdvancedCheckpointRequiresAnExplicitBoundedHead(t *testing.T) {
	choice := checkpointChoice{advanced: true, id: testAdvancedCheckpointID}
	if err := choice.validate(1, testAdvancedHistory()); err != nil {
		t.Fatalf("explicit advanced checkpoint rejected: %v", err)
	}
	for _, tc := range []struct {
		name    string
		choice  checkpointChoice
		history []string
	}{
		{"empty value", checkpointChoice{advanced: true}, testAdvancedHistory()},
		{"historical checkpoint named", checkpointChoice{advanced: true, id: historicalCheckpointID}, testHistoricalMigrations},
		{"earlier identifier", checkpointChoice{advanced: true, id: "20260101000000_earlier"}, testAdvancedHistory()},
		{"malformed identifier", checkpointChoice{advanced: true, id: "not-a-migration"}, testAdvancedHistory()},
		{"historical checkpoint missing", checkpointChoice{advanced: true, id: testAdvancedCheckpointID}, []string{"20260520000000_initial_schema", testAdvancedCheckpointID}},
		{"finalizer already applied", checkpointChoice{advanced: true, id: finalizeMigrationID}, append(testAdvancedHistory(), finalizeMigrationID)},
		{"head mismatch", checkpointChoice{advanced: true, id: testAdvancedCheckpointID}, append(append([]string(nil), testHistoricalMigrations...), "20260903130000_other_migration")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.choice.validate(1, tc.history); err == nil {
				t.Fatal("invalid advanced checkpoint accepted")
			}
		})
	}
}

func TestAdvancedRetiredTableManifestIsFrozen(t *testing.T) {
	expected := []string{
		"aichat_integration_preferences",
		"connector_execution_route_claims", "connector_legacy_identity_mappings",
		"connector_migration_inventory", "connector_mutation_journal", "connector_provider_rollouts",
		"connector_reversal_baselines", "connector_rollout_evidence", "connector_shadow_observations",
		"integration_action_policies", "integration_connection_grants", "integration_connection_health_events",
		"integration_connections", "integration_executions", "integration_oauth_client_configs",
		"integration_oauth_flows", "integration_oauth_recovery_operations", "integration_oauth_states",
		"integration_operation_receipts", "integration_provider_authorizations",
		"integration_provider_install_flows", "integration_provider_suite_states",
	}
	if len(advancedRetiredTables) != 22 || len(expected) != 22 {
		t.Fatalf("retired table manifest size changed: %d", len(advancedRetiredTables))
	}
	want := map[string]bool{}
	for _, name := range expected {
		want[name] = true
	}
	for _, name := range advancedRetiredTables {
		if !want[name] {
			t.Fatalf("unexpected retired table %s", name)
		}
		delete(want, name)
	}
	if len(want) != 0 {
		t.Fatalf("retired table manifest lost entries: %v", want)
	}
	if len(advancedBindingColumns) != 2 || len(advancedBindingConstraints) != 2 {
		t.Fatal("advanced compatibility fingerprint changed")
	}
	required := map[string][]string{
		"connector_target_bindings":            advancedBindingConstraints,
		"agent_resource_bindings":              {"agent_resource_bindings_type_check"},
		"connector_confirmation_continuations": {advancedNativeClaimConstraint},
	}
	if len(advancedConstraintGroups) != len(required) {
		t.Fatalf("advanced constraint group size changed: %d", len(advancedConstraintGroups))
	}
	for _, group := range advancedConstraintGroups {
		want, ok := required[group.table]
		if !ok || strings.Join(group.names, "\x00") != strings.Join(want, "\x00") {
			t.Fatalf("unexpected advanced constraint group %s %v", group.table, group.names)
		}
		delete(required, group.table)
	}
	if len(required) != 0 {
		t.Fatalf("advanced constraint groups lost entries: %v", required)
	}
	// The native claim FK exists before converter apply validates it, so the
	// catalog query must key on the constraint name and never on validation.
	if !strings.Contains(advancedConstraintQuery, "conname::text = ANY($2)") || strings.Contains(advancedConstraintQuery, "convalidated") {
		t.Fatal("advanced constraint fingerprint depends on constraint validation")
	}
}

func TestSourceDigestBindsTheCompleteMigrationLedger(t *testing.T) {
	first := fixture(t)
	second := fixture(t)
	second[migrationHistoryKey] = append(migrationRows([]string{"20260902170000_guard_connector_migration_sources"}), second[migrationHistoryKey]...)
	if hash(first) == hash(second) {
		t.Fatal("migration ledger is not part of the source digest")
	}
	firstPlan, firstErr := makePlan(first, testKeys())
	secondPlan, secondErr := makePlan(second, testKeys())
	if firstErr != nil || secondErr != nil || firstPlan.Report.SourceDigest == secondPlan.Report.SourceDigest {
		t.Fatal("dry-run source digest did not bind the migration ledger")
	}
}

func advancedCommandFixture(t *testing.T) (*commandDatabase, string) {
	t.Helper()
	return commandFixtureAt(t, testAdvancedHistory())
}

func TestAdvancedCheckpointReadOnlyRunsAndApplyStayAtomic(t *testing.T) {
	choice := checkpointChoice{advanced: true, id: testAdvancedCheckpointID}
	t.Run("dry-run", func(t *testing.T) {
		db, digest := advancedCommandFixture(t)
		before := hash(db.tx.source)
		var out report
		err := runOnConnection(context.Background(), db, "dry-run", testKeys(), choice, digest, "", func(r report) error { out = r; return nil })
		if err != nil || out.SourceDigest != digest || db.options.AccessMode != pgx.ReadOnly || db.options.IsoLevel != pgx.Serializable || db.tx.writes != 0 || db.tx.committed || hash(db.tx.source) != before {
			t.Fatal("advanced dry-run was not read-only", err)
		}
		if eventIndex(db.tx.events, "lock:migration-ledger") >= 0 || eventIndex(db.tx.events, "lock:advisory") >= 0 {
			t.Fatal("advanced dry-run acquired a lock")
		}
	})
	t.Run("apply", func(t *testing.T) {
		db, digest := advancedCommandFixture(t)
		var out report
		err := runOnConnection(context.Background(), db, "apply", testKeys(), choice, digest, "", func(r report) error { out = r; return nil })
		if err != nil || !db.tx.committed || !db.tx.validated || !db.tx.readyWritten || out.State != "applied" {
			t.Fatal("advanced apply failed", err)
		}
		if db.options.AccessMode == pgx.ReadOnly {
			t.Fatal("advanced apply used a read-only transaction")
		}
	})
}

func TestDefaultModeRefusesAnAdvancedHeadWithoutOptIn(t *testing.T) {
	db, digest := advancedCommandFixture(t)
	err := runOnConnection(context.Background(), db, "dry-run", testKeys(), checkpointChoice{}, digest, "", func(report) error { return nil })
	if err == nil || db.tx.writes != 0 || db.tx.committed {
		t.Fatal("default mode accepted an advanced migration head", err)
	}
}

func TestChangedMigrationHistoryInvalidatesTheReviewedDigest(t *testing.T) {
	choice := checkpointChoice{advanced: true, id: testAdvancedCheckpointID}
	db, digest := advancedCommandFixture(t)
	// The head is unchanged, so the checkpoint condition still holds, but a
	// different migration ledger was applied after the reviewed dry-run.
	db.tx.migrations = []string{
		"20260520000000_initial_schema",
		"20260901090000_create_connector_foundation",
		"20260902150000_create_connector_reversal_baselines",
		"20260902170000_guard_connector_migration_sources",
		historicalCheckpointID,
		testAdvancedCheckpointID,
	}
	err := runOnConnection(context.Background(), db, "apply", testKeys(), choice, digest, "", func(report) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "source changed") || db.tx.committed || db.tx.writes != 0 {
		t.Fatal("changed migration history was applied", err)
	}
	if db.tx.readyWritten {
		t.Fatal("stale apply marked readiness ready")
	}
}

func TestApplyRechecksTheLedgerAfterLocks(t *testing.T) {
	choice := checkpointChoice{advanced: true, id: testAdvancedCheckpointID}
	db, digest := advancedCommandFixture(t)
	db.tx.migrations = append(append([]string(nil), testHistoricalMigrations...), "20260903130000_unreviewed_migration")
	err := runOnConnection(context.Background(), db, "apply", testKeys(), choice, digest, "", func(report) error { return nil })
	if err == nil || db.tx.committed || db.tx.writes != 0 || db.tx.readyWritten {
		t.Fatal("ledger change during locking was applied")
	}
}

func TestAdvancedCheckpointRequiresTheRetainedStructure(t *testing.T) {
	choice := checkpointChoice{advanced: true, id: testAdvancedCheckpointID}
	for _, tc := range []struct {
		name   string
		mutate func(*commandTransaction)
	}{
		{"retired table missing", func(tx *commandTransaction) { tx.retiredTables = tx.retiredTables[1:] }},
		{"compatibility column missing", func(tx *commandTransaction) { tx.bindingColumns = []string{"compatibility"} }},
		{"compatibility constraint missing", func(tx *commandTransaction) { tx.constraintNames["public.connector_target_bindings"] = nil }},
		{"agent resource binding type check missing", func(tx *commandTransaction) { tx.constraintNames["public.agent_resource_bindings"] = nil }},
		{"native claim FK missing", func(tx *commandTransaction) { tx.constraintNames["public.connector_confirmation_continuations"] = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, digest := advancedCommandFixture(t)
			tc.mutate(db.tx)
			err := runOnConnection(context.Background(), db, "dry-run", testKeys(), choice, digest, "", func(report) error { return nil })
			if err == nil || db.tx.writes != 0 || db.tx.committed || db.tx.readyWritten {
				t.Fatal("advanced run accepted an incomplete schema", err)
			}
		})
	}
	t.Run("finalizer marker present", func(t *testing.T) {
		db, _ := commandFixtureAt(t, append(testAdvancedHistory(), finalizeMigrationID))
		err := runOnConnection(context.Background(), db, "dry-run", testKeys(), choice, "", "", func(report) error { return nil })
		if err == nil || db.tx.writes != 0 || db.tx.readyWritten {
			t.Fatal("advanced run accepted an already-finalized database", err)
		}
	})
}

// TestAdvancedApplyLocksTheMigrationLedgerFirst proves the revision-1 lock
// contract: the migration-ledger lock is the first statement, before the
// conversion advisory lock, before any SELECT and before any write, so the
// ledger read into the source digest cannot race a concurrent marker insert.
func TestAdvancedApplyLocksTheMigrationLedgerFirst(t *testing.T) {
	choice := checkpointChoice{advanced: true, id: testAdvancedCheckpointID}
	db, digest := advancedCommandFixture(t)
	var out report
	if err := runOnConnection(context.Background(), db, "apply", testKeys(), choice, digest, "", func(r report) error { out = r; return nil }); err != nil {
		t.Fatalf("advanced apply failed: %v", err)
	}
	if out.State != "applied" || !db.tx.readyWritten {
		t.Fatal("advanced apply did not complete")
	}
	lock := eventIndex(db.tx.events, "lock:migration-ledger")
	advisory := eventIndex(db.tx.events, "lock:advisory")
	ledgerRead := eventIndex(db.tx.events, "select:migration-ledger")
	firstSelect := firstEventWithPrefix(db.tx.events, "select:")
	firstWrite := firstEventWithPrefix(db.tx.events, "write:")
	if lock < 0 || advisory < 0 || ledgerRead < 0 || firstSelect < 0 || firstWrite < 0 {
		t.Fatalf("advanced apply did not lock, read and write: %v", db.tx.events)
	}
	if lock > advisory || lock > ledgerRead || lock > firstSelect || lock > firstWrite {
		t.Fatalf("migration ledger was not locked before reading and writing: %v", db.tx.events)
	}
}

// TestAdvancedReadOnlyCommandsTakeNoLock covers revision 1 for the read-only
// commands: only advanced apply locks the migration ledger.
func TestAdvancedReadOnlyCommandsTakeNoLock(t *testing.T) {
	choice := checkpointChoice{advanced: true, id: testAdvancedCheckpointID}
	for _, command := range []string{"inventory", "dry-run"} {
		t.Run(command, func(t *testing.T) {
			db, digest := advancedCommandFixture(t)
			before := hash(db.tx.source)
			if err := runOnConnection(context.Background(), db, command, testKeys(), choice, digest, "", func(report) error { return nil }); err != nil {
				t.Fatal(err)
			}
			if db.options.AccessMode != pgx.ReadOnly || db.tx.writes != 0 || db.tx.committed || !db.tx.rolledBack {
				t.Fatal("advanced read-only command mutated or committed")
			}
			if eventIndex(db.tx.events, "lock:migration-ledger") >= 0 || eventIndex(db.tx.events, "lock:advisory") >= 0 {
				t.Fatal("advanced read-only command acquired a lock")
			}
			if hash(db.tx.source) != before {
				t.Fatal("advanced read-only command mutated the source")
			}
		})
	}
}

// TestHistoricalApplyKeepsItsLockOrder guards that the migration-ledger lock is
// exclusive to the advanced opt-in.
func TestHistoricalApplyKeepsItsLockOrder(t *testing.T) {
	db, digest := commandFixture(t)
	if err := runOnConnection(context.Background(), db, "apply", testKeys(), checkpointChoice{}, digest, "", func(report) error { return nil }); err != nil {
		t.Fatalf("historical apply failed: %v", err)
	}
	if eventIndex(db.tx.events, "lock:migration-ledger") >= 0 {
		t.Fatal("historical apply changed its lock order")
	}
	advisory := eventIndex(db.tx.events, "lock:advisory")
	read := firstEventWithPrefix(db.tx.events, "select:")
	if advisory < 0 || read < 0 || advisory > read {
		t.Fatalf("historical apply changed its advisory-first order: %v", db.tx.events)
	}
}

// TestAdvancedApplyLockFailureNeverMarksReadiness proves a failing migration
// ledger lock aborts before any conversion write and never marks readiness.
func TestAdvancedApplyLockFailureNeverMarksReadiness(t *testing.T) {
	choice := checkpointChoice{advanced: true, id: testAdvancedCheckpointID}
	db, digest := advancedCommandFixture(t)
	db.tx.failLock = true
	err := runOnConnection(context.Background(), db, "apply", testKeys(), choice, digest, "", func(report) error { return nil })
	if err == nil || strings.Contains(err.Error(), secretMarker) || db.tx.committed || db.tx.writes != 0 || db.tx.readyWritten {
		t.Fatal("failing migration ledger lock wrote or marked readiness ready", err)
	}
	if !db.tx.rolledBack {
		t.Fatal("failing migration ledger lock was not rolled back")
	}
}
