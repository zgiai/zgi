package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/gorm"
)

const (
	userInputContinuationStatusAnswered = "answered"
	maxUserInputAnswerRunes             = 4000
	maxUserInputResponses               = 20
)

type UserInputContinuation struct {
	Conversation *runtimemodel.Conversation
	Message      *runtimemodel.Message
	Request      map[string]interface{}
	Response     map[string]interface{}
}

func (s *service) RunUserInputContinuationStream(
	ctx context.Context,
	scope Scope,
	conversationID uuid.UUID,
	messageID uuid.UUID,
	requestID string,
	req runtimedto.UserInputContinuationRequest,
	onEvent func(StreamEvent) error,
) (*ChatResult, error) {
	if onEvent == nil {
		return nil, fmt.Errorf("%w: event callback is required", ErrInvalidInput)
	}
	continuation, err := s.beginUserInputContinuation(ctx, scope, conversationID, messageID, requestID, req.Answers)
	if err != nil {
		if IsContinuationAlreadyRunningError(err) {
			if streamErr := s.StreamConversationEvents(ctx, scope, conversationID, messageID, "", onEvent); streamErr != nil {
				return nil, streamErr
			}
			return &ChatResult{Status: runtimemodel.MessageStatusStreaming}, nil
		}
		return nil, err
	}
	prepared, err := s.prepareUserInputContinuationChat(ctx, scope, continuation, req)
	if err != nil {
		s.failUserInputContinuation(context.WithoutCancel(ctx), continuation, err, onEvent)
		return nil, newFinalizedStreamError(err)
	}

	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	s.streams.Begin(messageID, cancel)
	defer func() {
		cancel()
		s.streams.Finish(messageID)
	}()
	if s.streams.IsStopped(messageID) {
		_ = s.persistStoppedAnswer(context.WithoutCancel(ctx), prepared, "", nil)
		return nil, ErrMessageStopped
	}

	s.emitPreparedEvent(ctx, prepared, streamEventMessageStart, messageStartPayload(continuation.Conversation, continuation.Message, false), onEvent)
	answer, usage, err := s.runPreparedSkillLoop(runCtx, context.WithoutCancel(ctx), prepared, nil, onEvent)
	if err != nil {
		return s.finishUserInputContinuationPendingOrError(ctx, prepared, answer, usage, err, onEvent)
	}
	if s.streams.IsStopped(messageID) {
		_ = s.persistStoppedAnswer(context.WithoutCancel(ctx), prepared, answer, usage)
		return nil, ErrMessageStopped
	}

	metadata := preparedResultMetadata(prepared.Message.Metadata, usage)
	prepared.Message.Metadata = metadata
	if err := s.completePreparedChat(context.WithoutCancel(ctx), prepared, answer, metadata); err != nil {
		return nil, err
	}
	s.emitPreparedEvent(context.WithoutCancel(ctx), prepared, streamEventMessageEnd, messageEndPayload(prepared, metadata), onEvent)
	return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusCompleted}, nil
}

