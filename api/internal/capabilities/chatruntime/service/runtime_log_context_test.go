package service

import (
	"encoding/json"
	"strings"
	"testing"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestModelInvocationLogContextSanitizesAndKeepsUsefulMessages(t *testing.T) {
	request := &adapter.ChatRequest{
		Provider: "provider-a",
		Model:    "model-a",
		Messages: []adapter.Message{
			{Role: "system", Content: "internal platform instructions"},
			{Role: "user", Content: "keep this user request"},
			{Role: "tool", ToolCallID: "call-1", Content: `{"token":"secret","value":4,"instructions":"hidden"}`},
			{Role: "assistant", ToolCalls: []adapter.ToolCall{{Function: adapter.FunctionCall{Name: "lookup", Arguments: `{"api_key":"secret","query":"safe"}`}}}},
		},
		Tools: []adapter.Tool{{Type: "function", Function: adapter.Function{Name: "lookup", Description: strings.Repeat("private schema", 100)}}},
	}

	context := modelInvocationLogContext(request, "visible user system prompt")
	messages, ok := context["messages"].([]interface{})
	if !ok || len(messages) != 4 {
		t.Fatalf("messages = %#v, want visible system plus three non-system messages", context["messages"])
	}
	first := messages[0].(map[string]interface{})
	if first["role"] != "system" || first["content"] != "visible user system prompt" {
		t.Fatalf("first message = %#v, want visible user system prompt", first)
	}
	encoded := string(mustJSON(t, context))
	for _, forbidden := range []string{"internal platform instructions", "private schema", "secret"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("log context leaked %q: %s", forbidden, encoded)
		}
	}
	for _, required := range []string{"keep this user request", agentLogRedactedSensitiveValue, agentLogHiddenInstructions, "lookup", "safe"} {
		if !strings.Contains(encoded, required) {
			t.Fatalf("log context missing %q: %s", required, encoded)
		}
	}
}

func TestModelInvocationLogContextBoundsLargeContext(t *testing.T) {
	messages := make([]adapter.Message, 0, 12)
	for index := 0; index < 12; index++ {
		messages = append(messages, adapter.Message{Role: "user", Content: strings.Repeat(string(rune('a'+index)), agentLogContextValueMaxBytes)})
	}
	request := &adapter.ChatRequest{Provider: "provider-a", Model: "model-a", Messages: messages}
	context := modelInvocationLogContext(request, "visible prompt")
	if size := len(mustJSON(t, context)); size > agentLogContextMaxBytes {
		t.Fatalf("serialized context size = %d, want <= %d", size, agentLogContextMaxBytes)
	}
	meta := context["snapshot_meta"].(map[string]interface{})
	if meta["truncated"] != true || intValueFromAny(meta["omitted_message_count"]) == 0 {
		t.Fatalf("snapshot_meta = %#v, want truncated context with omitted messages", meta)
	}
	encoded := string(mustJSON(t, context))
	if !strings.Contains(encoded, strings.Repeat("l", 128)) {
		t.Fatalf("bounded context did not retain the newest message: %s", encoded)
	}
}

func TestModelInvocationLogContextSummarizesLargeToolResults(t *testing.T) {
	request := &adapter.ChatRequest{Messages: []adapter.Message{{
		Role:    "tool",
		Content: `{"content":"` + strings.Repeat("file contents", 1000) + `","mime_type":"text/plain"}`,
	}}}

	context := modelInvocationLogContext(request, "")
	encoded := string(mustJSON(t, context))
	if strings.Contains(encoded, strings.Repeat("file contents", 10)) {
		t.Fatalf("large tool result was retained: %s", encoded)
	}
	for _, expected := range []string{"tool_result_content_omitted", "content_chars", "content_truncated"} {
		if !strings.Contains(encoded, expected) {
			t.Fatalf("large tool result summary missing %q: %s", expected, encoded)
		}
	}
}

func TestAgentLogContextIsAgentOnly(t *testing.T) {
	if !shouldPersistAgentLogContext(&PreparedChat{Caller: Caller{Type: runtimemodel.ConversationCallerAgent}}) {
		t.Fatal("agent caller should persist log context")
	}
	if shouldPersistAgentLogContext(&PreparedChat{Caller: Caller{Type: runtimemodel.ConversationCallerAIChat}}) {
		t.Fatal("AIChat caller should keep compact invocation metadata")
	}
}

func mustJSON(t *testing.T, value interface{}) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return data
}
