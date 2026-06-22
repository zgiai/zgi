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

const (
	streamEventClientActionResult = "client_action_result"

	clientActionStatusWaiting   = "waiting_client_action"
	clientActionStatusRunning   = "running"
	clientActionStatusSucceeded = "succeeded"
	clientActionStatusFailed    = "failed"
)

type ClientActionContinuation struct {
	Conversation *runtimemodel.Conversation
	Message      *runtimemodel.Message
	Event        map[string]interface{}
}

func (s *service) persistClientActionPending(ctx context.Context, prepared *PreparedChat, payload map[string]interface{}, usage *adapter.Usage) map[string]interface{} {
	if prepared == nil || prepared.Message == nil || prepared.Conversation == nil {
		return map[string]interface{}{}
	}
	pendingPayload := copyStringAnyMap(payload)
	if pendingPayload == nil {
		pendingPayload = map[string]interface{}{}
	}
	pendingPayload["conversation_id"] = prepared.Conversation.ID.String()
	pendingPayload["message_id"] = prepared.Message.ID.String()
	pendingPayload["status"] = clientActionStatusWaiting

	metadata := mergeClientActionMetadata(prepared.Message.Metadata, pendingPayload)
	metadata = preparedResultMetadata(metadata, usage)
	metadata["client_action_continuation"] = compactSkillInvocation(map[string]interface{}{
		"status":         clientActionStatusWaiting,
		"action_id":      clientActionID(pendingPayload),
		"action_type":    pendingPayload["action_type"],
		"skill_id":       pendingPayload["skill_id"],
		"tool_name":      pendingPayload["tool_name"],
		"href":           pendingPayload["href"],
		"label":          pendingPayload["label"],
		"original_query": prepared.Message.Query,
		"resume_policy":  "same_message",
	})
	prepared.Message.Metadata = metadata

	if s == nil || s.repos == nil || s.repos.Message == nil || s.repos.Conversation == nil {
		return metadata
	}
	if err := s.repos.Message.UpdateWaitingClientAction(ctx, prepared.Message.ID, metadata); err != nil {
		return metadata
	}
	_ = s.repos.Conversation.FinishContinuationMessage(ctx, prepared.Conversation.ID, prepared.Message.ID)
	return metadata
}

