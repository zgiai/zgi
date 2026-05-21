package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration20260521090000ID = "20260521090000_create_workflow_announcements"

func init() {
	registerSchemaMigration(
		migration20260521090000ID,
		upCreateWorkflowAnnouncements,
		downCreateWorkflowAnnouncements,
	)
}

func upCreateWorkflowAnnouncements(schema *mschema.Builder) error {
	return schema.Create("announcements", func(table *mschema.Blueprint) {
		table.ID()
		table.UUID("tenant_id").NotNull()
		table.UUID("app_id").NotNull()
		table.String("workflow_run_id").NotNull()
		table.String("node_id").NotNull()
		table.String("node_title").Nullable()
		table.Text("content").NotNull()
		table.Text("rendered_content").NotNull()
		table.String("access_token", 64).NotNull()
		table.TimestampTz("expiration_time").NotNull()
		table.TimestampsTz()
		table.Index("idx_announcements_tenant", "tenant_id")
		table.Index("idx_announcements_app", "app_id")
		table.Index("idx_announcements_run", "workflow_run_id")
		table.Index("idx_announcements_node", "node_id")
		table.Index("idx_announcements_expiration", "expiration_time")
		table.Unique("idx_announcements_access_token", "access_token")
		table.Unique("idx_announcements_run_node", "workflow_run_id", "node_id")
	})
}

func downCreateWorkflowAnnouncements(schema *mschema.Builder) error {
	return schema.DropIfExists("announcements")
}
