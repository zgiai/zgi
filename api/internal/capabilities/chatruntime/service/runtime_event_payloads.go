package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skilltrace"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/pkg/logger"
)

func skillCallStartPayload(prepared *PreparedChat, invocationID string, skillID string, toolName string, argumentsSummary map[string]interface{}) map[string]interface{} {
	return skilltrace.SkillCallStartPayload(skillTracePayloadIDs(prepared), invocationID, skillID, toolName, argumentsSummary)
}

func skillCallEndPayload(prepared *PreparedChat, trace skills.SkillTrace) map[string]interface{} {
	return skilltrace.SkillCallEndPayload(skillTracePayloadIDs(prepared), trace, true)
}

func clientActionRequiredPayload(prepared *PreparedChat, trace skills.SkillTrace, callID string) map[string]interface{} {
	if payload := routeNavigationClientActionRequiredPayload(prepared, trace, callID); len(payload) > 0 {
		return payload
	}
	if payload := agentManagementRouteNavigationClientActionRequiredPayload(prepared, trace, callID); len(payload) > 0 {
		return payload
	}
	if isNonBlockingAgentManagementMutation(trace) {
		return nil
	}
	if payload := assetObservationClientActionRequiredPayload(prepared, trace, callID); len(payload) > 0 {
		return payload
	}
	return nil
}
func enrichSkillTraceResultFromMessages(trace skills.SkillTrace, messages []tools.ToolInvokeMessage) skills.SkillTrace {
	summary := summarizeSkillToolResult(trace.SkillID, trace.ToolName, messages)
	if len(summary) == 0 {
		return trace
	}
	if trace.Result == nil {
		trace.Result = summary
		return trace
	}
	for key, value := range summary {
		if _, exists := trace.Result[key]; !exists {
			trace.Result[key] = value
		}
	}
	return trace
}

func routeNavigationClientActionRequiredPayload(prepared *PreparedChat, trace skills.SkillTrace, callID string) map[string]interface{} {
	if !strings.EqualFold(strings.TrimSpace(trace.SkillID), skills.SkillConsoleNavigator) ||
		!strings.EqualFold(strings.TrimSpace(trace.ToolName), "navigate") {
		return nil
	}
	result := trace.Result
	if strings.TrimSpace(stringFromAny(result["event_type"])) != "page_navigation_requested" {
		return nil
	}
	href := strings.TrimSpace(stringFromAny(result["href"]))
	if href == "" {
		return nil
	}
	actionID := strings.TrimSpace(callID)
	if actionID == "" {
		actionID = href
	}
	actionID = "route_navigation:" + actionID
	payload := withRuntimePayloadTimestamp(map[string]interface{}{
		"conversation_id":     prepared.Conversation.ID.String(),
		"message_id":          prepared.Message.ID.String(),
		"action_id":           actionID,
		"action_type":         "route_navigation",
		"event_type":          "client_action_required",
		"status":              "waiting_client_action",
		"continuation_policy": clientActionContinuationPolicyResumeModel,
		"blocking":            true,
		"skill_id":            strings.TrimSpace(trace.SkillID),
		"tool_name":           strings.TrimSpace(trace.ToolName),
		"href":                href,
		"result":              copyStringAnyMap(result),
	})
	if label := strings.TrimSpace(stringFromAny(result["label"])); label != "" {
		payload["label"] = label
	}
	if reason := strings.TrimSpace(stringFromAny(result["reason"])); reason != "" {
		payload["reason"] = reason
	}
	return payload
}

