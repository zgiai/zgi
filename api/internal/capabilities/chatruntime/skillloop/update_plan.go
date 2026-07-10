package skillloop

import (
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const maxPlanPhases = 16

func (r *Runner) handleUpdatePlanCall(callID string, args map[string]interface{}, evidence map[string]interface{}) skillStepResult {
	phases, err := normalizePlanSnapshot(args["plan"])
	if err == nil {
		err = validatePlanSnapshotEvidenceRefs(phases, evidence)
	}
	if err != nil {
		trace := failedSkillTrace("plan_update", skills.MetaToolUpdatePlan, err)
		return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableErrorPayload(err, updatePlanRecoveryHint(evidence))), false, false)
	}
	explanation := trimRunes(stringFromInterface(args["explanation"]), 500)
	result := map[string]interface{}{"plan": phases}
	if explanation != "" {
		result["explanation"] = explanation
	}
	trace := skills.SkillTrace{
		Kind:     "plan_update",
		ToolName: skills.MetaToolUpdatePlan,
		Status:   "success",
		Arguments: map[string]interface{}{
			"phase_count": len(phases),
		},
		Result: result,
	}
	return successfulSkillStep(trace, skills.ToolResultMessage(callID, map[string]interface{}{
		"status": "recorded",
		"plan":   phases,
	}), false, false)
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
		if status == "completed" && len(refs) == 0 {
			return nil, fmt.Errorf("%w: completed phase %s requires evidence_refs", ErrInvalidInput, id)
		}
		if status == "skipped" && note == "" {
			return nil, fmt.Errorf("%w: skipped phase %s requires note", ErrInvalidInput, id)
		}
		phase := map[string]interface{}{"id": id, "step": step, "title": step, "status": status}
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

func validatePlanSnapshotEvidenceRefs(phases []map[string]interface{}, evidence map[string]interface{}) error {
	for _, phase := range phases {
		if !strings.EqualFold(strings.TrimSpace(stringFromInterface(phase["status"])), "completed") {
			continue
		}
		phaseID := strings.TrimSpace(stringFromInterface(phase["id"]))
		for _, ref := range evidenceStringSliceFromAny(phase["evidence_refs"]) {
			if completionGateEvidenceRefSucceeded(evidence, ref) {
				continue
			}
			return fmt.Errorf("%w: completed phase %s references unavailable evidence %s", ErrInvalidInput, phaseID, ref)
		}
	}
	return nil
}

func updatePlanRecoveryHint(evidence map[string]interface{}) string {
	hint := "submit a complete plan snapshot with stable IDs, at most one in_progress phase, successful evidence refs for completed phases, and notes for skipped phases"
	refs := successfulPlanEvidenceRefs(evidence, 12)
	if len(refs) > 0 {
		hint += "; available successful refs: " + strings.Join(refs, ", ")
	}
	return hint
}

func successfulPlanEvidenceRefs(evidence map[string]interface{}, limit int) []string {
	records := evidenceMapsFromAny(evidence["evidence_ledger"])
	if len(records) == 0 {
		records = evidenceMapsFromAny(evidenceMapFromAny(evidence["operation_plan"])["evidence_ledger"])
	}
	refs := make([]string, 0, limit)
	seen := map[string]struct{}{}
	appendRef := func(ref string) {
		ref = canonicalPlanEvidenceRef(ref)
		if ref == "" || len(refs) >= limit {
			return
		}
		if _, exists := seen[ref]; exists {
			return
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}
	for _, record := range records {
		if !planEvidenceRecordSucceeded(record) {
			continue
		}
		appendRef(stringFromInterface(record["invocation_id"]))
		skillID := strings.TrimSpace(stringFromInterface(record["skill_id"]))
		toolName := strings.TrimSpace(stringFromInterface(record["tool_name"]))
		if skillID != "" && toolName != "" {
			appendRef("tool:" + skillID + "/" + toolName)
		}
	}
	return refs
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
