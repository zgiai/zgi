package service

import (
	"context"
	"strings"

	"github.com/google/uuid"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const (
	agentMemoryToolUpdate = "update_agent_memory"
	agentMemoryToolClear  = "clear_agent_memory"

	maxNativeAgentMemoryRounds  = 3
	maxNativeAgentMemoryRetries = 1

	minAgentMemoryDecisionConfidence = 0.55
)

type nativeAgentMemoryDecision = AgentMemoryPlannerDecision

func (s *service) runNativeAgentMemoryPreflight(
	ctx context.Context,
	persistCtx context.Context,
	prepared *PreparedChat,
	onEvent func(StreamEvent) error,
) (*adapter.Usage, error) {
	state, skipStatus := agentMemoryPreflightState(prepared, s.agentMemoryService, s.llmClient != nil)
	if skipStatus != "" {
		if prepared != nil && prepared.parts != nil && prepared.parts.AgentMemoryEnabled {
			trace := nativeAgentMemoryPlannerTrace(nativeAgentMemoryDecision{Action: "none", Reason: skipStatus}, skipStatus, nil)
			s.recordNativeAgentMemoryTrace(ctx, persistCtx, prepared, []skills.SkillTrace{}, trace)
			s.updateAgentMemoryRuntimeMetadataBestEffort(persistCtx, prepared, map[string]interface{}{
				"planner_status": skipStatus,
				"planner_action": "none",
			})
		}
		return nil, nil
	}

	if !shouldRunNativeAgentMemoryDecision(prepared.parts.Query) {
		trace := nativeAgentMemoryPlannerTrace(nativeAgentMemoryDecision{Action: "none", Reason: "empty latest user message"}, "skipped_empty_query", nil)
		s.recordNativeAgentMemoryTrace(ctx, persistCtx, prepared, []skills.SkillTrace{}, trace)
		s.updateAgentMemoryRuntimeMetadataBestEffort(persistCtx, prepared, map[string]interface{}{
			"planner_status": "skipped_empty_query",
			"planner_action": "none",
		})
		prepared.LLMRequest.Messages = append(prepared.LLMRequest.Messages, nativeAgentMemoryGuardNote("skipped_empty_query"))
		return nil, nil
	}

	baseMessages := append([]adapter.Message{}, prepared.LLMRequest.Messages...)
	slots := state.EnabledSlots
	messages := nativeAgentMemoryPlannerMessages(state, prepared.parts.Query, baseMessages)
	var usage *adapter.Usage
	retries := 0
	traces := []skills.SkillTrace{}
	recordTrace := func(trace skills.SkillTrace) {
		traces = s.recordNativeAgentMemoryTrace(ctx, persistCtx, prepared, traces, trace)
	}

	for round := 0; round < maxNativeAgentMemoryRounds; round++ {
		req := cloneChatRequest(prepared.LLMRequest)
		req.Messages = messages
		req.Stream = false
		req.Tools = nil
		req.ToolChoice = nil
		temperature := 0.0
		req.Temperature = &temperature
		if shouldUseAgentMemoryPlannerJSONMode(prepared) {
			req.ResponseFormat = &adapter.ResponseFormat{Type: "json_object"}
		} else {
			req.ResponseFormat = nil
		}
		maxTokens := 700
		req.MaxTokens = &maxTokens

		resp, err := s.llmClient.AppChat(ctx, newBillingAppContext(prepared), req)
		if err != nil {
			trace := nativeAgentMemoryPlannerTrace(nativeAgentMemoryDecision{Action: "none", Reason: "planner llm error"}, "error_llm", err)
			recordTrace(trace)
			s.updateAgentMemoryRuntimeMetadataBestEffort(persistCtx, prepared, map[string]interface{}{
				"planner_status": "error_llm",
				"planner_action": "none",
			})
			prepared.LLMRequest.Messages = append(baseMessages, nativeAgentMemoryGuardNote("error_llm"))
			return usage, nil
		}
		usage = mergeUsage(usage, planningRespUsage(resp))
		message := firstPlanningMessage(resp)
		decision, err := parseNativeAgentMemoryDecision(assistantMessageText(message))
		if err != nil {
			retries++
			if retries > maxNativeAgentMemoryRetries {
				recordTrace(nativeAgentMemoryPlannerTrace(nativeAgentMemoryDecision{Action: "none", Reason: "planner returned invalid JSON"}, "error_parse", err))
				s.updateAgentMemoryRuntimeMetadataBestEffort(persistCtx, prepared, map[string]interface{}{
					"planner_status": "error_parse",
					"planner_action": "none",
				})
				prepared.LLMRequest.Messages = append(baseMessages, nativeAgentMemoryGuardNote("error_parse"))
				break
			}
			recordTrace(nativeAgentMemoryPlannerTrace(nativeAgentMemoryDecision{Action: "none", Reason: "planner returned invalid JSON"}, "error_parse", err))
			messages = append(messages, nativeAgentMemoryDecisionRetryMessage(err))
			continue
		}
		if decisionNoop(decision) {
			recordTrace(nativeAgentMemoryPlannerTrace(decision, "success_none", nil))
			s.updateAgentMemoryRuntimeMetadataBestEffort(persistCtx, prepared, map[string]interface{}{
				"planner_status": "success_none",
				"planner_action": "none",
			})
			prepared.LLMRequest.Messages = append(baseMessages, nativeAgentMemoryGuardNote("success_none"))
			break
		}
		plannerStatus := nativeAgentMemoryPlannerSuccessStatus(decision)
		recordTrace(nativeAgentMemoryPlannerTrace(decision, plannerStatus, nil))
		s.updateAgentMemoryRuntimeMetadataBestEffort(persistCtx, prepared, map[string]interface{}{
			"planner_status": plannerStatus,
			"planner_action": decision.Action,
			"planner_key":    decision.Key,
		})
		result, trace, err := s.applyNativeAgentMemoryDecision(ctx, prepared, slots, decision, onEvent)
		recordTrace(trace)
		if err != nil {
			s.updateAgentMemoryRuntimeMetadataBestEffort(persistCtx, prepared, map[string]interface{}{
				"mutation_status": trace.Status,
				"mutation_key":    decision.Key,
			})
			prepared.LLMRequest.Messages = append(baseMessages, nativeAgentMemoryGuardNote(trace.Status))
			break
		}
		finalMessages := append([]adapter.Message{}, baseMessages...)
		finalMessages = append(finalMessages, nativeAgentMemorySuccessNote(decision, result))
		prepared.LLMRequest.Messages = finalMessages
		s.updateAgentMemoryRuntimeMetadataBestEffort(persistCtx, prepared, map[string]interface{}{
			"mutation_status": "success",
			"mutation_key":    stringResultValue(result, "key"),
		})
		break
	}
	return usage, nil
}

func shouldRunNativeAgentMemoryPreflight(prepared *PreparedChat, memoryService AgentMemoryContextService, llmConfigured bool) bool {
	if prepared == nil || prepared.parts == nil || prepared.LLMRequest == nil || !llmConfigured || memoryService == nil {
		return false
	}
	if !prepared.parts.AgentMemoryEnabled || len(enabledAgentMemorySlots(prepared.parts.AgentMemorySlots)) == 0 {
		return false
	}
	if prepared.Scope.WorkspaceID == nil || *prepared.Scope.WorkspaceID == uuid.Nil {
		return false
	}
	agentID, err := uuid.Parse(strings.TrimSpace(prepared.parts.AgentMemoryAgentID))
	return err == nil && agentID != uuid.Nil
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
