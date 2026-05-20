package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0041_add_protocol_v2_columns adds missing columns to llm_protocols table for V2 module
func M0041_add_protocol_v2_columns() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251214000041",
		Migrate: func(tx *gorm.DB) error {
			// Add sort_order column
			if err := tx.Exec(`
				ALTER TABLE llm_protocols
				ADD COLUMN IF NOT EXISTS sort_order INT DEFAULT 0
			`).Error; err != nil {
				return err
			}

			// Add config_schema column (JSONB)
			if err := tx.Exec(`
				ALTER TABLE llm_protocols
				ADD COLUMN IF NOT EXISTS config_schema JSONB DEFAULT '{}'
			`).Error; err != nil {
				return err
			}

			// Add display_name_i18n column (JSONB)
			if err := tx.Exec(`
				ALTER TABLE llm_protocols
				ADD COLUMN IF NOT EXISTS display_name_i18n JSONB DEFAULT '{}'
			`).Error; err != nil {
				return err
			}

			// Add description_i18n column (JSONB)
			if err := tx.Exec(`
				ALTER TABLE llm_protocols
				ADD COLUMN IF NOT EXISTS description_i18n JSONB DEFAULT '{}'
			`).Error; err != nil {
				return err
			}

			// Add icon_url column
			if err := tx.Exec(`
				ALTER TABLE llm_protocols
				ADD COLUMN IF NOT EXISTS icon_url VARCHAR(255)
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`ALTER TABLE llm_protocols DROP COLUMN IF EXISTS sort_order`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`ALTER TABLE llm_protocols DROP COLUMN IF EXISTS config_schema`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`ALTER TABLE llm_protocols DROP COLUMN IF EXISTS display_name_i18n`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`ALTER TABLE llm_protocols DROP COLUMN IF EXISTS description_i18n`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`ALTER TABLE llm_protocols DROP COLUMN IF EXISTS icon_url`).Error; err != nil {
				return err
			}
			return nil
		},
	}
}
