package skillloop

import "strings"

type completionGatePath string

const (
	completionGateDeterministicPass completionGatePath = "deterministic_pass"
	completionGateNeedsAction       completionGatePath = "needs_action"
	completionGateAskUser           completionGatePath = "ask_user"
	completionGateModelVerifier     completionGatePath = "model_verifier"
)

type completionGateDecision struct {
	Path              completionGatePath
	Reason            string
	MissingFacts      []string
	UnsupportedClaims []string
	NextActionHint    string
	FinalAnswer       string
}

func completionGateEvaluate(evidence map[string]interface{}, candidateAnswer string) completionGateDecision {
	candidateAnswer = strings.TrimSpace(candidateAnswer)
	if candidateAnswer == "" {
		return completionGateModelVerifierDecision("main model has not produced a final answer")
	}
	if boolFromAny(evidence["model_verifier_required"]) || boolFromAny(evidence["force_completion_verifier"]) {
		return completionGateModelVerifierDecision("model verifier explicitly required by completion evidence")
	}
	if completionVerificationCandidateAnswerLeaksInternalPlan(candidateAnswer) {
		return completionGateDecision{
			Path:              completionGateNeedsAction,
			Reason:            "candidate answer exposes internal planning state",
			UnsupportedClaims: []string{"internal planning state must not be shown to the user"},
			NextActionHint:    "rewrite the final answer using only user-visible evidence",
		}
	}
	if completionGateHasPendingRuntimeBlocker(evidence) {
		return completionGateDecision{
			Path:           completionGateNeedsAction,
			Reason:         "runtime state still contains an unresolved protocol blocker",
			MissingFacts:   []string{"pending_state: runtime.open_item"},
			NextActionHint: "continue from the latest runtime state without claiming completion",
		}
	}
	if fastPathEvidenceHasUnresolvedPlanFailure(evidence) {
		return completionGateModelVerifierDecision("operation plan has unresolved failed evidence")
	}
	if fastPathHasAgentConfigTargetMismatch(evidence) {
		return completionGateDecision{
			Path:           completionGateNeedsAction,
			Reason:         "post-update Agent config read conflicts with requested update facts",
			MissingFacts:   []string{"mismatch_fact: agent.config.post_update_read"},
			NextActionHint: "continue from the latest evidence and resolve the mismatched Agent config fields; do not prescribe a fixed tool script",
		}
	}
	if completionGateHasOnlyLegacyFacts(evidence) {
		return completionGateModelVerifierDecision("completion evidence is legacy-only and needs answer fidelity audit")
	}
	if completionGateCanAcceptCandidateAnswerFromEvidence(evidence, candidateAnswer) {
		return completionGatePassDecision(candidateAnswer, "main model final answer has no unresolved runtime blockers")
	}
	return completionGateModelVerifierDecision("completion requires model verifier audit")
}

// A verifier may point out an unsupported claim once. The next main-model turn
// owns the repaired answer; only protocol blockers and internal-state leaks can
// keep it from becoming the user-visible final response.
func completionGateEvaluateVerifierRepair(evidence map[string]interface{}, candidateAnswer string) completionGateDecision {
	candidateAnswer = strings.TrimSpace(candidateAnswer)
	if candidateAnswer == "" {
		return completionGateModelVerifierDecision("main model has not produced a repaired final answer")
	}
	if completionVerificationCandidateAnswerLeaksInternalPlan(candidateAnswer) {
		return completionGateDecision{
			Path:              completionGateNeedsAction,
			Reason:            "candidate answer exposes internal planning state",
			UnsupportedClaims: []string{"internal planning state must not be shown to the user"},
			NextActionHint:    "rewrite the final answer using only user-visible evidence",
		}
	}
	if completionGateHasPendingRuntimeBlocker(evidence) {
		return completionGateDecision{
			Path:           completionGateNeedsAction,
			Reason:         "runtime state still contains an unresolved protocol blocker",
			MissingFacts:   []string{"pending_state: runtime.open_item"},
			NextActionHint: "continue from the latest runtime state without claiming completion",
		}
	}
	return completionGatePassDecision(candidateAnswer, "main model repaired the answer after one evidence audit")
}

