package service

import (
	"testing"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestPreparedResultMetadataForPreparedAccumulatesContinuationUsage(t *testing.T) {
	prepared := &PreparedChat{
		Continuation: true,
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"usage": map[string]interface{}{
				"prompt_tokens":     100,
				"completion_tokens": 20,
				"total_tokens":      120,
			},
		}},
	}
	beginPreparedUsageExecution(prepared)

	// Simulate a partial-answer write replacing usage during the same execution.
	prepared.Message.Metadata["usage"] = usageMetadata(&adapter.Usage{
		PromptTokens: 50, CompletionTokens: 10, TotalTokens: 60,
	})
	metadata := preparedResultMetadataForPrepared(prepared, prepared.Message.Metadata, &adapter.Usage{
		PromptTokens: 50, CompletionTokens: 10, TotalTokens: 60,
	})

	assertUsageMetadata(t, metadata["usage"], adapter.Usage{
		PromptTokens: 150, CompletionTokens: 30, TotalTokens: 180,
	})
	assertUsageMetadata(t, metadata["latest_execution_usage"], adapter.Usage{
		PromptTokens: 50, CompletionTokens: 10, TotalTokens: 60,
	})
	if got := metadata["usage_scope"]; got != "message" {
		t.Fatalf("usage_scope = %#v, want message", got)
	}
}

func TestPreparedResultMetadataForPreparedDoesNotCarryUsageIntoNewMessage(t *testing.T) {
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"usage": map[string]interface{}{"total_tokens": 120},
		}},
	}
	beginPreparedUsageExecution(prepared)
	metadata := preparedResultMetadataForPrepared(prepared, prepared.Message.Metadata, &adapter.Usage{
		PromptTokens: 8, CompletionTokens: 2, TotalTokens: 10,
	})

	assertUsageMetadata(t, metadata["usage"], adapter.Usage{
		PromptTokens: 8, CompletionTokens: 2, TotalTokens: 10,
	})
	if _, ok := metadata["latest_execution_usage"]; ok {
		t.Fatal("latest_execution_usage should only be added for same-message continuations")
	}
}

func assertUsageMetadata(t *testing.T, value interface{}, want adapter.Usage) {
	t.Helper()
	usageMap, ok := value.(map[string]interface{})
	if !ok {
		t.Fatalf("usage metadata = %#v, want map", value)
	}
	got := adapter.Usage{
		PromptTokens:     intValueFromAny(usageMap["prompt_tokens"]),
		CompletionTokens: intValueFromAny(usageMap["completion_tokens"]),
		TotalTokens:      intValueFromAny(usageMap["total_tokens"]),
	}
	if got != want {
		t.Fatalf("usage metadata = %+v, want %+v", got, want)
	}
}