func (s *service) finishUserInputContinuationPendingOrError(
	ctx context.Context,
	prepared *PreparedChat,
	answer string,
	usage *adapter.Usage,
	cause error,
	onEvent func(StreamEvent) error,
) (*ChatResult, error) {
	persistCtx := context.WithoutCancel(ctx)
	var pendingGovernance *skillloop.ToolGovernancePendingError
	if errors.As(cause, &pendingGovernance) {
		metadata := s.persistToolGovernanceApprovalPending(persistCtx, prepared, pendingGovernance.Payload, usage)
		s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingApproval), onEvent)
		return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusWaitingApproval}, nil
	}
	var pendingApproval *skillloop.WorkflowApprovalPendingError
	if errors.As(cause, &pendingApproval) {
		metadata := s.persistWorkflowApprovalPending(persistCtx, prepared, pendingApproval.Payload, usage)
		s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingApproval), onEvent)
		return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusWaitingApproval}, nil
	}
	var pendingQuestion *skillloop.WorkflowQuestionPendingError
	if errors.As(cause, &pendingQuestion) {
		metadata := s.persistWorkflowQuestionPending(persistCtx, prepared, pendingQuestion.Payload, usage)
		s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingQuestion), onEvent)
		return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusWaitingQuestion}, nil
	}
	var pendingClientAction *skillloop.ClientActionPendingError
	if errors.As(cause, &pendingClientAction) {
		metadata := s.persistClientActionPending(persistCtx, prepared, pendingClientAction.Payload, usage)
		s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingClientAction), onEvent)
		return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusWaitingClientAction}, nil
	}
	var pendingUserInput *skillloop.UserInputPendingError
	if errors.As(cause, &pendingUserInput) {
		metadata := s.persistUserInputRequestPending(persistCtx, prepared, pendingUserInput.Payload, usage)
		s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingQuestion), onEvent)
		return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusWaitingQuestion}, nil
	}
	if errors.Is(cause, ErrMessageStopped) {
		_ = s.clearPreparedRuntime(persistCtx, prepared)
		return nil, cause
	}
	s.finalizePreparedError(persistCtx, prepared, cause, onEvent)
	return nil, newFinalizedStreamError(cause)
}

func (s *service) beginUserInputContinuation(
	ctx context.Context,
	scope Scope,
	conversationID uuid.UUID,
	messageID uuid.UUID,
	requestID string,
	answers map[string]string,
) (*UserInputContinuation, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil, fmt.Errorf("%w: user input request_id is required", ErrInvalidInput)
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
	if conversation.CurrentLeafMessageID == nil || *conversation.CurrentLeafMessageID != message.ID {
		return nil, fmt.Errorf("%w: message is not the current conversation leaf", ErrInvalidInput)
	}
	request := governanceMapFromAny(message.Metadata["user_input_request"])
	if len(request) == 0 || strings.TrimSpace(stringFromAny(request["request_id"])) != requestID {
		if userInputResponseRecorded(message.Metadata, requestID) {
			return nil, newContinuationAlreadyRunningError("user input continuation has already resolved; reconnect to the existing stream")
		}
		return nil, fmt.Errorf("%w: user input request not found", ErrNotFound)
	}
	if strings.EqualFold(strings.TrimSpace(stringFromAny(request["source"])), "agent_workflow_question_answer") {
		return nil, fmt.Errorf("%w: workflow questions use the workflow continuation endpoint", ErrInvalidInput)
	}
	response, err := normalizeUserInputContinuationResponse(requestID, request, answers)
	if err != nil {
		return nil, err
	}
	if message.Status == runtimemodel.MessageStatusStreaming {
		return nil, newContinuationAlreadyRunningError("user input continuation is already running; reconnect to the active stream")
	}
	if message.Status != runtimemodel.MessageStatusWaitingQuestion {
		return nil, fmt.Errorf("%w: message is not waiting for user input", ErrInvalidInput)
	}

	metadata := resolveUserInputContinuationMetadata(message.Metadata, request, response)
	if s.repos.DB != nil {
		err = s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			result := tx.Model(&runtimemodel.Message{}).
				Where("id = ? AND deleted_at IS NULL AND status = ?", message.ID, runtimemodel.MessageStatusWaitingQuestion).
				Updates(map[string]interface{}{"status": runtimemodel.MessageStatusStreaming, "error": nil})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return newContinuationAlreadyRunningError("user input continuation is already running; reconnect to the active stream")
			}
			txRepos := repository.NewRepositories(tx)
			if err := txRepos.Message.UpdateMetadata(ctx, message.ID, metadata); err != nil {
				return err
			}
			return txRepos.Conversation.StartStreaming(ctx, conversation.ID, scope.OrganizationID, scope.AccountID, message.ID)
		})
		if err != nil {
			if errors.Is(err, ErrInvalidInput) {
				return nil, err
			}
			return nil, mapRepoError(err)
		}
	}
	conversation.RuntimeStatus = runtimemodel.ConversationRuntimeStatusStreaming
	conversation.ActiveMessageID = &message.ID
	message.Status = runtimemodel.MessageStatusStreaming
	message.Metadata = metadata
	return &UserInputContinuation{Conversation: conversation, Message: message, Request: request, Response: response}, nil
}

