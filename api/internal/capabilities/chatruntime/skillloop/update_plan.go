package skillloop

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const maxPlanPhases = 16

func (r *Runner) handleUpdatePlanCall(callID string, args map[string]interface{}, evidence map[string]interface{}, round int) skillStepResult {
	var phases []map[string]interface{}
	resolvedTargetPhaseIDs := map[string]struct{}{}
	var phaseUpdates []map[string]interface{}
	var outcomes []map[string]interface{}
	var err error
	if args["plan"] != nil {
		phases, err = normalizePlanSnapshot(args["plan"])
	}
	if err == nil && args["outcomes"] != nil {
		outcomes, err = normalizeOutcomeSnapshot(args["outcomes"])
	}
	if err == nil && args["phase_updates"] != nil {
		phaseUpdates, err = normalizePlanPhaseUpdates(args["phase_updates"])
	}
	if err == nil && len(phaseUpdates) > 0 {
		if len(phases) > 0 || len(outcomes) > 0 {
			err = fmt.Errorf("%w: phase_updates cannot be combined with plan or outcomes", ErrInvalidInput)
		} else {
			phases, err = mergePlanPhaseUpdates(evidence, phaseUpdates)
		}
	}
	if err == nil && len(phases) == 0 && len(outcomes) == 0 {
		err = fmt.Errorf("%w: update_plan requires outcomes, plan, or phase_updates", ErrInvalidInput)
	}
	if err == nil && len(phases) > 0 {
		resolvedTargetPhaseIDs, err = validateProjectedExternalActionObservedTargetBindings(phases, evidence)
	}
	if err == nil && len(phases) > 0 {
		err = validateProjectedExternalActionPlanSnapshot(phases, evidence)
	}
	if err == nil && len(outcomes) > 0 && projectedExternalActionPlanRequiresPhaseLedger(evidence) {
		err = fmt.Errorf("%w: a projected external Action request requires the plan form with complete expected_action outcome bindings", ErrInvalidInput)
	}
	if err != nil {
		trace := failedSkillTrace("plan_update", skills.MetaToolUpdatePlan, err)
		return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableErrorPayload(err, "submit independently verifiable outcomes with stable IDs, or a compatibility plan snapshot with valid statuses")), false, false)
	}
	if len(phases) > 0 {
		baselineByID := map[string]map[string]interface{}{}
		for _, baseline := range evidenceMapsFromAny(evidenceMapFromAny(evidence["operation_plan"])["phases"]) {
			if id := strings.TrimSpace(evidenceStringFromAny(baseline["id"])); id != "" {
				baselineByID[id] = baseline
			}
		}
		for _, phase := range phases {
			expected := evidenceMapFromAny(phase["expected_action"])
			if _, external := projectedExternalActionKeyFromExpected(expected); !external ||
				projectedExpectedActionServerBindingIssue(expected, evidence) != "" {
				continue
			}
			phaseID := strings.TrimSpace(evidenceStringFromAny(phase["id"]))
			ledgerEpoch := strings.TrimSpace(evidenceStringFromAny(baselineByID[phaseID][operationPlanServerProjectedLedgerEpochKey]))
			if _, targetResolved := resolvedTargetPhaseIDs[phaseID]; targetResolved || ledgerEpoch == "" {
				ledgerEpoch = uuid.NewString()
			}
			phase[operationPlanServerProjectedLedgerEpochKey] = ledgerEpoch
		}
	}
	explanation := trimRunes(stringFromInterface(args["explanation"]), 500)
	result := map[string]interface{}{}
	if len(phases) > 0 {
		result["plan"] = phases
	}
	if len(outcomes) > 0 {
		result["outcomes"] = outcomes
	}
	if warnings := planEvidenceAuditWarnings(phases, evidence); len(warnings) > 0 {
		result["evidence_warnings"] = warnings
	}
	if explanation != "" {
		result["explanation"] = explanation
	}
	trace := skills.SkillTrace{
		Kind:     "plan_update",
		ToolName: skills.MetaToolUpdatePlan,
		Status:   "success",
		Arguments: map[string]interface{}{
			"phase_count":   len(phases),
			"outcome_count": len(outcomes),
			"round":         round,
			"call_id":       strings.TrimSpace(callID),
		},
		Result: result,
	}
	payload := map[string]interface{}{"status": "recorded"}
	if len(phases) > 0 {
		payload["plan"] = phases
	}
	if len(outcomes) > 0 {
		payload["outcomes"] = outcomes
	}
	return successfulSkillStep(trace, skills.ToolResultMessage(callID, payload), false, false)
}

