package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/gorm"
)

const (
	workflowContinuationStatusWaitingApproval  = "waiting_approval"
	workflowContinuationStatusWaitingQuestion  = "waiting_question"
	workflowContinuationStatusContinuing       = "continuing"
	workflowContinuationStatusSummarizing      = "summarizing"
	workflowContinuationStatusDirectOutput     = "direct_output"
	workflowContinuationStatusCompleted        = "completed"
	workflowContinuationStatusFailed           = "failed"
	workflowContinuationStatusStopped          = "stopped"
	workflowContinuationCheckpointInterval     = 750 * time.Millisecond
	workflowContinuationCheckpointBytes        = 4 * 1024
	workflowContinuationTerminalPersistTimeout = 10 * time.Second
)

type WorkflowApprovalContinuation struct {
	ConversationID            uuid.UUID
	MessageID                 uuid.UUID
	WorkflowRunID             string
	InvocationID              string
	InvocationMode            string
	InvocationProtocolVersion int
	AgentID                   string
	AgentType                 string
	BindingID                 string
	OriginalQuery             string
	UIApprovalAllowed         bool
	Completed                 bool
	Metadata                  map[string]interface{}
	Caller                    Caller
	RunConfig                 RunConfig

	answerMu             sync.Mutex
	answer               string
	answerObserved       bool
	lastCheckpointAnswer string
	lastCheckpointAt     time.Time
}

type WorkflowContinuationSummaryRequest struct {
	WorkflowRunID string
	Status        string
	Outputs       map[string]interface{}
	Error         string
}

func (s *service) BeginWorkflowApprovalContinuation(ctx context.Context, scope Scope, caller Caller, config RunConfig, conversationID, messageID uuid.UUID) (*WorkflowApprovalContinuation, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	conversation, err := s.getConversationByCallerScoped(ctx, scope, caller, conversationID)
	if err != nil {
		return nil, err
	}
	message, err := s.repos.Message.GetScoped(ctx, messageID, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	if message.ConversationID != conversation.ID {
		return nil, fmt.Errorf("%w: message belongs to another conversation", ErrInvalidInput)
	}
	state := workflowApprovalContinuationFromMetadata(message.Metadata)
	if state.WorkflowRunID == "" {
		return nil, fmt.Errorf("%w: message has no pending workflow continuation", ErrInvalidInput)
	}
	state.ConversationID = conversation.ID
	state.MessageID = message.ID
	state.OriginalQuery = message.Query
	state.Metadata = copyStringAnyMap(message.Metadata)
	state.answer = message.Answer
	state.lastCheckpointAnswer = message.Answer
	state.Caller = caller
	state.RunConfig = config
	if message.Status == runtimemodel.MessageStatusCompleted {
		state.Completed = true
		return state, nil
	}
	if err := validateWorkflowContinuationBinding(state, config.WorkflowBindings); err != nil {
		return nil, err
	}
	if message.Status == runtimemodel.MessageStatusWaitingApproval &&
		!workflowContinuationAllowsInlineApproval(caller, state) {
		return nil, fmt.Errorf("%w: this workflow approval must be completed through its configured approval channel", ErrInvalidInput)
	}
	if message.Status != runtimemodel.MessageStatusWaitingApproval && message.Status != runtimemodel.MessageStatusWaitingQuestion && message.Status != runtimemodel.MessageStatusStreaming {
		return nil, fmt.Errorf("%w: message is not waiting for workflow continuation", ErrInvalidInput)
	}
	if message.Status == runtimemodel.MessageStatusWaitingApproval || message.Status == runtimemodel.MessageStatusWaitingQuestion {
		if message.Status == runtimemodel.MessageStatusWaitingQuestion {
			state.Metadata = workflowContinuationMetadataWithoutUserInputRequest(state.Metadata)
		}
		err = s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			txRepos := repository.NewRepositories(tx)
			if err := txRepos.Conversation.StartStreaming(ctx, conversation.ID, scope.OrganizationID, scope.AccountID, message.ID); err != nil {
				return err
			}
			if message.Status == runtimemodel.MessageStatusWaitingQuestion {
				if err := txRepos.Message.UpdateMetadata(ctx, message.ID, state.Metadata); err != nil {
					return err
				}
			}
			return tx.Model(&runtimemodel.Message{}).
				Where("id = ? AND deleted_at IS NULL AND status IN ?", message.ID, []string{runtimemodel.MessageStatusWaitingApproval, runtimemodel.MessageStatusWaitingQuestion}).
				Updates(map[string]interface{}{"status": runtimemodel.MessageStatusStreaming}).Error
		})
		if err != nil {
			return nil, err
		}
	}
	return state, nil
}

