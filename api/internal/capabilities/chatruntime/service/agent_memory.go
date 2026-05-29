package service

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/agentmemoryruntime"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

type nativeAgentMemoryDecision = agentmemoryruntime.Decision

const (
	agentMemoryToolUpdate = agentmemoryruntime.ToolUpdate
	agentMemoryToolClear  = agentmemoryruntime.ToolClear
)

func (s *service) appendAgentMemoryContext(ctx context.Context, scope Scope, parts *chatRequestParts, systemPrompt string) (string, map[string]interface{}, error) {
	if parts == nil || !parts.AgentMemoryEnabled || len(enabledAgentMemorySlots(parts.AgentMemorySlots)) == 0 {
		return systemPrompt, nil, nil
	}
	agentID, _ := uuid.Parse(strings.TrimSpace(parts.AgentMemoryAgentID))
	workspaceID := uuid.Nil
	if scope.WorkspaceID != nil {
		workspaceID = *scope.WorkspaceID
	}
	result, err := agentmemoryruntime.BuildContext(ctx, agentmemoryruntime.ContextRequest{
		SystemPrompt:  systemPrompt,
		Enabled:       parts.AgentMemoryEnabled,
		Slots:         enabledAgentMemorySlots(parts.AgentMemorySlots),
		MemoryService: s.agentMemoryService,
		WorkspaceID:   workspaceID,
		AgentID:       agentID,
		UserID:        scope.AccountID,
		UserScope:     parts.AgentMemoryUserScope,
		Budget:        agentMemoryContextBudgetChars,
	})
	if result.State != nil {
		parts.AgentMemoryRuntimeState = result.State
	}
	return result.SystemPrompt, result.Metadata, err
}

func appendAgentMemoryPolicy(systemPrompt string, parts *chatRequestParts) string {
	if parts == nil {
		return systemPrompt
	}
	return agentmemoryruntime.AppendPolicy(systemPrompt, parts.AgentMemoryEnabled, enabledAgentMemorySlots(parts.AgentMemorySlots))
}

func renderAgentMemoryPolicy(parts *chatRequestParts) string {
	if parts == nil {
		return ""
	}
	return agentmemoryruntime.RenderPolicy(parts.AgentMemoryEnabled, enabledAgentMemorySlots(parts.AgentMemorySlots))
}

func renderAgentMemoryContext(values []agentmemory.SlotValueResponse, budget int) (string, int) {
	return agentmemoryruntime.RenderContext(values, budget)
}

func agentMemoryRuntimeSlots(input []AgentMemorySlotConfig) []agentmemory.RuntimeSlot {
	return agentmemoryruntime.RuntimeSlots(enabledAgentMemorySlots(input))
}

func enabledAgentMemorySlots(input []AgentMemorySlotConfig) []AgentMemorySlotConfig {
	normalized := normalizeAgentMemorySlots(input)
	if len(normalized) == 0 {
		return nil
	}
	out := make([]AgentMemorySlotConfig, 0, len(normalized))
	for _, slot := range normalized {
		if slot.Enabled {
			out = append(out, slot)
		}
	}
	return out
}

func (s *service) runNativeAgentMemoryPreflight(
	ctx context.Context,
	persistCtx context.Context,
	prepared *PreparedChat,
	onEvent func(StreamEvent) error,
) (*adapter.Usage, error) {
	state, skipStatus := agentMemoryPreflightState(prepared, s.agentMemoryService, s.llmClient != nil)
	if skipStatus != "" {
		if prepared != nil && prepared.parts != nil && prepared.parts.AgentMemoryEnabled {
			trace := agentmemoryruntime.PlannerTrace(agentmemoryruntime.Decision{Action: "none", Reason: skipStatus}, skipStatus, nil)
			s.recordNativeAgentMemoryTrace(ctx, persistCtx, prepared, []skills.SkillTrace{}, trace)
			s.updateAgentMemoryRuntimeMetadataBestEffort(persistCtx, prepared, map[string]interface{}{
				"planner_status": skipStatus,
				"planner_action": "none",
			})
		}
		return nil, nil
	}

	workspaceID := uuid.Nil
	if prepared.Scope.WorkspaceID != nil {
		workspaceID = *prepared.Scope.WorkspaceID
	}
	result := agentmemoryruntime.RunPreflight(ctx, agentmemoryruntime.PreflightRequest{
		LatestUserMessage: prepared.parts.Query,
		LLMRequest:        prepared.LLMRequest,
		State:             state,
		MemoryService:     s.agentMemoryService,
		WorkspaceID:       workspaceID,
		AgentID:           state.AgentID,
		UserID:            prepared.Scope.AccountID,
		UserScope:         prepared.parts.AgentMemoryUserScope,
		MutationMetadata:  agentMemoryMutationMetadata(prepared),
		LLMClient:         s.llmClient,
		AppContext:        newBillingAppContext(prepared),
		UseJSONMode:       shouldUseAgentMemoryPlannerJSONMode(prepared),
		OnToolCallStart: func(toolName string, arguments map[string]interface{}) {
			s.emitPreparedEvent(ctx, prepared, streamEventSkillCallStart, skillCallStartPayload(prepared, skills.SkillAgentMemory, toolName, arguments), onEvent)
		},
		OnToolCallEnd: func(trace skills.SkillTrace) {
			s.emitPreparedEvent(ctx, prepared, streamEventSkillCallEnd, skillCallEndPayload(prepared, trace), onEvent)
		},
	})
	traces := []skills.SkillTrace{}
	for _, trace := range result.Traces {
		traces = s.recordNativeAgentMemoryTrace(ctx, persistCtx, prepared, traces, trace)
	}
	if len(result.MetadataUpdates) > 0 {
		s.updateAgentMemoryRuntimeMetadataBestEffort(persistCtx, prepared, result.MetadataUpdates)
	}
	if len(result.Messages) > 0 {
		prepared.LLMRequest.Messages = result.Messages
	}
	return result.Usage, nil
}

