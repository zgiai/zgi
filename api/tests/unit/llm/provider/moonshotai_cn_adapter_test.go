package provider_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	provider "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters/provider"
)

func TestMoonshotAICNAdapterGetProviderInfo_UsesDocumentedEndpointAndCapabilities(t *testing.T) {
	t.Helper()

	a, err := provider.NewMoonshotAICNAdapter(&adapter.AdapterConfig{
		APIKey: "test-key",
	})
	if err != nil {
		t.Fatalf("NewMoonshotAICNAdapter() error = %v", err)
	}

	info := a.GetProviderInfo()
	if info == nil {
		t.Fatal("GetProviderInfo() = nil, want non-nil")
	}

	if info.BaseURL != "https://api.moonshot.cn/v1" {
		t.Fatalf("BaseURL = %q, want %q", info.BaseURL, "https://api.moonshot.cn/v1")
	}
	if !containsStringValue(info.Capabilities, "chat") {
		t.Fatalf("Capabilities = %#v, want to contain %q", info.Capabilities, "chat")
	}
	if !containsStringValue(info.Capabilities, "stream") {
		t.Fatalf("Capabilities = %#v, want to contain %q", info.Capabilities, "stream")
	}
	if !containsStringValue(info.Capabilities, "model_listing") {
		t.Fatalf("Capabilities = %#v, want to contain %q", info.Capabilities, "model_listing")
	}
	if containsStringValue(info.Capabilities, "completion") ||
		containsStringValue(info.Capabilities, "embedding") ||
		containsStringValue(info.Capabilities, "image") ||
		containsStringValue(info.Capabilities, "rerank") {
		t.Fatalf("Capabilities = %#v, should not advertise undocumented capabilities", info.Capabilities)
	}
}