func validateWorkflowContinuationBinding(continuation *WorkflowApprovalContinuation, bindings []AgentWorkflowBinding) error {
	if continuation == nil {
		return fmt.Errorf("%w: continuation is missing", ErrWorkflowBindingUnavailable)
	}
	bindingID := strings.TrimSpace(continuation.BindingID)
	if bindingID == "" {
		return fmt.Errorf("%w: continuation binding id is missing", ErrWorkflowBindingUnavailable)
	}
	agentID := strings.TrimSpace(continuation.AgentID)
	for _, binding := range bindings {
		if !strings.EqualFold(strings.TrimSpace(binding.BindingID), bindingID) {
			continue
		}
		if agentID != "" && !strings.EqualFold(strings.TrimSpace(binding.AgentID), agentID) {
			continue
		}
		return nil
	}
	return fmt.Errorf("%w: workflow binding %q is not active", ErrWorkflowBindingUnavailable, bindingID)
}

func (s *service) RecordWorkflowApprovalContinuationEvent(ctx context.Context, continuation *WorkflowApprovalContinuation, eventType string, payload map[string]interface{}) (*StreamEvent, error) {
	if continuation == nil || continuation.MessageID == uuid.Nil {
		return nil, fmt.Errorf("%w: workflow continuation is required", ErrInvalidInput)
	}
	eventPayload := copyStringAnyMap(payload)
	if eventPayload == nil {
		eventPayload = map[string]interface{}{}
	}
	eventPayload["conversation_id"] = continuation.ConversationID.String()
	eventPayload["message_id"] = continuation.MessageID.String()
	if _, ok := eventPayload["workflow_run_id"]; !ok && continuation.WorkflowRunID != "" {
		eventPayload["workflow_run_id"] = continuation.WorkflowRunID
	}
	if err := s.checkpointWorkflowContinuationAnswer(ctx, continuation, eventType, eventPayload, workflowContinuationEventForcesAnswerCheckpoint(eventType)); err != nil {
		return nil, err
	}
	if !shouldPersistWorkflowRunMetadataEvent(eventType) {
		return s.AppendWorkflowApprovalContinuationStreamEvent(ctx, continuation, eventType, eventPayload)
	}
	metadata := mergeWorkflowRunMetadata(continuation.Metadata, eventType, eventPayload)
	metadata["agent_workflow_continuation"] = mergeWorkflowMap(
		workflowRecordFromAny(metadata["agent_workflow_continuation"]),
		map[string]interface{}{"status": workflowContinuationStatusFromEvent(eventType)},
	)
	continuation.Metadata = metadata
	if err := s.repos.Message.UpdateMetadata(ctx, continuation.MessageID, metadata); err != nil {
		return nil, err
	}
	return s.AppendWorkflowApprovalContinuationStreamEvent(ctx, continuation, eventType, eventPayload)
}

func shouldPersistWorkflowRunMetadataEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case streamEventMessage, "text_chunk", streamEventMessageEnd:
		return false
	default:
		return true
	}
}

func (s *service) AppendWorkflowApprovalContinuationStreamEvent(ctx context.Context, continuation *WorkflowApprovalContinuation, eventType string, payload map[string]interface{}) (*StreamEvent, error) {
	if continuation == nil || continuation.MessageID == uuid.Nil || continuation.ConversationID == uuid.Nil {
		return nil, fmt.Errorf("%w: workflow continuation is required", ErrInvalidInput)
	}
	eventPayload := copyStringAnyMap(payload)
	if eventPayload == nil {
		eventPayload = map[string]interface{}{}
	}
	eventPayload["conversation_id"] = continuation.ConversationID.String()
	eventPayload["message_id"] = continuation.MessageID.String()
	if _, ok := eventPayload["workflow_run_id"]; !ok && continuation.WorkflowRunID != "" {
		eventPayload["workflow_run_id"] = continuation.WorkflowRunID
	}
	event := s.appendStreamEventBestEffort(ctx, continuation.MessageID, continuation.ConversationID, eventType, eventPayload)
	if event != nil {
		return event, nil
	}
	return &StreamEvent{
		EventType: eventType,
		Payload:   eventPayload,
		CreatedAt: time.Now().Unix(),
	}, nil
}

