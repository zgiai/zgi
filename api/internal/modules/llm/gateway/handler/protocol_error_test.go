package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestProtocolErrorEnvelopeIncludesCatalogFieldsForLegacyBranches(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "content policy", err: adapter.ErrContentPolicyViolation},
		{name: "unsupported protocol", err: adapter.ErrCapabilityUnsupported},
		{name: "platform unavailable", err: adapter.ErrPlatformChannelUnavailable},
		{name: "internal", err: gateway.ErrBillingLaneMismatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			ctx.Request.Header.Set("X-Request-ID", "req-envelope-legacy")
			ctx.Set("request_id", "req-envelope-legacy")
			writeOpenAIProtocolError(ctx, classifyProtocolError(tc.err))

			var body map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			for _, field := range []string{"code", "error_code", "message", "retryable", "error_id", "request_id", "params"} {
				if _, ok := body[field]; !ok {
					t.Fatalf("missing top-level %s in %s: %s", field, tc.name, recorder.Body.String())
				}
			}
		})
	}
}

func TestClassifyProtocolError(t *testing.T) {
	tests := []struct {
		name              string
		err               error
		wantStatus        int
		wantCode          string
		wantAnthropicType string
		// wantAppCode pins the cataloged identity when the row is about it.
		wantAppCode string
	}{
		{
			name:              "model authorization",
			err:               gateway.ErrModelNotAuthorized,
			wantStatus:        http.StatusForbidden,
			wantCode:          "model_not_authorized",
			wantAnthropicType: "permission_error",
			wantAppCode:       "llm.model.forbidden",
		},
		{
			name:              "model not found",
			err:               llmerrors.NewModelNotFoundErrorWithName("missing-model"),
			wantStatus:        http.StatusNotFound,
			wantCode:          "model_not_found",
			wantAnthropicType: "not_found_error",
		},
		{
			name:              "protocol unsupported",
			err:               errors.Join(adapter.ErrCapabilityUnsupported, errors.New("responses")),
			wantStatus:        http.StatusBadRequest,
			wantCode:          "unsupported_protocol",
			wantAnthropicType: "invalid_request_error",
		},
		{
			name:              "invalid max tokens",
			err:               llmerrors.DomainErrInvalidMaxTokens,
			wantStatus:        http.StatusBadRequest,
			wantCode:          "invalid_request",
			wantAnthropicType: "invalid_request_error",
		},
		{
			name:              "content policy",
			err:               adapter.ErrContentPolicyViolation,
			wantStatus:        http.StatusBadRequest,
			wantCode:          "content_policy_violation",
			wantAnthropicType: "invalid_request_error",
		},
		{
			name:              "provider quota exhausted",
			err:               adapter.NewAdapterError("insufficient_quota", "insufficient API quota", http.StatusTooManyRequests, adapter.ErrQuotaExhausted),
			wantStatus:        http.StatusTooManyRequests,
			wantCode:          "insufficient_quota",
			wantAnthropicType: "billing_error",
		},
		{
			name:              "channel not found has its own branch",
			err:               llmerrors.DomainErrChannelNotFound,
			wantStatus:        http.StatusNotFound,
			wantCode:          "llm.channel.not_found",
			wantAnthropicType: "not_found_error",
		},
		{
			name:              "platform channel unavailable",
			err:               adapter.ErrPlatformChannelUnavailable,
			wantStatus:        http.StatusServiceUnavailable,
			wantCode:          adapter.ErrorCodePlatformChannelUnavailable,
			wantAnthropicType: "api_error",
			wantAppCode:       "llm.provider.unavailable",
		},
		{
			name:              "billing lane mismatch",
			err:               gateway.ErrBillingLaneMismatch,
			wantStatus:        http.StatusBadGateway, // llm.invocation.failed is 502 in the frozen catalog,
			wantCode:          "internal_error",
			wantAnthropicType: "api_error",
		},
		{
			name:              "nil error is internal",
			err:               nil,
			wantStatus:        http.StatusBadGateway, // llm.invocation.failed is 502 in the frozen catalog,
			wantCode:          "internal_error",
			wantAnthropicType: "api_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyProtocolError(tt.err)
			if got.openAIStatus != tt.wantStatus {
				t.Fatalf("openAIStatus = %d, want %d", got.openAIStatus, tt.wantStatus)
			}
			if got.openAICode != tt.wantCode {
				t.Fatalf("openAICode = %q, want %q", got.openAICode, tt.wantCode)
			}
			if got.anthropicType != tt.wantAnthropicType {
				t.Fatalf("anthropicType = %q, want %q", got.anthropicType, tt.wantAnthropicType)
			}
			if tt.wantAppCode != "" && got.appCode != tt.wantAppCode {
				t.Fatalf("appCode = %q, want %q", got.appCode, tt.wantAppCode)
			}
		})
	}
}
