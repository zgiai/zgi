package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestTranscriptionHandlerStreamsPCMAndReturnsEditableText(t *testing.T) {
	gin.SetMode(gin.TestMode)
	audio := []byte("pcm-audio")
	stub := &transcriptionServiceStub{response: &gateway.TranscriptionResponse{
		RequestID: "11111111-1111-1111-1111-111111111111",
		Text:      "editable transcript",
	}}
	handler := NewTranscriptionHandler(stub)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("llm_api_key", &apikeymodel.TenantAPIKey{ID: "key-id", OrganizationID: "22222222-2222-2222-2222-222222222222"})
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions?model=volc.seedasr.sauc.duration", bytes.NewReader(audio))
	c.Request.Header.Set("Content-Type", "audio/pcm; rate=16000")

	handler.Transcribe(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if stub.request == nil || stub.request.Model != "volc.seedasr.sauc.duration" {
		t.Fatalf("request = %#v, want normalized model", stub.request)
	}
	if got := string(stub.audio); got != string(audio) {
		t.Fatalf("audio = %q, want %q", got, audio)
	}
	var response gateway.TranscriptionResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Text != "editable transcript" || response.RequestID == "" {
		t.Fatalf("response = %#v, want request ID and editable transcript", response)
	}
}

func TestTranscriptionHandlerRejectsInvalidBoundaryInput(t *testing.T) {
	tests := []struct {
		name        string
		target      string
		contentType string
		audio       []byte
	}{
		{name: "missing model", target: "/v1/audio/transcriptions", contentType: "audio/pcm", audio: []byte("pcm")},
		{name: "wrong content type", target: "/v1/audio/transcriptions?model=asr", contentType: "audio/webm", audio: []byte("audio")},
		{name: "empty audio", target: "/v1/audio/transcriptions?model=asr", contentType: "audio/pcm"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stub := &transcriptionServiceStub{}
			handler := NewTranscriptionHandler(stub)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Set("llm_api_key", &apikeymodel.TenantAPIKey{})
			c.Request = httptest.NewRequest(http.MethodPost, test.target, bytes.NewReader(test.audio))
			c.Request.Header.Set("Content-Type", test.contentType)

			handler.Transcribe(c)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
			if stub.calls != 0 {
				t.Fatalf("service calls = %d, want 0", stub.calls)
			}
		})
	}
}

func TestTranscriptionHandlerMapsServiceErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
	}{
		{name: "unsupported model", err: adapter.ErrCapabilityUnsupported, status: http.StatusBadRequest},
		{name: "model not found", err: gateway.ErrModelNotFound, status: http.StatusNotFound},
		{name: "insufficient balance", err: adapter.ErrInsufficientBalance, status: http.StatusTooManyRequests},
		{name: "timeout", err: context.DeadlineExceeded, status: http.StatusGatewayTimeout},
		{name: "cancelled", err: context.Canceled, status: 499},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := NewTranscriptionHandler(&transcriptionServiceStub{err: test.err})
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Set("llm_api_key", &apikeymodel.TenantAPIKey{})
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions?model=asr", bytes.NewReader([]byte("pcm")))
			c.Request.Header.Set("Content-Type", "audio/pcm")

			handler.Transcribe(c)

			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, test.status, recorder.Body.String())
			}
		})
	}
}

type transcriptionServiceStub struct {
	request  *gateway.TranscriptionRequest
	response *gateway.TranscriptionResponse
	err      error
	audio    []byte
	calls    int
}

func (s *transcriptionServiceStub) Transcribe(_ context.Context, _ *apikeymodel.TenantAPIKey, request *gateway.TranscriptionRequest) (*gateway.TranscriptionResponse, error) {
	s.calls++
	s.request = request
	if request != nil && request.Audio != nil {
		s.audio, _ = io.ReadAll(request.Audio)
	}
	return s.response, s.err
}
