package service

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/agentmemoryruntime"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const agentMemoryToolMutate = agentmemoryruntime.ToolMutate

func (s *service) appendAgentMemoryContext(ctx context.Context, scope Scope, parts *chatRequestParts, systemPrompt string, modelInputLimit int) (string, map[string]interface{}, error) {
	if parts == nil || !parts.AgentMemoryEnabled || len(enabledAgentMemorySlots(parts.AgentMemorySlots)) == 0 {
		return systemPrompt, nil, nil
	}
	agentID, _ := uuid.Parse(strings.TrimSpace(parts.AgentMemoryAgentID))
	workspaceID := uuid.Nil
	if scope.WorkspaceID != nil {
		workspaceID = *scope.WorkspaceID
	}
	budgetTokens := maxAgentMemoryContextTokens
	if modelInputLimit > 0 && modelInputLimit/10 < budgetTokens {
		budgetTokens = modelInputLimit / 10
	}
	if budgetTokens <= 0 {
		return systemPrompt, nil, nil
	}
	budgetChars := budgetTokens * 4
	result, err := agentmemoryruntime.BuildContext(ctx, agentmemoryruntime.ContextRequest{
		SystemPrompt: systemPrompt, Enabled: parts.AgentMemoryEnabled,
		Slots: enabledAgentMemorySlots(parts.AgentMemorySlots), MemoryService: s.agentMemoryService,
		WorkspaceID: workspaceID, AgentID: agentID, UserID: agentMemoryUserID(scope),
		UserScope: parts.AgentMemoryUserScope, Budget: budgetChars,
	})
	if strings.TrimSpace(result.Context) != "" && result.State != nil && s.tokenEstimator != nil {
		bestContext := ""
		var bestValues []map[string]interface{}
		low, high := 1, budgetChars
		for low <= high {
			mid := low + (high-low)/2
			candidate, values := agentmemoryruntime.RenderContextWithMetadata(result.State.SavedValues, mid)
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
	return result.SystemPrompt, result.Metadata, err
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
	if prepared.Scope.WorkspaceID == nil || *prepared.Scope.WorkspaceID == uuid.Nil || agentMemoryUserID(prepared.Scope) == uuid.Nil {
		return
	}
	agentID, err := uuid.Parse(strings.TrimSpace(prepared.parts.AgentMemoryAgentID))
	if err != nil || agentID == uuid.Nil {
		return
	}
	_, _ = s.agentMemoryExtractionScheduler.ScheduleExtraction(ctx, agentmemory.ScheduleExtractionRequest{
		WorkspaceID: prepared.Scope.WorkspaceID.String(), AgentID: agentID.String(),
		UserScope: prepared.parts.AgentMemoryUserScope, UserID: agentMemoryUserID(prepared.Scope).String(),
		ConversationID: prepared.Conversation.ID.String(), MessageWatermarkID: prepared.Message.ID.String(),
		ExtractorVersion: "agent-memory-v2",
	})
}

func globalAgentMemoryAutoExtractionEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("ZGI_AGENT_MEMORY_AUTO_EXTRACTION_ENABLED")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
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
	if prepared.Scope.WorkspaceID == nil || *prepared.Scope.WorkspaceID == uuid.Nil || agentMemoryUserID(prepared.Scope) == uuid.Nil {
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
		state = &AgentMemoryRuntimeState{Enabled: true, UserScope: parts.AgentMemoryUserScope, EnabledSlots: enabledAgentMemorySlots(parts.AgentMemorySlots)}
		parts.AgentMemoryRuntimeState = state
	}
	if len(state.EnabledSlots) == 0 {
		state.EnabledSlots = enabledAgentMemorySlots(parts.AgentMemorySlots)
	}
	agentID, _ := uuid.Parse(strings.TrimSpace(parts.AgentMemoryAgentID))
	state.AgentID = agentID
	proactiveAllowed := parts.AgentMemoryAutoExtractionEnabled && globalAgentMemoryAutoExtractionEnabled()
	definitions := agentmemoryruntime.Tools(state.EnabledSlots, proactiveAllowed)
	if len(definitions) == 0 {
		return nil
	}
	validationFailures := 0
	succeeded := false
	terminalFailure := false
	handler := func(ctx context.Context, call adapter.ToolCall) skillloop.RuntimeToolResult {
		if succeeded {
			return memoryRuntimeFailure("already_mutated", false)
		}
		if terminalFailure || validationFailures > 1 {
			// The model gets one correction attempt after a side-effect-free
			// validation failure. Further attempts remain harmless and final.
			return memoryRuntimeFailure("retry_exhausted", false)
		}
		workspaceID := *prepared.Scope.WorkspaceID
		mutationCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		result := agentmemoryruntime.Execute(mutationCtx, agentmemoryruntime.MutationRequest{
			MemoryService: s.agentMemoryService, WorkspaceID: workspaceID, AgentID: agentID,
			UserID: agentMemoryUserID(prepared.Scope), UserScope: parts.AgentMemoryUserScope,
			Slots: state.EnabledSlots, CurrentValues: state.SavedValues,
			MutationMetadata: agentMemoryMutationMetadata(prepared), LatestUserMessage: parts.Query,
			ProactiveAllowed: proactiveAllowed,
		}, call.Function.Arguments)
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
		succeeded = true
		s.recordAgentMemoryMutationEvents(ctx, prepared, result.Response, timeline)
		s.updateAgentMemoryRuntimeMetadataBestEffort(persistCtx, prepared, map[string]interface{}{
			"inline_status": "success", "inline_operation_count": len(result.Response.Operations),
		})
		return skillloop.RuntimeToolResult{Status: "success", Arguments: result.Arguments, Result: result.Result}
	}
	return []skillloop.RuntimeTool{{Definition: definitions[0], SkillID: skills.SkillAgentMemory, Handler: handler}}
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
