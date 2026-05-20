package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0061_add_model_temperature_params adds temperature parameter range fields to llm_models
// This allows per-model configuration of temperature min/max/default values
// Different providers have different temperature ranges (e.g., OpenAI: 0-2, Claude: 0-1)
func M0061_add_model_temperature_params() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251224000061",
		Migrate: func(tx *gorm.DB) error {
			// Add temperature parameter fields
			if err := tx.Exec(`
				ALTER TABLE llm_models
				ADD COLUMN IF NOT EXISTS temperature_min DECIMAL(4,2) DEFAULT 0,
				ADD COLUMN IF NOT EXISTS temperature_max DECIMAL(4,2) DEFAULT 2,
				ADD COLUMN IF NOT EXISTS temperature_default DECIMAL(4,2) DEFAULT 1
			`).Error; err != nil {
				return err
			}

			// Update default values based on known provider ranges
			// Claude/Anthropic models: 0-1
			if err := tx.Exec(`
				UPDATE llm_models
				SET temperature_max = 1
				WHERE provider IN ('anthropic', 'claude')
				AND temperature_max IS NULL OR temperature_max = 2
			`).Error; err != nil {
				return err
			}

			// Mistral models: 0-1, default 0.7
			if err := tx.Exec(`
				UPDATE llm_models
				SET temperature_max = 1, temperature_default = 0.7
				WHERE provider = 'mistral'
				AND (temperature_max IS NULL OR temperature_max = 2)
			`).Error; err != nil {
				return err
			}

			// Cohere models: 0-1, default 0.3
			if err := tx.Exec(`
				UPDATE llm_models
				SET temperature_max = 1, temperature_default = 0.3
				WHERE provider = 'cohere'
				AND (temperature_max IS NULL OR temperature_max = 2)
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE llm_models
				DROP COLUMN IF EXISTS temperature_min,
				DROP COLUMN IF EXISTS temperature_max,
				DROP COLUMN IF EXISTS temperature_default
			`).Error
		},
	}
}
