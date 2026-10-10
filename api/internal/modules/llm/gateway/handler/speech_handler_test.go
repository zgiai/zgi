package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/observability"
)

func TestSpeechHandlerStreamsAndFlushesMP3Chunks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &speechServiceStub{chunks: [][]byte{[]byte("MP3-A"), []byte("MP3-B")}}
	handler := NewSpeechHandler(stub)
	recorder := &speechFlushRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(recorder)
	c.Set("llm_api_key", &apikeymodel.TenantAPIKey{ID: "key-id", OrganizationID: "22222222-2222-2222-2222-222222222222"})
	c.Request = newSpeechRequest(t, gateway.SpeechRequest{
		Model:          "seed-tts-2.0",
		Input:          "你好。",
		Voice:          "verified-voice",
		ResponseFormat: "mp3",
	})

	handler.Generate(c)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("Generate() status = %d, want %d; body = %s", got, want, recorder.Body.String())
	}
	if got, want := recorder.Header().Get("Content-Type"), "audio/mpeg"; got != want {
		t.Fatalf("Generate() content type = %q, want %q", got, want)
	}
	if got, want := recorder.Body.String(), "MP3-AMP3-B"; got != want {
		t.Fatalf("Generate() audio = %q, want %q", got, want)
	}
	if got, want := recorder.flushes, 2; got != want {
		t.Fatalf("Generate() flushes = %d, want %d", got, want)
	}
	if stub.request == nil || stub.request.Model != "seed-tts-2.0" || stub.request.Input != "你好。" || stub.request.Voice != "verified-voice" || stub.request.ResponseFormat != "mp3" {
		t.Fatalf("Generate() request = %#v", stub.request)
	}
}

func TestSpeechHandlerRejectsInvalidBoundaryInput(t *testing.T) {
	valid := gateway.SpeechRequest{Model: "seed-tts-2.0", Input: "text", Voice: "voice", ResponseFormat: "mp3"}
	tests := []struct {
		name        string
		contentType string
		body        []byte
	}{
		{name: "wrong content type", contentType: "text/plain", body: mustSpeechJSON(t, valid)},
		{name: "empty body", contentType: "application/json"},
		{name: "missing model", contentType: "application/json", body: mustSpeechJSON(t, gateway.SpeechRequest{Input: "text", Voice: "voice", ResponseFormat: "mp3"})},
		{name: "missing input", contentType: "application/json", body: mustSpeechJSON(t, gateway.SpeechRequest{Model: "seed-tts-2.0", Voice: "voice", ResponseFormat: "mp3"})},
		{name: "missing voice", contentType: "application/json", body: mustSpeechJSON(t, gateway.SpeechRequest{Model: "seed-tts-2.0", Input: "text", ResponseFormat: "mp3"})},
		{name: "unsupported format", contentType: "application/json", body: mustSpeechJSON(t, gateway.SpeechRequest{Model: "seed-tts-2.0", Input: "text", Voice: "voice", ResponseFormat: "wav"})},
		{name: "format with whitespace", contentType: "application/json", body: mustSpeechJSON(t, gateway.SpeechRequest{Model: "seed-tts-2.0", Input: "text", Voice: "voice", ResponseFormat: " mp3 "})},
		{name: "oversized body", contentType: "application/json", body: mustSpeechJSON(t, gateway.SpeechRequest{Model: "seed-tts-2.0", Input: strings.Repeat("x", speechMaxRequestBodyBytes), Voice: "voice", ResponseFormat: "mp3"})},
		{name: "unknown field", contentType: "application/json", body: []byte(`{"model":"seed-tts-2.0","input":"text","voice":"voice","response_format":"mp3","speed":2}`)},
		{name: "trailing JSON", contentType: "application/json", body: append(mustSpeechJSON(t, valid), []byte(` {}`)...)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stub := &speechServiceStub{}
			handler := NewSpeechHandler(stub)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Set("llm_api_key", &apikeymodel.TenantAPIKey{})
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", bytes.NewReader(test.body))
			c.Request.Header.Set("Content-Type", test.contentType)

			handler.Generate(c)

			if got, want := recorder.Code, http.StatusBadRequest; got != want {
				t.Fatalf("Generate() status = %d, want %d; body = %s", got, want, recorder.Body.String())
			}
			if stub.calls != 0 {
				t.Fatalf("Generate() service calls = %d, want 0", stub.calls)
			}
		})
	}
}

