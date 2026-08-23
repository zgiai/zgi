package skillloop

import (
	"fmt"
	"strings"
)

type terminalStateGuardPath string

const (
	terminalStateGuardAccepted terminalStateGuardPath = "accepted"
	terminalStateGuardBlocked  terminalStateGuardPath = "blocked"
)

type terminalStateGuardDecision struct {
	Path                  terminalStateGuardPath
	Reason                string
	Blockers              []string
	FinalAnswer           string
	PendingExternalAction *terminalStateGuardPendingExternalAction
}

type terminalStateGuardPendingExternalAction struct {
	IntegrationID         string
	ActionID              string
	Effect                string
	PlanPhaseID           string
	GuideInvocationID     string
	GuideSequence         int
	RequiredArgumentCount int
	RetryKey              string
	RetryAllowed          bool
}

type terminalStateGuardExternalGuideInstance struct {
	pending    terminalStateGuardPendingExternalAction
	actionKey  string
	logicalKey string
	executable bool
}

type terminalStateGuardExternalExecutionAttempt struct {
	sequence    int
	actionKey   string
	planPhaseID string
}

func terminalStateGuardEvaluate(evidence map[string]interface{}, candidateAnswer string) terminalStateGuardDecision {
	candidateAnswer = strings.TrimSpace(candidateAnswer)
	if candidateAnswer == "" {
		return terminalStateGuardDecision{
			Path:     terminalStateGuardBlocked,
			Reason:   "final answer is empty",
			Blockers: []string{"missing_protocol:final_answer"},
		}
	}
	if blocker := terminalStateGuardPendingProtocolBlocker(evidence); blocker != "" {
		return terminalStateGuardDecision{
			Path:     terminalStateGuardBlocked,
			Reason:   "runtime protocol is still waiting for an external result",
			Blockers: []string{blocker},
		}
	}
	pending, waivedExternalAction := terminalStateGuardPendingExternalExecution(evidence, candidateAnswer)
	if pending != nil {
		return terminalStateGuardDecision{
			Path:                  terminalStateGuardBlocked,
			Reason:                "a ready external action guide was read for the current execution request but the action was not executed",
			Blockers:              []string{"missing_protocol:external_action_execution"},
			PendingExternalAction: pending,
		}
	}
	if terminalStateGuardHasUnconfirmedExternalAction(evidence) {
		return terminalStateGuardDecision{
			Path:        terminalStateGuardAccepted,
			Reason:      "replaced terminal answer because at least one external operation has no successful execution evidence",
			FinalAnswer: terminalStateGuardExternalActionFailureAnswer(evidence, candidateAnswer),
		}
	}
	if waivedExternalAction {
		return terminalStateGuardDecision{
			Path:        terminalStateGuardAccepted,
			Reason:      "accepted a truthful non-execution answer after waiving its blocked dependent external action",
			FinalAnswer: candidateAnswer,
		}
	}
	return terminalStateGuardDecision{
		Path:        terminalStateGuardAccepted,
		Reason:      "main model submitted a terminal answer with no active runtime protocol blocker",
		FinalAnswer: candidateAnswer,
	}
}

