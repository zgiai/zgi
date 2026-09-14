package skillloop

import "testing"

func TestCompletedToolPlanRequiresSuccessfulRuntimeEvidence(t *testing.T) {
	for _, status := range []string{"completed", "pending", "failed", "cancelled", "stopped", "missing"} {
		t.Run(status, func(t *testing.T) {
			evidence := map[string]interface{}{"evidence_ledger": []interface{}{
				map[string]interface{}{"kind": "tool_call", "skill_id": "private-package", "tool_name": "run_script", "status": status, "invocation_id": "run-1"},
			}}
			phase := map[string]interface{}{"id": "compute", "step": "Run calculation", "status": "completed", "completion_mode": "tool"}
			if status != "missing" {
				phase["evidence_refs"] = []interface{}{"invocation_id:run-1"}
			}
			step := (&Runner{}).handleUpdatePlanCall("plan-1", map[string]interface{}{"plan": []interface{}{phase}}, evidence, 2)
			if status == "completed" {
				if step.recoverable || step.trace.Status != "success" {
					t.Fatalf("successful execution evidence should be accepted: %#v", step.trace)
				}
			} else if !step.recoverable {
				t.Fatalf("%s evidence must require a corrected plan instead of accepting a claimed completion", status)
			}
		})
	}
}
