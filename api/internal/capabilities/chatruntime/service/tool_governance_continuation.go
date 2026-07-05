package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"gorm.io/gorm"
)

type ToolGovernanceContinuation struct {
	Conversation *runtimemodel.Conversation
	Message      *runtimemodel.Message
	Event        map[string]interface{}
}

func (s *service) RunToolGovernanceDecisionStream(
	ctx context.Context,
	scope Scope,
	conversationID uuid.UUID,
	messageID uuid.UUID,
	correlationID string,
	req runtimedto.ToolGovernanceDecisionRequest,
	onEvent func(StreamEvent) error,
) (*ChatResult, error) {
	if onEvent == nil {
		return nil, fmt.Errorf("%w: event callback is required", ErrInvalidInput)
	}
	decision, err := s.SubmitToolGovernanceDecision(ctx, scope, conversationID, messageID, correlationID, req)
	if err != nil {
		return nil, err
	}
	continuation, err := s.beginToolGovernanceContinuation(ctx, scope, conversationID, messageID, correlationID)
	if err != nil {
		if IsContinuationAlreadyRunningError(err) {
			if streamErr := s.StreamConversationEvents(ctx, scope, conversationID, messageID, "", onEvent); streamErr != nil {
				return nil, streamErr
			}
			return &ChatResult{Status: runtimemodel.MessageStatusStreaming}, nil
		}
		return nil, err
	}

	conversation, message, err := s.reloadToolGovernanceContinuationMessage(ctx, scope, conversationID, messageID)
	if err != nil {
		s.failToolGovernanceContinuation(context.WithoutCancel(ctx), continuation, err, onEvent)
		return nil, newFinalizedStreamError(err)
	}
	continuation.Conversation = conversation
	continuation.Message = message
	continuation.Event = decision.Event

	prepared, err := s.prepareToolGovernanceContinuationChat(ctx, scope, continuation)
	if err != nil {
		s.failToolGovernanceContinuation(context.WithoutCancel(ctx), continuation, err, onEvent)
		return nil, newFinalizedStreamError(err)
	}
	s.emitPreparedEvent(ctx, prepared, streamEventMessageStart, messageStartPayload(conversation, message, false), onEvent)
	s.emitPreparedEvent(ctx, prepared, streamEventToolGovernanceDecision, decision.Event, onEvent)

	switch strings.TrimSpace(decision.Action) {
	case toolGovernanceActionReject:
		return s.runToolGovernanceRejectionContinuation(ctx, prepared, req, decision.Event, onEvent)
	case toolGovernanceActionApprove:
		return s.runToolGovernanceApprovedContinuation(ctx, prepared, decision.Event, onEvent)
	default:
		return nil, fmt.Errorf("%w: action must be approve or reject", ErrInvalidInput)
	}
}

func (s *service) failToolGovernanceContinuation(ctx context.Context, continuation *ToolGovernanceContinuation, cause error, onEvent func(StreamEvent) error) {
	if continuation == nil || continuation.Conversation == nil || continuation.Message == nil || cause == nil {
		return
	}
	prepared := &PreparedChat{
		Conversation: continuation.Conversation,
		Message:      continuation.Message,
	}
	s.finalizePreparedError(ctx, prepared, cause, onEvent)
	s.emitPreparedEvent(ctx, prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, continuation.Message.Metadata, runtimemodel.MessageStatusError), onEvent)
}

func (s *service) beginToolGovernanceContinuation(ctx context.Context, scope Scope, conversationID, messageID uuid.UUID, correlationID string) (*ToolGovernanceContinuation, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
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
	event, ok := toolGovernanceDecisionEventFromMetadata(message.Metadata, correlationID)
	if !ok {
		return nil, fmt.Errorf("%w: tool governance approval event not found", ErrNotFound)
	}
	if message.Status == runtimemodel.MessageStatusStreaming {
		return nil, newContinuationAlreadyRunningError("tool governance continuation is already running; reconnect to the active stream instead of retrying the action")
	}
	if message.Status != runtimemodel.MessageStatusWaitingApproval {
		if isResolvedToolGovernanceDecisionEvent(event) {
			return nil, newContinuationAlreadyRunningError("tool governance continuation has already resolved; reconnect to the existing stream instead of retrying the action")
		}
		return nil, fmt.Errorf("%w: message is not waiting for tool governance approval", ErrInvalidInput)
	}
	if s.repos.DB == nil {
		conversation.RuntimeStatus = runtimemodel.ConversationRuntimeStatusStreaming
		conversation.ActiveMessageID = &message.ID
		message.Status = runtimemodel.MessageStatusStreaming
		return &ToolGovernanceContinuation{Conversation: conversation, Message: message, Event: event}, nil
	}
	err = s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&runtimemodel.Message{}).
			Where("id = ? AND deleted_at IS NULL AND status = ?", message.ID, runtimemodel.MessageStatusWaitingApproval).
			Updates(map[string]interface{}{"status": runtimemodel.MessageStatusStreaming, "error": nil})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return newContinuationAlreadyRunningError("tool governance continuation is already running; reconnect to the active stream instead of retrying the action")
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
	return &ToolGovernanceContinuation{Conversation: conversation, Message: message, Event: event}, nil
}

func isResolvedToolGovernanceDecisionEvent(event map[string]interface{}) bool {
	return strings.TrimSpace(stringFromAny(event["approval_status"])) != ""
}

func (s *service) reloadToolGovernanceContinuationMessage(ctx context.Context, scope Scope, conversationID, messageID uuid.UUID) (*runtimemodel.Conversation, *runtimemodel.Message, error) {
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

func (s *service) prepareToolGovernanceContinuationChat(ctx context.Context, scope Scope, continuation *ToolGovernanceContinuation) (*PreparedChat, error) {
	if continuation == nil || continuation.Conversation == nil || continuation.Message == nil {
		return nil, fmt.Errorf("%w: tool governance continuation is required", ErrInvalidInput)
	}
	message := continuation.Message
	parts, err := normalizeRegenerateRequest(runtimedto.RegenerateMessageRequest{}, message)
	if err != nil {
		return nil, err
	}
	restoreConsoleFilesContextFromMetadata(parts, message.Metadata, continuation.Event)
	restoreConsoleAgentsContextFromMetadata(parts, message.Metadata, continuation.Event)
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

func (s *service) runToolGovernanceApprovedContinuation(ctx context.Context, prepared *PreparedChat, event map[string]interface{}, onEvent func(StreamEvent) error) (*ChatResult, error) {
	if prepared == nil || prepared.LLMRequest == nil {
		return nil, fmt.Errorf("%w: prepared chat is required", ErrInvalidInput)
	}
	if result, handled, err := s.runToolGovernanceApprovedFrozenContinuation(ctx, context.WithoutCancel(ctx), prepared, event, onEvent); handled {
		if err != nil {
			s.finalizePreparedError(context.WithoutCancel(ctx), prepared, err, onEvent)
			return nil, newFinalizedStreamError(err)
		}
		return result, nil
	}
	prepared.LLMRequest.Messages = append(prepared.LLMRequest.Messages, toolGovernanceApprovalContinuationMessage(event))
	answer, usage, err := s.runPreparedSkillStreamWithCompletionVerifier(ctx, context.WithoutCancel(ctx), prepared, nil, onEvent)
	if err != nil {
		var pendingGovernance *skillloop.ToolGovernancePendingError
		if errors.As(err, &pendingGovernance) {
			metadata := s.persistToolGovernanceApprovalPending(context.WithoutCancel(ctx), prepared, pendingGovernance.Payload, usage)
			s.emitPreparedEvent(context.WithoutCancel(ctx), prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingApproval), onEvent)
			return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusWaitingApproval}, nil
		}
		var pendingClientAction *skillloop.ClientActionPendingError
		if errors.As(err, &pendingClientAction) {
			metadata := s.persistClientActionPending(context.WithoutCancel(ctx), prepared, pendingClientAction.Payload, usage)
			s.emitPreparedEvent(context.WithoutCancel(ctx), prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingClientAction), onEvent)
			return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusWaitingClientAction}, nil
		}
		var pendingUserInput *skillloop.UserInputPendingError
		if errors.As(err, &pendingUserInput) {
			metadata := s.persistUserInputRequestPending(context.WithoutCancel(ctx), prepared, pendingUserInput.Payload, usage)
			s.emitPreparedEvent(context.WithoutCancel(ctx), prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingQuestion), onEvent)
			return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusWaitingQuestion}, nil
		}
		s.finalizePreparedError(context.WithoutCancel(ctx), prepared, err, onEvent)
		return nil, newFinalizedStreamError(err)
	}
	metadata := preparedResultMetadata(prepared.Message.Metadata, usage)
	if err := s.completePreparedChat(context.WithoutCancel(ctx), prepared, answer, metadata); err != nil {
		return nil, err
	}
	s.emitPreparedEvent(context.WithoutCancel(ctx), prepared, streamEventMessageEnd, messageEndPayload(prepared, metadata), onEvent)
	return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusCompleted}, nil
}

