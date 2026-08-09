package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationCreateVideoRuntimeTasksID = "20260808090000_create_video_runtime_tasks"

func init() {
	registerSchemaMigration(migrationCreateVideoRuntimeTasksID, upCreateVideoRuntimeTasks, nil)
}

func upCreateVideoRuntimeTasks(schema *mschema.Builder) error {
	return schema.Raw(`
		CREATE TABLE IF NOT EXISTS public.video_runtime_tasks (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			organization_id uuid NOT NULL,
			account_id uuid NOT NULL,
			workspace_id uuid NULL,
			task_id varchar(160) NOT NULL,
			upstream_task_id varchar(255) NOT NULL DEFAULT '',
			provider varchar(100) NOT NULL,
			model varchar(255) NOT NULL,
			model_label varchar(255) NOT NULL DEFAULT '',
			prompt text NOT NULL DEFAULT '',
			status varchar(40) NOT NULL DEFAULT 'pending',
			video_url text NOT NULL DEFAULT '',
			error_message text NOT NULL DEFAULT '',
			duration_seconds integer NOT NULL DEFAULT 0,
			resolution varchar(32) NOT NULL DEFAULT '',
			ratio varchar(32) NOT NULL DEFAULT '',
			has_input_video boolean NOT NULL DEFAULT false,
			generate_audio boolean NOT NULL DEFAULT false,
			voice varchar(80) NOT NULL DEFAULT '',
			estimated_credits bigint NOT NULL DEFAULT 0,
			actual_credits bigint NOT NULL DEFAULT 0,
			request_payload jsonb NOT NULL DEFAULT '{}'::jsonb,
			response_payload jsonb NOT NULL DEFAULT '{}'::jsonb,
			created_at timestamp without time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamp without time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
			completed_at timestamp without time zone NULL
		);

		CREATE UNIQUE INDEX IF NOT EXISTS idx_video_runtime_tasks_task_id
		ON public.video_runtime_tasks (task_id);

		CREATE INDEX IF NOT EXISTS idx_video_runtime_tasks_scope_created
		ON public.video_runtime_tasks (organization_id, account_id, created_at DESC);

		CREATE INDEX IF NOT EXISTS idx_video_runtime_tasks_scope_status
		ON public.video_runtime_tasks (organization_id, account_id, status);
	`)
}
