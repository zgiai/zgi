package gateway

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/google/uuid"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/observability"
)

type gatewayObservabilityRecorder struct {
	events []observability.Event
}

func (*gatewayObservabilityRecorder) Name() string { return "gateway-test" }

func (r *gatewayObservabilityRecorder) Report(_ context.Context, event observability.Event) error {
	r.events = append(r.events, event)
	return nil
}

func (*gatewayObservabilityRecorder) Flush(context.Context) error { return nil }

func withGatewayObservabilityRecorder(t *testing.T) *gatewayObservabilityRecorder {
	t.Helper()
	previous := observability.DefaultReporter()
	recorder := &gatewayObservabilityRecorder{}
	observability.SetDefaultReporter(observability.NewZGIReporter(recorder))
	t.Cleanup(func() { observability.SetDefaultReporter(previous) })
	return recorder
}

func TestReportLLMSelectionFailureClassifiesMissingRouteAsConfiguration(t *testing.T) {
	recorder := withGatewayObservabilityRecorder(t)
	router := &ChannelRouter{}
	_, routeErr := router.candidateRoutesForResolvedModel(context.Background(), uuid.New(), "qwen-plus", "", 3, nil, nil, nil)
	if !errors.Is(routeErr, llmerrors.DomainErrRouteNotFound) {
		t.Fatalf("route error = %v, want DomainErrRouteNotFound", routeErr)
	}
	reportLLMSelectionFailure(context.Background(), routeErr, "qwen-plus", "org", "shadow")

	if len(recorder.events) != 1 {
		t.Fatalf("events = %d, want 1", len(recorder.events))
	}
	event := recorder.events[0]
	if event.Name != "llm.route.not_configured" || event.Level != observability.LevelWarning {
		t.Fatalf("event = %#v", event)
	}
	if event.Tags["error.category"] != "configuration" || event.Tags["error.source"] != "tenant" || event.Tags["error.code"] != "model_route_not_configured" {
		t.Fatalf("classification tags = %#v", event.Tags)
	}
}

func TestReportLLMSelectionFailureClassifiesGuardedPrivateRoutesAsProviderOutage(t *testing.T) {
	recorder := withGatewayObservabilityRecorder(t)
	reportLLMSelectionFailure(
		context.Background(),
		fmt.Errorf("all private routes guarded: %w", llmerrors.DomainErrPrivateChannelUpstreamUnavailable),
		"qwen-plus",
		"org",
		"shadow",
	)

	if len(recorder.events) != 1 {
		t.Fatalf("events = %d, want 1", len(recorder.events))
	}
	event := recorder.events[0]
	if event.Name != "llm.provider.unavailable" || event.Level != observability.LevelError {
		t.Fatalf("event = %#v", event)
	}
	if event.Tags["error.category"] != "dependency" || event.Tags["error.source"] != "provider" || event.Tags["error.code"] != "private_channel_upstream_unavailable" || event.Tags["error.retryable"] != "true" {
		t.Fatalf("classification tags = %#v", event.Tags)
	}
}

func TestReportLLMSelectionFailureClassifiesNoProviderAsTenantConfiguration(t *testing.T) {
	recorder := withGatewayObservabilityRecorder(t)
	reportLLMSelectionFailure(
		context.Background(),
		NewNoProviderAvailableError("qwen-plus", "org"),
		"qwen-plus",
		"org",
		"shadow",
	)

	if len(recorder.events) != 1 {
		t.Fatalf("events = %d, want 1", len(recorder.events))
	}
	event := recorder.events[0]
	if event.Name != "llm.provider.unavailable" || event.Level != observability.LevelWarning || event.Tags["error.source"] != "tenant" || event.Tags["error.code"] != "no_provider_available" {
		t.Fatalf("event = %#v", event)
	}
}

