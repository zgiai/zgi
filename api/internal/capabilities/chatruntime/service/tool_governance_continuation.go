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
	"github.com/zgiai/zgi/api/internal/modules/tools"
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
	return s.RunConfiguredToolGovernanceDecisionStream(
		ctx,
		scope,
		Caller{Type: runtimemodel.ConversationCallerAIChat},
		RunConfig{BillingAppType: runtimemodel.MessageBillingReasonSourceAIChat},
		conversationID,
		messageID,
		correlationID,
		req,
		onEvent,
	)
}

func (s *service) RunConfiguredToolGovernanceDecisionStream(
	ctx context.Context,
	scope Scope,
	caller Caller,
	config RunConfig,
	conversationID uuid.UUID,
	messageID uuid.UUID,
	correlationID string,
	req runtimedto.ToolGovernanceDecisionRequest,
	onEvent func(StreamEvent) error,
) (*ChatResult, error) {
	if onEvent == nil {
		return nil, fmt.Errorf("%w: event callback is required", ErrInvalidInput)
	}
	// Read through the caller-scoped repository before recording an approval.
	// A new entry may not reuse a legacy console approval or another publication.
	conversation, message, err := s.reloadToolGovernanceContinuationMessageForCaller(ctx, scope, caller, conversationID, messageID)
	if err != nil {
		return nil, err
	}
	event, exists := toolGovernanceDecisionEventFromMetadata(message.Metadata, correlationID)
	if !exists {
		return nil, ErrNotFound
	}
	resolved := isResolvedToolGovernanceDecisionEvent(event)
	if err := checkPlatformApprovalScope(ctx, &PreparedChat{Scope: scope, Caller: caller, RunConfig: config, Conversation: conversation, Message: message}, event, req.Action == toolGovernanceActionApprove && !resolved); err != nil {
		return nil, err
	}
	publicDecision, err := s.submitToolGovernanceDecisionForCaller(ctx, scope, caller, conversationID, messageID, correlationID, req)
	if err != nil {
		return nil, err
	}
	continuation, err := s.beginToolGovernanceContinuationForCaller(ctx, scope, caller, conversationID, messageID, correlationID)
	if err != nil {
		if IsContinuationAlreadyRunningError(err) {
			if streamErr := s.StreamConversationEventsForCaller(ctx, scope, caller, conversationID, messageID, "", onEvent); streamErr != nil {
				return nil, streamErr
			}
			return &ChatResult{Status: runtimemodel.MessageStatusStreaming}, nil
		}
		return nil, err
	}

	conversation, message, err = s.reloadToolGovernanceContinuationMessageForCaller(ctx, scope, caller, conversationID, messageID)
	if err != nil {
		s.failToolGovernanceContinuation(context.WithoutCancel(ctx), continuation, err, onEvent)
		return nil, newFinalizedStreamError(err)
	}
	continuation.Conversation = conversation
	continuation.Message = message
	// SubmitToolGovernanceDecision returns a browser-safe projection whose
	// Event intentionally omits internal Connection identifiers. Never use that
	// projection for verification or execution; reload the authoritative event.
	authoritativeEvent, err := authoritativeToolGovernanceContinuationEvent(message, correlationID, publicDecision.Action)
	if err != nil {
		s.failToolGovernanceContinuation(context.WithoutCancel(ctx), continuation, err, onEvent)
		return nil, newFinalizedStreamError(err)
	}
	continuation.Event = authoritativeEvent
	if frozen, found, _ := toolGovernanceFrozenInvocationFromEvent(authoritativeEvent); found && frozen.ExecutionOrigin != nil && publicDecision.Action == toolGovernanceActionReject {
		// Closing a scoped CLI approval needs neither live compute nor a model
		// call. The original Shell and its Job grant remain stopped/revoked.
		prepared := &PreparedChat{Scope: scope, Caller: caller, RunConfig: config, Conversation: conversation, Message: message, Continuation: true}
		persistCtx := context.WithoutCancel(ctx)
		if err := s.emitPreparedEvent(persistCtx, prepared, streamEventMessageStart, messageStartPayload(conversation, message, false), onEvent); err != nil {
			return nil, finalizedRuntimePersistenceError(err)
		}
		if err := newProcessTimelineRecorder(ctx, persistCtx, s, prepared, onEvent).RecordEvent(streamEventToolGovernanceDecision, authoritativeEvent); err != nil {
			return nil, finalizedRuntimePersistenceError(err)
		}
		if err := s.completePreparedChat(persistCtx, prepared, message.Answer, message.Metadata); err != nil {
			return nil, finalizedRuntimePersistenceError(err)
		}
		if err := s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayload(prepared, message.Metadata), onEvent); err != nil {
			return nil, finalizedRuntimePersistenceError(err)
		}
		return &ChatResult{Answer: message.Answer, Metadata: message.Metadata, Status: runtimemodel.MessageStatusCompleted}, nil
	}

	prepared, err := s.prepareConfiguredToolGovernanceContinuationChat(ctx, scope, caller, config, continuation)
	if err != nil {
		s.failToolGovernanceContinuation(context.WithoutCancel(ctx), continuation, err, onEvent)
		return nil, newFinalizedStreamError(err)
	}
	execution, err := s.beginRuntimeExecution(ctx, message.ID)
	if err != nil {
		s.failToolGovernanceContinuation(context.WithoutCancel(ctx), continuation, err, onEvent)
		return nil, newFinalizedStreamError(err)
	}
	defer execution.Finish()
	runCtx := execution.Context
	persistCtx := execution.PersistContext
	if err := s.emitPreparedEvent(persistCtx, prepared, streamEventMessageStart, messageStartPayload(conversation, message, false), onEvent); err != nil {
		return nil, finalizedRuntimePersistenceError(err)
	}
	timeline := newProcessTimelineRecorder(runCtx, persistCtx, s, prepared, onEvent)
	if err := timeline.RecordEvent(streamEventToolGovernanceDecision, authoritativeEvent); err != nil {
		return nil, finalizedRuntimePersistenceError(err)
	}

	switch strings.TrimSpace(publicDecision.Action) {
	case toolGovernanceActionReject:
		return s.runToolGovernanceRejectionContinuation(runCtx, prepared, req, authoritativeEvent, onEvent)
	case toolGovernanceActionApprove:
		return s.runToolGovernanceApprovedContinuation(runCtx, prepared, authoritativeEvent, onEvent)
	default:
		return nil, fmt.Errorf("%w: action must be approve or reject", ErrInvalidInput)
	}
}