func terminalStateGuardPendingExternalExecution(evidence map[string]interface{}, candidateAnswer string) (*terminalStateGuardPendingExternalAction, bool) {
	request := strings.TrimSpace(firstNonEmptyString(
		evidenceStringFromAny(evidence["latest_user_request"]),
		evidenceStringFromAny(evidence["user_request"]),
	))
	if request == "" || terminalStateGuardCapabilityDiscoveryRequest(request) {
		return nil, false
	}

	instances := make([]terminalStateGuardExternalGuideInstance, 0)
	instanceIndexByLogicalKey := map[string]int{}
	attempts := make([]terminalStateGuardExternalExecutionAttempt, 0)
	for index, record := range terminalStateGuardExternalInvocations(evidence) {
		sequence := index + 1
		if !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(record["skill_id"])), "external-apps") {
			continue
		}
		toolName := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(record["tool_name"])))
		if toolName == "execute_action" {
			attempts = append(attempts, terminalStateGuardExternalExecutionAttempt{
				sequence:    sequence,
				actionKey:   terminalStateGuardExternalActionKey(record),
				planPhaseID: terminalStateGuardExternalPlanPhaseID(record),
			})
			continue
		}
		if toolName != "get_action_guide" || terminalStateGuardFailureStatus(terminalStateGuardExternalActionStatus(record)) {
			continue
		}

		result := terminalStateGuardExternalActionResult(record)
		integrationID, actionID := terminalStateGuardExternalActionIdentity(record)
		actionKey := terminalStateGuardExternalActionKey(record)
		planPhaseID := terminalStateGuardExternalPlanPhaseID(record)
		guideInvocationID := terminalStateGuardExternalGuideInvocationID(record)
		logicalKey := terminalStateGuardExternalGuideLogicalKey(actionKey, planPhaseID, guideInvocationID, sequence)
		requiredArgumentCount := terminalStateGuardExternalGuideRequiredArgumentCount(result)

		if existingIndex, exists := instanceIndexByLogicalKey[logicalKey]; exists {
			instance := &instances[existingIndex]
			instance.executable = terminalStateGuardExternalGuideCanExecute(result)
			instance.pending.IntegrationID = integrationID
			instance.pending.ActionID = actionID
			instance.pending.Effect = strings.ToLower(strings.TrimSpace(evidenceStringFromAny(result["effect"])))
			instance.pending.RequiredArgumentCount = requiredArgumentCount
			continue
		}

		retryKey := terminalStateGuardExternalGuideRetryKey(logicalKey, guideInvocationID, sequence)
		instanceIndexByLogicalKey[logicalKey] = len(instances)
		instances = append(instances, terminalStateGuardExternalGuideInstance{
			actionKey:  actionKey,
			logicalKey: logicalKey,
			executable: terminalStateGuardExternalGuideCanExecute(result),
			pending: terminalStateGuardPendingExternalAction{
				IntegrationID:         integrationID,
				ActionID:              actionID,
				Effect:                strings.ToLower(strings.TrimSpace(evidenceStringFromAny(result["effect"]))),
				PlanPhaseID:           planPhaseID,
				GuideInvocationID:     guideInvocationID,
				GuideSequence:         sequence,
				RequiredArgumentCount: requiredArgumentCount,
				RetryKey:              retryKey,
				RetryAllowed:          actionKey != "",
			},
		})
	}

	var nonRetryablePending *terminalStateGuardPendingExternalAction
	waivedExternalAction := false
	waivedPrerequisites := map[int]struct{}{}
	for index := range instances {
		instance := &instances[index]
		if !instance.executable {
			continue
		}

		matchedAttempt := false
		ambiguousAttempt := false
		for _, attempt := range attempts {
			if terminalStateGuardExternalAttemptMatchesGuide(attempt, *instance) {
				matchedAttempt = true
				break
			}
			if attempt.actionKey == "" && attempt.sequence > instance.pending.GuideSequence {
				ambiguousAttempt = true
			}
		}
		if matchedAttempt {
			continue
		}
		if ambiguousAttempt {
			// A redacted continuation proves that an external call was attempted,
			// but not which identified Action it belongs to. Keep the Action
			// incomplete while disabling automatic replay; never let ambiguous
			// evidence make a different Action look completed.
			instance.pending.RetryAllowed = false
		}
		if instance.pending.RequiredArgumentCount > 0 && terminalStateGuardExternalClarificationAnswer(candidateAnswer) {
			continue
		}
		if instance.pending.RetryAllowed &&
			terminalStateGuardExternalActionDependsOnTarget(instance.pending) &&
			terminalStateGuardReportsPendingActionNonExecution(instance.pending, candidateAnswer) {
			prerequisiteSequence := terminalStateGuardExternalBlockingPrerequisiteSequence(evidence, instance.pending)
			if prerequisiteSequence > 0 {
				if _, alreadyUsed := waivedPrerequisites[prerequisiteSequence]; !alreadyUsed {
					waivedPrerequisites[prerequisiteSequence] = struct{}{}
					waivedExternalAction = true
					continue
				}
			}
		}
		pending := instance.pending
		if pending.RetryAllowed {
			return &pending, waivedExternalAction
		}
		if nonRetryablePending == nil {
			nonRetryablePending = &pending
		}
	}
	return nonRetryablePending, waivedExternalAction
}

