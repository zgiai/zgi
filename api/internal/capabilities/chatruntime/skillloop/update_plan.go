package skillloop

import (
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const maxPlanPhases = 16

func (r *Runner) handleUpdatePlanCall(callID string, args map[string]interface{}, evidence map[string]interface{}, round int) skillStepResult {
	var phases []map[string]interface{}
	var outcomes []map[string]interface{}
	var err error
	if args["plan"] != nil {
		phases, err = normalizePlanSnapshot(args["plan"])
	}
	if err == nil && args["outcomes"] != nil {
		outcomes, err = normalizeOutcomeSnapshot(args["outcomes"])
	}
	if err == nil && len(phases) == 0 && len(outcomes) == 0 {
		err = fmt.Errorf("%w: update_plan requires outcomes or a compatibility plan snapshot", ErrInvalidInput)
	}
	if err != nil {
		trace := failedSkillTrace("plan_update", skills.MetaToolUpdatePlan, err)
		return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableErrorPayload(err, "submit independently verifiable outcomes with stable IDs, or a compatibility plan snapshot with valid statuses")), false, false)
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
	targetRaw := evidenceMapFromAny(raw["target"])
	target := map[string]interface{}{}
	for _, key := range []string{"agent_id", "file_id", "asset_id", "resource_id", "dataset_id", "data_source_id", "table_id", "workflow_id", "binding_id", "href", "route"} {
		if value := trimRunes(stringFromInterface(targetRaw[key]), 240); value != "" {
			target[key] = value
		}
	}
	if len(target) > 0 {
		action["target"] = target
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
