package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

const doubaoSeedreamLiteTestModel = doubaoSeedreamModelPrefix + "-5-0-lite-260128"

func TestDoubaoAdapterChatCompletion_UsesArkChatCompletions(t *testing.T) {
	t.Helper()

	var (
		gotAuth string
		gotPath string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id":"chatcmpl-doubao-1",
			"object":"chat.completion",
			"created":1732083164,
			"model":"doubao-seed-1-6-250615",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}
		}`)
	}))
	defer server.Close()

	a, err := NewDoubaoAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v3",
	})
	if err != nil {
		t.Fatalf("NewDoubaoAdapter() error = %v", err)
	}

	resp, err := a.ChatCompletion(context.Background(), &adapter.ChatRequest{
		Model:    "doubao-seed-1-6-250615",
		Messages: []adapter.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}

	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotPath != "/api/v3/chat/completions" {
		t.Fatalf("path = %q, want %q", gotPath, "/api/v3/chat/completions")
	}
	if resp.Model != "doubao-seed-1-6-250615" {
		t.Fatalf("response model = %q, want %q", resp.Model, "doubao-seed-1-6-250615")
	}
}

func TestDoubaoAdapterCreateResponse_UsesArkResponses(t *testing.T) {
	t.Helper()

	var (
		gotAuth string
		gotPath string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id":"resp_123",
			"object":"response",
			"created_at":1732083164,
			"model":"doubao-seed-1-6-250615",
			"status":"completed",
			"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],
			"usage":{"input_tokens":11,"output_tokens":7,"total_tokens":18}
		}`)
	}))
	defer server.Close()

	a, err := NewDoubaoAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v3",
	})
	if err != nil {
		t.Fatalf("NewDoubaoAdapter() error = %v", err)
	}

	resp, err := a.CreateResponse(context.Background(), &adapter.CreateResponseRequest{
		Model: "doubao-seed-1-6-250615",
		Input: "hello",
	})
	if err != nil {
		t.Fatalf("CreateResponse() error = %v", err)
	}

	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotPath != "/api/v3/responses" {
		t.Fatalf("path = %q, want %q", gotPath, "/api/v3/responses")
	}
	if resp.Model != "doubao-seed-1-6-250615" {
		t.Fatalf("response model = %q, want %q", resp.Model, "doubao-seed-1-6-250615")
	}
}

func TestDoubaoAdapterCreateEmbeddings_UsesArkEmbeddings(t *testing.T) {
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
			"model":"doubao-embedding-text-240715",
			"data":[{"object":"embedding","index":0,"embedding":[0.1,0.2,0.3]}],
			"usage":{"prompt_tokens":7,"total_tokens":7}
		}`)
	}))
	defer server.Close()

	a, err := NewDoubaoAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v3",
	})
	if err != nil {
		t.Fatalf("NewDoubaoAdapter() error = %v", err)
	}

	resp, err := a.CreateEmbeddings(context.Background(), &adapter.EmbeddingsRequest{
		Model:      "doubao-embedding-text-240715",
		Input:      "hello",
		Dimensions: 1024,
	})
	if err != nil {
		t.Fatalf("CreateEmbeddings() error = %v", err)
	}

	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotPath != "/api/v3/embeddings" {
		t.Fatalf("path = %q, want %q", gotPath, "/api/v3/embeddings")
	}
	if got := gotPayload["dimensions"]; got != float64(1024) {
		t.Fatalf("payload.dimensions = %#v, want %d", got, 1024)
	}
	if resp.Model != "doubao-embedding-text-240715" {
		t.Fatalf("response model = %q, want %q", resp.Model, "doubao-embedding-text-240715")
	}
}

func TestDoubaoAdapterCreateImage_UsesArkImagesAndSeedreamNormalization(t *testing.T) {
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
			"data":[{"url":"https://cdn.example.com/image.png"}]
		}`)
	}))
	defer server.Close()

	a, err := NewDoubaoAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v3",
	})
	if err != nil {
		t.Fatalf("NewDoubaoAdapter() error = %v", err)
	}

	resp, err := a.CreateImage(context.Background(), &adapter.ImageRequest{
		Model:  doubaoSeedreamLiteTestModel,
		Prompt: "a cat",
		Size:   "1024x1024",
	})
	if err != nil {
		t.Fatalf("CreateImage() error = %v", err)
	}

	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotPath != "/api/v3/images/generations" {
		t.Fatalf("path = %q, want %q", gotPath, "/api/v3/images/generations")
	}
	if got := gotPayload["size"]; got != "1920x1920" {
		t.Fatalf("payload.size = %#v, want %q", got, "1920x1920")
	}
	if _, exists := gotPayload[doubaoSeedreamSequentialGenerationKey]; exists {
		t.Fatalf("payload.%s exists for nil N: %#v", doubaoSeedreamSequentialGenerationKey, gotPayload[doubaoSeedreamSequentialGenerationKey])
	}
	if _, exists := gotPayload[doubaoSeedreamSequentialOptionsKey]; exists {
		t.Fatalf("payload.%s exists for nil N: %#v", doubaoSeedreamSequentialOptionsKey, gotPayload[doubaoSeedreamSequentialOptionsKey])
	}
	if len(resp.Data) != 1 || resp.Data[0].URL != "https://cdn.example.com/image.png" {
		t.Fatalf("response data = %#v, want generated image url", resp.Data)
	}
}

