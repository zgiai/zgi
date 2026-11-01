package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/connectortools"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const operationPlanLedgerLimit = 100

func operationPlanRecordActionAttempt(plan map[string]interface{}, invocation map[string]interface{}, status string) {
	if len(plan) == 0 || len(invocation) == 0 {
		return
	}
	id := operationPlanInvocationPlanID(invocation)
	if id == "" {
		return
	}
	attempt := map[string]interface{}{
		"id":        id,
		"kind":      strings.TrimSpace(stringFromAny(invocation["kind"])),
		"status":    status,
		"skill_id":  strings.TrimSpace(stringFromAny(invocation["skill_id"])),
		"tool_name": strings.TrimSpace(stringFromAny(invocation["tool_name"])),
	}
	if sequence := operationPlanInvocationSequence(invocation); sequence > 0 {
		attempt["sequence"] = sequence
	}
	if target := operationPlanInvocationExpectedActionTarget(invocation); len(target) > 0 {
		attempt["target"] = target
	}
	if args := mapFromOperationContext(invocation["arguments"]); len(args) > 0 {
		if phaseID := strings.TrimSpace(stringFromAny(args["plan_phase_id"])); phaseID != "" {
			attempt["plan_phase_id"] = phaseID
			if outcomeID := operationPlanOutcomeIDForPhase(plan, phaseID); outcomeID != "" {
				attempt["outcome_id"] = outcomeID
			}
		}
	}
	if proof, native := connectortools.Evidence(invocation); native {
		attempt["operation"] = proof.Operation
		attempt["outcome_id"] = proof.OutcomeID.String()
		attempt["status"] = proof.Status
	}
	if message := strings.TrimSpace(firstNonEmptyString(invocation["error"], invocation["message"])); message != "" {
		attempt["message"] = truncateRunes(message, 240)
	}
	attempts := mapSliceFromAny(plan[operationPlanActionAttemptsKey])
	replaced := false
	for index, existing := range attempts {
		if strings.TrimSpace(stringFromAny(existing["id"])) != id {
			continue
		}
		attempts[index] = mergeInvocation(existing, attempt)
		replaced = true
		break
	}
	if !replaced {
		attempt["created_at"] = operationPlanInvocationCreatedAt(invocation)
		attempts = append(attempts, attempt)
	}
	if len(attempts) > operationPlanLedgerLimit {
		attempts = attempts[len(attempts)-operationPlanLedgerLimit:]
	}
	plan[operationPlanActionAttemptsKey] = mapsToInterfaceSlice(attempts)
}

func operationPlanInvocationCreatedAt(invocation map[string]interface{}) string {
	if value := strings.TrimSpace(firstNonEmptyString(invocation["created_at"], invocation["started_at"])); value != "" {
		return value
	}
	return time.Now().UTC().Format(time.RFC3339)
}

func operationPlanOutcomeIDForPhase(plan map[string]interface{}, phaseID string) string {
	phaseID = strings.TrimSpace(phaseID)
	for _, phase := range mapSliceFromAny(plan["phases"]) {
		if strings.TrimSpace(stringFromAny(phase["id"])) == phaseID {
			return strings.TrimSpace(stringFromAny(phase["outcome_id"]))
		}
	}
	return ""
}

func operationPlanRecordInvocationEffects(plan map[string]interface{}, invocation map[string]interface{}, status string) {
	if len(plan) == 0 || status != operationPlanStepStatusCompleted {
		return
	}
	for _, effect := range operationPlanEffectsFromInvocation(plan, invocation) {
		operationPlanAppendEffect(plan, effect)
	}
}

