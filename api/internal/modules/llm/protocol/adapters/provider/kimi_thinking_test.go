package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// zgi-pre#2913: Kimi only reasons with thinking.type. reasoning_effort follows
// the console's thinking_type dialect on private channels too.

var (
	kimiThinkingEnabled  = map[string]interface{}{"type": "enabled"}
	kimiThinkingDisabled = map[string]interface{}{"type": "disabled"}
)

func kimiChatRequest(model string, extras map[string]interface{}) *adapter.ChatRequest {
	return &adapter.ChatRequest{
		Model:                model,
		Messages:             []adapter.Message{{Role: "user", Content: "hi"}},
		AdditionalParameters: extras,
	}
}

func TestOpenAICompatibleChatPayload_KimiReasoningEffortMapsToThinking(t *testing.T) {
	tests := []struct {
		model string
		level string
		want  map[string]interface{}
	}{
		{"kimi-k2.6", "high", kimiThinkingEnabled},
		{"kimi-k2.6", "medium", kimiThinkingEnabled},
		{"kimi-k2.6", "low", kimiThinkingEnabled},
		{"kimi-k2.6", "minimal", kimiThinkingEnabled},
		{"kimi-k2.6", "none", kimiThinkingDisabled},
		{"Kimi-K2.6", " HIGH ", kimiThinkingEnabled},
		{"moonshot/kimi-k2.6", "high", kimiThinkingEnabled},
	}
	for _, tt := range tests {
		t.Run(tt.model+"/"+tt.level, func(t *testing.T) {
			payload := buildOpenAICompatibleChatPayload(kimiChatRequest(tt.model, map[string]interface{}{"reasoning_effort": tt.level}))
			if !reflect.DeepEqual(payload["thinking"], tt.want) {
				t.Fatalf("thinking = %#v, want %#v", payload["thinking"], tt.want)
			}
			if _, ok := payload["reasoning"]; ok {
				t.Fatalf("reasoning = %#v, want absent for Kimi", payload["reasoning"])
			}
			if _, ok := payload["reasoning_effort"]; ok {
				t.Fatalf("reasoning_effort = %#v, want absent for Kimi", payload["reasoning_effort"])
			}
		})
	}
}

func TestOpenAICompatibleChatPayload_NonKimiKeepsReasoningEffortMapping(t *testing.T) {
	for _, model := range []string{"qwen3.8-27b", "deepseek-v4-flash", "gpt-5.2", "moonshot-v1-8k"} {
		payload := buildOpenAICompatibleChatPayload(kimiChatRequest(model, map[string]interface{}{"reasoning_effort": "high"}))
		if !reflect.DeepEqual(payload["reasoning"], map[string]interface{}{"effort": "high"}) {
			t.Fatalf("%s: reasoning = %#v, want the existing reasoning.effort mapping", model, payload["reasoning"])
		}
		if _, ok := payload["thinking"]; ok {
			t.Fatalf("%s: thinking = %#v, want absent", model, payload["thinking"])
		}
	}
}

func TestOpenAICompatibleChatPayload_KimiWithoutReasoningEffortIsUnchanged(t *testing.T) {
	request := kimiChatRequest("kimi-k2.6", nil)
	payload := buildOpenAICompatibleChatPayload(request)
	if _, ok := payload["thinking"]; ok {
		t.Fatalf("thinking = %#v, want absent by default", payload["thinking"])
	}
	if _, ok := payload["reasoning"]; ok {
		t.Fatalf("reasoning = %#v, want absent by default", payload["reasoning"])
	}
	if !reflect.DeepEqual(payload, buildOpenAICompatibleChatCorePayload(request)) {
		t.Fatalf("payload = %#v, want exactly the typed fields", payload)
	}

	other := buildOpenAICompatibleChatPayload(kimiChatRequest("kimi-k2.6", map[string]interface{}{"top_k": 5}))
	if other["top_k"] != 5 {
		t.Fatalf("top_k = %#v, want passthrough", other["top_k"])
	}
	if _, ok := other["thinking"]; ok {
		t.Fatalf("thinking = %#v, want absent without reasoning_effort", other["thinking"])
	}
}

func TestOpenAICompatibleChatPayload_KimiNativeThinkingWins(t *testing.T) {
	payload := buildOpenAICompatibleChatPayload(kimiChatRequest("kimi-k2.6", map[string]interface{}{
		"reasoning_effort": "none",
		"thinking":         map[string]interface{}{"type": "enabled"},
	}))
	if !reflect.DeepEqual(payload["thinking"], kimiThinkingEnabled) {
		t.Fatalf("thinking = %#v, want the caller's native value", payload["thinking"])
	}
}

