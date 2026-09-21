package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/gorm"
)

func (s *service) RunConfiguredTaskTokenBudgetContinuationStream(
	ctx context.Context,
	scope Scope,
	caller Caller,
	config RunConfig,
	conversationID uuid.UUID,
	messageID uuid.UUID,
	requestID string,
	req runtimedto.TaskTokenBudgetContinuationRequest,
	onEvent func(StreamEvent) error,
) (*ChatResult, error) {
	if onEvent == nil {
		return nil, fmt.Errorf("%w: event callback is required", ErrInvalidInput)
	}
	request, message, err := s.taskTokenBudgetRequest(ctx, scope, caller, conversationID, messageID, requestID)
	if err != nil {
		return nil, err
	}
	switch action := strings.ToLower(strings.TrimSpace(req.Action)); action {
	case "stop":
		return s.stopTaskTokenBudget(ctx, scope, conversationID, messageID, message, onEvent)
	case "add":
		if message.Status == runtimemodel.MessageStatusStreaming {
			if err := s.StreamConversationEventsForCaller(ctx, scope, caller, conversationID, messageID, "", onEvent); err != nil {
				return nil, err
			}
			return &ChatResult{Status: runtimemodel.MessageStatusStreaming, Metadata: message.Metadata}, nil
		}
		return s.addTaskTokenBudget(ctx, scope, caller, config, conversationID, messageID, message, request, req, onEvent)
	default:
		return nil, fmt.Errorf("%w: action must be add or stop", ErrInvalidInput)
	}
}

// ValidateTaskTokenBudgetContinuation checks the durable budget request
// without changing message or conversation state. DigitalHuman uses this
// guard before transitioning its own Run from waiting to running so an
// expired or mismatched request cannot resume a run accidentally.
func (s *service) ValidateTaskTokenBudgetContinuation(
	ctx context.Context,
	scope Scope,
	caller Caller,
	conversationID uuid.UUID,
	messageID uuid.UUID,
	requestID string,
) error {
	_, _, err := s.taskTokenBudgetRequest(ctx, scope, caller, conversationID, messageID, requestID)
	return err
}

func (s *service) stopTaskTokenBudget(
	ctx context.Context,
	scope Scope,
	conversationID, messageID uuid.UUID,
	message *runtimemodel.Message,
	onEvent func(StreamEvent) error,
) (*ChatResult, error) {
	budget := taskTokenBudgetFromMetadata(message.Metadata)
	stopped, err := s.StopMessage(ctx, scope, messageID)
	if err != nil {
		return nil, err
	}
	if stopped == nil {
		return nil, fmt.Errorf("%w: stopped message is unavailable", ErrInvalidInput)
	}
	metadata := copyStringAnyMap(stopped.Metadata)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	delete(metadata, "task_token_budget_request")
	if budget != nil {
		metadata[taskTokenBudgetMetadataKey] = budget.metadata("stopped")
	}
	metadata["task_token_budget_continuation"] = map[string]interface{}{
		"status":     runtimemodel.MessageStatusStopped,
		"stopped_at": time.Now().Unix(),
	}
	if s.repos != nil && s.repos.Message != nil {
		if err := s.repos.Message.UpdateMetadataAnyStatus(ctx, messageID, metadata); err != nil {
			return nil, err
		}
	}
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: conversationID},
		Message:      &runtimemodel.Message{ID: messageID},
	}
	payload := messageEndPayloadForBudget(conversationID, messageID, metadata, stopped.Answer, runtimemodel.MessageStatusStopped)
	if err := s.emitTaskTokenBudgetEvent(ctx, prepared, streamEventMessageEnd, payload, onEvent); err != nil {
		return nil, err
	}
	return &ChatResult{Status: runtimemodel.MessageStatusStopped, Metadata: metadata}, nil
}

