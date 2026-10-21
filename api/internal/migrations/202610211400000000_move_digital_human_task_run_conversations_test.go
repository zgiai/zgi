package migrations

import (
	"strings"
	"testing"
)

func TestMoveDigitalHumanTaskRunConversationsMigration(t *testing.T) {
	compact := strings.ToLower(strings.Join(strings.Fields(moveDigitalHumanTaskRunConversationsSQL), " "))
	for _, required := range []string{
		"update public.chat_runtime_conversations",
		"set source = 'task'",
		"conversations.caller_type = 'digital_human'",
		"conversations.source = 'console'",
		"runs.surface = 'digital_human_task'",
		"runs.surface <> 'digital_human_task'",
	} {
		if !strings.Contains(compact, required) {
			t.Errorf("migration is missing %q", required)
		}
	}
}
