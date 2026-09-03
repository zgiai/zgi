package skillloop

import (
	"context"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestNormalizePlanSnapshotEnforcesStructuralProgressRules(t *testing.T) {
	if _, err := normalizePlanSnapshot([]interface{}{
		map[string]interface{}{"id": "phase-1", "step": "First", "status": "in_progress"},
		map[string]interface{}{"id": "phase-1", "step": "Second", "status": "pending"},
	}); err == nil {
		t.Fatal("normalizePlanSnapshot() error = nil, want duplicate explicit phase ID rejection")
	}

	_, err := normalizePlanSnapshot([]interface{}{
		map[string]interface{}{"id": "phase-1", "step": "First", "status": "in_progress"},
		map[string]interface{}{"id": "phase-2", "step": "Second", "status": "in_progress"},
	})
	if err == nil {
		t.Fatal("normalizePlanSnapshot() error = nil, want multiple in_progress rejection")
	}

	phasesWithoutEvidence, err := normalizePlanSnapshot([]interface{}{
		map[string]interface{}{"id": "phase-1", "step": "First", "status": "completed"},
		map[string]interface{}{"id": "phase-2", "step": "Optional cleanup", "status": "skipped"},
	})
	if err != nil || len(phasesWithoutEvidence) != 2 {
		t.Fatalf("normalizePlanSnapshot() = %#v, %v; want advisory phases accepted", phasesWithoutEvidence, err)
	}
	for _, phase := range phasesWithoutEvidence {
		if got := evidenceStringFromAny(phase[operationPlanPhaseSourceKey]); got != operationPlanPhaseSourceModelAdvisory {
			t.Fatalf("normalized model phase source = %q, want %q: %#v", got, operationPlanPhaseSourceModelAdvisory, phase)
		}
	}

	phases, err := normalizePlanSnapshot([]interface{}{
		map[string]interface{}{
			"id": "phase-1", "step": "Read file", "status": "completed", "evidence_refs": []interface{}{"file-reader/read_file"},
			"expected_action": map[string]interface{}{
				"skill_id": "file-reader", "tool_name": "read_file", "target": map[string]interface{}{"file_id": "file-1", "secret": "drop-me"},
			},
		},
		map[string]interface{}{"step": "Optional cleanup", "status": "skipped", "note": "Not requested"},
	})
	if err != nil {
		t.Fatalf("normalizePlanSnapshot() error = %v", err)
	}
	if phases[1]["id"] != "phase-amendment-1" {
		t.Fatalf("generated amendment ID = %#v, want phase-amendment-1", phases[1]["id"])
	}
	refs := evidenceStringSliceFromAny(phases[0]["evidence_refs"])
	if len(refs) != 1 || refs[0] != "tool:file-reader/read_file" {
		t.Fatalf("evidence_refs = %#v, want canonical tool ref", refs)
	}
	expectedAction := evidenceMapFromAny(phases[0]["expected_action"])
	if expectedAction["skill_id"] != "file-reader" || expectedAction["tool_name"] != "read_file" {
		t.Fatalf("expected_action = %#v", expectedAction)
	}
	target := evidenceMapFromAny(expectedAction["target"])
	if target["file_id"] != "file-1" || target["secret"] != nil {
		t.Fatalf("expected_action.target = %#v, want allowlisted target only", target)
	}
}

func TestHandleUpdatePlanCallPreservesServerPhaseAuthority(t *testing.T) {
	state := map[string]interface{}{"operation_plan": map[string]interface{}{
		"phases": []interface{}{map[string]interface{}{
			"id": "phase-outcome", "outcome_id": "outcome-send", "step": "Send", "status": "in_progress",
			"required": true, operationPlanPhaseSourceKey: operationPlanPhaseSourceServerOutcome,
		}},
		"outcomes": []interface{}{map[string]interface{}{
			"id": "outcome-send", "required": true, "verification_mode": "runtime_effects",
		}},
	}}
	step := (&Runner{}).handleUpdatePlanCall("call-plan", map[string]interface{}{
		"plan": []interface{}{map[string]interface{}{
			"id": "phase-outcome", "step": "Rename the model progress item", "status": "in_progress",
			operationPlanPhaseSourceKey: operationPlanPhaseSourceModelAdvisory,
		}},
	}, state, 1)
	if step.recoverable || step.fatalErr != nil || step.trace.Status != "success" {
		t.Fatalf("handleUpdatePlanCall() = %#v", step)
	}
	phase := evidenceMapsFromAny(step.trace.Result["plan"])[0]
	if got := evidenceStringFromAny(phase[operationPlanPhaseSourceKey]); got != operationPlanPhaseSourceServerOutcome {
		t.Fatalf("server phase source = %q, want preserved server_outcome: %#v", got, phase)
	}
	if got := evidenceStringFromAny(phase["outcome_id"]); got != "outcome-send" {
		t.Fatalf("server outcome linkage = %q, want outcome-send: %#v", got, phase)
	}
	if required, ok := phase["required"].(bool); !ok || !required {
		t.Fatalf("server required classification was not preserved: %#v", phase)
	}
}

func TestHandleUpdatePlanCallProducesPersistablePlanTrace(t *testing.T) {
	step := (&Runner{}).handleUpdatePlanCall("call-plan", map[string]interface{}{
		"explanation": "file read completed",
		"plan": []interface{}{map[string]interface{}{
			"id": "phase-1", "step": "Read file", "status": "completed", "evidence_refs": []interface{}{"tool:file-reader/read_file"},
		}},
	}, successfulReadFileEvidence(), 2)
	if step.fatalErr != nil || step.trace.Kind != "plan_update" || step.trace.Status != "success" {
		t.Fatalf("handleUpdatePlanCall() step = %#v", step)
	}
	if step.trace.ToolName != skills.MetaToolUpdatePlan {
		t.Fatalf("trace.ToolName = %q, want %q", step.trace.ToolName, skills.MetaToolUpdatePlan)
	}
	if got, ok := step.trace.Arguments["round"].(int); !ok || got != 2 {
		t.Fatalf("trace round = %#v, want 2", step.trace.Arguments["round"])
	}
}

func TestHandleUpdatePlanCallAcceptsOutcomeContractWithoutCompatibilityPlan(t *testing.T) {
	step := (&Runner{}).handleUpdatePlanCall("call-outcomes", map[string]interface{}{
		"explanation": "the user changed the requested result",
		"outcomes": []interface{}{
			map[string]interface{}{
				"id": "outcome-file", "goal": "Save the generated file", "status": "pending",
				"capabilities": []interface{}{"managed_file"},
			},
			map[string]interface{}{
				"id": "outcome-agent", "goal": "Update the Agent prompt", "status": "pending",
				"depends_on": []interface{}{"outcome-file"}, "capabilities": []interface{}{"agent.system_prompt"},
			},
		},
	}, nil, 3)
	if step.fatalErr != nil || step.recoverable || step.trace.Status != "success" {
		t.Fatalf("handleUpdatePlanCall() = %#v, want successful outcome revision", step)
	}
	outcomes := evidenceMapsFromAny(step.trace.Result["outcomes"])
	if len(outcomes) != 2 || evidenceStringFromAny(outcomes[1]["id"]) != "outcome-agent" {
		t.Fatalf("trace outcomes = %#v, want two normalized outcomes", outcomes)
	}
	if plan := evidenceMapsFromAny(step.trace.Result["plan"]); len(plan) != 0 {
		t.Fatalf("compatibility plan = %#v, want omitted", plan)
	}
}

func TestHandleProgressiveSkillCallSkipsHiddenPlanUpdate(t *testing.T) {
	runner := &Runner{}
	state := map[string]interface{}{runtimeStateAllowPlanUpdateKey: false}
	for _, call := range []adapter.ToolCall{
		{
			ID: "direct-plan",
			Function: adapter.FunctionCall{
				Name:      skills.MetaToolUpdatePlan,
				Arguments: `{"plan":[{"id":"phase-1","step":"Read","status":"in_progress"}]}`,
			},
		},
		{
			ID: "nested-plan",
			Function: adapter.FunctionCall{
				Name:      skills.MetaToolCallSkillTool,
				Arguments: `{"tool_name":"update_plan","arguments":{"plan":[{"id":"phase-1","step":"Read","status":"in_progress"}]}}`,
			},
		},
	} {
		step := runner.handleProgressiveSkillCall(
			context.Background(), nil, &skills.ResolvedSkills{}, call, skills.ExecutionContext{},
			0, map[string]int{}, map[string]struct{}{}, state, 1, nil,
		)
		if step.recoverable || step.fatalErr != nil {
			t.Fatalf("handleProgressiveSkillCall(%s) = %#v, want non-failing advisory", call.ID, step)
		}
		if step.trace.Kind != "planner_feedback" || step.trace.Arguments["reason_code"] != "control_tool_not_required" {
			t.Fatalf("trace(%s) = %#v, want suppressed control-tool advisory", call.ID, step.trace)
		}
	}
}

func TestHandleUpdatePlanCallRecordsUnavailableEvidenceAsAuditWarning(t *testing.T) {
	step := (&Runner{}).handleUpdatePlanCall("call-plan", map[string]interface{}{
		"plan": []interface{}{map[string]interface{}{
			"id": "phase-1", "step": "Delete agent", "status": "completed", "evidence_refs": []interface{}{"agent-management/delete_agent"},
		}},
	}, successfulReadFileEvidence(), 2)
	if step.fatalErr != nil || step.recoverable || step.trace.Status != "success" {
		t.Fatalf("handleUpdatePlanCall() step = %#v, want successful advisory plan trace", step)
	}
	warnings := evidenceStringSliceFromAny(step.trace.Result["evidence_warnings"])
	if len(warnings) != 1 || warnings[0] != "unresolved_evidence_ref:tool:agent-management/delete_agent" {
		t.Fatalf("evidence_warnings = %#v", warnings)
	}
}

func TestHandleUpdatePlanCallWithAvailableProjectionsAcceptsOrdinaryMultiPhaseRevision(t *testing.T) {
	state := map[string]interface{}{}
	step := (&Runner{}).handleUpdatePlanCall("call-plan", map[string]interface{}{
		"plan": []interface{}{
			map[string]interface{}{"id": "phase-file", "step": "Read file", "status": "in_progress"},
			map[string]interface{}{"id": "phase-agent", "step": "Update agent", "status": "pending"},
		},
	}, state, 1)
	if step.recoverable || step.fatalErr != nil || step.trace.Status != "success" {
		t.Fatalf("handleUpdatePlanCall() = %#v, projections should not force expected_action onto an ordinary plan", step)
	}
}

func TestHandleUpdatePlanCallRestoresServerOutcomeClassification(t *testing.T) {
	serverBound := nativeRunnerPhase("phase-a", "in_progress")
	serverBound["outcome_id"] = "outcome-a"
	state := map[string]interface{}{"operation_plan": map[string]interface{}{"phases": []interface{}{
		serverBound,
		map[string]interface{}{"id": "phase-b", "outcome_id": "outcome-b", "step": "Send second", "status": "pending"},
	}}}
	state["operation_plan"].(map[string]interface{})["outcomes"] = []interface{}{
		map[string]interface{}{"id": "outcome-a", "required": true, "verification_mode": "runtime_effects"},
		map[string]interface{}{"id": "outcome-b", "required": true, "verification_mode": "runtime_effects"},
	}

	step := (&Runner{}).handleUpdatePlanCall("call-plan", map[string]interface{}{
		"plan": []interface{}{
			map[string]interface{}{
				"id": "phase-a", "step": "Send first", "status": "in_progress",
				"expected_action": nativeRunnerExpectedAction(),
			},
			map[string]interface{}{"id": "phase-b", "step": "Pretend second is explanation", "status": "pending", "completion_mode": "final_answer"},
		},
	}, state, 1)
	if step.recoverable || step.fatalErr != nil || step.trace.Status != "success" {
		t.Fatalf("projected revision = %#v, want server policy restored", step)
	}
	phases := evidenceMapsFromAny(step.trace.Result["plan"])
	if got := evidenceStringFromAny(phases[1][operationPlanPhaseSourceKey]); got != operationPlanPhaseSourceServerOutcome ||
		evidenceStringFromAny(phases[1]["outcome_id"]) != "outcome-b" ||
		evidenceStringFromAny(phases[1]["completion_mode"]) != "" {
		t.Fatalf("model final-answer classification overrode server runtime Outcome: %#v", phases[1])
	}
}

func successfulReadFileEvidence() map[string]interface{} {
	return map[string]interface{}{
		"evidence_ledger": []interface{}{map[string]interface{}{
			"status": "completed", "skill_id": "file-reader", "tool_name": "read_file", "invocation_id": "runtime_id:read-1",
		}},
	}
}
