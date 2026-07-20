package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddWorkflowConversationRuntimeID = "20260719100000_add_workflow_conversation_runtime"

func init() {
	registerSchemaMigration(
		migrationAddWorkflowConversationRuntimeID,
		upAddWorkflowConversationRuntime,
		downAddWorkflowConversationRuntime,
	)
}

func upAddWorkflowConversationRuntime(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.agents_conversations
			ADD COLUMN IF NOT EXISTS runtime_status varchar(32) NOT NULL DEFAULT 'idle',
			ADD COLUMN IF NOT EXISTS active_workflow_run_id uuid,
			ADD COLUMN IF NOT EXISTS runtime_generation bigint NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS runtime_revision bigint NOT NULL DEFAULT 0;

		ALTER TABLE public.workflow_run_logs
			ADD COLUMN IF NOT EXISTS conversation_id uuid;

		CREATE INDEX IF NOT EXISTS idx_agents_conversations_active_workflow_run
			ON public.agents_conversations(active_workflow_run_id)
			WHERE active_workflow_run_id IS NOT NULL;
		CREATE INDEX IF NOT EXISTS idx_workflow_run_logs_conversation
			ON public.workflow_run_logs(conversation_id)
			WHERE conversation_id IS NOT NULL;

		-- This is a second line of defence behind the conversation row lock.  Old
		-- runs are not backfilled, so deploying the migration cannot manufacture a
		-- conflict from historical data.
		CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_run_logs_active_conversation_unique
			ON public.workflow_run_logs(conversation_id)
			WHERE conversation_id IS NOT NULL
			  AND deleted_at IS NULL
			  AND status IN ('running', 'paused');
	`)
}

func downAddWorkflowConversationRuntime(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP INDEX IF EXISTS public.idx_workflow_run_logs_active_conversation_unique;
		DROP INDEX IF EXISTS public.idx_workflow_run_logs_conversation;
		DROP INDEX IF EXISTS public.idx_agents_conversations_active_workflow_run;
		ALTER TABLE public.workflow_run_logs DROP COLUMN IF EXISTS conversation_id;
		ALTER TABLE public.agents_conversations
			DROP COLUMN IF EXISTS runtime_revision,
			DROP COLUMN IF EXISTS runtime_generation,
			DROP COLUMN IF EXISTS active_workflow_run_id,
			DROP COLUMN IF EXISTS runtime_status;
	`)
}
