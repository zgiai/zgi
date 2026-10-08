package service

import (
	"strings"
	"testing"
	"time"

	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestApprovedBusinessResultSurvivesGovernanceActionProjection(t *testing.T) {
	invocation := &skills.ToolInvocationResult{Messages: []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"result": float64(42), "expression": "6*7", "internal_id": "private-id",
			"nested": map[string]interface{}{"correlation_id": "private-correlation", "value": "retained"},
		},
	}}}
	event := map[string]interface{}{"effect": "read", "asset_type": "number"}
	result := toolGovernanceModelVisibleToolResult(event, invocation, nil)
	details := mapFromOperationContext(result["details"])
	if details["result"] != float64(42) || details["expression"] != "6*7" || details["internal_id"] != nil || mapFromOperationContext(details["nested"])["correlation_id"] != nil {
		t.Fatalf("approved numeric result=%#v", result)
	}
	message := toolGovernanceFrozenExecutionContinuationMessage(&runtimemodel.Message{Query: "calculate", Metadata: map[string]interface{}{
		"skill_invocations": []map[string]interface{}{{"skill_id": "agent_execution", "tool_name": "shell_exec", "status": "awaiting_approval"}},
	}}, event, invocation, nil)
	if content := messageContentText(message.Content); !strings.Contains(content, `"result":42`) || strings.Contains(content, "private-correlation") || !strings.Contains(content, "not still running or awaiting another approval") {
		t.Fatalf("model did not receive sanitized result: %s", content)
	}
	event["asset_type"] = "file"
	if result := toolGovernanceModelVisibleToolResult(event, invocation, nil); result["details"] != nil {
		t.Fatalf("file projection expanded: %#v", result)
	}
	event["asset_type"] = "number"
	invocation.Messages[0].Data = map[string]interface{}{"large": strings.Repeat("x", 32<<10)}
	if result := toolGovernanceModelVisibleToolResult(event, invocation, nil); result["details"] != nil || result["details_truncated"] != true {
		t.Fatalf("oversized result was admitted: %#v", result)
	}
}

func TestPlatformRejectionDoesNotNegateOriginalShellExecution(t *testing.T) {
	message := &runtimemodel.Message{Metadata: map[string]interface{}{
		"skill_invocations": []map[string]interface{}{{"skill_id": "agent_execution", "tool_name": "shell_exec", "status": "awaiting_approval"}},
	}}
	request := toolGovernanceRejectionLLMRequest(message, runtimedto.ToolGovernanceDecisionRequest{Action: "reject"}, nil)
	if content := messageContentText(request.Messages[1].Content); !strings.Contains(content, "Do not claim the original Shell never started") || !strings.Contains(content, "Only the frozen business tool was rejected") {
		t.Fatalf("rejection lost stopped Shell state: %s", content)
	}
	message.Metadata = nil
	request = toolGovernanceRejectionLLMRequest(message, runtimedto.ToolGovernanceDecisionRequest{Action: "reject"}, nil)
	if strings.Contains(messageContentText(request.Messages[1].Content), "Platform CLI rejection state") {
		t.Fatal("ordinary approvals were reclassified as CLI")
	}
}

func TestApprovedOrdinaryGovernedSuccessDoesNotWriteExternalProviderSummary(t *testing.T) {
	frozen := toolgovernance.NewFrozenInvocation(toolgovernance.FrozenInvocationRequest{
		CorrelationID: "calculator-correlation", SkillID: skills.SkillCalculator, ToolName: "calculate",
		Arguments: map[string]interface{}{"operation": "add", "left": 1, "right": 2}, Now: time.Now(),
	})
	metadata := map[string]interface{}{"operation_plan": map[string]interface{}{
		"status": operationPlanStatusRunning,
		"phases": []interface{}{map[string]interface{}{
			"id": "phase-calculate", "step": "calculate", "status": "in_progress",
			"expected_action": map[string]interface{}{"skill_id": skills.SkillCalculator, "tool_name": "calculate"},
		}},
	}}
	bound := bindPendingGovernedInvocationToOperationPlan(metadata, map[string]interface{}{
		"frozen_invocation": frozen, "plan_phase_id": "phase-calculate",
	})
	completed, terminal := completeApprovedFrozenInvocationSuccess(bound, frozen, &skills.ToolInvocationResult{Trace: skills.SkillTrace{
		Status: "success", Result: map[string]interface{}{"value": 3},
	}})
	if !terminal {
		t.Fatal("ordinary exact governed invocation did not complete its only phase")
	}
	phase := mapSliceFromAny(mapFromOperationContext(completed["operation_plan"])["phases"])[0]
	if stringFromAny(phase["status"]) != operationPlanStepStatusCompleted {
		t.Fatalf("ordinary governed phase = %#v", phase)
	}
	if summary := mapFromOperationContext(completed["operation_result_summary"]); summary["provider_success_confirmed"] != nil || summary["operation_status"] != nil {
		t.Fatalf("ordinary governed invocation received external provider summary: %#v", summary)
	}
}

