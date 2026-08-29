package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

const migrationBackfillVisibleImageRuntimeTasksID = "20260825123000_backfill_visible_image_runtime_tasks"

func init() {
	registerSchemaMigration(migrationBackfillVisibleImageRuntimeTasksID, upBackfillVisibleImageRuntimeTasks, nil)
}

func upBackfillVisibleImageRuntimeTasks(schema *mschema.Builder) error {
	return schema.DataFix("backfill visible image runtime pending messages", func(db *gorm.DB) error {
		return db.Exec(`
			UPDATE public.chat_runtime_conversations AS c
			SET
				current_leaf_message_id = t.message_id::uuid,
				active_message_id = t.message_id::uuid,
				runtime_status = 'streaming',
				dialogue_count = CASE WHEN c.dialogue_count < 1 THEN 1 ELSE c.dialogue_count END,
				updated_at = CASE WHEN c.updated_at < t.updated_at THEN t.updated_at ELSE c.updated_at END
			FROM public.image_runtime_tasks AS t
			WHERE t.status IN ('pending', 'running')
				AND t.conversation_id ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
				AND t.message_id ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
				AND c.id = t.conversation_id::uuid
				AND c.deleted_at IS NULL
				AND EXISTS (
					SELECT 1
					FROM public.chat_runtime_messages AS m
					WHERE m.id = t.message_id::uuid
						AND m.conversation_id = c.id
						AND m.deleted_at IS NULL
				)
				AND (
					c.current_leaf_message_id IS NULL
					OR c.current_leaf_message_id = t.message_id::uuid
				);
		`).Error
	})
}
