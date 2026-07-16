package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

const migrationCreateActionRuntimeRunsID = "20260614090000_create_action_runtime_runs"

func init() {
	registerSchemaMigration(
		migrationCreateActionRuntimeRunsID,
		upCreateActionRuntimeRuns,
		downCreateActionRuntimeRuns,
	)
}

func upCreateActionRuntimeRuns(schema *mschema.Builder) error {
	_ = schema
	// Compatibility marker for deployments that applied the removed action runtime migration.
	return nil
}

func downCreateActionRuntimeRuns(schema *mschema.Builder) error {
	_ = schema
	return nil
}
