package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Frozen, closed structural fingerprint of the pre-finalize schema. It mirrors
// the API-owned retirement manifest but is not imported from it. Object names
// are compile-time constants and are never assembled from caller input.
var advancedRetiredTables = []string{
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

var advancedBindingColumns = []string{"compatibility", "compatibility_state"}

// The retirement migration drops exactly these constraints together with the
// compatibility columns, so their presence proves the finalizer has not run.
var advancedBindingConstraints = []string{
	"connector_target_bindings_compatibility_check",
	"connector_target_bindings_compatibility_object_check",
}

// The native continuation ownership FK may still be NOT VALID at the advanced
// checkpoint, because converter apply validates it after conversion. Only its
// existence is required here, so the catalog query must not filter on
// convalidated and must not depend on the constraint being enforced.
const advancedNativeClaimConstraint = "connector_confirmation_continuations_native_claim_fk"

// Constraint groups are compile-time table names paired with compile-time
// constraint names. Groups keep the fingerprint closed: no caller input selects
// a table or a constraint.
var advancedConstraintGroups = []struct {
	table string
	names []string
	kind  string
}{
	{"connector_target_bindings", advancedBindingConstraints, "connector_target_bindings constraint"},
	{"agent_resource_bindings", []string{"agent_resource_bindings_type_check"}, "agent_resource_bindings constraint"},
	{"connector_confirmation_continuations", []string{advancedNativeClaimConstraint}, "connector_confirmation_continuations constraint"},
}

// Catalog queries take fixed object names as bind parameters. They only read
// PostgreSQL catalogs; no object name is ever assembled from caller input.
const (
	advancedTableQuery = `SELECT coalesce(array_agg(c.relname::text ORDER BY c.relname), '{}'::text[])
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = 'public' AND c.relkind IN ('r','p') AND c.relname::text = ANY($1)`
	advancedColumnQuery = `SELECT coalesce(array_agg(column_name::text ORDER BY column_name), '{}'::text[])
FROM information_schema.columns
WHERE table_schema = 'public' AND table_name = 'connector_target_bindings' AND column_name::text = ANY($1)`
	advancedConstraintQuery = `SELECT coalesce(array_agg(conname::text ORDER BY conname), '{}'::text[])
FROM pg_constraint
WHERE conrelid = to_regclass($1) AND conname::text = ANY($2)`
)

// checkAdvancedSchema refuses an advanced checkpoint whose retained structure is
// incomplete: either the retirement finalizer already ran or the database is not
// the schema the historical converter expects.
func checkAdvancedSchema(ctx context.Context, tx pgx.Tx) error {
	if err := requireAdvancedObjects(ctx, tx, advancedTableQuery, []any{advancedRetiredTables}, advancedRetiredTables, "retained Connector table"); err != nil {
		return err
	}
	if err := requireAdvancedObjects(ctx, tx, advancedColumnQuery, []any{advancedBindingColumns}, advancedBindingColumns, "connector_target_bindings column"); err != nil {
		return err
	}
	for _, group := range advancedConstraintGroups {
		if err := requireAdvancedObjects(ctx, tx, advancedConstraintQuery, []any{"public." + group.table, group.names}, group.names, group.kind); err != nil {
			return err
		}
	}
	return nil
}

// requireAdvancedObjects reports the first requested object the parameterized
// catalog query did not return.
func requireAdvancedObjects(ctx context.Context, tx pgx.Tx, query string, args []any, names []string, kind string) error {
	var found []string
	if err := tx.QueryRow(ctx, query, args...).Scan(&found); err != nil {
		return errors.New("cannot inspect the advanced conversion schema")
	}
	present := make(map[string]bool, len(found))
	for _, name := range found {
		present[name] = true
	}
	for _, name := range names {
		if !present[name] {
			return fmt.Errorf("advanced checkpoint requires %s %s", kind, name)
		}
	}
	return nil
}