func (s *service) UpdateWorkflowApprovalContinuationStatus(ctx context.Context, continuation *WorkflowApprovalContinuation, status string) (map[string]interface{}, error) {
	if continuation == nil || continuation.MessageID == uuid.Nil {
		return nil, fmt.Errorf("%w: workflow continuation is required", ErrInvalidInput)
	}
	status = strings.TrimSpace(status)
	if status == "" {
		return copyStringAnyMap(continuation.Metadata), nil
	}
	metadata := workflowContinuationMetadataWithStatus(continuation.Metadata, status)
	continuation.Metadata = metadata
	if err := s.repos.Message.UpdateMetadata(ctx, continuation.MessageID, metadata); err != nil {
		return nil, err
	}
	return metadata, nil
}

func (s *service) PauseWorkflowApprovalContinuation(ctx context.Context, continuation *WorkflowApprovalContinuation, status string) (map[string]interface{}, error) {
	if continuation == nil || continuation.MessageID == uuid.Nil || continuation.ConversationID == uuid.Nil {
		return nil, fmt.Errorf("%w: workflow continuation is required", ErrInvalidInput)
	}
	if err := s.checkpointWorkflowContinuationAnswer(ctx, continuation, "workflow_paused", nil, true); err != nil {
		return nil, err
	}
	status = strings.TrimSpace(status)
	if status == "" {
		status = workflowContinuationStatusWaitingApproval
	}
	metadata := workflowContinuationMetadataWithStatus(continuation.Metadata, status)
	continuation.Metadata = metadata
	switch status {
	case workflowContinuationStatusWaitingQuestion:
		if err := s.repos.Message.UpdateWaitingQuestion(ctx, continuation.MessageID, metadata); err != nil {
			return nil, err
		}
	default:
		if err := s.repos.Message.UpdateWaitingApproval(ctx, continuation.MessageID, metadata); err != nil {
			return nil, err
		}
	}
	if err := s.repos.Conversation.FinishContinuationMessage(ctx, continuation.ConversationID, continuation.MessageID); err != nil {
		return nil, err
	}
	return metadata, nil
}

