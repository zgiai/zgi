package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	appconfig "github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/llm/channelprovider"
)

func setLLMAllowPrivateBaseURL(t *testing.T, allow bool) {
	t.Helper()
	previous := appconfig.GlobalConfig
	next := &appconfig.Config{}
	if previous != nil {
		copied := *previous
		next = &copied
	}
	next.LLM.AllowPrivateBaseURL = allow
	appconfig.GlobalConfig = next
	t.Cleanup(func() {
		appconfig.GlobalConfig = previous
	})
}

func TestParsePlatformUpdateChannelRequestRejectsIsActive(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := `{"priority":200,"weight":100,"is_active":false}`
	c, w := newJSONContext(body)

	req, ok := parsePlatformUpdateChannelRequest(c)
	if ok {
		t.Fatalf("expected parser to reject is_active, got ok=true with req=%+v", req)
	}

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestParsePlatformUpdateChannelRequestAcceptsIsEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := `{"priority":200,"weight":100,"is_enabled":false}`
	c, _ := newJSONContext(body)

	req, ok := parsePlatformUpdateChannelRequest(c)
	if !ok {
		t.Fatalf("expected parser to accept is_enabled")
	}
	if req == nil {
		t.Fatalf("expected non-nil request")
	}
	if req.Priority == nil || *req.Priority != 200 {
		t.Fatalf("expected priority=200, got %+v", req.Priority)
	}
	if req.Weight == nil || *req.Weight != 100 {
		t.Fatalf("expected weight=100, got %+v", req.Weight)
	}
	if req.IsEnabled == nil || *req.IsEnabled != false {
		t.Fatalf("expected is_enabled=false, got %+v", req.IsEnabled)
	}
}

func TestParseCreateRouteRequestRejectsInitialQuota(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := `{"name":"test","channel_provider":"openai","api_key":"sk-test","initial_quota":100}`
	c, w := newJSONContext(body)

	req, ok := parseCreateRouteRequest(c)
	if ok {
		t.Fatalf("expected parser to reject initial_quota, got ok=true with req=%+v", req)
	}

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	msg, _ := resp["message"].(string)
	if !strings.Contains(msg, "initial_funds") {
		t.Fatalf("expected error message to mention initial_funds, got %q", msg)
	}
}

func TestHandleCreateRouteErrorReturnsInvalidParamForValidationFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, w := newJSONContext(`{}`)

	handleCreateRouteError(c, errors.New("channel validation failed: all representative models failed: qwen-plus: unauthorized"))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	msg, _ := resp["message"].(string)
	if !strings.Contains(msg, "channel validation failed") {
		t.Fatalf("expected validation failure message, got %q", msg)
	}
}

func TestHandleCreateRouteErrorKeepsSystemErrorsInternal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, w := newJSONContext(`{}`)

	handleCreateRouteError(c, errors.New("failed to create route: database unavailable"))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}
}

func TestParseCreateRouteRequestAcceptsInitialFunds(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := `{"name":"test","channel_provider":"openai","api_key":"sk-test","initial_funds":100}`
	c, _ := newJSONContext(body)

	req, ok := parseCreateRouteRequest(c)
	if !ok {
		t.Fatalf("expected parser to accept initial_funds")
	}
	if req == nil {
		t.Fatalf("expected non-nil request")
	}
	if req.InitialFunds != 100 {
		t.Fatalf("expected initial_funds=100, got %d", req.InitialFunds)
	}
	if req.ChannelProvider != "openai" {
		t.Fatalf("expected channel_provider=openai, got %q", req.ChannelProvider)
	}
}