func (s *service) addTaskTokenBudget(
	ctx context.Context,
	scope Scope,
	caller Caller,
	config RunConfig,
	conversationID, messageID uuid.UUID,
	message *runtimemodel.Message,
	request map[string]interface{},
	req runtimedto.TaskTokenBudgetContinuationRequest,
	onEvent func(StreamEvent) error,
) (*ChatResult, error) {
	if req.AdditionalTokens < 1 || req.AdditionalTokens > maxTaskTokenBudget {
		return nil, fmt.Errorf("%w: additional_tokens must be between 1 and %d", ErrInvalidInput, maxTaskTokenBudget)
	}
	budget := taskTokenBudgetFromMetadata(message.Metadata)
	if budget == nil {
		return nil, fmt.Errorf("%w: task token budget is unavailable", ErrInvalidInput)
	}
	if budget.MaxTokens > maxTaskTokenBudget-req.AdditionalTokens {
		return nil, fmt.Errorf("%w: task token budget is too large", ErrInvalidInput)
	}
	conversation, err := s.getConversationByCallerScoped(ctx, scope, caller, conversationID)
	if err != nil {
		return nil, err
	}
	if err := ensureConversationWorkspaceScope(scope, conversation); err != nil {
		return nil, err
	}
	if message.ConversationID != conversation.ID || conversation.CurrentLeafMessageID == nil || *conversation.CurrentLeafMessageID != message.ID {
		return nil, fmt.Errorf("%w: message is not the current conversation leaf", ErrInvalidInput)
	}
	if message.Status != runtimemodel.MessageStatusWaitingBudget {
		return nil, fmt.Errorf("%w: message is not waiting for token budget", ErrInvalidInput)
	}

	budget.MaxTokens += req.AdditionalTokens
	metadata := copyStringAnyMap(message.Metadata)
	delete(metadata, "task_token_budget_request")
	metadata[taskTokenBudgetMetadataKey] = budget.metadata("running")
	metadata["task_token_budget_continuation"] = map[string]interface{}{
		"status":       runtimemodel.MessageStatusStreaming,
		"request_kind": taskTokenBudgetRequestKind,
		"request_id":   request["request_id"],
		"resumed_at":   time.Now().Unix(),
	}
	// Build the continuation before claiming the waiting message. Preparation
	// is read-only; doing it first prevents a model/configuration error from
	// leaving the durable message stuck in `streaming`.
	prepared, err := s.prepareTaskTokenBudgetContinuationChat(ctx, scope, caller, config, conversation, message, req)
	if err != nil {
		return nil, err
	}
	prepared.taskTokenBudget = budget
	if s.repos.DB == nil {
		return nil, fmt.Errorf("%w: chat runtime database is unavailable", ErrInvalidInput)
	}
	err = s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&runtimemodel.Message{}).
			Where("id = ? AND deleted_at IS NULL AND status = ?", message.ID, runtimemodel.MessageStatusWaitingBudget).
			Updates(map[string]interface{}{"status": runtimemodel.MessageStatusStreaming, "error": nil, "metadata": metadata})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return newContinuationAlreadyRunningError("task token budget continuation is already running; reconnect to the active stream")
		}
		return repository.NewRepositories(tx).Conversation.StartStreaming(ctx, conversation.ID, scope.OrganizationID, scope.AccountID, message.ID)
	})
	if err != nil {
		return nil, mapRepoError(err)
	}
	message.Status = runtimemodel.MessageStatusStreaming
	message.Metadata = metadata
	conversation.RuntimeStatus = runtimemodel.ConversationRuntimeStatusStreaming
	conversation.ActiveMessageID = &message.ID
	prepared.Message = message
	prepared.Conversation = conversation
	return s.runPreparedTaskTokenBudgetContinuation(ctx, prepared, onEvent)
}

