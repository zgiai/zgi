package skilltrace

import (
	"encoding/json"
	"reflect"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

// PayloadIDs identifies the conversation/message pair a skill event belongs to.
type PayloadIDs struct {
	ConversationID string
	MessageID      string
}

// SkillCallStartPayload builds the public skill_call_start event payload.
func SkillCallStartPayload(ids PayloadIDs, skillID string, toolName string, argumentsSummary map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"conversation_id":   ids.ConversationID,
		"message_id":        ids.MessageID,
		"skill_id":          skillID,
		"tool_name":         toolName,
		"arguments":         argumentsSummary,
		"arguments_summary": argumentsSummary,
		"created_at":        time.Now().Unix(),
	}
}

// SkillCallEndPayload builds the public skill_call_end event payload.
func SkillCallEndPayload(ids PayloadIDs, trace skills.SkillTrace, includeKind bool) map[string]interface{} {
	payload := map[string]interface{}{
		"conversation_id": ids.ConversationID,
		"message_id":      ids.MessageID,
		"skill_id":        trace.SkillID,
		"tool_name":       trace.ToolName,
		"duration_ms":     trace.DurationMS,
		"status":          trace.Status,
		"created_at":      time.Now().Unix(),
	}
	if includeKind {
		payload["kind"] = trace.Kind
	}
	if trace.Message != "" {
		payload["message"] = trace.Message
	}
	if len(trace.Result) > 0 {
		payload["result"] = trace.Result
	}
	if trace.Governance != nil {
		payload["governance"] = trace.Governance
	}
	return payload
}

func ToolGovernanceDecisionPayload(ids PayloadIDs, trace skills.SkillTrace) map[string]interface{} {
	payload := map[string]interface{}{
		"conversation_id": ids.ConversationID,
		"message_id":      ids.MessageID,
		"skill_id":        trace.SkillID,
		"tool_name":       trace.ToolName,
		"status":          trace.Status,
		"duration_ms":     trace.DurationMS,
		"created_at":      time.Now().Unix(),
	}
	if trace.Governance != nil {
		payload["governance"] = trace.Governance
		payload["correlation_id"] = trace.Governance.CorrelationID
		payload["decision"] = trace.Governance.Status
		payload["requires_approval"] = trace.Governance.RequiresApproval
		payload["reason"] = trace.Governance.Reason
		payload["risk_level"] = trace.Governance.Manifest.RiskLevel
		payload["effect"] = trace.Governance.Manifest.Effect
		payload["asset_type"] = trace.Governance.Manifest.AssetType
		if len(trace.Governance.AssetOperationAudit) > 0 {
			payload["asset_operation_audit"] = trace.Governance.AssetOperationAudit
		}
		if trace.Governance.ApprovalEvent != nil {
			payload["approval_event"] = trace.Governance.ApprovalEvent
		}
	}
	return payload
}

// SkillCallErrorPayload builds the public skill_call_error event payload.
func SkillCallErrorPayload(ids PayloadIDs, trace skills.SkillTrace, status string, includeKind bool) map[string]interface{} {
	payload := map[string]interface{}{
		"conversation_id": ids.ConversationID,
		"message_id":      ids.MessageID,
		"skill_id":        trace.SkillID,
		"tool_name":       trace.ToolName,
		"duration_ms":     trace.DurationMS,
		"status":          status,
		"message":         trace.Error,
		"created_at":      time.Now().Unix(),
	}
	if includeKind {
		payload["kind"] = trace.Kind
	}
	if trace.Governance != nil {
		payload["governance"] = trace.Governance
	}
	return payload
}

// SkillLoadPayload builds the public skill_load_start event payload.
func SkillLoadPayload(ids PayloadIDs, skillID string) map[string]interface{} {
	return map[string]interface{}{
		"conversation_id": ids.ConversationID,
		"message_id":      ids.MessageID,
		"skill_id":        skillID,
		"created_at":      time.Now().Unix(),
	}
}

// SkillLoadEndPayload builds the public skill_load_end event payload.
func SkillLoadEndPayload(ids PayloadIDs, trace skills.SkillTrace) map[string]interface{} {
	return map[string]interface{}{
		"conversation_id": ids.ConversationID,
		"message_id":      ids.MessageID,
		"skill_id":        trace.SkillID,
		"duration_ms":     trace.DurationMS,
		"status":          trace.Status,
		"created_at":      time.Now().Unix(),
	}
}

