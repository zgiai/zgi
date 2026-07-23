package tokenestimate

import (
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestEstimateChatRequestIncludesPromptBearingComponents(t *testing.T) {
	temperature := 0.2
	request := &adapter.ChatRequest{
		Model:       "deepseek-chat",
		Temperature: &temperature,
		Messages: []adapter.Message{{
			Role:    "user",
			Content: "summarize the record",
		}},
		Tools: []adapter.Tool{{
			Type: "function",
			Function: adapter.Function{
				Name:        "lookup_record",
				Description: "Look up one record",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{"type": "string"},
					},
				},
			},
		}},
		ToolChoice:     map[string]interface{}{"type": "auto"},
		ResponseFormat: &adapter.ResponseFormat{Type: "json_object"},
		AdditionalParameters: map[string]interface{}{
			"reasoning_effort": "low",
		},
	}

	result := NewEstimator().EstimateChatRequest(request)
	for _, name := range []string{"messages", "tools", "tool_choice", "response_format", "additional_parameters"} {
		component, ok := result.Components[name]
		if !ok {
			t.Fatalf("component %q missing: %#v", name, result.Components)
		}
		if component.Tokens <= 0 || component.Characters <= 0 {
			t.Fatalf("component %q = %+v, want positive counts", name, component)
		}
	}

	total := 0
	for _, component := range result.Components {
		total += component.Tokens
	}
	if result.Tokens != total {
		t.Fatalf("tokens = %d, want component total %d", result.Tokens, total)
	}
	if result.Characters <= result.Components["messages"].Characters {
		t.Fatalf("request characters = %d, want larger than messages-only %d", result.Characters, result.Components["messages"].Characters)
	}
	if result.Tokenizer == "" {
		t.Fatal("tokenizer is empty")
	}
}

func TestEstimateChatRequestDoesNotTreatGenerationControlsAsPromptComponents(t *testing.T) {
	temperature := 0.7
	maxTokens := 4096
	request := &adapter.ChatRequest{
		Model:       "deepseek-chat",
		Messages:    []adapter.Message{{Role: "user", Content: "hello"}},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		Stop:        []string{"DONE"},
	}

	result := NewEstimator().EstimateChatRequest(request)
	if len(result.Components) != 1 {
		t.Fatalf("components = %#v, want messages only", result.Components)
	}
	if _, ok := result.Components["messages"]; !ok {
		t.Fatalf("messages component missing: %#v", result.Components)
	}
	if result.Characters <= result.Components["messages"].Characters {
		t.Fatalf("request characters = %d, want controls reflected beyond messages %d", result.Characters, result.Components["messages"].Characters)
	}
}
