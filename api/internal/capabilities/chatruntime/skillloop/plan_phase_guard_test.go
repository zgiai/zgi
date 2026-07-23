package skillloop

import (
	"context"
	"encoding/json"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestHandleProgressiveSkillCallTreatsMismatchedPlanPhaseAsAdvisory(t *testing.T) {
	runner, resolved, echoTool := newPlanPhaseGuardTestRunner(t)
	call := planPhaseGuardToolCall(t, "phase-navigation")
	result := runner.handleProgressiveSkillCall(
		context.Background(),
		NewPreparedChat("conv-phase-guard", "msg-phase-guard", "", "auto", &adapter.ChatRequest{}),
		resolved,
		call,
		skills.ExecutionContext{},
		0,
		map[string]int{},
		map[string]struct{}{"phase-guard-skill": {}},
		map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"phases": []interface{}{map[string]interface{}{
					"id": "phase-navigation", "status": "in_progress",
					"expected_action": map[string]interface{}{"skill_id": "console-navigator", "tool_name": "navigate"},
				}},
			},
		},
		1,
		nil,
	)

	if result.recoverable {
		t.Fatalf("result = %#v, want advisory phase association to execute", result)
	}
	if echoTool.calls != 1 {
		t.Fatalf("runtime calls = %d, want one", echoTool.calls)
	}
	if got := evidenceStringFromAny(result.trace.Arguments["plan_phase_id"]); got != "phase-navigation" {
		t.Fatalf("plan_phase_id = %q, want preserved advisory association", got)
	}
}

func TestHandleProgressiveSkillCallInfersUniqueStructuredPlanPhase(t *testing.T) {
	runner, resolved, echoTool := newPlanPhaseGuardTestRunner(t)
	call := planPhaseGuardToolCall(t, "")
	result := runner.handleProgressiveSkillCall(
		context.Background(),
		NewPreparedChat("conv-phase-infer", "msg-phase-infer", "", "auto", &adapter.ChatRequest{}),
		resolved,
		call,
		skills.ExecutionContext{},
		0,
		map[string]int{},
		map[string]struct{}{"phase-guard-skill": {}},
		map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"phases": []interface{}{map[string]interface{}{
					"id": "phase-business", "status": "pending",
					"expected_action": map[string]interface{}{"skill_id": "phase-guard-skill", "tool_name": "echo_value"},
				}},
			},
		},
		1,
		nil,
	)

	if result.recoverable || echoTool.calls != 1 {
		t.Fatalf("result = %#v calls=%d, want one successful runtime call", result, echoTool.calls)
	}
	if got := evidenceStringFromAny(result.trace.Arguments["plan_phase_id"]); got != "phase-business" {
		t.Fatalf("plan_phase_id = %q, want inferred phase-business", got)
	}
}

func TestHandleProgressiveSkillCallBindsExplicitCurrentUnstructuredPhase(t *testing.T) {
	runner, resolved, echoTool := newPlanPhaseGuardTestRunner(t)
	result := runner.handleProgressiveSkillCall(
		context.Background(),
		NewPreparedChat("conv-phase-bind", "msg-phase-bind", "", "auto", &adapter.ChatRequest{}),
		resolved,
		planPhaseGuardToolCall(t, "phase-business"),
		skills.ExecutionContext{},
		0,
		map[string]int{},
		map[string]struct{}{"phase-guard-skill": {}},
		map[string]interface{}{"operation_plan": map[string]interface{}{"phases": []interface{}{
			map[string]interface{}{"id": "phase-business", "status": "in_progress"},
			map[string]interface{}{"id": "phase-next", "status": "pending"},
		}}},
		1,
		nil,
	)

	if result.recoverable || echoTool.calls != 1 {
		t.Fatalf("result = %#v calls=%d, want explicit current phase to execute", result, echoTool.calls)
	}
	if got := evidenceStringFromAny(result.trace.Arguments["plan_phase_id"]); got != "phase-business" {
		t.Fatalf("plan_phase_id = %q, want phase-business", got)
	}
}

