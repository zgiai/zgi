package skillloop

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
	if _, ok := FastPathFinalAnswerForToolTrace(trace); ok {
		return nil
	}
	if payload := assetObservationClientActionRequiredPayload(prepared, trace, callID); len(payload) > 0 {
		return payload
	}
	return nil
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
	payload := map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"action_id":       actionID,
		"action_type":     "route_navigation",
		"event_type":      "client_action_required",
		"status":          "waiting_client_action",
		"skill_id":        strings.TrimSpace(trace.SkillID),
		"tool_name":       strings.TrimSpace(trace.ToolName),
		"href":            href,
		"result":          copyStringAnyMap(result),
		"created_at":      time.Now().Unix(),
	}
	if label := strings.TrimSpace(stringFromAny(result["label"])); label != "" {
		payload["label"] = label
	}
	if reason := strings.TrimSpace(stringFromAny(result["reason"])); reason != "" {
		payload["reason"] = reason
	}
	return payload
}

func agentManagementRouteNavigationClientActionRequiredPayload(prepared *PreparedChat, trace skills.SkillTrace, callID string) map[string]interface{} {
	if prepared == nil || prepared.Conversation == nil || prepared.Message == nil ||
		!strings.EqualFold(strings.TrimSpace(trace.SkillID), skills.SkillAgentManagement) {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(trace.Status), "success") {
		return nil
	}
	href := ""
	label := ""
	reason := ""
	switch strings.TrimSpace(trace.ToolName) {
	case "create_agent":
		if !createdAgentClientActionShouldOpenDetail(prepared) {
			return nil
		}
		href = agentDetailHrefFromTraceResult(trace.Result)
		label = "Agent detail"
		reason = "open_created_agent_detail"
	case "delete_agent":
		if !deletedAgentIsCurrentDetailPage(prepared, trace) {
			return nil
		}
		href = normalizeAgentConsoleHref(firstNonEmptyString(
			stringFromAny(trace.Result["route_after_delete"]),
			stringFromAny(trace.Result["href"]),
			"/console/agents",
		))
		label = "Agent list"
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
		"reason":     reason,
	}
	return map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"action_id":       actionID,
		"action_type":     "route_navigation",
		"event_type":      "client_action_required",
		"status":          "waiting_client_action",
		"skill_id":        skills.SkillConsoleNavigator,
		"tool_name":       "navigate",
		"href":            href,
		"label":           label,
		"reason":          reason,
		"result":          result,
		"created_at":      time.Now().Unix(),
	}
}

func isNonBlockingAgentManagementMutation(trace skills.SkillTrace) bool {
	if !strings.EqualFold(strings.TrimSpace(trace.SkillID), skills.SkillAgentManagement) ||
		!strings.EqualFold(strings.TrimSpace(trace.Status), "success") {
		return false
	}
	switch strings.TrimSpace(trace.ToolName) {
	case "update_agent_identity", "update_agent_config", "replace_agent_memory_slots", "replace_agent_skill_bindings", "replace_agent_knowledge_bindings", "replace_agent_database_bindings", "replace_agent_workflow_bindings":
		return true
	default:
		return false
	}
}

func createdAgentClientActionShouldOpenDetail(prepared *PreparedChat) bool {
	if prepared == nil {
		return false
	}
	return wantsCreatedAgentDetailNavigation(prepared.Query)
}

func wantsCreatedAgentDetailNavigation(query string) bool {
	normalized := strings.ToLower(strings.TrimSpace(query))
	if normalized == "" || createdAgentDetailNavigationNegated(normalized) {
		return false
	}
	hasCreate := false
	for _, marker := range []string{"\u521b\u5efa", "\u65b0\u5efa", "create", "new agent"} {
		if strings.Contains(normalized, marker) {
			hasCreate = true
			break
		}
	}
	if !hasCreate {
		return false
	}
	return payloadContainsAnySubstring(normalized, []string{
		"\u6253\u5f00",
		"\u8fdb\u5165",
		"\u8be6\u60c5",
		"open",
		"enter",
		"detail",
		"view",
	})
}

func createdAgentDetailNavigationNegated(normalized string) bool {
	return payloadContainsAnySubstring(normalized, []string{
		"do not navigate", "don't navigate", "dont navigate",
		"do not open", "don't open", "dont open",
		"do not enter", "don't enter", "dont enter",
		"without navigating", "without opening",
		"stay on the list", "stay on current page",
		"\u4e0d\u8981\u5bfc\u822a", "\u4e0d\u8981\u8df3\u8f6c", "\u4e0d\u8981\u6253\u5f00", "\u4e0d\u8981\u8fdb\u5165",
		"\u4e0d\u7528\u5bfc\u822a", "\u4e0d\u7528\u8df3\u8f6c", "\u4e0d\u7528\u6253\u5f00", "\u4e0d\u7528\u8fdb\u5165",
		"\u65e0\u9700\u5bfc\u822a", "\u65e0\u9700\u8df3\u8f6c", "\u65e0\u9700\u6253\u5f00", "\u65e0\u9700\u8fdb\u5165",
		"\u7559\u5728\u5217\u8868", "\u7559\u5728\u5f53\u524d\u9875",
	})
}

