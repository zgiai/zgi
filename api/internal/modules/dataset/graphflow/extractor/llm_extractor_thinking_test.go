package extractor

import (
	"testing"

	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestThinkingDisablingParametersTargetsKnownProvidersAndReasoningModels(t *testing.T) {
	tests := []struct {
		name              string
		provider          string
		model             string
		wantParams        bool
		wantThinkingField bool
	}{
		{
			name:       "qwen uses enable_thinking only",
			provider:   "qwen",
			model:      "qwen3.6-flash",
			wantParams: true,
		},
		{
			name:       "dashscope uses enable_thinking only",
			provider:   "DashScope",
			model:      "qwen-max",
			wantParams: true,
		},
		{
			name:              "deepseek also disables thinking object",
			provider:          "deepseek",
			model:             "deepseek-v4-flash",
			wantParams:        true,
			wantThinkingField: true,
		},
		{
			name:       "plain openai model is left untouched",
			provider:   "openai",
			model:      "gpt-4o",
			wantParams: false,
		},
		{
			name:              "openai reasoning model is disabled",
			provider:          "openai",
			model:             "o3-mini",
			wantParams:        true,
			wantThinkingField: true,
		},
		{
			name:              "unknown provider with thinking model is disabled",
			provider:          "openai-compatible",
			model:             "kimi-k2-thinking",
			wantParams:        true,
			wantThinkingField: true,
		},
		{
			name:       "unknown provider with plain model is left untouched",
			provider:   "openai-compatible",
			model:      "InnoSpark-235B",
			wantParams: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parameters := thinkingDisablingParameters(test.provider, test.model)
			if test.wantParams {
				if parameters == nil {
					t.Fatalf("expected parameters for provider=%q model=%q, got nil", test.provider, test.model)
				}
				if enabled, ok := parameters["enable_thinking"]; !ok || enabled != false {
					t.Fatalf("expected enable_thinking=false, got %#v", parameters["enable_thinking"])
				}
				_, hasThinking := parameters["thinking"]
				if hasThinking != test.wantThinkingField {
					t.Fatalf("thinking field presence=%v, want %v (params=%#v)", hasThinking, test.wantThinkingField, parameters)
				}
				return
			}
			if parameters != nil {
				t.Fatalf("expected no parameters for provider=%q model=%q, got %#v", test.provider, test.model, parameters)
			}
		})
	}
}

func TestApplyThinkingPolicyPreservesCallerParameters(t *testing.T) {
	req := &llmadapter.ChatRequest{
		Provider: "qwen",
		Model:    "qwen3.6-flash",
		AdditionalParameters: map[string]interface{}{
			"top_p": 0.9,
		},
	}

	applyThinkingPolicy(req)

	if got := req.AdditionalParameters["enable_thinking"]; got != false {
		t.Fatalf("enable_thinking=%#v, want false", got)
	}
	if got := req.AdditionalParameters["top_p"]; got != 0.9 {
		t.Fatalf("top_p=%#v, want 0.9", got)
	}
}

func TestApplyThinkingPolicyLeavesNonReasoningRequestUnchanged(t *testing.T) {
	req := &llmadapter.ChatRequest{Provider: "openai", Model: "gpt-4o"}

	applyThinkingPolicy(req)

	if req.AdditionalParameters != nil {
		t.Fatalf("expected no additional parameters, got %#v", req.AdditionalParameters)
	}
}
