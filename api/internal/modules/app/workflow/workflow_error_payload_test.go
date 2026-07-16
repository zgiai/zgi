package workflow

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestWorkflowBillingErrorCodeAndMessageModelPricingNotConfigured(t *testing.T) {
	code, message, ok := workflowBillingErrorCodeAndMessage(&gateway.BillingUserError{
		Kind: gateway.BillingUserErrorKindModelPricingNotConfigured,
	})

	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if code != response.ErrWorkflowModelPricingNotConfigured.Code {
		t.Fatalf("code = %d, want %d", code, response.ErrWorkflowModelPricingNotConfigured.Code)
	}
	if message != response.ErrWorkflowModelPricingNotConfigured.Message {
		t.Fatalf("message = %q, want %q", message, response.ErrWorkflowModelPricingNotConfigured.Message)
	}
}

func TestWorkflowStreamErrorPayloadIncludesBillingParams(t *testing.T) {
	payload := buildWorkflowStreamErrorPayload(&gateway.BillingUserError{
		Kind: gateway.BillingUserErrorKindModelPricingNotConfigured,
		Params: map[string]interface{}{
			"model_id":  "model-1",
			"operation": "image",
		},
	})

	params, ok := payload["params"].(map[string]any)
	if !ok {
		t.Fatalf("params = %#v, want map", payload["params"])
	}
	if params["model_id"] != "model-1" || params["operation"] != "image" {
		t.Fatalf("params = %#v, want billing params", params)
	}
}
