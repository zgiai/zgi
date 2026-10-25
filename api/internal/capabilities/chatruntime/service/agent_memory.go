package service

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/agentmemoryruntime"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/relationshipmemory"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const agentMemoryToolMutate = agentmemoryruntime.ToolMutate
const maxUserRelationshipMemoryContextChars = 4000

// minMemoryBlocksContextChars is the least room memory blocks (ADR 0013)
// get; they get as much as all blocks can hold, within 10% of the model's
// input, so a model with room never sees a block cut.
const minMemoryBlocksContextChars = 8000

// memoryBlocksContextChars is the room all enabled blocks need when full:
// their limits plus the file framing and descriptions around them.
func memoryBlocksContextChars(slots []AgentMemorySlotConfig) int {
	total := 600
	for _, slot := range slots {
		total += slot.MaxChars + utf8.RuneCountInString(slot.Description) + 160
	}
	if total < minMemoryBlocksContextChars {
		return minMemoryBlocksContextChars
	}
	return total
}

func (s *service) appendAgentMemoryContext(ctx context.Context, scope Scope, parts *chatRequestParts, systemPrompt string, modelInputLimit int) (string, map[string]interface{}, error) {
	if parts == nil || !parts.AgentMemoryEnabled || len(enabledAgentMemorySlots(parts.AgentMemorySlots)) == 0 {
		return systemPrompt, nil, nil
	}
	if relationshipmemory.RuntimeEnabled() {
		identity := agentRelationshipMemoryIdentity(scope, parts)
		decision := relationshipmemory.DecideAccess(identity)
		relationshipmemory.RecordAccessDecision(ctx, relationshipmemory.ConsumerAgent, identity, decision)
		if !decision.Read {
			// Fail closed for every downstream memory path. The inline tools and
			// extraction scheduler both use these resolved per-invocation flags.
			parts.AgentMemoryEnabled = false
			parts.AgentMemoryToolsEnabled = false
			parts.AgentMemoryAutoExtractionEnabled = false
			return systemPrompt, map[string]interface{}{
				"agent_memory": map[string]interface{}{
					"enabled": false, "available": false, "injected": false,
					"context_status": "policy_denied", "reason": decision.Reason,
				},
			}, nil
		}
	}
	agentID, _ := uuid.Parse(strings.TrimSpace(parts.AgentMemoryAgentID))
	workspaceID := uuid.Nil
	if resourceWorkspaceID := scope.AgentResourceWorkspaceID(); resourceWorkspaceID != nil {
		workspaceID = *resourceWorkspaceID
	}
	blocks := relationshipmemory.MemoryBlocksEnabled()
	budgetTokens := maxAgentMemoryContextTokens
	if blocks && modelInputLimit > 0 {
		budgetTokens = modelInputLimit / 10
	} else if modelInputLimit > 0 && modelInputLimit/10 < budgetTokens {
		budgetTokens = modelInputLimit / 10
	}
	if budgetTokens <= 0 {
		return systemPrompt, nil, nil
	}
	maxChars := maxUserRelationshipMemoryContextChars
	if blocks {
		maxChars = memoryBlocksContextChars(enabledAgentMemorySlots(parts.AgentMemorySlots))
	}
	budgetChars := budgetTokens * 4
	if budgetChars > maxChars {
		budgetChars = maxChars
	}
	result, err := agentmemoryruntime.BuildContext(ctx, agentmemoryruntime.ContextRequest{
		SystemPrompt: systemPrompt, Enabled: parts.AgentMemoryEnabled,
		Slots: enabledAgentMemorySlots(parts.AgentMemorySlots), MemoryService: s.agentMemoryService,
		WorkspaceID: workspaceID, AgentID: agentID, UserID: agentMemoryUserID(scope),
		UserScope: parts.AgentMemoryUserScope, ConfigScope: parts.AgentMemoryConfigScope,
		ConfigRevision: parts.AgentMemoryConfigRevision, Budget: budgetChars, Blocks: blocks,
	})
	if strings.TrimSpace(result.Context) != "" && result.State != nil && s.tokenEstimator != nil {
		bestContext := ""
		var bestValues []map[string]interface{}
		low, high := 1, budgetChars
		for low <= high {
			mid := low + (high-low)/2
			candidate, values := agentmemoryruntime.Render(result.State.SavedValues, mid, blocks)
			tokens := s.tokenEstimator.EstimateMessages([]adapter.Message{{Role: "user", Content: candidate}}, parts.ModelName).Tokens
			if strings.TrimSpace(candidate) != "" && tokens <= budgetTokens {
				bestContext, bestValues = candidate, values
				low = mid + 1
			} else {
				high = mid - 1
			}
		}
		result.Context = bestContext
		if memoryMetadata, ok := result.Metadata["agent_memory"].(map[string]interface{}); ok {
			memoryMetadata["available"] = len(bestValues) > 0
			memoryMetadata["injected"] = strings.TrimSpace(bestContext) != ""
			memoryMetadata["value_count"] = len(bestValues)
			memoryMetadata["values"] = bestValues
			memoryMetadata["budget_tokens"] = budgetTokens
		}
	}
	if result.State != nil {
		parts.AgentMemoryRuntimeState = result.State
	}
	parts.AgentMemoryContext = strings.TrimSpace(result.Context)
	if relationshipmemory.RuntimeEnabled() {
		injectedCount := 0
		if memoryMetadata, ok := result.Metadata["agent_memory"].(map[string]interface{}); ok {
			if value, ok := memoryMetadata["value_count"].(int); ok {
				injectedCount = value
			}
		}
		relationshipmemory.RecordContextInjection(ctx, relationshipmemory.ConsumerAgent, parts.AgentMemoryContext, injectedCount)
	}
	return result.SystemPrompt, result.Metadata, err
}

