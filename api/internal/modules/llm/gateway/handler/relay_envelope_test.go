package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// A Console 4xx relayed through the raw protocol path already carries the
// cataloged envelope; this edge must add the app-code header and answer with
// its own request_id, while keeping Console's error_id for support lookups.
func TestWriteRawProtocolErrorRelaysConsoleEnvelopeWithHeaderAndRequestID(t *testing.T) {
	handler := &LLMHandler{errorProjector: testApplicationErrorProjector(t)}
	context, recorder := newT17Context("en-US")
	err := &adapter.RawProtocolError{
		StatusCode:  http.StatusBadRequest,
		ContentType: "application/json",
		Body: []byte(`{"code":"llm.request.invalid","error":{"code":"upstream_error","message":"The model request is invalid.","type":"invalid_request_error"},` +
			`"error_code":"llm.request.invalid","error_id":"console-error-id","message":"The model request is invalid.","params":{},"request_id":"console-request-id","retryable":false}`),
	}

	if !handler.writeRawProtocolError(context, err, "gpt-4o", "openai") {
		t.Fatal("raw protocol error was not handled")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", recorder.Code)
	}
	if got := recorder.Header().Get("X-ZGI-App-Error-Code"); got != "llm.request.invalid" {
		t.Fatalf("X-ZGI-App-Error-Code = %q", got)
	}
	var body map[string]any
	if decodeErr := json.Unmarshal(recorder.Body.Bytes(), &body); decodeErr != nil {
		t.Fatalf("decode: %v", decodeErr)
	}
	if body["request_id"] != "req-t17" || body["error_id"] != "console-error-id" || body["error_code"] != "llm.request.invalid" {
		t.Fatalf("relayed envelope = %v", body)
	}
}

// A body whose code is not cataloged here is relayed unchanged, without a
// header: an arbitrary upstream string must never become a contract code.
func TestWriteRawProtocolErrorLeavesUncatalogedBodyAlone(t *testing.T) {
	handler := &LLMHandler{errorProjector: testApplicationErrorProjector(t)}
	context, recorder := newT17Context("en-US")
	raw := `{"code":"some_provider_code","request_id":"upstream-id"}`
	err := &adapter.RawProtocolError{StatusCode: http.StatusBadRequest, ContentType: "application/json", Body: []byte(raw)}

	if !handler.writeRawProtocolError(context, err, "gpt-4o", "openai") {
		t.Fatal("raw protocol error was not handled")
	}
	if got := recorder.Header().Get("X-ZGI-App-Error-Code"); got != "" {
		t.Fatalf("X-ZGI-App-Error-Code = %q, want none", got)
	}
	if !strings.Contains(recorder.Body.String(), `"request_id":"upstream-id"`) {
		t.Fatalf("body was rewritten: %s", recorder.Body.String())
	}
}

// In-stream error frames carry the cataloged envelope fields on all three
// protocols. The OpenAI frame keeps its legacy top-level code.
func TestStreamErrorFramesCarryCatalogEnvelope(t *testing.T) {
	handler := &LLMHandler{errorProjector: testApplicationErrorProjector(t)}
	settleErr := fmt.Errorf("console proxy settlement failed: %w", gateway.ErrBillingSettleFailed)
	frames := map[string]func() []byte{
		"openai": func() []byte {
			context, _ := newT17Context("en-US")
			return handler.openAIStreamError(context, settleErr)
		},
		"anthropic": func() []byte {
			context, _ := newT17Context("en-US")
			return handler.anthropicStreamError(context, settleErr)
		},
		"gemini": func() []byte {
			context, _ := newT17Context("en-US")
			return geminiStreamErrorBody(context, handler.localizedProtocolError(context, settleErr))
		},
	}
	for name, frame := range frames {
		t.Run(name, func(t *testing.T) {
			var body map[string]any
			if err := json.Unmarshal(frame(), &body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			for _, field := range []string{"code", "error_code", "message", "params", "retryable", "error_id", "request_id"} {
				if _, ok := body[field]; !ok {
					t.Fatalf("frame is missing %q: %v", field, body)
				}
			}
			if body["error_code"] != "billing.settle.failed" || body["request_id"] != "req-t17" {
				t.Fatalf("frame = %v", body)
			}
		})
	}
}
