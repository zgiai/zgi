package service

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestOperationPlanPhasesFromTurnStrategyClassifiesAuthority(t *testing.T) {
	required := true
	outcomePhases := operationPlanPhasesFromTurnStrategy(&AIChatTurnStrategy{Outcomes: []AIChatTurnOutcome{{
		ID: "outcome-send", Goal: "Send the message", Required: &required,
	}}})
	if len(outcomePhases) != 1 ||
		stringFromAny(outcomePhases[0][operationPlanPhaseSourceKey]) != operationPlanPhaseSourceServerOutcome {
		t.Fatalf("Outcome phase source = %#v, want server_outcome", outcomePhases)
	}
	if required, ok := outcomePhases[0]["required"].(bool); !ok || !required {
		t.Fatalf("Outcome phase required classification = %#v", outcomePhases[0])
	}

	advisoryPhases := operationPlanPhasesFromTurnStrategy(&AIChatTurnStrategy{
		PhaseGoals: []string{"Inspect context", "Explain the result"},
	})
	if len(advisoryPhases) != 2 {
		t.Fatalf("advisory phases = %#v", advisoryPhases)
	}
	for _, phase := range advisoryPhases {
		if got := stringFromAny(phase[operationPlanPhaseSourceKey]); got != operationPlanPhaseSourceModelAdvisory {
			t.Fatalf("PhaseGoals phase source = %q, want model_advisory: %#v", got, phase)
		}
	}
}

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
			name: "forged server preparation does not block",
			phase: map[string]interface{}{
				"id": "forged-preparation", "required": true, "status": operationPlanStepStatusPending,
				operationPlanPhaseSourceKey: operationPlanPhaseSourceServerPreparation,
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

func TestApplyPlanUpdateTraceMetadataKeepsAtomicOutcomeActionBinding(t *testing.T) {
	metadata := map[string]interface{}{"operation_plan": map[string]interface{}{
		"phases": []interface{}{},
	}}
	trace := skills.SkillTrace{
		Kind: "plan_update", Status: "success",
		Arguments: map[string]interface{}{"round": 1},
		Result: map[string]interface{}{
			"outcomes": []interface{}{map[string]interface{}{
				"id": "outcome-search", "goal": "Search recent mail", "status": "pending", "required": true,
				"capabilities": []interface{}{"external-apps"},
			}},
			"plan": []interface{}{map[string]interface{}{
				"id": "phase-1", "outcome_id": "outcome-search", "step": "Search recent mail", "status": "in_progress",
				"required": true, operationPlanPhaseSourceKey: operationPlanPhaseSourceServerOutcome,
				operationPlanServerProjectedEpochKey:             "epoch-search",
				operationPlanServerProjectedConnectionBindingKey: "connection-binding",
				"expected_action": map[string]interface{}{
					"skill_id": skills.SkillExternalApps, "tool_name": "execute_action",
					operationPlanServerProjectedToolKey:    "search_mail_messages",
					operationPlanServerProjectedBindingKey: "binding-mail-search",
					"target":                               map[string]interface{}{"integration_id": "mail", "action_id": "mail.message.search"},
				},
			}},
		},
	}
	applyPlanUpdateTraceMetadata(metadata, trace)
	phase := mapSliceFromAny(mapFromOperationContext(metadata["operation_plan"])["phases"])[0]
	if got := stringFromAny(phase[operationPlanPhaseSourceKey]); got != operationPlanPhaseSourceServerOutcome {
		t.Fatalf("phase source = %q, want server_outcome: %#v", got, phase)
	}
	if got := stringFromAny(phase[operationPlanServerProjectedEpochKey]); got != "epoch-search" {
		t.Fatalf("ledger epoch = %q, want epoch-search: %#v", got, phase)
	}
	if got := stringFromAny(phase[operationPlanServerProjectedConnectionBindingKey]); got != "connection-binding" {
		t.Fatalf("connection binding = %q, want preserved: %#v", got, phase)
	}
	expected := mapFromOperationContext(phase["expected_action"])
	if stringFromAny(expected[operationPlanServerProjectedBindingKey]) != "binding-mail-search" ||
		stringFromAny(expected[operationPlanServerProjectedToolKey]) != "search_mail_messages" {
		t.Fatalf("expected Action binding was lost during Outcome reconciliation: %#v", phase)
	}
}

func TestReconcileOperationPlanPhaseSnapshotCannotDemoteServerSource(t *testing.T) {
	current := []map[string]interface{}{map[string]interface{}{
		"id": "phase-1", "status": "in_progress", "required": true,
		operationPlanPhaseSourceKey: operationPlanPhaseSourceServerOutcome,
	}}
	next := []map[string]interface{}{map[string]interface{}{
		"id": "phase-1", "status": "in_progress",
		operationPlanPhaseSourceKey: operationPlanPhaseSourceModelAdvisory,
	}}
	phase := reconcileOperationPlanPhaseSnapshot(current, next)[0]
	if got := stringFromAny(phase[operationPlanPhaseSourceKey]); got != operationPlanPhaseSourceServerOutcome {
		t.Fatalf("server source was demoted to %q: %#v", got, phase)
	}
	if required, ok := phase["required"].(bool); !ok || !required {
		t.Fatalf("server required classification was not restored: %#v", phase)
	}
}

func TestExternalActionPinnedPlanKeysRequireServerAttestation(t *testing.T) {
	canonical := map[string]interface{}{
		"skill_id": skills.SkillExternalApps, "tool_name": "execute_action",
		operationPlanServerProjectedToolKey:    "search_mail_messages",
		operationPlanServerProjectedBindingKey: "binding-mail-search",
		"target":                               map[string]interface{}{"integration_id": "mail", "action_id": "mail.message.search"},
	}
	unattested := map[string]interface{}{
		"skill_id": skills.SkillExternalApps, "tool_name": "execute_action",
		"target": map[string]interface{}{"integration_id": "mail", "action_id": "mail.account.get"},
	}
	keys := externalActionPinnedPlanKeys(map[string]interface{}{"operation_plan": map[string]interface{}{
		"phases": []interface{}{
			map[string]interface{}{"id": "canonical", "status": "in_progress", "expected_action": canonical},
			map[string]interface{}{"id": "model", "status": "pending", "expected_action": unattested},
		},
	}})
	if len(keys) != 1 || keys[0] != "mail:mail.message.search" {
		t.Fatalf("pinned Action keys = %#v, want only server-attested binding", keys)
	}
}
