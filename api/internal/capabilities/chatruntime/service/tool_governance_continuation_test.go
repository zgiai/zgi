package service

import (
	"strings"
	"testing"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

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

func TestToolGovernanceFrozenContinuationNeedsSkillLoopForPendingOperationPlan(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"steps": []interface{}{
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_identity"),
						"status":    operationPlanStepStatusCompleted,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "update_agent_identity",
					},
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"),
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "update_agent_config",
					},
				},
				"step_status": map[string]interface{}{
					operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_identity"): operationPlanStepStatusCompleted,
					operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"):   operationPlanStepStatusPending,
				},
			},
		}},
	}

	if !toolGovernanceFrozenContinuationNeedsSkillLoop(prepared) {
		t.Fatal("toolGovernanceFrozenContinuationNeedsSkillLoop() = false, want true for pending operation plan step")
	}

	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	steps := mapSliceFromAny(plan["steps"])
	stepStatus := mapFromOperationContext(plan["step_status"])
	operationPlanSetStepStatus(steps, stepStatus, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"), operationPlanStepStatusCompleted)
	plan["steps"] = mapsToInterfaceSlice(steps)
	plan["step_status"] = stepStatus

	if !toolGovernanceFrozenContinuationNeedsSkillLoop(prepared) {
		t.Fatal("toolGovernanceFrozenContinuationNeedsSkillLoop() = false, want true so completed governed operations still pass post verification")
	}
}

func TestToolGovernanceFrozenFastPathUsesOperationPlanEvidence(t *testing.T) {
	deleteStepID := operationPlanToolStepID(skills.SkillAgentManagement, "delete_agent")
	updateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")
	prepared := &PreparedChat{
		parts: &chatRequestParts{Query: "delete then keep editing the agent"},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status": operationPlanStatusRunning,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        deleteStepID,
						"status":    operationPlanStepStatusCompleted,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "delete_agent",
					},
					map[string]interface{}{
						"id":        updateStepID,
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "update_agent_config",
					},
				},
				"step_status": map[string]interface{}{
					deleteStepID: operationPlanStepStatusCompleted,
					updateStepID: operationPlanStepStatusPending,
				},
				"pending_next_action": operationPlanToolStepTitle(skills.SkillAgentManagement, "update_agent_config"),
			},
		}},
	}

	answer, ok := toolGovernanceFrozenFastPathAnswer(prepared, skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "delete_agent",
		Status:   "success",
		Result: map[string]interface{}{
			"status":     "completed",
			"effect":     "deleted",
			"agent_id":   "agent-1",
			"agent_name": "Agent One",
		},
	})

	if ok {
		t.Fatalf("toolGovernanceFrozenFastPathAnswer() = (%q, true), want pending update step to keep skill loop running", answer)
	}
}

func TestToolGovernanceFrozenFastPathCoversSingleDeletePlanWithBatchResult(t *testing.T) {
	deleteStepID := operationPlanToolStepID(skills.SkillAgentManagement, "delete_agent")
	prepared := &PreparedChat{
		parts: &chatRequestParts{Query: "delete the first two visible agents"},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status": operationPlanStatusRunning,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        deleteStepID,
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "delete_agent",
					},
					map[string]interface{}{
						"id":     "observe",
						"title":  "Observe result",
						"status": operationPlanStepStatusPending,
					},
				},
				"step_status": map[string]interface{}{
					deleteStepID: operationPlanStepStatusPending,
					"observe":    operationPlanStepStatusPending,
				},
				"pending_next_action": operationPlanToolStepTitle(skills.SkillAgentManagement, "delete_agent"),
			},
		}},
	}
	trace := skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "delete_agents",
		Status:   "success",
		Result: map[string]interface{}{
			"status":         "completed",
			"effect":         "deleted",
			"operation_type": "agent.delete.batch",
			"target_count":   2,
			"deleted_count":  2,
			"failed_count":   0,
			"item_results": []interface{}{
				map[string]interface{}{"agent_id": "agent-1", "agent_name": "Agent One", "status": "succeeded"},
				map[string]interface{}{"agent_id": "agent-2", "agent_name": "Agent Two", "status": "succeeded"},
			},
			"operation_group": map[string]interface{}{
				"id":            "agent.delete.batch:test",
				"type":          "batch",
				"operation":     "agent.delete",
				"asset_type":    "agent",
				"status":        "completed",
				"target_count":  2,
				"success_count": 2,
				"failed_count":  0,
				"item_results": []interface{}{
					map[string]interface{}{"agent_id": "agent-1", "agent_name": "Agent One", "status": "succeeded"},
					map[string]interface{}{"agent_id": "agent-2", "agent_name": "Agent Two", "status": "succeeded"},
				},
			},
		},
	}
	ensureOperationPlanInvocationStep(prepared.Message.Metadata, skillInvocationFromTrace(trace, 0))

	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if got := stringFromAny(mapFromOperationContext(plan["step_status"])[deleteStepID]); got != operationPlanStepStatusCompleted {
		t.Fatalf("delete_agent step status = %q, want completed before fast path; plan=%#v", got, plan)
	}
	answer, ok := toolGovernanceFrozenFastPathAnswer(prepared, trace)
	if !ok {
		t.Fatalf("toolGovernanceFrozenFastPathAnswer() ok = false, want batch delete result to finish governed turn; plan=%#v", plan)
	}
	if !strings.Contains(answer, "成功删除 2 个智能体") ||
		!strings.Contains(answer, "Agent One") ||
		!strings.Contains(answer, "Agent Two") {
		t.Fatalf("answer = %q, want batch delete evidence summary", answer)
	}
}
