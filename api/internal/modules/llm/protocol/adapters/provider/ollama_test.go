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

func TestNormalizeOllamaBaseURL(t *testing.T) {
	t.Helper()

	tests := []struct {
		name       string
		baseURL    string
		wantNative string
		wantOpenAI string
	}{
		{
			name:       "root",
			baseURL:    "http://localhost:11434",
			wantNative: "http://localhost:11434/api",
			wantOpenAI: "http://localhost:11434/v1",
		},
		{
			name:       "native api",
			baseURL:    "http://localhost:11434/api",
			wantNative: "http://localhost:11434/api",
			wantOpenAI: "http://localhost:11434/v1",
		},
		{
			name:       "openai v1",
			baseURL:    "http://localhost:11434/v1/",
			wantNative: "http://localhost:11434/api",
			wantOpenAI: "http://localhost:11434/v1",
		},
		{
			name:       "path prefix",
			baseURL:    "https://proxy.example.com/ollama",
			wantNative: "https://proxy.example.com/ollama/api",
			wantOpenAI: "https://proxy.example.com/ollama/v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeOllamaBaseURL(tt.baseURL)
			if err != nil {
				t.Fatalf("normalizeOllamaBaseURL() error = %v", err)
			}
			if got.native != tt.wantNative {
				t.Fatalf("native = %q, want %q", got.native, tt.wantNative)
			}
			if got.openAI != tt.wantOpenAI {
				t.Fatalf("openAI = %q, want %q", got.openAI, tt.wantOpenAI)
			}
		})
	}
}

func TestOllamaAdapterChatCompletion_UsesOpenAIPathAndOptionalAuth(t *testing.T) {
	t.Helper()

	var gotPath string
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"id":"chatcmpl-ollama-1",
			"object":"chat.completion",
			"created":1732083164,
			"model":"qwen3.5:4b",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}
		}`)
	}))
	defer server.Close()

	a, err := NewOllamaAdapter(&adapter.AdapterConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewOllamaAdapter() error = %v", err)
	}

	resp, err := a.ChatCompletion(context.Background(), &adapter.ChatRequest{
		Model:    "qwen3.5:4b",
		Messages: []adapter.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}

	if gotPath != "/v1/chat/completions" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/chat/completions")
	}
	if gotAuth != "" {
		t.Fatalf("Authorization = %q, want empty", gotAuth)
	}
	if resp.Model != "qwen3.5:4b" {
		t.Fatalf("model = %q, want qwen3.5:4b", resp.Model)
	}
}

func TestOllamaAdapterChatCompletionStream_UsesOpenAIPath(t *testing.T) {
	t.Helper()

	var gotPath string
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			_ = r.Body.Close()
		}()
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"id\":\"chunk-1\",\"object\":\"chat.completion.chunk\",\"model\":\"qwen3.5:4b\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"ok\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	a, err := NewOllamaAdapter(&adapter.AdapterConfig{BaseURL: server.URL + "/api"})
	if err != nil {
		t.Fatalf("NewOllamaAdapter() error = %v", err)
	}

	stream, err := a.ChatCompletionStream(context.Background(), &adapter.ChatRequest{
		Model:    "qwen3.5:4b",
		Messages: []adapter.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletionStream() error = %v", err)
	}

	first := <-stream
	if first.Done || len(first.Choices) != 1 {
		t.Fatalf("first stream response = %#v, want one non-final chunk", first)
	}
	final := <-stream
	if !final.Done {
		t.Fatalf("final stream response = %#v, want done", final)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/chat/completions")
	}
	if gotPayload["stream"] != true {
		t.Fatalf("payload.stream = %#v, want true", gotPayload["stream"])
	}
}

func TestOllamaAdapterCreateEmbeddings_UsesNativeEmbed(t *testing.T) {
	t.Helper()

	var gotPath string
	var gotAuth string
	var gotPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			_ = r.Body.Close()
		}()

		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"model":"nomic-embed-text",
			"embeddings":[[0.1,0.2,0.3]],
			"prompt_eval_count":8
		}`)
	}))
	defer server.Close()

	a, err := NewOllamaAdapter(&adapter.AdapterConfig{
		BaseURL: server.URL + "/v1",
		APIKey:  "proxy-key",
	})
	if err != nil {
		t.Fatalf("NewOllamaAdapter() error = %v", err)
	}

	resp, err := a.CreateEmbeddings(context.Background(), &adapter.EmbeddingsRequest{
		Model:      "nomic-embed-text",
		Input:      "hello",
		Dimensions: 3,
	})
	if err != nil {
		t.Fatalf("CreateEmbeddings() error = %v", err)
	}

	if gotPath != "/api/embed" {
		t.Fatalf("path = %q, want %q", gotPath, "/api/embed")
	}
	if gotAuth != "Bearer proxy-key" {
		t.Fatalf("Authorization = %q, want Bearer proxy-key", gotAuth)
	}
	if gotPayload["model"] != "nomic-embed-text" {
		t.Fatalf("payload.model = %#v, want nomic-embed-text", gotPayload["model"])
	}
	if gotPayload["dimensions"] != float64(3) {
		t.Fatalf("payload.dimensions = %#v, want 3", gotPayload["dimensions"])
	}
	if len(resp.Data) != 1 || len(resp.Data[0].Embedding) != 3 {
		t.Fatalf("embedding response = %#v, want one 3-d embedding", resp.Data)
	}
	if resp.Usage.TotalTokens != 8 {
		t.Fatalf("usage.total_tokens = %d, want 8", resp.Usage.TotalTokens)
	}
}

