package service

import (
	"testing"

	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

func TestNormalizeChatRequestPropagatesRoutingStrategy(t *testing.T) {
	parts, err := normalizeChatRequest(runtimedto.ChatRequest{
		Query:           "hello",
		Model:           "deepseek-v4-pro",
		RoutingStrategy: "price",
	})
	if err != nil {
		t.Fatalf("normalize chat request: %v", err)
	}
	if parts.RoutingStrategy != "price" {
		t.Fatalf("parts routing strategy = %q, want price", parts.RoutingStrategy)
	}
	llmReq := newLLMChatRequest(parts, nil)
	if llmReq.RoutingStrategy != "price" {
		t.Fatalf("LLM routing strategy = %q, want price", llmReq.RoutingStrategy)
	}
}

func TestNormalizeRegenerateRequestPreservesSupportedStrategy(t *testing.T) {
	parts, err := normalizeRegenerateRequest(runtimedto.RegenerateMessageRequest{}, &runtimemodel.Message{
		Query:     "hello",
		ModelName: "deepseek-v4-pro",
		Metadata:  map[string]interface{}{"routing_strategy": "stability"},
	})
	if err != nil {
		t.Fatalf("normalize regenerate request: %v", err)
	}
	if parts.RoutingStrategy != "stability" {
		t.Fatalf("routing strategy = %q, want stability", parts.RoutingStrategy)
	}
}

func TestNormalizeRoutingStrategy(t *testing.T) {
	tests := map[string]string{
		"": "stability", "unexpected": "stability", "balanced": "stability",
		"speed": "speed", "PRICE": "price", " stability ": "stability",
	}
	for input, want := range tests {
		if got := normalizeRoutingStrategy(input); got != want {
			t.Fatalf("normalizeRoutingStrategy(%q) = %q, want %q", input, got, want)
		}
	}
}