func (s *service) SummarizeWorkflowApprovalContinuation(ctx context.Context, scope Scope, continuation *WorkflowApprovalContinuation, req WorkflowContinuationSummaryRequest, onEvent func(StreamEvent) error) (*ChatResult, error) {
	if continuation == nil || continuation.MessageID == uuid.Nil || continuation.ConversationID == uuid.Nil {
		return nil, fmt.Errorf("%w: workflow continuation is required", ErrInvalidInput)
	}
	if workflowContinuationInvocationMode(continuation) == "agent_task_tool" {
		return s.continueWorkflowTaskInvocation(ctx, scope, continuation, req, onEvent)
	}
	if len(req.Outputs) == 0 {
		answer := workflowNoDisplayableOutputAnswer(req.WorkflowRunID)
		metadata, err := s.CompleteWorkflowApprovalContinuation(ctx, continuation, answer, workflowContinuationStatusCompleted)
		if err != nil {
			return nil, err
		}
		if onEvent != nil && answer != "" {
			event, eventErr := s.AppendWorkflowApprovalContinuationStreamEvent(ctx, continuation, streamEventMessage, map[string]interface{}{
				"answer": answer,
			})
			if eventErr != nil {
				return nil, eventErr
			}
			if err := onEvent(*event); err != nil {
				return nil, err
			}
		}
		return &ChatResult{Answer: answer, Metadata: metadata}, nil
	}
	conversation, err := s.repos.Conversation.GetScoped(ctx, continuation.ConversationID, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	message, err := s.repos.Message.GetScoped(ctx, continuation.MessageID, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	if message.ConversationID != conversation.ID {
		return nil, fmt.Errorf("%w: message belongs to another conversation", ErrInvalidInput)
	}
	metadata, err := s.UpdateWorkflowApprovalContinuationStatus(ctx, continuation, workflowContinuationStatusSummarizing)
	if err != nil {
		return nil, err
	}
	message.Metadata = metadata
	prepared := &PreparedChat{
		Conversation: conversation,
		Message:      message,
		Scope:        scope,
		LLMRequest:   workflowSummaryLLMRequest(message, continuation, req),

		usageContinuation: true,
	}
	execution, err := s.beginRuntimeExecution(ctx, message.ID)
	if err != nil {
		return nil, err
	}
	defer execution.Finish()
	runCtx := execution.Context
	persistCtx := execution.PersistContext
	if s.streams.IsStopped(message.ID, execution.runID) {
		_ = s.persistStoppedAnswer(persistCtx, prepared, "", nil)
		return nil, ErrMessageStopped
	}
	stream, err := s.openChatStream(runCtx, prepared)
	if err != nil {
		if finalizeErr := s.finalizePreparedError(persistCtx, prepared, err, onEvent); finalizeErr != nil {
			return nil, finalizedRuntimePersistenceError(finalizeErr)
		}
		return nil, newFinalizedStreamError(err)
	}
	answer, usage, err := s.collectStreamAnswerWithEvents(runCtx, prepared, stream, onEvent, nil)
	if err != nil {
		if errors.Is(err, ErrMessageStopped) {
			_ = s.persistStoppedAnswer(persistCtx, prepared, answer, usage)
			return nil, err
		}
		if finalizeErr := s.finalizePreparedError(persistCtx, prepared, err, onEvent); finalizeErr != nil {
			return nil, finalizedRuntimePersistenceError(finalizeErr)
		}
		return nil, newFinalizedStreamError(err)
	}
	if s.streams.IsStopped(message.ID, execution.runID) {
		_ = s.persistStoppedAnswer(persistCtx, prepared, answer, usage)
		return nil, ErrMessageStopped
	}
	metadata = preparedResultMetadataForPrepared(prepared, continuation.Metadata, usage)
	continuation.Metadata = metadata
	metadata, err = s.CompleteWorkflowApprovalContinuation(persistCtx, continuation, answer, workflowContinuationStatusCompleted)
	if err != nil {
		return nil, finalizedRuntimePersistenceError(err)
	}
	return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage}, nil
}

func (s *service) CompleteWorkflowApprovalContinuation(ctx context.Context, continuation *WorkflowApprovalContinuation, answer string, status string) (map[string]interface{}, error) {
	if continuation == nil || continuation.MessageID == uuid.Nil || continuation.ConversationID == uuid.Nil {
		return nil, fmt.Errorf("%w: workflow continuation is required", ErrInvalidInput)
	}
	metadata := copyStringAnyMap(continuation.Metadata)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if strings.TrimSpace(status) == "" {
		status = runtimemodel.MessageStatusCompleted
	}
	metadata = workflowContinuationMetadataWithoutUserInputRequest(metadata)
	metadata = workflowContinuationMetadataWithStatus(metadata, status)
	continuation.Metadata = metadata
	if status == runtimemodel.MessageStatusStopped || status == workflowContinuationStatusStopped {
		continuation.answerMu.Lock()
		answerObserved := continuation.answerObserved
		if answerObserved {
			answer = continuation.answer
		}
		continuation.answerMu.Unlock()
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), workflowContinuationTerminalPersistTimeout)
		defer cancel()
		var err error
		if answerObserved || answer != "" {
			err = s.repos.Message.UpdateStoppedAnswer(persistCtx, continuation.MessageID, answer, metadata)
		} else {
			err = updateStoppedContinuationPreservingAnswer(persistCtx, s.repos, continuation.MessageID, metadata)
		}
		if err != nil {
			return nil, err
		}
		if err := s.repos.Conversation.FinishContinuationMessage(persistCtx, continuation.ConversationID, continuation.MessageID); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return metadata, nil
	}
	if err := s.repos.Message.UpdateCompleted(ctx, continuation.MessageID, answer, metadata); err != nil {
		return nil, err
	}
	if err := s.repos.Conversation.FinishContinuationMessage(ctx, continuation.ConversationID, continuation.MessageID); err != nil {
		return nil, err
	}
	return metadata, nil
}