func agentRelationshipMemoryIdentity(scope Scope, parts *chatRequestParts) relationshipmemory.InvocationIdentity {
	identity := relationshipmemory.InvocationIdentity{
		Principal:        relationshipmemory.PrincipalRef{ID: scope.AccountID},
		ConversationType: relationshipmemory.ConversationPrivate,
	}
	if parts == nil {
		return identity
	}
	switch strings.TrimSpace(parts.AgentMemoryUserScope) {
	case agentmemory.UserScopeAccount:
		identity.Principal.Type = "account"
		identity.Surface = relationshipmemory.SurfaceAgentConsole
		if strings.TrimSpace(parts.AgentMemoryConfigScope) == agentmemory.ConfigScopePublished {
			identity.Surface = relationshipmemory.SurfaceAgentWebApp
		}
		if scope.AccountID != uuid.Nil {
			subject := usersubject.AccountSubject(scope.AccountID)
			identity.UserSubject = &subject
		}
	case agentmemory.UserScopeEndUser:
		identity.Principal.Type = "agent_api_key"
		identity.Surface = relationshipmemory.SurfaceAgentAPI
		// An API end-user is valid only when the API edge explicitly resolved
		// `user`; falling back to the API key owner's account ID is forbidden.
		if scope.AgentMemoryUserID != nil && *scope.AgentMemoryUserID != uuid.Nil {
			subject := usersubject.Ref{Kind: usersubject.APIEndUser, ID: *scope.AgentMemoryUserID}
			identity.UserSubject = &subject
		}
	}
	return identity
}

func renderAgentMemoryContext(values []agentmemory.SlotValueResponse, budget int) (string, int) {
	return agentmemoryruntime.RenderContext(values, budget)
}

func agentMemoryContextMessage(parts *chatRequestParts) *adapter.Message {
	if parts == nil || strings.TrimSpace(parts.AgentMemoryContext) == "" {
		return nil
	}
	message := adapter.Message{Role: "user", Content: strings.TrimSpace(parts.AgentMemoryContext)}
	return &message
}

