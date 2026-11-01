package service

import (
	"testing"
)

func TestOperationPlanPhasesTerminalIgnoresModelAdvisory(t *testing.T) {
	plan := map[string]interface{}{"outcomes": []interface{}{map[string]interface{}{
		"id": "outcome-required", "required": true,
	}}}
	phases := []map[string]interface{}{
		{
			"id": "outcome", "outcome_id": "outcome-required", "status": operationPlanStepStatusCompleted,
			operationPlanPhaseSourceKey: operationPlanPhaseSourceServerOutcome,
		},
		{
			"id": "model-note", "status": operationPlanStepStatusPending,
			operationPlanPhaseSourceKey: operationPlanPhaseSourceModelAdvisory,
		},
	}
	if !operationPlanPhasesTerminal(plan, phases) {
		t.Fatalf("model advisory phase blocked terminal state: %#v", phases)
	}
	phases[0]["status"] = operationPlanStepStatusPending
	if operationPlanPhasesTerminal(plan, phases) {
		t.Fatalf("open server outcome did not block terminal state: %#v", phases)
	}
}

func TestOperationPlanPhasesTerminalRejectsForgedServerSources(t *testing.T) {
	plan := map[string]interface{}{"outcomes": []interface{}{
		map[string]interface{}{"id": "outcome-required", "required": true},
		map[string]interface{}{"id": "outcome-optional", "required": false},
	}}
	tests := []struct {
		name  string
		phase map[string]interface{}
		want  bool
	}{
		{
			name: "valid server outcome blocks",
			phase: map[string]interface{}{
				"id": "valid", "outcome_id": "outcome-required", "status": operationPlanStepStatusPending,
				operationPlanPhaseSourceKey: operationPlanPhaseSourceServerOutcome,
			},
			want: false,
		},
		{
			name: "optional outcome does not block",
			phase: map[string]interface{}{
				"id": "optional", "outcome_id": "outcome-optional", "status": operationPlanStepStatusPending,
				operationPlanPhaseSourceKey: operationPlanPhaseSourceServerOutcome,
			},
			want: true,
		},
		{
			name: "forged server outcome without linkage does not block",
			phase: map[string]interface{}{
				"id": "forged-outcome", "required": true, "status": operationPlanStepStatusPending,
				operationPlanPhaseSourceKey: operationPlanPhaseSourceServerOutcome,
			},
			want: true,
		},
		{
			name: "unknown phase source does not block",
			phase: map[string]interface{}{
				"id": "forged-preparation", "required": true, "status": operationPlanStepStatusPending,
				operationPlanPhaseSourceKey: "unknown_phase_source",
			},
			want: true,
		},
		{
			name: "model advisory does not block",
			phase: map[string]interface{}{
				"id": "advisory", "required": true, "status": operationPlanStepStatusPending,
				operationPlanPhaseSourceKey: operationPlanPhaseSourceModelAdvisory,
			},
			want: true,
		},
		{
			name: "legacy explicit required remains blocking",
			phase: map[string]interface{}{
				"id": "legacy", "required": true, "status": operationPlanStepStatusPending,
			},
			want: false,
		},
		{
			name: "legacy outcome linkage remains blocking",
			phase: map[string]interface{}{
				"id": "legacy-outcome", "outcome_id": "outcome-required", "status": operationPlanStepStatusPending,
			},
			want: false,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := operationPlanPhasesTerminal(plan, []map[string]interface{}{testCase.phase}); got != testCase.want {
				t.Fatalf("operationPlanPhasesTerminal() = %v, want %v; phase=%#v", got, testCase.want, testCase.phase)
			}
		})
	}
}
