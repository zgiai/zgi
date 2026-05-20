package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0065_openai_field_naming renames fields to align with OpenAI naming conventions
func M0065_openai_field_naming() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "0065_openai_field_naming",
		Migrate: func(tx *gorm.DB) error {
			// ============================================================
			// llm_models table
			// ============================================================

			// 1. Rename context_limit → context_window (OpenAI naming)
			if err := tx.Exec(`
				ALTER TABLE llm_models 
				RENAME COLUMN context_limit TO context_window
			`).Error; err != nil {
				tx.Logger.Warn(tx.Statement.Context, "context_window rename: %v", err)
			}

			// 2. Rename output_limit → max_output_tokens (OpenAI naming)
			if err := tx.Exec(`
				ALTER TABLE llm_models 
				RENAME COLUMN output_limit TO max_output_tokens
			`).Error; err != nil {
				tx.Logger.Warn(tx.Statement.Context, "max_output_tokens rename: %v", err)
			}

			// 3. Rename cost_input → input_price (OpenAI naming)
			if err := tx.Exec(`
				ALTER TABLE llm_models 
				RENAME COLUMN cost_input TO input_price
			`).Error; err != nil {
				tx.Logger.Warn(tx.Statement.Context, "input_price rename: %v", err)
			}

			// 4. Rename cost_output → output_price (OpenAI naming)
			if err := tx.Exec(`
				ALTER TABLE llm_models 
				RENAME COLUMN cost_output TO output_price
			`).Error; err != nil {
				tx.Logger.Warn(tx.Statement.Context, "output_price rename: %v", err)
			}

			// 5. Add max_input_tokens (new field)
			if err := tx.Exec(`
				ALTER TABLE llm_models 
				ADD COLUMN IF NOT EXISTS max_input_tokens INT DEFAULT 0
			`).Error; err != nil {
				return err
			}

			// ============================================================
			// llm_tenant_custom_models table
			// ============================================================

			// 1. Rename context_limit → context_window (OpenAI naming)
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_custom_models 
				RENAME COLUMN context_limit TO context_window
			`).Error; err != nil {
				tx.Logger.Warn(tx.Statement.Context, "tenant context_window rename: %v", err)
			}

			// 2. Rename output_limit → max_output_tokens (OpenAI naming)
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_custom_models 
				RENAME COLUMN output_limit TO max_output_tokens
			`).Error; err != nil {
				tx.Logger.Warn(tx.Statement.Context, "tenant max_output_tokens rename: %v", err)
			}

			// 3. Rename cost_input → input_price (OpenAI naming)
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_custom_models 
				RENAME COLUMN cost_input TO input_price
			`).Error; err != nil {
				tx.Logger.Warn(tx.Statement.Context, "tenant input_price rename: %v", err)
			}

			// 4. Rename cost_output → output_price (OpenAI naming)
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_custom_models 
				RENAME COLUMN cost_output TO output_price
			`).Error; err != nil {
				tx.Logger.Warn(tx.Statement.Context, "tenant output_price rename: %v", err)
			}

			// 5. Add max_input_tokens (new field)
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_custom_models 
				ADD COLUMN IF NOT EXISTS max_input_tokens INT DEFAULT 0
			`).Error; err != nil {
				return err
			}

			// Add comments for documentation
			tx.Exec(`COMMENT ON COLUMN llm_models.input_price IS 'Input token price per million tokens (OpenAI naming)'`)
			tx.Exec(`COMMENT ON COLUMN llm_models.output_price IS 'Output token price per million tokens (OpenAI naming)'`)
			tx.Exec(`COMMENT ON COLUMN llm_models.context_window IS 'Maximum context window size in tokens (OpenAI naming)'`)
			tx.Exec(`COMMENT ON COLUMN llm_models.max_output_tokens IS 'Maximum output tokens (OpenAI naming)'`)
			tx.Exec(`COMMENT ON COLUMN llm_models.max_input_tokens IS 'Maximum input tokens (OpenAI naming)'`)

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Reverse the renames
			tx.Exec(`ALTER TABLE llm_models RENAME COLUMN context_window TO context_limit`)
			tx.Exec(`ALTER TABLE llm_models RENAME COLUMN max_output_tokens TO output_limit`)
			tx.Exec(`ALTER TABLE llm_models RENAME COLUMN input_price TO cost_input`)
			tx.Exec(`ALTER TABLE llm_models RENAME COLUMN output_price TO cost_output`)
			tx.Exec(`ALTER TABLE llm_models DROP COLUMN IF EXISTS max_input_tokens`)

			tx.Exec(`ALTER TABLE llm_tenant_custom_models RENAME COLUMN context_window TO context_limit`)
			tx.Exec(`ALTER TABLE llm_tenant_custom_models RENAME COLUMN max_output_tokens TO output_limit`)
			tx.Exec(`ALTER TABLE llm_tenant_custom_models RENAME COLUMN input_price TO cost_input`)
			tx.Exec(`ALTER TABLE llm_tenant_custom_models RENAME COLUMN output_price TO cost_output`)
			tx.Exec(`ALTER TABLE llm_tenant_custom_models DROP COLUMN IF EXISTS max_input_tokens`)

			return nil
		},
	}
}