func authoritativeToolGovernanceContinuationEvent(
	message *runtimemodel.Message,
	correlationID string,
	action string,
) (map[string]interface{}, error) {
	if message == nil {
		return nil, fmt.Errorf("%w: continuation message is required", ErrInvalidInput)
	}
	event, ok := toolGovernanceDecisionEventFromMetadata(message.Metadata, correlationID)
	if !ok {
		return nil, fmt.Errorf("%w: authoritative tool governance event not found", ErrNotFound)
	}
	event = copyStringAnyMap(event)
	if strings.TrimSpace(action) != toolGovernanceActionApprove {
		return event, nil
	}
	frozen, found, err := toolGovernanceFrozenInvocationFromEvent(event)
	if err != nil {
		return nil, fmt.Errorf("%w: read authoritative frozen invocation: %v", ErrInvalidInput, err)
	}
	if found {
		if err := validateToolGovernanceFrozenInvocation(frozen, correlationID); err != nil {
			return nil, err
		}
	}
	return event, nil
}

func (s *service) failToolGovernanceContinuation(ctx context.Context, continuation *ToolGovernanceContinuation, cause error, onEvent func(StreamEvent) error) {
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

func (s *service) beginToolGovernanceContinuationForCaller(ctx context.Context, scope Scope, caller Caller, conversationID, messageID uuid.UUID, correlationID string) (*ToolGovernanceContinuation, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	conversation, err := s.getInteractiveContinuationConversation(ctx, scope, caller, conversationID)
	if err != nil {
		return nil, err
	}
	if err := ensureConversationWorkspaceScope(scope, conversation); err != nil {
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

func (s *service) reloadToolGovernanceContinuationMessageForCaller(ctx context.Context, scope Scope, caller Caller, conversationID, messageID uuid.UUID) (*runtimemodel.Conversation, *runtimemodel.Message, error) {
	conversation, err := s.getInteractiveContinuationConversation(ctx, scope, caller, conversationID)
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

func (s *service) prepareConfiguredToolGovernanceContinuationChat(ctx context.Context, scope Scope, caller Caller, config RunConfig, continuation *ToolGovernanceContinuation) (*PreparedChat, error) {
	if continuation == nil || continuation.Conversation == nil || continuation.Message == nil {
		return nil, fmt.Errorf("%w: tool governance continuation is required", ErrInvalidInput)
	}
	if err := ensureConversationWorkspaceScope(scope, continuation.Conversation); err != nil {
		return nil, err
	}
	message := continuation.Message
	regenerateReq := applyRunConfigToRegenerateRequest(config, runtimedto.RegenerateMessageRequest{})
	parts, err := normalizeRegenerateRequest(regenerateReq, message)
	if err != nil {
		return nil, err
	}
	applyRunConfigToParts(config, parts)
	applyCallerRuntimeSurfacePolicy(caller, parts)
	applyPersistedConversationSurface(continuation.Conversation, parts)
	restoreExecutionModeFromMetadata(parts, message.Metadata)
	restoreConsoleFilesContextFromMetadata(parts, message.Metadata, continuation.Event)
	restoreConsoleAgentsContextFromMetadata(parts, message.Metadata, continuation.Event)
	restoreTurnInitialContextFromMetadata(parts, message.Metadata)
	restoreCurrentPageContextFromMetadata(parts, message.Metadata)
	parts.Attachments = attachmentBundleFromMessageMetadata(message.Metadata)
	if configured, ok := stringSliceValue(message.Metadata["configured_skill_ids"]); ok && len(configured) > 0 {
		parts.ConfiguredSkillIDs = configured
	}
	if err := s.applyModelCapabilities(ctx, scope, caller, parts); err != nil {
		return nil, err
	}
	applyProtocolToolsPolicy(caller, parts)
	applyCallerMemoryPolicy(caller, parts)
	if err := s.applySkillConfig(ctx, scope, caller, &config, parts); err != nil {
		return nil, err
	}
	parts.TurnStartedAt = message.CreatedAt
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
	return &PreparedChat{
		Conversation:                   continuation.Conversation,
		Message:                        message,
		LLMRequest:                     llmRequest,
		Scope:                          scope,
		Caller:                         caller,
		RunConfig:                      config,
		ParentID:                       message.ParentID,
		Continuation:                   true,
		SuppressInitialNaturalProgress: true,
		parts:                          parts,
		contextBudget:                  contextResult,
		taskTokenBudget:                taskTokenBudgetFromMetadata(message.Metadata),
	}, nil
}

func (s *service) runToolGovernanceApprovedContinuation(ctx context.Context, prepared *PreparedChat, event map[string]interface{}, onEvent func(StreamEvent) error) (*ChatResult, error) {
	if prepared == nil || prepared.LLMRequest == nil {
		return nil, fmt.Errorf("%w: prepared chat is required", ErrInvalidInput)
	}
	prepared.ContinuationType = "tool_governance_approval"
	persistCtx := context.WithoutCancel(ctx)
	if result, handled, err := s.runToolGovernanceApprovedFrozenContinuation(ctx, persistCtx, prepared, event, onEvent); handled {
		if err != nil {
			if IsFinalizedStreamError(err) {
				return nil, err
			}
			if finalizeErr := s.finalizePreparedError(persistCtx, prepared, err, onEvent); finalizeErr != nil {
				return nil, finalizedRuntimePersistenceError(finalizeErr)
			}
			return nil, newFinalizedStreamError(err)
		}
		return result, nil
	}
	prepared.resumeWith(toolGovernanceApprovalContinuationMessage(event))
	if result, handled, budgetErr := s.handleTaskTokenBudgetPending(persistCtx, prepared, "", nil, nil, onEvent); handled {
		return result, budgetErr
	}
	answer, usage, err := s.runPreparedToolLoop(ctx, persistCtx, prepared, nil, onEvent)
	if err != nil {
		if result, handled, budgetErr := s.handleTaskTokenBudgetPending(persistCtx, prepared, answer, usage, err, onEvent); handled {
			return result, budgetErr
		}
		var pendingGovernance *skillloop.ToolGovernancePendingError
		if errors.As(err, &pendingGovernance) {
			metadata, persistErr := s.persistToolGovernanceApprovalPendingResult(persistCtx, prepared, pendingGovernance.Payload, usage)
			if persistErr != nil {
				return nil, s.finalizeToolGovernanceApprovalPersistenceError(persistCtx, prepared, persistErr, onEvent)
			}
			s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingApproval), onEvent)
			return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusWaitingApproval}, nil
		}
		var pendingConnector *skillloop.ConnectorConfirmationPendingError
		if errors.As(err, &pendingConnector) {
			metadata, persistErr := s.persistConnectorConfirmationPendingResult(persistCtx, prepared, pendingConnector.Payload, usage)
			if persistErr != nil {
				return nil, finalizedRuntimePersistenceError(persistErr)
			}
			s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingApproval), onEvent)
			return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusWaitingApproval}, nil
		}
		var pendingClientAction *skillloop.ClientActionPendingError
		if errors.As(err, &pendingClientAction) {
			metadata, persistErr := s.persistClientActionPendingResult(persistCtx, prepared, pendingClientAction.Payload, usage)
			if ownershipErr := finalizedRuntimeOwnershipError(persistErr); ownershipErr != nil {
				return nil, ownershipErr
			}
			s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingClientAction), onEvent)
			return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusWaitingClientAction}, nil
		}
		var pendingUserInput *skillloop.UserInputPendingError
		if errors.As(err, &pendingUserInput) {
			metadata, persistErr := s.persistUserInputRequestPendingResult(persistCtx, prepared, pendingUserInput.Payload, usage)
			if ownershipErr := finalizedRuntimeOwnershipError(persistErr); ownershipErr != nil {
				return nil, ownershipErr
			}
			s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingQuestion), onEvent)
			return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusWaitingQuestion}, nil
		}
		if finalizeErr := s.finalizePreparedError(persistCtx, prepared, err, onEvent); finalizeErr != nil {
			return nil, finalizedRuntimePersistenceError(finalizeErr)
		}
		return nil, newFinalizedStreamError(err)
	}
	if result, handled, budgetErr := s.handleTaskTokenBudgetPending(persistCtx, prepared, answer, usage, nil, onEvent); handled {
		return result, budgetErr
	}
	metadata := preparedResultMetadataForPrepared(prepared, prepared.Message.Metadata, usage)
	if err := s.completePreparedChat(persistCtx, prepared, answer, metadata); err != nil {
		return nil, finalizedRuntimePersistenceError(err)
	}
	s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayload(prepared, metadata), onEvent)
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
	if err := checkPlatformApprovalScope(ctx, prepared, event, true); err != nil {
		return nil, true, err
	}
	frozen = remapLegacyFileDeleteFrozenInvocation(frozen)
	prepared.ContinuationType = "tool_governance_approval"
	prepared.PreferredRestoredSkillID = strings.TrimSpace(frozen.SkillID)
	if result, handled, budgetErr := s.handleTaskTokenBudgetPending(persistCtx, prepared, "", nil, nil, onEvent); handled {
		return result, true, budgetErr
	}
	if s.skillRuntime == nil {
		return nil, true, fmt.Errorf("%w: skill runtime is not configured", ErrInvalidInput)
	}
	if prepared.parts == nil {
		return nil, true, fmt.Errorf("%w: prepared chat parts are required", ErrInvalidInput)
	}
	catalog, err := s.catalogSkillMetadata(ctx, prepared.Scope.OrganizationID)
	if err != nil {
		return nil, true, err
	}
	organizationEnabled, err := s.effectiveOrganizationSkillIDs(ctx, prepared.Scope.OrganizationID, catalog)
	if err != nil {
		return nil, true, err
	}
	// A market tool belongs to the request-scoped market Skill, which the current binding of the
	// subject supplies afresh below; it is not an organization catalog Skill.
	marketInvocation := strings.EqualFold(strings.TrimSpace(frozen.ProviderType), string(tools.ToolProviderTypeMarket))
	if !marketInvocation && !organizationAllowsSkillID(frozen.SkillID, catalog, organizationEnabled) {
		return nil, true, fmt.Errorf("%w: skill %s is not enabled by organization", ErrInvalidInput, frozen.SkillID)
	}
	prepared.parts.SkillIDs = ensureFrozenInvocationSkillID(prepared.parts.SkillIDs, frozen.SkillID)
	// The continuation has just verified the sealed invocation and the current
	// organization entitlement. Preserve that exact non-Connector Skill in the
	// explicit base projection so the post-execution model pass can summarize the
	// result. Do not copy arbitrary restored SkillIDs into BaseSkillIDs: Connector
	// capabilities must continue to come only from a fresh Target Binding lookup.
	if !marketInvocation && !strings.EqualFold(strings.TrimSpace(frozen.ProviderType), string(tools.ToolProviderTypeConnector)) {
		prepared.parts.BaseSkillIDs = ensureFrozenInvocationSkillID(prepared.parts.BaseSkillIDs, frozen.SkillID)
	}
	if len(prepared.parts.SkillIDs) > 0 {
		prepared.parts.SkillMode = skillModeAuto
	}

	custom, err := s.customSkillCatalogEntries(ctx, prepared.Scope.OrganizationID)
	if err != nil {
		return nil, true, err
	}
	resolved, err := s.skillRuntime.ResolveEnabledSkillsWithCustom(ctx, skillIDsWithout(prepared.parts.SkillIDs, []string{skills.SkillZGIMarketTools}), custom)
	if err != nil {
		return nil, true, err
	}
	if marketInvocation {
		if resolved, err = s.overlayMarketTools(ctx, prepared, resolved); err != nil {
			return nil, true, err
		}
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
	executionContext := s.skillExecutionContext(prepared)
	if frozen.ExecutionOrigin != nil {
		origin := *frozen.ExecutionOrigin
		executionContext.PlatformExecutionOrigin = &origin
		executionContext.PlatformInteractiveApproval = true
	}
	scriptModel, scriptModelUsage := s.newSkillScriptModelChannel(
		prepared,
		s.taskTokenUsageObserver(persistCtx, prepared, onEvent),
	)
	executionContext.ScriptModel = scriptModel
	executionContext.RuntimeParameters = skills.WithToolGovernanceCorrelationID(
		executionContext.RuntimeParameters,
		frozen.CorrelationID,
	)
	if err := timeline.RecordInvocationStart(callID, frozen.SkillID, frozen.ToolName, args); err != nil {
		return nil, true, finalizedRuntimePersistenceError(err)
	}
	invocation, err := s.skillRuntime.CallSkillTool(
		ctx,
		resolved,
		frozen.SkillID,
		frozen.ToolName,
		args,
		executionContext,
		callID,
	)
	frozenModelUsage := scriptModelUsage()
	executionErr := err
	if invocation == nil {
		if executionErr == nil {
			executionErr = fmt.Errorf("%w: frozen skill tool returned no invocation result", ErrInvalidInput)
		}
		invocation = recoverableFrozenInvocationFailure(nil, frozen, args, callID, executionErr, resolved)
		if err := timeline.RecordInvocationError(invocation.Trace); err != nil {
			return nil, true, finalizedRuntimePersistenceError(err)
		}
	} else {
		if invocation.Trace.Governance != nil {
			if invocation.Trace.Governance.Status != toolgovernance.DecisionStatusAllowed {
				return nil, true, fmt.Errorf("%w: frozen invocation was not allowed after approval", ErrInvalidInput)
			}
		}
		if executionErr != nil {
			invocation = recoverableFrozenInvocationFailure(invocation, frozen, args, callID, executionErr, resolved)
			if err := timeline.RecordInvocationError(invocation.Trace); err != nil {
				return nil, true, finalizedRuntimePersistenceError(err)
			}
		} else {
			invocation.Trace = enrichSkillTraceResultFromMessages(invocation.Trace, invocation.Messages)
			if err := timeline.RecordInvocationEnd(invocation.Trace); err != nil {
				return nil, true, finalizedRuntimePersistenceError(err)
			}
			for _, artifact := range skillArtifactsFromToolMessages(prepared, invocation.Trace, invocation.Messages) {
				s.persistGeneratedArtifactBestEffort(persistCtx, prepared, artifact)
				if err := timeline.RecordEvent(streamEventSkillArtifactCreated, artifact); err != nil {
					return nil, true, finalizedRuntimePersistenceError(err)
				}
			}
			if payload := clientActionRequiredPayload(prepared, invocation.Trace, callID); len(payload) > 0 {
				if err := timeline.RecordEvent(streamEventClientActionRequired, payload); err != nil {
					return nil, true, finalizedRuntimePersistenceError(err)
				}
				if clientActionRequiresModelContinuation(payload) {
					metadata, persistErr := s.persistClientActionPendingResult(persistCtx, prepared, payload, frozenModelUsage)
					if ownershipErr := finalizedRuntimeOwnershipError(persistErr); ownershipErr != nil {
						return nil, true, ownershipErr
					}
					s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingClientAction), onEvent)
					return &ChatResult{Answer: "", Metadata: metadata, Usage: frozenModelUsage, Status: runtimemodel.MessageStatusWaitingClientAction}, true, nil
				}
			}
		}
	}

	if invocation != nil {
		prepared.Message.Metadata = preparedOperationEvidenceMetadata(prepared.Message.Metadata)
	}
	if executionErr == nil {
		// The approved call is recorded in the ledgers; the model decides what
		// is left to do, and its checklist is its own (ADR 0049).
		metadata := prepared.Message.Metadata
		if s.repos != nil && s.repos.Message != nil {
			if err := s.repos.Message.UpdateMetadata(persistCtx, prepared.Message.ID, metadata); err != nil {
				return nil, true, finalizedRuntimePersistenceError(err)
			}
		}
	}

	if marketInvocation {
		// The transcript of the turn still ends with the tool result that said the call was
		// waiting for approval, and a model trusts that over the continuation note that follows.
		replacePendingMarketToolResult(prepared.LLMRequest, frozen.CorrelationID, toolGovernanceModelVisibleToolResult(event, invocation, executionErr))
	}
	prepared.resumeWith(toolGovernanceFrozenExecutionContinuationMessage(prepared.Message, event, invocation, executionErr))
	if frozen.ExecutionOrigin != nil {
		// A CLI approval resumes one sealed business invocation, not its stopped
		// script or a new planning loop. Summarization cannot issue more tools.
		prepared.TerminalOnly = true
		prepared.RunConfig.RuntimeToolFactory = nil
	}
	answer, usage, err := s.runPreparedToolLoop(ctx, persistCtx, prepared, nil, onEvent)
	usage = addUsageCloned(usage, frozenModelUsage)
	if err != nil {
		if result, handled, budgetErr := s.handleTaskTokenBudgetPending(persistCtx, prepared, answer, usage, err, onEvent); handled {
			return result, true, budgetErr
		}
		var pendingGovernance *skillloop.ToolGovernancePendingError
		if errors.As(err, &pendingGovernance) {
			metadata, persistErr := s.persistToolGovernanceApprovalPendingResult(persistCtx, prepared, pendingGovernance.Payload, usage)
			if persistErr != nil {
				return nil, true, s.finalizeToolGovernanceApprovalPersistenceError(persistCtx, prepared, persistErr, onEvent)
			}
			s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingApproval), onEvent)
			return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusWaitingApproval}, true, nil
		}
		var pendingConnector *skillloop.ConnectorConfirmationPendingError
		if errors.As(err, &pendingConnector) {
			metadata, persistErr := s.persistConnectorConfirmationPendingResult(persistCtx, prepared, pendingConnector.Payload, usage)
			if persistErr != nil {
				return nil, true, finalizedRuntimePersistenceError(persistErr)
			}
			s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingApproval), onEvent)
			return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusWaitingApproval}, true, nil
		}
		var pendingClientAction *skillloop.ClientActionPendingError
		if errors.As(err, &pendingClientAction) {
			metadata, persistErr := s.persistClientActionPendingResult(persistCtx, prepared, pendingClientAction.Payload, usage)
			if ownershipErr := finalizedRuntimeOwnershipError(persistErr); ownershipErr != nil {
				return nil, true, ownershipErr
			}
			s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingClientAction), onEvent)
			return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusWaitingClientAction}, true, nil
		}
		var pendingUserInput *skillloop.UserInputPendingError
		if errors.As(err, &pendingUserInput) {
			metadata, persistErr := s.persistUserInputRequestPendingResult(persistCtx, prepared, pendingUserInput.Payload, usage)
			if ownershipErr := finalizedRuntimeOwnershipError(persistErr); ownershipErr != nil {
				return nil, true, ownershipErr
			}
			s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingQuestion), onEvent)
			return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusWaitingQuestion}, true, nil
		}
		return nil, true, err
	}
	if result, handled, budgetErr := s.handleTaskTokenBudgetPending(persistCtx, prepared, answer, usage, nil, onEvent); handled {
		return result, true, budgetErr
	}
	metadata := preparedResultMetadataForPrepared(prepared, prepared.Message.Metadata, usage)
	if err := s.completePreparedChat(persistCtx, prepared, answer, metadata); err != nil {
		return nil, true, finalizedRuntimePersistenceError(err)
	}
	s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayload(prepared, metadata), onEvent)
	return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusCompleted}, true, nil
}

