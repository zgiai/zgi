package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration202606060429060180ID = "202606060429060180_create_system_short_links"

func init() {
	registerSchemaMigration(migration202606060429060180ID, up202606060429060180, down202606060429060180)
}

func up202606060429060180(schema *mschema.Builder) error {
	return schema.Create("system_short_links", func(table *mschema.Blueprint) {
		table.ID()
		table.String("short_token", 32).NotNull()
		table.String("target_kind", 64).NotNull()
		table.String("target_token", 128).NotNull()
		table.String("target_path", 512).NotNull()
		table.TimestampsTz()

		table.Unique("idx_system_short_links_token", "short_token")
		table.Unique("idx_system_short_links_target", "target_kind", "target_token")
		table.Index("idx_system_short_links_target_kind", "target_kind")
	})
}

func down202606060429060180(schema *mschema.Builder) error {
	return schema.DropIfExists("system_short_links")
}
