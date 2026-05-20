package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0019_llm_models_add_endpoints_finetuned adds endpoints and is_finetuned columns to llm_models table
// - endpoints: JSONB array to store supported API endpoints (e.g., ["chat", "embed", "rerank"])
// - is_finetuned: boolean to indicate if the model is a fine-tuned version
func M0019_llm_models_add_endpoints_finetuned() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251127120000",
		Migrate: func(tx *gorm.DB) error {
			// 1. Add endpoints column (JSONB array)
			if err := tx.Exec(`
				ALTER TABLE llm_models 
				ADD COLUMN IF NOT EXISTS endpoints JSONB DEFAULT '[]'::jsonb
			`).Error; err != nil {
				return err
			}

			// 2. Add is_finetuned column
			if err := tx.Exec(`
				ALTER TABLE llm_models 
				ADD COLUMN IF NOT EXISTS is_finetuned BOOLEAN DEFAULT false
			`).Error; err != nil {
				return err
			}

			// 3. Create GIN index for endpoints for efficient querying
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_llm_models_endpoints 
				ON llm_models USING GIN(endpoints)
			`).Error; err != nil {
				return err
			}

			// 4. Create index for is_finetuned
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_llm_models_finetuned 
				ON llm_models(is_finetuned)
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// 1. Drop is_finetuned index
			if err := tx.Exec(`DROP INDEX IF EXISTS idx_llm_models_finetuned`).Error; err != nil {
				return err
			}

			// 2. Drop endpoints index
			if err := tx.Exec(`DROP INDEX IF EXISTS idx_llm_models_endpoints`).Error; err != nil {
				return err
			}

			// 3. Drop is_finetuned column
			if err := tx.Exec(`ALTER TABLE llm_models DROP COLUMN IF EXISTS is_finetuned`).Error; err != nil {
				return err
			}

			// 4. Drop endpoints column
			if err := tx.Exec(`ALTER TABLE llm_models DROP COLUMN IF EXISTS endpoints`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
