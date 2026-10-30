package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway/types"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// capabilityStubService embeds the shared interface so it stays source
// compatible with the full GatewayService contract while only serving the
// additive capability lookup the gate type-asserts for.
type capabilityStubService struct {
	types.GatewayService

	caps  types.ModelCapabilities
	err   error
	calls int
}

func (s *capabilityStubService) ModelCapabilities(context.Context, string) (types.ModelCapabilities, error) {
	s.calls++
	return s.caps, s.err
}

func newCapabilityTestRequest(body string) *adapter.ChatRequest {
	req := &adapter.ChatRequest{Model: "test-model", Messages: []adapter.Message{{Role: "user", Content: "hi"}}}
	if body != "" {
		_ = json.Unmarshal([]byte(body), req)
	}
	return req
}

func TestRequiredChatCapabilities(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want []chatCapability
	}{
		{name: "plain text needs nothing"},
		{name: "json_schema needs structured output", body: `{"response_format":{"type":"json_schema"}}`, want: []chatCapability{chatCapabilityStructuredOutput}},
		{name: "json_object needs json mode", body: `{"response_format":{"type":"json_object"}}`, want: []chatCapability{chatCapabilityJSONMode}},
		{name: "text format is ignored", body: `{"response_format":{"type":"text"}}`},
		{name: "tools need tool calling", body: `{"tools":[{"type":"function"}]}`, want: []chatCapability{chatCapabilityToolCall}},
		{
			name: "image part needs vision",
			body: `{"messages":[{"role":"user","content":[{"type":"text","text":"what color"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AA=="}}]}]}`,
			want: []chatCapability{chatCapabilityVision},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := requiredChatCapabilities(newCapabilityTestRequest(tt.body))
			if len(got) != len(tt.want) {
				t.Fatalf("requiredChatCapabilities() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("requiredChatCapabilities() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestChatMessagesContainImage(t *testing.T) {
	t.Parallel()

	typed := []adapter.Message{{Role: "user", Content: []adapter.MessageContentPart{
		{Type: "image_url", ImageURL: &adapter.ImageURL{URL: "data:image/png;base64,AA=="}},
	}}}
	if !chatMessagesContainImage(typed) {
		t.Fatal("typed image part was not detected")
	}

	decoded := newCapabilityTestRequest(`{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]}]}`)
	if !chatMessagesContainImage(decoded.Messages) {
		t.Fatal("decoded image part was not detected")
	}

	if chatMessagesContainImage([]adapter.Message{{Role: "user", Content: "plain text"}}) {
		t.Fatal("plain text was reported as an image")
	}
}

func TestMissingChatCapability(t *testing.T) {
	t.Parallel()

	caps := types.ModelCapabilities{Known: true, ToolCall: true}
	if need, missing := missingChatCapability(caps, []chatCapability{chatCapabilityToolCall}); missing {
		t.Fatalf("declared tool calling was reported missing as %q", need)
	}
	if need, missing := missingChatCapability(caps, []chatCapability{chatCapabilityToolCall, chatCapabilityStructuredOutput}); !missing || need != chatCapabilityStructuredOutput {
		t.Fatalf("missing = %q/%v, want structured output", need, missing)
	}
}

func TestEnforceChatCapabilitiesRejectsUndeclaredCapability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv(gateway.CapabilityGateEnv, "")

	service := &capabilityStubService{caps: types.ModelCapabilities{Known: true, ToolCall: true}}
	handler := &LLMHandler{gatewayService: service}
	req := newCapabilityTestRequest(`{"response_format":{"type":"json_schema","json_schema":{"name":"x","schema":{"type":"object"}}}}`)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	if handler.enforceChatCapabilities(c, req) {
		t.Fatal("request with undeclared json_schema was allowed")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}

	var payload struct {
		Error struct {
			Code    string `json:"code"`
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
		Retryable *bool `json:"retryable"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if payload.Error.Code != "unsupported_capability" {
		t.Fatalf("error.code = %q, want unsupported_capability", payload.Error.Code)
	}
	if payload.Error.Type != "invalid_request_error" {
		t.Fatalf("error.type = %q, want invalid_request_error", payload.Error.Type)
	}
	if payload.Retryable == nil || *payload.Retryable {
		t.Fatalf("retryable = %v, want false", payload.Retryable)
	}
}

func TestEnforceChatCapabilitiesFailsOpen(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reqBody := `{"response_format":{"type":"json_schema","json_schema":{"name":"x","schema":{"type":"object"}}}}`

	tests := []struct {
		name     string
		caps     types.ModelCapabilities
		err      error
		noImpl   bool
		disabled bool
	}{
		{name: "metadata unknown", caps: types.ModelCapabilities{}},
		{name: "lookup failed", err: errors.New("database is down")},
		{name: "declared capability", caps: types.ModelCapabilities{Known: true, StructuredOutput: true}},
		{name: "gate disabled", disabled: true, caps: types.ModelCapabilities{Known: true}},
		{name: "service has no capability lookup", noImpl: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(gateway.CapabilityGateEnv, "")
			if tt.disabled {
				t.Setenv(gateway.CapabilityGateEnv, "off")
			}
			handler := &LLMHandler{gatewayService: &capabilityStubService{caps: tt.caps, err: tt.err}}
			if tt.noImpl {
				handler = &LLMHandler{gatewayService: noCapabilityService{}}
			}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

			if !handler.enforceChatCapabilities(c, newCapabilityTestRequest(reqBody)) {
				t.Fatalf("request was rejected, want fail open (status=%d body=%s)", recorder.Code, recorder.Body.String())
			}
			if recorder.Code == http.StatusBadRequest {
				t.Fatalf("fail-open path wrote a 400: %s", recorder.Body.String())
			}
		})
	}
}

// noCapabilityService implements the shared interface only, so the gate's type
// assertion fails and the request must fall through untouched.
type noCapabilityService struct {
	types.GatewayService
}

// chatHandlerStub serves the chat entry points the handler test exercises and
// records whether the request reached the gateway at all.
type chatHandlerStub struct {
	types.GatewayService

	caps      types.ModelCapabilities
	capsErr   error
	chatCalls int
}

func (s *chatHandlerStub) ModelCapabilities(context.Context, string) (types.ModelCapabilities, error) {
	return s.caps, s.capsErr
}

func (s *chatHandlerStub) ChatCompletion(context.Context, *apikeymodel.TenantAPIKey, *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	s.chatCalls++
	return &adapter.ChatResponse{ID: "chatcmpl-test", Object: "chat.completion", Model: "test-model"}, nil
}

func TestChatCompletionsRejectsUndeclaredCapabilityBeforeGateway(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv(gateway.CapabilityGateEnv, "")

	service := &chatHandlerStub{caps: types.ModelCapabilities{Known: true}}
	handler := &LLMHandler{gatewayService: service}
	body := `{"model":"deepseek-v4-pro","messages":[{"role":"user","content":"hi"}],"response_format":{"type":"json_schema","json_schema":{"name":"x","schema":{"type":"object"}}}}`

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("llm_api_key", &apikeymodel.TenantAPIKey{})

	handler.ChatCompletions(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
	}
	if service.chatCalls != 0 {
		t.Fatalf("gateway ChatCompletion called %d times, want 0", service.chatCalls)
	}
}

func TestChatCompletionsAllowsDeclaredCapability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv(gateway.CapabilityGateEnv, "")

	service := &chatHandlerStub{caps: types.ModelCapabilities{Known: true, StructuredOutput: true}}
	handler := &LLMHandler{gatewayService: service}
	body := `{"model":"gpt-5.2","messages":[{"role":"user","content":"hi"}],"response_format":{"type":"json_schema","json_schema":{"name":"x","schema":{"type":"object"}}}}`

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("llm_api_key", &apikeymodel.TenantAPIKey{})

	handler.ChatCompletions(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	if service.chatCalls != 1 {
		t.Fatalf("gateway ChatCompletion called %d times, want 1", service.chatCalls)
	}
}
