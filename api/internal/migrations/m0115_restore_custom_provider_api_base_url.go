package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0115_restore_custom_provider_api_base_url re-adds api_base_url to llm_custom_providers.
// This column was incorrectly dropped in M0114 and needs to be restored.
func M0115_restore_custom_provider_api_base_url() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260208000115",
		Migrate: func(tx *gorm.DB) error {
			var exists bool
			if err := tx.Raw(`
				SELECT EXISTS (
					SELECT 1
					FROM information_schema.tables
					WHERE table_schema = CURRENT_SCHEMA()
					  AND table_name = 'llm_custom_providers'
				)
			`).Scan(&exists).Error; err != nil {
				return err
			}
			if !exists {
				return nil
			}

			return tx.Exec(`
				ALTER TABLE llm_custom_providers ADD COLUMN IF NOT EXISTS api_base_url VARCHAR(255)
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}
