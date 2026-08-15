package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestHistoryMessageGroupsRestoresQueryTranscriptAndAnswer(t *testing.T) {
	transcript := []adapter.Message{
		{
			Role:             "assistant",
			Content:          "I will inspect both sources.",
			ReasoningContent: "do not persist",
			ToolCalls: []adapter.ToolCall{
				{ID: "call-a", Function: adapter.FunctionCall{Name: "search", Arguments: `{"query":"alpha"}`}},
				{ID: "call-b", Function: adapter.FunctionCall{Name: "read_file", Arguments: `{"path":"beta"}`}},
			},
		},
		{Role: "tool", ToolCallID: "call-a", Content: `{"matches":["alpha"]}`},
		{Role: "tool", ToolCallID: "call-b", Content: `{"content":"beta"}`},
		{Role: "assistant", Content: "final response"},
	}
	metadata := mergeAgentTranscriptMetadata(nil, transcript, "final response")
	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	var databaseMetadata map[string]interface{}
	if err := json.Unmarshal(encoded, &databaseMetadata); err != nil {
		t.Fatal(err)
	}
	message := &runtimemodel.Message{
		ID:       uuid.New(),
		Query:    "original question",
		Answer:   "final response",
		Status:   runtimemodel.MessageStatusCompleted,
		Metadata: databaseMetadata,
	}

	groups, err := (&service{}).historyMessageGroups(context.Background(), []*runtimemodel.Message{message}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0]) != 5 {
		t.Fatalf("history groups = %#v", groups)
	}
	group := groups[0]
	if group[0].Role != "user" || group[0].Content != "original question" {
		t.Fatalf("query message = %#v", group[0])
	}
	if group[1].Role != "assistant" || len(group[1].ToolCalls) != 2 || group[1].ToolCalls[0].Function.Arguments != `{"query":"alpha"}` {
		t.Fatalf("assistant tool call message = %#v", group[1])
	}
	if group[1].ReasoningContent != "" || group[2].ToolCallID != "call-a" || group[3].ToolCallID != "call-b" {
		t.Fatalf("tool messages = %#v", group[1:4])
	}
	if group[4].Role != "assistant" || group[4].Content != "final response" {
		t.Fatalf("final answer = %#v", group[4])
	}
}

func TestAgentTranscriptDropsIncompleteFinalToolBatch(t *testing.T) {
	transcript := []adapter.Message{
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "complete", Function: adapter.FunctionCall{Name: "search"}}}},
		{Role: "tool", ToolCallID: "complete", Content: `{"status":"ok"}`},
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "missing", Function: adapter.FunctionCall{Name: "read_file"}}}},
	}
	normalized := normalizeAgentTranscript(transcript, "")
	if len(normalized) != 2 || normalized[0].ToolCalls[0].ID != "complete" || normalized[1].ToolCallID != "complete" {
		t.Fatalf("normalized transcript = %#v", normalized)
	}
}

func TestClientVisibleMetadataOmitsAgentTranscript(t *testing.T) {
	metadata := mergeAgentTranscriptMetadata(map[string]interface{}{
		"public": "value",
		"model_invocations": []interface{}{
			map[string]interface{}{"request": map[string]interface{}{"messages": []interface{}{"private prompt"}}},
		},
	}, []adapter.Message{
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "call-1", Function: adapter.FunctionCall{Name: "search"}}}},
		{Role: "tool", ToolCallID: "call-1", Content: `{"secret":"model-only result"}`},
	}, "")
	visible := ClientVisibleMessageMetadata(metadata)
	if visible["public"] != "value" {
		t.Fatalf("public metadata was lost: %#v", visible)
	}
	if _, ok := visible[agentTranscriptMetadataKey]; ok {
		t.Fatalf("Agent transcript leaked into client metadata: %#v", visible)
	}
	if _, ok := visible[agentTranscriptVersionMetadataKey]; ok {
		t.Fatalf("Agent transcript version leaked into client metadata: %#v", visible)
	}
	if _, ok := visible["model_invocations"]; ok {
		t.Fatalf("model invocation payload leaked into client metadata: %#v", visible)
	}
	if visible["model_invocations_redacted"] != true || visible["model_invocation_count"] != 1 {
		t.Fatalf("model invocation redaction markers = %#v", visible)
	}
	if _, ok := metadata[agentTranscriptMetadataKey]; !ok {
		t.Fatalf("redaction mutated durable metadata: %#v", metadata)
	}
	if _, ok := metadata["model_invocations"]; !ok {
		t.Fatalf("redaction mutated durable model invocation metadata: %#v", metadata)
	}
}
