package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0016_llm_extends adds description field to llm_providers table
func M0016_llm_extends() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251125000000",
		Migrate: func(tx *gorm.DB) error {
			// Add description column to llm_providers table
			if err := tx.Exec(`
				ALTER TABLE llm_providers 
				ADD COLUMN IF NOT EXISTS description TEXT,
				ADD COLUMN IF NOT EXISTS openai_compatible BOOLEAN DEFAULT false
			`).Error; err != nil {
				return err
			}

			// Add columns to llm_models table
			if err := tx.Exec(`
				ALTER TABLE llm_models
				ADD COLUMN IF NOT EXISTS description TEXT,
				ADD COLUMN IF NOT EXISTS tokenizer VARCHAR(100),
				ADD COLUMN IF NOT EXISTS instruct_type VARCHAR(100),
				ADD COLUMN IF NOT EXISTS cost_image DECIMAL(10, 4),
				ADD COLUMN IF NOT EXISTS cost_audio DECIMAL(10, 4),
				ADD COLUMN IF NOT EXISTS supported_parameters JSONB,
				ADD COLUMN IF NOT EXISTS default_parameters JSONB,
				ADD COLUMN IF NOT EXISTS is_moderated BOOLEAN DEFAULT false
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Remove columns from llm_models table
			if err := tx.Exec(`
				ALTER TABLE llm_models
				DROP COLUMN IF EXISTS description,
				DROP COLUMN IF EXISTS tokenizer,
				DROP COLUMN IF EXISTS instruct_type,
				DROP COLUMN IF EXISTS cost_image,
				DROP COLUMN IF EXISTS cost_audio,
				DROP COLUMN IF EXISTS supported_parameters,
				DROP COLUMN IF EXISTS default_parameters,
				DROP COLUMN IF EXISTS is_moderated
			`).Error; err != nil {
				return err
			}

			// Remove description column from llm_providers table
			if err := tx.Exec(`
				ALTER TABLE llm_providers 
				DROP COLUMN IF EXISTS description,
				DROP COLUMN IF EXISTS openai_compatible
			`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