// SkillReferenceReadPayload builds the public skill_reference_read event payload.
func SkillReferenceReadPayload(ids PayloadIDs, trace skills.SkillTrace, path string) map[string]interface{} {
	return map[string]interface{}{
		"conversation_id": ids.ConversationID,
		"message_id":      ids.MessageID,
		"skill_id":        trace.SkillID,
		"path":            path,
		"duration_ms":     trace.DurationMS,
		"status":          trace.Status,
		"created_at":      time.Now().Unix(),
	}
}

// IntermediateAnswerPayload builds the public agent_intermediate_answer event payload.
func IntermediateAnswerPayload(ids PayloadIDs, trace skills.SkillTrace, answerID string, content string, index int, done bool, status string) map[string]interface{} {
	return map[string]interface{}{
		"conversation_id": ids.ConversationID,
		"message_id":      ids.MessageID,
		"answer_id":       answerID,
		"title":           trace.Title,
		"content":         content,
		"delta":           true,
		"index":           index,
		"done":            done,
		"status":          status,
		"created_at":      time.Now().Unix(),
	}
}

// SkillArtifactsFromToolMessages extracts public file artifacts from tool messages.
func SkillArtifactsFromToolMessages(ids PayloadIDs, trace skills.SkillTrace, messages []tools.ToolInvokeMessage) []map[string]interface{} {
	artifacts := make([]map[string]interface{}, 0)
	for _, message := range messages {
		if message.Type != tools.ToolInvokeMessageTypeFile || len(message.Meta) == 0 {
			continue
		}
		file, ok := message.Meta["file"].(map[string]interface{})
		if !ok || len(file) == 0 {
			continue
		}
		artifacts = append(artifacts, skillArtifactFromToolFile(ids, trace, message, file))
	}
	return artifacts
}

type resultSummaryBuilder func(toolName string, payload map[string]interface{}) map[string]interface{}

var resultSummaryBuilders = map[string]resultSummaryBuilder{
	skills.SkillUserMemory:        summarizeMemoryResult,
	skills.SkillAgentKnowledge:    summarizeAgentKnowledgeResult,
	skills.SkillInternalKnowledge: summarizeInternalKnowledgeResult,
	skills.SkillInternalDatabase:  summarizeDatabaseResult,
	skills.SkillAgentDatabase:     summarizeDatabaseResult,
	skills.SkillAgentWorkflow:     summarizeWorkflowResult,
	skills.SkillFileReader:        summarizeFileReaderResult,
}

// SummarizeToolResult returns the compact trace-visible result for known skill tools.
func SummarizeToolResult(skillID string, toolName string, messages []tools.ToolInvokeMessage) map[string]interface{} {
	builder := resultSummaryBuilders[strings.TrimSpace(skillID)]
	if builder == nil {
		return nil
	}
	return builder(strings.TrimSpace(toolName), firstJSONToolPayload(messages))
}

func summarizeMemoryResult(toolName string, payload map[string]interface{}) map[string]interface{} {
	switch toolName {
	case "add_user_memory", "update_user_memory":
		return compactMemoryEntryResult(payload)
	case "delete_user_memory":
		return compactFields(payload, "result", "entry_id")
	case "read_user_memory", "list_temporary_memories":
		return map[string]interface{}{
			"entries_count": len(interfaceSlice(payload["entries"])),
		}
	default:
		return nil
	}
}

func summarizeAgentKnowledgeResult(_ string, payload map[string]interface{}) map[string]interface{} {
	return compactKnowledgeRetrieveResult(payload)
}

func summarizeInternalKnowledgeResult(toolName string, payload map[string]interface{}) map[string]interface{} {
	return compactInternalKnowledgeResult(toolName, payload)
}

func summarizeDatabaseResult(toolName string, payload map[string]interface{}) map[string]interface{} {
	return compactDatabaseResult(toolName, payload)
}

func summarizeWorkflowResult(toolName string, payload map[string]interface{}) map[string]interface{} {
	if len(payload) == 0 {
		return nil
	}
	switch strings.TrimSpace(toolName) {
	case "list_agent_workflows":
		result := compactFields(payload, "status")
		result["workflows_count"] = collectionLen(payload["workflows"])
		return result
	case "run_agent_workflow", "get_workflow_run_status":
		return compactFields(payload, "status", "workflow_run_id", "elapsed_time", "output_keys", "primary_output", "error")
	default:
		return nil
	}
}