func normalizePlanPhaseUpdates(value interface{}) ([]map[string]interface{}, error) {
	raw := mapSliceFromAny(value)
	if len(raw) == 0 || len(raw) > maxPlanPhases {
		return nil, fmt.Errorf("%w: phase_updates requires 1-%d patches", ErrInvalidInput, maxPlanPhases)
	}
	seen := map[string]struct{}{}
	out := make([]map[string]interface{}, 0, len(raw))
	for _, item := range raw {
		id := strings.TrimSpace(stringFromInterface(item["id"]))
		if id == "" {
			return nil, fmt.Errorf("%w: every phase update requires id", ErrInvalidInput)
		}
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("%w: duplicate phase update id %q", ErrInvalidInput, id)
		}
		seen[id] = struct{}{}
		patch := map[string]interface{}{"id": id}
		if _, exists := item["step"]; exists {
			step := trimRunes(firstNonEmptyString(item["step"], item["title"]), 240)
			if step == "" {
				return nil, fmt.Errorf("%w: phase update %q has empty step", ErrInvalidInput, id)
			}
			patch["step"] = step
			patch["title"] = step
		}
		if _, exists := item["status"]; exists {
			status := strings.ToLower(strings.TrimSpace(stringFromInterface(item["status"])))
			switch status {
			case "pending", "in_progress", "completed", "skipped":
				patch["status"] = status
			default:
				return nil, fmt.Errorf("%w: invalid phase update status %q", ErrInvalidInput, status)
			}
		}
		if _, exists := item["completion_mode"]; exists {
			mode := strings.ToLower(strings.TrimSpace(stringFromInterface(item["completion_mode"])))
			switch mode {
			case "tool", "final_answer", "non_tool":
				patch["completion_mode"] = mode
			default:
				return nil, fmt.Errorf("%w: invalid phase update completion_mode %q", ErrInvalidInput, mode)
			}
		}
		if _, exists := item["expected_action"]; exists {
			expected := normalizePlanExpectedAction(item["expected_action"])
			if len(expected) == 0 {
				return nil, fmt.Errorf("%w: phase update %q has invalid expected_action", ErrInvalidInput, id)
			}
			patch["expected_action"] = expected
		}
		if _, exists := item["evidence_refs"]; exists {
			patch["evidence_refs"] = compactPlanEvidenceRefs(evidenceStringSliceFromAny(item["evidence_refs"]), 12, 240)
		}
		if _, exists := item["note"]; exists {
			patch["note"] = trimRunes(stringFromInterface(item["note"]), 500)
		}
		out = append(out, patch)
	}
	return out, nil
}

func mergePlanPhaseUpdates(evidence map[string]interface{}, updates []map[string]interface{}) ([]map[string]interface{}, error) {
	baseline := evidenceMapsFromAny(evidenceMapFromAny(evidence["operation_plan"])["phases"])
	if len(baseline) == 0 {
		return nil, fmt.Errorf("%w: phase_updates requires an existing operation plan", ErrInvalidInput)
	}
	byID := make(map[string]map[string]interface{}, len(updates))
	for _, update := range updates {
		byID[strings.TrimSpace(evidenceStringFromAny(update["id"]))] = update
	}
	out := make([]map[string]interface{}, 0, len(baseline))
	for _, source := range baseline {
		phase := copyStringAnyMap(source)
		id := strings.TrimSpace(evidenceStringFromAny(phase["id"]))
		if update := byID[id]; len(update) > 0 {
			for key, value := range update {
				if key != "id" {
					phase[key] = value
				}
			}
			delete(byID, id)
		}
		out = append(out, phase)
	}
	if len(byID) > 0 {
		for id := range byID {
			return nil, fmt.Errorf("%w: phase update references unknown id %q", ErrInvalidInput, id)
		}
	}
	return out, nil
}