func TestDoubaoAdapterCreateImage_SeedreamMultiImageUsesSequentialOptions(t *testing.T) {
	t.Helper()

	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"created":1732083164,
			"data":[
				{"url":"https://cdn.example.com/image-1.png"},
				{"url":"https://cdn.example.com/image-2.png"},
				{"url":"https://cdn.example.com/image-3.png"}
			]
		}`)
	}))
	defer server.Close()

	a, err := NewDoubaoAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v3",
	})
	if err != nil {
		t.Fatalf("NewDoubaoAdapter() error = %v", err)
	}

	n := 3
	resp, err := a.CreateImage(context.Background(), &adapter.ImageRequest{
		Model:  doubaoSeedreamLiteTestModel,
		Prompt: "a cat",
		Size:   "1024x1024",
		N:      &n,
	})
	if err != nil {
		t.Fatalf("CreateImage() error = %v", err)
	}

	if got := gotPayload[doubaoImagePayloadKeyN]; got != float64(n) {
		t.Fatalf("payload.%s = %#v, want %d", doubaoImagePayloadKeyN, got, n)
	}
	if got := gotPayload[doubaoImagePayloadKeyPrompt]; got != buildDoubaoSeedreamImagePrompt("a cat", n, true) {
		t.Fatalf("payload.%s = %#v, want multi-image prompt", doubaoImagePayloadKeyPrompt, got)
	}
	if got := gotPayload[doubaoSeedreamSequentialGenerationKey]; got != doubaoSeedreamSequentialGenerationAuto {
		t.Fatalf("payload.%s = %#v, want %q", doubaoSeedreamSequentialGenerationKey, got, doubaoSeedreamSequentialGenerationAuto)
	}
	options, ok := gotPayload[doubaoSeedreamSequentialOptionsKey].(map[string]any)
	if !ok {
		t.Fatalf("payload.%s = %#v, want object", doubaoSeedreamSequentialOptionsKey, gotPayload[doubaoSeedreamSequentialOptionsKey])
	}
	if got := options[doubaoSeedreamSequentialMaxImagesKey]; got != float64(n) {
		t.Fatalf("payload.%s.%s = %#v, want %d", doubaoSeedreamSequentialOptionsKey, doubaoSeedreamSequentialMaxImagesKey, got, n)
	}
	if len(resp.Data) != n {
		t.Fatalf("response data length = %d, want %d", len(resp.Data), n)
	}
}

func TestDoubaoAdapterCreateImage_SeedreamSingleImageDoesNotUseSequentialOptions(t *testing.T) {
	t.Helper()

	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"created":1732083164,
			"data":[{"url":"https://cdn.example.com/image.png"}]
		}`)
	}))
	defer server.Close()

	a, err := NewDoubaoAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v3",
	})
	if err != nil {
		t.Fatalf("NewDoubaoAdapter() error = %v", err)
	}

	n := 1
	_, err = a.CreateImage(context.Background(), &adapter.ImageRequest{
		Model:  doubaoSeedreamLiteTestModel,
		Prompt: "a cat",
		Size:   "1024x1024",
		N:      &n,
	})
	if err != nil {
		t.Fatalf("CreateImage() error = %v", err)
	}

	if got := gotPayload[doubaoImagePayloadKeyN]; got != float64(n) {
		t.Fatalf("payload.%s = %#v, want %d", doubaoImagePayloadKeyN, got, n)
	}
	if got := gotPayload[doubaoImagePayloadKeyPrompt]; got != "a cat" {
		t.Fatalf("payload.%s = %#v, want %q", doubaoImagePayloadKeyPrompt, got, "a cat")
	}
	if _, exists := gotPayload[doubaoSeedreamSequentialGenerationKey]; exists {
		t.Fatalf("payload.%s exists for N=1: %#v", doubaoSeedreamSequentialGenerationKey, gotPayload[doubaoSeedreamSequentialGenerationKey])
	}
	if _, exists := gotPayload[doubaoSeedreamSequentialOptionsKey]; exists {
		t.Fatalf("payload.%s exists for N=1: %#v", doubaoSeedreamSequentialOptionsKey, gotPayload[doubaoSeedreamSequentialOptionsKey])
	}
}

