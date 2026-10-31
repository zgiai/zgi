package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

const migrationChatRuntimeStorageCleanupID = "202610291500000000_chat_runtime_storage_cleanup"

func init() {
	registerSchemaMigration(migrationChatRuntimeStorageCleanupID, upChatRuntimeStorageCleanup, nil)
}

// upChatRuntimeStorageCleanup removes duplicated chat runtime storage
// (ADR 0040):
//   - a transcript preview equal to its payload is stored as NULL;
//   - indexes kept from the aichat_* tables that repeat a chat_runtime_* one
//     are dropped;
//   - messages reference their conversation; existing rows are validated
//     when none is an orphan.
func upChatRuntimeStorageCleanup(schema *mschema.Builder) error {
	if err := schema.Raw(`ALTER TABLE public.chat_runtime_message_transcript_entries ALTER COLUMN preview DROP NOT NULL`); err != nil {
		return err
	}
	if err := schema.DataFix("store transcript previews equal to their payload as NULL", func(db *gorm.DB) error {
		return db.Exec(`UPDATE public.chat_runtime_message_transcript_entries SET preview = NULL WHERE preview = payload`).Error
	}); err != nil {
		return err
	}
	return schema.Raw(`
		DROP INDEX IF EXISTS public.idx_aichat_messages_conversation_created;
		DROP INDEX IF EXISTS public.idx_aichat_messages_source_message;
		DROP INDEX IF EXISTS public.idx_aichat_conversations_owner_updated;
		DROP INDEX IF EXISTS public.idx_aichat_custom_skills_org_skill_active;

		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint WHERE conname = 'chat_runtime_messages_conversation_fkey'
			) THEN
				ALTER TABLE public.chat_runtime_messages
					ADD CONSTRAINT chat_runtime_messages_conversation_fkey
					FOREIGN KEY (conversation_id) REFERENCES public.chat_runtime_conversations(id)
					ON DELETE CASCADE NOT VALID;
			END IF;
			-- Existing rows are checked only when none lacks its conversation, so
			-- an orphan does not stop the upgrade; new rows are checked anyway.
			IF NOT EXISTS (
				SELECT 1 FROM public.chat_runtime_messages m
				WHERE NOT EXISTS (SELECT 1 FROM public.chat_runtime_conversations c WHERE c.id = m.conversation_id)
			) THEN
				ALTER TABLE public.chat_runtime_messages VALIDATE CONSTRAINT chat_runtime_messages_conversation_fkey;
			END IF;
		END $$;
	`)
}