func operationPlanEffectsFromInvocation(plan map[string]interface{}, invocation map[string]interface{}) []map[string]interface{} {
	if invocation["runtime_source"] == "connector" {
		return connectorPlanEffects(plan, invocation)
	}
	if len(invocation) == 0 {
		return nil
	}
	kind := strings.TrimSpace(stringFromAny(invocation["kind"]))
	skillID := strings.TrimSpace(stringFromAny(invocation["skill_id"]))
	toolName := strings.TrimSpace(stringFromAny(invocation["tool_name"]))
	result := mapFromOperationContext(invocation["result"])
	arguments := mapFromOperationContext(invocation["arguments"])
	sourceID := operationPlanInvocationPlanID(invocation)
	newEffect := func(effectType, resourceType, resourceID string) map[string]interface{} {
		effect := map[string]interface{}{
			"id":            fmt.Sprintf("%s:%s", effectType, sourceID),
			"type":          effectType,
			"source_action": sourceID,
			"status":        operationPlanStepStatusCompleted,
			"created_at":    operationPlanInvocationCreatedAt(invocation),
		}
		if resourceType != "" {
			effect["resource_type"] = resourceType
		}
		if resourceID != "" {
			effect["resource_id"] = resourceID
		}
		return effect
	}

	switch {
	case kind == "client_action" && operationPlanInvocationIsConsoleRouteNavigation(invocation):
		href := operationPlanInvocationHref(invocation)
		if href == "" {
			return nil
		}
		effect := newEffect("page.observed", "page", href)
		effect["route"] = href
		return []map[string]interface{}{effect}
	case kind == "tool_call" && isConsoleNavigatorNavigateTool(skillID, toolName):
		href := operationPlanInvocationHref(invocation)
		effect := newEffect("navigation.requested", "page", href)
		if href != "" {
			effect["route"] = href
		}
		return []map[string]interface{}{effect}
	case kind == "tool_call" && strings.EqualFold(skillID, skills.SkillFileReader) && strings.EqualFold(toolName, "read_file"):
		fileID := operationPlanFileIdentity(result, arguments)
		effect := newEffect("file.content.read", "file", fileID)
		if contentStatus := strings.TrimSpace(stringFromAny(result["content_status"])); contentStatus != "" {
			effect["content_status"] = contentStatus
		}
		if chars := intValueFromAny(result["content_chars"]); chars > 0 {
			effect["content_chars"] = chars
		}
		return []map[string]interface{}{effect}
	case kind == "tool_call" && isKnownArtifactGeneratorToolCall(skillID, toolName):
		fileID := operationPlanFileIdentity(result, arguments)
		effect := newEffect("artifact.generated", "artifact", fileID)
		operationPlanCopyEffectFileFacts(effect, result, arguments)
		return []map[string]interface{}{effect}
	case kind == "tool_call" && isFileManagerSaveToolCall(skillID, toolName):
		fileID := operationPlanFileIdentity(result, arguments)
		effect := newEffect("file.persisted", "file", fileID)
		operationPlanCopyEffectFileFacts(effect, result, arguments)
		return []map[string]interface{}{effect}
	case kind == "tool_call" && strings.EqualFold(skillID, skills.SkillAgentManagement) && strings.EqualFold(toolName, "get_agent_config"):
		agentID := firstNonEmptyString(result["agent_id"], arguments["agent_id"], operationPlanAgentResultField(result, "agent_id"))
		return []map[string]interface{}{newEffect("agent.config.read", "agent", agentID)}
	case kind == "tool_call" && strings.EqualFold(skillID, skills.SkillAgentManagement) && strings.EqualFold(toolName, "update_agent_config"):
		agentID := firstNonEmptyString(result["agent_id"], arguments["agent_id"], operationPlanAgentResultField(result, "agent_id"))
		effect := newEffect("agent.config.updated", "agent", agentID)
		fields := operationPlanNormalizedAgentConfigFieldsFromAny(result["updated_fields"])
		fields = appendUniqueStrings(fields, operationPlanNormalizedAgentConfigFieldsFromAny(result["satisfied_fields"])...)
		if len(fields) == 0 {
			fields = operationPlanAgentConfigFieldsFromArguments(arguments)
		}
		if len(fields) > 0 {
			effect["fields"] = fields
		}
		return []map[string]interface{}{effect}
	default:
		return nil
	}
}