func preparedOperationEvidenceMetadata(source map[string]interface{}) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if summary := operationResultSummaryForPrompt(metadata); len(summary) > 0 {
		metadata["operation_result_summary"] = summary
	}
	return metadata
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
	resolved *skills.ResolvedSkills,
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
		if skills.SkillToolUsesResolvedInputSchema(resolved, trace.SkillID, trace.ToolName) {
			trace.Arguments = map[string]interface{}{
				"schema_bound_arguments_redacted": true,
				"argument_count":                  len(args),
			}
		} else {
			trace.Arguments = summarizeSkillToolArguments(trace.SkillID, trace.ToolName, args)
		}
	}
	invocation.Trace = trace
	invocation.ToolMessage = skills.ToolResultMessage(callID, recoverableSkillToolErrorPayload(
		err,
		"explain the approved operation failure and decide whether to ask the user for input, suggest a configuration fix, or offer an alternative. Do not claim the operation succeeded",
		trace.SkillID,
		trace.ToolName,
		resolved,
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
	return validateToolGovernanceFrozenInvocationForDecision(frozen, correlationID, true)
}

func validateToolGovernanceFrozenInvocationForDecision(frozen toolgovernance.FrozenInvocation, correlationID string, approve bool) error {
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
	if approve && frozen.ExpiresAt != nil && time.Now().UTC().After(frozen.ExpiresAt.UTC()) {
		return fmt.Errorf("%w: frozen invocation expired", ErrInvalidInput)
	}
	return nil
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
		"You are continuing the same assistant turn after a governed tool call was approved and executed by runtime.",
		"The user already saw progress emitted before approval. Do not acknowledge or restate the original request, the latest correction, or completed progress. If you emit progress, begin directly with the newly reached evidence or next concrete action.",
		"Continue with the next necessary tool call, or answer if nothing is left. If you keep a checklist and its steps changed, update it with update_plan.",
		"Do not repeat the same approved tool call with the same arguments.",
		"Treat the model-visible runtime result as authoritative completed state.",
		"Use Current turn structured state as authoritative same-turn memory for derived facts and decisions recorded before approval.",
		"Treat Current-turn execution state as authoritative: continue with active_target when present, and do not create a replacement asset unless the original user request explicitly asks for another distinct target.",
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
			"You are continuing the same assistant turn after a governed tool call was approved and attempted by runtime.",
			"Do not repeat the same approved tool call, the same side-effecting operation, or the same asset target in this turn.",
			"Continue only independent remaining steps that do not require retrying the failed frozen operation; otherwise report the blocker.",
			"Treat the model-visible runtime result and failure feedback as authoritative.",
			"Use Current turn structured state as authoritative same-turn memory for derived facts and decisions recorded before approval.",
			"Treat Current-turn execution state as authoritative: continue with active_target when present, and do not create a replacement asset unless the original user request explicitly asks for another distinct target.",
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
	if toolGovernanceHasStoppedPlatformShell(message) {
		contentParts = append(contentParts, "Platform CLI continuation state: the original Shell ran until the approval boundary and was stopped there. It is not still running or awaiting another approval. This continuation attempted only the frozen business tool, not the Shell. Do not invent a new pending Shell, replay it, or claim that its remaining script statements executed. Its earlier awaiting_approval trace describes history, not current work.")
	}
	if planState := toolGovernanceContinuationPlanStateSummary(message); len(planState) > 0 {
		contentParts = append(contentParts, "Current operation plan continuation state JSON:\n"+compactJSON(planState))
	}
	if executionState := currentTurnExecutionStateSummary(message); len(executionState) > 0 {
		contentParts = append(contentParts, "Current-turn execution state JSON:\n"+compactJSON(executionState))
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
	summary := map[string]interface{}{
		"status":             strings.TrimSpace(firstNonEmptyString(plan["status"], "in_progress")),
		"plan_sync_status":   strings.TrimSpace(stringFromAny(plan["plan_sync_status"])),
		"original_user_goal": compactForPrompt(stringFromAny(plan["original_user_goal"]), 500),
		"instructions": []string{
			"Continue the same user turn after the approved frozen invocation.",
			"Treat phases as the model's advisory progress snapshot, not a required tool sequence.",
			"Use evidence_ledger result_facts as authoritative completed tool facts and choose the next action yourself.",
		},
	}
	if phases := operationPlanCompactPhasesForPrompt(plan["phases"], 8); len(phases) > 0 {
		summary["phases"] = phases
	}
	if outcomes := operationPlanCompactOutcomesForPrompt(plan[operationPlanOutcomesKey], 8); len(outcomes) > 0 {
		summary["outcomes"] = outcomes
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

func currentTurnExecutionStateSummary(message *runtimemodel.Message) map[string]interface{} {
	if message == nil || len(message.Metadata) == 0 {
		return nil
	}
	invocations := skillInvocationsFromMetadata(message.Metadata["skill_invocations"])
	if len(invocations) == 0 {
		return nil
	}
	out := map[string]interface{}{
		"instructions": []string{
			"Continue the same user turn from these recorded execution facts.",
			"Do not repeat completed side-effecting operations with the same target and arguments.",
			"Use active_target ids/names as the current asset target for remaining configuration or verification steps.",
			"If a failed operation is present, repair or continue from that exact failure context instead of restarting the whole task.",
		},
	}
	if target := currentTurnActiveAgentTarget(invocations); len(target) > 0 {
		out["active_target"] = target
	}
	if completed := currentTurnCompletedOperations(invocations, 8); len(completed) > 0 {
		out["completed_operations"] = mapsToInterfaceSlice(completed)
	}
	if failed := currentTurnFailedOperations(invocations, 4); len(failed) > 0 {
		out["failed_operations"] = mapsToInterfaceSlice(failed)
		out["has_failed_operation"] = true
	}
	if loaded := currentTurnLoadedSkills(invocations, 8); len(loaded) > 0 {
		out["loaded_skills"] = loaded
	}
	if len(out) == 1 {
		return nil
	}
	return out
}

func currentTurnActiveAgentTarget(invocations []map[string]interface{}) map[string]interface{} {
	for index := len(invocations) - 1; index >= 0; index-- {
		invocation := invocations[index]
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["kind"])), "tool_call") ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["skill_id"])), skills.SkillAgentManagement) ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["tool_name"])), "create_agent") ||
			!toolGovernanceContinuationInvocationSucceeded(invocation) {
			continue
		}
		if target := currentTurnAgentTargetFromInvocation(invocation); len(target) > 0 {
			target["asset_type"] = "agent"
			target["source_tool"] = "agent-management/create_agent"
			return target
		}
	}
	return nil
}

