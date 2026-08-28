package skillloop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/contextmgr"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestRunnerReadContextArtifactFeedsCompleteContentIntoNextModelCall(t *testing.T) {
	store := contextmgr.NewMemoryStore()
	manager, err := contextmgr.New(contextmgr.Config{
		AgentRunID:             "run-read-context-artifact",
		ConfiguredAgentWindowK: 256,
		ModelContextWindow:     1_000_000,
		MaxOutputTokens:        16_000,
		MaxToolResultTokens:    100,
	}, nil, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	original := strings.Repeat("restored knowledge evidence row 123456789\n", 1_000)
	hash := sha256.Sum256([]byte(original))
	hashText := hex.EncodeToString(hash[:])
	ref, err := store.Put(context.Background(), "run-read-context-artifact", hashText, original)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := json.Marshal(map[string]interface{}{
		"status":          "compacted",
		"artifact_ref":    ref,
		"content_hash":    hashText,
		"original_tokens": 10_000,
		"tool_call_id":    "search-1",
		"truncated":       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	readArgs, err := json.Marshal(map[string]interface{}{
		"artifact_ref": ref,
	})
	if err != nil {
		t.Fatal(err)
	}
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{
			Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{{
				ID: "read-1", Type: "function", Function: adapter.FunctionCall{Name: contextArtifactToolName, Arguments: string(readArgs)},
			}}},
			FinishReason: "tool_calls",
		}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "used restored evidence"}, FinishReason: "stop"}}},
	}}
	runner := &Runner{LLMClient: fakeLLM, AppContext: &llmclient.AppContext{}, ContextManager: manager}
	prepared := NewPreparedChat("conv-artifact", "msg-artifact", "", "auto", &adapter.ChatRequest{
		Model: "gpt-5",
		Messages: []adapter.Message{
			{Role: "user", Content: "answer from the compacted search result"},
			{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "search-1", Type: "function", Function: adapter.FunctionCall{Name: "search", Arguments: `{"query":"evidence"}`}}}},
			{Role: "tool", ToolCallID: "search-1", Content: string(receipt)},
		},
	})

	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:          prepared,
		Resolved:          nil,
		ProtocolToolsOnly: true,
		NativeAgentLoop:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if answer != "used restored evidence" || fakeLLM.appChatCalls != 2 {
		t.Fatalf("answer = %q calls = %d", answer, fakeLLM.appChatCalls)
	}
	if !runnerTestHasTool(fakeLLM.appChatRequests[0].Tools, contextArtifactToolName) {
		t.Fatalf("first request did not expose %s", contextArtifactToolName)
	}
	for _, tool := range fakeLLM.appChatRequests[0].Tools {
		if tool.Function.Name != contextArtifactToolName {
			continue
		}
		parameters, _ := tool.Function.Parameters.(map[string]interface{})
		properties, _ := parameters["properties"].(map[string]interface{})
		if len(properties) != 1 || properties["artifact_ref"] == nil {
			t.Fatalf("artifact reader still exposes paging parameters: %#v", properties)
		}
	}
	secondRequest := fakeLLM.appChatRequests[1]
	if !runnerTestMessagesContain(secondRequest.Messages, "restored knowledge evidence row") {
		t.Fatalf("second request did not contain complete restored artifact: %#v", secondRequest.Messages)
	}
	for _, message := range secondRequest.Messages {
		if message.ToolCallID != "read-1" {
			continue
		}
		parts := strings.SplitN(messageContent(message.Content), "\n\n[artifact content]\n", 2)
		if len(parts) != 2 || parts[1] != original {
			t.Fatalf("artifact tool message = %q", messageContent(message.Content))
		}
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(parts[0]), &payload); err != nil {
			t.Fatal(err)
		}
		returnedTokens := numericValue(payload["returned_tokens"])
		if returnedTokens <= 0 || returnedTokens != numericValue(payload["total_tokens"]) || payload["complete"] != true {
			t.Fatalf("artifact tool payload = %#v", payload)
		}
		return
	}
	t.Fatal("second request did not contain the artifact tool result")
}

func TestLegacyToolChatKeepsContextArtifactReader(t *testing.T) {
	tools := legacyToolChatTools([]adapter.Tool{contextArtifactTool()}, false)
	if !runnerTestHasTool(tools, contextArtifactToolName) {
		t.Fatalf("legacy tools removed %s", contextArtifactToolName)
	}
}
