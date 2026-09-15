package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddChatRuntimeExternalUserID = "20260903120000_add_chat_runtime_external_user"

func init() {
	registerSchemaMigration(
		migrationAddChatRuntimeExternalUserID,
		upAddChatRuntimeExternalUser,
		downAddChatRuntimeExternalUser,
	)
}

func upAddChatRuntimeExternalUser(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.chat_runtime_conversations
			ADD COLUMN IF NOT EXISTS external_user_id uuid;

		CREATE INDEX IF NOT EXISTS idx_chat_runtime_conversations_external_user_updated
			ON public.chat_runtime_conversations (
				organization_id,
				account_id,
				caller_type,
				caller_id,
				conversation_type,
				external_user_id,
				updated_at DESC
			)
			WHERE source = 'external-api'
			  AND external_user_id IS NOT NULL
			  AND deleted_at IS NULL;
	`)
}

func downAddChatRuntimeExternalUser(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP INDEX IF EXISTS public.idx_chat_runtime_conversations_external_user_updated;
		ALTER TABLE public.chat_runtime_conversations
			DROP COLUMN IF EXISTS external_user_id;
	`)
}
