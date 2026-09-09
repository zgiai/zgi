package service

import (
	"context"
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

func TestSkillScriptModelRejectsTruncatedResponses(t *testing.T) {
	llm := &toolGovernanceStreamLLM{appChatResponses: []*adapter.ChatResponse{{Choices: []adapter.Choice{{FinishReason: "length", Message: adapter.Message{Content: `{"answer":"partial"}`}}}}}}
	svc := &service{llmClient: llm}
	channel, _ := svc.newSkillScriptModelChannel(&PreparedChat{Conversation: &runtimemodel.Conversation{}, LLMRequest: &adapter.ChatRequest{Model: "test"}})
	if _, err := channel.Complete(context.Background(), "system", "user"); err == nil {
		t.Fatal("truncated response accepted")
	}
}
