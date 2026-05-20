package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0062_add_model_capabilities adds additional capability fields to llm_models
// - supports_audio: Audio input support (e.g., GPT-4o)
// - supports_function_call: Legacy OpenAI function calling
// - supports_json_mode: JSON mode output
// - supports_streaming: Streaming response support (was previously not persisted)
func M0062_add_model_capabilities() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251224000062",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE llm_models
				ADD COLUMN IF NOT EXISTS supports_audio BOOLEAN NOT NULL DEFAULT false,
				ADD COLUMN IF NOT EXISTS supports_function_call BOOLEAN NOT NULL DEFAULT false,
				ADD COLUMN IF NOT EXISTS supports_json_mode BOOLEAN NOT NULL DEFAULT false,
				ADD COLUMN IF NOT EXISTS supports_streaming BOOLEAN NOT NULL DEFAULT true
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE llm_models
				DROP COLUMN IF EXISTS supports_audio,
				DROP COLUMN IF EXISTS supports_function_call,
				DROP COLUMN IF EXISTS supports_json_mode,
				DROP COLUMN IF EXISTS supports_streaming
			`).Error
		},
	}
}
