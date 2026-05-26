package provider

import (
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestMistralAdapterResolveDefaultBaseURL(t *testing.T) {
	instance, err := adapter.NewAdapter(&adapter.AdapterConfig{
		ProviderName: "mistral",
		APIKey:       "test-key",
	})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	got, ok := instance.(*MistralAdapter)
	if !ok {
		t.Fatalf("instance = %T, want *MistralAdapter", instance)
	}
	if got.openAI.baseURL != defaultMistralBaseURL {
		t.Fatalf("baseURL = %q, want %q", got.openAI.baseURL, defaultMistralBaseURL)
	}
}

func TestMistralAdapterNormalizesToolCallIDs(t *testing.T) {
	request := normalizeMistralChatRequest(&adapter.ChatRequest{
		Model: "mistral-large-latest",
		Messages: []adapter.Message{
			{
				Role:    "assistant",
				Content: "",
				ToolCalls: []adapter.ToolCall{
					{ID: "long-tool-call-id", Type: "function", Function: adapter.FunctionCall{Name: "lookup"}},
				},
			},
			{Role: "tool", ToolCallID: "long-tool-call-id", Content: "done"},
		},
	})

	gotID := request.Messages[0].ToolCalls[0].ID
	if !mistralToolCallIDPattern.MatchString(gotID) {
		t.Fatalf("tool call id = %q, want 9 alphanumeric chars", gotID)
	}
	if request.Messages[1].ToolCallID != gotID {
		t.Fatalf("tool response id = %q, want %q", request.Messages[1].ToolCallID, gotID)
	}
	if request.Messages[0].Content != nil {
		t.Fatalf("assistant tool-call content = %#v, want nil", request.Messages[0].Content)
	}
}

func TestMistralAdapterGeneratesDistinctBlankToolCallIDs(t *testing.T) {
	request := normalizeMistralChatRequest(&adapter.ChatRequest{
		Model: "mistral-large-latest",
		Messages: []adapter.Message{
			{
				Role:    "assistant",
				Content: "",
				ToolCalls: []adapter.ToolCall{
					{Type: "function", Function: adapter.FunctionCall{Name: "lookup"}},
					{Type: "function", Function: adapter.FunctionCall{Name: "search"}},
				},
			},
		},
	})

	firstID := request.Messages[0].ToolCalls[0].ID
	secondID := request.Messages[0].ToolCalls[1].ID
	if !mistralToolCallIDPattern.MatchString(firstID) {
		t.Fatalf("first tool call id = %q, want 9 alphanumeric chars", firstID)
	}
	if !mistralToolCallIDPattern.MatchString(secondID) {
		t.Fatalf("second tool call id = %q, want 9 alphanumeric chars", secondID)
	}
	if firstID == secondID {
		t.Fatalf("blank tool call IDs normalized to the same value %q", firstID)
	}
}
