package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const migration20260526090000ID = "20260526090000_create_chat_runtime_account_skill_preferences"

func init() {
	registerMigration(&gormigrate.Migration{
		ID:      migration20260526090000ID,
		Migrate: upCreateChatRuntimeAccountSkillPreferences,
	})
}

func upCreateChatRuntimeAccountSkillPreferences(tx *gorm.DB) error {
	statements := []string{
		`
		CREATE TABLE IF NOT EXISTS chat_runtime_account_skill_preferences (
			organization_id uuid NOT NULL,
			account_id uuid NOT NULL,
			caller_type character varying(32) NOT NULL,
			enabled_skill_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
			created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (organization_id, account_id, caller_type)
		)
		`,
		`CREATE INDEX IF NOT EXISTS idx_chat_runtime_account_skill_preferences_account ON chat_runtime_account_skill_preferences (account_id, organization_id, caller_type)`,
	}
	for _, statement := range statements {
		if err := tx.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}
