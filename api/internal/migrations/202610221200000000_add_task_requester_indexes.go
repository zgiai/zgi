package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// The task center filters a DigitalHuman's tasks by who asked for them: the
// requesting subject, or the manager who created a task in the task center.
func init() {
	registerSchemaMigration("202610221200000000_add_task_requester_indexes", upAddTaskRequesterIndexes, downAddTaskRequesterIndexes)
}

func upAddTaskRequesterIndexes(schema *mschema.Builder) error {
	return schema.Raw(`
		CREATE INDEX IF NOT EXISTS idx_automation_tasks_owner_subject
			ON public.automation_tasks (subject_id, owner_subject_id, updated_at DESC) WHERE owner_subject_id IS NOT NULL;
		CREATE INDEX IF NOT EXISTS idx_automation_tasks_center_creator
			ON public.automation_tasks (subject_id, created_by, updated_at DESC) WHERE owner_kind = 'digital_human';
	`)
}

func downAddTaskRequesterIndexes(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP INDEX IF EXISTS public.idx_automation_tasks_center_creator;
		DROP INDEX IF EXISTS public.idx_automation_tasks_owner_subject;
	`)
}
