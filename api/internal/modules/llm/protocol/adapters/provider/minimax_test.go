package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

func TestMiniMaxAdapterGetProviderInfo_UsesDocumentedCapabilities(t *testing.T) {
	t.Helper()

	a, err := NewMiniMaxAdapter(&adapter.AdapterConfig{
		APIKey: "test-key",
	})
	if err != nil {
		t.Fatalf("NewMiniMaxAdapter() error = %v", err)
	}

	info := a.GetProviderInfo()
	if info == nil {
		t.Fatal("GetProviderInfo() = nil, want non-nil")
	}
	if info.BaseURL != "https://api.minimaxi.com/v1" {
		t.Fatalf("BaseURL = %q, want %q", info.BaseURL, "https://api.minimaxi.com/v1")
	}
	if !containsMiniMaxValue(info.Capabilities, "chat") ||
		!containsMiniMaxValue(info.Capabilities, "stream") ||
		!containsMiniMaxValue(info.Capabilities, "image") ||
		!containsMiniMaxValue(info.Capabilities, "model_listing") {
		t.Fatalf("Capabilities = %#v, want chat+stream+image+model_listing", info.Capabilities)
	}
	if containsMiniMaxValue(info.Capabilities, "embedding") ||
		containsMiniMaxValue(info.Capabilities, "rerank") ||
		containsMiniMaxValue(info.Capabilities, "completion") {
		t.Fatalf("Capabilities = %#v, should not advertise undocumented capabilities", info.Capabilities)
	}
}

func TestMiniMaxAdapterChatCompletion_UsesOpenAICompatibleEndpoint(t *testing.T) {
	t.Helper()

	var (
		gotAuth    string
		gotPath    string
		gotPayload map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path

		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id":"chatcmpl-1",
			"object":"chat.completion",
			"created":1732083164,
			"model":"MiniMax-M2.5",
			"choices":[
				{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}
			],
			"usage":{"prompt_tokens":9,"completion_tokens":4,"total_tokens":13}
		}`)
	}))
	defer server.Close()

	a, err := NewMiniMaxAdapter(&adapter.AdapterConfig{
		APIKey:  "config-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewMiniMaxAdapter() error = %v", err)
	}

	resp, err := a.ChatCompletion(context.Background(), &adapter.ChatRequest{
		Model: "MiniMax-M2.5",
		Messages: []adapter.Message{
			{Role: "user", Content: "hello"},
		},
		AdditionalParameters: map[string]any{
			"reasoning_split": true,
		},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}

	if gotAuth != "Bearer config-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer config-key")
	}
	if gotPath != "/chat/completions" {
		t.Fatalf("path = %q, want %q", gotPath, "/chat/completions")
	}
	if got := gotPayload["reasoning_split"]; got != true {
		t.Fatalf("reasoning_split = %#v, want true", got)
	}
	if resp.Model != "MiniMax-M2.5" {
		t.Fatalf("response model = %q, want %q", resp.Model, "MiniMax-M2.5")
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 13 {
		t.Fatalf("usage = %+v, want total=13", resp.Usage)
	}
}

func TestMiniMaxAdapterCreateImage_UsesOfficialEndpoint(t *testing.T) {
	t.Helper()

	var (
		gotAuth    string
		gotPath    string
		gotPayload map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path

		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"base_resp":{"status_code":0,"status_msg":"success"},
			"data":{
				"image_urls":["https://cdn.minimaxi.com/image-1.png"],
				"image_base64":["ZmFrZS1pbWFnZQ=="]
			}
		}`)
	}))
	defer server.Close()

	a, err := NewMiniMaxAdapter(&adapter.AdapterConfig{
		APIKey:  "config-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewMiniMaxAdapter() error = %v", err)
	}

	resp, err := a.CreateImage(context.Background(), &adapter.ImageRequest{
		Model:  "image-01",
		Prompt: "a cat astronaut",
		AdditionalParameters: map[string]any{
			"aspect_ratio":      "1:1",
			"subject_reference": []map[string]any{{"type": "image_url", "image_url": "https://example.com/cat.png"}},
		},
	})
	if err != nil {
		t.Fatalf("CreateImage() error = %v", err)
	}

	if gotAuth != "Bearer config-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer config-key")
	}
	if gotPath != "/image_generation" {
		t.Fatalf("path = %q, want %q", gotPath, "/image_generation")
	}
	if got := gotPayload["aspect_ratio"]; got != "1:1" {
		t.Fatalf("aspect_ratio = %#v, want %q", got, "1:1")
	}
	if _, ok := gotPayload["subject_reference"]; !ok {
		t.Fatalf("payload = %#v, want subject_reference to be forwarded", gotPayload)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("len(resp.Data) = %d, want 2", len(resp.Data))
	}
	if resp.Data[0].URL != "https://cdn.minimaxi.com/image-1.png" {
		t.Fatalf("resp.Data[0].URL = %q, want generated image url", resp.Data[0].URL)
	}
	if resp.Data[1].B64JSON != "ZmFrZS1pbWFnZQ==" {
		t.Fatalf("resp.Data[1].B64JSON = %q, want base64 image data", resp.Data[1].B64JSON)
	}
}