type stoppedAnswerPreservingMessageRepository interface {
	UpdateStoppedPreservingAnswer(context.Context, uuid.UUID, map[string]interface{}) error
}

func updateStoppedContinuationPreservingAnswer(ctx context.Context, repos *repository.Repositories, messageID uuid.UUID, metadata map[string]interface{}) error {
	if repos == nil {
		return fmt.Errorf("workflow continuation repositories are not configured")
	}
	if messageRepo, ok := repos.Message.(stoppedAnswerPreservingMessageRepository); ok {
		return messageRepo.UpdateStoppedPreservingAnswer(ctx, messageID, metadata)
	}
	if repos.DB == nil {
		return fmt.Errorf("workflow continuation stopped answer persistence is not configured")
	}
	return repository.UpdateMessageStoppedPreservingAnswer(ctx, repos.DB, messageID, metadata)
}

func (s *service) checkpointWorkflowContinuationAnswer(ctx context.Context, continuation *WorkflowApprovalContinuation, eventType string, payload map[string]interface{}, force bool) error {
	if continuation == nil || continuation.MessageID == uuid.Nil {
		return nil
	}
	chunk, observed := workflowContinuationAnswerChunk(eventType, payload)
	continuation.answerMu.Lock()
	defer continuation.answerMu.Unlock()
	if observed {
		continuation.answerObserved = true
		if strings.TrimSpace(eventType) == "text_replace" {
			continuation.answer = chunk
		} else {
			continuation.answer += chunk
		}
	}
	if continuation.answer == continuation.lastCheckpointAnswer {
		return nil
	}
	now := time.Now()
	answerGrowth := len(continuation.answer) - len(continuation.lastCheckpointAnswer)
	if answerGrowth < 0 {
		answerGrowth = -answerGrowth
	}
	if !force && !continuation.lastCheckpointAt.IsZero() && now.Sub(continuation.lastCheckpointAt) < workflowContinuationCheckpointInterval && answerGrowth < workflowContinuationCheckpointBytes {
		return nil
	}
	metadata := copyStringAnyMap(continuation.Metadata)
	if err := s.repos.Message.UpdatePartialAnswer(ctx, continuation.MessageID, continuation.answer, metadata); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("checkpoint workflow continuation answer: %w", err)
	}
	continuation.lastCheckpointAnswer = continuation.answer
	continuation.lastCheckpointAt = now
	return nil
}

func workflowContinuationAnswerChunk(eventType string, payload map[string]interface{}) (string, bool) {
	switch strings.TrimSpace(eventType) {
	case streamEventMessage, "text_chunk", "text_replace":
	default:
		return "", false
	}
	if payload == nil {
		return "", false
	}
	for _, key := range []string{"answer", "text", "answer_delta"} {
		if value, ok := payload[key].(string); ok {
			return value, true
		}
	}
	if data, ok := payload["data"].(map[string]interface{}); ok {
		return workflowContinuationAnswerChunk(eventType, data)
	}
	return "", false
}

func workflowContinuationEventForcesAnswerCheckpoint(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "text_replace", "workflow_finished", "workflow_failed", "workflow_stopped", "workflow_paused", "approval_requested", "question_answer_requested", streamEventMessageEnd:
		return true
	default:
		return false
	}
}

