package service

import (
	"encoding/json"
	"strings"
	"testing"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestModelInvocationRequestPayloadRedactsToolMessageContent(t *testing.T) {
	const rawFileContent = "CONFIDENTIAL FILE BODY SHOULD NOT BE STORED"
	req := modelInvocationToolContentTestRequest(rawFileContent)

	payload := modelInvocationRequestPayload(req, true)
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if strings.Contains(string(encoded), rawFileContent) {
		t.Fatalf("model invocation request leaked raw tool content: %s", string(encoded))
	}

	messages, ok := payload["messages"].([]interface{})
	if !ok || len(messages) != 3 {
		t.Fatalf("messages = %#v, want three messages", payload["messages"])
	}
	userMessage, _ := messages[0].(map[string]interface{})
	if userMessage["content"] != "summarize the selected file" {
		t.Fatalf("user content = %#v, want preserved", userMessage["content"])
	}
	toolMessage, _ := messages[1].(map[string]interface{})
	if toolMessage["content_redacted"] != true {
		t.Fatalf("tool content_redacted = %#v, want true", toolMessage["content_redacted"])
	}
	contentSummary, ok := toolMessage["content"].(map[string]interface{})
	if !ok {
		t.Fatalf("tool content = %#v, want summary map", toolMessage["content"])
	}
	if contentSummary["redacted"] != true {
		t.Fatalf("tool content summary = %#v, want redacted", contentSummary)
	}
	jsonSummary, ok := contentSummary["json"].(map[string]interface{})
	if !ok {
		t.Fatalf("tool json summary = %#v, want map", contentSummary["json"])
	}
	fields, ok := jsonSummary["fields"].(map[string]interface{})
	if !ok {
		t.Fatalf("tool json fields = %#v, want map", jsonSummary["fields"])
	}
	if fields["file_id"] != "file-1" || fields["content_status"] != "extracted" || fields["content_redacted"] != true {
		t.Fatalf("tool json fields = %#v, want safe file/content status summary", fields)
	}

	assistantMessage, _ := messages[2].(map[string]interface{})
	assistantEncoded, err := json.Marshal(assistantMessage)
	if err != nil {
		t.Fatalf("marshal assistant message: %v", err)
	}
	if !strings.Contains(string(assistantEncoded), "call_skill_tool") || !strings.Contains(string(assistantEncoded), "file-1") {
		t.Fatalf("assistant tool call arguments were not preserved for debugging: %s", string(assistantEncoded))
	}
}

func TestModelInvocationRequestPayloadCanPreserveToolMessageContent(t *testing.T) {
	const rawFileContent = "AGENT DEBUG RAW CONTENT"
	payload := modelInvocationRequestPayload(modelInvocationToolContentTestRequest(rawFileContent), false)
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if !strings.Contains(string(encoded), rawFileContent) {
		t.Fatalf("model invocation request did not preserve raw tool content when redaction is disabled: %s", string(encoded))
	}
}

func TestShouldRedactModelInvocationRequestKeepsAgentDebugRaw(t *testing.T) {
	if shouldRedactModelInvocationRequest(&PreparedChat{Caller: Caller{Type: runtimemodel.ConversationCallerAgent}}) {
		t.Fatal("shouldRedactModelInvocationRequest() = true for agent caller, want false")
	}
	if !shouldRedactModelInvocationRequest(&PreparedChat{Caller: Caller{Type: runtimemodel.ConversationCallerAIChat}}) {
		t.Fatal("shouldRedactModelInvocationRequest() = false for AIChat caller, want true")
	}
}

func modelInvocationToolContentTestRequest(rawFileContent string) *adapter.ChatRequest {
	return &adapter.ChatRequest{
		Provider: "deepseek",
		Model:    "deepseek-chat",
		Messages: []adapter.Message{
			{Role: "user", Content: "summarize the selected file"},
			{
				Role:       "tool",
				ToolCallID: "call-read-file",
				Content: `{"status":"completed","file_id":"file-1","name":"secret.md","content_status":"extracted","content":` +
					jsonQuoteString(rawFileContent) + `,"content_chars":128,"content_truncated":false}`,
			},
			{
				Role: "assistant",
				ToolCalls: []adapter.ToolCall{{
					ID:   "call-read-file",
					Type: "function",
					Function: adapter.FunctionCall{
						Name:      "call_skill_tool",
						Arguments: `{"skill_id":"file-reader","tool_name":"read_file","arguments":{"file_id":"file-1"}}`,
					},
				}},
			},
		},
	}
}

func jsonQuoteString(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}
