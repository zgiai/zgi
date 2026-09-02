package adapter

import "testing"

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