func (s *service) runToolGovernanceApprovedFrozenContinuation(
	ctx context.Context,
	persistCtx context.Context,
	prepared *PreparedChat,
	event map[string]interface{},
	onEvent func(StreamEvent) error,
) (*ChatResult, bool, error) {
	frozen, ok, err := toolGovernanceFrozenInvocationFromEvent(event)
	if err != nil {
		return nil, true, err
	}
	if !ok {
		return nil, false, nil
	}
	if err := validateToolGovernanceFrozenInvocation(frozen, toolGovernanceCorrelationID(event)); err != nil {
		return nil, true, err
	}
	frozen = remapLegacyFileDeleteFrozenInvocation(frozen)
	if s.skillRuntime == nil {
		return nil, true, fmt.Errorf("%w: skill runtime is not configured", ErrInvalidInput)
	}
	if prepared.parts == nil {
		return nil, true, fmt.Errorf("%w: prepared chat parts are required", ErrInvalidInput)
	}
	prepared.parts.SkillIDs = ensureFrozenInvocationSkillID(prepared.parts.SkillIDs, frozen.SkillID)
	if len(prepared.parts.SkillIDs) > 0 {
		prepared.parts.SkillMode = skillModeAuto
	}

	custom, err := s.customSkillCatalogEntries(ctx, prepared.Scope.OrganizationID)
	if err != nil {
		return nil, true, err
	}
	resolved, err := s.skillRuntime.ResolveEnabledSkillsWithCustom(ctx, prepared.parts.SkillIDs, custom)
	if err != nil {
		return nil, true, err
	}
	if len(resolved.Skills) == 0 {
		return nil, true, fmt.Errorf("%w: no skills available for configured skill ids", ErrInvalidInput)
	}

	timeline := newProcessTimelineRecorder(ctx, persistCtx, s, prepared, onEvent)
	args := copyStringAnyMap(frozen.Arguments)
	if args == nil {
		args = map[string]interface{}{}
	}
	callID := strings.TrimSpace(frozen.IdempotencyKey)
	if callID == "" {
		callID = strings.TrimSpace(frozen.ID)
	}
	timeline.RecordInvocationStart(frozen.SkillID, frozen.ToolName, args)
	invocation, err := s.skillRuntime.CallSkillTool(
		ctx,
		resolved,
		frozen.SkillID,
		frozen.ToolName,
		args,
		s.skillExecutionContext(prepared),
		callID,
	)
	executionErr := err
	if invocation == nil {
		if executionErr == nil {
			executionErr = fmt.Errorf("%w: frozen skill tool returned no invocation result", ErrInvalidInput)
		}
		invocation = recoverableFrozenInvocationFailure(nil, frozen, args, callID, executionErr)
		timeline.RecordInvocationError(invocation.Trace)
	} else {
		if invocation.Trace.Governance != nil {
			if invocation.Trace.Governance.Status != toolgovernance.DecisionStatusAllowed {
				return nil, true, fmt.Errorf("%w: frozen invocation was not allowed after approval", ErrInvalidInput)
			}
		}
		if executionErr != nil {
			invocation = recoverableFrozenInvocationFailure(invocation, frozen, args, callID, executionErr)
			timeline.RecordInvocationError(invocation.Trace)
		} else {
			invocation.Trace = enrichSkillTraceResultFromMessages(invocation.Trace, invocation.Messages)
			timeline.RecordInvocationEnd(invocation.Trace)
			for _, artifact := range skillArtifactsFromToolMessages(prepared, invocation.Trace, invocation.Messages) {
				s.persistGeneratedArtifactBestEffort(persistCtx, prepared, artifact)
				timeline.Emit(streamEventSkillArtifactCreated, artifact)
			}
			if payload := clientActionRequiredPayload(prepared, invocation.Trace, callID); len(payload) > 0 {
				timeline.RecordEvent(streamEventClientActionRequired, payload)
				metadata := s.persistClientActionPending(persistCtx, prepared, payload, nil)
				s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingClientAction), onEvent)
				return &ChatResult{Answer: "", Metadata: metadata, Usage: nil, Status: runtimemodel.MessageStatusWaitingClientAction}, true, nil
			}
		}
	}

	if invocation != nil {
		ensureOperationPlanInvocationStep(prepared.Message.Metadata, skillInvocationFromTrace(invocation.Trace, 0))
		prepared.Message.Metadata = preparedOperationEvidenceMetadata(prepared.Message.Metadata)
		if answer, ok := toolGovernanceFrozenFastPathAnswer(prepared, invocation.Trace); ok {
			s.emitPreparedEvent(persistCtx, prepared, streamEventMessage, map[string]interface{}{
				"conversation_id": prepared.Conversation.ID.String(),
				"message_id":      prepared.Message.ID.String(),
				"answer":          answer,
			}, onEvent)
			metadata := preparedResultMetadata(prepared.Message.Metadata, nil)
			if err := s.completePreparedChat(persistCtx, prepared, answer, metadata); err != nil {
				return nil, true, err
			}
			s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayload(prepared, metadata), onEvent)
			return &ChatResult{Answer: answer, Metadata: metadata, Usage: nil, Status: runtimemodel.MessageStatusCompleted}, true, nil
		}
	}

	if toolGovernanceFrozenContinuationNeedsSkillLoop(prepared) {
		prepared.LLMRequest.Messages = append(prepared.LLMRequest.Messages, toolGovernanceFrozenExecutionContinuationMessage(prepared.Message, event, invocation, executionErr))
		answer, usage, err := s.runPreparedSkillStreamWithCompletionVerifier(ctx, persistCtx, prepared, nil, onEvent)
		if err != nil {
			var pendingGovernance *skillloop.ToolGovernancePendingError
			if errors.As(err, &pendingGovernance) {
				metadata := s.persistToolGovernanceApprovalPending(persistCtx, prepared, pendingGovernance.Payload, usage)
				s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingApproval), onEvent)
				return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusWaitingApproval}, true, nil
			}
			var pendingClientAction *skillloop.ClientActionPendingError
			if errors.As(err, &pendingClientAction) {
				metadata := s.persistClientActionPending(persistCtx, prepared, pendingClientAction.Payload, usage)
				s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingClientAction), onEvent)
				return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusWaitingClientAction}, true, nil
			}
			var pendingUserInput *skillloop.UserInputPendingError
			if errors.As(err, &pendingUserInput) {
				metadata := s.persistUserInputRequestPending(persistCtx, prepared, pendingUserInput.Payload, usage)
				s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingQuestion), onEvent)
				return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusWaitingQuestion}, true, nil
			}
			return nil, true, err
		}
		metadata := preparedResultMetadata(prepared.Message.Metadata, usage)
		if err := s.completePreparedChat(persistCtx, prepared, answer, metadata); err != nil {
			return nil, true, err
		}
		s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayload(prepared, metadata), onEvent)
		return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusCompleted}, true, nil
	}

	prepared.LLMRequest = toolGovernanceExecutionResultLLMRequest(prepared.Message, event, invocation, executionErr)
	stream, err := s.openChatStream(ctx, prepared)
	if err != nil {
		return nil, true, err
	}
	answer, usage, err := s.collectStreamAnswerWithEvents(ctx, prepared, stream, onEvent, nil)
	if err != nil {
		return nil, true, err
	}
	metadata := preparedResultMetadata(prepared.Message.Metadata, usage)
	if err := s.completePreparedChat(persistCtx, prepared, answer, metadata); err != nil {
		return nil, true, err
	}
	s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayload(prepared, metadata), onEvent)
	return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusCompleted}, true, nil
}

func (s *service) persistFrozenContinuationToolTraceBestEffort(ctx context.Context, prepared *PreparedChat, trace skills.SkillTrace) {
	if prepared == nil || prepared.Message == nil {
		return
	}
	metadata := mergeFrozenContinuationToolTraceMetadata(prepared.Message.Metadata, trace)
	prepared.Message.Metadata = metadata
	if s == nil || s.repos == nil || s.repos.Message == nil {
		return
	}
	_ = s.repos.Message.UpdateMetadata(ctx, prepared.Message.ID, metadata)
}

