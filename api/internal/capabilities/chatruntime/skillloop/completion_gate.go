package skillloop

import (
	"fmt"
	"strings"
)

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
	if planDecision, decided := completionGateEvaluatePlan(evidence); decided {
		return planDecision
	}
	if completionGateHasOnlyLegacyFacts(evidence) {
		return completionGateModelVerifierDecision("completion evidence is legacy-only and needs answer fidelity audit")
	}
	if completionGateCanAcceptCandidateAnswerFromEvidence(evidence, candidateAnswer) {
		return completionGatePassDecision(candidateAnswer, "main model final answer has no unresolved runtime blockers")
	}
	return completionGateModelVerifierDecision("completion requires model verifier audit")
}

func completionGateEvaluateTerminal(evidence map[string]interface{}, candidateAnswer string) completionGateDecision {
	candidateAnswer = strings.TrimSpace(candidateAnswer)
	if candidateAnswer == "" {
		return completionGateDecision{
			Path:           completionGateNeedsAction,
			Reason:         "submit_final_answer requires a non-empty user-facing answer",
			MissingFacts:   []string{"missing_protocol: final_answer"},
			NextActionHint: "continue the model loop and submit a complete final answer when the work is terminal",
		}
	}
	if boolFromAny(evidence["model_verifier_required"]) || boolFromAny(evidence["force_completion_verifier"]) {
		return completionGateModelVerifierDecision("model verifier explicitly required by completion evidence")
	}
	if completionGateHasTerminalRuntimeBlocker(evidence) {
		return completionGateDecision{
			Path:           completionGateNeedsAction,
			Reason:         "runtime state still contains an unresolved protocol blocker",
			MissingFacts:   []string{"pending_state: runtime.open_item"},
			NextActionHint: "continue from the latest runtime state without claiming completion",
		}
	}
	if fastPathHasAgentConfigTargetMismatch(evidence) {
		return completionGateModelVerifierDecision("authoritative post-action evidence contains a concrete configuration conflict")
	}
	if planDecision, decided := completionGateEvaluatePlan(evidence); decided {
		return planDecision
	}
	return completionGatePassDecision(candidateAnswer, "main model submitted a terminal answer with no unresolved runtime blockers")
}

func completionGateCanBeginTerminalStream(evidence map[string]interface{}) bool {
	if boolFromAny(evidence["model_verifier_required"]) || boolFromAny(evidence["force_completion_verifier"]) {
		return false
	}
	if completionGateHasTerminalRuntimeBlocker(evidence) || fastPathHasAgentConfigTargetMismatch(evidence) {
		return false
	}
	_, planBlocks := completionGateEvaluatePlan(evidence)
	return !planBlocks
}

func completionEvidenceWithPlanSnapshot(evidence map[string]interface{}, phases []map[string]interface{}) map[string]interface{} {
	if len(phases) == 0 {
		return evidence
	}
	out := copyStringAnyMap(evidence)
	plan := copyStringAnyMap(evidenceMapFromAny(out["operation_plan"]))
	if plan == nil {
		plan = map[string]interface{}{}
	}
	plan["phases"] = phases
	out["operation_plan"] = plan
	return out
}

func completionGateEvaluatePlan(evidence map[string]interface{}) (completionGateDecision, bool) {
	plan := evidenceMapFromAny(evidence["operation_plan"])
	phases := evidenceMapsFromAny(plan["phases"])
	if len(phases) == 0 {
		return completionGateDecision{}, false
	}
	hasSkipped := false
	for _, phase := range phases {
		id := strings.TrimSpace(firstNonEmptyString(phase["id"], phase["step"], phase["title"]))
		status := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(phase["status"])))
		switch status {
		case "completed":
			refs := evidenceStringSliceFromAny(phase["evidence_refs"])
			if len(refs) == 0 {
				return completionGateNeedsPlanAction("invalid_evidence_ref:"+id, "completed plan phase has no evidence reference"), true
			}
			for _, ref := range refs {
				if !completionGateEvidenceRefSucceeded(evidence, ref) {
					return completionGateNeedsPlanAction("invalid_evidence_ref:"+ref, "completed plan phase references evidence that is missing or not successful"), true
				}
			}
		case "skipped":
			hasSkipped = true
		case "pending", "in_progress", "":
			return completionGateNeedsPlanAction("pending_phase:"+id, "model-maintained plan still contains unfinished work"), true
		default:
			return completionGateNeedsPlanAction("pending_phase:"+id, "model-maintained plan contains an unsupported phase status"), true
		}
	}
	if hasSkipped {
		return completionGateModelVerifierDecision("completed plan contains a skipped phase and needs a compact answer fidelity audit"), true
	}
	return completionGateDecision{}, false
}