func completionGateHasPendingRuntimeBlocker(evidence map[string]interface{}) bool {
	for _, key := range []string{"pending_approval", "pending_client_action", "pending_question", "pending_user_input"} {
		if completionVerificationEvidenceValuePresent(evidence[key]) {
			return true
		}
	}
	turnState := evidenceMapFromAny(evidence["turn_state"])
	for _, item := range evidenceMapsFromAny(turnState["open_items"]) {
		status := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(item["status"])))
		switch status {
		case "error", "failed", "failure", "rejected":
			continue
		default:
			return true
		}
	}
	status := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(turnState["status"])))
	switch status {
	case "waiting_approval", "waiting_client_action", "waiting_question", "needs_action", "blocked":
		return true
	default:
		return false
	}
}

func completionGateModelVerifierDecision(reason string) completionGateDecision {
	return completionGateDecision{
		Path:   completionGateModelVerifier,
		Reason: strings.TrimSpace(reason),
	}
}

func completionGatePassDecision(answer string, reason string) completionGateDecision {
	return completionGateDecision{
		Path:        completionGateDeterministicPass,
		Reason:      strings.TrimSpace(firstNonEmptyString(reason, "normalized evidence ledger verifies the requested operation")),
		FinalAnswer: strings.TrimSpace(answer),
	}
}

func completionGateCanAcceptCandidateAnswerFromEvidence(evidence map[string]interface{}, candidateAnswer string) bool {
	if strings.TrimSpace(candidateAnswer) == "" {
		return false
	}
	if !completionGateHasRuntimeContext(evidence) {
		return true
	}
	if !completionVerificationHasSuccessfulEvidence(evidence) {
		return false
	}
	return !fastPathEvidenceHasUnresolvedPlanFailure(evidence) &&
		!fastPathHasAgentConfigTargetMismatch(evidence)
}

func completionGateHasRuntimeContext(evidence map[string]interface{}) bool {
	for _, key := range []string{
		"evidence_ledger",
		"skill_invocations",
		"generated_files",
		"client_actions",
		"tool_governance",
		"operation_ledger",
		"execution_ledger",
		"operation_plan",
		"operation_result_summary",
		"turn_state",
	} {
		if completionVerificationEvidenceValuePresent(evidence[key]) {
			return true
		}
	}
	return false
}

func completionGateNotify(req RunRequest, decision completionGateDecision, fallbackAnswer string) {
	if req.OnCompletionVerification == nil {
		return
	}
	notifyCompletionVerificationResult(req, decision.completionVerificationDecision(), firstNonEmptyString(decision.FinalAnswer, fallbackAnswer))
}

func (d completionGateDecision) completionVerificationDecision() completionVerificationDecision {
	status := completionVerificationStatusFailed
	source := "completion_gate"
	switch d.Path {
	case completionGateDeterministicPass:
		status = completionVerificationStatusPass
		source = "main_model_final"
	case completionGateNeedsAction:
		status = completionVerificationStatusNeedsAction
	case completionGateAskUser:
		status = completionVerificationStatusAskUser
	case completionGateModelVerifier:
		status = completionVerificationStatusFailed
	}
	return completionVerificationDecision{
		Status:              status,
		Source:              source,
		Reason:              strings.TrimSpace(d.Reason),
		MissingSteps:        append([]string(nil), d.MissingFacts...),
		UnsupportedClaims:   append([]string(nil), d.UnsupportedClaims...),
		NextActionHint:      strings.TrimSpace(d.NextActionHint),
		FinalAnswer:         strings.TrimSpace(d.FinalAnswer),
		FinalAnswerGuidance: strings.TrimSpace(d.NextActionHint),
	}
}

func completionGateContractCoverageGaps(evidence map[string]interface{}) []string {
	return nil
}

func completionGateHasOnlyLegacyFacts(evidence map[string]interface{}) bool {
	if completionVerificationEvidenceValuePresent(evidence["evidence_ledger"]) {
		return false
	}
	ledger := evidenceMapFromAny(evidence["execution_ledger"])
	if completionVerificationEvidenceValuePresent(ledger["evidence_ledger"]) {
		return false
	}
	return completionVerificationEvidenceValuePresent(evidence["operation_ledger"]) ||
		completionVerificationEvidenceValuePresent(ledger["operation_ledger"])
}
