package contextmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func largeToolResult() string {
	var builder strings.Builder
	for line := 1; line <= 4_000; line++ {
		fmt.Fprintf(&builder, "row %d status ok latency %dms region east\n", line, line%97)
		if line == 2_500 {
			builder.WriteString("needle-777 connection reset by peer\n")
		}
	}
	return builder.String()
}

func projectionRequest(result string) *adapter.ChatRequest {
	return &adapter.ChatRequest{Model: "deepseek-v4-flash", Messages: []adapter.Message{
		{Role: "user", Content: "check the logs"},
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "logs-1", Type: "function", Function: adapter.FunctionCall{Name: "read_logs", Arguments: "{}"}}}},
		{Role: "tool", ToolCallID: "logs-1", Content: result},
	}}
}

// An oversized tool result reaches the model as its beginning and end with a
// reference, the same in every request, while the run keeps the original and
// reads parts of it back without any store (ADR 0037).
func TestOversizedToolResultIsProjectedAndReadBackInParts(t *testing.T) {
	ctx := context.Background()
	manager, err := New(Config{AgentRunID: "run-1", ModelContextWindow: 1_000_000, MaxOutputTokens: 16_000}, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	original := largeToolResult()
	assistant := projectionRequest(original).Messages[1]
	if err := manager.ObserveModelResponse(ctx, assistant, nil); err != nil {
		t.Fatal(err)
	}
	if err := manager.ReplaceMessages(ctx, projectionRequest(original).Messages); err != nil {
		t.Fatal(err)
	}

	first, _, err := manager.PrepareBeforeModelCall(ctx, projectionRequest(original))
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := manager.PrepareBeforeModelCall(ctx, projectionRequest(original))
	if err != nil {
		t.Fatal(err)
	}
	projected := contentString(first.Messages[len(first.Messages)-1].Content)
	if projected != contentString(second.Messages[len(second.Messages)-1].Content) {
		t.Fatal("the projection changed between requests")
	}
	var receipt map[string]interface{}
	if err := json.Unmarshal([]byte(projected), &receipt); err != nil || receipt["status"] != "projected" {
		t.Fatalf("tool result was not projected: %.200s", projected)
	}
	preview := fmt.Sprint(receipt["preview"])
	if !strings.HasPrefix(preview, "row 1 status ok") || !strings.Contains(preview, "row 4000 status ok") || strings.Contains(preview, "needle-777") {
		t.Fatalf("preview should keep the beginning and the end only: %.200s", preview)
	}
	for _, message := range manager.TurnTranscript() {
		if message.ToolCallID == "logs-1" && contentString(message.Content) != original {
			t.Fatal("the transcript lost the original tool result")
		}
	}

	ref := fmt.Sprint(receipt["artifact_ref"])
	part, err := manager.ReadContextArtifactPart(ctx, ref, ContextArtifactPart{Offset: 100, Limit: 50})
	if err != nil || part.Content != string([]rune(original)[100:150]) || part.Complete || part.TotalChars != len([]rune(original)) {
		t.Fatalf("range read = %#v, %v", part, err)
	}
	found, err := manager.ReadContextArtifactPart(ctx, ref, ContextArtifactPart{Query: "NEEDLE-777"})
	if err != nil || !strings.Contains(found.Content, "2501: needle-777 connection reset by peer") {
		t.Fatalf("query read = %q, %v", found.Content, err)
	}
	if _, err := manager.ReadContextArtifactPart(ctx, "agent-context://tool-results/run-1/"+strings.Repeat("0", 64), ContextArtifactPart{}); err == nil {
		t.Fatal("read an artifact the run never had")
	}
}

func TestSmallToolResultIsNotProjected(t *testing.T) {
	manager, err := New(Config{AgentRunID: "run-2", ModelContextWindow: 1_000_000, MaxOutputTokens: 16_000}, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	prepared, _, err := manager.PrepareBeforeModelCall(context.Background(), projectionRequest("status ok"))
	if err != nil {
		t.Fatal(err)
	}
	if got := contentString(prepared.Messages[len(prepared.Messages)-1].Content); got != "status ok" {
		t.Fatalf("small result = %q, want it unchanged", got)
	}
}
