package workflow

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/zgiai/ginext/internal/modules/llm/gateway"
)

func TestBuildWorkflowStreamErrorPayload_OrganizationBalanceInsufficient(t *testing.T) {
	err := errors.Join(
		errors.New("all providers failed"),
		&gateway.BillingUserError{
			Kind:  gateway.BillingUserErrorKindOrganizationBalanceInsufficient,
			Cause: gateway.ErrInsufficientBalance,
		},
	)

	payload := buildWorkflowStreamErrorPayload(err)

	if got := payload["code"]; got != 207011 {
		t.Fatalf("code = %#v, want %d", got, 207011)
	}
	if got := payload["message"]; got == nil || strings.Contains(got.(string), "all providers failed") {
		t.Fatalf("message = %#v, should not expose technical wrapper", got)
	}
	params, ok := payload["params"].(map[string]any)
	if !ok {
		t.Fatalf("params type = %T, want map[string]any", payload["params"])
	}
	if len(params) != 0 {
		t.Fatalf("params = %#v, want empty map", params)
	}
}

func TestBuildWorkflowStreamErrorPayload_FallsBackToRawMessage(t *testing.T) {
	payload := buildWorkflowStreamErrorPayload(errors.New("plain failure"))

	if _, exists := payload["code"]; exists {
		t.Fatalf("code should be absent, payload = %#v", payload)
	}
	if got := payload["message"]; got != "plain failure" {
		t.Fatalf("message = %#v, want %#v", got, "plain failure")
	}
}

func TestWrapNodeExecutionError_PreservesBillingUserErrorChain(t *testing.T) {
	testCases := []struct {
		name     string
		kind     gateway.BillingUserErrorKind
		cause    error
		wantCode int
	}{
		{
			name:     "organization balance insufficient",
			kind:     gateway.BillingUserErrorKindOrganizationBalanceInsufficient,
			cause:    gateway.ErrInsufficientBalance,
			wantCode: 207011,
		},
		{
			name:     "workspace quota insufficient",
			kind:     gateway.BillingUserErrorKindWorkspaceQuotaInsufficient,
			cause:    gateway.ErrInsufficientQuota,
			wantCode: 207012,
		},
		{
			name:     "private channel balance insufficient",
			kind:     gateway.BillingUserErrorKindPrivateChannelBalanceInsufficient,
			cause:    gateway.ErrInsufficientBalance,
			wantCode: 207013,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			originalErr := errors.Join(
				errors.New("all providers failed"),
				&gateway.BillingUserError{
					Kind:  tc.kind,
					Cause: tc.cause,
				},
			)

			wrappedErr := wrapNodeExecutionError("llm-node-1", originalErr)

			var userErr *gateway.BillingUserError
			if !errors.As(wrappedErr, &userErr) {
				t.Fatalf("errors.As(wrappedErr, *BillingUserError) = false, err = %v", wrappedErr)
			}
			if userErr.Kind != tc.kind {
				t.Fatalf("userErr.Kind = %q, want %q", userErr.Kind, tc.kind)
			}

			payload := buildWorkflowStreamErrorPayload(fmt.Errorf("node %s failed: %w", "llm-node-1", wrappedErr))
			if got := payload["code"]; got != tc.wantCode {
				t.Fatalf("payload code = %#v, want %d", got, tc.wantCode)
			}
			if got := payload["message"]; got == nil || strings.Contains(got.(string), "all providers failed") || strings.Contains(got.(string), "llm-node-1") {
				t.Fatalf("message = %#v, should hide technical wrapper", got)
			}
		})
	}
}
