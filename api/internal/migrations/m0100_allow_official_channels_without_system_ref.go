package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0100_allow_official_channels_without_system_ref modifies the chk_system_ref constraint
// to allow ZGI_CLOUD type routes without system_channel_id (for mixed load balancing with official channels)
func M0100_allow_official_channels_without_system_ref() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260203000100",
		Migrate: func(tx *gorm.DB) error {
			// Step 1: Drop the existing constraint
			if err := tx.Exec(`
				ALTER TABLE llm_routes 
				DROP CONSTRAINT IF EXISTS chk_system_ref
			`).Error; err != nil {
				return err
			}

			// Step 2: Add new constraint that allows ZGI_CLOUD without system_channel_id
			// This enables mixed load balancing where official channels can be created
			// without referencing a specific system channel
			if err := tx.Exec(`
				ALTER TABLE llm_routes 
				ADD CONSTRAINT chk_system_ref CHECK (
					(type = 'ZGI_CLOUD' AND (system_channel_id IS NOT NULL OR is_official = true)) OR
					(type = 'PRIVATE' AND user_credential_id IS NOT NULL)
				)
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Step 1: Drop the new constraint
			if err := tx.Exec(`
				ALTER TABLE llm_routes 
				DROP CONSTRAINT IF EXISTS chk_system_ref
			`).Error; err != nil {
				return err
			}

			// Step 2: Restore the original constraint
			if err := tx.Exec(`
				ALTER TABLE llm_routes 
				ADD CONSTRAINT chk_system_ref CHECK (
					(type = 'ZGI_CLOUD' AND system_channel_id IS NOT NULL) OR
					(type = 'PRIVATE' AND user_credential_id IS NOT NULL)
				)
			`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
