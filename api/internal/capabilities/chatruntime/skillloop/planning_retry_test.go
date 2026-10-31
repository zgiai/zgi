package skillloop

import (
	"context"
	"errors"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestRunnerDoesNotRetryContentFilterTermination(t *testing.T) {
	client := &runnerTestLLMClient{appChatStreams: [][]adapter.StreamResponse{
		{{Choices: []adapter.StreamChoice{{FinishReason: "content_filter"}}, Done: true}},
	}}
	runner := &Runner{LLMClient: client, SkillRuntime: skills.NewRuntime(nil, nil)}
	prepared := NewPreparedChat("conv-filtered", "msg-filtered", "qwen", "auto", &adapter.ChatRequest{Model: "qwen-plus"})

	_, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:             prepared,
		Resolved:             runnerTestResolvedSkills(),
		RuntimeStateSnapshot: func() map[string]interface{} { return map[string]interface{}{} },
	})
	var terminationErr *PlanningTerminationError
	if !errors.As(err, &terminationErr) || terminationErr.Recoverable {
		t.Fatalf("Run() error = %#v, want non-recoverable content filter termination", err)
	}
	if client.appChatStreamCalls != 1 {
		t.Fatalf("AppChatStream calls = %d, want 1", client.appChatStreamCalls)
	}
}
