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
	"github.com/zgiai/zgi/api/internal/modules/skills"
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
	s.persistPendingMessageAndFinishConversationBestEffort(
		ctx,
		prepared,
		"client action",
		func(repo repository.MessageRepository) error {
			return repo.UpdateWaitingClientAction(ctx, prepared.Message.ID, metadata)
		},
		func(repo repository.ConversationRepository) error {
			return repo.FinishContinuationMessage(ctx, prepared.Conversation.ID, prepared.Message.ID)
		},
	)
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
		if IsContinuationAlreadyRunningError(err) {
			if streamErr := s.StreamConversationEvents(ctx, scope, conversationID, messageID, "", onEvent); streamErr != nil {
				return nil, streamErr
			}
			return &ChatResult{Status: runtimemodel.MessageStatusStreaming}, nil
		}
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

	if answer, ok := clientActionContinuationFastPathAnswer(prepared); ok {
		s.emitPreparedEvent(ctx, prepared, streamEventMessage, map[string]interface{}{
			"conversation_id": prepared.Conversation.ID.String(),
			"message_id":      prepared.Message.ID.String(),
			"answer":          answer,
		}, onEvent)
		metadata := preparedResultMetadata(prepared.Message.Metadata, nil)
		prepared.Message.Metadata = metadata
		if err := s.completePreparedChat(context.WithoutCancel(ctx), prepared, answer, metadata); err != nil {
			return nil, err
		}
		s.emitPreparedEvent(context.WithoutCancel(ctx), prepared, streamEventMessageEnd, messageEndPayload(prepared, metadata), onEvent)
		return &ChatResult{Answer: answer, Metadata: metadata, Status: runtimemodel.MessageStatusCompleted}, nil
	}

	var answer string
	var usage *adapter.Usage
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

