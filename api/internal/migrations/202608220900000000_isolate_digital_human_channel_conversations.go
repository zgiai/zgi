package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

const migration202608220900000000ID = "202608220900000000_isolate_digital_human_channel_conversations"

const isolateDigitalHumanChannelConversationsSQL = `
	UPDATE public.chat_runtime_conversations AS conversations
	SET source = 'external-api', updated_at = CURRENT_TIMESTAMP
	WHERE conversations.id IN (
		SELECT mappings.local_conversation_id
		FROM public.external_conversation_mappings AS mappings
		WHERE mappings.local_conversation_id IS NOT NULL
	)
	AND conversations.caller_type = 'digital_human'
	AND conversations.source <> 'external-api';
`

func init() {
	registerSchemaMigration(migration202608220900000000ID, up202608220900000000, down202608220900000000)
}

func up202608220900000000(schema *mschema.Builder) error {
	return schema.DataFix("isolate DigitalHuman channel conversations from console history", func(db *gorm.DB) error {
		return db.Exec(isolateDigitalHumanChannelConversationsSQL).Error
	})
}

func down202608220900000000(schema *mschema.Builder) error {
	return schema.Raw(`
		UPDATE public.chat_runtime_conversations AS conversations
		SET source = 'console', updated_at = CURRENT_TIMESTAMP
		WHERE conversations.id IN (
			SELECT mappings.local_conversation_id
			FROM public.external_conversation_mappings AS mappings
			WHERE mappings.local_conversation_id IS NOT NULL
		)
		AND conversations.caller_type = 'digital_human'
		AND conversations.source = 'external-api';
	`)
}
