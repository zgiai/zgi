package service

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

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

// memoryFilesContextChars is the room the user's memory files need when
// full (ADR 0017): their total limit plus the file markup.
const memoryFilesContextChars = agentmemory.MaxMemoryChars + 2000

func (s *service) appendAgentMemoryContext(ctx context.Context, scope Scope, parts *chatRequestParts, systemPrompt string, modelInputLimit int) (string, map[string]interface{}, error) {
	if parts == nil || !parts.AgentMemoryEnabled {
		return systemPrompt, nil, nil
	}
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
	agentID, _ := uuid.Parse(strings.TrimSpace(parts.AgentMemoryAgentID))
	workspaceID, _ := agentMemoryWorkspaceID(scope, parts)
	budgetTokens := maxAgentMemoryContextTokens
	if modelInputLimit > 0 {
		budgetTokens = modelInputLimit / 10
	}
	if budgetTokens <= 0 {
		return systemPrompt, nil, nil
	}
	maxChars := memoryFilesContextChars
	budgetChars := budgetTokens * 4
	if budgetChars > maxChars {
		budgetChars = maxChars
	}
	result, err := agentmemoryruntime.BuildContext(ctx, agentmemoryruntime.ContextRequest{
		SystemPrompt: systemPrompt, Enabled: parts.AgentMemoryEnabled,
		Slots: enabledAgentMemorySlots(parts.AgentMemorySlots), MemoryService: s.agentMemoryService,
		WorkspaceID: workspaceID, Personal: isAIChatMemory(parts), AgentID: agentID, UserID: agentMemoryUserID(scope),
		UserScope: parts.AgentMemoryUserScope, ConfigScope: parts.AgentMemoryConfigScope,
		ConfigRevision: parts.AgentMemoryConfigRevision, Budget: budgetChars,
	})
	if strings.TrimSpace(result.Context) != "" && result.State != nil && s.tokenEstimator != nil {
		bestContext := ""
		var bestValues []map[string]interface{}
		low, high := 1, budgetChars
		for low <= high {
			mid := low + (high-low)/2
			candidate, values := agentmemoryruntime.Render(result.State.SavedValues, mid)
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
	injectedCount := 0
	if memoryMetadata, ok := result.Metadata["agent_memory"].(map[string]interface{}); ok {
		if value, ok := memoryMetadata["value_count"].(int); ok {
			injectedCount = value
		}
	}
	relationshipmemory.RecordContextInjection(ctx, relationshipmemory.ConsumerAgent, parts.AgentMemoryContext, injectedCount)
	return result.SystemPrompt, result.Metadata, err
}

// isAIChatMemory reports whether a run uses the person's AI chat memory,
// which goes through the Agent memory path with the AI chat consumer
// (ADR 0021).
func isAIChatMemory(parts *chatRequestParts) bool {
	return parts != nil && strings.TrimSpace(parts.AgentMemoryAgentID) == relationshipmemory.AIChatConsumerID.String()
}

// agentMemoryWorkspaceID is the workspace of the memory a run uses: the
// Agent's, or none for AI chat memory, which belongs to no organization.
func agentMemoryWorkspaceID(scope Scope, parts *chatRequestParts) (uuid.UUID, bool) {
	if isAIChatMemory(parts) {
		return uuid.Nil, true
	}
	if workspaceID := scope.AgentResourceWorkspaceID(); workspaceID != nil && *workspaceID != uuid.Nil {
		return *workspaceID, true
	}
	return uuid.Nil, false
}

// applyAIChatMemoryPolicy gives the person's own AI chat their memory: the
// main chat and the console's contextual assistant, not pages that embed a
// chat for others (ADR 0021). It has no builder, so the general rules decide
// what to keep.
func applyAIChatMemoryPolicy(caller Caller, parts *chatRequestParts) {
	if parts == nil || normalizeCallerType(caller.Type) != runtimemodel.ConversationCallerAIChat {
		return
	}
	switch normalizeAIChatSurface(parts.Surface) {
	case aiChatSurfaceWorkChat, aiChatSurfaceContextualSidebar:
	default:
		return
	}
	parts.AgentMemoryEnabled = true
	parts.AgentMemoryAutoExtractionEnabled = globalAgentMemoryAutoExtractionEnabled()
	parts.AgentMemorySlots = nil
	parts.AgentMemoryUserScope = agentmemory.UserScopeAccount
	parts.AgentMemoryConfigScope = agentmemory.ConfigScopePublished
	parts.AgentMemoryConfigRevision = relationshipmemory.AIChatConfigRevision
	parts.AgentMemoryInstructions = ""
	parts.AgentMemoryAgentID = relationshipmemory.AIChatConsumerID.String()
}

func agentRelationshipMemoryIdentity(scope Scope, parts *chatRequestParts) relationshipmemory.InvocationIdentity {
	identity := relationshipmemory.InvocationIdentity{
		Principal:        relationshipmemory.PrincipalRef{ID: scope.AccountID},
		ConversationType: relationshipmemory.ConversationPrivate,
	}
	if parts == nil {
		return identity
	}
	if isAIChatMemory(parts) {
		identity.Principal.Type = "account"
		identity.Surface = relationshipmemory.SurfaceAIChat
		if scope.AccountID != uuid.Nil {
			subject := usersubject.AccountSubject(scope.AccountID)
			identity.UserSubject = &subject
		}
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

// agentMemoryEventScope names whose memory an event changed, so the reply
// can open the right memory page.
func agentMemoryEventScope(parts *chatRequestParts) string {
	if isAIChatMemory(parts) {
		return relationshipmemory.ConsumerAIChat
	}
	return "agent"
}

func agentMemoryContextMessage(parts *chatRequestParts) *adapter.Message {
	if parts == nil || strings.TrimSpace(parts.AgentMemoryContext) == "" {
		return nil
	}
	message := adapter.Message{Role: "user", Content: strings.TrimSpace(parts.AgentMemoryContext)}
	return &message
}

func globalAgentMemoryAutoExtractionEnabled() bool {
	return relationshipmemory.AutomaticExtractionEnabled()
}

func globalAgentMemoryInlineToolsEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("ZGI_AGENT_MEMORY_INLINE_TOOLS_ENABLED")))
	return value == "" || value == "1" || value == "true" || value == "yes" || value == "on"
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
	if !parts.AgentMemoryEnabled || !parts.AgentMemoryToolsEnabled {
		return false
	}
	if _, ok := agentMemoryWorkspaceID(prepared.Scope, parts); !ok || agentMemoryUserID(prepared.Scope) == uuid.Nil {
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
	archiveAvailable := s.repos != nil && s.repos.DB != nil
	definitions := agentmemoryruntime.FileTools(proactiveAllowed, archiveAvailable)
	if len(definitions) == 0 {
		return nil
	}
	validationFailures := 0
	succeeded := false
	terminalFailure := false
	partialCalls := 0
	views := 0
	handler := func(ctx context.Context, call adapter.ToolCall) skillloop.RuntimeToolResult {
		if call.Function.Name == agentmemoryruntime.ToolMemoryView {
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
		workspaceID, ok := agentMemoryWorkspaceID(prepared.Scope, parts)
		if !ok {
			return memoryRuntimeFailure("workspace_missing", false)
		}
		mutationCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		request := agentmemoryruntime.MutationRequest{
			MemoryService: s.agentMemoryService, WorkspaceID: workspaceID, Personal: isAIChatMemory(parts), AgentID: agentID,
			UserID: agentMemoryUserID(prepared.Scope), UserScope: parts.AgentMemoryUserScope,
			Slots: state.EnabledSlots, CurrentValues: state.SavedValues,
			MutationMetadata: agentMemoryMutationMetadata(prepared), LatestUserMessage: currentTurnUserText(parts),
			ProactiveAllowed: proactiveAllowed, RecentUserMessages: parts.AgentMemoryRecentUserMessages, CallID: call.ID,
		}
		if archiveAvailable {
			userScope, userID := parts.AgentMemoryUserScope, request.UserID
			request.Archive = func(ctx context.Context, content, sourceKey string) error {
				return relationshipmemory.NewAgentAdapter(s.repos.DB).ArchiveText(ctx, workspaceID, agentID, userScope, userID, content, sourceKey, sourceKey, request.MutationMetadata)
			}
		}
		result := agentmemoryruntime.ExecuteEdits(mutationCtx, request, call.Function.Arguments)
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
		// A call whose edits were partly rejected may be followed by one more
		// call that corrects them.
		if editsRejected(result.Result) && partialCalls == 0 {
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
	tools := make([]skillloop.RuntimeTool, 0, len(definitions)+1)
	for _, definition := range definitions {
		tools = append(tools, skillloop.RuntimeTool{Definition: definition, SkillID: skills.SkillAgentMemory, Handler: handler})
	}
	if search, ok := s.agentConversationSearchTool(prepared, agentID, state.UserScope); ok {
		tools = append(tools, search)
	}
	if archive, ok := s.agentArchiveSearchTool(prepared, agentID, state.UserScope); ok {
		tools = append(tools, archive)
	}
	return tools
}

// agentMemoryEvidenceWindow is how many recent user messages memory_edit
// evidence may quote besides the current one.
const agentMemoryEvidenceWindow = 6

// currentTurnUserText is what the user said in this turn: the query and, after
// the Agent asked a question, the user's answers.
func currentTurnUserText(parts *chatRequestParts) string {
	if len(parts.UserInputAnswers) == 0 {
		return parts.Query
	}
	return strings.Join(append([]string{parts.Query}, parts.UserInputAnswers...), "\n")
}

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
			"memory_scope": agentMemoryEventScope(prepared.parts), "action": action, "key": operation.Key, "status": "success",
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
	if parts == nil || !parts.AgentMemoryEnabled || parts.AgentMemoryToolsEnabled {
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
