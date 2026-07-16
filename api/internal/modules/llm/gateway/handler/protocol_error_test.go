package handler

import (
	"errors"
	"net/http"
	"testing"

	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestClassifyProtocolError(t *testing.T) {
	tests := []struct {
		name              string
		err               error
		wantStatus        int
		wantCode          string
		wantAnthropicType string
	}{
		{
			name:              "model authorization",
			err:               gateway.ErrModelNotAuthorized,
			wantStatus:        http.StatusForbidden,
			wantCode:          "model_not_authorized",
			wantAnthropicType: "permission_error",
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
			name:              "provider resource unavailable",
			err:               llmerrors.DomainErrChannelNotFound,
			wantStatus:        http.StatusServiceUnavailable,
			wantCode:          "provider_unavailable",
			wantAnthropicType: "api_error",
		},
		{
			name:              "platform channel unavailable",
			err:               adapter.ErrPlatformChannelUnavailable,
			wantStatus:        http.StatusBadGateway,
			wantCode:          adapter.ErrorCodePlatformChannelUnavailable,
			wantAnthropicType: "api_error",
		},
		{
			name:              "billing lane mismatch",
			err:               gateway.ErrBillingLaneMismatch,
			wantStatus:        http.StatusInternalServerError,
			wantCode:          "internal_error",
			wantAnthropicType: "api_error",
		},
		{
			name:              "nil error is internal",
			err:               nil,
			wantStatus:        http.StatusInternalServerError,
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
		})
	}
}
