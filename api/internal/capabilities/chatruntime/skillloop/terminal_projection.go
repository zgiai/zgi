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
		messages = append(messages, adapter.Message{Role: "user", Content: goal})
	}
	return messages
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
			"agent_id", "asset_id", "resource_id", "file_id", "artifact_id", "workflow_id",
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
	"agent_id", "asset_id", "resource_id", "file_id", "artifact_id", "workflow_id", "conversation_id", "target_id",
	"filename", "name", "format", "mime_type", "size", "content_sha256", "content_summary", "content_chars",
	"system_prompt_digest", "updated_fields", "success_count", "failed_count", "target_count", "result", "latest_tool_result",
}

var terminalProjectionPriorityKeys = []string{
	"status", "decision", "outcome", "code", "error_code", "message", "summary",
	"skill_id", "tool_name", "invocation_id", "correlation_id", "runtime_id",
	"agent_id", "file_id", "artifact_id", "resource_id", "system_prompt_digest", "updated_fields", "result",
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
