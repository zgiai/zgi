package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration20260522090000ID = "20260522090000_create_account_memories"

func init() {
	registerSchemaMigration(
		migration20260522090000ID,
		upCreateAccountMemories,
		downCreateAccountMemories,
	)
}

func upCreateAccountMemories(schema *mschema.Builder) error {
	if err := schema.Create("account_memory_settings", func(table *mschema.Blueprint) {
		table.UUID("account_id").NotNull().Primary()
		table.Boolean("enabled").Default(false).NotNull()
		table.TimestampsTz()
		table.Foreign("fk_account_memory_settings_account", []string{"account_id"}, "accounts", []string{"id"}).CascadeOnDelete()
	}); err != nil {
		return err
	}
	return schema.Create("account_memory_entries", func(table *mschema.Blueprint) {
		table.ID()
		table.UUID("account_id").NotNull()
		table.Text("content").NotNull()
		table.String("category", 32).Default("other").NotNull()
		table.Boolean("enabled").Default(true).NotNull()
		table.TimestampsTz()
		table.Index("idx_account_memory_entries_account_updated", "account_id", "updated_at")
		table.Index("idx_account_memory_entries_category", "category")
		table.Foreign("fk_account_memory_entries_account", []string{"account_id"}, "accounts", []string{"id"}).CascadeOnDelete()
	})
}

func downCreateAccountMemories(schema *mschema.Builder) error {
	if err := schema.DropIfExists("account_memory_entries"); err != nil {
		return err
	}
	return schema.DropIfExists("account_memory_settings")
}
