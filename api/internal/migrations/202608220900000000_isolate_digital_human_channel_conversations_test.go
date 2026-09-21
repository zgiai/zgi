package migrations

import (
	"strings"
	"testing"
)

func TestIsolateDigitalHumanChannelConversationsMigration(t *testing.T) {
	compact := strings.ToLower(strings.Join(strings.Fields(isolateDigitalHumanChannelConversationsSQL), " "))
	for _, required := range []string{
		"update public.chat_runtime_conversations",
		"set source = 'external-api'",
		"from public.external_conversation_mappings",
		"where mappings.local_conversation_id is not null",
		"conversations.caller_type = 'digital_human'",
	} {
		if !strings.Contains(compact, required) {
			t.Errorf("migration is missing %q", required)
		}
	}
}