func (s *service) scheduleAgentMemoryExtractionBestEffort(ctx context.Context, prepared *PreparedChat) {
	if s == nil || s.agentMemoryExtractionScheduler == nil || prepared == nil || prepared.parts == nil || prepared.Message == nil || prepared.Conversation == nil {
		return
	}
	if !prepared.parts.AgentMemoryEnabled || !prepared.parts.AgentMemoryAutoExtractionEnabled || !globalAgentMemoryAutoExtractionEnabled() {
		return
	}
	if state := prepared.parts.AgentMemoryRuntimeState; state != nil && state.RelationshipSettingsResolved && (!state.Enabled || !state.AutoCaptureEnabled) {
		return
	}
	resourceWorkspaceID := prepared.Scope.AgentResourceWorkspaceID()
	if resourceWorkspaceID == nil || *resourceWorkspaceID == uuid.Nil || agentMemoryUserID(prepared.Scope) == uuid.Nil {
		return
	}
	agentID, err := uuid.Parse(strings.TrimSpace(prepared.parts.AgentMemoryAgentID))
	if err != nil || agentID == uuid.Nil {
		return
	}
	_, _ = s.agentMemoryExtractionScheduler.ScheduleExtraction(ctx, agentmemory.ScheduleExtractionRequest{
		WorkspaceID: resourceWorkspaceID.String(), AgentID: agentID.String(),
		UserScope: prepared.parts.AgentMemoryUserScope, UserID: agentMemoryUserID(prepared.Scope).String(),
		ConversationID: prepared.Conversation.ID.String(), MessageWatermarkID: prepared.Message.ID.String(),
		ExtractorVersion: "agent-memory-v2", ConfigScope: prepared.parts.AgentMemoryConfigScope,
		ConfigRevision: prepared.parts.AgentMemoryConfigRevision, Slots: agentMemoryRuntimeSlots(prepared.parts.AgentMemorySlots),
	})
}

func globalAgentMemoryAutoExtractionEnabled() bool {
	return relationshipmemory.AutomaticExtractionEnabled()
}

func globalAgentMemoryInlineToolsEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("ZGI_AGENT_MEMORY_INLINE_TOOLS_ENABLED")))
	return value == "" || value == "1" || value == "true" || value == "yes" || value == "on"
}

func agentMemoryRuntimeSlots(input []AgentMemorySlotConfig) []agentmemory.RuntimeSlot {
	return agentmemoryruntime.RuntimeSlots(enabledAgentMemorySlots(input))
}

func enabledAgentMemorySlots(input []AgentMemorySlotConfig) []AgentMemorySlotConfig {
	normalized := normalizeAgentMemorySlots(input)
	out := make([]AgentMemorySlotConfig, 0, len(normalized))
	for _, slot := range normalized {
		if slot.Enabled {
			out = append(out, slot)
		}
	}
	return out
}

func agentMemoryUserID(scope Scope) uuid.UUID {
	if scope.AgentMemoryUserID != nil && *scope.AgentMemoryUserID != uuid.Nil {
		return *scope.AgentMemoryUserID
	}
	return scope.AccountID
}

func agentMemoryMutationMetadata(prepared *PreparedChat) agentmemory.MutationMetadata {
	meta := agentmemory.MutationMetadata{ActorType: agentmemory.EventActorModel, Source: agentmemory.EventSourceAgent}
	if prepared != nil && prepared.parts != nil && prepared.parts.AgentMemoryRuntimeState != nil {
		state := prepared.parts.AgentMemoryRuntimeState
		if state.MemoryEpoch != nil {
			epoch := *state.MemoryEpoch
			meta.MemoryEpoch = &epoch
		}
		meta.ConfigScope = state.ConfigScope
		meta.ConfigRevision = state.ConfigRevision
	}
	if prepared != nil && prepared.Conversation != nil {
		id := prepared.Conversation.ID
		meta.SourceConversationID = &id
	}
	if prepared != nil && prepared.Message != nil {
		id := prepared.Message.ID
		meta.SourceMessageID = &id
		completedAt := time.Now()
		meta.SourceCompletedAt = &completedAt
	}
	return meta
}

func agentMemoryRuntimeAvailable(prepared *PreparedChat, memoryService AgentMemoryContextService) bool {
	if prepared == nil || prepared.parts == nil || memoryService == nil || !globalAgentMemoryInlineToolsEnabled() {
		return false
	}
	parts := prepared.parts
	if !parts.AgentMemoryEnabled || !parts.AgentMemoryToolsEnabled || len(enabledAgentMemorySlots(parts.AgentMemorySlots)) == 0 {
		return false
	}
	if prepared.Scope.AgentResourceWorkspaceID() == nil || *prepared.Scope.AgentResourceWorkspaceID() == uuid.Nil || agentMemoryUserID(prepared.Scope) == uuid.Nil {
		return false
	}
	agentID, err := uuid.Parse(strings.TrimSpace(parts.AgentMemoryAgentID))
	return err == nil && agentID != uuid.Nil
}

