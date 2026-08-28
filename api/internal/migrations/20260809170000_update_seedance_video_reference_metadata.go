package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

const migrationUpdateSeedanceVideoReferenceMetadataID = "20260809170000_update_seedance_video_reference_metadata"

func init() {
	registerSchemaMigration(
		migrationUpdateSeedanceVideoReferenceMetadataID,
		upUpdateSeedanceVideoReferenceMetadata,
		nil,
	)
}

func upUpdateSeedanceVideoReferenceMetadata(schema *mschema.Builder) error {
	return schema.DataFix("update Seedance video reference metadata", func(db *gorm.DB) error {
		return db.Exec(`
		UPDATE public.llm_models
		SET
			input_modalities = '["text", "image", "video", "audio"]'::jsonb,
			output_modalities = '["video", "audio"]'::jsonb,
			default_parameters = jsonb_set(
				COALESCE(default_parameters, '{}'::jsonb),
				'{capabilities}',
				COALESCE(default_parameters->'capabilities', '{}'::jsonb) || '{"video":{"references":{"audio_max_items":3,"image_max_items":9,"video_max_items":3},"audio":{"input":true,"generation":true},"reference_modes":["auto","first_last_frame"],"first_last_frame":{"image_max_items":2},"duration":[{"mode":"range","min_seconds":4,"max_seconds":15,"step_seconds":1}]}}'::jsonb,
				true
			),
			updated_at = NOW()
		WHERE provider = 'doubao'
			AND name IN (
				'doubao-seedance-2-0-260128',
				'doubao-seedance-2-0-fast-260128',
				'doubao-seedance-2-0-mini-260615'
			)
			AND deleted_at IS NULL
		`).Error
	})
}