func terminalStateGuardExternalGuideLogicalKey(actionKey string, planPhaseID string, guideInvocationID string, sequence int) string {
	if actionKey != "" {
		if planPhaseID != "" {
			return actionKey + "#phase:" + strings.ToLower(planPhaseID)
		}
		return actionKey
	}
	if guideInvocationID != "" {
		return "unkeyed-guide:" + strings.ToLower(guideInvocationID)
	}
	return fmt.Sprintf("unkeyed-guide-sequence:%d", sequence)
}

func terminalStateGuardExternalGuideRetryKey(logicalKey string, guideInvocationID string, sequence int) string {
	if guideInvocationID != "" {
		return "guide:" + strings.ToLower(guideInvocationID)
	}
	if logicalKey != "" {
		return fmt.Sprintf("action-instance:%s@%d", logicalKey, sequence)
	}
	return fmt.Sprintf("guide-sequence:%d", sequence)
}

func terminalStateGuardExternalAttemptMatchesGuide(attempt terminalStateGuardExternalExecutionAttempt, instance terminalStateGuardExternalGuideInstance) bool {
	if attempt.actionKey == "" || instance.actionKey == "" || attempt.actionKey != instance.actionKey {
		return false
	}
	if attempt.planPhaseID != "" && instance.pending.PlanPhaseID != "" {
		return strings.EqualFold(attempt.planPhaseID, instance.pending.PlanPhaseID)
	}
	// Missing phase identity is ambiguous for repeated operations of the same
	// Action. Treat it as attempted so a non-idempotent write is never replayed.
	return true
}

func terminalStateGuardExternalActionResult(record map[string]interface{}) map[string]interface{} {
	if result := evidenceMapFromAny(record["result"]); len(result) > 0 {
		return result
	}
	return evidenceMapFromAny(record["result_summary"])
}

func terminalStateGuardExternalPlanPhaseID(record map[string]interface{}) string {
	for _, source := range []map[string]interface{}{
		record,
		evidenceMapFromAny(record["arguments"]),
		evidenceMapFromAny(record["result"]),
		evidenceMapFromAny(record["result_summary"]),
	} {
		if value := strings.TrimSpace(evidenceStringFromAny(source["plan_phase_id"])); value != "" {
			return value
		}
	}
	return ""
}

func terminalStateGuardExternalGuideRequiredArgumentCount(result map[string]interface{}) int {
	if count := numericValue(result["required_argument_count"]); count > 0 {
		return count
	}
	if required := evidenceSliceFromAny(result["required_arguments"]); len(required) > 0 {
		return len(required)
	}
	inputSchema := evidenceMapFromAny(result["input_schema"])
	return len(evidenceSliceFromAny(inputSchema["required"]))
}

func terminalStateGuardExternalBlockingPrerequisiteSequence(
	evidence map[string]interface{},
	pending terminalStateGuardPendingExternalAction,
) int {
	blockingSequence := 0
	for index, record := range terminalStateGuardExternalInvocations(evidence) {
		sequence := index + 1
		if pending.GuideSequence > 0 && sequence >= pending.GuideSequence {
			break
		}
		if !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(record["skill_id"])), "external-apps") ||
			!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(record["tool_name"])), "execute_action") {
			continue
		}
		blocked := false
		if terminalStateGuardFailureStatus(terminalStateGuardExternalActionStatus(record)) {
			blocked = true
		}
		for _, result := range []map[string]interface{}{
			evidenceMapFromAny(record["result"]),
			evidenceMapFromAny(record["result_summary"]),
		} {
			if terminalStateGuardExternalResultIsEmpty(result) {
				blocked = true
			}
		}
		if blocked {
			blockingSequence = sequence
		} else {
			// Bind by the closest preceding execution boundary. A later successful
			// prerequisite means an older empty result must not waive this Action.
			blockingSequence = 0
		}
	}
	return blockingSequence
}

