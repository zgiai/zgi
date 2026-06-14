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
	if err := schema.Create("action_runtime_runs", func(table *mschema.Blueprint) {
		table.ID()
		table.UUID("organization_id").NotNull()
		table.UUID("workspace_id").Nullable()
		table.UUID("account_id").NotNull()
		table.UUID("conversation_id").Nullable()
		table.UUID("message_id").Nullable()
		table.String("idempotency_key", 128).Nullable()
		table.String("intent", 128).Default("").NotNull()
		table.String("capability_id", 128).NotNull()
		table.String("title", 255).Default("").NotNull()
		table.Text("summary").Default("").NotNull()
		table.String("status", 32).Default("planned").NotNull()
		table.String("risk_level", 32).Default("low").NotNull()
		table.Boolean("requires_confirmation").Default(false).NotNull()
		table.UUID("confirmed_by").Nullable()
		table.TimestampTz("confirmed_at").Nullable()
		table.TimestampTz("canceled_at").Nullable()
		table.Text("error").Nullable()
		table.JSONB("resources").DefaultSQL("'{}'::jsonb").NotNull()
		table.JSONB("arguments").DefaultSQL("'{}'::jsonb").NotNull()
		table.JSONB("ledger").DefaultSQL("'{}'::jsonb").NotNull()
		table.JSONB("metadata").DefaultSQL("'{}'::jsonb").NotNull()
		table.TimestampsTz()
		table.SoftDeletes()
		table.Index("idx_action_runtime_runs_owner_created", "organization_id", "account_id", "created_at")
		table.Index("idx_action_runtime_runs_workspace", "workspace_id")
		table.Index("idx_action_runtime_runs_conversation", "conversation_id")
		table.Index("idx_action_runtime_runs_message", "message_id")
		table.Index("idx_action_runtime_runs_capability", "capability_id")
		table.Index("idx_action_runtime_runs_status", "status")
		table.Index("idx_action_runtime_runs_idempotency", "organization_id", "account_id", "idempotency_key")
	}); err != nil {
		return err
	}

	return schema.Create("action_runtime_steps", func(table *mschema.Blueprint) {
		table.ID()
		table.UUID("run_id").NotNull()
		table.Integer("step_index").Default(0).NotNull()
		table.String("step_key", 128).Default("").NotNull()
		table.String("capability_id", 128).NotNull()
		table.String("title", 255).Default("").NotNull()
		table.String("status", 32).Default("pending").NotNull()
		table.String("risk_level", 32).Default("low").NotNull()
		table.Boolean("requires_confirmation").Default(false).NotNull()
		table.TimestampTz("started_at").Nullable()
		table.TimestampTz("completed_at").Nullable()
		table.Text("error").Nullable()
		table.JSONB("input").DefaultSQL("'{}'::jsonb").NotNull()
		table.JSONB("output").DefaultSQL("'{}'::jsonb").NotNull()
		table.JSONB("metadata").DefaultSQL("'{}'::jsonb").NotNull()
		table.TimestampsTz()
		table.Index("idx_action_runtime_steps_run_index", "run_id", "step_index")
		table.Foreign("fk_action_runtime_steps_run", []string{"run_id"}, "action_runtime_runs", []string{"id"}).CascadeOnDelete()
	})
}

func downCreateActionRuntimeRuns(schema *mschema.Builder) error {
	if err := schema.DropIfExists("action_runtime_steps"); err != nil {
		return err
	}
	return schema.DropIfExists("action_runtime_runs")
}
