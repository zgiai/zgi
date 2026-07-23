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

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestGLMAdapterChatCompletion_UsesOfficialEndpointAndPayload(t *testing.T) {
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
			"model":"glm-5",
			"choices":[
				{
					"index":0,
					"message":{"role":"assistant","content":"ok"},
					"finish_reason":"stop"
				}
			],
			"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}
		}`)
	}))
	defer server.Close()

	a, err := NewGLMAdapter(&adapter.AdapterConfig{
		APIKey:  "config-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewGLMAdapter() error = %v", err)
	}

	resp, err := a.ChatCompletion(context.Background(), &adapter.ChatRequest{
		Model: "glm-5",
		Messages: []adapter.Message{
			{
				Role: "user",
				Content: []adapter.MessageContentPart{
					{Type: "image_url", ImageURL: &adapter.ImageURL{URL: "https://example.com/cat.png"}},
					{Type: "text", Text: "describe"},
				},
			},
		},
		AdditionalParameters: map[string]any{
			"thinking":    map[string]any{"type": "enabled"},
			"tool_stream": true,
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
	if got := gotPayload["model"]; got != "glm-5" {
		t.Fatalf("model = %#v, want %q", got, "glm-5")
	}
	if got := gotPayload["tool_stream"]; got != true {
		t.Fatalf("tool_stream = %#v, want true", got)
	}
	thinking, ok := gotPayload["thinking"].(map[string]any)
	if !ok || thinking["type"] != "enabled" {
		t.Fatalf("thinking = %#v, want type=enabled", gotPayload["thinking"])
	}
	if resp.Model != "glm-5" {
		t.Fatalf("response model = %q, want %q", resp.Model, "glm-5")
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 18 {
		t.Fatalf("usage = %+v, want total=18", resp.Usage)
	}
}

func TestGLMAdapterCreateEmbeddings_UsesOfficialEndpoint(t *testing.T) {
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
			"object":"list",
			"model":"embedding-3",
			"data":[{"object":"embedding","index":0,"embedding":[0.1,0.2,0.3]}],
			"usage":{"prompt_tokens":7,"total_tokens":7}
		}`)
	}))
	defer server.Close()

	a, err := NewGLMAdapter(&adapter.AdapterConfig{
		APIKey:  "config-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewGLMAdapter() error = %v", err)
	}

	resp, err := a.CreateEmbeddings(context.Background(), &adapter.EmbeddingsRequest{
		Model:      "embedding-3",
		Input:      "hello",
		Dimensions: 512,
	})
	if err != nil {
		t.Fatalf("CreateEmbeddings() error = %v", err)
	}

	if gotAuth != "Bearer config-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer config-key")
	}
	if gotPath != "/embeddings" {
		t.Fatalf("path = %q, want %q", gotPath, "/embeddings")
	}
	if got := gotPayload["dimensions"]; got != float64(512) {
		t.Fatalf("dimensions = %#v, want %d", got, 512)
	}
	if resp.Model != "embedding-3" {
		t.Fatalf("response model = %q, want %q", resp.Model, "embedding-3")
	}
	if len(resp.Data) != 1 || len(resp.Data[0].Embedding) != 3 {
		t.Fatalf("response data = %#v, want one 3-d vector", resp.Data)
	}
}

