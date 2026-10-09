package service

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestBuildStreamErrorPayloadDoesNotGuessPrivateBalanceFromProviderError(t *testing.T) {
	prepared := streamErrorTestPrepared()
	err := fmt.Errorf(
		"all providers failed: %w",
		adapter.NewAdapterError("insufficient_quota", "account credits exhausted", 402, adapter.ErrInsufficientBalance),
	)

	payload := BuildStreamErrorPayload(prepared, err, "")

	if got := payload["code"]; got != response.ErrSystemError.Code {
		t.Fatalf("stream error code = %#v, want generic code %d", got, response.ErrSystemError.Code)
	}
	if got := payload["message"]; got != response.ErrSystemError.Message {
		t.Fatalf("stream error message = %#v, want generic message %q", got, response.ErrSystemError.Message)
	}
}

func TestBuildStreamErrorPayloadMapsTypedUpstreamErrorWithoutGuessingPrivateBalance(t *testing.T) {
	prepared := streamErrorTestPrepared()
	err := fmt.Errorf(
		"all providers failed: provider stream call failed: %w",
		adapter.NewAdapterError("", "Insufficient Balance", 402, adapter.ErrUpstreamError),
	)

	payload := BuildStreamErrorPayload(prepared, err, "")

	if got := payload["code"]; got != aichatErrorCodeModelServiceUnavailable {
		t.Fatalf("stream error code = %#v, want %q", got, aichatErrorCodeModelServiceUnavailable)
	}
	if got := payload["code"]; got == response.ErrWorkflowPrivateChannelBalanceInsufficient.Code {
		t.Fatalf("stream error code = %#v, must not guess private-channel balance from text", got)
	}
	if got := payload["message"]; got != aichatModelServiceUnavailableMessage {
		t.Fatalf("stream error message = %#v, want %q", got, aichatModelServiceUnavailableMessage)
	}
	if got := payload["error_code"]; got != "llm.provider.unavailable" {
		t.Fatalf("stable stream error code = %#v", got)
	}
	if payload["error_id"] == "" || payload["params"] == nil {
		t.Fatalf("stream error metadata missing: %#v", payload)
	}
}

func TestBuildStreamErrorPayloadMapsTypedPrivateChannelBalance(t *testing.T) {
	prepared := streamErrorTestPrepared()
	err := fmt.Errorf("billing failed: %w", &gateway.BillingUserError{
		Kind:  gateway.BillingUserErrorKindPrivateChannelBalanceInsufficient,
		Cause: adapter.ErrInsufficientBalance,
	})

	payload := BuildStreamErrorPayload(prepared, err, "")

	if got := payload["code"]; got != response.ErrWorkflowPrivateChannelBalanceInsufficient.Code {
		t.Fatalf("stream error code = %#v, want %d", got, response.ErrWorkflowPrivateChannelBalanceInsufficient.Code)
	}
	if got := payload["message"]; got != response.ErrWorkflowPrivateChannelBalanceInsufficient.Message {
		t.Fatalf("stream error message = %#v, want %#v", got, response.ErrWorkflowPrivateChannelBalanceInsufficient.Message)
	}
}

func TestBuildStreamErrorPayloadMapsPlatformChannelUnavailable(t *testing.T) {
	prepared := streamErrorTestPrepared()
	err := fmt.Errorf(
		"all providers failed: %w",
		adapter.NewAdapterError("platform_channel_unavailable", "Platform model service is temporarily unavailable", 502, adapter.ErrPlatformChannelUnavailable),
	)

	payload := BuildStreamErrorPayload(prepared, err, "")

	if got := payload["code"]; got != response.ErrWorkflowPlatformChannelUnavailable.Code {
		t.Fatalf("stream error code = %#v, want %d", got, response.ErrWorkflowPlatformChannelUnavailable.Code)
	}
	if got := payload["message"]; got != response.ErrWorkflowPlatformChannelUnavailable.Message {
		t.Fatalf("stream error message = %#v, want %#v", got, response.ErrWorkflowPlatformChannelUnavailable.Message)
	}
}

