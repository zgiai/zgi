package integrations

import (
	"errors"
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
		},
	)

	got := ProviderDiagnosticsFromError(err)
	if got.ErrorCode != "99991672" || got.RequestID != "20260728ABC-123" || got.HTTPStatus != 403 {
		t.Fatalf("ProviderDiagnosticsFromError() = %#v", got)
	}
	if got.RetryAfterAt == nil || got.RetryAfterAt.Location() != time.UTC {
		t.Fatalf("RetryAfterAt = %#v, want normalized UTC timestamp", got.RetryAfterAt)
	}
	if strings.Contains(got.ErrorCode, "secret") || strings.Contains(got.RequestID, "secret") {
		t.Fatalf("safe diagnostics leaked provider body: %#v", got)
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