func mergeFrozenContinuationToolTraceMetadata(source map[string]interface{}, trace skills.SkillTrace) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if strings.TrimSpace(trace.SkillID) == "" || strings.TrimSpace(trace.ToolName) == "" {
		return metadata
	}
	if strings.TrimSpace(trace.Kind) == "" || strings.EqualFold(strings.TrimSpace(trace.Kind), "tool_governance") {
		trace.Kind = "tool_call"
	}
	if strings.TrimSpace(trace.Status) == "" {
		trace.Status = "success"
	}
	invocation := skillInvocationFromTrace(trace, 0)
	if runtimeID := latestMatchingToolCallRuntimeID(metadata, trace.SkillID, trace.ToolName); runtimeID != "" {
		invocation["runtime_id"] = runtimeID
	}
	return mergeSkillInvocationMetadata(metadata, []map[string]interface{}{invocation})
}

func preparedOperationEvidenceMetadata(source map[string]interface{}) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	finalizeOperationPlanForResult(metadata)
	if summary := operationResultSummaryForPrompt(metadata); len(summary) > 0 {
		metadata["operation_result_summary"] = summary
	}
	return metadata
}

func latestMatchingToolCallRuntimeID(metadata map[string]interface{}, skillID string, toolName string) string {
	skillID = strings.TrimSpace(skillID)
	toolName = strings.TrimSpace(toolName)
	if len(metadata) == 0 || skillID == "" || toolName == "" {
		return ""
	}
	invocations := skillInvocationsFromMetadata(metadata["skill_invocations"])
	for idx := len(invocations) - 1; idx >= 0; idx-- {
		invocation := invocations[idx]
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["kind"])), "tool_call") ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["skill_id"])), skillID) ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["tool_name"])), toolName) {
			continue
		}
		if runtimeID := strings.TrimSpace(stringFromAny(invocation["runtime_id"])); runtimeID != "" {
			return runtimeID
		}
	}
	return ""
}

func toolGovernanceFrozenFastPathAnswer(prepared *PreparedChat, trace skills.SkillTrace) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(trace.Status)) {
	case "error", "failed", "failure":
		return "", false
	}
	if toolGovernanceFrozenPlanHasPendingFollowup(prepared, trace) {
		return "", false
	}
	if prepared != nil && prepared.Message != nil {
		evidence := skillLoopCompletionEvidence(prepared)()
		if answer, ok := skillloop.FastPathFinalAnswerForCompletionEvidence(evidence); ok {
			return answer, true
		}
		if answer, ok := skillloop.FastPathFinalAnswerForAgentMutationEvidence(evidence, trace); ok {
			return answer, true
		}
	}
	if answer, ok := toolGovernanceFrozenSimpleAgentConfigFastPathAnswer(prepared, trace); ok {
		return answer, true
	}
	if prepared == nil || prepared.Message == nil {
		return skillloop.FastPathFinalAnswerForToolTrace(trace)
	}
	return skillloop.FastPathFinalAnswerForToolTraceWithEvidence(trace, skillLoopCompletionEvidence(prepared)())
}

func toolGovernanceFrozenPlanHasPendingFollowup(prepared *PreparedChat, trace skills.SkillTrace) bool {
	if prepared == nil || prepared.Message == nil || len(prepared.Message.Metadata) == 0 {
		return false
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if len(plan) == 0 {
		return false
	}
	if toolGovernanceFrozenModelDecidesPlanHasPendingAgentWork(plan) {
		return true
	}
	for _, step := range operationPlanPendingExecutableSteps(plan, 8) {
		skillID := strings.TrimSpace(stringFromAny(step["skill_id"]))
		toolName := strings.TrimSpace(stringFromAny(step["tool_name"]))
		if skillID == "" && toolName == "" {
			continue
		}
		if strings.EqualFold(skillID, strings.TrimSpace(trace.SkillID)) &&
			strings.EqualFold(toolName, strings.TrimSpace(trace.ToolName)) {
			continue
		}
		return true
	}
	return false
}

func toolGovernanceFrozenSimpleAgentConfigFastPathAnswer(prepared *PreparedChat, trace skills.SkillTrace) (string, bool) {
	if prepared == nil || prepared.Message == nil ||
		!strings.EqualFold(strings.TrimSpace(trace.SkillID), skills.SkillAgentManagement) ||
		!strings.EqualFold(strings.TrimSpace(trace.ToolName), "update_agent_config") {
		return "", false
	}
	status := strings.ToLower(strings.TrimSpace(trace.Status))
	if status != "success" && status != "succeeded" && status != "completed" {
		return "", false
	}
	answer, ok := skillloop.FastPathFinalAnswerForToolTrace(trace)
	if !ok {
		return "", false
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if len(plan) == 0 || toolGovernanceFrozenPlanRequiresPostUpdateRead(plan) {
		return "", false
	}
	if toolGovernanceFrozenPlanHasPendingAgentMutationOtherThan(plan, trace.ToolName) {
		return "", false
	}
	expectedFields := toolGovernanceFrozenPlanExpectedAgentConfigFields(plan)
	if len(expectedFields) == 0 {
		expectedFields = operationPlanAgentConfigFieldsFromResult(trace.Result)
	}
	if len(expectedFields) == 0 || !toolGovernanceFrozenSimpleAgentConfigFields(expectedFields) {
		return "", false
	}
	updatedFields := operationPlanAgentConfigFieldsFromResult(trace.Result)
	for _, field := range expectedFields {
		if !stringSliceContainsFold(updatedFields, field) {
			return "", false
		}
	}
	return answer, true
}

func toolGovernanceFrozenPlanHasPendingAgentMutationOtherThan(plan map[string]interface{}, completedToolName string) bool {
	if len(plan) == 0 {
		return false
	}
	completedToolName = strings.ToLower(strings.TrimSpace(completedToolName))
	stepStatus := mapFromOperationContext(plan["step_status"])
	for _, step := range mapSliceFromAny(plan["steps"]) {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(step["skill_id"])), skills.SkillAgentManagement) {
			continue
		}
		toolName := strings.ToLower(strings.TrimSpace(stringFromAny(step["tool_name"])))
		if toolName == "" || toolName == completedToolName || !toolGovernanceFrozenPlanAgentToolIsMutation(toolName) {
			continue
		}
		id := strings.TrimSpace(stringFromAny(step["id"]))
		status := operationPlanNormalizeStepStatus(firstNonEmptyString(step["status"], stepStatus[id]))
		if status == operationPlanStepStatusCompleted || status == operationPlanStepStatusFailed {
			continue
		}
		return true
	}
	return false
}

func toolGovernanceFrozenPlanAgentToolIsMutation(toolName string) bool {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "create_agent", "delete_agent", "delete_agents", "update_agent_identity", "update_agent_config",
		"replace_agent_memory_slots", "replace_agent_skill_bindings", "replace_agent_knowledge_bindings",
		"replace_agent_database_bindings", "replace_agent_workflow_bindings":
		return true
	default:
		return false
	}
}

func toolGovernanceFrozenPlanExpectedAgentConfigFields(plan map[string]interface{}) []string {
	for _, step := range mapSliceFromAny(plan["steps"]) {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(step["skill_id"])), skills.SkillAgentManagement) ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(step["tool_name"])), "update_agent_config") {
			continue
		}
		if fields := operationPlanNormalizedAgentConfigFieldsFromAny(step[operationPlanExpectedUpdatedFieldsKey]); len(fields) > 0 {
			return fields
		}
		args := mapFromOperationContext(step["arguments"])
		if fields := operationPlanNormalizedAgentConfigFieldsFromAny(args[operationPlanExpectedUpdatedFieldsKey]); len(fields) > 0 {
			return fields
		}
	}
	return nil
}

func toolGovernanceFrozenSimpleAgentConfigFields(fields []string) bool {
	if len(fields) == 0 {
		return false
	}
	for _, field := range fields {
		switch strings.TrimSpace(field) {
		case "system_prompt", "model_provider", "model", "model_parameters",
			"agent_memory_enabled", "file_upload_enabled",
			"home_title", "input_placeholder", "theme_color", "suggested_questions":
		default:
			return false
		}
	}
	return true
}