func (s *service) FailWorkflowApprovalContinuation(ctx context.Context, continuation *WorkflowApprovalContinuation, message string) (map[string]interface{}, error) {
	if continuation == nil || continuation.MessageID == uuid.Nil || continuation.ConversationID == uuid.Nil {
		return nil, fmt.Errorf("%w: workflow continuation is required", ErrInvalidInput)
	}
	if err := s.checkpointWorkflowContinuationAnswer(ctx, continuation, "workflow_failed", nil, true); err != nil {
		return nil, err
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = "workflow approval continuation failed"
	}
	metadata := workflowContinuationMetadataWithoutUserInputRequest(continuation.Metadata)
	metadata = workflowContinuationMetadataWithStatus(metadata, workflowContinuationStatusFailed)
	continuation.Metadata = metadata
	if err := s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepos := repository.NewRepositories(tx)
		if err := txRepos.Message.UpdateMetadata(ctx, continuation.MessageID, metadata); err != nil {
			return err
		}
		if err := txRepos.Message.UpdateError(ctx, continuation.MessageID, message); err != nil {
			return err
		}
		return txRepos.Conversation.FinishContinuationMessage(ctx, continuation.ConversationID, continuation.MessageID)
	}); err != nil {
		return nil, err
	}
	return metadata, nil
}

func workflowApprovalContinuationFromMetadata(metadata map[string]interface{}) *WorkflowApprovalContinuation {
	state := workflowRecordFromAny(metadata["agent_workflow_continuation"])
	return &WorkflowApprovalContinuation{
		WorkflowRunID:             firstNonEmptyString(state["workflow_run_id"]),
		InvocationID:              firstNonEmptyString(state["invocation_id"]),
		InvocationMode:            firstNonEmptyString(state["invocation_mode"]),
		InvocationProtocolVersion: intFromWorkflowContinuation(state["invocation_protocol_version"]),
		AgentID:                   firstNonEmptyString(state["agent_id"]),
		AgentType:                 firstNonEmptyString(state["agent_type"]),
		BindingID:                 firstNonEmptyString(state["binding_id"]),
		OriginalQuery:             firstNonEmptyString(state["original_query"]),
		UIApprovalAllowed:         boolFromWorkflowContinuation(state["ui_approval_allowed"]),
		Metadata:                  copyStringAnyMap(metadata),
	}
}

func boolFromWorkflowContinuation(value interface{}) bool {
	allowed, _ := value.(bool)
	return allowed
}

func workflowContinuationAllowsInlineApproval(caller Caller, continuation *WorkflowApprovalContinuation) bool {
	switch strings.ToLower(strings.TrimSpace(caller.Source)) {
	case runtimemodel.ConversationSourceConsole:
		return true
	case runtimemodel.ConversationSourceWebApp, runtimemodel.ConversationSourceExternalAPI:
		return continuation != nil && continuation.UIApprovalAllowed
	default:
		return false
	}
}

func workflowContinuationInvocationMode(continuation *WorkflowApprovalContinuation) string {
	if continuation == nil {
		return ""
	}
	if mode := strings.ToLower(strings.TrimSpace(continuation.InvocationMode)); mode != "" {
		return mode
	}
	if strings.EqualFold(strings.TrimSpace(continuation.AgentType), "CONVERSATIONAL_WORKFLOW") {
		return "agent_conversation_delegate"
	}
	return "agent_task_tool"
}