func TestParseCreateRouteRequestRejectsMissingChannelProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := `{"name":"test","api_key":"sk-test","initial_funds":100}`
	c, w := newJSONContext(body)

	req, ok := parseCreateRouteRequest(c)
	if ok {
		t.Fatalf("expected parser to reject missing channel_provider, got ok=true with req=%+v", req)
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestParseCreateRouteRequestRejectsOpenAICompatibleWithoutBaseURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := `{"name":"test","channel_provider":"openai-compatible","api_key":"sk-test","models":["gpt-4o"]}`
	c, w := newJSONContext(body)

	req, ok := parseCreateRouteRequest(c)
	if ok {
		t.Fatalf("expected parser to reject missing api_base_url, got ok=true with req=%+v", req)
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestParseCreateRouteRequestRejectsOpenAICompatibleWithoutAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := `{"name":"test","channel_provider":"openai-compatible","api_base_url":"https://proxy.example.com/v1","models":["gpt-4o"]}`
	c, w := newJSONContext(body)

	req, ok := parseCreateRouteRequest(c)
	if ok {
		t.Fatalf("expected parser to reject missing api_key, got ok=true with req=%+v", req)
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestParseCreateRouteRequestAcceptsOllamaWithoutAPIKey(t *testing.T) {
	setLLMAllowPrivateBaseURL(t, true)
	gin.SetMode(gin.TestMode)

	body := `{"name":"local ollama","channel_provider":"ollama","api_base_url":"http://localhost:11434","models":["qwen3.5:4b"]}`
	c, _ := newJSONContext(body)

	req, ok := parseCreateRouteRequest(c)
	if !ok {
		t.Fatalf("expected parser to accept ollama without api_key")
	}
	if req == nil {
		t.Fatalf("expected non-nil request")
	}
	if req.APIKey != "" {
		t.Fatalf("expected empty api_key, got %q", req.APIKey)
	}
}

func TestParseCreateRouteRequestRejectsProtocol(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := `{"name":"test","channel_provider":"openai","api_key":"sk-test","protocol":"openai"}`
	c, w := newJSONContext(body)

	req, ok := parseCreateRouteRequest(c)
	if ok {
		t.Fatalf("expected parser to reject protocol, got ok=true with req=%+v", req)
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestParseCreateRouteRequestRejectsSupportedProtocols(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := `{"name":"test","channel_provider":"openai","api_key":"sk-test","supported_protocols":["openai"]}`
	c, w := newJSONContext(body)

	req, ok := parseCreateRouteRequest(c)
	if ok {
		t.Fatalf("expected parser to reject supported_protocols, got ok=true with req=%+v", req)
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestParseDraftTestChannelModelRequestRejectsOpenAICompatibleWithoutBaseURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := `{"channel_provider":"openai-compatible","api_key":"sk-test","model":"gpt-4o"}`
	c, w := newJSONContext(body)

	req, ok := parseDraftTestChannelModelRequest(c)
	if ok {
		t.Fatalf("expected parser to reject missing api_base_url, got ok=true with req=%+v", req)
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestParseDraftTestChannelModelRequestAcceptsValidPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := `{"channel_provider":"openai-compatible","api_key":"sk-test","api_base_url":"https://proxy.example.com/v1","model":"gpt-4o","test_method":"chat"}`
	c, _ := newJSONContext(body)

	req, ok := parseDraftTestChannelModelRequest(c)
	if !ok {
		t.Fatalf("expected parser to accept valid payload")
	}
	if req == nil {
		t.Fatalf("expected non-nil request")
	}
	if req.ChannelProvider != "openai-compatible" {
		t.Fatalf("expected channel_provider=openai-compatible, got %q", req.ChannelProvider)
	}
	if req.APIBaseURL != "https://proxy.example.com/v1" {
		t.Fatalf("expected api_base_url to round-trip, got %q", req.APIBaseURL)
	}
}

func TestParseDraftTestChannelModelRequestAcceptsOllamaWithoutAPIKey(t *testing.T) {
	setLLMAllowPrivateBaseURL(t, true)
	gin.SetMode(gin.TestMode)

	body := `{"channel_provider":"ollama","api_base_url":"http://localhost:11434","model":"qwen3.5:4b","test_method":"chat"}`
	c, _ := newJSONContext(body)

	req, ok := parseDraftTestChannelModelRequest(c)
	if !ok {
		t.Fatalf("expected parser to accept ollama without api_key")
	}
	if req == nil {
		t.Fatalf("expected non-nil request")
	}
	if req.APIKey != "" {
		t.Fatalf("expected empty api_key, got %q", req.APIKey)
	}
}

func TestHandleChannelMutationErrorMapsProviderAPIKeyInvalidToBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, w := newJSONContext(`{}`)
	err := fmt.Errorf("Authentication Fails, Your api key: ****1d35 is invalid: %w", channelprovider.ErrProviderAPIKeyInvalid)
	handleChannelMutationError(c, err)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got := resp["code"]; got != "199001" {
		t.Fatalf("expected code 199001, got %v", got)
	}
	expectedMsg, ok := channelprovider.UserVisibleValidationMessage(err)
	if !ok {
		t.Fatalf("expected provider api key error to be user visible")
	}
	msg, _ := resp["message"].(string)
	if msg != expectedMsg {
		t.Fatalf("expected friendly provider api key message, got %q", msg)
	}
	if strings.Contains(msg, "Authentication Fails") || strings.Contains(msg, "1d35") {
		t.Fatalf("message leaked upstream details: %q", msg)
	}
}

func newJSONContext(body string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/channels/platform", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}
