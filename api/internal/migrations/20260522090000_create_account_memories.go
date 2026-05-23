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
	if err := schema.Create("account_memory_entries", func(table *mschema.Blueprint) {
		table.ID()
		table.UUID("account_id").NotNull()
		table.Text("content").NotNull()
		table.String("category", 32).Default("other").NotNull()
		table.String("memory_type", 32).Default("long_term").NotNull()
		table.TimestampTz("expires_at").Nullable()
		table.Boolean("enabled").Default(true).NotNull()
		table.TimestampsTz()
		table.Index("idx_account_memory_entries_account_updated", "account_id", "updated_at")
		table.Index("idx_account_memory_entries_category", "category")
		table.Index("idx_account_memory_entries_type_expires", "account_id", "memory_type", "expires_at")
		table.Foreign("fk_account_memory_entries_account", []string{"account_id"}, "accounts", []string{"id"}).CascadeOnDelete()
	}); err != nil {
		return err
	}
	return schema.Create("account_memory_events", func(table *mschema.Blueprint) {
		table.ID()
		table.UUID("account_id").NotNull()
		table.UUID("entry_id").Nullable()
		table.String("action", 32).NotNull()
		table.String("actor_type", 32).Default("user").NotNull()
		table.String("source", 32).Default("api").NotNull()
		table.UUID("source_conversation_id").Nullable()
		table.UUID("source_message_id").Nullable()
		table.JSONB("before_snapshot").Nullable()
		table.JSONB("after_snapshot").Nullable()
		table.TimestampTz("created_at").DefaultSQL("CURRENT_TIMESTAMP").NotNull()
		table.Index("idx_account_memory_events_account_created", "account_id", "created_at")
		table.Index("idx_account_memory_events_entry", "entry_id")
		table.Index("idx_account_memory_events_action", "action")
		table.Index("idx_account_memory_events_actor_type", "actor_type")
		table.Index("idx_account_memory_events_source", "source")
		table.Foreign("fk_account_memory_events_account", []string{"account_id"}, "accounts", []string{"id"}).CascadeOnDelete()
	})
}

func downCreateAccountMemories(schema *mschema.Builder) error {
	if err := schema.DropIfExists("account_memory_events"); err != nil {
		return err
	}
	if err := schema.DropIfExists("account_memory_entries"); err != nil {
		return err
	}
	return schema.DropIfExists("account_memory_settings")
}
