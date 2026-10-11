package dto

import (
	"encoding/json"
	"testing"
)

func TestChatRequestDecodesRoutingStrategy(t *testing.T) {
	var req ChatRequest
	if err := json.Unmarshal([]byte(`{"query":"hello","model":"deepseek-v4-pro","routing_strategy":"price"}`), &req); err != nil {
		t.Fatalf("unmarshal chat request: %v", err)
	}
	if req.RoutingStrategy != "price" {
		t.Fatalf("routing strategy = %q, want price", req.RoutingStrategy)
	}
}

func TestRegenerateMessageRequestDecodesRoutingStrategy(t *testing.T) {
	var req RegenerateMessageRequest
	if err := json.Unmarshal([]byte(`{"routing_strategy":"price"}`), &req); err != nil {
		t.Fatalf("unmarshal regenerate request: %v", err)
	}
	if req.RoutingStrategy == nil || *req.RoutingStrategy != "price" {
		t.Fatalf("routing strategy = %v, want price", req.RoutingStrategy)
	}
}
