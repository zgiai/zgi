package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0114_use_cases_refactor migrates type→use_cases for models.
//
// Changes:
// 1. llm_models: backfill empty use_cases based on type (embedding→{embedding}, rerank→{rerank}, llm→{text-chat})
// 2. llm_custom_models: same backfill for empty use_cases
func M0114_use_cases_refactor() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260208000114",
		Migrate: func(tx *gorm.DB) error {
			tableExists := func(name string) (bool, error) {
				var exists bool
				err := tx.Raw(`
					SELECT EXISTS (
						SELECT 1
						FROM information_schema.tables
						WHERE table_schema = CURRENT_SCHEMA()
						  AND table_name = ?
					)
				`, name).Scan(&exists).Error
				return exists, err
			}

			// 1. Backfill llm_models: empty use_cases based on type
			exists, err := tableExists("llm_models")
			if err != nil {
				return err
			}
			if exists {
				if err := tx.Exec(`
					UPDATE llm_models SET use_cases = ARRAY['embedding']
					WHERE use_cases = '{}' AND type IN ('text-embedding', 'embedding')
				`).Error; err != nil {
					return err
				}
				if err := tx.Exec(`
					UPDATE llm_models SET use_cases = ARRAY['rerank']
					WHERE use_cases = '{}' AND type = 'rerank'
				`).Error; err != nil {
					return err
				}
				if err := tx.Exec(`
					UPDATE llm_models SET use_cases = ARRAY['text-chat']
					WHERE use_cases = '{}' AND (type = 'llm' OR type = 'chat' OR type = '' OR type IS NULL)
				`).Error; err != nil {
					return err
				}
			}

			// 2. Backfill llm_custom_models: same logic
			exists, err = tableExists("llm_custom_models")
			if err != nil {
				return err
			}
			if exists {
				if err := tx.Exec(`
					UPDATE llm_custom_models SET use_cases = ARRAY['embedding']
					WHERE use_cases = '{}' AND type IN ('text-embedding', 'embedding')
				`).Error; err != nil {
					return err
				}
				if err := tx.Exec(`
					UPDATE llm_custom_models SET use_cases = ARRAY['rerank']
					WHERE use_cases = '{}' AND type = 'rerank'
				`).Error; err != nil {
					return err
				}
				if err := tx.Exec(`
					UPDATE llm_custom_models SET use_cases = ARRAY['text-chat']
					WHERE use_cases = '{}' AND (type = 'llm' OR type = 'chat' OR type = '' OR type IS NULL)
				`).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}