func agentManagementRouteNavigationClientActionRequiredPayload(prepared *PreparedChat, trace skills.SkillTrace, callID string) map[string]interface{} {
	if prepared == nil || prepared.parts == nil || !strings.EqualFold(strings.TrimSpace(trace.SkillID), skills.SkillAgentManagement) {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(trace.Status), "success") {
		return nil
	}
	href := ""
	label := ""
	labelKey := ""
	routeKind := ""
	reason := ""
	switch strings.TrimSpace(trace.ToolName) {
	case "delete_agent":
		if !deletedAgentIsCurrentDetailPage(prepared, trace) {
			return nil
		}
		href = normalizeConsoleNavigationGuardHref(firstNonEmptyString(
			stringFromAny(trace.Result["route_after_delete"]),
			stringFromAny(trace.Result["href"]),
			"/console/agents",
		))
		label = "Agent list"
		labelKey = "agentList"
		routeKind = "agent_list"
		reason = "leave_deleted_agent_detail"
	default:
		return nil
	}
	if href == "" {
		return nil
	}
	actionID := strings.TrimSpace(callID)
	if actionID == "" {
		actionID = strings.TrimSpace(trace.SkillID) + ":" + strings.TrimSpace(trace.ToolName) + ":" + href
	}
	actionID = "route_navigation:" + actionID
	result := map[string]interface{}{
		"event_type": "page_navigation_requested",
		"href":       href,
		"label":      label,
		"label_key":  labelKey,
		"route_kind": routeKind,
		"reason":     reason,
	}
	return withRuntimePayloadTimestamp(map[string]interface{}{
		"conversation_id":     prepared.Conversation.ID.String(),
		"message_id":          prepared.Message.ID.String(),
		"action_id":           actionID,
		"action_type":         "route_navigation",
		"event_type":          "client_action_required",
		"status":              "waiting_client_action",
		"continuation_policy": clientActionContinuationPolicyResumeModel,
		"blocking":            true,
		"skill_id":            skills.SkillConsoleNavigator,
		"tool_name":           "navigate",
		"href":                href,
		"label":               label,
		"label_key":           labelKey,
		"route_kind":          routeKind,
		"reason":              reason,
		"result":              result,
	})
}

func deletedAgentIsCurrentDetailPage(prepared *PreparedChat, trace skills.SkillTrace) bool {
	if prepared == nil || prepared.parts == nil {
		return false
	}
	currentHref := normalizeAgentDetailHref(contextualTurnCurrentPage(prepared.parts))
	if currentHref == "" {
		return false
	}
	deletedHref := agentDetailHrefFromTrace(trace)
	if deletedHref == "" {
		if agentID := strings.TrimSpace(firstNonEmptyString(
			stringFromAny(trace.Result["agent_id"]),
			stringFromAny(trace.Result["id"]),
			skillToolCallArgumentString(trace.Arguments, "agent_id"),
			skillToolCallArgumentString(trace.Arguments, "id"),
		)); agentID != "" {
			deletedHref = consoleAgentDetailHref(agentID)
		}
	}
	return deletedHref != "" && consoleNavigationLoadedHrefMatchesTarget(currentHref, deletedHref)
}

func isNonBlockingAgentManagementMutation(trace skills.SkillTrace) bool {
	if !strings.EqualFold(strings.TrimSpace(trace.SkillID), skills.SkillAgentManagement) ||
		!strings.EqualFold(strings.TrimSpace(trace.Status), "success") {
		return false
	}
	switch strings.TrimSpace(trace.ToolName) {
	case "update_agent_identity", "update_agent_config", "replace_agent_memory_blocks", "replace_agent_memory_slots", "replace_agent_skill_bindings", "replace_agent_knowledge_bindings", "replace_agent_database_bindings", "replace_agent_workflow_bindings":
		return true
	default:
		return false
	}
}

func agentDetailHrefFromTrace(trace skills.SkillTrace) string {
	if href := agentDetailHrefFromTraceResult(trace.Result); href != "" {
		return href
	}
	if trace.Governance != nil {
		for _, asset := range trace.Governance.Assets {
			if !strings.EqualFold(strings.TrimSpace(asset.Type), "agent") {
				continue
			}
			if href := normalizeAgentDetailHref(firstNonEmptyString(
				stringFromAny(asset.Metadata["href"]),
				stringFromAny(asset.Metadata["detail_href"]),
			)); href != "" {
				return href
			}
			if agentID := strings.TrimSpace(asset.ID); agentID != "" {
				return consoleAgentDetailHref(agentID)
			}
		}
	}
	if agentID := strings.TrimSpace(firstNonEmptyString(
		stringFromAny(trace.Arguments["agent_id"]),
		stringFromAny(trace.Arguments["agentId"]),
	)); agentID != "" {
		return consoleAgentDetailHref(agentID)
	}
	return ""
}

