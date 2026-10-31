package service

import "testing"

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
