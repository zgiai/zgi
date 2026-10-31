package provider

import (
	"reflect"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestOpenAICompatibleChatPayload_ExtrasCannotOverrideProtectedFields(t *testing.T) {
	request := &adapter.ChatRequest{
		Model:    "gpt-5.2",
		Messages: []adapter.Message{{Role: "user", Content: "hi"}},
		AdditionalParameters: map[string]interface{}{
			"model":            "other-model",
			"stream":           true,
			"organization_id":  "another-org",
			"reasoning_effort": "low",
		},
	}

	payload := buildOpenAICompatibleChatPayload(request)

	if payload["model"] != "gpt-5.2" {
		t.Fatalf("model = %#v, want typed model", payload["model"])
	}
	if _, ok := payload["stream"]; ok {
		t.Fatalf("stream = %#v, want protected", payload["stream"])
	}
	if _, ok := payload["organization_id"]; ok {
		t.Fatalf("organization_id = %#v, want protected", payload["organization_id"])
	}
	if got := payload["reasoning"]; !reflect.DeepEqual(got, map[string]interface{}{"effort": "low"}) {
		t.Fatalf("reasoning = %#v, want OpenAI adapter mapping kept", got)
	}
}
