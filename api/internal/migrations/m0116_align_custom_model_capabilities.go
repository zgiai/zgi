package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0116_align_custom_model_capabilities adds missing capability columns to llm_custom_models
// to achieve strict parity with llm_models (issue #175).
func M0116_align_custom_model_capabilities() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260209000116",
		Migrate: func(tx *gorm.DB) error {
			var customModelsExists bool
			if err := tx.Raw(`
				SELECT EXISTS (
					SELECT 1
					FROM information_schema.tables
					WHERE table_schema = CURRENT_SCHEMA()
					  AND table_name = 'llm_custom_models'
				)
			`).Scan(&customModelsExists).Error; err != nil {
				return err
			}
			if !customModelsExists {
				return nil
			}

			sqls := []string{
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS provider VARCHAR(100)`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS image_generation BOOLEAN DEFAULT false`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS speech_generation BOOLEAN DEFAULT false`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS transcription BOOLEAN DEFAULT false`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS translation BOOLEAN DEFAULT false`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS moderation BOOLEAN DEFAULT false`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS realtime BOOLEAN DEFAULT false`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS batch BOOLEAN DEFAULT false`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS fine_tuning BOOLEAN DEFAULT false`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS assistants BOOLEAN DEFAULT false`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS responses BOOLEAN DEFAULT false`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS audio BOOLEAN DEFAULT false`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS distillation BOOLEAN DEFAULT false`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS system_prompt BOOLEAN DEFAULT true`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS logprobs BOOLEAN DEFAULT false`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS web_search BOOLEAN DEFAULT false`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS file_search BOOLEAN DEFAULT false`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS code_interpreter BOOLEAN DEFAULT false`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS computer_use BOOLEAN DEFAULT false`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS mcp BOOLEAN DEFAULT false`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS reasoning_effort BOOLEAN DEFAULT false`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS parallel_tool_calls BOOLEAN DEFAULT false`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS temperature BOOLEAN DEFAULT true`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS top_p BOOLEAN DEFAULT true`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS presence_penalty BOOLEAN DEFAULT false`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS frequency_penalty BOOLEAN DEFAULT false`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS logit_bias BOOLEAN DEFAULT false`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS seed BOOLEAN DEFAULT false`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS stop BOOLEAN DEFAULT true`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS max_stop_sequences INT DEFAULT 4`,
				`ALTER TABLE llm_custom_models ADD COLUMN IF NOT EXISTS default_parameters JSONB DEFAULT '{}'`,
			}
			for _, sql := range sqls {
				if err := tx.Exec(sql).Error; err != nil {
					return err
				}
			}

			var customProvidersExists bool
			if err := tx.Raw(`
				SELECT EXISTS (
					SELECT 1
					FROM information_schema.tables
					WHERE table_schema = CURRENT_SCHEMA()
					  AND table_name = 'llm_custom_providers'
				)
			`).Scan(&customProvidersExists).Error; err != nil {
				return err
			}
			if customProvidersExists {
				if err := tx.Exec(`
					UPDATE llm_custom_models cm
					SET provider = cp.provider
					FROM llm_custom_providers cp
					WHERE cm.provider_id = cp.id AND (cm.provider IS NULL OR cm.provider = '')
				`).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			columns := []string{
				"provider",
				"image_generation", "speech_generation", "transcription", "translation",
				"moderation", "realtime", "batch", "fine_tuning", "assistants", "responses",
				"audio", "distillation", "system_prompt", "logprobs",
				"web_search", "file_search", "code_interpreter", "computer_use", "mcp", "reasoning_effort",
				"parallel_tool_calls",
				"temperature", "top_p", "presence_penalty", "frequency_penalty",
				"logit_bias", "seed", "stop", "max_stop_sequences", "default_parameters",
			}
			for _, col := range columns {
				tx.Exec("ALTER TABLE llm_custom_models DROP COLUMN IF EXISTS " + col)
			}
			return nil
		},
	}
}
