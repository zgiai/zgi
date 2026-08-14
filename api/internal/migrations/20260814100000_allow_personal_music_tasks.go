package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAllowPersonalMusicTasksID = "20260814100000_allow_personal_music_tasks"

func init() {
	registerSchemaMigration(
		migrationAllowPersonalMusicTasksID,
		upAllowPersonalMusicTasks,
		nil,
	)
}

func upAllowPersonalMusicTasks(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.music_generation_tasks
			ALTER COLUMN workspace_id DROP NOT NULL;
		CREATE UNIQUE INDEX IF NOT EXISTS idx_music_tasks_personal_request
			ON public.music_generation_tasks (organization_id, account_id, request_id)
			WHERE workspace_id IS NULL;
		CREATE INDEX IF NOT EXISTS idx_music_tasks_account_created
			ON public.music_generation_tasks (organization_id, account_id, created_at DESC, id DESC)
	`)
}