func (s *service) RunClientActionContinuationStream(
	ctx context.Context,
	scope Scope,
	conversationID uuid.UUID,
	messageID uuid.UUID,
	actionID string,
	req runtimedto.ClientActionResultRequest,
	onEvent func(StreamEvent) error,
) (*ChatResult, error) {
	if onEvent == nil {
		return nil, fmt.Errorf("%w: event callback is required", ErrInvalidInput)
	}
	status, err := normalizeClientActionResultStatus(req.Status)
	if err != nil {
		return nil, err
	}
	req.Status = status

	continuation, err := s.beginClientActionContinuation(ctx, scope, conversationID, messageID, actionID)
	if err != nil {
		return nil, err
	}
	conversation, message, err := s.reloadClientActionContinuationMessage(ctx, scope, conversationID, messageID)
	if err != nil {
		s.failClientActionContinuation(context.WithoutCancel(ctx), continuation, err, onEvent)
		return nil, newFinalizedStreamError(err)
	}
	continuation.Conversation = conversation
	continuation.Message = message

	s.resetStreamEventsBestEffort(ctx, message.ID)
	prepared, err := s.prepareClientActionContinuationChat(ctx, scope, continuation, req)
	if err != nil {
		s.failClientActionContinuation(context.WithoutCancel(ctx), continuation, err, onEvent)
		return nil, newFinalizedStreamError(err)
	}

	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	s.streams.Begin(message.ID, cancel)
	defer func() {
		cancel()
		s.streams.Finish(message.ID)
	}()
	if s.streams.IsStopped(message.ID) {
		_ = s.persistStoppedAnswer(context.WithoutCancel(ctx), prepared, "", nil)
		return nil, ErrMessageStopped
	}

	s.emitPreparedEvent(ctx, prepared, streamEventMessageStart, messageStartPayload(conversation, message, false), onEvent)
	s.emitPreparedEvent(ctx, prepared, streamEventClientActionResult, clientActionResultPayload(prepared, continuation.Event, req), onEvent)

	var answer string
	var usage *adapter.Usage
	if shouldFinalizeClientActionWithoutSkillLoop(continuation.Event, req) {
		answer, usage, err = s.runClientActionFinalAnswerStream(runCtx, prepared, onEvent)
		if err != nil {
			if errors.Is(err, ErrMessageStopped) {
				_ = s.clearPreparedRuntime(context.WithoutCancel(ctx), prepared)
				return nil, err
			}
			s.finalizePreparedError(context.WithoutCancel(ctx), prepared, err, onEvent)
			return nil, newFinalizedStreamError(err)
		}
	} else {
		answer, usage, err = s.runPreparedSkillStream(runCtx, context.WithoutCancel(ctx), prepared, nil, onEvent)
		if err != nil {
			var pendingGovernance *skillloop.ToolGovernancePendingError
			if errors.As(err, &pendingGovernance) {
				metadata := s.persistToolGovernanceApprovalPending(context.WithoutCancel(ctx), prepared, pendingGovernance.Payload, usage)
				s.emitPreparedEvent(context.WithoutCancel(ctx), prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingApproval), onEvent)
				return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusWaitingApproval}, nil
			}
			var pendingApproval *skillloop.WorkflowApprovalPendingError
			if errors.As(err, &pendingApproval) {
				metadata := s.persistWorkflowApprovalPending(context.WithoutCancel(ctx), prepared, pendingApproval.Payload, usage)
				s.emitPreparedEvent(context.WithoutCancel(ctx), prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingApproval), onEvent)
				return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusWaitingApproval}, nil
			}
			var pendingQuestion *skillloop.WorkflowQuestionPendingError
			if errors.As(err, &pendingQuestion) {
				metadata := s.persistWorkflowQuestionPending(context.WithoutCancel(ctx), prepared, pendingQuestion.Payload, usage)
				s.emitPreparedEvent(context.WithoutCancel(ctx), prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingQuestion), onEvent)
				return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusWaitingQuestion}, nil
			}
			var pendingClientAction *skillloop.ClientActionPendingError
			if errors.As(err, &pendingClientAction) {
				metadata := s.persistClientActionPending(context.WithoutCancel(ctx), prepared, pendingClientAction.Payload, usage)
				s.emitPreparedEvent(context.WithoutCancel(ctx), prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingClientAction), onEvent)
				return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusWaitingClientAction}, nil
			}
			if errors.Is(err, ErrMessageStopped) {
				_ = s.clearPreparedRuntime(context.WithoutCancel(ctx), prepared)
				return nil, err
			}
			s.finalizePreparedError(context.WithoutCancel(ctx), prepared, err, onEvent)
			return nil, newFinalizedStreamError(err)
		}
	}
	if s.streams.IsStopped(message.ID) {
		_ = s.persistStoppedAnswer(context.WithoutCancel(ctx), prepared, answer, usage)
		return nil, ErrMessageStopped
	}

	metadata := resolveClientActionContinuationMetadata(prepared.Message.Metadata, actionID, req)
	metadata = preparedResultMetadata(metadata, usage)
	prepared.Message.Metadata = metadata
	if err := s.completePreparedChat(context.WithoutCancel(ctx), prepared, answer, metadata); err != nil {
		return nil, err
	}
	s.emitPreparedEvent(context.WithoutCancel(ctx), prepared, streamEventMessageEnd, messageEndPayload(prepared, metadata), onEvent)
	return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusCompleted}, nil
}

func shouldFinalizeClientActionWithoutSkillLoop(event map[string]interface{}, req runtimedto.ClientActionResultRequest) bool {
	return strings.EqualFold(strings.TrimSpace(stringFromAny(event["action_type"])), "asset_observation") &&
		strings.EqualFold(strings.TrimSpace(req.Status), clientActionStatusSucceeded)
}