func terminalStateGuardExternalActionDependsOnTarget(pending terminalStateGuardPendingExternalAction) bool {
	effect := strings.ToLower(strings.TrimSpace(pending.Effect))
	for _, marker := range []string{"send", "write", "create", "update", "delete", "remove", "invite", "mutation", "destructive"} {
		if strings.Contains(effect, marker) {
			return true
		}
	}
	actionID := strings.ToLower(strings.TrimSpace(pending.ActionID))
	for _, marker := range []string{".send", "_send", ".create", "_create", ".update", "_update", ".delete", "_delete", ".remove", "_remove", ".invite", "_invite", ".add", "_add"} {
		if strings.Contains(actionID, marker) {
			return true
		}
	}
	return false
}

func terminalStateGuardExternalResultIsEmpty(result map[string]interface{}) bool {
	if len(result) == 0 {
		return false
	}
	for _, key := range []string{"result_count", "matched_count", "total_count", "total", "count"} {
		if value, exists := result[key]; exists && numericValue(value) == 0 {
			return true
		}
	}
	for _, key := range []string{"items", "results", "records", "users", "members", "targets"} {
		if value, exists := result[key]; exists {
			switch typed := value.(type) {
			case []interface{}:
				if len(typed) == 0 {
					return true
				}
			case []map[string]interface{}:
				if len(typed) == 0 {
					return true
				}
			}
		}
	}
	return false
}

func terminalStateGuardReportsPendingActionNonExecution(pending terminalStateGuardPendingExternalAction, answer string) bool {
	normalized := strings.ToLower(strings.TrimSpace(answer))
	effectAndAction := strings.ToLower(strings.TrimSpace(pending.Effect + " " + pending.ActionID))
	if strings.Contains(effectAndAction, "send") || strings.Contains(effectAndAction, "message") || strings.Contains(effectAndAction, "notify") {
		if terminalStateGuardContainsAny(normalized, []string{"已经发送", "已发送", "实际发送", "has been sent", "was sent", "successfully sent"}) {
			return false
		}
		return terminalStateGuardContainsAny(normalized, []string{"未发送", "没有发送", "发送未完成", "not sent", "was not sent", "did not send", "nothing was sent", "send did not complete"})
	}
	if strings.Contains(effectAndAction, "create") || strings.Contains(effectAndAction, "invite") || strings.Contains(effectAndAction, ".add") || strings.Contains(effectAndAction, "_add") {
		if terminalStateGuardContainsAny(normalized, []string{"已经创建", "已创建", "已经添加", "已添加", "已经邀请", "已邀请", "was created", "created successfully", "was added", "added successfully", "was invited", "invited successfully"}) {
			return false
		}
		return terminalStateGuardContainsAny(normalized, []string{"未创建", "没有创建", "未添加", "没有添加", "未邀请", "没有邀请", "not created", "was not created", "did not create", "not added", "was not added", "did not add", "not invited", "was not invited", "did not invite"})
	}
	if strings.Contains(effectAndAction, "update") || strings.Contains(effectAndAction, "modify") || strings.Contains(effectAndAction, "edit") {
		if terminalStateGuardContainsAny(normalized, []string{"已经更新", "已更新", "已经修改", "已修改", "was updated", "updated successfully", "was modified", "modified successfully"}) {
			return false
		}
		return terminalStateGuardContainsAny(normalized, []string{"未更新", "没有更新", "未修改", "没有修改", "not updated", "was not updated", "did not update", "not modified", "was not modified", "did not modify"})
	}
	if strings.Contains(effectAndAction, "delete") || strings.Contains(effectAndAction, "remove") || strings.Contains(effectAndAction, "destructive") {
		if terminalStateGuardContainsAny(normalized, []string{"已经删除", "已删除", "已经移除", "已移除", "was deleted", "deleted successfully", "was removed", "removed successfully"}) {
			return false
		}
		return terminalStateGuardContainsAny(normalized, []string{"未删除", "没有删除", "未移除", "没有移除", "not deleted", "was not deleted", "did not delete", "not removed", "was not removed", "did not remove"})
	}
	return terminalStateGuardContainsAny(normalized, []string{"未执行", "没有执行", "未操作", "没有操作", "not executed", "did not execute", "not performed", "did not perform"})
}

