package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddMusicTaskOwnerIndexID = "20260806100000_add_music_task_owner_index"

func init() {
	registerSchemaMigration(
		migrationAddMusicTaskOwnerIndexID,
		upAddMusicTaskOwnerIndex,
		downAddMusicTaskOwnerIndex,
	)
}

func upAddMusicTaskOwnerIndex(schema *mschema.Builder) error {
	return schema.Raw(`
		CREATE INDEX IF NOT EXISTS idx_music_tasks_owner_created
		ON public.music_generation_tasks (organization_id, workspace_id, account_id, created_at DESC, id DESC)
	`)
}

func downAddMusicTaskOwnerIndex(schema *mschema.Builder) error {
	return schema.Raw(`DROP INDEX IF EXISTS public.idx_music_tasks_owner_created`)
}