func currentTurnCompletedOperations(invocations []map[string]interface{}, limit int) []map[string]interface{} {
	if limit <= 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, limit)
	for _, invocation := range invocations {
		if len(out) >= limit {
			break
		}
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["kind"])), "tool_call") ||
			!toolGovernanceContinuationInvocationSucceeded(invocation) {
			continue
		}
		skillID := strings.TrimSpace(stringFromAny(invocation["skill_id"]))
		toolName := strings.TrimSpace(stringFromAny(invocation["tool_name"]))
		if skillID == "" || toolName == "" || !skillLoopToolLooksAssetMutation(skillID, toolName) {
			continue
		}
		if item := currentTurnOperationSummary(invocation); len(item) > 0 {
			out = append(out, item)
		}
	}
	return out
}

func currentTurnFailedOperations(invocations []map[string]interface{}, limit int) []map[string]interface{} {
	if limit <= 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, limit)
	for index, invocation := range invocations {
		if len(out) >= limit {
			break
		}
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["kind"])), "tool_call") ||
			!toolGovernanceContinuationInvocationFailed(invocation) {
			continue
		}
		if currentTurnFailedOperationRecovered(invocations, index, invocation) {
			continue
		}
		if item := currentTurnOperationSummary(invocation); len(item) > 0 {
			out = append(out, item)
		}
	}
	return out
}

