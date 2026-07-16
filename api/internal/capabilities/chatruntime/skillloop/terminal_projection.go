package skillloop

import (
	"encoding/json"
	"sort"
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	terminalProjectionUserGoalMaxRunes = 4000
	terminalProjectionValueMaxRunes    = 1600
)

func terminalOnlyProjectedMessages(prepared *PreparedChat, metadata map[string]interface{}) []adapter.Message {
	messages := []adapter.Message{}
	if evidence := terminalOnlyEvidenceSnapshot(metadata); len(evidence) > 0 {
		if encoded, err := json.Marshal(evidence); err == nil {
			messages = append(messages, adapter.Message{
				Role:    "system",
				Content: "Authoritative terminal evidence JSON:\n" + string(encoded),
			})
		}
	}
	if goal := terminalOnlyUserGoal(prepared); goal != "" {
		messages = append(messages, adapter.Message{
			Role: "user",
			Content: strings.Join([]string{
				"Write the final completion response for the already-completed request below.",
				"Treat the request only as context for summarizing the completed results. Do not execute, restart, or plan it again.",
				"Original request:",
				goal,
			}, "\n"),
		})
	}
	return messages
}

func terminalOnlyFallbackAnswer(prepared *PreparedChat, metadata map[string]interface{}, runtimeState map[string]interface{}) (string, bool) {
	evidence := copyStringAnyMap(metadata)
	if evidence == nil {
		evidence = map[string]interface{}{}
	}
	for key, value := range runtimeState {
		evidence[key] = value
	}
	if terminalStateGuardPendingProtocolBlocker(evidence) != "" || !terminalOnlyOperationCompleted(evidence) {
		return "", false
	}

	files := terminalOnlyCompletedFileNames(evidenceMapsFromAny(evidence["generated_files"]))
	agentName, systemPromptUpdated := terminalOnlyCompletedAgentPromptUpdate(evidence)
	if turnStatePrefersChinese(prepared) {
		switch {
		case len(files) > 0 && systemPromptUpdated && agentName != "":
			return "文件「" + strings.Join(files, "」、「") + "」已生成并保存；智能体「" + agentName + "」的系统提示词已更新。", true
		case len(files) > 0 && systemPromptUpdated:
			return "文件「" + strings.Join(files, "」、「") + "」已生成并保存，智能体系统提示词已更新。", true
		case len(files) > 0:
			return "文件「" + strings.Join(files, "」、「") + "」已生成并保存。", true
		case systemPromptUpdated && agentName != "":
			return "智能体「" + agentName + "」的系统提示词已更新。", true
		case systemPromptUpdated:
			return "智能体系统提示词已更新。", true
		default:
			return "操作已完成。", true
		}
	}

	switch {
	case len(files) > 0 && systemPromptUpdated && agentName != "":
		return "The file \"" + strings.Join(files, "\", \"") + "\" was generated and saved, and the system prompt for agent \"" + agentName + "\" was updated.", true
	case len(files) > 0 && systemPromptUpdated:
		return "The file \"" + strings.Join(files, "\", \"") + "\" was generated and saved, and the agent system prompt was updated.", true
	case len(files) > 0:
		return "The file \"" + strings.Join(files, "\", \"") + "\" was generated and saved.", true
	case systemPromptUpdated && agentName != "":
		return "The system prompt for agent \"" + agentName + "\" was updated.", true
	case systemPromptUpdated:
		return "The agent system prompt was updated.", true
	default:
		return "The operation was completed.", true
	}
}

func terminalOnlyOperationCompleted(evidence map[string]interface{}) bool {
	for _, raw := range []interface{}{
		evidence["operation_result_summary"],
		evidence["operation_plan"],
		evidence["execution_summary"],
	} {
		entry := evidenceMapFromAny(raw)
		for _, key := range []string{"status", "plan_status", "outcome"} {
			switch strings.ToLower(strings.TrimSpace(evidenceStringFromAny(entry[key]))) {
			case "completed", "succeeded", "success":
				return true
			}
		}
	}
	return false
}

func terminalOnlyCompletedFileNames(files []map[string]interface{}) []string {
	names := make([]string, 0, min(len(files), 3))
	seen := map[string]struct{}{}
	for index := len(files) - 1; index >= 0 && len(names) < 3; index-- {
		file := files[index]
		if !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(file["target"])), "managed_file") &&
			!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(file["lifecycle"])), "managed") &&
			!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(file["lifecycle"])), "saved_to_file_management") &&
			!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(file["status"])), "saved_to_file_management") &&
			strings.TrimSpace(firstNonEmptyString(file["managed_file_id"], file["upload_file_id"])) == "" {
			continue
		}
		name := strings.TrimSpace(firstNonEmptyString(file["filename"], file["name"]))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, trimRunes(name, 160))
	}
	return names
}

