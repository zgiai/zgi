package adapter

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestIsDeterministicRejectionUsesTypedSentinelsOnly(t *testing.T) {
	for _, err := range []error{
		ErrInvalidRequest,
		fmt.Errorf("provider rejected request: %w", ErrContentPolicyViolation),
		ErrCapabilityUnsupported,
	} {
		if !IsDeterministicRejection(err) {
			t.Fatalf("IsDeterministicRejection(%v) = false", err)
		}
	}

	if IsDeterministicRejection(errors.New("upstream operation unsupported during maintenance")) {
		t.Fatal("untyped provider text must not suppress an operational failure")
	}
}

// TestHandleNonJSONErrorBranchesOnStatus pins the plain-text error path. A
// non-JSON body used to collapse into PARSE_ERROR/ErrUpstreamError regardless of
// status, so an upstream 401 surfaced as a retryable 502 invocation failure
// instead of llm.provider.auth_failed.
func TestHandleNonJSONErrorBranchesOnStatus(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		wantCode string
		wantErr  error
	}{
		{"unauthorized text", http.StatusUnauthorized, "invalid api key", "UNAUTHORIZED", ErrAuthFailed},
		{"forbidden text", http.StatusForbidden, "access denied", "FORBIDDEN", ErrAuthFailed},
		{"not found text", http.StatusNotFound, "no such model", "NOT_FOUND", ErrModelNotFound},
		{"bad request text", http.StatusBadRequest, "malformed", "BAD_REQUEST", ErrInvalidRequest},
		{"rate limited text", http.StatusTooManyRequests, "slow down", "RATE_LIMITED", ErrRateLimited},
		{"gateway timeout text", http.StatusGatewayTimeout, "timed out", "UPSTREAM_TIMEOUT", ErrTimeout},
		{"server error text", http.StatusBadGateway, "bad gateway", "PARSE_ERROR", ErrUpstreamError},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := HandleNonJSONError(testCase.status, []byte(testCase.body))
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("HandleNonJSONError() error = %v, want %v", err, testCase.wantErr)
			}
			if err.Code != testCase.wantCode {
				t.Fatalf("code = %q, want %q", err.Code, testCase.wantCode)
			}
			if err.StatusCode != testCase.status {
				t.Fatalf("status = %d, want %d", err.StatusCode, testCase.status)
			}
		})
	}
}
