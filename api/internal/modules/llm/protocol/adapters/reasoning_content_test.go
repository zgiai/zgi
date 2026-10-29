package adapter

import "testing"

func TestMessagesForModelKeepsReasoningOnlyForModelsThatReplayIt(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "check the server"},
		{Role: "assistant", ReasoningContent: "inspect the service first", ToolCalls: []ToolCall{{ID: "call-1"}}},
		{Role: "tool", ToolCallID: "call-1", Content: "active"},
	}
	for _, model := range []string{"deepseek-v4-pro", "deepseek-flash", "DeepSeek-R1", "kimi-k2-thinking", "deepseek/deepseek-chat"} {
		got := MessagesForModel(model, messages)
		if got[1].ReasoningContent != "inspect the service first" {
			t.Fatalf("%s: reasoning = %q, want kept", model, got[1].ReasoningContent)
		}
	}
	for _, model := range []string{"gpt-5", "qwen3-max", "glm-4.6", "mistral-large", ""} {
		got := MessagesForModel(model, messages)
		if got[1].ReasoningContent != "" {
			t.Fatalf("%s: reasoning = %q, want dropped", model, got[1].ReasoningContent)
		}
		if got[1].ToolCalls[0].ID != "call-1" || got[2].Content != "active" {
			t.Fatalf("%s: messages changed beyond reasoning: %#v", model, got)
		}
	}
	if messages[1].ReasoningContent != "inspect the service first" {
		t.Fatal("input messages were modified")
	}
}

func TestWithoutReasoningContentReturnsInputWhenNothingToClear(t *testing.T) {
	messages := []Message{{Role: "user", Content: "hi"}}
	if got := WithoutReasoningContent(messages); &got[0] != &messages[0] {
		t.Fatal("expected the input slice when no message carries reasoning")
	}
}
