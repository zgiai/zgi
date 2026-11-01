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

// zgi-pre#2913 was narrowed to the platform channels: the console translates
// reasoning_effort per selected channel. Private OpenAI-compatible channels do
// no model-specific mapping: reasoning_effort always becomes reasoning.effort,
// and every other extra, a native thinking value included, passes through.

var reasoningEffortTestModels = []string{"kimi-k2.6", "qwen3.8-27b", "deepseek-v4-flash", "gpt-5.2", "moonshot-v1-8k"}

func extrasChatRequest(model string, extras map[string]interface{}) *adapter.ChatRequest {
	return &adapter.ChatRequest{
		Model:                model,
		Messages:             []adapter.Message{{Role: "user", Content: "hi"}},
		AdditionalParameters: extras,
	}
}

func TestOpenAICompatibleChatPayload_ReasoningEffortMapsToReasoningForEveryModel(t *testing.T) {
	for _, model := range reasoningEffortTestModels {
		for _, level := range []string{"high", "none"} {
			payload := buildOpenAICompatibleChatPayload(extrasChatRequest(model, map[string]interface{}{"reasoning_effort": level}))
			if !reflect.DeepEqual(payload["reasoning"], map[string]interface{}{"effort": level}) {
				t.Fatalf("%s/%s: reasoning = %#v, want the reasoning.effort mapping", model, level, payload["reasoning"])
			}
			if _, ok := payload["thinking"]; ok {
				t.Fatalf("%s/%s: thinking = %#v, want absent (no model-specific mapping)", model, level, payload["thinking"])
			}
		}
	}
}

func TestOpenAICompatibleChatPayload_WithoutReasoningEffortIsUnchanged(t *testing.T) {
	for _, model := range reasoningEffortTestModels {
		request := extrasChatRequest(model, nil)
		payload := buildOpenAICompatibleChatPayload(request)
		if !reflect.DeepEqual(payload, buildOpenAICompatibleChatCorePayload(request)) {
			t.Fatalf("%s: payload = %#v, want exactly the typed fields", model, payload)
		}

		other := buildOpenAICompatibleChatPayload(extrasChatRequest(model, map[string]interface{}{"top_k": 5}))
		if other["top_k"] != 5 {
			t.Fatalf("%s: top_k = %#v, want passthrough", model, other["top_k"])
		}
		for _, key := range []string{"thinking", "reasoning"} {
			if _, ok := other[key]; ok {
				t.Fatalf("%s: %s = %#v, want absent without reasoning_effort", model, key, other[key])
			}
		}
	}
}

func TestOpenAICompatibleChatPayload_NativeThinkingPassesThrough(t *testing.T) {
	for _, model := range reasoningEffortTestModels {
		payload := buildOpenAICompatibleChatPayload(extrasChatRequest(model, map[string]interface{}{
			"reasoning_effort": "none",
			"thinking":         map[string]interface{}{"type": "enabled"},
		}))
		if !reflect.DeepEqual(payload["thinking"], map[string]interface{}{"type": "enabled"}) {
			t.Fatalf("%s: thinking = %#v, want the caller's native value", model, payload["thinking"])
		}
		if !reflect.DeepEqual(payload["reasoning"], map[string]interface{}{"effort": "none"}) {
			t.Fatalf("%s: reasoning = %#v, want the reasoning.effort mapping alongside", model, payload["reasoning"])
		}
	}
}

func TestOpenAICompatibleChatPayload_UnknownReasoningLevelKeepsMapping(t *testing.T) {
	for _, model := range reasoningEffortTestModels {
		payload := buildOpenAICompatibleChatPayload(extrasChatRequest(model, map[string]interface{}{"reasoning_effort": "max"}))
		if !reflect.DeepEqual(payload["reasoning"], map[string]interface{}{"effort": "max"}) {
			t.Fatalf("%s: reasoning = %#v, want the reasoning.effort mapping", model, payload["reasoning"])
		}
		if _, ok := payload["thinking"]; ok {
			t.Fatalf("%s: thinking = %#v, want absent", model, payload["thinking"])
		}
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

type extrasChatAdapter interface {
	ChatCompletion(ctx context.Context, request *adapter.ChatRequest) (*adapter.ChatResponse, error)
	ChatCompletionStream(ctx context.Context, request *adapter.ChatRequest) (<-chan adapter.StreamResponse, error)
}

// assertExtrasThroughAdapter checks, on the wire, that reasoning_effort becomes
// reasoning.effort for a Kimi model too, that a native thinking extra reaches
// the upstream unchanged, and that a request without extras carries neither.
func assertExtrasThroughAdapter(t *testing.T, newAdapter func(baseURL string) extrasChatAdapter) {
	t.Helper()
	server, bodies := capturingChatServer(t)
	defer server.Close()
	client := newAdapter(server.URL)

	send := func(extras map[string]interface{}) []map[string]interface{} {
		t.Helper()
		if _, err := client.ChatCompletion(context.Background(), extrasChatRequest("kimi-k2.6", extras)); err != nil {
			t.Fatalf("ChatCompletion(%#v) error = %v", extras, err)
		}
		stream, err := client.ChatCompletionStream(context.Background(), extrasChatRequest("kimi-k2.6", extras))
		if err != nil {
			t.Fatalf("ChatCompletionStream(%#v) error = %v", extras, err)
		}
		drainStream(t, stream)
		got := bodies()
		return got[len(got)-2:]
	}

	for _, body := range send(map[string]interface{}{"reasoning_effort": "high"}) {
		if !reflect.DeepEqual(body["reasoning"], map[string]interface{}{"effort": "high"}) {
			t.Fatalf("reasoning_effort: upstream reasoning = %#v, want {effort: high} (body %#v)", body["reasoning"], body)
		}
		if _, ok := body["thinking"]; ok {
			t.Fatalf("reasoning_effort: upstream body carries thinking: %#v", body)
		}
	}

	for _, body := range send(map[string]interface{}{"thinking": map[string]interface{}{"type": "enabled"}}) {
		if !reflect.DeepEqual(body["thinking"], map[string]interface{}{"type": "enabled"}) {
			t.Fatalf("native thinking: upstream thinking = %#v, want it passed through (body %#v)", body["thinking"], body)
		}
	}

	if _, err := client.ChatCompletion(context.Background(), extrasChatRequest("kimi-k2.6", nil)); err != nil {
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

func TestAgictoAdapter_ExtrasReachUpstream(t *testing.T) {
	assertExtrasThroughAdapter(t, func(baseURL string) extrasChatAdapter {
		client, err := NewAgictoAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: baseURL + "/v1"})
		if err != nil {
			t.Fatalf("NewAgictoAdapter() error = %v", err)
		}
		return client
	})
}