func currentTurnFailedOperationRecovered(invocations []map[string]interface{}, failedIndex int, failed map[string]interface{}) bool {
	if failedIndex < 0 || failedIndex >= len(invocations) || len(failed) == 0 {
		return false
	}
	for _, candidate := range invocations[failedIndex+1:] {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(candidate["kind"])), "tool_call") ||
			!toolGovernanceContinuationInvocationSucceeded(candidate) {
			continue
		}
		if currentTurnSameOperationRecovered(failed, candidate) ||
			currentTurnFileReadRecoveryCoversFailedCall(failed, candidate) {
			return true
		}
	}
	return false
}

func currentTurnSameOperationRecovered(failed map[string]interface{}, candidate map[string]interface{}) bool {
	if !strings.EqualFold(strings.TrimSpace(stringFromAny(failed["skill_id"])), strings.TrimSpace(stringFromAny(candidate["skill_id"]))) ||
		!strings.EqualFold(strings.TrimSpace(stringFromAny(failed["tool_name"])), strings.TrimSpace(stringFromAny(candidate["tool_name"]))) {
		return false
	}
	failedTarget := currentTurnOperationComparableTarget(failed)
	candidateTarget := currentTurnOperationComparableTarget(candidate)
	if failedTarget == "" || candidateTarget == "" {
		return false
	}
	return strings.EqualFold(failedTarget, candidateTarget)
}

