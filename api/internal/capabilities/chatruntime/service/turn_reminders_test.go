package service

import (
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// The date, memory and turn context sections already in the replayed history
// are not repeated; the turn boundary is sent every turn (ADR 0043).
func TestCurrentTurnRemindersSkipWhatTheHistoryHolds(t *testing.T) {
	parts := &chatRequestParts{
		CurrentTimeContext:  "Today is 2026-10-30.",
		TurnContextSections: []string{"Connector notes.", "Channel: group chat."},
		AgentMemoryContext:  "<memory used=\"5/10000\">\nName: Lin\n</memory>",
	}
	boundary := &adapter.Message{Role: "system", Content: "Turn boundary."}
	fresh := currentTurnReminders(parts, nil, nil, boundary)
	if strings.Join(fresh, "|") != "Today is 2026-10-30.|Connector notes.|Channel: group chat.|"+parts.AgentMemoryContext+"|Turn boundary." {
		t.Fatalf("reminders without history = %#v", fresh)
	}

	history := []adapter.Message{
		{Role: "user", Content: adapter.PrependSystemReminders([]string{"Today is 2026-10-29.", "Connector notes.", parts.AgentMemoryContext, "Turn boundary."}, "Hi")},
		{Role: "assistant", Content: "Hello."},
	}
	repeated := currentTurnReminders(parts, history, nil, boundary)
	if strings.Join(repeated, "|") != "Today is 2026-10-30.|Channel: group chat.|Turn boundary." {
		t.Fatalf("reminders with history = %#v, want the unchanged memory and notes left out", repeated)
	}

	sameDay := []adapter.Message{{Role: "user", Content: adapter.PrependSystemReminders([]string{"Today is 2026-10-30."}, "Hi")}}
	if reminders := currentTurnReminders(parts, sameDay, nil, boundary); strings.Contains(strings.Join(reminders, "|"), "Today is") {
		t.Fatalf("reminders on the same day = %#v, want the date left out", reminders)
	}

	parts.AgentMemoryContext = "<memory used=\"9/10000\">\nName: Lin\nCity: Hangzhou\n</memory>"
	changed := currentTurnReminders(parts, history, nil, boundary)
	if !strings.Contains(strings.Join(changed, "|"), "City: Hangzhou") {
		t.Fatalf("reminders after a memory change = %#v, want the new memory", changed)
	}
}