func (s *service) prepareTaskTokenBudgetContinuationChat(
	ctx context.Context,
	scope Scope,
	caller Caller,
	config RunConfig,
	conversation *runtimemodel.Conversation,
	message *runtimemodel.Message,
	req runtimedto.TaskTokenBudgetContinuationRequest,
) (*PreparedChat, error) {
	if conversation == nil || message == nil {
		return nil, fmt.Errorf("%w: task token budget continuation is required", ErrInvalidInput)
	}
	if err := ensureConversationWorkspaceScope(scope, conversation); err != nil {
		return nil, err
	}
	parts, err := normalizeRegenerateRequest(applyRunConfigToRegenerateRequest(config, runtimedto.RegenerateMessageRequest{
		Surface: req.Surface, RuntimeContext: req.RuntimeContext, OperationContext: req.OperationContext,
	}), message)
	if err != nil {
		return nil, err
	}
	applyRunConfigToParts(config, parts)
	applyCallerRuntimeSurfacePolicy(caller, parts)
	applyPersistedConversationSurface(conversation, parts)
	restoreExecutionModeFromMetadata(parts, message.Metadata)
	restoreConsoleFilesContextFromMetadata(parts, message.Metadata, nil)
	restoreConsoleAgentsContextFromMetadata(parts, message.Metadata, nil)
	restoreTurnInitialContextFromMetadata(parts, message.Metadata)
	restoreCurrentPageContextFromMetadata(parts, message.Metadata)
	parts.Attachments = attachmentBundleFromMessageMetadata(message.Metadata)
	callerType := normalizeCallerType(caller.Type)
	if !isAutonomousSubjectCaller(callerType) {
		if configured, ok := stringSliceValue(message.Metadata["configured_skill_ids"]); ok && len(configured) > 0 {
			parts.ConfiguredSkillIDs = configured
		}
	}
	if err := s.applyModelCapabilities(ctx, scope, caller, parts); err != nil {
		return nil, err
	}
	applyProtocolToolsPolicy(caller, parts)
	applyCallerMemoryPolicy(caller, parts)
	if err := s.applySkillConfig(ctx, scope, caller, &config, parts); err != nil {
		return nil, err
	}
	applyAgentMemoryToolsPolicy(parts)
	contextResult, err := s.buildUpstreamMessages(ctx, scope, message.ParentID, parts, message.ConversationID)
	if err != nil {
		return nil, err
	}
	parts.ContextControl = contextResult.Metadata
	llmRequest := newLLMChatRequest(parts, contextResult.Messages)
	s.appendPersistedAgentTranscript(ctx, llmRequest, message)
	if stateMessage := currentTurnAuthoritativeStateMessage(message); stateMessage != nil {
		llmRequest.Messages = append(llmRequest.Messages, *stateMessage)
	}
	llmRequest.Messages = append(llmRequest.Messages, taskTokenBudgetContinuationMessage(message, req.AdditionalTokens))
	return &PreparedChat{
		Conversation: conversation, Message: message, LLMRequest: llmRequest, Scope: scope, Caller: caller,
		RunConfig: config, ParentID: message.ParentID, Continuation: true, ContinuationType: "task_token_budget",
		parts: parts, contextBudget: contextResult,
	}, nil
}

func taskTokenBudgetContinuationMessage(message *runtimemodel.Message, additional int) adapter.Message {
	original := ""
	if message != nil {
		original = strings.TrimSpace(message.Query)
	}
	return adapter.Message{Role: "user", Content: fmt.Sprintf(
		"The same assistant task is resuming after the caller added %d model tokens to its task budget. Continue from the authoritative same-turn state; do not restart completed work or ask the caller to approve the budget again. Original request: %s",
		additional, original,
	)}
}

func (s *service) runPreparedTaskTokenBudgetContinuation(ctx context.Context, prepared *PreparedChat, onEvent func(StreamEvent) error) (*ChatResult, error) {
	execution, err := s.beginRuntimeExecution(ctx, prepared.Message.ID)
	if err != nil {
		s.finalizePreparedError(context.WithoutCancel(ctx), prepared, err, onEvent)
		return nil, newFinalizedStreamError(err)
	}
	defer execution.Finish()
	if s.streams.IsStopped(prepared.Message.ID, execution.runID) {
		_ = s.persistStoppedAnswer(execution.PersistContext, prepared, "", nil)
		return nil, ErrMessageStopped
	}
	s.emitPreparedEvent(execution.PersistContext, prepared, streamEventMessageStart, messageStartPayload(prepared.Conversation, prepared.Message, false), onEvent)
	if !prepared.toolLoopEnabled() {
		return s.runPreparedDirectTaskTokenBudgetContinuation(execution, prepared, onEvent)
	}
	answer, usage, err := s.runPreparedToolLoop(execution.Context, execution.PersistContext, prepared, nil, onEvent)
	if err != nil {
		return s.finishContinuationPendingOrError(execution.PersistContext, prepared, answer, usage, err, onEvent)
	}
	if budgetErr := prepared.taskTokenBudget.exhausted(); budgetErr != nil {
		return s.finishContinuationPendingOrError(execution.PersistContext, prepared, answer, usage, budgetErr, onEvent)
	}
	if s.streams.IsStopped(prepared.Message.ID, execution.runID) {
		_ = s.persistStoppedAnswer(execution.PersistContext, prepared, answer, usage)
		return nil, ErrMessageStopped
	}
	metadata := preparedResultMetadataForPrepared(prepared, prepared.Message.Metadata, usage)
	if err := s.completePreparedChat(execution.PersistContext, prepared, answer, metadata); err != nil {
		return nil, finalizedRuntimePersistenceError(err)
	}
	s.emitPreparedEvent(execution.PersistContext, prepared, streamEventMessageEnd, messageEndPayload(prepared, metadata), onEvent)
	return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusCompleted}, nil
}

