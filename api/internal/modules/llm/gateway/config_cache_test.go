package gateway

import (
	"encoding/json"
	"testing"

	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
)

func TestModelCachePreservesInternalCapabilities(t *testing.T) {
	model := &llmmodel.LLMModel{
		Provider:          "openai",
		Model:             "gpt-5",
		Responses:         true,
		ChatCompletions:   true,
		SupportsStreaming: true,
		SupportsToolCall:  true,
		SystemPrompt:      true,
	}

	data, err := marshalCachedModel(model)
	if err != nil {
		t.Fatalf("marshalCachedModel() error = %v", err)
	}

	var got llmmodel.LLMModel
	if err := unmarshalCachedModel(data, &got); err != nil {
		t.Fatalf("unmarshalCachedModel() error = %v", err)
	}
	if !got.Responses {
		t.Fatal("Responses = false, want true")
	}
	if !got.ChatCompletions {
		t.Fatal("ChatCompletions = false, want true")
	}
	if !got.SupportsStreaming {
		t.Fatal("SupportsStreaming = false, want true")
	}
	if !got.SupportsToolCall {
		t.Fatal("SupportsToolCall = false, want true")
	}
}

func TestModelCacheRejectsLegacyJSONWithoutInternalCapabilities(t *testing.T) {
	legacy, err := json.Marshal(&llmmodel.LLMModel{
		Provider:  "openai",
		Model:     "gpt-5",
		Responses: true,
	})
	if err != nil {
		t.Fatalf("marshal legacy model: %v", err)
	}

	var got llmmodel.LLMModel
	if err := unmarshalCachedModel(legacy, &got); err == nil {
		t.Fatal("unmarshalCachedModel() error = nil, want legacy cache rejection")
	}
}
