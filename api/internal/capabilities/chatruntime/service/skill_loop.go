package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/pkg/logger"
)

var agentManagementSkillIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

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
	return s.runPreparedSkillLoop(ctx, persistCtx, prepared, onChunk, onEvent)
}

func (s *service) runPreparedSkillLoop(
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
			markPreparedCurrentPageContextStale(prepared, trace)
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
	preferExplicitFinalAnswer := skillLoopPrefersExplicitFinalAnswer(prepared)
	if prepared.Message.Metadata == nil {
		prepared.Message.Metadata = map[string]interface{}{}
	}
	if preferExplicitFinalAnswer {
		prepared.Message.Metadata["final_answer_protocol"] = skills.MetaToolFinalAnswer + "_preferred"
	} else {
		prepared.Message.Metadata["final_answer_protocol"] = "assistant_content"
	}
	answer, usage, err := runner.Run(ctx, skillloop.RunRequest{
		Prepared:                       loopPrepared,
		Resolved:                       resolved,
		ExecutionContext:               s.skillExecutionContext(prepared),
		PreferExplicitFinalAnswer:      preferExplicitFinalAnswer,
		SuppressInitialNaturalProgress: prepared.SuppressInitialNaturalProgress,
		AdditionalSystemMessages:       skillLoopAdditionalSystemMessagesForResolved(prepared, resolved),
		RuntimeStateSnapshot:           skillLoopRuntimeStateSnapshot(prepared),
		CurrentMetadata:                skillLoopCurrentMetadata(prepared),
		OnTerminalStateGuardDecision:   skillLoopTerminalStateGuardDecision(prepared),
		OnTerminalCompletion:           skillLoopTerminalCompletionResult(prepared),
		OnChunk:                        onChunk,
	})
	if err != nil && strings.TrimSpace(answer) != "" {
		s.persistPartialSkillLoopAnswerBestEffort(persistCtx, prepared, answer, usage)
	}
	return answer, usage, err
}

func (s *service) persistPartialSkillLoopAnswerBestEffort(ctx context.Context, prepared *PreparedChat, answer string, usage *adapter.Usage) {
	if s == nil || s.repos == nil || s.repos.Message == nil || prepared == nil || prepared.Message == nil || strings.TrimSpace(answer) == "" {
		return
	}
	metadata := copyStringAnyMap(prepared.Message.Metadata)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadata["usage"] = usageMetadata(usage)
	metadata["system_prompt_version"] = systemPromptVersion
	if err := s.repos.Message.UpdatePartialAnswer(ctx, prepared.Message.ID, answer, metadata); err != nil {
		logger.WarnContext(ctx, "failed to persist partial aichat skill-loop answer", "message_id", prepared.Message.ID.String(), err)
		return
	}
	prepared.Message.Answer = answer
	prepared.Message.Metadata = metadata
}

func skillLoopTerminalStateGuardDecision(prepared *PreparedChat) func(skillloop.TerminalStateGuardDecisionRecord) {
	return func(decision skillloop.TerminalStateGuardDecisionRecord) {
		if prepared == nil || prepared.Message == nil {
			return
		}
		metadata := prepared.Message.Metadata
		if metadata == nil {
			metadata = map[string]interface{}{}
		}
		records := mapSliceFromAny(metadata["terminal_state_guard_decisions"])
		record := map[string]interface{}{
			"path":        strings.TrimSpace(decision.Path),
			"reason":      compactForPrompt(decision.Reason, 500),
			"observed_at": time.Now().UTC().Format(time.RFC3339),
		}
		if len(decision.Blockers) > 0 {
			record["blockers"] = compactStringSliceForPrompt(decision.Blockers, 8, 240)
		}
		records = append(records, record)
		if len(records) > 16 {
			records = records[len(records)-16:]
		}
		metadata["terminal_state_guard_decisions"] = mapsToInterfaceSlice(records)
		prepared.Message.Metadata = metadata
	}
}

func skillLoopCurrentMetadata(prepared *PreparedChat) func() map[string]interface{} {
	return func() map[string]interface{} {
		if prepared == nil || prepared.Message == nil {
			return nil
		}
		return copyStringAnyMap(prepared.Message.Metadata)
	}
}