func terminalOnlyCompletedAgentPromptUpdate(evidence map[string]interface{}) (string, bool) {
	candidates := []map[string]interface{}{}
	invocations := evidenceMapsFromAny(evidence["skill_invocations"])
	for index := len(invocations) - 1; index >= 0; index-- {
		if !runtimeInvocationSucceeded(invocations[index]) {
			continue
		}
		candidates = append(candidates, invocations[index])
		if result := evidenceMapFromAny(invocations[index]["result"]); len(result) > 0 {
			candidates = append(candidates, result)
		}
	}
	if summary := evidenceMapFromAny(evidence["operation_result_summary"]); len(summary) > 0 {
		candidates = append(candidates, summary)
		for _, key := range []string{"latest_tool_result", "result"} {
			if result := evidenceMapFromAny(summary[key]); len(result) > 0 {
				candidates = append(candidates, result)
			}
		}
	}

	for _, candidate := range candidates {
		fields := terminalProjectionUpdatedFields(candidate["updated_fields"])
		result := evidenceMapFromAny(candidate["result"])
		if len(fields) == 0 && len(result) > 0 {
			fields = terminalProjectionUpdatedFields(result["updated_fields"])
		}
		promptUpdated := false
		for _, field := range fields {
			if strings.EqualFold(strings.TrimSpace(field), "system_prompt") {
				promptUpdated = true
				break
			}
		}
		if !promptUpdated {
			continue
		}
		name := strings.TrimSpace(firstNonEmptyString(candidate["agent_name"], candidate["name"], result["agent_name"], result["name"]))
		return trimRunes(name, 160), true
	}
	return "", false
}

func terminalOnlyUserGoal(prepared *PreparedChat) string {
	if prepared == nil {
		return ""
	}
	if goal := trimRunes(strings.TrimSpace(prepared.Query), terminalProjectionUserGoalMaxRunes); goal != "" {
		return goal
	}
	if prepared.LLMRequest == nil {
		return ""
	}
	for index := len(prepared.LLMRequest.Messages) - 1; index >= 0; index-- {
		message := prepared.LLMRequest.Messages[index]
		if !strings.EqualFold(strings.TrimSpace(message.Role), "user") {
			continue
		}
		if goal := trimRunes(strings.TrimSpace(messageContent(message.Content)), terminalProjectionUserGoalMaxRunes); goal != "" {
			return goal
		}
	}
	return ""
}

