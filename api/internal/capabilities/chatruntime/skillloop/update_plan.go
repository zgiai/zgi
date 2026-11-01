package skillloop

import (
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const maxPlanPhases = 16

// handleUpdatePlanCall records the model's checklist (ADR 0049). The model
// owns it: the runtime checks only its shape, not whether a step is done.
func (r *Runner) handleUpdatePlanCall(callID string, args map[string]interface{}, evidence map[string]interface{}, round int) skillStepResult {
	steps, err := normalizePlanChecklist(args["plan"])
	if err != nil {
		trace := failedSkillTrace("plan_update", skills.MetaToolUpdatePlan, err)
		return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableErrorPayload(err, "send the whole checklist again: one to sixteen steps, each with a step and a status, at most one in_progress")), false, false)
	}
	result := map[string]interface{}{"plan": steps}
	if explanation := trimRunes(stringFromInterface(args["explanation"]), 500); explanation != "" {
		result["explanation"] = explanation
	}
	trace := skills.SkillTrace{
		Kind:     "plan_update",
		ToolName: skills.MetaToolUpdatePlan,
		Status:   "success",
		Arguments: map[string]interface{}{
			"step_count": len(steps),
			"round":      round,
			"call_id":    strings.TrimSpace(callID),
		},
		Result: result,
	}
	return successfulSkillStep(trace, skills.ToolResultMessage(callID, map[string]interface{}{"status": "recorded", "plan": steps}), false, false)
}

// normalizePlanChecklist checks a checklist's shape and numbers its steps.
func normalizePlanChecklist(value interface{}) ([]map[string]interface{}, error) {
	raw := evidenceMapsFromAny(value)
	if len(raw) == 0 || len(raw) > 16 {
		return nil, fmt.Errorf("%w: update_plan needs one to sixteen steps", ErrInvalidInput)
	}
	steps := make([]map[string]interface{}, 0, len(raw))
	inProgress := 0
	for index, item := range raw {
		text := trimRunes(strings.TrimSpace(stringFromInterface(item["step"])), 200)
		status := strings.ToLower(strings.TrimSpace(stringFromInterface(item["status"])))
		if text == "" {
			return nil, fmt.Errorf("%w: step %d has no text", ErrInvalidInput, index+1)
		}
		switch status {
		case "pending", "completed", "skipped":
		case "in_progress":
			inProgress++
		default:
			return nil, fmt.Errorf("%w: step %d has status %q; use pending, in_progress, completed or skipped", ErrInvalidInput, index+1, status)
		}
		steps = append(steps, map[string]interface{}{"id": fmt.Sprintf("step-%d", index+1), "step": text, "status": status})
	}
	if inProgress > 1 {
		return nil, fmt.Errorf("%w: only one step may be in_progress", ErrInvalidInput)
	}
	return steps, nil
}
