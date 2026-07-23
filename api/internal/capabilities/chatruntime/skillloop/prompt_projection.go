package skillloop

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const materializedContentProjectionMinRunes = 1024

type promptProjectionStats struct {
	refs         []string
	removedRunes int
}

func projectMaterializedFileContent(messages []adapter.Message, callID string, result map[string]interface{}) ([]adapter.Message, promptProjectionStats) {
	content, ok := generatedFileContentForCall(messages, callID)
	if !ok || len([]rune(content)) < materializedContentProjectionMinRunes {
		return messages, promptProjectionStats{}
	}
	ref := materializedFileReference(result)
	digest := sha256.Sum256([]byte(content))
	marker := fmt.Sprintf(
		"[materialized file content omitted from later model context; ref=%s; chars=%d; sha256:%x; summary=%q]",
		ref,
		len([]rune(content)),
		digest[:],
		materializedContentSummary(content),
	)
	stats := promptProjectionStats{refs: []string{ref}}
	for messageIndex := range messages {
		message := &messages[messageIndex]
		for toolIndex := range message.ToolCalls {
			call := &message.ToolCalls[toolIndex]
			arguments, valid := parseToolArguments(call.Function.Arguments)
			if !valid {
				continue
			}
			changed, removed := projectMatchingMaterializedStrings(arguments, content, marker)
			if !changed {
				continue
			}
			encoded, err := json.Marshal(arguments)
			if err != nil {
				continue
			}
			call.Function.Arguments = string(encoded)
			stats.removedRunes += removed
		}
		if !strings.EqualFold(strings.TrimSpace(message.Role), "tool") {
			continue
		}
		text, ok := message.Content.(string)
		if !ok || !strings.Contains(text, content) {
			continue
		}
		var payload interface{}
		if err := json.Unmarshal([]byte(text), &payload); err != nil {
			continue
		}
		changed, removed := projectMatchingMaterializedStrings(payload, content, marker)
		if !changed {
			continue
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			continue
		}
		message.Content = string(encoded)
		stats.removedRunes += removed
	}
	return messages, stats
}

func generatedFileContentForCall(messages []adapter.Message, callID string) (string, bool) {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return "", false
	}
	for messageIndex := len(messages) - 1; messageIndex >= 0; messageIndex-- {
		for _, call := range messages[messageIndex].ToolCalls {
			if strings.TrimSpace(call.ID) != callID || !strings.EqualFold(strings.TrimSpace(call.Function.Name), skills.MetaToolCallSkillTool) {
				continue
			}
			arguments, ok := parseToolArguments(call.Function.Arguments)
			if !ok || !strings.EqualFold(strings.TrimSpace(stringFromInterface(arguments["skill_id"])), skills.SkillFileGenerator) {
				return "", false
			}
			toolName := strings.TrimSpace(stringFromInterface(arguments["tool_name"]))
			switch toolName {
			case "generate_file", "generate_docx", "generate_pdf", "generate_pptx":
			default:
				return "", false
			}
			toolArgs := mapArg(arguments, "arguments")
			content, _ := toolArgs["content"].(string)
			return content, strings.TrimSpace(content) != ""
		}
	}
	return "", false
}

func parseToolArguments(raw string) (map[string]interface{}, bool) {
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &payload); err != nil || payload == nil {
		return nil, false
	}
	return payload, true
}

func projectMatchingMaterializedStrings(value interface{}, content string, marker string) (bool, int) {
	switch typed := value.(type) {
	case map[string]interface{}:
		changed := false
		removed := 0
		for key, item := range typed {
			if text, ok := item.(string); ok && text == content {
				typed[key] = marker
				changed = true
				removed += max(0, len([]rune(text))-len([]rune(marker)))
				continue
			}
			itemChanged, itemRemoved := projectMatchingMaterializedStrings(item, content, marker)
			changed = changed || itemChanged
			removed += itemRemoved
		}
		return changed, removed
	case []interface{}:
		changed := false
		removed := 0
		for index, item := range typed {
			if text, ok := item.(string); ok && text == content {
				typed[index] = marker
				changed = true
				removed += max(0, len([]rune(text))-len([]rune(marker)))
				continue
			}
			itemChanged, itemRemoved := projectMatchingMaterializedStrings(item, content, marker)
			changed = changed || itemChanged
			removed += itemRemoved
		}
		return changed, removed
	default:
		return false, 0
	}
}

