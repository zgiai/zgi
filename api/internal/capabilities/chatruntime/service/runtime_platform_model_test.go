package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestRuntimePlatformModelReusesAgentBillingAndTaskBudget(t *testing.T) {
	llm := &toolGovernanceStreamLLM{appChatResponses: []*adapter.ChatResponse{{Choices: []adapter.Choice{{Message: adapter.Message{Content: "answer"}}}, Usage: &adapter.Usage{TotalTokens: 12}}}}
	prepared := &PreparedChat{Conversation: &runtimemodel.Conversation{ID: uuid.New(), OrganizationID: uuid.New(), AccountID: uuid.New()}, Message: &runtimemodel.Message{ID: uuid.New()}, LLMRequest: &adapter.ChatRequest{Provider: "approved-provider", Model: "approved-model", Messages: []adapter.Message{{Role: "user", Content: "private history"}}}, taskTokenBudget: newTaskTokenBudget(100)}
	svc := &service{llmClient: llm}
	call, usage := svc.newRuntimePlatformModel(prepared, func(u *adapter.Usage) error {
		if err := accountTaskTokenUsage(prepared, u); err != nil {
			return err
		}
		return nil
	})
	answer, err := call(t.Context(), "script prompt", 512)
	if err != nil || answer != "answer" || usage().TotalTokens != 12 || prepared.taskTokenBudget.UsedTokens != 12 {
		t.Fatalf("answer=%q usage=%+v err=%v", answer, usage(), err)
	}
	req := llm.appChatRequests[0]
	app := llm.appContexts[0]
	if req.Model != "approved-model" || req.Provider != "approved-provider" || len(req.Messages) != 1 || req.Messages[0].Content != "script prompt" || len(req.Tools) != 0 || req.Stream || *req.MaxTokens != 100 || !app.SuppressInvocationContent || app.AccountID != prepared.Conversation.AccountID.String() {
		t.Fatal("routing, isolation, output budget or billing scope lost")
	}
	prepared.taskTokenBudget.UsedTokens = 100
	if _, err := call(context.Background(), "another prompt", 10); err == nil || len(llm.appChatRequests) != 1 {
		t.Fatal("exhausted budget called provider")
	}
	if _, err := call(t.Context(), "", 0); err == nil {
		t.Fatal("invalid request accepted")
	}
}
