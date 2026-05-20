package provider_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
	provider "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters/provider"
)

func TestOpenAICompatibleHashBaseURL_UsesExactChatEndpoint(t *testing.T) {
	t.Helper()

	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.URL.Path != "/proxy/chat" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/proxy/chat")
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": 123,
			"model":   "gpt-4o-mini",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "ok",
					},
					"finish_reason": "stop",
				},
			},
		})
	}))
	defer server.Close()

	a, err := provider.NewOpenAIAdapter(&adapter.AdapterConfig{
		ProviderName: "openai-compatible",
		APIKey:       "test-key",
		BaseURL:      server.URL + "/proxy/chat#",
	})
	if err != nil {
		t.Fatalf("NewOpenAIAdapter() error = %v", err)
	}

	_, err = a.ChatCompletion(context.Background(), &adapter.ChatRequest{
		Model: "gpt-4o-mini",
		Messages: []adapter.Message{
			{Role: "user", Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}

	if gotPath != "/proxy/chat" {
		t.Fatalf("path = %q, want %q", gotPath, "/proxy/chat")
	}
}

func TestOpenAICompatibleHashBaseURL_TrimsWhitespaceBeforeMarker(t *testing.T) {
	t.Helper()

	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.URL.Path != "/proxy/chat" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/proxy/chat")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-test",
			"object":"chat.completion",
			"created":123,
			"model":"gpt-4o-mini",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]
		}`))
	}))
	defer server.Close()

	a, err := provider.NewOpenAIAdapter(&adapter.AdapterConfig{
		ProviderName: "openai-compatible",
		APIKey:       "test-key",
		BaseURL:      server.URL + "/proxy/chat #",
	})
	if err != nil {
		t.Fatalf("NewOpenAIAdapter() error = %v", err)
	}

	_, err = a.ChatCompletion(context.Background(), &adapter.ChatRequest{
		Model: "gpt-4o-mini",
		Messages: []adapter.Message{
			{Role: "user", Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}

	if gotPath != "/proxy/chat" {
		t.Fatalf("path = %q, want %q", gotPath, "/proxy/chat")
	}
}

func TestOpenAICompatibleHashBaseURL_ListModelsReturnsCapabilityUnsupported(t *testing.T) {
	t.Helper()

	requested := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case requested <- struct{}{}:
		default:
		}
		t.Fatalf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	a, err := provider.NewOpenAIAdapter(&adapter.AdapterConfig{
		ProviderName: "openai-compatible",
		APIKey:       "test-key",
		BaseURL:      server.URL + "/proxy/chat#",
	})
	if err != nil {
		t.Fatalf("NewOpenAIAdapter() error = %v", err)
	}

	_, err = a.ListModels(context.Background(), "runtime-key")
	if !errors.Is(err, adapter.ErrCapabilityUnsupported) {
		t.Fatalf("ListModels() error = %v, want ErrCapabilityUnsupported", err)
	}

	select {
	case <-requested:
		t.Fatal("ListModels() should not issue an upstream request in exact endpoint mode")
	default:
	}
}

func TestOpenAICompatibleHashBaseURL_RejectsEmptyEndpoint(t *testing.T) {
	t.Helper()

	_, err := provider.NewOpenAIAdapter(&adapter.AdapterConfig{
		ProviderName: "openai-compatible",
		APIKey:       "test-key",
		BaseURL:      " # ",
	})
	if !errors.Is(err, adapter.ErrInvalidConfig) {
		t.Fatalf("NewOpenAIAdapter() error = %v, want ErrInvalidConfig", err)
	}
}