func toolGovernanceFrozenPlanRequiresPostUpdateRead(plan map[string]interface{}) bool {
	if len(plan) == 0 {
		return false
	}
	goal := strings.ToLower(strings.TrimSpace(firstNonEmptyString(
		plan["original_user_goal"],
		plan["user_goal"],
		plan["objective"],
	)))
	if goal != "" {
		hasAfter := strings.Contains(goal, "after update") ||
			strings.Contains(goal, "after completion") ||
			strings.Contains(goal, "then read") ||
			strings.Contains(goal, "read/observe") ||
			strings.Contains(goal, "read again") ||
			strings.Contains(goal, "verify") ||
			strings.Contains(goal, "confirm") ||
			strings.Contains(goal, "\u5b8c\u6210\u540e") ||
			strings.Contains(goal, "\u4e4b\u540e") ||
			strings.Contains(goal, "\u518d\u6b21\u8bfb\u53d6") ||
			strings.Contains(goal, "\u91cd\u65b0\u8bfb\u53d6") ||
			strings.Contains(goal, "\u9a8c\u8bc1")
		if hasAfter {
			return true
		}
	}
	for _, step := range mapSliceFromAny(plan["steps"]) {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(step["skill_id"])), skills.SkillAgentManagement) {
			continue
		}
		toolName := strings.ToLower(strings.TrimSpace(stringFromAny(step["tool_name"])))
		if toolName != "get_agent_config" && toolName != "get_agent" {
			continue
		}
		if toolGovernanceBoolFromAny(step["required_post_update_verification"]) ||
			strings.EqualFold(strings.TrimSpace(stringFromAny(step["phase"])), "post_update_verification") {
			return true
		}
		id := strings.ToLower(strings.TrimSpace(stringFromAny(step["id"])))
		if strings.Contains(id, "post_update") {
			return true
		}
	}
	return false
}

func toolGovernanceBoolFromAny(value interface{}) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true") ||
			strings.EqualFold(strings.TrimSpace(typed), "yes") ||
			strings.EqualFold(strings.TrimSpace(typed), "1")
	default:
		return false
	}
}

func toolGovernanceFrozenContinuationNeedsSkillLoop(prepared *PreparedChat) bool {
	if prepared == nil || prepared.parts == nil || prepared.Message == nil {
		return false
	}
	if plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"]); toolGovernanceFrozenPlanNeedsContinuation(plan) {
		return true
	}
	return len(managedFileCreateMissingSaveTargets(prepared.parts, prepared.Message.Metadata, nil)) > 0
}

func toolGovernanceFrozenPlanNeedsContinuation(plan map[string]interface{}) bool {
	if toolGovernanceFrozenPlanHasPendingExecutableFollowup(plan) {
		return true
	}
	return toolGovernanceFrozenPlanNeedsFailureVerifier(plan)
}

func toolGovernanceFrozenPlanHasPendingExecutableFollowup(plan map[string]interface{}) bool {
	if len(plan) == 0 {
		return false
	}
	if len(operationPlanPendingExecutableSteps(plan, 8)) > 0 {
		return true
	}
	return toolGovernanceFrozenModelDecidesPlanHasPendingAgentWork(plan)
}

func toolGovernanceFrozenModelDecidesPlanHasPendingAgentWork(plan map[string]interface{}) bool {
	if len(plan) == 0 || !operationPlanModelDecidesTools(plan) {
		return false
	}
	stepStatus := mapFromOperationContext(plan["step_status"])
	for _, step := range mapSliceFromAny(plan["steps"]) {
		if !operationPlanStepBlocksCompletion(step) {
			continue
		}
		id := strings.TrimSpace(stringFromAny(step["id"]))
		status := operationPlanNormalizeStepStatus(firstNonEmptyString(step["status"], stepStatus[id]))
		if status == operationPlanStepStatusCompleted || status == operationPlanStepStatusFailed {
			continue
		}
		if operationPlanStepIsPendingAgentMutation(step, stepStatus) || operationPlanStepIsPostUpdateAgentRead(step) {
			return true
		}
	}
	return toolGovernanceFrozenPendingActionMentionsAgentWork(plan["pending_next_action"])
}