func currentTurnFileReadRecoveryCoversFailedCall(failed map[string]interface{}, candidate map[string]interface{}) bool {
	if !strings.EqualFold(strings.TrimSpace(stringFromAny(candidate["skill_id"])), skills.SkillFileReader) ||
		!strings.EqualFold(strings.TrimSpace(stringFromAny(candidate["tool_name"])), "read_file") {
		return false
	}
	failedArgs := mapFromOperationContext(failed["arguments"])
	fileID := strings.TrimSpace(firstNonEmptyString(failedArgs["file_id"], failedArgs["id"], mapFromOperationContext(failed["result"])["file_id"], mapFromOperationContext(failed["result_summary"])["file_id"]))
	if fileID == "" {
		return false
	}
	candidateTarget := currentTurnOperationComparableTarget(candidate)
	return strings.EqualFold(candidateTarget, "file:"+fileID)
}

func currentTurnOperationComparableTarget(invocation map[string]interface{}) string {
	if len(invocation) == 0 {
		return ""
	}
	target := currentTurnAgentTargetFromInvocation(invocation)
	if id := strings.TrimSpace(stringFromAny(target["agent_id"])); id != "" {
		return "agent:" + id
	}
	result := mapFromOperationContext(invocation["result"])
	summary := mapFromOperationContext(invocation["result_summary"])
	args := mapFromOperationContext(invocation["arguments"])
	if id := strings.TrimSpace(firstNonEmptyString(result["file_id"], result["id"], summary["file_id"], summary["id"], args["file_id"], args["id"])); id != "" {
		return "file:" + id
	}
	return ""
}

