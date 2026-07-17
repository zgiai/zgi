package migrations

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func init() {
	registerMigration(&gormigrate.Migration{
		ID: "202607170900000000_add_chat_runtime_conversation_type",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				ALTER TABLE chat_runtime_conversations
				ADD COLUMN IF NOT EXISTS conversation_type varchar(32) NOT NULL DEFAULT 'chat'
			`).Error; err != nil {
				return fmt.Errorf("add chat runtime conversation type: %w", err)
			}
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_chat_runtime_conversations_type_owner_updated
				ON chat_runtime_conversations (organization_id, account_id, conversation_type, updated_at DESC)
				WHERE deleted_at IS NULL
			`).Error; err != nil {
				return fmt.Errorf("create chat runtime conversation type index: %w", err)
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return fmt.Errorf("rollback of chat runtime conversation type migration is not supported")
		},
	})
}
