package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
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
	metadata, err := s.persistTaskTokenBudgetPendingResult(ctx, prepared, usage)
	if err != nil {
		return nil, true, finalizedRuntimePersistenceError(err)
	}
	payload := taskTokenBudgetEventPayload(metadata, prepared.Conversation.ID.String(), prepared.Message.ID.String())
	if err := s.emitPreparedEvent(ctx, prepared, streamEventTokenBudgetRequested, payload, onEvent); err != nil {
		return nil, true, err
	}
	s.emitPreparedEvent(ctx, prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingQuestion), onEvent)
	return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusWaitingQuestion}, true, nil
}

func (s *service) persistTaskTokenBudgetPendingResult(ctx context.Context, prepared *PreparedChat, usage *adapter.Usage) (map[string]interface{}, error) {
	if prepared == nil || prepared.Message == nil || prepared.Conversation == nil || prepared.taskTokenBudget == nil {
		return map[string]interface{}{}, nil
	}
	payload := taskTokenBudgetRequestPayload(prepared.taskTokenBudget)
	metadata := copyStringAnyMap(prepared.Message.Metadata)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadata[taskTokenBudgetMetadataKey] = prepared.taskTokenBudget.metadata("waiting_question")
	prepared.Message.Metadata = metadata
	metadata, err := s.persistUserInputRequestPendingResult(ctx, prepared, payload, usage)
	if err != nil {
		return metadata, err
	}
	metadata[taskTokenBudgetMetadataKey] = prepared.taskTokenBudget.metadata("waiting_question")
	if metadata["user_input_request"] != nil {
		request := mapFromOperationContext(metadata["user_input_request"])
		request["request_kind"] = taskTokenBudgetRequestKind
		request["token_budget"] = prepared.taskTokenBudget.metadata("waiting_question")
		metadata["user_input_request"] = request
	}
	prepared.Message.Metadata = metadata
	if s.repos != nil && s.repos.Message != nil {
		if err := s.repos.Message.UpdateMetadata(ctx, prepared.Message.ID, metadata); err != nil {
			return metadata, err
		}
	}
	return metadata, nil
}

func (s *service) taskTokenBudgetRequest(ctx context.Context, scope Scope, caller Caller, conversationID, messageID uuid.UUID, requestID string) (map[string]interface{}, *runtimemodel.Message, error) {
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
	request := governanceMapFromAny(message.Metadata["user_input_request"])
	if strings.TrimSpace(stringFromAny(request["request_id"])) != strings.TrimSpace(requestID) ||
		strings.TrimSpace(stringFromAny(request["request_kind"])) != taskTokenBudgetRequestKind {
		return nil, nil, ErrNotFound
	}
	if message.Status != runtimemodel.MessageStatusWaitingQuestion {
		return nil, nil, ErrInvalidInput
	}
	return request, message, nil
}