func TestDoubaoAdapterCreateImage_AdditionalParametersOverrideSeedreamSequentialOptions(t *testing.T) {
	t.Helper()

	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"created":1732083164,
			"data":[{"url":"https://cdn.example.com/image.png"}]
		}`)
	}))
	defer server.Close()

	a, err := NewDoubaoAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v3",
	})
	if err != nil {
		t.Fatalf("NewDoubaoAdapter() error = %v", err)
	}

	n := 3
	const overrideSequentialGeneration = "disabled"
	const overrideMaxImages = 1
	_, err = a.CreateImage(context.Background(), &adapter.ImageRequest{
		Model:  doubaoSeedreamLiteTestModel,
		Prompt: "a cat",
		Size:   "1024x1024",
		N:      &n,
		AdditionalParameters: map[string]any{
			doubaoSeedreamSequentialGenerationKey: overrideSequentialGeneration,
			doubaoSeedreamSequentialOptionsKey: map[string]any{
				doubaoSeedreamSequentialMaxImagesKey: overrideMaxImages,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateImage() error = %v", err)
	}

	if got := gotPayload[doubaoSeedreamSequentialGenerationKey]; got != overrideSequentialGeneration {
		t.Fatalf("payload.%s = %#v, want %q", doubaoSeedreamSequentialGenerationKey, got, overrideSequentialGeneration)
	}
	options, ok := gotPayload[doubaoSeedreamSequentialOptionsKey].(map[string]any)
	if !ok {
		t.Fatalf("payload.%s = %#v, want object", doubaoSeedreamSequentialOptionsKey, gotPayload[doubaoSeedreamSequentialOptionsKey])
	}
	if got := options[doubaoSeedreamSequentialMaxImagesKey]; got != float64(overrideMaxImages) {
		t.Fatalf("payload.%s.%s = %#v, want %d", doubaoSeedreamSequentialOptionsKey, doubaoSeedreamSequentialMaxImagesKey, got, overrideMaxImages)
	}
}

func TestDoubaoAdapterCreateImage_NonSeedreamMultiImageDoesNotAppendPrompt(t *testing.T) {
	t.Helper()

	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"created":1732083164,
			"data":[{"url":"https://cdn.example.com/image.png"}]
		}`)
	}))
	defer server.Close()

	a, err := NewDoubaoAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v3",
	})
	if err != nil {
		t.Fatalf("NewDoubaoAdapter() error = %v", err)
	}

	n := 3
	_, err = a.CreateImage(context.Background(), &adapter.ImageRequest{
		Model:  "doubao-image-model",
		Prompt: "a cat",
		Size:   "1024x1024",
		N:      &n,
	})
	if err != nil {
		t.Fatalf("CreateImage() error = %v", err)
	}

	if got := gotPayload[doubaoImagePayloadKeyPrompt]; got != "a cat" {
		t.Fatalf("payload.%s = %#v, want %q", doubaoImagePayloadKeyPrompt, got, "a cat")
	}
	if _, exists := gotPayload[doubaoSeedreamSequentialGenerationKey]; exists {
		t.Fatalf("payload.%s exists for non-seedream model: %#v", doubaoSeedreamSequentialGenerationKey, gotPayload[doubaoSeedreamSequentialGenerationKey])
	}
	if _, exists := gotPayload[doubaoSeedreamSequentialOptionsKey]; exists {
		t.Fatalf("payload.%s exists for non-seedream model: %#v", doubaoSeedreamSequentialOptionsKey, gotPayload[doubaoSeedreamSequentialOptionsKey])
	}
}