func clientActionContinuationFastPathAnswer(prepared *PreparedChat) (string, bool) {
	if prepared == nil || prepared.Message == nil || prepared.parts == nil {
		return "", false
	}
	return skillloop.FastPathFinalAnswerForCompletionEvidence(skillLoopCompletionEvidence(prepared)())
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
		return nil, newContinuationAlreadyRunningError("client action continuation is already running; reconnect to the active stream instead of retrying the action")
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
			return newContinuationAlreadyRunningError("client action continuation is already running; reconnect to the active stream instead of retrying the action")
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
	if actionID := clientActionID(continuation.Event); actionID != "" {
		message.Metadata = resolveClientActionContinuationMetadata(message.Metadata, actionID, req)
	}
	injectClientActionContinuationContext(parts, continuation.Event, req, message.Metadata)
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
	ensureClientActionContinuationSkill(parts, continuation.Event)
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

func injectClientActionContinuationContext(parts *chatRequestParts, event map[string]interface{}, req runtimedto.ClientActionResultRequest, metadata map[string]interface{}) {
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
	appendClientActionContinuationResources(parts.RawOperationContext, record)
	appendClientActionContinuationResources(parts.OperationContext, record)
	if completed := completedClientActionRecordsFromMetadata(metadata); len(completed) > 0 {
		parts.RawOperationContext["completed_client_actions"] = completed
		parts.OperationContext["completed_client_actions"] = completed
	}
}

func appendClientActionContinuationResources(context map[string]interface{}, record map[string]interface{}) {
	if len(context) == 0 || len(record) == 0 {
		return
	}
	result := governanceMapFromAny(record["result"])
	resources := mapSliceFromAny(context["resources"])
	seen := map[string]struct{}{}
	for _, resource := range resources {
		key := clientActionContinuationResourceKey(resource)
		if key != "" {
			seen[key] = struct{}{}
		}
	}
	appendResource := func(resource map[string]interface{}) {
		if len(resource) == 0 {
			return
		}
		key := clientActionContinuationResourceKey(resource)
		if key != "" {
			if _, ok := seen[key]; ok {
				return
			}
			seen[key] = struct{}{}
		}
		resources = append(resources, resource)
	}
	for _, item := range mapSliceFromAny(result["context_items"]) {
		appendResource(clientActionContextItemResource(item))
	}
	if strings.TrimSpace(stringFromAny(record["action_type"])) == "route_navigation" {
		appendResource(clientActionRouteResource(record))
	}
	if len(resources) > 0 {
		context["resources"] = mapsToInterfaceSlice(resources)
	}
}

func clientActionContinuationResourceKey(resource map[string]interface{}) string {
	if len(resource) == 0 {
		return ""
	}
	metadata := mapFromOperationContext(resource["metadata"])
	resourceType := strings.TrimSpace(firstNonEmptyString(resource["resource_type"], resource["type"], metadata["resource_type"], metadata["type"]))
	resourceID := strings.TrimSpace(firstNonEmptyString(resource["resource_id"], resource["id"], resource["href"], metadata["resource_id"], metadata["id"], metadata["href"], metadata["route"]))
	if resourceType == "" && resourceID == "" {
		return ""
	}
	return resourceType + ":" + resourceID
}

func clientActionContextItemResource(item map[string]interface{}) map[string]interface{} {
	if len(item) == 0 {
		return nil
	}
	resourceType := strings.TrimSpace(firstNonEmptyString(item["resource_type"], item["type"]))
	resourceID := strings.TrimSpace(firstNonEmptyString(item["resource_id"], item["id"]))
	title := strings.TrimSpace(firstNonEmptyString(item["title"], item["name"]))
	if resourceType == "" || (resourceID == "" && title == "") {
		return nil
	}
	resource := map[string]interface{}{
		"resource_type": resourceType,
	}
	if resourceID != "" {
		resource["resource_id"] = resourceID
	}
	if title != "" {
		resource["title"] = title
	}
	if href := normalizeConsoleNavigationGuardHref(firstNonEmptyString(item["href"], item["route"], item["observed_path"], item["loaded_href"])); href != "" {
		resource["href"] = href
	}
	metadata := map[string]interface{}{}
	for _, key := range []string{
		"context_ready",
		"files_query_status",
		"agents_query_status",
		"visible_file_count",
		"total_file_count",
		"visible_agent_count",
		"loaded_agent_count",
	} {
		if value, ok := item[key]; ok && value != nil {
			metadata[key] = value
		}
	}
	if len(metadata) > 0 {
		resource["metadata"] = metadata
	}
	return resource
}

func clientActionRouteResource(record map[string]interface{}) map[string]interface{} {
	if len(record) == 0 {
		return nil
	}
	result := governanceMapFromAny(record["result"])
	href := normalizeConsoleNavigationGuardHref(firstNonEmptyString(
		record["href"],
		result["href"],
		result["observed_path"],
		result["loaded_href"],
		result["target_page"],
	))
	if href == "" {
		return nil
	}
	resource := map[string]interface{}{
		"resource_id":   href,
		"resource_type": "page",
		"title":         firstNonEmptyString(record["label"], result["label"], href),
		"href":          href,
	}
	metadata := map[string]interface{}{"route": href}
	if ready, ok := result["page_context_ready"]; ok && ready != nil {
		metadata["context_ready"] = ready
	}
	resource["metadata"] = metadata
	return resource
}

func ensureClientActionContinuationSkill(parts *chatRequestParts, event map[string]interface{}) {
	if parts == nil {
		return
	}
	skillID := strings.TrimSpace(stringFromAny(event["skill_id"]))
	if skillID == "" {
		return
	}
	if !skillIDEnabled(parts.SkillIDs, skillID) {
		parts.SkillIDs = append(parts.SkillIDs, skillID)
	}
	if !skillIDEnabled(parts.ConfiguredSkillIDs, skillID) {
		parts.ConfiguredSkillIDs = append(parts.ConfiguredSkillIDs, skillID)
	}
}

func clientActionContinuationMessage(message *runtimemodel.Message, event map[string]interface{}, req runtimedto.ClientActionResultRequest) adapter.Message {
	userQuery := ""
	if message != nil {
		userQuery = strings.TrimSpace(message.Query)
	}
	result := clientActionObservationRecord(event, req)
	contentParts := []string{
		"Original user request:\n" + userQuery,
		"The frontend completed a client-side action for this same AIChat message.",
		"Client action result JSON:\n" + compactJSON(result),
	}
	if preceding := clientActionPrecedingSuccessfulToolInvocation(message, event); len(preceding) > 0 {
		contentParts = append(contentParts, "Current-turn tool result immediately before this frontend action JSON:\n"+compactJSON(preceding))
	}
	if completedActions := completedClientActionsForContinuation(message); len(completedActions) > 0 {
		contentParts = append(contentParts, "Completed client actions in this same AIChat turn JSON:\n"+compactJSON(completedActions))
	}
	if progress := clientActionAgentCreateProgress(message); len(progress) > 0 {
		contentParts = append(contentParts, "Current-turn agent creation progress JSON:\n"+compactJSON(progress))
	}
	content := strings.Join(contentParts, "\n\n")
	system := strings.Join([]string{
		"You are continuing the same AIChat turn after a frontend client action.",
		"Use the updated transient ZGI page context already included in this request.",
		"If the client action status is succeeded and it loaded a route, do not call console-navigator/navigate again for the same route.",
		"Treat completed client actions listed below as authoritative completed steps. Continue from the next unfinished step instead of restarting the original plan or returning to an earlier completed route.",
		"For event_type=route_loaded, phrase route success from the user's point of view, for example that the target page has been opened or switched to.",
		"For event_type=route_already_loaded, say the requested page is already current only when useful, then continue the user's real task from the current page context.",
		"If the client action status is succeeded and observed a resource mutation, use the observation result and updated page context to confirm whether the changed resource is visible; do not repeat the same side-effecting tool only to verify it.",
		"If a current-turn tool result is provided, treat it as authoritative evidence for the current user request. If it completed the requested mutation, summarize that result as completed in this request and do not describe it as a previous round, previous conversation, last turn, earlier request, 上一轮, 上次, 之前, or 先前 unless the user explicitly asks about history.",
		"If Current-turn agent creation progress is present and missing_targets is non-empty, do not give a final completion answer yet. Continue by calling agent-management/create_agent for each exact missing target name. Do not treat a similar visible Agent with a different exact name as satisfying the missing target.",
		"Continue the user's original task from the new page context.",
		"If the client action failed or timed out, treat that as recoverable feedback and decide whether to retry, choose another route, or explain the limitation.",
		"Do not expose internal action ids, message ids, UUIDs, or raw JSON field names in the final user-visible answer.",
	}, " ")
	return adapter.Message{Role: "system", Content: system + "\n\n" + content}
}

func clientActionAgentCreateProgress(message *runtimemodel.Message) map[string]interface{} {
	if message == nil {
		return nil
	}
	goal := strings.TrimSpace(message.Query)
	if goal == "" {
		plan := mapFromOperationContext(metadataValue(message.Metadata, "operation_plan"))
		goal = strings.TrimSpace(stringFromAny(plan["original_user_goal"]))
	}
	if goal == "" || !agentManagementCreateRequested(goal) {
		return nil
	}
	requestedTargets := agentCreateTargetNamesFromText(goal)
	requestedCount := agentManagementCreateRequestedCount(goal)
	completedTargets := agentCreateCompletedTargetNames(message.Metadata)
	if requestedCount <= 1 && len(requestedTargets) <= 1 {
		return nil
	}
	if len(requestedTargets) > 0 && requestedCount < len(requestedTargets) {
		requestedCount = len(requestedTargets)
	}
	missingTargets := missingAgentCreateTargets(requestedTargets, completedTargets)
	if len(requestedTargets) == 0 && requestedCount > len(completedTargets) {
		for index := len(completedTargets); index < requestedCount; index++ {
			missingTargets = append(missingTargets, fmt.Sprintf("target_%d", index+1))
		}
	}
	out := map[string]interface{}{
		"operation":         "agent.create",
		"requested_count":   requestedCount,
		"completed_count":   len(completedTargets),
		"completed_targets": completedTargets,
		"missing_count":     len(missingTargets),
		"missing_targets":   missingTargets,
	}
	if len(requestedTargets) > 0 {
		out["requested_targets"] = requestedTargets
	}
	if description := agentCreateSharedDescriptionFromText(goal); description != "" {
		out["requested_description"] = description
	}
	if len(missingTargets) == 0 && requestedCount > 0 && len(completedTargets) >= requestedCount {
		out["status"] = "completed"
	} else {
		out["status"] = "partial"
	}
	return out
}

func agentCreateCompletedTargetNames(metadata map[string]interface{}) []string {
	calls := successfulMetadataToolCalls(metadata, skills.SkillAgentManagement, "create_agent")
	out := make([]string, 0, len(calls))
	seen := map[string]struct{}{}
	for _, call := range calls {
		name := strings.TrimSpace(firstNonEmptyString(call.Result["agent_name"], call.Result["name"], call.Arguments["name"]))
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, name)
	}
	return out
}

