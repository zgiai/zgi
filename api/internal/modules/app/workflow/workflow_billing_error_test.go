package workflow

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"

	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/observability"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestWorkflowNodeFailureReportPreservesOwnership(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		source    observability.ErrorSource
		category  observability.ErrorCategory
		code      string
		level     observability.Level
		retryable bool
	}{
		{
			name:      "private provider unavailable",
			err:       fmt.Errorf("node failed: %w", llmerrors.DomainErrPrivateChannelUpstreamUnavailable),
			source:    observability.ErrorSourceProvider,
			category:  observability.ErrorCategoryDependency,
			code:      "private_channel_upstream_unavailable",
			level:     observability.LevelError,
			retryable: true,
		},
		{
			name: "database transport outage during provider selection",
			err: gateway.NewProviderSelectionConversionError(
				fmt.Errorf("load gateway route: %w", &net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset")}),
			),
			source:    observability.ErrorSourceInfrastructure,
			category:  observability.ErrorCategoryDatabase,
			code:      "database_transport_failed",
			level:     observability.LevelError,
			retryable: true,
		},
		{
			name:      "database timeout during provider selection",
			err:       gateway.NewProviderSelectionConversionError(context.DeadlineExceeded),
			source:    observability.ErrorSourceInfrastructure,
			category:  observability.ErrorCategoryTimeout,
			code:      "database_timeout",
			level:     observability.LevelError,
			retryable: true,
		},
		{
			name:      "workflow persistence timeout",
			err:       fmt.Errorf("%w: %w", errWorkflowEventPersistenceFailed, context.DeadlineExceeded),
			source:    observability.ErrorSourceInfrastructure,
			category:  observability.ErrorCategoryTimeout,
			code:      "database_timeout",
			level:     observability.LevelError,
			retryable: true,
		},
		{
			name:      "provider context deadline is not database",
			err:       fmt.Errorf("invoke provider: %w", context.DeadlineExceeded),
			source:    observability.ErrorSourceProvider,
			category:  observability.ErrorCategoryTimeout,
			code:      "upstream_timeout",
			level:     observability.LevelError,
			retryable: true,
		},
		{
			name: "provider transport is not database",
			err: errors.Join(
				adapter.ErrUpstreamError,
				&net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset")},
			),
			source:    observability.ErrorSourceProvider,
			category:  observability.ErrorCategoryDependency,
			code:      "upstream_unavailable",
			level:     observability.LevelError,
			retryable: true,
		},
		{
			name:      "provider rate limited",
			err:       fmt.Errorf("failed to invoke LLM: %w", adapter.ErrRateLimited),
			source:    observability.ErrorSourceProvider,
			category:  observability.ErrorCategoryDependency,
			code:      "upstream_rate_limited",
			level:     observability.LevelError,
			retryable: true,
		},
		{
			name:      "provider proxy failure",
			err:       fmt.Errorf("proxy returned bad gateway: %w", adapter.ErrProxyError),
			source:    observability.ErrorSourceProvider,
			category:  observability.ErrorCategoryDependency,
			code:      "upstream_unavailable",
			level:     observability.LevelError,
			retryable: true,
		},
		{
			name:     "tenant rate limited",
			err:      fmt.Errorf("failed to invoke LLM: %w", llmerrors.DomainErrRateLimitExceeded),
			source:   observability.ErrorSourceTenant,
			category: observability.ErrorCategoryConfiguration,
			code:     "rate_limit_exceeded",
			level:    observability.LevelWarning,
		},
		{
			name:      "provider timeout",
			err:       fmt.Errorf("failed to invoke LLM: %w", llmerrors.DomainErrUpstreamTimeout),
			source:    observability.ErrorSourceProvider,
			category:  observability.ErrorCategoryTimeout,
			code:      "upstream_timeout",
			level:     observability.LevelError,
			retryable: true,
		},
		{
			name:     "content policy rejection",
			err:      fmt.Errorf("failed to invoke LLM: %w", adapter.ErrContentPolicyViolation),
			source:   observability.ErrorSourceTenant,
			category: observability.ErrorCategoryConfiguration,
			code:     "llm_request_rejected",
			level:    observability.LevelWarning,
		},
		{
			name:     "no provider available",
			err:      fmt.Errorf("failed to invoke LLM: %w", gateway.NewNoProviderAvailableError("qwen-plus", "org")),
			source:   observability.ErrorSourceTenant,
			category: observability.ErrorCategoryConfiguration,
			code:     "no_provider_available",
			level:    observability.LevelWarning,
		},
		{
			name:     "model route missing",
			err:      fmt.Errorf("failed to invoke LLM: %w", llmerrors.DomainErrRouteNotFound),
			source:   observability.ErrorSourceTenant,
			category: observability.ErrorCategoryConfiguration,
			code:     "model_route_not_configured",
			level:    observability.LevelWarning,
		},
		{
			name: "workspace quota",
			err: &gateway.BillingUserError{
				Kind: gateway.BillingUserErrorKindWorkspaceQuotaInsufficient,
			},
			source:   observability.ErrorSourceTenant,
			category: observability.ErrorCategoryConfiguration,
			code:     string(gateway.BillingUserErrorKindWorkspaceQuotaInsufficient),
			level:    observability.LevelWarning,
		},
		{
			name:     "internal node failure",
			err:      errors.New("executor invariant failed"),
			source:   observability.ErrorSourceZGI,
			category: observability.ErrorCategoryApplication,
			code:     "workflow_node_failed",
			level:    observability.LevelError,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			classification, level := workflowNodeFailureReport(test.err)
			if classification.Source != test.source || classification.Category != test.category || classification.Code != test.code || classification.Retryable != test.retryable || level != test.level {
				t.Fatalf("classification/level = %#v/%q", classification, level)
			}
		})
	}
}

func TestShouldReportWorkflowNodeFailureSkipsChannelOwnedCredentialEvents(t *testing.T) {
	for _, err := range []error{adapter.ErrAuthFailed, adapter.ErrInvalidConfig, adapter.ErrBillingUnavailable} {
		if shouldReportWorkflowNodeFailure(fmt.Errorf("failed to invoke LLM: %w", err)) {
			t.Fatalf("shouldReportWorkflowNodeFailure(%v) = true, want Gateway-owned event only", err)
		}
	}
	if !shouldReportWorkflowNodeFailure(adapter.ErrUpstreamError) {
		t.Fatal("ordinary upstream failure should retain workflow report")
	}
}

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

func TestBuildWorkflowStreamErrorPayload_PrivateChannelUpstreamUnavailable(t *testing.T) {
	err := fmt.Errorf("node llm-node-1 failed: failed to invoke LLM: %w", llmerrors.DomainErrPrivateChannelUpstreamUnavailable)

	payload := buildWorkflowStreamErrorPayload(err)

	if got := payload["code"]; got != response.ErrWorkflowPrivateChannelUpstreamUnavailable.Code {
		t.Fatalf("code = %#v, want %d", got, response.ErrWorkflowPrivateChannelUpstreamUnavailable.Code)
	}
	if got := payload["message"]; got != response.ErrWorkflowPrivateChannelUpstreamUnavailable.Message {
		t.Fatalf("message = %#v, want %#v", got, response.ErrWorkflowPrivateChannelUpstreamUnavailable.Message)
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
