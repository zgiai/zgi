package service

import (
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestProjectedExternalActionCompletionBridgeCompletesExactUnstructuredPhase(t *testing.T) {
	metadata := projectedExternalActionCompletionMetadata(false, "Alice", "epoch-1")
	invocation := projectedExternalActionCompletionInvocation("Alice", "epoch-1")

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{invocation})

	plan := mapFromOperationContext(metadata["operation_plan"])
	phase := mapSliceFromAny(plan["phases"])[0]
	if got := stringFromAny(phase["status"]); got != operationPlanStepStatusCompleted {
		t.Fatalf("phase status = %q, want completed", got)
	}
	effects := mapSliceFromAny(plan[operationPlanEffectLedgerKey])
	if len(effects) != 1 || stringFromAny(effects[0]["type"]) != operationPlanExternalActionEffectType {
		t.Fatalf("effects = %#v, want one canonical external completion effect", effects)
	}
	if got := stringFromAny(effects[0]["target_arguments"].(map[string]interface{})["recipient_ref"]); got != "Alice" {
		t.Fatalf("effect target = %q, want Alice", got)
	}
	attempts := mapSliceFromAny(plan[operationPlanActionAttemptsKey])
	if len(attempts) != 1 ||
		stringFromAny(attempts[0]["status"]) != operationPlanStepStatusCompleted ||
		stringFromAny(attempts[0][operationPlanServerProjectedEpochKey]) != "epoch-1" ||
		stringFromAny(attempts[0][operationPlanServerProjectedBindingKey]) != "binding-fingerprint-1" {
		t.Fatalf("attempts = %#v, want one epoch-and-fingerprint-bound attempt", attempts)
	}

	// Reprocessing persisted evidence must not append a second effect or attempt.
	applyOperationPlanInvocationState(metadata, []map[string]interface{}{invocation})
	plan = mapFromOperationContext(metadata["operation_plan"])
	if got := len(mapSliceFromAny(plan[operationPlanEffectLedgerKey])); got != 1 {
		t.Fatalf("effect count after replay = %d, want 1", got)
	}
	if got := len(mapSliceFromAny(plan[operationPlanActionAttemptsKey])); got != 1 {
		t.Fatalf("attempt count after replay = %d, want 1", got)
	}
}

func TestProjectedExternalActionAttemptStatusRequiresConfirmedProviderSuccess(t *testing.T) {
	tests := []struct {
		name            string
		operationStatus string
		removeStatus    bool
		wantStatus      string
	}{
		{name: "failed safe", operationStatus: "failed_safe", wantStatus: operationPlanStepStatusFailed},
		{name: "partially succeeded", operationStatus: "partially_succeeded", wantStatus: operationPlanStepStatusFailed},
		{name: "outcome unknown", operationStatus: "outcome_unknown", wantStatus: operationPlanStepStatusPending},
		{name: "executing", operationStatus: "executing", wantStatus: operationPlanStepStatusPending},
		{name: "missing", removeStatus: true, wantStatus: operationPlanStepStatusPending},
		{name: "unrecognized", operationStatus: "provider_says_maybe", wantStatus: operationPlanStepStatusPending},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metadata := projectedExternalActionCompletionMetadata(true, "Alice", "epoch-1")
			invocation := projectedExternalActionCompletionInvocation("Alice", "epoch-1")
			result := mapFromOperationContext(invocation["result"])
			delete(result, "provider_success_confirmed")
			if test.removeStatus {
				delete(result, "operation_status")
			} else {
				result["operation_status"] = test.operationStatus
			}

			applyOperationPlanInvocationState(metadata, []map[string]interface{}{invocation})

			plan := mapFromOperationContext(metadata["operation_plan"])
			attempts := mapSliceFromAny(plan[operationPlanActionAttemptsKey])
			if len(attempts) != 1 {
				t.Fatalf("attempts = %#v, want one attempt", attempts)
			}
			if got := stringFromAny(attempts[0]["status"]); got != test.wantStatus {
				t.Fatalf("attempt status = %q, want %q", got, test.wantStatus)
			}
			phase := mapSliceFromAny(plan["phases"])[0]
			if got := stringFromAny(phase["status"]); got == operationPlanStepStatusCompleted {
				t.Fatalf("phase completed without confirmed provider success: %#v", phase)
			}
			outcome := mapSliceFromAny(plan[operationPlanOutcomesKey])[0]
			if got := stringFromAny(outcome["status"]); got == operationPlanStepStatusCompleted {
				t.Fatalf("outcome completed without confirmed provider success: %#v", outcome)
			}
			if effects := mapSliceFromAny(plan[operationPlanEffectLedgerKey]); len(effects) != 0 {
				t.Fatalf("unconfirmed provider result produced effects: %#v", effects)
			}
		})
	}
}