func missingAgentCreateTargets(requested []string, completed []string) []string {
	if len(requested) == 0 {
		return nil
	}
	done := map[string]struct{}{}
	for _, name := range completed {
		name = strings.TrimSpace(name)
		if name != "" {
			done[strings.ToLower(name)] = struct{}{}
		}
	}
	missing := make([]string, 0)
	seen := map[string]struct{}{}
	for _, name := range requested {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if _, ok := done[key]; ok {
			continue
		}
		missing = append(missing, name)
	}
	return missing
}

func agentCreateTargetNamesFromText(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	lower := strings.ToLower(text)
	markers := []string{"名称分别为", "名字分别为", "名称为", "名字为", "named ", "names are "}
	start := -1
	markerLen := 0
	for _, marker := range markers {
		if index := strings.Index(lower, marker); index >= 0 && (start < 0 || index < start) {
			start = index
			markerLen = len(marker)
		}
	}
	if start < 0 {
		return nil
	}
	segment := strings.TrimSpace(text[start+markerLen:])
	stop := len(segment)
	for _, marker := range []string{
		"，描述", ", 描述", "，都写", ", 都写", "，描述都写", ", description", " with description",
		"，不要", ", 不要", "。不要", ". do not", "。完成", ". after", "完成后", "不要导航", "不要打开",
	} {
		if index := strings.Index(strings.ToLower(segment), strings.ToLower(marker)); index >= 0 && index < stop {
			stop = index
		}
	}
	segment = strings.TrimSpace(segment[:stop])
	return splitAgentCreateTargetNames(segment)
}

