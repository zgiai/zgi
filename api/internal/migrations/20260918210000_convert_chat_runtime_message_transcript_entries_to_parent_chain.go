package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationConvertChatRuntimeMessageTranscriptEntriesToParentChainID = "20260918210000_convert_chat_runtime_message_transcript_entries_to_parent_chain"

func init() {
	registerSchemaMigration(migrationConvertChatRuntimeMessageTranscriptEntriesToParentChainID, upConvertChatRuntimeMessageTranscriptEntriesToParentChain, nil)
}

// This migration is intentionally incremental. The original transcript-entry
// migration may already have been applied with the old cseq/seq schema, and
// editing an applied migration does not cause the migration runner to execute
// it again. The old table is retained under a legacy name and a fresh table is
// created at the canonical name; existing local test rows are not converted.
func upConvertChatRuntimeMessageTranscriptEntriesToParentChain(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.chat_runtime_messages
			ADD COLUMN IF NOT EXISTS transcript_head_entry_id uuid;

		DO $$
		BEGIN
			IF EXISTS (
				SELECT 1
				FROM information_schema.columns
				WHERE table_schema = 'public'
				  AND table_name = 'chat_runtime_message_transcript_entries'
				  AND column_name = 'cseq'
			) THEN
				ALTER INDEX IF EXISTS public.chat_runtime_message_transcript_entries_seq_key
					RENAME TO chat_runtime_message_transcript_entries_seq_key_legacy;
				ALTER INDEX IF EXISTS public.chat_runtime_message_transcript_entries_cseq_key
					RENAME TO chat_runtime_message_transcript_entries_cseq_key_legacy;
				ALTER INDEX IF EXISTS public.idx_chat_runtime_transcript_entries_message_cseq
					RENAME TO idx_chat_runtime_transcript_entries_message_cseq_legacy;
				ALTER INDEX IF EXISTS public.idx_chat_runtime_transcript_entries_org_conversation
					RENAME TO idx_chat_runtime_transcript_entries_org_conversation_legacy;
				ALTER TABLE public.chat_runtime_message_transcript_entries
					RENAME TO chat_runtime_message_transcript_entries_legacy;
			END IF;
		END $$;

		CREATE TABLE IF NOT EXISTS public.chat_runtime_message_transcript_entries (
			id uuid PRIMARY KEY,
			message_id uuid NOT NULL,
			conversation_id uuid NOT NULL,
			organization_id uuid NOT NULL,
			parent_entry_id uuid,
			covers_through_entry_id uuid,
			event_id varchar(255) NOT NULL DEFAULT '',
			kind varchar(32) NOT NULL,
			preview jsonb NOT NULL,
			payload jsonb NOT NULL,
			byte_size integer NOT NULL DEFAULT 0,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT chat_runtime_message_transcript_entries_byte_size_check CHECK (byte_size >= 0),
			CONSTRAINT chat_runtime_message_transcript_entries_message_fkey
				FOREIGN KEY (message_id) REFERENCES public.chat_runtime_messages(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_chat_runtime_transcript_entries_org_conversation
			ON public.chat_runtime_message_transcript_entries (organization_id, conversation_id);
		CREATE INDEX IF NOT EXISTS idx_chat_runtime_transcript_entries_parent
			ON public.chat_runtime_message_transcript_entries (parent_entry_id);
		CREATE INDEX IF NOT EXISTS idx_chat_runtime_messages_transcript_head
			ON public.chat_runtime_messages (transcript_head_entry_id)
			WHERE transcript_head_entry_id IS NOT NULL;
		CREATE UNIQUE INDEX IF NOT EXISTS uq_chat_runtime_transcript_entries_message_event
			ON public.chat_runtime_message_transcript_entries (message_id, event_id)
			WHERE event_id <> '';
	`)
}
