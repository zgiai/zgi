package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0068_clean_capability_naming renames capability fields to cleaner names
// and adds missing capability columns as flat fields (no JSONB)
func M0068_clean_capability_naming() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "0068_clean_capability_naming",
		Migrate: func(tx *gorm.DB) error {
			// ============================================================
			// llm_models table
			// ============================================================

			// Rename existing columns to cleaner names
			renameSQL := `
				-- Rename supports_xxx to simple names
				ALTER TABLE llm_models RENAME COLUMN supports_vision TO vision;
				ALTER TABLE llm_models RENAME COLUMN supports_reasoning TO reasoning;
				ALTER TABLE llm_models RENAME COLUMN supports_streaming TO streaming;
				ALTER TABLE llm_models RENAME COLUMN supports_tool_call TO function_calling;
				ALTER TABLE llm_models RENAME COLUMN supports_structured_output TO structured_output;
				ALTER TABLE llm_models RENAME COLUMN supports_json_mode TO json_mode;
				ALTER TABLE llm_models RENAME COLUMN supports_function_call TO legacy_function_call;
				ALTER TABLE llm_models RENAME COLUMN supports_audio TO audio;
				ALTER TABLE llm_models RENAME COLUMN supports_attachment TO attachment;
				ALTER TABLE llm_models RENAME COLUMN supports_temperature TO temperature;
			`
			if err := tx.Exec(renameSQL).Error; err != nil {
				// Some columns might not exist, continue
			}

			// Add new capability columns
			addColumnsSQL := `
				-- Endpoint capabilities
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS chat_completions BOOLEAN DEFAULT true;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS embeddings BOOLEAN DEFAULT false;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS image_generation BOOLEAN DEFAULT false;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS speech_generation BOOLEAN DEFAULT false;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS transcription BOOLEAN DEFAULT false;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS moderation BOOLEAN DEFAULT false;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS realtime BOOLEAN DEFAULT false;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS batch BOOLEAN DEFAULT false;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS fine_tuning BOOLEAN DEFAULT false;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS assistants BOOLEAN DEFAULT false;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS responses BOOLEAN DEFAULT false;

				-- Feature capabilities (some already exist after rename)
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS distillation BOOLEAN DEFAULT false;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS system_prompt BOOLEAN DEFAULT true;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS logprobs BOOLEAN DEFAULT false;

				-- Tool capabilities
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS web_search BOOLEAN DEFAULT false;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS file_search BOOLEAN DEFAULT false;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS code_interpreter BOOLEAN DEFAULT false;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS computer_use BOOLEAN DEFAULT false;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS mcp BOOLEAN DEFAULT false;

				-- Parameter capabilities
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS seed BOOLEAN DEFAULT false;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS stop BOOLEAN DEFAULT true;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS max_stop_sequences INT DEFAULT 4;

				-- Drop JSONB columns (we use flat columns now)
				ALTER TABLE llm_models DROP COLUMN IF EXISTS endpoints;
				ALTER TABLE llm_models DROP COLUMN IF EXISTS features;
				ALTER TABLE llm_models DROP COLUMN IF EXISTS tools;
				ALTER TABLE llm_models DROP COLUMN IF EXISTS parameters;
			`
			if err := tx.Exec(addColumnsSQL).Error; err != nil {
				return err
			}

			// ============================================================
			// llm_tenant_custom_models table - same changes
			// ============================================================
			renameCustomSQL := `
				ALTER TABLE llm_tenant_custom_models RENAME COLUMN supports_vision TO vision;
				ALTER TABLE llm_tenant_custom_models RENAME COLUMN supports_reasoning TO reasoning;
				ALTER TABLE llm_tenant_custom_models RENAME COLUMN supports_tool_call TO function_calling;
			`
			tx.Exec(renameCustomSQL) // Ignore errors for missing columns

			addCustomColumnsSQL := `
				-- Endpoint capabilities
				ALTER TABLE llm_tenant_custom_models ADD COLUMN IF NOT EXISTS chat_completions BOOLEAN DEFAULT true;
				ALTER TABLE llm_tenant_custom_models ADD COLUMN IF NOT EXISTS embeddings BOOLEAN DEFAULT false;
				ALTER TABLE llm_tenant_custom_models ADD COLUMN IF NOT EXISTS streaming BOOLEAN DEFAULT true;
				ALTER TABLE llm_tenant_custom_models ADD COLUMN IF NOT EXISTS structured_output BOOLEAN DEFAULT false;
				ALTER TABLE llm_tenant_custom_models ADD COLUMN IF NOT EXISTS json_mode BOOLEAN DEFAULT false;

				-- Drop JSONB columns
				ALTER TABLE llm_tenant_custom_models DROP COLUMN IF EXISTS endpoints;
				ALTER TABLE llm_tenant_custom_models DROP COLUMN IF EXISTS features;
				ALTER TABLE llm_tenant_custom_models DROP COLUMN IF EXISTS tools;
				ALTER TABLE llm_tenant_custom_models DROP COLUMN IF EXISTS parameters;
			`
			if err := tx.Exec(addCustomColumnsSQL).Error; err != nil {
				return err
			}

			// Add comments for documentation
			tx.Exec(`COMMENT ON COLUMN llm_models.vision IS 'Supports image/vision input'`)
			tx.Exec(`COMMENT ON COLUMN llm_models.reasoning IS 'Has reasoning/thinking capabilities'`)
			tx.Exec(`COMMENT ON COLUMN llm_models.function_calling IS 'Supports function/tool calling'`)
			tx.Exec(`COMMENT ON COLUMN llm_models.chat_completions IS 'Supports chat completions endpoint'`)

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Rollback: rename back to supports_xxx
			rollbackSQL := `
				ALTER TABLE llm_models RENAME COLUMN vision TO supports_vision;
				ALTER TABLE llm_models RENAME COLUMN reasoning TO supports_reasoning;
				ALTER TABLE llm_models RENAME COLUMN streaming TO supports_streaming;
				ALTER TABLE llm_models RENAME COLUMN function_calling TO supports_tool_call;
				ALTER TABLE llm_models RENAME COLUMN structured_output TO supports_structured_output;
				ALTER TABLE llm_models RENAME COLUMN json_mode TO supports_json_mode;
				ALTER TABLE llm_models RENAME COLUMN legacy_function_call TO supports_function_call;
				ALTER TABLE llm_models RENAME COLUMN audio TO supports_audio;
				ALTER TABLE llm_models RENAME COLUMN attachment TO supports_attachment;
				ALTER TABLE llm_models RENAME COLUMN temperature TO supports_temperature;

				ALTER TABLE llm_tenant_custom_models RENAME COLUMN vision TO supports_vision;
				ALTER TABLE llm_tenant_custom_models RENAME COLUMN reasoning TO supports_reasoning;
				ALTER TABLE llm_tenant_custom_models RENAME COLUMN function_calling TO supports_tool_call;
			`
			tx.Exec(rollbackSQL)

			// Drop new columns
			newColumns := []string{
				"chat_completions", "embeddings", "image_generation", "speech_generation",
				"transcription", "moderation", "realtime", "batch", "fine_tuning",
				"assistants", "responses", "distillation", "system_prompt", "logprobs",
				"web_search", "file_search", "code_interpreter", "computer_use", "mcp",
				"seed", "stop", "max_stop_sequences",
			}
			for _, col := range newColumns {
				tx.Exec("ALTER TABLE llm_models DROP COLUMN IF EXISTS " + col)
			}

			return nil
		},
	}
}
