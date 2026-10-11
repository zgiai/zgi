package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

// T17 acceptance 2/7: a platform lane pricing failure that reached the gateway
// as ErrPricingNotConfigured exits as HTTP 402 with the stable code, retryable
// false and the localized catalog message, never internal_error/500.
func TestT17PlatformPricingNotConfiguredExits402(t *testing.T) {
	handler := &LLMHandler{errorProjector: testApplicationErrorProjector(t)}
	context, recorder := newT17Context("zh-CN")

	err := fmt.Errorf("calculate authoritative token cost: %w", gateway.ErrPricingNotConfigured)
	protocolErr := handler.localizedProtocolError(context, err)
	writeOpenAIProtocolError(context, protocolErr)

	if recorder.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402", recorder.Code)
	}
	if got := recorder.Header().Get(apptransport.HeaderApplicationErrorCode); got != "billing.model.pricing_not_configured" {
		t.Fatalf("application error code header = %q", got)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "billing.model.pricing_not_configured" {
		t.Fatalf("code = %v", body["code"])
	}
	if body["retryable"] != false {
		t.Fatalf("retryable = %v, want false", body["retryable"])
	}
	if body["error_id"] == nil || body["error_id"] == "" {
		t.Fatal("missing error_id")
	}
	errorBody, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error envelope = %#v", body)
	}
	message, _ := errorBody["message"].(string)
	if !strings.Contains(message, "模型未配置价格") {
		t.Fatalf("message not localized: %q", message)
	}
	if strings.Contains(strings.ToLower(message), "internal") {
		t.Fatalf("pricing error leaked internal wording: %q", message)
	}
}
