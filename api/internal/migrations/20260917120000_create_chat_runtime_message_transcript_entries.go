package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationCreateChatRuntimeMessageTranscriptEntriesID = "20260917120000_create_chat_runtime_message_transcript_entries"

func init() {
	registerSchemaMigration(migrationCreateChatRuntimeMessageTranscriptEntriesID, upCreateChatRuntimeMessageTranscriptEntries, nil)
}

// The transcript entry table records the append-only protocol log. Existing
// main-branch messages remain readable through the legacy metadata fallback;
// new and resumed turns append to this table lazily.
func upCreateChatRuntimeMessageTranscriptEntries(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.chat_runtime_messages
			ADD COLUMN IF NOT EXISTS transcript_head_entry_id uuid;

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