func TestMoonshotAICNAdapterChatCompletion_PassesProviderSpecificParameters(t *testing.T) {
	t.Helper()

	var gotPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/chat/completions")
		}
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-test",
			"object":"chat.completion",
			"created":123,
			"model":"kimi-k2.5",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]
		}`))
	}))
	defer server.Close()

	a, err := provider.NewMoonshotAICNAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewMoonshotAICNAdapter() error = %v", err)
	}

	_, err = a.ChatCompletion(context.Background(), &adapter.ChatRequest{
		Model: "kimi-k2.5",
		Messages: []adapter.Message{
			{Role: "user", Content: "hello"},
		},
		AdditionalParameters: map[string]interface{}{
			"thinking": map[string]interface{}{"type": "disabled"},
		},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}

	if _, ok := gotPayload["thinking"]; !ok {
		t.Fatalf("request payload = %#v, want provider-specific parameter %q to be forwarded", gotPayload, "thinking")
	}
}

func TestMoonshotAICNAdapterUnsupportedCapabilities(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	a, err := provider.NewMoonshotAICNAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewMoonshotAICNAdapter() error = %v", err)
	}

	t.Run("embeddings", func(t *testing.T) {
		_, err := a.CreateEmbeddings(context.Background(), &adapter.EmbeddingsRequest{
			Model: "text-embedding-v4",
			Input: "hello",
		})
		if !errors.Is(err, adapter.ErrCapabilityUnsupported) {
			t.Fatalf("CreateEmbeddings() error = %v, want ErrCapabilityUnsupported", err)
		}
	})

	t.Run("image", func(t *testing.T) {
		_, err := a.CreateImage(context.Background(), &adapter.ImageRequest{
			Model:  "kimi-k2.5",
			Prompt: "hello",
		})
		if !errors.Is(err, adapter.ErrCapabilityUnsupported) {
			t.Fatalf("CreateImage() error = %v, want ErrCapabilityUnsupported", err)
		}
	})

	t.Run("rerank", func(t *testing.T) {
		_, err := a.Rerank(context.Background(), &adapter.RerankRequest{
			Model:     "kimi-k2.5",
			Query:     "hello",
			Documents: []string{"a", "b"},
		})
		if !errors.Is(err, adapter.ErrCapabilityUnsupported) {
			t.Fatalf("Rerank() error = %v, want ErrCapabilityUnsupported", err)
		}
	})
}

func TestMoonshotAICNAdapterListModels_NormalizesDocumentedCapabilities(t *testing.T) {
	t.Helper()

	var gotAuth string
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		if r.URL.Path != "/models" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/models")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"object": "list",
			"data": [
				{"id": "moonshot-v1-128k-vision-preview", "object": "model", "owned_by": "moonshot"},
				{"id": "kimi-k2.5", "object": "model", "owned_by": "moonshot"},
				{"id": "kimi-k2-turbo-preview", "object": "model", "owned_by": "moonshot"}
			]
		}`))
	}))
	defer server.Close()

	a, err := provider.NewMoonshotAICNAdapter(&adapter.AdapterConfig{
		APIKey:  "config-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewMoonshotAICNAdapter() error = %v", err)
	}

	models, err := a.ListModels(context.Background(), "runtime-key")
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}

	if gotAuth != "Bearer runtime-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer runtime-key")
	}
	if gotPath != "/models" {
		t.Fatalf("path = %q, want %q", gotPath, "/models")
	}
	if len(models) != 3 {
		t.Fatalf("len(models) = %d, want 3", len(models))
	}

	visionModel := models[0]
	if visionModel.ID != "moonshot-v1-128k-vision-preview" {
		t.Fatalf("models[0].ID = %q, want %q", visionModel.ID, "moonshot-v1-128k-vision-preview")
	}
	if visionModel.Type != "chat" {
		t.Fatalf("models[0].Type = %q, want %q", visionModel.Type, "chat")
	}
	if visionModel.ContextLength != 131072 {
		t.Fatalf("models[0].ContextLength = %d, want %d", visionModel.ContextLength, 131072)
	}
	assertMoonshotChatOnlyCapabilities(t, visionModel.Capabilities)
	assertMoonshotArchitectureContains(t, visionModel.Architecture, "text", "image")

	k2p5Model := models[1]
	if k2p5Model.ID != "kimi-k2.5" {
		t.Fatalf("models[1].ID = %q, want %q", k2p5Model.ID, "kimi-k2.5")
	}
	if k2p5Model.ContextLength != 262144 {
		t.Fatalf("models[1].ContextLength = %d, want %d", k2p5Model.ContextLength, 262144)
	}
	assertMoonshotChatOnlyCapabilities(t, k2p5Model.Capabilities)
	assertMoonshotArchitectureContains(t, k2p5Model.Architecture, "text", "image")

	k2TurboModel := models[2]
	if k2TurboModel.ID != "kimi-k2-turbo-preview" {
		t.Fatalf("models[2].ID = %q, want %q", k2TurboModel.ID, "kimi-k2-turbo-preview")
	}
	if k2TurboModel.ContextLength != 262144 {
		t.Fatalf("models[2].ContextLength = %d, want %d", k2TurboModel.ContextLength, 262144)
	}
	assertMoonshotChatOnlyCapabilities(t, k2TurboModel.Capabilities)
	assertMoonshotArchitectureContains(t, k2TurboModel.Architecture, "text")
	if containsStringValue(k2TurboModel.Architecture.InputModalities, "image") {
		t.Fatalf("models[2].Architecture.InputModalities = %#v, text-only K2 model should not advertise image input", k2TurboModel.Architecture.InputModalities)
	}
}

func assertMoonshotChatOnlyCapabilities(t *testing.T, capabilities []string) {
	t.Helper()

	if !containsStringValue(capabilities, "chat") {
		t.Fatalf("Capabilities = %#v, want to contain %q", capabilities, "chat")
	}
	if !containsStringValue(capabilities, "stream") {
		t.Fatalf("Capabilities = %#v, want to contain %q", capabilities, "stream")
	}
	if containsStringValue(capabilities, "embedding") ||
		containsStringValue(capabilities, "image") ||
		containsStringValue(capabilities, "rerank") {
		t.Fatalf("Capabilities = %#v, should not expose undocumented capabilities", capabilities)
	}
}

func assertMoonshotArchitectureContains(t *testing.T, architecture *adapter.ModelArchitecture, requiredInputModalities ...string) {
	t.Helper()

	if architecture == nil {
		t.Fatal("Architecture = nil, want non-nil")
	}
	if architecture.Modality != "text" {
		t.Fatalf("Architecture.Modality = %q, want %q", architecture.Modality, "text")
	}
	for _, modality := range requiredInputModalities {
		if !containsStringValue(architecture.InputModalities, modality) {
			t.Fatalf("Architecture.InputModalities = %#v, want to contain %q", architecture.InputModalities, modality)
		}
	}
	if !containsStringValue(architecture.OutputModalities, "text") {
		t.Fatalf("Architecture.OutputModalities = %#v, want to contain %q", architecture.OutputModalities, "text")
	}
}

func containsStringValue(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