func splitAgentCreateTargetNames(segment string) []string {
	replacer := strings.NewReplacer(
		"、", ",",
		"，", ",",
		"；", ",",
		";", ",",
		" 和 ", ",",
		" 和", ",",
		"和 ", ",",
		" and ", ",",
		"\n", ",",
	)
	segment = replacer.Replace(segment)
	parts := strings.Split(segment, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		name := strings.Trim(part, " \t\r\n\"'`“”‘’[]()（）{}<>:：.。")
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, name)
	}
	return out
}

func agentCreateSharedDescriptionFromText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	lower := strings.ToLower(text)
	for _, marker := range []string{"描述都写", "描述写成", "描述为", "description is ", "description: "} {
		index := strings.Index(lower, strings.ToLower(marker))
		if index < 0 {
			continue
		}
		segment := strings.TrimSpace(text[index+len(marker):])
		segment = strings.Trim(segment, " \t\r\n:：")
		if segment == "" {
			return ""
		}
		runes := []rune(segment)
		if len(runes) > 0 {
			quote := runes[0]
			endQuote := quote
			if quote == '“' {
				endQuote = '”'
			}
			if quote == '“' || quote == '"' || quote == '\'' || quote == '`' {
				rest := runes[1:]
				for index, char := range rest {
					if char == endQuote {
						return strings.TrimSpace(string(rest[:index]))
					}
				}
			}
		}
		for _, stop := range []string{"。", ".", "，不要", ", do not", "不要", "完成后"} {
			if index := strings.Index(segment, stop); index >= 0 {
				segment = segment[:index]
			}
		}
		return strings.Trim(segment, " \t\r\n\"'`“”‘’")
	}
	return ""
}

