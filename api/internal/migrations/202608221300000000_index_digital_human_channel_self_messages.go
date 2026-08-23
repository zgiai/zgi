package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration202608221300000000ID = "202608221300000000_index_digital_human_channel_self_messages"

const indexDigitalHumanChannelSelfMessagesSQL = `
	CREATE INDEX IF NOT EXISTS idx_channel_delivery_outbox_self_messages
		ON public.channel_delivery_outbox (
			installation_id,
			external_conversation_id,
			subject_id,
			provider_message_id
		)
		WHERE status = 'succeeded' AND provider_message_id <> '';
`

func init() {
	registerSchemaMigration(migration202608221300000000ID, up202608221300000000, down202608221300000000)
}

func up202608221300000000(schema *mschema.Builder) error {
	return schema.Raw(indexDigitalHumanChannelSelfMessagesSQL)
}

func down202608221300000000(schema *mschema.Builder) error {
	return schema.Raw(`DROP INDEX IF EXISTS public.idx_channel_delivery_outbox_self_messages;`)
}
