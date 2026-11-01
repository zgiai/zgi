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
	}, false)
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
	}, false)
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
	}, false)
	if got.MaxTokens == nil || *got.MaxTokens != jsonObjectCompletionFloor {
		t.Fatalf("max_tokens = %#v, want %d", got.MaxTokens, jsonObjectCompletionFloor)
	}
}

func TestShapeChatRequestForUpstreamCapsToModelWindow(t *testing.T) {
	requested := 240
	req := &adapter.ChatRequest{MaxTokens: &requested}
	got := shapeChatRequestForUpstream(req, &ProviderSelection{
		Model: llmmodel.LLMModel{SupportsReasoning: true, MaxOutputTokens: 512},
	}, false)
	if got.MaxTokens == nil || *got.MaxTokens != 512 {
		t.Fatalf("max_tokens = %#v, want 512", got.MaxTokens)
	}
}

func TestShapeChatRequestForUpstreamFillsAnthropicMaxTokens(t *testing.T) {
	claudeNative := func(maxOutput int) *ProviderSelection {
		return &ProviderSelection{
			ChannelProvider: "anthropic",
			Model:           llmmodel.LLMModel{Provider: "anthropic", Model: "claude-sonnet-5", MaxOutputTokens: maxOutput},
		}
	}
	tests := []struct {
		name      string
		selection *ProviderSelection
		streaming bool
		want      int
	}{
		{"streaming caps at 32000", claudeNative(128000), true, 32000},
		{"non-streaming caps at 16000", claudeNative(128000), false, 16000},
		{"catalog limit below the cap", claudeNative(8192), true, 8192},
		{"catalog without a limit", claudeNative(0), true, 8192},
		{"claude alias channel", &ProviderSelection{ChannelProvider: "claude", Model: llmmodel.LLMModel{Model: "claude-haiku-4-5", MaxOutputTokens: 64000}}, true, 32000},
		{"platform Claude by vendor", &ProviderSelection{ChannelProvider: "zgi-cloud", ModelVendor: "anthropic", Model: llmmodel.LLMModel{Model: "sonnet-public", MaxOutputTokens: 64000}}, false, 16000},
		{"platform Claude by model name", &ProviderSelection{ChannelProvider: "zgi-cloud", Model: llmmodel.LLMModel{Provider: "agicto", Model: "Claude-Sonnet-5", MaxOutputTokens: 128000}}, true, 32000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &adapter.ChatRequest{Model: "claude"}
			got := shapeChatRequestForUpstream(req, tt.selection, tt.streaming)
			if got.MaxTokens == nil || *got.MaxTokens != tt.want {
				t.Fatalf("max_tokens = %#v, want %d", got.MaxTokens, tt.want)
			}
			if req.MaxTokens != nil {
				t.Fatalf("caller request mutated: %#v", req.MaxTokens)
			}
		})
	}
}

func TestShapeChatRequestForUpstreamLeavesOtherRequestsAlone(t *testing.T) {
	explicit := 300
	tests := []struct {
		name      string
		req       *adapter.ChatRequest
		selection *ProviderSelection
		want      *int
	}{
		{"caller max_tokens wins on Claude", &adapter.ChatRequest{MaxTokens: &explicit}, &ProviderSelection{ChannelProvider: "anthropic", Model: llmmodel.LLMModel{Model: "claude-sonnet-5", MaxOutputTokens: 128000}}, &explicit},
		{"OpenAI-compatible keeps omitting", &adapter.ChatRequest{}, &ProviderSelection{ChannelProvider: "deepseek", Model: llmmodel.LLMModel{Model: "deepseek-v4-flash", MaxOutputTokens: 32768}}, nil},
		{"platform non-Claude keeps omitting", &adapter.ChatRequest{}, &ProviderSelection{ChannelProvider: "zgi-cloud", ModelVendor: "qwen", Model: llmmodel.LLMModel{Provider: "qwen", Model: "qwen3.8-27b", MaxOutputTokens: 32768}}, nil},
		{"Claude through an OpenAI-compatible channel keeps omitting", &adapter.ChatRequest{}, &ProviderSelection{ChannelProvider: "agicto", Model: llmmodel.LLMModel{Model: "claude-sonnet-5", MaxOutputTokens: 128000}}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shapeChatRequestForUpstream(tt.req, tt.selection, true)
			if tt.want == nil {
				if got.MaxTokens != nil {
					t.Fatalf("max_tokens = %d, want omitted", *got.MaxTokens)
				}
				return
			}
			if got.MaxTokens == nil || *got.MaxTokens != *tt.want {
				t.Fatalf("max_tokens = %#v, want %d", got.MaxTokens, *tt.want)
			}
		})
	}
}