func TestReportLLMSelectionFailurePreservesDatabaseTransportOwnership(t *testing.T) {
	recorder := withGatewayObservabilityRecorder(t)
	reportLLMSelectionFailure(
		context.Background(),
		fmt.Errorf("load routes: %w", &net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset")}),
		"qwen-plus",
		"org",
		"shadow",
	)

	if len(recorder.events) != 1 {
		t.Fatalf("events = %d, want 1", len(recorder.events))
	}
	event := recorder.events[0]
	if event.Tags["error.category"] != "database" || event.Tags["error.source"] != "infrastructure" || event.Tags["error.code"] != "database_transport_failed" || event.Tags["error.retryable"] != "true" {
		t.Fatalf("classification tags = %#v", event.Tags)
	}
}

func TestReportedNoProviderAvailableErrorEmitsTenantDiagnostic(t *testing.T) {
	recorder := withGatewayObservabilityRecorder(t)
	err := reportedNoProviderAvailableError(context.Background(), "qwen-plus", "org", "shadow")

	if !errors.Is(err, ErrNoProviderAvailable) {
		t.Fatalf("error = %v, want ErrNoProviderAvailable", err)
	}
	if len(recorder.events) != 1 {
		t.Fatalf("events = %d, want 1", len(recorder.events))
	}
	event := recorder.events[0]
	if event.Name != "llm.provider.unavailable" || event.Level != observability.LevelWarning || event.Tags["error.source"] != "tenant" || event.Tags["error.category"] != "configuration" {
		t.Fatalf("event = %#v", event)
	}
}

func TestReportLLMProviderFailureClassifiesTimeout(t *testing.T) {
	recorder := withGatewayObservabilityRecorder(t)
	reportLLMProviderFailure(context.Background(), context.DeadlineExceeded, "llm.provider.stream_failed", "qwen", "qwen3-max", "org", 0, nil, true, true)

	if len(recorder.events) != 1 {
		t.Fatalf("events = %d, want 1", len(recorder.events))
	}
	event := recorder.events[0]
	if event.Tags["error.category"] != "timeout" || event.Tags["error.source"] != "provider" || event.Tags["error.retryable"] != "true" {
		t.Fatalf("classification tags = %#v", event.Tags)
	}
}

func TestReportLLMSelectionFailureClassifiesUnsupportedCapabilityAsTenantConfiguration(t *testing.T) {
	recorder := withGatewayObservabilityRecorder(t)
	reportLLMSelectionFailure(context.Background(), adapter.ErrCapabilityUnsupported, "gpt-5", "org", "shadow-org")

	if len(recorder.events) != 1 {
		t.Fatalf("events = %#v, want one routing warning", recorder.events)
	}
	event := recorder.events[0]
	if event.Level != observability.LevelWarning || event.Tags["error.source"] != "tenant" || event.Tags["error.code"] != "model_capability_not_configured" {
		t.Fatalf("event = %#v", event)
	}
}

func TestReportLLMProviderFailureSkipsDeterministicClientRejections(t *testing.T) {
	for _, rejection := range []error{
		adapter.ErrInvalidRequest,
		adapter.ErrContentPolicyViolation,
		adapter.ErrCapabilityUnsupported,
	} {
		t.Run(rejection.Error(), func(t *testing.T) {
			recorder := withGatewayObservabilityRecorder(t)
			reportLLMProviderFailure(context.Background(), rejection, "llm.provider.request_failed", "qwen", "qwen3-max", "org", 0, nil, true, true)
			if len(recorder.events) != 0 {
				t.Fatalf("events = %#v, want deterministic rejection omitted", recorder.events)
			}
		})
	}
}

func TestReportLLMProviderFailureDoesNotSuppressUntypedUnsupportedMessage(t *testing.T) {
	recorder := withGatewayObservabilityRecorder(t)
	reportLLMProviderFailure(
		context.Background(),
		errors.New("upstream operation not supported during maintenance"),
		"llm.provider.request_failed",
		"openai",
		"gpt-5",
		"org",
		0,
		nil,
		true,
		true,
	)

	if len(recorder.events) != 1 || recorder.events[0].Name != "llm.provider.request_failed" {
		t.Fatalf("events = %#v, want untyped upstream failure reported", recorder.events)
	}
}

func TestReportLLMProviderFailureClassifiesPersistentChannelFailuresByOwner(t *testing.T) {
	tests := []struct {
		name              string
		err               error
		useSystemProvider bool
		level             observability.Level
		source            string
		code              string
	}{
		{name: "private credentials", err: adapter.ErrAuthFailed, level: observability.LevelWarning, source: "tenant", code: "private_channel_credentials_invalid"},
		{name: "private billing", err: adapter.ErrBillingUnavailable, level: observability.LevelWarning, source: "tenant", code: "private_channel_billing_unavailable"},
		{name: "system credentials", err: adapter.ErrAuthFailed, useSystemProvider: true, level: observability.LevelError, source: "zgi", code: "system_channel_credentials_invalid"},
		{name: "system billing", err: adapter.ErrQuotaExhausted, useSystemProvider: true, level: observability.LevelError, source: "zgi", code: "system_channel_billing_unavailable"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := withGatewayObservabilityRecorder(t)
			reportLLMProviderFailure(context.Background(), test.err, "llm.provider.request_failed", "openai", "gpt-5", "org", 0, nil, test.useSystemProvider, true)

			if len(recorder.events) != 1 {
				t.Fatalf("events = %#v, want one event", recorder.events)
			}
			event := recorder.events[0]
			if event.Level != test.level || event.Tags["error.category"] != "configuration" || event.Tags["error.source"] != test.source || event.Tags["error.code"] != test.code || event.Tags["error.retryable"] != "false" {
				t.Fatalf("event = %#v", event)
			}
		})
	}
}

func TestReportLLMAdapterFailureClassifiesPrivateConfigurationByOwner(t *testing.T) {
	recorder := withGatewayObservabilityRecorder(t)
	reportLLMAdapterFailure(context.Background(), adapter.ErrInvalidConfig, "openai", "gpt-5", "org", 0, nil, false, true)

	if len(recorder.events) != 1 {
		t.Fatalf("events = %#v, want one event", recorder.events)
	}
	event := recorder.events[0]
	if event.Level != observability.LevelWarning || event.Tags["error.source"] != "tenant" || event.Tags["error.code"] != "private_channel_credentials_invalid" || event.Tags["error.retryable"] != "false" {
		t.Fatalf("event = %#v", event)
	}
}

func TestProviderAttemptLevelOnlyErrorsOnFinalFailure(t *testing.T) {
	if got := providerAttemptLevel(false); got != observability.LevelWarning {
		t.Fatalf("non-final attempt level = %q", got)
	}
	if got := providerAttemptLevel(true); got != observability.LevelError {
		t.Fatalf("final attempt level = %q", got)
	}
}