func skillLoopHasOperationPlan(prepared *PreparedChat) bool {
	if prepared == nil || prepared.Message == nil {
		return false
	}
	return len(mapFromOperationContext(prepared.Message.Metadata["operation_plan"])) > 0
}

func skillLoopPrefersExplicitFinalAnswer(prepared *PreparedChat) bool {
	if prepared == nil || normalizeCallerType(prepared.Caller.Type) == runtimemodel.ConversationCallerAgent {
		return false
	}
	return skillLoopHasOperationPlan(prepared)
}

func skillLoopTerminalCompletionResult(prepared *PreparedChat) func(skillloop.TerminalCompletionResult) {
	return func(result skillloop.TerminalCompletionResult) {
		if prepared == nil || prepared.Message == nil {
			return
		}
		status := strings.TrimSpace(result.Status)
		if status == "blocked" {
			status = "needs_action"
		}
		metadata := copyStringAnyMap(prepared.Message.Metadata)
		applyOperationPlanTerminalCompletionResultWithSource(
			metadata,
			status,
			result.Source,
			result.Reason,
			result.Blockers,
			nil,
			"",
		)
		metadata = refreshOperationResultSummaryMetadata(metadata)
		prepared.Message.Metadata = metadata
	}
}

func refreshOperationResultSummaryMetadata(metadata map[string]interface{}) map[string]interface{} {
	if metadata == nil {
		return metadata
	}
	if summary := rebuiltOperationResultSummaryForPrompt(metadata); len(summary) > 0 {
		metadata["operation_result_summary"] = summary
		return metadata
	}
	delete(metadata, "operation_result_summary")
	return metadata
}

