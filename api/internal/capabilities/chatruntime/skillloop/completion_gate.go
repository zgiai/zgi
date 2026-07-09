package skillloop

import (
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/skills"
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
	if gaps := completionGateContractCoverageGaps(evidence); len(gaps) > 0 {
		path := completionGateNeedsAction
		finalAnswer := ""
		if completionGateContractNeedsClarification(evidence) && completionGateContractEffectHasConflictingEvidence(evidence) {
			path = completionGateAskUser
			finalAnswer = completionGateClarificationAnswer(evidence, strings.Join(gaps, "; "))
		}
		return completionGateDecision{
			Path:         path,
			Reason:       strings.Join(gaps, "; "),
			MissingFacts: gaps,
			NextActionHint: "continue from the latest evidence until the missing facts are present in evidence_ledger; " +
				"the model should choose the concrete next action from available tools and current context",
			FinalAnswer: finalAnswer,
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

func completionGateContractEffectMismatch(evidence map[string]interface{}) string {
	gaps := completionGateMissingExpectedEffectFacts(evidence)
	if len(gaps) == 0 {
		return ""
	}
	return gaps[0]
}

func completionGateContractCoverageGaps(evidence map[string]interface{}) []string {
	if len(evidence) == 0 {
		return nil
	}
	gaps := completionGateMissingExpectedEffectFacts(evidence)
	if completionGateDeclaredNeedsManagedPDFSave(evidence) && !completionGateEvidenceHasManagedFileWithExtension(evidence, "pdf") {
		gaps = append(gaps, "missing_fact: file.management.saved_pdf")
	}
	return dedupeStrings(gaps)
}

func completionGateMissingExpectedEffectFacts(evidence map[string]interface{}) []string {
	expected := completionGateExpectedEffects(evidence)
	if len(expected) == 0 {
		return nil
	}
	actual := completionGateActualEffects(evidence)
	gaps := []string{}
	for effect := range expected {
		if _, ok := actual[effect]; ok {
			continue
		}
		gaps = append(gaps, completionGateMissingFactForEffect(effect))
	}
	return dedupeStrings(gaps)
}

func completionGateContractEffectHasConflictingEvidence(evidence map[string]interface{}) bool {
	expected := completionGateExpectedEffects(evidence)
	if len(expected) == 0 {
		return false
	}
	actual := completionGateActualEffects(evidence)
	if len(actual) == 0 {
		return false
	}
	for effect := range expected {
		if actual[effect] {
			continue
		}
		return true
	}
	return false
}

func completionGateMissingFactForEffect(effect string) string {
	switch strings.TrimSpace(effect) {
	case "agent.system_prompt_update":
		return "missing_fact: agent.config.system_prompt_verified"
	case "agent.knowledge_binding":
		return "missing_fact: agent.knowledge_binding"
	default:
		if effect == "" {
			return "missing_fact: requested_effect"
		}
		return "missing_fact: " + effect
	}
}

func completionGateExpectedEffects(evidence map[string]interface{}) map[string]bool {
	out := map[string]bool{}
	for _, contract := range completionGateTaskContracts(evidence) {
		completionGateAddNormalizedEffects(out,
			contract["intended_effect"],
			contract["asset_effect"],
			contract["task_type"],
			contract["execution_intent"],
		)
		completionGateAddEffectsFromCapabilityGoals(out, contract["capability_goals"])
	}
	return completionGateNonEmptyEffectSet(out)
}

func completionGateAddEffectsFromCapabilityGoals(out map[string]bool, value interface{}) {
	if out == nil {
		return
	}
	for _, goal := range evidenceMapsFromAny(value) {
		action := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(goal["goal_action"])))
		switch action {
		case "update", "set", "replace", "enable", "disable", "bind", "unbind", "create", "delete", "remove", "add":
			completionGateAddNormalizedEffects(out,
				goal["intended_effect"],
				goal["capability_id"],
				goal["display_name"],
				goal["required_config_fields"],
				goal["required_binding_actions"],
			)
		default:
			continue
		}
	}
}

func completionGateTaskContracts(evidence map[string]interface{}) []map[string]interface{} {
	contracts := make([]map[string]interface{}, 0, 3)
	if contract := evidenceMapFromAny(evidence["task_contract"]); len(contract) > 0 {
		contracts = append(contracts, contract)
	}
	if contract := evidenceMapFromAny(evidence["turn_task_contract"]); len(contract) > 0 {
		contracts = append(contracts, contract)
	}
	for _, plan := range completionVerificationEvidenceOperationPlans(evidence) {
		if contract := evidenceMapFromAny(plan["task_contract"]); len(contract) > 0 {
			contracts = append(contracts, contract)
		}
	}
	return contracts
}

func completionGateActualEffects(evidence map[string]interface{}) map[string]bool {
	out := map[string]bool{}
	for _, invocation := range completionVerificationEvidenceInvocations(evidence) {
		if !completionVerificationInvocationSucceeded(invocation) {
			continue
		}
		completionGateAddEffectFromInvocation(out, invocation)
	}
	return completionGateNonEmptyEffectSet(out)
}

func completionGateAddEffectFromInvocation(out map[string]bool, invocation map[string]interface{}) {
	if out == nil || len(invocation) == 0 {
		return
	}
	completionGateAddNormalizedEffects(out,
		evidenceMapFromAny(invocation["result_facts"])["effect"],
		evidenceMapFromAny(invocation["result_summary"])["effect"],
		evidenceMapFromAny(invocation["result"])["effect"],
	)
	skillID := strings.TrimSpace(evidenceStringFromAny(invocation["skill_id"]))
	toolName := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(invocation["tool_name"])))
	if !strings.EqualFold(skillID, skills.SkillAgentManagement) {
		return
	}
	switch toolName {
	case "replace_agent_knowledge_bindings":
		out["agent.knowledge_binding"] = true
	case "update_agent_config":
		source := completionVerificationInvocationFactSource(invocation)
		fields := append(completionVerificationCanonicalAgentConfigFields(source["updated_fields"]),
			completionVerificationCanonicalAgentConfigFields(source["satisfied_fields"])...)
		for _, field := range fields {
			switch field {
			case "system_prompt":
				out["agent.system_prompt_update"] = true
			case "knowledge_dataset_ids":
				out["agent.knowledge_binding"] = true
			}
		}
		if status := evidenceMapFromAny(source["field_status"]); strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(status["system_prompt"])), "verified") {
			out["agent.system_prompt_update"] = true
		}
	}
}

