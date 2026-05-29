package service

import (
	"context"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

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

func nativeAgentMemoryPlannerTrace(decision nativeAgentMemoryDecision, status string, err error) skills.SkillTrace {
	trace := nativeAgentMemoryDecisionTrace(decision, status, nil, err)
	trace.Kind = "memory_planner"
	trace.ToolName = "plan_agent_memory"
	return trace
}

func nativeAgentMemoryDecisionTrace(decision nativeAgentMemoryDecision, status string, result map[string]interface{}, err error) skills.SkillTrace {
	trace := skills.SkillTrace{
		Kind:      "agent_memory",
		SkillID:   skills.SkillAgentMemory,
		Status:    status,
		Arguments: map[string]interface{}{},
		Result:    map[string]interface{}{},
	}
	if action := strings.TrimSpace(decision.Action); action != "" {
		trace.Arguments["action"] = action
	}
	if key := strings.TrimSpace(decision.Key); key != "" {
		trace.Arguments["key"] = key
	}
	if decision.Confidence != nil {
		trace.Arguments["confidence"] = *decision.Confidence
	}
	if reason := strings.TrimSpace(decision.Reason); reason != "" {
		trace.Result["reason"] = truncateNativeAgentMemoryRunes(reason, 160)
	}
	for key, value := range result {
		trace.Result[key] = value
	}
	if err != nil {
		trace.Error = err.Error()
	}
	return trace
}

func truncateNativeAgentMemoryRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "..."
}
