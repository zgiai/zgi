package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationCreateWorkflowTestGenerationTasksID = "20260525090000_create_workflow_test_generation_tasks"

const workflowTestGenerationTasksActiveIndexSQL = `
CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_test_generation_tasks_active_agent
ON public.workflow_test_generation_tasks (agent_id)
WHERE status IN ('queued', 'running', 'canceling')
`

func init() {
	registerSchemaMigration(
		migrationCreateWorkflowTestGenerationTasksID,
		upCreateWorkflowTestGenerationTasks,
		downCreateWorkflowTestGenerationTasks,
	)
}

func upCreateWorkflowTestGenerationTasks(schema *mschema.Builder) error {
	if err := schema.Create("workflow_test_generation_tasks", func(table *mschema.Blueprint) {
		table.ID()
		table.UUID("agent_id").NotNull()
		table.UUID("workspace_id").NotNull()
		table.UUID("account_id").NotNull()
		table.String("status", 32).Default("queued").NotNull()
		table.Integer("requested_count").Default(0).NotNull()
		table.Integer("created_count").Default(0).NotNull()
		table.JSONB("scenario_ids").DefaultSQL("'[]'::jsonb").NotNull()
		table.JSONB("question_types").DefaultSQL("'[]'::jsonb").NotNull()
		table.String("turn_strategy", 32).Default("mixed").NotNull()
		table.Text("prompt").Default("").NotNull()
		table.Text("context").Default("").NotNull()
		table.String("model_provider", 100).Default("").NotNull()
		table.String("model_name", 160).Default("").NotNull()
		table.Text("error").Default("").NotNull()
		table.TimestampTz("started_at").Nullable()
		table.TimestampTz("cancel_requested_at").Nullable()
		table.TimestampTz("completed_at").Nullable()
		table.TimestampsTz()

		table.Index("idx_workflow_test_generation_tasks_agent", "agent_id")
		table.Index("idx_workflow_test_generation_tasks_workspace", "workspace_id")
		table.Index("idx_workflow_test_generation_tasks_account", "account_id")
		table.Index("idx_workflow_test_generation_tasks_status", "status")
		table.Index("idx_workflow_test_generation_tasks_agent_status_created", "agent_id", "status", "created_at")
		table.Foreign("workflow_test_generation_tasks_agent_id_fkey", []string{"agent_id"}, "agents", []string{"id"}).CascadeOnDelete()
		table.Foreign("workflow_test_generation_tasks_workspace_id_fkey", []string{"workspace_id"}, "workspaces", []string{"id"}).CascadeOnDelete()
		table.Foreign("workflow_test_generation_tasks_account_id_fkey", []string{"account_id"}, "accounts", []string{"id"}).CascadeOnDelete()
	}); err != nil {
		return err
	}
	return schema.Raw(workflowTestGenerationTasksActiveIndexSQL)
}

func downCreateWorkflowTestGenerationTasks(schema *mschema.Builder) error {
	return schema.DropIfExists("workflow_test_generation_tasks")
}
