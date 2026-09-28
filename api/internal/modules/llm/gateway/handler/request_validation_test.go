package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
)

func TestValidatePublicNativeBodyRejectsInvalidProtocolParameters(t *testing.T) {
	tests := []struct {
		protocol string
		body     string
	}{
		{"responses", `{"model":"gpt-test","input":"hello","max_output_tokens":0}`},
		{"messages", `{"model":"claude-test","messages":[{"role":"user","content":"hello"}],"max_tokens":-1}`},
		{"gemini", `{"contents":[{"parts":[{"text":"hello"}]}],"generationConfig":{"maxOutputTokens":0}}`},
		{"responses", `{"model":"gpt-test","input":"hello","top_p":2}`},
	}
	for _, test := range tests {
		if err := validatePublicNativeBody(json.RawMessage(test.body), test.protocol); err == nil {
			t.Fatalf("invalid %s body was accepted: %s", test.protocol, test.body)
		}
	}
}

func TestInvalidParametersStopBeforeGatewayServiceForAllProtocols(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name string
		path string
		body string
		call func(*LLMHandler, *gin.Context)
	}{
		{"chat", "/v1/chat/completions", `{"model":"test","messages":[{"role":"user","content":"hi"}],"max_tokens":0}`, (*LLMHandler).ChatCompletions},
		{"chat_stream", "/v1/chat/completions", `{"model":"test","messages":[{"role":"user","content":"hi"}],"max_tokens":0,"stream":true}`, (*LLMHandler).ChatCompletions},
		{"responses", "/v1/responses", `{"model":"test","input":"hi","max_output_tokens":0}`, (*LLMHandler).CreateResponse},
		{"responses_stream", "/v1/responses", `{"model":"test","input":"hi","max_output_tokens":0,"stream":true}`, (*LLMHandler).CreateResponse},
		{"messages", "/v1/messages", `{"model":"test","messages":[{"role":"user","content":"hi"}],"max_tokens":0}`, (*LLMHandler).CreateAnthropicMessage},
		{"messages_stream", "/v1/messages", `{"model":"test","messages":[{"role":"user","content":"hi"}],"max_tokens":0,"stream":true}`, (*LLMHandler).CreateAnthropicMessage},
		{"gemini", "/v1beta/models/test:generateContent", `{"contents":[{"parts":[{"text":"hi"}]}],"generationConfig":{"maxOutputTokens":0}}`, (*LLMHandler).CreateGeminiContent},
		{"gemini_stream", "/v1beta/models/test:streamGenerateContent?alt=sse", `{"contents":[{"parts":[{"text":"hi"}]}],"generationConfig":{"maxOutputTokens":0}}`, (*LLMHandler).CreateGeminiContent},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stub := &invocationGatewayStub{}
			handler := NewLLMHandler(stub, testApplicationErrorProjector(t))
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Set("llm_api_key", &apikeymodel.TenantAPIKey{})
			c.Request = httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
			c.Request.Header.Set("Content-Type", "application/json")
			if strings.Contains(test.path, "/v1beta/") {
				modelAction := strings.TrimPrefix(strings.Split(test.path, "?")[0], "/v1beta/models")
				c.Params = gin.Params{{Key: "modelAction", Value: modelAction}}
			}
			test.call(handler, c)
			if recorder.Code != http.StatusBadRequest || stub.id != "" {
				t.Fatalf("status=%d gateway_id=%q body=%s", recorder.Code, stub.id, recorder.Body.String())
			}
		})
	}
}

func TestInvalidRequestUsesLocalizedCatalogMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &invocationGatewayStub{}
	handler := NewLLMHandler(stub, testApplicationErrorProjector(t))
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("llm_api_key", &apikeymodel.TenantAPIKey{})
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"messages":[]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Accept-Language", "zh-CN")

	handler.ChatCompletions(c)

	if recorder.Code != http.StatusBadRequest || stub.id != "" {
		t.Fatalf("status=%d gateway_id=%q body=%s", recorder.Code, stub.id, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "大模型请求内容不正确") || strings.Contains(body, "model is required") {
		t.Fatalf("request validation leaked an untranslated message: %s", body)
	}
}
