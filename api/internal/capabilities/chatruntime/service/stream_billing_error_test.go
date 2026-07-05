package service

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestChatRuntimeBillingErrorCodeAndMessageModelPricingNotConfigured(t *testing.T) {
	code, message, ok := aichatBillingErrorCodeAndMessage(&gateway.BillingUserError{
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