func (s *service) agentMemoryRuntimeTools(persistCtx context.Context, prepared *PreparedChat, timeline *processTimelineRecorder) []skillloop.RuntimeTool {
	if !agentMemoryRuntimeAvailable(prepared, s.agentMemoryService) {
		return nil
	}
	parts := prepared.parts
	state := parts.AgentMemoryRuntimeState
	if state == nil {
		state = &AgentMemoryRuntimeState{
			Enabled: true, AutoCaptureEnabled: true, UserScope: parts.AgentMemoryUserScope,
			ConfigScope: parts.AgentMemoryConfigScope, ConfigRevision: parts.AgentMemoryConfigRevision,
			EnabledSlots: enabledAgentMemorySlots(parts.AgentMemorySlots),
		}
		parts.AgentMemoryRuntimeState = state
	}
	if state.RelationshipSettingsResolved && !state.Enabled {
		return nil
	}
	if state.ConfigScope == "" {
		state.ConfigScope = parts.AgentMemoryConfigScope
	}
	if state.ConfigRevision == "" {
		state.ConfigRevision = parts.AgentMemoryConfigRevision
	}
	if len(state.EnabledSlots) == 0 {
		state.EnabledSlots = enabledAgentMemorySlots(parts.AgentMemorySlots)
	}
	agentID, _ := uuid.Parse(strings.TrimSpace(parts.AgentMemoryAgentID))
	state.AgentID = agentID
	relationshipAutoCaptureAllowed := true
	if state.RelationshipSettingsResolved {
		relationshipAutoCaptureAllowed = state.AutoCaptureEnabled
	}
	proactiveAllowed := parts.AgentMemoryAutoExtractionEnabled && relationshipAutoCaptureAllowed && globalAgentMemoryAutoExtractionEnabled()
	blocks := relationshipmemory.MemoryBlocksEnabled()
	var definitions []adapter.Tool
	if blocks {
		definitions = agentmemoryruntime.FileTools(state.EnabledSlots, proactiveAllowed)
	} else {
		definitions = agentmemoryruntime.Tools(state.EnabledSlots, proactiveAllowed)
	}
	if len(definitions) == 0 {
		return nil
	}
	validationFailures := 0
	succeeded := false
	terminalFailure := false
	partialCalls := 0
	views := 0
	handler := func(ctx context.Context, call adapter.ToolCall) skillloop.RuntimeToolResult {
		if blocks && call.Function.Name == agentmemoryruntime.ToolMemoryView {
			views++
			if views > 5 {
				return memoryRuntimeFailure("view_limit", false)
			}
			result := agentmemoryruntime.ExecuteView(agentmemoryruntime.MutationRequest{Slots: state.EnabledSlots, CurrentValues: state.SavedValues}, call.Function.Arguments)
			return skillloop.RuntimeToolResult{Status: fmt.Sprint(result["status"]), Arguments: map[string]interface{}{}, Result: result}
		}
		if succeeded {
			return memoryRuntimeFailure("already_mutated", false)
		}
		if terminalFailure || validationFailures > 1 {
			// The model gets one correction attempt after a side-effect-free
			// validation failure. Further attempts remain harmless and final.
			return memoryRuntimeFailure("retry_exhausted", false)
		}
		resourceWorkspaceID := prepared.Scope.AgentResourceWorkspaceID()
		if resourceWorkspaceID == nil || *resourceWorkspaceID == uuid.Nil {
			return memoryRuntimeFailure("workspace_missing", false)
		}
		workspaceID := *resourceWorkspaceID
		mutationCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		request := agentmemoryruntime.MutationRequest{
			MemoryService: s.agentMemoryService, WorkspaceID: workspaceID, AgentID: agentID,
			UserID: agentMemoryUserID(prepared.Scope), UserScope: parts.AgentMemoryUserScope,
			Slots: state.EnabledSlots, CurrentValues: state.SavedValues,
			MutationMetadata: agentMemoryMutationMetadata(prepared), LatestUserMessage: parts.Query,
			ProactiveAllowed: proactiveAllowed, RecentUserMessages: parts.AgentMemoryRecentUserMessages, CallID: call.ID,
		}
		var result agentmemoryruntime.MutationResult
		if blocks {
			result = agentmemoryruntime.ExecuteEdits(mutationCtx, request, call.Function.Arguments)
		} else {
			result = agentmemoryruntime.Execute(mutationCtx, request, call.Function.Arguments)
		}
		if result.Error != nil {
			if agentmemoryruntime.ValidationError(result.Error) {
				validationFailures++
				retryAllowed := validationFailures == 1
				terminalFailure = !retryAllowed
				result.Result["retry_allowed"] = retryAllowed
				return skillloop.RuntimeToolResult{Status: "failed", Arguments: result.Arguments, Result: result.Result, Error: result.Error, Recoverable: retryAllowed}
			}
			terminalFailure = true
			result.Result["retry_allowed"] = false
			return skillloop.RuntimeToolResult{Status: "failed", Arguments: result.Arguments, Result: result.Result, Error: result.Error}
		}
		// Under memory blocks, a call whose edits were partly rejected may be
		// followed by one more call that corrects them.
		if blocks && editsRejected(result.Result) && partialCalls == 0 {
			partialCalls++
		} else {
			succeeded = true
		}
		s.recordAgentMemoryMutationEvents(ctx, prepared, result.Response, timeline)
		s.updateAgentMemoryRuntimeMetadataBestEffort(persistCtx, prepared, map[string]interface{}{
			"inline_status": "success", "inline_operation_count": len(result.Response.Operations),
		})
		return skillloop.RuntimeToolResult{Status: "success", Arguments: result.Arguments, Result: result.Result}
	}
	tools := make([]skillloop.RuntimeTool, 0, len(definitions))
	for _, definition := range definitions {
		tools = append(tools, skillloop.RuntimeTool{Definition: definition, SkillID: skills.SkillAgentMemory, Handler: handler})
	}
	return tools
}