func terminalStateGuardContainsAny(text string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func terminalStateGuardExternalGuideCanExecute(result map[string]interface{}) bool {
	if canExecute, exists := result["can_execute"].(bool); exists {
		return canExecute
	}
	switch strings.ToLower(strings.TrimSpace(evidenceStringFromAny(result["availability"]))) {
	case "scope_upgrade_required", "disabled_by_policy", "data_egress_blocked", "unavailable", "disabled":
		return false
	default:
		return true
	}
}

func terminalStateGuardExternalActionIdentity(record map[string]interface{}) (string, string) {
	for _, source := range []map[string]interface{}{
		evidenceMapFromAny(record["arguments"]),
		evidenceMapFromAny(record["result"]),
		evidenceMapFromAny(record["result_summary"]),
	} {
		integrationID := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(source["integration_id"])))
		actionID := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(source["action_id"])))
		if integrationID != "" && actionID != "" {
			return integrationID, actionID
		}
	}
	return "", ""
}

func terminalStateGuardExternalGuideInvocationID(record map[string]interface{}) string {
	for _, key := range []string{"invocation_id", "runtime_id", "call_id", "id"} {
		if value := strings.TrimSpace(evidenceStringFromAny(record[key])); value != "" {
			return value
		}
	}
	return ""
}

func terminalStateGuardExternalInvocations(evidence map[string]interface{}) []map[string]interface{} {
	// The top-level runtime snapshot is the authoritative ordered timeline for
	// the current turn. The nested execution ledger is a compatibility fallback
	// and may duplicate or lag the top-level records; mixing both can make an old
	// guide appear newer than a completed write and provoke an unsafe retry.
	if records := evidenceMapsFromAny(evidence["skill_invocations"]); len(records) > 0 {
		return records
	}
	if execution := evidenceMapFromAny(evidence["execution_ledger"]); len(execution) > 0 {
		return evidenceMapsFromAny(execution["skill_invocations"])
	}
	return nil
}

func terminalStateGuardExternalActionKey(record map[string]interface{}) string {
	integrationID, actionID := terminalStateGuardExternalActionIdentity(record)
	if integrationID != "" && actionID != "" {
		return integrationID + ":" + actionID
	}
	return ""
}

