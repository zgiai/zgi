package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0147_remove_llm_model_type backfills use_cases and removes legacy type columns.
func M0147_remove_llm_model_type() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260418000047",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(backfillUseCasesSQL("llm_models")).Error; err != nil {
				return err
			}
			if err := tx.Exec(backfillUseCasesSQL("llm_custom_models")).Error; err != nil {
				return err
			}
			if err := tx.Exec(`
				DROP INDEX IF EXISTS idx_llm_models_type;
				DROP INDEX IF EXISTS idx_llm_custom_models_type;
				ALTER TABLE IF EXISTS llm_models DROP COLUMN IF EXISTS type;
				ALTER TABLE IF EXISTS llm_custom_models DROP COLUMN IF EXISTS type;
			`).Error; err != nil {
				return err
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				ALTER TABLE IF EXISTS llm_models ADD COLUMN IF NOT EXISTS type VARCHAR(50);
				ALTER TABLE IF EXISTS llm_custom_models ADD COLUMN IF NOT EXISTS type VARCHAR(50);
			`).Error; err != nil {
				return err
			}
			if err := tx.Exec(restoreTypeSQL("llm_models")).Error; err != nil {
				return err
			}
			if err := tx.Exec(restoreTypeSQL("llm_custom_models")).Error; err != nil {
				return err
			}
			return tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_llm_models_type ON llm_models(type);
				CREATE INDEX IF NOT EXISTS idx_llm_custom_models_type ON llm_custom_models(type);
			`).Error
		},
	}
}

func backfillUseCasesSQL(table string) string {
	return `
		UPDATE ` + table + `
		SET use_cases = CASE
			WHEN type IS NOT NULL AND BTRIM(type) <> '' THEN CASE LOWER(BTRIM(type))
				WHEN 'llm' THEN ARRAY['text-chat']::text[]
				WHEN 'chat' THEN ARRAY['text-chat']::text[]
				WHEN 'text-embedding' THEN ARRAY['embedding']::text[]
				WHEN 'embedding' THEN ARRAY['embedding']::text[]
				WHEN 'embeddings' THEN ARRAY['embedding']::text[]
				WHEN 'rerank' THEN ARRAY['rerank']::text[]
				WHEN 'image' THEN ARRAY['image-gen']::text[]
				WHEN 'image-gen' THEN ARRAY['image-gen']::text[]
				WHEN 'image-generation' THEN ARRAY['image-gen']::text[]
				WHEN 'tts' THEN ARRAY['text-to-speech']::text[]
				WHEN 'speech' THEN ARRAY['text-to-speech']::text[]
				WHEN 'stt' THEN ARRAY['speech-to-text']::text[]
				WHEN 'transcription' THEN ARRAY['speech-to-text']::text[]
				WHEN 'moderation' THEN ARRAY['moderation']::text[]
				ELSE NULL
			END
			WHEN COALESCE(embeddings, false) THEN ARRAY['embedding']::text[]
			WHEN COALESCE(image_generation, false) THEN ARRAY['image-gen']::text[]
			WHEN COALESCE(speech_generation, false) THEN ARRAY['text-to-speech']::text[]
			WHEN COALESCE(transcription, false) THEN ARRAY['speech-to-text']::text[]
			WHEN COALESCE(moderation, false) THEN ARRAY['moderation']::text[]
			ELSE ARRAY['text-chat']::text[]
		END
		WHERE COALESCE(array_length(use_cases, 1), 0) = 0;
	`
}

func restoreTypeSQL(table string) string {
	return `
		UPDATE ` + table + `
		SET type = CASE
			WHEN 'embedding' = ANY(use_cases) THEN 'embedding'
			WHEN 'rerank' = ANY(use_cases) THEN 'rerank'
			WHEN 'image-gen' = ANY(use_cases) THEN 'image'
			WHEN 'text-to-speech' = ANY(use_cases) THEN 'tts'
			WHEN 'speech-to-text' = ANY(use_cases) THEN 'transcription'
			WHEN 'moderation' = ANY(use_cases) THEN 'moderation'
			ELSE 'llm'
		END
		WHERE type IS NULL OR BTRIM(type) = '';
	`
}