func TestHandleProgressiveSkillCallAllowsExplicitFutureOutcomePhase(t *testing.T) {
	runner, resolved, echoTool := newPlanPhaseGuardTestRunner(t)
	result := runner.handleProgressiveSkillCall(
		context.Background(),
		NewPreparedChat("conv-phase-future", "msg-phase-future", "", "auto", &adapter.ChatRequest{}),
		resolved,
		planPhaseGuardToolCall(t, "phase-next"),
		skills.ExecutionContext{},
		0,
		map[string]int{},
		map[string]struct{}{"phase-guard-skill": {}},
		map[string]interface{}{"operation_plan": map[string]interface{}{"phases": []interface{}{
			map[string]interface{}{"id": "phase-current", "status": "in_progress"},
			map[string]interface{}{"id": "phase-next", "status": "pending"},
		}}},
		1,
		nil,
	)

	if result.recoverable {
		t.Fatalf("result = %#v, want future outcome phase association to execute", result)
	}
	if echoTool.calls != 1 {
		t.Fatalf("runtime calls = %d, want one", echoTool.calls)
	}
	if got := evidenceStringFromAny(result.trace.Arguments["plan_phase_id"]); got != "phase-next" {
		t.Fatalf("plan_phase_id = %q, want phase-next", got)
	}
}

func TestHandleProgressiveSkillCallDropsUnknownPhaseAssociationWithoutBlockingTool(t *testing.T) {
	runner, resolved, echoTool := newPlanPhaseGuardTestRunner(t)
	result := runner.handleProgressiveSkillCall(
		context.Background(),
		NewPreparedChat("conv-phase-stale", "msg-phase-stale", "", "auto", &adapter.ChatRequest{}),
		resolved,
		planPhaseGuardToolCall(t, "phase-from-old-plan"),
		skills.ExecutionContext{},
		0,
		map[string]int{},
		map[string]struct{}{"phase-guard-skill": {}},
		map[string]interface{}{"operation_plan": map[string]interface{}{"phases": []interface{}{
			map[string]interface{}{"id": "phase-current", "status": "in_progress"},
		}}},
		1,
		nil,
	)

	if result.recoverable || echoTool.calls != 1 {
		t.Fatalf("result = %#v calls=%d, want stale phase hint dropped and tool executed", result, echoTool.calls)
	}
	if got := evidenceStringFromAny(result.trace.Arguments["plan_phase_id"]); got != "" {
		t.Fatalf("plan_phase_id = %q, want stale association omitted", got)
	}
}

func TestHandleProgressiveSkillCallKeepsMixedPlanUnstructuredPhaseUsable(t *testing.T) {
	runner, resolved, echoTool := newPlanPhaseGuardTestRunner(t)
	result := runner.handleProgressiveSkillCall(
		context.Background(),
		NewPreparedChat("conv-phase-mixed", "msg-phase-mixed", "", "auto", &adapter.ChatRequest{}),
		resolved,
		planPhaseGuardToolCall(t, ""),
		skills.ExecutionContext{},
		0,
		map[string]int{},
		map[string]struct{}{"phase-guard-skill": {}},
		map[string]interface{}{"operation_plan": map[string]interface{}{"phases": []interface{}{
			map[string]interface{}{"id": "phase-current", "status": "in_progress"},
			map[string]interface{}{
				"id": "phase-future", "status": "pending",
				"expected_action": map[string]interface{}{"skill_id": "another-skill", "tool_name": "another_tool"},
			},
		}}},
		1,
		nil,
	)

	if result.recoverable || echoTool.calls != 1 {
		t.Fatalf("result = %#v calls=%d, want mixed plan call to execute", result, echoTool.calls)
	}
}

func newPlanPhaseGuardTestRunner(t *testing.T) (*Runner, *skills.ResolvedSkills, *runnerProtocolEchoTool) {
	t.Helper()
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, "phase-guard-skill", `---
name: phase-guard-skill
description: Test operation-plan phase guard.
when_to_use: Use for phase guard tests.
provider_type: builtin
provider_id: protocol_batch
runtime_type: tool
tools:
  - echo_value
---

# Phase Guard

Call echo_value once.
`)
	echoTool := &runnerProtocolEchoTool{}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(&runnerProtocolEchoProvider{tool: echoTool}); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	runtime := skills.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir)
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{"phase-guard-skill"})
	if err != nil {
		t.Fatalf("resolve skill: %v", err)
	}
	return &Runner{SkillRuntime: runtime}, resolved, echoTool
}

func planPhaseGuardToolCall(t *testing.T, phaseID string) adapter.ToolCall {
	t.Helper()
	payload := map[string]interface{}{
		"skill_id":  "phase-guard-skill",
		"tool_name": "echo_value",
		"arguments": map[string]interface{}{"value": "hello"},
	}
	if phaseID != "" {
		payload["plan_phase_id"] = phaseID
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return adapter.ToolCall{
		ID:   "call-phase-guard",
		Type: "function",
		Function: adapter.FunctionCall{
			Name:      skills.MetaToolCallSkillTool,
			Arguments: string(encoded),
		},
	}
}
