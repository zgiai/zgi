package skillloop

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/contextmgr"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// Reasoning from a tool-call round is sent back within the turn to models
// that require it, such as DeepSeek, and left out for the others.
func TestPrepareContextRequestReplaysReasoningOnlyForModelsThatExpectIt(t *testing.T) {
	cases := map[string]string{
		"deepseek-v4-pro": "check the service before restarting",
		"qwen-plus":       "",
	}
	for model, want := range cases {
		for _, withManager := range []bool{false, true} {
			assistant := adapter.Message{
				Role:             "assistant",
				ReasoningContent: "check the service before restarting",
				ToolCalls:        []adapter.ToolCall{{ID: "call-1", Type: "function", Function: adapter.FunctionCall{Name: "service_status", Arguments: `{}`}}},
			}
			result := adapter.Message{Role: "tool", ToolCallID: "call-1", Content: "active"}
			messages := []adapter.Message{{Role: "system", Content: "Ops"}, {Role: "user", Content: "is nginx up?"}, assistant, result}
			runner := &Runner{}
			if withManager {
				manager, err := contextmgr.New(contextmgr.Config{
					AgentRunID:         "reasoning-replay",
					ModelContextWindow: 64_000,
					MaxOutputTokens:    8_000,
				}, nil, nil, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				if _, _, err := manager.PrepareBeforeModelCall(context.Background(), &adapter.ChatRequest{Model: model, Messages: messages[:2]}); err != nil {
					t.Fatal(err)
				}
				if err := manager.ObserveModelResponse(context.Background(), assistant, nil); err != nil {
					t.Fatal(err)
				}
				runner.ContextManager = manager
			}

			prepared, err := runner.prepareContextRequest(context.Background(), &adapter.ChatRequest{Model: model, Messages: messages})
			if err != nil {
				t.Fatalf("%s manager=%v: %v", model, withManager, err)
			}
			found := false
			for _, message := range prepared.Messages {
				if len(message.ToolCalls) == 0 {
					continue
				}
				found = true
				if message.ReasoningContent != want {
					t.Fatalf("%s manager=%v: reasoning = %q, want %q", model, withManager, message.ReasoningContent, want)
				}
			}
			if !found {
				t.Fatalf("%s manager=%v: tool-call round missing: %#v", model, withManager, prepared.Messages)
			}
			if withManager {
				for _, message := range runner.ContextManager.TurnTranscript() {
					if message.ReasoningContent != "" {
						t.Fatalf("%s: reasoning stored in the turn transcript: %#v", model, message)
					}
				}
			}
		}
	}
}
