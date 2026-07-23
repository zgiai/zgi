package adapter

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestChatRequestJSONDoesNotExposeProviderHint(t *testing.T) {
	request := ChatRequest{
		Provider: "deepseek",
		Model:    "deepseek-chat",
		Messages: []Message{
			{Role: "user", Content: "hello"},
		},
	}

	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	body := string(payload)
	if strings.Contains(body, `"provider"`) {
		t.Fatalf("json payload = %s, want provider hint omitted", body)
	}
	if !strings.Contains(body, `"model":"deepseek-chat"`) {
		t.Fatalf("json payload = %s, want model preserved", body)
	}
	if !strings.Contains(body, `"messages"`) {
		t.Fatalf("json payload = %s, want messages preserved", body)
	}
}

func TestNormalizeSystemMessagesMovesAndMergesTextBeforeConversation(t *testing.T) {
	messages := []Message{
		{Role: "system", Content: "base instructions"},
		{Role: "user", Content: "question"},
		{Role: "assistant", Content: "working", ToolCalls: []ToolCall{{ID: "call-1"}}},
		{Role: "tool", ToolCallID: "call-1", Content: "result"},
		{Role: "SYSTEM", Content: []MessageContentPart{{Type: "text", Text: "latest runtime guidance"}}},
	}
	original := append([]Message(nil), messages...)

	normalized := NormalizeSystemMessages(messages)

	if len(normalized) != 4 {
		t.Fatalf("len(normalized) = %d, want 4: %#v", len(normalized), normalized)
	}
	if normalized[0].Role != "system" {
		t.Fatalf("normalized[0].Role = %q, want system", normalized[0].Role)
	}
	if got, want := normalized[0].Content, "base instructions\n\nlatest runtime guidance"; got != want {
		t.Fatalf("normalized system content = %#v, want %#v", got, want)
	}
	for index, wantRole := range []string{"user", "assistant", "tool"} {
		if got := normalized[index+1].Role; got != wantRole {
			t.Fatalf("normalized[%d].Role = %q, want %q", index+1, got, wantRole)
		}
	}
	if !reflect.DeepEqual(messages, original) {
		t.Fatalf("NormalizeSystemMessages() mutated input:\nactual: %#v\nwant: %#v", messages, original)
	}
}

func TestNormalizeSystemMessagesKeepsStructuredSystemContentLosslessly(t *testing.T) {
	structured := map[string]interface{}{"type": "provider_specific", "value": "keep me"}
	messages := []Message{
		{Role: "user", Content: "question"},
		{Role: "system", Content: "text instructions"},
		{Role: "SYSTEM", Content: structured},
		{Role: "assistant", Content: "answer"},
	}

	normalized := NormalizeSystemMessages(messages)

	if len(normalized) != len(messages) {
		t.Fatalf("len(normalized) = %d, want %d", len(normalized), len(messages))
	}
	if normalized[0].Role != "system" || normalized[0].Content != "text instructions" {
		t.Fatalf("normalized[0] = %#v, want first system message", normalized[0])
	}
	if normalized[1].Role != "system" || !reflect.DeepEqual(normalized[1].Content, structured) {
		t.Fatalf("normalized[1] = %#v, want structured system message", normalized[1])
	}
	if normalized[2].Role != "user" || normalized[3].Role != "assistant" {
		t.Fatalf("conversation order = %#v, want user then assistant", normalized[2:])
	}
}