func (s *service) runPreparedDirectTaskTokenBudgetContinuation(
	execution *runtimeExecution,
	prepared *PreparedChat,
	onEvent func(StreamEvent) error,
) (*ChatResult, error) {
	appendDirectAnswerGroundingMessage(prepared)
	ctx := execution.Context
	persistCtx := execution.PersistContext
	startedAt := time.Now()
	modelProgress := s.startPreparedModelProgress(ctx, prepared, 0, prepared.LLMRequest.Messages, onEvent)
	defer modelProgress.Stop()
	stream, err := s.openChatStream(ctx, prepared)
	if err != nil {
		s.persistModelInvocationBestEffort(persistCtx, prepared, skillloop.ModelInvocationTrace{
			Phase: "final_answer", Round: -1, Streaming: true, StartedAt: startedAt,
			DurationMS: time.Since(startedAt).Milliseconds(), Request: prepared.LLMRequest, Error: err.Error(),
		})
		if s.isStoppedContext(ctx, prepared.Message.ID) {
			_ = s.persistStoppedAnswer(persistCtx, prepared, "", nil)
			return nil, ErrMessageStopped
		}
		s.finalizePreparedError(persistCtx, prepared, err, onEvent)
		return nil, newFinalizedStreamError(err)
	}
	answer, callUsage, err := s.collectStreamAnswerWithEventsAndProgress(ctx, prepared, stream, onEvent, nil, modelProgress)
	if settleErr := s.settleClaimedRuntimeInputBatches(persistCtx, prepared, err == nil); settleErr != nil {
		err = errors.Join(err, settleErr)
	}
	if err != nil {
		accountTaskTokenUsage(prepared, callUsage)
		s.persistModelInvocationBestEffort(persistCtx, prepared, skillloop.ModelInvocationTrace{
			Phase: "final_answer", Round: -1, Streaming: true, StartedAt: startedAt,
			DurationMS: time.Since(startedAt).Milliseconds(), Request: prepared.LLMRequest,
			Response: &adapter.Message{Role: "assistant", Content: answer}, Usage: callUsage, Error: err.Error(),
		})
		s.emitTaskTokenUsageUpdated(persistCtx, prepared, callUsage, onEvent)
		if result, handled, budgetErr := s.handleTaskTokenBudgetPending(persistCtx, prepared, answer, callUsage, err, onEvent); handled {
			return result, budgetErr
		}
		if errors.Is(err, ErrMessageStopped) {
			_ = s.clearPreparedRuntime(persistCtx, prepared)
			return nil, err
		}
		s.finalizePreparedError(persistCtx, prepared, err, onEvent)
		return nil, newFinalizedStreamError(err)
	}
	accountTaskTokenUsage(prepared, callUsage)
	s.persistModelInvocationBestEffort(persistCtx, prepared, skillloop.ModelInvocationTrace{
		Phase: "final_answer", Round: -1, Streaming: true, StartedAt: startedAt,
		DurationMS: time.Since(startedAt).Milliseconds(), Request: prepared.LLMRequest,
		Response: &adapter.Message{Role: "assistant", Content: answer}, Usage: callUsage,
	})
	s.emitTaskTokenUsageUpdated(persistCtx, prepared, callUsage, onEvent)
	if result, handled, budgetErr := s.handleTaskTokenBudgetPending(persistCtx, prepared, answer, callUsage, nil, onEvent); handled {
		return result, budgetErr
	}
	if s.streams.IsStopped(prepared.Message.ID, execution.runID) {
		_ = s.persistStoppedAnswer(persistCtx, prepared, answer, callUsage)
		return nil, ErrMessageStopped
	}
	metadata := preparedResultMetadataForPrepared(prepared, prepared.Message.Metadata, callUsage)
	if err := s.completePreparedChat(persistCtx, prepared, answer, metadata); err != nil {
		return nil, finalizedRuntimePersistenceError(err)
	}
	s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayload(prepared, metadata), onEvent)
	return &ChatResult{Answer: answer, Metadata: metadata, Usage: callUsage, Status: runtimemodel.MessageStatusCompleted}, nil
}

func messageEndPayloadForBudget(conversationID, messageID uuid.UUID, metadata map[string]interface{}, answer, status string) map[string]interface{} {
	return map[string]interface{}{
		"conversation_id": conversationID.String(),
		"message_id":      messageID.String(),
		"status":          status,
		"metadata":        ClientVisibleMessageMetadata(metadata),
		"answer":          answer,
	}
}
