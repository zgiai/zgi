package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestSkillScriptModelUsesAgentRoutingAndBillingWithoutHistory(t *testing.T) {
	llm := &toolGovernanceStreamLLM{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{Message: adapter.Message{Content: `{"items":[]}`}}}, Usage: &adapter.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Content: `{"answer":"ok"}`}}}, Usage: &adapter.Usage{PromptTokens: 20, CompletionTokens: 6, TotalTokens: 26}},
	}}
	workspace := uuid.New()
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New(), OrganizationID: uuid.New(), AccountID: uuid.New(), WorkspaceID: &workspace},
		Message:      &runtimemodel.Message{ID: uuid.New()},
		LLMRequest:   &adapter.ChatRequest{Model: "agent-model", Provider: "agent-provider", Messages: []adapter.Message{{Role: "user", Content: "unrelated conversation"}}, Tools: []adapter.Tool{{Type: "function"}}},
	}
	svc := &service{llmClient: llm}
	channel, usage := svc.newSkillScriptModelChannel(prepared)
	for range 2 {
		if _, err := channel.Complete(context.Background(), "workflow system", "workflow user"); err != nil {
			t.Fatal(err)
		}
	}
	for _, req := range llm.appChatRequests {
		if req.Model != "agent-model" || req.Provider != "agent-provider" || len(req.Messages) != 2 || req.Messages[0].Content != "workflow system" || req.Messages[1].Content != "workflow user" {
			t.Fatalf("unexpected routing/prompts: %#v", req)
		}
		if len(req.Tools) != 0 || req.Stream || req.Temperature == nil || *req.Temperature != 0 || req.MaxTokens == nil || *req.MaxTokens != 4096 || req.ResponseFormat.Type != "json_object" {
			t.Fatal("unexpected model controls")
		}
	}
	for _, app := range llm.appContexts {
		if app.OrganizationID != prepared.Conversation.OrganizationID.String() || app.AccountID != prepared.Conversation.AccountID.String() || app.WorkspaceID != workspace.String() || app.ConversationID != prepared.Conversation.ID.String() || !app.SuppressInvocationContent {
			t.Fatalf("scope or privacy lost: %#v", app)
		}
	}
	if usage().TotalTokens != 41 {
		t.Fatalf("usage lost: %#v", usage())
	}
	copy := usage()
	copy.TotalTokens = 0
	if usage().TotalTokens != 41 {
		t.Fatal("usage snapshot is mutable")
	}
}

func TestSkillScriptModelNotifiesUsageObserverAndPropagatesBudgetStop(t *testing.T) {
	llm := &toolGovernanceStreamLLM{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{Message: adapter.Message{Content: `{"answer":"ok"}`}}}, Usage: &adapter.Usage{TotalTokens: 25}},
	}}
	prepared := &PreparedChat{
		Conversation:    &runtimemodel.Conversation{ID: uuid.New()},
		Message:         &runtimemodel.Message{ID: uuid.New(), Metadata: map[string]interface{}{}},
		taskTokenBudget: newTaskTokenBudget(20),
		LLMRequest:      &adapter.ChatRequest{Model: "test-model", Provider: "test-provider"},
	}
	var observed *adapter.Usage
	svc := &service{llmClient: llm}
	channel, _ := svc.newSkillScriptModelChannel(prepared, func(usage *adapter.Usage) error {
		observed = usage
		return accountTaskTokenUsage(prepared, usage)
	})
	if _, err := channel.Complete(context.Background(), "system", "user"); err == nil {
		t.Fatal("script model call succeeded after exhausting task budget")
	} else {
		var exceeded *TaskTokenBudgetExceededError
		if !errors.As(err, &exceeded) {
			t.Fatalf("script model error = %v, want task budget error", err)
		}
	}
	if observed == nil || observed.TotalTokens != 25 {
		t.Fatalf("observed usage = %#v, want 25 total tokens", observed)
	}
	if prepared.taskTokenBudget.UsedTokens != 25 {
		t.Fatalf("budget used tokens = %d, want 25", prepared.taskTokenBudget.UsedTokens)
	}
}

func TestSkillScriptModelRejectsTruncatedResponses(t *testing.T) {
	llm := &toolGovernanceStreamLLM{appChatResponses: []*adapter.ChatResponse{{Choices: []adapter.Choice{{FinishReason: "length", Message: adapter.Message{Content: `{"answer":"partial"}`}}}}}}
	svc := &service{llmClient: llm}
	channel, _ := svc.newSkillScriptModelChannel(&PreparedChat{Conversation: &runtimemodel.Conversation{}, LLMRequest: &adapter.ChatRequest{Model: "test"}})
	if _, err := channel.Complete(context.Background(), "system", "user"); err == nil {
		t.Fatal("truncated response accepted")
	}
}