func TestBuildStreamErrorPayloadMapsPrivateChannelUpstreamUnavailable(t *testing.T) {
	prepared := streamErrorTestPrepared()
	err := fmt.Errorf("failed to select provider: %w", llmerrors.DomainErrPrivateChannelUpstreamUnavailable)

	payload := BuildStreamErrorPayload(prepared, err, "")

	if got := payload["code"]; got != response.ErrWorkflowPrivateChannelUpstreamUnavailable.Code {
		t.Fatalf("stream error code = %#v, want %d", got, response.ErrWorkflowPrivateChannelUpstreamUnavailable.Code)
	}
	if got := payload["message"]; got != response.ErrWorkflowPrivateChannelUpstreamUnavailable.Message {
		t.Fatalf("stream error message = %#v, want %#v", got, response.ErrWorkflowPrivateChannelUpstreamUnavailable.Message)
	}
}

func TestBuildStreamErrorPayloadSanitizesOrdinaryError(t *testing.T) {
	prepared := streamErrorTestPrepared()
	err := errors.New("redis password leaked by internal error")

	payload := BuildStreamErrorPayload(prepared, err, "")

	if got := payload["message"]; got != response.ErrSystemError.Message {
		t.Fatalf("stream error message = %#v, want %#v", got, response.ErrSystemError.Message)
	}
	if got := payload["code"]; got != response.ErrSystemError.Code {
		t.Fatalf("stream error code = %#v, want %d", got, response.ErrSystemError.Code)
	}
}

func TestBuildStreamErrorPayloadMapsFinalAnswerUnavailable(t *testing.T) {
	prepared := streamErrorTestPrepared()
	err := errors.Join(
		skillloop.ErrFinalAnswerUnavailable,
		errors.New("terminal-only model returned no final answer"),
	)

	payload := BuildStreamErrorPayload(prepared, err, "")

	if got := payload["code"]; got != aichatErrorCodeFinalAnswerUnavailable {
		t.Fatalf("stream error code = %#v, want %q", got, aichatErrorCodeFinalAnswerUnavailable)
	}
	if got := payload["message"]; got != aichatFinalAnswerUnavailableMessage {
		t.Fatalf("stream error message = %#v, want %q", got, aichatFinalAnswerUnavailableMessage)
	}
	if payload["params"] == nil {
		t.Fatalf("stream error params missing: %#v", payload)
	}
}

func TestBuildStreamErrorPayloadMapsModelIdleTimeout(t *testing.T) {
	prepared := streamErrorTestPrepared()

	payload := BuildStreamErrorPayload(prepared, ErrModelIdleTimeout, "")

	if got := payload["code"]; got != aichatErrorCodeModelServiceTimeout {
		t.Fatalf("stream error code = %#v, want %q", got, aichatErrorCodeModelServiceTimeout)
	}
	if got := payload["message"]; got != aichatModelServiceTimeoutMessage {
		t.Fatalf("stream error message = %#v, want %q", got, aichatModelServiceTimeoutMessage)
	}
}

func TestBuildStreamErrorPayloadMapsPlanningTermination(t *testing.T) {
	prepared := streamErrorTestPrepared()
	err := &skillloop.PlanningTerminationError{Reason: "content_filter"}

	payload := BuildStreamErrorPayload(prepared, err, "")

	if got := payload["code"]; got != aichatErrorCodeModelInvocationFailed {
		t.Fatalf("stream error code = %#v, want %q", got, aichatErrorCodeModelInvocationFailed)
	}
	if got := payload["message"]; got != aichatModelInvocationFailedMessage {
		t.Fatalf("stream error message = %#v, want %q", got, aichatModelInvocationFailedMessage)
	}
}

