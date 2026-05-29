package skillloop

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/pkg/logger"
)

func skillCallStartPayload(prepared *PreparedChat, skillID string, toolName string, argumentsSummary map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"conversation_id":   prepared.Conversation.ID.String(),
		"message_id":        prepared.Message.ID.String(),
		"skill_id":          skillID,
		"tool_name":         toolName,
		"arguments":         argumentsSummary,
		"arguments_summary": argumentsSummary,
		"created_at":        time.Now().Unix(),
	}
}

func skillCallEndPayload(prepared *PreparedChat, trace skills.SkillTrace) map[string]interface{} {
	payload := map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"skill_id":        trace.SkillID,
		"tool_name":       trace.ToolName,
		"duration_ms":     trace.DurationMS,
		"status":          trace.Status,
		"created_at":      time.Now().Unix(),
	}
	if trace.Message != "" {
		payload["message"] = trace.Message
	}
	if len(trace.Result) > 0 {
		payload["result"] = trace.Result
	}
	return payload
}

func skillArtifactsFromToolMessages(prepared *PreparedChat, trace skills.SkillTrace, messages []tools.ToolInvokeMessage) []map[string]interface{} {
	artifacts := make([]map[string]interface{}, 0)
	for _, message := range messages {
		if message.Type != tools.ToolInvokeMessageTypeFile || len(message.Meta) == 0 {
			continue
		}
		file, ok := message.Meta["file"].(map[string]interface{})
		if !ok || len(file) == 0 {
			continue
		}
		artifacts = append(artifacts, skillArtifactFromToolFile(prepared, trace, message, file))
	}
	return artifacts
}

func summarizeSkillToolResult(skillID string, toolName string, messages []tools.ToolInvokeMessage) map[string]interface{} {
	if strings.TrimSpace(skillID) != "user-memory" {
		return nil
	}
	payload := firstJSONToolPayload(messages)
	switch strings.TrimSpace(toolName) {
	case "add_user_memory", "update_user_memory":
		return compactMemoryEntryResult(payload)
	case "delete_user_memory":
		return compactMemoryFields(payload, "result", "entry_id")
	case "read_user_memory", "list_temporary_memories":
		return map[string]interface{}{
			"entries_count": len(interfaceSlice(payload["entries"])),
		}
	default:
		return nil
	}
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
	return compactMemoryFields(payload, "id", "entry_id", "content", "category", "memory_type", "expires_at", "status", "enabled")
}

func compactMemoryFields(payload map[string]interface{}, keys ...string) map[string]interface{} {
	if len(payload) == 0 {
		return nil
	}
	result := make(map[string]interface{}, len(keys))
	for _, key := range keys {
		if value, ok := payload[key]; ok && value != nil && value != "" {
			result[key] = value
		}
	}
	return result
}

func interfaceSlice(value interface{}) []interface{} {
	switch typed := value.(type) {
	case []interface{}:
		return typed
	default:
		return nil
	}
}

func skillArtifactFromToolFile(prepared *PreparedChat, trace skills.SkillTrace, message tools.ToolInvokeMessage, file map[string]interface{}) map[string]interface{} {
	url := firstNonEmptyString(file["url"], message.Text)
	downloadURL := firstNonEmptyString(file["download_url"])
	if downloadURL == "" {
		downloadURL = appendDownloadQuery(url)
	}
	artifact := map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
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

func skillCallErrorPayload(prepared *PreparedChat, trace skills.SkillTrace) map[string]interface{} {
	return map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"skill_id":        trace.SkillID,
		"tool_name":       trace.ToolName,
		"duration_ms":     trace.DurationMS,
		"status":          runtimemodel.MessageStatusError,
		"message":         trace.Error,
		"created_at":      time.Now().Unix(),
	}
}

func skillLoadPayload(prepared *PreparedChat, skillID string) map[string]interface{} {
	return map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"skill_id":        skillID,
		"created_at":      time.Now().Unix(),
	}
}

func skillLoadEndPayload(prepared *PreparedChat, trace skills.SkillTrace) map[string]interface{} {
	return map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"skill_id":        trace.SkillID,
		"duration_ms":     trace.DurationMS,
		"status":          trace.Status,
		"created_at":      time.Now().Unix(),
	}
}

func skillReferenceReadPayload(prepared *PreparedChat, trace skills.SkillTrace, path string) map[string]interface{} {
	return map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"skill_id":        trace.SkillID,
		"path":            path,
		"duration_ms":     trace.DurationMS,
		"status":          trace.Status,
		"created_at":      time.Now().Unix(),
	}
}

