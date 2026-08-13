package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddVideoTaskClientRequestID = "20260812101000_add_video_task_client_request_id"

func init() {
	registerSchemaMigration(migrationAddVideoTaskClientRequestID, upAddVideoTaskClientRequestID, nil)
}

func upAddVideoTaskClientRequestID(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.video_runtime_tasks
			ADD COLUMN IF NOT EXISTS client_request_id varchar(120) NOT NULL DEFAULT '';

		CREATE UNIQUE INDEX IF NOT EXISTS idx_video_runtime_tasks_client_request
		ON public.video_runtime_tasks (organization_id, account_id, client_request_id)
		WHERE client_request_id <> '';
	`)
}
