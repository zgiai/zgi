package skillloop

import (
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// Guidance the history already holds is not sent again, and the question is
// reported in the form the provider receives it (ADR 0043).
func TestTurnRemindersAreSentOnceAndReportedAsSent(t *testing.T) {
	instructions := "Active skill: file-generator\nInstructions:\nWrite files."
	history := []adapter.Message{
		{Role: "system", Content: "System prompt."},
		{Role: "user", Content: "Make a file.\n\n" + adapter.SystemReminder(instructions)},
		{Role: "assistant", Content: "Done."},
		{Role: "user", Content: "Another one."},
	}
	tail := withoutRemindersInHistory(history, []adapter.Message{
		{Role: "system", Content: instructions},
		{Role: "system", Content: "Workflow bindings."},
	})
	if len(tail) != 1 || tail[0].Content != "Workflow bindings." {
		t.Fatalf("tail = %#v, want only the guidance the history lacks", tail)
	}

	content, ok := placedTurnUserContent(append(history, tail...))
	if !ok || content != "Another one.\n\n"+adapter.SystemReminder("Workflow bindings.") {
		t.Fatalf("placed question = %#v, %v", content, ok)
	}
	if _, ok := placedTurnUserContent(append(history, adapter.Message{Role: "assistant", Content: "x"})); ok {
		t.Fatal("a question followed by an assistant message was reported")
	}
}
