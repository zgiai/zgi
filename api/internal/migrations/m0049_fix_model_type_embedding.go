package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0049FixModelTypeEmbedding migrates model type from 'embedding' to 'text-embedding'
func M0049FixModelTypeEmbedding() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251216000049",
		Migrate: func(tx *gorm.DB) error {
			// Update all models with type 'embedding' to 'text-embedding'
			return tx.Exec(`
				UPDATE llm_models
				SET type = 'text-embedding'
				WHERE type = 'embedding'
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			// Rollback: convert back to 'embedding'
			return tx.Exec(`
				UPDATE llm_models
				SET type = 'embedding'
				WHERE type = 'text-embedding'
			`).Error
		},
	}
}