func (s *service) runClientActionFinalAnswerStream(ctx context.Context, prepared *PreparedChat, onEvent func(StreamEvent) error) (string, *adapter.Usage, error) {
	if prepared == nil || prepared.LLMRequest == nil {
		return "", nil, fmt.Errorf("%w: prepared chat is required", ErrInvalidInput)
	}
	prepared.LLMRequest.Tools = nil
	prepared.LLMRequest.ToolChoice = nil
	prepared.LLMRequest.Functions = nil
	prepared.LLMRequest.FunctionCall = nil
	stream, err := s.openChatStream(ctx, prepared)
	if err != nil {
		return "", nil, err
	}
	return s.collectStreamAnswerWithEvents(ctx, prepared, stream, onEvent, nil)
}

func (s *service) beginClientActionContinuation(ctx context.Context, scope Scope, conversationID, messageID uuid.UUID, actionID string) (*ClientActionContinuation, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	actionID = strings.TrimSpace(actionID)
	if actionID == "" {
		return nil, fmt.Errorf("%w: client action_id is required", ErrInvalidInput)
	}
	conversation, err := s.getConversation(ctx, scope, conversationID)
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
	event, ok := clientActionEventFromMetadata(message.Metadata, actionID)
	if !ok {
		return nil, fmt.Errorf("%w: client action event not found", ErrNotFound)
	}
	if message.Status == runtimemodel.MessageStatusStreaming {
		return nil, fmt.Errorf("%w: client action continuation is already running; reconnect to the active stream instead of retrying the action", ErrInvalidInput)
	}
	if message.Status != runtimemodel.MessageStatusWaitingClientAction {
		return nil, fmt.Errorf("%w: message is not waiting for client action continuation", ErrInvalidInput)
	}
	if s.repos.DB == nil {
		conversation.RuntimeStatus = runtimemodel.ConversationRuntimeStatusStreaming
		conversation.ActiveMessageID = &message.ID
		message.Status = runtimemodel.MessageStatusStreaming
		return &ClientActionContinuation{Conversation: conversation, Message: message, Event: event}, nil
	}
	err = s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&runtimemodel.Message{}).
			Where("id = ? AND deleted_at IS NULL AND status = ?", message.ID, runtimemodel.MessageStatusWaitingClientAction).
			Updates(map[string]interface{}{"status": runtimemodel.MessageStatusStreaming, "error": nil})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("%w: client action continuation is already running; reconnect to the active stream instead of retrying the action", ErrInvalidInput)
		}
		txRepos := repository.NewRepositories(tx)
		if err := txRepos.Conversation.StartStreaming(ctx, conversation.ID, scope.OrganizationID, scope.AccountID, message.ID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrInvalidInput) {
			return nil, err
		}
		return nil, mapRepoError(err)
	}
	conversation.RuntimeStatus = runtimemodel.ConversationRuntimeStatusStreaming
	conversation.ActiveMessageID = &message.ID
	message.Status = runtimemodel.MessageStatusStreaming
	return &ClientActionContinuation{Conversation: conversation, Message: message, Event: event}, nil
}

func (s *service) reloadClientActionContinuationMessage(ctx context.Context, scope Scope, conversationID, messageID uuid.UUID) (*runtimemodel.Conversation, *runtimemodel.Message, error) {
	conversation, err := s.getConversation(ctx, scope, conversationID)
	if err != nil {
		return nil, nil, err
	}
	message, err := s.repos.Message.GetScoped(ctx, messageID, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return nil, nil, mapRepoError(err)
	}
	if message.ConversationID != conversation.ID {
		return nil, nil, fmt.Errorf("%w: message belongs to another conversation", ErrInvalidInput)
	}
	return conversation, message, nil
}

