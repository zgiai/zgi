package provider

import (
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestClaudeAdapterConvertToClaude_DefaultMaxTokens(t *testing.T) {
	a, err := NewClaudeAdapter(&adapter.AdapterConfig{ProviderName: "claude", APIKey: "test-key", BaseURL: "https://api.anthropic.com"})
	if err != nil {
		t.Fatalf("NewClaudeAdapter() error = %v", err)
	}
	messages := []adapter.Message{{Role: "user", Content: "hi"}}

	req, err := a.convertToClaude(&adapter.ChatRequest{Model: "claude-sonnet-5", Messages: messages})
	if err != nil {
		t.Fatalf("convertToClaude() error = %v", err)
	}
	if req.MaxTokens != 8192 {
		t.Fatalf("default max_tokens = %d, want 8192 (zgi-pre#2914)", req.MaxTokens)
	}

	explicit := 300
	req, err = a.convertToClaude(&adapter.ChatRequest{Model: "claude-sonnet-5", Messages: messages, MaxTokens: &explicit})
	if err != nil {
		t.Fatalf("convertToClaude() error = %v", err)
	}
	if req.MaxTokens != 300 {
		t.Fatalf("explicit max_tokens = %d, want 300", req.MaxTokens)
	}
}
