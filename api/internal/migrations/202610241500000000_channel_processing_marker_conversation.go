package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// A DingTalk processing reaction addresses the message by its conversation
// as well as its ID, so a marker keeps the provider conversation ID.
func init() {
	registerSchemaMigration("202610241500000000_channel_processing_marker_conversation", upChannelProcessingMarkerConversation, downChannelProcessingMarkerConversation)
}

func upChannelProcessingMarkerConversation(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.channel_processing_markers
			ADD COLUMN IF NOT EXISTS provider_conversation_id varchar(255) NOT NULL DEFAULT '';
	`)
}

func downChannelProcessingMarkerConversation(schema *mschema.Builder) error {
	return schema.Raw(`ALTER TABLE public.channel_processing_markers DROP COLUMN IF EXISTS provider_conversation_id;`)
}
