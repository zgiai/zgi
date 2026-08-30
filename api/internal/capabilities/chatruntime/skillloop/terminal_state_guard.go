package skillloop

import (
	"encoding/json"
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
	LedgerEpoch           string
	RetryKey              string
	RetryAllowed          bool
}

type terminalStateGuardExternalGuideInstance struct {
	pending               terminalStateGuardPendingExternalAction
	actionKey             string
	logicalKey            string
	targetKey             string
	executable            bool
	preparationActionKeys map[string]struct{}
}

type terminalStateGuardExternalExecutionAttempt struct {
	sequence    int
	actionKey   string
	planPhaseID string
	targetKey   string
	ledgerEpoch string
	confirmed   bool
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
	if terminalStateGuardHasUnavailableProjectedExpectedAction(evidence) {
		return terminalStateGuardDecision{
			Path:        terminalStateGuardAccepted,
			Reason:      "replaced terminal answer because a pending projected Action is no longer exposed or authorized",
			FinalAnswer: terminalStateGuardExternalActionFailureAnswer(evidence, candidateAnswer),
		}
	}
	if issue := terminalStateGuardProjectedPlanLedgerIssue(evidence); issue != "" {
		return terminalStateGuardDecision{
			Path:     terminalStateGuardBlocked,
			Reason:   issue,
			Blockers: []string{"missing_protocol:external_action_plan_ledger"},
		}
	}
	pending, waivedExternalAction := terminalStateGuardPendingExternalExecution(evidence, candidateAnswer)
	if pending != nil {
		if !pending.RetryAllowed && terminalStateGuardHasUnconfirmedExternalAction(evidence) {
			return terminalStateGuardDecision{
				Path:        terminalStateGuardAccepted,
				Reason:      "replaced terminal answer because an exact external operation attempt was not provider-confirmed and cannot be replayed safely",
				FinalAnswer: terminalStateGuardExternalActionFailureAnswer(evidence, candidateAnswer),
			}
		}
		return terminalStateGuardDecision{
			Path:                  terminalStateGuardBlocked,
			Reason:                "a server-attested external Action expected by the current request has no matching execution attempt",
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
	instances := terminalStateGuardProjectedPlanInstances(evidence)
	if request == "" && len(instances) == 0 {
		return nil, false
	}
	if terminalStateGuardPureCapabilityDiscoveryRequest(request) &&
		!terminalStateGuardHasIntentMatchedMutationProjection(evidence) &&
		len(instances) == 0 {
		return nil, false
	}
	instanceIndexByLogicalKey := map[string]int{}
	for index := range instances {
		instanceIndexByLogicalKey[instances[index].logicalKey] = index
	}
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
				targetKey:   terminalStateGuardExternalActionTargetKey(record),
				ledgerEpoch: terminalStateGuardExternalLedgerEpoch(record),
				confirmed:   terminalStateGuardExternalActionProviderConfirmed(record),
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
		targetKey := terminalStateGuardExternalActionTargetKey(record)
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
			instance.pending.GuideInvocationID = guideInvocationID
			instance.pending.GuideSequence = sequence
			instance.pending.RetryKey = terminalStateGuardExternalGuideRetryKey(logicalKey, guideInvocationID, sequence)
			if targetKey != "" {
				instance.targetKey = targetKey
			}
			instance.preparationActionKeys = terminalStateGuardMergeActionKeys(
				instance.preparationActionKeys,
				terminalStateGuardGuidePreparationActionKeys(integrationID, result),
			)
			continue
		}

		retryKey := terminalStateGuardExternalGuideRetryKey(logicalKey, guideInvocationID, sequence)
		instanceIndexByLogicalKey[logicalKey] = len(instances)
		instances = append(instances, terminalStateGuardExternalGuideInstance{
			actionKey:             actionKey,
			logicalKey:            logicalKey,
			targetKey:             targetKey,
			executable:            terminalStateGuardExternalGuideCanExecute(result),
			preparationActionKeys: terminalStateGuardGuidePreparationActionKeys(integrationID, result),
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
				if attempt.confirmed {
					matchedAttempt = true
					break
				}
				// The exact call happened, so a non-idempotent mutation must not
				// be replayed automatically. Without a provider-confirmed status it
				// still cannot satisfy the pending phase or justify a success answer.
				ambiguousAttempt = true
				continue
			}
			if terminalStateGuardExternalAttemptAmbiguousForInstance(attempt, *instance) &&
				attempt.sequence > instance.pending.GuideSequence {
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
			prerequisiteSequence := terminalStateGuardExternalBlockingPrerequisiteSequence(
				evidence,
				instance.pending,
				instance.preparationActionKeys,
			)
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
		if !strings.EqualFold(attempt.planPhaseID, instance.pending.PlanPhaseID) {
			return false
		}
		if attempt.targetKey != "" && instance.targetKey != "" && attempt.targetKey != instance.targetKey {
			return false
		}
		return terminalStateGuardExternalAttemptMatchesLedgerEpoch(attempt, instance)
	}
	if attempt.targetKey != "" && instance.targetKey != "" {
		if attempt.targetKey != instance.targetKey {
			return false
		}
		return terminalStateGuardExternalAttemptMatchesLedgerEpoch(attempt, instance)
	}
	if instance.pending.PlanPhaseID != "" || instance.targetKey != "" {
		return false
	}
	// Missing phase identity is ambiguous for repeated operations of the same
	// Action. Treat it as attempted so a non-idempotent write is never replayed.
	return terminalStateGuardExternalAttemptMatchesLedgerEpoch(attempt, instance)
}

func terminalStateGuardExternalAttemptMatchesLedgerEpoch(attempt terminalStateGuardExternalExecutionAttempt, instance terminalStateGuardExternalGuideInstance) bool {
	if instance.pending.LedgerEpoch == "" {
		return true
	}
	return attempt.ledgerEpoch != "" && attempt.ledgerEpoch == instance.pending.LedgerEpoch
}

func terminalStateGuardProjectedPlanInstances(evidence map[string]interface{}) []terminalStateGuardExternalGuideInstance {
	trusted := projectedExternalActionKeys(evidence)
	if len(trusted) == 0 {
		return nil
	}
	phases := evidenceMapsFromAny(evidenceMapFromAny(evidence["operation_plan"])["phases"])
	instances := make([]terminalStateGuardExternalGuideInstance, 0, len(phases))
	for _, phase := range phases {
		if !operationPlanPhaseRequiredByServer(phase, evidence) {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(phase["status"])))
		if status == "completed" {
			continue
		}
		expected := evidenceMapFromAny(phase["expected_action"])
		actionKey, external := projectedExternalActionKeyFromExpected(expected)
		if !external || actionKey == "" {
			continue
		}
		if _, ok := trusted[actionKey]; !ok {
			continue
		}
		if projectedExpectedActionServerBindingIssue(expected, evidence) != "" {
			continue
		}
		integrationID, actionID, _ := strings.Cut(actionKey, ":")
		phaseID := strings.TrimSpace(evidenceStringFromAny(phase["id"]))
		logicalKey := terminalStateGuardExternalGuideLogicalKey(actionKey, phaseID, "", 0)
		projection := terminalStateGuardProjectedActionEvidence(expected, evidence)
		instances = append(instances, terminalStateGuardExternalGuideInstance{
			actionKey:             actionKey,
			logicalKey:            logicalKey,
			targetKey:             terminalStateGuardExpectedActionTargetKey(expected),
			executable:            true,
			preparationActionKeys: terminalStateGuardProjectionPreparationActionKeys(projection),
			pending: terminalStateGuardPendingExternalAction{
				IntegrationID: integrationID,
				ActionID:      actionID,
				Effect:        strings.ToLower(strings.TrimSpace(evidenceStringFromAny(projection["effect"]))),
				PlanPhaseID:   phaseID,
				LedgerEpoch:   strings.TrimSpace(evidenceStringFromAny(phase[operationPlanServerProjectedLedgerEpochKey])),
				RetryKey:      "projected-plan:" + logicalKey,
				RetryAllowed:  status != "skipped" && status != "failed",
			},
		})
	}
	return instances
}

func terminalStateGuardExpectedActionTargetKey(expected map[string]interface{}) string {
	if targetArguments := evidenceMapFromAny(expected["target_arguments"]); len(targetArguments) > 0 {
		return terminalStateGuardExternalTargetKey(targetArguments)
	}
	target := copyStringAnyMap(evidenceMapFromAny(expected["target"]))
	delete(target, "integration_id")
	delete(target, "action_id")
	return terminalStateGuardExternalTargetKey(target)
}

func terminalStateGuardExternalActionTargetKey(record map[string]interface{}) string {
	arguments := evidenceMapFromAny(record["arguments"])
	if target := evidenceMapFromAny(arguments["operation_plan_target"]); len(target) > 0 {
		return terminalStateGuardExternalTargetKey(target)
	}
	return terminalStateGuardExternalTargetKey(operationPlanSkillCallTarget(arguments))
}

func terminalStateGuardExternalTargetKey(target map[string]interface{}) string {
	if len(target) == 0 {
		return ""
	}
	encoded, err := json.Marshal(target)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func terminalStateGuardExternalAttemptAmbiguousForInstance(attempt terminalStateGuardExternalExecutionAttempt, instance terminalStateGuardExternalGuideInstance) bool {
	if attempt.actionKey == "" {
		return true
	}
	if attempt.actionKey != instance.actionKey {
		return false
	}
	if attempt.planPhaseID != "" && instance.pending.PlanPhaseID != "" {
		if !strings.EqualFold(attempt.planPhaseID, instance.pending.PlanPhaseID) {
			return false
		}
		if attempt.targetKey != "" && instance.targetKey != "" && attempt.targetKey != instance.targetKey {
			return false
		}
		return instance.pending.LedgerEpoch != "" && attempt.ledgerEpoch == "" &&
			!strings.EqualFold(instance.pending.Effect, "read")
	}
	if attempt.targetKey != "" && instance.targetKey != "" {
		if attempt.targetKey != instance.targetKey {
			return false
		}
		return instance.pending.LedgerEpoch != "" && attempt.ledgerEpoch == "" &&
			!strings.EqualFold(instance.pending.Effect, "read")
	}
	return instance.pending.PlanPhaseID != "" || instance.targetKey != ""
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

func terminalStateGuardExternalLedgerEpoch(record map[string]interface{}) string {
	for _, source := range []map[string]interface{}{
		record,
		evidenceMapFromAny(record["arguments"]),
		evidenceMapFromAny(record["result"]),
		evidenceMapFromAny(record["result_summary"]),
	} {
		if value := strings.TrimSpace(evidenceStringFromAny(source[operationPlanServerProjectedLedgerEpochKey])); value != "" {
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
	allowedActionKeys map[string]struct{},
) int {
	if len(allowedActionKeys) == 0 {
		return 0
	}
	blockingByAction := map[string]int{}
	for index, record := range terminalStateGuardExternalInvocations(evidence) {
		sequence := index + 1
		if pending.GuideSequence > 0 && sequence >= pending.GuideSequence {
			break
		}
		if !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(record["skill_id"])), "external-apps") ||
			!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(record["tool_name"])), "execute_action") {
			continue
		}
		actionKey := terminalStateGuardExternalActionKey(record)
		if _, allowed := allowedActionKeys[actionKey]; !allowed {
			continue
		}
		if pending.PlanPhaseID != "" {
			recordPhaseID := terminalStateGuardExternalPlanPhaseID(record)
			if recordPhaseID == "" || !strings.EqualFold(recordPhaseID, pending.PlanPhaseID) {
				continue
			}
		}
		if pending.LedgerEpoch != "" && terminalStateGuardExternalLedgerEpoch(record) != pending.LedgerEpoch {
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
			blockingByAction[actionKey] = sequence
		} else {
			// A later success of this exact server-attested prerequisite means its
			// older empty result must no longer waive the dependent Action. Success
			// of a different preparation Action cannot erase that evidence.
			delete(blockingByAction, actionKey)
		}
	}
	blockingSequence := 0
	for _, sequence := range blockingByAction {
		if sequence > blockingSequence {
			blockingSequence = sequence
		}
	}
	return blockingSequence
}

func terminalStateGuardProjectedActionEvidence(expected map[string]interface{}, evidence map[string]interface{}) map[string]interface{} {
	if candidate := projectedExternalActionCandidateForExpected(expected, evidence); len(candidate) > 0 {
		return candidate
	}
	serverAlias := strings.TrimSpace(evidenceStringFromAny(expected[planExpectedActionServerProjectionKey]))
	for _, projection := range evidenceMapsFromAny(evidence[runtimeStateNativeExternalActionProjectionsKey]) {
		if strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(projection["tool_name"])), serverAlias) {
			return projection
		}
	}
	return nil
}