func TestMiniMaxAdapterUnsupportedCapabilities(t *testing.T) {
	t.Helper()

	a, err := NewMiniMaxAdapter(&adapter.AdapterConfig{APIKey: "test-key"})
	if err != nil {
		t.Fatalf("NewMiniMaxAdapter() error = %v", err)
	}

	t.Run("responses", func(t *testing.T) {
		_, err := a.CreateResponse(context.Background(), &adapter.CreateResponseRequest{Model: "MiniMax-M1"})
		if !errors.Is(err, adapter.ErrCapabilityUnsupported) {
			t.Fatalf("CreateResponse() error = %v, want ErrCapabilityUnsupported", err)
		}
	})

	t.Run("embeddings", func(t *testing.T) {
		_, err := a.CreateEmbeddings(context.Background(), &adapter.EmbeddingsRequest{
			Model: "text-embedding",
			Input: "hello",
		})
		if !errors.Is(err, adapter.ErrCapabilityUnsupported) {
			t.Fatalf("CreateEmbeddings() error = %v, want ErrCapabilityUnsupported", err)
		}
	})

	t.Run("rerank", func(t *testing.T) {
		_, err := a.Rerank(context.Background(), &adapter.RerankRequest{
			Model:     "rerank",
			Query:     "hello",
			Documents: []string{"a", "b"},
		})
		if !errors.Is(err, adapter.ErrCapabilityUnsupported) {
			t.Fatalf("Rerank() error = %v, want ErrCapabilityUnsupported", err)
		}
	})
}