func toolGovernanceFrozenPendingActionMentionsAgentWork(value interface{}) bool {
	text := strings.ToLower(strings.TrimSpace(stringFromAny(value)))
	if text == "" {
		return false
	}
	for _, token := range []string{
		"agent-management/create_agent",
		"agent-management/delete_agent",
		"agent-management/delete_agents",
		"agent-management/update_agent_identity",
		"agent-management/update_agent_config",
		"agent-management/replace_agent_skill_bindings",
		"agent-management/replace_agent_knowledge_bindings",
		"agent-management/replace_agent_database_bindings",
		"agent-management/replace_agent_workflow_bindings",
		"agent-management/get_agent_config",
		"agent-management/get_agent",
		"create_agent",
		"delete_agent",
		"delete_agents",
		"update_agent_identity",
		"update_agent_config",
		"replace_agent_skill_bindings",
		"replace_agent_knowledge_bindings",
		"replace_agent_database_bindings",
		"replace_agent_workflow_bindings",
		"get_agent_config",
		"get_agent",
	} {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

func toolGovernanceFrozenPlanNeedsFailureVerifier(plan map[string]interface{}) bool {
	if len(plan) == 0 {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(stringFromAny(plan["status"]))) {
	case operationPlanStatusFailed, "error", "failure", "blocked":
		return true
	}
	stepStatus := mapFromOperationContext(plan["step_status"])
	for _, step := range mapSliceFromAny(plan["steps"]) {
		id := strings.TrimSpace(stringFromAny(step["id"]))
		status := operationPlanNormalizeStepStatus(firstNonEmptyString(step["status"], stepStatus[id]))
		if status == operationPlanStepStatusFailed {
			return true
		}
	}
	return false
}

func remapLegacyFileDeleteFrozenInvocation(frozen toolgovernance.FrozenInvocation) toolgovernance.FrozenInvocation {
	if strings.TrimSpace(frozen.SkillID) == skills.SkillFileReader && strings.TrimSpace(frozen.ToolName) == "delete_file" {
		frozen.SkillID = skills.SkillFileManager
	}
	return frozen
}

func ensureSkillID(skillIDs []string, skillID string) []string {
	id := strings.TrimSpace(skillID)
	if id == "" {
		return skillIDs
	}
	for _, raw := range skillIDs {
		if strings.EqualFold(strings.TrimSpace(raw), id) {
			return skillIDs
		}
	}
	out := append(append([]string(nil), skillIDs...), id)
	return out
}

func ensureFrozenInvocationSkillID(skillIDs []string, skillID string) []string {
	return ensureSkillID(skillIDs, skillID)
}

func recoverableFrozenInvocationFailure(
	invocation *skills.ToolInvocationResult,
	frozen toolgovernance.FrozenInvocation,
	args map[string]interface{},
	callID string,
	err error,
) *skills.ToolInvocationResult {
	if invocation == nil {
		invocation = &skills.ToolInvocationResult{}
	}
	trace := invocation.Trace
	if strings.TrimSpace(trace.Kind) == "" {
		trace.Kind = "tool_call"
	}
	if strings.TrimSpace(trace.SkillID) == "" {
		trace.SkillID = strings.TrimSpace(frozen.SkillID)
	}
	if strings.TrimSpace(trace.ToolName) == "" {
		trace.ToolName = strings.TrimSpace(frozen.ToolName)
	}
	trace.Status = "error"
	if strings.TrimSpace(trace.Error) == "" && err != nil {
		trace.Error = err.Error()
	}
	if trace.Arguments == nil {
		trace.Arguments = summarizeSkillToolArguments(trace.SkillID, trace.ToolName, args)
	}
	invocation.Trace = trace
	invocation.ToolMessage = skills.ToolResultMessage(callID, recoverableSkillToolErrorPayload(
		err,
		"explain the approved operation failure and decide whether to ask the user for input, suggest a configuration fix, or offer an alternative. Do not claim the operation succeeded",
		trace.SkillID,
		trace.ToolName,
	))
	return invocation
}

func toolGovernanceFrozenInvocationFromEvent(event map[string]interface{}) (toolgovernance.FrozenInvocation, bool, error) {
	approvalEvent := toolGovernanceApprovalEventFromEvent(event)
	governance := governanceMapFromAny(event["governance"])
	candidates := []interface{}{
		event["frozen_invocation"],
		approvalEvent["frozen_invocation"],
		governance["frozen_invocation"],
	}
	if nestedApproval := governanceMapFromAny(governance["approval_event"]); len(nestedApproval) > 0 {
		candidates = append(candidates, nestedApproval["frozen_invocation"])
	}
	for _, candidate := range candidates {
		frozen, ok, err := toolgovernance.FrozenInvocationFromAny(candidate)
		if err != nil || ok {
			return frozen, ok, err
		}
	}
	return toolgovernance.FrozenInvocation{}, false, nil
}

func validateToolGovernanceFrozenInvocation(frozen toolgovernance.FrozenInvocation, correlationID string) error {
	if strings.TrimSpace(frozen.SkillID) == "" || strings.TrimSpace(frozen.ToolName) == "" {
		return fmt.Errorf("%w: frozen invocation is missing skill or tool", ErrInvalidInput)
	}
	if len(frozen.Arguments) == 0 && len(frozen.Assets) == 0 && len(frozen.ExpectedAssets) == 0 {
		return fmt.Errorf("%w: frozen invocation is missing arguments and asset targets", ErrInvalidInput)
	}
	if strings.TrimSpace(frozen.CorrelationID) != "" &&
		strings.TrimSpace(correlationID) != "" &&
		strings.TrimSpace(frozen.CorrelationID) != strings.TrimSpace(correlationID) {
		return fmt.Errorf("%w: frozen invocation correlation_id mismatch", ErrInvalidInput)
	}
	if !toolgovernance.FrozenInvocationHashMatches(frozen) {
		return fmt.Errorf("%w: frozen invocation hash mismatch", ErrInvalidInput)
	}
	if frozen.ExpiresAt != nil && time.Now().UTC().After(frozen.ExpiresAt.UTC()) {
		return fmt.Errorf("%w: frozen invocation expired", ErrInvalidInput)
	}
	return nil
}

func toolGovernanceExecutionResultLLMRequest(
	message *runtimemodel.Message,
	event map[string]interface{},
	invocation *skills.ToolInvocationResult,
	executionErr error,
) *adapter.ChatRequest {
	provider := ""
	if message != nil && message.ModelProvider != nil {
		provider = strings.TrimSpace(*message.ModelProvider)
	}
	model := ""
	userQuery := ""
	if message != nil {
		model = strings.TrimSpace(message.ModelName)
		userQuery = strings.TrimSpace(message.Query)
	}
	operationSummary := toolGovernanceModelVisibleOperationSummary(event, invocation)
	toolResult := toolGovernanceModelVisibleToolResult(event, invocation, executionErr)
	outcome := "The user approved the pending governed tool call, and the runtime has already executed the frozen invocation exactly once."
	systemPrompt := strings.Join([]string{
		"You are continuing an AIChat turn after a governed tool call was approved and executed by runtime.",
		"Do not call tools.",
		"Answer in the user's language.",
		"State the actual outcome based only on the model-visible runtime result.",
		"For successful file actions, mention only the file name and action result.",
		"Do not expose internal IDs, UUIDs, workspace identifiers, correlation values, raw JSON field names, or tool count fields.",
		"Mention any error or limitation plainly.",
	}, " ")
	if executionErr != nil {
		outcome = "The user approved the pending governed tool call, and the runtime attempted to execute the frozen invocation exactly once, but it failed. The failure is recoverable model feedback, not a top-level stream failure."
		systemPrompt = strings.Join([]string{
			"You are continuing an AIChat turn after a governed tool call was approved and attempted by runtime.",
			"Do not call tools.",
			"Answer in the user's language.",
			"Treat the model-visible runtime result and failure feedback as authoritative.",
			"Explain the failure plainly and decide the next safe step.",
			"Do not claim the operation succeeded.",
			"Do not expose internal IDs, UUIDs, workspace identifiers, correlation values, raw JSON field names, or tool count fields.",
		}, " ")
	}
	contentParts := []string{
		"Original user request:\n" + userQuery,
		outcome,
		"Approved operation summary JSON:\n" + compactJSON(operationSummary),
		"Model-visible runtime result JSON:\n" + compactJSON(toolResult),
	}
	if executionErr != nil {
		contentParts = append(contentParts, "Runtime failure feedback:\n"+toolGovernanceSafeErrorText(event, invocation, executionErr))
	}
	content := strings.Join(contentParts, "\n\n")
	chatReq := &adapter.ChatRequest{
		Provider: provider,
		Model:    model,
		Stream:   true,
		Messages: []adapter.Message{
			{
				Role:    "system",
				Content: systemPrompt,
			},
			{Role: "user", Content: content},
		},
	}
	if message != nil {
		applyModelParameters(chatReq, message.ModelParameters)
	}
	return chatReq
}

func toolGovernanceFrozenExecutionContinuationMessage(
	message *runtimemodel.Message,
	event map[string]interface{},
	invocation *skills.ToolInvocationResult,
	executionErr error,
) adapter.Message {
	userQuery := ""
	if message != nil {
		userQuery = strings.TrimSpace(message.Query)
	}
	operationSummary := toolGovernanceModelVisibleOperationSummary(event, invocation)
	toolResult := toolGovernanceModelVisibleToolResult(event, invocation, executionErr)
	outcome := "The user approved the pending governed tool call, and the runtime has already executed the frozen invocation exactly once."
	systemPrompt := strings.Join([]string{
		"You are continuing the same AIChat turn after a governed tool call was approved and executed by runtime.",
		"Do not repeat the same approved tool call with the same arguments.",
		"Treat the model-visible runtime result as authoritative completed state.",
		"Use Current turn structured state as authoritative same-turn memory for derived facts and decisions recorded before approval.",
		"When Current turn structured state or the operation plan already contains the file-derived summary, selected target, model choice, or configuration fact needed for the next step, reuse that recorded evidence directly instead of navigating back or rerunning the earlier read/list tool.",
		"Continue any remaining user-requested steps using the available skills and current page context.",
		"All user-visible progress updates and final answers must use the user's language.",
		"If all requested work is complete, answer in the user's language.",
		"For successful file actions, mention only the file name and action result.",
		"Do not expose internal IDs, UUIDs, workspace identifiers, correlation values, raw JSON field names, or tool count fields.",
	}, " ")
	if executionErr != nil {
		outcome = "The user approved the pending governed tool call, and the runtime attempted to execute the frozen invocation exactly once, but it failed."
		systemPrompt = strings.Join([]string{
			"You are continuing the same AIChat turn after a governed tool call was approved and attempted by runtime.",
			"Do not repeat the same approved tool call, the same side-effecting operation, or the same asset target in this turn.",
			"Continue only independent remaining steps that do not require retrying the failed frozen operation; otherwise report the blocker.",
			"Treat the model-visible runtime result and failure feedback as authoritative.",
			"Use Current turn structured state as authoritative same-turn memory for derived facts and decisions recorded before approval.",
			"When Current turn structured state or the operation plan already contains the file-derived summary, selected target, model choice, or configuration fact needed for the next step, reuse that recorded evidence directly instead of navigating back or rerunning the earlier read/list tool.",
			"All user-visible progress updates and final answers must use the user's language.",
			"Explain the failure plainly and decide the next safe step.",
			"Do not claim the operation succeeded.",
			"Do not expose internal IDs, UUIDs, workspace identifiers, correlation values, raw JSON field names, or tool count fields.",
		}, " ")
	}
	contentParts := []string{
		"Original user request:\n" + userQuery,
		outcome,
		"Approved operation summary JSON:\n" + compactJSON(operationSummary),
		"Model-visible runtime result JSON:\n" + compactJSON(toolResult),
	}
	if planState := toolGovernanceContinuationPlanStateSummary(message); len(planState) > 0 {
		contentParts = append(contentParts, "Current operation plan continuation state JSON:\n"+compactJSON(planState))
	}
	if turnState := turnStateContinuationSummary(message); len(turnState) > 0 {
		contentParts = append(contentParts, "Current turn structured state JSON:\n"+compactJSON(turnState))
	}
	if message != nil {
		if summary := mapFromOperationContext(message.Metadata["operation_result_summary"]); len(summary) > 0 {
			contentParts = append(contentParts, "Authoritative operation result facts JSON:\n"+compactJSON(summary))
		}
	}
	if executionErr != nil {
		contentParts = append(contentParts, "Runtime failure feedback:\n"+toolGovernanceSafeErrorText(event, invocation, executionErr))
	}
	return adapter.Message{Role: "system", Content: systemPrompt + "\n\n" + strings.Join(contentParts, "\n\n")}
}

func toolGovernanceContinuationPlanStateSummary(message *runtimemodel.Message) map[string]interface{} {
	if message == nil || len(message.Metadata) == 0 {
		return nil
	}
	plan := mapFromOperationContext(message.Metadata["operation_plan"])
	if len(plan) == 0 {
		return nil
	}
	stepStatus := mapFromOperationContext(plan["step_status"])
	completed := make([]map[string]interface{}, 0, 6)
	pending := make([]map[string]interface{}, 0, 6)
	failed := make([]map[string]interface{}, 0, 3)
	for _, step := range mapSliceFromAny(plan["steps"]) {
		status := operationPlanStepResolvedStatus(step, stepStatus)
		item := toolGovernanceContinuationPlanStepSummary(step, status)
		if len(item) == 0 {
			continue
		}
		switch status {
		case operationPlanStepStatusCompleted:
			if len(completed) < 6 {
				completed = append(completed, item)
			}
		case operationPlanStepStatusFailed:
			if len(failed) < 3 {
				failed = append(failed, item)
			}
		default:
			if len(pending) < 6 {
				pending = append(pending, item)
			}
		}
	}
	summary := map[string]interface{}{
		"status": strings.TrimSpace(firstNonEmptyString(plan["status"], "in_progress")),
		"instructions": []string{
			"Continue this same turn from the pending steps only.",
			"Do not repeat completed steps or restate them as uncertain.",
			"Use evidence_ledger result_facts as authoritative completed tool facts when pending steps depend on earlier tool output.",
			"If pending_steps is empty, do not call more tools; produce a concise final answer grounded in completed_steps and tool results.",
		},
	}
	if len(completed) > 0 {
		summary["completed_steps"] = completed
	}
	if len(pending) > 0 {
		summary["pending_steps"] = pending
	} else {
		summary["pending_steps"] = []interface{}{}
		summary["all_required_steps_completed"] = true
	}
	if len(failed) > 0 {
		summary["failed_steps"] = failed
	}
	if next := strings.TrimSpace(stringFromAny(plan["pending_next_action"])); next != "" {
		summary["pending_next_action"] = next
	}
	if verification := mapFromOperationContext(plan["completion_verification"]); len(verification) > 0 &&
		(len(pending) > 0 || strings.EqualFold(strings.TrimSpace(stringFromAny(verification["status"])), "pass")) {
		summary["last_completion_verification"] = verification
	}
	if ledger := toolGovernanceContinuationEvidenceLedgerSummary(plan); len(ledger) > 0 {
		summary["evidence_ledger"] = mapsToInterfaceSlice(ledger)
	}
	return summary
}

func toolGovernanceContinuationEvidenceLedgerSummary(plan map[string]interface{}) []map[string]interface{} {
	if len(plan) == 0 {
		return nil
	}
	if state := mapFromOperationContext(plan["strategy_state"]); len(state) > 0 {
		if ledger := mapSliceFromAny(state["evidence_ledger"]); len(ledger) > 0 {
			if len(ledger) > 8 {
				ledger = ledger[len(ledger)-8:]
			}
			return ledger
		}
	}
	return operationPlanCompactEvidenceLedger(plan[operationPlanEvidenceLedgerKey], 8)
}

func toolGovernanceContinuationPlanStepSummary(step map[string]interface{}, status string) map[string]interface{} {
	if len(step) == 0 {
		return nil
	}
	skillID := strings.TrimSpace(stringFromAny(step["skill_id"]))
	toolName := strings.TrimSpace(stringFromAny(step["tool_name"]))
	id := strings.TrimSpace(stringFromAny(step["id"]))
	title := strings.TrimSpace(firstNonEmptyString(step["title"], step["label"], step["description"]))
	if title == "" && (skillID != "" || toolName != "") {
		title = operationPlanToolStepTitle(skillID, toolName)
	}
	if id == "" && title == "" && skillID == "" && toolName == "" {
		return nil
	}
	item := map[string]interface{}{
		"status": operationPlanNormalizeStepStatus(status),
	}
	if id != "" {
		item["id"] = id
	}
	if title != "" {
		item["title"] = title
	}
	if skillID != "" {
		item["skill_id"] = skillID
	}
	if toolName != "" {
		item["tool_name"] = toolName
	}
	if target := operationPlanContinuationStepTargetSummary(step); target != "" {
		item["target"] = target
	}
	if reason := strings.TrimSpace(stringFromAny(step["skipped_reason"])); reason != "" {
		item["skipped_reason"] = reason
	}
	return item
}

func operationPlanContinuationStepTargetSummary(step map[string]interface{}) string {
	for _, key := range []string{"target_name", "asset_name", "agent_name", "filename", "name"} {
		if value := strings.TrimSpace(stringFromAny(step[key])); value != "" {
			return value
		}
	}
	target := mapFromOperationContext(step["asset_target"])
	for _, key := range []string{"name", "filename", "agent_name", "page", "href"} {
		if value := strings.TrimSpace(stringFromAny(target[key])); value != "" {
			return value
		}
	}
	args := mapFromOperationContext(step["arguments"])
	for _, key := range []string{"name", "agent_name", "filename", "href"} {
		if value := strings.TrimSpace(stringFromAny(args[key])); value != "" {
			return value
		}
	}
	return ""
}

func toolGovernanceModelVisibleOperationSummary(event map[string]interface{}, invocation *skills.ToolInvocationResult) map[string]interface{} {
	summary := map[string]interface{}{}
	if action := toolGovernanceUserVisibleAction(event, invocation); action != "" {
		summary["action"] = action
	}
	if assetType := toolGovernanceUserVisibleAssetType(event); assetType != "" {
		summary["asset_type"] = assetType
	}
	if files := toolGovernanceUserVisibleFiles(event, invocation); len(files) > 0 {
		summary["files"] = files
	}
	if len(summary) == 0 {
		summary["action"] = "approved operation"
	}
	return summary
}

func toolGovernanceModelVisibleToolResult(event map[string]interface{}, invocation *skills.ToolInvocationResult, executionErr error) map[string]interface{} {
	result := map[string]interface{}{
		"status": "completed",
	}
	if executionErr != nil {
		result["status"] = "failed"
		result["recoverable_feedback"] = true
		result["error"] = toolGovernanceSafeErrorText(event, invocation, executionErr)
		result["next_step"] = "explain the failure and ask for a safe next step only when needed"
	}
	if action := toolGovernanceUserVisibleAction(event, invocation); action != "" {
		result["action"] = action
		if executionErr == nil {
			result["action_result"] = toolGovernanceUserVisibleActionResult(action)
		}
	}
	if files := toolGovernanceUserVisibleFiles(event, invocation); len(files) > 0 {
		result["files"] = files
	}

	payload := toolGovernanceFirstJSONToolPayload(invocation)
	if status := strings.TrimSpace(stringFromAny(payload["status"])); status != "" && executionErr == nil {
		result["status"] = status
	}
	for _, key := range []string{"content_status", "content", "content_truncated", "content_error"} {
		if value, ok := payload[key]; ok {
			result[key] = toolGovernanceSanitizedModelVisibleValue(value)
		}
	}
	if instruction := strings.TrimSpace(stringFromAny(payload["instruction"])); instruction != "" {
		result["tool_guidance"] = instruction
	}
	if len(result) == 1 && len(payload) > 0 {
		result["details"] = toolGovernanceSanitizedModelVisibleValue(payload)
	}
	return result
}

func toolGovernanceUserVisibleAction(event map[string]interface{}, invocation *skills.ToolInvocationResult) string {
	approvalEvent := toolGovernanceApprovalEventFromEvent(event)
	governance := governanceMapFromAny(event["governance"])
	action := strings.ToLower(strings.TrimSpace(firstNonEmptyString(
		approvalEvent["effect"],
		event["effect"],
		governance["effect"],
	)))
	if action != "" {
		return action
	}
	toolID := strings.ToLower(strings.TrimSpace(firstNonEmptyString(
		approvalEvent["tool_id"],
		event["tool_id"],
		governance["tool_id"],
	)))
	switch {
	case strings.HasSuffix(toolID, ".delete") || strings.Contains(toolID, ".delete_"):
		return "delete"
	case strings.HasSuffix(toolID, ".read") || strings.Contains(toolID, ".read_"):
		return "read"
	}
	toolName := ""
	if invocation != nil {
		toolName = strings.ToLower(strings.TrimSpace(invocation.Trace.ToolName))
	}
	if toolName == "" {
		toolName = strings.ToLower(strings.TrimSpace(firstNonEmptyString(
			event["tool_name"],
			approvalEvent["tool_name"],
			governance["tool_name"],
		)))
	}
	switch toolName {
	case "delete_file":
		return "delete"
	case "read_file":
		return "read"
	default:
		return strings.ReplaceAll(toolName, "_", " ")
	}
}

func toolGovernanceUserVisibleActionResult(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "delete":
		return "deleted"
	case "read":
		return "read"
	default:
		return "completed"
	}
}

func toolGovernanceUserVisibleAssetType(event map[string]interface{}) string {
	approvalEvent := toolGovernanceApprovalEventFromEvent(event)
	governance := governanceMapFromAny(event["governance"])
	return strings.TrimSpace(firstNonEmptyString(
		approvalEvent["asset_type"],
		event["asset_type"],
		governance["asset_type"],
	))
}

func toolGovernanceUserVisibleFiles(event map[string]interface{}, invocation *skills.ToolInvocationResult) []map[string]interface{} {
	seen := map[string]struct{}{}
	files := []map[string]interface{}{}
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		files = append(files, map[string]interface{}{"name": name})
	}

	for _, asset := range toolGovernanceAssetMapsFromEvent(event) {
		assetType := strings.ToLower(strings.TrimSpace(firstNonEmptyString(asset["type"], asset["asset_type"])))
		if assetType != "" && assetType != "file" {
			continue
		}
		add(firstNonEmptyString(asset["name"], asset["title"], asset["file_name"]))
	}

	payload := toolGovernanceFirstJSONToolPayload(invocation)
	if file := governanceMapFromAny(payload["file"]); len(file) > 0 {
		add(firstNonEmptyString(file["name"], file["title"], file["file_name"]))
	}
	for _, file := range mapSliceFromAny(payload["files"]) {
		add(firstNonEmptyString(file["name"], file["title"], file["file_name"]))
	}
	return files
}