func TestProjectedExternalActionCompletionBridgeCompletesRedactedRuntimeTrace(t *testing.T) {
	metadata := projectedExternalActionCompletionMetadata(false, "Alice", "epoch-1")
	invocation := projectedExternalActionCompletionInvocation("Alice", "epoch-1")
	arguments := mapFromOperationContext(invocation["arguments"])
	delete(arguments, "integration_id")
	delete(arguments, "action_id")
	delete(arguments, "connection_id")
	delete(mapFromOperationContext(invocation["result"]), "connection_id")

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{invocation})

	plan := mapFromOperationContext(metadata["operation_plan"])
	phase := mapSliceFromAny(plan["phases"])[0]
	if got := stringFromAny(phase["status"]); got != operationPlanStepStatusCompleted {
		t.Fatalf("redacted trace phase status = %q, want completed", got)
	}
	effect := mapSliceFromAny(plan[operationPlanEffectLedgerKey])[0]
	if stringFromAny(effect[operationPlanServerProjectedBindingKey]) != "binding-fingerprint-1" ||
		stringFromAny(effect["integration_id"]) != "wecom" ||
		stringFromAny(effect["action_id"]) != "wecom.message.send" {
		t.Fatalf("redacted trace effect identity = %#v", effect)
	}
}

func TestProjectedExternalActionCompletionBridgePreservesLegacyExecuteActionPhaseMatching(t *testing.T) {
	metadata := map[string]interface{}{"operation_plan": map[string]interface{}{
		"status": operationPlanStatusRunning,
		"phases": []interface{}{map[string]interface{}{
			"id": "legacy-send", "step": "send message", "status": "in_progress",
			"expected_action": map[string]interface{}{
				"skill_id": skills.SkillExternalApps, "tool_name": "execute_action",
				"target": map[string]interface{}{"integration_id": "wecom", "action_id": "wecom.message.send"},
			},
		}},
	}}
	invocation := map[string]interface{}{
		"kind": "tool_call", "status": "success", "runtime_id": "legacy-runtime-1",
		"skill_id": skills.SkillExternalApps, "tool_name": "execute_action",
		"arguments": map[string]interface{}{
			"plan_phase_id":         "legacy-send",
			"operation_plan_target": map[string]interface{}{"integration_id": "wecom", "action_id": "wecom.message.send"},
		},
		"result": map[string]interface{}{"operation_status": "completed"},
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{invocation})

	phase := mapSliceFromAny(mapFromOperationContext(metadata["operation_plan"])["phases"])[0]
	if got := stringFromAny(phase["status"]); got != operationPlanStepStatusCompleted {
		t.Fatalf("legacy execute_action phase status = %q, want completed", got)
	}
}

