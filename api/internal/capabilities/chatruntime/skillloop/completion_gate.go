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
	if len(evidence) == 0 {
		return completionGateModelVerifierDecision("no completion evidence available")
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
	if fastPathCompletionEvidenceNeedsAgentConfigPostRead(evidence) {
		return completionGateDecision{
			Path:           completionGateNeedsAction,
			Reason:         "requested Agent config update still lacks post-update read verification",
			MissingFacts:   []string{"missing_fact: agent.config.post_update_read"},
			NextActionHint: "continue from the latest evidence until fresh Agent config facts verify the requested fields",
		}
	}
	if answer, ok := FastPathPreferredFinalAnswerForCompletionEvidence(evidence, candidateAnswer); ok {
		return completionGatePassDecision(answer, "normalized evidence ledger verifies the requested operation")
	}
	if answer, ok := FastPathFinalAnswerForCompletionEvidence(evidence); ok {
		return completionGatePassDecision(answer, "normalized evidence ledger verifies the requested operation")
	}
	if completionVerificationLedgerSatisfiesAgentConfigPostRead(evidence) {
		answer, _ := agentConfigPostUpdateVerifiedFastPathAnswerFromEvidence(evidence)
		return completionGatePassDecision(answer, "normalized evidence ledger verifies the requested Agent configuration")
	}
	if completionGateCanAcceptCandidateAnswerFromEvidence(evidence, candidateAnswer) {
		return completionGatePassDecision(candidateAnswer, "main model final answer is backed by successful runtime evidence")
	}
	if completionGateHasOnlyLegacyFacts(evidence) {
		return completionGateModelVerifierDecision("completion evidence is legacy-only and needs answer fidelity audit")
	}
	return completionGateModelVerifierDecision("completion requires model verifier audit")
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
	if !completionVerificationHasSuccessfulEvidence(evidence) {
		return false
	}
	if fastPathEvidenceHasUnresolvedPlanFailure(evidence) ||
		fastPathHasAgentConfigTargetMismatch(evidence) ||
		fastPathCompletionEvidenceNeedsAgentConfigPostRead(evidence) {
		return false
	}
	return fastPathEvidenceHasSuccessfulAgentConfigUpdate(evidence)
}

func completionGateNotify(req RunRequest, decision completionGateDecision, fallbackAnswer string) {
	if req.OnCompletionVerification == nil {
		return
	}
	notifyCompletionVerificationResult(req, decision.completionVerificationDecision(), firstNonEmptyString(decision.FinalAnswer, fallbackAnswer))
}

func (d completionGateDecision) completionVerificationDecision() completionVerificationDecision {
	status := completionVerificationStatusFailed
	switch d.Path {
	case completionGateDeterministicPass:
		status = completionVerificationStatusPass
	case completionGateNeedsAction:
		status = completionVerificationStatusNeedsAction
	case completionGateAskUser:
		status = completionVerificationStatusAskUser
	case completionGateModelVerifier:
		status = completionVerificationStatusFailed
	}
	return completionVerificationDecision{
		Status:              status,
		Source:              "completion_gate",
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
