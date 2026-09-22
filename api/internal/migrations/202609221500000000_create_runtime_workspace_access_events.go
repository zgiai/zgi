package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationCreateRuntimeWorkspaceAccessEventsID = "202609221500000000_create_runtime_workspace_access_events"

func init() {
	registerSchemaMigration(migrationCreateRuntimeWorkspaceAccessEventsID, upCreateRuntimeWorkspaceAccessEvents, downCreateRuntimeWorkspaceAccessEvents)
}

// upCreateRuntimeWorkspaceAccessEvents adds the append-only audit sink for
// authenticated Runtime Workspace calls. It is intentionally independent of
// the existing application audit tables because Runtime Subjects are not ZGI
// Accounts and the external request envelope has different privacy rules.
func upCreateRuntimeWorkspaceAccessEvents(schema *mschema.Builder) error {
	return schema.Create("runtime_workspace_access_events", func(table *mschema.Blueprint) {
		table.ID()
		table.UUID("runtime_workspace_id").NotNull()
		table.UUID("organization_id").NotNull()
		table.UUID("workspace_id").NotNull()
		table.UUID("credential_id").Nullable()
		table.UUID("subject_id").Nullable()
		table.UUID("agent_id").Nullable()
		table.String("method", 16).NotNull()
		table.String("route", 255).NotNull()
		table.Integer("status_code").NotNull()
		table.String("result", 16).NotNull()
		table.JSONB("details").DefaultSQL("'{}'::jsonb").NotNull()
		table.TimestampTz("created_at").DefaultSQL("CURRENT_TIMESTAMP").NotNull()
		table.Index("idx_runtime_access_events_scope_created", "runtime_workspace_id", "created_at")
		table.Index("idx_runtime_access_events_credential_created", "credential_id", "created_at")
		table.Index("idx_runtime_access_events_subject_created", "subject_id", "created_at")
		table.Index("idx_runtime_access_events_agent_created", "agent_id", "created_at")
		table.Foreign("fk_runtime_access_events_runtime_workspace", []string{"runtime_workspace_id"}, "runtime_workspaces", []string{"runtime_id"}).CascadeOnDelete()
	})
}

func downCreateRuntimeWorkspaceAccessEvents(schema *mschema.Builder) error {
	return schema.DropIfExists("runtime_workspace_access_events")
}
