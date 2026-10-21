package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/datatypes"
)

// T13 acceptance 1: by-second video settlement must charge the upstream actual
// duration, not the requested/declared duration.
func TestT13VideoSettlementBySecondUsesUpstreamActualDuration(t *testing.T) {
	service := &llmGatewayServiceImpl{}
	selection := t13PerSecondVideoSelection()
	req := &adapter.VideoTaskRequest{
		Model: "video-per-second",
		AdditionalParameters: map[string]interface{}{
			// Declared duration is deliberately different from the actual one.
			"resolution": "720p",
			"duration":   5,
		},
	}
	resp := &adapter.VideoResponse{Status: "succeeded", Raw: map[string]interface{}{"duration": 8}}

	quote, err := service.quoteVideoPricingForSelection(context.Background(), selection, req, resp)
	if err != nil {
		t.Fatalf("quoteVideoPricingForSelection() error = %v", err)
	}
	// $0.02 per second x 8 actual seconds = $0.16 = 160000 milli-points.
	if quote.TotalCredits != 160000 {
		t.Fatalf("total credits = %d, want 160000 (actual 8s, not declared 5s)", quote.TotalCredits)
	}
	var snapshot map[string]interface{}
	if err := json.Unmarshal(quote.PricingSnapshot, &snapshot); err != nil {
		t.Fatalf("unmarshal pricing snapshot: %v", err)
	}
	if snapshot["duration_seconds"] != float64(8) {
		t.Fatalf("snapshot duration_seconds = %#v, want 8", snapshot["duration_seconds"])
	}
}

// T13 acceptance 1: a by-second task without an upstream actual duration must
// fail closed and never settle the requested 5 seconds.
func TestT13VideoSettlementBySecondFailsClosedWithoutUpstreamDuration(t *testing.T) {
	service := &llmGatewayServiceImpl{}
	selection := t13PerSecondVideoSelection()
	req := &adapter.VideoTaskRequest{
		Model: "video-per-second",
		AdditionalParameters: map[string]interface{}{
			"resolution": "720p",
			"duration":   5,
		},
	}
	resp := &adapter.VideoResponse{Status: "succeeded"}

	_, err := service.quoteVideoPricingForSelection(context.Background(), selection, req, resp)
	if !errors.Is(err, ErrBillingSettleFailed) {
		t.Fatalf("quote error = %v, want ErrBillingSettleFailed", err)
	}
	if errors.Is(err, ErrPricingNotConfigured) {
		t.Fatalf("missing duration must not be reported as a pricing configuration error: %v", err)
	}
}

// T13 acceptance 1: the success settlement path must return the fail-closed
// error before touching the billing provider, so the pre-deduct hold stays
// unsettled for reconciliation instead of being charged with a fabricated
// duration.
func TestT13VideoSettlementSuccessPathDoesNotSettleWithoutDuration(t *testing.T) {
	local := &fakeBillingProvider{checkBalanceResult: true}
	service := &llmGatewayServiceImpl{localBilling: local}
	selection := t13PerSecondVideoSelection()
	req := &adapter.VideoTaskRequest{
		Model: "video-per-second",
		AdditionalParameters: map[string]interface{}{
			"resolution":         "720p",
			"duration":           5,
			"billing_request_id": "t13-video-request",
			"billing_attempt_id": "t13-video-request:0",
		},
	}
	resp := &adapter.VideoResponse{Status: "succeeded"}

	err := service.settleVideoTaskSuccess(
		context.Background(),
		nil,
		nil,
		selection,
		uuid.New(),
		uuid.New(),
		req,
		resp,
	)
	if !errors.Is(err, ErrBillingSettleFailed) {
		t.Fatalf("settleVideoTaskSuccess() error = %v, want ErrBillingSettleFailed", err)
	}
	if local.settleCalls != 0 {
		t.Fatalf("billing settle calls = %d, want 0 (must not settle without actual duration)", local.settleCalls)
	}
}

func t13PerSecondVideoSelection() *ProviderSelection {
	return &ProviderSelection{
		OrganizationID: uuid.New(),
		Model: llmmodel.LLMModel{
			ID:       uuid.New(),
			Provider: "example",
			Model:    "video-per-second",
			Currency: "USD",
			Pricing: datatypes.JSON([]byte(`{
				"video_generation": {
					"currency": "USD",
					"billing_unit": "second",
					"rates": [
						{"resolution": "720p", "price_per_second": 0.02}
					]
				}
			}`)),
		},
	}
}