func TestProjectedExternalActionCompletionBridgeRejectsInexactEvidence(t *testing.T) {
	tests := []struct {
		name           string
		mutateMetadata func(map[string]interface{})
		mutate         func(map[string]interface{})
	}{
		{
			name: "wrong Action",
			mutate: func(invocation map[string]interface{}) {
				mapFromOperationContext(invocation["arguments"])["action_id"] = "wecom.calendar.create"
			},
		},
		{
			name: "result Action disagrees with invoked Action",
			mutate: func(invocation map[string]interface{}) {
				mapFromOperationContext(invocation["result"])["action_id"] = "wecom.calendar.create"
			},
		},
		{
			name: "connection binding disagrees with provider result",
			mutate: func(invocation map[string]interface{}) {
				mapFromOperationContext(invocation["result"])["connection_id"] = "connection-2"
			},
		},
		{
			name: "target IDs are case-sensitive",
			mutate: func(invocation map[string]interface{}) {
				mapFromOperationContext(mapFromOperationContext(invocation["arguments"])["operation_plan_target"])["recipient_ref"] = "alice"
			},
		},
		{
			name: "missing target",
			mutate: func(invocation map[string]interface{}) {
				delete(mapFromOperationContext(invocation["arguments"]), "operation_plan_target")
			},
		},
		{
			name: "missing phase identity is ambiguous",
			mutate: func(invocation map[string]interface{}) {
				delete(mapFromOperationContext(invocation["arguments"]), "plan_phase_id")
			},
		},
		{
			name: "missing ledger epoch",
			mutate: func(invocation map[string]interface{}) {
				delete(mapFromOperationContext(invocation["arguments"]), operationPlanServerProjectedEpochKey)
			},
		},
		{
			name: "wrong ledger epoch",
			mutate: func(invocation map[string]interface{}) {
				mapFromOperationContext(invocation["arguments"])[operationPlanServerProjectedEpochKey] = "epoch-old"
			},
		},
		{
			name: "missing server binding fingerprint",
			mutate: func(invocation map[string]interface{}) {
				delete(mapFromOperationContext(invocation["arguments"]), operationPlanServerProjectedBindingKey)
			},
		},
		{
			name: "wrong server binding fingerprint",
			mutate: func(invocation map[string]interface{}) {
				mapFromOperationContext(invocation["arguments"])[operationPlanServerProjectedBindingKey] = "binding-fingerprint-old"
			},
		},
		{
			name: "phase missing server binding fingerprint",
			mutateMetadata: func(metadata map[string]interface{}) {
				phase := mapSliceFromAny(mapFromOperationContext(metadata["operation_plan"])["phases"])[0]
				delete(mapFromOperationContext(phase["expected_action"]), operationPlanServerProjectedBindingKey)
			},
			mutate: func(map[string]interface{}) {},
		},
		{
			name: "failed execution",
			mutate: func(invocation map[string]interface{}) {
				invocation["status"] = "error"
			},
		},
		{
			name: "partial provider outcome",
			mutate: func(invocation map[string]interface{}) {
				result := mapFromOperationContext(invocation["result"])
				result["operation_status"] = "partially_succeeded"
				delete(result, "provider_success_confirmed")
			},
		},
		{
			name: "unknown provider outcome",
			mutate: func(invocation map[string]interface{}) {
				result := mapFromOperationContext(invocation["result"])
				result["operation_status"] = "outcome_unknown"
				delete(result, "provider_success_confirmed")
			},
		},
		{
			name: "missing provider outcome",
			mutate: func(invocation map[string]interface{}) {
				result := mapFromOperationContext(invocation["result"])
				delete(result, "operation_status")
				delete(result, "provider_success_confirmed")
			},
		},
		{
			name: "unrecognized provider outcome",
			mutate: func(invocation map[string]interface{}) {
				result := mapFromOperationContext(invocation["result"])
				result["operation_status"] = "provider_says_maybe"
				delete(result, "provider_success_confirmed")
			},
		},
		{
			name: "completed provider outcome explicitly unconfirmed",
			mutate: func(invocation map[string]interface{}) {
				result := mapFromOperationContext(invocation["result"])
				result["operation_status"] = "completed"
				result["provider_success_confirmed"] = false
			},
		},
		{
			name: "completed provider outcome has malformed confirmation",
			mutate: func(invocation map[string]interface{}) {
				result := mapFromOperationContext(invocation["result"])
				result["operation_status"] = "completed"
				result["provider_success_confirmed"] = "false"
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metadata := projectedExternalActionCompletionMetadata(false, "Alice", "epoch-1")
			invocation := projectedExternalActionCompletionInvocation("Alice", "epoch-1")
			if test.mutateMetadata != nil {
				test.mutateMetadata(metadata)
			}
			test.mutate(invocation)

			applyOperationPlanInvocationState(metadata, []map[string]interface{}{invocation, invocation})

			plan := mapFromOperationContext(metadata["operation_plan"])
			phase := mapSliceFromAny(plan["phases"])[0]
			if got := stringFromAny(phase["status"]); got == operationPlanStepStatusCompleted {
				t.Fatalf("inexact evidence completed phase: %#v", phase)
			}
			if effects := mapSliceFromAny(plan[operationPlanEffectLedgerKey]); len(effects) != 0 {
				t.Fatalf("inexact evidence produced effects: %#v", effects)
			}
			if got := len(mapSliceFromAny(plan[operationPlanActionAttemptsKey])); got != 1 {
				t.Fatalf("deduplicated attempt count = %d, want 1", got)
			}
		})
	}
}

