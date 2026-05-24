package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration20260522090000WorkspaceAssetMoveEventsID = "20260522090000_create_workspace_asset_move_events"

func init() {
	registerSchemaMigration(
		migration20260522090000WorkspaceAssetMoveEventsID,
		upCreateWorkspaceAssetMoveEvents,
		downCreateWorkspaceAssetMoveEvents,
	)
}

func upCreateWorkspaceAssetMoveEvents(schema *mschema.Builder) error {
	return schema.Create("workspace_asset_move_events", func(table *mschema.Blueprint) {
		table.ID()
		table.UUID("organization_id").NotNull()
		table.UUID("actor_account_id").NotNull()
		table.String("asset_type", 32).NotNull()
		table.String("asset_id").NotNull()
		table.UUID("from_workspace_id").Nullable()
		table.UUID("to_workspace_id").NotNull()
		table.JSONB("warnings").DefaultSQL("'[]'::jsonb").NotNull()
		table.TimestampTz("created_at").DefaultSQL("CURRENT_TIMESTAMP").NotNull()

		table.Index("idx_workspace_asset_move_events_org_created", "organization_id", "created_at")
		table.Index("idx_workspace_asset_move_events_asset", "asset_type", "asset_id")
		table.Index("idx_workspace_asset_move_events_actor", "actor_account_id")
	})
}

func downCreateWorkspaceAssetMoveEvents(schema *mschema.Builder) error {
	return schema.DropIfExists("workspace_asset_move_events")
}
