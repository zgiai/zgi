package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0064_add_use_cases adds use_cases field to llm_models and llm_tenant_custom_models tables
func M0064_add_use_cases() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "0064_add_use_cases",
		Migrate: func(tx *gorm.DB) error {
			// 1. Add use_cases column to llm_models
			if err := tx.Exec(`
				ALTER TABLE llm_models 
				ADD COLUMN IF NOT EXISTS use_cases TEXT[] DEFAULT '{}' NOT NULL
			`).Error; err != nil {
				return err
			}

			// 2. Add use_cases column to llm_tenant_custom_models
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_custom_models 
				ADD COLUMN IF NOT EXISTS use_cases TEXT[] DEFAULT '{}' NOT NULL
			`).Error; err != nil {
				return err
			}

			// 3. Create GIN index for efficient array queries on llm_models
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_llm_models_use_cases 
				ON llm_models USING GIN (use_cases)
			`).Error; err != nil {
				return err
			}

			// 4. Create GIN index for llm_tenant_custom_models
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_llm_tenant_custom_models_use_cases 
				ON llm_tenant_custom_models USING GIN (use_cases)
			`).Error; err != nil {
				return err
			}

			// 5. Add CHECK constraint for valid use_cases values on llm_models
			if err := tx.Exec(`
				ALTER TABLE llm_models 
				ADD CONSTRAINT IF NOT EXISTS check_llm_models_use_cases_values CHECK (
					use_cases <@ ARRAY[
						'text-chat', 'vision', 'image-gen', 'embedding', 'rerank',
						'speech-to-text', 'text-to-speech', 'realtime-audio',
						'video-gen', 'moderation', 'reasoning', 'function-calling'
					]::TEXT[]
				)
			`).Error; err != nil {
				// Constraint might already exist or syntax varies, continue
				tx.Logger.Warn(tx.Statement.Context, "Warning: could not add check constraint to llm_models: %v", err)
			}

			// 6. Add CHECK constraint for llm_tenant_custom_models
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_custom_models 
				ADD CONSTRAINT IF NOT EXISTS check_llm_tenant_custom_models_use_cases_values CHECK (
					use_cases <@ ARRAY[
						'text-chat', 'vision', 'image-gen', 'embedding', 'rerank',
						'speech-to-text', 'text-to-speech', 'realtime-audio',
						'video-gen', 'moderation', 'reasoning', 'function-calling'
					]::TEXT[]
				)
			`).Error; err != nil {
				// Constraint might already exist or syntax varies, continue
				tx.Logger.Warn(tx.Statement.Context, "Warning: could not add check constraint to llm_tenant_custom_models: %v", err)
			}

			// 7. Add comments for documentation
			tx.Exec(`COMMENT ON COLUMN llm_models.use_cases IS 'Usage scenarios: text-chat, vision, image-gen, embedding, rerank, speech-to-text, text-to-speech, realtime-audio, video-gen, moderation, reasoning, function-calling'`)
			tx.Exec(`COMMENT ON COLUMN llm_tenant_custom_models.use_cases IS 'Usage scenarios for tenant custom models'`)

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop constraints first
			tx.Exec(`ALTER TABLE llm_models DROP CONSTRAINT IF EXISTS check_llm_models_use_cases_values`)
			tx.Exec(`ALTER TABLE llm_tenant_custom_models DROP CONSTRAINT IF EXISTS check_llm_tenant_custom_models_use_cases_values`)

			// Drop indexes
			tx.Exec(`DROP INDEX IF EXISTS idx_llm_models_use_cases`)
			tx.Exec(`DROP INDEX IF EXISTS idx_llm_tenant_custom_models_use_cases`)

			// Drop columns
			if err := tx.Exec(`ALTER TABLE llm_models DROP COLUMN IF EXISTS use_cases`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`ALTER TABLE llm_tenant_custom_models DROP COLUMN IF EXISTS use_cases`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