func TestSpeechHandlerMapsErrorsBeforeAudioStarts(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
	}{
		{name: "unsupported model", err: adapter.ErrCapabilityUnsupported, status: http.StatusBadRequest},
		{name: "model not found", err: gateway.ErrModelNotFound, status: http.StatusNotFound},
		// The ledger records llm.balance.insufficient (403) for a declined
		// reservation, so the egress must answer 403 and not a retryable 429.
		{name: "insufficient balance", err: adapter.ErrInsufficientBalance, status: http.StatusForbidden},
		{name: "timeout", err: context.DeadlineExceeded, status: http.StatusGatewayTimeout},
		{name: "cancelled", err: context.Canceled, status: statusClientClosedRequest},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := NewSpeechHandler(&speechServiceStub{err: test.err})
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Set("llm_api_key", &apikeymodel.TenantAPIKey{})
			c.Request = newSpeechRequest(t, gateway.SpeechRequest{
				Model: "seed-tts-2.0", Input: "text", Voice: "voice", ResponseFormat: "mp3",
			})

			handler.Generate(c)

			if got := recorder.Code; got != test.status {
				t.Fatalf("Generate() status = %d, want %d; body = %s", got, test.status, recorder.Body.String())
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
				t.Fatalf("Generate() content type = %q, want JSON", got)
			}
		})
	}
}

func TestSpeechHandlerDoesNotAppendJSONAfterPartialAudio(t *testing.T) {
	stub := &speechServiceStub{
		chunks: [][]byte{[]byte("partial-audio")},
		err:    fmt.Errorf("%w: stream interrupted", adapter.ErrUpstreamError),
	}
	handler := NewSpeechHandler(stub)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("llm_api_key", &apikeymodel.TenantAPIKey{})
	c.Request = newSpeechRequest(t, gateway.SpeechRequest{
		Model: "seed-tts-2.0", Input: "text", Voice: "voice", ResponseFormat: "mp3",
	})

	handler.Generate(c)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("Generate() status = %d, want %d", got, want)
	}
	if got, want := recorder.Body.String(), "partial-audio"; got != want {
		t.Fatalf("Generate() body = %q, want audio only %q", got, want)
	}
	if len(c.Errors) != 1 {
		t.Fatalf("Generate() recorded errors = %d, want 1", len(c.Errors))
	}
	if hint, ok := c.Errors[0].Meta.(observability.FailureReportHint); !ok || hint.EventName != "llm.stream.failed" || hint.Classification.Source != observability.ErrorSourceProvider {
		t.Fatalf("Generate() report hint = %#v, want provider stream failure", c.Errors[0].Meta)
	}
}

func TestSpeechHandlerSuppressesNoProviderStatusFallback(t *testing.T) {
	handler := NewSpeechHandler(&speechServiceStub{err: gateway.NewNoProviderAvailableError("tts", "org")})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("llm_api_key", &apikeymodel.TenantAPIKey{})
	c.Request = newSpeechRequest(t, gateway.SpeechRequest{Model: "tts", Input: "text", Voice: "voice", ResponseFormat: "mp3"})

	handler.Generate(c)

	if recorder.Code != http.StatusServiceUnavailable || len(c.Errors) != 1 {
		t.Fatalf("Generate() status/errors = %d/%d, want 503/1", recorder.Code, len(c.Errors))
	}
	if hint, ok := c.Errors[0].Meta.(observability.FailureReportHint); !ok || !hint.Suppress {
		t.Fatalf("Generate() report hint = %#v, want suppressed fallback", c.Errors[0].Meta)
	}
}

func newSpeechRequest(t *testing.T, body gateway.SpeechRequest) *http.Request {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", bytes.NewReader(mustSpeechJSON(t, body)))
	request.Header.Set("Content-Type", "application/json")
	return request
}

func mustSpeechJSON(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode JSON: %v", err)
	}
	return payload
}

type speechServiceStub struct {
	calls   int
	request *gateway.SpeechRequest
	chunks  [][]byte
	err     error
}

func (s *speechServiceStub) GenerateSpeech(_ context.Context, _ *apikeymodel.TenantAPIKey, request *gateway.SpeechRequest, dst io.Writer) error {
	s.calls++
	s.request = request
	for _, chunk := range s.chunks {
		if _, err := dst.Write(chunk); err != nil {
			return err
		}
	}
	return s.err
}

type speechFlushRecorder struct {
	*httptest.ResponseRecorder
	flushes int
}

func (r *speechFlushRecorder) Flush() {
	r.flushes++
	r.ResponseRecorder.Flush()
}

// TestSpeechHandlerProjectsFailuresWithTheProjector is the speech equivalent of
// the transcription projector test: the egress must carry the catalog identity
// and localized message once a projector is attached.
func TestSpeechHandlerProjectsFailuresWithTheProjector(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewSpeechHandler(&speechServiceStub{err: adapter.ErrInsufficientBalance})
	handler.SetErrorProjector(testApplicationErrorProjector(t))

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("llm_api_key", &apikeymodel.TenantAPIKey{})
	c.Request = newSpeechRequest(t, gateway.SpeechRequest{
		Model: "seed-tts-2.0", Input: "text", Voice: "voice", ResponseFormat: "mp3",
	})
	c.Request.Header.Set("Accept-Language", "zh-CN")

	handler.Generate(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	if got := recorder.Header().Get("X-ZGI-App-Error-Code"); got != "llm.balance.insufficient" {
		t.Fatalf("X-ZGI-App-Error-Code = %q, want llm.balance.insufficient", got)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["error_code"] != "llm.balance.insufficient" {
		t.Fatalf("error_code = %v, want llm.balance.insufficient", body["error_code"])
	}
}