func TestProjectedExternalActionCompletionBridgeReconcilesStructuredOutcome(t *testing.T) {
	metadata := projectedExternalActionCompletionMetadata(true, "Alice", "epoch-1")
	applyOperationPlanInvocationState(metadata, []map[string]interface{}{
		projectedExternalActionCompletionInvocation("Alice", "epoch-1"),
	})

	plan := mapFromOperationContext(metadata["operation_plan"])
	phase := mapSliceFromAny(plan["phases"])[0]
	outcome := mapSliceFromAny(plan[operationPlanOutcomesKey])[0]
	if got := stringFromAny(phase["status"]); got != operationPlanStepStatusCompleted {
		t.Fatalf("phase status = %q, want completed", got)
	}
	if got := stringFromAny(outcome["status"]); got != operationPlanStepStatusCompleted {
		t.Fatalf("outcome status = %q, want completed", got)
	}
	if got := stringFromAny(plan["status"]); got != operationPlanStatusCompleted {
		t.Fatalf("plan status = %q, want completed", got)
	}
	effect := mapSliceFromAny(plan[operationPlanEffectLedgerKey])[0]
	if stringFromAny(effect["phase_id"]) != "phase-send" ||
		stringFromAny(effect["integration_id"]) != "wecom" ||
		stringFromAny(effect["action_id"]) != "wecom.message.send" ||
		stringFromAny(effect["connection_id"]) != "connection-1" ||
		stringFromAny(effect[operationPlanServerProjectedEpochKey]) != "epoch-1" ||
		stringFromAny(effect[operationPlanServerProjectedBindingKey]) != "binding-fingerprint-1" {
		t.Fatalf("external effect identity = %#v", effect)
	}
	specs := mapSliceFromAny(mapFromOperationContext(outcome["acceptance"])["effects"])
	if len(specs) != 1 || stringFromAny(specs[0]["type"]) != operationPlanExternalActionEffectType ||
		stringFromAny(specs[0]["resource_id"]) != "phase-send" {
		t.Fatalf("outcome acceptance = %#v, want phase-bound external effect", specs)
	}
	// The persisted plan consumed by the next turn must no longer advertise the
	// projected Action as open work.
	continuation := compactOperationPlanForPrompt(plan)
	if got := stringFromAny(mapSliceFromAny(continuation["phases"])[0]["status"]); got != operationPlanStepStatusCompleted {
		t.Fatalf("continuation phase status = %q, want completed", got)
	}
	if got := stringFromAny(mapSliceFromAny(continuation["outcomes"])[0]["status"]); got != operationPlanStepStatusCompleted {
		t.Fatalf("continuation outcome status = %q, want completed", got)
	}
}