func normalizeUserInputContinuationResponse(requestID string, request map[string]interface{}, answers map[string]string) (map[string]interface{}, error) {
	questions := mapSliceFromAny(request["questions"])
	if len(questions) == 0 {
		return nil, fmt.Errorf("%w: user input request has no questions", ErrInvalidInput)
	}
	normalized := make([]interface{}, 0, len(questions))
	for index, question := range questions {
		questionID := strings.TrimSpace(stringFromAny(question["id"]))
		if questionID == "" {
			questionID = fmt.Sprintf("q%d", index+1)
		}
		answer := strings.TrimSpace(answers[questionID])
		if answer == "" {
			return nil, fmt.Errorf("%w: answer is required for question %s", ErrInvalidInput, questionID)
		}
		if utf8.RuneCountInString(answer) > maxUserInputAnswerRunes {
			return nil, fmt.Errorf("%w: answer for question %s is too long", ErrInvalidInput, questionID)
		}
		normalized = append(normalized, map[string]interface{}{
			"question_id": questionID,
			"question":    strings.TrimSpace(stringFromAny(question["question"])),
			"value":       answer,
		})
	}
	now := time.Now()
	return map[string]interface{}{
		"request_id":   requestID,
		"status":       userInputContinuationStatusAnswered,
		"message":      strings.TrimSpace(stringFromAny(request["message"])),
		"answers":      normalized,
		"answer_count": len(normalized),
		"answered_at":  now.Unix(),
	}, nil
}

func resolveUserInputContinuationMetadata(source map[string]interface{}, request map[string]interface{}, response map[string]interface{}) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	requestID := strings.TrimSpace(stringFromAny(response["request_id"]))
	delete(metadata, "user_input_request")

	responses := mapSliceFromAny(metadata["user_input_responses"])
	replaced := false
	for index, existing := range responses {
		if strings.TrimSpace(stringFromAny(existing["request_id"])) == requestID {
			responses[index] = copyStringAnyMap(response)
			replaced = true
			break
		}
	}
	if !replaced {
		responses = append(responses, copyStringAnyMap(response))
	}
	if len(responses) > maxUserInputResponses {
		responses = responses[len(responses)-maxUserInputResponses:]
	}
	metadata["user_input_responses"] = mapsToInterfaceSlice(responses)
	existingContinuation := mapFromOperationContext(metadata["user_input_continuation"])
	metadata["user_input_continuation"] = compactSkillInvocation(map[string]interface{}{
		"status":          userInputContinuationStatusAnswered,
		"request_id":      requestID,
		"answered_at":     response["answered_at"],
		"answer_count":    response["answer_count"],
		"request_message": request["message"],
		"resume_policy":   "same_message",
		"next_action":     userInputPendingActionReplan,
		"original_query":  existingContinuation["original_query"],
	})

	plan := copyStringAnyMap(mapFromOperationContext(metadata["operation_plan"]))
	if plan == nil {
		plan = map[string]interface{}{}
	}
	plan["status"] = operationPlanStatusRunning
	plan["pending_next_action"] = userInputPendingActionReplan
	invocation := map[string]interface{}{
		"kind":       "user_input_response",
		"status":     "success",
		"runtime_id": "user_input:" + requestID,
		"result_summary": map[string]interface{}{
			"status":       "completed",
			"request_id":   requestID,
			"answer_count": response["answer_count"],
		},
		"result": map[string]interface{}{
			"status":       "completed",
			"request_id":   requestID,
			"answer_count": response["answer_count"],
		},
	}
	operationPlanAppendEvidenceLedgerEntry(plan, invocation, []string{"user_input:" + requestID})
	metadata["operation_plan"] = plan

	summary := copyStringAnyMap(mapFromOperationContext(metadata["operation_result_summary"]))
	if summary == nil {
		summary = map[string]interface{}{}
	}
	summary["status"] = "user_input_received"
	summary["plan_status"] = operationPlanStatusRunning
	summary["pending_next_action"] = userInputPendingActionReplan
	summary["updated_at"] = time.Now().UTC().Format(time.RFC3339)
	metadata["operation_result_summary"] = summary
	return metadata
}