func completionGateNeedsPlanAction(missing string, reason string) completionGateDecision {
	return completionGateDecision{
		Path:           completionGateNeedsAction,
		Reason:         strings.TrimSpace(reason),
		MissingFacts:   []string{missing},
		NextActionHint: "review the latest plan and evidence, then update the plan or continue the necessary work without following a fixed tool script",
	}
}

func completionGateEvidenceRefSucceeded(evidence map[string]interface{}, ref string) bool {
	ref = canonicalPlanEvidenceRef(ref)
	if ref == "" {
		return false
	}
	if strings.HasPrefix(ref, "turn_state:") {
		key := strings.TrimSpace(strings.TrimPrefix(ref, "turn_state:"))
		turnState := evidenceMapFromAny(evidence["turn_state"])
		for _, item := range evidenceMapsFromAny(turnState["items"]) {
			if strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(item["key"])), key) {
				return true
			}
		}
		return false
	}
	if strings.HasPrefix(ref, "page_context:") {
		route := strings.TrimSpace(strings.TrimPrefix(ref, "page_context:"))
		current := evidenceMapFromAny(evidence["current_page_context"])
		return strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(current["status"])), "ready") &&
			strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(current["route"])), route)
	}
	entries := evidenceMapsFromAny(evidence["evidence_ledger"])
	if len(entries) == 0 {
		entries = evidenceMapsFromAny(evidenceMapFromAny(evidence["operation_plan"])["evidence_ledger"])
	}
	if completionGateRefMatchesSuccessfulRecord(entries, ref) {
		return true
	}
	if strings.HasPrefix(ref, "tool:") {
		pair := strings.TrimSpace(strings.TrimPrefix(ref, "tool:"))
		skillID, toolName, ok := strings.Cut(pair, "/")
		if ok && completionGateHasSuccessfulToolRecord(entries, skillID, toolName) {
			return true
		}
	}
	for _, key := range []string{"client_actions", "skill_invocations"} {
		if completionGateRefMatchesSuccessfulRecord(evidenceMapsFromAny(evidence[key]), ref) {
			return true
		}
	}
	return false
}

func completionGateTerminalBlockerError(gate completionGateDecision) error {
	missing := ""
	if len(gate.MissingFacts) > 0 {
		missing = strings.TrimSpace(gate.MissingFacts[0])
	}
	if strings.HasPrefix(missing, "pending_phase:") || strings.HasPrefix(missing, "invalid_evidence_ref:") {
		return fmt.Errorf("%w: completion plan blocker: %s", ErrInvalidInput, firstNonEmptyString(missing, gate.Reason))
	}
	return fmt.Errorf("%w: completion runtime blocker: %s", ErrInvalidInput, firstNonEmptyString(missing, gate.Reason))
}

func completionGateHasSuccessfulToolRecord(records []map[string]interface{}, skillID string, toolName string) bool {
	for _, record := range records {
		if !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(record["skill_id"])), strings.TrimSpace(skillID)) ||
			!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(record["tool_name"])), strings.TrimSpace(toolName)) {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(record["status"])))
		if status == "success" || status == "succeeded" || status == "completed" || status == "verified" {
			return true
		}
	}
	return false
}

func completionGateRefMatchesSuccessfulRecord(records []map[string]interface{}, ref string) bool {
	for _, record := range records {
		matched := false
		for _, key := range []string{"invocation_id", "runtime_id", "action_id", "call_id"} {
			value := strings.TrimSpace(evidenceStringFromAny(record[key]))
			if value == "" {
				continue
			}
			if ref == value || ref == key+":"+value {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(record["status"])))
		switch status {
		case "success", "succeeded", "completed", "complete", "pass", "verified", "recorded":
			return true
		default:
			return false
		}
	}
	return false
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

func completionGateHasTerminalRuntimeBlocker(evidence map[string]interface{}) bool {
	for _, key := range []string{"pending_approval", "pending_client_action", "pending_question", "pending_user_input"} {
		if completionVerificationEvidenceValuePresent(evidence[key]) {
			return true
		}
	}
	turnState := evidenceMapFromAny(evidence["turn_state"])
	for _, item := range evidenceMapsFromAny(turnState["open_items"]) {
		status := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(item["status"])))
		switch status {
		case "resolved", "recovered", "completed", "success", "succeeded", "ignored":
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

func completionGateRecord(req RunRequest, decision completionGateDecision) {
	if req.OnCompletionGateDecision == nil {
		return
	}
	req.OnCompletionGateDecision(CompletionGateDecisionRecord{
		Path:         string(decision.Path),
		Reason:       strings.TrimSpace(decision.Reason),
		MissingFacts: append([]string(nil), decision.MissingFacts...),
	})
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
