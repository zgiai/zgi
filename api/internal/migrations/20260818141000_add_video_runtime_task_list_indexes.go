package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddVideoRuntimeTaskListIndexesID = "20260818141000_add_video_runtime_task_list_indexes"

func init() {
	registerSchemaMigration(migrationAddVideoRuntimeTaskListIndexesID, upAddVideoRuntimeTaskListIndexes, nil)
}

func upAddVideoRuntimeTaskListIndexes(schema *mschema.Builder) error {
	return schema.Raw(`
		CREATE INDEX IF NOT EXISTS idx_video_runtime_tasks_scope_created_id
		ON public.video_runtime_tasks (organization_id, account_id, created_at DESC, id DESC);

		CREATE INDEX IF NOT EXISTS idx_video_runtime_tasks_workspace_created_id
		ON public.video_runtime_tasks (organization_id, account_id, workspace_id, created_at DESC, id DESC);
	`)
}
