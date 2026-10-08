package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

func init() {
	registerSchemaMigration("202610082000000000_chat_file_upload_policy", upChatFileUploadPolicy, nil)
}

func upChatFileUploadPolicy(schema *mschema.Builder) error {
	return schema.Raw(`ALTER TABLE public.digital_human_versions ADD COLUMN IF NOT EXISTS file_upload_policy jsonb;`)
}
