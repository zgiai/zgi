package gateway

import (
	"errors"
	"testing"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// T17 acceptance 8: stable error_code and ErrorInfo.Reason decide the cause;
// free-text substrings are only a temporary compatibility path.
func TestT17BillingRPCErrorCodeMappingPrefersStableCode(t *testing.T) {
	cases := []struct {
		name      string
		errorCode string
		message   string
		target    error
	}{
		{"organization balance", "billing.organization.balance_insufficient", "", ErrInsufficientBalance},
		{"private channel balance", "billing.private_channel.balance_insufficient", "", ErrInsufficientBalance},
		{"workspace quota", "billing.workspace.quota_insufficient", "", ErrInsufficientQuota},
		{"api key quota", "llm.quota.exceeded", "", ErrInsufficientQuota},
		{"pricing not configured", "billing.model.pricing_not_configured", "", ErrPricingNotConfigured},
		{"prededuct failed", "billing.prededuct.failed", "", ErrBillingPreDeductFailed},
		{"deduction not found", "billing.deduction.not_found", "", ErrBalanceNotFound},
		{"request invalid", "billing.request.invalid", "", ErrInvalidRequest},
		{"legacy code", "INSUFFICIENT_CREDITS", "", ErrInsufficientBalance},
		{"legacy org not found", "ORG_NOT_FOUND", "", ErrBalanceNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mapped, ok := mapBillingRPCErrorCode(tc.errorCode, tc.message)
			if !ok || !errors.Is(mapped, tc.target) {
				t.Fatalf("mapBillingRPCErrorCode(%q, %q) = %v, %v; want %v", tc.errorCode, tc.message, mapped, ok, tc.target)
			}
		})
	}
}

// A stable error_code must win even when the free-text message contains a
// misleading substring from the compatibility vocabulary.
func TestT17StableCodeWinsOverMisleadingMessage(t *testing.T) {
	mapped, ok := mapBillingRPCErrorCode("billing.workspace.quota_insufficient", "insufficient credits for reservation")
	if !ok || !errors.Is(mapped, ErrInsufficientQuota) || errors.Is(mapped, ErrInsufficientBalance) {
		t.Fatalf("code-first mapping failed: %v, %v", mapped, ok)
	}
}

// T17 acceptance 8: the compatibility branch still classifies old Console
// messages and is explicitly marked for T21 removal.
func TestT17CompatibilityMessageBranch(t *testing.T) {
	mapped, ok := mapBillingRPCErrorCode("", "insufficient credits for reservation")
	if !ok || !errors.Is(mapped, ErrInsufficientBalance) {
		t.Fatalf("compat mapping = %v, %v", mapped, ok)
	}
	if _, ok := mapBillingRPCErrorCode("", "meters charged on a different plan"); ok {
		t.Fatal("unrelated message must stay unclassified")
	}
}

// T17 acceptance 8: the gRPC ErrorInfo.Reason is available to callers without
// parsing transport text.
func TestT17BillingRPCErrorCarriesErrorInfoReason(t *testing.T) {
	statusError := status.New(codes.ResourceExhausted, "resource exhausted")
	withDetails, err := statusError.WithDetails(&errdetails.ErrorInfo{
		Reason: "billing.organization.balance_insufficient",
		Domain: "zgi.billing",
	})
	if err != nil {
		t.Fatal(err)
	}
	wrapped := newBillingRPCError(withDetails.Err())
	if reason := billingRPCErrorReason(wrapped); reason != "billing.organization.balance_insufficient" {
		t.Fatalf("reason = %q", reason)
	}
	mapped, ok := mapBillingRPCErrorCode(billingRPCErrorReason(wrapped), "")
	if !ok || !errors.Is(mapped, ErrInsufficientBalance) {
		t.Fatalf("reason mapping = %v, %v", mapped, ok)
	}

	if reason := billingRPCErrorReason(errors.New("plain error")); reason != "" {
		t.Fatalf("plain error reported a reason: %q", reason)
	}
}
