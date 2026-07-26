package workflow

import (
	"testing"

	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"

	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestBuildWorkflowStreamErrorPayloadRuntimeCodes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
	}{
		{name: "ownership lost", err: workflowpause.ErrExecutionOwnershipLost, code: "workflow_execution_ownership_lost"},
		{name: "event persistence", err: errWorkflowEventPersistenceFailed, code: "workflow_event_persistence_failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := buildWorkflowStreamErrorPayload(tt.err)
			if got, _ := payload["code"].(string); got != tt.code {
				t.Fatalf("error code = %q, want %q", got, tt.code)
			}
		})
	}
}

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