func summarizeFileReaderResult(toolName string, payload map[string]interface{}) map[string]interface{} {
	if len(payload) == 0 {
		return nil
	}
	switch strings.TrimSpace(toolName) {
	case "list_visible_files":
		result := compactFields(payload, "status", "count", "selected_count")
		if files := compactFileReaderFiles(payload["files"]); len(files) > 0 {
			result["files"] = files
		}
		return result
	case "read_file":
		result := compactFields(payload, "status", "max_chars", "content_status", "content_chars", "content_truncated", "from_cache")
		if file := recordFromAny(payload["file"]); len(file) > 0 {
			for _, field := range []string{"id", "name", "workspace_id", "extension", "mime_type", "size"} {
				if value, ok := file[field]; ok {
					result["file_"+field] = value
				}
			}
		}
		if content := strings.TrimSpace(stringFromAny(payload["content"])); content != "" {
			result["content_returned_chars"] = len([]rune(content))
			result["content_redacted"] = true
		} else if _, exists := payload["content"]; exists {
			result["content_redacted"] = true
		}
		if contentError := strings.TrimSpace(stringFromAny(payload["content_error"])); contentError != "" {
			result["content_error_chars"] = len([]rune(contentError))
			result["content_error_present"] = true
		}
		return result
	case "delete_file":
		result := compactFields(payload, "status", "deleted_count", "reversible", "error")
		if file := recordFromAny(payload["file"]); len(file) > 0 {
			for _, field := range []string{"id", "name", "workspace_id", "extension", "mime_type", "size"} {
				if value, ok := file[field]; ok {
					result["file_"+field] = value
				}
			}
		}
		return result
	default:
		return nil
	}
}

func compactFileReaderFiles(value interface{}) []map[string]interface{} {
	files := recordsFromAny(value)
	if len(files) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, min(len(files), 20))
	for index, file := range files {
		if index >= 20 {
			break
		}
		item := map[string]interface{}{}
		for _, field := range []string{"visible_index", "id", "file_id", "name", "workspace_id", "extension", "mime_type", "file_type", "size", "selected", "content_status", "content_chars", "content_truncated", "from_cache"} {
			if value, ok := file[field]; ok && value != nil && value != "" {
				item[field] = value
			}
		}
		if content := strings.TrimSpace(stringFromAny(file["content"])); content != "" {
			item["content_returned_chars"] = len([]rune(content))
			item["content_redacted"] = true
		} else if _, exists := file["content"]; exists {
			item["content_redacted"] = true
		}
		if preview := strings.TrimSpace(stringFromAny(file["content_preview"])); preview != "" {
			item["content_preview_chars"] = len([]rune(preview))
			item["content_preview_redacted"] = true
		}
		if len(item) > 0 {
			out = append(out, item)
		}
	}
	return out
}

func compactKnowledgeRetrieveResult(payload map[string]interface{}) map[string]interface{} {
	if len(payload) == 0 {
		return nil
	}
	return compactFields(payload, "query", "status", "result_count", "top_score", "source_summary", "warnings")
}

func compactInternalKnowledgeResult(toolName string, payload map[string]interface{}) map[string]interface{} {
	switch strings.TrimSpace(toolName) {
	case "list_accessible_knowledge_bases":
		return compactFields(payload, "query", "status", "result_count", "fallback_used", "limit", "warnings")
	case "retrieve_knowledge":
		return compactKnowledgeRetrieveResult(payload)
	default:
		return nil
	}
}

func compactDatabaseResult(toolName string, payload map[string]interface{}) map[string]interface{} {
	if len(payload) == 0 {
		return nil
	}
	switch strings.TrimSpace(toolName) {
	case "list_accessible_databases":
		result := compactFields(payload, "query", "limit")
		result["databases_count"] = collectionLen(payload["databases"])
		return result
	case "list_database_tables":
		result := compactDatabaseResourceResult(payload)
		result["tables_count"] = collectionLen(payload["tables"])
		return result
	case "describe_database_table":
		result := compactDatabaseResourceResult(payload)
		result["columns_count"] = collectionLen(payload["columns"])
		return result
	case "query_table_records":
		result := compactDatabaseResourceResult(payload)
		copyCompactField(result, payload, "has_more")
		copyCompactField(result, payload, "total_num")
		result["records_count"] = collectionLen(payload["records"])
		return result
	case "insert_table_records", "update_table_records", "delete_table_records":
		result := compactDatabaseResourceResult(payload)
		copyCompactField(result, payload, "affected_rows")
		return result
	default:
		return nil
	}
}

