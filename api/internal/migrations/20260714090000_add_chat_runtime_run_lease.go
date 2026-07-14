package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddChatRuntimeRunLeaseID = "20260714090000_add_chat_runtime_run_lease"

func init() {
	registerSchemaMigration(
		migrationAddChatRuntimeRunLeaseID,
		upAddChatRuntimeRunLease,
		downAddChatRuntimeRunLease,
	)
}

func upAddChatRuntimeRunLease(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.chat_runtime_messages
			ADD COLUMN IF NOT EXISTS runtime_run_id uuid,
			ADD COLUMN IF NOT EXISTS runtime_heartbeat_at timestamptz;
		CREATE INDEX IF NOT EXISTS idx_chat_runtime_messages_active_lease
			ON public.chat_runtime_messages (runtime_heartbeat_at)
			WHERE deleted_at IS NULL
				AND status IN ('pending', 'streaming')
	`)
}

func downAddChatRuntimeRunLease(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP INDEX IF EXISTS public.idx_chat_runtime_messages_active_lease;
		ALTER TABLE public.chat_runtime_messages
			DROP COLUMN IF EXISTS runtime_heartbeat_at,
			DROP COLUMN IF EXISTS runtime_run_id
	`)
}
