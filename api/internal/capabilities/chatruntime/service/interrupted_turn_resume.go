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
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/gorm"
)

const (
	interruptedTurnResumeKey        = "interrupted_turn_resume"
	interruptedTurnContinuationType = "interrupted_turn"
	// skillCallReadOnlyKey marks a call's start when its runtime tool is
	// read-only; the skill loop sets it in the start event.
	skillCallReadOnlyKey = "read_only"
)

// ErrInterruptedTurnNotResumable reports a turn that stopped with its process
// and must not be resumed: it is not an interrupted turn, it was resumed
// before, or a call that may have changed something has no recorded result.
var ErrInterruptedTurnNotResumable = errors.New("interrupted turn cannot be resumed")

// RunInterruptedTurnResumeStream resumes an AI chat turn whose process stopped
// in the middle of it, as the AI chat's other continuations run: with its
// caller and billing (ADR 0042).
func (s *service) RunInterruptedTurnResumeStream(
	ctx context.Context,
	scope Scope,
	conversationID uuid.UUID,
	messageID uuid.UUID,
	onEvent func(StreamEvent) error,
) (*ChatResult, error) {
	return s.RunConfiguredInterruptedTurnResume(
		ctx,
		scope,
		Caller{Type: runtimemodel.ConversationCallerAIChat},
		RunConfig{BillingAppType: runtimemodel.MessageBillingReasonSourceAIChat},
		conversationID,
		messageID,
		onEvent,
	)
}

// RunConfiguredInterruptedTurnResume resumes a turn whose process stopped in
// the middle of it (ADR 0035). The lease sweeper has marked such a message
// error/runtime_lease_expired; its transcript holds every model response and
// tool result up to the stop. The turn is rebuilt from them and its loop runs
// on, once. A tool call that started without a recorded result is repeated
// only if it is read-only; otherwise the turn is not resumed.
func (s *service) RunConfiguredInterruptedTurnResume(
	ctx context.Context,
	scope Scope,
	caller Caller,
	config RunConfig,
	conversationID uuid.UUID,
	messageID uuid.UUID,
	onEvent func(StreamEvent) error,
) (*ChatResult, error) {
	if onEvent == nil {
		return nil, fmt.Errorf("%w: event callback is required", ErrInvalidInput)
	}
	message, conversation, err := s.GetMessageByCaller(ctx, scope, caller, messageID)
	if err != nil {
		return nil, err
	}
	if message == nil || conversation == nil || message.ConversationID != conversationID || conversation.ID != conversationID {
		return nil, fmt.Errorf("%w: message is not in the conversation", ErrInvalidInput)
	}
	if !interruptedTurnIsLatest(conversation, message) {
		return nil, fmt.Errorf("%w: the conversation has moved on from this turn", ErrInterruptedTurnNotResumable)
	}
	if !interruptedTurnAwaitingResume(message) {
		return nil, fmt.Errorf("%w: the turn did not stop with its process, or was resumed once already", ErrInterruptedTurnNotResumable)
	}
	// Preparation is read-only; doing it before the claim keeps a
	// configuration error from leaving the message in `streaming`.
	prepared, err := s.prepareSameTurnContinuationChat(ctx, scope, caller, config, conversation, message, runtimedto.RegenerateMessageRequest{}, interruptedTurnContinuationType, false)
	if err == nil && !prepared.toolLoopEnabled() {
		err = fmt.Errorf("%w: the turn has no tool loop to resume", ErrInterruptedTurnNotResumable)
	}
	if err == nil {
		err = interruptedTurnResumable(message)
	}
	if err != nil {
		// Record the outcome so recovery does not try this turn again.
		s.recordInterruptedTurnNotResumed(ctx, message, err)
		return nil, err
	}
	prepared.resumeWith(interruptedTurnResumeMessage(message))
	prepared.taskTokenBudget = taskTokenBudgetFromMetadata(message.Metadata)

	metadata := copyStringAnyMap(message.Metadata)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	// A call that never finished is not running any more.
	for _, invocation := range skillInvocationMaps(metadata) {
		if strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["status"])), "running") {
			invocation["status"] = "interrupted"
		}
	}
	metadata[interruptedTurnResumeKey] = map[string]interface{}{
		"status":     runtimemodel.MessageStatusStreaming,
		"resumed_at": time.Now().Unix(),
		"cause":      staleActiveMessageError,
	}
	if s.repos == nil || s.repos.DB == nil {
		return nil, fmt.Errorf("%w: chat runtime database is unavailable", ErrInvalidInput)
	}
	err = s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&runtimemodel.Message{}).
			Where("id = ? AND deleted_at IS NULL AND status = ? AND error = ?", message.ID, runtimemodel.MessageStatusError, staleActiveMessageError).
			Updates(map[string]interface{}{"status": runtimemodel.MessageStatusStreaming, "error": nil, "answer": "", "metadata": metadata})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("%w: the turn was resumed or changed by another process", ErrInterruptedTurnNotResumable)
		}
		return repository.NewRepositories(tx).Conversation.StartStreaming(ctx, conversation.ID, scope.OrganizationID, scope.AccountID, message.ID)
	})
	if err != nil {
		if errors.Is(err, ErrInterruptedTurnNotResumable) {
			return nil, err
		}
		return nil, mapRepoError(err)
	}
	message.Status = runtimemodel.MessageStatusStreaming
	message.Error = nil
	message.Answer = ""
	message.Metadata = metadata
	conversation.RuntimeStatus = runtimemodel.ConversationRuntimeStatusStreaming
	conversation.ActiveMessageID = &message.ID
	prepared.Message = message
	prepared.Conversation = conversation
	return s.runPreparedInterruptedTurnResume(ctx, prepared, onEvent)
}