func terminalStateGuardCapabilityDiscoveryRequest(request string) bool {
	normalized := strings.ToLower(strings.TrimSpace(request))
	markers := []string{
		"有哪些功能", "有什么功能", "支持哪些", "支持什么", "能做什么", "可用功能", "可用操作", "应用能力",
		"如何使用", "怎么使用", "怎么用", "使用方法", "需要哪些参数", "参数说明", "功能说明", "功能介绍", "使用指南",
		"what can", "which actions", "available actions", "available capabilities", "supported actions", "supported capabilities",
		"how to use", "which arguments", "required arguments", "action guide",
	}
	for _, marker := range markers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func terminalStateGuardExternalInputClarification(evidence map[string]interface{}, answer string) bool {
	if !terminalStateGuardLatestGuideRequiresArguments(evidence) {
		return false
	}
	return terminalStateGuardExternalClarificationAnswer(answer)
}

func terminalStateGuardExternalClarificationAnswer(answer string) bool {
	normalized := strings.ToLower(strings.TrimSpace(answer))
	markers := []string{
		"请提供", "请补充", "请指定", "请确认要", "需要你提供", "需要您提供", "缺少必填",
		"please provide", "please specify", "which specific", "required input", "missing required",
	}
	for _, marker := range markers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	if strings.ContainsAny(normalized, "?？") {
		for _, marker := range []string{"哪", "什么", "谁", "多少", "何时", "哪个", "which", "what", "who", "when", "how many"} {
			if strings.Contains(normalized, marker) {
				return true
			}
		}
	}
	return false
}

func terminalStateGuardLatestGuideRequiresArguments(evidence map[string]interface{}) bool {
	var latest map[string]interface{}
	for _, record := range terminalStateGuardExternalInvocations(evidence) {
		if strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(record["skill_id"])), "external-apps") &&
			strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(record["tool_name"])), "get_action_guide") &&
			!terminalStateGuardFailureStatus(terminalStateGuardExternalActionStatus(record)) {
			latest = evidenceMapFromAny(record["result"])
		}
	}
	if len(latest) == 0 {
		return false
	}
	if numericValue(latest["required_argument_count"]) > 0 {
		return true
	}
	if required := evidenceSliceFromAny(latest["required_arguments"]); len(required) > 0 {
		return true
	}
	inputSchema := evidenceMapFromAny(latest["input_schema"])
	return len(evidenceSliceFromAny(inputSchema["required"])) > 0
}

func terminalStateGuardRequiresExternalExecutionRetry(decision terminalStateGuardDecision) bool {
	for _, blocker := range decision.Blockers {
		if blocker == "missing_protocol:external_action_execution" {
			return true
		}
	}
	return false
}

func terminalStateGuardCanRetryPendingExternalAction(
	pending *terminalStateGuardPendingExternalAction,
	retryCounts map[string]int,
) bool {
	if pending == nil ||
		!pending.RetryAllowed ||
		strings.TrimSpace(pending.IntegrationID) == "" ||
		strings.TrimSpace(pending.ActionID) == "" ||
		strings.TrimSpace(pending.RetryKey) == "" {
		return false
	}
	return retryCounts[pending.RetryKey] < 1
}

func terminalStateGuardSafeExternalNonExecutionDecision(evidence map[string]interface{}, candidateAnswer string) terminalStateGuardDecision {
	return terminalStateGuardDecision{
		Path:        terminalStateGuardAccepted,
		Reason:      "external action completion retry was unavailable or exhausted; returned a safe non-execution answer",
		FinalAnswer: terminalStateGuardExternalActionFailureAnswer(evidence, candidateAnswer),
	}
}

func terminalStateGuardExternalExecutionRetryMessage(pending *terminalStateGuardPendingExternalAction) string {
	integrationID := "the current guide's integration_id"
	actionID := "the current guide's action_id"
	if pending != nil {
		if pending.IntegrationID != "" {
			integrationID = pending.IntegrationID
		}
		if pending.ActionID != "" {
			actionID = pending.ActionID
		}
	}
	return fmt.Sprintf("The current user asked to perform an external-app operation. The latest pending guide is integration_id=%q and action_id=%q, but there is no matching external-apps/execute_action attempt for that action. Re-evaluate the latest tool results and only this pending action. Do not repeat any earlier successful or attempted action. If its prerequisites are satisfied, call external-apps/execute_action once and let the normal governance flow run. If a prerequisite failed or returned no target, explain truthfully that the operation was not performed. If a required business argument is genuinely missing, ask one concise clarification instead.", integrationID, actionID)
}

