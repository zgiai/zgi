package provider

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestNormalizeOpenAISDKTransportError(t *testing.T) {
	assertSDKTimeoutNormalization(t, normalizeOpenAISDKTransportError, func(status int) error {
		return &openai.Error{
			StatusCode: status,
			Request:    sdkTestRequest(t),
			Response:   &http.Response{StatusCode: status},
		}
	})
}

func TestNormalizeAnthropicSDKTransportError(t *testing.T) {
	assertSDKTimeoutNormalization(t, normalizeAnthropicSDKTransportError, func(status int) error {
		return &anthropic.Error{
			StatusCode: status,
			Request:    sdkTestRequest(t),
			Response:   &http.Response{StatusCode: status},
		}
	})
}

func assertSDKTimeoutNormalization(t *testing.T, normalize func(error) error, apiError func(int) error) {
	t.Helper()

	deadline := normalize(context.DeadlineExceeded)
	if !errors.Is(deadline, adapter.ErrTimeout) || !errors.Is(deadline, context.DeadlineExceeded) {
		t.Fatalf("deadline normalization lost timeout identity or cause: %v", deadline)
	}

	gatewayTimeout := apiError(http.StatusGatewayTimeout)
	normalized := normalize(gatewayTimeout)
	if !errors.Is(normalized, adapter.ErrTimeout) {
		t.Fatalf("504 normalization = %v, want ErrTimeout", normalized)
	}
	if !errors.Is(normalized, gatewayTimeout) {
		t.Fatalf("504 normalization lost SDK cause: %v", normalized)
	}

	unavailable := apiError(http.StatusServiceUnavailable)
	if normalized := normalize(unavailable); errors.Is(normalized, adapter.ErrTimeout) {
		t.Fatalf("503 normalization = %v, must not match ErrTimeout", normalized)
	}
}

func sdkTestRequest(t *testing.T) *http.Request {
	t.Helper()
	target, err := url.Parse("https://provider.example.test/v1")
	if err != nil {
		t.Fatal(err)
	}
	return &http.Request{Method: http.MethodPost, URL: target}
}