func TestDoubaoAdapterListModels_NormalizesRemoteCatalog(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/models" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/api/v3/models")
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"data":[
				{"id":"doubao-seed-1-6-250615","created":1732083164,"owned_by":"bytedance"},
				{"id":"doubao-embedding-text-240715","created":1732083164,"owned_by":"bytedance"},
				{"id":"doubao-seedream-5-0-lite-260128","created":1732083164,"owned_by":"bytedance"}
			]
		}`)
	}))
	defer server.Close()

	a, err := NewDoubaoAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v3",
	})
	if err != nil {
		t.Fatalf("NewDoubaoAdapter() error = %v", err)
	}

	models, err := a.ListModels(context.Background(), "runtime-key")
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(models) != 3 {
		t.Fatalf("len(models) = %d, want 3", len(models))
	}
	if models[0].Type != "chat" {
		t.Fatalf("models[0].Type = %q, want %q", models[0].Type, "chat")
	}
	if got := models[0].Capabilities; len(got) == 0 || got[0] != "chat" {
		t.Fatalf("models[0].Capabilities = %#v, want chat capabilities", got)
	}
	if models[1].Type != "embedding" {
		t.Fatalf("models[1].Type = %q, want %q", models[1].Type, "embedding")
	}
	if models[2].Type != "image" {
		t.Fatalf("models[2].Type = %q, want %q", models[2].Type, "image")
	}
}

func TestDoubaoAdapterListModels_UnsupportedEndpointReturnsCapabilityUnsupported(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":{"message":"not found","type":"not_found_error","code":"not_found"}}`)
	}))
	defer server.Close()

	a, err := NewDoubaoAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v3",
	})
	if err != nil {
		t.Fatalf("NewDoubaoAdapter() error = %v", err)
	}

	_, err = a.ListModels(context.Background(), "runtime-key")
	if !errors.Is(err, adapter.ErrCapabilityUnsupported) {
		t.Fatalf("ListModels() error = %v, want ErrCapabilityUnsupported", err)
	}
}

func TestDoubaoAdapterGetProviderInfo(t *testing.T) {
	a, err := NewDoubaoAdapter(&adapter.AdapterConfig{
		APIKey: "test-key",
	})
	if err != nil {
		t.Fatalf("NewDoubaoAdapter() error = %v", err)
	}

	info := a.GetProviderInfo()
	if info == nil {
		t.Fatal("GetProviderInfo() = nil, want non-nil")
	}
	if info.Name != "doubao" {
		t.Fatalf("info.Name = %q, want %q", info.Name, "doubao")
	}
	if info.BaseURL != doubaoDefaultBaseURL {
		t.Fatalf("info.BaseURL = %q, want %q", info.BaseURL, doubaoDefaultBaseURL)
	}
}