// interruptedTurnIsLatest reports that nothing happened in the conversation
// after the stopped turn: its leaf is the turn itself or the message the turn
// answers (a turn becomes the leaf only when it completes), and no other turn
// is active.
func interruptedTurnIsLatest(conversation *runtimemodel.Conversation, message *runtimemodel.Message) bool {
	if conversation.ActiveMessageID != nil && *conversation.ActiveMessageID != message.ID {
		return false
	}
	leaf := conversation.CurrentLeafMessageID
	switch {
	case leaf == nil, *leaf == message.ID:
		return true
	default:
		return message.ParentID != nil && *leaf == *message.ParentID
	}
}

// interruptedTurnAwaitingResume reports a turn its lease sweeper stopped that
// no resume has tried yet.
func interruptedTurnAwaitingResume(message *runtimemodel.Message) bool {
	if message.Status != runtimemodel.MessageStatusError || message.Error == nil || strings.TrimSpace(*message.Error) != staleActiveMessageError {
		return false
	}
	_, tried := message.Metadata[interruptedTurnResumeKey]
	return !tried
}

func (s *service) recordInterruptedTurnNotResumed(ctx context.Context, message *runtimemodel.Message, cause error) {
	if s.repos == nil || s.repos.Message == nil {
		return
	}
	metadata := copyStringAnyMap(message.Metadata)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadata[interruptedTurnResumeKey] = map[string]interface{}{
		"status": "not_resumed",
		"reason": cause.Error(),
		"cause":  staleActiveMessageError,
	}
	_ = s.repos.Message.UpdateMetadataAnyStatus(context.WithoutCancel(ctx), message.ID, metadata)
}

// interruptedTurnResumable checks that a message is a turn its lease sweeper
// stopped, that it was not resumed before, and that every tool call it
// started and did not finish is read-only, so repeating it changes nothing.
// A call is started once its start is recorded; the skill loop runs no call
// before that. A runtime tool's call records whether the tool is read-only.
func interruptedTurnResumable(message *runtimemodel.Message) error {
	if !interruptedTurnAwaitingResume(message) {
		return fmt.Errorf("%w: the turn did not stop with its process, or was resumed once already", ErrInterruptedTurnNotResumable)
	}
	for _, invocation := range skillInvocationMaps(message.Metadata) {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["status"])), "running") {
			continue
		}
		if readOnly, _ := invocation[skillCallReadOnlyKey].(bool); readOnly {
			continue
		}
		skillID := strings.TrimSpace(stringFromAny(invocation["skill_id"]))
		toolName := strings.TrimSpace(stringFromAny(invocation["tool_name"]))
		if !skillLoopToolLooksReadOnly(skillID, toolName) {
			return fmt.Errorf("%w: %s/%s started without a recorded result and may have changed something", ErrInterruptedTurnNotResumable, skillID, toolName)
		}
	}
	return nil
}

func interruptedTurnResumeMessage(message *runtimemodel.Message) adapter.Message {
	original := ""
	if message != nil {
		original = strings.TrimSpace(message.Query)
	}
	return adapter.Message{Role: "user", Content: "The process running this assistant task stopped in the middle of it, and the same task is resuming. " +
		"Continue from the tool results and the authoritative same-turn state above; do not restart completed work. " +
		"A tool call shown without a recorded result either never started or was read-only: call it again if you still need its result. " +
		"Original request: " + original}
}

func (s *service) runPreparedInterruptedTurnResume(ctx context.Context, prepared *PreparedChat, onEvent func(StreamEvent) error) (*ChatResult, error) {
	execution, err := s.beginRuntimeExecution(ctx, prepared.Message.ID)
	if err != nil {
		s.finalizePreparedError(context.WithoutCancel(ctx), prepared, err, onEvent)
		return nil, newFinalizedStreamError(err)
	}
	defer execution.Finish()
	s.emitPreparedEvent(execution.PersistContext, prepared, streamEventMessageStart, messageStartPayload(prepared.Conversation, prepared.Message, false), onEvent)
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
	if record, ok := metadata[interruptedTurnResumeKey].(map[string]interface{}); ok {
		record["status"] = runtimemodel.MessageStatusCompleted
	}
	if err := s.completePreparedChat(execution.PersistContext, prepared, answer, metadata); err != nil {
		return nil, finalizedRuntimePersistenceError(err)
	}
	s.emitPreparedEvent(execution.PersistContext, prepared, streamEventMessageEnd, messageEndPayload(prepared, metadata), onEvent)
	return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusCompleted}, nil
}
