package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestAliyunAdapterChatUsesCompatibleEndpointForStableAndSnapshotModels(t *testing.T) {
	models := []string{"qwen3.8-max", "qwen3.7-plus", "qwen3.7-plus-2026-05-26", "qwen3.7-plus-us"}
	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			var payload map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/compatible-mode/v1/chat/completions" {
					t.Errorf("path = %q, want compatible chat endpoint", r.URL.Path)
				}
				defer r.Body.Close()
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Errorf("decode request: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"id":"chat-1","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
			}))
			defer server.Close()

			a, err := NewAliyunAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: server.URL + "/api/v1"})
			if err != nil {
				t.Fatalf("NewAliyunAdapter() error = %v", err)
			}
			_, err = a.ChatCompletion(context.Background(), &adapter.ChatRequest{
				Model:    model,
				Messages: []adapter.Message{{Role: "user", Content: "hello"}},
			})
			if err != nil {
				t.Fatalf("ChatCompletion() error = %v", err)
			}
			if payload["model"] != model {
				t.Fatalf("model = %#v, want %q", payload["model"], model)
			}
		})
	}
}

func TestAliyunAdapterChatCompatibleEndpointPreservesVisionAndTools(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/compatible-mode/v1/chat/completions" {
			t.Errorf("path = %q, want compatible chat endpoint", r.URL.Path)
		}
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"seen"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()

	a, err := NewAliyunAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: server.URL + "/api/v1"})
	if err != nil {
		t.Fatalf("NewAliyunAdapter() error = %v", err)
	}
	_, err = a.ChatCompletion(context.Background(), &adapter.ChatRequest{
		Model: "qwen3.7-plus",
		Messages: []adapter.Message{{Role: "user", Content: []adapter.MessageContentPart{
			{Type: "text", Text: "describe"},
			{Type: "image_url", ImageURL: &adapter.ImageURL{URL: "data:image/png;base64,abc"}},
		}}},
		Tools: []adapter.Tool{{Type: "function", Function: adapter.Function{Name: "lookup"}}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}

	if tools, ok := payload["tools"].([]any); !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v, want one tool", payload["tools"])
	}
	messages, _ := payload["messages"].([]any)
	message, _ := messages[0].(map[string]any)
	content, _ := message["content"].([]any)
	imagePart, _ := content[1].(map[string]any)
	imageURL, _ := imagePart["image_url"].(map[string]any)
	if imageURL["url"] != "data:image/png;base64,abc" {
		t.Fatalf("image_url = %#v, want data URL", imageURL)
	}
}

func TestAliyunAdapterCompatibleChatRejectsPrivateImageURLBeforeUpstream(t *testing.T) {
	var upstreamCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	a, err := NewAliyunAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: server.URL + "/api/v1"})
	if err != nil {
		t.Fatalf("NewAliyunAdapter() error = %v", err)
	}
	request := &adapter.ChatRequest{
		Model: "qwen3.7-plus",
		Messages: []adapter.Message{{Role: "user", Content: []adapter.MessageContentPart{
			{Type: "image_url", ImageURL: &adapter.ImageURL{URL: "http://127.0.0.1/private.png"}},
		}}},
	}

	if _, err := a.ChatCompletion(context.Background(), request); !errors.Is(err, adapter.ErrInvalidRequest) {
		t.Fatalf("ChatCompletion() error = %v, want invalid request", err)
	}

	// Public JSON binding stores interface-typed content as []interface{}.
	var decodedRequest adapter.ChatRequest
	if err := json.Unmarshal([]byte(`{
		"model":"qwen3.7-plus",
		"messages":[{"role":"user","content":[
			{"type":"image_url","image_url":{"url":"http://localhost/private.png"}}
		]}]
	}`), &decodedRequest); err != nil {
		t.Fatalf("decode public request: %v", err)
	}
	if _, err := a.ChatCompletionStream(context.Background(), &decodedRequest); !errors.Is(err, adapter.ErrInvalidRequest) {
		t.Fatalf("ChatCompletionStream() error = %v, want invalid request", err)
	}
	if upstreamCalls != 0 {
		t.Fatalf("upstream calls = %d, want 0", upstreamCalls)
	}
}

func TestAliyunAdapterChatStreamUsesCompatibleEndpointAndPreservesBillingCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/compatible-mode/v1/chat/completions" {
			t.Errorf("path = %q, want compatible chat endpoint", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"code":"PostpaidBillOverdue","message":"postpaid bill overdue"}}`)
	}))
	defer server.Close()

	a, err := NewAliyunAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: server.URL + "/api/v1"})
	if err != nil {
		t.Fatalf("NewAliyunAdapter() error = %v", err)
	}
	_, err = a.ChatCompletionStream(context.Background(), &adapter.ChatRequest{
		Model:    "qwen3.7-plus",
		Messages: []adapter.Message{{Role: "user", Content: "hello"}},
	})
	if !errors.Is(err, adapter.ErrBillingUnavailable) {
		t.Fatalf("ChatCompletionStream() error = %v, want billing unavailable", err)
	}
}
