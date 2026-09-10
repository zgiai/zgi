package gateway

import (
	"encoding/json"
	"testing"
)

func TestEstimateNativeProtocolTokensIncludesProviderFramingReserve(t *testing.T) {
	s := &llmGatewayServiceImpl{tokenEstimator: NewTokenEstimator()}
	body := json.RawMessage(`{"model":"deepseek-v4-pro","input":"Reply OK","max_output_tokens":16,"store":false}`)

	promptTokens, completionTokens := s.estimateNativeProtocolTokens(body, "deepseek-v4-pro")
	contentOnly := s.tokenEstimator.EstimateTextTokensForModel("deepseek-v4-pro", `"Reply OK"`)
	if promptTokens < contentOnly+nativeProtocolInputReservationOverheadTokens {
		t.Fatalf("prompt reservation = %d, want at least content %d + overhead %d", promptTokens, contentOnly, nativeProtocolInputReservationOverheadTokens)
	}
	if completionTokens != 16 {
		t.Fatalf("completion reservation = %d, want explicit max_output_tokens 16", completionTokens)
	}
}

func TestSaturatingAddPositiveDoesNotOverflow(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	if got := saturatingAddPositive(maxInt-1, 128); got != maxInt {
		t.Fatalf("saturatingAddPositive() = %d, want max int %d", got, maxInt)
	}
}
