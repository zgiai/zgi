package gateway

import (
	"encoding/json"
	"testing"

	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
)

func TestEstimateNativeProtocolTokensIncludesProviderFramingReserve(t *testing.T) {
	s := &llmGatewayServiceImpl{tokenEstimator: NewTokenEstimator()}
	body := json.RawMessage(`{"model":"deepseek-v4-pro","input":"Reply OK","max_output_tokens":16,"store":false}`)

	promptTokens, completionTokens := s.estimateNativeProtocolTokens(body, "deepseek-v4-pro", nil)
	contentOnly := s.tokenEstimator.EstimateTextTokensForModel("deepseek-v4-pro", `"Reply OK"`)
	if promptTokens < contentOnly+nativeProtocolInputReservationOverheadTokens {
		t.Fatalf("prompt reservation = %d, want at least content %d + overhead %d", promptTokens, contentOnly, nativeProtocolInputReservationOverheadTokens)
	}
	if completionTokens != 16 {
		t.Fatalf("completion reservation = %d, want explicit max_output_tokens 16", completionTokens)
	}
}

func TestEstimateNativeProtocolTokensAppliesReasoningCatalogFloor(t *testing.T) {
	s := &llmGatewayServiceImpl{tokenEstimator: NewTokenEstimator()}
	body := json.RawMessage(`{"model":"doubao-seed-2-0-mini-260215","input":"Reply OK","max_output_tokens":8}`)
	selection := &ProviderSelection{
		Model: llmmodel.LLMModel{SupportsReasoning: true, MaxOutputTokens: 32768},
	}

	_, completionTokens := s.estimateNativeProtocolTokens(body, "doubao-seed-2-0-mini-260215", selection)
	want := min(32768, reasoningCompletionReservationCap)
	if completionTokens != want {
		t.Fatalf("reasoning completion reservation = %d, want %d", completionTokens, want)
	}
}

func TestSaturatingAddPositiveDoesNotOverflow(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	if got := saturatingAddPositive(maxInt-1, 128); got != maxInt {
		t.Fatalf("saturatingAddPositive() = %d, want max int %d", got, maxInt)
	}
}
