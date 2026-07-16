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
	return terminalStateGuardDecision{
		Path:        terminalStateGuardAccepted,
		Reason:      "main model submitted a terminal answer with no active runtime protocol blocker",
		FinalAnswer: candidateAnswer,
	}
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
