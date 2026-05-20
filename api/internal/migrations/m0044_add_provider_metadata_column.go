package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0044_add_provider_metadata_column adds metadata column to llm_providers table
func M0044_add_provider_metadata_column() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251215000044",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE llm_providers
				ADD COLUMN IF NOT EXISTS metadata JSONB DEFAULT '{}'
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE llm_providers DROP COLUMN IF EXISTS metadata
			`).Error
		},
	}
}
