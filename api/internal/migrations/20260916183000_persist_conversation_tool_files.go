package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const migrationPersistConversationToolFilesID = "20260916183000_persist_conversation_tool_files"

func init() {
	registerMigration(&gormigrate.Migration{
		ID:      migrationPersistConversationToolFilesID,
		Migrate: persistConversationToolFiles,
	})
}

// Conversation-bound tool files come from chat/Agent generation. Workflow image
// outputs have no conversation binding and keep their explicitly chosen lifetime.
func persistConversationToolFiles(db *gorm.DB) error {
	return db.Table("tool_files").
		Where("conversation_id IS NOT NULL AND (lifecycle = ? OR expires_at IS NOT NULL)", "temporary").
		Updates(map[string]any{"lifecycle": "persistent", "expires_at": nil}).Error
}
