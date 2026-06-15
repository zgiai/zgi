package skillloop

import (
	"context"
	"strings"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skilltrace"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/pkg/logger"
)

func skillCallStartPayload(prepared *PreparedChat, skillID string, toolName string, argumentsSummary map[string]interface{}) map[string]interface{} {
	return skilltrace.SkillCallStartPayload(skillTracePayloadIDs(prepared), skillID, toolName, argumentsSummary)
}

func skillCallEndPayload(prepared *PreparedChat, trace skills.SkillTrace) map[string]interface{} {
	return skilltrace.SkillCallEndPayload(skillTracePayloadIDs(prepared), trace, true)
}

func toolGovernanceDecisionPayload(prepared *PreparedChat, trace skills.SkillTrace) map[string]interface{} {
	payload := skilltrace.ToolGovernanceDecisionPayload(skillTracePayloadIDs(prepared), trace)
	return payload
}

func skillArtifactsFromToolMessages(prepared *PreparedChat, trace skills.SkillTrace, messages []tools.ToolInvokeMessage) []map[string]interface{} {
	return skilltrace.SkillArtifactsFromToolMessages(skillTracePayloadIDs(prepared), trace, messages)
}

func summarizeSkillToolResult(skillID string, toolName string, messages []tools.ToolInvokeMessage) map[string]interface{} {
	return skilltrace.SummarizeToolResult(skillID, toolName, messages)
}

func skillCallErrorPayload(prepared *PreparedChat, trace skills.SkillTrace) map[string]interface{} {
	return skilltrace.SkillCallErrorPayload(skillTracePayloadIDs(prepared), trace, runtimemodel.MessageStatusError, true)
}

func skillLoadPayload(prepared *PreparedChat, skillID string) map[string]interface{} {
	return skilltrace.SkillLoadPayload(skillTracePayloadIDs(prepared), skillID)
}

func skillLoadEndPayload(prepared *PreparedChat, trace skills.SkillTrace) map[string]interface{} {
	return skilltrace.SkillLoadEndPayload(skillTracePayloadIDs(prepared), trace)
}

func skillReferenceReadPayload(prepared *PreparedChat, trace skills.SkillTrace, path string) map[string]interface{} {
	return skilltrace.SkillReferenceReadPayload(skillTracePayloadIDs(prepared), trace, path)
}

func intermediateAnswerPayload(prepared *PreparedChat, trace skills.SkillTrace, answerID string, content string, index int, done bool, status string) map[string]interface{} {
	return skilltrace.IntermediateAnswerPayload(skillTracePayloadIDs(prepared), trace, answerID, content, index, done, status)
}

func skillTracePayloadIDs(prepared *PreparedChat) skilltrace.PayloadIDs {
	return skilltrace.PayloadIDs{
		ConversationID: prepared.Conversation.ID.String(),
		MessageID:      prepared.Message.ID.String(),
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

func userInputGuardrailPayload(result FinalAnswerGuardResult, blockedMessage string, questions []map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"error":             strings.TrimSpace(result.Message),
		"status":            "blocked",
		"blocked_tool":      "request_user_input",
		"blocked_message":   strings.TrimSpace(blockedMessage),
		"blocked_questions": userInputQuestionSummaries(questions),
		"skill_id":          strings.TrimSpace(result.SkillID),
		"tool_name":         strings.TrimSpace(result.ToolName),
		"next_step":         "continue planning and call the required skill/tool instead of asking the user to clarify information already resolved in runtime context",
	}
}
