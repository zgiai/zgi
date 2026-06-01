package agentmemoryruntime

import (
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func PlannerTrace(decision Decision, status string, err error) skills.SkillTrace {
	trace := DecisionTrace(decision, status, nil, err)
	trace.Kind = "memory_planner"
	trace.ToolName = "plan_agent_memory"
	return trace
}

func DecisionTrace(decision Decision, status string, result map[string]interface{}, err error) skills.SkillTrace {
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
		trace.Result["reason"] = TruncateRunes(reason, 160)
	}
	for key, value := range result {
		trace.Result[key] = value
	}
	if err != nil {
		trace.Error = err.Error()
	}
	return trace
}

func TruncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "..."
}