func operationPlanFileIdentity(values ...map[string]interface{}) string {
	for _, value := range values {
		if id := strings.TrimSpace(firstNonEmptyString(
			value["managed_file_id"], value["upload_file_id"], value["file_id"], value["tool_file_id"], value["artifact_id"], value["source_file_id"],
		)); id != "" {
			return id
		}
		if file := mapFromOperationContext(value["file"]); len(file) > 0 {
			if id := strings.TrimSpace(firstNonEmptyString(file["managed_file_id"], file["upload_file_id"], file["file_id"], file["id"])); id != "" {
				return id
			}
		}
	}
	return ""
}

func operationPlanCopyEffectFileFacts(effect map[string]interface{}, values ...map[string]interface{}) {
	for _, value := range values {
		for _, key := range []string{"filename", "file_name", "managed_filename", "mime_type", "size", "sha256", "digest"} {
			if _, exists := effect[key]; exists {
				continue
			}
			if fact := value[key]; fact != nil && strings.TrimSpace(stringFromAny(fact)) != "" {
				effect[key] = fact
			}
		}
	}
}

func operationPlanAgentConfigFieldsFromArguments(arguments map[string]interface{}) []string {
	fields := []string{}
	for _, key := range []string{
		"model", "model_provider", "system_prompt", "agent_memory_enabled", "file_upload_enabled", "suggested_questions",
		"enabled_skill_ids", "knowledge_dataset_ids", "database_bindings", "workflow_bindings",
	} {
		if _, exists := arguments[key]; exists {
			fields = appendUniqueStrings(fields, operationPlanAgentConfigCanonicalField(key))
		}
	}
	if _, exists := arguments["system_prompt_source"]; exists {
		fields = appendUniqueStrings(fields, "system_prompt")
	}
	if _, exists := arguments["system_prompt_patch"]; exists {
		fields = appendUniqueStrings(fields, "system_prompt")
	}
	return fields
}

func operationPlanAppendEffect(plan map[string]interface{}, effect map[string]interface{}) {
	if len(plan) == 0 || len(effect) == 0 {
		return
	}
	effects := mapSliceFromAny(plan[operationPlanEffectLedgerKey])
	for _, existing := range effects {
		if strings.TrimSpace(stringFromAny(existing["id"])) == strings.TrimSpace(stringFromAny(effect["id"])) {
			return
		}
	}
	effects = append(effects, effect)
	if len(effects) > operationPlanLedgerLimit {
		effects = effects[len(effects)-operationPlanLedgerLimit:]
	}
	plan[operationPlanEffectLedgerKey] = mapsToInterfaceSlice(effects)
}

func operationPlanReconcileOutcomes(plan map[string]interface{}) bool {
	if operationPlanIsTerminalFailure(plan) {
		return false
	}
	operationPlanBindReferencedPhaseActions(plan)
	outcomes := mapSliceFromAny(plan[operationPlanOutcomesKey])
	if len(outcomes) == 0 {
		return false
	}
	for _, outcome := range outcomes {
		if stringFromAny(outcome["completion_source"]) == "runtime_phase_evidence" {
			outcome["status"] = operationPlanStepStatusPending
			delete(outcome, "completed_at")
			delete(outcome, "effect_refs")
		}
	}
	effects := mapSliceFromAny(plan[operationPlanEffectLedgerKey])
	changed := false
	for pass := 0; pass < len(outcomes); pass++ {
		passChanged := false
		for _, outcome := range outcomes {
			if !operationPlanPhaseOpen(outcome) || !operationPlanOutcomeDependenciesSatisfied(outcome, outcomes) {
				continue
			}
			acceptance := mapFromOperationContext(outcome["acceptance"])
			specs := mapSliceFromAny(acceptance["effects"])
			if len(specs) == 0 {
				if refs, satisfied := operationPlanOutcomePhaseEvidence(plan, outcome, false); satisfied {
					outcome["status"] = operationPlanStepStatusCompleted
					outcome["completed_at"] = time.Now().UTC().Format(time.RFC3339)
					outcome["effect_refs"] = refs
					outcome["completion_source"] = "runtime_phase_evidence"
					passChanged, changed = true, true
				}
				continue
			}
			refs := make([]string, 0, len(specs))
			allSatisfied := true
			for _, spec := range specs {
				ref := operationPlanMatchingEffectID(spec, effects)
				if ref == "" {
					allSatisfied = false
					break
				}
				refs = appendUniqueStrings(refs, ref)
			}
			if !allSatisfied {
				continue
			}
			outcome["status"] = operationPlanStepStatusCompleted
			outcome["completed_at"] = time.Now().UTC().Format(time.RFC3339)
			outcome["effect_refs"] = refs
			passChanged = true
			changed = true
		}
		if !passChanged {
			break
		}
	}
	plan[operationPlanOutcomesKey] = mapsToInterfaceSlice(outcomes)
	operationPlanSyncOutcomePhases(plan, outcomes)
	if operationPlanOutcomesTerminal(outcomes) {
		plan["status"] = operationPlanStatusCompleted
		plan["pending_next_action"] = "none"
	} else {
		plan["status"] = operationPlanStatusRunning
		plan["pending_next_action"] = "continue_unsatisfied_outcomes"
	}
	return changed
}

