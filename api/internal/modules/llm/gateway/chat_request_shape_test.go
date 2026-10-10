package gateway

import (
	"testing"

	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestShapeChatRequestForUpstreamRaisesReasoningBudget(t *testing.T) {
	requested := 240
	req := &adapter.ChatRequest{
		Model:     "deepseek-v4-pro",
		MaxTokens: &requested,
		ToolChoice: map[string]interface{}{
			"type": "tool",
			"name": "get_weather",
		},
	}
	got := shapeChatRequestForUpstream(req, &ProviderSelection{
		Model: llmmodel.LLMModel{SupportsReasoning: true, MaxOutputTokens: 32768},
	})
	if got.MaxTokens == nil || *got.MaxTokens != reasoningUpstreamCompletionFloor {
		t.Fatalf("max_tokens = %#v, want %d", got.MaxTokens, reasoningUpstreamCompletionFloor)
	}
	if requested != 240 {
		t.Fatalf("original max_tokens mutated: %d", requested)
	}
	named, ok := got.ToolChoice.(map[string]interface{})
	if !ok {
		t.Fatalf("tool_choice = %#v, want named function object", got.ToolChoice)
	}
	function, _ := named["function"].(map[string]string)
	if named["type"] != "function" || function["name"] != "get_weather" {
		t.Fatalf("tool_choice = %#v, want OpenAI function object", got.ToolChoice)
	}
}

func TestShapeChatRequestForUpstreamKeepsExplicitTruncation(t *testing.T) {
	requested := 8
	req := &adapter.ChatRequest{MaxTokens: &requested}
	got := shapeChatRequestForUpstream(req, &ProviderSelection{
		Model: llmmodel.LLMModel{SupportsReasoning: true, MaxOutputTokens: 32768},
	})
	if got.MaxTokens == nil || *got.MaxTokens != 8 {
		t.Fatalf("max_tokens = %#v, want 8", got.MaxTokens)
	}
}

func TestShapeChatRequestForUpstreamRaisesJSONObjectBudget(t *testing.T) {
	requested := 120
	req := &adapter.ChatRequest{
		MaxTokens:      &requested,
		ResponseFormat: &adapter.ResponseFormat{Type: "json_object"},
	}
	got := shapeChatRequestForUpstream(req, &ProviderSelection{
		Model: llmmodel.LLMModel{SupportsReasoning: false},
	})
	if got.MaxTokens == nil || *got.MaxTokens != jsonObjectCompletionFloor {
		t.Fatalf("max_tokens = %#v, want %d", got.MaxTokens, jsonObjectCompletionFloor)
	}
}

func TestShapeChatRequestForUpstreamCapsToModelWindow(t *testing.T) {
	requested := 240
	req := &adapter.ChatRequest{MaxTokens: &requested}
	got := shapeChatRequestForUpstream(req, &ProviderSelection{
		Model: llmmodel.LLMModel{SupportsReasoning: true, MaxOutputTokens: 512},
	})
	if got.MaxTokens == nil || *got.MaxTokens != 512 {
		t.Fatalf("max_tokens = %#v, want 512", got.MaxTokens)
	}
}
