package migrationsv2

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0034_add_aichat_conversation_metadata() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2AddAIChatConversationMetadataID,
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE public.aichat_conversations ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}'::jsonb`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE public.aichat_conversations DROP COLUMN IF EXISTS metadata`).Error
		},
	}
}
