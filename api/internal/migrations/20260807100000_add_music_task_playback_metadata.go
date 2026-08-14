package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddMusicTaskPlaybackMetadataID = "20260807100000_add_music_task_playback_metadata"

func init() {
	registerSchemaMigration(
		migrationAddMusicTaskPlaybackMetadataID,
		upAddMusicTaskPlaybackMetadata,
		nil,
	)
}

func upAddMusicTaskPlaybackMetadata(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.music_generation_tasks
			ADD COLUMN IF NOT EXISTS title varchar(255) NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS style_tags jsonb NOT NULL DEFAULT '[]'::jsonb,
			ADD COLUMN IF NOT EXISTS duration_ms bigint NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS waveform_peaks jsonb NOT NULL DEFAULT '[]'::jsonb
	`)
}