func TestOllamaAdapterChatCompletion_UsesExactURLWhenBaseURLEndsWithHash(t *testing.T) {
	t.Helper()

	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"id":"chatcmpl-ollama-exact",
			"object":"chat.completion",
			"created":1732083164,
			"model":"qwen3.5:4b",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]
		}`)
	}))
	defer server.Close()

	a, err := NewOllamaAdapter(&adapter.AdapterConfig{BaseURL: server.URL + "/custom-chat-endpoint#"})
	if err != nil {
		t.Fatalf("NewOllamaAdapter() error = %v", err)
	}

	_, err = a.ChatCompletion(context.Background(), &adapter.ChatRequest{
		Model:    "qwen3.5:4b",
		Messages: []adapter.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}

	if gotPath != "/custom-chat-endpoint" {
		t.Fatalf("path = %q, want %q", gotPath, "/custom-chat-endpoint")
	}
}

func TestOllamaAdapterCreateEmbeddings_UsesExactURLWhenBaseURLEndsWithHash(t *testing.T) {
	t.Helper()

	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"model":"nomic-embed-text",
			"embeddings":[[0.1,0.2,0.3]],
			"prompt_eval_count":8
		}`)
	}))
	defer server.Close()

	a, err := NewOllamaAdapter(&adapter.AdapterConfig{BaseURL: server.URL + "/very-custom-embed-path #"})
	if err != nil {
		t.Fatalf("NewOllamaAdapter() error = %v", err)
	}

	_, err = a.CreateEmbeddings(context.Background(), &adapter.EmbeddingsRequest{
		Model: "nomic-embed-text",
		Input: "hello",
	})
	if err != nil {
		t.Fatalf("CreateEmbeddings() error = %v", err)
	}

	if gotPath != "/very-custom-embed-path" {
		t.Fatalf("path = %q, want %q", gotPath, "/very-custom-embed-path")
	}
}

func TestOllamaAdapterListModels_ReturnsUnsupportedWhenBaseURLEndsWithHash(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Fatalf("ListModels() should not call upstream, got %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	a, err := NewOllamaAdapter(&adapter.AdapterConfig{BaseURL: server.URL + "/custom-tags-endpoint#"})
	if err != nil {
		t.Fatalf("NewOllamaAdapter() error = %v", err)
	}

	_, err = a.ListModels(context.Background(), "")
	if !errors.Is(err, adapter.ErrCapabilityUnsupported) {
		t.Fatalf("ListModels() error = %v, want ErrCapabilityUnsupported", err)
	}
}

func TestOllamaAdapterListModels_UsesTagsAndInfersCapabilities(t *testing.T) {
	t.Helper()

	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"models":[
				{"name":"qwen3.5:4b","model":"qwen3.5:4b","modified_at":"2026-04-23T16:26:43Z","details":{"family":"qwen35"}},
				{"name":"nomic-embed-text:latest","model":"nomic-embed-text:latest","modified_at":"2026-04-23T16:26:43Z"},
				{"name":"dengcao/Qwen3-Reranker-8B:Q4_K_M","model":"dengcao/Qwen3-Reranker-8B:Q4_K_M","modified_at":"2026-04-23T16:26:43Z"}
			]
		}`)
	}))
	defer server.Close()

	a, err := NewOllamaAdapter(&adapter.AdapterConfig{BaseURL: server.URL + "/api"})
	if err != nil {
		t.Fatalf("NewOllamaAdapter() error = %v", err)
	}

	models, err := a.ListModels(context.Background(), "")
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}

	if gotPath != "/api/tags" {
		t.Fatalf("path = %q, want %q", gotPath, "/api/tags")
	}
	if len(models) != 3 {
		t.Fatalf("models length = %d, want 3", len(models))
	}
	if !containsValue(models[0].Capabilities, "chat") {
		t.Fatalf("chat model capabilities = %#v, want chat", models[0].Capabilities)
	}
	if !containsValue(models[1].Capabilities, "embedding") {
		t.Fatalf("embedding model capabilities = %#v, want embedding", models[1].Capabilities)
	}
	if models[2].Type != "unsupported" {
		t.Fatalf("reranker model type = %q, want unsupported", models[2].Type)
	}
	if len(models[2].Capabilities) != 0 {
		t.Fatalf("reranker model capabilities = %#v, want none", models[2].Capabilities)
	}
}

func TestOllamaAdapterRerank_IsUnsupported(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Fatalf("Rerank() should not call upstream, got %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	a, err := NewOllamaAdapter(&adapter.AdapterConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewOllamaAdapter() error = %v", err)
	}

	_, err = a.Rerank(context.Background(), &adapter.RerankRequest{
		Model:     "reranker",
		Query:     "hello",
		Documents: []string{"doc"},
	})
	if !errors.Is(err, adapter.ErrCapabilityUnsupported) {
		t.Fatalf("Rerank() error = %v, want ErrCapabilityUnsupported", err)
	}
}

func containsValue(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
