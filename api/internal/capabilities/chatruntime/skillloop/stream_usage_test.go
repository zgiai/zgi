package skillloop

import (
	"context"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestSkillPlanningStreamDoesNotAddRepeatedUsageSnapshots(t *testing.T) {
	client := &runnerTestLLMClient{appChatStreams: [][]adapter.StreamResponse{{
		{
			Choices: []adapter.StreamChoice{{Delta: adapter.Message{Content: "WEBAPP_"}}},
			Usage:   &adapter.Usage{PromptTokens: 5078, CompletionTokens: 50, TotalTokens: 5128},
		},
		{
			Choices: []adapter.StreamChoice{{Delta: adapter.Message{Content: "SMOKE_OK"}}},
			Usage:   &adapter.Usage{PromptTokens: 5078, CompletionTokens: 239, TotalTokens: 5317},
		},
		{
			Choices: []adapter.StreamChoice{{FinishReason: "stop"}},
			Usage:   &adapter.Usage{PromptTokens: 5078, CompletionTokens: 239, TotalTokens: 5317},
			Done:    true,
		},
	}}}
	runner := &Runner{LLMClient: client}
	prepared := NewPreparedChat("conv-1", "msg-1", "qwen", "auto", &adapter.ChatRequest{Model: "qwen-plus"})

	result, ok, err := runner.runSkillPlanningStream(context.Background(), prepared, prepared.LLMRequest, 0, nil, false, false, false)
	if err != nil {
		t.Fatalf("runSkillPlanningStream() error = %v", err)
	}
	if !ok {
		t.Fatal("runSkillPlanningStream() ok = false, want true")
	}
	if result.usage == nil {
		t.Fatal("runSkillPlanningStream() usage = nil")
	}
	want := adapter.Usage{PromptTokens: 5078, CompletionTokens: 239, TotalTokens: 5317}
	if *result.usage != want {
		t.Fatalf("runSkillPlanningStream() usage = %+v, want %+v", *result.usage, want)
	}
}
