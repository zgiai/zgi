package service

import (
	"testing"
)

// The run's changing guidance follows the history, so the system prompt and
// the history stay a stable prefix (ADR 0032).
func TestTurnContextFollowsHistory(t *testing.T) {
	parts := &chatRequestParts{TurnContext: "  Current time: 2026-10-28 15:00  "}
	message := turnContextMessage(parts)
	if message == nil || message.Role != "system" || message.Content != "Current time: 2026-10-28 15:00" {
		t.Fatalf("turnContextMessage() = %#v", message)
	}
	if turnContextMessage(&chatRequestParts{TurnContext: " "}) != nil || turnContextMessage(nil) != nil {
		t.Fatal("an empty turn context produced a message")
	}
	config := RunConfig{SystemPrompt: "Persona", TurnContext: "Current time"}
	applied := &chatRequestParts{}
	applyRunConfigToParts(config, applied)
	if applied.SystemPrompt != "Persona" || applied.TurnContext != "Current time" {
		t.Fatalf("applyRunConfigToParts() = %q / %q", applied.SystemPrompt, applied.TurnContext)
	}
}