func terminalStateGuardProjectionPreparationActionKeys(projection map[string]interface{}) map[string]struct{} {
	out := map[string]struct{}{}
	for _, key := range evidenceStringSliceFromAny(projection["preparation_action_keys"]) {
		if key = strings.ToLower(strings.TrimSpace(key)); key != "" {
			out[key] = struct{}{}
		}
	}
	return out
}

func terminalStateGuardGuidePreparationActionKeys(integrationID string, result map[string]interface{}) map[string]struct{} {
	integrationID = strings.ToLower(strings.TrimSpace(integrationID))
	if integrationID == "" {
		return nil
	}
	out := map[string]struct{}{}
	for _, hint := range evidenceMapsFromAny(result["preparation_hints"]) {
		actionID := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(hint["action_id"])))
		if actionID != "" {
			out[integrationID+":"+actionID] = struct{}{}
		}
	}
	return out
}

func terminalStateGuardMergeActionKeys(current map[string]struct{}, extra map[string]struct{}) map[string]struct{} {
	if len(extra) == 0 {
		return current
	}
	if current == nil {
		current = map[string]struct{}{}
	}
	for key := range extra {
		current[key] = struct{}{}
	}
	return current
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

func terminalStateGuardPureCapabilityDiscoveryRequest(request string) bool {
	if !terminalStateGuardCapabilityDiscoveryRequest(request) {
		return false
	}
	normalized := strings.ToLower(strings.TrimSpace(request))
	// Treat discovery as a narrow whole-request classification. Sentence marks
	// and sequencing words create semantic clauses; every non-empty clause must
	// itself be discovery-only. Failing closed here avoids maintaining a second,
	// inevitably incomplete vocabulary of imperative verbs alongside the
	// server-owned external Action intent matcher.
	clauses := terminalStateGuardCapabilityRequestClauses(normalized)
	sawDiscovery := false
	for _, clause := range clauses {
		clause = strings.TrimSpace(clause)
		if clause == "" {
			continue
		}
		if terminalStateGuardCapabilityDiscoveryRequest(clause) {
			sawDiscovery = true
			if terminalStateGuardTargetedExecutionClause(clause) {
				return false
			}
			continue
		}
		return false
	}
	return sawDiscovery
}

func terminalStateGuardCapabilityRequestClauses(request string) []string {
	replacer := strings.NewReplacer(
		"？", "\n", "?", "\n", "。", "\n", ".", "\n", "！", "\n", "!", "\n",
		"；", "\n", ";", "\n", "，", "\n", ",", "\n", "：", "\n", ":", "\n",
		"然后", "\n", "接着", "\n", "顺便", "\n", "同时", "\n", "之后", "\n",
		"最后", "\n", "另外", "\n", "再帮", "\n", "并帮", "\n",
		" after that ", "\n", " finally ", "\n", " then ", "\n", " also ", "\n", " and ", "\n",
	)
	return strings.FieldsFunc(replacer.Replace(request), func(char rune) bool { return char == '\n' || char == '\r' })
}

func terminalStateGuardTargetedExecutionClause(clause string) bool {
	return terminalStateGuardContainsAny(clause, []string{
		"帮我", "请帮", "请给", "替我", "为我", "给我", "给 ", "发给", "发送给", "发送消息",
		"创建", "删除", "更新", "执行", "邀请", "提交", "发布", "转发", "现在就", "立即",
		" send ", " create ", " delete ", " update ", " execute ", " invite ", " submit ", " publish ",
	})
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

func terminalStateGuardRequiresProjectedPlanLedgerRetry(decision terminalStateGuardDecision) bool {
	for _, blocker := range decision.Blockers {
		if blocker == "missing_protocol:external_action_plan_ledger" {
			return true
		}
	}
	return false
}

func terminalStateGuardProjectedPlanLedgerRetryMessage() string {
	return "The previous answer tried to finish an intent-matched external operation without a complete server-owned operation plan and server-canonical expected_action ledger. Do not claim that the external operation succeeded. First call update_plan and preserve every open required phase. Bind each tool phase to the exact exposed business function in expected_action; a server-classified final-answer-only phase may remain non-tool. In the same assistant turn, call the required projected business Action after update_plan when its arguments are known."
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
	return fmt.Sprintf("The current user asked to perform an external-app operation. The pending server-attested Action is integration_id=%q and action_id=%q, but there is no matching external-apps/execute_action attempt for that Action and plan phase. Re-evaluate the latest tool results and only this pending Action. Use its exposed projected business function when available; otherwise use the external-apps execute_action fallback. Do not repeat any earlier successful or attempted Action. If its prerequisites are satisfied, execute it once and let the normal governance flow run. If a prerequisite failed or returned no target, explain truthfully that the operation was not performed. If a required business argument is genuinely missing, ask one concise clarification instead.", integrationID, actionID)
}

func terminalStateGuardHasUnconfirmedExternalAction(evidence map[string]interface{}) bool {
	latestConfirmationByAction := map[string]bool{}
	for _, record := range terminalStateGuardExternalInvocations(evidence) {
		if !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(record["skill_id"])), "external-apps") ||
			!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(record["tool_name"])), "execute_action") {
			continue
		}
		latestConfirmationByAction[terminalStateGuardExternalExecutionLogicalKey(record)] =
			terminalStateGuardExternalActionProviderConfirmed(record)
	}
	for _, confirmed := range latestConfirmationByAction {
		if !confirmed {
			return true
		}
	}
	return false
}

