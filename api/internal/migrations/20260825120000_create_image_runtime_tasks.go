package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationCreateImageRuntimeTasksID = "20260825120000_create_image_runtime_tasks"

func init() {
	registerSchemaMigration(migrationCreateImageRuntimeTasksID, upCreateImageRuntimeTasks, nil)
}

func upCreateImageRuntimeTasks(schema *mschema.Builder) error {
	return schema.Raw(`
		CREATE TABLE IF NOT EXISTS public.image_runtime_tasks (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			organization_id uuid NOT NULL,
			account_id uuid NOT NULL,
			workspace_id uuid NULL,
			task_id varchar(160) NOT NULL,
			client_request_id varchar(120) NOT NULL DEFAULT '',
			conversation_id varchar(160) NOT NULL DEFAULT '',
			message_id varchar(160) NOT NULL DEFAULT '',
			provider varchar(100) NOT NULL,
			model varchar(255) NOT NULL,
			model_label varchar(255) NOT NULL DEFAULT '',
			prompt text NOT NULL DEFAULT '',
			status varchar(40) NOT NULL DEFAULT 'pending',
			size varchar(64) NOT NULL DEFAULT '',
			count integer NOT NULL DEFAULT 1,
			generation_mode varchar(40) NOT NULL DEFAULT '',
			max_images integer NULL,
			files jsonb NOT NULL DEFAULT '[]'::jsonb,
			reference_image jsonb NOT NULL DEFAULT 'null'::jsonb,
			error_message text NOT NULL DEFAULT '',
			request_payload jsonb NOT NULL DEFAULT '{}'::jsonb,
			response_payload jsonb NOT NULL DEFAULT '{}'::jsonb,
			created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
			completed_at timestamp with time zone NULL
		);

		CREATE UNIQUE INDEX IF NOT EXISTS idx_image_runtime_tasks_task_id
		ON public.image_runtime_tasks (task_id);

		CREATE UNIQUE INDEX IF NOT EXISTS idx_image_runtime_tasks_client_request
		ON public.image_runtime_tasks (organization_id, account_id, client_request_id)
		WHERE client_request_id <> '';

		CREATE INDEX IF NOT EXISTS idx_image_runtime_tasks_scope_created
		ON public.image_runtime_tasks (organization_id, account_id, created_at DESC);

		CREATE INDEX IF NOT EXISTS idx_image_runtime_tasks_scope_status
		ON public.image_runtime_tasks (organization_id, account_id, status);
		`)
}