func completionGateAddNormalizedEffects(out map[string]bool, values ...interface{}) {
	if out == nil {
		return
	}
	for _, value := range values {
		for _, text := range completionGateFlattenEffectText(value) {
			switch completionGateNormalizeEffect(text) {
			case "agent.system_prompt_update":
				out["agent.system_prompt_update"] = true
			case "agent.knowledge_binding":
				out["agent.knowledge_binding"] = true
			}
		}
	}
}

func completionGateFlattenEffectText(value interface{}) []string {
	if len(evidenceMapFromAny(value)) > 0 {
		out := []string{}
		for _, nested := range evidenceMapFromAny(value) {
			out = append(out, completionGateFlattenEffectText(nested)...)
		}
		return out
	}
	if values := evidenceStringSliceFromAny(value); len(values) > 0 {
		return values
	}
	text := strings.TrimSpace(evidenceStringFromAny(value))
	if text == "" {
		return nil
	}
	return []string{text}
}

func completionGateNormalizeEffect(value string) string {
	text := strings.ToLower(strings.TrimSpace(value))
	if text == "" {
		return ""
	}
	switch {
	case strings.Contains(text, "agent.system_prompt_update"),
		strings.Contains(text, "agent.system_prompt"),
		strings.Contains(text, "system_prompt"),
		strings.Contains(text, "system prompt"):
		return "agent.system_prompt_update"
	case strings.Contains(text, "agent.knowledge_binding"),
		strings.Contains(text, "knowledge_binding"),
		strings.Contains(text, "knowledge binding"),
		strings.Contains(text, "knowledge_dataset"),
		strings.Contains(text, "knowledge dataset"):
		return "agent.knowledge_binding"
	default:
		return ""
	}
}

func completionGateNonEmptyEffectSet(values map[string]bool) map[string]bool {
	for key, ok := range values {
		if ok && strings.TrimSpace(key) != "" {
			return values
		}
	}
	return nil
}

func completionGateContractNeedsClarification(evidence map[string]interface{}) bool {
	for _, contract := range completionGateTaskContracts(evidence) {
		if boolFromAny(contract["low_confidence"]) ||
			boolFromAny(contract["needs_clarification"]) ||
			strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(contract["status"])), "ambiguous") {
			return true
		}
	}
	return false
}