func TestBuildStreamErrorPayloadMapsPlanningOutputTruncation(t *testing.T) {
	prepared := streamErrorTestPrepared()
	err := fmt.Errorf(
		"planning_output_truncated: %w",
		&skillloop.PlanningTerminationError{Reason: "length", Recoverable: true},
	)

	payload := BuildStreamErrorPayload(prepared, err, "")

	if got := payload["code"]; got != aichatErrorCodePlanningOutputTruncated {
		t.Fatalf("stream error code = %#v, want %q", got, aichatErrorCodePlanningOutputTruncated)
	}
	if got := payload["message"]; got != aichatPlanningOutputTruncatedMessage {
		t.Fatalf("stream error message = %#v, want %q", got, aichatPlanningOutputTruncatedMessage)
	}
}

func TestBuildStreamErrorPayloadMapsNativeAgentOutputTruncation(t *testing.T) {
	prepared := streamErrorTestPrepared()
	err := fmt.Errorf("native round failed: %w", skillloop.ErrAgentOutputTruncated)

	payload := BuildStreamErrorPayload(prepared, err, "")

	if got := payload["code"]; got != aichatErrorCodeAgentOutputTruncated {
		t.Fatalf("stream error code = %#v, want %q", got, aichatErrorCodeAgentOutputTruncated)
	}
	if got := payload["message"]; got != aichatAgentOutputTruncatedMessage {
		t.Fatalf("stream error message = %#v, want %q", got, aichatAgentOutputTruncatedMessage)
	}
}

func TestBuildStreamErrorPayloadPreservesProviderCauseAfterFinalAnswerRetry(t *testing.T) {
	prepared := streamErrorTestPrepared()
	err := errors.Join(
		skillloop.ErrFinalAnswerUnavailable,
		adapter.NewAdapterError("upstream_error", "provider failed", 502, adapter.ErrUpstreamError),
	)

	payload := BuildStreamErrorPayload(prepared, err, "")

	if got := payload["code"]; got != aichatErrorCodeModelServiceUnavailable {
		t.Fatalf("stream error code = %#v, want %q", got, aichatErrorCodeModelServiceUnavailable)
	}
	if got := payload["message"]; got != aichatModelServiceUnavailableMessage {
		t.Fatalf("stream error message = %#v, want %q", got, aichatModelServiceUnavailableMessage)
	}
}

func TestBuildStreamErrorPayloadPreservesTypedBillingErrorOverFinalAnswerError(t *testing.T) {
	prepared := streamErrorTestPrepared()
	billingErr := &gateway.BillingUserError{
		Kind:  gateway.BillingUserErrorKindPrivateChannelBalanceInsufficient,
		Cause: adapter.ErrInsufficientBalance,
	}
	err := errors.Join(skillloop.ErrFinalAnswerUnavailable, billingErr)

	payload := BuildStreamErrorPayload(prepared, err, "")

	if got := payload["code"]; got != response.ErrWorkflowPrivateChannelBalanceInsufficient.Code {
		t.Fatalf("stream error code = %#v, want billing code %d", got, response.ErrWorkflowPrivateChannelBalanceInsufficient.Code)
	}
}

func TestPublicAichatStoredErrorMessageDoesNotGuessPrivateBalance(t *testing.T) {
	raw := "all providers failed: Insufficient Balance: upstream service error"

	got := publicAichatStoredErrorMessage(raw)

	if got != raw {
		t.Fatalf("stored error message = %q, want original error %q", got, raw)
	}
}

func TestPublicAichatStoredErrorMessageKeepsOrdinaryError(t *testing.T) {
	raw := "plain provider failure"

	got := publicAichatStoredErrorMessage(raw)

	if got != raw {
		t.Fatalf("stored error message = %q, want %q", got, raw)
	}
}

func streamErrorTestPrepared() *PreparedChat {
	return &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message:      &runtimemodel.Message{ID: uuid.New()},
	}
}

