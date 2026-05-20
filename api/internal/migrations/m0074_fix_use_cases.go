package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0073_fix_use_cases fixes missing use_cases data for embedding models
func M0074_fix_use_cases() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260107000073",
		Migrate: func(tx *gorm.DB) error {
			// Update all text-embedding models to have 'embedding' use_case
			if err := tx.Exec(`
				UPDATE llm_models
				SET use_cases = ARRAY['embedding']::text[]
				WHERE type IN ('text-embedding', 'embedding')
				  AND (use_cases IS NULL OR array_length(use_cases, 1) IS NULL OR array_length(use_cases, 1) = 0)
			`).Error; err != nil {
				return err
			}

			// Update rerank models
			if err := tx.Exec(`
				UPDATE llm_models
				SET use_cases = ARRAY['rerank']::text[]
				WHERE type = 'rerank'
				  AND (use_cases IS NULL OR array_length(use_cases, 1) IS NULL OR array_length(use_cases, 1) = 0)
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// No rollback needed - data fix is safe
			return nil
		},
	}
}
