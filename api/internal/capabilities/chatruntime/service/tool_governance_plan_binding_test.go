package service

import (
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
)

func TestGovernedInvocationCompletesUniqueBoundPlanPhase(t *testing.T) {
	frozen := toolgovernance.NewFrozenInvocation(toolgovernance.FrozenInvocationRequest{
		CorrelationID: "correlation-1",
		SkillID:       "agent-management",
		ToolName:      "update_agent_config",
		Arguments:     map[string]interface{}{"agent_id": "agent-1"},
		Now:           time.Now(),
	})
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":            operationPlanStatusRunning,
			"evidence_revision": 3,
			"phases": []interface{}{
				map[string]interface{}{"id": "phase-1", "status": operationPlanStepStatusCompleted},
				map[string]interface{}{"id": "phase-2", "status": "in_progress", "expected_action": map[string]interface{}{
					"skill_id": "agent-management", "tool_name": "update_agent_config", "target": map[string]interface{}{"agent_id": "agent-1"},
				}},
			},
		},
	}

	bound := bindPendingGovernedInvocationToOperationPlan(metadata, map[string]interface{}{"frozen_invocation": frozen, "plan_phase_id": "phase-2"})
	phases := mapSliceFromAny(mapFromOperationContext(bound["operation_plan"])["phases"])
	if len(phases) != 2 || len(mapFromOperationContext(phases[1][operationPlanRuntimeBindingKey])) == 0 {
		t.Fatalf("phases = %#v, want runtime binding on phase-2", phases)
	}

	completed, terminal := completeBoundGovernedInvocationOperationPlan(bound, frozen)
	if !terminal {
		t.Fatal("terminal = false, want true")
	}
	plan := mapFromOperationContext(completed["operation_plan"])
	phases = mapSliceFromAny(plan["phases"])
	if got := stringFromAny(phases[1]["status"]); got != operationPlanStepStatusCompleted {
		t.Fatalf("phase status = %q, want completed", got)
	}
	if got := stringFromAny(plan["status"]); got != operationPlanStatusCompleted {
		t.Fatalf("plan status = %q, want completed", got)
	}
	if got := intValueFromAny(plan["evidence_revision_at_plan_update"]); got != 3 {
		t.Fatalf("evidence revision baseline = %d, want 3", got)
	}
	if got := stringFromAny(plan["plan_sync_status"]); got != "current" {
		t.Fatalf("plan sync status = %q, want current", got)
	}
	refs := stringSliceFromAny(phases[1]["evidence_refs"])
	if len(refs) < 2 {
		t.Fatalf("evidence refs = %#v, want tool and invocation refs", refs)
	}
}

func TestGovernedInvocationDoesNotBindAmbiguousActivePlanPhases(t *testing.T) {
	frozen := toolgovernance.NewFrozenInvocation(toolgovernance.FrozenInvocationRequest{
		CorrelationID: "correlation-1",
		SkillID:       "agent-management",
		ToolName:      "update_agent_config",
		Arguments:     map[string]interface{}{"agent_id": "agent-1"},
	})
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"phases": []interface{}{
				map[string]interface{}{"id": "phase-1", "status": "in_progress", "expected_action": map[string]interface{}{"skill_id": "agent-management", "tool_name": "update_agent_config"}},
				map[string]interface{}{"id": "phase-2", "status": "in_progress", "expected_action": map[string]interface{}{"skill_id": "agent-management", "tool_name": "update_agent_config"}},
			},
		},
	}

	bound := bindPendingGovernedInvocationToOperationPlan(metadata, map[string]interface{}{"frozen_invocation": frozen})
	for _, phase := range mapSliceFromAny(mapFromOperationContext(bound["operation_plan"])["phases"]) {
		if len(mapFromOperationContext(phase[operationPlanRuntimeBindingKey])) > 0 {
			t.Fatalf("ambiguous phase unexpectedly bound: %#v", phase)
		}
	}
}

