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
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestBuildStreamErrorPayloadDoesNotGuessPrivateBalanceFromProviderError(t *testing.T) {
	prepared := streamErrorTestPrepared()
	err := fmt.Errorf(
		"all providers failed: %w",
		adapter.NewAdapterError("insufficient_quota", "account credits exhausted", 402, adapter.ErrInsufficientBalance),
	)

	payload := BuildStreamErrorPayload(prepared, err)

	if _, ok := payload["code"]; ok {
		t.Fatalf("stream error code = %#v, want no guessed private-channel code", payload["code"])
	}
	if got := payload["message"]; got != err.Error() {
		t.Fatalf("stream error message = %#v, want original error %q", got, err.Error())
	}
}

func TestBuildStreamErrorPayloadMapsTypedUpstreamErrorWithoutGuessingPrivateBalance(t *testing.T) {
	prepared := streamErrorTestPrepared()
	err := fmt.Errorf(
		"all providers failed: provider stream call failed: %w",
		adapter.NewAdapterError("", "Insufficient Balance", 402, adapter.ErrUpstreamError),
	)

	payload := BuildStreamErrorPayload(prepared, err)

	if got := payload["code"]; got != aichatErrorCodeModelServiceUnavailable {
		t.Fatalf("stream error code = %#v, want %q", got, aichatErrorCodeModelServiceUnavailable)
	}
	if got := payload["code"]; got == response.ErrWorkflowPrivateChannelBalanceInsufficient.Code {
		t.Fatalf("stream error code = %#v, must not guess private-channel balance from text", got)
	}
	if got := payload["message"]; got != aichatModelServiceUnavailableMessage {
		t.Fatalf("stream error message = %#v, want %q", got, aichatModelServiceUnavailableMessage)
	}
}

func TestBuildStreamErrorPayloadMapsTypedPrivateChannelBalance(t *testing.T) {
	prepared := streamErrorTestPrepared()
	err := fmt.Errorf("billing failed: %w", &gateway.BillingUserError{
		Kind:  gateway.BillingUserErrorKindPrivateChannelBalanceInsufficient,
		Cause: adapter.ErrInsufficientBalance,
	})

	payload := BuildStreamErrorPayload(prepared, err)

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

	payload := BuildStreamErrorPayload(prepared, err)

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

	payload := BuildStreamErrorPayload(prepared, err)

	if got := payload["code"]; got != response.ErrWorkflowPrivateChannelUpstreamUnavailable.Code {
		t.Fatalf("stream error code = %#v, want %d", got, response.ErrWorkflowPrivateChannelUpstreamUnavailable.Code)
	}
	if got := payload["message"]; got != response.ErrWorkflowPrivateChannelUpstreamUnavailable.Message {
		t.Fatalf("stream error message = %#v, want %#v", got, response.ErrWorkflowPrivateChannelUpstreamUnavailable.Message)
	}
}

func TestBuildStreamErrorPayloadKeepsOrdinaryError(t *testing.T) {
	prepared := streamErrorTestPrepared()
	err := errors.New("plain provider failure")

	payload := BuildStreamErrorPayload(prepared, err)

	if got := payload["message"]; got != "plain provider failure" {
		t.Fatalf("stream error message = %#v, want plain provider failure", got)
	}
	if _, ok := payload["code"]; ok {
		t.Fatalf("stream error code = %#v, want no code for ordinary error", payload["code"])
	}
}

func TestBuildStreamErrorPayloadMapsFinalAnswerUnavailable(t *testing.T) {
	prepared := streamErrorTestPrepared()
	err := errors.Join(
		skillloop.ErrFinalAnswerUnavailable,
		errors.New("terminal-only model returned no final answer"),
	)

	payload := BuildStreamErrorPayload(prepared, err)

	if got := payload["code"]; got != aichatErrorCodeFinalAnswerUnavailable {
		t.Fatalf("stream error code = %#v, want %q", got, aichatErrorCodeFinalAnswerUnavailable)
	}
	if got := payload["message"]; got != aichatFinalAnswerUnavailableMessage {
		t.Fatalf("stream error message = %#v, want %q", got, aichatFinalAnswerUnavailableMessage)
	}
	if _, ok := payload["params"]; ok {
		t.Fatalf("stream error params = %#v, want no params", payload["params"])
	}
}

func TestBuildStreamErrorPayloadMapsModelIdleTimeout(t *testing.T) {
	prepared := streamErrorTestPrepared()

	payload := BuildStreamErrorPayload(prepared, ErrModelIdleTimeout)

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

	payload := BuildStreamErrorPayload(prepared, err)

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

	payload := BuildStreamErrorPayload(prepared, err)

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

	payload := BuildStreamErrorPayload(prepared, err)

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

	payload := BuildStreamErrorPayload(prepared, err)

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

	payload := BuildStreamErrorPayload(prepared, err)

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