func terminalStateGuardHasUnconfirmedExternalAction(evidence map[string]interface{}) bool {
	latestStatusByAction := map[string]string{}
	for _, record := range terminalStateGuardExternalInvocations(evidence) {
		if !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(record["skill_id"])), "external-apps") ||
			!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(record["tool_name"])), "execute_action") {
			continue
		}
		status := terminalStateGuardExternalActionStatus(record)
		if status == "" {
			continue
		}
		latestStatusByAction[terminalStateGuardExternalExecutionLogicalKey(record)] = status
	}
	for _, status := range latestStatusByAction {
		if terminalStateGuardFailureStatus(status) {
			return true
		}
	}
	return false
}

func terminalStateGuardExternalExecutionLogicalKey(record map[string]interface{}) string {
	actionKey := terminalStateGuardExternalActionKey(record)
	if actionKey != "" {
		if planPhaseID := terminalStateGuardExternalPlanPhaseID(record); planPhaseID != "" {
			return actionKey + "#phase:" + strings.ToLower(planPhaseID)
		}
		return actionKey
	}
	if planPhaseID := terminalStateGuardExternalPlanPhaseID(record); planPhaseID != "" {
		return "unkeyed-execution#phase:" + strings.ToLower(planPhaseID)
	}
	if invocationID := terminalStateGuardExternalGuideInvocationID(record); invocationID != "" {
		return "unkeyed-execution:" + strings.ToLower(invocationID)
	}
	// Legacy/redacted projections may omit all identity fields. Preserve their
	// historical last-attempt-wins semantics; keyed Actions above remain fully
	// independent and cannot hide one another.
	return "unkeyed-execution"
}

func terminalStateGuardExternalActionStatus(record map[string]interface{}) string {
	recordStatus := terminalStateGuardStatusFromMap(record)
	if terminalStateGuardFailureStatus(recordStatus) {
		return recordStatus
	}
	for _, source := range []map[string]interface{}{
		evidenceMapFromAny(record["result"]),
		evidenceMapFromAny(record["result_summary"]),
	} {
		if status := terminalStateGuardStatusFromMap(source); status != "" {
			return status
		}
	}
	if recordStatus != "" {
		return recordStatus
	}
	if strings.TrimSpace(evidenceStringFromAny(record["error"])) != "" {
		return "error"
	}
	return ""
}

func terminalStateGuardStatusFromMap(source map[string]interface{}) string {
	for _, key := range []string{"operation_status", "status", "result_status", "outcome"} {
		if status := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(source[key]))); status != "" {
			return status
		}
	}
	return ""
}

func terminalStateGuardFailureStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "error", "failed", "failure", "failed_safe", "partial_failed", "partially_failed", "partially_succeeded", "outcome_unknown",
		"executing", "pending", "blocked", "rejected", "denied", "cancelled", "canceled", "expired", "needs_approval", "waiting_approval":
		return true
	default:
		return false
	}
}

func terminalStateGuardExternalActionFailureAnswer(evidence map[string]interface{}, candidateAnswer string) string {
	text := strings.TrimSpace(evidenceStringFromAny(evidence["user_request"])) + candidateAnswer
	return LocalizedExternalActionFailureAnswer(text, ExternalActionFailureSendOrOperation)
}

func terminalStateGuardCanStream(evidence map[string]interface{}) bool {
	if terminalStateGuardPendingProtocolBlocker(evidence) != "" {
		return false
	}
	if terminalStateGuardHasUnconfirmedExternalAction(evidence) {
		return false
	}
	// Keep a provisional success claim out of the user-visible stream until the
	// guard has correlated every executable guide with an execution attempt.
	pending, _ := terminalStateGuardPendingExternalExecution(evidence, "")
	return pending == nil
}