// The frozen catalog owns the retryable flag; the stream payload must copy it
// instead of guessing from the error text, and must carry the transport request
// identity when the caller has one.
func TestBuildStreamErrorPayloadPublishesCatalogRetryableAndRequestID(t *testing.T) {
	catalog, err := appcatalog.New(
		appcatalog.LocaleEnglishUS,
		appcatalog.CodeInternal,
		append(appcatalog.DefaultDefinitions(), llmerrors.CatalogDefinitions()...)...,
	)
	if err != nil {
		t.Fatalf("compose catalog: %v", err)
	}
	SetApplicationErrorCatalog(catalog)
	t.Cleanup(func() { SetApplicationErrorCatalog(nil) })

	prepared := streamErrorTestPrepared()
	streamErr := fmt.Errorf("provider stream call failed: %w", adapter.ErrUpstreamError)

	payload := BuildStreamErrorPayload(prepared, streamErr, "req-123")

	if got, ok := payload["retryable"]; !ok || got != true {
		t.Fatalf("retryable = %#v, want true from the catalog for llm.provider.unavailable", got)
	}
	if got := payload["request_id"]; got != "req-123" {
		t.Fatalf("request_id = %#v, want req-123", got)
	}

	// Without a published catalog the field is omitted rather than guessed, and
	// without a transport request id the field is omitted as well.
	SetApplicationErrorCatalog(nil)
	payload = BuildStreamErrorPayload(prepared, streamErr, "")
	if _, ok := payload["retryable"]; ok {
		t.Fatalf("retryable = %#v, want omitted when the catalog is unavailable", payload["retryable"])
	}
	if _, ok := payload["request_id"]; ok {
		t.Fatalf("request_id = %#v, want omitted without a transport request id", payload["request_id"])
	}
}

// An unknown model, and a model no route can serve, are published with their
// own cataloged codes. They used to fall through to llm.invocation.failed with
// retryable=true, while the skill loop already treats a missing model as not
// retryable.
func TestBuildStreamErrorPayloadMapsModelNotFoundAndNoProvider(t *testing.T) {
	catalog, err := appcatalog.New(
		appcatalog.LocaleEnglishUS,
		appcatalog.CodeInternal,
		append(appcatalog.DefaultDefinitions(), llmerrors.CatalogDefinitions()...)...,
	)
	if err != nil {
		t.Fatalf("compose catalog: %v", err)
	}
	SetApplicationErrorCatalog(catalog)
	t.Cleanup(func() { SetApplicationErrorCatalog(nil) })

	prepared := streamErrorTestPrepared()
	for _, tc := range []struct {
		name      string
		err       error
		code      string
		retryable bool
	}{
		{"domain model not found", fmt.Errorf("failed to select provider: %w", llmerrors.NewModelNotFoundErrorWithName("zzz-unknown-qa")), "llm.model.not_found", false},
		{"gateway model not found", fmt.Errorf("resolve: %w", gateway.ErrModelNotFound), "llm.model.not_found", false},
		{"adapter model not found", fmt.Errorf("upstream: %w", adapter.ErrModelNotFound), "llm.model.not_found", false},
		{"no provider", fmt.Errorf("select: %w", gateway.ErrNoProviderAvailable), "llm.provider.none_available", true},
		{"route not found", fmt.Errorf("select: %w", llmerrors.DomainErrRouteNotFound), "llm.provider.none_available", true},
	} {
		payload := BuildStreamErrorPayload(prepared, tc.err, "")
		if got := payload["error_code"]; got != tc.code {
			t.Fatalf("%s: error_code = %#v, want %s", tc.name, got, tc.code)
		}
		if got, ok := payload["retryable"]; !ok || got != tc.retryable {
			t.Fatalf("%s: retryable = %#v, want %t from the catalog", tc.name, got, tc.retryable)
		}
	}
}