func completedClientActionsForContinuation(message *runtimemodel.Message) []map[string]interface{} {
	if message == nil {
		return nil
	}
	return completedClientActionRecordsFromMetadata(message.Metadata)
}

func completedClientActionRecordsFromMetadata(metadata map[string]interface{}) []map[string]interface{} {
	actions := mapSliceFromAny(metadataValue(metadata, "client_actions"))
	if len(actions) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, min(len(actions), 8))
	for _, action := range actions {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(action["status"])), clientActionStatusSucceeded) {
			continue
		}
		item := map[string]interface{}{
			"action_type": stringFromAny(action["action_type"]),
			"status":      clientActionStatusSucceeded,
		}
		for _, key := range []string{"href", "label", "reason", "skill_id", "tool_name", "resolved_at"} {
			if value := strings.TrimSpace(stringFromAny(action[key])); value != "" {
				item[key] = value
			}
		}
		if result := governanceMapFromAny(action["result"]); len(result) > 0 {
			if href := strings.TrimSpace(stringFromAny(result["href"])); href != "" {
				item["href"] = href
			}
			if contextItems := compactClientActionContextItems(result["context_items"]); len(contextItems) > 0 {
				item["context_items"] = contextItems
			}
			if contextItemCount := stringFromAny(result["context_item_count"]); strings.TrimSpace(contextItemCount) != "" {
				item["context_item_count"] = contextItemCount
			}
			if visibleCount := stringFromAny(result["visible_count"]); strings.TrimSpace(visibleCount) != "" {
				item["visible_count"] = visibleCount
			}
		}
		out = append(out, item)
		if len(out) >= 8 {
			break
		}
	}
	return out
}

func compactClientActionContextItems(value interface{}) []map[string]interface{} {
	items := operationItemsFromValue(value)
	if len(items) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, min(len(items), 8))
	for _, item := range items {
		source, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		compact := map[string]interface{}{}
		for _, key := range []string{"type", "title", "id", "href"} {
			if value := strings.TrimSpace(stringFromAny(source[key])); value != "" {
				compact[key] = value
			}
		}
		if len(compact) == 0 {
			continue
		}
		out = append(out, compact)
		if len(out) >= 8 {
			break
		}
	}
	return out
}

func clientActionPrecedingSuccessfulToolInvocation(message *runtimemodel.Message, event map[string]interface{}) map[string]interface{} {
	if message == nil {
		return nil
	}
	invocations := skillInvocationsFromMetadata(metadataValue(message.Metadata, "skill_invocations"))
	if len(invocations) == 0 {
		return nil
	}
	actionID := clientActionID(event)
	var last map[string]interface{}
	for _, invocation := range invocations {
		kind := strings.TrimSpace(stringFromAny(invocation["kind"]))
		if kind == "client_action" && actionID != "" && clientActionID(invocation) == actionID {
			break
		}
		if kind == "tool_call" && strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["status"])), "success") {
			last = invocation
		}
	}
	if len(last) == 0 {
		for index := len(invocations) - 1; index >= 0; index-- {
			invocation := invocations[index]
			if strings.TrimSpace(stringFromAny(invocation["kind"])) != "tool_call" {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["status"])), "success") {
				last = invocation
				break
			}
		}
	}
	if len(last) == 0 {
		return nil
	}
	return compactSkillInvocation(map[string]interface{}{
		"kind":      "tool_call",
		"skill_id":  last["skill_id"],
		"tool_name": last["tool_name"],
		"status":    last["status"],
		"result":    governanceMapFromAny(last["result"]),
	})
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
	prepared := &PreparedChat{
		Conversation: continuation.Conversation,
		Message:      continuation.Message,
	}
	s.finalizePreparedError(ctx, prepared, cause, onEvent)
	s.emitPreparedEvent(ctx, prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, continuation.Message.Metadata, runtimemodel.MessageStatusError), onEvent)
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

	invocations := sanitizeSkillInvocationsForMetadata(skillInvocationsFromMetadata(metadata["skill_invocations"]))
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
	applyOperationPlanInvocationState(metadata, invocations)
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
