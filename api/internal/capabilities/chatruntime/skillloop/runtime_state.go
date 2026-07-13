package skillloop

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func runtimeStateForRun(req RunRequest) map[string]interface{} {
	state := map[string]interface{}{}
	if req.RuntimeStateSnapshot != nil {
		state = copyStringAnyMap(req.RuntimeStateSnapshot())
	}
	if state == nil {
		state = map[string]interface{}{}
	}
	for _, key := range []string{
		"operation_plan",
		"operation_result_summary",
		"evidence_ledger",
		"turn_state",
		"execution_summary",
		"execution_ledger",
		"agent_create_progress",
		"generated_files",
		"client_actions",
		"pending_approval",
		"pending_client_action",
		"pending_question",
		"pending_user_input",
	} {
		if _, exists := state[key]; exists {
			continue
		}
		if value, ok := currentMetadataForRun(req)[key]; ok && value != nil {
			state[key] = value
		}
	}
	if text := strings.TrimSpace(latestUserRequestText(req)); text != "" {
		state["latest_user_request"] = text
	}
	return state
}

func runtimeStateWithSuccessfulToolCalls(req RunRequest, successful []SkillToolCallRef) map[string]interface{} {
	state := runtimeStateForRun(req)
	if len(successful) == 0 {
		return state
	}
	invocations := evidenceSliceFromAny(state["skill_invocations"])
	existing := map[string]struct{}{}
	for _, raw := range invocations {
		invocation := evidenceMapFromAny(raw)
		if len(invocation) == 0 || !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(invocation["kind"])), "tool_call") {
			continue
		}
		signature := skillToolCallEvidenceSignature(
			evidenceStringFromAny(invocation["skill_id"]),
			evidenceStringFromAny(invocation["tool_name"]),
			evidenceMapFromAny(invocation["arguments"]),
			evidenceMapFromAny(invocation["result"]),
		)
		if signature != "" {
			existing[signature] = struct{}{}
		}
	}
	for _, call := range successful {
		skillID := strings.TrimSpace(call.SkillID)
		toolName := strings.TrimSpace(call.ToolName)
		if skillID == "" || toolName == "" {
			continue
		}
		signature := skillToolCallEvidenceSignature(skillID, toolName, call.Arguments, call.Result)
		if _, ok := existing[signature]; ok && signature != "" {
			continue
		}
		invocation := map[string]interface{}{
			"kind":      "tool_call",
			"status":    "success",
			"skill_id":  skillID,
			"tool_name": toolName,
		}
		if len(call.Arguments) > 0 {
			invocation["arguments"] = copyStringAnyMap(call.Arguments)
		}
		if len(call.Result) > 0 {
			invocation["result"] = copyStringAnyMap(call.Result)
		}
		invocations = append(invocations, invocation)
		if signature != "" {
			existing[signature] = struct{}{}
		}
	}
	if len(invocations) > 0 {
		state["skill_invocations"] = invocations
	}
	return state
}

func latestUserRequestText(req RunRequest) string {
	if req.Prepared != nil {
		if text := strings.TrimSpace(req.Prepared.Query); text != "" {
			return text
		}
	}
	return latestUserMessageText(req)
}

func latestUserMessageText(req RunRequest) string {
	if req.Prepared == nil || req.Prepared.LLMRequest == nil {
		return ""
	}
	messages := req.Prepared.LLMRequest.Messages
	for index := len(messages) - 1; index >= 0; index-- {
		if !strings.EqualFold(strings.TrimSpace(messages[index].Role), "user") {
			continue
		}
		if text := strings.TrimSpace(messageContent(messages[index].Content)); text != "" {
			return text
		}
	}
	return ""
}

func skillToolCallEvidenceSignature(skillID string, toolName string, arguments map[string]interface{}, result map[string]interface{}) string {
	skillID = strings.TrimSpace(skillID)
	toolName = strings.TrimSpace(toolName)
	if skillID == "" || toolName == "" {
		return ""
	}
	payload := map[string]interface{}{"skill_id": skillID, "tool_name": toolName}
	if len(arguments) > 0 {
		payload["arguments"] = arguments
	}
	if len(result) > 0 {
		payload["result"] = result
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return skillID + "/" + toolName
	}
	return string(data)
}

func runtimeInvocationSucceeded(invocation map[string]interface{}) bool {
	result := evidenceMapFromAny(invocation["result"])
	if len(result) == 0 {
		result = evidenceMapFromAny(invocation["result_summary"])
	}
	if runtimeResultHasFailedItems(result) {
		return false
	}
	resultStatus := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(result["status"])))
	if resultStatus == "" {
		resultStatus = strings.ToLower(strings.TrimSpace(evidenceStringFromAny(result["result_status"])))
	}
	switch resultStatus {
	case "error", "failed", "partial_failed", "partially_failed":
		return false
	}
	switch strings.ToLower(strings.TrimSpace(evidenceStringFromAny(invocation["status"]))) {
	case "success", "succeeded", "completed", "allowed", "approved":
		return true
	}
	switch resultStatus {
	case "success", "succeeded", "completed", "allowed", "approved":
		return true
	default:
		return false
	}
}

func runtimeResultHasFailedItems(result map[string]interface{}) bool {
	if len(result) == 0 {
		return false
	}
	for _, source := range []map[string]interface{}{result, evidenceMapFromAny(result["operation_group"])} {
		if numericValue(source["failed_count"]) > 0 {
			return true
		}
		for _, item := range evidenceMapsFromAny(source["item_results"]) {
			switch strings.ToLower(strings.TrimSpace(evidenceStringFromAny(item["status"]))) {
			case "failed", "error", "blocked", "rejected":
				return true
			}
		}
	}
	return false
}

func numericValue(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return int(parsed)
		}
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	}
	return 0
}

func evidenceMapFromAny(value interface{}) map[string]interface{} {
	if typed, ok := value.(map[string]interface{}); ok {
		return typed
	}
	return nil
}

func evidenceSliceFromAny(value interface{}) []interface{} {
	switch typed := value.(type) {
	case []interface{}:
		return typed
	case []map[string]interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	default:
		return nil
	}
}

func evidenceMapsFromAny(value interface{}) []map[string]interface{} {
	if typed, ok := value.([]map[string]interface{}); ok {
		return typed
	}
	values := evidenceSliceFromAny(value)
	out := make([]map[string]interface{}, 0, len(values))
	for _, value := range values {
		if item := evidenceMapFromAny(value); len(item) > 0 {
			out = append(out, item)
		}
	}
	return out
}

func evidenceStringFromAny(value interface{}) string {
	if value == nil {
		return ""
	}
	if typed, ok := value.(string); ok {
		return typed
	}
	if typed, ok := value.(fmt.Stringer); ok {
		return typed.String()
	}
	return fmt.Sprint(value)
}

func evidenceValuePresent(value interface{}) bool {
	return len(evidenceMapFromAny(value)) > 0 || len(evidenceSliceFromAny(value)) > 0
}

func mapSliceFromAny(value interface{}) []map[string]interface{} {
	return evidenceMapsFromAny(value)
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