// Resolve only explicit phase contracts against the server-owned invocation
// ledger. Model status, notes, and evidence_refs are never completion proof.
// Domain effect acceptance is evaluated separately and cannot be weakened here.
func operationPlanOutcomePhaseEvidence(plan, outcome map[string]interface{}, finalAnswer bool) ([]string, bool) {
	var refs []string
	linked := false
	for _, phase := range mapSliceFromAny(plan["phases"]) {
		if stringFromAny(phase["outcome_id"]) != stringFromAny(outcome["id"]) {
			continue
		}
		linked = true
		expected := mapFromOperationContext(phase["expected_action"])
		if len(expected) == 0 {
			classification := copyStringAnyMap(phase)
			delete(classification, "outcome_id")
			if !finalAnswer || !operationPlanPhaseCanCompleteWithFinalAnswer(classification) {
				return nil, false
			}
			refs = appendUniqueStrings(refs, "final_answer:main_model_final")
			continue
		}
		// Connector operations require their stricter server-projected effect and
		// ledger-epoch binding, never the generic tool-success fallback.
		if strings.EqualFold(stringFromAny(expected["skill_id"]), skills.SkillConnectorRuntime) {
			return nil, false
		}
		ledger := mapSliceFromAny(plan["evidence_ledger"])
		ref := ""
		attemptID := ""
		attempts := mapSliceFromAny(plan[operationPlanActionAttemptsKey])
		for index := len(attempts) - 1; index >= 0; index-- {
			attempt := attempts[index]
			if !operationPlanPhaseAttemptMatches(plan, phase, attempt) {
				continue
			}
			if !operationPlanEvidenceEntrySucceeded(attempt) {
				return nil, false
			}
			attemptID = stringFromAny(attempt["id"])
			break
		}
		if attemptID == "" {
			return nil, false
		}
		for index := len(ledger) - 1; index >= 0; index-- {
			entry := ledger[index]
			if !operationPlanEntryHasInvocation(entry, attemptID) {
				continue
			}
			if stringFromAny(entry["kind"]) != "tool_call" {
				continue
			}
			if operationPlanEvidenceEntrySucceeded(entry) {
				ref = stringFromAny(entry["invocation_id"])
			}
			break
		}
		if ref == "" {
			return nil, false
		}
		refs = appendUniqueStrings(refs, ref)
	}
	return refs, linked && len(refs) > 0
}

