package handler

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway/types"
)

func TestChatCompletionsForwardsProviderExtrasToGateway(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv(gateway.CapabilityGateEnv, "")

	service := &chatHandlerStub{caps: types.ModelCapabilities{Known: true}}
	handler := &LLMHandler{gatewayService: service}
	body := `{"model":"qwen3.8-27b","messages":[{"role":"user","content":"hi"}],"enable_thinking":false,"chat_template_kwargs":{"enable_thinking":false},"reasoning_effort":"low"}`

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("llm_api_key", &apikeymodel.TenantAPIKey{})

	handler.ChatCompletions(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	if service.chatRequest == nil {
		t.Fatal("gateway ChatCompletion was not called")
	}
	want := map[string]interface{}{
		"enable_thinking":      false,
		"chat_template_kwargs": map[string]interface{}{"enable_thinking": false},
		"reasoning_effort":     "low",
	}
	if got := service.chatRequest.AdditionalParameters; !reflect.DeepEqual(got, want) {
		t.Fatalf("AdditionalParameters = %#v, want %#v", got, want)
	}
}