func agentDetailHrefFromTraceResult(result map[string]interface{}) string {
	href := normalizeAgentDetailHref(firstNonEmptyString(
		stringFromAny(result["href"]),
		stringFromAny(result["detail_href"]),
	))
	if href != "" {
		return href
	}
	if agent := governanceMapFromAny(result["agent"]); len(agent) > 0 {
		href = normalizeAgentDetailHref(firstNonEmptyString(
			stringFromAny(agent["href"]),
			stringFromAny(agent["detail_href"]),
		))
		if href != "" {
			return href
		}
		if agentID := strings.TrimSpace(firstNonEmptyString(
			stringFromAny(agent["agent_id"]),
			stringFromAny(agent["id"]),
		)); agentID != "" {
			return consoleAgentDetailHref(agentID)
		}
	}
	if agentID := strings.TrimSpace(firstNonEmptyString(
		stringFromAny(result["agent_id"]),
		stringFromAny(result["id"]),
	)); agentID != "" {
		return consoleAgentDetailHref(agentID)
	}
	return ""
}

func assetObservationClientActionRequiredPayload(prepared *PreparedChat, trace skills.SkillTrace, callID string) map[string]interface{} {
	if !strings.EqualFold(strings.TrimSpace(trace.Status), "success") {
		return nil
	}
	if isTemporaryFileGenerationTrace(trace) {
		return nil
	}
	audit := assetOperationAuditFromTrace(trace)
	if len(audit) == 0 {
		return nil
	}
	effect := normalizeClientActionToken(firstNonEmptyPayloadText(
		audit["effect"],
		governanceManifestEffect(trace),
	))
	if !requiresAssetObservation(effect) {
		return nil
	}
	assetType := normalizeClientActionToken(firstNonEmptyPayloadText(
		audit["asset_type"],
		governanceManifestAssetType(trace),
	))
	if assetType == "" {
		assetType = "asset"
	}
	actionID := firstNonEmptyPayloadText(audit["correlation_id"], callID, trace.SkillID+":"+trace.ToolName)
	actionID = "asset_observation:" + actionID
	payload := withRuntimePayloadTimestamp(map[string]interface{}{
		"conversation_id":       prepared.Conversation.ID.String(),
		"message_id":            prepared.Message.ID.String(),
		"action_id":             actionID,
		"action_type":           "asset_observation",
		"event_type":            "client_action_required",
		"status":                clientActionStatusSucceeded,
		"continuation_policy":   clientActionContinuationPolicyRecordOnly,
		"blocking":              false,
		"skill_id":              strings.TrimSpace(trace.SkillID),
		"tool_name":             strings.TrimSpace(trace.ToolName),
		"effect":                effect,
		"asset_type":            assetType,
		"asset_operation_audit": copyStringAnyMap(audit),
		"observation_requested": true,
		// The call that changed the asset is the row; the observation only
		// refreshes it (ADR 0050).
		"presentation_visibility": "internal",
		"refresh_before_resume":   false,
	})
	if correlationID := strings.TrimSpace(payloadValueText(audit["correlation_id"])); correlationID != "" {
		payload["correlation_id"] = correlationID
	}
	if toolID := firstNonEmptyPayloadText(audit["tool_id"], governanceManifestToolID(trace)); toolID != "" {
		payload["tool_id"] = toolID
	}
	if assets := firstNonEmptyAssetRefs(audit["assets"], audit["expected_assets"], governanceAssets(trace)); assets != nil {
		payload["assets"] = assets
	}
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

func skillTracePayloadIDs(prepared *PreparedChat) skilltrace.PayloadIDs {
	return skilltrace.PayloadIDs{
		ConversationID: prepared.Conversation.ID.String(),
		MessageID:      prepared.Message.ID.String(),
	}
}

func withRuntimePayloadTimestamp(payload map[string]interface{}) map[string]interface{} {
	now := time.Now()
	payload["created_at"] = now.Unix()
	payload["created_at_ms"] = now.UnixMilli()
	return payload
}

func assetOperationAuditFromTrace(trace skills.SkillTrace) map[string]interface{} {
	if trace.Governance != nil && len(trace.Governance.AssetOperationAudit) > 0 {
		return copyStringAnyMap(trace.Governance.AssetOperationAudit)
	}
	if audit, ok := trace.Result["asset_operation_audit"].(map[string]interface{}); ok && len(audit) > 0 {
		return copyStringAnyMap(audit)
	}
	return nil
}

func isTemporaryFileGenerationTrace(trace skills.SkillTrace) bool {
	return skilltrace.TraceLooksLikeTemporaryFileArtifact(trace)
}

func requiresAssetObservation(effect string) bool {
	switch normalizeClientActionToken(effect) {
	case "create", "update", "delete", "publish":
		return true
	default:
		return false
	}
}

func normalizeClientActionToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func firstNonEmptyPayloadText(values ...interface{}) string {
	for _, value := range values {
		if text := strings.TrimSpace(payloadValueText(value)); text != "" {
			return text
		}
	}
	return ""
}

func payloadValueText(value interface{}) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprint(value)
	}
}

