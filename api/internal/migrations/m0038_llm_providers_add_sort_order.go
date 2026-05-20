package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0038_llm_providers_add_sort_order adds sort_order column to llm_providers table
func M0038_llm_providers_add_sort_order() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251214000038",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE llm_providers
				ADD COLUMN IF NOT EXISTS sort_order INT DEFAULT 0
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE llm_providers DROP COLUMN IF EXISTS sort_order
			`).Error
		},
	}
}

// M0039_llm_models_add_sort_order adds sort_order column to llm_models table
func M0039_llm_models_add_sort_order() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251214000039",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE llm_models
				ADD COLUMN IF NOT EXISTS sort_order INT DEFAULT 0
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE llm_models DROP COLUMN IF EXISTS sort_order
			`).Error
		},
	}
}
