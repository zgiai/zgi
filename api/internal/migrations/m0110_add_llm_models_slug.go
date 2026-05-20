package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0110_add_llm_models_slug adds slug column to llm_models table
// This fixes the "column does not exist" error during vector generation (embedding)
func M0110_add_llm_models_slug() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260204020000",
		Migrate: func(tx *gorm.DB) error {
			// Add column
			if err := tx.Exec(`
				ALTER TABLE llm_models 
				ADD COLUMN IF NOT EXISTS slug VARCHAR(255) DEFAULT '';
			`).Error; err != nil {
				return err
			}
			
			// Populate empty slugs with name (best guess fallback)
			return tx.Exec(`UPDATE llm_models SET slug = name WHERE slug = '' OR slug IS NULL;`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE llm_models 
				DROP COLUMN IF EXISTS slug;
			`).Error
		},
	}
}