func normalizeOutcomeSnapshot(value interface{}) ([]map[string]interface{}, error) {
	raw := mapSliceFromAny(value)
	if len(raw) == 0 {
		return nil, fmt.Errorf("%w: update_plan outcomes must not be empty", ErrInvalidInput)
	}
	if len(raw) > maxPlanPhases {
		return nil, fmt.Errorf("%w: update_plan supports at most %d outcomes", ErrInvalidInput, maxPlanPhases)
	}
	used := map[string]struct{}{}
	out := make([]map[string]interface{}, 0, len(raw))
	for index, item := range raw {
		goal := trimRunes(firstNonEmptyString(item["goal"], item["title"], item["step"]), 240)
		if goal == "" {
			return nil, fmt.Errorf("%w: every outcome requires goal", ErrInvalidInput)
		}
		id := strings.ToLower(strings.TrimSpace(stringFromInterface(item["id"])))
		if id == "" {
			id = fmt.Sprintf("outcome-amendment-%d", index+1)
		}
		id = trimRunes(id, 80)
		if _, exists := used[id]; exists {
			return nil, fmt.Errorf("%w: duplicate outcome id %q", ErrInvalidInput, id)
		}
		used[id] = struct{}{}
		status := strings.ToLower(strings.TrimSpace(stringFromInterface(item["status"])))
		if status == "" {
			status = "pending"
		}
		switch status {
		case "pending", "in_progress", "completed", "skipped":
		default:
			return nil, fmt.Errorf("%w: invalid outcome status %q", ErrInvalidInput, status)
		}
		outcome := map[string]interface{}{
			"id":     id,
			"goal":   goal,
			"status": status,
		}
		for _, key := range []string{"target_resource_type", "target_resource_id"} {
			if text := trimRunes(stringFromInterface(item[key]), 160); text != "" {
				outcome[key] = text
			}
		}
		for _, key := range []string{"depends_on", "capabilities", "constraints", "evidence_refs"} {
			if values := compactStringSlice(evidenceStringSliceFromAny(item[key]), 12, 180); len(values) > 0 {
				outcome[key] = values
			}
		}
		if required, ok := item["required"].(bool); ok {
			outcome["required"] = required
		}
		out = append(out, outcome)
	}
	for _, outcome := range out {
		for _, dependency := range evidenceStringSliceFromAny(outcome["depends_on"]) {
			if _, exists := used[strings.ToLower(strings.TrimSpace(dependency))]; !exists {
				return nil, fmt.Errorf("%w: unknown outcome dependency %q", ErrInvalidInput, dependency)
			}
		}
	}
	return out, nil
}