func toolGovernanceAssetMapsFromEvent(event map[string]interface{}) []map[string]interface{} {
	if len(event) == 0 {
		return nil
	}
	out := []map[string]interface{}{}
	appendAssets := func(value interface{}) {
		for _, asset := range mapSliceFromAny(value) {
			out = append(out, asset)
		}
	}
	appendAssets(event["assets"])
	approvalEvent := toolGovernanceApprovalEventFromEvent(event)
	appendAssets(approvalEvent["assets"])
	governance := governanceMapFromAny(event["governance"])
	appendAssets(governance["assets"])
	if grant := governanceMapFromAny(approvalEvent["grant"]); len(grant) > 0 {
		appendAssets(grant["assets"])
	}
	if result := governanceMapFromAny(governance["approval_result"]); len(result) > 0 {
		if grant := governanceMapFromAny(result["approved_grant"]); len(grant) > 0 {
			appendAssets(grant["assets"])
		}
		if grant := governanceMapFromAny(result["session_grant"]); len(grant) > 0 {
			appendAssets(grant["assets"])
		}
	}
	return out
}

func toolGovernanceFirstJSONToolPayload(invocation *skills.ToolInvocationResult) map[string]interface{} {
	if invocation == nil {
		return nil
	}
	for _, message := range invocation.Messages {
		if string(message.Type) == "json" && len(message.Data) > 0 {
			return message.Data
		}
	}
	content := strings.TrimSpace(messageContentText(invocation.ToolMessage.Content))
	if content == "" {
		return nil
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(content), &payload); err == nil && payload != nil {
		return payload
	}
	return nil
}

