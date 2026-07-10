package skillloop

import (
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func applyPageContextInvalidationAdvisory(invocation *skills.ToolInvocationResult) {
	if invocation == nil || !pageContextInvalidatingMutation(invocation.Trace) {
		return
	}
	payload := toolMessageJSONPayload(invocation.ToolMessage)
	if len(payload) == 0 {
		return
	}
	payload["page_context_invalidation"] = map[string]interface{}{
		"status":      "stale",
		"reason":      "The successful mutation may have changed the current backend list or detail state.",
		"instruction": "Before resolving another ordinal target from that page, use a refreshed backend list or current_page_context. Do not reuse the pre-mutation visible order.",
	}
	invocation.ToolMessage = skills.ToolResultMessage(invocation.ToolMessage.ToolCallID, payload)
}

func pageContextInvalidatingMutation(trace skills.SkillTrace) bool {
	if !strings.EqualFold(strings.TrimSpace(trace.Status), "success") {
		return false
	}
	skillID := strings.ToLower(strings.TrimSpace(trace.SkillID))
	toolName := strings.ToLower(strings.TrimSpace(trace.ToolName))
	if toolName == "" {
		return false
	}
	readOnly := strings.HasPrefix(toolName, "get_") || strings.HasPrefix(toolName, "list_") || strings.HasPrefix(toolName, "read_") || strings.HasPrefix(toolName, "search_")
	if readOnly {
		return false
	}
	return skillID == skills.SkillAgentManagement || skillID == skills.SkillFileManager || skillID == skills.SkillFileReader
}
