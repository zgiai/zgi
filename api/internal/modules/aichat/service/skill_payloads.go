package service

import (
	"context"
	"strings"
	"time"

	aichatmodel "github.com/zgiai/ginext/internal/modules/aichat/model"
	"github.com/zgiai/ginext/internal/modules/skills"
	"github.com/zgiai/ginext/internal/modules/tools"
	"github.com/zgiai/ginext/pkg/logger"
)

func skillCallStartPayload(prepared *PreparedChat, skillID string, toolName string, argumentsSummary map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"conversation_id":   prepared.Conversation.ID.String(),
		"message_id":        prepared.Message.ID.String(),
		"skill_id":          skillID,
		"tool_name":         toolName,
		"arguments_summary": argumentsSummary,
		"created_at":        time.Now().Unix(),
	}
}

func skillCallEndPayload(prepared *PreparedChat, trace skills.SkillTrace) map[string]interface{} {
	return map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"skill_id":        trace.SkillID,
		"tool_name":       trace.ToolName,
		"duration_ms":     trace.DurationMS,
		"status":          trace.Status,
		"created_at":      time.Now().Unix(),
	}
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
		"status":          aichatmodel.MessageStatusError,
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

func (s *service) emitSkillError(ctx context.Context, prepared *PreparedChat, trace skills.SkillTrace, onEvent func(StreamEvent) error) {
	s.emitPreparedEvent(ctx, prepared, streamEventSkillCallError, skillCallErrorPayload(prepared, trace), onEvent)
}

func (s *service) logSkillTrace(ctx context.Context, prepared *PreparedChat, trace skills.SkillTrace) {
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

func metadataExposedTrace(skillIDs []string) skills.SkillTrace {
	return skills.SkillTrace{
		Kind:   "metadata_exposed",
		Status: "success",
		Arguments: map[string]interface{}{
			"skill_ids": strings.Join(skillIDs, ","),
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

func guardrailPayload(trace skills.SkillTrace) map[string]interface{} {
	return map[string]interface{}{
		"error":     trace.Error,
		"status":    trace.Status,
		"skill_id":  trace.SkillID,
		"tool_name": trace.ToolName,
		"next_step": "call load_skill with the same skill_id before reading references or calling skill tools",
	}
}
