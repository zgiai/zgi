package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	taskTokenBudgetMetadataKey = "task_token_budget"
	taskTokenBudgetRequestKind = "task_token_budget"
	maxTaskTokenBudget         = 100_000_000
)

// taskTokenBudgetState is intentionally in-memory while a turn is running;
// the compact representation is copied to message metadata after every model
// invocation so an API continuation can resume after a worker restart.
type taskTokenBudgetState struct {
	MaxTokens  int
	UsedTokens int
}

type TaskTokenBudgetExceededError struct {
	UsedTokens int
	MaxTokens  int
}

func (e *TaskTokenBudgetExceededError) Error() string {
	if e == nil {
		return "task token budget exhausted"
	}
	return fmt.Sprintf("task token budget exhausted: used %d of %d tokens", e.UsedTokens, e.MaxTokens)
}

func newTaskTokenBudget(maxTokens int) *taskTokenBudgetState {
	if maxTokens <= 0 {
		return nil
	}
	if maxTokens > maxTaskTokenBudget {
		maxTokens = maxTaskTokenBudget
	}
	return &taskTokenBudgetState{MaxTokens: maxTokens}
}

func taskTokenBudgetFromMetadata(metadata map[string]interface{}) *taskTokenBudgetState {
	values := mapFromOperationContext(metadata[taskTokenBudgetMetadataKey])
	if len(values) == 0 {
		return nil
	}
	budget := newTaskTokenBudget(intValueFromAny(values["max_tokens"]))
	if budget == nil {
		return nil
	}
	budget.UsedTokens = max(0, intValueFromAny(values["used_tokens"]))
	return budget
}

func (b *taskTokenBudgetState) addUsage(usage *adapter.Usage) {
	if b == nil || usage == nil {
		return
	}
	total := usage.TotalTokens
	if total <= 0 {
		total = usage.PromptTokens + usage.CompletionTokens
	}
	if total > 0 {
		b.UsedTokens += total
	}
}

func (b *taskTokenBudgetState) exhausted() *TaskTokenBudgetExceededError {
	if b == nil || b.MaxTokens <= 0 || b.UsedTokens < b.MaxTokens {
		return nil
	}
	return &TaskTokenBudgetExceededError{UsedTokens: b.UsedTokens, MaxTokens: b.MaxTokens}
}

func (b *taskTokenBudgetState) metadata(status string) map[string]interface{} {
	if b == nil || b.MaxTokens <= 0 {
		return nil
	}
	if strings.TrimSpace(status) == "" {
		status = "running"
	}
	return map[string]interface{}{
		"max_tokens":       b.MaxTokens,
		"used_tokens":      b.UsedTokens,
		"remaining_tokens": max(0, b.MaxTokens-b.UsedTokens),
		"status":           status,
	}
}

func taskTokenBudgetRequestPayload(budget *taskTokenBudgetState) map[string]interface{} {
	requestID := uuid.New().String()
	used, limit := 0, 0
	if budget != nil {
		used, limit = budget.UsedTokens, budget.MaxTokens
	}
	return map[string]interface{}{
		"request_id":   requestID,
		"request_kind": taskTokenBudgetRequestKind,
		"created_at":   time.Now().Unix(),
		"message":      fmt.Sprintf("本次任务已使用 %d tokens，预算上限为 %d tokens。请追加预算或停止运行。", used, limit),
		"questions": []interface{}{
			map[string]interface{}{"id": "action", "question": "请选择追加任务预算或停止运行"},
			map[string]interface{}{"id": "additional_tokens", "question": "追加 token 数量（选择追加时填写）"},
		},
		"token_budget": map[string]interface{}{
			"used_tokens":      used,
			"max_tokens":       limit,
			"remaining_tokens": max(0, limit-used),
			"actions": []interface{}{
				map[string]interface{}{"id": "add", "label": "追加预算"},
				map[string]interface{}{"id": "stop", "label": "停止运行"},
			},
		},
	}
}

// taskTokenUsageUpdatedPayload is the public progress payload for token
// accounting. Usage is emitted for every runtime surface so console/debug UIs
// can show actual consumption. The cumulative task budget is intentionally
// included only for API requests that supplied a task limit.
func taskTokenUsageUpdatedPayload(prepared *PreparedChat, usage *adapter.Usage) map[string]interface{} {
	if prepared == nil || prepared.Conversation == nil || prepared.Message == nil || usage == nil {
		return nil
	}
	total := usage.TotalTokens
	if total <= 0 {
		total = usage.PromptTokens + usage.CompletionTokens
	}
	if total <= 0 {
		return nil
	}
	payload := map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"usage": map[string]interface{}{
			"prompt_tokens":     usage.PromptTokens,
			"completion_tokens": usage.CompletionTokens,
			"total_tokens":      total,
		},
	}
	if prepared.taskTokenBudget != nil && normalizeConversationSource(prepared.Caller.Source) == runtimemodel.ConversationSourceExternalAPI {
		status := "running"
		if prepared.taskTokenBudget.exhausted() != nil {
			status = "exhausted"
		}
		payload["task_token_budget"] = prepared.taskTokenBudget.metadata(status)
	}
	return payload
}

func (s *service) emitTaskTokenUsageUpdated(ctx context.Context, prepared *PreparedChat, usage *adapter.Usage, onEvent func(StreamEvent) error) {
	payload := taskTokenUsageUpdatedPayload(prepared, usage)
	if len(payload) == 0 {
		return
	}
	_ = s.emitPreparedEvent(ctx, prepared, streamEventTokenUsageUpdated, payload, onEvent)
}

func taskTokenBudgetEventPayload(metadata map[string]interface{}, conversationID, messageID string) map[string]interface{} {
	payload := copyStringAnyMap(mapFromOperationContext(metadata["user_input_request"]))
	if payload == nil {
		payload = map[string]interface{}{}
	}
	payload["conversation_id"] = conversationID
	payload["message_id"] = messageID
	return payload
}