func intFromWorkflowContinuation(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func (s *service) continueWorkflowTaskInvocation(ctx context.Context, scope Scope, continuation *WorkflowApprovalContinuation, req WorkflowContinuationSummaryRequest, onEvent func(StreamEvent) error) (*ChatResult, error) {
	conversation, err := s.repos.Conversation.GetScoped(ctx, continuation.ConversationID, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	message, err := s.repos.Message.GetScoped(ctx, continuation.MessageID, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	if message.ConversationID != conversation.ID {
		return nil, fmt.Errorf("%w: message belongs to another conversation", ErrInvalidInput)
	}

	prepared, err := s.prepareWorkflowTaskContinuationChat(ctx, scope, continuation, conversation, message, req)
	if err != nil {
		return nil, err
	}
	execution, err := s.beginRuntimeExecution(ctx, message.ID)
	if err != nil {
		return nil, err
	}
	defer execution.Finish()
	runCtx := execution.Context
	persistCtx := execution.PersistContext
	if s.streams.IsStopped(message.ID, execution.runID) {
		_ = s.persistStoppedAnswer(persistCtx, prepared, "", nil)
		return nil, ErrMessageStopped
	}

	answer, usage, err := s.runPreparedToolLoop(runCtx, persistCtx, prepared, nil, onEvent)
	if err != nil {
		return s.finishUserInputContinuationPendingOrError(persistCtx, prepared, answer, usage, err, onEvent)
	}
	if s.streams.IsStopped(message.ID, execution.runID) {
		_ = s.persistStoppedAnswer(persistCtx, prepared, answer, usage)
		return nil, ErrMessageStopped
	}
	metadata := preparedResultMetadataForPrepared(prepared, prepared.Message.Metadata, usage)
	continuation.Metadata = metadata
	metadata, err = s.CompleteWorkflowApprovalContinuation(persistCtx, continuation, answer, workflowContinuationStatusCompleted)
	if err != nil {
		return nil, finalizedRuntimePersistenceError(err)
	}
	return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusCompleted}, nil
}

func (s *service) prepareWorkflowTaskContinuationChat(ctx context.Context, scope Scope, continuation *WorkflowApprovalContinuation, conversation *runtimemodel.Conversation, message *runtimemodel.Message, req WorkflowContinuationSummaryRequest) (*PreparedChat, error) {
	regenerateReq := applyRunConfigToRegenerateRequest(continuation.RunConfig, runtimedto.RegenerateMessageRequest{})
	parts, err := normalizeRegenerateRequest(regenerateReq, message)
	if err != nil {
		return nil, err
	}
	applyRunConfigToParts(continuation.RunConfig, parts)
	applyCallerRuntimeSurfacePolicy(continuation.Caller, parts)
	applyPersistedConversationSurface(conversation, parts)
	restoreExecutionModeFromMetadata(parts, message.Metadata)
	restoreConsoleFilesContextFromMetadata(parts, message.Metadata, nil)
	restoreConsoleAgentsContextFromMetadata(parts, message.Metadata, nil)
	restoreTurnInitialContextFromMetadata(parts, message.Metadata)
	restoreCurrentPageContextFromMetadata(parts, message.Metadata)
	parts.Attachments = attachmentBundleFromMessageMetadata(message.Metadata)
	if err := s.applyModelCapabilities(ctx, scope, continuation.Caller, parts); err != nil {
		return nil, err
	}
	applyProtocolToolsPolicy(continuation.Caller, parts)
	applyManagedUserMemoryPolicy(continuation.Caller, parts)
	if err := s.applySkillConfig(ctx, scope, continuation.Caller, &continuation.RunConfig, parts); err != nil {
		return nil, err
	}
	applyAgentMemoryToolsPolicy(parts)
	contextResult, err := s.buildUpstreamMessages(ctx, scope, message.ParentID, parts)
	if err != nil {
		return nil, err
	}
	parts.ContextControl = contextResult.Metadata
	llmRequest := newLLMChatRequest(parts, contextResult.Messages)
	if stateMessage := currentTurnAuthoritativeStateMessage(message); stateMessage != nil {
		llmRequest.Messages = append(llmRequest.Messages, *stateMessage)
	}
	llmRequest.Messages = append(llmRequest.Messages, continuationMessageForExecutionMode(workflowTaskContinuationMessage(continuation, req), parts.ExecutionMode))
	return &PreparedChat{
		Conversation:     conversation,
		Message:          message,
		LLMRequest:       llmRequest,
		Scope:            scope,
		Caller:           continuation.Caller,
		RunConfig:        continuation.RunConfig,
		ParentID:         message.ParentID,
		Continuation:     true,
		ContinuationType: "agent_workflow_task",
		parts:            parts,
	}, nil
}

func workflowTaskContinuationMessage(continuation *WorkflowApprovalContinuation, req WorkflowContinuationSummaryRequest) adapter.Message {
	payload := map[string]interface{}{
		"invocation_id":   continuation.InvocationID,
		"workflow_run_id": firstNonEmptyString(req.WorkflowRunID, continuation.WorkflowRunID),
		"status":          strings.TrimSpace(req.Status),
		"outputs":         req.Outputs,
		"error":           strings.TrimSpace(req.Error),
	}
	return adapter.Message{
		Role: "user",
		Content: strings.Join([]string{
			"The task workflow tool invocation for this same Agent turn has reached a terminal state.",
			"Treat the JSON below as authoritative tool evidence. Continue the original Agent plan from this result; do not rerun the same workflow invocation.",
			"You may answer the user if the task is complete, or use other already-authorized tools when the original request still requires work.",
			"Workflow task result JSON:\n" + compactJSONForPrompt(payload, 24000),
		}, "\n"),
	}
}

func workflowContinuationStatusFromEvent(eventType string) string {
	switch strings.TrimSpace(eventType) {
	case "workflow_finished":
		return "finishing"
	case "workflow_failed", "error":
		return workflowContinuationStatusFailed
	case "workflow_stopped":
		return workflowContinuationStatusFailed
	case "approval_requested":
		return workflowContinuationStatusWaitingApproval
	case "question_answer_requested":
		return workflowContinuationStatusWaitingQuestion
	default:
		return workflowContinuationStatusContinuing
	}
}

func workflowContinuationMetadataWithStatus(metadata map[string]interface{}, status string) map[string]interface{} {
	next := copyStringAnyMap(metadata)
	if next == nil {
		next = map[string]interface{}{}
	}
	next["agent_workflow_continuation"] = mergeWorkflowMap(
		workflowRecordFromAny(next["agent_workflow_continuation"]),
		map[string]interface{}{"status": strings.TrimSpace(status)},
	)
	return next
}

func workflowContinuationMetadataWithoutUserInputRequest(metadata map[string]interface{}) map[string]interface{} {
	next := copyStringAnyMap(metadata)
	if next == nil {
		return map[string]interface{}{}
	}
	delete(next, "user_input_request")
	return next
}

func workflowSummaryLLMRequest(message *runtimemodel.Message, continuation *WorkflowApprovalContinuation, req WorkflowContinuationSummaryRequest) *adapter.ChatRequest {
	provider := ""
	if message != nil && message.ModelProvider != nil {
		provider = strings.TrimSpace(*message.ModelProvider)
	}
	model := ""
	if message != nil {
		model = strings.TrimSpace(message.ModelName)
	}
	outputsJSON := workflowOutputsJSON(req.Outputs)
	errorText := strings.TrimSpace(req.Error)
	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = "succeeded"
	}
	workflowRunID := strings.TrimSpace(req.WorkflowRunID)
	if workflowRunID == "" && continuation != nil {
		workflowRunID = continuation.WorkflowRunID
	}
	userQuery := ""
	if continuation != nil {
		userQuery = strings.TrimSpace(continuation.OriginalQuery)
	}
	content := fmt.Sprintf("Original user request:\n%s\n\nWorkflow run id:\n%s\n\nWorkflow status:\n%s\n\nWorkflow error:\n%s\n\nWorkflow outputs JSON:\n%s", userQuery, workflowRunID, status, errorText, outputsJSON)
	chatReq := &adapter.ChatRequest{
		Provider: provider,
		Model:    model,
		Stream:   true,
		Messages: []adapter.Message{
			{
				Role:    "system",
				Content: "You are writing the final response for an Agent after a task workflow completed. Use only the workflow outputs, workflow status, workflow error, and workflow_run_id provided by the user message. Do not invent results, files, approvals, or data that are not present. Do not answer the original user request yourself; treat it only as context for explaining the workflow result. If the workflow outputs do not contain the requested business result, say what the workflow actually returned and include the workflow_run_id. If the workflow failed, explain the failure briefly and include the workflow_run_id. If the workflow outputs are enough, summarize them clearly for the user.",
			},
			{Role: "user", Content: content},
		},
	}
	if message != nil {
		applyModelParameters(chatReq, message.ModelParameters)
	}
	return chatReq
}

func workflowOutputsJSON(outputs map[string]interface{}) string {
	if len(outputs) == 0 {
		return "{}"
	}
	data, err := json.MarshalIndent(outputs, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}

func workflowNoDisplayableOutputAnswer(workflowRunID string) string {
	workflowRunID = strings.TrimSpace(workflowRunID)
	if workflowRunID == "" {
		return "工作流已运行，但未返回可展示输出。"
	}
	return fmt.Sprintf("工作流已运行，但未返回可展示输出。workflow_run_id: %s", workflowRunID)
}
