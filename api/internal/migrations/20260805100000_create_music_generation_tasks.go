package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationCreateMusicGenerationTasksID = "20260805100000_create_music_generation_tasks"

func init() {
	registerSchemaMigration(
		migrationCreateMusicGenerationTasksID,
		upCreateMusicGenerationTasks,
		downCreateMusicGenerationTasks,
	)
}

func upCreateMusicGenerationTasks(schema *mschema.Builder) error {
	return schema.Raw(`
		CREATE TABLE IF NOT EXISTS public.music_generation_tasks (
			id uuid PRIMARY KEY,
			organization_id uuid NOT NULL,
			workspace_id uuid NOT NULL,
			account_id uuid NOT NULL,
			request_id uuid NOT NULL,
			model varchar(255) NOT NULL,
			mode varchar(32) NOT NULL,
			prompt text NOT NULL,
			lyrics text NOT NULL DEFAULT '',
			response_format varchar(16) NOT NULL,
			status varchar(32) NOT NULL,
			file_id uuid,
			error_code varchar(64) NOT NULL DEFAULT '',
			error_message varchar(255) NOT NULL DEFAULT '',
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			started_at timestamptz,
			completed_at timestamptz
		);
		CREATE INDEX IF NOT EXISTS idx_music_tasks_scope_created
			ON public.music_generation_tasks (organization_id, workspace_id, created_at DESC);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_music_tasks_request
			ON public.music_generation_tasks (organization_id, workspace_id, account_id, request_id);
		CREATE INDEX IF NOT EXISTS idx_music_tasks_status_updated
			ON public.music_generation_tasks (status, updated_at)
	`)
}

func downCreateMusicGenerationTasks(schema *mschema.Builder) error {
	return schema.DropIfExists("music_generation_tasks")
}
