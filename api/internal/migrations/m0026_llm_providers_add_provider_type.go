package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0026_llm_providers_add_provider_type adds provider_type field to llm_providers table
func M0026_llm_providers_add_provider_type() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251201000000",
		Migrate: func(tx *gorm.DB) error {
			// Add provider_type column to llm_providers table
			if err := tx.Exec(`
				ALTER TABLE llm_providers 
				ADD COLUMN IF NOT EXISTS provider_type VARCHAR(20) DEFAULT 'vendor'
			`).Error; err != nil {
				return err
			}

			// Add comment for the column
			if err := tx.Exec(`
				COMMENT ON COLUMN llm_providers.provider_type IS 'Provider type: vendor (model vendor), aggregator (model aggregator), cloud (cloud service provider)'
			`).Error; err != nil {
				return err
			}

			// Create index for provider_type
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_llm_providers_provider_type 
				ON llm_providers(provider_type)
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop index
			if err := tx.Exec(`
				DROP INDEX IF EXISTS idx_llm_providers_provider_type
			`).Error; err != nil {
				return err
			}

			// Remove provider_type column from llm_providers table
			if err := tx.Exec(`
				ALTER TABLE llm_providers 
				DROP COLUMN IF EXISTS provider_type
			`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
