package usermemoryruntime

import (
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func PlannerTrace(decision Decision, status string, err error) skills.SkillTrace {
	trace := DecisionTrace(decision, status, nil, err)
	trace.Kind = "memory_planner"
	trace.ToolName = "plan_user_memory"
	return trace
}

func DecisionTrace(decision Decision, status string, result map[string]interface{}, err error) skills.SkillTrace {
	trace := skills.SkillTrace{
		Kind:      "user_memory",
		Status:    status,
		Arguments: map[string]interface{}{},
		Result:    map[string]interface{}{},
	}
	if action := strings.TrimSpace(decision.Action); action != "" {
		trace.Arguments["action"] = action
	}
	if entryID := strings.TrimSpace(decision.EntryID); entryID != "" {
		trace.Arguments["entry_id"] = entryID
	}
	if category := strings.TrimSpace(decision.Category); category != "" {
		trace.Arguments["category"] = category
	}
	if memoryType := strings.TrimSpace(decision.MemoryType); memoryType != "" {
		trace.Arguments["memory_type"] = memoryType
	}
	if decision.Confidence != nil {
		trace.Arguments["confidence"] = *decision.Confidence
	}
	if reason := strings.TrimSpace(decision.Reason); reason != "" {
		trace.Result["reason"] = truncateRunes(reason, 160)
	}
	for key, value := range result {
		trace.Result[key] = value
	}
	if err != nil {
		trace.Error = err.Error()
	}
	return trace
}
