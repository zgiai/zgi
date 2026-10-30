package service

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

// Reproduces a mixed outcomes/plan update: the accepted plan's IDs and
// execution bindings must survive, rather than becoming unverified placeholders.
func jointSnapshotMetadata() map[string]interface{} {
	m := map[string]interface{}{}
	applyPlanUpdateTraceMetadata(m, skills.SkillTrace{Kind: "plan_update", Status: "success", Result: map[string]interface{}{
		"outcomes": []interface{}{
			map[string]interface{}{"id": "prepare", "goal": "Prepare input"},
			map[string]interface{}{"id": "compute", "goal": "Compute result"},
			map[string]interface{}{"id": "report", "goal": "Explain result", "depends_on": []string{"compute"}},
		},
		"plan": []interface{}{
			map[string]interface{}{"id": "prepare-step", "step": "Prepare", "status": "completed", "completion_mode": "tool", "expected_action": map[string]interface{}{"skill_id": "pipeline", "tool_name": "prepare"}},
			map[string]interface{}{"id": "compute-step", "step": "Compute", "status": "completed", "completion_mode": "tool", "expected_action": map[string]interface{}{"skill_id": "calculator", "tool_name": "run"}},
			map[string]interface{}{"id": "report-step", "step": "Explain", "status": "completed", "completion_mode": "final_answer"},
		},
	}})
	return m
}

func TestJointSnapshotOutcomeOnlyRevisionPreservesExecutionContract(t *testing.T) {
	m := jointSnapshotMetadata()
	plan := mapFromOperationContext(m["operation_plan"])
	applyPlanUpdateTraceMetadata(m, skills.SkillTrace{Kind: "plan_update", Status: "success", Result: map[string]interface{}{"outcomes": plan["outcomes"]}})
	phases := mapSliceFromAny(mapFromOperationContext(m["operation_plan"])["phases"])
	if phases[0]["id"] != "prepare-step" || len(mapFromOperationContext(phases[0]["expected_action"])) == 0 || phases[2]["completion_mode"] != "final_answer" {
		t.Fatalf("outcome-only update discarded phase contract: %#v", phases)
	}
}
