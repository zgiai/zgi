package service

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestBuildStreamErrorPayloadMapsProviderInsufficientBalance(t *testing.T) {
	prepared := streamErrorTestPrepared()
	err := fmt.Errorf(
		"all providers failed: %w",
		adapter.NewAdapterError("insufficient_quota", "account credits exhausted", 402, adapter.ErrInsufficientBalance),
	)

	payload := BuildStreamErrorPayload(prepared, err)

	if got := payload["code"]; got != response.ErrWorkflowPrivateChannelBalanceInsufficient.Code {
		t.Fatalf("stream error code = %#v, want %d", got, response.ErrWorkflowPrivateChannelBalanceInsufficient.Code)
	}
	if got := payload["message"]; got != response.ErrWorkflowPrivateChannelBalanceInsufficient.Message {
		t.Fatalf("stream error message = %#v, want %#v", got, response.ErrWorkflowPrivateChannelBalanceInsufficient.Message)
	}
}

func TestBuildStreamErrorPayloadMapsProviderInsufficientBalanceTextFallback(t *testing.T) {
	prepared := streamErrorTestPrepared()
	err := fmt.Errorf(
		"all providers failed: provider stream call failed: %w",
		adapter.NewAdapterError("", "Insufficient Balance", 402, adapter.ErrUpstreamError),
	)

	payload := BuildStreamErrorPayload(prepared, err)

	if got := payload["code"]; got != response.ErrWorkflowPrivateChannelBalanceInsufficient.Code {
		t.Fatalf("stream error code = %#v, want %d", got, response.ErrWorkflowPrivateChannelBalanceInsufficient.Code)
	}
	if got := payload["message"]; got != response.ErrWorkflowPrivateChannelBalanceInsufficient.Message {
		t.Fatalf("stream error message = %#v, want %#v", got, response.ErrWorkflowPrivateChannelBalanceInsufficient.Message)
	}
	if got, _ := payload["message"].(string); got == "" || got == err.Error() {
		t.Fatalf("stream error message = %q, want public insufficient-balance message", got)
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

func TestPublicAichatStoredErrorMessageMapsLegacyInsufficientBalance(t *testing.T) {
	raw := "all providers failed: Insufficient Balance: upstream service error"

	got := publicAichatStoredErrorMessage(raw)

	if got != response.ErrWorkflowPrivateChannelBalanceInsufficient.Message {
		t.Fatalf("stored error message = %q, want %q", got, response.ErrWorkflowPrivateChannelBalanceInsufficient.Message)
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
