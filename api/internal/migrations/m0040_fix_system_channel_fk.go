package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0040_fix_system_channel_fk fixes the foreign key constraint on llm_system_channels
// to reference llm_system_credentials instead of llm_credentials
func M0040_fix_system_channel_fk() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251214000040",
		Migrate: func(tx *gorm.DB) error {
			// Check if llm_system_channels table exists
			var exists bool
			err := tx.Raw(`
				SELECT EXISTS (
					SELECT FROM information_schema.tables 
					WHERE table_schema = CURRENT_SCHEMA() 
					AND table_name = 'llm_system_channels'
				)
			`).Scan(&exists).Error
			if err != nil {
				return err
			}

			// Skip if table doesn't exist
			if !exists {
				return nil
			}

			// Check if credential_id column exists
			var colExists bool
			err = tx.Raw(`
				SELECT EXISTS (
					SELECT FROM information_schema.columns 
					WHERE table_schema = CURRENT_SCHEMA() 
					AND table_name = 'llm_system_channels'
					AND column_name = 'credential_id'
				)
			`).Scan(&colExists).Error
			if err != nil {
				return err
			}

			// Skip if column doesn't exist
			if !colExists {
				return nil
			}

			// Drop the old foreign key constraint
			if err := tx.Exec(`
				ALTER TABLE llm_system_channels
				DROP CONSTRAINT IF EXISTS llm_system_channels_credential_id_fkey
			`).Error; err != nil {
				return err
			}

			// Add new foreign key constraint referencing llm_system_credentials
			return tx.Exec(`
				ALTER TABLE llm_system_channels
				ADD CONSTRAINT llm_system_channels_credential_id_fkey
				FOREIGN KEY (credential_id) REFERENCES llm_system_credentials(id) ON DELETE RESTRICT
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop the new constraint
			if err := tx.Exec(`
				ALTER TABLE llm_system_channels
				DROP CONSTRAINT IF EXISTS llm_system_channels_credential_id_fkey
			`).Error; err != nil {
				return err
			}

			// Restore old constraint
			return tx.Exec(`
				ALTER TABLE llm_system_channels
				ADD CONSTRAINT llm_system_channels_credential_id_fkey
				FOREIGN KEY (credential_id) REFERENCES llm_credentials(id) ON DELETE RESTRICT
			`).Error
		},
	}
}
