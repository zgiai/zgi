package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

const migrationPersistConversationToolFilesID = "20260916183000_persist_conversation_tool_files"

func init() {
	registerSchemaMigration(migrationPersistConversationToolFilesID, upPersistConversationToolFiles, nil)
}

func upPersistConversationToolFiles(schema *mschema.Builder) error {
	return schema.DataFix("persist conversation-bound tool files", persistConversationToolFiles)
}

// Conversation-bound tool files come from chat/Agent generation. Workflow image
// outputs have no conversation binding and keep their explicitly chosen lifetime.
func persistConversationToolFiles(db *gorm.DB) error {
	return db.Table("tool_files").
		Where("conversation_id IS NOT NULL AND (lifecycle = ? OR expires_at IS NOT NULL)", "temporary").
		Updates(map[string]any{"lifecycle": "persistent", "expires_at": nil}).Error
}