func normalizePlanSnapshot(value interface{}) ([]map[string]interface{}, error) {
	raw := mapSliceFromAny(value)
	if len(raw) == 0 {
		return nil, fmt.Errorf("%w: update_plan plan is required", ErrInvalidInput)
	}
	if len(raw) > maxPlanPhases {
		return nil, fmt.Errorf("%w: update_plan supports at most %d phases", ErrInvalidInput, maxPlanPhases)
	}
	usedIDs := map[string]struct{}{}
	for _, item := range raw {
		if id := strings.TrimSpace(stringFromInterface(item["id"])); id != "" {
			if _, exists := usedIDs[id]; exists {
				return nil, fmt.Errorf("%w: duplicate plan phase id %q", ErrInvalidInput, id)
			}
			usedIDs[id] = struct{}{}
		}
	}
	nextAmendment := 1
	inProgress := 0
	out := make([]map[string]interface{}, 0, len(raw))
	for _, item := range raw {
		step := trimRunes(firstNonEmptyString(item["step"], item["title"]), 240)
		if step == "" {
			return nil, fmt.Errorf("%w: every plan phase requires step", ErrInvalidInput)
		}
		status := strings.ToLower(strings.TrimSpace(stringFromInterface(item["status"])))
		switch status {
		case "pending", "completed", "skipped":
		case "in_progress":
			inProgress++
		default:
			return nil, fmt.Errorf("%w: invalid plan phase status %q", ErrInvalidInput, status)
		}
		id := strings.TrimSpace(stringFromInterface(item["id"]))
		if id == "" {
			for {
				id = fmt.Sprintf("phase-amendment-%d", nextAmendment)
				nextAmendment++
				if _, exists := usedIDs[id]; !exists {
					break
				}
			}
			usedIDs[id] = struct{}{}
		}
		note := trimRunes(stringFromInterface(item["note"]), 500)
		refs := compactPlanEvidenceRefs(evidenceStringSliceFromAny(item["evidence_refs"]), 12, 240)
		phase := map[string]interface{}{"id": id, "step": step, "title": step, "status": status}
		if completionMode := strings.ToLower(strings.TrimSpace(stringFromInterface(item["completion_mode"]))); completionMode != "" {
			switch completionMode {
			case "tool", "final_answer", "non_tool":
				phase["completion_mode"] = completionMode
			default:
				return nil, fmt.Errorf("%w: invalid plan phase completion_mode %q", ErrInvalidInput, completionMode)
			}
		}
		if expectedAction := normalizePlanExpectedAction(item["expected_action"]); len(expectedAction) > 0 {
			phase["expected_action"] = expectedAction
		}
		if len(refs) > 0 {
			phase["evidence_refs"] = refs
		}
		if note != "" {
			phase["note"] = note
		}
		out = append(out, phase)
	}
	if inProgress > 1 {
		return nil, fmt.Errorf("%w: at most one plan phase may be in_progress", ErrInvalidInput)
	}
	return out, nil
}

func normalizePlanExpectedAction(value interface{}) map[string]interface{} {
	raw := evidenceMapFromAny(value)
	if len(raw) == 0 {
		return nil
	}
	skillID := strings.ToLower(strings.TrimSpace(stringFromInterface(raw["skill_id"])))
	toolName := strings.TrimSpace(stringFromInterface(raw["tool_name"]))
	if skillID == "" || toolName == "" {
		return nil
	}
	action := map[string]interface{}{
		"skill_id":  trimRunes(skillID, 120),
		"tool_name": trimRunes(toolName, 160),
	}
	if projectedToolName := trimRunes(stringFromInterface(raw["projected_tool_name"]), 160); projectedToolName != "" {
		action["projected_tool_name"] = projectedToolName
	}
	if serverProjection := trimRunes(stringFromInterface(raw[planExpectedActionServerProjectionKey]), 160); serverProjection != "" {
		action[planExpectedActionServerProjectionKey] = serverProjection
	}
	if fingerprint := trimRunes(stringFromInterface(raw[planExpectedActionServerBindingFingerprintKey]), 160); fingerprint != "" {
		action[planExpectedActionServerBindingFingerprintKey] = fingerprint
	}
	targetRaw := evidenceMapFromAny(raw["target"])
	target := map[string]interface{}{}
	for _, key := range []string{"integration_id", "action_id", "agent_id", "file_id", "asset_id", "resource_id", "dataset_id", "data_source_id", "table_id", "workflow_id", "binding_id", "href", "route"} {
		if value := trimRunes(stringFromInterface(targetRaw[key]), 240); value != "" {
			target[key] = value
		}
	}
	if len(target) > 0 {
		action["target"] = target
	}
	targetArguments := map[string]interface{}{}
	for path, rawValue := range evidenceMapFromAny(raw["target_arguments"]) {
		path = trimRunes(strings.TrimSpace(path), 160)
		value := trimRunes(stringFromInterface(rawValue), 240)
		if path != "" && value != "" && len(targetArguments) < 16 {
			targetArguments[path] = value
		}
	}
	if len(targetArguments) > 0 {
		action["target_arguments"] = targetArguments
	}
	return action
}

