package service

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

var managedFileTargetPattern = regexp.MustCompile(`(?i)([^\s，,。；;、：:（）()【】\[\]{}"'“”‘’<>]+?\.(?:txt|md|markdown|html|json|csv|svg|pdf|docx|xlsx|pptx))`)

var runtimeContextRoutePattern = regexp.MustCompile(`(?i)(?:^|[\s,;])route=([^\s,;]+)`)

var agentManagementSkillIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
var agentManagementModelIdentifierPattern = regexp.MustCompile(`(?i)\b(?:gpt-[a-z0-9._-]+|claude[a-z0-9._-]*|deepseek[a-z0-9._-]*|qwen[a-z0-9._-]*|gemini[a-z0-9._-]*|llama[a-z0-9._-]*|mistral[a-z0-9._-]*|kimi[a-z0-9._-]*|doubao[a-z0-9._-]*)\b`)

func (p *PreparedChat) skillsEnabled() bool {
	return p != nil && chatPartsSkillsEnabled(p.parts)
}

func (s *service) runPreparedSkillStream(
	ctx context.Context,
	persistCtx context.Context,
	prepared *PreparedChat,
	onChunk func(string) error,
	onEvent func(StreamEvent) error,
) (string, *adapter.Usage, error) {
	return s.runPreparedSkillStreamWithCompletionVerifier(ctx, persistCtx, prepared, onChunk, onEvent)
}

func (s *service) runPreparedSkillStreamWithCompletionVerifier(
	ctx context.Context,
	persistCtx context.Context,
	prepared *PreparedChat,
	onChunk func(string) error,
	onEvent func(StreamEvent) error,
) (string, *adapter.Usage, error) {
	if s.skillRuntime == nil {
		return "", nil, fmt.Errorf("%w: skill runtime is not configured", ErrInvalidInput)
	}
	if s.llmClient == nil {
		return "", nil, fmt.Errorf("llm client is not configured")
	}
	custom, err := s.customSkillCatalogEntries(ctx, prepared.Scope.OrganizationID)
	if err != nil {
		return "", nil, err
	}
	resolved, err := s.skillRuntime.ResolveEnabledSkillsWithCustom(ctx, prepared.parts.SkillIDs, custom)
	if err != nil {
		return "", nil, err
	}
	unrestrictedResolved := resolved
	resolved = restrictResolvedSkillsForPreparedTurn(prepared, resolved)
	if len(resolved.Skills) == 0 && skillLoopHasOperationPlan(prepared) {
		resolved = unrestrictedResolved
	}
	if len(resolved.Skills) == 0 {
		return "", nil, fmt.Errorf("%w: no skills available for configured skill ids", ErrInvalidInput)
	}

	timeline := newProcessTimelineRecorder(ctx, persistCtx, s, prepared, onEvent)
	runner := &skillloop.Runner{
		LLMClient:    s.llmClient,
		SkillRuntime: s.skillRuntime,
		AppContext:   newBillingAppContext(prepared),
		OnEvent: func(event skillloop.Event) error {
			if event.Type == skillloop.EventUserInputRequested {
				s.persistUserInputRequestBestEffort(persistCtx, prepared, event.Payload)
			}
			timeline.RecordEvent(event.Type, event.Payload)
			return nil
		},
		OnTrace: func(traces []skills.SkillTrace, trace skills.SkillTrace) {
			timeline.RecordTrace(traces, trace)
		},
		OnArtifact: func(artifact map[string]interface{}) {
			s.persistGeneratedArtifactBestEffort(ctx, prepared, artifact)
		},
		OnModelInvocation: func(trace skillloop.ModelInvocationTrace) {
			s.persistModelInvocationBestEffort(persistCtx, prepared, trace)
		},
	}
	loopPrepared := skillloop.NewPreparedChat(
		prepared.Conversation.ID.String(),
		prepared.Message.ID.String(),
		prepared.parts.Provider,
		prepared.parts.SkillMode,
		prepared.LLMRequest,
	)
	loopPrepared.Query = strings.TrimSpace(prepared.parts.Query)
	loopPrepared.CurrentRoute = contextualTurnCurrentPage(prepared.parts)
	loopPrepared.Surface = normalizeAIChatSurface(prepared.parts.Surface)
	return runner.Run(ctx, skillloop.RunRequest{
		Prepared:                 loopPrepared,
		Resolved:                 resolved,
		ExecutionContext:         s.skillExecutionContext(prepared),
		AdditionalSystemMessages: skillLoopAdditionalSystemMessagesForResolved(prepared, resolved),
		PlanToolGuard:            skillLoopPlanToolCallGuardWithResolved(prepared, resolved),
		ToolArgumentResolver:     skillLoopToolArgumentResolver(prepared),
		CompletionEvidence:       skillLoopCompletionEvidence(prepared),
		CurrentMetadata:          skillLoopCurrentMetadata(prepared),
		OnCompletionVerification: skillLoopCompletionVerificationResult(prepared),
		OnChunk:                  onChunk,
	})
}

func skillLoopCurrentMetadata(prepared *PreparedChat) func() map[string]interface{} {
	return func() map[string]interface{} {
		if prepared == nil || prepared.Message == nil {
			return nil
		}
		return copyStringAnyMap(prepared.Message.Metadata)
	}
}

func skillLoopToolArgumentResolver(prepared *PreparedChat) skillloop.ToolArgumentResolver {
	return func(req skillloop.ToolCallGuardRequest) (map[string]interface{}, bool) {
		return skillLoopResolveBoundToolArguments(prepared, req)
	}
}

func skillLoopResolveBoundToolArguments(prepared *PreparedChat, req skillloop.ToolCallGuardRequest) (map[string]interface{}, bool) {
	if prepared == nil || prepared.Message == nil {
		return nil, false
	}
	binding := skillLoopPlanArgsBindingForTool(prepared.Message.Metadata, req.SkillID, req.ToolName)
	if len(binding) == 0 && skillLoopCreateAndEditPlanActive(prepared) &&
		strings.EqualFold(strings.TrimSpace(req.SkillID), skills.SkillAgentManagement) &&
		agentManagementToolRequiresSingleAgentTarget(req.ToolName) {
		targets := skillLoopCreatedAgentTargets(prepared, req)
		if len(targets) == 1 {
			binding = map[string]string{"agent_id": aiChatStructuredFirstCreatedAgentIDExpr}
		}
	}
	resolved := copyStringAnyMap(req.Arguments)
	changed := false
	if len(binding) > 0 {
		for key, expr := range binding {
			key = strings.TrimSpace(key)
			expr = strings.TrimSpace(expr)
			if key == "" || expr == "" {
				continue
			}
			value, ok := skillLoopResolveToolArgBindingExpression(prepared, req, expr)
			if !ok || value == "" {
				continue
			}
			if strings.TrimSpace(stringFromAny(resolved[key])) == value {
				continue
			}
			resolved[key] = value
			changed = true
		}
	}
	if !changed {
		return nil, false
	}
	return resolved, true
}

func skillLoopPlanArgsBindingForTool(metadata map[string]interface{}, skillID string, toolName string) map[string]string {
	plan := mapFromOperationContext(metadataValue(metadata, "operation_plan"))
	if len(plan) == 0 {
		return nil
	}
	skillID = strings.TrimSpace(skillID)
	toolName = strings.TrimSpace(toolName)
	if skillID == "" || toolName == "" {
		return nil
	}
	for _, source := range []interface{}{
		mapFromOperationContext(plan["structured_plan"])["required_tool_sequence"],
		mapFromOperationContext(plan["structured_plan"])["operations"],
		plan["steps"],
	} {
		if binding := skillLoopPlanArgsBindingForToolFromItems(source, skillID, toolName); len(binding) > 0 {
			return binding
		}
	}
	return nil
}

func skillLoopPlanArgsBindingForToolFromItems(value interface{}, skillID string, toolName string) map[string]string {
	for _, item := range mapSliceFromAny(value) {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(item["skill_id"])), skillID) ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(item["tool_name"])), toolName) {
			continue
		}
		if binding := cleanStringAnyStringMap(mapFromOperationContext(item["args_binding"])); len(binding) > 0 {
			return binding
		}
	}
	return nil
}

func skillLoopResolveToolArgBindingExpression(prepared *PreparedChat, req skillloop.ToolCallGuardRequest, expr string) (string, bool) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "", false
	}
	switch {
	case strings.EqualFold(expr, "$created_agent.agent_id"):
		return skillLoopResolveCreatedAgentsBinding(prepared, req, "$created_agents[index=0].agent_id")
	case strings.EqualFold(expr, "$created_agents.agent_id"):
		return skillLoopResolveCreatedAgentsBinding(prepared, req, "$created_agents[index=0].agent_id")
	case strings.HasPrefix(expr, "$created_agents"):
		return skillLoopResolveCreatedAgentsBinding(prepared, req, expr)
	default:
		return "", false
	}
}

func skillLoopCompletionVerificationResult(prepared *PreparedChat) func(skillloop.CompletionVerificationResult) {
	return func(result skillloop.CompletionVerificationResult) {
		if prepared == nil || prepared.Message == nil {
			return
		}
		result = skillloop.ReconcileCompletionVerificationResultWithEvidence(skillLoopCompletionEvidence(prepared)(), result)
		metadata := copyStringAnyMap(prepared.Message.Metadata)
		applyOperationPlanCompletionVerificationResult(
			metadata,
			result.Status,
			result.Reason,
			result.MissingSteps,
			result.UnsupportedClaims,
			result.NextActionHint,
		)
		prepared.Message.Metadata = metadata
	}
}

func skillLoopCompletionEvidence(prepared *PreparedChat) skillloop.CompletionEvidenceFunc {
	return func() map[string]interface{} {
		evidence := map[string]interface{}{}
		if prepared == nil || prepared.parts == nil {
			return evidence
		}
		evidence["user_request"] = strings.TrimSpace(prepared.parts.Query)
		evidence["surface"] = normalizeAIChatSurface(prepared.parts.Surface)
		evidence["skill_mode"] = prepared.parts.SkillMode
		if skillLoopShouldSuppressAutoFinalAnswerFastPath(prepared.parts) {
			evidence["suppress_auto_final_answer_fast_path"] = true
			evidence["suppress_auto_final_answer_reason"] = "client_action_continuation"
		}
		if len(prepared.parts.SkillIDs) > 0 {
			evidence["configured_skill_ids"] = append([]string{}, prepared.parts.SkillIDs...)
		}
		pageContext := skillLoopCompletionPageContextEvidence(prepared.parts)
		if len(pageContext) > 0 {
			evidence["page_context"] = pageContext
		}
		if prepared.Message == nil {
			return evidence
		}
		metadata := skillLoopCompletionEvidenceMetadata(prepared.Message.Metadata)
		if len(pageContext) > 0 {
			applyOperationPlanPageEvidence(metadata, pageContext)
		}
		prepared.Message.Metadata = metadata
		for _, key := range []string{
			"turn_strategy",
			"operation_plan",
			"operation_ledger",
			"skill_invocations",
			"generated_files",
			"client_actions",
			"tool_governance",
			consoleFilesContextSnapshotKey,
			consoleAgentsContextSnapshotKey,
		} {
			if value, ok := metadata[key]; ok && value != nil {
				evidence[key] = value
			}
		}
		executionLedger := map[string]interface{}{}
		for _, key := range []string{"operation_ledger", "skill_invocations", "generated_files", "client_actions", "tool_governance"} {
			if value, ok := metadata[key]; ok && value != nil {
				executionLedger[key] = value
			}
		}
		if progress := clientActionAgentCreateProgress(prepared.Message); len(progress) > 0 {
			evidence["agent_create_progress"] = progress
			executionLedger["agent_create_progress"] = progress
		}
		if summary := skillLoopCompletionExecutionSummary(metadata); len(summary) > 0 {
			if operationSummary := skillLoopCompletionOperationResultSummary(summary); len(operationSummary) > 0 {
				summary["operation_result_summary"] = operationSummary
				evidence["operation_result_summary"] = operationSummary
				executionLedger["operation_result_summary"] = operationSummary
			}
			evidence["execution_summary"] = summary
			executionLedger["summary"] = summary
		}
		if len(executionLedger) > 0 {
			evidence["execution_ledger"] = executionLedger
		}
		return evidence
	}
}

func skillLoopShouldSuppressAutoFinalAnswerFastPath(parts *chatRequestParts) bool {
	if parts == nil {
		return false
	}
	for _, source := range []map[string]interface{}{parts.RawOperationContext, parts.OperationContext} {
		continuation := mapFromOperationContext(source["client_action_continuation"])
		if len(continuation) == 0 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(stringFromAny(continuation["action_type"])), "route_navigation") ||
			isConsoleNavigatorNavigateTool(stringFromAny(continuation["skill_id"]), stringFromAny(continuation["tool_name"])) {
			return true
		}
	}
	return false
}

func skillLoopCompletionPageContextEvidence(parts *chatRequestParts) map[string]interface{} {
	if parts == nil {
		return nil
	}
	out := map[string]interface{}{}
	currentPage := contextualTurnCurrentPage(parts)
	if currentPage != "" {
		out["current_page"] = currentPage
	}
	if runtimeRoute := consoleRouteFromRuntimeContext(parts.RuntimeContext); runtimeRoute != "" {
		out["runtime_route"] = runtimeRoute
	}
	if target, ok := resolveConsoleNavigationTargetForParts(parts); ok && strings.TrimSpace(target.Href) != "" {
		resolved := map[string]interface{}{
			"href": target.Href,
		}
		if target.Label != "" {
			resolved["label"] = target.Label
		}
		out["resolved_target_from_user_request"] = resolved
		if consoleNavigationRouteAlreadyAvailable(parts, target.Href) {
			out["target_route_already_available"] = true
			out["route_evidence"] = "current_page_context_matches_target"
		}
	}
	if resources := skillLoopCompactPageContextResources(parts, 12); len(resources) > 0 {
		out["resources"] = resources
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func skillLoopCompactPageContextResources(parts *chatRequestParts, limit int) []interface{} {
	if parts == nil || limit <= 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]interface{}, 0, limit)
	for _, source := range []map[string]interface{}{parts.RawOperationContext, parts.OperationContext} {
		for _, item := range mapSliceFromAny(source["resources"]) {
			if len(out) >= limit {
				return out
			}
			compact := skillLoopCompactPageContextResource(item)
			if len(compact) == 0 {
				continue
			}
			key := strings.Join([]string{
				strings.TrimSpace(firstNonEmptyString(compact["resource_type"], compact["type"], compact["asset_type"])),
				strings.TrimSpace(firstNonEmptyString(compact["resource_id"], compact["id"], compact["agent_id"], compact["href"])),
			}, ":")
			if key != ":" {
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
			}
			out = append(out, compact)
		}
	}
	return out
}

func skillLoopCompactPageContextResource(item map[string]interface{}) map[string]interface{} {
	if len(item) == 0 {
		return nil
	}
	compact := map[string]interface{}{}
	metadata := mapFromOperationContext(item["metadata"])
	for _, key := range []string{
		"index",
		"resource_id",
		"resource_type",
		"id",
		"agent_id",
		"agent_name",
		"name",
		"title",
		"type",
		"asset_type",
		"workspace_id",
		"status",
		"href",
		"route",
		"context_ready",
		"files_query_status",
		"agents_query_status",
		"visible_file_count",
		"total_file_count",
		"visible_agent_count",
		"loaded_agent_count",
	} {
		if value := firstNonEmptyString(item[key], metadata[key]); value != "" {
			compact[key] = compactForPrompt(value, 240)
			continue
		}
		if itemValue, ok := item[key]; ok && itemValue != nil {
			if _, ok := compact[key]; !ok {
				compact[key] = itemValue
			}
		}
		if metadataValue, ok := metadata[key]; ok && metadataValue != nil {
			if _, ok := compact[key]; !ok {
				compact[key] = metadataValue
			}
		}
	}
	return compact
}

func skillLoopCompletionEvidenceMetadata(source map[string]interface{}) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if invocations := mapSliceFromAny(metadata["skill_invocations"]); len(invocations) > 0 {
		applyOperationPlanInvocationState(metadata, invocations)
	}
	if files := mapSliceFromAny(metadata["generated_files"]); len(files) > 0 {
		applyOperationPlanArtifactState(metadata, files)
	}
	finalizeOperationPlanForResult(metadata)
	return metadata
}

func skillLoopCompletionExecutionSummary(metadata map[string]interface{}) map[string]interface{} {
	if len(metadata) == 0 {
		return nil
	}
	summary := map[string]interface{}{}
	if plan := skillLoopCompletionPlanSummary(mapFromOperationContext(metadata["operation_plan"])); len(plan) > 0 {
		summary["operation_plan"] = plan
	}
	if toolResults := skillLoopCompletionToolResults(mapSliceFromAny(metadata["skill_invocations"]), 12); len(toolResults) > 0 {
		summary["tool_results"] = toolResults
	}
	if clientActions := skillLoopCompletionToolResults(mapSliceFromAny(metadata["client_actions"]), 8); len(clientActions) > 0 {
		summary["client_actions"] = clientActions
	}
	if groups := skillLoopCompletionOperationGroups(metadata, 6); len(groups) > 0 {
		summary["operation_groups"] = groups
	}
	if files := operationPlanCompactOperationItems(metadata["generated_files"], 8); len(files) > 0 {
		summary["generated_files"] = files
	}
	if len(summary) == 0 {
		return nil
	}
	return summary
}

func skillLoopCompletionOperationResultSummary(summary map[string]interface{}) map[string]interface{} {
	if len(summary) == 0 {
		return nil
	}
	out := map[string]interface{}{}
	latestToolSettlesOperation := false
	planStatus := ""
	if plan := mapFromOperationContext(summary["operation_plan"]); len(plan) > 0 {
		planStatus = strings.TrimSpace(stringFromAny(plan["status"]))
		if status := strings.TrimSpace(stringFromAny(plan["status"])); status != "" {
			out["plan_status"] = status
		}
		copyOperationResultSummaryFields(out, plan, "pending_next_action", "current_page")
		if result := mapFromOperationContext(plan["tool_result"]); len(result) > 0 {
			latestToolSettlesOperation = applyOperationResultSummaryToolResult(out, result, planStatus)
		}
		if _, ok := out["operation_group"]; !ok && !latestToolSettlesOperation {
			if group := mapFromOperationContext(plan["operation_group"]); len(group) > 0 {
				out["operation_group"] = operationPlanCompactOperationGroup(group)
				copyOperationResultSummaryFields(out, group, "status", "operation", "asset_type", "target_count", "success_count", "failed_count")
			}
		}
	}
	if toolResults := mapSliceFromAny(summary["tool_results"]); len(toolResults) > 0 {
		if result := mapFromOperationContext(toolResults[0]); len(result) > 0 {
			current := mapFromOperationContext(out["latest_tool_result"])
			if operationResultSummaryShouldPreferToolResult(current, result) {
				latestToolSettlesOperation = applyOperationResultSummaryToolResult(out, result, "")
			}
		}
		out["tool_result_count"] = len(toolResults)
	}
	if groups := mapSliceFromAny(summary["operation_groups"]); len(groups) > 0 {
		if _, ok := out["operation_group"]; !ok && !latestToolSettlesOperation {
			if group := mapFromOperationContext(groups[0]); len(group) > 0 {
				out["operation_group"] = operationPlanCompactOperationGroup(group)
				copyOperationResultSummaryFields(out, group, "status", "operation", "asset_type", "target_count", "success_count", "failed_count")
			}
		}
	}
	if clientActions := mapSliceFromAny(summary["client_actions"]); len(clientActions) > 0 {
		out["client_action_count"] = len(clientActions)
		if action := mapFromOperationContext(clientActions[0]); len(action) > 0 {
			out["latest_client_action"] = action
		}
	}
	if files := mapSliceFromAny(summary["generated_files"]); len(files) > 0 {
		out["generated_file_count"] = len(files)
		out["generated_files"] = files
	}
	applyOperationResultSummaryPlanStatus(out, planStatus)
	if len(out) == 0 {
		return nil
	}
	out["source"] = "execution_summary"
	return out
}

func applyOperationResultSummaryPlanStatus(out map[string]interface{}, planStatus string) {
	if out == nil {
		return
	}
	switch strings.ToLower(strings.TrimSpace(planStatus)) {
	case operationPlanStatusFailed, "error", "errored", "blocked", "rejected", "partial_failed", "partially_failed", "needs_action":
		out["status"] = operationPlanStatusFailed
	case operationPlanStatusRunning:
		if current := strings.ToLower(strings.TrimSpace(stringFromAny(out["status"]))); current == "" ||
			current == "success" || current == "succeeded" || current == "completed" {
			out["status"] = operationPlanStatusRunning
		}
	}
}

func applyOperationResultSummaryToolResult(out map[string]interface{}, result map[string]interface{}, planStatus string) bool {
	if out == nil || len(result) == 0 {
		return false
	}
	out["latest_tool_result"] = result
	if skillID := strings.TrimSpace(stringFromAny(result["skill_id"])); skillID != "" {
		out["skill_id"] = skillID
	}
	if toolName := strings.TrimSpace(stringFromAny(result["tool_name"])); toolName != "" {
		out["tool_name"] = toolName
	}
	resultSummary := mapFromOperationContext(result["result_summary"])
	if group := mapFromOperationContext(resultSummary["operation_group"]); len(group) > 0 {
		out["operation_group"] = operationPlanCompactOperationGroup(group)
		copyOperationResultSummaryFields(out, group, "status", "operation", "asset_type", "target_count", "success_count", "failed_count")
	}
	if status := strings.TrimSpace(stringFromAny(resultSummary["status"])); status != "" {
		out["status"] = status
	}
	if status := strings.TrimSpace(stringFromAny(result["status"])); status != "" {
		out["latest_tool_status"] = status
		if _, ok := out["status"]; !ok {
			out["status"] = status
		}
	}
	if len(resultSummary) > 0 {
		copyOperationResultSummaryFields(out, resultSummary,
			"effect",
			"agent_id",
			"agent_name",
			"filename",
			"file_name",
			"managed_filename",
			"target_count",
			"deleted_count",
			"failed_count",
			"requires_refresh",
			"refresh_target",
			"error",
		)
	}
	return operationResultSummaryLatestToolSettlesOperation(planStatus, result)
}

func operationResultSummaryShouldPreferToolResult(current map[string]interface{}, candidate map[string]interface{}) bool {
	if len(candidate) == 0 {
		return false
	}
	if len(current) == 0 {
		return true
	}
	currentKind := strings.ToLower(strings.TrimSpace(stringFromAny(current["kind"])))
	candidateKind := strings.ToLower(strings.TrimSpace(stringFromAny(candidate["kind"])))
	if candidateKind == "tool_call" && currentKind != "tool_call" {
		return true
	}
	currentStatus := operationPlanNormalizeStepStatus(firstNonEmptyString(
		mapFromOperationContext(current["result_summary"])["status"],
		current["status"],
	))
	candidateStatus := operationPlanNormalizeStepStatus(firstNonEmptyString(
		mapFromOperationContext(candidate["result_summary"])["status"],
		candidate["status"],
	))
	if candidateStatus == operationPlanStepStatusCompleted && currentStatus != operationPlanStepStatusCompleted {
		return true
	}
	if operationPlanInvocationIsAssetMutation(candidate) && !operationPlanInvocationIsAssetMutation(current) {
		return true
	}
	return false
}

func operationResultSummaryLatestToolSettlesOperation(planStatus string, result map[string]interface{}) bool {
	if len(result) == 0 {
		return false
	}
	if operationResultSummaryPlanStatusBlocksToolSettlement(planStatus) {
		return false
	}
	resultSummary := mapFromOperationContext(result["result_summary"])
	if group := mapFromOperationContext(resultSummary["operation_group"]); len(group) > 0 {
		return false
	}
	status := operationPlanNormalizeStepStatus(firstNonEmptyString(resultSummary["status"], result["status"]))
	if status != operationPlanStepStatusCompleted {
		return false
	}
	if strings.TrimSpace(stringFromAny(resultSummary["error"])) != "" {
		return false
	}
	if operationPlanInvocationIsAssetMutation(result) {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(planStatus), operationPlanStatusCompleted)
}

func operationResultSummaryPlanStatusBlocksToolSettlement(planStatus string) bool {
	switch strings.ToLower(strings.TrimSpace(planStatus)) {
	case operationPlanStatusFailed, "error", "errored", "blocked", "rejected", "partial_failed", "partially_failed", "needs_action":
		return true
	default:
		return false
	}
}

func copyOperationResultSummaryFields(dst map[string]interface{}, src map[string]interface{}, keys ...string) {
	if dst == nil || len(src) == 0 {
		return
	}
	for _, key := range keys {
		value, ok := src[key]
		if !ok || value == nil {
			continue
		}
		if _, exists := dst[key]; exists {
			continue
		}
		dst[key] = value
	}
}

func skillLoopCompletionPlanSummary(plan map[string]interface{}) map[string]interface{} {
	if len(plan) == 0 {
		return nil
	}
	summary := map[string]interface{}{}
	for _, key := range []string{
		"status",
		"intent",
		"task_type",
		"pending_next_action",
		"current_page",
		"original_user_goal",
		"risk_level",
		"approval",
		"planning_mode",
		"operation_group_status",
	} {
		if value := strings.TrimSpace(stringFromAny(plan[key])); value != "" {
			summary[key] = compactForPrompt(value, 500)
		}
	}
	modelDecides := operationPlanModelDecidesTools(plan)
	if value, ok := plan["approval_required"].(bool); ok {
		summary["approval_required"] = value
	}
	if value, ok := plan["needs_exact_agent_runtime"].(bool); ok {
		summary["needs_exact_agent_runtime"] = value
	}
	if value, ok := plan["current_context_may_be_summary"].(bool); ok {
		summary["current_context_may_be_summary"] = value
	}
	if phases := stringSliceFromAny(plan["phase_goals"]); len(phases) > 0 {
		summary["phase_goals"] = compactStringSliceForPrompt(phases, 8, 180)
	}
	if evidence := stringSliceFromAny(plan["evidence_required"]); len(evidence) > 0 {
		summary["evidence_required"] = compactStringSliceForPrompt(evidence, 10, 180)
	}
	if capabilities := stringSliceFromAny(plan["recommended_capabilities"]); len(capabilities) > 0 {
		summary["recommended_capabilities"] = compactStringSliceForPrompt(capabilities, 10, 160)
	}
	if actions := stringSliceFromAny(plan["approval_actions"]); len(actions) > 0 {
		summary["approval_actions"] = compactStringSliceForPrompt(actions, 8, 180)
	}
	if criteria := stringSliceFromAny(plan["success_criteria"]); len(criteria) > 0 {
		summary["success_criteria"] = compactStringSliceForPrompt(criteria, 8, 240)
	}
	if criteria := stringSliceFromAny(plan["completion_criteria"]); len(criteria) > 0 {
		summary["completion_criteria"] = compactStringSliceForPrompt(criteria, 8, 240)
	}
	if target := mapFromOperationContext(plan["asset_target"]); len(target) > 0 {
		summary["asset_target"] = target
	}
	if toolResult := mapFromOperationContext(plan["tool_result"]); len(toolResult) > 0 {
		summary["tool_result"] = toolResult
	}
	if assetState := mapFromOperationContext(plan["asset_state"]); len(assetState) > 0 {
		summary["asset_state"] = assetState
	}
	if pageEvidence := operationPlanCompactPageEvidence(mapFromOperationContext(plan["page_evidence"])); len(pageEvidence) > 0 {
		summary["page_evidence"] = pageEvidence
	}
	if phases := operationPlanCompactPhasesForPrompt(plan["phases"], 8); len(phases) > 0 {
		summary["phases"] = phases
	}
	if state := mapFromOperationContext(plan["strategy_state"]); len(state) > 0 {
		summary["strategy_state"] = operationPlanCompactStrategyStateForPrompt(state, modelDecides)
	}
	if !modelDecides {
		if stepStatus := mapFromOperationContext(plan["step_status"]); len(stepStatus) > 0 {
			summary["step_status"] = stepStatus
		}
	}
	if !modelDecides {
		if structuredPlan := operationPlanCompactStructuredPlanForPrompt(plan["structured_plan"], 12); len(structuredPlan) > 0 {
			summary["structured_plan"] = structuredPlan
		}
	}
	if completedSteps := operationPlanCompactProgressStepRecords(plan["completed_steps"], 8); len(completedSteps) > 0 && !modelDecides {
		summary["completed_steps"] = completedSteps
	}
	if failedSteps := operationPlanCompactProgressStepRecords(plan["failed_steps"], 8); len(failedSteps) > 0 && !modelDecides {
		summary["failed_steps"] = failedSteps
	}
	if group := mapFromOperationContext(plan["operation_group"]); len(group) > 0 {
		summary["operation_group"] = operationPlanCompactOperationGroup(group)
	}
	if targetSet := operationPlanCompactOperationItems(plan["target_set"], 20); len(targetSet) > 0 {
		summary["target_set"] = targetSet
	}
	if itemSteps := operationPlanCompactOperationItems(plan["item_steps"], 20); len(itemSteps) > 0 {
		summary["item_steps"] = itemSteps
	}
	if deviations := skillLoopCompletionPlanDeviations(plan["deviations"], 8); len(deviations) > 0 {
		summary["deviations"] = deviations
	}
	if blockedDeviations := skillLoopCompletionPlanDeviations(plan["blocked_deviations"], 8); len(blockedDeviations) > 0 {
		summary["blocked_deviations"] = blockedDeviations
	}
	if steps := operationPlanCompactStepsForPrompt(plan["steps"], 8); len(steps) > 0 && !modelDecides {
		summary["steps"] = steps
	}
	if len(summary) == 0 {
		return nil
	}
	return summary
}

func operationPlanCompactStrategyStateForPrompt(state map[string]interface{}, modelDecides bool) map[string]interface{} {
	if len(state) == 0 {
		return nil
	}
	out := copyStringAnyMap(state)
	if goals := operationPlanCompactCapabilityGoals(out["capability_goals"], 6); len(goals) > 0 {
		out["capability_goals"] = goals
	} else {
		delete(out, "capability_goals")
	}
	if modelDecides {
		delete(out, "plan_steps")
		delete(out, "structured_plan")
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func operationPlanCompactPhasesForPrompt(value interface{}, limit int) []interface{} {
	phases := mapSliceFromAny(value)
	if len(phases) == 0 || limit <= 0 {
		return nil
	}
	out := make([]interface{}, 0, minInt(len(phases), limit))
	for _, phase := range phases {
		if len(out) >= limit {
			break
		}
		item := map[string]interface{}{}
		for _, key := range []string{"id", "title", "status"} {
			if text := strings.TrimSpace(stringFromAny(phase[key])); text != "" {
				item[key] = compactForPrompt(text, 240)
			}
		}
		if evidence := stringSliceFromAny(phase["evidence"]); len(evidence) > 0 {
			item["evidence"] = compactStringSliceForPrompt(evidence, 6, 180)
		}
		if points := stringSliceFromAny(phase["observation_points"]); len(points) > 0 {
			item["observation_points"] = compactStringSliceForPrompt(points, 6, 180)
		}
		if criteria := stringSliceFromAny(phase["success_criteria"]); len(criteria) > 0 {
			item["success_criteria"] = compactStringSliceForPrompt(criteria, 6, 180)
		}
		if len(item) > 0 {
			out = append(out, item)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func operationPlanCompactStructuredPlanForPrompt(value interface{}, limit int) map[string]interface{} {
	plan := mapFromOperationContext(value)
	if len(plan) == 0 || limit <= 0 {
		return nil
	}
	out := map[string]interface{}{}
	for _, key := range []string{
		"schema_version",
		"domain",
		"intent",
		"target",
		"status",
		"if_blocked",
	} {
		if text := strings.TrimSpace(stringFromAny(plan[key])); text != "" {
			out[key] = compactForPrompt(text, 500)
		}
	}
	for _, key := range []string{"requires_approval", "read_before_write"} {
		if value, ok := plan[key].(bool); ok {
			out[key] = value
		}
	}
	if counts := mapFromOperationContext(plan["operation_counts"]); len(counts) > 0 {
		out["operation_counts"] = counts
	}
	if operations := operationPlanCompactStructuredOperationsForPrompt(plan["operations"], limit); len(operations) > 0 {
		out["operations"] = operations
	}
	if goals := operationPlanCompactCapabilityGoals(plan["capability_goals"], 6); len(goals) > 0 {
		out["capability_goals"] = goals
	}
	if criteria := stringSliceFromAny(plan["completion_criteria"]); len(criteria) > 0 {
		out["completion_criteria"] = compactStringSliceForPrompt(criteria, 8, 240)
	}
	if warnings := stringSliceFromAny(plan["validation_warnings"]); len(warnings) > 0 {
		out["validation_warnings"] = compactStringSliceForPrompt(warnings, 8, 240)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func operationPlanCompactStructuredOperationsForPrompt(value interface{}, limit int) []interface{} {
	operations := mapSliceFromAny(value)
	if len(operations) == 0 || limit <= 0 {
		return nil
	}
	out := make([]interface{}, 0, minInt(len(operations), limit))
	for _, operation := range operations {
		if len(out) >= limit {
			break
		}
		compact := map[string]interface{}{}
		for _, key := range []string{
			"action",
			"resource_type",
			"resource_name",
			"effect",
			"if_missing",
			"status",
			"last_invocation_id",
			"last_invocation_kind",
			"goal",
			"error",
		} {
			if text := strings.TrimSpace(stringFromAny(operation[key])); text != "" {
				compact[key] = compactForPrompt(text, 500)
			}
		}
		if fields := stringSliceFromAny(operation["fields"]); len(fields) > 0 {
			compact["fields"] = compactStringSliceForPrompt(fields, 8, 160)
		}
		if group := mapFromOperationContext(operation["operation_group"]); len(group) > 0 {
			compact["operation_group"] = operationPlanCompactOperationGroup(group)
		}
		if items := operationPlanCompactOperationItems(operation["item_steps"], 20); len(items) > 0 {
			compact["item_steps"] = items
		}
		if len(compact) > 0 {
			out = append(out, compact)
		}
	}
	return out
}

func cleanStringAnyStringMap(values map[string]interface{}) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := map[string]string{}
	for key, value := range values {
		key = strings.TrimSpace(key)
		text := strings.TrimSpace(stringFromAny(value))
		if key == "" || text == "" {
			continue
		}
		out[key] = text
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func operationPlanCompactStepsForPrompt(value interface{}, limit int) []interface{} {
	steps := mapSliceFromAny(value)
	if len(steps) == 0 || limit <= 0 {
		return nil
	}
	promptSteps := operationPlanPromptSteps(steps, limit)
	out := make([]interface{}, 0, len(promptSteps))
	for _, step := range promptSteps {
		item := map[string]interface{}{}
		for _, key := range []string{"id", "title", "status", "skill_id", "tool_name"} {
			if value := strings.TrimSpace(stringFromAny(step[key])); value != "" {
				item[key] = compactForPrompt(value, 180)
			}
		}
		if index := intValueFromAny(step["sequence_index"]); index > 0 {
			item["sequence_index"] = index
		}
		if target := mapFromOperationContext(step["asset_target"]); len(target) > 0 {
			item["asset_target"] = target
		}
		if goal := strings.TrimSpace(stringFromAny(step[operationPlanConfigGoalKey])); goal != "" {
			item[operationPlanConfigGoalKey] = compactForPrompt(goal, 240)
		}
		if len(item) > 0 {
			out = append(out, item)
		}
	}
	return out
}

func skillLoopCompletionPlanDeviations(value interface{}, limit int) []interface{} {
	deviations := mapSliceFromAny(value)
	if len(deviations) == 0 || limit <= 0 {
		return nil
	}
	out := make([]interface{}, 0, minInt(len(deviations), limit))
	for _, item := range deviations {
		if len(out) >= limit {
			break
		}
		compact := map[string]interface{}{}
		for _, key := range []string{"skill_id", "tool_name", "reason", "outcome"} {
			if value, ok := item[key]; ok && value != nil {
				compact[key] = value
			}
		}
		if target := mapFromOperationContext(item["asset_target"]); len(target) > 0 {
			compact["asset_target"] = target
		}
		if len(compact) > 0 {
			out = append(out, compact)
		}
	}
	return out
}

func skillLoopCompletionToolResults(invocations []map[string]interface{}, limit int) []interface{} {
	if len(invocations) == 0 || limit <= 0 {
		return nil
	}
	out := make([]interface{}, 0, minInt(len(invocations), limit))
	for i := len(invocations) - 1; i >= 0 && len(out) < limit; i-- {
		invocation := invocations[i]
		if !operationPlanInvocationIsActionable(invocation) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["kind"])), "tool_governance") {
			continue
		}
		result := operationPlanToolResult(invocation)
		if len(result) == 0 {
			continue
		}
		if id := operationPlanInvocationPlanID(invocation); id != "" {
			result["invocation_id"] = id
		}
		out = append(out, result)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func skillLoopCompletionOperationGroups(metadata map[string]interface{}, limit int) []interface{} {
	if len(metadata) == 0 || limit <= 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]interface{}, 0, limit)
	appendGroup := func(group map[string]interface{}) {
		if len(group) == 0 || len(out) >= limit {
			return
		}
		compact := operationPlanCompactOperationGroup(group)
		if len(compact) == 0 {
			return
		}
		key := firstNonEmptyString(compact["id"], compact["operation"], compact["asset_type"])
		if key != "" {
			if _, ok := seen[key]; ok {
				return
			}
			seen[key] = struct{}{}
		}
		out = append(out, compact)
	}
	if plan := mapFromOperationContext(metadata["operation_plan"]); len(plan) > 0 {
		appendGroup(mapFromOperationContext(plan["operation_group"]))
	}
	for _, invocation := range mapSliceFromAny(metadata["skill_invocations"]) {
		appendGroup(operationPlanOperationGroupFromInvocation(invocation))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func restrictResolvedSkillsForTurnStrategy(parts *chatRequestParts, resolved *skills.ResolvedSkills) *skills.ResolvedSkills {
	if parts == nil || resolved == nil || len(resolved.Skills) == 0 {
		return resolved
	}
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		return resolved
	}
	if aiChatTurnStrategyModelDecidesTools(strategy) {
		return resolved
	}
	return filterResolvedSkillsByAllowedIDs(resolved, turnStrategyAllowedSkillIDs(strategy), false)
}

func restrictResolvedSkillsForPreparedTurn(prepared *PreparedChat, resolved *skills.ResolvedSkills) *skills.ResolvedSkills {
	if prepared == nil || resolved == nil || len(resolved.Skills) == 0 {
		return resolved
	}
	if skillLoopModelDecidesToolChoice(prepared) {
		return resolved
	}
	if exposedTools := skillLoopExposedPlannedTools(prepared); len(exposedTools) > 0 {
		return filterResolvedSkillsByAllowedTools(resolved, exposedTools, false)
	}
	if allowed := operationPlanAllowedSkillIDs(prepared); len(allowed) > 0 {
		return filterResolvedSkillsByAllowedIDs(resolved, allowed, false)
	}
	if skillLoopShouldRestrictToOperationPlan(prepared) {
		return &skills.ResolvedSkills{}
	}
	return restrictResolvedSkillsForTurnStrategy(prepared.parts, resolved)
}

func operationPlanAllowedSkillIDs(prepared *PreparedChat) map[string]struct{} {
	if prepared == nil || prepared.Message == nil {
		return nil
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if len(plan) == 0 {
		return nil
	}
	if operationPlanModelDecidesTools(plan) {
		return nil
	}
	allowed := map[string]struct{}{}
	stepStatus := mapFromOperationContext(plan["step_status"])
	for _, step := range operationPlanPendingExecutableStepsForToolExposure(plan, 8) {
		if !operationPlanStepWaitForReadyFromPlan(step, stepStatus) {
			continue
		}
		skillID := strings.TrimSpace(stringFromAny(step["skill_id"]))
		if skillID == "" {
			continue
		}
		allowed[skillID] = struct{}{}
		toolName := strings.TrimSpace(stringFromAny(step["tool_name"]))
		if skills.RequiresPromptProfessionalizerPreflight(skillID, toolName) {
			allowed[skills.SkillPromptProfessionalizer] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		return nil
	}
	return allowed
}

func filterResolvedSkillsByAllowedIDs(resolved *skills.ResolvedSkills, allowed map[string]struct{}, strict bool) *skills.ResolvedSkills {
	if resolved == nil || len(resolved.Skills) == 0 || len(allowed) == 0 {
		return resolved
	}
	filtered := make([]skills.SkillDocument, 0, len(resolved.Skills))
	for _, doc := range resolved.Skills {
		skillID := strings.TrimSpace(doc.Metadata.ID)
		if _, ok := allowed[skillID]; ok {
			filtered = append(filtered, doc)
		}
	}
	if len(filtered) == 0 {
		if strict {
			return &skills.ResolvedSkills{}
		}
		return resolved
	}
	return &skills.ResolvedSkills{Skills: filtered}
}

func filterResolvedSkillsByAllowedTools(resolved *skills.ResolvedSkills, allowed map[string]struct{}, strict bool) *skills.ResolvedSkills {
	if resolved == nil || len(resolved.Skills) == 0 || len(allowed) == 0 {
		return resolved
	}
	filtered := make([]skills.SkillDocument, 0, len(resolved.Skills))
	for _, doc := range resolved.Skills {
		skillID := strings.TrimSpace(doc.Metadata.ID)
		if skillID == "" {
			continue
		}
		if _, ok := allowed[skillLoopToolWildcardAllowKey(skillID)]; ok {
			filtered = append(filtered, doc)
			continue
		}
		if len(doc.Tools) == 0 {
			if skillLoopAllowedToolsContainSkill(allowed, skillID) {
				filtered = append(filtered, doc)
			}
			continue
		}
		next := doc
		next.Tools = make([]skills.SkillToolDefinition, 0, len(doc.Tools))
		for _, tool := range doc.Tools {
			toolName := strings.TrimSpace(tool.Name)
			if toolName == "" {
				continue
			}
			if _, ok := allowed[skillLoopToolAllowKey(skillID, toolName)]; ok {
				next.Tools = append(next.Tools, tool)
			}
		}
		if len(next.Tools) > 0 {
			filtered = append(filtered, next)
		}
	}
	if len(filtered) == 0 {
		if strict {
			return &skills.ResolvedSkills{}
		}
		return resolved
	}
	return &skills.ResolvedSkills{Skills: filtered}
}

func turnStrategyAllowedSkillIDs(strategy *AIChatTurnStrategy) map[string]struct{} {
	if strategy == nil {
		return nil
	}
	allowed := map[string]struct{}{}
	add := func(skillID string) {
		skillID = strings.TrimSpace(skillID)
		if skillID != "" {
			allowed[skillID] = struct{}{}
		}
	}
	addTool := func(tool AIChatTurnStrategyTool) {
		skillID := strings.TrimSpace(tool.SkillID)
		toolName := strings.TrimSpace(tool.ToolName)
		add(skillID)
		if skills.RequiresPromptProfessionalizerPreflight(skillID, toolName) {
			add(skills.SkillPromptProfessionalizer)
		}
	}
	if strategy.RequiredNextTool != nil {
		addTool(*strategy.RequiredNextTool)
	}
	for _, tool := range strategy.PlannedTools {
		addTool(tool)
	}
	if len(allowed) > 0 {
		return allowed
	}
	if !turnStrategyIntentShouldRestrictToPrimary(strategy.Intent) {
		return nil
	}
	for _, skillID := range strategy.PrimarySkills {
		add(skillID)
	}
	if len(allowed) == 0 {
		return nil
	}
	return allowed
}

func turnStrategyIntentShouldRestrictToPrimary(intent string) bool {
	intent = strings.TrimSpace(intent)
	if strings.HasPrefix(intent, "continue_") {
		return true
	}
	switch intent {
	case "manage_agent_asset",
		"save_generated_file_to_file_management",
		"generate_temporary_file_artifact",
		"delete_visible_file",
		"read_visible_file_content",
		"navigate_console_page":
		return true
	default:
		return false
	}
}

func skillLoopPlanToolCallGuard(prepared *PreparedChat) skillloop.ToolCallGuard {
	return skillLoopPlanToolCallGuardWithResolved(prepared, nil)
}

func recordOperationPlanToolGuardDeviation(metadata map[string]interface{}, skillID string, toolName string, reason string, result skillloop.FinalAnswerGuardResult) {
	if result.Advisory {
		recordOperationPlanToolDeviation(metadata, skillID, toolName, reason)
		return
	}
	recordOperationPlanToolBlockedDeviation(metadata, skillID, toolName, reason)
}

func recordOperationPlanToolGuardDeviationForPrepared(prepared *PreparedChat, skillID string, toolName string, reason string, result skillloop.FinalAnswerGuardResult) {
	if prepared == nil || prepared.Message == nil {
		return
	}
	recordOperationPlanToolGuardDeviation(prepared.Message.Metadata, skillID, toolName, reason, result)
}

func skillLoopPlanToolCallGuardWithResolved(prepared *PreparedChat, resolved *skills.ResolvedSkills) skillloop.ToolCallGuard {
	contextualGuard := skillLoopToolCallGuard(prepared)
	operationPlanCompletedOnEntry := skillLoopOperationPlanStatusCompleted(prepared)
	return func(req skillloop.ToolCallGuardRequest) (skillloop.FinalAnswerGuardResult, bool) {
		skillID := strings.TrimSpace(req.SkillID)
		toolName := strings.TrimSpace(req.ToolName)
		if skillID == "" {
			return skillloop.FinalAnswerGuardResult{}, false
		}
		if skillLoopModelDecidesToolChoice(prepared) {
			return skillLoopModelDecidesSafetyToolCallGuard(prepared, resolved, req)
		}
		if contextualGuard != nil && skillLoopShouldApplyContextualPlanGuard(prepared) {
			if guardResult, blocked := contextualGuard(req); blocked {
				if prepared != nil && prepared.Message != nil {
					recordOperationPlanToolGuardDeviation(prepared.Message.Metadata, req.SkillID, req.ToolName, "contextual_execution_evidence_requires_different_next_step", guardResult)
				}
				return guardResult, true
			}
		}
		allowed := skillLoopAllowedPlannedTools(prepared)
		if skillLoopShouldBlockTerminalRecentAgentMutationContinuation(prepared, skillID, toolName) {
			recordOperationPlanToolBlockedDeviation(prepared.Message.Metadata, skillID, toolName, "ambiguous_continuation_cannot_repeat_terminal_agent_mutation")
			return skillLoopTerminalRecentAgentMutationContinuationGuardResult(skillID, toolName), true
		}
		if skillLoopShouldBlockDuplicateArtifactToolCall(req) {
			return skillLoopDuplicateArtifactGuardResult(req), true
		}
		if skillLoopShouldBlockDuplicateMutationToolCall(prepared, resolved, req) {
			return skillLoopDuplicateMutationGuardResultForRequest(prepared, req), true
		}
		if len(allowed) == 0 && !skillLoopShouldRestrictToOperationPlan(prepared) && !skillLoopCreateAndEditPlanActive(prepared) {
			return skillloop.FinalAnswerGuardResult{}, false
		}
		if toolName == "" {
			if skillLoopShouldAllowUnplannedSkillLoad(prepared, skillID) {
				recordOperationPlanToolDeviation(prepared.Message.Metadata, skillID, "", "model_loaded_enabled_skill_for_context")
				return skillloop.FinalAnswerGuardResult{}, false
			}
		}
		if toolName == "" && skillLoopAllowedToolsContainSkill(allowed, skillID) {
			return skillloop.FinalAnswerGuardResult{}, false
		}
		if skillLoopShouldAllowReadyPlannedNavigationTool(prepared, skillID, toolName, req.Arguments) {
			return skillloop.FinalAnswerGuardResult{}, false
		}
		if skillLoopShouldBlockRepeatedLoadedNavigation(prepared, skillID, toolName, req.Arguments) {
			return skillLoopRepeatedLoadedNavigationGuardResult(req.Arguments), true
		}
		if skillLoopShouldBlockAgentMutationForReadOnlyCapabilityGoal(prepared, req) {
			result := skillLoopReadOnlyAgentCapabilityMutationGuardResult(skillID, toolName)
			recordOperationPlanToolGuardDeviation(prepared.Message.Metadata, skillID, toolName, "model_requested_agent_mutation_for_readonly_capability_goal", result)
			return result, true
		}
		if skillLoopShouldAllowReadOnlyAgentCandidateLookup(prepared, skillID, toolName) {
			recordOperationPlanToolDeviation(prepared.Message.Metadata, skillID, toolName, "model_collected_readonly_agent_candidate_evidence")
			return skillloop.FinalAnswerGuardResult{}, false
		}
		if issue, blocked := skillLoopShouldBlockOverbroadAgentCandidateSelectionUpdate(prepared, req); blocked {
			result := skillLoopOverbroadAgentCandidateSelectionUpdateGuardResult(issue)
			recordOperationPlanToolGuardDeviation(prepared.Message.Metadata, skillID, toolName, "model_selected_too_many_agent_binding_candidates", result)
			return result, true
		}
		if issue, blocked := skillLoopShouldBlockInvalidAgentSkillBindingUpdate(req); blocked {
			result := skillLoopInvalidAgentSkillBindingUpdateGuardResult(issue)
			recordOperationPlanToolGuardDeviation(prepared.Message.Metadata, skillID, toolName, "model_used_unresolved_agent_skill_binding_target", result)
			return result, true
		}
		if result, blocked := skillLoopCreatedAgentTargetMismatchGuard(prepared, req); blocked {
			recordOperationPlanToolGuardDeviation(prepared.Message.Metadata, skillID, toolName, "model_targeted_different_agent_after_create_and_edit", result)
			return result, true
		}
		if result, blocked := skillLoopMissingCreatedAgentBindingGuard(prepared, req); blocked {
			recordOperationPlanToolGuardDeviation(prepared.Message.Metadata, skillID, toolName, "created_agent_binding_unresolved", result)
			return result, true
		}
		if skillLoopShouldAllowRepeatedPlannedMutation(prepared, req) {
			recordOperationPlanToolDeviation(prepared.Message.Metadata, skillID, toolName, "model_repeated_planned_mutation_within_user_goal")
			amendOperationPlanRepeatedToolStep(prepared.Message.Metadata, skillID, toolName, "model_repeated_planned_mutation_within_user_goal")
			return skillloop.FinalAnswerGuardResult{}, false
		}
		if !operationPlanCompletedOnEntry {
			if result, blocked := skillLoopCompletedMutationStepRepeatGuard(prepared, skillID, toolName); blocked {
				recordOperationPlanToolGuardDeviation(prepared.Message.Metadata, skillID, toolName, "model_repeated_completed_mutation_step_before_pending_plan_step", result)
				return result, true
			}
		}
		if result, blocked := skillLoopCompletedReadStepRepeatGuard(prepared, skillID, toolName); blocked {
			recordOperationPlanToolGuardDeviation(prepared.Message.Metadata, skillID, toolName, "model_repeated_completed_read_step_before_pending_plan_step", result)
			return result, true
		}
		if !operationPlanCompletedOnEntry && skillLoopShouldBlockEmptyAgentIdentityUpdate(req) {
			result := skillLoopEmptyAgentIdentityUpdateGuardResult(req.Arguments)
			recordOperationPlanToolGuardDeviation(prepared.Message.Metadata, skillID, toolName, "model_requested_empty_agent_identity_update", result)
			return result, true
		}
		if !operationPlanCompletedOnEntry && skillLoopShouldBlockEmptyAgentConfigUpdate(req) {
			result := skillLoopEmptyAgentConfigUpdateGuardResult(req.Arguments)
			recordOperationPlanToolGuardDeviation(prepared.Message.Metadata, skillID, toolName, "model_requested_empty_agent_config_update", result)
			return result, true
		}
		if !operationPlanCompletedOnEntry && skillLoopShouldBlockPartialAgentModelConfigUpdate(req) {
			result := skillLoopPartialAgentModelConfigUpdateGuardResult(req.Arguments)
			recordOperationPlanToolGuardDeviation(prepared.Message.Metadata, skillID, toolName, "model_requested_partial_agent_model_pair", result)
			return result, true
		}
		if skillLoopToolAllowed(allowed, skillID, toolName) {
			return skillloop.FinalAnswerGuardResult{}, false
		}
		if skillLoopShouldAllowUnplannedGovernedReadTool(prepared, resolved, skillID, toolName) {
			recordOperationPlanToolDeviation(prepared.Message.Metadata, skillID, toolName, "model_collected_manifest_read_evidence")
			return skillloop.FinalAnswerGuardResult{}, false
		}
		if skillLoopShouldAllowUnplannedArtifactGeneration(prepared, req) {
			recordOperationPlanToolDeviation(prepared.Message.Metadata, skillID, toolName, "model_generated_temporary_artifact_within_user_goal")
			amendOperationPlanToolStep(prepared.Message.Metadata, skillID, toolName, "model_generated_temporary_artifact_within_user_goal")
			return skillloop.FinalAnswerGuardResult{}, false
		}
		if skillLoopShouldAllowGovernedMutationDeviation(prepared, resolved, req) {
			recordOperationPlanToolDeviation(prepared.Message.Metadata, skillID, toolName, "model_requested_governed_mutation_under_runtime_governance")
			amendOperationPlanToolStep(prepared.Message.Metadata, skillID, toolName, "model_requested_governed_mutation_under_runtime_governance")
			return skillloop.FinalAnswerGuardResult{}, false
		}
		if skillLoopCanAmendOperationPlanForTool(prepared, skillID, toolName) {
			recordOperationPlanToolDeviation(prepared.Message.Metadata, skillID, toolName, "model_amended_operation_plan_within_user_goal")
			amendOperationPlanToolStep(prepared.Message.Metadata, skillID, toolName, "model_tool_call_within_user_goal")
			return skillloop.FinalAnswerGuardResult{}, false
		}
		if skillLoopShouldAllowUnplannedEvidenceTool(prepared, skillID, toolName) {
			recordOperationPlanToolDeviation(prepared.Message.Metadata, skillID, toolName, "model_collected_additional_evidence_within_user_goal")
			return skillloop.FinalAnswerGuardResult{}, false
		}
		if skillLoopShouldAllowUnplannedObservationTool(prepared, skillID, toolName) {
			recordOperationPlanToolDeviation(prepared.Message.Metadata, skillID, toolName, "model_collected_unplanned_readonly_evidence")
			return skillloop.FinalAnswerGuardResult{}, false
		}
		if skillLoopShouldAllowUnplannedNavigationTool(prepared, skillID, toolName, req.Arguments) {
			recordOperationPlanToolDeviation(prepared.Message.Metadata, skillID, toolName, "model_navigated_for_page_context_within_user_goal")
			return skillloop.FinalAnswerGuardResult{}, false
		}
		if skillLoopShouldAllowUnplannedAdvisoryTool(prepared, skillID, toolName) {
			recordOperationPlanToolDeviation(prepared.Message.Metadata, skillID, toolName, "model_used_unplanned_non_mutating_tool")
			return skillloop.FinalAnswerGuardResult{}, false
		}
		recordOperationPlanToolBlockedDeviation(prepared.Message.Metadata, skillID, toolName, "model_requested_unplanned_tool_without_safe_current_goal_match")
		return skillLoopUnplannedToolGuardResult(skillID, toolName), true
	}
}

func skillLoopModelDecidesSafetyToolCallGuard(prepared *PreparedChat, resolved *skills.ResolvedSkills, req skillloop.ToolCallGuardRequest) (skillloop.FinalAnswerGuardResult, bool) {
	skillID := strings.TrimSpace(req.SkillID)
	toolName := strings.TrimSpace(req.ToolName)
	if skillID == "" {
		return skillloop.FinalAnswerGuardResult{}, false
	}
	if skillLoopShouldAllowRepeatedPlannedMutation(prepared, req) {
		recordOperationPlanToolDeviation(prepared.Message.Metadata, skillID, toolName, "model_repeated_planned_mutation_within_user_goal")
		amendOperationPlanRepeatedToolStep(prepared.Message.Metadata, skillID, toolName, "model_repeated_planned_mutation_within_user_goal")
		return skillloop.FinalAnswerGuardResult{}, false
	}
	if skillLoopShouldBlockDuplicateArtifactToolCall(req) {
		return skillLoopDuplicateArtifactGuardResult(req), true
	}
	if skillLoopShouldBlockDuplicateMutationToolCall(prepared, resolved, req) {
		return skillLoopDuplicateMutationGuardResultForRequest(prepared, req), true
	}
	if skillLoopShouldBlockAgentMutationForReadOnlyCapabilityGoal(prepared, req) {
		result := skillLoopReadOnlyAgentCapabilityMutationGuardResult(skillID, toolName)
		recordOperationPlanToolGuardDeviationForPrepared(prepared, skillID, toolName, "model_requested_agent_mutation_for_readonly_capability_goal", result)
		return result, true
	}
	if result, blocked := skillLoopCreatedAgentTargetMismatchGuard(prepared, req); blocked {
		recordOperationPlanToolGuardDeviationForPrepared(prepared, skillID, toolName, "model_targeted_different_agent_after_create_and_edit", result)
		return result, true
	}
	if result, blocked := skillLoopMissingCreatedAgentBindingGuard(prepared, req); blocked {
		recordOperationPlanToolGuardDeviationForPrepared(prepared, skillID, toolName, "created_agent_binding_unresolved", result)
		return result, true
	}
	return skillloop.FinalAnswerGuardResult{}, false
}

func skillLoopCreatedAgentTargetMismatchGuard(prepared *PreparedChat, req skillloop.ToolCallGuardRequest) (skillloop.FinalAnswerGuardResult, bool) {
	if prepared == nil || prepared.Message == nil ||
		!strings.EqualFold(strings.TrimSpace(req.SkillID), skills.SkillAgentManagement) ||
		!agentManagementToolRequiresSingleAgentTarget(req.ToolName) ||
		!skillLoopCreateAndEditPlanActive(prepared) {
		return skillloop.FinalAnswerGuardResult{}, false
	}
	targets := skillLoopBoundAgentTargets(prepared, req)
	if len(targets) == 0 {
		return skillloop.FinalAnswerGuardResult{}, false
	}
	requestedIDs := agentManagementSingleAgentTargetIDs(req.Arguments)
	if len(requestedIDs) == 0 {
		return skillloop.FinalAnswerGuardResult{}, false
	}
	for _, requestedID := range requestedIDs {
		for _, target := range targets {
			if strings.EqualFold(requestedID, target.AgentID) {
				return skillloop.FinalAnswerGuardResult{}, false
			}
		}
	}
	targetLabel := skillLoopCreatedAgentTargetsLabel(targets)
	if targetLabel == "" {
		targetLabel = targets[0].AgentID
	}
	message := strings.Join([]string{
		"agent-management/" + strings.TrimSpace(req.ToolName) + " targeted a different Agent than the one created in this turn.",
		"Continue with the created Agent target: " + targetLabel + ".",
	}, " ")
	systemMessage := strings.Join([]string{
		message,
		"The current create-and-edit task target is the Agent created earlier in this same turn.",
		"Do not use stale current-page or old visible Agent IDs for this follow-up edit.",
		"Retry agent-management/" + strings.TrimSpace(req.ToolName) + " with the bound created Agent target.",
	}, " ")
	return skillloop.FinalAnswerGuardResult{
		SkillID:       skills.SkillAgentManagement,
		ToolName:      strings.TrimSpace(req.ToolName),
		Message:       message,
		SystemMessage: systemMessage,
		Advisory:      true,
	}, true
}

func skillLoopMissingCreatedAgentBindingGuard(prepared *PreparedChat, req skillloop.ToolCallGuardRequest) (skillloop.FinalAnswerGuardResult, bool) {
	if prepared == nil || prepared.Message == nil ||
		!strings.EqualFold(strings.TrimSpace(req.SkillID), skills.SkillAgentManagement) ||
		!agentManagementToolRequiresSingleAgentTarget(req.ToolName) ||
		!skillLoopCreateAndEditPlanActive(prepared) {
		return skillloop.FinalAnswerGuardResult{}, false
	}
	binding := skillLoopPlanArgsBindingForTool(prepared.Message.Metadata, req.SkillID, req.ToolName)
	expr := strings.TrimSpace(binding["agent_id"])
	if expr == "" || !strings.HasPrefix(expr, "$created_agents") {
		return skillloop.FinalAnswerGuardResult{}, false
	}
	if value, ok := skillLoopResolveToolArgBindingExpression(prepared, req, expr); ok && strings.TrimSpace(value) != "" {
		return skillloop.FinalAnswerGuardResult{}, false
	}
	toolLabel := "agent-management/" + strings.TrimSpace(req.ToolName)
	message := toolLabel + " requires the Agent created earlier in this turn, but no successful create_agent result is available."
	systemMessage := strings.Join([]string{
		message,
		"Do not use the current page Agent or any visible stale Agent as a fallback target.",
		"If create_agent failed, retry create_agent with corrected arguments when safe; otherwise report the exact failure to the user.",
	}, " ")
	return skillloop.FinalAnswerGuardResult{
		SkillID:       skills.SkillAgentManagement,
		ToolName:      strings.TrimSpace(req.ToolName),
		Message:       message,
		SystemMessage: systemMessage,
		Advisory:      true,
	}, true
}

func skillLoopBoundAgentTargets(prepared *PreparedChat, req skillloop.ToolCallGuardRequest) []skillLoopCreatedAgentTargetRef {
	binding := skillLoopPlanArgsBindingForTool(prepared.Message.Metadata, req.SkillID, req.ToolName)
	if expr := strings.TrimSpace(binding["agent_id"]); expr != "" {
		targets := skillLoopCreatedAgentTargets(prepared, req)
		selector, _, ok := skillLoopParseCreatedAgentsBinding(expr)
		if !ok {
			return nil
		}
		target, ok := skillLoopSelectCreatedAgentTarget(targets, selector)
		if !ok || target.AgentID == "" {
			return nil
		}
		return []skillLoopCreatedAgentTargetRef{target}
	}
	targets := skillLoopCreatedAgentTargets(prepared, req)
	if len(targets) != 1 {
		return nil
	}
	return targets
}

func skillLoopCreatedAgentTargetsLabel(targets []skillLoopCreatedAgentTargetRef) string {
	labels := make([]string, 0, len(targets))
	for _, target := range targets {
		if target.AgentID == "" {
			continue
		}
		label := target.AgentID
		if target.AgentName != "" {
			label = target.AgentName + " (" + target.AgentID + ")"
		}
		labels = append(labels, label)
	}
	return strings.Join(labels, ", ")
}

func skillLoopCreateAndEditPlanActive(prepared *PreparedChat) bool {
	if prepared == nil || prepared.Message == nil {
		return false
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if len(plan) == 0 {
		return false
	}
	if structuredPlan := mapFromOperationContext(plan["structured_plan"]); len(structuredPlan) > 0 &&
		strings.EqualFold(strings.TrimSpace(stringFromAny(structuredPlan["intent"])), "agent.create_and_edit") {
		return true
	}
	capabilityGoals := preparedAgentCapabilityGoals(prepared)
	if !skillLoopAgentManagementCreateEffectActive(prepared, plan) {
		return false
	}
	if agentCapabilityGoalsRequireConfigMutation(capabilityGoals) {
		return true
	}
	if prepared.parts != nil && agentManagementModelIntentRequestsIdentityMutation(prepared.parts.ModelTurnIntent) {
		return true
	}
	if operationPlanHasToolStepWithStatus(plan, skills.SkillAgentManagement, "update_agent_identity", operationPlanStepStatusPending) ||
		operationPlanHasToolStepWithStatus(plan, skills.SkillAgentManagement, "update_agent_identity", operationPlanStepStatusCompleted) {
		return true
	}
	return false
}

func skillLoopAgentManagementCreateEffectActive(prepared *PreparedChat, plan map[string]interface{}) bool {
	if len(plan) > 0 {
		if target := mapFromOperationContext(plan["asset_target"]); strings.EqualFold(strings.TrimSpace(stringFromAny(target["effect"])), "create") {
			return true
		}
		if operationPlanHasToolStepWithStatus(plan, skills.SkillAgentManagement, "create_agent", operationPlanStepStatusPending) ||
			operationPlanHasToolStepWithStatus(plan, skills.SkillAgentManagement, "create_agent", operationPlanStepStatusCompleted) {
			return true
		}
	}
	if prepared != nil && prepared.parts != nil && prepared.parts.ModelTurnIntent != nil &&
		strings.EqualFold(strings.TrimSpace(prepared.parts.ModelTurnIntent.AssetEffect), "create") {
		return true
	}
	if prepared != nil && prepared.Message != nil &&
		len(successfulMetadataToolCalls(prepared.Message.Metadata, skills.SkillAgentManagement, "create_agent")) > 0 {
		return true
	}
	return false
}

func agentManagementModelIntentRequestsIdentityMutation(intent *AIChatModelTurnIntent) bool {
	return modelTurnIntentHasRecommendedCapability(intent,
		"agent.identity",
		"agent.profile",
		"agent.name",
		"agent.description",
		"agent.icon",
	)
}

type skillLoopCreatedAgentTargetRef struct {
	AgentID   string
	AgentName string
	Href      string
	ClientKey string
}

func skillLoopCreatedAgentTargets(prepared *PreparedChat, req skillloop.ToolCallGuardRequest) []skillLoopCreatedAgentTargetRef {
	if prepared == nil || prepared.Message == nil {
		return nil
	}
	calls := append(successfulMetadataToolCalls(prepared.Message.Metadata, skills.SkillAgentManagement, "create_agent"),
		matchingSkillToolCalls(req.SuccessfulToolCalls, skills.SkillAgentManagement, "create_agent")...)
	targets := make([]skillLoopCreatedAgentTargetRef, 0, len(calls))
	seen := map[string]struct{}{}
	for _, call := range calls {
		target, ok := skillLoopCreatedAgentTargetFromCall(call)
		if !ok {
			continue
		}
		if _, exists := seen[strings.ToLower(target.AgentID)]; exists {
			continue
		}
		seen[strings.ToLower(target.AgentID)] = struct{}{}
		targets = append(targets, target)
	}
	return targets
}

func skillLoopCreatedAgentTargetFromCall(call skillloop.SkillToolCallRef) (skillLoopCreatedAgentTargetRef, bool) {
	agent := mapFromOperationContext(call.Result["agent"])
	agentID := strings.TrimSpace(firstNonEmptyString(
		call.Result["agent_id"],
		call.Result["id"],
		agent["agent_id"],
		agent["id"],
	))
	if agentID == "" {
		return skillLoopCreatedAgentTargetRef{}, false
	}
	agentName := strings.TrimSpace(firstNonEmptyString(
		call.Result["agent_name"],
		call.Result["name"],
		agent["agent_name"],
		agent["name"],
		call.Arguments["name"],
		call.Arguments["agent_name"],
	))
	clientKey := strings.TrimSpace(firstNonEmptyString(
		call.Result["client_key"],
		agent["client_key"],
		call.Arguments["client_key"],
	))
	if clientKey == "" {
		clientKey = normalizeAgentBindingClientKey(agentName)
	}
	return skillLoopCreatedAgentTargetRef{
		AgentID:   agentID,
		AgentName: agentName,
		Href: strings.TrimSpace(firstNonEmptyString(
			call.Result["href"],
			agent["href"],
		)),
		ClientKey: clientKey,
	}, true
}

func normalizeAgentBindingClientKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		switch {
		case unicode.IsSpace(r) || r == '-' || r == '_':
			if !lastUnderscore && b.Len() > 0 {
				b.WriteRune('_')
				lastUnderscore = true
			}
		default:
			b.WriteRune(r)
			lastUnderscore = false
		}
	}
	return strings.Trim(b.String(), "_")
}

func skillLoopResolveCreatedAgentsBinding(prepared *PreparedChat, req skillloop.ToolCallGuardRequest, expr string) (string, bool) {
	targets := skillLoopCreatedAgentTargets(prepared, req)
	if len(targets) == 0 {
		return "", false
	}
	selector, field, ok := skillLoopParseCreatedAgentsBinding(expr)
	if !ok {
		return "", false
	}
	target, ok := skillLoopSelectCreatedAgentTarget(targets, selector)
	if !ok {
		return "", false
	}
	switch field {
	case "agent_id", "id":
		return target.AgentID, target.AgentID != ""
	case "name", "agent_name":
		return target.AgentName, target.AgentName != ""
	case "href":
		return target.Href, target.Href != ""
	case "client_key":
		return target.ClientKey, target.ClientKey != ""
	default:
		return "", false
	}
}

func skillLoopParseCreatedAgentsBinding(expr string) (map[string]string, string, bool) {
	expr = strings.TrimSpace(expr)
	if expr == "" || !strings.HasPrefix(expr, "$created_agents") {
		return nil, "", false
	}
	rest := strings.TrimPrefix(expr, "$created_agents")
	selector := map[string]string{}
	if strings.HasPrefix(rest, "[") {
		end := strings.Index(rest, "]")
		if end < 0 {
			return nil, "", false
		}
		rawSelector := strings.TrimSpace(rest[1:end])
		rest = rest[end+1:]
		if rawSelector != "" {
			if idx, err := strconv.Atoi(rawSelector); err == nil {
				selector["index"] = strconv.Itoa(idx)
			} else {
				parts := strings.SplitN(rawSelector, "=", 2)
				if len(parts) != 2 {
					return nil, "", false
				}
				key := strings.ToLower(strings.TrimSpace(parts[0]))
				value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
				if key == "" || value == "" {
					return nil, "", false
				}
				selector[key] = value
			}
		}
	}
	if !strings.HasPrefix(rest, ".") {
		return nil, "", false
	}
	field := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(rest, ".")))
	if field == "" || strings.Contains(field, ".") {
		return nil, "", false
	}
	return selector, field, true
}

func skillLoopSelectCreatedAgentTarget(targets []skillLoopCreatedAgentTargetRef, selector map[string]string) (skillLoopCreatedAgentTargetRef, bool) {
	if len(targets) == 0 {
		return skillLoopCreatedAgentTargetRef{}, false
	}
	if len(selector) == 0 {
		return targets[0], true
	}
	if rawIndex := strings.TrimSpace(selector["index"]); rawIndex != "" {
		index, err := strconv.Atoi(rawIndex)
		if err != nil || index < 0 || index >= len(targets) {
			return skillLoopCreatedAgentTargetRef{}, false
		}
		return targets[index], true
	}
	if name := strings.TrimSpace(selector["name"]); name != "" {
		for _, target := range targets {
			if strings.EqualFold(strings.TrimSpace(target.AgentName), name) {
				return target, true
			}
		}
	}
	if clientKey := normalizeAgentBindingClientKey(selector["client_key"]); clientKey != "" {
		for _, target := range targets {
			if strings.EqualFold(normalizeAgentBindingClientKey(target.ClientKey), clientKey) ||
				strings.EqualFold(normalizeAgentBindingClientKey(target.AgentName), clientKey) {
				return target, true
			}
		}
	}
	return skillLoopCreatedAgentTargetRef{}, false
}

func agentManagementToolRequiresSingleAgentTarget(toolName string) bool {
	switch strings.TrimSpace(toolName) {
	case "get_agent",
		"get_agent_config",
		"update_agent_identity",
		"update_agent_config",
		"replace_agent_memory_slots",
		"list_agent_skill_candidates",
		"list_agent_knowledge_candidates",
		"list_agent_database_candidates",
		"list_agent_database_tables",
		"list_agent_workflow_binding_candidates":
		return true
	default:
		return false
	}
}

func agentManagementSingleAgentTargetIDs(args map[string]interface{}) []string {
	if len(args) == 0 {
		return nil
	}
	ids := []string{}
	addID := func(value interface{}) {
		if id := strings.TrimSpace(firstNonEmptyString(value)); id != "" {
			ids = appendUniqueStrings(ids, id)
		}
	}
	for _, key := range []string{"agent_id", "id", "asset_id", "resource_id"} {
		addID(args[key])
	}
	for _, agent := range mapSliceFromAny(args["agents"]) {
		addID(firstNonEmptyString(agent["agent_id"], agent["id"], agent["asset_id"], agent["resource_id"]))
	}
	if raw := strings.TrimSpace(stringFromAny(args["agents"])); raw != "" {
		var parsed []map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			for _, agent := range parsed {
				addID(firstNonEmptyString(agent["agent_id"], agent["id"], agent["asset_id"], agent["resource_id"]))
			}
		}
	}
	return ids
}

func skillLoopCompletedMutationStepRepeatGuard(prepared *PreparedChat, skillID string, toolName string) (skillloop.FinalAnswerGuardResult, bool) {
	if prepared == nil || prepared.Message == nil || prepared.parts == nil {
		return skillloop.FinalAnswerGuardResult{}, false
	}
	skillID = strings.TrimSpace(skillID)
	toolName = strings.TrimSpace(toolName)
	if skillID == "" || toolName == "" || !skillLoopToolLooksAssetMutation(skillID, toolName) {
		return skillloop.FinalAnswerGuardResult{}, false
	}
	if strings.EqualFold(skillID, skills.SkillAgentManagement) && strings.EqualFold(toolName, "create_agent") {
		return skillloop.FinalAnswerGuardResult{}, false
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if len(plan) == 0 || strings.EqualFold(strings.TrimSpace(stringFromAny(plan["status"])), operationPlanStatusCompleted) {
		return skillloop.FinalAnswerGuardResult{}, false
	}
	if !operationPlanHasToolStepWithStatus(plan, skillID, toolName, operationPlanStepStatusCompleted) {
		return skillloop.FinalAnswerGuardResult{}, false
	}
	next, ok := skillLoopNextReadyPendingPlanStep(plan)
	if !ok {
		return skillloop.FinalAnswerGuardResult{}, false
	}
	nextSkillID := strings.TrimSpace(stringFromAny(next["skill_id"]))
	nextToolName := strings.TrimSpace(stringFromAny(next["tool_name"]))
	if nextSkillID == "" || nextToolName == "" ||
		(strings.EqualFold(nextSkillID, skillID) && strings.EqualFold(nextToolName, toolName)) {
		return skillloop.FinalAnswerGuardResult{}, false
	}
	blockedTool := skillID + "/" + toolName
	nextTool := nextSkillID + "/" + nextToolName
	message := strings.Join([]string{
		blockedTool + " already has successful mutation evidence for this turn.",
		"Continue with the next pending planned step: " + nextTool + ".",
	}, " ")
	systemMessage := strings.Join([]string{
		message,
		"Do not repeat completed asset mutation tools or request another approval for the same completed plan step.",
		"Use the existing mutation result already present in the conversation context.",
		"Call the next pending planned tool instead; if required IDs or page context are missing, explain the missing evidence truthfully instead of rerunning the completed mutation.",
	}, " ")
	return skillloop.FinalAnswerGuardResult{
		SkillID:       nextSkillID,
		ToolName:      nextToolName,
		Message:       message,
		SystemMessage: systemMessage,
		Advisory:      true,
	}, true
}

func skillLoopNextReadyPendingPlanStep(plan map[string]interface{}) (map[string]interface{}, bool) {
	if len(plan) == 0 {
		return nil, false
	}
	stepStatus := mapFromOperationContext(plan["step_status"])
	for _, step := range operationPlanPendingExecutableStepsForToolExposure(plan, 8) {
		if !operationPlanStepWaitForReadyFromPlan(step, stepStatus) {
			continue
		}
		skillID := strings.TrimSpace(stringFromAny(step["skill_id"]))
		toolName := strings.TrimSpace(stringFromAny(step["tool_name"]))
		if skillID == "" || toolName == "" {
			continue
		}
		return step, true
	}
	return nil, false
}

func skillLoopCompletedReadStepRepeatGuard(prepared *PreparedChat, skillID string, toolName string) (skillloop.FinalAnswerGuardResult, bool) {
	if prepared == nil || prepared.Message == nil || prepared.parts == nil {
		return skillloop.FinalAnswerGuardResult{}, false
	}
	skillID = strings.TrimSpace(skillID)
	toolName = strings.TrimSpace(toolName)
	if skillID == "" || toolName == "" || !skillLoopToolLooksReadOnly(skillID, toolName) {
		return skillloop.FinalAnswerGuardResult{}, false
	}
	if skillLoopCompletedReadStepCanReplayBeforePending(skillID, toolName) {
		return skillloop.FinalAnswerGuardResult{}, false
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if len(plan) == 0 || strings.EqualFold(strings.TrimSpace(stringFromAny(plan["status"])), operationPlanStatusCompleted) {
		return skillloop.FinalAnswerGuardResult{}, false
	}
	if !operationPlanHasToolStepWithStatus(plan, skillID, toolName, operationPlanStepStatusCompleted) {
		return skillloop.FinalAnswerGuardResult{}, false
	}
	pending := operationPlanPendingExecutableStepsForToolExposure(plan, 1)
	if len(pending) == 0 {
		return skillloop.FinalAnswerGuardResult{}, false
	}
	next := pending[0]
	nextSkillID := strings.TrimSpace(stringFromAny(next["skill_id"]))
	nextToolName := strings.TrimSpace(stringFromAny(next["tool_name"]))
	if nextSkillID == "" || nextToolName == "" ||
		(strings.EqualFold(nextSkillID, skillID) && strings.EqualFold(nextToolName, toolName)) {
		return skillloop.FinalAnswerGuardResult{}, false
	}
	message := strings.Join([]string{
		skillID + "/" + toolName + " already has successful evidence for this turn.",
		"Continue with the next pending planned step: " + nextSkillID + "/" + nextToolName + ".",
	}, " ")
	systemMessage := strings.Join([]string{
		message,
		"Do not repeat the completed read/list/search/observe tool.",
		"Use the existing tool result already present in the conversation context.",
		"Call the next pending planned tool instead; if required IDs are still missing, explain the missing evidence instead of looping.",
	}, " ")
	return skillloop.FinalAnswerGuardResult{
		SkillID:       nextSkillID,
		ToolName:      nextToolName,
		Message:       message,
		SystemMessage: systemMessage,
		Advisory:      true,
	}, true
}

func skillLoopCompletedReadStepCanReplayBeforePending(skillID string, toolName string) bool {
	if strings.EqualFold(strings.TrimSpace(skillID), skills.SkillAgentManagement) {
		switch strings.TrimSpace(toolName) {
		case "get_agent", "get_agent_config", "list_available_models":
			return true
		default:
			return false
		}
	}
	return false
}

func skillLoopShouldApplyContextualPlanGuard(prepared *PreparedChat) bool {
	if prepared == nil || prepared.Message == nil || prepared.parts == nil {
		return false
	}
	if plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"]); len(plan) > 0 {
		return true
	}
	return isContinuationIntent(prepared.parts.Query) && generatedFileMetadataHasAnyArtifact(prepared.Message.Metadata)
}

func skillLoopOperationPlanStatusCompleted(prepared *PreparedChat) bool {
	if prepared == nil || prepared.Message == nil {
		return false
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	return len(plan) > 0 && strings.EqualFold(strings.TrimSpace(stringFromAny(plan["status"])), operationPlanStatusCompleted)
}

func skillLoopShouldBlockEmptyAgentIdentityUpdate(req skillloop.ToolCallGuardRequest) bool {
	return strings.EqualFold(strings.TrimSpace(req.SkillID), skills.SkillAgentManagement) &&
		strings.EqualFold(strings.TrimSpace(req.ToolName), "update_agent_identity") &&
		!agentIdentityUpdateHasRequestedFields(req.Arguments)
}

func skillLoopShouldBlockEmptyAgentConfigUpdate(req skillloop.ToolCallGuardRequest) bool {
	return strings.EqualFold(strings.TrimSpace(req.SkillID), skills.SkillAgentManagement) &&
		strings.EqualFold(strings.TrimSpace(req.ToolName), "update_agent_config") &&
		len(req.Arguments) > 0 &&
		!agentConfigUpdateHasPatchFields(req.Arguments)
}

func skillLoopShouldBlockPartialAgentModelConfigUpdate(req skillloop.ToolCallGuardRequest) bool {
	if !strings.EqualFold(strings.TrimSpace(req.SkillID), skills.SkillAgentManagement) ||
		!strings.EqualFold(strings.TrimSpace(req.ToolName), "update_agent_config") ||
		len(req.Arguments) == 0 {
		return false
	}
	modelProvider := skillToolCallArgumentString(req.Arguments, "model_provider")
	model := skillToolCallArgumentString(req.Arguments, "model")
	return (modelProvider == "") != (model == "")
}

func skillLoopEmptyAgentIdentityUpdateGuardResult(arguments map[string]interface{}) skillloop.FinalAnswerGuardResult {
	agentID := strings.TrimSpace(firstNonEmptyString(arguments["agent_id"], arguments["id"], arguments["asset_id"]))
	message := "update_agent_identity was called without any identity fields"
	if agentID != "" {
		message = "update_agent_identity was called for Agent " + agentID + " without any identity fields"
	}
	return skillloop.FinalAnswerGuardResult{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_identity",
		Message:  message,
		Advisory: true,
		SystemMessage: strings.Join([]string{
			message + ".",
			"This would not change the Agent name, description, or icon, so do not request governance approval for it.",
			"Use the existing create/update/read evidence to continue, call a read tool if verification is still needed, or provide the final answer truthfully from the ledger.",
		}, " "),
	}
}

func skillLoopEmptyAgentConfigUpdateGuardResult(arguments map[string]interface{}) skillloop.FinalAnswerGuardResult {
	agentID := strings.TrimSpace(firstNonEmptyString(arguments["agent_id"], arguments["id"], arguments["asset_id"]))
	message := "update_agent_config was called without any config patch fields"
	if agentID != "" {
		message = "update_agent_config was called for Agent " + agentID + " without any config patch fields"
	}
	return skillloop.FinalAnswerGuardResult{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Message:  message,
		Advisory: true,
		SystemMessage: strings.Join([]string{
			message + ".",
			"A config_goal is only a planning note; it is not an executable patch.",
			"Do not request governance approval for this no-op update.",
			"Use get_agent_config and read-only candidate tools to answer capability or configuration questions, or retry update_agent_config only with concrete fields such as model_provider, model, system_prompt, file_upload_enabled, enabled_skill_ids, knowledge_dataset_ids, database_bindings, or workflow_bindings.",
		}, " "),
	}
}

func skillLoopPartialAgentModelConfigUpdateGuardResult(arguments map[string]interface{}) skillloop.FinalAnswerGuardResult {
	agentID := strings.TrimSpace(firstNonEmptyString(arguments["agent_id"], arguments["id"], arguments["asset_id"]))
	modelProvider := skillToolCallArgumentString(arguments, "model_provider")
	model := skillToolCallArgumentString(arguments, "model")
	missing := "model_provider"
	if model == "" {
		missing = "model"
	}
	message := "update_agent_config was called with an incomplete provider/model pair"
	if agentID != "" {
		message = "update_agent_config was called for Agent " + agentID + " with an incomplete provider/model pair"
	}
	return skillloop.FinalAnswerGuardResult{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Message:  message,
		Advisory: true,
		SystemMessage: strings.Join([]string{
			message + ".",
			"Model replacement must update model_provider and model together from the same list_available_models result item.",
			"Missing field: " + missing + ".",
			"Current attempted model_provider=" + firstNonEmptyString(modelProvider, "<empty>") + ", model=" + firstNonEmptyString(model, "<empty>") + ".",
			"Do not request governance approval for this invalid patch.",
			"Call agent-management/list_available_models if a valid pair has not already been resolved, then retry update_agent_config with both model_provider and model.",
		}, " "),
	}
}

func skillLoopShouldAllowReadOnlyAgentCandidateLookup(prepared *PreparedChat, skillID string, toolName string) bool {
	if prepared == nil || prepared.parts == nil ||
		!strings.EqualFold(strings.TrimSpace(skillID), skills.SkillAgentManagement) {
		return false
	}
	toolName = strings.TrimSpace(toolName)
	if !agentManagementToolIsReadOnlyCandidateLookup(toolName) {
		return false
	}
	capabilityGoals := preparedAgentCapabilityGoals(prepared)
	capabilityMutationRequested := agentCapabilityGoalsRequireConfigMutation(capabilityGoals)
	if capabilityMutationRequested {
		return false
	}
	capabilityReadOnly := agentCapabilityGoalsAreExplicitReadOnly(capabilityGoals)
	if !capabilityReadOnly {
		return false
	}
	if skillLoopPlanHasAgentManagementMutationStep(prepared) {
		return false
	}
	return true
}

func skillLoopShouldBlockAgentMutationForReadOnlyCapabilityGoal(prepared *PreparedChat, req skillloop.ToolCallGuardRequest) bool {
	if prepared == nil || prepared.Message == nil || prepared.parts == nil {
		return false
	}
	skillID := strings.TrimSpace(req.SkillID)
	toolName := strings.TrimSpace(req.ToolName)
	if !strings.EqualFold(skillID, skills.SkillAgentManagement) || toolName == "" {
		return false
	}
	if !skillLoopToolLooksAssetMutation(skillID, toolName) {
		return false
	}
	if !agentCapabilityGoalsAreExplicitReadOnly(preparedAgentCapabilityGoals(prepared)) {
		return false
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	return !operationPlanHasToolStepWithStatus(plan, skillID, toolName, operationPlanStepStatusPending)
}

func skillLoopReadOnlyAgentCapabilityMutationGuardResult(skillID string, toolName string) skillloop.FinalAnswerGuardResult {
	return skillloop.FinalAnswerGuardResult{
		SkillID:  strings.TrimSpace(skillID),
		ToolName: strings.TrimSpace(toolName),
		Message:  "asset-changing Agent tool call conflicts with the current read-only Agent capability goal",
		SystemMessage: strings.Join([]string{
			"The current Agent capability goal is explicitly read-only.",
			"Do not call asset-changing Agent tools unless the operation plan contains a concrete pending mutation step for the same tool.",
			"Use get_agent_config or read-only candidate-list tools, then answer from the returned evidence.",
		}, " "),
	}
}

func agentManagementToolIsReadOnlyCandidateLookup(toolName string) bool {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "list_agent_skill_candidates",
		"list_agent_knowledge_candidates",
		"list_agent_database_candidates",
		"list_agent_database_tables",
		"list_agent_workflow_binding_candidates",
		"list_available_models",
		"list_agents":
		return true
	default:
		return false
	}
}

func skillLoopPlanHasAgentManagementMutationStep(prepared *PreparedChat) bool {
	if prepared == nil || prepared.Message == nil {
		return false
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if len(plan) == 0 {
		return false
	}
	for _, step := range mapSliceFromAny(plan["steps"]) {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(step["skill_id"])), skills.SkillAgentManagement) {
			continue
		}
		toolName := strings.TrimSpace(stringFromAny(step["tool_name"]))
		if toolName == "" || !skillLoopToolLooksAssetMutation(skills.SkillAgentManagement, toolName) {
			continue
		}
		status := operationPlanNormalizeStepStatus(firstNonEmptyString(step["status"], mapFromOperationContext(plan["step_status"])[strings.TrimSpace(stringFromAny(step["id"]))]))
		if status != operationPlanStepStatusFailed {
			return true
		}
	}
	return false
}

type agentCandidateSelectionUpdateIssue struct {
	OverLimitFields []string
	InvalidSkillIDs []string
}

type agentSkillBindingUpdateIssue struct {
	InvalidSkillIDs []string
}

func skillLoopShouldBlockInvalidAgentSkillBindingUpdate(req skillloop.ToolCallGuardRequest) (agentSkillBindingUpdateIssue, bool) {
	if !strings.EqualFold(strings.TrimSpace(req.SkillID), skills.SkillAgentManagement) ||
		!strings.EqualFold(strings.TrimSpace(req.ToolName), "update_agent_config") ||
		len(req.Arguments) == 0 {
		return agentSkillBindingUpdateIssue{}, false
	}
	issue := agentSkillBindingUpdateIssue{}
	for _, field := range []string{"enabled_skill_ids", "add_enabled_skill_ids", "remove_enabled_skill_ids"} {
		for _, skillID := range skillLoopStringListArgument(req.Arguments, field) {
			skillID = strings.TrimSpace(skillID)
			if skillID != "" && !skillLoopLooksLikeAgentSkillID(skillID) {
				issue.InvalidSkillIDs = appendUniqueStrings(issue.InvalidSkillIDs, skillID)
			}
		}
	}
	return issue, len(issue.InvalidSkillIDs) > 0
}

func skillLoopInvalidAgentSkillBindingUpdateGuardResult(issue agentSkillBindingUpdateIssue) skillloop.FinalAnswerGuardResult {
	message := "update_agent_config includes unresolved Agent Skill binding targets"
	details := []string{}
	if len(issue.InvalidSkillIDs) > 0 {
		details = append(details, "Invalid skill values: "+strings.Join(issue.InvalidSkillIDs, ", ")+".")
	}
	return skillloop.FinalAnswerGuardResult{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Message:  message,
		Advisory: true,
		SystemMessage: strings.Join(append([]string{
			message + ".",
			"Do not request governance approval for Agent Skill binding changes unless the target Skill is resolved to a real skill_id.",
			"Call list_agent_skill_candidates with the user's requested Skill name when needed.",
			"If no matching candidate is returned, stop this Skill binding change and tell the user no matching Skill was found.",
			"Use only candidate.skill_id values in enabled_skill_ids, add_enabled_skill_ids, or remove_enabled_skill_ids; display names belong only in display_names.",
		}, details...), " "),
	}
}

func skillLoopShouldBlockOverbroadAgentCandidateSelectionUpdate(prepared *PreparedChat, req skillloop.ToolCallGuardRequest) (agentCandidateSelectionUpdateIssue, bool) {
	if prepared == nil || prepared.parts == nil ||
		!strings.EqualFold(strings.TrimSpace(req.SkillID), skills.SkillAgentManagement) ||
		!strings.EqualFold(strings.TrimSpace(req.ToolName), "update_agent_config") ||
		len(req.Arguments) == 0 {
		return agentCandidateSelectionUpdateIssue{}, false
	}
	if !skillLoopPlanRequiresSingleAgentBindingCandidate(prepared) {
		return agentCandidateSelectionUpdateIssue{}, false
	}

	issue := agentCandidateSelectionUpdateIssue{}
	addOverLimit := func(field string, count int) {
		if count > 1 {
			issue.OverLimitFields = appendUniqueStrings(issue.OverLimitFields, field)
		}
	}
	addOverLimit("skills", len(skillLoopStringListArgument(req.Arguments, "add_enabled_skill_ids")))
	addOverLimit("skills", len(skillLoopStringListArgument(req.Arguments, "enabled_skill_ids")))
	addOverLimit("knowledge bases", len(skillLoopStringListArgument(req.Arguments, "add_knowledge_dataset_ids")))
	addOverLimit("knowledge bases", len(skillLoopStringListArgument(req.Arguments, "knowledge_dataset_ids")))
	addOverLimit("database tables", skillLoopAgentDatabaseBindingTargetCount(req.Arguments["add_database_bindings"]))
	addOverLimit("database tables", skillLoopAgentDatabaseBindingTargetCount(req.Arguments["database_bindings"]))
	addOverLimit("workflows", skillLoopAgentBindingObjectCount(req.Arguments["add_workflow_bindings"]))
	addOverLimit("workflows", skillLoopAgentBindingObjectCount(req.Arguments["workflow_bindings"]))

	for _, skillID := range append(skillLoopStringListArgument(req.Arguments, "add_enabled_skill_ids"), skillLoopStringListArgument(req.Arguments, "enabled_skill_ids")...) {
		if skillID = strings.TrimSpace(skillID); skillID != "" && !skillLoopLooksLikeAgentSkillID(skillID) {
			issue.InvalidSkillIDs = appendUniqueStrings(issue.InvalidSkillIDs, skillID)
		}
	}

	return issue, len(issue.OverLimitFields) > 0 || len(issue.InvalidSkillIDs) > 0
}

func skillLoopPlanRequiresSingleAgentBindingCandidate(prepared *PreparedChat) bool {
	if prepared == nil || prepared.Message == nil {
		return false
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if len(plan) == 0 {
		return false
	}
	for _, step := range mapSliceFromAny(plan["steps"]) {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(step["skill_id"])), skills.SkillAgentManagement) ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(step["tool_name"])), "update_agent_config") {
			continue
		}
		if skillLoopCandidateSelectionPolicyIsAtMostOne(step) {
			return true
		}
		if skillLoopCandidateSelectionPolicyIsAtMostOne(mapFromOperationContext(step["arguments"])) {
			return true
		}
	}
	if structured := mapFromOperationContext(plan["structured_plan"]); len(structured) > 0 {
		for _, operation := range mapSliceFromAny(structured["operations"]) {
			if !strings.EqualFold(strings.TrimSpace(stringFromAny(operation["skill_id"])), skills.SkillAgentManagement) ||
				!strings.EqualFold(strings.TrimSpace(stringFromAny(operation["tool_name"])), "update_agent_config") {
				continue
			}
			if skillLoopCandidateSelectionPolicyIsAtMostOne(operation) {
				return true
			}
			if skillLoopCandidateSelectionPolicyIsAtMostOne(mapFromOperationContext(operation["arguments"])) {
				return true
			}
		}
	}
	return false
}

func skillLoopCandidateSelectionPolicyIsAtMostOne(values map[string]interface{}) bool {
	if len(values) == 0 {
		return false
	}
	policy := strings.ToLower(strings.TrimSpace(stringFromAny(values[operationPlanCandidateSelectionPolicyKey])))
	if policy == "" {
		return false
	}
	switch policy {
	case operationPlanCandidateSelectionAtMostOnePerField,
		"at_most_one",
		"one_per_binding_field":
		return true
	default:
		return false
	}
}

func skillLoopOverbroadAgentCandidateSelectionUpdateGuardResult(issue agentCandidateSelectionUpdateIssue) skillloop.FinalAnswerGuardResult {
	message := "update_agent_config selected invalid or too many Agent binding candidates"
	details := []string{}
	if len(issue.OverLimitFields) > 0 {
		details = append(details, "Fields over the requested one-candidate limit: "+strings.Join(issue.OverLimitFields, ", ")+".")
	}
	if len(issue.InvalidSkillIDs) > 0 {
		details = append(details, "Skill values that look like display names instead of IDs: "+strings.Join(issue.InvalidSkillIDs, ", ")+".")
	}
	return skillloop.FinalAnswerGuardResult{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Message:  message,
		Advisory: true,
		SystemMessage: strings.Join(append([]string{
			message + ".",
			"The user asked to choose at most one candidate for each requested resource type, so do not request governance approval for a broader update.",
			"Retry update_agent_config only if the mutation is still needed, with at most one add_enabled_skill_ids value, one add_knowledge_dataset_ids value, one database table binding, and one workflow binding.",
			"For skills, pass the candidate id from list_agent_skill_candidates, not the display name.",
			"For database tables, copy one binding_candidates[].binding object from list_agent_database_tables instead of recombining IDs manually.",
		}, details...), " "),
	}
}

func skillLoopStringListArgument(args map[string]interface{}, key string) []string {
	if args == nil {
		return nil
	}
	return skillLoopStringListFromAny(args[key])
}

func skillLoopStringListFromAny(value interface{}) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(item); text != "" {
				out = append(out, text)
			}
		}
		return out
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" && text != "<nil>" {
				out = append(out, text)
			}
		}
		return out
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil
		}
		var stringItems []string
		if err := json.Unmarshal([]byte(text), &stringItems); err == nil {
			return skillLoopStringListFromAny(stringItems)
		}
		var anyItems []interface{}
		if err := json.Unmarshal([]byte(text), &anyItems); err == nil {
			return skillLoopStringListFromAny(anyItems)
		}
		return []string{text}
	default:
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" || text == "<nil>" {
			return nil
		}
		return []string{text}
	}
}

func skillLoopAgentBindingObjectCount(value interface{}) int {
	items, ok := skillLoopObjectListFromAny(value)
	if ok {
		return len(items)
	}
	return len(skillLoopStringListFromAny(value))
}

func skillLoopAgentDatabaseBindingTargetCount(value interface{}) int {
	items, ok := skillLoopObjectListFromAny(value)
	if !ok {
		return len(skillLoopStringListFromAny(value))
	}
	total := 0
	for _, item := range items {
		targets := []string{}
		for _, key := range []string{
			"table_ids",
			"writable_table_ids",
			"database_table_ids",
			"database_table_keys",
			"resource_ids",
			"ids",
		} {
			targets = appendUniqueStrings(targets, skillLoopStringListFromAny(item[key])...)
		}
		for _, key := range []string{
			"table_id",
			"writable_table_id",
			"database_table_id",
			"resource_id",
			"id",
		} {
			targets = appendUniqueStrings(targets, skillLoopStringListFromAny(item[key])...)
		}
		if len(targets) == 0 {
			total++
			continue
		}
		total += len(targets)
	}
	return total
}

func skillLoopObjectListFromAny(value interface{}) ([]map[string]interface{}, bool) {
	switch typed := value.(type) {
	case nil:
		return nil, false
	case []map[string]interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, copyStringAnyMap(item))
		}
		return out, true
	case []interface{}:
		out := mapSliceFromAny(typed)
		if len(out) == 0 && len(typed) > 0 {
			return nil, false
		}
		return out, true
	case map[string]interface{}:
		return []map[string]interface{}{copyStringAnyMap(typed)}, true
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return []map[string]interface{}{}, true
		}
		var items []map[string]interface{}
		if err := json.Unmarshal([]byte(text), &items); err == nil {
			return items, true
		}
		var item map[string]interface{}
		if err := json.Unmarshal([]byte(text), &item); err == nil {
			return []map[string]interface{}{item}, true
		}
		return nil, false
	default:
		out := mapSliceFromAny(value)
		if len(out) == 0 {
			return nil, false
		}
		return out, true
	}
}

func skillLoopLooksLikeAgentSkillID(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && agentManagementSkillIDPattern.MatchString(value)
}

func skillLoopShouldBlockTerminalRecentAgentMutationContinuation(prepared *PreparedChat, skillID string, toolName string) bool {
	if prepared == nil || prepared.parts == nil ||
		!strings.EqualFold(strings.TrimSpace(skillID), skills.SkillAgentManagement) ||
		!skillLoopToolLooksAssetMutation(skillID, toolName) ||
		!isContinuationIntent(prepared.parts.Query) {
		return false
	}
	if agentManagementContinuationHasExplicitMutationIntent(prepared.parts) {
		return false
	}
	if recentOperationPlansContainIncompleteWork(prepared.parts.RecentOperationPlans) {
		return false
	}
	return recentOperationPlansContainTerminalAgentMutation(prepared.parts.RecentOperationPlans)
}

func agentManagementContinuationHasExplicitMutationIntent(parts *chatRequestParts) bool {
	if parts == nil {
		return false
	}
	if intent := parts.ModelTurnIntent; intent != nil {
		if modelTurnIntentAssetEffectIsMutation(intent.AssetEffect) {
			return true
		}
		if agentCapabilityGoalsRequireConfigMutation(agentManagementCapabilityGoalsFromModelIntent(intent)) {
			return true
		}
		for _, capability := range intent.RecommendedCapabilities {
			if strings.EqualFold(strings.TrimSpace(capability), "asset_mutation") {
				return true
			}
		}
		return false
	}
	return false
}

func modelTurnIntentAssetEffectIsDelete(effect string) bool {
	switch strings.ToLower(strings.TrimSpace(effect)) {
	case "delete", "remove", "destroy":
		return true
	default:
		return false
	}
}

func modelTurnIntentAssetEffectIsMutation(effect string) bool {
	switch strings.ToLower(strings.TrimSpace(effect)) {
	case "create", "update", "edit", "modify", "change", "delete", "remove", "bind", "unbind", "replace",
		"asset_mutation":
		return true
	default:
		return false
	}
}

func recentOperationPlansContainTerminalAgentMutation(plans []map[string]interface{}) bool {
	for _, plan := range plans {
		if !operationPlanIsTerminal(plan) {
			continue
		}
		if operationPlanContainsAgentMutationStep(plan) {
			return true
		}
	}
	return false
}

func operationPlanIsTerminal(plan map[string]interface{}) bool {
	switch strings.ToLower(strings.TrimSpace(stringFromAny(plan["status"]))) {
	case operationPlanStatusCompleted:
		return true
	default:
		return operationPlanIsTerminalFailure(plan)
	}
}

func operationPlanContainsAgentMutationStep(plan map[string]interface{}) bool {
	if len(plan) == 0 {
		return false
	}
	for _, step := range mapSliceFromAny(plan["steps"]) {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(step["skill_id"])), skills.SkillAgentManagement) {
			continue
		}
		if skillLoopToolLooksAssetMutation(skills.SkillAgentManagement, stringFromAny(step["tool_name"])) {
			return true
		}
	}
	return false
}

func skillLoopTerminalRecentAgentMutationContinuationGuardResult(skillID string, toolName string) skillloop.FinalAnswerGuardResult {
	message := "ambiguous continuation cannot repeat a completed, failed, or rejected Agent mutation"
	if strings.TrimSpace(toolName) != "" {
		message = strings.TrimSpace(toolName) + " is blocked because the recent Agent mutation already reached a terminal state"
	}
	return skillloop.FinalAnswerGuardResult{
		SkillID:  strings.TrimSpace(skillID),
		ToolName: strings.TrimSpace(toolName),
		Message:  message,
		SystemMessage: strings.Join([]string{
			message + ".",
			"The latest user message is only a weak continuation, while the recent Agent mutation plan is already completed, failed, or rejected.",
			"Do not resurrect old delete/create/update/bind operations from terminal history.",
			"Answer from the existing operation result, or ask the user for a new explicit Agent mutation instruction before calling another asset-changing tool.",
		}, " "),
	}
}

func skillLoopShouldBlockDuplicateMutationToolCall(prepared *PreparedChat, resolved *skills.ResolvedSkills, req skillloop.ToolCallGuardRequest) bool {
	if prepared == nil || prepared.Message == nil || prepared.parts == nil {
		return false
	}
	skillID := strings.TrimSpace(req.SkillID)
	toolName := strings.TrimSpace(req.ToolName)
	if skillID == "" || toolName == "" || !skillIDEnabled(prepared.parts.SkillIDs, skillID) {
		return false
	}
	if !skillLoopToolLooksAssetMutation(skillID, toolName) && !skillLoopToolHasGovernedMutationEffect(resolved, skillID, toolName) {
		return false
	}
	if skillLoopRepeatedAgentCreateTargetAlreadySucceeded(prepared, req) {
		return true
	}
	if skillLoopToolCallAlreadyAttemptedWithSameArguments(req) {
		return true
	}
	if skillLoopSingleTargetAgentMutationAlreadySucceeded(prepared, req) {
		return true
	}
	for _, call := range successfulMetadataToolCalls(prepared.Message.Metadata, skillID, toolName) {
		if skillLoopToolArgumentsEqual(call.Arguments, req.Arguments) {
			return true
		}
	}
	return false
}

func skillLoopRepeatedAgentCreateTargetAlreadySucceeded(prepared *PreparedChat, req skillloop.ToolCallGuardRequest) bool {
	if prepared == nil || prepared.Message == nil ||
		!strings.EqualFold(strings.TrimSpace(req.SkillID), skills.SkillAgentManagement) ||
		!strings.EqualFold(strings.TrimSpace(req.ToolName), "create_agent") {
		return false
	}
	calls := append(successfulMetadataToolCalls(prepared.Message.Metadata, skills.SkillAgentManagement, "create_agent"),
		matchingSkillToolCalls(req.SuccessfulToolCalls, skills.SkillAgentManagement, "create_agent")...)
	calls = append(calls, completedCreateAgentPlanStepCalls(prepared.Message.Metadata)...)
	if agentManagementCreateTargetAlreadySucceeded(calls, req.Arguments) {
		return true
	}
	if skillLoopCompletedCreateAgentPlanAlreadySatisfied(prepared, req) {
		return true
	}
	return agentCreateProgressTargetAlreadyCompleted(prepared.Message.Metadata, req.Arguments)
}

func completedCreateAgentPlanStepCalls(metadata map[string]interface{}) []skillloop.SkillToolCallRef {
	plan := mapFromOperationContext(metadataValue(metadata, "operation_plan"))
	if len(plan) == 0 {
		return nil
	}
	out := []skillloop.SkillToolCallRef{}
	stepStatus := mapFromOperationContext(plan["step_status"])
	for _, step := range mapSliceFromAny(plan["steps"]) {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(step["skill_id"])), skills.SkillAgentManagement) ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(step["tool_name"])), "create_agent") ||
			operationPlanStepResolvedStatus(step, stepStatus) != operationPlanStepStatusCompleted {
			continue
		}
		out = append(out, skillloop.SkillToolCallRef{
			SkillID:   skills.SkillAgentManagement,
			ToolName:  "create_agent",
			Arguments: mapFromOperationContext(step["arguments"]),
			Result:    mapFromOperationContext(step["result"]),
		})
	}
	return out
}

func skillLoopCompletedCreateAgentPlanAlreadySatisfied(prepared *PreparedChat, req skillloop.ToolCallGuardRequest) bool {
	if prepared == nil || prepared.Message == nil ||
		!strings.EqualFold(strings.TrimSpace(req.SkillID), skills.SkillAgentManagement) ||
		!strings.EqualFold(strings.TrimSpace(req.ToolName), "create_agent") {
		return false
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if len(plan) == 0 ||
		!operationPlanHasToolStepWithStatus(plan, skills.SkillAgentManagement, "create_agent", operationPlanStepStatusCompleted) {
		return false
	}
	return !skillLoopShouldAllowRepeatedPlannedMutation(prepared, req)
}

func agentCreateProgressTargetAlreadyCompleted(metadata map[string]interface{}, args map[string]interface{}) bool {
	name := strings.ToLower(strings.TrimSpace(firstNonEmptyString(args["name"], args["agent_name"])))
	if name == "" {
		return false
	}
	for _, completed := range agentCreateCompletedTargetNamesFromProgress(metadata) {
		if strings.EqualFold(strings.TrimSpace(completed), name) {
			return true
		}
	}
	return false
}

func agentCreateCompletedTargetNamesFromProgress(metadata map[string]interface{}) []string {
	progress := mapFromOperationContext(metadataValue(metadata, "agent_create_progress"))
	if len(progress) == 0 {
		ledger := mapFromOperationContext(metadataValue(metadata, "execution_ledger"))
		progress = mapFromOperationContext(ledger["agent_create_progress"])
		if len(progress) == 0 {
			summary := mapFromOperationContext(ledger["summary"])
			progress = mapFromOperationContext(summary["agent_create_progress"])
		}
	}
	return stringSliceFromAny(progress["completed_targets"])
}

func skillLoopSingleTargetAgentMutationAlreadySucceeded(prepared *PreparedChat, req skillloop.ToolCallGuardRequest) bool {
	if prepared == nil || prepared.Message == nil {
		return false
	}
	skillID := strings.TrimSpace(req.SkillID)
	toolName := strings.TrimSpace(req.ToolName)
	if !strings.EqualFold(skillID, skills.SkillAgentManagement) {
		return false
	}
	switch strings.ToLower(toolName) {
	case "update_agent_config", "update_agent_identity":
	default:
		return false
	}
	if skillLoopAgentIdentityUpdateAlreadyCoveredByCreate(prepared, req) {
		return true
	}
	return false
}

func skillLoopAgentIdentityUpdateAlreadyCoveredByCreate(prepared *PreparedChat, req skillloop.ToolCallGuardRequest) bool {
	if prepared == nil || prepared.Message == nil ||
		!strings.EqualFold(strings.TrimSpace(req.SkillID), skills.SkillAgentManagement) ||
		!strings.EqualFold(strings.TrimSpace(req.ToolName), "update_agent_identity") ||
		!agentIdentityUpdateHasRequestedFields(req.Arguments) {
		return false
	}
	createCalls := append(successfulMetadataToolCalls(prepared.Message.Metadata, skills.SkillAgentManagement, "create_agent"),
		matchingSkillToolCalls(req.SuccessfulToolCalls, skills.SkillAgentManagement, "create_agent")...)
	for _, call := range createCalls {
		if agentIdentityUpdateRepeatsCreateCall(call, req.Arguments) {
			return true
		}
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	for _, step := range mapSliceFromAny(plan["steps"]) {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(step["skill_id"])), skills.SkillAgentManagement) ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(step["tool_name"])), "create_agent") ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(step["status"])), operationPlanStepStatusCompleted) {
			continue
		}
		call := skillloop.SkillToolCallRef{
			SkillID:   skills.SkillAgentManagement,
			ToolName:  "create_agent",
			Arguments: mapFromOperationContext(step["arguments"]),
			Result:    mapFromOperationContext(step["result"]),
		}
		if agentIdentityUpdateRepeatsCreateCall(call, req.Arguments) {
			return true
		}
	}
	return false
}

func agentIdentityUpdateHasRequestedFields(arguments map[string]interface{}) bool {
	for _, key := range []string{"name", "description", "icon_type", "icon", "icon_background"} {
		if _, ok := arguments[key]; ok {
			return true
		}
	}
	return false
}

func agentConfigUpdateHasPatchFields(arguments map[string]interface{}) bool {
	for _, key := range []string{
		"model_provider",
		"model",
		"model_parameters",
		"system_prompt",
		"pre_prompt",
		"agent_memory_enabled",
		"file_upload_enabled",
		"home_title",
		"input_placeholder",
		"theme_color",
		"suggested_questions",
		"agent_memory_slots",
		"memory_slots",
		"replace_agent_memory_slots",
		"enabled_skill_ids",
		"add_enabled_skill_ids",
		"remove_enabled_skill_ids",
		"knowledge_dataset_ids",
		"add_knowledge_dataset_ids",
		"remove_knowledge_dataset_ids",
		"database_bindings",
		"add_database_bindings",
		"remove_database_bindings",
		"workflow_bindings",
		"add_workflow_bindings",
		"remove_workflow_bindings",
		"knowledge_retrieval_config",
	} {
		if _, ok := arguments[key]; ok {
			return true
		}
	}
	return false
}

func agentIdentityUpdateRepeatsCreateCall(call skillloop.SkillToolCallRef, updateArgs map[string]interface{}) bool {
	updateAgentID := strings.TrimSpace(firstNonEmptyString(updateArgs["agent_id"], updateArgs["id"], updateArgs["asset_id"]))
	createdAgentID, hasCreatedAgentID := agentCreateIdentityEvidence(call, "agent_id")
	if updateAgentID != "" {
		if !hasCreatedAgentID || updateAgentID != createdAgentID {
			return false
		}
	}
	matchedField := false
	for _, key := range []string{"name", "description", "icon_type", "icon", "icon_background"} {
		requested, ok := updateArgs[key]
		if !ok {
			continue
		}
		matchedField = true
		created, hasCreated := agentCreateIdentityEvidence(call, key)
		if !hasCreated || strings.TrimSpace(stringFromAny(requested)) != created {
			return false
		}
	}
	return matchedField
}

func agentCreateIdentityEvidence(call skillloop.SkillToolCallRef, key string) (string, bool) {
	nestedAgent := mapFromOperationContext(call.Result["agent"])
	switch key {
	case "agent_id":
		return firstAgentIdentityEvidenceValue(
			identityEvidenceCandidate{values: call.Result, key: "agent_id"},
			identityEvidenceCandidate{values: call.Result, key: "id"},
			identityEvidenceCandidate{values: nestedAgent, key: "agent_id"},
			identityEvidenceCandidate{values: nestedAgent, key: "id"},
			identityEvidenceCandidate{values: call.Arguments, key: "agent_id"},
			identityEvidenceCandidate{values: call.Arguments, key: "id"},
		)
	case "name":
		return firstAgentIdentityEvidenceValue(
			identityEvidenceCandidate{values: call.Result, key: "agent_name"},
			identityEvidenceCandidate{values: call.Result, key: "name"},
			identityEvidenceCandidate{values: nestedAgent, key: "name"},
			identityEvidenceCandidate{values: call.Arguments, key: "name"},
		)
	case "description":
		return firstAgentIdentityEvidenceValue(
			identityEvidenceCandidate{values: call.Result, key: "agent_description"},
			identityEvidenceCandidate{values: call.Result, key: "description"},
			identityEvidenceCandidate{values: nestedAgent, key: "description"},
			identityEvidenceCandidate{values: call.Arguments, key: "description"},
		)
	case "icon_type":
		return firstAgentIdentityEvidenceValue(
			identityEvidenceCandidate{values: call.Result, key: "agent_icon_type"},
			identityEvidenceCandidate{values: call.Result, key: "icon_type"},
			identityEvidenceCandidate{values: nestedAgent, key: "icon_type"},
			identityEvidenceCandidate{values: call.Arguments, key: "icon_type"},
		)
	case "icon":
		return firstAgentIdentityEvidenceValue(
			identityEvidenceCandidate{values: call.Result, key: "agent_icon"},
			identityEvidenceCandidate{values: call.Result, key: "icon"},
			identityEvidenceCandidate{values: nestedAgent, key: "icon"},
			identityEvidenceCandidate{values: call.Arguments, key: "icon"},
		)
	case "icon_background":
		return firstAgentIdentityEvidenceValue(
			identityEvidenceCandidate{values: call.Result, key: "agent_icon_background"},
			identityEvidenceCandidate{values: call.Result, key: "icon_background"},
			identityEvidenceCandidate{values: nestedAgent, key: "icon_background"},
			identityEvidenceCandidate{values: call.Arguments, key: "icon_background"},
		)
	default:
		return "", false
	}
}

type identityEvidenceCandidate struct {
	values map[string]interface{}
	key    string
}

func firstAgentIdentityEvidenceValue(candidates ...identityEvidenceCandidate) (string, bool) {
	for _, candidate := range candidates {
		if candidate.values == nil {
			continue
		}
		value, ok := candidate.values[candidate.key]
		if !ok {
			continue
		}
		return strings.TrimSpace(stringFromAny(value)), true
	}
	return "", false
}

func skillLoopShouldAllowUnplannedGovernedReadTool(prepared *PreparedChat, resolved *skills.ResolvedSkills, skillID string, toolName string) bool {
	if prepared == nil || prepared.Message == nil || prepared.parts == nil {
		return false
	}
	skillID = strings.TrimSpace(skillID)
	toolName = strings.TrimSpace(toolName)
	if skillID == "" || toolName == "" || !skillIDEnabled(prepared.parts.SkillIDs, skillID) {
		return false
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if len(plan) == 0 {
		return false
	}
	return skillLoopToolLooksReadOnlyWithResolved(resolved, skillID, toolName)
}

func skillLoopShouldAllowUnplannedArtifactGeneration(prepared *PreparedChat, req skillloop.ToolCallGuardRequest) bool {
	if prepared == nil || prepared.Message == nil || prepared.parts == nil {
		return false
	}
	skillID := strings.TrimSpace(req.SkillID)
	toolName := strings.TrimSpace(req.ToolName)
	if skillID == "" || toolName == "" || !skillIDEnabled(prepared.parts.SkillIDs, skillID) {
		return false
	}
	if !isKnownArtifactGeneratorToolCall(skillID, toolName) {
		return false
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if len(plan) == 0 || strings.EqualFold(strings.TrimSpace(stringFromAny(plan["status"])), operationPlanStatusCompleted) {
		return false
	}
	if operationPlanHasToolStepWithStatus(plan, skillID, toolName, operationPlanStepStatusCompleted) {
		return false
	}
	if skillLoopToolCallAlreadyAttemptedWithSameArguments(req) {
		return false
	}
	goal := operationPlanAmendmentGoal(prepared)
	if strings.TrimSpace(goal) == "" {
		return false
	}
	if isChartGeneratorToolCall(skillID, toolName) {
		if prepared.parts.ModelTurnIntent != nil {
			return shouldPreferChartArtifactProducer(prepared.parts)
		}
		return false
	}
	return isTemporaryFileGenerateIntent(goal) || isManagedFileCreateIntent(goal) || isContinuationIntent(goal)
}

func skillLoopShouldBlockDuplicateArtifactToolCall(req skillloop.ToolCallGuardRequest) bool {
	if !isKnownArtifactGeneratorToolCall(req.SkillID, req.ToolName) {
		return false
	}
	return skillLoopToolCallAlreadyAttemptedWithSameArguments(req)
}

func skillLoopDuplicateArtifactGuardResult(req skillloop.ToolCallGuardRequest) skillloop.FinalAnswerGuardResult {
	skillID := strings.TrimSpace(req.SkillID)
	toolName := strings.TrimSpace(req.ToolName)
	return skillloop.FinalAnswerGuardResult{
		SkillID:  skillID,
		ToolName: toolName,
		Message:  "the artifact-generating tool call was already attempted in this turn",
		SystemMessage: strings.Join([]string{
			"The artifact-generating tool call was already attempted with the same arguments in this turn.",
			"Do not generate another artifact for the same request.",
			"Use the existing generated artifact result as evidence, save it if the user asked for File Management, or answer truthfully from the ledger.",
		}, " "),
	}
}

func skillLoopDuplicateMutationGuardResult(skillID string, toolName string) skillloop.FinalAnswerGuardResult {
	return skillloop.FinalAnswerGuardResult{
		SkillID:  strings.TrimSpace(skillID),
		ToolName: strings.TrimSpace(toolName),
		Message:  "the asset-changing tool call was already attempted or completed in this turn",
		SystemMessage: strings.Join([]string{
			"The asset-changing tool call was already attempted or completed in this turn.",
			"Do not repeat the operation unless a distinct pending plan step still requires it.",
			"Use the existing tool result as evidence, adjust the arguments if a real retry is required, or provide a truthful answer from the ledger.",
		}, " "),
	}
}

func skillLoopDuplicateMutationGuardResultForRequest(prepared *PreparedChat, req skillloop.ToolCallGuardRequest) skillloop.FinalAnswerGuardResult {
	skillID := strings.TrimSpace(req.SkillID)
	toolName := strings.TrimSpace(req.ToolName)
	if skillLoopRepeatedAgentCreateTargetAlreadySucceeded(prepared, req) {
		return skillloop.FinalAnswerGuardResult{
			SkillID:  skillID,
			ToolName: toolName,
			Message:  "the requested Agent has already been created in this turn",
			SystemMessage: strings.Join([]string{
				"Do not call create_agent again for the same requested Agent.",
				"The existing create_agent result is the evidence for the created Agent.",
				"Continue with follow-up read, route, or config tools if the user requested later steps; otherwise answer from the existing tool result.",
			}, " "),
		}
	}
	if skillLoopSingleTargetAgentMutationAlreadySucceeded(prepared, req) {
		return skillloop.FinalAnswerGuardResult{
			SkillID:  skillID,
			ToolName: toolName,
			Message:  "the requested Agent identity update is already covered by create_agent",
			SystemMessage: strings.Join([]string{
				"The create_agent step already covers the requested Agent identity fields.",
				"Do not call update_agent_identity just to repeat name, description, icon, or icon background values already used during creation.",
				"Continue with distinct pending config, binding, route, or verification work if needed.",
			}, " "),
		}
	}
	return skillLoopDuplicateMutationGuardResult(skillID, toolName)
}

func skillLoopUnplannedToolGuardResult(skillID string, toolName string) skillloop.FinalAnswerGuardResult {
	message := "tool call is not covered by the current operation evidence"
	if skillLoopToolLooksAssetMutation(skillID, toolName) {
		message = "asset-changing tool call is not covered by the current operation evidence"
	}
	return skillloop.FinalAnswerGuardResult{
		SkillID:  strings.TrimSpace(skillID),
		ToolName: strings.TrimSpace(toolName),
		Message:  message,
		SystemMessage: strings.Join([]string{
			"The operation plan is advisory, but tools still need to fit the enabled skill set, runtime governance, duplicate protection, available execution evidence, and a clear current-goal match.",
			"This tool call is not a planned pending step, an allowed evidence/navigation/advisory deviation, or a governed mutation that can amend the active plan.",
			"Do not run it automatically.",
			"Use read/list/observe tools to collect evidence, ask the user if the goal changed, or continue from the current tool results.",
		}, " "),
	}
}

func skillLoopShouldBlockRepeatedLoadedNavigation(prepared *PreparedChat, skillID string, toolName string, args map[string]interface{}) bool {
	if prepared == nil || prepared.parts == nil || !isConsoleNavigatorNavigateTool(skillID, toolName) {
		return false
	}
	href := normalizeConsoleNavigationGuardHref(skillToolCallArgumentString(args, "href"))
	if href == "" || !strings.HasPrefix(href, "/console") {
		return false
	}
	return consoleNavigationRouteAlreadyAvailable(prepared.parts, href) ||
		clientActionContinuationLoadedRoute(prepared.parts, href) ||
		(prepared.Message != nil && clientActionMetadataHasActiveRoute(prepared.Message.Metadata, href)) ||
		(prepared.Message != nil && clientActionMetadataHasCompletedRoute(prepared.Message.Metadata, href))
}

func skillLoopRepeatedLoadedNavigationGuardResult(args map[string]interface{}) skillloop.FinalAnswerGuardResult {
	href := normalizeConsoleNavigationGuardHref(skillToolCallArgumentString(args, "href"))
	message := "The requested console route is already loaded or already pending in the current page context."
	if href != "" {
		message = "The requested console route " + href + " is already loaded or already pending in the current page context."
	}
	return skillloop.FinalAnswerGuardResult{
		SkillID:  skills.SkillConsoleNavigator,
		ToolName: "navigate",
		Message:  message,
		Advisory: true,
		SystemMessage: strings.Join([]string{
			message,
			"Do not call console-navigator/navigate again for the same route.",
			"Continue the user's task from the current page context and use available page evidence or the next relevant tool.",
		}, " "),
	}
}

func skillLoopShouldAllowGovernedMutationDeviation(prepared *PreparedChat, resolved *skills.ResolvedSkills, req skillloop.ToolCallGuardRequest) bool {
	if prepared == nil || prepared.Message == nil || prepared.parts == nil || resolved == nil {
		return false
	}
	skillID := strings.TrimSpace(req.SkillID)
	toolName := strings.TrimSpace(req.ToolName)
	if skillID == "" || toolName == "" || !skillIDEnabled(prepared.parts.SkillIDs, skillID) {
		return false
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if len(plan) == 0 || strings.EqualFold(strings.TrimSpace(stringFromAny(plan["status"])), operationPlanStatusCompleted) {
		return false
	}
	if !skillLoopToolHasGovernedMutationEffect(resolved, skillID, toolName) {
		return false
	}
	return !skillLoopToolCallAlreadyAttemptedWithSameArguments(req)
}

func skillLoopToolHasGovernedMutationEffect(resolved *skills.ResolvedSkills, skillID string, toolName string) bool {
	effect, ok := skillLoopToolGovernanceEffect(resolved, skillID, toolName)
	if !ok {
		return false
	}
	return skillLoopGovernanceEffectIsMutation(effect)
}

func skillLoopGovernanceEffectIsMutation(effect toolgovernance.Effect) bool {
	switch effect {
	case toolgovernance.EffectCreate,
		toolgovernance.EffectUpdate,
		toolgovernance.EffectDelete,
		toolgovernance.EffectPublish,
		toolgovernance.EffectInvoke,
		toolgovernance.EffectSchedule,
		toolgovernance.EffectExternalSend:
		return true
	default:
		return false
	}
}

func skillLoopToolGovernanceEffect(resolved *skills.ResolvedSkills, skillID string, toolName string) (toolgovernance.Effect, bool) {
	if resolved == nil {
		return "", false
	}
	doc, ok := resolved.Get(skillID)
	if !ok || doc == nil {
		return "", false
	}
	for _, tool := range doc.Tools {
		if !strings.EqualFold(strings.TrimSpace(tool.Name), strings.TrimSpace(toolName)) || tool.Governance == nil {
			continue
		}
		manifest := toolgovernance.NormalizeManifest(*tool.Governance)
		return manifest.Effect, true
	}
	return "", false
}

func skillLoopToolCallAlreadyAttemptedWithSameArguments(req skillloop.ToolCallGuardRequest) bool {
	for _, call := range append(append([]skillloop.SkillToolCallRef{}, req.SuccessfulToolCalls...), req.AttemptedToolCalls...) {
		if !strings.EqualFold(strings.TrimSpace(call.SkillID), strings.TrimSpace(req.SkillID)) ||
			!strings.EqualFold(strings.TrimSpace(call.ToolName), strings.TrimSpace(req.ToolName)) {
			continue
		}
		if skillLoopToolArgumentsEqual(call.Arguments, req.Arguments) {
			return true
		}
	}
	return false
}

func skillLoopToolArgumentsEqual(left map[string]interface{}, right map[string]interface{}) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	if leftErr != nil || rightErr != nil {
		return reflect.DeepEqual(left, right)
	}
	return string(leftJSON) == string(rightJSON)
}

func skillLoopShouldAllowRepeatedPlannedMutation(prepared *PreparedChat, req skillloop.ToolCallGuardRequest) bool {
	if prepared == nil || prepared.Message == nil || prepared.parts == nil {
		return false
	}
	skillID := strings.TrimSpace(req.SkillID)
	toolName := strings.TrimSpace(req.ToolName)
	if skillID == "" || toolName == "" || !skillIDEnabled(prepared.parts.SkillIDs, skillID) {
		return false
	}
	if !skillLoopToolLooksAssetMutation(skillID, toolName) {
		return false
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if len(plan) == 0 || !operationPlanHasToolStepWithStatus(plan, skillID, toolName, operationPlanStepStatusCompleted) {
		return false
	}
	if !strings.EqualFold(skillID, skills.SkillAgentManagement) {
		return false
	}
	if !strings.EqualFold(toolName, "create_agent") {
		return false
	}
	requestedTargets, requestedCount, ok := agentManagementCreateTargetsFromStructuredState(prepared.Message.Metadata, plan)
	if !ok {
		return false
	}
	if len(requestedTargets) > requestedCount {
		requestedCount = len(requestedTargets)
	}
	if requestedCount <= 1 {
		return false
	}
	if agentCreateProgressTargetAlreadyCompleted(prepared.Message.Metadata, req.Arguments) {
		return false
	}
	calls := append(successfulMetadataToolCalls(prepared.Message.Metadata, skillID, toolName), matchingSkillToolCalls(req.SuccessfulToolCalls, skillID, toolName)...)
	if agentManagementUniqueCreateTargetCount(calls) >= requestedCount {
		return false
	}
	if agentManagementCreateTargetAlreadySucceeded(calls, req.Arguments) {
		return false
	}
	return agentManagementCreateTargetAllowedByRequestedTargets(requestedTargets, req.Arguments)
}

func agentManagementCreateTargetsFromStructuredState(metadata map[string]interface{}, plan map[string]interface{}) ([]string, int, bool) {
	targets := []string{}
	addTarget := func(values ...interface{}) {
		for _, value := range values {
			name := strings.TrimSpace(stringFromAny(value))
			if name == "" || stringSliceContainsFold(targets, name) {
				continue
			}
			targets = append(targets, name)
		}
	}

	count := 0
	if progress := mapFromOperationContext(metadata["agent_create_progress"]); len(progress) > 0 {
		for _, target := range stringSliceFromAny(progress["requested_targets"]) {
			addTarget(target)
		}
		if len(targets) == 0 {
			for _, target := range stringSliceFromAny(progress["completed_targets"]) {
				addTarget(target)
			}
			for _, target := range stringSliceFromAny(progress["missing_targets"]) {
				addTarget(target)
			}
		}
		count = maxInt(count, positiveIntFromAny(progress["requested_count"]))
	}

	if len(plan) > 0 {
		for _, target := range stringSliceFromAny(plan["agent_create_targets"]) {
			addTarget(target)
		}
		count = maxInt(count, positiveIntFromAny(plan["agent_create_count"]))

		structuredPlan := mapFromOperationContext(plan["structured_plan"])
		for _, operation := range mapSliceFromAny(structuredPlan["operations"]) {
			if !strings.EqualFold(strings.TrimSpace(stringFromAny(operation["tool_name"])), "create_agent") &&
				!strings.EqualFold(strings.TrimSpace(stringFromAny(operation["action"])), "create") {
				continue
			}
			if resourceType := strings.TrimSpace(stringFromAny(operation["resource_type"])); resourceType != "" &&
				!strings.EqualFold(resourceType, "agent") {
				continue
			}
			args := mapFromOperationContext(operation["arguments"])
			target := mapFromOperationContext(operation["asset_target"])
			addTarget(operation["resource_name"], args["name"], args["agent_name"], target["name"], target["agent_name"])
		}
		for _, tool := range mapSliceFromAny(structuredPlan["required_tool_sequence"]) {
			if !strings.EqualFold(strings.TrimSpace(stringFromAny(tool["tool_name"])), "create_agent") {
				continue
			}
			args := mapFromOperationContext(tool["arguments"])
			addTarget(args["name"], args["agent_name"])
		}
		for _, step := range mapSliceFromAny(plan["steps"]) {
			if !strings.EqualFold(strings.TrimSpace(stringFromAny(step["skill_id"])), skills.SkillAgentManagement) ||
				!strings.EqualFold(strings.TrimSpace(stringFromAny(step["tool_name"])), "create_agent") {
				continue
			}
			args := mapFromOperationContext(step["arguments"])
			target := mapFromOperationContext(step["asset_target"])
			addTarget(step["resource_name"], args["name"], args["agent_name"], target["name"], target["agent_name"])
			count++
		}
	}

	if len(targets) > count {
		count = len(targets)
	}
	return targets, count, count > 0 || len(targets) > 0
}

func positiveIntFromAny(value interface{}) int {
	switch typed := value.(type) {
	case int:
		if typed > 0 {
			return typed
		}
	case int64:
		if typed > 0 {
			return int(typed)
		}
	case int32:
		if typed > 0 {
			return int(typed)
		}
	case float64:
		if typed > 0 {
			return int(typed)
		}
	case float32:
		if typed > 0 {
			return int(typed)
		}
	case json.Number:
		if parsed, err := strconv.Atoi(typed.String()); err == nil && parsed > 0 {
			return parsed
		}
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil && parsed > 0 {
			return parsed
		}
	}
	return 0
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func agentManagementUniqueCreateTargetCount(calls []skillloop.SkillToolCallRef) int {
	seen := map[string]struct{}{}
	unknown := 0
	for _, call := range calls {
		key := agentManagementCreateTargetKeyFromCall(call)
		if key == "" {
			unknown++
			continue
		}
		seen[key] = struct{}{}
	}
	return len(seen) + unknown
}

func agentManagementCreateTargetKeyFromCall(call skillloop.SkillToolCallRef) string {
	for _, value := range []interface{}{
		call.Arguments["name"],
		call.Arguments["agent_name"],
		call.Result["name"],
		call.Result["agent_name"],
		call.Result["agent_id"],
		call.Result["id"],
	} {
		if key := strings.ToLower(strings.TrimSpace(stringFromAny(value))); key != "" {
			return key
		}
	}
	if agent := governanceMapFromAny(call.Result["agent"]); len(agent) > 0 {
		for _, value := range []interface{}{agent["name"], agent["agent_name"], agent["agent_id"], agent["id"]} {
			if key := strings.ToLower(strings.TrimSpace(stringFromAny(value))); key != "" {
				return key
			}
		}
	}
	return ""
}

func agentManagementCreateTargetAlreadySucceeded(calls []skillloop.SkillToolCallRef, args map[string]interface{}) bool {
	name := strings.ToLower(strings.TrimSpace(firstNonEmptyString(args["name"], args["agent_name"])))
	if name == "" {
		return false
	}
	for _, call := range calls {
		callName := strings.ToLower(strings.TrimSpace(firstNonEmptyString(
			call.Arguments["name"],
			call.Arguments["agent_name"],
			call.Result["name"],
			call.Result["agent_name"],
		)))
		if callName == name {
			return true
		}
		if agent := governanceMapFromAny(call.Result["agent"]); len(agent) > 0 {
			callName = strings.ToLower(strings.TrimSpace(firstNonEmptyString(agent["name"], agent["agent_name"])))
			if callName == name {
				return true
			}
		}
	}
	return false
}

func agentManagementCreateTargetAllowedByRequestedTargets(requestedTargets []string, args map[string]interface{}) bool {
	if len(requestedTargets) == 0 {
		return true
	}
	name := strings.ToLower(strings.TrimSpace(firstNonEmptyString(args["name"], args["agent_name"])))
	if name == "" {
		return false
	}
	for _, target := range requestedTargets {
		if strings.ToLower(strings.TrimSpace(target)) == name {
			return true
		}
	}
	return false
}

func skillLoopShouldAllowReadyPlannedNavigationTool(prepared *PreparedChat, skillID string, toolName string, args map[string]interface{}) bool {
	if prepared == nil || prepared.Message == nil || prepared.parts == nil {
		return false
	}
	if !isConsoleNavigatorNavigateTool(skillID, toolName) || !skillIDEnabled(prepared.parts.SkillIDs, skillID) {
		return false
	}
	href := normalizeConsoleNavigationGuardHref(skillToolCallArgumentString(args, "href"))
	if href == "" || !strings.HasPrefix(href, "/console") {
		return false
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	return operationPlanHasReadyDependentRouteTarget(plan, href)
}

func skillLoopShouldAllowUnplannedNavigationTool(prepared *PreparedChat, skillID string, toolName string, args map[string]interface{}) bool {
	if prepared == nil || prepared.Message == nil || prepared.parts == nil {
		return false
	}
	if !isConsoleNavigatorNavigateTool(skillID, toolName) || !skillIDEnabled(prepared.parts.SkillIDs, skillID) {
		return false
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if len(plan) == 0 {
		return false
	}
	href := normalizeConsoleNavigationGuardHref(skillToolCallArgumentString(args, "href"))
	if href == "" || !strings.HasPrefix(href, "/console") {
		return false
	}
	if operationPlanHasPendingRouteTargetWaitingForPlanStep(plan, href) {
		return false
	}
	return !operationPlanHasCompletedRouteTarget(plan, href)
}

func operationPlanHasReadyDependentRouteTarget(plan map[string]interface{}, href string) bool {
	href = normalizeConsoleNavigationGuardHref(href)
	if len(plan) == 0 || href == "" {
		return false
	}
	stepStatus := mapFromOperationContext(plan["step_status"])
	for _, step := range mapSliceFromAny(plan["steps"]) {
		if !operationPlanStepIsRoute(step) {
			continue
		}
		target := operationPlanStepTargetPage(step)
		if target == "" || href != normalizeConsoleNavigationGuardHref(target) {
			continue
		}
		waitFor := strings.TrimSpace(stringFromAny(step["wait_for"]))
		if waitFor == "" || strings.EqualFold(waitFor, "continue") {
			continue
		}
		id := strings.TrimSpace(stringFromAny(step["id"]))
		status := operationPlanNormalizeStepStatus(firstNonEmptyString(step["status"], stepStatus[id]))
		if status == operationPlanStepStatusCompleted || status == operationPlanStepStatusFailed {
			continue
		}
		if operationPlanStepWaitForReadyFromPlan(step, stepStatus) {
			return true
		}
	}
	return false
}

func operationPlanHasPendingRouteTargetWaitingForPlanStep(plan map[string]interface{}, href string) bool {
	href = normalizeConsoleNavigationGuardHref(href)
	if len(plan) == 0 || href == "" {
		return false
	}
	stepStatus := mapFromOperationContext(plan["step_status"])
	for _, step := range mapSliceFromAny(plan["steps"]) {
		if !operationPlanStepIsRoute(step) {
			continue
		}
		target := operationPlanStepTargetPage(step)
		if target == "" || href != normalizeConsoleNavigationGuardHref(target) {
			continue
		}
		id := strings.TrimSpace(stringFromAny(step["id"]))
		status := operationPlanNormalizeStepStatus(firstNonEmptyString(step["status"], stepStatus[id]))
		if status == operationPlanStepStatusCompleted {
			continue
		}
		if !operationPlanStepWaitForReadyFromPlan(step, stepStatus) {
			return true
		}
	}
	return false
}

func operationPlanHasCompletedRouteTarget(plan map[string]interface{}, href string) bool {
	href = normalizeConsoleNavigationGuardHref(href)
	if len(plan) == 0 || href == "" {
		return false
	}
	stepStatus := mapFromOperationContext(plan["step_status"])
	for _, step := range mapSliceFromAny(plan["steps"]) {
		if !operationPlanStepIsRoute(step) {
			continue
		}
		target := operationPlanStepTargetPage(step)
		if target == "" || href != normalizeConsoleNavigationGuardHref(target) {
			continue
		}
		id := strings.TrimSpace(stringFromAny(step["id"]))
		status := operationPlanNormalizeStepStatus(firstNonEmptyString(step["status"], stepStatus[id]))
		if status == operationPlanStepStatusCompleted {
			return true
		}
	}
	return false
}

func skillLoopShouldAllowUnplannedObservationTool(prepared *PreparedChat, skillID string, toolName string) bool {
	if prepared == nil || prepared.Message == nil || prepared.parts == nil {
		return false
	}
	skillID = strings.TrimSpace(skillID)
	toolName = strings.TrimSpace(toolName)
	if skillID == "" || toolName == "" || !skillIDEnabled(prepared.parts.SkillIDs, skillID) {
		return false
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if len(plan) == 0 {
		return false
	}
	return skillLoopToolLooksReadOnly(skillID, toolName)
}

func skillLoopShouldAllowUnplannedAdvisoryTool(prepared *PreparedChat, skillID string, toolName string) bool {
	if prepared == nil || prepared.Message == nil || prepared.parts == nil {
		return false
	}
	skillID = strings.TrimSpace(skillID)
	toolName = strings.TrimSpace(toolName)
	if skillID == "" || toolName == "" || !skillIDEnabled(prepared.parts.SkillIDs, skillID) {
		return false
	}
	if isConsoleNavigatorNavigateTool(skillID, toolName) {
		return false
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if len(plan) == 0 {
		return false
	}
	if target := operationPlanToolStepAssetTarget(skillID, toolName); len(target) > 0 {
		effect := strings.ToLower(strings.TrimSpace(stringFromAny(target["effect"])))
		if effect != "" && effect != "read" {
			return false
		}
	}
	return !skillLoopToolLooksAssetMutation(skillID, toolName)
}

func skillLoopToolLooksAssetMutation(skillID string, toolName string) bool {
	if manifest, ok := skillLoopToolGovernanceManifest(skillID, toolName); ok {
		return skillLoopGovernanceEffectIsMutation(toolgovernance.NormalizeManifest(manifest).Effect)
	}
	if strings.EqualFold(strings.TrimSpace(skillID), skills.SkillAgentManagement) {
		switch strings.ToLower(strings.TrimSpace(toolName)) {
		case "create_agent",
			"update_agent_identity",
			"update_agent_config",
			"replace_agent_memory_slots",
			"delete_agent",
			"delete_agents":
			return true
		}
	}
	if strings.EqualFold(strings.TrimSpace(skillID), skills.SkillFileManager) {
		switch strings.ToLower(strings.TrimSpace(toolName)) {
		case "delete_file",
			"save_file_to_management":
			return true
		}
	}
	if strings.EqualFold(strings.TrimSpace(skillID), skills.SkillAgentWorkflow) {
		switch strings.ToLower(strings.TrimSpace(toolName)) {
		case "run_agent_workflow":
			return true
		}
	}
	return false
}

func skillLoopToolLooksReadOnly(skillID string, toolName string) bool {
	if manifest, ok := skillLoopToolGovernanceManifest(skillID, toolName); ok {
		return toolgovernance.NormalizeManifest(manifest).Effect == toolgovernance.EffectRead
	}
	if strings.EqualFold(strings.TrimSpace(skillID), skills.SkillInternalDatabase) {
		switch strings.ToLower(strings.TrimSpace(toolName)) {
		case "list_accessible_databases",
			"list_database_tables",
			"describe_database_table",
			"query_table_records":
			return true
		}
	}
	if strings.EqualFold(strings.TrimSpace(skillID), skills.SkillAgentDatabase) {
		switch strings.ToLower(strings.TrimSpace(toolName)) {
		case "query_table_records":
			return true
		}
	}
	if strings.EqualFold(strings.TrimSpace(skillID), skills.SkillAgentWorkflow) {
		switch strings.ToLower(strings.TrimSpace(toolName)) {
		case "list_agent_workflows",
			"get_workflow_run_status":
			return true
		}
	}
	if strings.EqualFold(strings.TrimSpace(skillID), skills.SkillAgentManagement) {
		switch strings.ToLower(strings.TrimSpace(toolName)) {
		case "list_agents",
			"get_agent",
			"get_agent_config",
			"list_agent_skill_candidates",
			"list_agent_knowledge_candidates",
			"list_agent_database_candidates",
			"list_agent_database_tables",
			"list_agent_workflow_binding_candidates",
			"list_available_models":
			return true
		}
	}
	return false
}

func skillLoopToolLooksReadOnlyWithResolved(resolved *skills.ResolvedSkills, skillID string, toolName string) bool {
	if effect, ok := skillLoopToolGovernanceEffect(resolved, skillID, toolName); ok {
		return effect == toolgovernance.EffectRead
	}
	return skillLoopToolLooksReadOnly(skillID, toolName)
}

func skillLoopToolGovernanceManifest(skillID string, toolName string) (toolgovernance.Manifest, bool) {
	if manifest, ok := skills.SystemSkillToolGovernanceManifest(skillID, toolName); ok {
		return manifest, true
	}
	if strings.TrimSpace(skillID) != "" {
		return toolgovernance.Manifest{}, false
	}
	return skills.SystemSkillToolGovernanceManifestByToolName(toolName)
}

func skillLoopShouldAllowUnplannedSkillLoad(prepared *PreparedChat, skillID string) bool {
	if prepared == nil || prepared.Message == nil || prepared.parts == nil {
		return false
	}
	skillID = strings.TrimSpace(skillID)
	if skillID == "" || !skillIDEnabled(prepared.parts.SkillIDs, skillID) {
		return false
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if len(plan) == 0 {
		return false
	}
	return true
}

func skillLoopShouldAllowUnplannedEvidenceTool(prepared *PreparedChat, skillID string, toolName string) bool {
	if prepared == nil || prepared.Message == nil || prepared.parts == nil {
		return false
	}
	skillID = strings.TrimSpace(skillID)
	toolName = strings.TrimSpace(toolName)
	if skillID == "" || toolName == "" || !skillIDEnabled(prepared.parts.SkillIDs, skillID) {
		return false
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if len(plan) == 0 {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(stringFromAny(plan["status"])), operationPlanStatusCompleted) {
		return false
	}
	if !operationPlanToolIsReplayableEvidence(skillID, toolName) {
		return false
	}
	goal := operationPlanAmendmentGoal(prepared)
	if strings.TrimSpace(goal) == "" {
		return false
	}
	if strings.EqualFold(skillID, skills.SkillAgentManagement) {
		return true
	}
	if strings.EqualFold(skillID, skills.SkillFileReader) {
		return strings.EqualFold(toolName, "list_visible_files") || strings.EqualFold(toolName, "read_file") && isFileReadIntent(goal)
	}
	return false
}

func skillLoopCanAmendOperationPlanForTool(prepared *PreparedChat, skillID string, toolName string) bool {
	if prepared == nil || prepared.Message == nil || prepared.parts == nil {
		return false
	}
	skillID = strings.TrimSpace(skillID)
	toolName = strings.TrimSpace(toolName)
	if skillID == "" || toolName == "" {
		return false
	}
	if !skillIDEnabled(prepared.parts.SkillIDs, skillID) {
		return false
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if len(plan) == 0 {
		return false
	}
	if operationPlanHasToolStepWithStatus(plan, skillID, toolName, operationPlanStepStatusCompleted) &&
		!operationPlanToolIsReplayableEvidence(skillID, toolName) {
		return false
	}
	goal := operationPlanAmendmentGoal(prepared)
	if goal == "" {
		return false
	}
	if strings.EqualFold(skillID, skills.SkillAgentManagement) {
		return true
	}
	if strings.EqualFold(skillID, skills.SkillConsoleNavigator) && strings.EqualFold(toolName, "navigate") {
		return isConsoleNavigationIntent(goal)
	}
	if strings.EqualFold(skillID, skills.SkillFileManager) {
		if strings.EqualFold(toolName, "delete_file") {
			return isFileDeleteIntent(goal)
		}
		if strings.EqualFold(toolName, "save_file_to_management") {
			return isManagedFileCreateIntent(goal) || isContinuationIntent(goal)
		}
	}
	if strings.EqualFold(skillID, skills.SkillFileReader) {
		return strings.EqualFold(toolName, "read_file") && isFileReadIntent(goal)
	}
	return false
}

func operationPlanAmendmentGoal(prepared *PreparedChat) string {
	if prepared == nil {
		return ""
	}
	values := []string{}
	if prepared.parts != nil {
		values = append(values, prepared.parts.Query)
	}
	if prepared.Message != nil {
		plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
		values = append(values, stringFromAny(plan["original_user_goal"]))
	}
	return strings.TrimSpace(strings.Join(values, "\n"))
}

func preparedAgentCapabilityGoals(prepared *PreparedChat) []AIChatAgentCapabilityGoal {
	if prepared == nil {
		return nil
	}
	if prepared.Message != nil {
		plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
		if goals := agentCapabilityGoalsFromOperationPlan(plan); len(goals) > 0 {
			return goals
		}
	}
	if prepared.parts != nil {
		return agentManagementCapabilityGoalsFromModelIntent(prepared.parts.ModelTurnIntent)
	}
	return nil
}

func operationPlanHasToolStepWithStatus(plan map[string]interface{}, skillID string, toolName string, status string) bool {
	if len(plan) == 0 {
		return false
	}
	stepStatus := mapFromOperationContext(plan["step_status"])
	for _, step := range mapSliceFromAny(plan["steps"]) {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(step["skill_id"])), skillID) ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(step["tool_name"])), toolName) {
			continue
		}
		stepState := operationPlanStepResolvedStatus(step, stepStatus)
		if stepState == status {
			return true
		}
	}
	return false
}

func skillLoopShouldRestrictToOperationPlan(prepared *PreparedChat) bool {
	if prepared == nil || prepared.Message == nil {
		return false
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if len(plan) == 0 {
		return false
	}
	if operationPlanModelDecidesTools(plan) {
		return false
	}
	for _, step := range mapSliceFromAny(plan["steps"]) {
		skillID := strings.TrimSpace(stringFromAny(step["skill_id"]))
		toolName := strings.TrimSpace(stringFromAny(step["tool_name"]))
		if skillID != "" && toolName != "" {
			return true
		}
	}
	return false
}

func skillLoopHasOperationPlan(prepared *PreparedChat) bool {
	if prepared == nil || prepared.Message == nil {
		return false
	}
	return len(mapFromOperationContext(prepared.Message.Metadata["operation_plan"])) > 0
}

func skillLoopAllowedPlannedTools(prepared *PreparedChat) map[string]struct{} {
	if prepared == nil || prepared.Message == nil {
		return nil
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if len(plan) == 0 {
		return nil
	}
	if operationPlanModelDecidesTools(plan) {
		return nil
	}
	allowed := map[string]struct{}{}
	stepStatus := mapFromOperationContext(plan["step_status"])
	for _, step := range operationPlanPendingExecutableStepsForToolExposure(plan, 8) {
		if !operationPlanStepWaitForReadyFromPlan(step, stepStatus) {
			continue
		}
		skillID := strings.TrimSpace(stringFromAny(step["skill_id"]))
		toolName := strings.TrimSpace(stringFromAny(step["tool_name"]))
		if skillID == "" || toolName == "" {
			continue
		}
		allowed[skillLoopToolAllowKey(skillID, toolName)] = struct{}{}
		if skills.RequiresPromptProfessionalizerPreflight(skillID, toolName) {
			allowed[skillLoopToolWildcardAllowKey(skills.SkillPromptProfessionalizer)] = struct{}{}
		}
	}
	addOperationPlanReplayableTools(plan, allowed)
	if len(allowed) == 0 {
		return nil
	}
	return allowed
}

func skillLoopExposedPlannedTools(prepared *PreparedChat) map[string]struct{} {
	if prepared == nil || prepared.Message == nil {
		return nil
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if len(plan) == 0 {
		return nil
	}
	if operationPlanModelDecidesTools(plan) {
		return nil
	}
	exposed := map[string]struct{}{}
	for _, step := range operationPlanPendingExecutableStepsForToolExposure(plan, 8) {
		skillID := strings.TrimSpace(stringFromAny(step["skill_id"]))
		toolName := strings.TrimSpace(stringFromAny(step["tool_name"]))
		if skillID == "" || toolName == "" {
			continue
		}
		exposed[skillLoopToolAllowKey(skillID, toolName)] = struct{}{}
		if skills.RequiresPromptProfessionalizerPreflight(skillID, toolName) {
			exposed[skillLoopToolWildcardAllowKey(skills.SkillPromptProfessionalizer)] = struct{}{}
		}
	}
	addOperationPlanReplayableTools(plan, exposed)
	if len(exposed) == 0 {
		return nil
	}
	return exposed
}

func operationPlanStepWaitForReadyFromPlan(step map[string]interface{}, stepStatus map[string]interface{}) bool {
	waitForIDs := operationPlanStepWaitForIDs(step)
	if len(waitForIDs) == 0 {
		return true
	}
	if len(stepStatus) == 0 {
		return false
	}
	for _, waitFor := range waitForIDs {
		if operationPlanNormalizeStepStatus(stringFromAny(stepStatus[waitFor])) != operationPlanStepStatusCompleted {
			return false
		}
	}
	return true
}

func addOperationPlanReplayableTools(plan map[string]interface{}, allowed map[string]struct{}) {
	if len(plan) == 0 || allowed == nil {
		return
	}
	stepStatus := mapFromOperationContext(plan["step_status"])
	for _, step := range mapSliceFromAny(plan["steps"]) {
		id := strings.TrimSpace(stringFromAny(step["id"]))
		if id == "" {
			continue
		}
		status := operationPlanNormalizeStepStatus(firstNonEmptyString(step["status"], stepStatus[id]))
		if status == operationPlanStepStatusFailed {
			continue
		}
		skillID := strings.TrimSpace(stringFromAny(step["skill_id"]))
		toolName := strings.TrimSpace(stringFromAny(step["tool_name"]))
		if skillID == "" || toolName == "" || !operationPlanToolIsReplayableEvidence(skillID, toolName) {
			continue
		}
		allowed[skillLoopToolAllowKey(skillID, toolName)] = struct{}{}
		if skills.RequiresPromptProfessionalizerPreflight(skillID, toolName) {
			allowed[skillLoopToolWildcardAllowKey(skills.SkillPromptProfessionalizer)] = struct{}{}
		}
	}
}

func operationPlanToolIsReplayableEvidence(skillID, toolName string) bool {
	skillID = strings.TrimSpace(skillID)
	toolName = strings.TrimSpace(toolName)
	if skillID == "" || toolName == "" {
		return false
	}
	if strings.EqualFold(skillID, skills.SkillAgentManagement) {
		switch toolName {
		case "list_agents",
			"get_agent",
			"get_agent_config",
			"list_agent_skill_candidates",
			"list_agent_knowledge_candidates",
			"list_agent_database_candidates",
			"list_agent_database_tables",
			"list_agent_workflow_binding_candidates",
			"list_available_models":
			return true
		default:
			return false
		}
	}
	if strings.EqualFold(skillID, skills.SkillFileReader) {
		return strings.EqualFold(toolName, "read_file") || strings.EqualFold(toolName, "list_visible_files")
	}
	return false
}

func skillLoopAllowedToolsContainSkill(allowed map[string]struct{}, skillID string) bool {
	skillID = strings.ToLower(strings.TrimSpace(skillID))
	if skillID == "" {
		return false
	}
	prefix := skillID + "/"
	for key := range allowed {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func skillLoopToolAllowed(allowed map[string]struct{}, skillID string, toolName string) bool {
	if _, ok := allowed[skillLoopToolAllowKey(skillID, toolName)]; ok {
		return true
	}
	_, ok := allowed[skillLoopToolWildcardAllowKey(skillID)]
	return ok
}

func skillLoopToolAllowKey(skillID string, toolName string) string {
	return strings.ToLower(strings.TrimSpace(skillID)) + "/" + strings.ToLower(strings.TrimSpace(toolName))
}

func skillLoopToolWildcardAllowKey(skillID string) string {
	return strings.ToLower(strings.TrimSpace(skillID)) + "/*"
}

func combineFinalAnswerGuards(guards ...skillloop.FinalAnswerGuard) skillloop.FinalAnswerGuard {
	active := make([]skillloop.FinalAnswerGuard, 0, len(guards))
	for _, guard := range guards {
		if guard != nil {
			active = append(active, guard)
		}
	}
	if len(active) == 0 {
		return nil
	}
	return func(req skillloop.FinalAnswerGuardRequest) (skillloop.FinalAnswerGuardResult, bool) {
		for _, guard := range active {
			if result, blocked := guard(req); blocked {
				return result, true
			}
		}
		return skillloop.FinalAnswerGuardResult{}, false
	}
}

func (s *service) skillExecutionContext(prepared *PreparedChat) skills.ExecutionContext {
	appID := prepared.Conversation.ID.String()
	if strings.TrimSpace(prepared.RunConfig.BillingAppID) != "" {
		appID = strings.TrimSpace(prepared.RunConfig.BillingAppID)
	}
	invokeFrom := tools.ToolInvokeFromAIChat
	if normalizeCallerType(prepared.Caller.Type) == runtimemodel.ConversationCallerAgent {
		invokeFrom = tools.ToolInvokeFromAgent
	}
	return skills.ExecutionContext{
		OrganizationID:    prepared.Scope.OrganizationID.String(),
		UserID:            prepared.Scope.AccountID.String(),
		ConversationID:    prepared.Conversation.ID.String(),
		AppID:             appID,
		MessageID:         prepared.Message.ID.String(),
		InvokeFrom:        invokeFrom,
		RuntimeParameters: skillRuntimeParametersForPrepared(prepared),
	}
}

func skillRuntimeParameters(scope Scope, config RunConfig) map[string]interface{} {
	return runtimeCapabilityConfigFromRunConfig(config).RuntimeParameters(scope, config.BillingAppType)
}

func skillRuntimeParametersForPrepared(prepared *PreparedChat) map[string]interface{} {
	params := skillRuntimeParameters(prepared.Scope, prepared.RunConfig)
	if workspaceID := preparedSkillWorkspaceID(prepared); workspaceID != "" {
		params["workspace_id"] = workspaceID
	}
	params = applySkillToolGovernanceRuntimeParameters(params, prepared)
	if recentAgents := consoleAgentsRecentMutationPayloads(prepared); len(recentAgents) > 0 {
		params["console_agents_recent_agent_updates"] = recentAgents
	}
	if prepared != nil && prepared.parts != nil && isConsoleFilesContext(prepared.parts.RuntimeContext, prepared.parts.RawOperationContext, prepared.parts.OperationContext) {
		params["console_files_page"] = true
		if visibleFiles := consoleFilesPromptVisibleFiles(prepared.parts); len(visibleFiles) > 0 {
			params["console_files_visible_files"] = visibleFiles
		}
	}
	if prepared != nil && prepared.parts != nil && isConsoleAgentsContext(prepared.parts.RuntimeContext, prepared.parts.RawOperationContext, prepared.parts.OperationContext) {
		params["console_agents_page"] = true
		if route := consoleRouteFromRuntimeContext(prepared.parts.RuntimeContext); route != "" {
			params["console_current_route"] = route
			params["console_agents_current_route"] = route
		}
		if visibleAgents := consoleAgentsPromptVisibleAgents(prepared.parts); len(visibleAgents) > 0 {
			params["console_agents_visible_agents"] = visibleAgents
		}
	}
	if history := workflowConversationHistoryFromPrepared(prepared); len(history) > 0 {
		params["workflow_context"] = map[string]interface{}{
			"conversation_history": history,
		}
	}
	return params
}

func consoleAgentsRecentMutationPayloads(prepared *PreparedChat) []map[string]interface{} {
	if prepared == nil || prepared.Message == nil {
		return nil
	}
	byID := map[string]map[string]interface{}{}
	order := []string{}
	addPayload := func(payload map[string]interface{}) {
		agentID := strings.TrimSpace(firstNonEmptyString(payload["agent_id"], payload["id"]))
		if agentID == "" {
			return
		}
		if _, exists := byID[agentID]; !exists {
			order = append(order, agentID)
		}
		byID[agentID] = mergeConsoleAgentPayload(byID[agentID], payload)
	}
	if payload := consoleAgentPayloadFromOperationToolResult(mapFromOperationContext(prepared.Message.Metadata["operation_plan"])); len(payload) > 0 {
		addPayload(payload)
	}
	if payload := consoleAgentPayloadFromOperationToolResult(mapFromOperationContext(prepared.Message.Metadata["operation_result_summary"])); len(payload) > 0 {
		addPayload(payload)
	}
	invocations := skillInvocationsFromMetadata(prepared.Message.Metadata["skill_invocations"])
	for _, invocation := range invocations {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["skill_id"])), skills.SkillAgentManagement) ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["status"])), "success") {
			continue
		}
		switch strings.TrimSpace(stringFromAny(invocation["tool_name"])) {
		case "create_agent", "update_agent_identity", "update_agent_config":
		default:
			continue
		}
		payload := consoleAgentPayloadFromInvocationResult(mapFromOperationContext(invocation["result"]))
		agentID := strings.TrimSpace(firstNonEmptyString(payload["agent_id"], payload["id"]))
		if agentID == "" {
			continue
		}
		addPayload(payload)
	}
	if len(order) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(order))
	for _, agentID := range order {
		if payload := byID[agentID]; len(payload) > 0 {
			out = append(out, payload)
		}
	}
	return out
}

func consoleAgentPayloadFromOperationToolResult(plan map[string]interface{}) map[string]interface{} {
	if len(plan) == 0 {
		return nil
	}
	candidates := []map[string]interface{}{}
	for _, key := range []string{"tool_result", "latest_tool_result", "result_summary"} {
		if candidate := mapFromOperationContext(plan[key]); len(candidate) > 0 {
			candidates = append(candidates, candidate)
			if summary := mapFromOperationContext(candidate["result_summary"]); len(summary) > 0 {
				candidates = append(candidates, summary)
			}
		}
	}
	for _, candidate := range candidates {
		if len(candidate) == 0 {
			continue
		}
		if skillID := strings.TrimSpace(stringFromAny(candidate["skill_id"])); skillID != "" &&
			!strings.EqualFold(skillID, skills.SkillAgentManagement) {
			continue
		}
		switch strings.TrimSpace(stringFromAny(candidate["tool_name"])) {
		case "", "create_agent", "update_agent_identity", "update_agent_config":
		default:
			continue
		}
		if payload := consoleAgentPayloadFromInvocationResult(candidate); len(payload) > 0 {
			return payload
		}
	}
	return nil
}

func consoleAgentPayloadFromInvocationResult(result map[string]interface{}) map[string]interface{} {
	if len(result) == 0 {
		return nil
	}
	payload := map[string]interface{}{}
	agent := mapFromOperationContext(result["agent"])
	copyConsoleAgentPayloadFields(payload, agent)
	copyConsoleAgentPayloadFields(payload, result)
	if id := strings.TrimSpace(firstNonEmptyString(payload["agent_id"], payload["id"], result["agent_id"], result["id"])); id != "" {
		payload["agent_id"] = id
		payload["id"] = id
		if _, ok := payload["href"]; !ok {
			payload["href"] = consoleAgentDetailHref(id)
		}
	}
	if name := strings.TrimSpace(firstNonEmptyString(payload["name"], payload["agent_name"], result["agent_name"])); name != "" {
		payload["name"] = name
		payload["agent_name"] = name
	}
	if workspaceID := strings.TrimSpace(firstNonEmptyString(payload["workspace_id"], result["workspace_id"])); workspaceID != "" {
		payload["workspace_id"] = workspaceID
	}
	return payload
}

func mergeConsoleAgentPayload(existing map[string]interface{}, latest map[string]interface{}) map[string]interface{} {
	if len(existing) == 0 {
		return copyStringAnyMap(latest)
	}
	out := copyStringAnyMap(existing)
	copyConsoleAgentPayloadFields(out, latest)
	return out
}

func copyConsoleAgentPayloadFields(target map[string]interface{}, source map[string]interface{}) {
	if target == nil || len(source) == 0 {
		return
	}
	for _, key := range []string{
		"id",
		"agent_id",
		"name",
		"agent_name",
		"description",
		"workspace_id",
		"icon_url",
		"icon_type",
		"icon",
		"href",
	} {
		if value, ok := source[key]; ok && value != nil && strings.TrimSpace(fmt.Sprint(value)) != "" {
			target[key] = value
		}
	}
}

func preparedSkillWorkspaceID(prepared *PreparedChat) string {
	if prepared == nil {
		return ""
	}
	if prepared.Scope.WorkspaceID != nil && *prepared.Scope.WorkspaceID != uuid.Nil {
		return prepared.Scope.WorkspaceID.String()
	}
	if prepared.Conversation != nil && prepared.Conversation.WorkspaceID != nil && *prepared.Conversation.WorkspaceID != uuid.Nil {
		return prepared.Conversation.WorkspaceID.String()
	}
	return ""
}

func skillLoopAdditionalSystemMessages(prepared *PreparedChat) []adapter.Message {
	return skillLoopAdditionalSystemMessagesForResolved(prepared, nil)
}

func skillLoopAdditionalSystemMessagesForResolved(prepared *PreparedChat, resolved *skills.ResolvedSkills) []adapter.Message {
	if prepared == nil {
		return nil
	}
	messages := make([]adapter.Message, 0, 5)
	if message, ok := agentWorkflowAvailableBindingsMessage(prepared.RunConfig.WorkflowBindings); ok {
		messages = append(messages, message)
	}
	if message, ok := contextualAIChatTurnStrategyMessage(prepared); ok {
		messages = append(messages, message)
	}
	if message, ok := contextualConsoleNavigationSkillMessageForResolved(prepared, resolved); ok {
		messages = append(messages, message)
	}
	if message, ok := contextualConsoleAgentsSkillMessageForResolved(prepared, resolved); ok {
		messages = append(messages, message)
	}
	if message, ok := contextualConsoleFilesSkillMessage(prepared); ok {
		messages = append(messages, message)
	}
	return messages
}

func contextualAIChatTurnStrategyMessage(prepared *PreparedChat) (adapter.Message, bool) {
	strategy := contextualAIChatTurnStrategy(prepared)
	if strategy == nil {
		return adapter.Message{}, false
	}
	promptStrategy := aiChatTurnStrategyPromptView(strategy)
	encoded, err := json.Marshal(promptStrategy)
	if err != nil {
		return adapter.Message{}, false
	}
	content := strings.Join([]string{
		"ZGI AIChat turn strategy guidance:",
		"This is a soft execution strategy for the current user turn, not a fixed action runtime plan.",
		"Use it to understand phases, target assets, success criteria, and verification points. Choose concrete tools from the currently enabled tool schemas and latest evidence.",
		"When structured_plan is present, treat structured_plan.operations as a phase/status checklist, not a required tool script.",
		"Treat structured_plan as advisory: if tool results, page evidence, or client action evidence contradict it, follow the evidence, continue from the actual state, and explain blockers truthfully.",
		"For multi-action user requests, keep a private progress checklist from the full user goal. Do not stop after the first successful mutation if later requested outcomes remain unfinished.",
		"If the initial strategy omits a clearly requested later step, choose the next relevant enabled skill/tool, let the operation plan be amended by evidence, and continue from the newest tool result.",
		"Do not claim a structured operation is complete until a matching tool result or page/client evidence supports it. If a candidate or target resource is missing, stop and report the missing evidence instead of calling a governed mutation.",
		"Do not expose this strategy JSON, internal IDs, or raw fields to the user.",
		"Turn strategy JSON: " + string(encoded),
	}, "\n")
	return adapter.Message{Role: "system", Content: content}, true
}

func aiChatTurnStrategyPromptView(strategy *AIChatTurnStrategy) map[string]interface{} {
	if strategy == nil {
		return nil
	}
	encoded, err := json.Marshal(strategy)
	if err != nil {
		return nil
	}
	var view map[string]interface{}
	if err := json.Unmarshal(encoded, &view); err != nil {
		return nil
	}
	delete(view, "required_next_tool")
	delete(view, "planned_tools")
	if goals := mapSliceFromAny(view["capability_goals"]); len(goals) > 0 {
		view["capability_goals"] = aiChatTurnStrategyPromptCapabilityGoals(goals)
	}
	if structured := mapFromOperationContext(view["structured_plan"]); len(structured) > 0 {
		view["structured_plan"] = aiChatTurnStrategyPromptStructuredPlan(structured)
	}
	view["planning_contract"] = map[string]interface{}{
		"planner_role":     "phase_and_success_criteria_only",
		"tool_choice":      "model_decides_from_enabled_tools_and_latest_evidence",
		"verification":     "compare tool/page evidence to success criteria before final answer",
		"retry_policy":     "retry only when the latest evidence explains a recoverable issue; otherwise report the blocker",
		"completion_basis": "final answers must be grounded in successful tool results or page/client observations",
	}
	return view
}

func aiChatTurnStrategyPromptStructuredPlan(structured map[string]interface{}) map[string]interface{} {
	if len(structured) == 0 {
		return structured
	}
	delete(structured, "required_tool_sequence")
	if operations := mapSliceFromAny(structured["operations"]); len(operations) > 0 {
		for _, operation := range operations {
			delete(operation, "skill_id")
			delete(operation, "tool_name")
		}
		structured["operations"] = mapsToInterfaceSlice(operations)
	}
	if goals := mapSliceFromAny(structured["capability_goals"]); len(goals) > 0 {
		structured["capability_goals"] = aiChatTurnStrategyPromptCapabilityGoals(goals)
	}
	return structured
}

func aiChatTurnStrategyPromptCapabilityGoals(goals []map[string]interface{}) []interface{} {
	for _, goal := range goals {
		delete(goal, "candidate_tool")
	}
	return mapsToInterfaceSlice(goals)
}

const aiChatTurnToolChoiceModelDecides = "model_decides"

func aiChatTurnStrategyModelDecidesTools(strategy *AIChatTurnStrategy) bool {
	if strategy == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(strategy.ToolChoiceMode), aiChatTurnToolChoiceModelDecides)
}

func operationPlanModelDecidesTools(plan map[string]interface{}) bool {
	if len(plan) == 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(stringFromAny(plan["tool_choice_mode"])), aiChatTurnToolChoiceModelDecides)
}

func skillLoopModelDecidesToolChoice(prepared *PreparedChat) bool {
	if prepared == nil {
		return false
	}
	if prepared.Message != nil {
		plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
		if len(plan) > 0 {
			return operationPlanModelDecidesTools(plan)
		}
	}
	return aiChatTurnStrategyModelDecidesTools(contextualAIChatTurnStrategy(prepared))
}

func skillLoopShouldUsePlainStreamForPassiveAnswer(prepared *PreparedChat) bool {
	if prepared == nil || prepared.parts == nil || !prepared.skillsEnabled() {
		return false
	}
	if prepared.parts.Attachments != nil && len(prepared.parts.Attachments.Files) > 0 {
		return false
	}
	strategy := contextualAIChatTurnStrategy(prepared)
	if strategy == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(strategy.Intent), "answer_or_explain_zgi_context") {
		return false
	}
	if strategy.RouteRequired || strategy.RequiredNextTool != nil || len(strategy.PlannedTools) > 0 || len(strategy.RemainingRouteSequence) > 0 {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(strategy.Approval), "none") {
		return false
	}
	if strategy.NeedsExactAgentRuntime {
		return false
	}
	effect := strings.ToLower(strings.TrimSpace(strategy.AssetEffect))
	if effect != "" && effect != "none" {
		return false
	}
	if len(strategy.PrimarySkills) > 0 {
		return false
	}
	return true
}

// AIChatTurnStrategy is the typed, internal plan hint for one contextual sidebar turn.
// It is guidance for the skill loop, not an executable action plan.
type AIChatTurnStrategy struct {
	Surface                  string                      `json:"surface"`
	CurrentPage              string                      `json:"current_page,omitempty"`
	Source                   string                      `json:"source,omitempty"`
	SourceReason             string                      `json:"source_reason,omitempty"`
	Intent                   string                      `json:"intent"`
	TaskType                 string                      `json:"task_type,omitempty"`
	TargetPage               string                      `json:"target_page,omitempty"`
	RouteRequired            bool                        `json:"route_required"`
	PrimarySkills            []string                    `json:"primary_skills"`
	SupportingSkills         []string                    `json:"supporting_skills"`
	PhaseGoals               []string                    `json:"phase_goals,omitempty"`
	EvidenceRequired         []string                    `json:"evidence_required,omitempty"`
	RecommendedCapabilities  []string                    `json:"recommended_capabilities,omitempty"`
	NeedsExactAgentRuntime   bool                        `json:"needs_exact_agent_runtime,omitempty"`
	CurrentContextMaySummary bool                        `json:"current_context_may_be_summary,omitempty"`
	OpenCreatedAgentDetail   bool                        `json:"open_created_agent_detail,omitempty"`
	AssetEffect              string                      `json:"asset_effect"`
	AssetRisk                string                      `json:"asset_risk"`
	Approval                 string                      `json:"approval"`
	SuccessCriteria          []string                    `json:"success_criteria"`
	ObservationPoints        []string                    `json:"observation_points"`
	ArtifactSource           string                      `json:"artifact_source,omitempty"`
	ToolChoiceMode           string                      `json:"tool_choice_mode,omitempty"`
	ExecutionScope           string                      `json:"execution_scope,omitempty"`
	WaitForContinue          bool                        `json:"wait_for_continue,omitempty"`
	Avoid                    []string                    `json:"avoid,omitempty"`
	CapabilityGoals          []AIChatAgentCapabilityGoal `json:"capability_goals,omitempty"`

	RequiredNextTool       *AIChatTurnStrategyTool       `json:"required_next_tool,omitempty"`
	RemainingRouteSequence []AIChatTurnStrategyRouteStep `json:"remaining_route_sequence,omitempty"`
	PlannedTools           []AIChatTurnStrategyTool      `json:"planned_tools,omitempty"`
	StructuredPlan         *AIChatStructuredPlan         `json:"structured_plan,omitempty"`
}

const (
	aiChatTurnStrategySourceDefault      = "default_contextual"
	aiChatTurnStrategySourceModelIntent  = "model_intent"
	aiChatTurnStrategySourceTurnProtocol = "turn_protocol"
)

type AIChatTurnStrategyTool struct {
	SkillID       string            `json:"skill_id"`
	ToolName      string            `json:"tool_name"`
	Arguments     map[string]string `json:"arguments"`
	StepID        string            `json:"step_id,omitempty"`
	WaitForStepID string            `json:"wait_for_step_id,omitempty"`
	OutputAlias   string            `json:"output_alias,omitempty"`
	ArgsBinding   map[string]string `json:"args_binding,omitempty"`
}

type AIChatTurnStrategyRouteStep struct {
	Href   string `json:"href"`
	Label  string `json:"label,omitempty"`
	Status string `json:"status"`
}

func contextualAIChatTurnStrategy(prepared *PreparedChat) *AIChatTurnStrategy {
	if prepared == nil {
		return nil
	}
	return contextualAIChatTurnStrategyFromParts(prepared.parts)
}

func contextualAIChatTurnStrategyFromParts(parts *chatRequestParts) *AIChatTurnStrategy {
	if parts == nil || !isContextualAIChatSurface(parts.Surface) || !chatPartsSkillsEnabled(parts) {
		return nil
	}
	strategyParts, stagedCurrent, stagedResume := stagedExecutionScopedParts(parts)
	if strategyParts != nil {
		parts = strategyParts
	}
	currentPage := contextualTurnCurrentPage(parts)
	strategy := &AIChatTurnStrategy{
		Surface:           normalizeAIChatSurface(parts.Surface),
		CurrentPage:       currentPage,
		Source:            aiChatTurnStrategySourceDefault,
		SourceReason:      "base_contextual_sidebar_strategy",
		Intent:            "answer_or_explain_zgi_context",
		TargetPage:        currentPage,
		RouteRequired:     false,
		PrimarySkills:     []string{},
		SupportingSkills:  []string{},
		AssetEffect:       "none",
		AssetRisk:         "low",
		Approval:          "none",
		SuccessCriteria:   []string{"answer from the current ZGI page context and enabled skills"},
		ObservationPoints: []string{"current_page_context"},
		ToolChoiceMode:    aiChatTurnToolChoiceModelDecides,
	}
	if stagedCurrent {
		strategy.ExecutionScope = "current_turn_before_continue"
		strategy.WaitForContinue = true
		strategy.SuccessCriteria = appendUniqueStrings(strategy.SuccessCriteria,
			"complete only the instructions before the user's continue marker in this turn",
			"wait for the user's continue message before executing deferred instructions",
		)
		strategy.Avoid = appendUniqueStrings(strategy.Avoid,
			"do not execute instructions after the user's continue marker in this turn",
		)
	} else if stagedResume {
		strategy.ExecutionScope = "staged_goal_after_continue"
		strategy.SuccessCriteria = appendUniqueStrings(strategy.SuccessCriteria,
			"execute the deferred instructions from the prior staged goal",
		)
	}

	if target, ok := resolveConsoleNavigationTargetForParts(parts); ok {
		strategy.TargetPage = target.Href
		routeRequired := !clientActionContinuationLoadedRoute(parts, target.Href)
		if consoleNavigationRouteAlreadyAvailable(parts, target.Href) {
			routeRequired = false
		}
		strategy.RouteRequired = routeRequired
		if routeRequired && skillIDEnabled(parts.SkillIDs, skills.SkillConsoleNavigator) {
			strategy.PrimarySkills = appendUniqueStrings(strategy.PrimarySkills, skills.SkillConsoleNavigator)
		}
	}

	if modelIntent := parts.ModelTurnIntent; modelIntent != nil {
		if modeled, ok := contextualAIChatTurnStrategyFromModelIntent(parts, strategy, modelIntent); ok {
			return finalizeAIChatTurnStrategy(parts, modeled)
		}
		strategy = markAIChatTurnStrategySource(strategy, aiChatTurnStrategySourceModelIntent, "model_intent_not_accepted_"+strings.TrimSpace(modelIntent.Intent))
		strategy.ToolChoiceMode = aiChatTurnToolChoiceModelDecides
		if skillIDEnabled(parts.SkillIDs, skills.SkillConsoleNavigator) {
			strategy.SupportingSkills = appendUniqueStrings(strategy.SupportingSkills, skills.SkillConsoleNavigator)
		}
		return finalizeAIChatTurnStrategy(parts, strategy)
	}

	// When model intent is unavailable, keep execution model-led instead of
	// reclassifying the user's business intent through substring rules. The
	// only retained fallback here is the explicit continue/resume control
	// command, which is part of the turn protocol rather than an asset-domain
	// decision.
	switch {
	case isContinuationIntent(parts.Query):
		strategy = markAIChatTurnStrategySource(strategy, aiChatTurnStrategySourceTurnProtocol, "continuation_query_rule")
		strategy = contextualContinuationStrategy(parts, strategy)
	default:
		strategy = markAIChatTurnStrategySource(strategy, aiChatTurnStrategySourceDefault, "default_contextual_page_answer")
		strategy.ToolChoiceMode = aiChatTurnToolChoiceModelDecides
		if skillIDEnabled(parts.SkillIDs, skills.SkillConsoleNavigator) {
			strategy.SupportingSkills = appendUniqueStrings(strategy.SupportingSkills, skills.SkillConsoleNavigator)
		}
	}
	return finalizeAIChatTurnStrategy(parts, strategy)
}

func markAIChatTurnStrategySource(strategy *AIChatTurnStrategy, source string, reason string) *AIChatTurnStrategy {
	if strategy == nil {
		return nil
	}
	if source = strings.TrimSpace(source); source != "" {
		strategy.Source = source
	}
	if reason = strings.TrimSpace(reason); reason != "" {
		strategy.SourceReason = reason
	}
	return strategy
}

func finalizeAIChatTurnStrategy(parts *chatRequestParts, strategy *AIChatTurnStrategy) *AIChatTurnStrategy {
	if parts == nil || strategy == nil || !strategy.RouteRequired ||
		!skillIDEnabled(parts.SkillIDs, skills.SkillConsoleNavigator) {
		return enrichAIChatTurnStrategyPlannedTools(parts, strategy)
	}
	target := consoleNavigationRouteHintForHref(strategy.TargetPage)
	if target.Href == "" {
		target = consoleNavigationRouteHint{Href: strategy.TargetPage, Label: "Console page"}
	}
	if strings.TrimSpace(target.Href) == "" {
		return enrichAIChatTurnStrategyPlannedTools(parts, strategy)
	}
	strategy.RequiredNextTool = &AIChatTurnStrategyTool{
		SkillID:  skills.SkillConsoleNavigator,
		ToolName: "navigate",
		Arguments: map[string]string{
			"href": target.Href,
		},
	}
	if target.Label != "" {
		strategy.RequiredNextTool.Arguments["reason"] = "open " + target.Label + " for the current user request"
	}
	strategy.RemainingRouteSequence = remainingConsoleNavigationRouteSequence(parts, target)
	strategy.Avoid = appendUniqueStrings(strategy.Avoid,
		"treat the route target as a preferred next phase, but if current page evidence already satisfies the target or a low-risk observe/read/list step is needed, continue from evidence and record the deviation instead of forcing a redundant route",
	)
	return enrichAIChatTurnStrategyPlannedTools(parts, strategy)
}

func enrichAIChatTurnStrategyPlannedTools(parts *chatRequestParts, strategy *AIChatTurnStrategy) *AIChatTurnStrategy {
	if parts == nil || strategy == nil {
		return strategy
	}
	if strategy.Intent == "manage_agent_asset" {
		strategy.ToolChoiceMode = aiChatTurnToolChoiceModelDecides
		strategy = appendAgentManagementModelDecidesGuidance(parts, strategy)
		strategy.PlannedTools = nil
		return attachContextualSidebarStructuredPlan(parts, strategy, parts.Query)
	}
	if strategy.Intent == "navigate_console_page" {
		return attachContextualSidebarStructuredPlan(parts, strategy, parts.Query)
	}
	switch strings.TrimSpace(strategy.Intent) {
	case "save_generated_file_to_file_management", "generate_temporary_file_artifact", "delete_visible_file":
		strategy.PlannedTools = nil
	}
	return attachContextualSidebarStructuredPlan(parts, strategy, parts.Query)
}

func appendAgentManagementModelDecidesGuidance(parts *chatRequestParts, strategy *AIChatTurnStrategy) *AIChatTurnStrategy {
	if parts == nil || strategy == nil || !skillIDEnabled(parts.SkillIDs, skills.SkillAgentManagement) {
		return strategy
	}
	capabilityGoals := agentManagementCapabilityGoalsFromModelIntent(parts.ModelTurnIntent)
	if len(capabilityGoals) > 0 {
		strategy.CapabilityGoals = appendAgentCapabilityGoals(strategy.CapabilityGoals, capabilityGoals...)
		strategy.SuccessCriteria = appendUniqueStrings(strategy.SuccessCriteria, agentCapabilityGoalSuccessCriteria(capabilityGoals)...)
		if agentManagementCapabilityGoalsNeedSkillCandidateLookup(strategy.CapabilityGoals) {
			strategy.Avoid = appendUniqueStrings(strategy.Avoid,
				"do not treat system_prompt or file_upload_enabled alone as proof of a skill-backed Agent capability; resolve and bind a matching Skill, then verify enabled_skill_ids",
			)
		}
	}
	if agentManagementNeedsFileReadPrecondition(parts) {
		strategy.SupportingSkills = appendUniqueStrings(strategy.SupportingSkills, skills.SkillFileReader)
		strategy.SuccessCriteria = appendUniqueStrings(strategy.SuccessCriteria,
			"when the Agent task depends on file content, read the requested file before deriving Agent name, prompt, or configuration",
			"use the file-reader/read_file result as the source evidence for downstream Agent edits instead of page metadata or guessed content",
		)
		strategy.ObservationPoints = appendUniqueStrings(strategy.ObservationPoints, "files_page_visible_list", "read_file_result")
	}
	if agentManagementStrategyNeedsAvailableModelResolution(strategy) {
		strategy.SuccessCriteria = appendUniqueStrings(strategy.SuccessCriteria,
			"before changing an Agent model, resolve an available provider/model pair with the appropriate use case and use that exact pair in the config update",
		)
	}
	if agentManagementStrategyNeedsSkillCandidateResolution(strategy) {
		strategy.SuccessCriteria = appendUniqueStrings(strategy.SuccessCriteria,
			"for skill-backed Agent abilities, resolve a matching bindable Skill before updating enabled_skill_ids",
		)
	}
	if agentManagementStrategyNeedsPostMutationConfigVerification(strategy) {
		strategy.SuccessCriteria = appendUniqueStrings(strategy.SuccessCriteria,
			"read current Agent config when needed, preserve unmentioned existing settings, and verify updated config after mutation",
		)
		strategy.Avoid = appendUniqueStrings(strategy.Avoid,
			"do not stop after the first Agent mutation when the user asked for later create, edit, model, skill, upload, memory, knowledge, database, or workflow changes",
		)
	}
	return strategy
}

func agentManagementStrategyNeedsAvailableModelResolution(strategy *AIChatTurnStrategy) bool {
	if strategy == nil {
		return false
	}
	if aiChatTurnStrategyHasPlannedTool(strategy, skills.SkillAgentManagement, "list_available_models") {
		return true
	}
	for _, field := range agentCapabilityGoalsExpectedConfigFields(strategy.CapabilityGoals) {
		switch operationPlanAgentConfigCanonicalField(field) {
		case "model", "model_provider":
			return true
		}
	}
	return false
}

func agentManagementStrategyNeedsSkillCandidateResolution(strategy *AIChatTurnStrategy) bool {
	if strategy == nil {
		return false
	}
	return aiChatTurnStrategyHasPlannedTool(strategy, skills.SkillAgentManagement, "list_agent_skill_candidates") ||
		agentManagementCapabilityGoalsNeedSkillCandidateLookup(strategy.CapabilityGoals)
}

func agentManagementStrategyNeedsPostMutationConfigVerification(strategy *AIChatTurnStrategy) bool {
	if strategy == nil {
		return false
	}
	if agentManagementCapabilityGoalsNeedPostUpdateRead(strategy.CapabilityGoals) {
		return true
	}
	for _, tool := range strategy.PlannedTools {
		if !strings.EqualFold(strings.TrimSpace(tool.SkillID), skills.SkillAgentManagement) {
			continue
		}
		switch strings.TrimSpace(tool.ToolName) {
		case "update_agent_config", "update_agent_identity":
			return true
		}
	}
	return false
}

func aiChatTurnStrategyHasPlannedTool(strategy *AIChatTurnStrategy, skillID string, toolName string) bool {
	if strategy == nil {
		return false
	}
	for _, tool := range strategy.PlannedTools {
		if !strings.EqualFold(strings.TrimSpace(tool.SkillID), strings.TrimSpace(skillID)) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(tool.ToolName), strings.TrimSpace(toolName)) {
			return true
		}
	}
	return false
}

func agentManagementNeedsFileReadPrecondition(parts *chatRequestParts) bool {
	if parts == nil || !skillIDEnabled(parts.SkillIDs, skills.SkillFileReader) {
		return false
	}
	if !modelTurnIntentHasRecommendedCapability(parts.ModelTurnIntent, "visible_file_content", "file_content", "source_file_content") {
		return false
	}
	if consoleFilesRouteAlreadyAvailable(parts) {
		return true
	}
	return modelTurnIntentHasRecommendedCapability(parts.ModelTurnIntent, "page_navigation")
}

func agentManagementCurrentAgentIDFromParts(parts *chatRequestParts) string {
	if parts == nil || !isConsoleAgentsContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		return ""
	}
	if agentID := agentIDFromConsoleAgentPageRoute(consoleRouteFromRuntimeContext(parts.RuntimeContext)); agentID != "" {
		return agentID
	}
	agents := consoleAgentsPromptVisibleAgents(parts)
	selectedID := ""
	selectedCount := 0
	for _, agent := range agents {
		selected, _ := agent["selected"].(bool)
		if !selected {
			continue
		}
		if id := strings.TrimSpace(firstNonEmptyString(agent["agent_id"], agent["id"], agent["resource_id"])); id != "" {
			selectedID = id
			selectedCount++
		}
	}
	if selectedCount == 1 {
		return selectedID
	}
	return ""
}

func agentIDFromConsoleAgentPageRoute(route string) string {
	route = strings.TrimSpace(route)
	if route == "" {
		return ""
	}
	const prefix = "/console/agents/"
	idx := strings.Index(route, prefix)
	if idx < 0 {
		return ""
	}
	rest := strings.TrimPrefix(route[idx:], prefix)
	agentID, _, _ := strings.Cut(rest, "/")
	agentID = strings.TrimSpace(agentID)
	if agentID == "" || agentID == "new" {
		return ""
	}
	return agentID
}

func agentManagementModelIntentResolvesVisibleTarget(parts *chatRequestParts) bool {
	if parts == nil || !isConsoleAgentsContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		return false
	}
	if parts.ModelTurnIntent == nil || parts.ModelTurnIntent.TargetVisibleIndex <= 0 {
		return false
	}
	if len(consoleAgentsPromptVisibleAgents(parts)) == 0 {
		return false
	}
	return true
}

func agentManagementResourceMarkerNegatedInClause(text string, markerStart int) bool {
	if markerStart <= 0 {
		return false
	}
	if agentManagementResourceMarkerNegatedInListScope(text, markerStart) {
		return true
	}
	clauseStart := agentManagementClauseStart(text, markerStart)
	prefix := strings.TrimSpace(text[clauseStart:markerStart])
	if prefix == "" {
		return false
	}
	if containsAnySubstring(prefix, []string{
		"do not", "don't", "dont", "without", "never", "not ", "no ",
		"keep unchanged", "leave unchanged", "keep the same", "keep ", "preserve ", "retain ", "unchanged",
		"do not say", "don't say", "dont say", "do not claim", "don't claim", "dont claim",
		"do not mention", "don't mention", "dont mention", "do not report", "don't report", "dont report",
	}) {
		return true
	}
	compact := strings.NewReplacer(
		" ", "",
		"\t", "",
		"\n", "",
		"\r", "",
		",", "",
		".", "",
		";", "",
		":", "",
		"\uff0c", "",
		"\u3002", "",
		"\uff1b", "",
		"\uff1a", "",
		"\u3001", "",
	).Replace(prefix)
	if containsAnySubstring(compact, []string{
		"\u4e0d\u6539",
		"\u4e0d\u8981\u6539",
		"\u522b\u6539",
		"\u4e0d\u7528\u6539",
		"\u4e0d\u4fee\u6539",
		"\u4e0d\u8981\u4fee\u6539",
		"\u522b\u4fee\u6539",
		"\u4e0d\u66f4\u6539",
		"\u4e0d\u8981\u66f4\u6539",
		"\u4e0d\u53d8\u66f4",
		"\u4e0d\u8bbe\u7f6e",
		"\u4e0d\u5207\u6362",
		"\u4e0d\u66ff\u6362",
		"\u4e0d\u7ed1\u5b9a",
		"\u4e0d\u5173\u8054",
		"\u4e0d\u89e3\u7ed1",
		"\u4e0d\u8981\u52a8",
		"\u65e0\u9700",
		"\u4e0d\u9700\u8981",
		"\u4fdd\u6301\u4e0d\u53d8",
		"\u4fdd\u6301\u539f\u6837",
		"\u7ef4\u6301\u539f\u6837",
		"\u4fdd\u7559",
		"\u4e0d\u8981\u8bf4",
		"\u522b\u8bf4",
		"\u4e0d\u8981\u58f0\u79f0",
		"\u522b\u58f0\u79f0",
		"\u4e0d\u8981\u5ba3\u79f0",
		"\u4e0d\u8981\u56de\u7b54",
		"\u4e0d\u8981\u56de\u590d",
		"\u4e0d\u8981\u63d0\u5230",
		"\u4e0d\u8981\u5199",
		"\u4e0d\u8981\u8f93\u51fa",
	}) {
		return true
	}
	return agentManagementMutationMarkerNegated(text, markerStart)
}

func agentManagementResourceMarkerNegatedInListScope(text string, markerStart int) bool {
	if markerStart <= 0 || markerStart > len(text) {
		return false
	}
	sentenceStart := agentManagementNegatedListScopeStart(text, markerStart)
	prefix := strings.ToLower(strings.TrimSpace(text[sentenceStart:markerStart]))
	if prefix == "" {
		return false
	}
	negationStart := agentManagementLastNegatedListIntro(prefix)
	if negationStart < 0 {
		return false
	}
	tail := prefix[negationStart:]
	if containsAnySubstring(tail, []string{
		" but ", " however ", " except ", " unless ", " then ", " and then ",
		"\u4f46", "\u4f46\u662f", "\u4e0d\u8fc7", "\u9664\u975e", "\u7136\u540e",
	}) {
		return false
	}
	return true
}

func agentManagementNegatedListScopeStart(text string, markerStart int) int {
	if markerStart <= 0 || markerStart > len(text) {
		return 0
	}
	start := 0
	for _, separator := range []string{
		"\u3002", "\uff1b", "\uff01", "\uff1f",
		".", ";", "!", "?", "\n", "\r",
	} {
		if separator == "" {
			continue
		}
		if idx := strings.LastIndex(text[:markerStart], separator); idx >= 0 && idx+len(separator) > start {
			start = idx + len(separator)
		}
	}
	return start
}

func agentManagementLastNegatedListIntro(prefix string) int {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if prefix == "" {
		return -1
	}
	best := -1
	for _, marker := range []string{
		"do not change", "don't change", "dont change",
		"do not modify", "don't modify", "dont modify",
		"do not update", "don't update", "dont update",
		"do not edit", "don't edit", "dont edit",
		"do not set", "don't set", "dont set",
		"do not switch", "don't switch", "dont switch",
		"do not replace", "don't replace", "dont replace",
		"without changing", "without modifying", "without updating",
		"keep unchanged", "leave unchanged", "keep the same", "preserve",
	} {
		if idx := strings.LastIndex(prefix, marker); idx > best {
			best = idx
		}
	}
	compact := strings.NewReplacer(
		" ", "",
		"\t", "",
		"\n", "",
		"\r", "",
		",", "",
		";", "",
		":", "",
		"\uff0c", "",
		"\uff1b", "",
		"\uff1a", "",
		"\u3001", "",
	).Replace(prefix)
	for _, marker := range []string{
		"\u4e0d\u6539",
		"\u4e0d\u8981\u6539",
		"\u522b\u6539",
		"\u4e0d\u7528\u6539",
		"\u4e0d\u4fee\u6539",
		"\u4e0d\u8981\u4fee\u6539",
		"\u522b\u4fee\u6539",
		"\u4e0d\u66f4\u6539",
		"\u4e0d\u8981\u66f4\u6539",
		"\u4e0d\u8981\u52a8",
		"\u65e0\u9700\u4fee\u6539",
		"\u4fdd\u6301\u4e0d\u53d8",
		"\u4fdd\u6301\u539f\u6837",
		"\u7ef4\u6301\u539f\u6837",
		"\u4fdd\u7559",
	} {
		if idx := strings.LastIndex(compact, marker); idx >= 0 && idx > best {
			best = idx
		}
	}
	return best
}

func agentManagementClauseStart(text string, markerStart int) int {
	if markerStart <= 0 || markerStart > len(text) {
		return 0
	}
	start := 0
	for _, separator := range []string{
		"\uff0c", "\u3002", "\uff1b", "\uff1a", "\uff01", "\uff1f",
		",", ".", ";", ":", "!", "?", "\n", "\r",
	} {
		if separator == "" {
			continue
		}
		if idx := strings.LastIndex(text[:markerStart], separator); idx >= 0 && idx+len(separator) > start {
			start = idx + len(separator)
		}
	}
	return start
}

func agentManagementMutationMarkerNegated(text string, markerStart int) bool {
	if markerStart <= 0 {
		return false
	}
	prefixStart := markerStart - 48
	if prefixStart < 0 {
		prefixStart = 0
	}
	prefix := strings.TrimSpace(text[prefixStart:markerStart])
	if prefix == "" {
		return false
	}
	if agentManagementEnglishOrNegationPrefix(prefix) {
		return true
	}
	if containsAnySuffix(prefix, []string{
		"do not", "don't", "dont", "without", "never", "no", "do not make any",
		"do not bind or", "don't bind or", "dont bind or", "without binding or",
		"do not add or", "don't add or", "dont add or", "without adding or",
		"do not enable or", "don't enable or", "dont enable or", "without enabling or",
		"do not associate or", "don't associate or", "dont associate or", "without associating or",
	}) {
		return true
	}
	compact := strings.NewReplacer(
		" ", "",
		"\t", "",
		"\n", "",
		"\r", "",
		",", "",
		".", "",
		";", "",
		":", "",
		"\uff0c", "",
		"\u3002", "",
		"\uff1b", "",
		"\uff1a", "",
		"\u3001", "",
	).Replace(prefix)
	return containsAnySuffix(compact, []string{
		"\u4e0d",
		"\u522b",
		"\u7981\u6b62",
		"\u4e0d\u8981",
		"\u4e0d\u7528",
		"\u65e0\u9700",
		"\u4e0d\u9700\u8981",
		"\u4e0d\u53ef",
		"\u4e0d\u505a",
		"\u4e0d\u8981\u505a",
		"\u4e0d\u505a\u4efb\u4f55",
		"\u4e0d\u8981\u505a\u4efb\u4f55",
		"\u4e0d\u8981\u6dfb\u52a0\u6216",
		"\u4e0d\u7528\u6dfb\u52a0\u6216",
		"\u65e0\u9700\u6dfb\u52a0\u6216",
		"\u4e0d\u9700\u8981\u6dfb\u52a0\u6216",
		"\u4e0d\u8981\u7ed1\u5b9a\u6216",
		"\u4e0d\u7ed1\u5b9a\u6216",
		"\u4e0d\u7528\u7ed1\u5b9a\u6216",
		"\u65e0\u9700\u7ed1\u5b9a\u6216",
		"\u4e0d\u9700\u8981\u7ed1\u5b9a\u6216",
		"\u4e0d\u8981\u542f\u7528\u6216",
		"\u4e0d\u542f\u7528\u6216",
		"\u4e0d\u7528\u542f\u7528\u6216",
		"\u65e0\u9700\u542f\u7528\u6216",
		"\u4e0d\u9700\u8981\u542f\u7528\u6216",
		"\u4e0d\u8981\u5173\u8054\u6216",
		"\u4e0d\u5173\u8054\u6216",
		"\u4e0d\u7528\u5173\u8054\u6216",
		"\u65e0\u9700\u5173\u8054\u6216",
		"\u4e0d\u9700\u8981\u5173\u8054\u6216",
	}) || agentManagementMutationMarkerHasListNegationPrefix(prefix)
}

func agentManagementEnglishOrNegationPrefix(prefix string) bool {
	prefix = strings.TrimSpace(strings.ToLower(prefix))
	if prefix == "" || !strings.HasSuffix(prefix, " or") {
		return false
	}
	for _, marker := range []string{
		"do not ",
		"don't ",
		"dont ",
		"without ",
		"never ",
		"no ",
	} {
		if strings.LastIndex(prefix, marker) >= 0 {
			return true
		}
	}
	return false
}

func agentManagementMutationMarkerHasListNegationPrefix(prefix string) bool {
	prefix = strings.TrimSpace(strings.ToLower(prefix))
	if prefix == "" {
		return false
	}
	for _, marker := range []string{
		"do not", "don't", "dont", "without", "never", "no",
		"\u4e0d\u8981", "\u4e0d\u7528", "\u65e0\u9700", "\u4e0d\u9700\u8981", "\u4e0d\u505a", "\u7981\u6b62", "\u522b", "\u4e0d",
	} {
		idx := strings.LastIndex(prefix, marker)
		if idx < 0 {
			continue
		}
		tail := strings.TrimSpace(prefix[idx+len(marker):])
		if tail == "" {
			continue
		}
		if agentManagementEnglishMutationListTail(tail) || agentManagementChineseMutationListTail(tail) {
			return true
		}
	}
	return false
}

func agentManagementEnglishMutationListTail(tail string) bool {
	tail = strings.NewReplacer(
		",", " ",
		".", " ",
		";", " ",
		":", " ",
		"/", " ",
		"\\", " ",
		"(", " ",
		")", " ",
		"[", " ",
		"]", " ",
		"{", " ",
		"}", " ",
	).Replace(strings.ToLower(strings.TrimSpace(tail)))
	if tail == "" {
		return false
	}
	for _, token := range strings.Fields(tail) {
		switch token {
		case "and", "or", "nor", "also", "then", "any", "the", "this", "that",
			"modify", "modifying", "modified",
			"edit", "editing", "edited",
			"change", "changing", "changed",
			"update", "updating", "updated",
			"set", "setting",
			"replace", "replacing", "replaced",
			"switch", "switching", "switched",
			"enable", "enabling", "enabled",
			"disable", "disabling", "disabled",
			"bind", "binding", "bound",
			"unbind", "unbinding", "unbound",
			"create", "creating", "created",
			"add", "adding", "added",
			"delete", "deleting", "deleted",
			"remove", "removing", "removed",
			"save", "saving", "saved",
			"asset", "assets", "resource", "resources", "config", "configuration":
			continue
		default:
			return false
		}
	}
	return true
}

func agentManagementChineseMutationListTail(tail string) bool {
	tail = strings.NewReplacer(
		" ", "",
		"\t", "",
		"\n", "",
		"\r", "",
		",", "",
		".", "",
		";", "",
		":", "",
		"\uff0c", "",
		"\u3002", "",
		"\uff1b", "",
		"\uff1a", "",
		"\u3001", "",
	).Replace(strings.TrimSpace(tail))
	if tail == "" {
		return false
	}
	for steps := 0; tail != "" && steps < 128; steps++ {
		matched := false
		for _, piece := range []string{
			"\u4fee\u6539", "\u66f4\u6539", "\u7f16\u8f91", "\u8c03\u6574", "\u53d8\u66f4", "\u66f4\u65b0",
			"\u8bbe\u7f6e", "\u66ff\u6362", "\u5207\u6362", "\u542f\u7528", "\u7981\u7528", "\u505c\u7528",
			"\u7ed1\u5b9a", "\u89e3\u7ed1", "\u5173\u8054", "\u6dfb\u52a0", "\u521b\u5efa", "\u65b0\u5efa",
			"\u65b0\u589e", "\u5220\u9664", "\u79fb\u9664", "\u4fdd\u5b58",
			"\u6216\u8005", "\u4ee5\u53ca", "\u4efb\u4f55", "\u914d\u7f6e", "\u8d44\u6e90", "\u8d44\u4ea7",
			"\u548c", "\u6216", "\u53ca", "\u4e0e", "\u4e5f",
		} {
			if strings.HasPrefix(tail, piece) {
				tail = strings.TrimPrefix(tail, piece)
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return tail == ""
}

func containsAnySuffix(text string, suffixes []string) bool {
	text = strings.TrimSpace(strings.ToLower(text))
	if text == "" {
		return false
	}
	for _, suffix := range suffixes {
		suffix = strings.TrimSpace(strings.ToLower(suffix))
		if suffix != "" && strings.HasSuffix(text, suffix) {
			return true
		}
	}
	return false
}

func appendPlannedToolWithStep(strategy *AIChatTurnStrategy, skillID string, toolName string, arguments map[string]string, stepID string, waitForStepID string) *AIChatTurnStrategy {
	if strategy == nil {
		return strategy
	}
	skillID = strings.TrimSpace(skillID)
	toolName = strings.TrimSpace(toolName)
	if skillID == "" || toolName == "" {
		return strategy
	}
	id := strings.TrimSpace(stepID)
	if id == "" {
		id = operationPlanToolStepID(skillID, toolName)
	}
	for idx, existing := range strategy.PlannedTools {
		if aiChatTurnStrategyToolStepID(existing) == id {
			if len(arguments) > 0 {
				strategy.PlannedTools[idx].Arguments = mergeTurnStrategyToolArguments(strategy.PlannedTools[idx].Arguments, arguments)
			}
			return strategy
		}
	}
	tool := AIChatTurnStrategyTool{
		SkillID:  skillID,
		ToolName: toolName,
	}
	if len(arguments) > 0 {
		tool.Arguments = arguments
	}
	if strings.TrimSpace(stepID) != "" {
		tool.StepID = strings.TrimSpace(stepID)
	}
	if strings.TrimSpace(waitForStepID) != "" {
		tool.WaitForStepID = strings.TrimSpace(waitForStepID)
	}
	strategy.PlannedTools = append(strategy.PlannedTools, tool)
	return strategy
}

func mergeTurnStrategyToolArguments(current map[string]string, additions map[string]string) map[string]string {
	if len(additions) == 0 {
		return current
	}
	next := map[string]string{}
	for key, value := range current {
		key = strings.TrimSpace(key)
		if key != "" {
			next[key] = value
		}
	}
	for key, value := range additions {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		if key == operationPlanExpectedUpdatedFieldsKey {
			next[key] = strings.Join(mergeCommaSeparatedValues(next[key], value), ",")
			continue
		}
		if key == operationPlanExpectedBindingActionsKey {
			next[key] = mergeAgentConfigBindingActionSpecs(next[key], value)
			continue
		}
		if strings.TrimSpace(next[key]) == "" {
			next[key] = value
		}
	}
	if len(next) == 0 {
		return nil
	}
	return next
}

func mergeAgentConfigBindingActionSpecs(values ...string) string {
	merged := map[string]string{}
	for _, value := range values {
		for field, action := range operationPlanAgentConfigBindingActionsFromAny(value) {
			merged[field] = action
		}
	}
	return operationPlanEncodeAgentConfigBindingActions(merged)
}

func mergeCommaSeparatedValues(values ...string) []string {
	out := []string{}
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			item = strings.TrimSpace(item)
			if item != "" {
				out = appendUniqueStrings(out, item)
			}
		}
	}
	return out
}

func aiChatTurnStrategyToolStepID(tool AIChatTurnStrategyTool) string {
	if id := strings.TrimSpace(tool.StepID); id != "" {
		return id
	}
	return operationPlanToolStepID(tool.SkillID, tool.ToolName)
}

func chatPartsSkillsEnabled(parts *chatRequestParts) bool {
	return parts != nil && parts.SkillMode != skillModeDisabled && len(parts.SkillIDs) > 0
}

func contextualAgentManagementStrategy(parts *chatRequestParts, strategy *AIChatTurnStrategy) *AIChatTurnStrategy {
	strategy.Intent = "manage_agent_asset"
	strategy.TargetPage = agentManagementTargetPage(parts, strategy)
	if strings.TrimSpace(strategy.AssetEffect) == "" || strings.EqualFold(strings.TrimSpace(strategy.AssetEffect), "none") {
		strategy.AssetEffect = "agent_asset_change_or_inspection"
	}
	strategy.AssetRisk = "medium"
	strategy.Approval = "agent-management mutations are governed; delete always requires approval"
	strategy.SuccessCriteria = []string{
		"Agent page or Agent detail context is available before mutating Agent assets",
		"agent-management uses a resolved Agent ID for edit, config, or delete operations",
		"only supported MVP fields are changed; publishing, rollback, invocation, API keys, and WebApp online/offline state are not attempted",
		"binding and unbinding edits use supported draft config binding lists when exact current bindings and candidates are known",
		"agent-management tool results and get_agent_config reads are authoritative backend evidence for Agent state",
	}
	strategy.ObservationPoints = nil

	primary := append([]string(nil), strategy.PrimarySkills...)
	supporting := append([]string(nil), strategy.SupportingSkills...)
	if isConsoleAgentsContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		if detailTarget, ok := agentManagementExplicitDetailNavigationTarget(parts); ok {
			strategy.TargetPage = detailTarget.Href
			strategy.RouteRequired = !consoleNavigationRouteAlreadyAvailable(parts, detailTarget.Href) &&
				!clientActionContinuationLoadedRoute(parts, detailTarget.Href)
			if strategy.RouteRequired && skillIDEnabled(parts.SkillIDs, skills.SkillConsoleNavigator) {
				primary = appendUniqueStrings(primary, skills.SkillConsoleNavigator)
			}
			strategy.SuccessCriteria = appendUniqueStrings(strategy.SuccessCriteria,
				"console-navigator/navigate opens the explicitly requested Agent detail page before claiming it is open",
			)
		} else {
			strategy.RouteRequired = false
		}
		if skillIDEnabled(parts.SkillIDs, skills.SkillAgentManagement) {
			primary = appendUniqueStrings(primary, skills.SkillAgentManagement)
		}
		if agentManagementModelIntentResolvesVisibleTarget(parts) {
			strategy.SuccessCriteria = appendUniqueStrings(strategy.SuccessCriteria,
				"visible Agent targets from current page context are used directly when model intent resolves a visible Agent ordinal target",
			)
			strategy.Avoid = appendUniqueStrings(strategy.Avoid,
				"avoid redundant agent-management/list_agents before operating on visible ordinal Agent targets already present in page context",
				"avoid navigation after an Agent list-page mutation unless the user explicitly asked to open a page or the current detail page was deleted",
				"avoid waiting for asset_observation or refreshed page context after Agent mutations when agent-management tool results already confirm the state",
			)
		}
	} else {
		strategy.RouteRequired = true
		if skillIDEnabled(parts.SkillIDs, skills.SkillConsoleNavigator) {
			primary = appendUniqueStrings(primary, skills.SkillConsoleNavigator)
		}
		if skillIDEnabled(parts.SkillIDs, skills.SkillAgentManagement) {
			supporting = appendUniqueStrings(supporting, skills.SkillAgentManagement)
		}
	}
	strategy.PrimarySkills = primary
	strategy.SupportingSkills = supporting
	return strategy
}

func agentManagementTargetPage(parts *chatRequestParts, strategy *AIChatTurnStrategy) string {
	runtimeContext := ""
	rawOperationContext := map[string]interface{}(nil)
	operationContext := map[string]interface{}(nil)
	if parts != nil {
		runtimeContext = firstNonEmptyString(parts.RuntimeContext, "")
		rawOperationContext = parts.RawOperationContext
		operationContext = parts.OperationContext
	}
	explicitTargetPage := normalizeConsoleNavigationGuardHref(stringFromAny(strategy.TargetPage))
	currentPage := normalizeConsoleNavigationGuardHref(stringFromAny(firstNonEmptyString(strategy.CurrentPage, "")))
	if explicitTargetPage != "" && explicitTargetPage != currentPage && strings.HasPrefix(explicitTargetPage, "/console/agents") {
		return explicitTargetPage
	}
	for _, candidate := range []string{
		currentPage,
		consoleRouteFromRuntimeContext(runtimeContext),
	} {
		if isConsoleAgentDetailRoute(candidate) {
			return normalizeConsoleNavigationGuardHref(candidate)
		}
	}
	for _, source := range []map[string]interface{}{rawOperationContext, operationContext} {
		agents := visibleAgentResources(source)
		for _, agent := range agents {
			if agent.Selected && isConsoleAgentDetailRoute(agent.Href) {
				return normalizeConsoleNavigationGuardHref(agent.Href)
			}
		}
		if len(agents) == 1 && isConsoleAgentDetailRoute(agents[0].Href) {
			return normalizeConsoleNavigationGuardHref(agents[0].Href)
		}
	}
	return "/console/agents"
}

func agentManagementExplicitDetailNavigationTarget(parts *chatRequestParts) (consoleNavigationRouteHint, bool) {
	if parts == nil {
		return consoleNavigationRouteHint{}, false
	}
	normalized := normalizeConsoleNavigationQuery(parts.Query)
	if normalized == "" || createdAgentDetailNavigationNegated(normalized) {
		return consoleNavigationRouteHint{}, false
	}
	if !containsAnySubstring(normalized, []string{
		"open", "enter", "detail", "details", "edit page", "configuration page", "config page", "settings page",
		"\u6253\u5f00", "\u8fdb\u5165", "\u8be6\u60c5", "\u7f16\u8f91\u9875", "\u914d\u7f6e\u9875", "\u8bbe\u7f6e\u9875",
	}) {
		return consoleNavigationRouteHint{}, false
	}
	if parts.ModelTurnIntent != nil && modelTurnIntentAssetEffectIsDelete(parts.ModelTurnIntent.AssetEffect) {
		return consoleNavigationRouteHint{}, false
	}
	agents := visibleAgentsForAgentManagementTargetResolution(parts)
	matches := make([]visibleConsoleAgentResource, 0, 1)
	for _, agent := range agents {
		name := normalizeConsoleNavigationQuery(agent.Title)
		if name == "" || !strings.Contains(normalized, name) {
			continue
		}
		matches = append(matches, agent)
	}
	if len(matches) == 1 {
		href := normalizeAgentDetailHref(matches[0].Href)
		if href == "" && strings.TrimSpace(matches[0].ID) != "" {
			href = consoleAgentDetailHref(matches[0].ID)
		}
		if href != "" {
			return consoleNavigationRouteHint{Href: href, Label: firstNonEmptyString(matches[0].Title, "Agent detail")}, true
		}
	}
	if len(matches) > 1 {
		return consoleNavigationRouteHint{}, false
	}
	if containsAnySubstring(normalized, []string{"current agent", "this agent", "\u5f53\u524d\u667a\u80fd\u4f53", "\u8fd9\u4e2a\u667a\u80fd\u4f53"}) {
		for _, agent := range agents {
			if !agent.Selected {
				continue
			}
			if href := normalizeAgentDetailHref(agent.Href); href != "" {
				return consoleNavigationRouteHint{Href: href, Label: firstNonEmptyString(agent.Title, "Agent detail")}, true
			}
		}
	}
	return consoleNavigationRouteHint{}, false
}

func visibleAgentsForAgentManagementTargetResolution(parts *chatRequestParts) []visibleConsoleAgentResource {
	if parts == nil {
		return nil
	}
	seen := map[string]struct{}{}
	out := []visibleConsoleAgentResource{}
	for _, source := range []map[string]interface{}{parts.RawOperationContext, parts.OperationContext} {
		for _, agent := range visibleAgentResources(source) {
			id := strings.TrimSpace(agent.ID)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, agent)
		}
	}
	return out
}

func isConsoleAgentDetailRoute(value string) bool {
	return normalizeAgentDetailHref(value) != ""
}

func contextualContinuationStrategy(parts *chatRequestParts, strategy *AIChatTurnStrategy) *AIChatTurnStrategy {
	strategy.Intent = "continue_previous_task"
	strategy.RouteRequired = false
	strategy.AssetEffect = "continue_prior_effect"
	strategy.AssetRisk = "context_dependent"
	strategy.Approval = "reuse the prior task's tool governance rules; do not skip approval for new asset mutations"
	strategy.SuccessCriteria = []string{
		"continue from the recent execution context instead of treating the request as a new generic question",
		"do not repeat successful side-effecting tool calls from the recent task",
		"perform only the remaining requested step and report the actual tool result",
	}
	strategy.ObservationPoints = []string{"recent_execution_context", "current_page_context", "remaining_task_step"}
	if isManagedFileCreateIntent(parts.Query) {
		strategy.TargetPage = consoleFilesRouteHint().Href
		strategy.RouteRequired = !consoleNavigationRouteAlreadyAvailable(parts, consoleFilesRouteHint().Href) &&
			!clientActionContinuationLoadedRoute(parts, consoleFilesRouteHint().Href)
		strategy.AssetEffect = "create"
		strategy.AssetRisk = "medium"
		strategy.Approval = "continue the prior task while applying file-manager/save_file_to_management governance for each managed file"
		strategy.SuccessCriteria = []string{
			"continue from the recent execution context instead of treating the request as a new generic question",
			"do not repeat successful side-effecting tool calls from the recent task",
			"each requested destination file has exactly one temporary artifact selected",
			"file-manager/save_file_to_management succeeds for each selected artifact",
			"asset observation or refreshed page context confirms every created file is visible",
		}
		strategy.ObservationPoints = []string{"recent_execution_context", "route_loaded:/console/files", "asset_observation:file.create", "files_page_visible_list"}
		if strategy.RouteRequired {
			if skillIDEnabled(parts.SkillIDs, skills.SkillConsoleNavigator) {
				strategy.PrimarySkills = appendUniqueStrings(strategy.PrimarySkills, skills.SkillConsoleNavigator)
			}
			if len(parts.RecentGeneratedArtifacts) == 0 {
				strategy.SupportingSkills = appendArtifactProducerSkills(strategy.SupportingSkills, parts)
			}
			if skillIDEnabled(parts.SkillIDs, skills.SkillFileManager) {
				strategy.SupportingSkills = appendUniqueStrings(strategy.SupportingSkills, skills.SkillFileManager)
			}
		} else {
			if len(parts.RecentGeneratedArtifacts) == 0 {
				strategy.PrimarySkills = appendArtifactProducerSkills(strategy.PrimarySkills, parts)
			} else {
				strategy.PrimarySkills = removeArtifactProducerSkills(strategy.PrimarySkills)
				strategy.ArtifactSource = "recent_generated_file"
			}
			if skillIDEnabled(parts.SkillIDs, skills.SkillFileManager) {
				strategy.PrimarySkills = appendUniqueStrings(strategy.PrimarySkills, skills.SkillFileManager)
			}
		}
		strategy.Avoid = appendUniqueStrings(strategy.Avoid,
			"after a temporary artifact is generated, do not regenerate it; save each unsaved generated artifact with file-manager/save_file_to_management",
		)
	}
	if skillIDEnabled(parts.SkillIDs, skills.SkillConsoleNavigator) {
		strategy.SupportingSkills = appendUniqueStrings(strategy.SupportingSkills, skills.SkillConsoleNavigator)
	}
	if isConsoleFilesContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		for _, skillID := range []string{skills.SkillFileReader, skills.SkillFileManager, skills.SkillFileGenerator, skills.SkillChartGenerator} {
			if skillIDEnabled(parts.SkillIDs, skillID) {
				strategy.SupportingSkills = appendUniqueStrings(strategy.SupportingSkills, skillID)
			}
		}
	}
	if isConsoleAgentsContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) &&
		skillIDEnabled(parts.SkillIDs, skills.SkillAgentManagement) &&
		recentOperationPlanHasPendingSkill(parts, skills.SkillAgentManagement) {
		strategy.SupportingSkills = appendUniqueStrings(strategy.SupportingSkills, skills.SkillAgentManagement)
	}
	return applyRecentOperationPlanToContinuationStrategy(parts, strategy)
}

func contextualManagedFileCreateStrategy(parts *chatRequestParts, strategy *AIChatTurnStrategy) *AIChatTurnStrategy {
	strategy.Intent = "save_generated_file_to_file_management"
	strategy.TargetPage = consoleFilesRouteHint().Href
	strategy.AssetEffect = "create"
	strategy.AssetRisk = "medium"
	strategy.Approval = "file-manager/save_file_to_management is governed; approval depends on the user's permission tier"
	strategy.SuccessCriteria = []string{
		"Files page context is loaded before File Management mutation when needed",
		"each requested destination file has exactly one temporary artifact selected",
		"file-manager/save_file_to_management succeeds for each selected artifact",
		"asset observation or refreshed page context confirms the created file is visible",
	}
	strategy.ObservationPoints = []string{"route_loaded:/console/files", "asset_observation:file.create", "files_page_visible_list"}

	primary := append([]string(nil), strategy.PrimarySkills...)
	supporting := append([]string(nil), strategy.SupportingSkills...)
	if requiresConsoleFilesRouteBeforeManagedFileCreate(parts) {
		strategy.RouteRequired = true
		primary = appendUniqueStrings(primary, skills.SkillConsoleNavigator)
		if len(parts.RecentGeneratedArtifacts) == 0 {
			supporting = appendArtifactProducerSkills(supporting, parts)
		}
		supporting = appendUniqueStrings(supporting, skills.SkillFileManager)
	} else if shouldReuseRecentGeneratedArtifactForManagedCreate(parts) {
		primary = removeArtifactProducerSkills(primary)
		primary = appendUniqueStrings(primary, skills.SkillFileManager)
		strategy.ArtifactSource = "recent_generated_file"
		strategy.Avoid = []string{"do not generate another file when the user refers to a recent generated file"}
	} else {
		primary = appendArtifactProducerSkills(primary, parts)
		primary = appendUniqueStrings(primary, skills.SkillFileManager)
	}
	strategy.PrimarySkills = primary
	strategy.SupportingSkills = supporting
	return strategy
}

func contextualTemporaryFileGenerateStrategy(parts *chatRequestParts, strategy *AIChatTurnStrategy) *AIChatTurnStrategy {
	strategy.Intent = "generate_temporary_file_artifact"
	strategy.AssetEffect = "create_temporary_artifact"
	strategy.AssetRisk = "medium"
	strategy.Approval = "artifact generation follows the producing skill manifest; do not save into File Management unless the user explicitly asks"
	strategy.SuccessCriteria = []string{
		"the appropriate artifact-producing skill creates exactly one requested temporary artifact per requested file",
		"generated_files metadata records the temporary artifact",
		"file-manager/save_file_to_management is not called unless the user explicitly asks to save or create in File Management",
	}
	strategy.ObservationPoints = []string{"generated_file_metadata", "message_file_card"}
	strategy.PrimarySkills = appendArtifactProducerSkills(strategy.PrimarySkills, parts)
	if skillIDEnabled(parts.SkillIDs, skills.SkillConsoleNavigator) {
		strategy.SupportingSkills = appendUniqueStrings(strategy.SupportingSkills, skills.SkillConsoleNavigator)
	}
	strategy.Avoid = appendUniqueStrings(strategy.Avoid,
		"do not answer that a temporary file was generated until an artifact-producing tool succeeds",
		"do not call file-manager/save_file_to_management for temporary-only generation requests",
	)
	return strategy
}

func contextualFileDeleteStrategy(parts *chatRequestParts, strategy *AIChatTurnStrategy) *AIChatTurnStrategy {
	strategy.Intent = "delete_visible_file"
	strategy.TargetPage = "/console/files"
	strategy.RouteRequired = false
	strategy.AssetEffect = "delete"
	strategy.AssetRisk = "high"
	strategy.Approval = "file-manager/delete_file always requires governed approval unless an approved session grant applies"
	if skillIDEnabled(parts.SkillIDs, skills.SkillFileManager) {
		strategy.PrimarySkills = appendUniqueStrings(strategy.PrimarySkills, skills.SkillFileManager)
	}
	strategy.SuccessCriteria = []string{
		"resolved visible file target is used as the tool argument",
		"file-manager/delete_file succeeds or reports the actual failure",
		"asset observation or refreshed page context confirms deletion state",
	}
	strategy.ObservationPoints = []string{"resolved_files_page_target", "asset_observation:file.delete", "files_page_visible_list"}
	return strategy
}

func contextualFileReadStrategy(parts *chatRequestParts, strategy *AIChatTurnStrategy) *AIChatTurnStrategy {
	strategy.Intent = "read_visible_file_content"
	strategy.TargetPage = "/console/files"
	strategy.RouteRequired = false
	strategy.AssetEffect = "read"
	strategy.AssetRisk = "low"
	strategy.Approval = "none for ordinary file read when workspace permissions allow it"
	if skillIDEnabled(parts.SkillIDs, skills.SkillFileReader) {
		strategy.PrimarySkills = appendUniqueStrings(strategy.PrimarySkills, skills.SkillFileReader)
	}
	strategy.SuccessCriteria = []string{
		"resolved visible file target is used as the tool argument",
		"file-reader/read_file returns extracted content or an explicit read failure",
		"final answer is based on the returned file content instead of page metadata only",
	}
	strategy.ObservationPoints = []string{"resolved_files_page_target", "read_file_result"}
	return strategy
}

func contextualNavigationStrategy(parts *chatRequestParts, strategy *AIChatTurnStrategy) *AIChatTurnStrategy {
	strategy.Intent = "navigate_console_page"
	strategy.AssetEffect = "none"
	strategy.AssetRisk = "low"
	strategy.Approval = "none"
	if skillIDEnabled(parts.SkillIDs, skills.SkillConsoleNavigator) {
		strategy.PrimarySkills = appendUniqueStrings(strategy.PrimarySkills, skills.SkillConsoleNavigator)
	}
	strategy.SuccessCriteria = []string{
		"console-navigator/navigate succeeds for the resolved route",
		"frontend client action reports route_loaded for the same href",
		"the same AIChat turn continues from updated page context",
	}
	strategy.ObservationPoints = []string{"route_navigation_client_action", "updated_page_context"}
	return strategy
}

func appendArtifactProducerSkills(values []string, parts *chatRequestParts) []string {
	if parts == nil {
		return values
	}
	if skillIDEnabled(parts.SkillIDs, skills.SkillChartGenerator) && shouldPreferChartArtifactProducer(parts) {
		return appendUniqueStrings(values, skills.SkillChartGenerator)
	}
	if skillIDEnabled(parts.SkillIDs, skills.SkillFileGenerator) {
		return appendUniqueStrings(values, skills.SkillFileGenerator)
	}
	if skillIDEnabled(parts.SkillIDs, skills.SkillChartGenerator) {
		return appendUniqueStrings(values, skills.SkillChartGenerator)
	}
	return values
}

func removeArtifactProducerSkills(values []string) []string {
	if len(values) == 0 {
		return values
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || value == skills.SkillFileGenerator || value == skills.SkillChartGenerator {
			continue
		}
		out = appendUniqueStrings(out, value)
	}
	return out
}

func shouldReuseRecentGeneratedArtifactForManagedCreate(parts *chatRequestParts) bool {
	if parts == nil || len(parts.RecentGeneratedArtifacts) == 0 || !isManagedFileCreateIntent(parts.Query) {
		return false
	}
	if isRecentGeneratedArtifactReferenceIntent(parts.Query) {
		return true
	}
	text := strings.ToLower(strings.TrimSpace(parts.Query))
	if containsAnySubstring(text, []string{
		"create", "generate", "write", "export", "make", "produce",
		"\u521b\u5efa", "\u65b0\u5efa", "\u751f\u6210", "\u5199", "\u5199\u4e00\u4e2a", "\u5bfc\u51fa", "\u505a\u4e00\u4e2a",
	}) {
		return false
	}
	return containsAnySubstring(text, []string{
		"save", "upload", "import", "add", "put",
		"\u4fdd\u5b58", "\u4e0a\u4f20", "\u5bfc\u5165", "\u6dfb\u52a0", "\u52a0\u5230", "\u653e\u5230", "\u5b58\u5230",
	})
}

func contextualTurnCurrentPage(parts *chatRequestParts) string {
	if parts == nil {
		return ""
	}
	for _, source := range []map[string]interface{}{parts.RawOperationContext, parts.OperationContext} {
		for _, resource := range mapSliceFromAny(source["resources"]) {
			if strings.TrimSpace(stringFromAny(resource["resource_type"])) != "page" {
				continue
			}
			if href := normalizeConsoleNavigationGuardHref(stringFromAny(resource["href"])); href != "" {
				return href
			}
			if metadata := governanceMapFromAny(resource["metadata"]); len(metadata) > 0 {
				if route := normalizeConsoleNavigationGuardHref(stringFromAny(metadata["route"])); route != "" {
					return route
				}
			}
		}
		if metadata := governanceMapFromAny(source["metadata"]); len(metadata) > 0 {
			if route := normalizeConsoleNavigationGuardHref(stringFromAny(metadata["route"])); route != "" {
				return route
			}
		}
	}
	if route := consoleRouteFromRuntimeContext(parts.RuntimeContext); route != "" {
		return route
	}
	if isConsoleFilesContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		return "/console/files"
	}
	return ""
}

func appendUniqueStrings(values []string, additions ...string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values)+len(additions))
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
	for _, value := range additions {
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

type consoleNavigationRouteHint struct {
	Href     string   `json:"href"`
	Label    string   `json:"label"`
	Keywords []string `json:"keywords,omitempty"`
}

type resolvedConsoleNavigationTarget struct {
	Hint     consoleNavigationRouteHint
	Position int
}

var consoleNavigationRouteHints = []consoleNavigationRouteHint{
	{Href: "/console", Label: "首页", Keywords: []string{"首页", "主页", "控制台首页", "home"}},
	{Href: "/console/work/chat", Label: "对话", Keywords: []string{"对话页面", "聊天页面", "会话页面", "conversation", "chat page"}},
	{Href: "/console/work/image", Label: "绘图", Keywords: []string{"绘图", "图像生成", "图片生成", "image page", "drawing"}},
	{Href: "/console/work/app", Label: "应用", Keywords: []string{"应用页面", "应用管理", "app page", "apps page"}},
	{Href: "/console/work/task", Label: "定时任务", Keywords: []string{"定时任务", "计划任务", "任务页面", "scheduled task", "tasks page"}},
	{Href: "/console/agents", Label: "智能体", Keywords: []string{"智能体", "agent page", "agents page", "agent list"}},
	{Href: "/console/agents", Label: "工作流", Keywords: []string{"工作流页面", "工作流列表", "workflow page", "workflows page"}},
	{Href: "/console/dataset", Label: "知识库", Keywords: []string{"知识库", "数据集", "dataset", "knowledge base"}},
	{Href: "/console/db", Label: "数据库", Keywords: []string{"数据库", "数据表", "database", "db page"}},
	{Href: "/console/files", Label: "文件管理", Keywords: []string{"文件管理", "文件页", "文件页面", "文件模块", "files page", "file management"}},
	{Href: "/console/prompts", Label: "提示词", Keywords: []string{"提示词", "prompt", "prompts page"}},
	{Href: "/console/developer/content-parse", Label: "文件识别", Keywords: []string{"文件识别", "内容解析", "content parse", "file recognition"}},
	{Href: "/console/workspace", Label: "工作空间", Keywords: []string{"工作空间", "workspace"}},
	{Href: "/console/settings", Label: "系统设置", Keywords: []string{"系统设置", "设置页面", "settings"}},
}

func contextualConsoleNavigationSkillMessageForResolved(prepared *PreparedChat, resolved *skills.ResolvedSkills) (adapter.Message, bool) {
	if prepared == nil || prepared.parts == nil || !skillIDEnabled(prepared.parts.SkillIDs, skills.SkillConsoleNavigator) {
		return adapter.Message{}, false
	}
	if !skillLoopResolvedToolAvailable(resolved, skills.SkillConsoleNavigator, "navigate") {
		return adapter.Message{}, false
	}

	routes := make([]map[string]string, 0, len(consoleNavigationRouteHints))
	seen := map[string]struct{}{}
	for _, route := range consoleNavigationRouteHints {
		route = normalizeConsoleNavigationRouteHint(route)
		key := route.Href + "\x00" + route.Label
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		routes = append(routes, map[string]string{
			"href":  route.Href,
			"label": route.Label,
		})
	}
	payload := map[string]interface{}{
		"skill_id":  skills.SkillConsoleNavigator,
		"tool_name": "navigate",
		"routes":    routes,
	}
	target, hasResolvedTarget := resolveConsoleNavigationTargetForPrepared(prepared)
	if hasResolvedTarget {
		payload["resolved_target_from_user_request"] = map[string]string{
			"href":  target.Href,
			"label": target.Label,
		}
		if consoleNavigationRouteAlreadyAvailable(prepared.parts, target.Href) {
			payload["target_route_already_available"] = true
			payload["route_evidence"] = "current_page_context_matches_target"
		}
		if !clientActionContinuationLoadedRoute(prepared.parts, target.Href) &&
			!consoleNavigationRouteAlreadyAvailable(prepared.parts, target.Href) {
			payload["preferred_route_action"] = map[string]interface{}{
				"skill_id":  skills.SkillConsoleNavigator,
				"tool_name": "navigate",
				"arguments": map[string]string{
					"href":   target.Href,
					"reason": "open " + target.Label + " for the current user request",
				},
			}
			payload["remaining_route_sequence"] = remainingConsoleNavigationRouteSequence(prepared.parts, target)
		}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return adapter.Message{}, false
	}

	content := strings.Join([]string{
		"ZGI console navigation guidance:",
		"Use console-navigator/navigate when the user asks to open, go to, enter, switch to, or navigate to a known ZGI console module page.",
		"When the user explicitly asks to save, upload, import, or write a file into File Management from another console page, navigate to /console/files only if the current page context is not already Files and the File Management page context is still needed for the save.",
		"When preferred_route_action is present in Console navigation JSON, treat it as a suggested route phase, not an immutable script: load console-navigator if needed and call navigate only when the current page does not already match the target.",
		"If current page evidence already satisfies the target, or a low-risk observe/read/list step is needed to complete the user's goal, continue from that evidence instead of forcing a redundant navigate call.",
		"When remaining_route_sequence has more than one pending route, complete exactly one navigate call, wait for the frontend route_loaded continuation, then continue with the next pending route from the resumed context.",
		"Do not use request_user_input when the destination is resolved from the site map.",
		"If the resolved target already matches the current page context, treat the current page context as successful page evidence, do not call navigate only to create proof, and answer or continue from the visible page context.",
		"Do not say a different page has been opened unless console-navigator/navigate succeeded in this turn or the current page context already matches the requested target. If the navigate tool fails, report that failure plainly.",
		"Navigation does not mutate user assets and must use only whitelisted internal /console routes.",
		"Console navigation JSON: " + string(encoded),
	}, "\n")
	return adapter.Message{Role: "system", Content: content}, true
}

func contextualConsoleAgentsSkillMessage(prepared *PreparedChat) (adapter.Message, bool) {
	return contextualConsoleAgentsSkillMessageForResolved(prepared, nil)
}

func contextualConsoleAgentsSkillMessageForResolved(prepared *PreparedChat, resolved *skills.ResolvedSkills) (adapter.Message, bool) {
	if prepared == nil || prepared.parts == nil {
		return adapter.Message{}, false
	}
	parts := prepared.parts
	if !skillIDEnabled(parts.SkillIDs, skills.SkillAgentManagement) ||
		!isConsoleAgentsContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		return adapter.Message{}, false
	}
	tools := filterAgentManagementPromptToolsForResolved(resolved, []map[string]string{
		{"skill_id": skills.SkillAgentManagement, "tool_name": "list_agents", "purpose": "list or search Agents when visible page context is missing or insufficient"},
		{"skill_id": skills.SkillAgentManagement, "tool_name": "get_agent", "purpose": "read one Agent's basic details"},
		{"skill_id": skills.SkillAgentManagement, "tool_name": "create_agent", "purpose": "create one draft AGENT in the current workspace"},
		{"skill_id": skills.SkillAgentManagement, "tool_name": "update_agent_identity", "purpose": "change one Agent's name, description, or icon"},
		{"skill_id": skills.SkillAgentManagement, "tool_name": "delete_agent", "purpose": "delete one resolved Agent after governance approval"},
		{"skill_id": skills.SkillAgentManagement, "tool_name": "delete_agents", "purpose": "delete multiple resolved visible/listed Agents as one governed frozen batch"},
		{"skill_id": skills.SkillAgentManagement, "tool_name": "get_agent_config", "purpose": "read one Agent's draft runtime config"},
		{"skill_id": skills.SkillAgentManagement, "tool_name": "update_agent_config", "purpose": "patch supported draft runtime config fields; use add/remove binding fields for specific skill/knowledge/database/workflow bind or unbind changes"},
		{"skill_id": skills.SkillAgentManagement, "tool_name": "replace_agent_memory_slots", "purpose": "replace one Agent's draft memory slot list"},
		{"skill_id": skills.SkillAgentManagement, "tool_name": "list_agent_knowledge_candidates", "purpose": "list knowledge bases in the target Agent workspace before binding"},
		{"skill_id": skills.SkillAgentManagement, "tool_name": "list_agent_database_candidates", "purpose": "list databases in the target Agent workspace before binding tables"},
		{"skill_id": skills.SkillAgentManagement, "tool_name": "list_agent_database_tables", "purpose": "list tables for one resolved database candidate before binding"},
		{"skill_id": skills.SkillAgentManagement, "tool_name": "list_agent_workflow_binding_candidates", "purpose": "list published workflow candidates in the target Agent workspace before binding"},
		{"skill_id": skills.SkillAgentManagement, "tool_name": "list_available_models", "purpose": "list user-available models by use_case before replacing an Agent model"},
	})
	navigationAllowed := agentManagementNavigationGuidanceAllowed(prepared)
	navigationToolAvailable := navigationAllowed && skillLoopResolvedToolAvailable(resolved, skills.SkillConsoleNavigator, "navigate")
	if navigationToolAvailable {
		tools = append(tools, map[string]string{"skill_id": skills.SkillConsoleNavigator, "tool_name": "navigate", "purpose": "open a resolved Agent detail href only when the goal still needs that page and current context is insufficient"})
	}
	listAgentsAvailable := agentManagementPromptToolListed(tools, skills.SkillAgentManagement, "list_agents")
	getAgentAvailable := agentManagementPromptToolListed(tools, skills.SkillAgentManagement, "get_agent")
	payload := map[string]interface{}{
		"page":                    "console.agents",
		"preferred_skill":         skills.SkillAgentManagement,
		"visible_agents":          consoleAgentsPromptVisibleAgents(parts),
		"tools":                   tools,
		"unsupported_in_this_mvp": []string{"publish_agent", "rollback_agent", "invoke_agent", "api_key", "webapp_online_offline"},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return adapter.Message{}, false
	}
	lines := []string{
		"ZGI Agent management guidance:",
		"Use agent-management for explicit Agent list, create, edit identity, delete, inspect draft config, update supported draft config, replace Agent memory slots, or edit knowledge/database/workflow bindings.",
		"Use the Agent capability model as the semantic contract for planning: skill-backed capabilities require a matching enabled Skill; file upload and memory are config switches; model changes require a provider/model pair; knowledge, database, and workflow access are binding changes.",
		"The tools array in Agent management JSON is the authoritative callable tool list for this turn. Do not call any agent-management or console-navigator tool that is absent from that tools array.",
		"When Agent management JSON includes visible_agents and the user refers to visible Agents, selected Agents, the current page, the current Agent, first-N/top-N Agents, or these Agents, treat visible_agents as authoritative resolved targets. Use their agent_id/name/href directly; do not call list_agents only to rediscover the same visible targets.",
		"For binding edits, read current config or list exact candidates only when the needed current binding set or candidate IDs/names are not already present in page context or prior tool results. Use one update_agent_config call with add_enabled_skill_ids/remove_enabled_skill_ids, add_knowledge_dataset_ids/remove_knowledge_dataset_ids, add_database_bindings/remove_database_bindings, add_workflow_bindings/remove_workflow_bindings, or full replacement fields when the user explicitly asks to replace or clear an entire binding section. Never pass resources the user wants to unbind as the final replacement list. Never guess resource IDs and never bind resources from another workspace.",
		"Do not publish, roll back, invoke Agents, manage API keys, or change WebApp online/offline state in this MVP. If the user asks for those, explain the limit.",
		"For edits, deletes, and config updates, resolve the Agent first and pass the exact agent_id. Do not invent IDs.",
		"When the user asks to create a new Agent, create_agent must succeed before any edit of that new Agent. If create_agent fails or is not allowed, do not update the currently visible Agent as a fallback; report the actual failure.",
		"Before changing an Agent model, call agent-management/list_available_models with the appropriate use_case, then pass the returned provider/model pair to update_agent_config.",
		"Do not call get_current_page_context; current page context is already injected into this turn as page evidence, not exposed as a callable tool.",
		"Tool governance owns approval for mutations. Do not ask for a separate natural-language confirmation when governance will pause the turn.",
		"For plural, range, first-N, selected, or page-list Agent deletion requests, call delete_agents once with an agents array containing the exact frozen targets and visible names. Do not loop delete_agent for batch deletion.",
		"After updating one current Agent detail page, verify from the mutation result and current page evidence. Do not call list_agents only to verify that same single Agent; if structured post-update state is still needed, use get_agent_config with the current agent_id.",
		"After ordinary edit, binding, unbinding, or Agent list-page batch deletion succeeds, prefer refreshed page context or asset observation over navigation. Do not navigate only to prove the mutation happened.",
		"Do not navigate after deleting Agents from the list page.",
	}
	if listAgentsAvailable {
		lines = append(lines, "Use list_agents only when visible_agents is missing or insufficient, the user asks to search/find Agents by name, asks what Agents exist beyond the visible page context, or gives a name without an exact visible match.")
	} else {
		lines = append(lines, "Because list_agents is absent from the tools array for this turn, do not call list_agents; use visible_agents, prior tool results, or an available read/config tool instead.")
	}
	if getAgentAvailable {
		lines = append(lines, "Use get_agent only for basic Agent details when page context or get_agent_config is insufficient.")
	} else {
		lines = append(lines, "Because get_agent is absent from the tools array for this turn, do not call get_agent; use get_agent_config or current page evidence for Agent details.")
	}
	lines = append(lines, "For read-only current Agent configuration checks, get_agent_config is enough for identity, model/provider, prompt, memory/file upload settings, and currently bound resource counts. Do not call candidate-list tools or list_agent_database_tables just to inspect current counts; use candidate tools only when the user asks what resources are available/bindable/selectable, or when a bind/unbind/replace operation needs exact resource IDs.")
	lines = append(lines, agentManagementCapabilityDefinitionGuidance()...)
	lines = append(lines, agentManagementActiveCapabilityGoalGuidance(prepared)...)
	lines = append(lines, agentManagementActiveConfigPlanGuidance(prepared)...)
	if navigationToolAvailable {
		lines = append(lines,
			"If the current operation plan includes console-navigator/navigate, treat it as a route hint: call it only when the user goal still needs that page and current context has not already satisfied the step.",
			"If delete_agent or delete_agents succeeds while the current route is a deleted Agent detail page and the operation plan requires returning to the list, call console-navigator/navigate with /console/agents before the final answer. Do not navigate after deleting Agents from the list page.",
		)
	} else {
		lines = append(lines,
			"Avoid console-navigator/navigate during Agent inspection, identity edit, or config edit turns unless the user explicitly asked to open another page or the current goal cannot proceed from current page evidence.",
			"Because console-navigator/navigate is not available in the tools JSON for this turn, do not say you need to navigate, open, enter, or switch pages. Continue from the current Agent page evidence.",
		)
	}
	lines = append(lines, "Agent management JSON: "+string(encoded))
	return adapter.Message{Role: "system", Content: strings.Join(lines, "\n")}, true
}

func filterAgentManagementPromptToolsForResolved(resolved *skills.ResolvedSkills, tools []map[string]string) []map[string]string {
	if resolved == nil {
		return tools
	}
	filtered := make([]map[string]string, 0, len(tools))
	for _, tool := range tools {
		skillID := strings.TrimSpace(tool["skill_id"])
		toolName := strings.TrimSpace(tool["tool_name"])
		if skillLoopResolvedToolAvailable(resolved, skillID, toolName) {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}

func agentManagementPromptToolListed(tools []map[string]string, skillID string, toolName string) bool {
	for _, tool := range tools {
		if strings.EqualFold(strings.TrimSpace(tool["skill_id"]), skillID) &&
			strings.EqualFold(strings.TrimSpace(tool["tool_name"]), toolName) {
			return true
		}
	}
	return false
}

func skillLoopResolvedToolAvailable(resolved *skills.ResolvedSkills, skillID string, toolName string) bool {
	if resolved == nil {
		return true
	}
	skillID = strings.TrimSpace(skillID)
	toolName = strings.TrimSpace(toolName)
	if skillID == "" || toolName == "" {
		return false
	}
	for _, doc := range resolved.Skills {
		if !strings.EqualFold(strings.TrimSpace(doc.Metadata.ID), skillID) {
			continue
		}
		for _, tool := range doc.Tools {
			if strings.EqualFold(strings.TrimSpace(tool.Name), toolName) {
				return true
			}
		}
		return false
	}
	return false
}

func agentManagementCapabilityDefinitionGuidance() []string {
	definitions := agentManagementCapabilityDefinitionsForPrompt()
	if len(definitions) == 0 {
		return nil
	}
	payload, err := json.Marshal(map[string]interface{}{
		"capabilities": definitions,
	})
	if err != nil {
		return nil
	}
	return []string{
		"Agent capability model JSON: " + string(payload),
		"Use these capability definitions before choosing tools. If a requested ability maps to a skill-backed capability, candidate lookup plus enabled_skill_ids evidence is required; do not treat prompt wording or file_upload_enabled as sufficient.",
	}
}

func agentManagementActiveCapabilityGoalGuidance(prepared *PreparedChat) []string {
	if prepared == nil || prepared.Message == nil {
		return nil
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if len(plan) == 0 {
		return nil
	}
	goals := operationPlanCompactCapabilityGoals(plan["capability_goals"], 6)
	if len(goals) == 0 {
		if structured := mapFromOperationContext(plan["structured_plan"]); len(structured) > 0 {
			goals = operationPlanCompactCapabilityGoals(structured["capability_goals"], 6)
		}
	}
	if len(goals) == 0 {
		return nil
	}
	payload, err := json.Marshal(map[string]interface{}{
		"capability_goals": goals,
	})
	if err != nil {
		return nil
	}
	return []string{
		"Active Agent capability goals JSON: " + string(payload),
		"Use capability goal meaning/enable_by/not_sufficient/verify_by as the current task contract: do not claim a capability is complete from a listed not_sufficient evidence source, and verify against the stated config fields or binding actions before the final answer.",
	}
}

func agentManagementActiveConfigPlanGuidance(prepared *PreparedChat) []string {
	if prepared == nil || prepared.Message == nil {
		return nil
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if len(plan) == 0 {
		return nil
	}
	var updateStep map[string]interface{}
	for _, step := range mapSliceFromAny(plan["steps"]) {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(step["skill_id"])), skills.SkillAgentManagement) ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(step["tool_name"])), "update_agent_config") {
			continue
		}
		updateStep = step
		break
	}
	if len(updateStep) == 0 {
		return nil
	}
	expectedFields := compactStringSliceForPrompt(
		stringSliceFromAny(updateStep[operationPlanExpectedUpdatedFieldsKey]),
		12,
		80,
	)
	expectedActions := operationPlanAgentConfigBindingActionsFromAny(updateStep[operationPlanExpectedBindingActionsKey])
	if len(expectedFields) == 0 && len(expectedActions) == 0 {
		return nil
	}
	lines := []string{}
	if len(expectedFields) > 0 {
		lines = append(lines, "Active Agent config plan expects update_agent_config to satisfy these fields before the final answer: "+strings.Join(expectedFields, ", ")+".")
	}
	if len(expectedActions) > 0 {
		actionPairs := agentManagementConfigBindingActionPairs(expectedActions)
		if len(actionPairs) > 0 {
			lines = append(lines, "Active Agent config plan expects these binding actions in the same user goal: "+strings.Join(actionPairs, ", ")+".")
		}
		removeArgs := agentManagementConfigRemoveArgsForExpectedUnbinds(expectedActions)
		if len(removeArgs) > 0 {
			lines = append(lines,
				"For expected unbind actions, use get_agent_config/current page evidence to build one update_agent_config call containing all applicable remove fields: "+strings.Join(removeArgs, ", ")+".",
				"Do not replace a governed mutation with a natural-language confirmation; call the governed update tool so the approval card freezes the actual operation.",
				"Do not finish after a partial binding edit. If any expected remove field cannot be constructed, report exactly which field lacks evidence instead of claiming completion.",
			)
		}
	}
	return lines
}

func agentManagementConfigBindingActionPairs(actions map[string]string) []string {
	if len(actions) == 0 {
		return nil
	}
	pairs := []string{}
	for _, field := range []string{"enabled_skill_ids", "knowledge_dataset_ids", "database_bindings", "workflow_bindings"} {
		action := operationPlanCanonicalAgentConfigBindingAction(actions[field])
		if action == "" {
			continue
		}
		pairs = append(pairs, field+"="+action)
	}
	return pairs
}

func agentManagementConfigRemoveArgsForExpectedUnbinds(actions map[string]string) []string {
	if len(actions) == 0 {
		return nil
	}
	argByField := map[string]string{
		"enabled_skill_ids":     "remove_enabled_skill_ids",
		"knowledge_dataset_ids": "remove_knowledge_dataset_ids",
		"database_bindings":     "remove_database_bindings",
		"workflow_bindings":     "remove_workflow_bindings",
	}
	args := []string{}
	for _, field := range []string{"enabled_skill_ids", "knowledge_dataset_ids", "database_bindings", "workflow_bindings"} {
		if operationPlanCanonicalAgentConfigBindingAction(actions[field]) != "unbind" {
			continue
		}
		if arg := argByField[field]; arg != "" {
			args = append(args, arg)
		}
	}
	return args
}

func agentManagementNavigationGuidanceAllowed(prepared *PreparedChat) bool {
	allowed := skillLoopAllowedPlannedTools(prepared)
	if skillLoopToolAllowed(allowed, skills.SkillConsoleNavigator, "navigate") {
		return true
	}
	if prepared == nil || prepared.parts == nil {
		return false
	}
	strategy := contextualAIChatTurnStrategyFromParts(prepared.parts)
	return strategy != nil && strategy.RouteRequired && strategy.RequiredNextTool != nil &&
		isConsoleNavigatorNavigateTool(strategy.RequiredNextTool.SkillID, strategy.RequiredNextTool.ToolName)
}

func contextualConsoleFilesSkillMessage(prepared *PreparedChat) (adapter.Message, bool) {
	if prepared == nil || prepared.parts == nil {
		return adapter.Message{}, false
	}
	parts := prepared.parts
	if !isConsoleFilesContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		return adapter.Message{}, false
	}
	fileReaderEnabled := skillIDEnabled(parts.SkillIDs, skills.SkillFileReader)
	fileManagerEnabled := skillIDEnabled(parts.SkillIDs, skills.SkillFileManager)
	fileGeneratorEnabled := skillIDEnabled(parts.SkillIDs, skills.SkillFileGenerator)
	chartGeneratorEnabled := skillIDEnabled(parts.SkillIDs, skills.SkillChartGenerator)
	artifactProducerEnabled := fileGeneratorEnabled || chartGeneratorEnabled
	if !fileReaderEnabled && !fileManagerEnabled && !artifactProducerEnabled {
		return adapter.Message{}, false
	}
	hasRead := fileReaderEnabled && hasConsoleFilesReadCapability(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext)
	hasDelete := fileManagerEnabled && hasConsoleFilesCapability(parts.RuntimeContext, consoleFilesDeleteCapabilityPattern, parts.RawOperationContext, parts.OperationContext)
	hasCreate := artifactProducerEnabled && fileManagerEnabled && hasConsoleFilesCreateCapability(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext)
	if !hasRead && !hasDelete && !hasCreate {
		return adapter.Message{}, false
	}

	payload := map[string]interface{}{
		"page":          "console.files",
		"visible_files": consoleFilesPromptVisibleFiles(parts),
	}
	if recentGeneratedFiles := consoleFilesPromptRecentGeneratedFiles(parts); len(recentGeneratedFiles) > 0 {
		payload["recent_generated_files"] = recentGeneratedFiles
	}
	preferredSkills := []string{}
	if hasRead {
		preferredSkills = append(preferredSkills, skills.SkillFileReader)
	}
	if hasDelete {
		preferredSkills = append(preferredSkills, skills.SkillFileManager)
	}
	if hasCreate {
		if fileGeneratorEnabled {
			preferredSkills = append(preferredSkills, skills.SkillFileGenerator)
		}
		if chartGeneratorEnabled {
			preferredSkills = append(preferredSkills, skills.SkillChartGenerator)
		}
		if !hasDelete {
			preferredSkills = append(preferredSkills, skills.SkillFileManager)
		}
	}
	payload["preferred_skills"] = preferredSkills
	if len(preferredSkills) == 1 {
		payload["preferred_skill"] = preferredSkills[0]
	}
	tools := make([]map[string]string, 0, 7)
	if hasRead {
		tools = append(tools, map[string]string{
			"capability_id": "file.list_visible",
			"skill_id":      skills.SkillFileReader,
			"tool_name":     "list_visible_files",
		})
		tools = append(tools, map[string]string{
			"capability_id": "file.read",
			"skill_id":      skills.SkillFileReader,
			"tool_name":     "read_file",
		})
	}
	if hasDelete {
		tools = append(tools, map[string]string{
			"capability_id": "file.delete",
			"skill_id":      skills.SkillFileManager,
			"tool_name":     "delete_file",
		})
	}
	if hasCreate {
		if fileGeneratorEnabled {
			for _, toolName := range []string{"generate_file", "generate_docx", "generate_pdf", "generate_pptx"} {
				tools = append(tools, map[string]string{
					"capability_id": "file.generate_temporary_artifact",
					"skill_id":      skills.SkillFileGenerator,
					"tool_name":     toolName,
				})
			}
		}
		if chartGeneratorEnabled {
			tools = append(tools, map[string]string{
				"capability_id": "file.generate_temporary_artifact",
				"skill_id":      skills.SkillChartGenerator,
				"tool_name":     "generate_chart",
			})
		}
		tools = append(tools, map[string]string{
			"capability_id": "file.create",
			"skill_id":      skills.SkillFileManager,
			"tool_name":     "save_file_to_management",
		})
	}
	payload["tools"] = tools
	if targets := consoleFilesPromptResolvedTargets(parts); len(targets) > 0 {
		payload["resolved_targets_from_user_request"] = targets
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return adapter.Message{}, false
	}

	lines := []string{
		"Contextual files-page tool guidance:",
		"The user is operating on the Console Files page. Treat visible file resources in operation_context as concrete user assets.",
		"Answer in the user's language. Use internal file and workspace identifiers only as tool arguments; do not mention internal IDs, UUIDs, workspace identifiers, raw JSON field names, or tool count fields in user-visible answers.",
		"When reporting file operation outcomes to a normal user, mention only the file name and the user-visible action result. For successful deletion, say that the named file was deleted; do not report raw counters or internal identifiers.",
		"When a file tool fails, explain the failure plainly in the user's language, do not claim success, and ask for the next safe step only when needed.",
		"For requests that only ask what files are visible, available, selected, or present on the Files page, answer directly from visible_files in the Files-page context JSON when it is present and sufficient. Use file-reader/list_visible_files only when that context is missing, ambiguous, or needs an authoritative refresh.",
		"For typed ordinal requests such as \"the second Excel file\", \"\u7b2c\u4e8c\u4e2a Excel\", or \"\u6700\u540e\u4e00\u4e2a PDF\", resolve among files of that type using file_type_rank or extension_rank. Do not treat \"second Excel\" as visible_index 2 unless that file is also the second Excel in visible_files.",
		"For requests about reading, previewing, summarizing, analyzing, or translating visible file contents, use file-reader/read_file with the resolved file_id.",
		"When resolved_targets_from_user_request is present, the target is already resolved from the current page. Use the listed file_id exactly for file-reader/read_file or file-manager/delete_file; it overrides any other ordinal or file-type interpretation.",
		"Do not ask the user to select a file, repeat the file name, or choose another visible file with the same type when a resolved target is present.",
		"After read_file returns content_status \"extracted\", answer from the returned content field and continue requested post-processing such as summary or translation. Do not say the file cannot be read.",
		"For requests about deleting or removing a resolved visible file, use file-manager/delete_file with exactly that file_id. Tool governance handles the approval card before deletion; do not ask for a separate natural-language confirmation first.",
		"If a prior approval or session grant exists, it only skips the approval prompt. You must still call file-manager/delete_file in this turn and wait for the tool result before saying the file was deleted.",
		"Never claim a file was deleted, removed, updated, created, saved, or otherwise changed based only on previous conversation context.",
		"If the target file is missing or ambiguous, call request_user_input with a concise clarification instead of guessing.",
		"For requests to create, generate, write, save, upload, import, or export a new file into File Management or the current Files page, use a two-step flow: first use the appropriate artifact-producing skill to create a temporary artifact, then use file-manager/save_file_to_management with source_type \"tool_file\", the generated tool_file_id/file_id, and the destination filename.",
		"Use file-generator for regular files, documents, generic SVG/vector files, PDFs, DOCX, PPTX, XLSX, CSV, JSON, Markdown, HTML, or TXT. Use chart-generator only when the user explicitly asks for a chart, graph, data visualization, or a supported chart type.",
		"When the user says this file, the previous file, the generated file, or the file just created and asks to save/upload/import it into File Management, resolve that reference from recent_generated_files before considering visible_files. Use the listed tool_file_id only as a tool argument.",
		"Do not treat a visible File Management asset as the same file as a recent temporary generated artifact unless the filenames and requested action make that explicit.",
		"For requests to save or import a public external URL into File Management, use file-manager/save_file_to_management with source_type \"url\" and the destination filename.",
		"For generated or downloadable files without an explicit File Management, current Files page, save, create, or upload target, keep the default temporary artifact behavior and do not call file-manager/save_file_to_management.",
		"Creating a File Management file is a governed file.create operation owned by file-manager/save_file_to_management. Tool governance handles the approval card when the permission tier requires it; do not ask for a separate natural-language confirmation first.",
		"Do not call unrelated discovery or domain tools, such as database, knowledge, or calculator, before completing the requested files-page operation.",
		"For existing-file read/delete operations, do not call file-generation tools before the requested read/delete is completed.",
		"Files-page context JSON: " + string(encoded),
	}
	if hint := consoleFilesGuardTargetArgumentHint(consoleFilesPromptResolvedTargets(prepared.parts), ""); hint != "" {
		lines = append(lines, hint)
	}
	content := strings.Join(lines, "\n")
	return adapter.Message{Role: "system", Content: content}, true
}

func skillLoopFinalAnswerGuard(prepared *PreparedChat) skillloop.FinalAnswerGuard {
	if prepared == nil || prepared.parts == nil {
		return nil
	}
	metadata := map[string]interface{}(nil)
	if prepared.Message != nil {
		metadata = prepared.Message.Metadata
	}
	guards := make([]skillloop.FinalAnswerGuard, 0, 5)
	if prepared.Message != nil && operationPlanModelDecidesTools(mapFromOperationContext(prepared.Message.Metadata["operation_plan"])) {
		return nil
	}
	if guard := skillLoopConsoleNavigationFinalAnswerGuard(prepared); guard != nil {
		guards = append(guards, guard)
	}
	if guard := skillLoopAgentManagementFinalAnswerGuard(prepared); guard != nil {
		guards = append(guards, guard)
	}
	if guard := skillLoopTemporaryFileGenerateFinalAnswerGuard(prepared); guard != nil {
		guards = append(guards, guard)
	}
	if guard := consoleFilesContinuationPendingDeleteFinalAnswerGuard(prepared.parts, metadata); guard != nil {
		guards = append(guards, guard)
	}
	if guard := skillLoopConsoleFilesFinalAnswerGuard(prepared); guard != nil {
		guards = append(guards, guard)
	}
	return combineFinalAnswerGuards(guards...)
}

func skillLoopToolCallGuard(prepared *PreparedChat) skillloop.ToolCallGuard {
	if prepared == nil || prepared.parts == nil {
		return nil
	}
	if prepared.Message != nil && operationPlanModelDecidesTools(mapFromOperationContext(prepared.Message.Metadata["operation_plan"])) {
		return nil
	}
	parts := prepared.parts
	executionParts, stagedCurrent, _ := stagedExecutionScopedParts(parts)
	if executionParts == nil {
		executionParts = parts
	}
	metadata := map[string]interface{}(nil)
	if prepared.Message != nil {
		metadata = prepared.Message.Metadata
	}
	managedFileCreateIntent := isManagedFileCreateIntent(executionParts.Query)
	managedFileContinuationFlow := managedFileCreateContinuationSaveFlowActive(parts, metadata)
	continuationIntent := isContinuationIntent(parts.Query)
	continuationGeneratedAssetFlow := continuationIntent && generatedFileMetadataHasAnyArtifact(metadata)
	continuationFileManagerFlow := continuationIntent && skillIDEnabled(parts.SkillIDs, skills.SkillFileManager)
	if !managedFileCreateIntent && !managedFileContinuationFlow && !continuationGeneratedAssetFlow && !continuationFileManagerFlow &&
		!isConsoleNavigationIntent(executionParts.Query) && !stagedCurrent {
		return nil
	}
	return func(req skillloop.ToolCallGuardRequest) (skillloop.FinalAnswerGuardResult, bool) {
		if stagedCurrent {
			if result, blocked := stagedContinuationToolCallGuardResult(executionParts, req); blocked {
				return result, true
			}
		}
		if continuationFileManagerFlow && isFileManagerDeleteToolCall(req.SkillID, req.ToolName) &&
			(finalAnswerGuardHasToolForTargets(req.SuccessfulToolCalls, skills.SkillFileManager, "delete_file", nil) ||
				len(successfulMetadataToolCalls(metadata, skills.SkillFileManager, "delete_file")) > 0) {
			successfulDeletes := append(successfulMetadataToolCalls(metadata, skills.SkillFileManager, "delete_file"), matchingSkillToolCalls(req.SuccessfulToolCalls, skills.SkillFileManager, "delete_file")...)
			return consoleFilesDeleteAlreadySucceededGuardResult(successfulDeletes), true
		}
		if continuationGeneratedAssetFlow {
			if isKnownArtifactGeneratorToolCall(req.SkillID, req.ToolName) {
				if saveArgs := latestUnsavedGeneratedArtifactSaveArgumentsFromMetadata(metadata, req.SuccessfulToolCalls); len(saveArgs) > 0 {
					return fileManagerSaveRequiredToolGuardResult(saveArgs), true
				}
				if saveArgs := latestUnsavedGeneratedArtifactSaveArguments(req.SuccessfulToolCalls); len(saveArgs) > 0 {
					return fileManagerSaveRequiredToolGuardResult(saveArgs), true
				}
				if generatedFileMetadataArtifactsAlreadySaved(metadata, req.SuccessfulToolCalls) {
					return continuationGeneratedFilesAlreadySatisfiedGuardResult(), true
				}
			}
			if isFileManagerSaveToolCall(req.SkillID, req.ToolName) &&
				len(pendingGeneratedArtifactSaveArgumentCandidates(executionParts, metadata, req.SuccessfulToolCalls)) == 0 &&
				generatedFileMetadataArtifactsAlreadySaved(metadata, req.SuccessfulToolCalls) {
				return continuationGeneratedFilesAlreadySatisfiedGuardResult(), true
			}
		}
		if isConsoleNavigatorNavigateTool(req.SkillID, req.ToolName) {
			if target, ok := resolveConsoleNavigationTargetForParts(executionParts); ok &&
				clientActionContinuationLoadedRoute(executionParts, target.Href) &&
				consoleNavigationLoadedHrefMatchesTarget(skillToolCallArgumentString(req.Arguments, "href"), target.Href) {
				return skillloop.FinalAnswerGuardResult{
					SkillID:  skills.SkillConsoleNavigator,
					ToolName: "navigate",
					Message: strings.Join([]string{
						"The requested console page is already loaded by the previous client navigation action.",
						"Do not navigate to the same page again; continue from the current page context.",
					}, " "),
					SystemMessage: strings.Join([]string{
						"The route navigation client action has already completed successfully for this request.",
						"Do not call console-navigator/navigate again for the same href.",
						"Continue with any remaining page operation or provide the final answer from the loaded page context.",
					}, " "),
				}, true
			}
		}
		if requiresConsoleFilesRouteBeforeManagedFileCreate(executionParts) {
			target := consoleFilesRouteHint()
			if isConsoleNavigatorNavigateTool(req.SkillID, req.ToolName) &&
				consoleNavigationLoadedHrefMatchesTarget(skillToolCallArgumentString(req.Arguments, "href"), target.Href) {
				return skillloop.FinalAnswerGuardResult{}, false
			}
			if isConsoleNavigatorNavigateTool(req.SkillID, req.ToolName) {
				result := consoleNavigationRequiredToolGuardResult(target)
				result.Message = strings.Join([]string{
					"The user asked to create or save a file in File Management from another console page.",
					"Route to the Files page only when page context is still needed; do not navigate to a different page for this save.",
				}, " ")
				if result.SystemMessage != "" {
					result.SystemMessage = result.Message + " " + result.SystemMessage
				}
				return result, true
			}
		}

		if managedFileCreateIntent || managedFileContinuationFlow {
			if isFileManagerDeleteToolCall(req.SkillID, req.ToolName) {
				pendingSaveArgs := pendingGeneratedArtifactSaveArgumentCandidates(executionParts, metadata, req.SuccessfulToolCalls)
				if len(pendingSaveArgs) > 0 {
					result := fileManagerSaveRequiredToolGuardResult(pendingSaveArgs[0])
					prefix := "A generated temporary artifact is still not saved to File Management, so deletion cannot run yet."
					result.Message = prefix + " " + result.Message
					result.SystemMessage = prefix + " Save every requested generated artifact before delete_file or any destructive follow-up step. " + result.SystemMessage
					return result, true
				}
			}
			if isConsoleNavigatorNavigateTool(req.SkillID, req.ToolName) &&
				managedFileCreateShouldBlockReturningToCompletedRoute(metadata, skillToolCallArgumentString(req.Arguments, "href")) {
				message := strings.Join([]string{
					"The requested route was already loaded and observed earlier in this same File Management creation task.",
					"The Files page has also been loaded, so do not navigate back to a completed precursor page or restart earlier steps.",
					"Use the completed client action context already provided for that page and continue the file generation/save flow from the current Files page.",
				}, " ")
				return skillloop.FinalAnswerGuardResult{
					SkillID:       skills.SkillFileGenerator,
					ToolName:      "generate_file",
					Message:       message,
					SystemMessage: message,
				}, true
			}
			if isConsoleNavigatorNavigateTool(req.SkillID, req.ToolName) &&
				consoleNavigationLoadedHrefMatchesTarget(skillToolCallArgumentString(req.Arguments, "href"), consoleFilesRouteHint().Href) &&
				(consoleFilesRouteAlreadyAvailable(executionParts) || clientActionContinuationLoadedRoute(executionParts, consoleFilesRouteHint().Href)) {
				return skillloop.FinalAnswerGuardResult{
					SkillID:  skills.SkillFileGenerator,
					ToolName: "generate_file",
					Message: strings.Join([]string{
						"The Files page is already loaded for this request.",
						"Do not navigate to the same Files page again; continue the file creation flow from the current page context.",
					}, " "),
					SystemMessage: strings.Join([]string{
						"The Files page is already loaded for this File Management creation request.",
						"Do not call console-navigator/navigate again.",
						"Generate the requested temporary artifact with the appropriate artifact-producing skill if none exists, then call file-manager/save_file_to_management.",
					}, " "),
				}, true
			}
			if isChartGeneratorToolCall(req.SkillID, req.ToolName) && !shouldPreferChartArtifactProducer(executionParts) {
				message := strings.Join([]string{
					"The user asked to create a regular SVG or file in File Management, not a chart or data visualization.",
					"Use file-generator/generate_file for generic SVG/vector file creation, then save the generated artifact with file-manager/save_file_to_management.",
				}, " ")
				return skillloop.FinalAnswerGuardResult{
					SkillID:       skills.SkillFileGenerator,
					ToolName:      "generate_file",
					Message:       message,
					SystemMessage: message,
				}, true
			}
			if isFileManagerSaveToolCall(req.SkillID, req.ToolName) {
				pendingSaveArgs := pendingGeneratedArtifactSaveArgumentCandidates(executionParts, metadata, req.SuccessfulToolCalls)
				if len(pendingSaveArgs) > 0 && !fileManagerSaveArgumentsMatchAnyPendingArtifact(req.Arguments, pendingSaveArgs) {
					saveArgs := pendingGeneratedArtifactSaveArgumentsForAttempt(req.Arguments, pendingSaveArgs)
					result := fileManagerSaveRequiredToolGuardResult(saveArgs)
					prefix := "The attempted file-manager/save_file_to_management arguments do not match any pending generated temporary artifact."
					result.Message = prefix + " " + result.Message
					result.SystemMessage = prefix + " Use the resolved generated-file save JSON exactly; do not substitute a managed file id, a previous save result id, or a different filename. " + result.SystemMessage
					return result, true
				}
			}
			if isKnownArtifactGeneratorToolCall(req.SkillID, req.ToolName) {
				if managedFileContinuationFlow {
					if saveArgs := latestUnsavedGeneratedArtifactSaveArgumentsFromMetadata(metadata, req.SuccessfulToolCalls); len(saveArgs) > 0 {
						return fileManagerSaveRequiredToolGuardResult(saveArgs), true
					}
				}
				if saveArgs := latestUnsavedGeneratedArtifactSaveArgumentsForTargetsFromMetadata(
					metadata,
					managedFileCreateMissingSaveTargets(executionParts, metadata, req.SuccessfulToolCalls),
				); len(saveArgs) > 0 {
					return fileManagerSaveRequiredToolGuardResult(saveArgs), true
				}
				if saveArgs := latestUnsavedGeneratedArtifactSaveArguments(req.SuccessfulToolCalls); len(saveArgs) > 0 {
					if allowAdditionalRequestedManagedFileGeneration(executionParts, req, saveArgs) {
						return skillloop.FinalAnswerGuardResult{}, false
					}
					return fileManagerSaveRequiredToolGuardResult(saveArgs), true
				}
				if saveArgs := latestRecentGeneratedArtifactSaveArguments(executionParts); len(saveArgs) > 0 {
					return fileManagerSaveRequiredToolGuardResult(saveArgs), true
				}
			}
		}
		return skillloop.FinalAnswerGuardResult{}, false
	}
}

func skillLoopConsoleFilesFinalAnswerGuard(prepared *PreparedChat) skillloop.FinalAnswerGuard {
	if prepared == nil || prepared.parts == nil {
		return nil
	}
	parts := prepared.parts
	if executionParts, _, _ := stagedExecutionScopedParts(parts); executionParts != nil {
		parts = executionParts
	}
	metadata := map[string]interface{}(nil)
	if prepared.Message != nil {
		metadata = prepared.Message.Metadata
	}
	isFilesContext := isConsoleFilesContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext)
	managedCreateGuardActive := isManagedFileCreateIntent(parts.Query) || managedFileCreateContinuationSaveFlowActive(prepared.parts, metadata)
	if !isFilesContext && !managedCreateGuardActive {
		return nil
	}
	if skillIDEnabled(parts.SkillIDs, skills.SkillFileManager) &&
		(skillIDEnabled(parts.SkillIDs, skills.SkillFileGenerator) || skillIDEnabled(parts.SkillIDs, skills.SkillChartGenerator)) &&
		managedCreateGuardActive {
		createGuard := consoleFilesFileManagementCreateFinalAnswerGuard(parts, metadata)
		if isFilesContext {
			return combineFinalAnswerGuards(createGuard, consoleFilesContinuationPendingDeleteFinalAnswerGuard(parts, metadata))
		}
		return createGuard
	}
	if !isFilesContext {
		return nil
	}
	if guard := consoleFilesContinuationPendingDeleteFinalAnswerGuard(parts, metadata); guard != nil {
		return guard
	}
	if shouldSkipConsoleFilesFinalAnswerGuardForNavigationObservation(prepared, parts, metadata) {
		return nil
	}
	targets := consoleFilesPromptResolvedTargets(parts)
	if len(targets) == 0 {
		return nil
	}
	if hasConsoleFilesCapability(parts.RuntimeContext, consoleFilesDeleteCapabilityPattern, parts.RawOperationContext, parts.OperationContext) &&
		isFileDeleteIntent(parts.Query) {
		if !skillIDEnabled(parts.SkillIDs, skills.SkillFileManager) {
			return nil
		}
		return consoleFilesRequiredToolFinalAnswerGuard(skills.SkillFileManager, targets, "delete_file", []string{
			"The user's current files-page request is a concrete file deletion request for {target}.",
			"Do not finish with a natural-language success message yet.",
			"Load the file-manager skill if needed, then call call_skill_tool with skill_id \"file-manager\", tool_name \"delete_file\", and the resolved file_id for the target file.",
			"A session approval grant may skip the approval card, but it does not replace the delete_file tool call.",
			"Only after delete_file succeeds in this turn may you tell the user that the file was deleted. If the tool fails or the file is already missing, report the actual tool result.",
		})
	}
	if hasConsoleFilesReadCapability(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) &&
		isFileReadIntent(parts.Query) {
		if !skillIDEnabled(parts.SkillIDs, skills.SkillFileReader) {
			return nil
		}
		return consoleFilesRequiredToolFinalAnswerGuard(skills.SkillFileReader, targets, "read_file", []string{
			"The user's current files-page request requires reading the actual content of {target}.",
			"Do not finish from visible page metadata, file names, or prior conversation context.",
			"Load the file-reader skill if needed, then call call_skill_tool with skill_id \"file-reader\", tool_name \"read_file\", and the resolved file_id for the target file.",
			"Only after read_file succeeds in this turn may you summarize, translate, quote, or answer from the file content. If the tool fails or returns empty content, report the actual tool result.",
		})
	}
	return nil
}

func shouldSkipConsoleFilesFinalAnswerGuardForNavigationObservation(prepared *PreparedChat, parts *chatRequestParts, metadata map[string]interface{}) bool {
	if parts == nil || !isConsoleNavigationIntent(parts.Query) {
		return false
	}
	if isManagedFileCreateIntent(parts.Query) || isFileDeleteIntent(parts.Query) || consoleFilesQueryHasExplicitFileReadTarget(parts) {
		return false
	}
	if completedConsoleNavigationOperationPlan(metadata) {
		return true
	}
	if prepared != nil {
		if strategy := contextualAIChatTurnStrategy(prepared); strategy != nil && strings.EqualFold(strategy.Intent, "navigate_console_page") {
			return true
		}
	}
	return false
}

func completedConsoleNavigationOperationPlan(metadata map[string]interface{}) bool {
	plan := mapFromOperationContext(metadata["operation_plan"])
	if len(plan) == 0 {
		return false
	}
	intent := strings.TrimSpace(stringFromAny(plan["intent"]))
	if !strings.EqualFold(intent, "navigate_console_page") && !strings.EqualFold(intent, "continue_navigate_console_page") {
		return false
	}
	return !operationPlanHasIncompleteWork(plan)
}

func consoleFilesQueryHasExplicitFileReadTarget(parts *chatRequestParts) bool {
	if parts == nil {
		return false
	}
	query := strings.TrimSpace(parts.Query)
	text := strings.ToLower(query)
	if text == "" {
		return false
	}
	if consoleFilesSelectedReferenceFromQuery(query) || consoleFilesRecentReferenceFromQuery(query) {
		return true
	}
	if len(plannerResourceRefsFromNamedVisibleFiles(parts)) > 0 {
		return true
	}
	targetText := consoleFilesQueryFileTargetText(text)
	targetText = consoleFilesQueryWithoutFileMutationNegations(targetText)
	if consoleFilesQueryReadsNonFileResource(targetText) {
		return false
	}
	if _, _, ok := consoleFilesOrdinalFromQuery(query); ok {
		if strings.Contains(targetText, "\u8bfb\u53d6") || strings.Contains(targetText, "\u8bfb") ||
			containsAnySubstring(targetText, []string{"read", "file", "\u6587\u4ef6", "\u5185\u5bb9", "pdf", "excel", "\u8868\u683c", "\u6587\u6863"}) {
			return true
		}
	}
	return containsAnySubstring(targetText, []string{
		"read file",
		"read the file",
		"file content",
		"contents of",
		"summarize file",
		"summarise file",
		"translate file",
		"preview file",
		"analyze file",
		"analyse file",
		".pdf",
		".xlsx",
		".xls",
		".csv",
		".docx",
		".pptx",
		".md",
		".txt",
		".svg",
		"读文件",
		"读取文件",
		"总结文件",
		"摘要文件",
		"翻译文件",
		"分析文件",
		"解释文件",
		"预览文件",
		"查看文件内容",
		"文件内容",
		"这个文件",
		"该文件",
	})
}

func consoleFilesQueryFileTargetText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return ""
	}
	for _, token := range []string{
		"file management",
		"files page",
		"current files page",
		"/console/files",
		"\u6587\u4ef6\u7ba1\u7406",
		"\u6587\u4ef6\u9875",
		"\u5f53\u524d\u6587\u4ef6\u9875",
		"\u6587\u4ef6\u6a21\u5757",
	} {
		text = strings.ReplaceAll(text, token, "")
	}
	return text
}

func consoleFilesQueryWithoutFileMutationNegations(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	for _, token := range []string{
		"\u4e0d\u8981\u521b\u5efa\u6587\u4ef6",
		"\u4e0d\u8981\u5220\u9664\u6587\u4ef6",
		"\u4e0d\u8981\u521b\u5efa\u6216\u5220\u9664\u6587\u4ef6",
		"\u4e0d\u8981\u521b\u5efa\u3001\u5220\u9664\u6587\u4ef6",
		"\u4e0d\u9700\u8981\u521b\u5efa\u6587\u4ef6",
		"\u4e0d\u9700\u8981\u5220\u9664\u6587\u4ef6",
		"do not create files",
		"do not delete files",
		"do not create or delete files",
		"don't create files",
		"don't delete files",
		"without creating files",
		"without deleting files",
	} {
		text = strings.ReplaceAll(text, token, "")
	}
	return strings.TrimSpace(text)
}

func consoleFilesQueryReadsNonFileResource(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return false
	}
	if !containsAnySubstring(text, []string{
		"\u8bfb",
		"read",
		"summarize",
		"summarise",
		"\u603b\u7ed3",
		"\u6458\u8981",
	}) {
		return false
	}
	if containsAnySubstring(text, []string{
		"file",
		"\u6587\u4ef6",
		"\u5185\u5bb9",
		".pdf",
		".xlsx",
		".xls",
		".csv",
		".docx",
		".pptx",
		".md",
		".txt",
		".svg",
		"pdf",
		"excel",
	}) {
		return false
	}
	return containsAnySubstring(text, []string{
		"agent",
		"\u667a\u80fd\u4f53",
		"\u6570\u636e\u5e93",
		"database",
		"\u5de5\u4f5c\u6d41",
		"workflow",
		"\u77e5\u8bc6\u5e93",
		"knowledge base",
		"\u5e94\u7528",
		"app",
	})
}

func skillLoopTemporaryFileGenerateFinalAnswerGuard(prepared *PreparedChat) skillloop.FinalAnswerGuard {
	if prepared == nil || prepared.parts == nil {
		return nil
	}
	parts := prepared.parts
	if executionParts, _, _ := stagedExecutionScopedParts(parts); executionParts != nil {
		parts = executionParts
	}
	if !modelTurnIntentRequestsTemporaryFileArtifact(parts.ModelTurnIntent) {
		return nil
	}
	skillID, toolName := temporaryFileGenerateRequiredTool(parts)
	if skillID == "" || toolName == "" {
		return nil
	}
	metadata := map[string]interface{}(nil)
	if prepared.Message != nil {
		metadata = prepared.Message.Metadata
	}
	return func(req skillloop.FinalAnswerGuardRequest) (skillloop.FinalAnswerGuardResult, bool) {
		if generatedFileMetadataHasAnyArtifact(metadata) || finalAnswerGuardHasSuccessfulArtifactProducerTool(req) {
			return skillloop.FinalAnswerGuardResult{}, false
		}
		if finalAnswerGuardHasAttemptedArtifactProducerTool(req) {
			return skillloop.FinalAnswerGuardResult{}, false
		}
		return skillloop.FinalAnswerGuardResult{
			SkillID:  skillID,
			ToolName: toolName,
			Message: strings.Join([]string{
				"The user's current request asks to generate a temporary file artifact.",
				"Do not finish by saying the file was generated before an artifact-producing tool succeeds.",
				"Call the requested artifact-producing tool and keep the result temporary unless the user explicitly asks to save it to File Management.",
			}, " "),
			SystemMessage: strings.Join([]string{
				"The candidate final answer is premature for this temporary file generation request.",
				"Load the selected skill if needed, then call call_skill_tool with skill_id \"" + skillID + "\" and tool_name \"" + toolName + "\" when generation is still needed.",
				"Use the requested filename, format, and content. Keep lifecycle/target temporary and do not call file-manager/save_file_to_management.",
				"Only after the artifact-producing tool succeeds may you tell the user the temporary file was generated.",
			}, " "),
		}, true
	}
}

func modelTurnIntentRequestsTemporaryFileArtifact(intent *AIChatModelTurnIntent) bool {
	if intent == nil {
		return false
	}
	if normalizeModelTurnIntent(intent.Intent) == "generate_temporary_file_artifact" {
		return true
	}
	return modelTurnIntentHasRecommendedCapability(
		intent,
		"generated_artifact",
		"chart_artifact",
		"data_visualization_artifact",
		"visualization_artifact",
		"file_artifact",
		"document_artifact",
		"svg_artifact",
		"text_artifact",
		"pdf_artifact",
		"spreadsheet_artifact",
	)
}

func temporaryFileGenerateRequiredTool(parts *chatRequestParts) (string, string) {
	if parts == nil {
		return "", ""
	}
	if shouldPreferChartArtifactProducer(parts) && skillIDEnabled(parts.SkillIDs, skills.SkillChartGenerator) {
		return skills.SkillChartGenerator, "generate_chart"
	}
	if skillIDEnabled(parts.SkillIDs, skills.SkillFileGenerator) {
		return skills.SkillFileGenerator, "generate_file"
	}
	if skillIDEnabled(parts.SkillIDs, skills.SkillChartGenerator) {
		return skills.SkillChartGenerator, "generate_chart"
	}
	return "", ""
}

func shouldPreferChartArtifactProducer(parts *chatRequestParts) bool {
	if parts == nil {
		return false
	}
	if parts.ModelTurnIntent != nil {
		switch {
		case modelTurnIntentHasRecommendedCapability(parts.ModelTurnIntent, "chart_artifact", "data_visualization_artifact", "visualization_artifact"):
			return true
		case modelTurnIntentHasRecommendedCapability(parts.ModelTurnIntent, "file_artifact", "document_artifact", "svg_artifact", "text_artifact", "pdf_artifact", "spreadsheet_artifact"):
			return false
		}
	}
	return false
}

func modelTurnIntentHasRecommendedCapability(intent *AIChatModelTurnIntent, values ...string) bool {
	if intent == nil || len(values) == 0 {
		return false
	}
	want := map[string]struct{}{}
	for _, value := range values {
		if canonical := canonicalModelTurnCapabilityHint(value); canonical != "" {
			want[canonical] = struct{}{}
		}
	}
	if len(want) == 0 {
		return false
	}
	for _, capability := range intent.RecommendedCapabilities {
		canonical := canonicalModelTurnCapabilityHint(capability)
		if _, ok := want[canonical]; ok {
			return true
		}
	}
	return false
}

func canonicalModelTurnCapabilityHint(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer("-", "_", " ", "_").Replace(value)
	return value
}

func finalAnswerGuardHasSuccessfulArtifactProducerTool(req skillloop.FinalAnswerGuardRequest) bool {
	for _, call := range req.SuccessfulToolCalls {
		if isKnownArtifactGeneratorToolCall(call.SkillID, call.ToolName) {
			return true
		}
	}
	return false
}

func finalAnswerGuardHasAttemptedArtifactProducerTool(req skillloop.FinalAnswerGuardRequest) bool {
	for _, call := range req.AttemptedToolCalls {
		if isKnownArtifactGeneratorToolCall(call.SkillID, call.ToolName) {
			return true
		}
	}
	return false
}

func consoleFilesFileManagementCreateFinalAnswerGuard(parts *chatRequestParts, metadata map[string]interface{}) skillloop.FinalAnswerGuard {
	return func(req skillloop.FinalAnswerGuardRequest) (skillloop.FinalAnswerGuardResult, bool) {
		metadataSaveCalls := managedFileSaveCallsFromGeneratedFilesMetadata(metadata)
		successfulSaveCalls := append([]skillloop.SkillToolCallRef{}, metadataSaveCalls...)
		successfulSaveCalls = append(successfulSaveCalls, req.SuccessfulToolCalls...)
		missingTargets := managedFileCreateMissingSaveTargets(parts, metadata, req.SuccessfulToolCalls)
		hasSuccessfulSave := finalAnswerGuardHasSuccessfulFileManagerSaveTool(req) || len(metadataSaveCalls) > 0
		if len(missingTargets) == 0 && hasSuccessfulSave {
			return skillloop.FinalAnswerGuardResult{}, false
		}
		if len(missingTargets) > 0 && hasSuccessfulSave {
			message := strings.Join([]string{
				"The user's current files-page request asks to create or save multiple files into File Management.",
				"The following requested file targets have not been saved yet: " + strings.Join(missingTargets, ", ") + ".",
				"Do not say all requested files were created or saved.",
				"Continue the missing file generation/save flow and only report success after file-manager/save_file_to_management succeeds for each target.",
			}, " ")
			systemMessage := message
			if saveArgs := latestUnsavedGeneratedArtifactSaveArgumentsForTargetsFromMetadata(metadata, missingTargets); len(saveArgs) > 0 {
				if encoded, err := json.Marshal(map[string]interface{}{
					"skill_id":  skills.SkillFileManager,
					"tool_name": "save_file_to_management",
					"arguments": saveArgs,
				}); err == nil {
					systemMessage = strings.Join([]string{
						message,
						"An unsaved generated artifact for a missing requested target already exists.",
						"Do not regenerate it; call file-manager/save_file_to_management with the resolved arguments JSON below.",
						"Resolved generated-file save JSON for tool arguments only; do not reveal internal IDs to the user: " + string(encoded),
					}, " ")
				}
			}
			return skillloop.FinalAnswerGuardResult{
				SkillID:       skills.SkillFileManager,
				ToolName:      "save_file_to_management",
				Message:       message,
				SystemMessage: systemMessage,
			}, true
		}
		if finalAnswerGuardHasAttemptedFileManagerSaveTool(req) {
			return skillloop.FinalAnswerGuardResult{}, false
		}
		messageLines := []string{
			"The user's current files-page request explicitly asks to create or save a new file into File Management or the current Files page.",
			"Do not finish by saying this is unsupported.",
			"Load the appropriate artifact-producing skill and file-manager if needed. For each destination file, create one temporary artifact, then call file-manager/save_file_to_management with source_type \"tool_file\", the generated tool_file_id/file_id, and that destination filename.",
			"Use file-generator for normal files and generic SVG/vector files. Use chart-generator only when the user explicitly asks for a chart, graph, data visualization, or a supported chart type.",
			"Keep generated files temporary only when the user did not explicitly ask for File Management, current Files page, save, create, upload, or import as the target.",
			"Only after file-manager/save_file_to_management succeeds may you say the File Management file was created. If approval is required, wait for tool governance instead of asking for a separate natural-language confirmation.",
		}
		systemLines := append([]string{}, messageLines...)
		saveArgs := latestGeneratedArtifactSaveArguments(req.SuccessfulToolCalls)
		saveSourceMessage := "A temporary artifact has already been generated in this turn. Do not generate another file for the same request."
		if len(saveArgs) == 0 {
			saveArgs = latestRecentGeneratedArtifactSaveArguments(parts)
			if len(saveArgs) > 0 {
				saveSourceMessage = "The user is referring to a recent generated/downloadable file from the conversation. Do not generate another file or substitute a visible File Management asset for it."
			}
		}
		if len(saveArgs) > 0 {
			systemLines = append(systemLines,
				saveSourceMessage,
				"Load file-manager if needed, then call call_skill_tool with skill_id \"file-manager\", tool_name \"save_file_to_management\", and the resolved arguments JSON below.",
			)
			if encoded, err := json.Marshal(map[string]interface{}{
				"skill_id":  skills.SkillFileManager,
				"tool_name": "save_file_to_management",
				"arguments": saveArgs,
			}); err == nil {
				systemLines = append(systemLines, "Resolved generated-file save JSON for tool arguments only; do not reveal internal IDs to the user: "+string(encoded))
			}
		}
		message := strings.Join(messageLines, " ")
		return skillloop.FinalAnswerGuardResult{
			SkillID:       skills.SkillFileManager,
			ToolName:      "save_file_to_management",
			Message:       message,
			SystemMessage: strings.Join(systemLines, " "),
		}, true
	}
}

func fileManagerSaveRequiredToolGuardResult(saveArgs map[string]interface{}) skillloop.FinalAnswerGuardResult {
	messageLines := []string{
		"A temporary artifact has already been generated for this File Management creation request.",
		"Do not generate another file for the same request.",
		"Call file-manager/save_file_to_management with the generated temporary artifact.",
	}
	systemLines := append([]string{}, messageLines...)
	if len(saveArgs) > 0 {
		if encoded, err := json.Marshal(map[string]interface{}{
			"skill_id":  skills.SkillFileManager,
			"tool_name": "save_file_to_management",
			"arguments": saveArgs,
		}); err == nil {
			systemLines = append(systemLines, "Resolved generated-file save JSON for tool arguments only; do not reveal internal IDs to the user: "+string(encoded))
		}
	}
	return skillloop.FinalAnswerGuardResult{
		SkillID:       skills.SkillFileManager,
		ToolName:      "save_file_to_management",
		Message:       strings.Join(messageLines, " "),
		SystemMessage: strings.Join(systemLines, " "),
	}
}

func continuationGeneratedFilesAlreadySatisfiedGuardResult() skillloop.FinalAnswerGuardResult {
	message := strings.Join([]string{
		"The continuation already has generated file artifacts recorded and no unsaved generated artifact remains.",
		"Do not generate or save another file for the same continuation step.",
		"Continue with the next planned non-generation action, such as refreshing visible files or deleting the frozen target, or provide the final answer if all steps are complete.",
	}, " ")
	return skillloop.FinalAnswerGuardResult{
		SkillID:       skills.SkillFileGenerator,
		ToolName:      "generate_file",
		Message:       message,
		SystemMessage: message,
	}
}

func skillLoopConsoleNavigationFinalAnswerGuard(prepared *PreparedChat) skillloop.FinalAnswerGuard {
	if prepared == nil || prepared.parts == nil || !skillIDEnabled(prepared.parts.SkillIDs, skills.SkillConsoleNavigator) {
		return nil
	}
	preparedForGuard := prepared
	if parts, _, _ := stagedExecutionScopedParts(prepared.parts); parts != nil && parts != prepared.parts {
		copyPrepared := *prepared
		copyPrepared.parts = parts
		preparedForGuard = &copyPrepared
	}
	if isManagedFileCreateIntent(preparedForGuard.parts.Query) {
		return nil
	}
	if preparedForGuard.Message != nil {
		if completedConsoleNavigationOperationPlan(preparedForGuard.Message.Metadata) {
			return nil
		}
	}
	target, ok := resolveConsoleNavigationTargetForPrepared(preparedForGuard)
	if !ok {
		return nil
	}
	if consoleNavigationLoadedHrefMatchesTarget("/console/files", target.Href) &&
		consoleFilesRouteAlreadyAvailable(preparedForGuard.parts) {
		return nil
	}
	if consoleNavigationRouteAlreadyAvailable(preparedForGuard.parts, target.Href) {
		return nil
	}
	if clientActionContinuationLoadedRoute(preparedForGuard.parts, target.Href) {
		return nil
	}
	return consoleNavigationRequiredToolFinalAnswerGuard(target)
}

func skillLoopAgentManagementFinalAnswerGuard(prepared *PreparedChat) skillloop.FinalAnswerGuard {
	if prepared == nil || prepared.parts == nil ||
		!skillIDEnabled(prepared.parts.SkillIDs, skills.SkillAgentManagement) {
		return nil
	}
	hasConsoleNavigator := skillIDEnabled(prepared.parts.SkillIDs, skills.SkillConsoleNavigator)
	currentAgentID := currentConsoleAgentID(prepared.parts)
	configReadTargetID := agentManagementConfigReadTargetID(prepared.parts)
	deleteTarget := consoleNavigationRouteHint{Href: "/console/agents", Label: "Agent list"}
	wantsCreatedDetail := hasConsoleNavigator && wantsCreatedAgentDetailNavigationForPrepared(prepared)
	modelDecidesPlan := preparedOperationPlanModelDecides(prepared)
	requiredBindingTools := []string(nil)
	if !modelDecidesPlan {
		requiredBindingTools = agentBindingGuardRequiredToolsForPrepared(prepared)
	}
	operationPlan := map[string]interface{}(nil)
	if prepared.Message != nil {
		operationPlan = mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	}
	requiresConfigRead := !modelDecidesPlan && configReadTargetID != "" &&
		agentManagementFinalAnswerRequiresConfigRead(prepared, operationPlan) &&
		len(requiredBindingTools) == 0
	if currentAgentID == "" && !wantsCreatedDetail && !requiresConfigRead {
		return nil
	}
	return func(req skillloop.FinalAnswerGuardRequest) (skillloop.FinalAnswerGuardResult, bool) {
		if requiresConfigRead &&
			!finalAnswerGuardHasSuccessfulTool(req, skills.SkillAgentManagement, "get_agent_config") &&
			!finalAnswerGuardHasAttemptedTool(req, skills.SkillAgentManagement, "get_agent_config") {
			return agentConfigReadRequiresToolGuardResult(), true
		}
		if missingTool := firstMissingAgentBindingMutation(req, requiredBindingTools); missingTool != "" {
			return agentBindingRequiresMutationGuardResult(missingTool, requiredBindingTools), true
		}
		if wantsCreatedDetail {
			if createdHref := createdAgentDetailHrefFromCalls(req.SuccessfulToolCalls); createdHref != "" &&
				!clientActionContinuationLoadedRoute(prepared.parts, createdHref) &&
				!finalAnswerGuardHasExactConsoleNavigateCall(req.SuccessfulToolCalls, createdHref) &&
				!finalAnswerGuardHasExactConsoleNavigateCall(req.AttemptedToolCalls, createdHref) {
				return createdAgentRequiresDetailNavigationGuardResult(consoleNavigationRouteHint{
					Href:  createdHref,
					Label: "Agent detail",
				}), true
			}
		}
		if !hasConsoleNavigator || currentAgentID == "" || !finalAnswerGuardHasAgentDeleteCall(req.SuccessfulToolCalls, currentAgentID) {
			return skillloop.FinalAnswerGuardResult{}, false
		}
		if finalAnswerGuardHasExactConsoleNavigateCall(req.SuccessfulToolCalls, deleteTarget.Href) ||
			finalAnswerGuardHasExactConsoleNavigateCall(req.AttemptedToolCalls, deleteTarget.Href) {
			return skillloop.FinalAnswerGuardResult{}, false
		}
		return agentDeleteRequiresListNavigationGuardResult(deleteTarget), true
	}
}

func preparedOperationPlanModelDecides(prepared *PreparedChat) bool {
	if prepared == nil || prepared.Message == nil {
		return false
	}
	plan := mapFromOperationContext(metadataValue(prepared.Message.Metadata, "operation_plan"))
	if len(plan) == 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(stringFromAny(plan["planning_mode"])), "phase_only_model_decides") ||
		strings.EqualFold(strings.TrimSpace(stringFromAny(plan["tool_choice_mode"])), aiChatTurnToolChoiceModelDecides)
}

func wantsCreatedAgentDetailNavigationForPrepared(prepared *PreparedChat) bool {
	if prepared == nil || prepared.parts == nil {
		return false
	}
	if intent := prepared.parts.ModelTurnIntent; intent != nil {
		return intent.OpenCreatedAgentDetail
	}
	if prepared.Message != nil {
		plan := mapFromOperationContext(metadataValue(prepared.Message.Metadata, "operation_plan"))
		if len(plan) > 0 {
			if boolFromPlanValue(plan["open_created_agent_detail"]) {
				return true
			}
		}
	}
	return false
}

func boolFromPlanValue(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "yes", "1", "required":
			return true
		default:
			return false
		}
	default:
		return strings.EqualFold(strings.TrimSpace(stringFromAny(value)), "true")
	}
}

func agentManagementFinalAnswerRequiresConfigRead(prepared *PreparedChat, plan map[string]interface{}) bool {
	if operationPlanHasToolStepWithStatus(plan, skills.SkillAgentManagement, "get_agent_config", operationPlanStepStatusPending) {
		return true
	}
	if len(plan) > 0 {
		return false
	}
	return false
}

func agentManagementConfigReadTargetID(parts *chatRequestParts) string {
	if parts == nil || !isConsoleAgentsContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		return ""
	}
	if agentID := currentConsoleAgentID(parts); agentID != "" {
		return agentID
	}
	visible := consoleAgentsPromptVisibleAgents(parts)
	if len(visible) == 0 {
		return ""
	}
	if targetID := agentManagementVisibleIndexTargetID(parts.ModelTurnIntent, visible); targetID != "" {
		return targetID
	}
	query := strings.ToLower(strings.TrimSpace(parts.Query))
	if query == "" {
		return ""
	}
	if targets := agentDeleteTargetsMatchingQueryText(visible, query); len(targets) == 1 {
		return strings.TrimSpace(firstNonEmptyString(targets[0]["agent_id"], targets[0]["id"]))
	}
	return ""
}

func agentManagementVisibleIndexTargetID(intent *AIChatModelTurnIntent, visible []map[string]interface{}) string {
	if intent == nil || intent.TargetVisibleIndex <= 0 || len(visible) == 0 {
		return ""
	}
	for index, agent := range visible {
		visibleIndex := firstPositiveInt(
			intValueFromAny(agent["visible_index"]),
			intValueFromAny(agent["visible_ordinal"]),
			intValueFromAny(agent["visible_rank"]),
			index+1,
		)
		if visibleIndex != intent.TargetVisibleIndex {
			continue
		}
		return strings.TrimSpace(firstNonEmptyString(agent["agent_id"], agent["id"], agent["resource_id"]))
	}
	return ""
}

func agentConfigReadRequiresToolGuardResult() skillloop.FinalAnswerGuardResult {
	return skillloop.FinalAnswerGuardResult{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "get_agent_config",
		Message:  "The user asked for the current Agent configuration, but no agent-management/get_agent_config evidence exists yet.",
		SystemMessage: strings.Join([]string{
			"The candidate final answer is premature for this Agent configuration question.",
			"Call agent-management/get_agent_config for the current Agent before answering.",
			"Base the final answer only on the tool result and page evidence.",
			"If get_agent_config fails, explain that failure truthfully instead of inferring enabled skills, model, provider, bindings, or other configuration from unsupported assumptions.",
		}, " "),
	}
}

func agentBindingGuardRequiredToolsForPrepared(prepared *PreparedChat) []string {
	if prepared == nil {
		return nil
	}
	if prepared.Message != nil {
		plan := mapFromOperationContext(metadataValue(prepared.Message.Metadata, "operation_plan"))
		if tools := agentBindingGuardRequiredToolsFromOperationPlan(plan); len(tools) > 0 {
			return tools
		}
	}
	return nil
}

func agentBindingGuardRequiredToolsFromOperationPlan(plan map[string]interface{}) []string {
	if len(plan) == 0 {
		return nil
	}
	actions := map[string]string{}
	mergeActions := func(next map[string]string) {
		for field, action := range next {
			canonicalField := operationPlanAgentConfigCanonicalField(field)
			canonicalAction := operationPlanCanonicalAgentConfigBindingAction(action)
			if canonicalField == "" || canonicalAction == "" {
				continue
			}
			if operationPlanCanonicalAgentConfigBindingAction(actions[canonicalField]) == "" {
				actions[canonicalField] = canonicalAction
			}
		}
	}

	mergeActions(agentCapabilityGoalsExpectedBindingActions(agentCapabilityGoalsFromOperationPlan(plan)))
	for _, step := range mapSliceFromAny(plan["steps"]) {
		mergeActions(operationPlanAgentConfigBindingActionsFromAny(step[operationPlanExpectedBindingActionsKey]))
		if args := mapFromOperationContext(step["arguments"]); len(args) > 0 {
			mergeActions(operationPlanAgentConfigBindingActionsFromAny(args[operationPlanExpectedBindingActionsKey]))
		}
	}
	if structured := mapFromOperationContext(plan["structured_plan"]); len(structured) > 0 {
		mergeActions(agentCapabilityGoalsExpectedBindingActions(agentCapabilityGoalsFromMaps(structured["capability_goals"])))
		for _, operation := range mapSliceFromAny(structured["operations"]) {
			mergeActions(operationPlanAgentConfigBindingActionsFromAny(operation[operationPlanExpectedBindingActionsKey]))
			if args := mapFromOperationContext(operation["arguments"]); len(args) > 0 {
				mergeActions(operationPlanAgentConfigBindingActionsFromAny(args[operationPlanExpectedBindingActionsKey]))
			}
		}
	}
	return agentBindingGuardRequiredToolsFromActions(actions)
}

func agentBindingGuardRequiredToolsFromActions(actions map[string]string) []string {
	if len(actions) == 0 {
		return nil
	}
	out := []string{}
	for field, action := range actions {
		field = operationPlanAgentConfigCanonicalField(field)
		if field != "" && operationPlanCanonicalAgentConfigBindingAction(action) != "" {
			out = appendUniqueStrings(out, agentBindingUpdateConfigRequirement(field))
		}
	}
	return out
}

func agentBindingUpdateConfigRequirement(field string) string {
	field = operationPlanAgentConfigCanonicalField(field)
	if field == "" {
		return "update_agent_config"
	}
	return "update_agent_config:" + field
}

func agentBindingRequirementField(requirement string) string {
	requirement = strings.TrimSpace(requirement)
	if strings.HasPrefix(requirement, "update_agent_config:") {
		return operationPlanAgentConfigCanonicalField(strings.TrimPrefix(requirement, "update_agent_config:"))
	}
	descriptor, ok := agentBindingToolDescriptorForTool(requirement)
	if !ok {
		return ""
	}
	return operationPlanAgentConfigCanonicalField(descriptor.field)
}

func agentBindingRequirementDisplay(requirement string) string {
	if field := agentBindingRequirementField(requirement); field != "" {
		return "update_agent_config." + field
	}
	return "update_agent_config"
}

func firstMissingAgentBindingMutation(req skillloop.FinalAnswerGuardRequest, requiredTools []string) string {
	for _, toolName := range requiredTools {
		toolName = strings.TrimSpace(toolName)
		if toolName == "" {
			continue
		}
		if agentBindingMutationRequirementSatisfied(req, toolName) {
			continue
		}
		return toolName
	}
	return ""
}

func agentBindingMutationRequirementSatisfied(req skillloop.FinalAnswerGuardRequest, toolName string) bool {
	if finalAnswerGuardHasSuccessfulTool(req, skills.SkillAgentManagement, toolName) ||
		finalAnswerGuardHasAttemptedTool(req, skills.SkillAgentManagement, toolName) {
		return true
	}
	if field := agentBindingRequirementField(toolName); field != "" {
		if agentBindingUpdateConfigCoversField(req.SuccessfulToolCalls, field) {
			return true
		}
		if agentBindingLegacyMutationCoversField(req.SuccessfulToolCalls, field) {
			return true
		}
		if agentBindingLegacyMutationCoversField(req.AttemptedToolCalls, field) {
			return true
		}
		return agentBindingUpdateConfigAttemptedWithoutSuccessfulResult(req)
	}
	if agentBindingUpdateConfigCoversTool(req.SuccessfulToolCalls, toolName) {
		return true
	}
	return agentBindingUpdateConfigAttemptedWithoutSuccessfulResult(req)
}

func agentBindingUpdateConfigAttemptedWithoutSuccessfulResult(req skillloop.FinalAnswerGuardRequest) bool {
	if finalAnswerGuardHasSuccessfulTool(req, skills.SkillAgentManagement, "update_agent_config") {
		return false
	}
	return finalAnswerGuardHasAttemptedTool(req, skills.SkillAgentManagement, "update_agent_config")
}

func agentBindingUpdateConfigCoversTool(calls []skillloop.SkillToolCallRef, toolName string) bool {
	if field := agentBindingRequirementField(toolName); field != "" {
		return agentBindingUpdateConfigCoversField(calls, field)
	}
	for _, call := range calls {
		if !strings.EqualFold(strings.TrimSpace(call.SkillID), skills.SkillAgentManagement) ||
			!strings.EqualFold(strings.TrimSpace(call.ToolName), "update_agent_config") {
			continue
		}
		if agentBindingUpdateConfigResultCoversTool(call.Result, toolName) {
			return true
		}
	}
	return false
}

func agentBindingUpdateConfigCoversField(calls []skillloop.SkillToolCallRef, field string) bool {
	field = operationPlanAgentConfigCanonicalField(field)
	if field == "" {
		return false
	}
	for _, call := range calls {
		if !strings.EqualFold(strings.TrimSpace(call.SkillID), skills.SkillAgentManagement) ||
			!strings.EqualFold(strings.TrimSpace(call.ToolName), "update_agent_config") {
			continue
		}
		if agentBindingUpdateConfigResultCoversField(call.Result, field) {
			return true
		}
	}
	return false
}

func agentBindingLegacyMutationCoversField(calls []skillloop.SkillToolCallRef, field string) bool {
	field = operationPlanAgentConfigCanonicalField(field)
	if field == "" {
		return false
	}
	for _, call := range calls {
		if !strings.EqualFold(strings.TrimSpace(call.SkillID), skills.SkillAgentManagement) {
			continue
		}
		descriptor, ok := agentBindingToolDescriptorForTool(call.ToolName)
		if !ok || !strings.EqualFold(operationPlanAgentConfigCanonicalField(descriptor.field), field) {
			continue
		}
		return true
	}
	return false
}

func agentBindingUpdateConfigResultCoversTool(result map[string]interface{}, toolName string) bool {
	field, kind := agentBindingRequirementFieldAndKind(toolName)
	if field == "" && kind == "" {
		return false
	}
	if field != "" && agentBindingUpdateConfigResultCoversField(result, field) {
		return true
	}
	return agentBindingUpdateConfigResultCoversKind(result, kind)
}

func agentBindingUpdateConfigResultCoversField(result map[string]interface{}, field string) bool {
	field = operationPlanAgentConfigCanonicalField(field)
	if field == "" {
		return false
	}
	if field != "" && stringSliceContainsFold(stringSliceFromAny(result["updated_fields"]), field) {
		return true
	}
	for _, key := range []string{"binding_changes", "config_changes"} {
		for _, change := range mapSliceFromAny(result[key]) {
			if field != "" && strings.EqualFold(strings.TrimSpace(stringFromAny(change["field"])), field) {
				return true
			}
		}
	}
	return false
}

func agentBindingUpdateConfigResultCoversKind(result map[string]interface{}, kind string) bool {
	if kind != "" && strings.EqualFold(strings.TrimSpace(stringFromAny(result["binding_kind"])), kind) {
		return true
	}
	for _, key := range []string{"binding_changes", "config_changes"} {
		for _, change := range mapSliceFromAny(result[key]) {
			if kind != "" && strings.EqualFold(strings.TrimSpace(stringFromAny(change["binding_kind"])), kind) {
				return true
			}
		}
	}
	return false
}

func agentBindingRequirementFieldAndKind(toolName string) (string, string) {
	descriptor, ok := agentBindingToolDescriptorForTool(toolName)
	if !ok {
		return "", ""
	}
	return operationPlanAgentConfigCanonicalField(descriptor.field), strings.TrimSpace(descriptor.bindingKind)
}

func stringSliceContainsFold(values []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), target) {
			return true
		}
	}
	return false
}

func agentBindingRequiresMutationGuardResult(missingTool string, requiredTools []string) skillloop.FinalAnswerGuardResult {
	missing := []string{}
	for _, requirement := range requiredTools {
		missing = appendUniqueStrings(missing, agentBindingRequirementDisplay(requirement))
	}
	if len(missing) == 0 {
		missing = []string{agentBindingRequirementDisplay(missingTool)}
	}
	messageLines := []string{
		"The user explicitly requested Agent binding changes, but at least one requested binding section has no mutation evidence yet.",
		"Before claiming completion, call agent-management/update_agent_config with the remaining binding changes.",
	}
	systemLines := []string{
		"The candidate final answer is premature for this Agent binding request.",
		"Requested binding sections still missing evidence: " + strings.Join(missing, ", ") + ".",
		"The accepted agent-management binding mutation tool for this turn is update_agent_config.",
		"The currently missing section should be represented as update_agent_config patch fields; missing marker: " + agentBindingRequirementDisplay(missingTool) + ".",
		"Use update_agent_config when the current config and exact candidate IDs are known, because it can update multiple binding sections in one governed call.",
		"If a mutation tool fails, explain that failure; do not claim the binding was completed.",
	}
	return skillloop.FinalAnswerGuardResult{
		SkillID:       skills.SkillAgentManagement,
		ToolName:      "update_agent_config",
		Message:       strings.Join(messageLines, " "),
		SystemMessage: strings.Join(systemLines, " "),
	}
}

func currentConsoleAgentID(parts *chatRequestParts) string {
	if parts == nil {
		return ""
	}
	for _, source := range []map[string]interface{}{parts.RawOperationContext, parts.OperationContext} {
		agents := visibleAgentResources(source)
		for _, agent := range agents {
			if agent.Selected && strings.TrimSpace(agent.ID) != "" {
				return strings.TrimSpace(agent.ID)
			}
		}
		if len(agents) == 1 && strings.TrimSpace(agents[0].ID) != "" {
			return strings.TrimSpace(agents[0].ID)
		}
	}
	return ""
}

func wantsCreatedAgentDetailNavigation(query string) bool {
	normalized := normalizeConsoleNavigationQuery(query)
	if normalized == "" {
		return false
	}
	if createdAgentDetailNavigationNegated(normalized) {
		return false
	}
	hasCreate := false
	for _, marker := range []string{
		"\u521b\u5efa", "\u65b0\u5efa", "\u65b0\u589e",
		"create", "new agent",
	} {
		if strings.Contains(normalized, marker) {
			hasCreate = true
			break
		}
	}
	for _, marker := range []string{"创建", "新建", "create", "new agent"} {
		if strings.Contains(normalized, marker) {
			hasCreate = true
			break
		}
	}
	if !hasCreate {
		return false
	}
	for _, marker := range []string{
		"\u6253\u5f00", "\u8fdb\u5165", "\u8fdb\u5230",
		"\u8be6\u60c5", "\u8be6\u7ec6", "\u7f16\u8f91\u9875", "\u914d\u7f6e\u9875",
		"open", "enter", "detail", "details", "view", "config page", "settings page", "edit page",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	for _, marker := range []string{"打开", "进入", "详情", "open", "enter", "detail", "view"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func createdAgentDetailNavigationNegated(normalized string) bool {
	return containsAnySubstring(normalized, []string{
		"do not navigate", "don't navigate", "dont navigate",
		"do not switch pages", "don't switch pages", "dont switch pages",
		"do not open", "don't open", "dont open",
		"do not enter", "don't enter", "dont enter",
		"without navigating", "without opening", "without switching pages",
		"stay on the list", "stay on current page", "stay on the current page",
		"\u4e0d\u8981\u5bfc\u822a", "\u4e0d\u8981\u8df3\u8f6c", "\u4e0d\u8981\u5207\u6362", "\u4e0d\u8981\u5207\u6362\u9875\u9762", "\u4e0d\u8981\u5207\u6362\u5230\u5176\u4ed6\u9875\u9762", "\u4e0d\u8981\u6253\u5f00", "\u4e0d\u8981\u8fdb\u5165",
		"\u4e0d\u7528\u5bfc\u822a", "\u4e0d\u7528\u8df3\u8f6c", "\u4e0d\u7528\u5207\u6362", "\u4e0d\u7528\u5207\u6362\u9875\u9762", "\u4e0d\u7528\u6253\u5f00", "\u4e0d\u7528\u8fdb\u5165",
		"\u65e0\u9700\u5bfc\u822a", "\u65e0\u9700\u8df3\u8f6c", "\u65e0\u9700\u5207\u6362", "\u65e0\u9700\u5207\u6362\u9875\u9762", "\u65e0\u9700\u6253\u5f00", "\u65e0\u9700\u8fdb\u5165",
		"\u7559\u5728\u5217\u8868", "\u7559\u5728\u5f53\u524d\u9875", "\u7559\u5728\u5f53\u524d\u9875\u9762",
	})
}

func consoleNavigationRequestNegated(query string) bool {
	normalized := normalizeConsoleNavigationQuery(query)
	if normalized == "" {
		return false
	}
	return createdAgentDetailNavigationNegated(normalized)
}

func createdAgentDetailHrefFromCalls(calls []skillloop.SkillToolCallRef) string {
	for _, call := range calls {
		if !strings.EqualFold(strings.TrimSpace(call.SkillID), skills.SkillAgentManagement) ||
			!strings.EqualFold(strings.TrimSpace(call.ToolName), "create_agent") {
			continue
		}
		if href := normalizeAgentDetailHref(firstNonEmptyString(
			stringFromAny(call.Result["href"]),
			stringFromAny(call.Result["detail_href"]),
		)); href != "" {
			return href
		}
		if agent := governanceMapFromAny(call.Result["agent"]); len(agent) > 0 {
			if href := normalizeAgentDetailHref(firstNonEmptyString(
				stringFromAny(agent["href"]),
				stringFromAny(agent["detail_href"]),
			)); href != "" {
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
			stringFromAny(call.Result["agent_id"]),
			stringFromAny(call.Result["id"]),
		)); agentID != "" {
			return consoleAgentDetailHref(agentID)
		}
	}
	return ""
}

func consoleAgentDetailHref(agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return ""
	}
	return "/console/agents/" + agentID + "/agent"
}

func consoleAgentIDFromDetailHref(href string) string {
	href = normalizeConsoleNavigationGuardHref(href)
	if !strings.HasPrefix(href, "/console/agents/") {
		return ""
	}
	rest := strings.TrimPrefix(href, "/console/agents/")
	rest = strings.Trim(rest, "/")
	if rest == "" {
		return ""
	}
	if idx := strings.Index(rest, "/"); idx >= 0 {
		rest = rest[:idx]
	}
	return strings.TrimSpace(rest)
}

func normalizeAgentDetailHref(href string) string {
	if agentID := consoleAgentIDFromDetailHref(href); agentID != "" {
		return consoleAgentDetailHref(agentID)
	}
	return ""
}

func clientActionContinuationLoadedRoute(parts *chatRequestParts, href string) bool {
	href = normalizeConsoleNavigationGuardHref(href)
	if parts == nil || href == "" {
		return false
	}
	for _, source := range []map[string]interface{}{parts.RawOperationContext, parts.OperationContext} {
		if len(source) == 0 {
			continue
		}
		continuation := governanceMapFromAny(source["client_action_continuation"])
		if len(continuation) == 0 {
			continue
		}
		if strings.TrimSpace(stringFromAny(continuation["action_type"])) != "route_navigation" {
			continue
		}
		if strings.TrimSpace(stringFromAny(continuation["status"])) != clientActionStatusSucceeded {
			continue
		}
		if consoleNavigationLoadedHrefMatchesTarget(stringFromAny(continuation["href"]), href) {
			return true
		}
		if consoleNavigationResultMatchesTarget(governanceMapFromAny(continuation["result"]), href) {
			return true
		}
	}
	return false
}

func managedFileCreateShouldBlockReturningToCompletedRoute(metadata map[string]interface{}, href string) bool {
	href = normalizeConsoleNavigationGuardHref(href)
	if href == "" || consoleNavigationLoadedHrefMatchesTarget(href, consoleFilesRouteHint().Href) {
		return false
	}
	if !clientActionMetadataHasCompletedRoute(metadata, consoleFilesRouteHint().Href) {
		return false
	}
	return clientActionMetadataHasCompletedRoute(metadata, href)
}

func clientActionMetadataHasCompletedRoute(metadata map[string]interface{}, href string) bool {
	href = normalizeConsoleNavigationGuardHref(href)
	if len(metadata) == 0 || href == "" {
		return false
	}
	for _, action := range mapSliceFromAny(metadata["client_actions"]) {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(action["status"])), clientActionStatusSucceeded) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(action["action_type"])), "route_navigation") {
			continue
		}
		if consoleNavigationLoadedHrefMatchesTarget(stringFromAny(action["href"]), href) {
			return true
		}
		if consoleNavigationResultMatchesTarget(governanceMapFromAny(action["result"]), href) {
			return true
		}
	}
	return false
}

func clientActionMetadataHasActiveRoute(metadata map[string]interface{}, href string) bool {
	href = normalizeConsoleNavigationGuardHref(href)
	if len(metadata) == 0 || href == "" {
		return false
	}
	if clientActionRouteRecordActiveForTarget(governanceMapFromAny(metadata["client_action_continuation"]), href) {
		return true
	}
	for _, action := range mapSliceFromAny(metadata["client_actions"]) {
		if clientActionRouteRecordActiveForTarget(action, href) {
			return true
		}
	}
	for _, invocation := range skillInvocationsFromMetadata(metadata["skill_invocations"]) {
		if strings.TrimSpace(stringFromAny(invocation["kind"])) != "client_action" {
			continue
		}
		if clientActionRouteRecordActiveForTarget(invocation, href) {
			return true
		}
	}
	return false
}

func clientActionRouteRecordActiveForTarget(action map[string]interface{}, href string) bool {
	if len(action) == 0 || href == "" {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(stringFromAny(action["action_type"])), "route_navigation") &&
		!isConsoleNavigatorNavigateTool(stringFromAny(action["skill_id"]), stringFromAny(action["tool_name"])) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(stringFromAny(action["status"]))) {
	case clientActionStatusWaiting, clientActionStatusRunning, "pending", "loading", "streaming":
	default:
		return false
	}
	if consoleNavigationLoadedHrefMatchesTarget(stringFromAny(action["href"]), href) {
		return true
	}
	return consoleNavigationResultMatchesTarget(governanceMapFromAny(action["result"]), href)
}

func consoleNavigationResultMatchesTarget(result map[string]interface{}, href string) bool {
	if len(result) == 0 {
		return false
	}
	for _, key := range []string{"href", "observed_path", "loaded_href", "target_page"} {
		if consoleNavigationLoadedHrefMatchesTarget(stringFromAny(result[key]), href) {
			return true
		}
	}
	return false
}

func consoleFilesRouteAlreadyAvailable(parts *chatRequestParts) bool {
	if parts == nil {
		return false
	}
	return isConsoleFilesContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext)
}

func consoleNavigationRouteAlreadyAvailable(parts *chatRequestParts, href string) bool {
	href = normalizeConsoleNavigationGuardHref(href)
	if parts == nil || href == "" {
		return false
	}
	if runtimeHref := consoleRouteFromRuntimeContext(parts.RuntimeContext); consoleNavigationLoadedHrefMatchesTarget(runtimeHref, href) {
		return true
	}
	switch href {
	case "/console/files":
		if consoleFilesRouteAlreadyAvailable(parts) {
			return true
		}
	case "/console/agents":
		if isConsoleAgentsContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
			return true
		}
	case "/console/workflows":
		if runtimeHref := consoleRouteFromRuntimeContext(parts.RuntimeContext); consoleNavigationLoadedHrefMatchesTarget(runtimeHref, href) {
			return true
		}
	}
	for _, source := range []map[string]interface{}{parts.RawOperationContext, parts.OperationContext} {
		if consoleNavigationContextContainsRoute(source, href) {
			return true
		}
	}
	return false
}

func consoleRouteFromRuntimeContext(runtimeContext string) string {
	runtimeContext = strings.TrimSpace(runtimeContext)
	if runtimeContext == "" {
		return ""
	}
	matches := runtimeContextRoutePattern.FindStringSubmatch(runtimeContext)
	if len(matches) < 2 {
		return ""
	}
	return normalizeConsoleNavigationGuardHref(matches[1])
}

func consoleNavigationContextContainsRoute(context map[string]interface{}, href string) bool {
	if len(context) == 0 || strings.TrimSpace(href) == "" {
		return false
	}
	for _, item := range operationItemsFromValue(context["resources"]) {
		resource, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		metadata := mapFromOperationContext(resource["metadata"])
		resourceType := strings.ToLower(strings.TrimSpace(firstNonEmptyString(
			resource["resource_type"],
			resource["type"],
			resource["kind"],
			metadata["resource_kind"],
			metadata["type"],
		)))
		if resourceType != "page" {
			continue
		}
		for _, value := range []interface{}{
			resource["href"],
			resource["route"],
			metadata["href"],
			metadata["route"],
		} {
			if consoleNavigationLoadedHrefMatchesTarget(stringMetadataValue(value), href) {
				return true
			}
		}
	}
	return false
}

func requiresConsoleFilesRouteBeforeManagedFileCreate(parts *chatRequestParts) bool {
	if parts == nil || !isManagedFileCreateIntent(parts.Query) {
		return false
	}
	if consoleFilesRouteAlreadyAvailable(parts) {
		return false
	}
	if clientActionContinuationLoadedRoute(parts, consoleFilesRouteHint().Href) {
		return false
	}
	return skillIDEnabled(parts.SkillIDs, skills.SkillConsoleNavigator)
}

func resolveConsoleNavigationTargetForPrepared(prepared *PreparedChat) (consoleNavigationRouteHint, bool) {
	if prepared == nil {
		return consoleNavigationRouteHint{}, false
	}
	return resolveConsoleNavigationTargetForParts(prepared.parts)
}

func resolveConsoleNavigationTargetForParts(parts *chatRequestParts) (consoleNavigationRouteHint, bool) {
	if parts == nil {
		return consoleNavigationRouteHint{}, false
	}
	if consoleNavigationRequestNegated(parts.Query) {
		return consoleNavigationRouteHint{}, false
	}
	if requiresConsoleFilesRouteBeforeManagedFileCreate(parts) {
		return consoleFilesRouteHint(), true
	}
	targets := consoleNavigationResolvedTargets(parts.Query)
	if len(targets) == 0 {
		return consoleNavigationRouteHint{}, false
	}
	if len(targets) == 1 {
		return targets[0], true
	}
	completedHrefs := completedConsoleNavigationHrefsFromParts(parts)
	for _, target := range targets {
		if consumeCompletedConsoleNavigationHref(&completedHrefs, target.Href) {
			continue
		}
		if !clientActionContinuationLoadedRoute(parts, target.Href) {
			return target, true
		}
	}
	return targets[len(targets)-1], true
}

func normalizeConsoleNavigationRouteHint(route consoleNavigationRouteHint) consoleNavigationRouteHint {
	if route.Href != "/console/agents" {
		return route
	}
	for _, keyword := range route.Keywords {
		if strings.Contains(strings.ToLower(strings.TrimSpace(keyword)), "workflow") {
			route.Href = "/console/workflows"
			if strings.TrimSpace(route.Label) == "" {
				route.Label = "Workflows"
			}
			return route
		}
	}
	return route
}

func remainingConsoleNavigationRouteSequence(parts *chatRequestParts, nextTarget consoleNavigationRouteHint) []AIChatTurnStrategyRouteStep {
	if parts == nil {
		return nil
	}
	targets := consoleNavigationResolvedTargets(parts.Query)
	if len(targets) == 0 && strings.TrimSpace(nextTarget.Href) != "" {
		targets = []consoleNavigationRouteHint{nextTarget}
	}
	sequence := make([]AIChatTurnStrategyRouteStep, 0, len(targets))
	completedHrefs := completedConsoleNavigationHrefsFromParts(parts)
	for _, target := range targets {
		if strings.TrimSpace(target.Href) == "" {
			continue
		}
		if consumeCompletedConsoleNavigationHref(&completedHrefs, target.Href) {
			continue
		}
		status := "pending"
		if consoleNavigationLoadedHrefMatchesTarget(target.Href, nextTarget.Href) {
			status = "next"
		}
		sequence = append(sequence, AIChatTurnStrategyRouteStep{
			Href:   target.Href,
			Label:  target.Label,
			Status: status,
		})
	}
	if len(sequence) == 0 && strings.TrimSpace(nextTarget.Href) != "" {
		sequence = append(sequence, AIChatTurnStrategyRouteStep{
			Href:   nextTarget.Href,
			Label:  nextTarget.Label,
			Status: "next",
		})
	}
	return sequence
}

func consoleNavigationRouteHintForHref(href string) consoleNavigationRouteHint {
	href = normalizeConsoleNavigationGuardHref(href)
	if href == "" {
		return consoleNavigationRouteHint{}
	}
	for _, route := range consoleNavigationRouteHints {
		if consoleNavigationLoadedHrefMatchesTarget(route.Href, href) {
			return route
		}
	}
	return consoleNavigationRouteHint{Href: href, Label: "Console page"}
}

func completedConsoleNavigationHrefsFromParts(parts *chatRequestParts) []string {
	if parts == nil {
		return nil
	}
	for _, source := range []map[string]interface{}{parts.RawOperationContext, parts.OperationContext} {
		completed := completedConsoleNavigationHrefsFromOperationContext(source)
		if len(completed) > 0 {
			return completed
		}
	}
	return nil
}

func completedConsoleNavigationHrefsFromOperationContext(source map[string]interface{}) []string {
	if len(source) == 0 {
		return nil
	}
	hrefs := make([]string, 0, 4)
	for _, action := range mapSliceFromAny(source["completed_client_actions"]) {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(action["status"])), clientActionStatusSucceeded) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(action["action_type"])), "route_navigation") {
			continue
		}
		if href := clientActionRouteHref(action); href != "" {
			hrefs = append(hrefs, href)
		}
	}
	continuation := governanceMapFromAny(source["client_action_continuation"])
	if strings.EqualFold(strings.TrimSpace(stringFromAny(continuation["status"])), clientActionStatusSucceeded) &&
		strings.EqualFold(strings.TrimSpace(stringFromAny(continuation["action_type"])), "route_navigation") {
		if href := clientActionRouteHref(continuation); href != "" && !lastConsoleNavigationHrefMatches(hrefs, href) {
			hrefs = append(hrefs, href)
		}
	}
	return hrefs
}

func lastConsoleNavigationHrefMatches(hrefs []string, href string) bool {
	if len(hrefs) == 0 {
		return false
	}
	return consoleNavigationLoadedHrefMatchesTarget(hrefs[len(hrefs)-1], href)
}

func consumeCompletedConsoleNavigationHref(completed *[]string, targetHref string) bool {
	if completed == nil || len(*completed) == 0 {
		return false
	}
	for len(*completed) > 0 {
		href := (*completed)[0]
		*completed = (*completed)[1:]
		if consoleNavigationLoadedHrefMatchesTarget(href, targetHref) {
			return true
		}
	}
	return false
}

func consoleFilesRouteHint() consoleNavigationRouteHint {
	for _, route := range consoleNavigationRouteHints {
		if consoleNavigationLoadedHrefMatchesTarget(route.Href, "/console/files") {
			return route
		}
	}
	return consoleNavigationRouteHint{Href: "/console/files", Label: "File Management"}
}

func isConsoleNavigatorNavigateTool(skillID string, toolName string) bool {
	return strings.EqualFold(strings.TrimSpace(skillID), skills.SkillConsoleNavigator) &&
		strings.EqualFold(strings.TrimSpace(toolName), "navigate")
}

func isFileGeneratorToolCall(skillID string, toolName string) bool {
	if !strings.EqualFold(strings.TrimSpace(skillID), skills.SkillFileGenerator) {
		return false
	}
	switch strings.TrimSpace(toolName) {
	case "generate_file", "generate_docx", "generate_pdf", "generate_pptx":
		return true
	default:
		return false
	}
}

func isChartGeneratorToolCall(skillID string, toolName string) bool {
	return strings.EqualFold(strings.TrimSpace(skillID), skills.SkillChartGenerator) &&
		strings.EqualFold(strings.TrimSpace(toolName), "generate_chart")
}

func isKnownArtifactGeneratorToolCall(skillID string, toolName string) bool {
	return isFileGeneratorToolCall(skillID, toolName) || isChartGeneratorToolCall(skillID, toolName)
}

func isFileManagerSaveToolCall(skillID string, toolName string) bool {
	return strings.EqualFold(strings.TrimSpace(skillID), skills.SkillFileManager) &&
		strings.EqualFold(strings.TrimSpace(toolName), "save_file_to_management")
}

func isFileManagerDeleteToolCall(skillID string, toolName string) bool {
	return strings.EqualFold(strings.TrimSpace(skillID), skills.SkillFileManager) &&
		strings.EqualFold(strings.TrimSpace(toolName), "delete_file")
}

func skillLoopUserInputGuard(prepared *PreparedChat) skillloop.UserInputGuard {
	finalGuard := skillLoopFinalAnswerGuard(prepared)
	if finalGuard == nil {
		return nil
	}
	return func(req skillloop.UserInputGuardRequest) (skillloop.FinalAnswerGuardResult, bool) {
		result, blocked := finalGuard(skillloop.FinalAnswerGuardRequest{
			Answer:              req.Message,
			Round:               req.Round,
			SkillUsed:           req.SkillUsed,
			ToolCallCount:       req.ToolCallCount,
			AttemptedToolCalls:  req.AttemptedToolCalls,
			SuccessfulToolCalls: req.SuccessfulToolCalls,
		})
		if !blocked {
			return skillloop.FinalAnswerGuardResult{}, false
		}
		if strings.EqualFold(strings.TrimSpace(result.SkillID), skills.SkillConsoleNavigator) {
			result.Message = strings.Join([]string{
				"The request_user_input call was blocked because the user asked for a known ZGI console route.",
				"Do not ask which page to open when the destination is already resolved from the site map.",
				result.Message,
			}, " ")
		} else {
			result.Message = strings.Join([]string{
				"The request_user_input call was blocked because the files-page target is already resolved in runtime context.",
				"Do not ask the user to choose between visible files, repeat a known file name, or confirm information already represented by resolved_targets_from_user_request.",
				result.Message,
			}, " ")
		}
		return result, true
	}
}

func agentDeleteTargetsMatchingQueryText(visible []map[string]interface{}, query string) []map[string]interface{} {
	targets := make([]map[string]interface{}, 0, len(visible))
	for _, agent := range visible {
		id := strings.ToLower(strings.TrimSpace(firstNonEmptyString(agent["agent_id"], agent["id"])))
		name := strings.ToLower(strings.TrimSpace(stringFromAny(agent["name"])))
		if name != "" && strings.Contains(query, name) {
			targets = append(targets, agentDeleteTargetFromVisibleAgent(agent))
			continue
		}
		if id != "" && strings.Contains(query, id) {
			targets = append(targets, agentDeleteTargetFromVisibleAgent(agent))
		}
	}
	return targets
}

func agentDeleteTargetFromVisibleAgent(agent map[string]interface{}) map[string]interface{} {
	target := map[string]interface{}{}
	if id := strings.TrimSpace(firstNonEmptyString(agent["agent_id"], agent["id"])); id != "" {
		target["agent_id"] = id
		target["id"] = id
	}
	if name := strings.TrimSpace(stringFromAny(agent["name"])); name != "" {
		target["name"] = name
	}
	if href := strings.TrimSpace(stringFromAny(agent["href"])); href != "" {
		target["href"] = href
	}
	return target
}

func consoleNavigationRequiredToolFinalAnswerGuard(target consoleNavigationRouteHint) skillloop.FinalAnswerGuard {
	return func(req skillloop.FinalAnswerGuardRequest) (skillloop.FinalAnswerGuardResult, bool) {
		if finalAnswerGuardHasSuccessfulToolForConsoleHref(req, skills.SkillConsoleNavigator, "navigate", target.Href) ||
			finalAnswerGuardHasAttemptedToolForConsoleHref(req, skills.SkillConsoleNavigator, "navigate", target.Href) {
			return skillloop.FinalAnswerGuardResult{}, false
		}
		return consoleNavigationRequiredToolGuardResult(target), true
	}
}

func consoleNavigationRequiredToolGuardResult(target consoleNavigationRouteHint) skillloop.FinalAnswerGuardResult {
	message := strings.Join([]string{
		fmt.Sprintf("The user's current request is to open the ZGI console page %s (%s).", target.Label, target.Href),
		"Do not finish with a natural-language message saying the page has opened yet.",
		fmt.Sprintf("Load the console-navigator skill if needed, then call call_skill_tool with skill_id %q, tool_name %q, and href %q.", skills.SkillConsoleNavigator, "navigate", target.Href),
		"Only after navigate succeeds in this turn may you tell the user that the page was opened. If the tool fails, report the actual tool result.",
	}, " ")
	payload := map[string]interface{}{
		"skill_id":  skills.SkillConsoleNavigator,
		"tool_name": "navigate",
		"arguments": map[string]interface{}{
			"href": target.Href,
		},
	}
	if target.Label != "" {
		payload["label"] = target.Label
	}
	encoded, err := json.Marshal(payload)
	systemMessage := message
	if err == nil {
		systemMessage = systemMessage + " Resolved route JSON for tool arguments: " + string(encoded)
	}
	return skillloop.FinalAnswerGuardResult{
		SkillID:       skills.SkillConsoleNavigator,
		ToolName:      "navigate",
		Message:       message,
		SystemMessage: systemMessage,
	}
}

func createdAgentRequiresDetailNavigationGuardResult(target consoleNavigationRouteHint) skillloop.FinalAnswerGuardResult {
	message := strings.Join([]string{
		fmt.Sprintf("The Agent was created successfully and the user asked to open its detail page (%s).", target.Href),
		"Do not finish with a natural-language message saying the detail page is open yet.",
		fmt.Sprintf("Call call_skill_tool with skill_id %q, tool_name %q, and href %q.", skills.SkillConsoleNavigator, "navigate", target.Href),
		"Only after navigate succeeds in this turn may you tell the user that the Agent was created and opened.",
	}, " ")
	payload := map[string]interface{}{
		"skill_id":  skills.SkillConsoleNavigator,
		"tool_name": "navigate",
		"arguments": map[string]interface{}{
			"href": target.Href,
		},
		"label": target.Label,
	}
	encoded, err := json.Marshal(payload)
	systemMessage := message
	if err == nil {
		systemMessage = systemMessage + " Resolved route JSON for tool arguments: " + string(encoded)
	}
	return skillloop.FinalAnswerGuardResult{
		SkillID:       skills.SkillConsoleNavigator,
		ToolName:      "navigate",
		Message:       message,
		SystemMessage: systemMessage,
	}
}

func agentDeleteRequiresListNavigationGuardResult(target consoleNavigationRouteHint) skillloop.FinalAnswerGuardResult {
	message := strings.Join([]string{
		"The current Agent detail page was deleted successfully.",
		"Do not finish with a natural-language message saying the Agent list is open yet.",
		fmt.Sprintf("Call call_skill_tool with skill_id %q, tool_name %q, and href %q to navigate back to the Agent list.", skills.SkillConsoleNavigator, "navigate", target.Href),
		"Only after navigate succeeds in this turn may you tell the user that the Agent was deleted and the list page is open.",
	}, " ")
	payload := map[string]interface{}{
		"skill_id":  skills.SkillConsoleNavigator,
		"tool_name": "navigate",
		"arguments": map[string]interface{}{
			"href": target.Href,
		},
		"label": target.Label,
	}
	encoded, err := json.Marshal(payload)
	systemMessage := message
	if err == nil {
		systemMessage = systemMessage + " Resolved route JSON for tool arguments: " + string(encoded)
	}
	return skillloop.FinalAnswerGuardResult{
		SkillID:       skills.SkillConsoleNavigator,
		ToolName:      "navigate",
		Message:       message,
		SystemMessage: systemMessage,
	}
}

func consoleFilesRequiredToolFinalAnswerGuard(skillID string, targets []map[string]interface{}, toolName string, messageTemplates []string) skillloop.FinalAnswerGuard {
	targetSummary := consoleFilesGuardTargetSummary(targets)
	targetFileIDs := consoleFilesGuardTargetFileIDs(targets)
	targetArgumentHint := consoleFilesGuardTargetArgumentHint(targets, toolName)
	return func(req skillloop.FinalAnswerGuardRequest) (skillloop.FinalAnswerGuardResult, bool) {
		if finalAnswerGuardHasSuccessfulToolForTargets(req, skillID, toolName, targetFileIDs) ||
			finalAnswerGuardHasAttemptedToolForTargets(req, skillID, toolName, targetFileIDs) {
			return skillloop.FinalAnswerGuardResult{}, false
		}
		lines := make([]string, 0, len(messageTemplates))
		for _, template := range messageTemplates {
			lines = append(lines, strings.ReplaceAll(template, "{target}", targetSummary))
		}
		systemLines := append([]string{}, lines...)
		if targetArgumentHint != "" {
			systemLines = append(systemLines, targetArgumentHint)
		}
		return skillloop.FinalAnswerGuardResult{
			SkillID:       skillID,
			ToolName:      toolName,
			Message:       strings.Join(lines, " "),
			SystemMessage: strings.Join(systemLines, " "),
		}, true
	}
}

func consoleFilesContinuationPendingDeleteFinalAnswerGuard(parts *chatRequestParts, metadata map[string]interface{}) skillloop.FinalAnswerGuard {
	if parts == nil || !isContinuationIntent(parts.Query) || !skillIDEnabled(parts.SkillIDs, skills.SkillFileManager) {
		return nil
	}
	return func(req skillloop.FinalAnswerGuardRequest) (skillloop.FinalAnswerGuardResult, bool) {
		if managedFileCreateContinuationSaveFlowActive(parts, metadata) {
			pendingSaveArgs := pendingGeneratedArtifactSaveArgumentCandidates(parts, metadata, req.SuccessfulToolCalls)
			if len(pendingSaveArgs) > 0 {
				result := fileManagerSaveRequiredToolGuardResult(pendingSaveArgs[0])
				prefix := "A generated temporary artifact is still not saved to File Management, so deletion cannot run yet."
				result.Message = prefix + " " + result.Message
				result.SystemMessage = prefix + " Save every requested generated artifact before delete_file or any destructive follow-up step. " + result.SystemMessage
				return result, true
			}
		}
		successfulDeleteCalls := append(successfulMetadataToolCalls(metadata, skills.SkillFileManager, "delete_file"), matchingSkillToolCalls(req.SuccessfulToolCalls, skills.SkillFileManager, "delete_file")...)
		if len(successfulDeleteCalls) > 0 {
			return skillloop.FinalAnswerGuardResult{}, false
		}
		return skillloop.FinalAnswerGuardResult{}, false
	}
}

func consoleFilesDeleteAlreadySucceededGuardResult(successfulCalls []skillloop.SkillToolCallRef) skillloop.FinalAnswerGuardResult {
	deletedName := latestSuccessfulDeleteFileName(successfulCalls)
	message := "A file-manager/delete_file call has already succeeded for the frozen deletion target. Do not re-resolve the current third file after deletion and do not ask for another deletion confirmation."
	if deletedName != "" {
		message = "The frozen deletion target " + deletedName + " was already deleted successfully. Do not re-resolve the current third file after deletion and do not ask for another deletion confirmation."
	}
	return skillloop.FinalAnswerGuardResult{
		SkillID:  skills.SkillFileManager,
		ToolName: "delete_file",
		Message:  message,
		SystemMessage: strings.Join([]string{
			message,
			"Provide the final user-visible answer now.",
			"Summarize the completed txt/svg save and the single completed deletion. Do not call another destructive tool unless the user makes a new explicit request.",
		}, " "),
	}
}

func latestSuccessfulDeleteFileName(calls []skillloop.SkillToolCallRef) string {
	for idx := len(calls) - 1; idx >= 0; idx-- {
		call := calls[idx]
		if !strings.EqualFold(strings.TrimSpace(call.SkillID), skills.SkillFileManager) ||
			!strings.EqualFold(strings.TrimSpace(call.ToolName), "delete_file") {
			continue
		}
		if name := firstNonEmptyString(call.Result["file_name"], call.Result["filename"], call.Result["name"], call.Arguments["filename"], call.Arguments["name"]); name != "" {
			return name
		}
		return strings.TrimSpace(stringFromAny(call.Arguments["file_id"]))
	}
	return ""
}

func finalAnswerGuardHasSuccessfulTool(req skillloop.FinalAnswerGuardRequest, skillID string, toolName string) bool {
	return finalAnswerGuardHasSuccessfulToolForTargets(req, skillID, toolName, nil)
}

func finalAnswerGuardHasSuccessfulToolForTargets(req skillloop.FinalAnswerGuardRequest, skillID string, toolName string, targetFileIDs []string) bool {
	return finalAnswerGuardHasToolForTargets(req.SuccessfulToolCalls, skillID, toolName, targetFileIDs)
}

func finalAnswerGuardHasAttemptedTool(req skillloop.FinalAnswerGuardRequest, skillID string, toolName string) bool {
	return finalAnswerGuardHasAttemptedToolForTargets(req, skillID, toolName, nil)
}

func finalAnswerGuardHasSuccessfulFileManagerSaveTool(req skillloop.FinalAnswerGuardRequest) bool {
	return finalAnswerGuardHasFileManagerSaveCall(req.SuccessfulToolCalls)
}

func finalAnswerGuardHasAttemptedFileManagerSaveTool(req skillloop.FinalAnswerGuardRequest) bool {
	return finalAnswerGuardHasFileManagerSaveCall(req.AttemptedToolCalls)
}

func finalAnswerGuardHasFileManagerSaveCall(calls []skillloop.SkillToolCallRef) bool {
	for _, call := range calls {
		if !strings.EqualFold(strings.TrimSpace(call.SkillID), skills.SkillFileManager) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(call.ToolName), "save_file_to_management") {
			return true
		}
	}
	return false
}

func latestGeneratedArtifactSaveArguments(calls []skillloop.SkillToolCallRef) map[string]interface{} {
	for idx := len(calls) - 1; idx >= 0; idx-- {
		call := calls[idx]
		if args := generatedArtifactSaveArguments(call); len(args) > 0 {
			return args
		}
	}
	return nil
}

func latestUnsavedGeneratedArtifactSaveArguments(calls []skillloop.SkillToolCallRef) map[string]interface{} {
	savedToolFileIDs := map[string]struct{}{}
	for _, call := range calls {
		if id := fileManagerSaveToolFileID(call); id != "" {
			savedToolFileIDs[id] = struct{}{}
		}
	}
	for idx := len(calls) - 1; idx >= 0; idx-- {
		call := calls[idx]
		args := generatedArtifactSaveArguments(call)
		if len(args) == 0 {
			continue
		}
		toolFileID := strings.TrimSpace(stringFromAny(args["tool_file_id"]))
		if toolFileID == "" {
			continue
		}
		if _, ok := savedToolFileIDs[toolFileID]; ok {
			continue
		}
		return args
	}
	return nil
}

func allowAdditionalRequestedManagedFileGeneration(parts *chatRequestParts, req skillloop.ToolCallGuardRequest, pendingSaveArgs map[string]interface{}) bool {
	if parts == nil {
		return false
	}
	targets := requestedManagedFileTargetsFromQuery(parts.Query)
	if len(targets) <= 1 {
		return false
	}
	currentTarget := managedFileTargetFromArguments(req.Arguments)
	if currentTarget.Filename == "" && currentTarget.Extension == "" {
		return false
	}
	pendingTarget := managedFileTargetFromArguments(pendingSaveArgs)
	if managedFileTargetsMatch(currentTarget, pendingTarget) {
		return false
	}
	if !managedFileTargetMatchesAny(currentTarget, targets) {
		return false
	}
	for _, call := range req.SuccessfulToolCalls {
		if managedFileTargetsMatch(currentTarget, managedFileTargetFromSuccessfulCall(call)) {
			return false
		}
	}
	return true
}

func latestRecentGeneratedArtifactSaveArguments(parts *chatRequestParts) map[string]interface{} {
	if parts == nil || !isRecentGeneratedArtifactReferenceIntent(parts.Query) {
		return nil
	}
	for _, artifact := range parts.RecentGeneratedArtifacts {
		if args := generatedArtifactMapSaveArguments(artifact); len(args) > 0 {
			return args
		}
	}
	return nil
}

func pendingGeneratedArtifactSaveArgumentCandidates(parts *chatRequestParts, metadata map[string]interface{}, successfulCalls []skillloop.SkillToolCallRef) []map[string]interface{} {
	candidates := []map[string]interface{}{}
	seen := map[string]struct{}{}
	addCandidate := func(args map[string]interface{}) {
		if len(args) == 0 {
			return
		}
		key := "id:" + fileManagerSaveArgumentsToolFileID(args)
		if key == "id:" {
			target := managedFileTargetFromArguments(args)
			key = "target:" + target.Filename + ":" + target.Extension
		}
		if key == "target::" {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		candidates = append(candidates, args)
	}

	for _, artifact := range unsavedGeneratedArtifactsFromMetadata(metadata, successfulCalls) {
		addCandidate(generatedArtifactMapSaveArguments(artifact))
	}
	for _, args := range unsavedGeneratedArtifactSaveArguments(successfulCalls) {
		addCandidate(args)
	}
	if len(candidates) == 0 {
		addCandidate(latestRecentGeneratedArtifactSaveArguments(parts))
	}
	return candidates
}

func unsavedGeneratedArtifactSaveArguments(calls []skillloop.SkillToolCallRef) []map[string]interface{} {
	savedToolFileIDs := map[string]struct{}{}
	for _, call := range calls {
		if id := fileManagerSaveToolFileID(call); id != "" {
			savedToolFileIDs[id] = struct{}{}
		}
	}
	candidates := []map[string]interface{}{}
	for _, call := range calls {
		args := generatedArtifactSaveArguments(call)
		if len(args) == 0 {
			continue
		}
		toolFileID := strings.TrimSpace(stringFromAny(args["tool_file_id"]))
		if toolFileID == "" {
			continue
		}
		if _, saved := savedToolFileIDs[toolFileID]; saved {
			continue
		}
		candidates = append(candidates, args)
	}
	return candidates
}

func fileManagerSaveArgumentsMatchAnyPendingArtifact(args map[string]interface{}, candidates []map[string]interface{}) bool {
	for _, candidate := range candidates {
		if fileManagerSaveArgumentsMatchPendingArtifact(args, candidate) {
			return true
		}
	}
	return false
}

func fileManagerSaveArgumentsMatchPendingArtifact(args map[string]interface{}, expected map[string]interface{}) bool {
	if len(args) == 0 || len(expected) == 0 {
		return false
	}
	expectedID := fileManagerSaveArgumentsToolFileID(expected)
	if expectedID == "" || fileManagerSaveArgumentsToolFileID(args) != expectedID {
		return false
	}
	sourceType := strings.ToLower(strings.TrimSpace(stringFromAny(args["source_type"])))
	if sourceType != "" && sourceType != "tool_file" {
		return false
	}
	expectedTarget := managedFileTargetFromArguments(expected)
	actualTarget := managedFileTargetFromArguments(args)
	if actualTarget.Filename != "" && expectedTarget.Filename != "" && actualTarget.Filename != expectedTarget.Filename {
		return false
	}
	if actualTarget.Filename == "" && actualTarget.Extension != "" && expectedTarget.Extension != "" && actualTarget.Extension != expectedTarget.Extension {
		return false
	}
	return true
}

func pendingGeneratedArtifactSaveArgumentsForAttempt(args map[string]interface{}, candidates []map[string]interface{}) map[string]interface{} {
	if len(candidates) == 0 {
		return nil
	}
	attemptedID := fileManagerSaveArgumentsToolFileID(args)
	if attemptedID != "" {
		for _, candidate := range candidates {
			if fileManagerSaveArgumentsToolFileID(candidate) == attemptedID {
				return candidate
			}
		}
	}
	attemptedTarget := managedFileTargetFromArguments(args)
	if attemptedTarget.Filename != "" || attemptedTarget.Extension != "" {
		for _, candidate := range candidates {
			if managedFileTargetsMatch(attemptedTarget, managedFileTargetFromArguments(candidate)) {
				return candidate
			}
		}
	}
	return candidates[len(candidates)-1]
}

func latestUnsavedGeneratedArtifactSaveArgumentsForTargetsFromMetadata(metadata map[string]interface{}, missingTargets []string) map[string]interface{} {
	if len(metadata) == 0 || len(missingTargets) == 0 {
		return nil
	}
	artifacts := generatedFilesFromMetadata(metadata["generated_files"])
	if len(artifacts) == 0 {
		return nil
	}
	savedToolFileIDs := map[string]struct{}{}
	for _, artifact := range artifacts {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(artifact["target"])), "managed_file") &&
			strings.TrimSpace(stringFromAny(artifact["upload_file_id"])) == "" {
			continue
		}
		toolFileID := strings.TrimSpace(firstNonEmptyString(
			artifact["source_tool_file_id"],
			artifact["source_file_id"],
			artifact["tool_file_id"],
		))
		if toolFileID != "" {
			savedToolFileIDs[toolFileID] = struct{}{}
		}
	}
	for idx := len(artifacts) - 1; idx >= 0; idx-- {
		artifact := artifacts[idx]
		args := generatedArtifactMapSaveArguments(artifact)
		if len(args) == 0 {
			continue
		}
		toolFileID := strings.TrimSpace(stringFromAny(args["tool_file_id"]))
		if toolFileID == "" {
			continue
		}
		if _, saved := savedToolFileIDs[toolFileID]; saved {
			continue
		}
		if !managedFileTargetMatchesAny(managedFileTargetFromArguments(args), managedFileTargetsFromMissingTargetLabels(missingTargets)) {
			continue
		}
		return args
	}
	return nil
}

func managedFileCreateHasUnsavedExplicitTargets(prepared *PreparedChat) bool {
	if prepared == nil || prepared.parts == nil {
		return false
	}
	metadata := map[string]interface{}(nil)
	if prepared.Message != nil {
		metadata = prepared.Message.Metadata
	}
	if !isManagedFileCreateIntent(prepared.parts.Query) && !managedFileCreateContinuationSaveFlowActive(prepared.parts, metadata) {
		return false
	}
	missingTargets := managedFileCreateMissingSaveTargets(prepared.parts, metadata, nil)
	return len(missingTargets) > 0 &&
		(len(latestUnsavedGeneratedArtifactSaveArgumentsForTargetsFromMetadata(metadata, missingTargets)) > 0 ||
			len(latestUnsavedGeneratedArtifactSaveArgumentsFromMetadata(metadata, nil)) > 0)
}

func managedFileCreateMissingSaveTargets(parts *chatRequestParts, metadata map[string]interface{}, successfulCalls []skillloop.SkillToolCallRef) []string {
	metadataSaveCalls := managedFileSaveCallsFromGeneratedFilesMetadata(metadata)
	allSuccessfulCalls := append([]skillloop.SkillToolCallRef{}, metadataSaveCalls...)
	allSuccessfulCalls = append(allSuccessfulCalls, successfulCalls...)
	if missingTargets := missingRequestedManagedFileSaveTargets(parts, allSuccessfulCalls); len(missingTargets) > 0 {
		return missingTargets
	}
	if !managedFileCreateContinuationSaveFlowActive(parts, metadata) {
		return nil
	}
	return unsavedGeneratedArtifactTargetLabelsFromMetadata(metadata, successfulCalls)
}

func managedFileCreateContinuationSaveFlowActive(parts *chatRequestParts, metadata map[string]interface{}) bool {
	if parts == nil || !isContinuationIntent(parts.Query) {
		return false
	}
	return len(managedFileSaveCallsFromGeneratedFilesMetadata(metadata)) > 0 &&
		len(unsavedGeneratedArtifactsFromMetadata(metadata, nil)) > 0
}

func generatedFileMetadataHasAnyArtifact(metadata map[string]interface{}) bool {
	return len(generatedFilesFromMetadata(metadataValue(metadata, "generated_files"))) > 0
}

func generatedFileMetadataArtifactsAlreadySaved(metadata map[string]interface{}, successfulCalls []skillloop.SkillToolCallRef) bool {
	if !generatedFileMetadataHasAnyArtifact(metadata) {
		return false
	}
	return len(unsavedGeneratedArtifactsFromMetadata(metadata, successfulCalls)) == 0
}

func unsavedGeneratedArtifactTargetLabelsFromMetadata(metadata map[string]interface{}, successfulCalls []skillloop.SkillToolCallRef) []string {
	artifacts := unsavedGeneratedArtifactsFromMetadata(metadata, successfulCalls)
	if len(artifacts) == 0 {
		return nil
	}
	labels := make([]string, 0, len(artifacts))
	seen := map[string]struct{}{}
	for _, artifact := range artifacts {
		label := normalizeManagedFileTargetName(firstNonEmptyString(artifact["filename"], artifact["name"]))
		if label == "" {
			if ext := normalizeManagedFileTargetExtension(firstNonEmptyString(artifact["extension"], artifact["file_type"])); ext != "" {
				label = "*." + ext
			}
		}
		if label == "" {
			continue
		}
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		labels = append(labels, label)
	}
	return labels
}

func latestUnsavedGeneratedArtifactSaveArgumentsFromMetadata(metadata map[string]interface{}, successfulCalls []skillloop.SkillToolCallRef) map[string]interface{} {
	artifacts := unsavedGeneratedArtifactsFromMetadata(metadata, successfulCalls)
	for idx := len(artifacts) - 1; idx >= 0; idx-- {
		if args := generatedArtifactMapSaveArguments(artifacts[idx]); len(args) > 0 {
			return args
		}
	}
	return nil
}

func unsavedGeneratedArtifactsFromMetadata(metadata map[string]interface{}, successfulCalls []skillloop.SkillToolCallRef) []map[string]interface{} {
	if len(metadata) == 0 {
		return nil
	}
	artifacts := generatedFilesFromMetadata(metadataValue(metadata, "generated_files"))
	if len(artifacts) == 0 {
		return nil
	}
	savedToolFileIDs := map[string]struct{}{}
	for _, artifact := range artifacts {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(artifact["target"])), "managed_file") &&
			strings.TrimSpace(stringFromAny(artifact["upload_file_id"])) == "" {
			continue
		}
		toolFileID := strings.TrimSpace(firstNonEmptyString(
			artifact["source_tool_file_id"],
			artifact["source_file_id"],
			artifact["tool_file_id"],
		))
		if toolFileID != "" {
			savedToolFileIDs[toolFileID] = struct{}{}
		}
	}
	for _, call := range successfulCalls {
		if toolFileID := fileManagerSaveToolFileID(call); toolFileID != "" {
			savedToolFileIDs[toolFileID] = struct{}{}
		}
	}
	unsaved := make([]map[string]interface{}, 0, len(artifacts))
	for _, artifact := range artifacts {
		args := generatedArtifactMapSaveArguments(artifact)
		if len(args) == 0 {
			continue
		}
		toolFileID := strings.TrimSpace(stringFromAny(args["tool_file_id"]))
		if toolFileID == "" {
			continue
		}
		if _, saved := savedToolFileIDs[toolFileID]; saved {
			continue
		}
		unsaved = append(unsaved, artifact)
	}
	return unsaved
}

func managedFileSaveCallsFromGeneratedFilesMetadata(metadata map[string]interface{}) []skillloop.SkillToolCallRef {
	artifacts := generatedFilesFromMetadata(metadataValue(metadata, "generated_files"))
	if len(artifacts) == 0 {
		return nil
	}
	calls := make([]skillloop.SkillToolCallRef, 0, len(artifacts))
	for _, artifact := range artifacts {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(artifact["target"])), "managed_file") &&
			strings.TrimSpace(stringFromAny(artifact["upload_file_id"])) == "" {
			continue
		}
		filename := strings.TrimSpace(firstNonEmptyString(
			artifact["filename"],
			artifact["file_name"],
			artifact["name"],
		))
		sourceFileID := strings.TrimSpace(firstNonEmptyString(
			artifact["source_tool_file_id"],
			artifact["source_file_id"],
			artifact["tool_file_id"],
		))
		if filename == "" && sourceFileID == "" {
			continue
		}
		calls = append(calls, skillloop.SkillToolCallRef{
			SkillID:  skills.SkillFileManager,
			ToolName: "save_file_to_management",
			Arguments: map[string]interface{}{
				"filename":     filename,
				"tool_file_id": sourceFileID,
				"source_type":  "tool_file",
			},
			Result: map[string]interface{}{
				"file_name":      filename,
				"filename":       filename,
				"source_file_id": sourceFileID,
				"target":         "managed_file",
			},
		})
	}
	return calls
}

type requestedManagedFileTarget struct {
	Filename  string
	Extension string
}

func managedFileTargetFromArguments(args map[string]interface{}) requestedManagedFileTarget {
	filename := normalizeManagedFileTargetName(firstNonEmptyString(
		args["filename"],
		args["output_filename"],
		args["name"],
		args["file_name"],
	))
	extension := normalizeManagedFileTargetExtension(firstNonEmptyString(
		args["format"],
		args["extension"],
		args["file_type"],
	))
	if extension == "" {
		extension = managedFileTargetExtension(filename)
	}
	if filename != "" && managedFileTargetExtension(filename) == "" && extension != "" {
		filename = filename + "." + extension
	}
	return requestedManagedFileTarget{
		Filename:  filename,
		Extension: extension,
	}
}

func managedFileTargetFromSuccessfulCall(call skillloop.SkillToolCallRef) requestedManagedFileTarget {
	if finalAnswerGuardHasFileManagerSaveCall([]skillloop.SkillToolCallRef{call}) {
		return managedFileTargetFromArguments(map[string]interface{}{
			"filename": firstNonEmptyString(
				call.Result["file_name"],
				call.Result["filename"],
				call.Result["name"],
				call.Arguments["filename"],
			),
			"extension": firstNonEmptyString(
				call.Result["extension"],
				call.Arguments["extension"],
			),
		})
	}
	return managedFileTargetFromArguments(generatedArtifactSaveArguments(call))
}

func managedFileTargetsMatch(left, right requestedManagedFileTarget) bool {
	if left.Filename != "" && right.Filename != "" {
		return left.Filename == right.Filename
	}
	return left.Extension != "" && right.Extension != "" && left.Extension == right.Extension
}

func managedFileTargetMatchesAny(target requestedManagedFileTarget, candidates []requestedManagedFileTarget) bool {
	for _, candidate := range candidates {
		if managedFileTargetsMatch(target, candidate) {
			return true
		}
	}
	return false
}

func managedFileTargetsFromMissingTargetLabels(labels []string) []requestedManagedFileTarget {
	targets := make([]requestedManagedFileTarget, 0, len(labels))
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		if strings.HasPrefix(label, "*.") {
			extension := normalizeManagedFileTargetExtension(strings.TrimPrefix(label, "*."))
			if extension != "" {
				targets = append(targets, requestedManagedFileTarget{Extension: extension})
			}
			continue
		}
		filename := normalizeManagedFileTargetName(label)
		if filename == "" {
			continue
		}
		targets = append(targets, requestedManagedFileTarget{
			Filename:  filename,
			Extension: managedFileTargetExtension(filename),
		})
	}
	return targets
}

func requestedManagedFileTargetsFromQuery(query string) []requestedManagedFileTarget {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	seen := map[string]struct{}{}
	targets := []requestedManagedFileTarget{}
	for _, match := range managedFileTargetPattern.FindAllString(query, -1) {
		filename := normalizeManagedFileTargetName(match)
		if filename == "" {
			continue
		}
		key := "name:" + filename
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		targets = append(targets, requestedManagedFileTarget{
			Filename:  filename,
			Extension: managedFileTargetExtension(filename),
		})
	}
	if len(targets) > 0 {
		return targets
	}
	text := normalizeConsoleNavigationQuery(query)
	if text == "" || !containsAnySubstring(text, []string{"two files", "2 files", "\u4e24\u4e2a\u6587\u4ef6", "2\u4e2a\u6587\u4ef6", "\u4e00\u4e2a\u6587\u672c", "\u4e00\u4e2asvg"}) {
		return nil
	}
	if containsAnySubstring(text, []string{"txt", "text file", "\u6587\u672c\u6587\u4ef6"}) {
		targets = append(targets, requestedManagedFileTarget{Extension: "txt"})
	}
	if containsAnySubstring(text, []string{"svg"}) {
		targets = append(targets, requestedManagedFileTarget{Extension: "svg"})
	}
	return targets
}

func missingRequestedManagedFileSaveTargets(parts *chatRequestParts, calls []skillloop.SkillToolCallRef) []string {
	if parts == nil {
		return nil
	}
	targets := requestedManagedFileTargetsFromQuery(parts.Query)
	if len(targets) <= 1 {
		return nil
	}
	savedNames := map[string]int{}
	savedExtensions := map[string]int{}
	for _, call := range calls {
		if !finalAnswerGuardHasFileManagerSaveCall([]skillloop.SkillToolCallRef{call}) {
			continue
		}
		name := savedManagedFileName(call)
		if name == "" {
			continue
		}
		savedNames[name]++
		if ext := managedFileTargetExtension(name); ext != "" {
			savedExtensions[ext]++
		}
	}
	missing := []string{}
	for _, target := range targets {
		if target.Filename != "" {
			if savedNames[target.Filename] > 0 {
				continue
			}
			missing = append(missing, target.Filename)
			continue
		}
		if target.Extension != "" {
			if savedExtensions[target.Extension] > 0 {
				savedExtensions[target.Extension]--
				continue
			}
			missing = append(missing, "*."+target.Extension)
		}
	}
	return missing
}

func savedManagedFileName(call skillloop.SkillToolCallRef) string {
	if !finalAnswerGuardHasFileManagerSaveCall([]skillloop.SkillToolCallRef{call}) {
		return ""
	}
	return normalizeManagedFileTargetName(firstNonEmptyString(
		call.Result["file_name"],
		call.Result["filename"],
		call.Result["name"],
		call.Arguments["filename"],
		call.Arguments["output_filename"],
	))
}

func fileManagerSaveToolFileID(call skillloop.SkillToolCallRef) string {
	if !finalAnswerGuardHasFileManagerSaveCall([]skillloop.SkillToolCallRef{call}) {
		return ""
	}
	return strings.TrimSpace(firstNonEmptyString(
		call.Arguments["tool_file_id"],
		call.Arguments["file_id"],
		call.Result["source_tool_file_id"],
		call.Result["source_file_id"],
		call.Result["tool_file_id"],
	))
}

func fileManagerSaveArgumentsToolFileID(args map[string]interface{}) string {
	if len(args) == 0 {
		return ""
	}
	return strings.TrimSpace(firstNonEmptyString(
		args["tool_file_id"],
		args["file_id"],
		args["source_tool_file_id"],
		args["source_file_id"],
	))
}

func normalizeManagedFileTargetName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Trim(name, " \t\r\n\"'`.,，。;；:：!！?？)）]】}》>“”‘’")
	name = strings.ReplaceAll(name, "\\", "/")
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	return strings.ToLower(strings.TrimSpace(name))
}

func managedFileTargetExtension(filename string) string {
	filename = normalizeManagedFileTargetName(filename)
	if filename == "" {
		return ""
	}
	idx := strings.LastIndex(filename, ".")
	if idx < 0 || idx == len(filename)-1 {
		return ""
	}
	ext := strings.TrimPrefix(filename[idx+1:], ".")
	if ext == "markdown" {
		return "md"
	}
	return ext
}

func normalizeManagedFileTargetExtension(extension string) string {
	extension = strings.ToLower(strings.TrimSpace(extension))
	extension = strings.TrimPrefix(extension, ".")
	if extension == "markdown" {
		return "md"
	}
	return extension
}

func generatedArtifactSaveArguments(call skillloop.SkillToolCallRef) map[string]interface{} {
	if finalAnswerGuardHasFileManagerSaveCall([]skillloop.SkillToolCallRef{call}) {
		return nil
	}
	if !toolCallResultLooksLikeGeneratedArtifact(call) {
		return nil
	}
	toolFileID := strings.TrimSpace(firstNonEmptyString(call.Result["tool_file_id"], call.Result["file_id"]))
	if toolFileID == "" {
		return nil
	}
	filename := strings.TrimSpace(firstNonEmptyString(
		call.Result["filename"],
		call.Result["name"],
		call.Arguments["filename"],
		call.Arguments["output_filename"],
	))
	args := map[string]interface{}{
		"source_type":  "tool_file",
		"tool_file_id": toolFileID,
	}
	if filename != "" {
		args["filename"] = filename
	}
	return args
}

func generatedArtifactMapSaveArguments(artifact map[string]interface{}) map[string]interface{} {
	if len(artifact) == 0 {
		return nil
	}
	if strings.TrimSpace(stringFromAny(artifact["upload_file_id"])) != "" ||
		strings.EqualFold(strings.TrimSpace(stringFromAny(artifact["target"])), "managed_file") {
		return nil
	}
	toolFileID := strings.TrimSpace(firstNonEmptyString(artifact["tool_file_id"], artifact["file_id"]))
	if toolFileID == "" {
		return nil
	}
	args := map[string]interface{}{
		"source_type":  "tool_file",
		"tool_file_id": toolFileID,
	}
	if filename := strings.TrimSpace(firstNonEmptyString(artifact["filename"], artifact["name"])); filename != "" {
		args["filename"] = filename
	}
	return args
}

func isRecentGeneratedArtifactReferenceIntent(query string) bool {
	text := strings.ToLower(strings.TrimSpace(query))
	if text == "" {
		return false
	}
	if !containsAnySubstring(text, []string{
		"save", "upload", "import", "add", "put",
		"\u4fdd\u5b58", "\u4e0a\u4f20", "\u5bfc\u5165", "\u6dfb\u52a0", "\u52a0\u5230", "\u653e\u5230", "\u5b58\u5230",
	}) {
		return false
	}
	return containsAnySubstring(text, []string{
		"this file", "that file", "previous file", "last file", "generated file", "created file", "the file just",
		"\u8fd9\u4e2a\u6587\u4ef6", "\u8fd9\u4efd\u6587\u4ef6", "\u8fd9\u4e2a", "\u8fd9\u4efd",
		"\u521a\u521a\u7684\u6587\u4ef6", "\u521a\u624d\u7684\u6587\u4ef6", "\u521a\u751f\u6210\u7684\u6587\u4ef6",
		"\u4e0a\u4e00\u4e2a\u6587\u4ef6", "\u4e0a\u4efd\u6587\u4ef6", "\u751f\u6210\u7684\u6587\u4ef6",
	})
}

func toolCallResultLooksLikeGeneratedArtifact(call skillloop.SkillToolCallRef) bool {
	if len(call.Result) == 0 {
		return false
	}
	if strings.TrimSpace(stringFromAny(call.Result["upload_file_id"])) != "" ||
		strings.EqualFold(strings.TrimSpace(stringFromAny(call.Result["target"])), "managed_file") {
		return false
	}
	if strings.TrimSpace(stringFromAny(call.Result["tool_file_id"])) != "" {
		return true
	}
	if isKnownArtifactGeneratorToolCall(call.SkillID, call.ToolName) &&
		strings.TrimSpace(stringFromAny(call.Result["file_id"])) != "" {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(stringFromAny(call.Result["transfer_method"])), "tool_file") {
		return true
	}
	if !hasGeneratedArtifactURL(call.Result) {
		return false
	}
	if strings.TrimSpace(stringFromAny(call.Result["file_id"])) == "" {
		return false
	}
	return strings.TrimSpace(firstNonEmptyString(
		call.Result["filename"],
		call.Result["name"],
		call.Result["mime_type"],
		call.Result["format"],
	)) != ""
}

func hasGeneratedArtifactURL(result map[string]interface{}) bool {
	return strings.TrimSpace(firstNonEmptyString(result["download_url"], result["url"])) != ""
}

func finalAnswerGuardHasAttemptedToolForTargets(req skillloop.FinalAnswerGuardRequest, skillID string, toolName string, targetFileIDs []string) bool {
	return finalAnswerGuardHasToolForTargets(req.AttemptedToolCalls, skillID, toolName, targetFileIDs)
}

func finalAnswerGuardHasToolForTargets(calls []skillloop.SkillToolCallRef, skillID string, toolName string, targetFileIDs []string) bool {
	if len(targetFileIDs) == 0 {
		for _, call := range calls {
			if strings.EqualFold(strings.TrimSpace(call.SkillID), skillID) &&
				strings.EqualFold(strings.TrimSpace(call.ToolName), toolName) {
				return true
			}
		}
		return false
	}
	required := map[string]struct{}{}
	for _, id := range targetFileIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			required[id] = struct{}{}
		}
	}
	if len(required) == 0 {
		return false
	}
	matched := map[string]struct{}{}
	for _, call := range calls {
		if !strings.EqualFold(strings.TrimSpace(call.SkillID), skillID) ||
			!strings.EqualFold(strings.TrimSpace(call.ToolName), toolName) {
			continue
		}
		actual := skillToolCallFileIDs(call.Arguments)
		for _, got := range actual {
			if _, ok := required[got]; ok {
				matched[got] = struct{}{}
			}
		}
	}
	return len(matched) == len(required)
}

func successfulMetadataToolCalls(metadata map[string]interface{}, skillID string, toolName string) []skillloop.SkillToolCallRef {
	invocations := skillInvocationsFromMetadata(metadataValue(metadata, "skill_invocations"))
	if len(invocations) == 0 {
		return nil
	}
	calls := make([]skillloop.SkillToolCallRef, 0)
	for _, invocation := range invocations {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["kind"])), "tool_call") ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["status"])), "success") ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["skill_id"])), skillID) ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["tool_name"])), toolName) {
			continue
		}
		calls = append(calls, skillloop.SkillToolCallRef{
			SkillID:   strings.TrimSpace(stringFromAny(invocation["skill_id"])),
			ToolName:  strings.TrimSpace(stringFromAny(invocation["tool_name"])),
			Arguments: governanceMapFromAny(invocation["arguments"]),
			Result:    governanceMapFromAny(invocation["result"]),
		})
	}
	return calls
}

func matchingSkillToolCalls(calls []skillloop.SkillToolCallRef, skillID string, toolName string) []skillloop.SkillToolCallRef {
	matches := make([]skillloop.SkillToolCallRef, 0)
	for _, call := range calls {
		if strings.EqualFold(strings.TrimSpace(call.SkillID), skillID) &&
			strings.EqualFold(strings.TrimSpace(call.ToolName), toolName) {
			matches = append(matches, call)
		}
	}
	return matches
}

func finalAnswerGuardHasSuccessfulToolForConsoleHref(req skillloop.FinalAnswerGuardRequest, skillID string, toolName string, href string) bool {
	return finalAnswerGuardHasToolForConsoleHref(req.SuccessfulToolCalls, skillID, toolName, href)
}

func finalAnswerGuardHasAttemptedToolForConsoleHref(req skillloop.FinalAnswerGuardRequest, skillID string, toolName string, href string) bool {
	return finalAnswerGuardHasToolForConsoleHref(req.AttemptedToolCalls, skillID, toolName, href)
}

func finalAnswerGuardHasToolForConsoleHref(calls []skillloop.SkillToolCallRef, skillID string, toolName string, href string) bool {
	href = normalizeConsoleNavigationGuardHref(href)
	if href == "" {
		return false
	}
	for _, call := range calls {
		if !strings.EqualFold(strings.TrimSpace(call.SkillID), skillID) ||
			!strings.EqualFold(strings.TrimSpace(call.ToolName), toolName) {
			continue
		}
		if consoleNavigationLoadedHrefMatchesTarget(skillToolCallArgumentString(call.Arguments, "href"), href) {
			return true
		}
	}
	return false
}

func finalAnswerGuardHasAgentDeleteCall(calls []skillloop.SkillToolCallRef, agentID string) bool {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return false
	}
	for _, call := range calls {
		if !strings.EqualFold(strings.TrimSpace(call.SkillID), skills.SkillAgentManagement) {
			continue
		}
		switch strings.TrimSpace(call.ToolName) {
		case "delete_agent":
			if strings.TrimSpace(firstNonEmptyString(
				skillToolCallArgumentString(call.Arguments, "agent_id"),
				skillToolCallArgumentString(call.Arguments, "id"),
				skillToolCallArgumentString(call.Arguments, "asset_id"),
			)) == agentID {
				return true
			}
		case "delete_agents":
			if finalAnswerGuardBatchAgentDeleteHasTarget(call, agentID) {
				return true
			}
		}
	}
	return false
}

func finalAnswerGuardBatchAgentDeleteHasTarget(call skillloop.SkillToolCallRef, agentID string) bool {
	hasItemEvidence := false
	for _, item := range mapSliceFromAny(call.Result["item_results"]) {
		hasItemEvidence = true
		if strings.TrimSpace(firstNonEmptyString(
			item["agent_id"],
			item["id"],
			item["asset_id"],
			item["resource_id"],
		)) == agentID &&
			finalAnswerGuardAgentDeleteItemSucceeded(item["status"]) {
			return true
		}
	}
	group := mapFromOperationContext(call.Result["operation_group"])
	for _, item := range mapSliceFromAny(group["item_results"]) {
		hasItemEvidence = true
		if strings.TrimSpace(firstNonEmptyString(
			item["agent_id"],
			item["id"],
			item["asset_id"],
			item["resource_id"],
		)) == agentID &&
			finalAnswerGuardAgentDeleteItemSucceeded(item["status"]) {
			return true
		}
	}
	if hasItemEvidence {
		return false
	}
	return false
}

func finalAnswerGuardAgentDeleteItemSucceeded(status interface{}) bool {
	switch strings.ToLower(strings.TrimSpace(stringFromAny(status))) {
	case "succeeded", "success", "completed":
		return true
	default:
		return false
	}
}

func finalAnswerGuardHasExactConsoleNavigateCall(calls []skillloop.SkillToolCallRef, href string) bool {
	href = normalizeConsoleNavigationGuardHref(href)
	if href == "" {
		return false
	}
	for _, call := range calls {
		if !isConsoleNavigatorNavigateTool(call.SkillID, call.ToolName) {
			continue
		}
		if normalizeConsoleNavigationGuardHref(skillToolCallArgumentString(call.Arguments, "href")) == href {
			return true
		}
	}
	return false
}

func consoleNavigationLoadedHrefMatchesTarget(loadedHref string, targetHref string) bool {
	loadedHref = normalizeConsoleNavigationGuardHref(loadedHref)
	targetHref = normalizeConsoleNavigationGuardHref(targetHref)
	if loadedHref == "" || targetHref == "" {
		return false
	}
	if loadedAgentHref := normalizeAgentDetailHref(loadedHref); loadedAgentHref != "" {
		if targetAgentHref := normalizeAgentDetailHref(targetHref); targetAgentHref != "" {
			return loadedAgentHref == targetAgentHref
		}
	}
	if loadedHref == targetHref {
		return true
	}
	if targetHref == "/" || targetHref == "/console" {
		return false
	}
	switch targetHref {
	case "/console/agents", "/console/workflows":
		return strings.HasPrefix(loadedHref, targetHref+"/")
	default:
		return false
	}
}

func skillToolCallArgumentString(arguments map[string]interface{}, key string) string {
	if len(arguments) == 0 {
		return ""
	}
	return strings.TrimSpace(stringFromAny(arguments[key]))
}

func normalizeConsoleNavigationGuardHref(rawHref string) string {
	rawHref = strings.TrimSpace(rawHref)
	if rawHref == "" {
		return ""
	}
	if parsed, err := strings.CutPrefix(rawHref, "http://localhost:2780"); err {
		rawHref = parsed
	}
	if parsed, err := strings.CutPrefix(rawHref, "https://localhost:2780"); err {
		rawHref = parsed
	}
	if !strings.HasPrefix(rawHref, "/") {
		rawHref = "/" + rawHref
	}
	if idx := strings.IndexAny(rawHref, "?#"); idx >= 0 {
		rawHref = rawHref[:idx]
	}
	rawHref = strings.TrimRight(rawHref, "/")
	if rawHref == "" {
		return "/"
	}
	return rawHref
}

func consoleNavigationResolvedTargets(query string) []consoleNavigationRouteHint {
	if !isConsoleNavigationIntent(query) {
		return nil
	}
	normalized := normalizeConsoleNavigationQuery(query)
	if normalized == "" {
		return nil
	}
	resolved := make([]resolvedConsoleNavigationTarget, 0, 4)
	seen := map[string]struct{}{}
	for _, route := range consoleNavigationRouteHints {
		route = normalizeConsoleNavigationRouteHint(route)
		href := strings.ToLower(route.Href)
		for _, position := range consoleRouteHrefIndexes(normalized, href) {
			key := route.Href + "\x00" + strconv.Itoa(position)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			resolved = append(resolved, resolvedConsoleNavigationTarget{Hint: route, Position: position})
		}
		for _, keyword := range consoleNavigationRouteKeywords(route) {
			keyword = normalizeConsoleNavigationQuery(keyword)
			if keyword == "" {
				continue
			}
			for _, position := range allStringIndexes(normalized, keyword) {
				if !consoleNavigationKeywordPositionAllowed(keyword, normalized, position) {
					continue
				}
				key := route.Href + "\x00" + strconv.Itoa(position)
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				resolved = append(resolved, resolvedConsoleNavigationTarget{Hint: route, Position: position})
			}
		}
	}
	sort.SliceStable(resolved, func(i, j int) bool {
		return resolved[i].Position < resolved[j].Position
	})
	targets := make([]consoleNavigationRouteHint, 0, len(resolved))
	for _, item := range resolved {
		if len(targets) > 0 && consoleNavigationLoadedHrefMatchesTarget(targets[len(targets)-1].Href, item.Hint.Href) {
			continue
		}
		targets = append(targets, item.Hint)
	}
	return targets
}

func consoleRouteHrefIndexes(haystack string, href string) []int {
	indexes := allStringIndexes(haystack, href)
	if href != "/console" || len(indexes) == 0 {
		return indexes
	}
	filtered := indexes[:0]
	for _, position := range indexes {
		next := position + len(href)
		if next < len(haystack) && haystack[next] == '/' {
			continue
		}
		filtered = append(filtered, position)
	}
	return filtered
}

func consoleNavigationKeywordPositionAllowed(keyword, normalized string, position int) bool {
	if keyword == "" || normalized == "" || position < 0 || position+len(keyword) > len(normalized) {
		return false
	}
	if consoleNavigationWorkspaceKeywordIsScopePhrase(keyword, normalized, position) {
		return false
	}
	if consoleNavigationKeywordHasExplicitPageCue(keyword) {
		return true
	}
	suffix := normalized[position+len(keyword):]
	for _, blockedSuffix := range []string{
		"\u540d\u79f0",
		"\u540d\u5b57",
		"id",
		"\u7f16\u53f7",
		"\u914d\u7f6e",
		"\u5185\u5bb9",
		"\u63cf\u8ff0",
		"\u7ed3\u679c",
		"\u8f93\u51fa",
		"\u7ed1\u5b9a",
		"\u6570\u91cf",
		"\u72b6\u6001",
	} {
		if strings.HasPrefix(suffix, blockedSuffix) {
			return false
		}
	}
	return true
}

func consoleNavigationWorkspaceKeywordIsScopePhrase(keyword, normalized string, position int) bool {
	keyword = strings.TrimSpace(keyword)
	if keyword != "工作空间" && keyword != "workspace" {
		return false
	}
	prefix := normalized[:position]
	suffix := normalized[position+len(keyword):]
	for _, pageCue := range []string{"页", "页面", "管理", " page", " module"} {
		if strings.HasPrefix(suffix, pageCue) {
			return false
		}
	}
	for _, scopePrefix := range []string{"当前", "本", "这个", "该", "所在", "当前 ", "current ", "this "} {
		if strings.HasSuffix(prefix, scopePrefix) {
			return true
		}
	}
	return false
}

func consoleNavigationKeywordHasExplicitPageCue(keyword string) bool {
	return containsAnySubstring(keyword, []string{
		"/console",
		"page",
		"list",
		"\u9875",
		"\u9875\u9762",
		"\u7ba1\u7406",
		"\u6a21\u5757",
		"\u9996\u9875",
		"\u4e3b\u9875",
		"\u63a7\u5236\u53f0",
	})
}

func allStringIndexes(haystack string, needle string) []int {
	if haystack == "" || needle == "" {
		return nil
	}
	indexes := []int{}
	offset := 0
	for {
		idx := strings.Index(haystack[offset:], needle)
		if idx < 0 {
			return indexes
		}
		position := offset + idx
		indexes = append(indexes, position)
		offset = position + len(needle)
		if offset >= len(haystack) {
			return indexes
		}
	}
}

func consoleNavigationRouteKeywords(route consoleNavigationRouteHint) []string {
	keywords := append([]string(nil), route.Keywords...)
	switch route.Href {
	case "/console":
		keywords = append(keywords, "\u9996\u9875", "\u4e3b\u9875", "\u63a7\u5236\u53f0\u9996\u9875")
	case "/console/work/chat":
		keywords = append(keywords, "\u5bf9\u8bdd", "\u804a\u5929", "\u4f1a\u8bdd", "\u5bf9\u8bdd\u9875", "\u804a\u5929\u9875")
	case "/console/work/image":
		keywords = append(keywords, "\u7ed8\u56fe", "\u56fe\u50cf\u751f\u6210", "\u56fe\u7247\u751f\u6210")
	case "/console/work/app":
		keywords = append(keywords, "\u5e94\u7528", "\u5e94\u7528\u9875", "\u5e94\u7528\u7ba1\u7406")
	case "/console/work/task":
		keywords = append(keywords, "\u5b9a\u65f6\u4efb\u52a1", "\u8ba1\u5212\u4efb\u52a1", "\u4efb\u52a1\u9875")
	case "/console/agents":
		keywords = append(keywords, "\u667a\u80fd\u4f53", "\u667a\u80fd\u4f53\u9875", "\u667a\u80fd\u4f53\u9875\u9762", "\u667a\u80fd\u4f53\u7ba1\u7406", "\u5de5\u4f5c\u6d41", "\u5de5\u4f5c\u6d41\u9875")
	case "/console/dataset":
		keywords = append(keywords, "\u77e5\u8bc6\u5e93", "\u77e5\u8bc6\u5e93\u9875", "\u6570\u636e\u96c6")
	case "/console/db":
		keywords = append(keywords, "\u6570\u636e\u5e93", "\u6570\u636e\u5e93\u9875", "\u6570\u636e\u8868")
	case "/console/files":
		keywords = append(keywords, "\u6587\u4ef6\u7ba1\u7406", "\u6587\u4ef6\u9875", "\u6587\u4ef6\u9875\u9762", "\u6587\u4ef6\u6a21\u5757")
	case "/console/prompts":
		keywords = append(keywords, "\u63d0\u793a\u8bcd", "\u63d0\u793a\u8bcd\u9875")
	case "/console/developer/content-parse":
		keywords = append(keywords, "\u6587\u4ef6\u8bc6\u522b", "\u5185\u5bb9\u89e3\u6790")
	case "/console/workspace":
		keywords = append(keywords, "\u5de5\u4f5c\u7a7a\u95f4")
	case "/console/settings":
		keywords = append(keywords, "\u7cfb\u7edf\u8bbe\u7f6e", "\u8bbe\u7f6e\u9875")
	}
	return keywords
}

func clientActionRouteHref(event map[string]interface{}) string {
	if len(event) == 0 {
		return ""
	}
	if href := normalizeConsoleNavigationGuardHref(stringFromAny(event["href"])); href != "" {
		return href
	}
	result := governanceMapFromAny(event["result"])
	for _, key := range []string{"observed_path", "href", "loaded_href", "target_page"} {
		if href := normalizeConsoleNavigationGuardHref(stringFromAny(result[key])); href != "" {
			return href
		}
	}
	return ""
}

func isConsoleNavigationIntent(query string) bool {
	normalized := normalizeConsoleNavigationQuery(query)
	if normalized == "" {
		return false
	}
	for _, marker := range []string{
		"带我去", "带我到", "打开", "跳转", "切换到", "切到", "进入", "前往", "导航到", "转到", "去到",
		"\u5e26\u6211\u53bb", "\u5e26\u6211\u5230", "\u6253\u5f00", "\u8df3\u8f6c", "\u5207\u6362\u5230", "\u5207\u5230", "\u8fdb\u5165", "\u524d\u5f80", "\u5bfc\u822a", "\u5bfc\u822a\u5230", "\u8f6c\u5230", "\u53bb\u5230", "\u4f9d\u6b21\u5bfc\u822a",
		"go to", "open", "switch to", "navigate to", "take me to", "show me",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func isContinuationIntent(query string) bool {
	normalized := normalizeConsoleNavigationQuery(query)
	if normalized == "" {
		return false
	}
	if isStagedContinuationInstruction(normalized) {
		return false
	}
	if isWaitForFutureContinuationInstruction(normalized) {
		return false
	}
	if isCanonicalChineseContinuationIntent(normalized) {
		return true
	}
	runeCount := len([]rune(normalized))
	if runeCount > 24 {
		return isLongContinuationIntent(normalized, runeCount)
	}
	for _, exact := range []string{
		"继续",
		"继续吧",
		"继续处理",
		"继续执行",
		"接着",
		"接着做",
		"下一步",
		"继续上一步",
		"继续刚才的任务",
		"\u7ee7\u7eed",
		"\u7ee7\u7eed\u5427",
		"\u7ee7\u7eed\u5904\u7406",
		"\u7ee7\u7eed\u6267\u884c",
		"\u63a5\u7740",
		"\u63a5\u7740\u505a",
		"\u4e0b\u4e00\u6b65",
		"\u8fdb\u884c\u5904\u7406",
		"\u90a3\u5c31\u505a",
		"\u90a3\u5c31\u5904\u7406",
		"\u90a3\u5c31\u6267\u884c",
		"\u5c31\u8fd9\u4e48\u505a",
		"\u6309\u8fd9\u4e2a\u505a",
		"\u6309\u8fd9\u4e2a\u5904\u7406",
		"\u6309\u65b9\u6848\u505a",
		"\u6309\u65b9\u6848\u5904\u7406",
		"\u5f00\u59cb\u5904\u7406",
		"\u5f00\u59cb\u5904\u7406\u5427",
		"\u5f00\u59cb\u6267\u884c",
		"\u5f00\u59cb\u6267\u884c\u5427",
		"\u53ef\u4ee5\u5f00\u59cb",
		"\u7ee7\u7eed\u4e0a\u4e00\u6b65",
		"\u7ee7\u7eed\u521a\u624d\u7684\u4efb\u52a1",
		"continue",
		"continue please",
		"go on",
		"keep going",
		"next",
		"next step",
		"proceed",
	} {
		if normalized == exact {
			return true
		}
	}
	for _, marker := range []string{"继续", "接着", "下一步", "continue", "go on", "keep going", "next step"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func isCanonicalChineseContinuationIntent(normalized string) bool {
	if normalized == "" {
		return false
	}
	runeCount := len([]rune(normalized))
	exact := map[string]struct{}{
		"\u7ee7\u7eed":                               {},
		"\u7ee7\u7eed\u5427":                         {},
		"\u7ee7\u7eed\u5904\u7406":                   {},
		"\u7ee7\u7eed\u6267\u884c":                   {},
		"\u63a5\u7740":                               {},
		"\u63a5\u7740\u505a":                         {},
		"\u4e0b\u4e00\u6b65":                         {},
		"\u8fdb\u884c\u5904\u7406":                   {},
		"\u90a3\u5c31\u505a":                         {},
		"\u90a3\u5c31\u5904\u7406":                   {},
		"\u90a3\u5c31\u6267\u884c":                   {},
		"\u5c31\u8fd9\u4e48\u505a":                   {},
		"\u6309\u8fd9\u4e2a\u505a":                   {},
		"\u6309\u8fd9\u4e2a\u5904\u7406":             {},
		"\u6309\u65b9\u6848\u505a":                   {},
		"\u6309\u65b9\u6848\u5904\u7406":             {},
		"\u5f00\u59cb\u5904\u7406":                   {},
		"\u5f00\u59cb\u5904\u7406\u5427":             {},
		"\u5f00\u59cb\u6267\u884c":                   {},
		"\u5f00\u59cb\u6267\u884c\u5427":             {},
		"\u53ef\u4ee5\u5f00\u59cb":                   {},
		"\u7ee7\u7eed\u4e0a\u4e00\u6b65":             {},
		"\u7ee7\u7eed\u521a\u624d\u7684\u4efb\u52a1": {},
	}
	if _, ok := exact[normalized]; ok {
		return true
	}
	if runeCount <= 24 {
		return containsAnySubstring(normalized, []string{
			"\u7ee7\u7eed",
			"\u63a5\u7740",
			"\u4e0b\u4e00\u6b65",
			"\u90a3\u5c31\u505a",
			"\u90a3\u5c31\u5904\u7406",
			"\u90a3\u5c31\u6267\u884c",
			"\u5c31\u8fd9\u4e48\u505a",
			"\u6309\u8fd9\u4e2a\u505a",
			"\u6309\u8fd9\u4e2a\u5904\u7406",
			"\u6309\u65b9\u6848\u505a",
			"\u6309\u65b9\u6848\u5904\u7406",
		})
	}
	return false
}

func isWaitForFutureContinuationInstruction(normalized string) bool {
	if normalized == "" {
		return false
	}
	compact := strings.NewReplacer(
		" ", "",
		"\u201c", "",
		"\u201d", "",
		"\"", "",
		"'", "",
		"`", "",
	).Replace(normalized)
	return containsAnySubstring(compact, []string{
		"\u7b49\u5f85\u7ee7\u7eed",
		"\u7b49\u7ee7\u7eed",
		"\u7b49\u5f85\u7528\u6237\u7ee7\u7eed",
		"\u7b49\u7528\u6237\u7ee7\u7eed",
	}) || containsAnySubstring(normalized, []string{
		"wait for continue",
		"wait for the user to continue",
		"wait for user continue",
	})
}

func stagedExecutionScopedParts(parts *chatRequestParts) (*chatRequestParts, bool, bool) {
	if parts == nil {
		return nil, false, false
	}
	if isContinuationIntent(parts.Query) {
		if current, ok := stagedContinuationResumeQuery(parts); ok {
			return chatRequestPartsWithQuery(parts, current), false, true
		}
		return parts, false, false
	}
	current, _, ok := splitStagedContinuationQuery(parts.Query)
	if ok {
		if strings.TrimSpace(current) == "" {
			return parts, true, false
		}
		return chatRequestPartsWithQuery(parts, current), true, false
	}
	return parts, false, false
}

func chatRequestPartsWithQuery(parts *chatRequestParts, query string) *chatRequestParts {
	if parts == nil {
		return nil
	}
	next := *parts
	next.Query = strings.TrimSpace(query)
	return &next
}

func stagedContinuationResumeQuery(parts *chatRequestParts) (string, bool) {
	goal := recentOperationPlanOriginalGoal(parts)
	if goal == "" {
		return "", false
	}
	_, deferred, ok := splitStagedContinuationQuery(goal)
	if !ok || strings.TrimSpace(deferred) == "" {
		return "", false
	}
	return stagedContinuationDeferredExecutionQuery(deferred), true
}

func splitStagedContinuationQuery(query string) (string, string, bool) {
	raw := strings.TrimSpace(query)
	if raw == "" {
		return "", "", false
	}
	lower := strings.ToLower(raw)
	for _, marker := range stagedContinuationRawMarkers() {
		if marker == "" {
			continue
		}
		if idx := strings.Index(lower, marker); idx >= 0 {
			return strings.TrimSpace(raw[:idx]), strings.TrimSpace(raw[idx:]), true
		}
	}
	normalized := normalizeConsoleNavigationQuery(raw)
	if !isStagedContinuationInstruction(normalized) {
		return raw, "", false
	}
	if idx := strings.Index(lower, "\u7ee7\u7eed"); idx >= 0 {
		return strings.TrimSpace(raw[:idx]), strings.TrimSpace(raw[idx:]), true
	}
	if idx := strings.Index(lower, "continue"); idx >= 0 {
		return strings.TrimSpace(raw[:idx]), strings.TrimSpace(raw[idx:]), true
	}
	return raw, "", true
}

func stagedContinuationRawMarkers() []string {
	return []string{
		"\u7b49\u5f85\u6211\u8bf4",
		"\u7b49\u6211\u8bf4",
		"\u7b49\u6211\u8f93\u5165",
		"\u7b49\u6211\u53d1",
		"\u5f53\u6211\u8bf4",
		"\u6211\u8bf4\u7ee7\u7eed\u540e",
		"\u6211\u8bf4\u4e86\u7ee7\u7eed",
		"\u8bf4\u7ee7\u7eed\u540e",
		"wait for me to say continue",
		"wait until i say continue",
		"after i say continue",
		"when i say continue",
		"once i say continue",
		"wait for my continue",
	}
}

func stagedContinuationDeferredExecutionQuery(deferred string) string {
	deferred = strings.TrimSpace(deferred)
	if deferred == "" {
		return ""
	}
	for _, separator := range []string{"\uff1a", ":"} {
		if idx := strings.Index(deferred, separator); idx >= 0 && idx+len(separator) < len(deferred) {
			return strings.TrimSpace(deferred[idx+len(separator):])
		}
	}
	replacer := strings.NewReplacer(
		"\u7b49\u5f85\u6211\u8bf4", "",
		"\u7b49\u6211\u8bf4", "",
		"\u6211\u8bf4\u201c\u7ee7\u7eed\u201d\u540e", "",
		"\u6211\u8bf4\u2018\u7ee7\u7eed\u2019\u540e", "",
		"\u6211\u8bf4\u7ee7\u7eed\u540e", "",
		"\u6211\u8bf4\u4e86\u7ee7\u7eed", "",
		"\u8bf4\u7ee7\u7eed\u540e", "",
		"\u201c\u7ee7\u7eed\u201d", "",
		"\u2018\u7ee7\u7eed\u2019", "",
		"\u7ee7\u7eed", "",
		"\u540e\u518d\u6267\u884c", "",
		"\u540e\u518d", "",
	)
	cleaned := strings.TrimSpace(replacer.Replace(deferred))
	cleaned = strings.TrimLeftFunc(cleaned, func(r rune) bool {
		if unicode.IsSpace(r) {
			return true
		}
		switch r {
		case '.', ',', ';', ':', '\uff0c', '\u3002', '\uff1b', '\uff1a', '\u3001':
			return true
		default:
			return false
		}
	})
	return strings.TrimSpace(cleaned)
}

func stagedContinuationToolCallGuardResult(parts *chatRequestParts, req skillloop.ToolCallGuardRequest) (skillloop.FinalAnswerGuardResult, bool) {
	if parts == nil {
		return skillloop.FinalAnswerGuardResult{}, false
	}
	if isConsoleNavigatorNavigateTool(req.SkillID, req.ToolName) {
		href := skillToolCallArgumentString(req.Arguments, "href")
		if !consoleNavigationHrefAllowedForCurrentScope(href, consoleNavigationResolvedTargets(parts.Query)) {
			return stagedContinuationDeferredToolGuardResult(req.SkillID, req.ToolName), true
		}
		return skillloop.FinalAnswerGuardResult{}, false
	}
	if isKnownArtifactGeneratorToolCall(req.SkillID, req.ToolName) &&
		!isTemporaryFileGenerateIntent(parts.Query) && !isManagedFileCreateIntent(parts.Query) {
		return stagedContinuationDeferredToolGuardResult(req.SkillID, req.ToolName), true
	}
	if isFileManagerSaveToolCall(req.SkillID, req.ToolName) && !isManagedFileCreateIntent(parts.Query) {
		return stagedContinuationDeferredToolGuardResult(req.SkillID, req.ToolName), true
	}
	if isFileManagerDeleteToolCall(req.SkillID, req.ToolName) && !isFileDeleteIntent(parts.Query) {
		return stagedContinuationDeferredToolGuardResult(req.SkillID, req.ToolName), true
	}
	return skillloop.FinalAnswerGuardResult{}, false
}

func consoleNavigationHrefAllowedForCurrentScope(href string, targets []consoleNavigationRouteHint) bool {
	href = normalizeConsoleNavigationGuardHref(href)
	if href == "" {
		return false
	}
	for _, target := range targets {
		if consoleNavigationLoadedHrefMatchesTarget(href, target.Href) {
			return true
		}
	}
	return false
}

func stagedContinuationDeferredToolGuardResult(skillID, toolName string) skillloop.FinalAnswerGuardResult {
	message := strings.Join([]string{
		"The user's message contains a staged continuation.",
		"Only execute instructions before the continue marker in this turn.",
		"Wait for the user to say continue before running deferred navigation, file generation, file management, or deletion steps.",
	}, " ")
	return skillloop.FinalAnswerGuardResult{
		SkillID:       strings.TrimSpace(skillID),
		ToolName:      strings.TrimSpace(toolName),
		Message:       message,
		SystemMessage: message,
	}
}

func isStagedContinuationInstruction(normalized string) bool {
	if normalized == "" {
		return false
	}
	compact := strings.NewReplacer(
		" ", "",
		"\u201c", "",
		"\u201d", "",
		"\u2018", "",
		"\u2019", "",
		"\"", "",
		"'", "",
		"`", "",
		"\u300c", "",
		"\u300d", "",
		"\u300e", "",
		"\u300f", "",
		"\u300a", "",
		"\u300b", "",
	).Replace(normalized)
	if containsAnySubstring(compact, []string{
		"\u7b49\u5f85\u6211\u8bf4\u7ee7\u7eed",
		"\u7b49\u6211\u8bf4\u7ee7\u7eed",
		"\u7b49\u6211\u8f93\u5165\u7ee7\u7eed",
		"\u7b49\u6211\u53d1\u7ee7\u7eed",
		"\u6211\u8bf4\u7ee7\u7eed\u540e",
		"\u8bf4\u7ee7\u7eed\u540e",
		"\u5f53\u6211\u8bf4\u7ee7\u7eed",
		"\u6211\u8bf4\u4e86\u7ee7\u7eed\u518d",
		"\u6211\u8bf4\u7ee7\u7eed\u518d",
		"\u7b49\u5f85\u7ee7\u7eed\u6307\u4ee4",
	}) {
		return true
	}
	return containsAnySubstring(normalized, []string{
		"wait for me to say continue",
		"wait until i say continue",
		"after i say continue",
		"when i say continue",
		"once i say continue",
		"wait for my continue",
	})
}

func isLongContinuationIntent(normalized string, runeCount int) bool {
	if normalized == "" {
		return false
	}
	if hasContinuationPrefix(normalized) {
		return true
	}
	continuationMarkerFound := false
	for _, marker := range []string{"\u7ee7\u7eed", "\u63a5\u7740", "\u4e0b\u4e00\u6b65", "continue", "go on", "keep going", "next step"} {
		if strings.Contains(normalized, marker) {
			continuationMarkerFound = true
			break
		}
	}
	if !continuationMarkerFound {
		return false
	}
	if runeCount <= 64 {
		return true
	}
	return containsAnySubstring(normalized, []string{
		"\u4efb\u52a1\u6807\u8bb0",
		"task marker",
		"task tag",
		"smoke-",
	})
}

func hasContinuationPrefix(normalized string) bool {
	normalized = strings.TrimSpace(normalized)
	if normalized == "" {
		return false
	}
	for _, prefix := range []string{
		"\u7ee7\u7eed",
		"\u63a5\u7740",
		"\u4e0b\u4e00\u6b65",
		"continue",
		"go on",
		"keep going",
		"next step",
	} {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return false
}

func normalizeConsoleNavigationQuery(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("，", " ", "。", " ", "？", " ", "?", " ", "！", " ", "!", " ", ",", " ", ".", " ")
	value = replacer.Replace(value)
	return strings.Join(strings.Fields(value), " ")
}

func skillToolCallFileIDs(arguments map[string]interface{}) []string {
	seen := map[string]struct{}{}
	out := []string{}
	add := func(value interface{}) {
		switch typed := value.(type) {
		case []string:
			for _, item := range typed {
				if id := strings.TrimSpace(item); id != "" {
					if _, ok := seen[id]; !ok {
						seen[id] = struct{}{}
						out = append(out, id)
					}
				}
			}
		case []interface{}:
			for _, item := range typed {
				if id := strings.TrimSpace(stringFromAny(item)); id != "" {
					if _, ok := seen[id]; !ok {
						seen[id] = struct{}{}
						out = append(out, id)
					}
				}
			}
		default:
			if id := strings.TrimSpace(stringFromAny(value)); id != "" {
				if _, ok := seen[id]; !ok {
					seen[id] = struct{}{}
					out = append(out, id)
				}
			}
		}
	}
	add(arguments["file_id"])
	add(arguments["file_ids"])
	return out
}

func consoleFilesGuardTargetFileIDs(targets []map[string]interface{}) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, target := range targets {
		id := strings.TrimSpace(stringFromAny(target["file_id"]))
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func consoleFilesGuardTargetArgumentHint(targets []map[string]interface{}, toolName string) string {
	type targetRef struct {
		Name   string `json:"name,omitempty"`
		FileID string `json:"file_id"`
	}
	refs := []targetRef{}
	seen := map[string]struct{}{}
	for _, target := range targets {
		id := strings.TrimSpace(stringFromAny(target["file_id"]))
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		refs = append(refs, targetRef{
			Name:   strings.TrimSpace(stringFromAny(target["name"])),
			FileID: id,
		})
	}
	if len(refs) == 0 {
		return ""
	}
	payload := map[string]interface{}{
		"skill_id":                             skills.SkillFileReader,
		"resolved_targets_for_tool_arguments":  refs,
		"tool_argument_visibility_restriction": "internal_only_do_not_reveal_to_user",
	}
	if toolName = strings.TrimSpace(toolName); toolName != "" {
		payload["tool_name"] = toolName
	}
	if len(refs) == 1 {
		payload["arguments"] = map[string]interface{}{"file_id": refs[0].FileID}
	} else {
		payload["arguments"] = map[string]interface{}{"file_ids": consoleFilesGuardTargetFileIDs(targets)}
		payload["call_instruction"] = "Call this tool once per resolved target if the tool schema accepts only a single file_id."
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return "Resolved internal target JSON for tool arguments only; do not reveal internal IDs to the user: " + string(encoded)
}

func consoleFilesGuardTargetSummary(targets []map[string]interface{}) string {
	if len(targets) == 0 {
		return "the resolved visible file"
	}
	parts := make([]string, 0, len(targets))
	for _, target := range targets {
		name := strings.TrimSpace(stringFromAny(target["name"]))
		if name != "" {
			parts = append(parts, name)
		}
	}
	if len(parts) == 0 {
		return "the resolved visible file"
	}
	return strings.Join(parts, ", ")
}

func skillIDEnabled(skillIDs []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	if target == "" {
		return false
	}
	for _, raw := range skillIDs {
		if strings.EqualFold(strings.TrimSpace(raw), target) {
			return true
		}
	}
	return false
}

func consoleFilesPromptVisibleFiles(parts *chatRequestParts) []map[string]interface{} {
	if parts == nil {
		return nil
	}
	files := visibleFileResources(parts.RawOperationContext)
	if len(files) == 0 {
		files = visibleFileResources(parts.OperationContext)
	}
	out := make([]map[string]interface{}, 0, min(len(files), 10))
	for idx, file := range files {
		if idx >= 10 {
			break
		}
		item := map[string]interface{}{
			"visible_index": file.VisibleIndex,
			"file_id":       file.ID,
			"name":          file.Title,
			"extension":     file.Extension,
			"mime_type":     file.MimeType,
			"selected":      file.Selected,
		}
		if file.FileTypeRank > 0 {
			item["file_type_rank"] = file.FileTypeRank
		}
		if file.ExtensionRank > 0 {
			item["extension_rank"] = file.ExtensionRank
		}
		if strings.TrimSpace(file.FileType) != "" {
			item["file_type"] = strings.TrimSpace(file.FileType)
		}
		if strings.TrimSpace(file.WorkspaceID) != "" {
			item["workspace_id"] = strings.TrimSpace(file.WorkspaceID)
		}
		out = append(out, item)
	}
	return out
}

func consoleAgentsPromptVisibleAgents(parts *chatRequestParts) []map[string]interface{} {
	if parts == nil {
		return nil
	}
	agents := visibleAgentResources(parts.RawOperationContext)
	if len(agents) == 0 {
		agents = visibleAgentResources(parts.OperationContext)
	}
	out := make([]map[string]interface{}, 0, min(len(agents), 10))
	for idx, agent := range agents {
		if idx >= 10 {
			break
		}
		item := map[string]interface{}{
			"visible_index": agent.VisibleIndex,
			"type":          "agent",
			"asset_type":    "agent",
			"agent_id":      agent.ID,
			"id":            agent.ID,
			"name":          agent.Title,
			"href":          agent.Href,
		}
		if strings.TrimSpace(agent.Description) != "" {
			item["description"] = strings.TrimSpace(agent.Description)
		}
		if strings.TrimSpace(agent.AgentType) != "" {
			item["agent_type"] = strings.TrimSpace(agent.AgentType)
		}
		if strings.TrimSpace(agent.WorkspaceID) != "" {
			item["workspace_id"] = strings.TrimSpace(agent.WorkspaceID)
		}
		if agent.Selected {
			item["selected"] = true
		}
		if agent.CanEdit {
			item["can_edit"] = true
		}
		out = append(out, item)
	}
	return out
}

func consoleFilesPromptRecentGeneratedFiles(parts *chatRequestParts) []map[string]interface{} {
	if parts == nil || len(parts.RecentGeneratedArtifacts) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, min(len(parts.RecentGeneratedArtifacts), 5))
	for idx, artifact := range parts.RecentGeneratedArtifacts {
		if idx >= 5 {
			break
		}
		toolFileID := strings.TrimSpace(firstNonEmptyString(artifact["tool_file_id"], artifact["file_id"]))
		if toolFileID == "" {
			continue
		}
		item := map[string]interface{}{
			"tool_file_id": toolFileID,
		}
		if filename := strings.TrimSpace(firstNonEmptyString(artifact["filename"], artifact["name"])); filename != "" {
			item["filename"] = filename
		}
		for _, key := range []string{"artifact_id", "status", "lifecycle", "extension", "mime_type", "file_type", "skill_id", "tool_name", "source_message_id"} {
			if value := strings.TrimSpace(stringFromAny(artifact[key])); value != "" {
				item[key] = value
			}
		}
		out = append(out, item)
	}
	return out
}

func consoleFilesPromptResolvedTargets(parts *chatRequestParts) []map[string]interface{} {
	refs := plannerResourceRefsFromConsoleFilesQuery(parts)
	if len(refs) == 0 {
		return nil
	}
	result := resolveChatResourceRefs(parts, refs)
	if !allResourceRefsResolved(result.Results) || len(result.FileIDs) == 0 {
		return nil
	}
	namesByID := map[string]string{}
	for _, file := range append(visibleFileResources(parts.RawOperationContext), visibleFileResources(parts.OperationContext)...) {
		if file.ID != "" && file.Title != "" {
			namesByID[file.ID] = file.Title
		}
	}
	for _, resource := range result.Resources {
		if strings.TrimSpace(resource.ID) != "" && strings.TrimSpace(resource.Name) != "" {
			namesByID[strings.TrimSpace(resource.ID)] = strings.TrimSpace(resource.Name)
		}
	}
	out := make([]map[string]interface{}, 0, len(result.FileIDs))
	for _, id := range result.FileIDs {
		item := map[string]interface{}{"file_id": id}
		if name := namesByID[id]; name != "" {
			item["name"] = name
		}
		out = append(out, item)
	}
	return out
}

func agentWorkflowAvailableBindingsMessage(bindings []AgentWorkflowBinding) (adapter.Message, bool) {
	items := agentWorkflowPromptBindings(bindings)
	if len(items) == 0 {
		return adapter.Message{}, false
	}
	payload, err := json.Marshal(map[string]interface{}{"available_workflows": items})
	if err != nil {
		return adapter.Message{}, false
	}
	content := strings.Join([]string{
		"The current Agent can call these bound workflows through the agent-workflow skill.",
		"Use this injected available_workflows list first when selecting a workflow binding. Call list_agent_workflows only if this list is missing, ambiguous, or stale.",
		"Never invent workflow IDs or pass workflow_id/agent_id. Call run_agent_workflow with a binding_id from available_workflows.",
		"For single-input or conversational workflows, pass the user's current request in inputs.query unless the binding's input_schema, required_inputs, or default_input_key says otherwise.",
		"Available workflows JSON: " + string(payload),
	}, "\n")
	return adapter.Message{Role: "system", Content: content}, true
}

func agentWorkflowPromptBindings(bindings []AgentWorkflowBinding) []map[string]interface{} {
	normalized := copyAgentWorkflowBindings(bindings)
	out := make([]map[string]interface{}, 0, len(normalized))
	seen := map[string]struct{}{}
	for _, binding := range normalized {
		if strings.TrimSpace(binding.BindingID) == "" {
			continue
		}
		if _, exists := seen[binding.BindingID]; exists {
			continue
		}
		seen[binding.BindingID] = struct{}{}
		defaultInputKey := agentWorkflowDefaultInputKey(binding)
		requiredInputs := agentWorkflowRequiredInputs(binding)
		out = append(out, map[string]interface{}{
			"binding_id":        binding.BindingID,
			"label":             binding.Label,
			"description":       binding.Description,
			"agent_type":        binding.AgentType,
			"version_strategy":  agentWorkflowVersionStrategy(binding.VersionStrategy),
			"timeout_seconds":   agentWorkflowTimeoutSeconds(binding.TimeoutSeconds),
			"input_schema":      agentWorkflowInputSchema(binding, requiredInputs),
			"required_inputs":   requiredInputs,
			"default_input_key": defaultInputKey,
			"start_inputs":      binding.StartInputs,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.Compare(fmt.Sprint(out[i]["binding_id"]), fmt.Sprint(out[j]["binding_id"])) < 0
	})
	return out
}

func agentWorkflowInputSchema(binding AgentWorkflowBinding, requiredInputs []string) map[string]interface{} {
	if len(binding.StartInputs) > 0 {
		properties := map[string]interface{}{}
		for _, input := range binding.StartInputs {
			variable := strings.TrimSpace(input.Variable)
			if variable == "" {
				continue
			}
			description := strings.TrimSpace(input.Label)
			if description == "" {
				description = "Workflow start input."
			}
			properties[variable] = map[string]interface{}{
				"type":        agentWorkflowJSONSchemaType(input.Type),
				"description": description,
			}
		}
		return map[string]interface{}{
			"type":                 "object",
			"properties":           properties,
			"required":             requiredInputs,
			"additionalProperties": true,
		}
	}
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "The user's current request or instruction to pass into the workflow.",
			},
		},
		"required":             []string{"query"},
		"additionalProperties": true,
	}
}

func agentWorkflowRequiredInputs(binding AgentWorkflowBinding) []string {
	if len(binding.RequiredInputs) > 0 {
		allowed := map[string]struct{}{}
		for _, input := range binding.StartInputs {
			if variable := strings.TrimSpace(input.Variable); variable != "" {
				allowed[variable] = struct{}{}
			}
		}
		out := make([]string, 0, len(binding.RequiredInputs))
		seen := map[string]struct{}{}
		for _, item := range binding.RequiredInputs {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if len(allowed) > 0 {
				if _, ok := allowed[item]; !ok {
					continue
				}
			}
			if _, exists := seen[item]; exists {
				continue
			}
			seen[item] = struct{}{}
			out = append(out, item)
		}
		if len(out) > 0 {
			return out
		}
	}
	out := make([]string, 0, len(binding.StartInputs))
	for _, input := range binding.StartInputs {
		if input.Required && strings.TrimSpace(input.Variable) != "" {
			out = append(out, strings.TrimSpace(input.Variable))
		}
	}
	if len(out) > 0 {
		return out
	}
	if len(binding.StartInputs) == 0 {
		return []string{"query"}
	}
	return []string{}
}

func agentWorkflowDefaultInputKey(binding AgentWorkflowBinding) string {
	key := strings.TrimSpace(binding.DefaultInputKey)
	if key != "" && agentWorkflowStartInputExists(binding.StartInputs, key) {
		return key
	}
	required := agentWorkflowRequiredInputs(binding)
	if len(required) == 1 {
		return required[0]
	}
	if agentWorkflowStartInputExists(binding.StartInputs, "query") {
		return "query"
	}
	if len(binding.StartInputs) == 1 {
		return strings.TrimSpace(binding.StartInputs[0].Variable)
	}
	return "query"
}

func agentWorkflowStartInputExists(inputs []AgentWorkflowStartInput, key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	for _, input := range inputs {
		if strings.TrimSpace(input.Variable) == key {
			return true
		}
	}
	return false
}

func agentWorkflowJSONSchemaType(inputType string) string {
	switch strings.ToLower(strings.TrimSpace(inputType)) {
	case "datetime", "date-time":
		return "string"
	case "number", "integer":
		return "number"
	case "boolean", "bool":
		return "boolean"
	case "object":
		return "object"
	case "array":
		return "array"
	default:
		return "string"
	}
}

func agentWorkflowVersionStrategy(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "latest_published"
	}
	return value
}

func agentWorkflowTimeoutSeconds(value int) int {
	if value <= 0 {
		return 600
	}
	if value < 30 {
		return 30
	}
	if value > 1800 {
		return 1800
	}
	return value
}

func workflowConversationHistoryFromPrepared(prepared *PreparedChat) []map[string]interface{} {
	if prepared == nil || prepared.LLMRequest == nil || len(prepared.LLMRequest.Messages) == 0 {
		return nil
	}
	messages := prepared.LLMRequest.Messages
	lastUserIndex := -1
	for idx := len(messages) - 1; idx >= 0; idx-- {
		if strings.EqualFold(strings.TrimSpace(messages[idx].Role), "user") {
			lastUserIndex = idx
			break
		}
	}
	out := make([]map[string]interface{}, 0, len(messages))
	for idx, message := range messages {
		if idx == lastUserIndex {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(message.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(messageContentText(message.Content))
		if content == "" {
			continue
		}
		out = append(out, map[string]interface{}{
			"role":    role,
			"content": content,
		})
	}
	return out
}

func messageContentText(content interface{}) string {
	switch typed := content.(type) {
	case string:
		return typed
	case []adapter.MessageContentPart:
		var builder strings.Builder
		for _, part := range typed {
			if strings.TrimSpace(part.Text) == "" {
				continue
			}
			if builder.Len() > 0 {
				builder.WriteString("\n")
			}
			builder.WriteString(part.Text)
		}
		return builder.String()
	case []interface{}:
		var builder strings.Builder
		for _, raw := range typed {
			part, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			text := strings.TrimSpace(fmt.Sprint(part["text"]))
			if text == "" || text == "<nil>" {
				continue
			}
			if builder.Len() > 0 {
				builder.WriteString("\n")
			}
			builder.WriteString(text)
		}
		return builder.String()
	default:
		return ""
	}
}

func copyAgentDatabaseBindings(input []AgentDatabaseBinding) []AgentDatabaseBinding {
	out := make([]AgentDatabaseBinding, 0, len(input))
	for _, binding := range input {
		if strings.TrimSpace(binding.DataSourceID) == "" || len(binding.TableIDs) == 0 {
			continue
		}
		out = append(out, AgentDatabaseBinding{
			DataSourceID:     strings.TrimSpace(binding.DataSourceID),
			TableIDs:         append([]string(nil), binding.TableIDs...),
			WritableTableIDs: append([]string(nil), binding.WritableTableIDs...),
		})
	}
	return out
}

func copyAgentWorkflowBindings(input []AgentWorkflowBinding) []AgentWorkflowBinding {
	out := make([]AgentWorkflowBinding, 0, len(input))
	for _, binding := range input {
		if strings.TrimSpace(binding.BindingID) == "" || strings.TrimSpace(binding.AgentID) == "" || strings.TrimSpace(binding.WorkflowID) == "" {
			continue
		}
		out = append(out, AgentWorkflowBinding{
			BindingID:       strings.TrimSpace(binding.BindingID),
			Label:           strings.TrimSpace(binding.Label),
			Description:     strings.TrimSpace(binding.Description),
			AgentID:         strings.TrimSpace(binding.AgentID),
			WorkflowID:      strings.TrimSpace(binding.WorkflowID),
			AgentType:       strings.TrimSpace(binding.AgentType),
			VersionStrategy: strings.TrimSpace(binding.VersionStrategy),
			VersionUUID:     strings.TrimSpace(binding.VersionUUID),
			TimeoutSeconds:  binding.TimeoutSeconds,
			StartInputs:     copyAgentWorkflowStartInputs(binding.StartInputs),
			RequiredInputs:  append([]string(nil), binding.RequiredInputs...),
			DefaultInputKey: strings.TrimSpace(binding.DefaultInputKey),
		})
	}
	return out
}

func copyAgentWorkflowStartInputs(input []AgentWorkflowStartInput) []AgentWorkflowStartInput {
	out := make([]AgentWorkflowStartInput, 0, len(input))
	for _, item := range input {
		variable := strings.TrimSpace(item.Variable)
		if variable == "" {
			continue
		}
		out = append(out, AgentWorkflowStartInput{
			Variable:            variable,
			Label:               strings.TrimSpace(item.Label),
			Type:                strings.TrimSpace(item.Type),
			Required:            item.Required,
			Default:             item.Default,
			DefaultDateTimeMode: strings.TrimSpace(item.DefaultDateTimeMode),
		})
	}
	return out
}

func mergeUsage(current *adapter.Usage, next *adapter.Usage) *adapter.Usage {
	if next == nil {
		return current
	}
	if current == nil {
		cloned := *next
		return &cloned
	}
	current.PromptTokens += next.PromptTokens
	current.CompletionTokens += next.CompletionTokens
	current.TotalTokens += next.TotalTokens
	return current
}

func cloneChatRequest(source *adapter.ChatRequest) *adapter.ChatRequest {
	if source == nil {
		return &adapter.ChatRequest{}
	}
	cloned := *source
	cloned.Messages = append([]adapter.Message{}, source.Messages...)
	cloned.Stop = append([]string{}, source.Stop...)
	if source.AdditionalParameters != nil {
		cloned.AdditionalParameters = copyStringAnyMap(source.AdditionalParameters)
	}
	if source.LogitBias != nil {
		cloned.LogitBias = make(map[string]float64, len(source.LogitBias))
		for key, value := range source.LogitBias {
			cloned.LogitBias[key] = value
		}
	}
	return &cloned
}

func agenticSkillLoopSystemMessage() adapter.Message {
	return skillloop.AgenticSkillLoopSystemMessage()
}
