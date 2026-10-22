package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// A task's client reference is an integrator's own identifier, given when an
// API end user's task is created and echoed back in reports, notifications,
// and webhooks, so the integrator can route results without ZGI keeping the
// end user's identifier.
func init() {
	registerSchemaMigration("202610221000000000_add_task_client_reference", upAddTaskClientReference, downAddTaskClientReference)
}

func upAddTaskClientReference(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.automation_tasks ADD COLUMN IF NOT EXISTS client_reference varchar(255);
		CREATE INDEX IF NOT EXISTS idx_automation_tasks_client_reference
			ON public.automation_tasks (subject_id, client_reference) WHERE client_reference IS NOT NULL;
	`)
}

func downAddTaskClientReference(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP INDEX IF EXISTS public.idx_automation_tasks_client_reference;
		ALTER TABLE public.automation_tasks DROP COLUMN IF EXISTS client_reference;
	`)
}