func TestMiniMaxAdapterListModels_UsesRemoteModelsAndDecisionSafeCapabilities(t *testing.T) {
	t.Helper()

	var (
		gotAuth string
		gotPath string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path

		if r.URL.Path != "/models" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/models")
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"object":"list",
			"data":[
				{"id":"MiniMax-M1","object":"model","owned_by":"minimax"},
				{"id":"MiniMax-M2.5-highspeed","object":"model","owned_by":"minimax"},
				{"id":"MiniMax-VL-01","object":"model","owned_by":"minimax"},
				{"id":"image-01","object":"model","owned_by":"minimax"}
			]
		}`)
	}))
	defer server.Close()

	a, err := NewMiniMaxAdapter(&adapter.AdapterConfig{
		APIKey:  "config-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewMiniMaxAdapter() error = %v", err)
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

	m25 := findMiniMaxModel(t, models, "MiniMax-M2.5-highspeed")
	if m25.Type != "chat" {
		t.Fatalf("MiniMax-M2.5-highspeed type = %q, want %q", m25.Type, "chat")
	}
	if !containsMiniMaxValue(m25.Capabilities, "chat") || !containsMiniMaxValue(m25.Capabilities, "stream") {
		t.Fatalf("MiniMax-M2.5-highspeed capabilities = %#v, want chat+stream", m25.Capabilities)
	}
	if containsMiniMaxValue(m25.Capabilities, "image") {
		t.Fatalf("MiniMax-M2.5-highspeed capabilities = %#v, text model should not advertise image generation", m25.Capabilities)
	}

	vl := findMiniMaxModel(t, models, "MiniMax-VL-01")
	if vl.Type != "chat" {
		t.Fatalf("MiniMax-VL-01 type = %q, want %q", vl.Type, "chat")
	}
	if !containsMiniMaxValue(vl.Capabilities, "chat") || !containsMiniMaxValue(vl.Capabilities, "stream") {
		t.Fatalf("MiniMax-VL-01 capabilities = %#v, want chat+stream", vl.Capabilities)
	}
	if containsMiniMaxValue(vl.Capabilities, "image") {
		t.Fatalf("MiniMax-VL-01 capabilities = %#v, vision chat should not be treated as image generation", vl.Capabilities)
	}
	if vl.Architecture == nil || !containsMiniMaxValue(vl.Architecture.InputModalities, "image") || !containsMiniMaxValue(vl.Architecture.InputModalities, "video") {
		t.Fatalf("MiniMax-VL-01 architecture = %#v, want multimodal image/video input", vl.Architecture)
	}

	imageModel := findMiniMaxModel(t, models, "image-01")
	if imageModel.Type != "image" {
		t.Fatalf("image-01 type = %q, want %q", imageModel.Type, "image")
	}
	if !containsMiniMaxValue(imageModel.Capabilities, "image") || containsMiniMaxValue(imageModel.Capabilities, "chat") {
		t.Fatalf("image-01 capabilities = %#v, want image only", imageModel.Capabilities)
	}
}

func TestMiniMaxAdapterListModels_FallsBackToDocumentedCatalogWhenModelsEndpointUnsupported(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/models")
		}
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":{"code":"not_found","message":"not found"}}`)
	}))
	defer server.Close()

	a, err := NewMiniMaxAdapter(&adapter.AdapterConfig{
		APIKey:  "config-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewMiniMaxAdapter() error = %v", err)
	}

	models, err := a.ListModels(context.Background(), "runtime-key")
	if err != nil {
		t.Fatalf("ListModels() error = %v, want documented fallback", err)
	}

	if findMiniMaxModel(t, models, "MiniMax-M2.5").Type != "chat" {
		t.Fatalf("MiniMax-M2.5 missing from fallback catalog: %#v", models)
	}
	if findMiniMaxModel(t, models, "MiniMax-M2.1-highspeed").Type != "chat" {
		t.Fatalf("MiniMax-M2.1-highspeed missing from fallback catalog: %#v", models)
	}
	if findMiniMaxModel(t, models, "image-01-live").Type != "image" {
		t.Fatalf("image-01-live missing from fallback catalog: %#v", models)
	}
}

func TestMiniMaxAdapterListModels_DoesNotMaskAuthFailure(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"code":"invalid_api_key","message":"bad key"}}`)
	}))
	defer server.Close()

	a, err := NewMiniMaxAdapter(&adapter.AdapterConfig{
		APIKey:  "config-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewMiniMaxAdapter() error = %v", err)
	}

	_, err = a.ListModels(context.Background(), "runtime-key")
	if !errors.Is(err, adapter.ErrAuthFailed) {
		t.Fatalf("ListModels() error = %v, want ErrAuthFailed", err)
	}
}

func TestNewAdapter_CreatesMiniMaxAdapterFromFactory(t *testing.T) {
	t.Helper()

	instance, err := adapter.NewAdapter(&adapter.AdapterConfig{
		ProviderName: "minimax",
		APIKey:       "test-key",
	})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}
	if _, ok := instance.(*MiniMaxAdapter); !ok {
		t.Fatalf("adapter type = %T, want *MiniMaxAdapter", instance)
	}
}

func findMiniMaxModel(t *testing.T, models []adapter.Model, id string) adapter.Model {
	t.Helper()
	for _, model := range models {
		if model.ID == id {
			return model
		}
	}
	t.Fatalf("model %q not found in %#v", id, models)
	return adapter.Model{}
}

func containsMiniMaxValue(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}
