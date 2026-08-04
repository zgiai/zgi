package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddMusicGenerationEndpointID = "20260804140000_add_music_generation_endpoint"

func init() {
	registerSchemaMigration(
		migrationAddMusicGenerationEndpointID,
		upAddMusicGenerationEndpoint,
		nil,
	)
}

func upAddMusicGenerationEndpoint(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.llm_models
			ADD COLUMN IF NOT EXISTS music_generation boolean NOT NULL DEFAULT false
	`)
}