func TestGovernedInvocationDoesNotCompleteMismatchedTarget(t *testing.T) {
	first := toolgovernance.NewFrozenInvocation(toolgovernance.FrozenInvocationRequest{
		CorrelationID: "correlation-1",
		SkillID:       "agent-management",
		ToolName:      "update_agent_config",
		Arguments:     map[string]interface{}{"agent_id": "agent-1"},
	})
	second := toolgovernance.NewFrozenInvocation(toolgovernance.FrozenInvocationRequest{
		CorrelationID: "correlation-2",
		SkillID:       "agent-management",
		ToolName:      "update_agent_config",
		Arguments:     map[string]interface{}{"agent_id": "agent-2"},
	})
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"phases": []interface{}{map[string]interface{}{"id": "phase-1", "status": "in_progress", operationPlanRuntimeBindingKey: governedInvocationPlanBinding(first)}},
		},
	}

	completed, terminal := completeBoundGovernedInvocationOperationPlan(metadata, second)
	if terminal {
		t.Fatal("terminal = true for mismatched frozen invocation")
	}
	phase := mapSliceFromAny(mapFromOperationContext(completed["operation_plan"])["phases"])[0]
	if got := stringFromAny(phase["status"]); got != "in_progress" {
		t.Fatalf("phase status = %q, want in_progress", got)
	}
}

func TestGovernedInvocationBindsBusinessPhaseInsteadOfActiveNavigationPhase(t *testing.T) {
	frozen := toolgovernance.NewFrozenInvocation(toolgovernance.FrozenInvocationRequest{
		CorrelationID: "correlation-update-agent",
		SkillID:       "agent-management",
		ToolName:      "update_agent_config",
		Arguments:     map[string]interface{}{"agent_id": "agent-1"},
	})
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": operationPlanStatusRunning,
			"phases": []interface{}{
				map[string]interface{}{"id": "phase-1", "status": operationPlanStepStatusCompleted, "step": "读取第十章"},
				map[string]interface{}{"id": "phase-2", "status": operationPlanStepStatusCompleted, "step": "生成并保存文件"},
				map[string]interface{}{"id": "phase-3", "status": "in_progress", "step": "回到智能体管理", "expected_action": map[string]interface{}{
					"skill_id": "console-navigator", "tool_name": "navigate", "target": map[string]interface{}{"href": "/console/agents"},
				}},
				map[string]interface{}{"id": "phase-4", "status": operationPlanStepStatusPending, "step": "更新当前智能体的系统提示词", "expected_action": map[string]interface{}{
					"skill_id": "agent-management", "tool_name": "update_agent_config", "target": map[string]interface{}{"agent_id": "agent-1"},
				}},
			},
		},
	}

	bound := bindPendingGovernedInvocationToOperationPlan(metadata, map[string]interface{}{"frozen_invocation": frozen, "plan_phase_id": "phase-4"})
	phases := mapSliceFromAny(mapFromOperationContext(bound["operation_plan"])["phases"])
	if binding := mapFromOperationContext(phases[2][operationPlanRuntimeBindingKey]); len(binding) != 0 {
		t.Fatalf("navigation phase binding = %#v, want none", binding)
	}
	binding := mapFromOperationContext(phases[3][operationPlanRuntimeBindingKey])
	if got := stringFromAny(binding["phase_id"]); got != "phase-4" {
		t.Fatalf("business phase binding = %#v, want phase-4", binding)
	}

	completed, terminal := completeBoundGovernedInvocationOperationPlan(bound, frozen)
	if terminal {
		t.Fatal("terminal = true while navigation phase remains in_progress")
	}
	plan := mapFromOperationContext(completed["operation_plan"])
	phases = mapSliceFromAny(plan["phases"])
	if got := stringFromAny(phases[2]["status"]); got != "in_progress" {
		t.Fatalf("navigation phase status = %q, want unchanged", got)
	}
	if got := stringFromAny(phases[3]["status"]); got != operationPlanStepStatusCompleted {
		t.Fatalf("business phase status = %q, want completed", got)
	}
	if got := stringFromAny(plan["status"]); got == operationPlanStatusCompleted {
		t.Fatalf("plan status = %q, must not complete with unfinished phase", got)
	}
}

