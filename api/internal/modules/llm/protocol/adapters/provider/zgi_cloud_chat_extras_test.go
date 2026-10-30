package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func decodeCallerChatRequest(t *testing.T, body string) *adapter.ChatRequest {
	t.Helper()
	var request adapter.ChatRequest
	if err := json.Unmarshal([]byte(body), &request); err != nil {
		t.Fatalf("json.Unmarshal(ChatRequest) error = %v", err)
	}
	return &request
}

func TestZGICloudAdapterChatCompletion_ForwardsProviderExtras(t *testing.T) {
	var gotPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(raw, &gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"chatcmpl-1","object":"chat.completion","model":"qwen3.8-27b","choices":[]}`)
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{BaseURL: server.URL + "/v1/internal", APIKey: "test-key"})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	request := decodeCallerChatRequest(t, `{
		"model":"qwen3.8-27b",
		"messages":[{"role":"user","content":"hi"}],
		"enable_thinking":false,
		"chat_template_kwargs":{"enable_thinking":false},
		"thinking":{"type":"disabled"},
		"reasoning_effort":"high"
	}`)
	if _, err := a.ChatCompletion(context.Background(), request); err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}

	if got := gotPayload["enable_thinking"]; got != false {
		t.Fatalf("enable_thinking = %#v, want false", got)
	}
	if got := gotPayload["chat_template_kwargs"]; !reflect.DeepEqual(got, map[string]interface{}{"enable_thinking": false}) {
		t.Fatalf("chat_template_kwargs = %#v", got)
	}
	if got := gotPayload["thinking"]; !reflect.DeepEqual(got, map[string]interface{}{"type": "disabled"}) {
		t.Fatalf("thinking = %#v", got)
	}
	if got := gotPayload["reasoning_effort"]; got != "high" {
		t.Fatalf("reasoning_effort = %#v, want verbatim %q", got, "high")
	}
	if _, ok := gotPayload["reasoning"]; ok {
		t.Fatalf("payload = %#v, want reasoning_effort not rewritten to reasoning", gotPayload)
	}
}

func TestZGICloudChatPayload_ExtrasCannotOverrideProtectedOrTypedFields(t *testing.T) {
	temperature := 0.3
	request := &adapter.ChatRequest{
		Model:       "qwen3.8-27b",
		Messages:    []adapter.Message{{Role: "user", Content: "hi"}},
		Temperature: &temperature,
		AdditionalParameters: map[string]interface{}{
			"model":           "other-model",
			"messages":        []interface{}{},
			"stream":          true,
			"stream_options":  map[string]interface{}{"include_usage": false},
			"organization_id": "another-org",
			"user":            "someone-else",
			"n":               4,
			"tools":           []interface{}{map[string]interface{}{"type": "function"}},
			"response_format": map[string]interface{}{"type": "json_object"},
			"temperature":     1.5,
			"enable_thinking": true,
		},
	}

	payload := buildZGICloudChatPayload(request)

	if payload["model"] != "qwen3.8-27b" {
		t.Fatalf("model = %#v, want typed model", payload["model"])
	}
	if messages, ok := payload["messages"].([]adapter.Message); !ok || len(messages) != 1 {
		t.Fatalf("messages = %#v, want typed messages", payload["messages"])
	}
	if payload["temperature"] != 0.3 {
		t.Fatalf("temperature = %#v, want typed 0.3", payload["temperature"])
	}
	for _, key := range []string{"stream", "stream_options", "organization_id", "user", "n", "tools", "response_format"} {
		if value, ok := payload[key]; ok {
			t.Fatalf("payload[%q] = %#v, want protected key not set by extras", key, value)
		}
	}
	if payload["enable_thinking"] != true {
		t.Fatalf("enable_thinking = %#v, want forwarded", payload["enable_thinking"])
	}
}

func TestZGICloudChatPayload_RoundTripKeepsCallerBody(t *testing.T) {
	body := `{
		"model":"deepseek-v4-pro",
		"messages":[{"role":"user","content":"hi"}],
		"temperature":0.7,
		"stream":true,
		"stream_options":{"include_usage":true},
		"thinking":{"type":"enabled"},
		"reasoning_effort":"medium",
		"chat_template_kwargs":{"enable_thinking":true}
	}`

	payload, err := json.Marshal(buildZGICloudChatPayload(decodeCallerChatRequest(t, body)))
	if err != nil {
		t.Fatalf("json.Marshal(payload) error = %v", err)
	}

	var want, got map[string]interface{}
	if err := json.Unmarshal([]byte(body), &want); err != nil {
		t.Fatalf("decode caller body: %v", err)
	}
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode forwarded body: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("forwarded body = %s, want caller body %s", payload, body)
	}
}
