package skillloop

import (
	"context"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

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

func successfulReadFileEvidence() map[string]interface{} {
	return map[string]interface{}{
		"evidence_ledger": []interface{}{map[string]interface{}{
			"status": "completed", "skill_id": "file-reader", "tool_name": "read_file", "invocation_id": "runtime_id:read-1",
		}},
	}
}
