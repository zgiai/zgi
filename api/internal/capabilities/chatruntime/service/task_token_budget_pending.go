package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func (s *service) handleTaskTokenBudgetPending(
	ctx context.Context,
	prepared *PreparedChat,
	answer string,
	usage *adapter.Usage,
	cause error,
	onEvent func(StreamEvent) error,
) (*ChatResult, bool, error) {
	var exceeded *TaskTokenBudgetExceededError
	if prepared != nil && prepared.taskTokenBudget != nil {
		exceeded = prepared.taskTokenBudget.exhausted()
	}
	if exceeded == nil && cause != nil && !errors.As(cause, &exceeded) {
		return nil, false, nil
	}
	if exceeded == nil {
		return nil, false, nil
	}
	metadata, err := s.persistTaskTokenBudgetPendingResult(ctx, prepared, answer, usage)
	if err != nil {
		return nil, true, finalizedRuntimePersistenceError(err)
	}
	payload := taskTokenBudgetEventPayload(metadata, prepared.Conversation.ID.String(), prepared.Message.ID.String())
	if err := s.emitTaskTokenBudgetEvent(ctx, prepared, streamEventTokenBudgetRequested, payload, onEvent); err != nil {
		return nil, true, err
	}
	if err := s.emitTaskTokenBudgetEvent(ctx, prepared, streamEventMessageEnd, messageEndPayloadForBudget(prepared.Conversation.ID, prepared.Message.ID, metadata, answer, runtimemodel.MessageStatusWaitingBudget), onEvent); err != nil {
		return nil, true, err
	}
	return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusWaitingBudget}, true, nil
}

func (s *service) persistTaskTokenBudgetPendingResult(ctx context.Context, prepared *PreparedChat, answer string, usage *adapter.Usage) (map[string]interface{}, error) {
	if prepared == nil || prepared.Message == nil || prepared.Conversation == nil || prepared.taskTokenBudget == nil {
		return map[string]interface{}{}, nil
	}
	payload := taskTokenBudgetRequestPayload(prepared.taskTokenBudget)
	metadata := copyStringAnyMap(prepared.Message.Metadata)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadata[taskTokenBudgetMetadataKey] = prepared.taskTokenBudget.metadata("waiting")
	metadata["task_token_budget_request"] = payload
	delete(metadata, "user_input_request")
	metadata["task_token_budget_continuation"] = map[string]interface{}{
		"status":         runtimemodel.MessageStatusWaitingBudget,
		"request_id":     payload["request_id"],
		"original_query": prepared.Message.Query,
		"resume_policy":  "same_message",
	}
	prepared.Message.Answer = answer
	prepared.Message.Metadata = metadata
	metadata = preparedResultMetadataForPrepared(prepared, metadata, usage)
	// preparedResultMetadataForPrepared is also used by normal terminal turns and
	// therefore defaults the budget status to completed. A budget pause is not a
	// completed task: restore the independent waiting state after usage metadata
	// has been merged.
	metadata[taskTokenBudgetMetadataKey] = prepared.taskTokenBudget.metadata("waiting")
	prepared.Message.Metadata = metadata
	if s == nil || s.repos == nil || s.repos.Message == nil || s.repos.Conversation == nil {
		return metadata, nil
	}
	err := s.persistPendingMessageAndFinishConversationBestEffort(
		ctx,
		prepared,
		"task token budget",
		func(repo repository.MessageRepository) error {
			if updater, ok := repo.(interface {
				UpdateWaitingBudgetWithAnswer(context.Context, uuid.UUID, string, map[string]interface{}) error
			}); ok {
				return updater.UpdateWaitingBudgetWithAnswer(ctx, prepared.Message.ID, answer, metadata)
			}
			// Compatibility fallback for repository doubles that have not adopted
			// the answer-aware update yet. The production repository uses the
			// atomic method above.
			if strings.TrimSpace(answer) != "" {
				if err := repo.UpdatePartialAnswer(ctx, prepared.Message.ID, answer, metadata); err != nil {
					return err
				}
			}
			updater, ok := repo.(interface {
				UpdateWaitingBudget(context.Context, uuid.UUID, map[string]interface{}) error
			})
			if !ok {
				return fmt.Errorf("%w: waiting budget persistence is unavailable", ErrInvalidInput)
			}
			return updater.UpdateWaitingBudget(ctx, prepared.Message.ID, metadata)
		},
		func(repo repository.ConversationRepository) error {
			return repo.FinishWaitingApprovalMessage(ctx, prepared.Conversation.ID, prepared.Message.ID)
		},
	)
	if err != nil {
		return metadata, err
	}
	return metadata, nil
}

func (s *service) taskTokenBudgetRequest(ctx context.Context, scope Scope, caller Caller, conversationID, messageID uuid.UUID, requestID string) (map[string]interface{}, *runtimemodel.Message, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, nil, err
	}
	if normalizeConversationSource(caller.Source) != runtimemodel.ConversationSourceExternalAPI {
		return nil, nil, fmt.Errorf("%w: task token budget continuation is only available for external API calls", ErrInvalidInput)
	}
	conversation, err := s.getConversationByCallerScoped(ctx, scope, caller, conversationID)
	if err != nil {
		return nil, nil, err
	}
	message, err := s.repos.Message.GetScoped(ctx, messageID, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return nil, nil, mapRepoError(err)
	}
	if message.ConversationID != conversation.ID || conversation.CurrentLeafMessageID == nil || *conversation.CurrentLeafMessageID != message.ID {
		return nil, nil, ErrInvalidInput
	}
	request := governanceMapFromAny(message.Metadata["task_token_budget_request"])
	if strings.TrimSpace(stringFromAny(request["request_id"])) == strings.TrimSpace(requestID) &&
		strings.TrimSpace(stringFromAny(request["request_kind"])) == taskTokenBudgetRequestKind &&
		message.Status == runtimemodel.MessageStatusWaitingBudget {
		return request, message, nil
	}
	continuation := governanceMapFromAny(message.Metadata["task_token_budget_continuation"])
	if message.Status == runtimemodel.MessageStatusStreaming &&
		strings.TrimSpace(stringFromAny(continuation["request_id"])) == strings.TrimSpace(requestID) &&
		strings.TrimSpace(stringFromAny(continuation["status"])) == runtimemodel.MessageStatusStreaming {
		return continuation, message, nil
	}
	if message.Status != runtimemodel.MessageStatusWaitingBudget {
		return nil, nil, ErrInvalidInput
	}
	return nil, nil, ErrNotFound
}
