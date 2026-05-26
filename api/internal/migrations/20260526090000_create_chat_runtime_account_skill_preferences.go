package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

const migration20260526090000ID = "20260526090000_create_chat_runtime_account_skill_preferences"

func init() {
	registerSchemaMigration(
		migration20260526090000ID,
		upCreateChatRuntimeAccountSkillPreferences,
		downCreateChatRuntimeAccountSkillPreferences,
	)
}

func upCreateChatRuntimeAccountSkillPreferences(schema *mschema.Builder) error {
	return schema.Create("chat_runtime_account_skill_preferences", func(table *mschema.Blueprint) {
		table.UUID("organization_id").NotNull()
		table.UUID("account_id").NotNull()
		table.String("caller_type", 32).NotNull()
		table.JSONB("enabled_skill_ids").DefaultSQL("'[]'::jsonb").NotNull()
		table.TimestampsTz()
		table.Primary("organization_id", "account_id", "caller_type")
		table.Index("idx_chat_runtime_account_skill_preferences_account", "account_id", "organization_id", "caller_type")
	})
}

func downCreateChatRuntimeAccountSkillPreferences(schema *mschema.Builder) error {
	return schema.DropIfExists("chat_runtime_account_skill_preferences")
}
