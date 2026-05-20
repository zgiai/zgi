package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestNativeRawProviders_ResponseEndpoints(t *testing.T) {
	tests := []struct {
		name       string
		baseSuffix string
		wantPath   string
		newAdapter func(string) (adapter.RawResponseCapable, error)
	}{
		{
			name:       "qwen",
			baseSuffix: "/compatible-mode/v1",
			wantPath:   "/compatible-mode/v1/responses",
			newAdapter: func(baseURL string) (adapter.RawResponseCapable, error) {
				return NewAliyunAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: baseURL})
			},
		},
		{
			name:       "doubao",
			baseSuffix: "/api/v3",
			wantPath:   "/api/v3/responses",
			newAdapter: func(baseURL string) (adapter.RawResponseCapable, error) {
				return NewDoubaoAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: baseURL})
			},
		},
		{
			name:       "ollama",
			baseSuffix: "",
			wantPath:   "/v1/responses",
			newAdapter: func(baseURL string) (adapter.RawResponseCapable, error) {
				return NewOllamaAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: baseURL})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				gotPath    string
				gotAuth    string
				gotPayload map[string]any
			)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer r.Body.Close()
				gotPath = r.URL.Path
				gotAuth = r.Header.Get("Authorization")
				if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
					t.Fatalf("decode request body: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"id":"resp_1","object":"response","status":"completed","usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}`)
			}))
			defer server.Close()

			rawCapable, err := tt.newAdapter(server.URL + tt.baseSuffix)
			if err != nil {
				t.Fatalf("new adapter error = %v", err)
			}
			resp, err := rawCapable.CreateResponseRaw(context.Background(), &adapter.RawResponseRequest{
				Model: "test-model",
				Body:  json.RawMessage(`{"model":"test-model","input":[{"role":"user","content":"hello"}],"tools":[{"type":"function","name":"lookup"}]}`),
			})
			if err != nil {
				t.Fatalf("CreateResponseRaw() error = %v", err)
			}

			if gotPath != tt.wantPath {
				t.Fatalf("path = %q, want %q", gotPath, tt.wantPath)
			}
			if gotAuth != "Bearer test-key" {
				t.Fatalf("Authorization = %q, want Bearer test-key", gotAuth)
			}
			if gotPayload["tools"] == nil {
				t.Fatalf("payload missing tools: %#v", gotPayload)
			}
			if resp.Usage == nil || resp.Usage.PromptTokens != 3 || resp.Usage.CompletionTokens != 2 {
				t.Fatalf("usage = %+v, want prompt=3 completion=2", resp.Usage)
			}
		})
	}
}

func TestNativeRawProviders_AnthropicMessagesEndpoints(t *testing.T) {
	tests := []struct {
		name       string
		wantPath   string
		wantAuth   string
		newAdapter func(string) (adapter.AnthropicMessagesCapable, error)
	}{
		{
			name:     "qwen",
			wantPath: "/apps/anthropic/v1/messages",
			wantAuth: "x-api-key",
			newAdapter: func(baseURL string) (adapter.AnthropicMessagesCapable, error) {
				return NewAliyunAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: baseURL + "/compatible-mode/v1"})
			},
		},
		{
			name:     "openrouter",
			wantPath: "/api/v1/messages",
			wantAuth: "bearer",
			newAdapter: func(baseURL string) (adapter.AnthropicMessagesCapable, error) {
				return NewOpenRouterAdapter(&adapter.AdapterConfig{
					APIKey:       "test-key",
					BaseURL:      baseURL + "/api/v1",
					CustomParams: map[string]any{},
				})
			},
		},
		{
			name:     "deepseek",
			wantPath: "/anthropic/v1/messages",
			wantAuth: "x-api-key",
			newAdapter: func(baseURL string) (adapter.AnthropicMessagesCapable, error) {
				return NewDeepSeekAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: baseURL + "/v1"})
			},
		},
		{
			name:     "minimax",
			wantPath: "/v1/messages",
			wantAuth: "x-api-key",
			newAdapter: func(baseURL string) (adapter.AnthropicMessagesCapable, error) {
				return NewMiniMaxAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: baseURL + "/v1"})
			},
		},
		{
			name:     "kimi",
			wantPath: "/anthropic/v1/messages",
			wantAuth: "bearer",
			newAdapter: func(baseURL string) (adapter.AnthropicMessagesCapable, error) {
				return NewMoonshotAIAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: baseURL + "/v1"})
			},
		},
		{
			name:     "zai",
			wantPath: "/api/anthropic/v1/messages",
			wantAuth: "bearer",
			newAdapter: func(baseURL string) (adapter.AnthropicMessagesCapable, error) {
				return NewGLMAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: baseURL + "/api/paas/v4"})
			},
		},
		{
			name:     "ollama",
			wantPath: "/v1/messages",
			wantAuth: "x-api-key",
			newAdapter: func(baseURL string) (adapter.AnthropicMessagesCapable, error) {
				return NewOllamaAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: baseURL})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				gotPath          string
				gotAPIKey        string
				gotAuthorization string
				gotVersion       string
				gotPayload       map[string]any
			)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer r.Body.Close()
				gotPath = r.URL.Path
				gotAPIKey = r.Header.Get("x-api-key")
				gotAuthorization = r.Header.Get("Authorization")
				gotVersion = r.Header.Get("anthropic-version")
				if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
					t.Fatalf("decode request body: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":4,"output_tokens":3}}`)
			}))
			defer server.Close()

			messagesCapable, err := tt.newAdapter(server.URL)
			if err != nil {
				t.Fatalf("new adapter error = %v", err)
			}
			resp, err := messagesCapable.CreateAnthropicMessage(context.Background(), &adapter.AnthropicMessageRequest{
				Model: "test-model",
				Body:  json.RawMessage(`{"model":"test-model","max_tokens":64,"messages":[{"role":"user","content":"hello"}],"tools":[{"name":"lookup","input_schema":{"type":"object"}}]}`),
			})
			if err != nil {
				t.Fatalf("CreateAnthropicMessage() error = %v", err)
			}

			if gotPath != tt.wantPath {
				t.Fatalf("path = %q, want %q", gotPath, tt.wantPath)
			}
			switch tt.wantAuth {
			case "x-api-key":
				if gotAPIKey != "test-key" {
					t.Fatalf("x-api-key = %q, want test-key", gotAPIKey)
				}
				if gotAuthorization != "" {
					t.Fatalf("Authorization = %q, want empty", gotAuthorization)
				}
			case "bearer":
				if gotAuthorization != "Bearer test-key" {
					t.Fatalf("Authorization = %q, want Bearer test-key", gotAuthorization)
				}
				if gotAPIKey != "" {
					t.Fatalf("x-api-key = %q, want empty", gotAPIKey)
				}
			default:
				t.Fatalf("unknown wantAuth %q", tt.wantAuth)
			}
			if gotVersion != "2023-06-01" {
				t.Fatalf("anthropic-version = %q, want 2023-06-01", gotVersion)
			}
			if gotPayload["tools"] == nil {
				t.Fatalf("payload missing tools: %#v", gotPayload)
			}
			if resp.Usage == nil || resp.Usage.PromptTokens != 4 || resp.Usage.CompletionTokens != 3 {
				t.Fatalf("usage = %+v, want prompt=4 completion=3", resp.Usage)
			}
		})
	}
}

