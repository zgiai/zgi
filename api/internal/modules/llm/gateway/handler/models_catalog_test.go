package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func preCatalogTestModels() []adapter.Model {
	return []adapter.Model{
		{ID: "chat-only", Name: "Chat Only", OwnedBy: "legacy-provider", SupportedProtocols: []string{"openai_chat_completions"}},
		{ID: "agent-a", Name: "Agent A", OwnedBy: "zgi", ContextLength: 200000, MaxInputTokens: 180000, MaxOutputTokens: 16000, Capabilities: []string{"tool_use", "thinking"}, ParallelToolCalls: true, SupportedProtocols: []string{modelsProtocolResponses, modelsProtocolAnthropic}},
		{ID: "agent-b", Name: "Agent B", OwnedBy: "zgi", SupportedProtocols: []string{modelsProtocolAnthropic}},
	}
}

func TestModelsCatalogDefaultsToOpenAIShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	writeModelsCatalog(c, preCatalogTestModels())
	var payload struct {
		Object string           `json:"object"`
		Data   []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Object != "list" || len(payload.Data) != 3 || payload.Data[0]["owned_by"] != "zgi" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestModelsCatalogMergesDuplicateLogicalModels(t *testing.T) {
	models := []adapter.Model{
		{ID: "logical-model", OwnedBy: "provider-a", ContextLength: 64000, SupportedProtocols: []string{modelsProtocolResponses}},
		{ID: "logical-model", OwnedBy: "provider-b", ContextLength: 128000, Capabilities: []string{"thinking"}, SupportedProtocols: []string{modelsProtocolAnthropic}},
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	writeModelsCatalog(c, models)

	var payload struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 1 || payload.Data[0]["id"] != "logical-model" || payload.Data[0]["owned_by"] != "zgi" {
		t.Fatalf("payload = %#v", payload)
	}
	protocols, ok := payload.Data[0]["supported_protocols"].([]any)
	if !ok || len(protocols) != 2 {
		t.Fatalf("supported protocols = %#v", payload.Data[0]["supported_protocols"])
	}
}

func TestModelsCatalogCodexWinsAndFiltersResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models?client_version=0.148.0", nil)
	c.Request.Header.Set("anthropic-version", "2023-06-01")
	writeModelsCatalog(c, preCatalogTestModels())
	var payload struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Models) != 1 || payload.Models[0]["slug"] != "agent-a" || payload.Models[0]["supports_parallel_tool_calls"] != true {
		t.Fatalf("models = %#v", payload.Models)
	}
	if payload.Models[0]["tool_mode"] != nil {
		t.Fatalf("tool_mode = %#v, want null for an uncertified third-party model", payload.Models[0]["tool_mode"])
	}
	for _, key := range []string{"include_skills_usage_instructions", "include_plugin_usage_instructions", "include_apps_usage_instructions"} {
		if payload.Models[0][key] != false {
			t.Fatalf("%s = %#v, want false without explicit capability", key, payload.Models[0][key])
		}
	}
	if payload.Models[0]["multi_agent_version"] != nil {
		t.Fatalf("multi_agent_version = %#v, want null without explicit capability", payload.Models[0]["multi_agent_version"])
	}
	for _, key := range []string{"display_name", "base_instructions", "default_reasoning_level", "supported_reasoning_levels", "shell_type", "visibility", "supported_in_api", "context_window", "max_context_window", "input_modalities"} {
		if _, ok := payload.Models[0][key]; !ok {
			t.Fatalf("Codex model missing %s: %#v", key, payload.Models[0])
		}
	}
}

func TestCodexCatalogOnlyEnablesCodeModeWhenCertified(t *testing.T) {
	model := preCatalogTestModels()[1]
	model.Capabilities = append(model.Capabilities, "codex_code_mode")
	if got := codexCatalogModel(model, 1)["tool_mode"]; got != "code_mode_only" {
		t.Fatalf("tool_mode = %#v, want code_mode_only", got)
	}
}

func TestModelsCatalogAnthropicPaginationAndCapabilities(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models?limit=1", nil)
	c.Request.Header.Set("anthropic-version", "2023-06-01")
	writeModelsCatalog(c, preCatalogTestModels())
	var payload anthropicCatalogResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 1 || payload.Data[0].ID != "agent-a" || !payload.HasMore || payload.FirstID == nil || payload.LastID == nil {
		t.Fatalf("payload = %#v", payload)
	}
	model := payload.Data[0]
	if model.Type != "model" || model.MaxInputTokens != 180000 || model.MaxTokens != 16000 || len(model.Capabilities) != 2 {
		t.Fatalf("model = %#v", model)
	}
}

func TestModelsDetailAnthropicRejectsProtocolMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models/chat-only", nil)
	c.Request.Header.Set("anthropic-version", "2023-06-01")
	c.Params = gin.Params{{Key: "model_id", Value: "chat-only"}}
	writeModelsDetail(c, preCatalogTestModels())
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestModelsCatalogExposesCapabilities(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)

	writeModelsCatalog(c, []adapter.Model{
		{ID: "agent-a", Capabilities: []string{"tool_use", "structured_output"}},
		{ID: "plain-text"},
	})

	var payload struct {
		Data []struct {
			ID           string   `json:"id"`
			Capabilities []string `json:"capabilities"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 2 {
		t.Fatalf("data = %#v", payload.Data)
	}
	if got := payload.Data[0].Capabilities; len(got) != 2 || got[0] != "tool_use" || got[1] != "structured_output" {
		t.Fatalf("agent-a capabilities = %v", got)
	}
	if got := payload.Data[1].Capabilities; len(got) != 1 || got[0] != "messages" {
		t.Fatalf("plain-text capabilities = %v, want the messages fallback", got)
	}
}
