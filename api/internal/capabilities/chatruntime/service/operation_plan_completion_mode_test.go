package service

import "testing"

func TestFinalAnswerCompletesExplicitReportPhaseAfterRuntimeExecution(t *testing.T) {
	for _, mode := range []string{"final_answer", "non_tool"} {
		t.Run(mode, func(t *testing.T) {
			metadata := map[string]interface{}{"operation_plan": map[string]interface{}{
				"status": "running",
				"phases": []interface{}{
					map[string]interface{}{"id": "compute", "status": "in_progress", "completion_mode": "tool", "expected_action": map[string]interface{}{"skill_id": "private-package", "tool_name": "run_script"}},
					map[string]interface{}{"id": "report", "status": "pending", "completion_mode": mode},
				},
			}}
			applyOperationPlanInvocationState(metadata, []map[string]interface{}{{
				"kind": "tool_call", "status": "success", "runtime_id": "compute-1",
				"skill_id": "private-package", "tool_name": "run_script",
				"result": map[string]interface{}{"exit_code": 0},
			}})
			finalizeOperationPlanForCompletedResult(metadata)
			plan := mapFromOperationContext(metadata["operation_plan"])
			if plan["status"] != "completed" || mapFromOperationContext(metadata["completion_verification"])["status"] != "pass" {
				t.Fatalf("completed execution and report should pass: %#v", plan)
			}
			phases := mapSliceFromAny(plan["phases"])
			if len(stringSliceFromAny(phases[0]["evidence_refs"])) == 0 || len(stringSliceFromAny(phases[1]["evidence_refs"])) == 0 {
				t.Fatalf("both phases must retain their distinct evidence: %#v", phases)
			}
		})
	}
}

func TestFinalAnswerCannotReplaceUnfinishedRuntimePhase(t *testing.T) {
	for _, status := range []string{"pending", "failed", "cancelled", "stopped"} {
		t.Run(status, func(t *testing.T) {
			metadata := map[string]interface{}{"operation_plan": map[string]interface{}{
				"status": "running", "phases": []interface{}{
					map[string]interface{}{"id": "compute", "status": "in_progress", "completion_mode": "tool", "expected_action": map[string]interface{}{"skill_id": "private-package", "tool_name": "run_script"}},
					map[string]interface{}{"id": "report", "status": "pending", "completion_mode": "final_answer"},
				},
			}}
			applyOperationPlanInvocationState(metadata, []map[string]interface{}{{"kind": "tool_call", "status": status, "skill_id": "private-package", "tool_name": "run_script"}})
			applyOperationPlanTerminalCompletionResultWithSource(metadata, "pass", "main_model_final", "answer submitted", nil, nil, "")
			want := "incomplete"
			if status == "failed" {
				want = "failed"
			}
			if got := mapFromOperationContext(metadata["completion_verification"])["status"]; got != want {
				t.Fatalf("runtime status %s must block completion, got %v", status, got)
			}
		})
	}
}

func TestFinalAnswerClassificationCannotBypassRuntimeBinding(t *testing.T) {
	for _, binding := range []map[string]interface{}{
		{"expected_action": map[string]interface{}{"skill_id": "file-manager", "tool_name": "save_file_to_management"}},
		{"outcome_id": "required-save"},
		{"verification_mode": "runtime_effects"},
	} {
		phase := map[string]interface{}{"status": "pending", "completion_mode": "final_answer"}
		for key, value := range binding {
			phase[key] = value
		}
		if operationPlanPhaseCanCompleteWithFinalAnswer(phase) {
			t.Fatalf("final answer classification must not override binding: %#v", phase)
		}
	}
}
