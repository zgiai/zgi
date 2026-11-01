package provider

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// #2913: the moonshot adapter used to serialize ChatRequest as is, which
// drops AdditionalParameters (json:"-"), so no provider extra reached the
// upstream. Extras now travel like on the other OpenAI-compatible adapters.

func TestMoonshotAIAdapter_ExtrasReachUpstream(t *testing.T) {
	assertExtrasThroughAdapter(t, func(baseURL string) extrasChatAdapter {
		client, err := NewMoonshotAIAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: baseURL})
		if err != nil {
			t.Fatalf("NewMoonshotAIAdapter() error = %v", err)
		}
		return client
	})
}

func TestMoonshotAIAdapter_RequestWithoutExtrasIsSentAsBefore(t *testing.T) {
	server, bodies := capturingChatServer(t)
	defer server.Close()
	client, err := NewMoonshotAIAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewMoonshotAIAdapter() error = %v", err)
	}
	temperature := 0.3
	maxTokens := 512
	request := &adapter.ChatRequest{
		Model:       "kimi-k2.6",
		Messages:    []adapter.Message{{Role: "system", Content: "be brief"}, {Role: "user", Content: "hi"}},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		ToolChoice:  "auto",
		Tools: []adapter.Tool{{Type: "function", Function: adapter.Function{
			Name:       "lookup",
			Parameters: map[string]interface{}{"type": "object"},
		}}},
	}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	var want map[string]interface{}
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}

	if _, err := client.ChatCompletion(context.Background(), request); err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}
	if got := bodies()[0]; !reflect.DeepEqual(got, want) {
		t.Fatalf("upstream body changed without extras:\n got %#v\nwant %#v", got, want)
	}
}

func TestMoonshotAICNAdapter_ExtrasReachUpstream(t *testing.T) {
	assertExtrasThroughAdapter(t, func(baseURL string) extrasChatAdapter {
		client, err := NewMoonshotAICNAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: baseURL})
		if err != nil {
			t.Fatalf("NewMoonshotAICNAdapter() error = %v", err)
		}
		return client
	})
}