func TestGLMAdapterCreateImage_UsesOfficialEndpoint(t *testing.T) {
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
			"created":1732083164,
			"data":[{"url":"https://cdn.bigmodel.cn/fake.png"}]
		}`)
	}))
	defer server.Close()

	a, err := NewGLMAdapter(&adapter.AdapterConfig{
		APIKey:  "config-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewGLMAdapter() error = %v", err)
	}

	resp, err := a.CreateImage(context.Background(), &adapter.ImageRequest{
		Model:  "glm-image",
		Prompt: "a cat",
		Size:   "1280x1280",
		AdditionalParameters: map[string]any{
			"watermark_enabled": true,
		},
	})
	if err != nil {
		t.Fatalf("CreateImage() error = %v", err)
	}

	if gotAuth != "Bearer config-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer config-key")
	}
	if gotPath != "/images/generations" {
		t.Fatalf("path = %q, want %q", gotPath, "/images/generations")
	}
	if got := gotPayload["watermark_enabled"]; got != true {
		t.Fatalf("watermark_enabled = %#v, want true", got)
	}
	if len(resp.Data) != 1 || resp.Data[0].URL != "https://cdn.bigmodel.cn/fake.png" {
		t.Fatalf("response data = %#v, want one generated image url", resp.Data)
	}
}

func TestGLMAdapterRerank_UsesOfficialSchema(t *testing.T) {
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
			"id":"rerank-1",
			"created":1732083164,
			"results":[
				{"index":1,"document":"second","relevance_score":0.97},
				{"index":0,"document":"first","relevance_score":0.66}
			],
			"usage":{"prompt_tokens":12,"total_tokens":12}
		}`)
	}))
	defer server.Close()

	a, err := NewGLMAdapter(&adapter.AdapterConfig{
		APIKey:  "config-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewGLMAdapter() error = %v", err)
	}

	returnDocuments := true
	resp, err := a.Rerank(context.Background(), &adapter.RerankRequest{
		Model:           "rerank",
		Query:           "hello",
		Documents:       []string{"first", "second"},
		TopN:            intPtrGLMTest(1),
		ReturnDocuments: &returnDocuments,
	})
	if err != nil {
		t.Fatalf("Rerank() error = %v", err)
	}

	if gotAuth != "Bearer config-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer config-key")
	}
	if gotPath != "/rerank" {
		t.Fatalf("path = %q, want %q", gotPath, "/rerank")
	}
	if got := gotPayload["top_n"]; got != float64(1) {
		t.Fatalf("top_n = %#v, want %d", got, 1)
	}
	if got := gotPayload["return_documents"]; got != true {
		t.Fatalf("return_documents = %#v, want true", got)
	}
	if resp.ID != "rerank-1" {
		t.Fatalf("response ID = %q, want %q", resp.ID, "rerank-1")
	}
	if len(resp.Results) != 2 {
		t.Fatalf("results len = %d, want 2", len(resp.Results))
	}
	if resp.Results[0].Text != "second" || resp.Results[0].Document != "second" {
		t.Fatalf("results[0] = %#v, want returned document text", resp.Results[0])
	}
	if resp.Usage == nil || resp.Usage.PromptTokens != 12 || resp.Usage.TotalTokens != 12 {
		t.Fatalf("usage = %+v, want prompt=12 total=12", resp.Usage)
	}
}

func TestGLMAdapterRerank_RejectsStructuredDocuments(t *testing.T) {
	t.Helper()

	a, err := NewGLMAdapter(&adapter.AdapterConfig{APIKey: "test-key"})
	if err != nil {
		t.Fatalf("NewGLMAdapter() error = %v", err)
	}

	_, err = a.Rerank(context.Background(), &adapter.RerankRequest{
		Model:     "rerank",
		Query:     "hello",
		Documents: []map[string]any{{"text": "hello"}},
	})
	if !errors.Is(err, adapter.ErrInvalidRequest) {
		t.Fatalf("Rerank() error = %v, want ErrInvalidRequest", err)
	}
}

func TestGLMAdapterListModels_UsesRemoteModelsAndDecisionSafeCapabilities(t *testing.T) {
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
		fmt.Fprint(w, `{
			"object":"list",
			"data":[
				{"id":"glm-5","object":"model","owned_by":"zhipu"},
				{"id":"glm-4.6v","object":"model","owned_by":"zhipu"},
				{"id":"glm-image","object":"model","owned_by":"zhipu"},
				{"id":"embedding-3","object":"model","owned_by":"zhipu"},
				{"id":"rerank","object":"model","owned_by":"zhipu"}
			]
		}`)
	}))
	defer server.Close()

	a, err := NewGLMAdapter(&adapter.AdapterConfig{
		APIKey:  "config-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewGLMAdapter() error = %v", err)
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

	glm5 := findGLMModel(t, models, "glm-5")
	if glm5.Type != "chat" {
		t.Fatalf("glm-5 type = %q, want %q", glm5.Type, "chat")
	}
	if !containsGLMValue(glm5.Capabilities, "chat") || !containsGLMValue(glm5.Capabilities, "stream") {
		t.Fatalf("glm-5 capabilities = %#v, want chat+stream", glm5.Capabilities)
	}
	if containsGLMValue(glm5.Capabilities, "image") || containsGLMValue(glm5.Capabilities, "embedding") || containsGLMValue(glm5.Capabilities, "rerank") {
		t.Fatalf("glm-5 capabilities = %#v, should not expose unrelated capabilities", glm5.Capabilities)
	}

	vision := findGLMModel(t, models, "glm-4.6v")
	if vision.Type != "chat" {
		t.Fatalf("glm-4.6v type = %q, want %q", vision.Type, "chat")
	}
	if !containsGLMValue(vision.Capabilities, "chat") {
		t.Fatalf("glm-4.6v capabilities = %#v, want chat", vision.Capabilities)
	}
	if containsGLMValue(vision.Capabilities, "image") {
		t.Fatalf("glm-4.6v capabilities = %#v, should not be treated as image generation", vision.Capabilities)
	}
	if vision.Architecture == nil || !containsGLMValue(vision.Architecture.InputModalities, "image") || !containsGLMValue(vision.Architecture.InputModalities, "video") || !containsGLMValue(vision.Architecture.InputModalities, "file") {
		t.Fatalf("glm-4.6v input modalities = %#v, want image/video/file support", vision.Architecture)
	}

	imageModel := findGLMModel(t, models, "glm-image")
	if imageModel.Type != "image" {
		t.Fatalf("glm-image type = %q, want %q", imageModel.Type, "image")
	}
	if !containsGLMValue(imageModel.Capabilities, "image") || containsGLMValue(imageModel.Capabilities, "chat") {
		t.Fatalf("glm-image capabilities = %#v, want only image-related support", imageModel.Capabilities)
	}

	embeddingModel := findGLMModel(t, models, "embedding-3")
	if embeddingModel.Type != "embedding" || !containsGLMValue(embeddingModel.Capabilities, "embedding") {
		t.Fatalf("embedding-3 = %#v, want embedding capability", embeddingModel)
	}

	rerankModel := findGLMModel(t, models, "rerank")
	if rerankModel.Type != "rerank" || !containsGLMValue(rerankModel.Capabilities, "rerank") {
		t.Fatalf("rerank = %#v, want rerank capability", rerankModel)
	}
}