func materializedFileReference(result map[string]interface{}) string {
	for _, key := range []string{"managed_file_id", "upload_file_id", "tool_file_id", "file_id", "artifact_id"} {
		if value := strings.TrimSpace(stringFromInterface(result[key])); value != "" {
			return key + ":" + value
		}
	}
	if filename := strings.TrimSpace(firstNonEmptyString(result["filename"], result["name"])); filename != "" {
		return "filename:" + filename
	}
	return "generated_file"
}

func materializedContentSummary(content string) string {
	return trimRunes(strings.Join(strings.Fields(content), " "), 160)
}

func appendUniqueProjectionRefs(current []string, values ...string) []string {
	seen := make(map[string]struct{}, len(current)+len(values))
	for _, value := range current {
		seen[value] = struct{}{}
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		current = append(current, value)
	}
	return current
}

func enrichGeneratedArtifactContentMetadata(artifact map[string]interface{}, trace skills.SkillTrace) map[string]interface{} {
	if len(artifact) == 0 || !isFileGeneratorTool(trace.SkillID, trace.ToolName) {
		return artifact
	}
	enriched := copyStringAnyMap(artifact)
	for _, key := range []string{"content_chars", "content_sha256", "content_summary"} {
		if value, ok := trace.Arguments[key]; ok && value != nil {
			enriched[key] = value
		}
	}
	return enriched
}

func managedFileArtifactFromSaveResult(trace skills.SkillTrace, messages []tools.ToolInvokeMessage) map[string]interface{} {
	if !strings.EqualFold(strings.TrimSpace(trace.SkillID), skills.SkillFileManager) ||
		!strings.EqualFold(strings.TrimSpace(trace.ToolName), "save_file_to_management") {
		return nil
	}
	payload := firstJSONToolInvokePayload(messages)
	if len(payload) == 0 || !strings.EqualFold(strings.TrimSpace(stringFromInterface(payload["target"])), "managed_file") {
		return nil
	}
	artifact := map[string]interface{}{}
	for _, key := range []string{
		"file_id", "upload_file_id", "filename", "mime_type", "target", "workspace_id",
		"folder_id", "transfer_method", "source_type", "source_file_id", "source_tool_file_id",
	} {
		if value, ok := payload[key]; ok && value != nil && strings.TrimSpace(stringFromInterface(value)) != "" {
			artifact[key] = value
		}
	}
	for _, key := range []string{"size", "created_at", "expires_at"} {
		if value, ok := payload[key]; ok && value != nil {
			artifact[key] = value
		}
	}
	if strings.TrimSpace(stringFromInterface(artifact["source_tool_file_id"])) == "" {
		if sourceToolFileID := strings.TrimSpace(firstNonEmptyString(
			stringFromInterface(trace.Arguments["tool_file_id"]),
			stringFromInterface(trace.Arguments["source_tool_file_id"]),
			stringFromInterface(trace.Arguments["source_file_id"]),
		)); sourceToolFileID != "" && sourceToolFileID != strings.TrimSpace(stringFromInterface(artifact["file_id"])) {
			artifact["source_tool_file_id"] = sourceToolFileID
		}
	}
	if trace.Governance != nil {
		if correlationID := strings.TrimSpace(trace.Governance.CorrelationID); correlationID != "" {
			artifact["correlation_id"] = correlationID
			artifact["operation_id"] = "tool_governance:" + correlationID
		}
		if len(trace.Governance.AssetOperationAudit) > 0 {
			artifact["asset_operation_audit"] = trace.Governance.AssetOperationAudit
		}
	}
	artifact["skill_id"] = trace.SkillID
	artifact["tool_name"] = trace.ToolName
	return artifact
}
