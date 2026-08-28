package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration202608270900000000ID = "202608270900000000_add_chat_runtime_inputs"

func init() {
	registerSchemaMigration(migration202608270900000000ID, up202608270900000000, down202608270900000000)
}

func up202608270900000000(schema *mschema.Builder) error {
	return schema.Raw(`
		CREATE TABLE IF NOT EXISTS public.chat_runtime_inputs (
			id uuid PRIMARY KEY,
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
			workspace_id uuid REFERENCES public.workspaces(id) ON DELETE CASCADE,
			account_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE CASCADE,
			conversation_id uuid NOT NULL REFERENCES public.chat_runtime_conversations(id) ON DELETE CASCADE,
			target_message_id uuid NOT NULL REFERENCES public.chat_runtime_messages(id) ON DELETE CASCADE,
			consumed_message_id uuid REFERENCES public.chat_runtime_messages(id) ON DELETE SET NULL,
			client_input_id uuid NOT NULL,
			sequence bigint NOT NULL,
			content text NOT NULL,
			status varchar(32) NOT NULL DEFAULT 'pending',
			consumption_mode varchar(32),
			claim_batch_id uuid,
			claimed_at timestamptz,
			claim_expires_at timestamptz,
			consumed_at timestamptz,
			canceled_at timestamptz,
			error text,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT chat_runtime_inputs_status_check CHECK (status IN ('pending', 'claimed', 'consumed', 'canceled', 'failed')),
			CONSTRAINT chat_runtime_inputs_consumption_mode_check CHECK (consumption_mode IS NULL OR consumption_mode IN ('current_run', 'follow_up')),
			CONSTRAINT chat_runtime_inputs_sequence_check CHECK (sequence > 0),
			CONSTRAINT chat_runtime_inputs_content_check CHECK (length(btrim(content)) > 0)
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_runtime_inputs_conversation_client
			ON public.chat_runtime_inputs (conversation_id, client_input_id);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_runtime_inputs_conversation_sequence
			ON public.chat_runtime_inputs (conversation_id, sequence);
		CREATE INDEX IF NOT EXISTS idx_chat_runtime_inputs_target_status_sequence
			ON public.chat_runtime_inputs (target_message_id, status, sequence);
		CREATE INDEX IF NOT EXISTS idx_chat_runtime_inputs_claim_batch
			ON public.chat_runtime_inputs (claim_batch_id) WHERE claim_batch_id IS NOT NULL;
		CREATE INDEX IF NOT EXISTS idx_chat_runtime_inputs_stale_claim
			ON public.chat_runtime_inputs (status, claim_expires_at) WHERE status = 'claimed';
		CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_runtime_messages_source_message
			ON public.chat_runtime_messages (source_message_id)
			WHERE source_message_id IS NOT NULL AND deleted_at IS NULL;
	`)
}

func down202608270900000000(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP INDEX IF EXISTS public.idx_chat_runtime_messages_source_message;
		DROP TABLE IF EXISTS public.chat_runtime_inputs;
	`)
}
