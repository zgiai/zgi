package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0055_add_model_tier adds model tier and recommendation fields
// This enables categorizing models as flagship/premium/standard/basic
// for easier model selection and recommendation
func M0055_add_model_tier() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251217000055",
		Migrate: func(tx *gorm.DB) error {
			// Add model_tier and is_recommended columns
			if err := tx.Exec(`
				ALTER TABLE llm_models
				ADD COLUMN IF NOT EXISTS model_tier VARCHAR(20) DEFAULT NULL,
				ADD COLUMN IF NOT EXISTS is_recommended BOOLEAN DEFAULT false;
			`).Error; err != nil {
				return err
			}

			// Add index for model_tier queries
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_model_tier
				ON llm_models(model_tier)
				WHERE model_tier IS NOT NULL AND deleted_at IS NULL;
			`).Error; err != nil {
				return err
			}

			// Add index for is_recommended queries
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_model_recommended
				ON llm_models(is_recommended)
				WHERE is_recommended = true AND deleted_at IS NULL;
			`).Error; err != nil {
				return err
			}

			// Auto-set is_recommended flag for flagship and premium tiers
			// This is a one-time initialization, future updates will be handled by application logic
			if err := tx.Exec(`
				UPDATE llm_models
				SET is_recommended = true
				WHERE model_tier IN ('flagship', 'premium')
				AND deleted_at IS NULL;
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop indexes first
			if err := tx.Exec(`
				DROP INDEX IF EXISTS idx_model_tier;
				DROP INDEX IF EXISTS idx_model_recommended;
			`).Error; err != nil {
				return err
			}

			// Drop columns
			if err := tx.Exec(`
				ALTER TABLE llm_models
				DROP COLUMN IF EXISTS model_tier,
				DROP COLUMN IF EXISTS is_recommended;
			`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
