package service

import (
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// Memory and turn context sections already in the replayed history are not
// repeated; the time and the turn boundary are sent every turn (ADR 0043).
func TestCurrentTurnRemindersSkipWhatTheHistoryHolds(t *testing.T) {
	parts := &chatRequestParts{
		CurrentTimeContext:  "Current time: 10:00.",
		TurnContextSections: []string{"Connector notes.", "Channel: group chat."},
		AgentMemoryContext:  "<memory used=\"5/10000\">\nName: Lin\n</memory>",
	}
	boundary := &adapter.Message{Role: "system", Content: "Turn boundary."}
	fresh := currentTurnReminders(parts, nil, nil, boundary)
	if strings.Join(fresh, "|") != "Current time: 10:00.|Connector notes.|Channel: group chat.|"+parts.AgentMemoryContext+"|Turn boundary." {
		t.Fatalf("reminders without history = %#v", fresh)
	}

	history := []adapter.Message{
		{Role: "user", Content: adapter.PrependSystemReminders([]string{"Current time: 09:00.", "Connector notes.", parts.AgentMemoryContext, "Turn boundary."}, "Hi")},
		{Role: "assistant", Content: "Hello."},
	}
	repeated := currentTurnReminders(parts, history, nil, boundary)
	if strings.Join(repeated, "|") != "Current time: 10:00.|Channel: group chat.|Turn boundary." {
		t.Fatalf("reminders with history = %#v, want the unchanged memory and notes left out", repeated)
	}

	parts.AgentMemoryContext = "<memory used=\"9/10000\">\nName: Lin\nCity: Hangzhou\n</memory>"
	changed := currentTurnReminders(parts, history, nil, boundary)
	if !strings.Contains(strings.Join(changed, "|"), "City: Hangzhou") {
		t.Fatalf("reminders after a memory change = %#v, want the new memory", changed)
	}
}