func TestNativeRawProviders_GLMDefaultAnthropicMessagesBaseURL(t *testing.T) {
	a := &GLMAdapter{baseURL: defaultGLMBaseURL}

	got := a.anthropicMessagesBaseURL()
	if got != defaultGLMAnthropicMessagesURL {
		t.Fatalf("anthropicMessagesBaseURL() = %q, want %q", got, defaultGLMAnthropicMessagesURL)
	}
}

func TestNativeRawProviders_AnthropicStreamPreservesEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if r.URL.Path != "/anthropic/v1/messages" {
			t.Fatalf("path = %q, want /anthropic/v1/messages", r.URL.Path)
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer does not support flushing")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: message_start\n")
		fmt.Fprint(w, "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":4,\"output_tokens\":0}}}\n\n")
		fmt.Fprint(w, "event: content_block_delta\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}\n\n")
		fmt.Fprint(w, "event: message_stop\n")
		fmt.Fprint(w, "data: {\"type\":\"message_stop\"}\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	a, err := NewDeepSeekAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: server.URL + "/v1"})
	if err != nil {
		t.Fatalf("NewDeepSeekAdapter() error = %v", err)
	}
	stream, err := a.CreateAnthropicMessageStream(context.Background(), &adapter.AnthropicMessageRequest{
		Model: "deepseek-chat",
		Body:  json.RawMessage(`{"model":"deepseek-chat","stream":true,"max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`),
	})
	if err != nil {
		t.Fatalf("CreateAnthropicMessageStream() error = %v", err)
	}

	var events []string
	for event := range stream {
		if event.Error != nil {
			t.Fatalf("stream event error = %v", event.Error)
		}
		if event.Done {
			continue
		}
		events = append(events, event.Event)
		if strings.Contains(string(event.Data), `"choices"`) {
			t.Fatalf("stream data contains chat choices: %s", string(event.Data))
		}
	}

	want := []string{"message_start", "content_block_delta", "message_stop"}
	if strings.Join(events, ",") != strings.Join(want, ",") {
		t.Fatalf("events = %v, want %v", events, want)
	}
}
