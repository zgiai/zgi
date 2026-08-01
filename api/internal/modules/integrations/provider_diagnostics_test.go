package integrations

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestProviderDiagnosticsFromErrorReturnsOnlyStructuredMetadata(t *testing.T) {
	retryAfter := time.Date(2026, 7, 28, 12, 30, 0, 0, time.FixedZone("test", 8*60*60))
	err := NewProviderError(
		ErrorCodeAccessDenied,
		"provider request was denied",
		errors.New(`provider body: {"msg":"secret response"}`),
		ProviderDiagnostics{
			ErrorCode:    "99991672",
			RequestID:    "20260728ABC-123",
			HTTPStatus:   403,
			RetryAfterAt: &retryAfter,
			InvalidField: "page_size",
		},
	)

	got := ProviderDiagnosticsFromError(err)
	if got.ErrorCode != "99991672" || got.RequestID != "20260728ABC-123" || got.HTTPStatus != 403 {
		t.Fatalf("ProviderDiagnosticsFromError() = %#v", got)
	}
	if got.RetryAfterAt == nil || got.RetryAfterAt.Location() != time.UTC {
		t.Fatalf("RetryAfterAt = %#v, want normalized UTC timestamp", got.RetryAfterAt)
	}
	if got.InvalidField != "page_size" {
		t.Fatalf("InvalidField = %#v, want safe field", got.InvalidField)
	}
	if strings.Contains(got.ErrorCode, "secret") || strings.Contains(got.RequestID, "secret") {
		t.Fatalf("safe diagnostics leaked provider body: %#v", got)
	}
}

func TestProviderValidationRecoveryIsStructuralAndValueFree(t *testing.T) {
	err := NewProviderError(
		ErrorCodeInvalidInput,
		"provider rejected a secret field value",
		errors.New("raw provider body must remain private"),
		ProviderDiagnostics{
			ErrorCode:    "99992402",
			RequestID:    "request-1",
			HTTPStatus:   400,
			InvalidField: "page_size",
		},
	)
	recoveryProvider, ok := err.(interface{ PublicErrorRecovery() map[string]interface{} })
	if !ok {
		t.Fatal("provider error does not expose safe recovery")
	}
	recovery := recoveryProvider.PublicErrorRecovery()
	if recovery["reason_code"] != "provider_field_validation_failed" ||
		recovery["provider_error_code"] != "99992402" || recovery["provider_request_sent"] != true {
		t.Fatalf("recovery = %#v", recovery)
	}
	fields, ok := recovery["invalid_fields"].([]string)
	if !ok || len(fields) != 1 || fields[0] != "page_size" {
		t.Fatalf("invalid_fields = %#v", recovery["invalid_fields"])
	}
	for _, forbidden := range []string{"raw", "body", "secret", "request_id"} {
		for key, value := range recovery {
			if strings.Contains(strings.ToLower(key), forbidden) || strings.Contains(strings.ToLower(fmt.Sprint(value)), forbidden) {
				t.Fatalf("recovery leaked %q: %#v", forbidden, recovery)
			}
		}
	}
}

func TestProviderDiagnosticsRejectsMessagesBodiesAndInvalidStatuses(t *testing.T) {
	err := NewProviderError(
		ErrorCodeUpstream,
		"provider unavailable",
		nil,
		ProviderDiagnostics{
			ErrorCode:  `{"code":500,"message":"do not persist"}`,
			RequestID:  "request id with spaces and body text",
			HTTPStatus: 42,
		},
	)

	got := ProviderDiagnosticsFromError(err)
	if got.ErrorCode != "" || got.RequestID != "" || got.HTTPStatus != 0 {
		t.Fatalf("unsafe diagnostics were retained: %#v", got)
	}
}

func TestProviderDiagnosticsRejectsCredentialShapedValues(t *testing.T) {
	tests := []ProviderDiagnostics{
		{
			ErrorCode: "github_pat_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			RequestID: "safe-request-id",
		},
		{
			ErrorCode: "safe_error",
			RequestID: "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJzZWNyZXQifQ.signaturepart",
		},
		{
			ErrorCode: "safe_error",
			RequestID: "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890",
		},
	}
	for _, diagnostics := range tests {
		got := normalizeProviderDiagnostics(diagnostics)
		if got.ErrorCode != "" && got.RequestID != "" {
			t.Fatalf("credential-shaped diagnostics survived normalization: %#v", got)
		}
	}
}

func TestProviderDiagnosticsSurviveWrappedIntegrationError(t *testing.T) {
	err := NewProviderError(
		ErrorCodeRateLimited,
		"provider rate limited",
		nil,
		ProviderDiagnostics{ErrorCode: "rate_limit_exceeded", HTTPStatus: 429},
	)
	wrapped := NewError(
		ErrorCodeUpstream,
		"adapter invocation failed",
		errors.Join(errors.New("transport wrapper"), err),
	)
	got := ProviderDiagnosticsFromError(wrapped)
	if got.ErrorCode != "rate_limit_exceeded" || got.HTTPStatus != 429 {
		t.Fatalf("ProviderDiagnosticsFromError(wrapped) = %#v", got)
	}
}
