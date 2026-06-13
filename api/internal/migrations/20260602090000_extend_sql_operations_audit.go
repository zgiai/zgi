package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration20260602090000ID = "20260602090000_extend_sql_operations_audit"

func init() {
	registerSchemaMigration(
		migration20260602090000ID,
		upExtendSQLOperationsAudit,
		nil,
	)
}

func upExtendSQLOperationsAudit(schema *mschema.Builder) error {
	return schema.Table("data_source_sql_operations", func(table *mschema.Blueprint) {
		table.UUID("workspace_id").Nullable()
		table.String("client_type", 32).Default("unknown").NotNull()
		table.String("workflow_run_id").Nullable()
		table.String("node_id").Nullable()
		table.JSONB("params_json").Nullable()
		table.BigInteger("row_count").Nullable()
		table.BigInteger("duration_ms").Nullable()
		table.String("error_code", 64).Nullable()
		table.Text("error_message").Nullable()
		table.TimestampTz("executed_at").Nullable()
		table.String("request_id", 128).Nullable()

		table.Index("idx_data_source_sql_operations_workspace_id", "workspace_id")
		table.Index("idx_data_source_sql_operations_client_type", "client_type")
		table.Index("idx_data_source_sql_operations_workflow_run_id", "workflow_run_id")
		table.Index("idx_data_source_sql_operations_node_id", "node_id")
		table.Index("idx_data_source_sql_operations_executed_at", "executed_at")
	})
}