func TestOpenAICompatibleChatPayload_KimiUnknownLevelKeepsExistingMapping(t *testing.T) {
	payload := buildOpenAICompatibleChatPayload(kimiChatRequest("kimi-k2.6", map[string]interface{}{"reasoning_effort": "max"}))
	if !reflect.DeepEqual(payload["reasoning"], map[string]interface{}{"effort": "max"}) {
		t.Fatalf("reasoning = %#v, want the existing mapping for a non-unified level", payload["reasoning"])
	}
	if _, ok := payload["thinking"]; ok {
		t.Fatalf("thinking = %#v, want absent", payload["thinking"])
	}
}

// capturingChatServer records every request body and answers a minimal chat
// completion, or a minimal SSE stream when the request asks for one.
func capturingChatServer(t *testing.T) (*httptest.Server, func() []map[string]interface{}) {
	t.Helper()
	var mu sync.Mutex
	var bodies []map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]interface{}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("decode upstream body: %v", err)
		}
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		if body["stream"] == true {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"id\":\"c\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"kimi-k2.6\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"c","object":"chat.completion","created":1,"model":"kimi-k2.6","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	return server, func() []map[string]interface{} {
		mu.Lock()
		defer mu.Unlock()
		return append([]map[string]interface{}(nil), bodies...)
	}
}

func drainStream(t *testing.T, stream <-chan adapter.StreamResponse) {
	t.Helper()
	for chunk := range stream {
		if chunk.Error != nil {
			t.Fatalf("stream error: %v", chunk.Error)
		}
	}
}

type kimiChatAdapter interface {
	ChatCompletion(ctx context.Context, request *adapter.ChatRequest) (*adapter.ChatResponse, error)
	ChatCompletionStream(ctx context.Context, request *adapter.ChatRequest) (<-chan adapter.StreamResponse, error)
}

func assertKimiThinkingThroughAdapter(t *testing.T, newAdapter func(baseURL string) kimiChatAdapter) {
	t.Helper()
	server, bodies := capturingChatServer(t)
	defer server.Close()
	client := newAdapter(server.URL)

	for _, tt := range []struct {
		level string
		want  map[string]interface{}
	}{
		{"high", kimiThinkingEnabled},
		{"none", kimiThinkingDisabled},
	} {
		if _, err := client.ChatCompletion(context.Background(), kimiChatRequest("kimi-k2.6", map[string]interface{}{"reasoning_effort": tt.level})); err != nil {
			t.Fatalf("%s: ChatCompletion() error = %v", tt.level, err)
		}
		stream, err := client.ChatCompletionStream(context.Background(), kimiChatRequest("kimi-k2.6", map[string]interface{}{"reasoning_effort": tt.level}))
		if err != nil {
			t.Fatalf("%s: ChatCompletionStream() error = %v", tt.level, err)
		}
		drainStream(t, stream)

		got := bodies()
		for _, body := range got[len(got)-2:] {
			if !reflect.DeepEqual(body["thinking"], tt.want) {
				t.Fatalf("%s: upstream thinking = %#v, want %#v (body %#v)", tt.level, body["thinking"], tt.want, body)
			}
			if _, ok := body["reasoning"]; ok {
				t.Fatalf("%s: upstream body carries reasoning: %#v", tt.level, body)
			}
		}
	}

	if _, err := client.ChatCompletion(context.Background(), kimiChatRequest("kimi-k2.6", nil)); err != nil {
		t.Fatalf("default ChatCompletion() error = %v", err)
	}
	got := bodies()
	last := got[len(got)-1]
	for _, key := range []string{"thinking", "reasoning", "reasoning_effort"} {
		if _, ok := last[key]; ok {
			t.Fatalf("default request carries %q: %#v", key, last)
		}
	}
}

func TestAgictoAdapter_KimiReasoningEffortReachesUpstreamAsThinking(t *testing.T) {
	assertKimiThinkingThroughAdapter(t, func(baseURL string) kimiChatAdapter {
		client, err := NewAgictoAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: baseURL + "/v1"})
		if err != nil {
			t.Fatalf("NewAgictoAdapter() error = %v", err)
		}
		return client
	})
}