func TestGovernedInvocationCompletesExactPendingPhaseAndEntersTerminalOnly(t *testing.T) {
	frozen := toolgovernance.NewFrozenInvocation(toolgovernance.FrozenInvocationRequest{
		CorrelationID: "correlation-update-agent-terminal",
		SkillID:       "agent-management",
		ToolName:      "update_agent_config",
		Arguments:     map[string]interface{}{"agent_id": "agent-1"},
	})
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": operationPlanStatusRunning,
			"phases": []interface{}{
				map[string]interface{}{"id": "phase-1", "status": operationPlanStepStatusCompleted, "step": "回到智能体管理"},
				map[string]interface{}{"id": "phase-2", "status": operationPlanStepStatusPending, "step": "更新当前智能体配置", "expected_action": map[string]interface{}{
					"skill_id": "agent-management", "tool_name": "update_agent_config", "target": map[string]interface{}{"agent_id": "agent-1"},
				}},
			},
		},
	}
	bound := bindPendingGovernedInvocationToOperationPlan(metadata, map[string]interface{}{"frozen_invocation": frozen, "plan_phase_id": "phase-2"})
	completed, terminal := completeBoundGovernedInvocationOperationPlan(bound, frozen)
	if !terminal {
		t.Fatal("terminal = false, want terminal-only after exact final phase completes")
	}
	if got := stringFromAny(mapFromOperationContext(completed["operation_plan"])["status"]); got != operationPlanStatusCompleted {
		t.Fatalf("plan status = %q, want completed", got)
	}
}

func TestGovernedInvocationEntersTerminalOnlyFromReconciledOutcomeEffect(t *testing.T) {
	frozen := toolgovernance.NewFrozenInvocation(toolgovernance.FrozenInvocationRequest{
		CorrelationID: "correlation-outcome-update",
		SkillID:       "agent-management",
		ToolName:      "update_agent_config",
		Arguments:     map[string]interface{}{"agent_id": "agent-1", "system_prompt_patch": map[string]interface{}{"operation": "append"}},
	})
	strategy := &AIChatTurnStrategy{
		Intent:         "manage_agent_asset",
		ToolChoiceMode: aiChatTurnToolChoiceModelDecides,
		Outcomes: []AIChatTurnOutcome{{
			ID: "outcome-agent", Goal: "Update the Agent prompt", TargetResourceType: "agent", TargetResourceID: "agent-1",
			Capabilities: []string{agentCapabilitySystemPrompt},
		}},
	}
	metadata := map[string]interface{}{
		"operation_plan": operationPlanFromTurnStrategy("task-governed-outcome", &chatRequestParts{Query: "Update the Agent prompt."}, strategy),
	}
	applyOperationPlanInvocationState(metadata, []map[string]interface{}{{
		"kind": "tool_call", "status": "success", "runtime_id": "approved-update-1",
		"skill_id": "agent-management", "tool_name": "update_agent_config",
		"arguments": frozen.Arguments,
		"result":    map[string]interface{}{"status": "completed", "agent_id": "agent-1", "updated_fields": []interface{}{"system_prompt"}},
	}})

	completed, terminal := completeBoundGovernedInvocationOperationPlan(metadata, frozen)
	if !terminal {
		t.Fatal("terminal = false, want reconciled required outcome to enter terminal-only")
	}
	plan := mapFromOperationContext(completed["operation_plan"])
	if got := stringFromAny(plan["status"]); got != operationPlanStatusCompleted {
		t.Fatalf("plan status = %q, want completed", got)
	}
	outcome := mapSliceFromAny(plan[operationPlanOutcomesKey])[0]
	if got := stringFromAny(outcome["status"]); got != operationPlanStepStatusCompleted {
		t.Fatalf("outcome status = %q, want completed", got)
	}
}