func shouldRunNativeAgentMemoryPreflight(prepared *PreparedChat, memoryService AgentMemoryContextService, llmConfigured bool) bool {
	_, skipStatus := agentMemoryPreflightState(prepared, memoryService, llmConfigured)
	return skipStatus == ""
}

func agentMemoryPreflightState(prepared *PreparedChat, memoryService AgentMemoryContextService, llmConfigured bool) (*AgentMemoryRuntimeState, string) {
	if prepared == nil || prepared.parts == nil || prepared.LLMRequest == nil {
		return nil, ""
	}
	if !prepared.parts.AgentMemoryEnabled {
		return nil, ""
	}
	slots := enabledAgentMemorySlots(prepared.parts.AgentMemorySlots)
	if len(slots) == 0 {
		return nil, "skipped_scope"
	}
	if memoryService == nil || !llmConfigured {
		return nil, "skipped_scope"
	}
	state := prepared.parts.AgentMemoryRuntimeState
	if state == nil {
		state = &AgentMemoryRuntimeState{
			Enabled:      true,
			UserScope:    strings.TrimSpace(prepared.parts.AgentMemoryUserScope),
			EnabledSlots: slots,
		}
		prepared.parts.AgentMemoryRuntimeState = state
	}
	if len(state.EnabledSlots) == 0 {
		state.EnabledSlots = slots
	}
	if prepared.Scope.WorkspaceID == nil || *prepared.Scope.WorkspaceID == uuid.Nil {
		return state, "skipped_scope"
	}
	agentID, err := uuid.Parse(strings.TrimSpace(prepared.parts.AgentMemoryAgentID))
	if err != nil || agentID == uuid.Nil {
		return state, "skipped_scope"
	}
	state.AgentID = agentID
	return state, ""
}

func agentMemoryMutationMetadata(prepared *PreparedChat) agentmemory.MutationMetadata {
	meta := agentmemory.MutationMetadata{
		ActorType: agentmemory.EventActorModel,
		Source:    agentmemory.EventSourceAgent,
	}
	if prepared != nil && prepared.Conversation != nil {
		id := prepared.Conversation.ID
		meta.SourceConversationID = &id
	}
	if prepared != nil && prepared.Message != nil {
		id := prepared.Message.ID
		meta.SourceMessageID = &id
	}
	return meta
}

func (s *service) recordNativeAgentMemoryTrace(ctx context.Context, persistCtx context.Context, prepared *PreparedChat, traces []skills.SkillTrace, trace skills.SkillTrace) []skills.SkillTrace {
	if strings.TrimSpace(trace.Status) == "" {
		trace.Status = "success_none"
	}
	traces = append(traces, trace)
	s.persistSkillTracesBestEffort(persistCtx, prepared, traces)
	s.logSkillTrace(ctx, prepared, trace)
	return traces
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
		if stringValue, ok := value.(string); ok && strings.TrimSpace(stringValue) == "" {
			continue
		}
		agentMemory[key] = value
	}
	contextControl["agent_memory"] = agentMemory
	metadata["context_control"] = contextControl
	prepared.Message.Metadata = metadata
	if prepared.parts != nil {
		prepared.parts.ContextControl = contextControl
	}
	if s == nil || s.repos == nil || s.repos.Message == nil {
		return
	}
	_ = s.repos.Message.UpdateMetadata(ctx, prepared.Message.ID, metadata)
}