func terminalStateGuardPendingProtocolBlocker(evidence map[string]interface{}) string {
	for _, key := range []string{"pending_approval", "pending_client_action", "pending_question", "pending_user_input"} {
		if evidenceValuePresent(evidence[key]) {
			return "pending_protocol:" + strings.TrimPrefix(key, "pending_")
		}
	}
	for _, source := range terminalStateGuardEvidenceSources(evidence) {
		if terminalStateGuardHasPendingGovernance(evidenceMapsFromAny(source["tool_governance"])) {
			return "pending_protocol:approval"
		}
		if terminalStateGuardHasPendingClientAction(evidenceMapsFromAny(source["client_actions"])) {
			return "pending_protocol:client_action"
		}
	}
	return ""
}

func terminalStateGuardEvidenceSources(evidence map[string]interface{}) []map[string]interface{} {
	sources := []map[string]interface{}{evidence}
	if execution := evidenceMapFromAny(evidence["execution_ledger"]); len(execution) > 0 {
		sources = append(sources, execution)
	}
	return sources
}

func terminalStateGuardHasPendingGovernance(records []map[string]interface{}) bool {
	for _, record := range terminalStateGuardLatestRecords(records, []string{"correlation_id", "invocation_id", "call_id", "id"}) {
		approvalStatus := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(record["approval_status"])))
		switch approvalStatus {
		case "approved", "rejected", "resolved", "completed", "succeeded", "failed":
			continue
		}
		switch strings.ToLower(strings.TrimSpace(evidenceStringFromAny(record["status"]))) {
		case "pending", "waiting", "needs_approval", "waiting_approval":
			return true
		}
	}
	return false
}

func terminalStateGuardHasPendingClientAction(records []map[string]interface{}) bool {
	for _, record := range terminalStateGuardLatestRecords(records, []string{"action_id", "runtime_id", "call_id", "id"}) {
		switch strings.ToLower(strings.TrimSpace(evidenceStringFromAny(record["status"]))) {
		case "pending", "waiting", "running", "loading", "streaming", "waiting_client_action":
			return true
		}
	}
	return false
}

func terminalStateGuardLatestRecords(records []map[string]interface{}, keyFields []string) []map[string]interface{} {
	if len(records) < 2 {
		return records
	}
	latest := map[string]map[string]interface{}{}
	order := make([]string, 0, len(records))
	for index, record := range records {
		key := ""
		for _, field := range keyFields {
			if value := strings.TrimSpace(evidenceStringFromAny(record[field])); value != "" {
				key = field + ":" + value
				break
			}
		}
		if key == "" {
			key = fmt.Sprintf("record:%d", index)
		}
		if _, exists := latest[key]; !exists {
			order = append(order, key)
		}
		latest[key] = record
	}
	out := make([]map[string]interface{}, 0, len(order))
	for _, key := range order {
		out = append(out, latest[key])
	}
	return out
}

func terminalStateGuardError(decision terminalStateGuardDecision) error {
	blocker := ""
	if len(decision.Blockers) > 0 {
		blocker = strings.TrimSpace(decision.Blockers[0])
	}
	return fmt.Errorf("%w: terminal state blocked: %s", ErrInvalidInput, firstNonEmptyString(blocker, decision.Reason))
}

func terminalStateGuardNotify(req RunRequest, decision terminalStateGuardDecision) {
	if req.OnTerminalCompletion == nil {
		return
	}
	status := "blocked"
	source := "terminal_state_guard"
	if decision.Path == terminalStateGuardAccepted {
		status = "pass"
		source = "main_model_final"
	}
	req.OnTerminalCompletion(TerminalCompletionResult{
		Status:   status,
		Source:   source,
		Reason:   strings.TrimSpace(decision.Reason),
		Blockers: append([]string(nil), decision.Blockers...),
	})
}

func terminalStateGuardRecord(req RunRequest, decision terminalStateGuardDecision) {
	if req.OnTerminalStateGuardDecision == nil {
		return
	}
	req.OnTerminalStateGuardDecision(TerminalStateGuardDecisionRecord{
		Path:     string(decision.Path),
		Reason:   strings.TrimSpace(decision.Reason),
		Blockers: append([]string(nil), decision.Blockers...),
	})
}