func intermediateAnswerPayload(prepared *PreparedChat, trace skills.SkillTrace, answerID string, content string, index int, done bool, status string) map[string]interface{} {
	return map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
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

func (r *Runner) emitSkillError(ctx context.Context, prepared *PreparedChat, trace skills.SkillTrace) {
	r.emitEvent(EventSkillCallError, skillCallErrorPayload(prepared, trace))
}

func (r *Runner) logSkillTrace(ctx context.Context, prepared *PreparedChat, trace skills.SkillTrace) {
	if prepared == nil || prepared.Conversation == nil || prepared.Message == nil {
		return
	}
	fields := []interface{}{
		"conversation_id", prepared.Conversation.ID.String(),
		"message_id", prepared.Message.ID.String(),
		"kind", trace.Kind,
		"skill_id", trace.SkillID,
		"tool_name", trace.ToolName,
		"status", trace.Status,
		"duration_ms", trace.DurationMS,
	}
	if trace.Error != "" {
		fields = append(fields, "error", trace.Error)
	}
	switch trace.Status {
	case "error":
		logger.WarnContext(ctx, "aichat skill step failed", fields...)
	case "blocked":
		logger.DebugContext(ctx, "aichat skill step blocked", fields...)
	default:
		logger.DebugContext(ctx, "aichat skill step completed", fields...)
	}
}

func failedSkillTrace(kind string, toolName string, err error) skills.SkillTrace {
	message := ""
	if err != nil {
		message = err.Error()
	}
	return skills.SkillTrace{
		Kind:     kind,
		ToolName: toolName,
		Status:   "error",
		Error:    message,
	}
}

func skillToolLimitExceededTrace(skillID string, toolName string, args map[string]interface{}, err error) skills.SkillTrace {
	trace := failedSkillTrace("tool_call", toolName, err)
	trace.SkillID = strings.ToLower(strings.TrimSpace(skillID))
	trace.Arguments = summarizeSkillToolArguments(trace.SkillID, toolName, args)
	return trace
}

func blockedSkillGuardrailTrace(skillID string, toolName string, message string) skills.SkillTrace {
	return skills.SkillTrace{
		Kind:     "guardrail",
		SkillID:  strings.TrimSpace(skillID),
		ToolName: strings.TrimSpace(toolName),
		Status:   "blocked",
		Error:    strings.TrimSpace(message),
		Arguments: map[string]interface{}{
			"next_step": "load_skill",
		},
	}
}

func metadataExposedTrace(skillIDs []string, stats skills.SkillMetadataPromptStats) skills.SkillTrace {
	return skills.SkillTrace{
		Kind:   "metadata_exposed",
		Status: "success",
		Arguments: map[string]interface{}{
			"skill_ids":     strings.Join(skillIDs, ","),
			"enabled_count": stats.EnabledCount,
			"exposed_count": stats.ExposedCount,
			"omitted_count": stats.OmittedCount,
			"truncated":     stats.Truncated,
		},
	}
}

func skillDocumentPayload(doc *skills.SkillDocument) map[string]interface{} {
	if doc == nil {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"skill_id":     doc.Metadata.ID,
		"name":         doc.Metadata.Name,
		"description":  doc.Metadata.Description,
		"when_to_use":  doc.Metadata.WhenToUse,
		"instructions": doc.Instructions,
		"references":   doc.Metadata.References,
		"tools":        doc.Metadata.Tools,
	}
}

func errorPayload(err error) map[string]interface{} {
	message := ""
	if err != nil {
		message = err.Error()
	}
	return map[string]interface{}{
		"error": message,
	}
}

func recoverableErrorPayload(err error, nextAction string) map[string]interface{} {
	payload := errorPayload(err)
	payload["recoverable"] = true
	payload["next_action"] = strings.TrimSpace(nextAction)
	return payload
}

func recoverableSkillToolErrorPayload(err error, nextAction string, skillID string, toolName string) map[string]interface{} {
	payload := recoverableErrorPayload(err, nextAction)
	if expected := skills.ExpectedSkillToolArguments(skillID, toolName); expected != nil {
		payload["expected_arguments"] = expected
		payload["next_action"] = strings.TrimSpace(nextAction + ". Retry call_skill_tool with arguments matching expected_arguments.schema")
	}
	return payload
}

func guardrailPayload(trace skills.SkillTrace) map[string]interface{} {
	return map[string]interface{}{
		"error":     trace.Error,
		"status":    trace.Status,
		"skill_id":  trace.SkillID,
		"tool_name": trace.ToolName,
		"next_step": "call load_skill with the same skill_id before reading references or calling skill tools",
	}
}
