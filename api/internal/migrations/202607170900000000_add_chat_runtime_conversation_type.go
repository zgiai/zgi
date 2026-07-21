package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddChatRuntimeConversationTypeID = "202607170900000000_add_chat_runtime_conversation_type"

func init() {
	registerSchemaMigration(migrationAddChatRuntimeConversationTypeID, upAddChatRuntimeConversationType, nil)
}

func upAddChatRuntimeConversationType(schema *mschema.Builder) error {
	if err := schema.Raw(`
		ALTER TABLE chat_runtime_conversations
		ADD COLUMN IF NOT EXISTS conversation_type varchar(32) NOT NULL DEFAULT 'chat'
	`); err != nil {
		return err
	}
	return schema.Raw(`
		CREATE INDEX IF NOT EXISTS idx_chat_runtime_conversations_type_owner_updated
		ON chat_runtime_conversations (organization_id, account_id, conversation_type, updated_at DESC)
		WHERE deleted_at IS NULL
	`)
}