func TestGovernedInvocationDoesNotBindUniqueActivePhaseWithoutExpectedAction(t *testing.T) {
	frozen := toolgovernance.NewFrozenInvocation(toolgovernance.FrozenInvocationRequest{
		CorrelationID: "correlation-no-structure",
		SkillID:       "agent-management",
		ToolName:      "update_agent_config",
		Arguments:     map[string]interface{}{"agent_id": "agent-1"},
	})
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"phases": []interface{}{
				map[string]interface{}{"id": "phase-navigation", "status": "in_progress", "step": "return to Agent management"},
			},
		},
	}

	bound := bindPendingGovernedInvocationToOperationPlan(metadata, map[string]interface{}{"frozen_invocation": frozen})
	phase := mapSliceFromAny(mapFromOperationContext(bound["operation_plan"])["phases"])[0]
	if binding := mapFromOperationContext(phase[operationPlanRuntimeBindingKey]); len(binding) > 0 {
		t.Fatalf("binding = %#v, want no pure-status fallback", binding)
	}
}

func TestGovernedInvocationBindsButDoesNotCompleteExplicitUnstructuredPhase(t *testing.T) {
	frozen := toolgovernance.NewFrozenInvocation(toolgovernance.FrozenInvocationRequest{
		CorrelationID: "correlation-unstructured",
		SkillID:       "agent-management",
		ToolName:      "update_agent_config",
		Arguments:     map[string]interface{}{"agent_id": "agent-1"},
	})
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": operationPlanStatusRunning,
			"phases": []interface{}{map[string]interface{}{
				"id": "phase-update-outcome", "status": "in_progress", "step": "update the Agent",
			}},
		},
	}

	bound := bindPendingGovernedInvocationToOperationPlan(metadata, map[string]interface{}{
		"frozen_invocation": frozen,
		"plan_phase_id":     "phase-update-outcome",
	})
	phase := mapSliceFromAny(mapFromOperationContext(bound["operation_plan"])["phases"])[0]
	binding := mapFromOperationContext(phase[operationPlanRuntimeBindingKey])
	if got := stringFromAny(binding["frozen_invocation_id"]); got != frozen.ID {
		t.Fatalf("runtime binding = %#v, want exact frozen invocation %q", binding, frozen.ID)
	}

	completed, terminal := completeBoundGovernedInvocationOperationPlan(bound, frozen)
	if terminal {
		t.Fatal("terminal = true, want unstructured phase to remain open")
	}
	phase = mapSliceFromAny(mapFromOperationContext(completed["operation_plan"])["phases"])[0]
	if got := stringFromAny(phase["status"]); got != "in_progress" {
		t.Fatalf("phase status = %q, want in_progress", got)
	}
	if completion := mapFromOperationContext(phase["completion_action"]); len(completion) != 0 {
		t.Fatalf("completion_action = %#v, want none without acceptance facts", completion)
	}
}