func toolGovernanceSafeErrorText(event map[string]interface{}, invocation *skills.ToolInvocationResult, err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return "the approved operation failed"
	}
	replacements := toolGovernanceInternalIDReplacements(event, invocation)
	for id, replacement := range replacements {
		if id == "" {
			continue
		}
		if replacement == "" {
			replacement = "the file"
		}
		message = strings.ReplaceAll(message, id, replacement)
	}
	return toolGovernanceScrubModelVisibleInternalTokens(message)
}

var toolGovernanceModelVisibleInternalTokenPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`),
	regexp.MustCompile(`(?i)\b[0-9a-f]{24,}\b`),
}

func toolGovernanceScrubModelVisibleInternalTokens(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	for _, pattern := range toolGovernanceModelVisibleInternalTokenPatterns {
		text = pattern.ReplaceAllString(text, "the asset")
	}
	return text
}

func toolGovernanceInternalIDReplacements(event map[string]interface{}, invocation *skills.ToolInvocationResult) map[string]string {
	replacements := map[string]string{}
	add := func(id string, replacement string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		replacement = strings.TrimSpace(replacement)
		if existing, ok := replacements[id]; ok && existing != "" && replacement == "" {
			return
		}
		replacements[id] = replacement
	}
	for _, asset := range toolGovernanceAssetMapsFromEvent(event) {
		name := strings.TrimSpace(firstNonEmptyString(asset["name"], asset["title"], asset["file_name"]))
		add(stringFromAny(asset["id"]), name)
	}
	if invocation != nil {
		if args := invocation.Trace.Arguments; len(args) > 0 {
			add(stringFromAny(args["file_id"]), "")
			for _, id := range toolGovernanceStringSliceFromAny(args["file_ids"]) {
				add(id, "")
			}
		}
	}
	return replacements
}

func toolGovernanceStringSliceFromAny(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(stringFromAny(item))
			if text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		text := strings.TrimSpace(stringFromAny(value))
		if text == "" {
			return nil
		}
		return []string{text}
	}
}

func toolGovernanceSanitizedModelVisibleValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		out := map[string]interface{}{}
		for key, item := range typed {
			if toolGovernanceDropModelVisibleKey(key) {
				continue
			}
			out[key] = toolGovernanceSanitizedModelVisibleValue(item)
		}
		return out
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, toolGovernanceSanitizedModelVisibleValue(item))
		}
		return out
	case []map[string]interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, toolGovernanceSanitizedModelVisibleValue(item))
		}
		return out
	default:
		return value
	}
}

func toolGovernanceDropModelVisibleKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return false
	}
	if key == "id" || strings.HasSuffix(key, "_id") || strings.HasSuffix(key, "_ids") || strings.Contains(key, "uuid") {
		return true
	}
	if strings.Contains(key, "correlation") || strings.Contains(key, "grant") || strings.Contains(key, "frozen_invocation") {
		return true
	}
	if key == "deleted_count" || strings.HasSuffix(key, "_count") {
		return true
	}
	switch key {
	case "workspace_id", "upload_file_id", "conversation_id", "message_id", "organization_id", "account_id", "user_id":
		return true
	default:
		return false
	}
}

func (s *service) runToolGovernanceRejectionContinuation(ctx context.Context, prepared *PreparedChat, req runtimedto.ToolGovernanceDecisionRequest, event map[string]interface{}, onEvent func(StreamEvent) error) (*ChatResult, error) {
	prepared.LLMRequest = toolGovernanceRejectionLLMRequest(prepared.Message, req, event)
	stream, err := s.openChatStream(ctx, prepared)
	if err != nil {
		s.finalizePreparedError(context.WithoutCancel(ctx), prepared, err, onEvent)
		return nil, newFinalizedStreamError(err)
	}
	answer, usage, err := s.collectStreamAnswerWithEvents(ctx, prepared, stream, onEvent, nil)
	if err != nil {
		s.finalizePreparedError(context.WithoutCancel(ctx), prepared, err, onEvent)
		return nil, newFinalizedStreamError(err)
	}
	metadata := preparedResultMetadata(prepared.Message.Metadata, usage)
	if err := s.completePreparedChat(context.WithoutCancel(ctx), prepared, answer, metadata); err != nil {
		return nil, err
	}
	s.emitPreparedEvent(context.WithoutCancel(ctx), prepared, streamEventMessageEnd, messageEndPayload(prepared, metadata), onEvent)
	return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusCompleted}, nil
}

func toolGovernanceApprovalContinuationMessage(event map[string]interface{}) adapter.Message {
	lines := []string{
		"The user approved the pending tool governance request for this same AIChat message.",
		"Continue the original user task. Retry the previously blocked skill tool call only if it is still the correct next step.",
		"The approval is scoped to the governance grant injected into runtime parameters; do not ask for the same approval again in this continuation.",
		"The approved governance event is an authoritative asset resolution for the previously blocked tool call.",
		"If the approved governance event contains asset ids, use those asset ids directly and do not ask the user to identify the approved assets again unless the tool reports that an approved asset is missing or inaccessible.",
		"Do not claim that the action succeeded until the corresponding skill/tool call actually succeeds.",
		"Answer in the user's language. Use internal identifiers only as tool arguments; do not mention internal IDs, UUIDs, workspace identifiers, correlation values, raw JSON field names, or tool count fields in the final user-visible answer.",
		"After a successful file action, mention only the file name and action result. If the tool fails, explain the failure as recoverable feedback and do not claim success.",
	}
	lines = append(lines, toolGovernanceApprovedAssetInstructions(event)...)
	lines = append(lines, "Approved tool target JSON for tool arguments only: "+compactJSON(toolGovernanceApprovedToolTargetPayload(event)))
	return adapter.Message{Role: "system", Content: strings.Join(lines, "\n")}
}

func toolGovernanceApprovedToolTargetPayload(event map[string]interface{}) map[string]interface{} {
	approvalEvent := toolGovernanceApprovalEventFromEvent(event)
	skillID, toolName, _ := toolGovernanceApprovedToolRef(event, approvalEvent)
	payload := map[string]interface{}{}
	if skillID != "" {
		payload["skill_id"] = skillID
	}
	if toolName != "" {
		payload["tool_name"] = toolName
	}
	assets := mapSliceFromAny(approvalEvent["assets"])
	if len(assets) == 0 {
		if governance := governanceMapFromAny(event["governance"]); len(governance) > 0 {
			assets = mapSliceFromAny(governance["assets"])
		}
	}
	targets := make([]map[string]interface{}, 0, min(len(assets), 10))
	for index, asset := range assets {
		if index >= 10 {
			break
		}
		id := strings.TrimSpace(stringFromAny(asset["id"]))
		name := strings.TrimSpace(firstNonEmptyString(asset["name"], asset["title"], asset["file_name"]))
		assetType := strings.TrimSpace(firstNonEmptyString(asset["type"], asset["asset_type"]))
		target := map[string]interface{}{}
		if name != "" {
			target["name"] = name
		}
		if assetType != "" {
			target["type"] = assetType
		}
		if id != "" {
			if isFileDeleteToolRef(skillID, toolName) {
				target["file_id"] = id
			} else {
				target["id"] = id
			}
		}
		if len(target) > 0 {
			targets = append(targets, target)
		}
	}
	if len(targets) > 0 {
		payload["targets"] = targets
	}
	if len(targets) == 1 {
		if fileID := strings.TrimSpace(stringFromAny(targets[0]["file_id"])); fileID != "" {
			payload["arguments"] = map[string]interface{}{"file_id": fileID}
		}
	}
	return payload
}

func toolGovernanceApprovedAssetInstructions(event map[string]interface{}) []string {
	approvalEvent := governanceMapFromAny(event["approval_event"])
	if len(approvalEvent) == 0 {
		if governance := governanceMapFromAny(event["governance"]); len(governance) > 0 {
			approvalEvent = governanceMapFromAny(governance["approval_event"])
		}
	}
	assets := mapSliceFromAny(approvalEvent["assets"])
	if len(assets) == 0 {
		if governance := governanceMapFromAny(event["governance"]); len(governance) > 0 {
			assets = mapSliceFromAny(governance["assets"])
		}
	}
	if len(assets) == 0 {
		return nil
	}

	assetSummaries := make([]string, 0, min(len(assets), 5))
	for index, asset := range assets {
		if index >= 5 {
			break
		}
		name := strings.TrimSpace(firstNonEmptyString(
			stringFromAny(asset["name"]),
			stringFromAny(asset["title"]),
			stringFromAny(asset["file_name"]),
		))
		assetType := strings.TrimSpace(stringFromAny(asset["type"]))
		parts := make([]string, 0, 2)
		if name != "" {
			parts = append(parts, name)
		}
		if assetType != "" {
			parts = append(parts, "type="+assetType)
		}
		if len(parts) > 0 {
			assetSummaries = append(assetSummaries, strings.Join(parts, " "))
		}
	}
	lines := []string{}
	if len(assetSummaries) > 0 {
		lines = append(lines, "Approved asset names: "+strings.Join(assetSummaries, "; "))
	}
	skillID, toolName, toolID := toolGovernanceApprovedToolRef(event, approvalEvent)
	if skillID != "" && toolName != "" {
		targetIDInstruction := "approved asset ids as the target identifiers required by the tool"
		if len(assets) == 1 {
			targetIDInstruction = "the approved asset id as the target identifier required by the tool"
		}
		if toolID == "file.delete" && isFileDeleteToolRef(skillID, toolName) && len(assets) == 1 {
			targetIDInstruction = "file_id equal to the approved file asset id"
		}
		lines = append(lines, "For this approved governed operation, call "+skillID+"/"+toolName+" with "+targetIDInstruction+" before answering.")
	}
	return lines
}

func toolGovernanceApprovedToolRef(event map[string]interface{}, approvalEvent map[string]interface{}) (string, string, string) {
	if len(approvalEvent) == 0 {
		approvalEvent = toolGovernanceApprovalEventFromEvent(event)
	}
	governance := governanceMapFromAny(event["governance"])
	toolID := strings.TrimSpace(firstNonEmptyString(
		approvalEvent["tool_id"],
		event["tool_id"],
		governance["tool_id"],
	))
	skillID := strings.TrimSpace(firstNonEmptyString(
		approvalEvent["skill_id"],
		event["skill_id"],
		governance["skill_id"],
	))
	toolName := strings.TrimSpace(firstNonEmptyString(
		event["tool_name"],
		approvalEvent["tool_name"],
		governance["tool_name"],
	))
	// Compatibility for older file deletion approval events that carried only
	// file.delete as the governed tool id.
	if toolName == "" && toolID == "file.delete" && (skillID == skills.SkillFileReader || skillID == skills.SkillFileManager) {
		toolName = "delete_file"
	}
	if toolID == "file.delete" && skillID == skills.SkillFileReader && toolName == "delete_file" {
		skillID = skills.SkillFileManager
	}
	return skillID, toolName, toolID
}

func isFileDeleteToolRef(skillID string, toolName string) bool {
	skillID = strings.TrimSpace(skillID)
	toolName = strings.TrimSpace(toolName)
	return toolName == "delete_file" && (skillID == skills.SkillFileManager || skillID == skills.SkillFileReader)
}

func toolGovernanceApprovalEventFromEvent(event map[string]interface{}) map[string]interface{} {
	approvalEvent := governanceMapFromAny(event["approval_event"])
	if len(approvalEvent) > 0 {
		return approvalEvent
	}
	if governance := governanceMapFromAny(event["governance"]); len(governance) > 0 {
		return governanceMapFromAny(governance["approval_event"])
	}
	return nil
}

func approvedGovernanceAssetSummary(approvalEvent map[string]interface{}, event map[string]interface{}) string {
	assets := mapSliceFromAny(approvalEvent["assets"])
	if len(assets) == 0 {
		if governance := governanceMapFromAny(event["governance"]); len(governance) > 0 {
			assets = mapSliceFromAny(governance["assets"])
		}
	}
	if len(assets) == 0 {
		return "the approved asset"
	}
	asset := assets[0]
	name := strings.TrimSpace(firstNonEmptyString(asset["name"], asset["title"], asset["file_name"]))
	switch {
	case name != "":
		return name
	default:
		return "the approved asset"
	}
}

func toolGovernanceRejectionLLMRequest(message *runtimemodel.Message, req runtimedto.ToolGovernanceDecisionRequest, event map[string]interface{}) *adapter.ChatRequest {
	provider := ""
	if message != nil && message.ModelProvider != nil {
		provider = strings.TrimSpace(*message.ModelProvider)
	}
	model := ""
	if message != nil {
		model = strings.TrimSpace(message.ModelName)
	}
	userQuery := ""
	if message != nil {
		userQuery = strings.TrimSpace(message.Query)
	}
	content := strings.Join([]string{
		"Original user request:\n" + userQuery,
		"User rejected the pending tool governance request.",
		"The rejected action was not executed.",
		"User rejection reason:\n" + strings.TrimSpace(req.Reason),
		"Rejected operation summary JSON:\n" + compactJSON(toolGovernanceRejectedOperationSummary(event)),
	}, "\n\n")
	chatReq := &adapter.ChatRequest{
		Provider: provider,
		Model:    model,
		Stream:   true,
		Messages: []adapter.Message{
			{
				Role:    "system",
				Content: "You are continuing an AIChat turn after the user rejected a governed tool call. Do not execute or claim the rejected action. Answer in the user's language. Briefly explain that the action was not performed; for Chinese, explicitly say it was \u672a\u6267\u884c. Then offer safe alternatives or ask for a safer next step when useful. Do not expose internal IDs, UUIDs, workspace identifiers, correlation values, raw JSON field names, or tool count fields.",
			},
			{Role: "user", Content: content},
		},
	}
	if message != nil {
		applyModelParameters(chatReq, message.ModelParameters)
	}
	return chatReq
}

func toolGovernanceRejectedOperationSummary(event map[string]interface{}) map[string]interface{} {
	summary := toolGovernanceModelVisibleOperationSummary(event, nil)
	summary["status"] = "rejected"
	summary["executed"] = false
	summary["user_visible_result"] = "not_executed"
	return summary
}

func compactJSON(value interface{}) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}
