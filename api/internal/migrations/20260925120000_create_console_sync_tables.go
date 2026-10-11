package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

const migrationCreateConsoleSyncTablesID = "20260925120000_create_console_sync_tables"

func init() {
	registerSchemaMigration(migrationCreateConsoleSyncTablesID, upCreateConsoleSyncTables, downCreateConsoleSyncTables)
}

// upCreateConsoleSyncTables creates the local state of the console control plane.
//
// These tables belong to the consolesync module only; no existing table is touched.
func upCreateConsoleSyncTables(schema *mschema.Builder) error {
	if err := schema.Create("pre_sync_state", func(table *mschema.Blueprint) {
		table.Integer("id").Primary()
		table.UUID("downstream_client_id").Nullable()
		table.BigInteger("cursor_seq").NotNull().Default(0)
		table.BigInteger("applied_version").NotNull().Default(0)
		table.TimestampTz("last_heartbeat_at").Nullable()
		table.TimestampTz("last_pull_at").Nullable()
		table.Integer("consecutive_failures").NotNull().Default(0)
		table.TimestampTz("next_poll_at").Nullable()
		table.String("credential_status", 32).Nullable()
		table.Text("last_error").Nullable()
		table.TimestampsTz()
	}); err != nil {
		return err
	}

	return schema.Create("pre_applied_commands", func(table *mschema.Blueprint) {
		table.UUID("command_id").Primary()
		table.BigInteger("seq").NotNull()
		table.String("type", 64).NotNull()
		table.String("status", 20).NotNull()
		table.JSONB("before_state").Nullable()
		table.JSONB("after_state").Nullable()
		table.Text("error").Nullable()
		table.TimestampTz("applied_at").NotNull()
		table.Index("idx_pre_applied_commands_seq", "seq")
	})
}

func downCreateConsoleSyncTables(schema *mschema.Builder) error {
	if err := schema.DropIfExists("pre_applied_commands"); err != nil {
		return err
	}
	return schema.DropIfExists("pre_sync_state")
}
