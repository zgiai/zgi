package skillloop

import (
	"encoding/json"
	"fmt"
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func applyStateHandoffAdvisoryToToolMessage(invocation *skills.ToolInvocationResult) {
	if invocation == nil || !stateHandoffAdvisoryRecommended(invocation.Trace) {
		return
	}
	payload := toolMessageJSONPayload(invocation.ToolMessage)
	if len(payload) == 0 {
		return
	}
	if _, exists := payload["state_handoff"]; exists {
		return
	}
	payload["state_handoff"] = stateHandoffAdvisoryPayload(invocation.Trace)
	invocation.ToolMessage = skills.ToolResultMessage(invocation.ToolMessage.ToolCallID, payload)
}

func toolMessageJSONPayload(message adapter.Message) map[string]interface{} {
	switch content := message.Content.(type) {
	case string:
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(content), &payload); err == nil && payload != nil {
			return payload
		}
	case []byte:
		var payload map[string]interface{}
		if err := json.Unmarshal(content, &payload); err == nil && payload != nil {
			return payload
		}
	case map[string]interface{}:
		return copyStringAnyMap(content)
	default:
		data, err := json.Marshal(content)
		if err != nil {
			return nil
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(data, &payload); err == nil && payload != nil {
			return payload
		}
	}
	return nil
}

func stateHandoffAdvisoryRecommended(trace skills.SkillTrace) bool {
	if !strings.EqualFold(strings.TrimSpace(trace.Status), "success") {
		return false
	}
	toolName := strings.ToLower(strings.TrimSpace(trace.ToolName))
	if toolName == "" {
		return false
	}
	for _, prefix := range []string{"read", "get", "list", "search", "inspect", "lookup", "query"} {
		if toolName == prefix || strings.HasPrefix(toolName, prefix+"_") || strings.HasPrefix(toolName, prefix+"-") {
			return true
		}
	}
	if strings.Contains(toolName, "read") || strings.Contains(toolName, "list") || strings.Contains(toolName, "search") {
		return true
	}
	return false
}

func stateHandoffAdvisoryPayload(trace skills.SkillTrace) map[string]interface{} {
	suggestedKeys := stateHandoffSuggestedKeys(trace)
	return map[string]interface{}{
		"recommended": true,
		"reason": strings.Join([]string{
			"Approvals, page navigation, refresh, or a long multi-step loop can make implicit working memory unreliable.",
			"If any value from this tool result will affect later tool arguments, naming, configuration, verification, or the final answer, record the reusable fact with submit_turn_state before crossing that boundary.",
		}, " "),
		"record_when": []interface{}{
			"you will reuse exact short text or a derived theme later",
			"you will use this result after navigation, approval, refresh, or another tool phase",
			"this result determines a target name, model, skill choice, prompt, asset, or verification conclusion",
		},
		"do_not_record": []interface{}{
			"irrelevant transient details",
			"large full documents; record a concise summary/theme and re-read when exact full text is needed",
		},
		"suggested_state_kind": "working_fact",
		"suggested_keys":       suggestedKeys,
		"example_call": fmt.Sprintf(
			`submit_turn_state({"items":[{"kind":"working_fact","visibility":"model_only","key":"%s","value":"<exact reusable fact>","source":"%s/%s"}]})`,
			firstNonEmptyStringFromList(suggestedKeys, "tool_result_fact"),
			strings.TrimSpace(trace.SkillID),
			strings.TrimSpace(trace.ToolName),
		),
	}
}

func stateHandoffSuggestedKeys(trace skills.SkillTrace) []interface{} {
	skillID := strings.TrimSpace(trace.SkillID)
	toolName := strings.TrimSpace(trace.ToolName)
	switch skillID {
	case skills.SkillFileReader:
		if strings.Contains(strings.ToLower(toolName), "read") {
			return []interface{}{"source_file_summary", "source_file_theme", "exact_short_text"}
		}
	case skills.SkillAgentManagement:
		return []interface{}{"selected_agent", "agent_config_fact", "agent_edit_decision"}
	case skills.SkillFileManager:
		return []interface{}{"selected_file", "file_operation_fact"}
	case skills.SkillConsoleNavigator:
		return []interface{}{"current_page_fact", "route_decision"}
	}
	if toolName != "" {
		return []interface{}{strings.ToLower(strings.ReplaceAll(toolName, "-", "_")) + "_fact"}
	}
	return []interface{}{"tool_result_fact"}
}

func firstNonEmptyStringFromList(values []interface{}, fallback string) string {
	for _, value := range values {
		text := strings.TrimSpace(stringFromInterface(value))
		if text != "" {
			return text
		}
	}
	return fallback
}
