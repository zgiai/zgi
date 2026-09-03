package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddSetupDefaultScopeID = "202609020900009760_add_setup_default_scope"

func init() {
	registerSchemaMigration(
		migrationAddSetupDefaultScopeID,
		upAddSetupDefaultScope,
		nil,
	)
}

func upAddSetupDefaultScope(schema *mschema.Builder) error {
	return schema.Table("zgi_setups", func(table *mschema.Blueprint) {
		table.UUID("organization_id").Nullable()
		table.UUID("workspace_id").Nullable()
	})
}
