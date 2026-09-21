package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	llmshared "github.com/zgiai/zgi/api/internal/modules/llm/shared"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

func billingUserCause(kind gateway.BillingUserErrorKind) error {
	return fmt.Errorf("request failed: %w", &gateway.BillingUserError{
		Kind:  kind,
		Cause: errors.New("private-database-detail"),
	})
}

// T17 acceptance 1: the four quota causes are branched before the generic
// DomainErrBillingFailed and each keeps its own stable code.
func TestT17ClassifyBillingUserCausesDistinct(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode string
	}{
		{
			name:     "organization balance",
			err:      billingUserCause(gateway.BillingUserErrorKindOrganizationBalanceInsufficient),
			wantCode: "billing.organization.balance_insufficient",
		},
		{
			name:     "private channel balance",
			err:      billingUserCause(gateway.BillingUserErrorKindPrivateChannelBalanceInsufficient),
			wantCode: "billing.private_channel.balance_insufficient",
		},
		{
			name:     "workspace quota",
			err:      billingUserCause(gateway.BillingUserErrorKindWorkspaceQuotaInsufficient),
			wantCode: "billing.workspace.quota_insufficient",
		},
		{
			name:     "api key quota",
			err:      billingUserCause(gateway.BillingUserErrorKindAPIKeyQuotaInsufficient),
			wantCode: "llm.quota.exceeded",
		},
	}
	seen := map[string]string{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyProtocolError(tc.err)
			if got.appCode != tc.wantCode {
				t.Fatalf("appCode = %q, want %q", got.appCode, tc.wantCode)
			}
			if got.openAICode == "internal_error" {
				t.Fatalf("billing cause collapsed into internal_error: %#v", got)
			}
			if tc.wantCode != "llm.quota.exceeded" && got.openAICode == "insufficient_quota" {
				t.Fatalf("billing cause collapsed into insufficient_quota: %#v", got)
			}
			if previous, exists := seen[got.appCode]; exists {
				t.Fatalf("code %q reused for %q and %q", got.appCode, previous, tc.name)
			}
			seen[got.appCode] = tc.name
		})
	}
}

// T17 acceptance 2: unconfigured pricing is a stable configuration failure,
// never a 500.
func TestT17PricingNotConfiguredIsConfigurationFailure(t *testing.T) {
	for _, err := range []error{
		gateway.ErrPricingNotConfigured,
		billingUserCause(gateway.BillingUserErrorKindModelPricingNotConfigured),
	} {
		got := classifyProtocolError(err)
		if got.appCode != "billing.model.pricing_not_configured" {
			t.Fatalf("appCode = %q, want billing.model.pricing_not_configured", got.appCode)
		}
		if got.openAIStatus != http.StatusPaymentRequired {
			t.Fatalf("status = %d, want 402", got.openAIStatus)
		}
		if got.appCode == "internal_error" || got.openAICode == "internal_error" {
			t.Fatalf("pricing error collapsed into internal_error: %#v", got)
		}
	}
}

// T17 acceptance 3: the four provider selection failures keep separate codes.
func TestT17ProviderFailuresHaveSeparateBranches(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode string
		wantHTTP int
	}{
		{"no provider", gateway.NewNoProviderAvailableError("m", "o"), "llm.provider.none_available", http.StatusServiceUnavailable},
		{"provider not found", gateway.ErrProviderNotFound, "llm.provider.not_found", http.StatusNotFound},
		{"channel not found", llmerrors.DomainErrChannelNotFound, "llm.channel.not_found", http.StatusNotFound},
		{"upstream unavailable", llmerrors.DomainErrUpstreamUnavailable, "llm.provider.unavailable", http.StatusServiceUnavailable},
	}
	seen := map[string]struct{}{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyProtocolError(tc.err)
			if got.appCode != tc.wantCode || got.openAIStatus != tc.wantHTTP {
				t.Fatalf("got code=%q status=%d, want code=%q status=%d", got.appCode, got.openAIStatus, tc.wantCode, tc.wantHTTP)
			}
			if got.appCode == "provider_unavailable" {
				t.Fatal("provider failure collapsed into provider_unavailable")
			}
			seen[got.appCode] = struct{}{}
		})
	}
	if len(seen) != len(cases) {
		t.Fatalf("provider branches collapsed: %d distinct codes for %d causes", len(seen), len(cases))
	}
}

func newT17Context(language string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	request.Header.Set("Accept-Language", language)
	context.Request = request
	context.Header("X-Request-ID", "req-t17")
	return context, recorder
}

// T17 acceptance 5: the projected HTTP body carries code/message/retryable/
// error_id/request_id, and 429 responses carry Retry-After.
func TestT17ProjectedErrorEnvelope(t *testing.T) {
	handler := &LLMHandler{errorProjector: testApplicationErrorProjector(t)}

	t.Run("organization balance", func(t *testing.T) {
		context, recorder := newT17Context("zh-CN")
		protocolErr := handler.localizedProtocolError(context, billingUserCause(gateway.BillingUserErrorKindOrganizationBalanceInsufficient))
		writeOpenAIProtocolError(context, protocolErr)

		var body map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["code"] != "billing.organization.balance_insufficient" {
			t.Fatalf("code = %v", body["code"])
		}
		if body["retryable"] != false {
			t.Fatalf("retryable = %v", body["retryable"])
		}
		if body["error_id"] == nil || body["error_id"] == "" {
			t.Fatal("missing error_id")
		}
		if body["request_id"] != "req-t17" {
			t.Fatalf("request_id = %v", body["request_id"])
		}
		errorBody := body["error"].(map[string]any)
		if message, _ := errorBody["message"].(string); !strings.Contains(message, "组织") {
			t.Fatalf("message not localized: %q", message)
		}
		if strings.Contains(recorder.Body.String(), "private-database-detail") {
			t.Fatal("diagnostic cause leaked")
		}
		if recorder.Header().Get(apptransport.HeaderApplicationErrorCode) != "billing.organization.balance_insufficient" {
			t.Fatalf("missing stable code header: %v", recorder.Header())
		}
	})

	t.Run("api key quota sets Retry-After", func(t *testing.T) {
		context, recorder := newT17Context("en-US")
		protocolErr := handler.localizedProtocolError(context, billingUserCause(gateway.BillingUserErrorKindAPIKeyQuotaInsufficient))
		writeOpenAIProtocolError(context, protocolErr)
		if recorder.Code != http.StatusTooManyRequests {
			t.Fatalf("status = %d, want 429", recorder.Code)
		}
		if recorder.Header().Get("Retry-After") == "" {
			t.Fatal("429 response is missing Retry-After")
		}
	})
}

// T17 acceptance 4/5: the streaming path never echoes diagnostics and keeps
// the legacy SSE error event name.
func TestT17StreamErrorKeepsEventNameAndSanitizes(t *testing.T) {
	handler := &LLMHandler{errorProjector: testApplicationErrorProjector(t)}
	context, recorder := newT17Context("en-US")
	writer, err := llmshared.NewStreamWriter(context)
	if err != nil {
		t.Fatal(err)
	}
	writer.WriteErrorMessage(handler.localizedChatStreamError(context, errors.New("upstream secret sk-live-123")))

	payload := recorder.Body.String()
	if !strings.Contains(payload, "event:error") {
		t.Fatalf("SSE event name changed: %q", payload)
	}
	if strings.Contains(payload, "sk-live-123") {
		t.Fatalf("stream error leaked diagnostics: %q", payload)
	}
}
