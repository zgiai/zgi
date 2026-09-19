package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

func (s *service) runTaskTokenBudgetContinuationStream(
	ctx context.Context,
	scope Scope,
	caller Caller,
	config RunConfig,
	conversationID uuid.UUID,
	messageID uuid.UUID,
	requestID string,
	req runtimedto.UserInputContinuationRequest,
	onEvent func(StreamEvent) error,
) (*ChatResult, error) {
	if onEvent == nil {
		return nil, fmt.Errorf("%w: event callback is required", ErrInvalidInput)
	}
	_, message, err := s.taskTokenBudgetRequest(ctx, scope, caller, conversationID, messageID, requestID)
	if err != nil {
		return nil, err
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	switch action {
	case "stop":
		stopped, err := s.StopMessage(ctx, scope, messageID)
		if err != nil {
			return nil, err
		}
		if stopped == nil {
			return nil, fmt.Errorf("%w: stopped message is unavailable", ErrInvalidInput)
		}
		metadata := copyStringAnyMap(stopped.Metadata)
		if budget := taskTokenBudgetFromMetadata(message.Metadata); budget != nil {
			if metadata == nil {
				metadata = map[string]interface{}{}
			}
			metadata[taskTokenBudgetMetadataKey] = budget.metadata("stopped")
			if s.repos != nil && s.repos.Message != nil {
				if err := s.repos.Message.UpdateMetadata(ctx, messageID, metadata); err != nil {
					return nil, err
				}
			}
			stopped.Metadata = metadata
		}
		payload := map[string]interface{}{
			"conversation_id": conversationID.String(),
			"message_id":      messageID.String(),
			"status":          runtimemodel.MessageStatusStopped,
			"metadata":        ClientVisibleMessageMetadata(metadata),
			"answer":          stopped.Answer,
		}
		s.appendStreamEventBestEffort(ctx, messageID, conversationID, streamEventMessageEnd, payload)
		if err := onEvent(StreamEvent{EventType: streamEventMessageEnd, Payload: payload}); err != nil {
			return nil, err
		}
		return &ChatResult{Status: runtimemodel.MessageStatusStopped}, nil
	case "add":
		if req.AdditionalTokens <= 0 || req.AdditionalTokens > maxTaskTokenBudget {
			return nil, fmt.Errorf("%w: additional_tokens must be between 1 and %d", ErrInvalidInput, maxTaskTokenBudget)
		}
		budget := taskTokenBudgetFromMetadata(message.Metadata)
		if budget == nil {
			return nil, fmt.Errorf("%w: task token budget is unavailable", ErrInvalidInput)
		}
		if budget.MaxTokens > maxTaskTokenBudget-req.AdditionalTokens {
			return nil, fmt.Errorf("%w: task token budget is too large", ErrInvalidInput)
		}
		// Re-enter the existing user-input continuation path with a structured
		// answer. This preserves the same message, conversation leaf and worker
		// lease semantics used by all other Skill pauses.
		normalized := runtimedto.UserInputContinuationRequest{
			Answers: map[string]string{
				"action":            action,
				"additional_tokens": strconv.Itoa(req.AdditionalTokens),
			},
			Surface:          req.Surface,
			RuntimeContext:   req.RuntimeContext,
			OperationContext: req.OperationContext,
		}
		return s.runConfiguredUserInputContinuationStreamWithBudgetIncrease(ctx, scope, caller, config, conversationID, messageID, requestID, normalized, req.AdditionalTokens, onEvent)
	default:
		return nil, fmt.Errorf("%w: action must be add or stop", ErrInvalidInput)
	}
}

func (s *service) runConfiguredUserInputContinuationStreamWithBudgetIncrease(
	ctx context.Context,
	scope Scope,
	caller Caller,
	config RunConfig,
	conversationID uuid.UUID,
	messageID uuid.UUID,
	requestID string,
	req runtimedto.UserInputContinuationRequest,
	additional int,
	onEvent func(StreamEvent) error,
) (*ChatResult, error) {
	continuation, err := s.beginUserInputContinuation(ctx, scope, caller, conversationID, messageID, requestID, req.Answers)
	if err != nil {
		return nil, err
	}
	prepared, err := s.prepareUserInputContinuationChat(ctx, scope, caller, config, continuation, req)
	if err != nil {
		s.failUserInputContinuation(context.WithoutCancel(ctx), continuation, err, onEvent)
		return nil, newFinalizedStreamError(err)
	}
	if prepared.taskTokenBudget == nil {
		prepared.taskTokenBudget = taskTokenBudgetFromMetadata(messageMetadataAfterContinuation(continuation))
	}
	if prepared.taskTokenBudget == nil {
		return nil, fmt.Errorf("%w: task token budget is unavailable", ErrInvalidInput)
	}
	prepared.taskTokenBudget.MaxTokens += additional
	if prepared.Message.Metadata == nil {
		prepared.Message.Metadata = map[string]interface{}{}
	}
	prepared.Message.Metadata[taskTokenBudgetMetadataKey] = prepared.taskTokenBudget.metadata("running")
	if s.repos != nil && s.repos.Message != nil {
		if err := s.repos.Message.UpdateMetadata(context.WithoutCancel(ctx), prepared.Message.ID, prepared.Message.Metadata); err != nil {
			s.failUserInputContinuation(context.WithoutCancel(ctx), continuation, err, onEvent)
			return nil, finalizedRuntimePersistenceError(err)
		}
	}
	return s.runPreparedUserInputContinuation(ctx, prepared, continuation, onEvent)
}

func messageMetadataAfterContinuation(continuation *UserInputContinuation) map[string]interface{} {
	if continuation == nil || continuation.Message == nil {
		return nil
	}
	return continuation.Message.Metadata
}

func (s *service) runPreparedUserInputContinuation(ctx context.Context, prepared *PreparedChat, continuation *UserInputContinuation, onEvent func(StreamEvent) error) (*ChatResult, error) {
	execution, err := s.beginRuntimeExecution(ctx, prepared.Message.ID)
	if err != nil {
		s.failUserInputContinuation(context.WithoutCancel(ctx), continuation, err, onEvent)
		return nil, newFinalizedStreamError(err)
	}
	defer execution.Finish()
	runCtx, persistCtx := execution.Context, execution.PersistContext
	if s.streams.IsStopped(prepared.Message.ID, execution.runID) {
		_ = s.persistStoppedAnswer(persistCtx, prepared, "", nil)
		return nil, ErrMessageStopped
	}
	s.emitPreparedEvent(persistCtx, prepared, streamEventMessageStart, messageStartPayload(continuation.Conversation, continuation.Message, false), onEvent)
	answer, usage, err := s.runPreparedToolLoop(runCtx, persistCtx, prepared, nil, onEvent)
	if err != nil {
		return s.finishUserInputContinuationPendingOrError(persistCtx, prepared, answer, usage, err, onEvent)
	}
	if budgetErr := prepared.taskTokenBudget.exhausted(); budgetErr != nil {
		return s.finishUserInputContinuationPendingOrError(persistCtx, prepared, answer, usage, budgetErr, onEvent)
	}
	metadata := preparedResultMetadataForPrepared(prepared, prepared.Message.Metadata, usage)
	if err := s.completePreparedChat(persistCtx, prepared, answer, metadata); err != nil {
		return nil, finalizedRuntimePersistenceError(err)
	}
	s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayload(prepared, metadata), onEvent)
	return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusCompleted}, nil
}