func (s *service) prepareClientActionContinuationChat(ctx context.Context, scope Scope, continuation *ClientActionContinuation, req runtimedto.ClientActionResultRequest) (*PreparedChat, error) {
	if continuation == nil || continuation.Conversation == nil || continuation.Message == nil {
		return nil, fmt.Errorf("%w: client action continuation is required", ErrInvalidInput)
	}
	message := continuation.Message
	parts, err := normalizeRegenerateRequest(runtimedto.RegenerateMessageRequest{
		Surface:          req.Surface,
		RuntimeContext:   req.RuntimeContext,
		OperationContext: req.OperationContext,
	}, message)
	if err != nil {
		return nil, err
	}
	injectClientActionContinuationContext(parts, continuation.Event, req)
	parts.Attachments = attachmentBundleFromMessageMetadata(message.Metadata)
	if configured, ok := stringSliceValue(message.Metadata["configured_skill_ids"]); ok && len(configured) > 0 {
		parts.ConfiguredSkillIDs = configured
	}
	if err := s.applyModelCapabilities(ctx, scope, parts); err != nil {
		return nil, err
	}
	if err := s.applySkillConfig(ctx, scope, Caller{Type: runtimemodel.ConversationCallerAIChat}, nil, parts); err != nil {
		return nil, err
	}
	contextResult, err := s.buildUpstreamMessages(ctx, scope, message.ParentID, parts)
	if err != nil {
		return nil, err
	}
	parts.ContextControl = contextResult.Metadata
	llmRequest := newLLMChatRequest(parts, contextResult.Messages)
	llmRequest.Messages = append(llmRequest.Messages, clientActionContinuationMessage(message, continuation.Event, req))
	return &PreparedChat{
		Conversation: continuation.Conversation,
		Message:      message,
		LLMRequest:   llmRequest,
		Scope:        scope,
		Caller:       Caller{Type: runtimemodel.ConversationCallerAIChat},
		ParentID:     message.ParentID,
		parts:        parts,
	}, nil
}

func injectClientActionContinuationContext(parts *chatRequestParts, event map[string]interface{}, req runtimedto.ClientActionResultRequest) {
	if parts == nil {
		return
	}
	record := clientActionObservationRecord(event, req)
	if parts.RawOperationContext == nil {
		parts.RawOperationContext = map[string]interface{}{}
	}
	if parts.OperationContext == nil {
		parts.OperationContext = map[string]interface{}{}
	}
	parts.RawOperationContext["client_action_continuation"] = record
	parts.OperationContext["client_action_continuation"] = record
}

func clientActionContinuationMessage(message *runtimemodel.Message, event map[string]interface{}, req runtimedto.ClientActionResultRequest) adapter.Message {
	userQuery := ""
	if message != nil {
		userQuery = strings.TrimSpace(message.Query)
	}
	result := clientActionObservationRecord(event, req)
	content := strings.Join([]string{
		"Original user request:\n" + userQuery,
		"The frontend completed a client-side action for this same AIChat message.",
		"Client action result JSON:\n" + compactJSON(result),
	}, "\n\n")
	system := strings.Join([]string{
		"You are continuing the same AIChat turn after a frontend client action.",
		"Use the updated transient ZGI page context already included in this request.",
		"If the client action status is succeeded and it loaded a route, do not call console-navigator/navigate again for the same route.",
		"If the client action status is succeeded and observed a resource mutation, use the observation result and updated page context to confirm whether the changed resource is visible; do not repeat the same side-effecting tool only to verify it.",
		"Continue the user's original task from the new page context.",
		"If the client action failed or timed out, treat that as recoverable feedback and decide whether to retry, choose another route, or explain the limitation.",
		"Do not expose internal action ids, message ids, UUIDs, or raw JSON field names in the final user-visible answer.",
	}, " ")
	return adapter.Message{Role: "system", Content: system + "\n\n" + content}
}

func clientActionResultPayload(prepared *PreparedChat, event map[string]interface{}, req runtimedto.ClientActionResultRequest) map[string]interface{} {
	payload := clientActionObservationRecord(event, req)
	if prepared != nil && prepared.Conversation != nil {
		payload["conversation_id"] = prepared.Conversation.ID.String()
	}
	if prepared != nil && prepared.Message != nil {
		payload["message_id"] = prepared.Message.ID.String()
	}
	payload["event_type"] = streamEventClientActionResult
	payload["created_at"] = time.Now().Unix()
	return payload
}

func clientActionObservationRecord(event map[string]interface{}, req runtimedto.ClientActionResultRequest) map[string]interface{} {
	record := copyStringAnyMap(event)
	if record == nil {
		record = map[string]interface{}{}
	}
	record["status"] = strings.TrimSpace(req.Status)
	record["result"] = copyStringAnyMap(req.Result)
	if record["result"] == nil {
		record["result"] = map[string]interface{}{}
	}
	if errText := strings.TrimSpace(req.Error); errText != "" {
		record["error"] = errText
	}
	record["resolved_at"] = time.Now().UTC().Format(time.RFC3339)
	return compactSkillInvocation(record)
}