func currentTurnLoadedSkills(invocations []map[string]interface{}, limit int) []string {
	if limit <= 0 {
		return nil
	}
	out := make([]string, 0, limit)
	seen := map[string]struct{}{}
	for _, invocation := range invocations {
		if len(out) >= limit {
			break
		}
		kind := strings.ToLower(strings.TrimSpace(stringFromAny(invocation["kind"])))
		if kind != "skill_load" && kind != "load_skill" {
			continue
		}
		if !toolGovernanceContinuationInvocationSucceeded(invocation) {
			continue
		}
		skillID := strings.TrimSpace(stringFromAny(invocation["skill_id"]))
		if skillID == "" {
			skillID = strings.TrimSpace(stringFromAny(invocation["tool_name"]))
		}
		if skillID == "" {
			continue
		}
		key := strings.ToLower(skillID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, skillID)
	}
	return out
}

func currentTurnOperationSummary(invocation map[string]interface{}) map[string]interface{} {
	skillID := strings.TrimSpace(stringFromAny(invocation["skill_id"]))
	toolName := strings.TrimSpace(stringFromAny(invocation["tool_name"]))
	if skillID == "" && toolName == "" {
		return nil
	}
	item := map[string]interface{}{}
	if skillID != "" {
		item["skill_id"] = skillID
	}
	if toolName != "" {
		item["tool_name"] = toolName
	}
	if status := strings.TrimSpace(firstNonEmptyString(invocation["status"], invocation["result_status"])); status != "" {
		item["status"] = status
	}
	if target := currentTurnAgentTargetFromInvocation(invocation); len(target) > 0 {
		item["target"] = target
	}
	if errText := strings.TrimSpace(firstNonEmptyString(invocation["error"], mapFromOperationContext(invocation["result"])["error"], mapFromOperationContext(invocation["result_summary"])["error"])); errText != "" {
		item["error"] = truncateRunes(errText, 240)
	}
	return item
}

func currentTurnAgentTargetFromInvocation(invocation map[string]interface{}) map[string]interface{} {
	result := mapFromOperationContext(invocation["result"])
	resultSummary := mapFromOperationContext(invocation["result_summary"])
	agent := mapFromOperationContext(result["agent"])
	args := mapFromOperationContext(invocation["arguments"])
	target := map[string]interface{}{}
	if id := strings.TrimSpace(firstNonEmptyString(result["agent_id"], result["id"], resultSummary["agent_id"], resultSummary["id"], agent["agent_id"], agent["id"], args["agent_id"], args["id"])); id != "" {
		target["agent_id"] = id
	}
	if name := strings.TrimSpace(firstNonEmptyString(result["agent_name"], result["name"], resultSummary["agent_name"], resultSummary["name"], agent["agent_name"], agent["name"], args["agent_name"], args["name"])); name != "" {
		target["name"] = name
	}
	if len(target) == 0 {
		return nil
	}
	return target
}

func toolGovernanceContinuationInvocationSucceeded(invocation map[string]interface{}) bool {
	status := strings.ToLower(strings.TrimSpace(firstNonEmptyString(invocation["status"], invocation["result_status"])))
	switch status {
	case "success", "succeeded", "completed", "approved":
		return true
	default:
		return false
	}
}

func toolGovernanceContinuationInvocationFailed(invocation map[string]interface{}) bool {
	status := strings.ToLower(strings.TrimSpace(firstNonEmptyString(invocation["status"], invocation["result_status"])))
	switch status {
	case "error", "failed", "failure", "partial_failed", "partially_failed":
		return true
	}
	if errText := strings.TrimSpace(firstNonEmptyString(invocation["error"], mapFromOperationContext(invocation["result"])["error"], mapFromOperationContext(invocation["result_summary"])["error"])); errText != "" {
		return true
	}
	return false
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
	// Generic business outputs (for example calculator values) must not be
	// discarded merely because governance also supplies an action/effect.
	// Keep file-operation projections narrow and preserve their existing rules.
	if executionErr == nil && len(payload) > 0 && (len(result) == 1 || toolGovernanceUserVisibleAssetType(event) != "file") {
		details := toolGovernanceSanitizedModelVisibleValue(payload)
		encoded, err := json.Marshal(details)
		if err == nil && len(encoded) <= 32<<10 {
			result["details"] = details
		} else {
			result["details_truncated"] = true
		}
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
	persistCtx := context.WithoutCancel(ctx)
	if result, handled, budgetErr := s.handleTaskTokenBudgetPending(persistCtx, prepared, "", nil, nil, onEvent); handled {
		return result, budgetErr
	}
	prepared.LLMRequest = toolGovernanceRejectionLLMRequest(prepared.Message, req, event)
	stream, err := s.openChatStream(ctx, prepared)
	if err != nil {
		if finalizeErr := s.finalizePreparedError(persistCtx, prepared, err, onEvent); finalizeErr != nil {
			return nil, finalizedRuntimePersistenceError(finalizeErr)
		}
		return nil, newFinalizedStreamError(err)
	}
	answer, usage, err := s.collectStreamAnswerWithEvents(ctx, prepared, stream, onEvent, nil)
	if err != nil {
		if finalizeErr := s.finalizePreparedError(persistCtx, prepared, err, onEvent); finalizeErr != nil {
			return nil, finalizedRuntimePersistenceError(finalizeErr)
		}
		return nil, newFinalizedStreamError(err)
	}
	accountTaskTokenUsage(prepared, usage)
	s.emitTaskTokenUsageUpdated(persistCtx, prepared, usage, onEvent)
	if result, handled, budgetErr := s.handleTaskTokenBudgetPending(persistCtx, prepared, answer, usage, nil, onEvent); handled {
		return result, budgetErr
	}
	metadata := preparedResultMetadataForPrepared(prepared, prepared.Message.Metadata, usage)
	if err := s.completePreparedChat(persistCtx, prepared, answer, metadata); err != nil {
		return nil, finalizedRuntimePersistenceError(err)
	}
	s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayload(prepared, metadata), onEvent)
	return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusCompleted}, nil
}

