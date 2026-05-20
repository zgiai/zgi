package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0037_add_supported_protocols adds supported_protocols and validation_report fields
func M0037_add_supported_protocols() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251209000037",
		Migrate: func(tx *gorm.DB) error {
			// 1. Add columns to llm_tenant_routes
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_routes 
				ADD COLUMN IF NOT EXISTS supported_protocols JSONB DEFAULT '[]'::jsonb,
				ADD COLUMN IF NOT EXISTS validation_report JSONB DEFAULT '{}'::jsonb
			`).Error; err != nil {
				return err
			}

			// 2. Add columns to llm_system_channels
			if err := tx.Exec(`
				ALTER TABLE llm_system_channels 
				ADD COLUMN IF NOT EXISTS supported_protocols JSONB DEFAULT '[]'::jsonb,
				ADD COLUMN IF NOT EXISTS validation_report JSONB DEFAULT '{}'::jsonb
			`).Error; err != nil {
				return err
			}

			// 3. Migrate existing data for tenant routes
			if err := tx.Exec(`
				UPDATE llm_tenant_routes 
				SET supported_protocols = jsonb_build_array(protocol)
				WHERE protocol IS NOT NULL 
				  AND protocol != '' 
				  AND (supported_protocols IS NULL OR supported_protocols = '[]'::jsonb)
			`).Error; err != nil {
				return err
			}

			// 4. Migrate existing data for system channels
			if err := tx.Exec(`
				UPDATE llm_system_channels 
				SET supported_protocols = jsonb_build_array(protocol)
				WHERE protocol IS NOT NULL 
				  AND protocol != '' 
				  AND (supported_protocols IS NULL OR supported_protocols = '[]'::jsonb)
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			tx.Exec(`ALTER TABLE llm_tenant_routes DROP COLUMN IF EXISTS supported_protocols`)
			tx.Exec(`ALTER TABLE llm_tenant_routes DROP COLUMN IF EXISTS validation_report`)
			tx.Exec(`ALTER TABLE llm_system_channels DROP COLUMN IF EXISTS supported_protocols`)
			tx.Exec(`ALTER TABLE llm_system_channels DROP COLUMN IF EXISTS validation_report`)
			return nil
		},
	}
}