func normalizeClientActionResultStatus(status string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case clientActionStatusSucceeded, "success", "completed", "complete", "ok":
		return clientActionStatusSucceeded, nil
	case clientActionStatusFailed, "failure", "error", "timeout", "timed_out":
		return clientActionStatusFailed, nil
	default:
		return "", fmt.Errorf("%w: client action status must be succeeded or failed", ErrInvalidInput)
	}
}

func (s *service) failClientActionContinuation(ctx context.Context, continuation *ClientActionContinuation, cause error, onEvent func(StreamEvent) error) {
	if continuation == nil || continuation.Conversation == nil || continuation.Message == nil || cause == nil {
		return
	}
	s.finalizePreparedError(ctx, &PreparedChat{
		Conversation: continuation.Conversation,
		Message:      continuation.Message,
	}, cause, onEvent)
}

func mergeClientActionMetadata(source map[string]interface{}, event map[string]interface{}) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	actionID := clientActionID(event)
	if actionID == "" {
		return metadata
	}
	records := mapSliceFromAny(metadata["client_actions"])
	replaced := false
	for index, existing := range records {
		if clientActionID(existing) == actionID {
			records[index] = mergeInvocation(existing, event)
			replaced = true
			break
		}
	}
	if !replaced {
		records = append(records, copyStringAnyMap(event))
	}
	metadata["client_actions"] = mapsToInterfaceSlice(records)

	invocations := skillInvocationsFromMetadata(metadata["skill_invocations"])
	invocationReplaced := false
	for index, invocation := range invocations {
		if strings.TrimSpace(stringFromAny(invocation["kind"])) != "client_action" {
			continue
		}
		if clientActionID(invocation) != actionID {
			continue
		}
		invocations[index] = mergeInvocation(invocation, event)
		invocationReplaced = true
		break
	}
	if !invocationReplaced {
		values := copyStringAnyMap(event)
		if values == nil {
			values = map[string]interface{}{}
		}
		values["runtime_id"] = "client_action:" + actionID
		invocations = append(invocations, newSkillInvocation(
			"client_action",
			stringFromAny(event["skill_id"]),
			stringFromAny(event["tool_name"]),
			firstNonEmptyString(event["status"], clientActionStatusWaiting),
			values,
		))
	}
	applySkillInvocationSummary(metadata, invocations)
	return metadata
}

func resolveClientActionContinuationMetadata(source map[string]interface{}, actionID string, req runtimedto.ClientActionResultRequest) map[string]interface{} {
	actionID = strings.TrimSpace(actionID)
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	event, _ := clientActionEventFromMetadata(metadata, actionID)
	if len(event) == 0 {
		event = map[string]interface{}{"action_id": actionID}
	}
	resolved := clientActionObservationRecord(event, req)
	metadata = mergeClientActionMetadata(metadata, resolved)
	continuation := governanceMapFromAny(metadata["client_action_continuation"])
	if len(continuation) > 0 && clientActionID(continuation) == actionID {
		continuation = mergeInvocation(continuation, resolved)
		metadata["client_action_continuation"] = compactSkillInvocation(continuation)
	}
	return metadata
}

func clientActionEventFromMetadata(metadata map[string]interface{}, actionID string) (map[string]interface{}, bool) {
	actionID = strings.TrimSpace(actionID)
	if actionID == "" {
		return nil, false
	}
	for _, event := range mapSliceFromAny(metadataValue(metadata, "client_actions")) {
		if clientActionID(event) == actionID {
			return event, true
		}
	}
	continuation := governanceMapFromAny(metadataValue(metadata, "client_action_continuation"))
	if clientActionID(continuation) == actionID {
		return continuation, true
	}
	for _, invocation := range skillInvocationsFromMetadata(metadataValue(metadata, "skill_invocations")) {
		if strings.TrimSpace(stringFromAny(invocation["kind"])) != "client_action" {
			continue
		}
		if clientActionID(invocation) == actionID {
			return invocation, true
		}
	}
	return nil, false
}

func clientActionID(event map[string]interface{}) string {
	if len(event) == 0 {
		return ""
	}
	return strings.TrimSpace(stringFromAny(event["action_id"]))
}