func TestProjectedExternalActionCompletionBridgeCompletesGovernedResumeExactlyOnce(t *testing.T) {
	frozen := toolgovernance.NewFrozenInvocation(toolgovernance.FrozenInvocationRequest{
		CorrelationID: "correlation-external-1",
		SkillID:       skills.SkillExternalApps,
		ToolName:      "execute_action",
		Arguments: map[string]interface{}{
			"integration_id": "wecom",
			"action_id":      "wecom.message.send",
			"connection_id":  "connection-1",
			"arguments":      map[string]interface{}{"recipient_ref": "Alice", "content": "hello"},
		},
		Now: time.Now(),
	})
	metadata := projectedExternalActionCompletionMetadata(true, "Alice", "epoch-1")
	bound := bindPendingGovernedInvocationToOperationPlan(metadata, map[string]interface{}{
		"frozen_invocation": frozen,
		"plan_phase_id":     "phase-send",
	})
	phase := mapSliceFromAny(mapFromOperationContext(bound["operation_plan"])["phases"])[0]
	binding := mapFromOperationContext(phase[operationPlanRuntimeBindingKey])
	if stringFromAny(binding[operationPlanServerProjectedEpochKey]) != "epoch-1" ||
		stringFromAny(binding[operationPlanServerProjectedBindingKey]) != "binding-fingerprint-1" ||
		stringFromAny(mapFromOperationContext(binding["target"])["connection_id"]) != "connection-1" ||
		stringFromAny(mapFromOperationContext(binding["target_arguments"])["recipient_ref"]) != "Alice" {
		t.Fatalf("governed projected binding = %#v", binding)
	}

	completed, terminal := completeBoundGovernedInvocationOperationPlan(bound, frozen)
	if !terminal {
		t.Fatal("terminal = false, want exact governed external outcome completed")
	}
	plan := mapFromOperationContext(completed["operation_plan"])
	if got := stringFromAny(mapSliceFromAny(plan["phases"])[0]["status"]); got != operationPlanStepStatusCompleted {
		t.Fatalf("phase status = %q, want completed", got)
	}
	if got := stringFromAny(mapSliceFromAny(plan[operationPlanOutcomesKey])[0]["status"]); got != operationPlanStepStatusCompleted {
		t.Fatalf("outcome status = %q, want completed", got)
	}

	completedAgain, terminalAgain := completeBoundGovernedInvocationOperationPlan(completed, frozen)
	if !terminalAgain {
		t.Fatal("terminal after repeated continuation = false, want already-completed plan terminal")
	}
	if got := len(mapSliceFromAny(mapFromOperationContext(completedAgain["operation_plan"])[operationPlanEffectLedgerKey])); got != 1 {
		t.Fatalf("effect count after repeated continuation = %d, want 1", got)
	}
}

func TestProjectedExternalActionCompletionBridgeCompletesGovernedUnstructuredPhase(t *testing.T) {
	frozen := projectedExternalActionCompletionFrozenInvocation("Alice")
	metadata := projectedExternalActionCompletionMetadata(false, "Alice", "epoch-1")
	bound := bindPendingGovernedInvocationToOperationPlan(metadata, map[string]interface{}{
		"frozen_invocation": frozen, "plan_phase_id": "phase-send",
	})

	completed, terminal := completeBoundGovernedInvocationOperationPlan(bound, frozen)
	if !terminal {
		t.Fatal("terminal = false, want exact governed unstructured phase completed")
	}
	phase := mapSliceFromAny(mapFromOperationContext(completed["operation_plan"])["phases"])[0]
	if got := stringFromAny(phase["status"]); got != operationPlanStepStatusCompleted {
		t.Fatalf("phase status = %q, want completed", got)
	}
}