func payloadContainsAnySubstring(text string, needles []string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func deletedAgentIsCurrentDetailPage(prepared *PreparedChat, trace skills.SkillTrace) bool {
	currentHref := normalizeAgentDetailHref(currentConsoleRouteFromPrepared(prepared))
	if currentHref == "" {
		return false
	}
	deletedHref := agentDetailHrefFromTrace(trace)
	return deletedHref != "" && currentHref == deletedHref
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
				return "/console/agents/" + agentID + "/agent"
			}
		}
	}
	if agentID := strings.TrimSpace(firstNonEmptyString(
		stringFromAny(trace.Arguments["agent_id"]),
		stringFromAny(trace.Arguments["agentId"]),
		stringFromAny(trace.Arguments["id"]),
	)); agentID != "" {
		return "/console/agents/" + agentID + "/agent"
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
	if agent := payloadMap(result, "agent"); len(agent) > 0 {
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
			return "/console/agents/" + agentID + "/agent"
		}
	}
	if agentID := strings.TrimSpace(firstNonEmptyString(
		stringFromAny(result["agent_id"]),
		stringFromAny(result["id"]),
	)); agentID != "" {
		return "/console/agents/" + agentID + "/agent"
	}
	return ""
}

func normalizeAgentDetailHref(href string) string {
	href = normalizeAgentConsoleHref(href)
	if strings.HasPrefix(href, "/console/agents/") && strings.HasSuffix(href, "/agent") {
		return href
	}
	return ""
}

func normalizeAgentConsoleHref(href string) string {
	href = strings.TrimSpace(href)
	if index := strings.IndexAny(href, "?#"); index >= 0 {
		href = href[:index]
	}
	href = strings.TrimRight(href, "/")
	if href == "" || !strings.HasPrefix(href, "/console/") || strings.Contains(href, "://") {
		return ""
	}
	return href
}

func currentConsoleRouteFromPrepared(prepared *PreparedChat) string {
	if prepared == nil {
		return ""
	}
	return normalizeAgentConsoleHref(prepared.CurrentRoute)
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
	payload := map[string]interface{}{
		"conversation_id":       prepared.Conversation.ID.String(),
		"message_id":            prepared.Message.ID.String(),
		"action_id":             actionID,
		"action_type":           "asset_observation",
		"event_type":            "client_action_required",
		"status":                "waiting_client_action",
		"skill_id":              strings.TrimSpace(trace.SkillID),
		"tool_name":             strings.TrimSpace(trace.ToolName),
		"effect":                effect,
		"asset_type":            assetType,
		"asset_operation_audit": copyStringAnyMap(audit),
		"observation_requested": true,
		"refresh_before_resume": true,
		"created_at":            time.Now().Unix(),
	}
	if correlationID := strings.TrimSpace(payloadText(audit["correlation_id"])); correlationID != "" {
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
	if !strings.EqualFold(strings.TrimSpace(trace.SkillID), skills.SkillFileGenerator) {
		return false
	}
	switch strings.TrimSpace(trace.ToolName) {
	case "generate_file", "generate_docx", "generate_pdf", "generate_pptx":
		return true
	default:
		return false
	}
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
		if text := strings.TrimSpace(payloadText(value)); text != "" {
			return text
		}
	}
	return ""
}

func payloadText(value interface{}) string {
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
		default:
			return value
		}
	}
	return nil
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

func plannerFeedbackTrace(skillID string, toolName string, err error) skills.SkillTrace {
	message := ""
	if err != nil {
		message = err.Error()
	}
	return skills.SkillTrace{
		Kind:     "planner_feedback",
		SkillID:  strings.TrimSpace(skillID),
		ToolName: strings.TrimSpace(toolName),
		Status:   "advisory",
		Error:    message,
		Arguments: map[string]interface{}{
			"next_step": "continue_planning",
		},
	}
}

func internalPlannerFeedbackTrace(trace skills.SkillTrace) bool {
	return strings.TrimSpace(trace.Kind) == "planner_feedback"
}

func skillToolLimitExceededTrace(skillID string, toolName string, args map[string]interface{}, err error) skills.SkillTrace {
	trace := failedSkillTrace("tool_call", toolName, err)
	trace.SkillID = strings.ToLower(strings.TrimSpace(skillID))
	trace.Arguments = summarizeSkillToolArguments(trace.SkillID, toolName, args)
	return trace
}

func blockedSkillGuardrailTrace(skillID string, toolName string, message string) skills.SkillTrace {
	trace := plannerFeedbackTrace(skillID, toolName, nil)
	trace.Error = strings.TrimSpace(message)
	trace.Arguments["next_step"] = "load_skill"
	return trace
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

func plannerFeedbackAdvisoryPayload(message string, nextAction string, skillID string, toolName string) map[string]interface{} {
	return map[string]interface{}{
		"status":      "advisory",
		"advisory":    "planner_feedback",
		"message":     strings.TrimSpace(message),
		"next_action": strings.TrimSpace(nextAction),
		"skill_id":    strings.TrimSpace(skillID),
		"tool_name":   strings.TrimSpace(toolName),
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

func toolCallGuardrailPayload(result FinalAnswerGuardResult, blockedSkillID string, blockedToolName string) map[string]interface{} {
	nextStep := strings.TrimSpace(result.SystemMessage)
	if nextStep == "" {
		nextStep = strings.TrimSpace(result.Message)
	}
	return map[string]interface{}{
		"error":          strings.TrimSpace(result.Message),
		"status":         "blocked",
		"blocked_tool":   strings.TrimSpace(blockedSkillID) + "/" + strings.TrimSpace(blockedToolName),
		"skill_id":       strings.TrimSpace(result.SkillID),
		"tool_name":      strings.TrimSpace(result.ToolName),
		"model_feedback": nextStep,
		"next_step":      "continue planning with the required skill/tool before retrying the blocked action",
	}
}