func compactPlanEvidenceRefs(values []string, limit int, maxRunes int) []string {
	canonical := make([]string, 0, len(values))
	for _, value := range values {
		canonical = append(canonical, canonicalPlanEvidenceRef(value))
	}
	return compactStringSlice(canonical, limit, maxRunes)
}

func canonicalPlanEvidenceRef(value string) string {
	ref := strings.TrimSpace(value)
	if ref == "" {
		return ""
	}
	lower := strings.ToLower(ref)
	for _, prefix := range []string{"tool:", "turn_state:", "page_context:", "runtime_id:", "invocation_id:", "action_id:", "call_id:"} {
		if strings.HasPrefix(lower, prefix) {
			return prefix + strings.TrimSpace(ref[len(prefix):])
		}
	}
	if !strings.HasPrefix(ref, "/") && strings.Count(ref, "/") == 1 && !strings.ContainsAny(ref, " \t\r\n") {
		skillID, toolName, ok := strings.Cut(ref, "/")
		if ok && strings.TrimSpace(skillID) != "" && strings.TrimSpace(toolName) != "" {
			return "tool:" + strings.TrimSpace(skillID) + "/" + strings.TrimSpace(toolName)
		}
	}
	return ref
}

func planEvidenceAuditWarnings(phases []map[string]interface{}, evidence map[string]interface{}) []string {
	warnings := []string{}
	for _, phase := range phases {
		if !strings.EqualFold(strings.TrimSpace(stringFromInterface(phase["status"])), "completed") {
			continue
		}
		phaseID := strings.TrimSpace(stringFromInterface(phase["id"]))
		refs := evidenceStringSliceFromAny(phase["evidence_refs"])
		if len(refs) == 0 {
			warnings = append(warnings, "completed_phase_without_evidence:"+phaseID)
			continue
		}
		for _, ref := range refs {
			if planEvidenceRefSucceeded(evidence, ref) {
				continue
			}
			warnings = append(warnings, "unresolved_evidence_ref:"+canonicalPlanEvidenceRef(ref))
		}
	}
	return compactStringSlice(warnings, 16, 280)
}

func planEvidenceRefSucceeded(evidence map[string]interface{}, ref string) bool {
	ref = canonicalPlanEvidenceRef(ref)
	if ref == "" {
		return false
	}
	if strings.HasPrefix(ref, "turn_state:") {
		key := strings.TrimSpace(strings.TrimPrefix(ref, "turn_state:"))
		for _, item := range evidenceMapsFromAny(evidenceMapFromAny(evidence["turn_state"])["items"]) {
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
	records := evidenceMapsFromAny(evidence["evidence_ledger"])
	if len(records) == 0 {
		records = evidenceMapsFromAny(evidenceMapFromAny(evidence["operation_plan"])["evidence_ledger"])
	}
	if strings.HasPrefix(ref, "tool:") {
		pair := strings.TrimSpace(strings.TrimPrefix(ref, "tool:"))
		skillID, toolName, ok := strings.Cut(pair, "/")
		if ok {
			for _, record := range records {
				if strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(record["skill_id"])), strings.TrimSpace(skillID)) &&
					strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(record["tool_name"])), strings.TrimSpace(toolName)) &&
					planEvidenceRecordSucceeded(record) {
					return true
				}
			}
		}
	}
	for _, record := range records {
		if !planEvidenceRecordSucceeded(record) {
			continue
		}
		for _, field := range []string{"invocation_id", "runtime_id", "action_id", "call_id"} {
			value := strings.TrimSpace(evidenceStringFromAny(record[field]))
			if value != "" && (ref == value || ref == field+":"+value) {
				return true
			}
		}
	}
	return false
}

func planEvidenceRecordSucceeded(record map[string]interface{}) bool {
	switch strings.ToLower(strings.TrimSpace(stringFromInterface(record["status"]))) {
	case "success", "succeeded", "completed", "complete", "pass", "verified", "recorded":
		return true
	default:
		return false
	}
}

func compactStringSlice(values []string, limit int, maxRunes int) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = trimRunes(strings.TrimSpace(value), maxRunes)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
		if len(out) >= limit {
			break
		}
	}
	return out
}