func governanceManifestEffect(trace skills.SkillTrace) string {
	if trace.Governance == nil {
		return ""
	}
	return string(trace.Governance.Manifest.Effect)
}

func governanceManifestAssetType(trace skills.SkillTrace) string {
	if trace.Governance == nil {
		return ""
	}
	return strings.TrimSpace(trace.Governance.Manifest.AssetType)
}

func governanceManifestToolID(trace skills.SkillTrace) string {
	if trace.Governance == nil {
		return ""
	}
	return strings.TrimSpace(trace.Governance.Manifest.ToolID)
}

func governanceAssets(trace skills.SkillTrace) interface{} {
	if trace.Governance == nil || len(trace.Governance.Assets) == 0 {
		return nil
	}
	return trace.Governance.Assets
}

func firstNonEmptyAssetRefs(values ...interface{}) interface{} {
	for _, value := range values {
		if value == nil {
			continue
		}
		switch typed := value.(type) {
		case []interface{}:
			if len(typed) > 0 {
				return typed
			}
		case []map[string]interface{}:
			if len(typed) > 0 {
				return mapsToInterfaceSlice(typed)
			}
		default:
			return value
		}
	}
	return nil
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

func recoverableSkillToolErrorPayload(err error, nextAction string, skillID string, toolName string, resolvedSkills ...*skills.ResolvedSkills) map[string]interface{} {
	payload := recoverableErrorPayload(err, nextAction)
	var surfaceExpected map[string]interface{}
	if len(resolvedSkills) > 0 && resolvedSkills[0] != nil {
		surfaceExpected = skills.ExpectedSkillToolArgumentsForResolved(resolvedSkills[0], skillID, toolName)
	} else {
		surfaceExpected = skills.ExpectedSkillToolArguments(skillID, toolName)
	}
	if recovery := skills.PublicToolErrorRecoveryForInvocation(err, skillID, toolName, surfaceExpected); len(recovery) > 0 {
		for key, value := range recovery {
			payload[key] = value
		}
		if code, ok := recovery["error_code"].(string); ok && strings.TrimSpace(code) != "" {
			payload["error"] = strings.TrimSpace(code)
		}
		if retryAction, ok := recovery["retry_action"].(string); ok && strings.TrimSpace(retryAction) != "" {
			payload["next_action"] = strings.TrimSpace(retryAction)
		}
		return payload
	}
	var expected map[string]interface{}
	if len(resolvedSkills) > 0 && resolvedSkills[0] != nil {
		expected = skills.ExpectedSkillToolArgumentsForResolved(resolvedSkills[0], skillID, toolName)
	} else {
		expected = skills.ExpectedSkillToolArguments(skillID, toolName)
	}
	if expected != nil {
		payload["expected_arguments"] = expected
		payload["next_action"] = strings.TrimSpace(nextAction + ". Retry call_skill_tool with arguments matching expected_arguments.schema")
	}
	if issues := skills.SkillToolArgumentValidationIssues(err); len(issues) > 0 {
		payload["argument_errors"] = issues
	}
	return payload
}