func TestGovernedInvocationExplicitFinalIntentCompletesUniqueReconciliationPhase(t *testing.T) {
	frozen := toolgovernance.NewFrozenInvocation(toolgovernance.FrozenInvocationRequest{
		CorrelationID: "correlation-final-reconciliation",
		SkillID:       "agent-management",
		ToolName:      "update_agent_config",
		Arguments:     map[string]interface{}{"agent_id": "agent-1"},
	})
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": operationPlanStatusRunning,
			"phases": []interface{}{map[string]interface{}{
				"id":                "phase-model-reconciliation",
				"status":            "in_progress",
				"verification_mode": "model_reconciliation",
			}},
		},
	}

	bound := bindPendingGovernedInvocationToOperationPlan(metadata, map[string]interface{}{
		"frozen_invocation": frozen,
		"completion_intent": governedCompletionIntentFinalizeIfSuccess,
	})
	phase := mapSliceFromAny(mapFromOperationContext(bound["operation_plan"])["phases"])[0]
	binding := mapFromOperationContext(phase[operationPlanRuntimeBindingKey])
	if got := stringFromAny(binding["completion_intent"]); got != governedCompletionIntentFinalizeIfSuccess {
		t.Fatalf("completion_intent = %q, want %q; binding=%#v", got, governedCompletionIntentFinalizeIfSuccess, binding)
	}

	completed, terminal := completeBoundGovernedInvocationOperationPlan(bound, frozen)
	if !terminal {
		t.Fatal("terminal = false, want explicit final approved action to close the only reconciliation phase")
	}
	plan := mapFromOperationContext(completed["operation_plan"])
	if got := stringFromAny(plan["status"]); got != operationPlanStatusCompleted {
		t.Fatalf("plan status = %q, want completed", got)
	}
}

func TestGovernedInvocationFinalIntentDoesNotChooseAmbiguousReconciliationPhase(t *testing.T) {
	frozen := toolgovernance.NewFrozenInvocation(toolgovernance.FrozenInvocationRequest{
		CorrelationID: "correlation-ambiguous-reconciliation",
		SkillID:       "agent-management",
		ToolName:      "update_agent_config",
		Arguments:     map[string]interface{}{"agent_id": "agent-1"},
	})
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"phases": []interface{}{
				map[string]interface{}{"id": "phase-1", "status": "in_progress", "verification_mode": "model_reconciliation"},
				map[string]interface{}{"id": "phase-2", "status": operationPlanStepStatusPending, "verification_mode": "model_reconciliation"},
			},
		},
	}

	bound := bindPendingGovernedInvocationToOperationPlan(metadata, map[string]interface{}{
		"frozen_invocation": frozen,
		"completion_intent": governedCompletionIntentFinalizeIfSuccess,
	})
	for _, phase := range mapSliceFromAny(mapFromOperationContext(bound["operation_plan"])["phases"]) {
		if binding := mapFromOperationContext(phase[operationPlanRuntimeBindingKey]); len(binding) > 0 {
			t.Fatalf("ambiguous reconciliation phase unexpectedly bound: %#v", binding)
		}
	}
}

func TestGovernedInvocationRejectsPlanPhaseIDWithMismatchedExpectedAction(t *testing.T) {
	frozen := toolgovernance.NewFrozenInvocation(toolgovernance.FrozenInvocationRequest{
		CorrelationID: "correlation-wrong-phase",
		SkillID:       "agent-management",
		ToolName:      "update_agent_config",
		Arguments:     map[string]interface{}{"agent_id": "agent-1"},
	})
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"phases": []interface{}{
				map[string]interface{}{
					"id": "phase-navigation", "status": "in_progress",
					"expected_action": map[string]interface{}{"skill_id": "console-navigator", "tool_name": "navigate", "target": map[string]interface{}{"href": "/console/agents"}},
				},
				map[string]interface{}{
					"id": "phase-update", "status": operationPlanStepStatusPending,
					"expected_action": map[string]interface{}{"skill_id": "agent-management", "tool_name": "update_agent_config", "target": map[string]interface{}{"agent_id": "agent-1"}},
				},
			},
		},
	}

	bound := bindPendingGovernedInvocationToOperationPlan(metadata, map[string]interface{}{
		"frozen_invocation": frozen,
		"plan_phase_id":     "phase-navigation",
	})
	for _, phase := range mapSliceFromAny(mapFromOperationContext(bound["operation_plan"])["phases"]) {
		if binding := mapFromOperationContext(phase[operationPlanRuntimeBindingKey]); len(binding) > 0 {
			t.Fatalf("mismatched requested phase unexpectedly bound: %#v", binding)
		}
	}
}
