package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration202608231000000000ID = "202608231000000000_add_channel_event_preview"

const addChannelEventPreviewSQL = `
	ALTER TABLE public.channel_inbound_events
		ADD COLUMN IF NOT EXISTS conversation_type varchar(16) NOT NULL DEFAULT '',
		ADD COLUMN IF NOT EXISTS content_preview varchar(512) NOT NULL DEFAULT '',
		ADD COLUMN IF NOT EXISTS mention_only boolean NOT NULL DEFAULT false;

	ALTER TABLE public.channel_inbound_events
		DROP CONSTRAINT IF EXISTS channel_inbound_events_conversation_type_check;
	ALTER TABLE public.channel_inbound_events
		ADD CONSTRAINT channel_inbound_events_conversation_type_check CHECK (
			conversation_type IN ('', 'direct', 'group')
		);
`

func init() {
	registerSchemaMigration(migration202608231000000000ID, up202608231000000000, down202608231000000000)
}

func up202608231000000000(schema *mschema.Builder) error {
	return schema.Raw(addChannelEventPreviewSQL)
}

func down202608231000000000(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.channel_inbound_events
			DROP CONSTRAINT IF EXISTS channel_inbound_events_conversation_type_check,
			DROP COLUMN IF EXISTS mention_only,
			DROP COLUMN IF EXISTS content_preview,
			DROP COLUMN IF EXISTS conversation_type;
	`)
}