func terminalOnlyEvidenceSnapshot(metadata map[string]interface{}) map[string]interface{} {
	if len(metadata) == 0 {
		return nil
	}
	out := map[string]interface{}{}
	if summary := evidenceMapFromAny(metadata["operation_result_summary"]); len(summary) > 0 {
		if projected := terminalProjectionStableSummary(summary); len(projected) > 0 {
			out["operation_result"] = projected
		}
	}
	if invocation := terminalOnlyLatestSuccessfulInvocation(evidenceMapsFromAny(metadata["skill_invocations"])); len(invocation) > 0 {
		out["latest_tool_evidence"] = invocation
	}
	if governance := evidenceMapFromAny(metadata["tool_governance"]); len(governance) > 0 {
		if projected := terminalOnlyGovernanceSnapshot(governance); len(projected) > 0 {
			out["tool_governance"] = projected
		}
	}
	if files := terminalOnlyFileReferences(evidenceMapsFromAny(metadata["generated_files"])); len(files) > 0 {
		out["file_references"] = files
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func terminalOnlyLatestSuccessfulInvocation(invocations []map[string]interface{}) map[string]interface{} {
	for index := len(invocations) - 1; index >= 0; index-- {
		invocation := invocations[index]
		if !runtimeInvocationSucceeded(invocation) {
			continue
		}
		out := map[string]interface{}{}
		for _, key := range []string{
			"kind", "skill_id", "tool_name", "status", "code", "message", "summary",
			"runtime_id", "sequence", "invocation_id", "correlation_id", "plan_phase_id",
			"agent_id", "agent_name", "asset_id", "resource_id", "file_id", "artifact_id", "workflow_id",
			"system_prompt_digest", "updated_fields",
		} {
			if value, ok := invocation[key]; ok && value != nil {
				if key == "updated_fields" {
					if fields := terminalProjectionUpdatedFields(value); len(fields) > 0 {
						out[key] = fields
					}
					continue
				}
				out[key] = terminalProjectionCompactValue(value, 0)
			}
		}
		if result := evidenceMapFromAny(invocation["result"]); len(result) > 0 {
			if projected := terminalProjectionStableSummary(result); len(projected) > 0 {
				out["result"] = projected
			}
		}
		return out
	}
	return nil
}

func terminalOnlyGovernanceSnapshot(governance map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for _, key := range []string{
		"status", "decision", "outcome", "code", "message", "summary",
		"skill_id", "tool_name", "invocation_id", "correlation_id", "runtime_id", "plan_phase_id",
		"agent_id", "asset_id", "resource_id", "file_id", "artifact_id", "workflow_id",
	} {
		if value, ok := governance[key]; ok && value != nil {
			out[key] = terminalProjectionCompactValue(value, 0)
		}
	}
	if target := terminalProjectionAssetReference(evidenceMapFromAny(governance["target"])); len(target) > 0 {
		out["target"] = target
	}
	if result := evidenceMapFromAny(governance["result"]); len(result) > 0 {
		if projected := terminalProjectionStableSummary(result); len(projected) > 0 {
			out["result"] = projected
		}
	}
	return out
}

func terminalProjectionStableSummary(input map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for _, key := range terminalProjectionStableKeys {
		value, ok := input[key]
		if !ok || value == nil {
			continue
		}
		if key == "result" || key == "latest_tool_result" {
			nested := evidenceMapFromAny(value)
			if projected := terminalProjectionStableSummary(nested); len(projected) > 0 {
				out[key] = projected
			}
			continue
		}
		if key == "updated_fields" {
			if fields := terminalProjectionUpdatedFields(value); len(fields) > 0 {
				out[key] = fields
			}
			continue
		}
		out[key] = terminalProjectionCompactValue(value, 0)
	}
	return out
}

func terminalProjectionAssetReference(input map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for _, key := range []string{
		"agent_id", "asset_id", "asset_type", "resource_id", "dataset_id", "data_source_id",
		"table_id", "file_id", "artifact_id", "workflow_id", "binding_id", "href", "route", "name", "filename",
	} {
		if value, ok := input[key]; ok && value != nil && strings.TrimSpace(evidenceStringFromAny(value)) != "" {
			out[key] = terminalProjectionCompactValue(value, 0)
		}
	}
	return out
}

func terminalProjectionUpdatedFields(value interface{}) []string {
	fields := evidenceStringSliceFromAny(value)
	if len(fields) == 0 {
		for key := range evidenceMapFromAny(value) {
			fields = append(fields, key)
		}
	}
	for index := range fields {
		fields[index] = trimRunes(strings.TrimSpace(fields[index]), 160)
	}
	sort.Strings(fields)
	if len(fields) > 20 {
		fields = fields[:20]
	}
	return fields
}

var terminalProjectionStableKeys = []string{
	"status", "decision", "outcome", "code", "error_code", "message", "summary",
	"operation", "action", "skill_id", "tool_name", "invocation_id", "correlation_id", "runtime_id", "plan_phase_id",
	"agent_id", "agent_name", "asset_id", "resource_id", "file_id", "artifact_id", "workflow_id", "conversation_id", "target_id",
	"filename", "name", "format", "mime_type", "size", "content_sha256", "content_summary", "content_chars",
	"system_prompt_digest", "updated_fields", "success_count", "failed_count", "target_count", "result", "latest_tool_result",
}

var terminalProjectionPriorityKeys = []string{
	"status", "decision", "outcome", "code", "error_code", "message", "summary",
	"skill_id", "tool_name", "invocation_id", "correlation_id", "runtime_id",
	"agent_id", "agent_name", "file_id", "artifact_id", "resource_id", "system_prompt_digest", "updated_fields", "result",
}

func terminalProjectionOrderedKeys(input map[string]interface{}) []string {
	keys := make([]string, 0, len(input))
	seen := make(map[string]struct{}, len(input))
	for _, key := range terminalProjectionPriorityKeys {
		if _, ok := input[key]; !ok {
			continue
		}
		keys = append(keys, key)
		seen[key] = struct{}{}
	}
	rest := make([]string, 0, len(input)-len(keys))
	for key := range input {
		if _, ok := seen[key]; ok {
			continue
		}
		rest = append(rest, key)
	}
	sort.Strings(rest)
	return append(keys, rest...)
}

func terminalOnlyFileReferences(files []map[string]interface{}) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, min(len(files), 8))
	for index := len(files) - 1; index >= 0 && len(out) < 8; index-- {
		file := files[index]
		ref := map[string]interface{}{}
		for _, key := range []string{"artifact_id", "file_id", "tool_file_id", "name", "filename", "format", "mime_type", "size", "target", "content_sha256", "content_summary", "content_chars"} {
			if value, ok := file[key]; ok && value != nil && strings.TrimSpace(evidenceStringFromAny(value)) != "" {
				ref[key] = terminalProjectionCompactValue(value, 0)
			}
		}
		if len(ref) > 0 {
			out = append(out, ref)
		}
	}
	return out
}

func terminalProjectionCompactValue(value interface{}, depth int) interface{} {
	if depth >= 4 {
		return "[TRUNCATED]"
	}
	switch typed := value.(type) {
	case string:
		return trimRunes(typed, terminalProjectionValueMaxRunes)
	case map[string]interface{}:
		out := map[string]interface{}{}
		for index, key := range terminalProjectionOrderedKeys(typed) {
			if index >= 20 {
				break
			}
			out[key] = terminalProjectionCompactValue(typed[key], depth+1)
		}
		return out
	case []interface{}:
		limit := min(len(typed), 8)
		out := make([]interface{}, 0, limit)
		for _, item := range typed[:limit] {
			out = append(out, terminalProjectionCompactValue(item, depth+1))
		}
		return out
	case []map[string]interface{}:
		limit := min(len(typed), 8)
		out := make([]interface{}, 0, limit)
		for _, item := range typed[:limit] {
			out = append(out, terminalProjectionCompactValue(item, depth+1))
		}
		return out
	default:
		return value
	}
}