// agentMemoryEvidenceWindow is how many recent user messages memory_edit
// evidence may quote besides the current one.
const agentMemoryEvidenceWindow = 6

func editsRejected(result map[string]interface{}) bool {
	outcomes, _ := result["edits"].([]agentmemoryruntime.EditOutcome)
	for _, outcome := range outcomes {
		if outcome.Status != "accepted" {
			return true
		}
	}
	return false
}

func hasAgentMemoryRuntimeTool(tools []skillloop.RuntimeTool) bool {
	for _, tool := range tools {
		if tool.SkillID == skills.SkillAgentMemory {
			return true
		}
	}
	return false
}

func recentUserQueries(branch []*runtimemodel.Message, limit int) []string {
	queries := make([]string, 0, limit)
	for index := len(branch) - 1; index >= 0 && len(queries) < limit; index-- {
		if branch[index] != nil && strings.TrimSpace(branch[index].Query) != "" {
			queries = append(queries, branch[index].Query)
		}
	}
	return queries
}

func memoryRuntimeFailure(code string, recoverable bool) skillloop.RuntimeToolResult {
	return skillloop.RuntimeToolResult{
		Status: "failed", Arguments: map[string]interface{}{},
		Result: map[string]interface{}{"status": "failed", "error_code": code, "retry_allowed": recoverable},
		Error:  context.Canceled, Recoverable: recoverable,
	}
}

func (s *service) emitAgentMemoryMutationEvents(ctx context.Context, prepared *PreparedChat, response *agentmemory.MutateValuesResponse, onEvent func(StreamEvent) error) {
	s.forEachAgentMemoryMutationEvent(prepared, response, func(eventType string, payload map[string]interface{}) {
		_ = s.emitPreparedEvent(ctx, prepared, eventType, payload, onEvent)
	})
}

func (s *service) recordAgentMemoryMutationEvents(ctx context.Context, prepared *PreparedChat, response *agentmemory.MutateValuesResponse, timeline *processTimelineRecorder) {
	s.forEachAgentMemoryMutationEvent(prepared, response, func(eventType string, payload map[string]interface{}) {
		if timeline == nil {
			_ = s.emitPreparedEvent(ctx, prepared, eventType, payload, nil)
			return
		}
		if err := timeline.RecordEvent(eventType, payload); err != nil {
			// The memory mutation has already committed. Keep chat and live event
			// delivery independent from a best-effort runtime-log write.
			logger.WarnContext(context.WithoutCancel(ctx), "failed to persist agent memory runtime event",
				"message_id", prepared.Message.ID.String(),
				"event_type", eventType,
				"operation_id", payloadString(payload, "operation_id"),
				err,
			)
			_ = timeline.Emit(eventType, payload)
		}
	})
}

