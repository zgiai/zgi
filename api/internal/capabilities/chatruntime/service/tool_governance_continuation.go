package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

	s.resetStreamEventsBestEffort(ctx, message.ID)
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
	s.finalizePreparedError(ctx, &PreparedChat{
		Conversation: continuation.Conversation,
		Message:      continuation.Message,
	}, cause, onEvent)
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
	if message.Status != runtimemodel.MessageStatusWaitingApproval && message.Status != runtimemodel.MessageStatusStreaming {
		return nil, fmt.Errorf("%w: message is not waiting for tool governance approval", ErrInvalidInput)
	}
	if s.repos.DB == nil {
		return &ToolGovernanceContinuation{Conversation: conversation, Message: message, Event: event}, nil
	}
	err = s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepos := repository.NewRepositories(tx)
		if err := txRepos.Conversation.StartStreaming(ctx, conversation.ID, scope.OrganizationID, scope.AccountID, message.ID); err != nil {
			return err
		}
		return tx.Model(&runtimemodel.Message{}).
			Where("id = ? AND deleted_at IS NULL AND status = ?", message.ID, runtimemodel.MessageStatusWaitingApproval).
			Updates(map[string]interface{}{"status": runtimemodel.MessageStatusStreaming, "error": nil}).Error
	})
	if err != nil {
		return nil, mapRepoError(err)
	}
	conversation.RuntimeStatus = runtimemodel.ConversationRuntimeStatusStreaming
	conversation.ActiveMessageID = &message.ID
	message.Status = runtimemodel.MessageStatusStreaming
	return &ToolGovernanceContinuation{Conversation: conversation, Message: message, Event: event}, nil
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
	answer, usage, err := s.runPreparedSkillStreamWithFinalAnswerGuard(ctx, context.WithoutCancel(ctx), prepared, nil, onEvent, toolGovernanceApprovedFinalAnswerGuard(event))
	if err != nil {
		var pendingGovernance *skillloop.ToolGovernancePendingError
		if errors.As(err, &pendingGovernance) {
			metadata := s.persistToolGovernanceApprovalPending(context.WithoutCancel(ctx), prepared, pendingGovernance.Payload, usage)
			s.emitPreparedEvent(context.WithoutCancel(ctx), prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingApproval), onEvent)
			return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusWaitingApproval}, nil
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
	if s.skillRuntime == nil {
		return nil, true, fmt.Errorf("%w: skill runtime is not configured", ErrInvalidInput)
	}
	if prepared.parts == nil {
		return nil, true, fmt.Errorf("%w: prepared chat parts are required", ErrInvalidInput)
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
			timeline.RecordEvent(streamEventToolGovernanceDecision, toolGovernanceDecisionPayload(prepared, invocation.Trace))
			if invocation.Trace.Governance.Status != toolgovernance.DecisionStatusAllowed {
				return nil, true, fmt.Errorf("%w: frozen invocation was not allowed after approval", ErrInvalidInput)
			}
		}
		if executionErr != nil {
			invocation = recoverableFrozenInvocationFailure(invocation, frozen, args, callID, executionErr)
			timeline.RecordInvocationError(invocation.Trace)
		} else {
			timeline.RecordInvocationEnd(invocation.Trace)
			for _, artifact := range skillArtifactsFromToolMessages(prepared, invocation.Trace, invocation.Messages) {
				s.persistGeneratedArtifactBestEffort(persistCtx, prepared, artifact)
				timeline.Emit(streamEventSkillArtifactCreated, artifact)
			}
		}
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
	toolResult := map[string]interface{}{}
	if invocation != nil {
		toolResult["trace"] = invocation.Trace
		toolResult["messages"] = invocation.Messages
		toolResult["tool_message"] = invocation.ToolMessage.Content
	}
	outcome := "The user approved the pending governed tool call, and the runtime has already executed the frozen invocation exactly once."
	systemPrompt := "You are continuing an AIChat turn after a governed tool call was approved and executed by runtime. Do not call tools. Answer in the user's language. State the actual outcome based only on the executed tool result, and mention any error or limitation plainly."
	if executionErr != nil {
		outcome = "The user approved the pending governed tool call, and the runtime attempted to execute the frozen invocation exactly once, but it failed."
		systemPrompt = "You are continuing an AIChat turn after a governed tool call was approved and attempted by runtime. Do not call tools. Answer in the user's language. Treat the tool result and execution error as authoritative runtime feedback, explain the failure plainly, and decide the next safe step. Do not claim the operation succeeded."
		toolResult["execution_error"] = executionErr.Error()
	}
	contentParts := []string{
		"Original user request:\n" + userQuery,
		outcome,
		"Approved governance event JSON:\n" + compactJSON(event),
		"Executed tool result JSON:\n" + compactJSON(toolResult),
	}
	if executionErr != nil {
		contentParts = append(contentParts, "Execution error:\n"+executionErr.Error())
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
	}
	lines = append(lines, toolGovernanceApprovedAssetInstructions(event)...)
	lines = append(lines, "Approved governance event JSON: "+compactJSON(event))
	return adapter.Message{Role: "system", Content: strings.Join(lines, "\n")}
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
		id := strings.TrimSpace(stringFromAny(asset["id"]))
		name := strings.TrimSpace(firstNonEmptyString(
			stringFromAny(asset["name"]),
			stringFromAny(asset["title"]),
			stringFromAny(asset["file_name"]),
		))
		assetType := strings.TrimSpace(stringFromAny(asset["type"]))
		parts := make([]string, 0, 3)
		if name != "" {
			parts = append(parts, name)
		}
		if assetType != "" {
			parts = append(parts, "type="+assetType)
		}
		if id != "" {
			parts = append(parts, "id="+id)
		}
		if len(parts) > 0 {
			assetSummaries = append(assetSummaries, strings.Join(parts, " "))
		}
	}
	lines := []string{}
	if len(assetSummaries) > 0 {
		lines = append(lines, "Approved assets: "+strings.Join(assetSummaries, "; "))
	}
	skillID, toolName, toolID := toolGovernanceApprovedToolRef(event, approvalEvent)
	if skillID != "" && toolName != "" {
		targetIDInstruction := "approved asset ids as the target identifiers required by the tool"
		if len(assets) == 1 {
			targetIDInstruction = "the approved asset id as the target identifier required by the tool"
		}
		if toolID == "file.delete" && skillID == "file-reader" && len(assets) == 1 {
			targetIDInstruction = "file_id equal to the approved file asset id"
		}
		lines = append(lines, "For this approved governed operation, call "+skillID+"/"+toolName+" with "+targetIDInstruction+" before answering.")
	}
	return lines
}

