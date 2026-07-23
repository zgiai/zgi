package service

import (
	"fmt"
	"reflect"
	"strings"
	"time"

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
	for _, effect := range operationPlanEffectsFromInvocation(invocation) {
		operationPlanAppendEffect(plan, effect)
	}
}

func operationPlanEffectsFromInvocation(invocation map[string]interface{}) []map[string]interface{} {
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
	outcomes := mapSliceFromAny(plan[operationPlanOutcomesKey])
	if len(outcomes) == 0 {
		return false
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

func operationPlanCompleteFinalAnswerOutcomes(plan map[string]interface{}) bool {
	outcomes := mapSliceFromAny(plan[operationPlanOutcomesKey])
	if len(outcomes) == 0 {
		return false
	}
	changed := false
	for pass := 0; pass < len(outcomes); pass++ {
		passChanged := false
		for _, outcome := range outcomes {
			if !operationPlanPhaseOpen(outcome) ||
				!strings.EqualFold(strings.TrimSpace(stringFromAny(outcome["verification_mode"])), "final_answer") ||
				!operationPlanOutcomeDependenciesSatisfied(outcome, outcomes) {
				continue
			}
			outcome["status"] = operationPlanStepStatusCompleted
			outcome["completed_at"] = time.Now().UTC().Format(time.RFC3339)
			outcome["completion_source"] = "main_model_final"
			passChanged = true
			changed = true
		}
		if !passChanged {
			break
		}
	}
	if !changed {
		return false
	}
	plan[operationPlanOutcomesKey] = mapsToInterfaceSlice(outcomes)
	operationPlanSyncOutcomePhases(plan, outcomes)
	if operationPlanOutcomesTerminal(outcomes) {
		plan["status"] = operationPlanStatusCompleted
		plan["pending_next_action"] = "none"
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

func reconcileOperationPlanOutcomeSnapshot(plan map[string]interface{}, next []map[string]interface{}) []map[string]interface{} {
	current := mapSliceFromAny(plan[operationPlanOutcomesKey])
	byID := make(map[string]map[string]interface{}, len(current))
	for _, outcome := range current {
		if id := strings.TrimSpace(stringFromAny(outcome["id"])); id != "" {
			byID[id] = outcome
		}
	}
	out := make([]map[string]interface{}, 0, len(next))
	for index, raw := range next {
		id := strings.TrimSpace(stringFromAny(raw["id"]))
		if id == "" {
			id = fmt.Sprintf("outcome-amendment-%d", index+1)
		}
		required := true
		if value, exists := raw["required"]; exists {
			required = operationPlanBoolValue(value)
		}
		contract := AIChatTurnOutcome{
			ID:                 id,
			Goal:               strings.TrimSpace(firstNonEmptyString(raw["goal"], raw["title"], raw["step"])),
			TargetResourceType: strings.TrimSpace(stringFromAny(raw["target_resource_type"])),
			TargetResourceID:   strings.TrimSpace(stringFromAny(raw["target_resource_id"])),
			DependsOn:          stringSliceFromAny(raw["depends_on"]),
			Capabilities:       stringSliceFromAny(raw["capabilities"]),
			Constraints:        stringSliceFromAny(raw["constraints"]),
			Required:           &required,
		}
		acceptance := operationPlanOutcomeAcceptance(nil, contract)
		verificationMode := "runtime_unverified"
		if len(mapSliceFromAny(acceptance["effects"])) > 0 {
			verificationMode = "runtime_effects"
		}
		item := map[string]interface{}{
			"id":                id,
			"goal":              contract.Goal,
			"title":             contract.Goal,
			"status":            operationPlanStepStatusPending,
			"required":          required,
			"verification_mode": verificationMode,
		}
		if contract.TargetResourceType != "" {
			item["target_resource_type"] = contract.TargetResourceType
		}
		if contract.TargetResourceID != "" {
			item["target_resource_id"] = contract.TargetResourceID
		}
		for key, values := range map[string][]string{
			"depends_on": contract.DependsOn, "capabilities": contract.Capabilities, "constraints": contract.Constraints,
		} {
			if len(values) > 0 {
				item[key] = append([]string(nil), values...)
			}
		}
		if len(acceptance) > 0 {
			item["acceptance"] = acceptance
		}
		if previous := byID[id]; len(previous) > 0 && operationPlanOutcomeContractEqual(previous, item) {
			if mode := strings.TrimSpace(stringFromAny(previous["verification_mode"])); mode != "" && len(acceptance) == 0 {
				item["verification_mode"] = mode
			}
			switch strings.ToLower(strings.TrimSpace(stringFromAny(previous["status"]))) {
			case operationPlanStepStatusCompleted, "skipped":
				item["status"] = previous["status"]
				for _, key := range []string{"completed_at", "completion_source", "effect_refs"} {
					if value, exists := previous[key]; exists {
						item[key] = value
					}
				}
			}
		}
		if !required && strings.EqualFold(strings.TrimSpace(stringFromAny(raw["status"])), "skipped") {
			item["status"] = "skipped"
			item["completion_source"] = "model_outcome_revision"
		}
		out = append(out, item)
	}
	return out
}

func operationPlanOutcomeContractEqual(left, right map[string]interface{}) bool {
	for _, key := range []string{"goal", "target_resource_type", "target_resource_id", "required"} {
		if !reflect.DeepEqual(left[key], right[key]) {
			return false
		}
	}
	for _, key := range []string{"depends_on", "capabilities", "constraints"} {
		if !sameStringSet(stringSliceFromAny(left[key]), stringSliceFromAny(right[key])) {
			return false
		}
	}
	return true
}

func operationPlanPhasesFromOutcomeSnapshot(current []map[string]interface{}, outcomes []map[string]interface{}) []map[string]interface{} {
	byOutcomeID := map[string]map[string]interface{}{}
	for _, phase := range current {
		if id := strings.TrimSpace(stringFromAny(phase["outcome_id"])); id != "" {
			byOutcomeID[id] = phase
		}
	}
	phases := make([]map[string]interface{}, 0, len(outcomes))
	for index, outcome := range outcomes {
		outcomeID := strings.TrimSpace(stringFromAny(outcome["id"]))
		phaseID := fmt.Sprintf("phase-%d", index+1)
		if previous := byOutcomeID[outcomeID]; len(previous) > 0 {
			if value := strings.TrimSpace(stringFromAny(previous["id"])); value != "" {
				phaseID = value
			}
		}
		phase := map[string]interface{}{
			"id":                phaseID,
			"outcome_id":        outcomeID,
			"step":              stringFromAny(outcome["goal"]),
			"title":             stringFromAny(outcome["goal"]),
			"status":            stringFromAny(outcome["status"]),
			"completion_source": "runtime_effect_reconciliation",
		}
		if refs := stringSliceFromAny(outcome["effect_refs"]); len(refs) > 0 {
			phase["effect_refs"] = refs
		}
		phases = append(phases, phase)
	}
	operationPlanAdvanceNextPendingPhase(phases)
	return phases
}