func (s *service) forEachAgentMemoryMutationEvent(prepared *PreparedChat, response *agentmemory.MutateValuesResponse, visit func(string, map[string]interface{})) {
	if response == nil || prepared == nil || prepared.Conversation == nil || prepared.Message == nil {
		return
	}
	if visit == nil {
		return
	}
	for _, operation := range response.Operations {
		if operation.Status == agentmemory.MutationStatusUnchanged {
			continue
		}
		eventType := streamEventMemoryUpdate
		action := "update"
		if operation.Action == agentmemory.MutationActionClear {
			eventType, action = streamEventMemoryClear, "clear"
		}
		payload := map[string]interface{}{
			"conversation_id": prepared.Conversation.ID.String(), "message_id": prepared.Message.ID.String(),
			"memory_scope": "agent", "action": action, "key": operation.Key, "status": "success",
			"mutation_status": operation.Status, "source_kind": operation.SourceKind,
			"operation_id": operation.OperationID, "revision": operation.Revision,
		}
		if displayName := agentMemorySlotDisplayName(prepared.parts, operation.Key); displayName != "" {
			payload["display_name"] = displayName
		}
		if operation.UndoableUntil != nil {
			payload["undoable_until"] = *operation.UndoableUntil
		}
		visit(eventType, payload)
	}
}

func agentMemorySlotDisplayName(parts *chatRequestParts, key string) string {
	if parts == nil {
		return ""
	}
	key = strings.TrimSpace(key)
	for _, slot := range parts.AgentMemorySlots {
		if strings.EqualFold(strings.TrimSpace(slot.Key), key) {
			return strings.TrimSpace(slot.Name)
		}
	}
	return ""
}

func agentMemoryUnavailableSystemMessage(parts *chatRequestParts) *adapter.Message {
	if parts == nil || !parts.AgentMemoryEnabled || len(enabledAgentMemorySlots(parts.AgentMemorySlots)) == 0 || parts.AgentMemoryToolsEnabled {
		return nil
	}
	message := adapter.Message{Role: "system", Content: strings.Join([]string{
		"Agent memory mutation tools are unavailable for this model or temporarily disabled.",
		"Continue the conversation normally, but do not claim that memory was saved, corrected, or deleted.",
		"If the user asks for a memory change, explain that they can use My Memory in the WebApp or the Agent Memory API.",
	}, "\n")}
	return &message
}

func (s *service) updateAgentMemoryRuntimeMetadataBestEffort(ctx context.Context, prepared *PreparedChat, updates map[string]interface{}) {
	if prepared == nil || prepared.Message == nil || len(updates) == 0 {
		return
	}
	metadata := copyStringAnyMap(prepared.Message.Metadata)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	contextControl := map[string]interface{}{}
	if prepared.parts != nil && prepared.parts.ContextControl != nil {
		contextControl = copyStringAnyMap(prepared.parts.ContextControl)
	}
	if existing, ok := metadata["context_control"].(map[string]interface{}); ok {
		for key, value := range existing {
			contextControl[key] = value
		}
	}
	agentMemory := map[string]interface{}{}
	if existing, ok := contextControl["agent_memory"].(map[string]interface{}); ok {
		agentMemory = copyStringAnyMap(existing)
	}
	for key, value := range updates {
		agentMemory[key] = value
	}
	contextControl["agent_memory"] = agentMemory
	metadata["context_control"] = contextControl
	prepared.Message.Metadata = metadata
	if prepared.parts != nil {
		prepared.parts.ContextControl = contextControl
	}
	if s != nil && s.repos != nil && s.repos.Message != nil {
		_ = s.repos.Message.UpdateMetadata(ctx, prepared.Message.ID, metadata)
	}
}

func truncateNativeAgentMemoryRunes(value string, limit int) string {
	return agentmemoryruntime.TruncateRunes(value, limit)
}