func toolGovernanceApprovedFinalAnswerGuard(event map[string]interface{}) skillloop.FinalAnswerGuard {
	approvalEvent := toolGovernanceApprovalEventFromEvent(event)
	if len(approvalEvent) == 0 {
		return nil
	}
	skillID, toolName, _ := toolGovernanceApprovedToolRef(event, approvalEvent)
	if skillID == "" || toolName == "" {
		return nil
	}
	targetSummary := approvedGovernanceAssetSummary(approvalEvent, event)
	return func(req skillloop.FinalAnswerGuardRequest) (skillloop.FinalAnswerGuardResult, bool) {
		if finalAnswerGuardHasSuccessfulTool(req, skillID, toolName) ||
			finalAnswerGuardHasAttemptedTool(req, skillID, toolName) {
			return skillloop.FinalAnswerGuardResult{}, false
		}
		return skillloop.FinalAnswerGuardResult{
			SkillID:  skillID,
			ToolName: toolName,
			Message: strings.Join([]string{
				"The user approved the pending governed tool call, but approval is not the operation itself.",
				"Before producing a final answer, retry the approved call with call_skill_tool using skill_id \"" + skillID + "\" and tool_name \"" + toolName + "\".",
				"Use the approved asset id for " + targetSummary + " when the tool needs a target.",
				"Only after " + toolName + " is attempted in this continuation may you report the actual outcome.",
			}, " "),
		}, true
	}
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
	if toolName == "" && toolID == "file.delete" && skillID == "file-reader" {
		toolName = "delete_file"
	}
	return skillID, toolName, toolID
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
	id := strings.TrimSpace(stringFromAny(asset["id"]))
	switch {
	case name != "" && id != "":
		return name + " (" + id + ")"
	case name != "":
		return name
	case id != "":
		return id
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
		"User rejection reason:\n" + strings.TrimSpace(req.Reason),
		"Rejected governance event JSON:\n" + compactJSON(event),
	}, "\n\n")
	chatReq := &adapter.ChatRequest{
		Provider: provider,
		Model:    model,
		Stream:   true,
		Messages: []adapter.Message{
			{
				Role:    "system",
				Content: "You are continuing an AIChat turn after the user rejected a governed tool call. Do not execute or claim the rejected action. Briefly explain that the action was not performed, then offer safe alternatives or ask for a safer next step when useful.",
			},
			{Role: "user", Content: content},
		},
	}
	if message != nil {
		applyModelParameters(chatReq, message.ModelParameters)
	}
	return chatReq
}

func compactJSON(value interface{}) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}