func skillLoopRuntimeStateSnapshot(prepared *PreparedChat) skillloop.RuntimeStateSnapshotFunc {
	return func() map[string]interface{} {
		evidence := map[string]interface{}{}
		if prepared == nil || prepared.parts == nil {
			return evidence
		}
		evidence["user_request"] = strings.TrimSpace(prepared.parts.Query)
		evidence["surface"] = normalizeAIChatSurface(prepared.parts.Surface)
		evidence["skill_mode"] = prepared.parts.SkillMode
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
		metadata := skillLoopRuntimeStateSnapshotMetadata(prepared.Message.Metadata)
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
			"turn_state",
			currentPageContextKey,
			consoleFilesContextSnapshotKey,
			consoleAgentsContextSnapshotKey,
		} {
			if value, ok := metadata[key]; ok && value != nil {
				evidence[key] = value
			}
		}
		executionLedger := map[string]interface{}{}
		for _, key := range []string{"operation_ledger", "skill_invocations", "generated_files", "client_actions", "tool_governance", "turn_state"} {
			if value, ok := metadata[key]; ok && value != nil {
				executionLedger[key] = value
			}
		}
		if ledger := skillLoopRuntimeStateSnapshotLedger(metadata); len(ledger) > 0 {
			evidenceLedger := mapsToInterfaceSlice(ledger)
			evidence["evidence_ledger"] = evidenceLedger
			executionLedger["evidence_ledger"] = evidenceLedger
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

func skillLoopRuntimeStateSnapshotLedger(metadata map[string]interface{}) []map[string]interface{} {
	if len(metadata) == 0 {
		return nil
	}
	plan := mapFromOperationContext(metadata["operation_plan"])
	if ledger := operationPlanCompactEvidenceLedger(plan[operationPlanEvidenceLedgerKey], 50); len(ledger) > 0 {
		return ledger
	}
	state := mapFromOperationContext(plan["strategy_state"])
	if ledger := mapSliceFromAny(state["evidence_ledger"]); len(ledger) > 0 {
		if len(ledger) > 12 {
			ledger = ledger[len(ledger)-12:]
		}
		return ledger
	}
	return nil
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

func skillLoopRuntimeStateSnapshotMetadata(source map[string]interface{}) map[string]interface{} {
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
		"current_page",
		"original_user_goal",
		"risk_level",
		"approval",
		"planning_mode",
		"plan_sync_status",
	} {
		if value := strings.TrimSpace(stringFromAny(plan[key])); value != "" {
			summary[key] = compactForPrompt(value, 500)
		}
	}
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
	if criteria := stringSliceFromAny(plan["success_criteria"]); len(criteria) > 0 {
		summary["success_criteria"] = compactStringSliceForPrompt(criteria, 8, 240)
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
		summary["strategy_state"] = operationPlanCompactStrategyStateForPrompt(state, true)
	}
	for _, key := range []string{"last_plan_update_round", "evidence_revision", "evidence_revision_at_plan_update", "evidence_sequence_at_plan_update", "evidence_after_last_plan_update"} {
		if value := intValueFromAny(plan[key]); value > 0 {
			summary[key] = value
		}
	}
	if len(summary) == 0 {
		return nil
	}
	return summary
}

func operationPlanCompactStrategyStateForPrompt(state map[string]interface{}, _ bool) map[string]interface{} {
	if len(state) == 0 {
		return nil
	}
	out := copyStringAnyMap(state)
	if goals := operationPlanCompactCapabilityGoals(out["capability_goals"], 6); len(goals) > 0 {
		out["capability_goals"] = goals
	} else {
		delete(out, "capability_goals")
	}
	for _, key := range []string{
		"plan_steps", "structured_plan", "completed_steps", "failed_steps",
		"plan_deviations", "blocked_deviations", "last_plan_deviation", "last_blocked_deviation",
		"completed_step_count", "failed_step_count", "plan_deviation_count", "blocked_deviation_count",
		"pending_next_action", "approval_actions", "completion_criteria", "target_resource",
	} {
		delete(out, key)
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
		for _, key := range []string{"id", "step", "title", "status", "note"} {
			if text := strings.TrimSpace(stringFromAny(phase[key])); text != "" {
				item[key] = compactForPrompt(text, 240)
			}
		}
		if evidence := stringSliceFromAny(phase["evidence"]); len(evidence) > 0 {
			item["evidence"] = compactStringSliceForPrompt(evidence, 6, 180)
		}
		if refs := stringSliceFromAny(phase["evidence_refs"]); len(refs) > 0 {
			item["evidence_refs"] = compactStringSliceForPrompt(refs, 12, 240)
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
	return restrictResolvedSkillsForTurnStrategy(prepared.parts, resolved)
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
		if route := contextualTurnCurrentPage(prepared.parts); route != "" {
			params["console_current_route"] = route
			params["console_agents_current_route"] = route
		}
		if currentAgent := consoleAgentsPromptCurrentAgent(prepared.parts); len(currentAgent) > 0 {
			params["console_current_agent_id"] = firstNonEmptyString(currentAgent["agent_id"], currentAgent["id"])
			params["console_agents_current_agent"] = currentAgent
		} else if visibleAgents := consoleAgentsPromptVisibleAgents(prepared.parts); len(visibleAgents) > 0 {
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
		"Current assistant turn task contract:",
		"This is a soft semantic contract for the current user turn, not a fixed action runtime plan.",
		"Use it to understand phases, target assets, success criteria, and verification points. Treat intent as a broad compatibility label, not as the full task meaning.",
		"Choose concrete tools from the currently enabled tool schemas and latest evidence.",
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
	delete(view, "planned_tools")
	if goals := mapSliceFromAny(view["capability_goals"]); len(goals) > 0 {
		view["capability_goals"] = aiChatTurnStrategyPromptCapabilityGoals(goals)
	}
	if contract := operationPlanTaskContractFromTurnStrategy(strategy); len(contract) > 0 {
		if goals := mapSliceFromAny(contract["capability_goals"]); len(goals) > 0 {
			contract["capability_goals"] = aiChatTurnStrategyPromptCapabilityGoals(goals)
		}
		view["task_contract"] = contract
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

// AIChatTurnStrategy is the typed, internal plan hint for one contextual sidebar turn.
// It is guidance for the skill loop, not an executable action plan.
type AIChatTurnStrategy struct {
	Surface                  string                      `json:"surface"`
	CurrentPage              string                      `json:"current_page,omitempty"`
	Source                   string                      `json:"source,omitempty"`
	SourceReason             string                      `json:"source_reason,omitempty"`
	Intent                   string                      `json:"intent"`
	CompatibilityIntent      string                      `json:"compatibility_intent,omitempty"`
	TaskType                 string                      `json:"task_type,omitempty"`
	TargetPage               string                      `json:"target_page,omitempty"`
	RouteRequired            bool                        `json:"route_required"`
	PrimarySkills            []string                    `json:"primary_skills"`
	SupportingSkills         []string                    `json:"supporting_skills"`
	PhaseGoals               []string                    `json:"phase_goals,omitempty"`
	EvidenceRequired         []string                    `json:"evidence_required,omitempty"`
	RecommendedCapabilities  []string                    `json:"recommended_capabilities,omitempty"`
	LowConfidence            bool                        `json:"low_confidence,omitempty"`
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

	PlannedTools []AIChatTurnStrategyTool `json:"planned_tools,omitempty"`
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
		Surface:          normalizeAIChatSurface(parts.Surface),
		CurrentPage:      currentPage,
		Source:           aiChatTurnStrategySourceDefault,
		SourceReason:     "base_contextual_sidebar_strategy",
		Intent:           "model_decides",
		TargetPage:       currentPage,
		RouteRequired:    false,
		PrimarySkills:    []string{},
		SupportingSkills: []string{},
		AssetEffect:      "model_decides",
		AssetRisk:        "tool_dependent",
		Approval:         "determined by the selected tool governance policy",
		SuccessCriteria: []string{
			"interpret the complete user request from conversation history and current page context",
			"for multi-step work, maintain the phase plan and continue until the requested outcomes are handled",
			"ground claims about actions in successful tool or client-action evidence",
		},
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

	// Continuation is a turn protocol command, not a business-domain intent.
	// It must win over stale or broad model intents because approve/retry/refresh
	// resumes should continue the frozen task state instead of starting a new
	// asset-management turn.
	if stagedResume || partsRequestsContinuationWithFallback(parts, "") {
		strategy = markAIChatTurnStrategySource(strategy, aiChatTurnStrategySourceTurnProtocol, "continuation_query_rule")
		strategy = contextualContinuationStrategy(parts, strategy)
		return finalizeAIChatTurnStrategy(parts, strategy)
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

	if stagedCurrent {
		if len(consoleNavigationResolvedTargetsForParts(parts)) > 0 {
			strategy = markAIChatTurnStrategySource(strategy, aiChatTurnStrategySourceTurnProtocol, "staged_current_navigation_scope")
			return finalizeAIChatTurnStrategy(parts, contextualNavigationStrategy(parts, strategy))
		}
	}

	strategy = markAIChatTurnStrategySource(strategy, aiChatTurnStrategySourceDefault, "main_model_contextual_task")
	strategy.ToolChoiceMode = aiChatTurnToolChoiceModelDecides
	if skillIDEnabled(parts.SkillIDs, skills.SkillConsoleNavigator) {
		strategy.SupportingSkills = appendUniqueStrings(strategy.SupportingSkills, skills.SkillConsoleNavigator)
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
	strategy.PrimarySkills = appendUniqueStrings(strategy.PrimarySkills, skills.SkillConsoleNavigator)
	strategy.Avoid = appendUniqueStrings(strategy.Avoid,
		"treat route_required and target_page as navigation context, not as a fixed tool script; choose console-navigator/navigate only when current page evidence does not already satisfy the user goal",
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
		return strategy
	}
	if strategy.Intent == "navigate_console_page" {
		return strategy
	}
	switch strings.TrimSpace(strategy.Intent) {
	case "save_generated_file_to_file_management", "generate_temporary_file_artifact", "delete_visible_file":
		strategy.PlannedTools = nil
	}
	return strategy
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
	if agentID := agentIDFromConsoleAgentPageRoute(contextualTurnCurrentPage(parts)); agentID != "" {
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
	if turnTaskContractRequestsManagedFileCreate(parts, nil, "") {
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
	strategy = applyRecentOperationPlanToContinuationStrategy(parts, strategy)
	return ensureContinuationFileArtifactSkills(parts, strategy)
}

func ensureContinuationFileArtifactSkills(parts *chatRequestParts, strategy *AIChatTurnStrategy) *AIChatTurnStrategy {
	if parts == nil || strategy == nil || strategy.ExecutionScope != "staged_goal_after_continue" {
		return strategy
	}
	hasExplicitFileTargets := len(requestedManagedFileTargetsFromParts(parts)) > 0
	targetsFilesPage := consoleNavigationLoadedHrefMatchesTarget(strategy.TargetPage, consoleFilesRouteHint().Href)
	if !hasExplicitFileTargets && !targetsFilesPage {
		return strategy
	}
	if len(parts.RecentGeneratedArtifacts) == 0 {
		strategy.SupportingSkills = appendArtifactProducerSkills(strategy.SupportingSkills, parts)
	}
	if skillIDEnabled(parts.SkillIDs, skills.SkillFileManager) {
		strategy.SupportingSkills = appendUniqueStrings(strategy.SupportingSkills, skills.SkillFileManager)
	}
	strategy.SuccessCriteria = appendUniqueStrings(strategy.SuccessCriteria,
		"continue the deferred file-related goal using the explicitly named file targets from the prior instruction",
	)
	return strategy
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
	if targets := consoleNavigationResolvedTargetsForParts(parts); len(targets) > 0 {
		target := targets[0]
		strategy.TargetPage = target.Href
		routeRequired := !clientActionContinuationLoadedRoute(parts, target.Href)
		if consoleNavigationRouteAlreadyAvailable(parts, target.Href) {
			routeRequired = false
		}
		strategy.RouteRequired = routeRequired
	}
	if skillIDEnabled(parts.SkillIDs, skills.SkillConsoleNavigator) {
		strategy.PrimarySkills = appendUniqueStrings(strategy.PrimarySkills, skills.SkillConsoleNavigator)
	}
	strategy.SuccessCriteria = []string{
		"console-navigator/navigate succeeds for the resolved route",
		"frontend client action reports route_loaded for the same href",
		"the same assistant turn continues from updated page context",
	}
	strategy.ObservationPoints = []string{"route_navigation_client_action", "updated_page_context"}
	return strategy
}

func contextualTurnCurrentPage(parts *chatRequestParts) string {
	if parts == nil {
		return ""
	}
	for _, source := range []map[string]interface{}{parts.RawOperationContext, parts.OperationContext} {
		if current := mapFromOperationContext(source[currentPageContextKey]); len(current) > 0 {
			return normalizeConsoleNavigationGuardHref(stringFromAny(current["route"]))
		}
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
	Href  string `json:"href"`
	Label string `json:"label"`
}

var consoleNavigationRouteHints = []consoleNavigationRouteHint{
	{Href: "/console", Label: "首页"},
	{Href: "/console/work/chat", Label: "对话"},
	{Href: "/console/work/image", Label: "绘图"},
	{Href: "/console/work/app", Label: "应用"},
	{Href: "/console/work/task", Label: "定时任务"},
	{Href: "/console/agents", Label: "智能体"},
	{Href: "/console/workflows", Label: "工作流"},
	{Href: "/console/dataset", Label: "知识库"},
	{Href: "/console/db", Label: "数据库"},
	{Href: "/console/files", Label: "文件管理"},
	{Href: "/console/prompts", Label: "提示词"},
	{Href: "/console/developer/content-parse", Label: "文件识别"},
	{Href: "/console/workspace", Label: "工作空间"},
	{Href: "/console/settings", Label: "系统设置"},
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
	encoded, err := json.Marshal(payload)
	if err != nil {
		return adapter.Message{}, false
	}

	content := strings.Join([]string{
		"Console navigation guidance:",
		"Use console-navigator/navigate when the user asks to open, go to, enter, switch to, or navigate to a known console module page.",
		"When the user explicitly asks to save, upload, import, or write a file into File Management from another console page, navigate to /console/files only if the current page context is not already Files and the File Management page context is still needed for the save.",
		"If current page evidence already satisfies the target, or a low-risk observe/read/list step is needed to complete the user's goal, continue from that evidence instead of forcing a redundant navigate call.",
		"Choose the destination from the route catalog and current page evidence; do not ask the user to repeat the page name when the request is already clear.",
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
	agents := consoleAgentsPromptVisibleAgents(parts)
	currentAgent := consoleAgentsPromptCurrentAgent(parts)
	payload := map[string]interface{}{
		"page":                    "console.agents",
		"preferred_skill":         skills.SkillAgentManagement,
		"tools":                   tools,
		"unsupported_in_this_mvp": []string{"publish_agent", "rollback_agent", "invoke_agent", "api_key", "webapp_online_offline"},
	}
	if len(agents) > 0 {
		payload["visible_agents"] = agents
	}
	if len(currentAgent) > 0 {
		payload["current_agent"] = currentAgent
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return adapter.Message{}, false
	}
	lines := []string{
		"Agent management guidance:",
		"Use agent-management for explicit Agent list, create, edit identity, delete, inspect draft config, update supported draft config, replace Agent memory slots, or edit knowledge/database/workflow bindings.",
		"Use the Agent capability model as the semantic contract for planning: skill-backed capabilities require a matching enabled Skill; file upload and memory are config switches; model changes require a provider/model pair; knowledge, database, and workflow access are binding changes.",
		"The tools array in Agent management JSON is the authoritative callable tool list for this turn. Do not call any agent-management or console-navigator tool that is absent from that tools array.",
		"When Agent management JSON includes visible_agents and the user refers to visible, selected, first-N/top-N, or page-list Agents, treat that backend-backed list and order as authoritative resolved targets. Use their agent_id/name/href directly; do not call list_agents only to rediscover the same visible targets.",
		"When Agent management JSON includes current_agent, it describes only the Agent detail page target. Do not treat current_agent as a visible Agent list or use it to resolve ordinal list references.",
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
	if navigationToolAvailable {
		lines = append(lines,
			"Choose console-navigator/navigate only when the user goal still needs another page and current context has not already satisfied that need.",
			"If delete_agent or delete_agents succeeds while the current route is a deleted Agent detail page, navigate to /console/agents when continuing requires a valid Agent page. Do not navigate after deleting Agents from the list page.",
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
		"Treat these capability goals as advisory context. Choose tools from the latest page and tool evidence, and describe only effects that the executed tools actually produced.",
	}
}

func agentManagementNavigationGuidanceAllowed(prepared *PreparedChat) bool {
	if prepared == nil || prepared.parts == nil {
		return false
	}
	return skillIDEnabled(prepared.parts.SkillIDs, skills.SkillConsoleNavigator)
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
		"For typed ordinal requests such as \"the second Excel file\", \"\u7b2c\u4e8c\u4e2a Excel\", or \"\u6700\u540e\u4e00\u4e2a PDF\", decide the target from visible_files using file_type_rank or extension_rank. Do not treat \"second Excel\" as visible_index 2 unless that file is also the second Excel in visible_files.",
		"When visible file content is needed to answer, call file-reader/read_file with a file_id from the Files-page context JSON.",
		"After read_file returns content_status \"extracted\", answer from the returned content field and continue requested post-processing such as summary or translation. Do not say the file cannot be read.",
		"When deleting or removing a visible file is the next action you choose, call file-manager/delete_file with a file_id from the Files-page context JSON. Tool governance handles the approval card before deletion; do not ask for a separate natural-language confirmation first.",
		"If a prior approval or session grant exists, it only skips the approval prompt. You must still call file-manager/delete_file in this turn and wait for the tool result before saying the file was deleted.",
		"Never claim a file was deleted, removed, updated, created, saved, or otherwise changed based only on previous conversation context.",
		"If the target file cannot be determined from structured context, call request_user_input with a concise clarification instead of guessing.",
		"For requests to create, generate, write, save, upload, import, or export a new file into File Management or the current Files page, use a two-step flow: first use the appropriate artifact-producing skill to create a temporary artifact, then use file-manager/save_file_to_management with source_type \"tool_file\", the generated tool_file_id/file_id, and the destination filename.",
		"Choose the artifact-producing skill that best matches the requested output. file-generator fits regular documents and files such as SVG, PDF, DOCX, PPTX, XLSX, CSV, JSON, Markdown, HTML, or TXT; chart-generator fits charts, graphs, and data visualizations.",
		"When the user says this file, the previous file, the generated file, or the file just created and asks to save/upload/import it into File Management, resolve that reference from recent_generated_files before considering visible_files. Use the listed tool_file_id only as a tool argument.",
		"Do not treat a visible File Management asset as the same file as a recent temporary generated artifact unless the filenames and requested action make that explicit.",
		"For requests to save or import a public external URL into File Management, use file-manager/save_file_to_management with source_type \"url\" and the destination filename.",
		"For generated or downloadable files without an explicit File Management, current Files page, save, create, or upload target, keep the default temporary artifact behavior and do not call file-manager/save_file_to_management.",
		"Creating a File Management file is a governed file.create operation owned by file-manager/save_file_to_management. Tool governance handles the approval card when the permission tier requires it; do not ask for a separate natural-language confirmation first.",
		"Do not call unrelated discovery or domain tools, such as database, knowledge, or calculator, before completing the requested files-page operation.",
		"For existing-file read/delete operations, do not call file-generation tools before the requested read/delete is completed.",
		"Files-page context JSON: " + string(encoded),
	}
	content := strings.Join(lines, "\n")
	return adapter.Message{Role: "system", Content: content}, true
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

func isFileManagerSaveToolCall(skillID string, toolName string) bool {
	return strings.EqualFold(strings.TrimSpace(skillID), skills.SkillFileManager) &&
		strings.EqualFold(strings.TrimSpace(toolName), "save_file_to_management")
}

func isFileManagerDeleteToolCall(skillID string, toolName string) bool {
	return strings.EqualFold(strings.TrimSpace(skillID), skills.SkillFileManager) &&
		strings.EqualFold(strings.TrimSpace(toolName), "delete_file")
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

func skillToolCallArgumentString(arguments map[string]interface{}, key string) string {
	if len(arguments) == 0 {
		return ""
	}
	return strings.TrimSpace(stringFromAny(arguments[key]))
}

func stagedExecutionScopedParts(parts *chatRequestParts) (*chatRequestParts, bool, bool) {
	if parts == nil {
		return nil, false, false
	}
	if partsRequestsContinuationWithFallback(parts, "") {
		if current, ok := stagedContinuationResumeQuery(parts); ok {
			return chatRequestPartsWithQueryAndIntent(parts, current, "continue_previous_task"), false, true
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

func chatRequestPartsWithQueryAndIntent(parts *chatRequestParts, query string, intent string) *chatRequestParts {
	next := chatRequestPartsWithQuery(parts, query)
	if next == nil {
		return nil
	}
	intent = strings.TrimSpace(intent)
	if intent == "" {
		return next
	}
	modelIntent := AIChatModelTurnIntent{}
	if next.ModelTurnIntent != nil {
		modelIntent = *next.ModelTurnIntent
	}
	modelIntent.Intent = intent
	next.ModelTurnIntent = &modelIntent
	return next
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
	if currentPageContextIsStale(parts) {
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

func consoleAgentsPromptAgents(parts *chatRequestParts) []map[string]interface{} {
	if parts == nil {
		return nil
	}
	if currentPageContextIsStale(parts) {
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

func consoleAgentsPromptVisibleAgents(parts *chatRequestParts) []map[string]interface{} {
	if isConsoleAgentDetailRoute(contextualTurnCurrentPage(parts)) {
		return nil
	}
	return consoleAgentsPromptAgents(parts)
}

func consoleAgentsPromptCurrentAgent(parts *chatRequestParts) map[string]interface{} {
	route := contextualTurnCurrentPage(parts)
	if !isConsoleAgentDetailRoute(route) {
		return nil
	}
	agentID := agentIDFromConsoleAgentPageRoute(route)
	agents := consoleAgentsPromptAgents(parts)
	for _, agent := range agents {
		if strings.EqualFold(strings.TrimSpace(firstNonEmptyString(agent["agent_id"], agent["id"])), agentID) {
			return copyStringAnyMap(agent)
		}
	}
	if len(agents) == 1 {
		return copyStringAnyMap(agents[0])
	}
	return nil
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
