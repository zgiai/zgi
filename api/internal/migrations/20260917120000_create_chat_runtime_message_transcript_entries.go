package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationCreateChatRuntimeMessageTranscriptEntriesID = "20260917120000_create_chat_runtime_message_transcript_entries"

func init() {
	registerSchemaMigration(migrationCreateChatRuntimeMessageTranscriptEntriesID, upCreateChatRuntimeMessageTranscriptEntries, nil)
}

// The transcript entry table records one protocol message per row (the raw
// conversational facts of a turn). It is append-only: payload never changes,
// preview is a derived effective form that can be recomputed from payload.
// Context summaries live on the message row as (summary, summary_through_seq),
// where the boundary is the cseq of the last entry covered by the summary.
func upCreateChatRuntimeMessageTranscriptEntries(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.chat_runtime_messages
			ADD COLUMN IF NOT EXISTS summary text,
			ADD COLUMN IF NOT EXISTS summary_through_seq bigint,
			ADD COLUMN IF NOT EXISTS transcript_format_version smallint NOT NULL DEFAULT 1;

		CREATE TABLE IF NOT EXISTS public.chat_runtime_message_transcript_entries (
			id uuid PRIMARY KEY,
			message_id uuid NOT NULL,
			conversation_id uuid NOT NULL,
			organization_id uuid NOT NULL,
			cseq bigint NOT NULL,
			seq integer NOT NULL,
			kind varchar(32) NOT NULL,
			preview jsonb NOT NULL,
			payload jsonb NOT NULL,
			byte_size integer NOT NULL DEFAULT 0,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT chat_runtime_message_transcript_entries_seq_key UNIQUE (message_id, seq),
			CONSTRAINT chat_runtime_message_transcript_entries_cseq_key UNIQUE (conversation_id, cseq),
			CONSTRAINT chat_runtime_message_transcript_entries_byte_size_check CHECK (byte_size >= 0),
			CONSTRAINT chat_runtime_message_transcript_entries_message_fkey
				FOREIGN KEY (message_id) REFERENCES public.chat_runtime_messages(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_chat_runtime_transcript_entries_message_cseq
			ON public.chat_runtime_message_transcript_entries (conversation_id, message_id, cseq);

		CREATE INDEX IF NOT EXISTS idx_chat_runtime_transcript_entries_org_conversation
			ON public.chat_runtime_message_transcript_entries (organization_id, conversation_id);
	`)
}