func userInputResponseRecorded(metadata map[string]interface{}, requestID string) bool {
	for _, response := range mapSliceFromAny(metadata["user_input_responses"]) {
		if strings.TrimSpace(stringFromAny(response["request_id"])) == strings.TrimSpace(requestID) {
			return true
		}
	}
	return false
}

func (s *service) prepareUserInputContinuationChat(ctx context.Context, scope Scope, continuation *UserInputContinuation, req runtimedto.UserInputContinuationRequest) (*PreparedChat, error) {
	if continuation == nil || continuation.Conversation == nil || continuation.Message == nil {
		return nil, fmt.Errorf("%w: user input continuation is required", ErrInvalidInput)
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
	restoreConsoleFilesContextFromMetadata(parts, message.Metadata, nil)
	restoreConsoleAgentsContextFromMetadata(parts, message.Metadata, nil)
	restoreTurnInitialContextFromMetadata(parts, message.Metadata)
	restoreCurrentPageContextFromMetadata(parts, message.Metadata)
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
	if stateMessage := currentTurnAuthoritativeStateMessage(message); stateMessage != nil {
		llmRequest.Messages = append(llmRequest.Messages, *stateMessage)
	}
	llmRequest.Messages = append(llmRequest.Messages, userInputContinuationMessage(message, continuation.Request, continuation.Response))
	return &PreparedChat{
		Conversation: continuation.Conversation,
		Message:      message,
		LLMRequest:   llmRequest,
		Scope:        scope,
		Caller:       Caller{Type: runtimemodel.ConversationCallerAIChat},
		ParentID:     message.ParentID,
		Continuation: true,
		parts:        parts,
	}, nil
}

func userInputContinuationMessage(message *runtimemodel.Message, request map[string]interface{}, response map[string]interface{}) adapter.Message {
	payload := map[string]interface{}{
		"request_id":       response["request_id"],
		"request_message":  request["message"],
		"questions":        request["questions"],
		"answers":          response["answers"],
		"original_request": "",
	}
	if message != nil {
		payload["original_request"] = strings.TrimSpace(message.Query)
	}
	return adapter.Message{
		Role: "user",
		Content: strings.Join([]string{
			"This is the user's clarification for the same AIChat turn.",
			"Continue the unfinished task from the authoritative same-turn state. Do not restart completed work and do not ask the same question again.",
			"Before calling any remaining business tool, revise the current plan with update_plan so it reflects this clarification. You may call update_plan first and the next business tool in the same assistant response.",
			"Preserve completed phases and their evidence_refs. Change only the pending or ambiguous phases affected by the clarification.",
			"Treat the clarification as user-provided context, not as a new conversation turn.",
			"User clarification JSON:\n" + compactJSONForPrompt(payload, 12000),
		}, "\n"),
	}
}

func (s *service) failUserInputContinuation(ctx context.Context, continuation *UserInputContinuation, cause error, onEvent func(StreamEvent) error) {
	if continuation == nil || continuation.Conversation == nil || continuation.Message == nil || cause == nil {
		return
	}
	prepared := &PreparedChat{
		Conversation: continuation.Conversation,
		Message:      continuation.Message,
		Continuation: true,
	}
	s.finalizePreparedError(ctx, prepared, cause, onEvent)
	s.emitPreparedEvent(ctx, prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, continuation.Message.Metadata, runtimemodel.MessageStatusError), onEvent)
}
