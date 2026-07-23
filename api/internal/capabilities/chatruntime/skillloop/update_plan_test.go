package skillloop

import (
	"context"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestNormalizePlanSnapshotEnforcesStructuralProgressRules(t *testing.T) {
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

func successfulReadFileEvidence() map[string]interface{} {
	return map[string]interface{}{
		"evidence_ledger": []interface{}{map[string]interface{}{
			"status": "completed", "skill_id": "file-reader", "tool_name": "read_file", "invocation_id": "runtime_id:read-1",
		}},
	}
}
