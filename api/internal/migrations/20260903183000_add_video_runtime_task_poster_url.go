package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddVideoRuntimeTaskPosterURLID = "20260903183000_add_video_runtime_task_poster_url"

func init() {
	registerSchemaMigration(migrationAddVideoRuntimeTaskPosterURLID, upAddVideoRuntimeTaskPosterURL, nil)
}

func upAddVideoRuntimeTaskPosterURL(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.video_runtime_tasks
		ADD COLUMN IF NOT EXISTS poster_url text NOT NULL DEFAULT '';
	`)
}