func TestProjectedExternalActionCompletionBridgeDoesNotBindGovernedMismatch(t *testing.T) {
	tests := []struct {
		name              string
		target            string
		actionID          string
		ledgerEpoch       string
		removeFingerprint bool
	}{
		{name: "wrong exact target", target: "alice", actionID: "wecom.message.send", ledgerEpoch: "epoch-1"},
		{name: "wrong Action", target: "Alice", actionID: "wecom.calendar.create", ledgerEpoch: "epoch-1"},
		{name: "missing server epoch", target: "Alice", actionID: "wecom.message.send", ledgerEpoch: ""},
		{name: "missing server fingerprint", target: "Alice", actionID: "wecom.message.send", ledgerEpoch: "epoch-1", removeFingerprint: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			frozen := toolgovernance.NewFrozenInvocation(toolgovernance.FrozenInvocationRequest{
				CorrelationID: "correlation-external-mismatch",
				SkillID:       skills.SkillExternalApps,
				ToolName:      "execute_action",
				Arguments: map[string]interface{}{
					"integration_id": "wecom", "action_id": test.actionID, "connection_id": "connection-1",
					"arguments": map[string]interface{}{"recipient_ref": test.target},
				},
			})
			metadata := projectedExternalActionCompletionMetadata(true, "Alice", test.ledgerEpoch)
			if test.removeFingerprint {
				phase := mapSliceFromAny(mapFromOperationContext(metadata["operation_plan"])["phases"])[0]
				delete(mapFromOperationContext(phase["expected_action"]), operationPlanServerProjectedBindingKey)
			}
			bound := bindPendingGovernedInvocationToOperationPlan(metadata, map[string]interface{}{
				"frozen_invocation": frozen, "plan_phase_id": "phase-send",
			})
			phase := mapSliceFromAny(mapFromOperationContext(bound["operation_plan"])["phases"])[0]
			if binding := mapFromOperationContext(phase[operationPlanRuntimeBindingKey]); len(binding) != 0 {
				t.Fatalf("mismatched governed invocation was bound: %#v", binding)
			}
			completed, terminal := completeBoundGovernedInvocationOperationPlan(bound, frozen)
			if terminal {
				t.Fatal("mismatched governed invocation completed structured plan")
			}
			phase = mapSliceFromAny(mapFromOperationContext(completed["operation_plan"])["phases"])[0]
			if got := stringFromAny(phase["status"]); got == operationPlanStepStatusCompleted {
				t.Fatalf("mismatched governed phase status = %q", got)
			}
		})
	}
}

func projectedExternalActionCompletionMetadata(structured bool, recipient string, ledgerEpoch string) map[string]interface{} {
	phase := map[string]interface{}{
		"id": "phase-send", "step": "send message", "status": "in_progress",
		operationPlanServerProjectedEpochKey: ledgerEpoch,
		"expected_action": map[string]interface{}{
			"skill_id":                             skills.SkillExternalApps,
			"tool_name":                            "execute_action",
			operationPlanServerProjectedToolKey:    "wecom_send_message",
			operationPlanServerProjectedBindingKey: "binding-fingerprint-1",
			"target": map[string]interface{}{
				"integration_id": "wecom",
				"action_id":      "wecom.message.send",
			},
			"target_arguments": map[string]interface{}{"recipient_ref": recipient},
		},
	}
	plan := map[string]interface{}{
		"status": operationPlanStatusRunning,
		"phases": []interface{}{phase},
	}
	if structured {
		phase["outcome_id"] = "outcome-send"
		plan[operationPlanOutcomesKey] = []interface{}{map[string]interface{}{
			"id": "outcome-send", "goal": "send message", "status": "in_progress", "required": true,
			"verification_mode": "runtime_effects",
		}}
	}
	return map[string]interface{}{"operation_plan": plan}
}

func projectedExternalActionCompletionInvocation(recipient string, ledgerEpoch string) map[string]interface{} {
	return map[string]interface{}{
		"kind": "tool_call", "status": "success", "runtime_id": "runtime-send-1",
		"skill_id": skills.SkillExternalApps, "tool_name": "execute_action",
		"arguments": map[string]interface{}{
			"integration_id": "wecom", "action_id": "wecom.message.send", "connection_id": "connection-1",
			"plan_phase_id": "phase-send", operationPlanServerProjectedEpochKey: ledgerEpoch,
			operationPlanServerProjectedBindingKey: "binding-fingerprint-1",
			"operation_plan_target":                map[string]interface{}{"recipient_ref": recipient},
		},
		"result": map[string]interface{}{
			"operation_status": "completed", "provider_success_confirmed": true,
			"integration_id": "wecom", "action_id": "wecom.message.send", "connection_id": "connection-1",
		},
	}
}

func projectedExternalActionCompletionFrozenInvocation(recipient string) toolgovernance.FrozenInvocation {
	return toolgovernance.NewFrozenInvocation(toolgovernance.FrozenInvocationRequest{
		CorrelationID: "correlation-external-helper",
		SkillID:       skills.SkillExternalApps,
		ToolName:      "execute_action",
		Arguments: map[string]interface{}{
			"integration_id": "wecom", "action_id": "wecom.message.send", "connection_id": "connection-1",
			"arguments": map[string]interface{}{"recipient_ref": recipient, "content": "hello"},
		},
		Now: time.Now(),
	})
}