func operationPlanPhaseAttemptMatches(plan, phase, attempt map[string]interface{}) bool {
	if stringFromAny(attempt["kind"]) != "tool_call" || !operationPlanExpectedActionMatches(mapFromOperationContext(phase["expected_action"]),
		stringFromAny(attempt["skill_id"]), stringFromAny(attempt["tool_name"]), mapFromOperationContext(attempt["target"])) {
		return false
	}
	if id := stringFromAny(attempt["plan_phase_id"]); id != "" {
		return id == stringFromAny(phase["id"])
	}
	// A coarse tool identity must not satisfy two distinct phases using one
	// call. Repeated identical bindings require explicit phase correlation.
	for _, other := range mapSliceFromAny(plan["phases"]) {
		if stringFromAny(other["id"]) != stringFromAny(phase["id"]) && operationPlanExpectedActionMatches(mapFromOperationContext(other["expected_action"]),
			stringFromAny(attempt["skill_id"]), stringFromAny(attempt["tool_name"]), mapFromOperationContext(attempt["target"])) {
			return false
		}
	}
	return true
}

func operationPlanOutcomeDependenciesSatisfied(outcome map[string]interface{}, outcomes []map[string]interface{}) bool {
	for _, dependency := range stringSliceFromAny(outcome["depends_on"]) {
		found := false
		for _, candidate := range outcomes {
			if strings.TrimSpace(stringFromAny(candidate["id"])) != strings.TrimSpace(dependency) {
				continue
			}
			found = true
			status := strings.ToLower(strings.TrimSpace(stringFromAny(candidate["status"])))
			if status != operationPlanStepStatusCompleted && status != "skipped" {
				return false
			}
			break
		}
		if !found {
			return false
		}
	}
	return true
}

func operationPlanMatchingEffectID(spec map[string]interface{}, effects []map[string]interface{}) string {
	for _, effect := range effects {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(spec["type"])), strings.TrimSpace(stringFromAny(effect["type"]))) {
			continue
		}
		if resourceType := strings.TrimSpace(stringFromAny(spec["resource_type"])); resourceType != "" &&
			!strings.EqualFold(resourceType, strings.TrimSpace(stringFromAny(effect["resource_type"]))) {
			continue
		}
		if resourceID := strings.TrimSpace(stringFromAny(spec["resource_id"])); resourceID != "" &&
			resourceID != strings.TrimSpace(stringFromAny(effect["resource_id"])) {
			continue
		}
		if requiredFields := stringSliceFromAny(spec["fields"]); len(requiredFields) > 0 &&
			!operationPlanStringSetContains(stringSliceFromAny(effect["fields"]), requiredFields) {
			continue
		}
		return strings.TrimSpace(stringFromAny(effect["id"]))
	}
	return ""
}

func operationPlanStringSetContains(actual []string, required []string) bool {
	for _, expected := range required {
		found := false
		for _, value := range actual {
			if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(expected)) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func operationPlanSyncOutcomePhases(plan map[string]interface{}, outcomes []map[string]interface{}) {
	phases := mapSliceFromAny(plan["phases"])
	for _, phase := range phases {
		outcomeID := strings.TrimSpace(stringFromAny(phase["outcome_id"]))
		if outcomeID == "" {
			continue
		}
		for _, outcome := range outcomes {
			if strings.TrimSpace(stringFromAny(outcome["id"])) != outcomeID {
				continue
			}
			phase["status"] = outcome["status"]
			if completedAt := strings.TrimSpace(stringFromAny(outcome["completed_at"])); completedAt != "" {
				phase["completed_at"] = completedAt
			}
			if refs := stringSliceFromAny(outcome["effect_refs"]); len(refs) > 0 {
				phase["effect_refs"] = refs
			}
			break
		}
	}
	operationPlanAdvanceNextPendingPhase(phases)
	plan["phases"] = mapsToInterfaceSlice(phases)
}

func operationPlanOutcomesTerminal(outcomes []map[string]interface{}) bool {
	if len(outcomes) == 0 {
		return false
	}
	for _, outcome := range outcomes {
		required := true
		if raw, exists := outcome["required"]; exists {
			required = operationPlanBoolValue(raw)
		}
		if !required {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(stringFromAny(outcome["status"]))) {
		case operationPlanStepStatusCompleted, "skipped":
		default:
			return false
		}
	}
	return true
}

func operationPlanHasStructuredOutcomes(plan map[string]interface{}) bool {
	return len(mapSliceFromAny(plan[operationPlanOutcomesKey])) > 0
}