func nativeAgentMemoryDecisionStateMessage(state *AgentMemoryRuntimeState, latestUserMessage string) adapter.Message {
	return agentmemoryruntime.DecisionStateMessage(state, latestUserMessage)
}

func nativeAgentMemoryDecisionStateMessageWithHistory(state *AgentMemoryRuntimeState, latestUserMessage string, sourceMessages []adapter.Message) adapter.Message {
	return agentmemoryruntime.DecisionStateMessageWithHistory(state, latestUserMessage, sourceMessages)
}

func nativeAgentMemoryPlannerMessages(state *AgentMemoryRuntimeState, latestUserMessage string, sourceMessages []adapter.Message) []adapter.Message {
	return agentmemoryruntime.PlannerMessages(state, latestUserMessage, sourceMessages)
}

func nativeAgentMemoryDecisionMessage(slots []AgentMemorySlotConfig) adapter.Message {
	return agentmemoryruntime.DecisionMessage(slots)
}

func nativeAgentMemoryDecisionRetryMessage(err error) adapter.Message {
	return agentmemoryruntime.DecisionRetryMessage(err)
}

func parseNativeAgentMemoryDecision(raw string) (nativeAgentMemoryDecision, error) {
	return agentmemoryruntime.ParseDecision(raw)
}

func decisionNoop(decision nativeAgentMemoryDecision) bool {
	return agentmemoryruntime.DecisionNoop(decision)
}

func nativeAgentMemoryDecisionToolCall(decision nativeAgentMemoryDecision) (adapter.ToolCall, error) {
	return agentmemoryruntime.DecisionToolCall(decision)
}

func nativeAgentMemorySuccessNote(decision nativeAgentMemoryDecision, result map[string]interface{}) adapter.Message {
	return agentmemoryruntime.SuccessNote(decision, result)
}

func nativeAgentMemoryGuardNote(status string) adapter.Message {
	return agentmemoryruntime.GuardNote(status)
}

func nativeAgentMemoryPlannerSuccessStatus(decision nativeAgentMemoryDecision) string {
	return agentmemoryruntime.PlannerSuccessStatus(decision)
}

func shouldUseAgentMemoryPlannerJSONMode(_ *PreparedChat) bool {
	return false
}

func shouldRunNativeAgentMemoryDecision(query string) bool {
	return agentmemoryruntime.ShouldRunDecision(query)
}

func nativeAgentMemoryTools(slots []AgentMemorySlotConfig) []adapter.Tool {
	return agentmemoryruntime.Tools(slots)
}

func nativeAgentMemoryToolCalls(calls []adapter.ToolCall) []adapter.ToolCall {
	return agentmemoryruntime.ToolCalls(calls)
}

func validateNativeAgentMemoryDecision(decision nativeAgentMemoryDecision, slots []AgentMemorySlotConfig) (string, map[string]interface{}, error) {
	return agentmemoryruntime.ValidateDecision(decision, slots)
}

func findEnabledAgentMemorySlot(slots []AgentMemorySlotConfig, key string) (AgentMemorySlotConfig, bool) {
	key = strings.TrimSpace(key)
	for _, slot := range slots {
		if slot.Enabled && strings.TrimSpace(slot.Key) == key {
			return slot, true
		}
	}
	return AgentMemorySlotConfig{}, false
}

func containsSensitiveAgentMemoryContent(content string) bool {
	return agentmemoryruntime.ContainsSensitiveContent(content)
}

func containsLongDigitRun(value string, limit int) bool {
	run := 0
	if limit <= 0 {
		return false
	}
	for _, r := range value {
		if r >= '0' && r <= '9' {
			run++
			if run >= limit {
				return true
			}
			continue
		}
		run = 0
	}
	return false
}

func nativeAgentMemoryPlannerTrace(decision nativeAgentMemoryDecision, status string, err error) skills.SkillTrace {
	return agentmemoryruntime.PlannerTrace(decision, status, err)
}

func nativeAgentMemoryDecisionTrace(decision nativeAgentMemoryDecision, status string, result map[string]interface{}, err error) skills.SkillTrace {
	return agentmemoryruntime.DecisionTrace(decision, status, result, err)
}

func truncateNativeAgentMemoryRunes(value string, limit int) string {
	return agentmemoryruntime.TruncateRunes(value, limit)
}

func stringResultValue(result map[string]interface{}, key string) string {
	return agentmemoryruntime.StringResultValue(result, key)
}

func summarizeNativeAgentMemoryArguments(toolName string, args map[string]interface{}) map[string]interface{} {
	return agentmemoryruntime.SummarizeArguments(toolName, args)
}
