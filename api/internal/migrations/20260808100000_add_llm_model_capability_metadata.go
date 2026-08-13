package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddLLMModelCapabilityMetadataID = "20260808100000_add_llm_model_capability_metadata"

func init() {
	registerSchemaMigration(
		migrationAddLLMModelCapabilityMetadataID,
		upAddLLMModelCapabilityMetadata,
		nil,
	)
}

func upAddLLMModelCapabilityMetadata(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.llm_models
			ADD COLUMN IF NOT EXISTS videos boolean DEFAULT false,
			ADD COLUMN IF NOT EXISTS image_edit boolean DEFAULT false,
			ADD COLUMN IF NOT EXISTS input_modalities jsonb DEFAULT '[]'::jsonb,
			ADD COLUMN IF NOT EXISTS output_modalities jsonb DEFAULT '[]'::jsonb,
			ADD COLUMN IF NOT EXISTS supported_parameters jsonb DEFAULT '[]'::jsonb,
			ADD COLUMN IF NOT EXISTS default_parameters jsonb DEFAULT '{}'::jsonb,
			ADD COLUMN IF NOT EXISTS config_parameters jsonb NOT NULL DEFAULT '[]'::jsonb,
			ADD COLUMN IF NOT EXISTS pricing jsonb NOT NULL DEFAULT '{}'::jsonb
	`)
}
