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
	Path        terminalStateGuardPath
	Reason      string
	Blockers    []string
	FinalAnswer string
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
	if terminalStateGuardExternalExecutionRequired(evidence, candidateAnswer) {
		return terminalStateGuardDecision{
			Path:     terminalStateGuardBlocked,
			Reason:   "a ready external action guide was read for the current execution request but the action was not executed",
			Blockers: []string{"missing_protocol:external_action_execution"},
		}
	}
	if terminalStateGuardLatestExternalActionFailed(evidence) {
		return terminalStateGuardDecision{
			Path:        terminalStateGuardAccepted,
			Reason:      "replaced terminal answer because the latest external operation has no successful execution evidence",
			FinalAnswer: terminalStateGuardExternalActionFailureAnswer(evidence, candidateAnswer),
		}
	}
	return terminalStateGuardDecision{
		Path:        terminalStateGuardAccepted,
		Reason:      "main model submitted a terminal answer with no active runtime protocol blocker",
		FinalAnswer: candidateAnswer,
	}
}

func terminalStateGuardExternalExecutionRequired(evidence map[string]interface{}, candidateAnswer string) bool {
	request := strings.TrimSpace(firstNonEmptyString(
		evidenceStringFromAny(evidence["latest_user_request"]),
		evidenceStringFromAny(evidence["user_request"]),
	))
	if request == "" || terminalStateGuardCapabilityDiscoveryRequest(request) ||
		terminalStateGuardExternalInputClarification(evidence, candidateAnswer) {
		return false
	}
	sequence := 0
	earliestGuideSequence := -1
	latestGuideSequence := -1
	latestGuideKey := ""
	earliestGuideSequenceByKey := map[string]int{}
	latestExecutionSequenceByKey := map[string]int{}
	latestExecutionSequence := -1
	latestUnkeyedExecutionSequence := -1
	for _, record := range terminalStateGuardExternalInvocations(evidence) {
		sequence++
		if !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(record["skill_id"])), "external-apps") {
			continue
		}
		toolName := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(record["tool_name"])))
		if toolName == "execute_action" {
			latestExecutionSequence = sequence
			if key := terminalStateGuardExternalActionKey(record); key != "" {
				latestExecutionSequenceByKey[key] = sequence
			} else {
				// Approval continuations and public timeline projections may retain
				// the execution outcome while intentionally removing internal
				// routing identifiers. Treat that execution as completion evidence
				// instead of asking the model to repeat a possibly non-idempotent
				// write operation.
				latestUnkeyedExecutionSequence = sequence
			}
			continue
		}
		if toolName != "get_action_guide" || terminalStateGuardFailureStatus(terminalStateGuardExternalActionStatus(record)) {
			continue
		}
		result := evidenceMapFromAny(record["result"])
		availability := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(result["availability"])))
		if availability == "" || availability == "ready" || availability == "available" {
			if earliestGuideSequence < 0 {
				earliestGuideSequence = sequence
			}
			latestGuideSequence = sequence
			latestGuideKey = terminalStateGuardExternalActionKey(record)
			if latestGuideKey != "" {
				if _, exists := earliestGuideSequenceByKey[latestGuideKey]; !exists {
					earliestGuideSequenceByKey[latestGuideKey] = sequence
				}
			}
		}
	}
	if latestGuideSequence < 0 {
		return false
	}
	if latestGuideKey != "" {
		matchingExecution := latestExecutionSequenceByKey[latestGuideKey]
		if firstGuide := earliestGuideSequenceByKey[latestGuideKey]; firstGuide > 0 && matchingExecution > firstGuide {
			return false
		}
		if latestUnkeyedExecutionSequence > earliestGuideSequence {
			return false
		}
		return true
	}
	return latestExecutionSequence < latestGuideSequence
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
	for _, source := range []map[string]interface{}{
		evidenceMapFromAny(record["arguments"]),
		evidenceMapFromAny(record["result"]),
		evidenceMapFromAny(record["result_summary"]),
	} {
		integrationID := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(source["integration_id"])))
		actionID := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(source["action_id"])))
		if integrationID != "" && actionID != "" {
			return integrationID + ":" + actionID
		}
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
	normalized := strings.ToLower(strings.TrimSpace(answer))
	if !terminalStateGuardLatestGuideRequiresArguments(evidence) {
		return false
	}
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

func terminalStateGuardExternalExecutionRetryMessage() string {
	return "The current user asked to perform an external-app operation. You already read a ready action guide but attempted to finish without calling external-apps/execute_action, so nothing has happened yet. Re-evaluate only the latest user request, do not reuse an action from an earlier turn, and call execute_action with the exact integration_id and action_id from the current guide. If a required business argument is genuinely missing, ask one concise clarification instead. Do not claim that the function is unavailable when the current catalog contains it."
}

func terminalStateGuardLatestExternalActionFailed(evidence map[string]interface{}) bool {
	for _, source := range terminalStateGuardEvidenceSources(evidence) {
		latestStatus := ""
		for _, record := range evidenceMapsFromAny(source["skill_invocations"]) {
			if !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(record["skill_id"])), "external-apps") ||
				!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(record["tool_name"])), "execute_action") {
				continue
			}
			latestStatus = terminalStateGuardExternalActionStatus(record)
		}
		if latestStatus != "" {
			return terminalStateGuardFailureStatus(latestStatus)
		}
	}
	return false
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
	for _, key := range []string{"status", "result_status", "outcome"} {
		if status := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(source[key]))); status != "" {
			return status
		}
	}
	return ""
}

func terminalStateGuardFailureStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "error", "failed", "failure", "partial_failed", "partially_failed", "partially_succeeded", "outcome_unknown":
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
	return terminalStateGuardPendingProtocolBlocker(evidence) == ""
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