func TestGLMAdapterListModels_FallsBackToOfficialCatalogWhenModelsEndpointUnsupported(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/models")
		}
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":{"code":"not_found","message":"not found"}}`)
	}))
	defer server.Close()

	a, err := NewGLMAdapter(&adapter.AdapterConfig{
		APIKey:  "config-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewGLMAdapter() error = %v", err)
	}

	models, err := a.ListModels(context.Background(), "runtime-key")
	if err != nil {
		t.Fatalf("ListModels() error = %v, want documented fallback", err)
	}

	if findGLMModel(t, models, "glm-5.2").Type != "chat" {
		t.Fatalf("glm-5.2 missing from fallback catalog: %#v", models)
	}
	if findGLMModel(t, models, "glm-5.1").Type != "chat" {
		t.Fatalf("glm-5.1 missing from fallback catalog: %#v", models)
	}
	if findGLMModel(t, models, "glm-5").Type != "chat" {
		t.Fatalf("glm-5 missing from fallback catalog: %#v", models)
	}
	if findGLMModel(t, models, "glm-image").Type != "image" {
		t.Fatalf("glm-image missing from fallback catalog: %#v", models)
	}
	if findGLMModel(t, models, "embedding-3").Type != "embedding" {
		t.Fatalf("embedding-3 missing from fallback catalog: %#v", models)
	}
	if findGLMModel(t, models, "rerank").Type != "rerank" {
		t.Fatalf("rerank missing from fallback catalog: %#v", models)
	}
}

func TestGLMAdapterListModels_DoesNotMaskAuthFailure(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"code":"invalid_api_key","message":"bad key"}}`)
	}))
	defer server.Close()

	a, err := NewGLMAdapter(&adapter.AdapterConfig{
		APIKey:  "config-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewGLMAdapter() error = %v", err)
	}

	_, err = a.ListModels(context.Background(), "runtime-key")
	if !errors.Is(err, adapter.ErrAuthFailed) {
		t.Fatalf("ListModels() error = %v, want ErrAuthFailed", err)
	}
}

func TestNewAdapter_CreatesGLMAdapterFromFactory(t *testing.T) {
	t.Helper()

	instance, err := adapter.NewAdapter(&adapter.AdapterConfig{
		ProviderName: "glm",
		APIKey:       "test-key",
	})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}
	if _, ok := instance.(*GLMAdapter); !ok {
		t.Fatalf("adapter type = %T, want *GLMAdapter", instance)
	}
}

func intPtrGLMTest(v int) *int {
	return &v
}

func findGLMModel(t *testing.T, models []adapter.Model, id string) adapter.Model {
	t.Helper()
	for _, model := range models {
		if model.ID == id {
			return model
		}
	}
	t.Fatalf("model %q not found in %#v", id, models)
	return adapter.Model{}
}

func containsGLMValue(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}
