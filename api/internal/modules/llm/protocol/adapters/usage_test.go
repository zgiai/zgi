package adapter

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUsageNormalizeCacheTokensSeparatesOpenAIInput(t *testing.T) {
	usage := &Usage{
		PromptTokens:     1000,
		CompletionTokens: 200,
		TotalTokens:      1200,
		PromptTokensDetails: PromptTokensDetails{
			CachedTokens:        600,
			CacheCreationTokens: 100,
		},
	}

	usage.NormalizeCacheTokens()

	if usage.UncachedInputTokens != 300 || usage.CacheReadTokens != 600 || usage.CacheWriteTokens != 100 {
		t.Fatalf("input buckets = %d/%d/%d, want 300/600/100", usage.UncachedInputTokens, usage.CacheReadTokens, usage.CacheWriteTokens)
	}
	if usage.TotalTokens != 1200 {
		t.Fatalf("total tokens = %d, want 1200", usage.TotalTokens)
	}
}

func TestUsageNormalizeCacheTokensDerivesCacheCreationTTLTotal(t *testing.T) {
	usage := &Usage{
		PromptTokens:     1000,
		CompletionTokens: 200,
		PromptTokensDetails: PromptTokensDetails{
			CachedTokens:          300,
			CacheCreation5mTokens: 100,
			CacheCreation1hTokens: 200,
		},
	}

	usage.NormalizeCacheTokens()

	if usage.CacheWriteTokens != 300 || usage.CacheWrite5mTokens != 100 || usage.CacheWrite1hTokens != 200 {
		t.Fatalf("cache write buckets = %d/%d/%d, want 300/100/200", usage.CacheWriteTokens, usage.CacheWrite5mTokens, usage.CacheWrite1hTokens)
	}
	if usage.UncachedInputTokens != 400 {
		t.Fatalf("uncached input tokens = %d, want 400", usage.UncachedInputTokens)
	}
}

func TestUsageAlwaysReportsCachedTokens(t *testing.T) {
	usage := Usage{PromptTokens: 12, CompletionTokens: 4, TotalTokens: 16}
	usage.NormalizeCacheTokens()
	payload, err := json.Marshal(usage)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !strings.Contains(string(payload), `"prompt_tokens_details":{"uncached_tokens":12,"cached_tokens":0}`) {
		t.Fatalf("usage = %s, want prompt_tokens_details.cached_tokens present", payload)
	}
}