func TestEnsureFrozenInvocationSkillIDAddsRuntimeManagedSkill(t *testing.T) {
	got := ensureFrozenInvocationSkillID([]string{skills.SkillCalculator}, skills.SkillAgentManagement)
	if !skillIDEnabled(got, skills.SkillAgentManagement) {
		t.Fatalf("ensureFrozenInvocationSkillID() = %#v, want %s added", got, skills.SkillAgentManagement)
	}
	if !skillIDEnabled(got, skills.SkillCalculator) {
		t.Fatalf("ensureFrozenInvocationSkillID() = %#v, want existing skill preserved", got)
	}
}

func TestEnsureFrozenInvocationSkillIDPreservesExistingSkill(t *testing.T) {
	input := []string{skills.SkillAgentManagement, skills.SkillCalculator}
	got := ensureFrozenInvocationSkillID(input, skills.SkillAgentManagement)
	if len(got) != len(input) {
		t.Fatalf("ensureFrozenInvocationSkillID() length = %d, want %d", len(got), len(input))
	}
}

func TestToolGovernanceFrozenContinuationMessageIncludesTurnState(t *testing.T) {
	message := &runtimemodel.Message{
		Query: "create an agent from the file theme",
		Metadata: map[string]interface{}{
			"turn_state": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"kind":       "working_fact",
						"visibility": "model_only",
						"key":        "agent_theme",
						"value":      "water fee confirmation",
						"source":     "file-reader/read_file",
					},
				},
			},
		},
	}
	msg := toolGovernanceFrozenExecutionContinuationMessage(message, map[string]interface{}{}, nil, nil)
	content := strings.TrimSpace(stringFromAny(msg.Content))
	for _, want := range []string{
		"Current turn structured state JSON",
		"agent_theme",
		"water fee confirmation",
		"authoritative same-turn memory",
		"first model response after this continuation",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("continuation message missing %q in:\n%s", want, content)
		}
	}
}

func TestToolGovernanceFrozenContinuationMessageIncludesExecutionState(t *testing.T) {
	message := &runtimemodel.Message{
		Query: "create a test agent, then edit and verify it",
		Metadata: map[string]interface{}{
			"skill_invocations": []map[string]interface{}{
				{
					"kind":     "skill_load",
					"status":   "success",
					"skill_id": skills.SkillAgentManagement,
				},
				{
					"kind":      "tool_call",
					"status":    "success",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "create_agent",
					"arguments": map[string]interface{}{"name": "Smoke Agent"},
					"result": map[string]interface{}{
						"status":     "completed",
						"agent_id":   "agent-1",
						"agent_name": "Smoke Agent",
					},
				},
				{
					"kind":      "tool_call",
					"status":    "error",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_identity",
					"arguments": map[string]interface{}{"agent_id": "agent-1", "name": "Duplicate Agent"},
					"error":     "agent with the same name already exists",
				},
			},
		},
	}

	msg := toolGovernanceFrozenExecutionContinuationMessage(message, map[string]interface{}{}, nil, nil)
	content := strings.TrimSpace(stringFromAny(msg.Content))
	for _, want := range []string{
		"Current-turn execution state JSON",
		"active_target",
		"Smoke Agent",
		"failed_operations",
		"agent with the same name already exists",
		"do not create a replacement asset",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("continuation message missing %q in:\n%s", want, content)
		}
	}
}

func TestToolGovernanceFrozenExecutionContinuationKeepsProgressInUserLanguage(t *testing.T) {
	message := &runtimemodel.Message{
		Query: "\u521b\u5efa\u4e24\u4e2a\u6d4b\u8bd5 Agent",
		Metadata: map[string]interface{}{
			"operation_result_summary": map[string]interface{}{
				"status":        "completed",
				"skill_id":      skills.SkillAgentManagement,
				"tool_name":     "create_agent",
				"success_count": 1,
			},
		},
	}
	msg := toolGovernanceFrozenExecutionContinuationMessage(message, map[string]interface{}{}, nil, nil)
	content := messageContentText(msg.Content)
	for _, want := range []string{
		"All user-visible progress updates and final answers must use the user's language.",
		"If all requested work is complete, answer in the user's language.",
		"Authoritative operation result facts JSON",
		"\u521b\u5efa\u4e24\u4e2a\u6d4b\u8bd5 Agent",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("continuation message missing %q in %q", want, content)
		}
	}
}