func terminalStateGuardExternalActionProviderConfirmed(record map[string]interface{}) bool {
	if terminalStateGuardFailureStatus(terminalStateGuardStatusFromMap(record)) ||
		strings.TrimSpace(evidenceStringFromAny(record["error"])) != "" {
		return false
	}
	result := evidenceMapFromAny(record["result"])
	resultSummary := evidenceMapFromAny(record["result_summary"])
	operationStatus := ""
	for _, source := range []map[string]interface{}{result, resultSummary} {
		if operationStatus = strings.ToLower(strings.TrimSpace(evidenceStringFromAny(source["operation_status"]))); operationStatus != "" {
			break
		}
	}
	switch operationStatus {
	case "completed", "already_completed", "succeeded":
	default:
		return false
	}
	for _, source := range []map[string]interface{}{record, result, resultSummary} {
		if raw, exists := source["provider_success_confirmed"]; exists {
			confirmed, ok := raw.(bool)
			if !ok || !confirmed {
				return false
			}
		}
	}
	return true
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
	if terminalStateGuardHasUnavailableProjectedExpectedAction(evidence) {
		return false
	}
	if terminalStateGuardProjectedPlanLedgerIssue(evidence) != "" {
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

func terminalStateGuardHasUnavailableProjectedExpectedAction(evidence map[string]interface{}) bool {
	for _, phase := range evidenceMapsFromAny(evidenceMapFromAny(evidence["operation_plan"])["phases"]) {
		if !operationPlanPhaseOpenForToolCall(phase) || !operationPlanPhaseRequiredByServer(phase, evidence) {
			continue
		}
		expected := evidenceMapFromAny(phase["expected_action"])
		if _, external := projectedExternalActionKeyFromExpected(expected); !external {
			continue
		}
		if strings.TrimSpace(evidenceStringFromAny(expected[planExpectedActionServerBindingFingerprintKey])) == "" &&
			strings.TrimSpace(evidenceStringFromAny(expected[planExpectedActionServerProjectionKey])) == "" {
			continue
		}
		issue := projectedExpectedActionServerBindingIssue(expected, evidence)
		if issue != "" && issue != projectedExternalMutationTargetMissingIssue {
			return true
		}
	}
	return false
}

func terminalStateGuardProjectedPlanLedgerIssue(evidence map[string]interface{}) string {
	phases := evidenceMapsFromAny(evidenceMapFromAny(evidence["operation_plan"])["phases"])
	request := strings.TrimSpace(firstNonEmptyString(
		evidenceStringFromAny(evidence["latest_user_request"]),
		evidenceStringFromAny(evidence["user_request"]),
	))
	pureDiscovery := terminalStateGuardPureCapabilityDiscoveryRequest(request)
	intentMatchedMutation := terminalStateGuardHasIntentMatchedMutationProjection(evidence)
	if len(phases) == 0 && pureDiscovery && !intentMatchedMutation {
		return ""
	}
	if terminalStateGuardHasIntentMatchedProjection(evidence) &&
		(!pureDiscovery || intentMatchedMutation) &&
		!terminalStateGuardHasCanonicalProjectedPlanLedger(phases, evidence) {
		return "an intent-matched projected external Action has no server-canonical expected_action ledger"
	}
	if err := validateProjectedExternalActionCandidateContracts(phases, evidence); err != nil {
		return strings.TrimPrefix(err.Error(), ErrInvalidInput.Error()+": ")
	}
	requiredPhaseIDs := projectedExternalActionRequiredPhaseIDs(evidence)
	if len(requiredPhaseIDs) == 0 {
		return ""
	}
	for _, phase := range evidenceMapsFromAny(evidenceMapFromAny(evidence["operation_plan"])["phases"]) {
		phaseID := strings.TrimSpace(evidenceStringFromAny(phase["id"]))
		if _, required := requiredPhaseIDs[phaseID]; !required {
			continue
		}
		expected := evidenceMapFromAny(phase["expected_action"])
		if len(expected) == 0 {
			return "a required runtime-effect outcome has no server-canonical expected_action ledger"
		}
		if _, external := projectedExternalActionKeyFromExpected(expected); !external {
			return "an external runtime-effect outcome is bound to a non-external expected_action"
		}
		if issue := projectedExpectedActionServerBindingIssue(expected, evidence); issue != "" {
			return issue
		}
		if issue := projectedExpectedActionMutationTargetIssue(expected, evidence); issue != "" {
			return issue
		}
	}
	return ""
}

func terminalStateGuardHasCanonicalProjectedPlanLedger(
	phases []map[string]interface{},
	evidence map[string]interface{},
) bool {
	for _, phase := range phases {
		expected := evidenceMapFromAny(phase["expected_action"])
		if _, external := projectedExternalActionKeyFromExpected(expected); !external {
			continue
		}
		if projectedExpectedActionServerBindingIssue(expected, evidence) == "" {
			return true
		}
	}
	return false
}

func terminalStateGuardHasIntentMatchedProjection(evidence map[string]interface{}) bool {
	if matched, _ := evidence[runtimeStateNativeExternalActionIntentMatchedKey].(bool); matched {
		return true
	}
	for _, projection := range evidenceMapsFromAny(evidence[runtimeStateNativeExternalActionCandidatesKey]) {
		if matched, _ := projection["intent_matched"].(bool); matched {
			return true
		}
	}
	return false
}

func terminalStateGuardHasIntentMatchedMutationProjection(evidence map[string]interface{}) bool {
	for _, key := range []string{
		runtimeStateNativeExternalActionCandidatesKey,
		runtimeStateNativeExternalActionProjectionsKey,
	} {
		for _, projection := range evidenceMapsFromAny(evidence[key]) {
			matched, _ := projection["intent_matched"].(bool)
			if !matched {
				continue
			}
			effect := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(projection["effect"])))
			if effect != "" && effect != "none" && effect != "read" {
				return true
			}
		}
	}
	return false
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
