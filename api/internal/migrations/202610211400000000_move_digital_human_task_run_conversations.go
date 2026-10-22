package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

const migration202610211400000000ID = "202610211400000000_move_digital_human_task_run_conversations"

// Task runs used to persist their conversation as console history, so every
// scheduled or manual run surfaced in the DigitalHuman chat list. A
// conversation used only by task runs is the task's work record.
const moveDigitalHumanTaskRunConversationsSQL = `
	UPDATE public.chat_runtime_conversations AS conversations
	SET source = 'task', updated_at = CURRENT_TIMESTAMP
	WHERE conversations.caller_type = 'digital_human'
	AND conversations.source = 'console'
	AND EXISTS (
		SELECT 1 FROM public.digital_human_runs AS runs
		WHERE runs.conversation_id = conversations.id AND runs.surface = 'digital_human_task'
	)
	AND NOT EXISTS (
		SELECT 1 FROM public.digital_human_runs AS runs
		WHERE runs.conversation_id = conversations.id AND runs.surface <> 'digital_human_task'
	);
`

func init() {
	registerSchemaMigration(migration202610211400000000ID, up202610211400000000, down202610211400000000)
}

func up202610211400000000(schema *mschema.Builder) error {
	return schema.DataFix("move DigitalHuman task run conversations out of console history", func(db *gorm.DB) error {
		return db.Exec(moveDigitalHumanTaskRunConversationsSQL).Error
	})
}

func down202610211400000000(schema *mschema.Builder) error {
	return schema.Raw(`
		UPDATE public.chat_runtime_conversations AS conversations
		SET source = 'console', updated_at = CURRENT_TIMESTAMP
		WHERE conversations.caller_type = 'digital_human'
		AND conversations.source = 'task'
		AND EXISTS (
			SELECT 1 FROM public.digital_human_runs AS runs
			WHERE runs.conversation_id = conversations.id AND runs.surface = 'digital_human_task'
			AND COALESCE(runs.invocation_event -> 'context' ->> 'linux_execution_entry', '') <> 'task'
		);
	`)
}
