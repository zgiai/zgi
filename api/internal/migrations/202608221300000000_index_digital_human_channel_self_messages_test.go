package migrations

import (
	"strings"
	"testing"
)

func TestIndexDigitalHumanChannelSelfMessagesMigration(t *testing.T) {
	compact := strings.ToLower(strings.Join(strings.Fields(indexDigitalHumanChannelSelfMessagesSQL), " "))
	for _, required := range []string{
		"create index if not exists idx_channel_delivery_outbox_self_messages",
		"installation_id, external_conversation_id, subject_id, provider_message_id",
		"where status = 'succeeded' and provider_message_id <> ''",
	} {
		if !strings.Contains(compact, required) {
			t.Errorf("migration is missing %q", required)
		}
	}
}
