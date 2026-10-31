package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationChatRuntimeMessageRecordsID = "202610291400000000_chat_runtime_message_records"

func init() {
	registerSchemaMigration(migrationChatRuntimeMessageRecordsID, upChatRuntimeMessageRecords, nil)
}

// Message records hold the lists a turn appends to while it runs, one row per
// entry, instead of message metadata that was rewritten whole on every entry
// (ADR 0039). Existing messages keep their lists in metadata and stay readable.
func upChatRuntimeMessageRecords(schema *mschema.Builder) error {
	return schema.Raw(`
		CREATE TABLE IF NOT EXISTS public.chat_runtime_message_records (
			id uuid PRIMARY KEY,
			message_id uuid NOT NULL,
			organization_id uuid NOT NULL,
			kind varchar(64) NOT NULL,
			record_id varchar(255) NOT NULL,
			sequence integer NOT NULL DEFAULT 0,
			payload jsonb NOT NULL,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT chat_runtime_message_records_message_fkey
				FOREIGN KEY (message_id) REFERENCES public.chat_runtime_messages(id) ON DELETE CASCADE
		);

		CREATE UNIQUE INDEX IF NOT EXISTS uq_chat_runtime_message_records_entry
			ON public.chat_runtime_message_records (message_id, kind, record_id);
		CREATE INDEX IF NOT EXISTS idx_chat_runtime_message_records_order
			ON public.chat_runtime_message_records (message_id, kind, sequence);

		CREATE TABLE IF NOT EXISTS public.chat_runtime_system_prompts (
			hash varchar(64) PRIMARY KEY,
			content text NOT NULL,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`)
}
