package provider

import (
	"encoding/json"
	"testing"
)

func TestAnthropicUsageFromRawParsesCacheCreationTTLBreakdown(t *testing.T) {
	usage := anthropicUsageFromRaw(json.RawMessage(`{
		"usage": {
			"input_tokens": 400,
			"cache_read_input_tokens": 300,
			"cache_creation": {
				"ephemeral_5m_input_tokens": 100,
				"ephemeral_1h_input_tokens": 200
			},
			"output_tokens": 50
		}
	}`), nil)

	if usage == nil {
		t.Fatal("usage is nil")
	}
	if usage.UncachedInputTokens != 400 || usage.CacheReadTokens != 300 {
		t.Fatalf("input buckets = %d/%d, want 400/300", usage.UncachedInputTokens, usage.CacheReadTokens)
	}
	if usage.CacheWriteTokens != 300 || usage.CacheWrite5mTokens != 100 || usage.CacheWrite1hTokens != 200 {
		t.Fatalf("cache write buckets = %d/%d/%d, want 300/100/200", usage.CacheWriteTokens, usage.CacheWrite5mTokens, usage.CacheWrite1hTokens)
	}
	if usage.PromptTokens != 1000 || usage.TotalTokens != 1050 {
		t.Fatalf("totals = %d/%d, want 1000/1050", usage.PromptTokens, usage.TotalTokens)
	}
}