func toolGovernanceApprovalContinuationMessage(event map[string]interface{}) adapter.Message {
	lines := []string{
		"The user approved the pending tool governance request for this same assistant message.",
		"Continue with the approved tool call or the next necessary one, or answer if nothing is left.",
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
		"The rejected action was not executed. This rejection applies only to the rejected governed tool call; do not negate or rewrite earlier successful tool results in the same turn.",
		"User rejection reason:\n" + strings.TrimSpace(req.Reason),
		"Rejected operation summary JSON:\n" + compactJSON(toolGovernanceRejectedOperationSummary(event)),
		"Completed work before this rejection JSON:\n" + compactJSON(toolGovernanceCompletedWorkSummary(message)),
	}, "\n\n")
	if toolGovernanceHasStoppedPlatformShell(message) {
		content += "\n\nPlatform CLI rejection state: the original Shell ran until the business tool requested approval and was stopped. Only the frozen business tool was rejected and not executed. Do not claim the original Shell never started or that another approval is pending. Do not replay the Shell or the rejected operation."
	}
	chatReq := &adapter.ChatRequest{
		Provider: provider,
		Model:    model,
		Stream:   true,
		Messages: []adapter.Message{
			{
				Role:    "system",
				Content: "You are continuing an assistant turn after the user rejected a governed tool call. Do not execute or claim the rejected action. Preserve earlier successful tool results from the same turn as completed facts. Answer in the user's language. Briefly explain that the rejected action was not performed; for Chinese, explicitly say that specific rejected action was \u672a\u6267\u884c. Then report completed, rejected, and still-pending parts separately when the original task had multiple steps, and offer safe alternatives or ask for a safer next step when useful. Do not expose internal IDs, UUIDs, workspace identifiers, correlation values, raw JSON field names, or tool count fields.",
			},
			{Role: "user", Content: content},
		},
	}
	if message != nil {
		applyModelParameters(chatReq, message.ModelParameters)
	}
	return chatReq
}

func toolGovernanceHasStoppedPlatformShell(message *runtimemodel.Message) bool {
	if message == nil {
		return false
	}
	for _, trace := range mapSliceFromAny(message.Metadata["skill_invocations"]) {
		if trace["skill_id"] == "agent_execution" && trace["tool_name"] == "shell_exec" && trace["status"] == "awaiting_approval" {
			return true
		}
	}
	return false
}

func toolGovernanceCompletedWorkSummary(message *runtimemodel.Message) map[string]interface{} {
	summary := map[string]interface{}{}
	if message == nil || len(message.Metadata) == 0 {
		return summary
	}
	if resultSummary := mapFromOperationContext(message.Metadata["operation_result_summary"]); len(resultSummary) > 0 {
		summary["operation_result_summary"] = compactOperationResultSummaryForGovernanceRejection(resultSummary)
	}
	steps := toolGovernanceCompletedWorkSteps(message.Metadata)
	if len(steps) > 0 {
		summary["completed_steps"] = mapsToInterfaceSlice(steps)
	}
	if len(summary) == 0 {
		return map[string]interface{}{"completed_steps": []interface{}{}}
	}
	return summary
}

func compactOperationResultSummaryForGovernanceRejection(source map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for _, key := range []string{
		"status",
		"plan_status",
		"pending_next_action",
		"current_page",
		"tool_result_count",
		"client_action_count",
	} {
		if value, ok := source[key]; ok && strings.TrimSpace(stringFromAny(value)) != "" {
			out[key] = value
		}
	}
	if latest := mapFromOperationContext(source["latest_tool_result"]); len(latest) > 0 {
		out["latest_tool_result"] = compactGovernanceRejectionStep(latest)
	}
	return out
}

func toolGovernanceCompletedWorkSteps(metadata map[string]interface{}) []map[string]interface{} {
	turnState := mapFromOperationContext(metadata["turn_state"])
	steps := mapSliceFromAny(turnState["steps"])
	if len(steps) == 0 {
		steps = skillInvocationsFromMetadata(metadata["skill_invocations"])
	}
	out := make([]map[string]interface{}, 0, min(len(steps), 12))
	for _, step := range steps {
		status := strings.ToLower(strings.TrimSpace(stringFromAny(step["status"])))
		if status != "success" && status != "succeeded" && status != "completed" {
			continue
		}
		kind := strings.TrimSpace(stringFromAny(step["kind"]))
		if kind != "tool_call" && kind != "client_action" {
			continue
		}
		out = append(out, compactGovernanceRejectionStep(step))
		if len(out) >= 12 {
			break
		}
	}
	return out
}

func compactGovernanceRejectionStep(step map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for _, key := range []string{"kind", "status", "skill_id", "tool_name", "action_type"} {
		if value := strings.TrimSpace(stringFromAny(step[key])); value != "" {
			out[key] = value
		}
	}
	if target := mapFromOperationContext(step["target"]); len(target) > 0 {
		targetOut := map[string]interface{}{}
		for _, key := range []string{"name", "asset_type", "type"} {
			if value := strings.TrimSpace(stringFromAny(target[key])); value != "" {
				targetOut[key] = value
			}
		}
		if len(targetOut) > 0 {
			out["target"] = targetOut
		}
	}
	return out
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