func compactDatabaseResourceResult(payload map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{}
	dataSource := recordFromAny(payload["data_source"])
	table := recordFromAny(payload["table"])
	if name := firstNonEmptyString(dataSource["name"]); name != "" {
		result["database_name"] = name
	}
	if schemaName := firstNonEmptyString(dataSource["schema_name"]); schemaName != "" {
		result["schema_name"] = schemaName
	}
	if tableName := firstNonEmptyString(table["name"]); tableName != "" {
		result["table_name"] = tableName
	}
	return result
}

func firstJSONToolPayload(messages []tools.ToolInvokeMessage) map[string]interface{} {
	for _, message := range messages {
		if len(message.Data) > 0 {
			return message.Data
		}
		if strings.TrimSpace(message.Text) == "" {
			continue
		}
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(message.Text), &payload); err == nil && len(payload) > 0 {
			return payload
		}
	}
	return map[string]interface{}{}
}

func compactMemoryEntryResult(payload map[string]interface{}) map[string]interface{} {
	return compactFields(payload, "id", "entry_id", "content", "category", "memory_type", "expires_at", "status", "enabled")
}

func compactFields(payload map[string]interface{}, keys ...string) map[string]interface{} {
	if len(payload) == 0 {
		return nil
	}
	result := make(map[string]interface{}, len(keys))
	for _, key := range keys {
		copyCompactField(result, payload, key)
	}
	return result
}

func copyCompactField(result map[string]interface{}, payload map[string]interface{}, key string) {
	if value, ok := payload[key]; ok && value != nil && value != "" {
		result[key] = value
	}
}

func recordFromAny(value interface{}) map[string]interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return typed
	default:
		return map[string]interface{}{}
	}
}

func recordsFromAny(value interface{}) []map[string]interface{} {
	switch typed := value.(type) {
	case []map[string]interface{}:
		return typed
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			if record, ok := item.(map[string]interface{}); ok {
				out = append(out, record)
			}
		}
		return out
	default:
		return nil
	}
}

func interfaceSlice(value interface{}) []interface{} {
	switch typed := value.(type) {
	case []interface{}:
		return typed
	default:
		return nil
	}
}

func collectionLen(value interface{}) int {
	if value == nil {
		return 0
	}
	typed := reflect.ValueOf(value)
	switch typed.Kind() {
	case reflect.Array, reflect.Slice:
		return typed.Len()
	default:
		return 0
	}
}

func skillArtifactFromToolFile(ids PayloadIDs, trace skills.SkillTrace, message tools.ToolInvokeMessage, file map[string]interface{}) map[string]interface{} {
	url := firstNonEmptyString(file["url"], message.Text)
	downloadURL := firstNonEmptyString(file["download_url"])
	if downloadURL == "" {
		downloadURL = appendDownloadQuery(url)
	}
	artifact := map[string]interface{}{
		"conversation_id": ids.ConversationID,
		"message_id":      ids.MessageID,
		"artifact_type":   "file",
		"skill_id":        trace.SkillID,
		"tool_name":       trace.ToolName,
		"file_id":         firstNonEmptyString(file["id"], file["related_id"]),
		"filename":        stringFromAny(file["filename"]),
		"extension":       stringFromAny(file["extension"]),
		"mime_type":       stringFromAny(file["mime_type"]),
		"size":            file["size"],
		"url":             url,
		"download_url":    downloadURL,
		"transfer_method": stringFromAny(file["transfer_method"]),
		"created_at":      time.Now().Unix(),
	}
	if fileType := stringFromAny(file["type"]); fileType != "" {
		artifact["file_type"] = fileType
	}
	return artifact
}

func firstNonEmptyString(values ...interface{}) string {
	for _, value := range values {
		if text := strings.TrimSpace(stringFromAny(value)); text != "" {
			return text
		}
	}
	return ""
}

func appendDownloadQuery(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" || strings.Contains(rawURL, "download=1") {
		return rawURL
	}
	if strings.Contains(rawURL, "?") {
		return rawURL + "&download=1"
	}
	return rawURL + "?download=1"
}

func stringFromAny(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case *string:
		if typed == nil {
			return ""
		}
		return *typed
	default:
		return ""
	}
}
