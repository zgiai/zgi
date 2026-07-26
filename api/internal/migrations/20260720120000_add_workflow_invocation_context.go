package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddWorkflowInvocationContextID = "20260720120000_add_workflow_invocation_context"

func init() {
	registerSchemaMigration(
		migrationAddWorkflowInvocationContextID,
		upAddWorkflowInvocationContext,
		downAddWorkflowInvocationContext,
	)
}

func upAddWorkflowInvocationContext(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.workflow_run_logs
			ADD COLUMN IF NOT EXISTS invocation_protocol_version integer NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS invocation_mode varchar(64),
			ADD COLUMN IF NOT EXISTS parent_conversation_id varchar(255),
			ADD COLUMN IF NOT EXISTS parent_message_id varchar(255),
			ADD COLUMN IF NOT EXISTS parent_invocation_id varchar(128),
			ADD COLUMN IF NOT EXISTS invocation_binding_id varchar(255),
			ADD COLUMN IF NOT EXISTS invocation_context_digest varchar(128);

		CREATE INDEX IF NOT EXISTS idx_workflow_run_logs_parent_conversation
			ON public.workflow_run_logs(parent_conversation_id)
			WHERE parent_conversation_id IS NOT NULL;
		CREATE INDEX IF NOT EXISTS idx_workflow_run_logs_parent_message
			ON public.workflow_run_logs(parent_message_id)
			WHERE parent_message_id IS NOT NULL;
		CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_run_logs_parent_invocation_unique
			ON public.workflow_run_logs(parent_invocation_id)
			WHERE parent_invocation_id IS NOT NULL AND deleted_at IS NULL;
	`)
}

func downAddWorkflowInvocationContext(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP INDEX IF EXISTS public.idx_workflow_run_logs_parent_invocation_unique;
		DROP INDEX IF EXISTS public.idx_workflow_run_logs_parent_message;
		DROP INDEX IF EXISTS public.idx_workflow_run_logs_parent_conversation;
		ALTER TABLE public.workflow_run_logs
			DROP COLUMN IF EXISTS invocation_context_digest,
			DROP COLUMN IF EXISTS invocation_binding_id,
			DROP COLUMN IF EXISTS parent_invocation_id,
			DROP COLUMN IF EXISTS parent_message_id,
			DROP COLUMN IF EXISTS parent_conversation_id,
			DROP COLUMN IF EXISTS invocation_mode,
			DROP COLUMN IF EXISTS invocation_protocol_version;
	`)
}