func completionGateClarificationAnswer(evidence map[string]interface{}, reason string) string {
	if !completionVerificationUserRequestedChinese(evidence) {
		return "I need to confirm the intended Agent update before making or claiming another change: should this be written into the Agent system prompt, or bound as a knowledge file?"
	}
	if strings.Contains(reason, "agent.knowledge_binding") {
		return "我需要先确认一下：你是希望把这个文件绑定为智能体知识库，还是把文件里的设定写入智能体的系统提示词？这两种会产生不同配置变更。"
	}
	return "我需要先确认一下：你是希望更新智能体的系统提示词，还是绑定文件作为知识来源？这两种会产生不同配置变更。"
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

func completionGateDeclaredNeedsManagedPDFSave(evidence map[string]interface{}) bool {
	if len(evidence) == 0 {
		return false
	}
	for _, contract := range completionGateTaskContracts(evidence) {
		if completionGateTextSetRequiresManagedPDFSave(completionGateCoverageTextValues(contract)) {
			return true
		}
	}
	for _, plan := range completionVerificationEvidenceOperationPlans(evidence) {
		if completionGateTextSetRequiresManagedPDFSave(completionGateCoverageTextValues(plan)) {
			return true
		}
		for _, phase := range evidenceMapsFromAny(plan["phases"]) {
			if completionGateTextSetRequiresManagedPDFSave(completionGateCoverageTextValues(phase)) {
				return true
			}
		}
		if structured := evidenceMapFromAny(plan["structured_plan"]); len(structured) > 0 {
			for _, operation := range evidenceMapsFromAny(structured["operations"]) {
				if completionGateTextSetRequiresManagedPDFSave(completionGateCoverageTextValues(operation)) {
					return true
				}
			}
		}
	}
	return false
}

func completionGateCoverageTextValues(source map[string]interface{}) []string {
	if len(source) == 0 {
		return nil
	}
	values := []string{}
	for _, key := range []string{
		"id",
		"title",
		"intent",
		"task_type",
		"asset_effect",
		"pending_next_action",
		"required_next_action",
		"next_action",
	} {
		if text := strings.TrimSpace(evidenceStringFromAny(source[key])); text != "" {
			values = append(values, text)
		}
	}
	for _, key := range []string{
		"phases",
		"success_criteria",
		"completion_criteria",
		"evidence_required",
		"recommended_capabilities",
		"observation_points",
	} {
		values = append(values, evidenceStringSliceFromAny(source[key])...)
	}
	return values
}

func completionGateTextSetRequiresManagedPDFSave(values []string) bool {
	for _, value := range values {
		if completionGateTextRequiresManagedPDFSave(value) {
			return true
		}
	}
	return false
}

func completionGateTextRequiresManagedPDFSave(value string) bool {
	text := strings.ToLower(strings.TrimSpace(value))
	if text == "" {
		return false
	}
	normalized := strings.NewReplacer("_", " ", "-", " ", ".", " ").Replace(text)
	hasPDF := strings.Contains(normalized, "pdf")
	if !hasPDF {
		return false
	}
	hasSave := strings.Contains(normalized, "save") ||
		strings.Contains(normalized, "\u4fdd\u5b58")
	hasManagedFile := strings.Contains(normalized, "file management") ||
		strings.Contains(normalized, "managed file") ||
		strings.Contains(text, "file_management") ||
		strings.Contains(text, "managed_file") ||
		strings.Contains(normalized, "\u6587\u4ef6\u7ba1\u7406")
	return hasSave && hasManagedFile
}

func completionGateEvidenceHasManagedFileWithExtension(evidence map[string]interface{}, extension string) bool {
	extension = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(extension)), ".")
	if extension == "" {
		return false
	}
	for _, invocation := range completionVerificationEvidenceInvocations(evidence) {
		if !completionVerificationInvocationSucceeded(invocation) {
			continue
		}
		if !completionGateInvocationIsManagedFileSave(invocation) {
			continue
		}
		if completionGateInvocationFileExtension(invocation) == extension {
			return true
		}
	}
	return false
}

func completionGateInvocationIsManagedFileSave(invocation map[string]interface{}) bool {
	if len(invocation) == 0 {
		return false
	}
	skillID := strings.TrimSpace(evidenceStringFromAny(invocation["skill_id"]))
	toolName := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(invocation["tool_name"])))
	if strings.EqualFold(skillID, skills.SkillFileManager) && toolName == "save_file_to_management" {
		return true
	}
	for _, source := range completionGateInvocationFactSources(invocation) {
		target := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(firstNonEmptyEvidence(
			source["target"],
			source["file_lifecycle"],
			source["lifecycle"],
		))))
		switch target {
		case "managed_file", "file_management":
			return true
		}
	}
	return false
}

func completionGateInvocationFileExtension(invocation map[string]interface{}) string {
	for _, source := range completionGateInvocationFactSources(invocation) {
		if ext := completionGateFileExtensionFromSource(source); ext != "" {
			return ext
		}
	}
	return ""
}

func completionGateInvocationFactSources(invocation map[string]interface{}) []map[string]interface{} {
	sources := make([]map[string]interface{}, 0, 5)
	for _, key := range []string{"result_facts", "result_summary", "result", "target"} {
		if source := evidenceMapFromAny(invocation[key]); len(source) > 0 {
			sources = append(sources, source)
		}
	}
	return sources
}

func completionGateFileExtensionFromSource(source map[string]interface{}) string {
	if len(source) == 0 {
		return ""
	}
	for _, key := range []string{"file_extension", "extension", "format", "file_format"} {
		ext := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(source[key])))
		ext = strings.TrimPrefix(ext, ".")
		if ext != "" {
			return ext
		}
	}
	for _, key := range []string{"filename", "file_name", "name"} {
		name := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(source[key])))
		if strings.HasSuffix(name, ".pdf") {
			return "pdf"
		}
	}
	mimeType := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(firstNonEmptyEvidence(
		source["mime_type"],
		source["file_mime_type"],
		source["content_type"],
	))))
	switch mimeType {
	case "application/pdf":
		return "pdf"
	default:
		return ""
	}
}
