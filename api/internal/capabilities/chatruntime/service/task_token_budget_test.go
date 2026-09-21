package service

import (
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestTaskTokenBudgetAccumulatesProviderUsage(t *testing.T) {
	budget := newTaskTokenBudget(100)
	budget.addUsage(&adapter.Usage{PromptTokens: 20, CompletionTokens: 30, TotalTokens: 50})
	budget.addUsage(&adapter.Usage{PromptTokens: 12, CompletionTokens: 8})
	if budget.UsedTokens != 70 {
		t.Fatalf("used tokens = %d, want 70", budget.UsedTokens)
	}
	if budget.exhausted() != nil {
		t.Fatal("budget should not be exhausted")
	}
	budget.addUsage(&adapter.Usage{TotalTokens: 30})
	if budget.exhausted() == nil {
		t.Fatal("budget should be exhausted")
	}
}

func TestTaskTokenBudgetMetadataRoundTrip(t *testing.T) {
	original := newTaskTokenBudget(500)
	original.UsedTokens = 123
	metadata := map[string]interface{}{taskTokenBudgetMetadataKey: original.metadata("waiting_question")}
	loaded := taskTokenBudgetFromMetadata(metadata)
	if loaded == nil || loaded.MaxTokens != 500 || loaded.UsedTokens != 123 {
		t.Fatalf("loaded budget = %#v, want max=500 used=123", loaded)
	}
	if got := loaded.metadata("waiting_question")["remaining_tokens"]; got != 377 {
		t.Fatalf("remaining tokens = %#v, want 377", got)
	}
}

func TestTaskTokenUsageUpdatedPayloadContainsDeltaAndCumulativeBudget(t *testing.T) {
	conversationID := uuid.New()
	messageID := uuid.New()
	prepared := &PreparedChat{
		Conversation:    &runtimemodel.Conversation{ID: conversationID},
		Message:         &runtimemodel.Message{ID: messageID},
		Caller:          Caller{Source: runtimemodel.ConversationSourceExternalAPI},
		taskTokenBudget: newTaskTokenBudget(100),
	}
	prepared.taskTokenBudget.UsedTokens = 40
	payload := taskTokenUsageUpdatedPayload(prepared, &adapter.Usage{PromptTokens: 12, CompletionTokens: 8})
	if payload == nil {
		t.Fatal("payload = nil")
	}
	if payload["conversation_id"] != conversationID.String() || payload["message_id"] != messageID.String() {
		t.Fatalf("identity = %#v", payload)
	}
	usage, ok := payload["usage"].(map[string]interface{})
	if !ok || usage["total_tokens"] != 20 {
		t.Fatalf("usage = %#v, want total_tokens=20", payload["usage"])
	}
	budget, ok := payload["task_token_budget"].(map[string]interface{})
	if !ok || budget["used_tokens"] != 40 || budget["remaining_tokens"] != 60 || budget["status"] != "running" {
		t.Fatalf("task_token_budget = %#v", payload["task_token_budget"])
	}
	prepared.taskTokenBudget.addUsage(&adapter.Usage{TotalTokens: 60})
	payload = taskTokenUsageUpdatedPayload(prepared, &adapter.Usage{TotalTokens: 60})
	budget = payload["task_token_budget"].(map[string]interface{})
	if budget["status"] != "exhausted" || budget["remaining_tokens"] != 0 {
		t.Fatalf("exhausted task_token_budget = %#v", budget)
	}
}

func TestTaskTokenUsageUpdatedPayloadIncludesConsoleUsageWithoutBudget(t *testing.T) {
	prepared := &PreparedChat{
		Conversation:    &runtimemodel.Conversation{ID: uuid.New()},
		Message:         &runtimemodel.Message{ID: uuid.New()},
		Caller:          Caller{Source: runtimemodel.ConversationSourceConsole},
		taskTokenBudget: newTaskTokenBudget(100),
	}
	payload := taskTokenUsageUpdatedPayload(prepared, &adapter.Usage{TotalTokens: 1})
	if payload == nil {
		t.Fatal("console payload = nil")
	}
	if _, ok := payload["usage"]; !ok {
		t.Fatalf("console payload = %#v, want usage", payload)
	}
	if _, ok := payload["task_token_budget"]; ok {
		t.Fatalf("console payload = %#v, must not include API task budget", payload)
	}
}

func TestTaskTokenBudgetRequestPayloadIsIndependentFromUserInput(t *testing.T) {
	budget := newTaskTokenBudget(100)
	budget.UsedTokens = 125
	payload := taskTokenBudgetRequestPayload(budget)
	if payload["request_kind"] != taskTokenBudgetRequestKind {
		t.Fatalf("request_kind = %#v", payload["request_kind"])
	}
	if _, ok := payload["questions"]; ok {
		t.Fatalf("budget request must not contain user-input questions: %#v", payload)
	}
	if len(payload["actions"].([]interface{})) != 2 {
		t.Fatalf("actions = %#v", payload["actions"])
	}
	tokenBudget := payload["token_budget"].(map[string]interface{})
	if tokenBudget["status"] != "waiting" || tokenBudget["used_tokens"] != 125 || tokenBudget["remaining_tokens"] != 0 {
		t.Fatalf("token_budget = %#v", tokenBudget)
	}
}

func TestTaskTokenBudgetMessageEndPayloadPreservesPartialAnswer(t *testing.T) {
	conversationID := uuid.New()
	messageID := uuid.New()
	answer := "已经完成了一部分"
	payload := messageEndPayloadForBudget(
		conversationID,
		messageID,
		map[string]interface{}{"task_token_budget": map[string]interface{}{"status": "waiting"}},
		answer,
		runtimemodel.MessageStatusWaitingBudget,
	)
	if payload["conversation_id"] != conversationID.String() || payload["message_id"] != messageID.String() {
		t.Fatalf("identity = %#v", payload)
	}
	if payload["status"] != runtimemodel.MessageStatusWaitingBudget {
		t.Fatalf("status = %#v", payload["status"])
	}
	if payload["answer"] != answer {
		t.Fatalf("answer = %#v, want %q", payload["answer"], answer)
	}
}
