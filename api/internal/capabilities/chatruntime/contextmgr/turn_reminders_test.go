package contextmgr

import (
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// With reasoning kept across turns, guidance added after a tool call is
// recorded as the user turn the provider received, once (ADR 0043).
func TestTurnTranscriptRecordsRemindersAfterToolCalls(t *testing.T) {
	call := adapter.ToolCall{ID: "call-1", Type: "function", Function: adapter.FunctionCall{Name: "activate_skills"}}
	m := &Manager{}
	m.state.Model = "deepseek-v4-flash"
	m.state.TurnTranscript = []adapter.Message{{Role: "assistant", ToolCalls: []adapter.ToolCall{call}}}
	messages := []adapter.Message{
		{Role: "system", Content: "System prompt."},
		{Role: "user", Content: "Make a file."},
		{Role: "system", Content: "Initial guidance stays with the question."},
		{Role: "assistant", ToolCalls: []adapter.ToolCall{call}},
		{Role: "tool", ToolCallID: "call-1", Content: "activated"},
		{Role: "system", Content: "Active skill: file-generator"},
		{Role: "system", Content: "Read the reference."},
	}
	m.appendNewTurnToolMessages(messages)
	m.appendNewTurnToolMessages(messages)
	transcript := m.state.TurnTranscript
	if len(transcript) != 3 {
		t.Fatalf("transcript = %#v, want the call, its result and one reminder turn", transcript)
	}
	want := adapter.SystemReminder("Active skill: file-generator") + "\n\n" + adapter.SystemReminder("Read the reference.")
	if transcript[2].Role != "user" || transcript[2].Content != want {
		t.Fatalf("reminder turn = %#v, want %q", transcript[2], want)
	}
	placed := adapter.PlaceSystemMessages(messages)
	if last := placed[len(placed)-1]; last.Content != want {
		t.Fatalf("recorded reminder %q differs from the placed request %q", want, last.Content)
	}
}

// A model that does not replay reasoning gains nothing from them.
func TestTurnTranscriptKeepsNoRemindersForModelsWithoutReasoningReplay(t *testing.T) {
	call := adapter.ToolCall{ID: "call-1", Type: "function"}
	m := &Manager{}
	m.state.Model = "gpt-5"
	m.state.TurnTranscript = []adapter.Message{{Role: "assistant", ToolCalls: []adapter.ToolCall{call}}}
	m.appendNewTurnToolMessages([]adapter.Message{
		{Role: "assistant", ToolCalls: []adapter.ToolCall{call}},
		{Role: "tool", ToolCallID: "call-1", Content: "ok"},
		{Role: "system", Content: "Active skill: file-generator"},
	})
	if len(m.state.TurnTranscript) != 2 {
		t.Fatalf("transcript = %#v, want no reminder turn", m.state.TurnTranscript)
	}
}
