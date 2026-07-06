package skillloop

import (
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestFastPathFinalAnswerForToolTraceCoversAgentIdentityUpdate(t *testing.T) {
	answer, ok := FastPathFinalAnswerForToolTrace(skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_identity",
		Status:   "success",
		Result: map[string]interface{}{
			"status":         "completed",
			"effect":         "updated",
			"agent_name":     "客服智能体",
			"updated_fields": []interface{}{"description", "icon", "icon_type"},
		},
	})

	if !ok {
		t.Fatal("FastPathFinalAnswerForToolTrace() ok = false, want true for successful Agent identity update")
	}
	for _, want := range []string{"智能体「客服智能体」", "描述", "图标"} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want %q", answer, want)
		}
	}
	if strings.Contains(answer, "icon_type") {
		t.Fatalf("answer = %q, want user-facing field labels only", answer)
	}
}

func TestFastPathCompletionEvidenceWaitsForPostDeleteNavigationAndEdit(t *testing.T) {
	goal := "\u5e2e\u6211\u5220\u9664\u672c\u9875\u9762\u524d\u4e24\u4e2a\u667a\u80fd\u4f53\uff0c\u7136\u540e\u5728\u5220\u9664\u540e\u8fdb\u5165\u7b2c\u4e00\u4e2a\u667a\u80fd\u4f53\u7684\u8be6\u60c5\uff0c\u628a\u8fd9\u4e2a\u667a\u80fd\u4f53\u6539\u9020\u4e3a\u4e00\u4e2a\u5c0f\u8bf4\u521b\u4f5c\u667a\u80fd\u4f53"
	plan := map[string]interface{}{
		"status":              "running",
		"original_user_goal":  goal,
		"pending_next_action": "Navigate to page",
		"steps": []interface{}{
			map[string]interface{}{
				"id":        "tool:agent-management/delete_agents",
				"status":    "completed",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "delete_agents",
			},
			map[string]interface{}{
				"id":        "route:/console/agents/agent-3/agent",
				"status":    "pending",
				"skill_id":  skills.SkillConsoleNavigator,
				"tool_name": "navigate",
			},
			map[string]interface{}{
				"id":        "tool:agent-management/update_agent_config",
				"status":    "pending",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
				"wait_for":  "route:/console/agents/agent-3/agent",
			},
		},
		"step_status": map[string]interface{}{
			"tool:agent-management/delete_agents":       "completed",
			"route:/console/agents/agent-3/agent":       "pending",
			"tool:agent-management/update_agent_config": "pending",
		},
		"tool_result": map[string]interface{}{
			"kind":      "tool_call",
			"status":    "success",
			"skill_id":  skills.SkillAgentManagement,
			"tool_name": "delete_agents",
			"result_summary": map[string]interface{}{
				"status":        "completed",
				"target_count":  2,
				"deleted_count": 2,
				"item_results": []interface{}{
					map[string]interface{}{"status": "succeeded", "agent_name": "Agent One"},
					map[string]interface{}{"status": "succeeded", "agent_name": "Agent Two"},
				},
			},
		},
	}
	if answer, ok := FastPathFinalAnswerForCompletionEvidence(map[string]interface{}{
		"user_request":   goal,
		"operation_plan": plan,
	}); ok {
		t.Fatalf("FastPathFinalAnswerForCompletionEvidence() = (%q, true), want blocked until route/edit steps run", answer)
	}
}

func TestFastPathCompletionEvidenceSuppressesAutoFinalAnswerDuringClientActionContinuation(t *testing.T) {
	evidence := map[string]interface{}{
		"suppress_auto_final_answer_fast_path": true,
		"operation_plan": map[string]interface{}{
			"status":              "running",
			"pending_next_action": "asset_observation",
		},
		"operation_result_summary": map[string]interface{}{
			"latest_client_action": map[string]interface{}{
				"status":      "succeeded",
				"action_type": "route_navigation",
				"skill_id":    skills.SkillConsoleNavigator,
				"tool_name":   "navigate",
				"label":       "Agent detail",
				"result": map[string]interface{}{
					"loaded_href": "/console/agents/agent-1/agent",
				},
			},
		},
	}

	if answer, ok := FastPathFinalAnswerForCompletionEvidence(evidence); ok {
		t.Fatalf("FastPathFinalAnswerForCompletionEvidence() = (%q, true), want model continuation", answer)
	}
}

func TestFastPathToolTraceWithEvidenceSuppressesAutoFinalAnswerDuringClientActionContinuation(t *testing.T) {
	trace := skills.SkillTrace{
		Kind:     "tool_call",
		Status:   "success",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "delete_agent",
		Result: map[string]interface{}{
			"status":     "completed",
			"agent_name": "Old Agent",
			"deleted":    true,
		},
	}
	if _, ok := FastPathFinalAnswerForToolTrace(trace); !ok {
		t.Fatal("FastPathFinalAnswerForToolTrace() ok = false, test setup needs a fast-pathable trace")
	}

	if answer, ok := FastPathFinalAnswerForToolTraceWithEvidence(trace, map[string]interface{}{
		"suppress_auto_final_answer_fast_path": true,
	}); ok {
		t.Fatalf("FastPathFinalAnswerForToolTraceWithEvidence() = (%q, true), want model continuation", answer)
	}
}

func TestFastPathCompletionEvidenceWaitsForPendingStructuredPlanOperation(t *testing.T) {
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "running",
			"structured_plan": map[string]interface{}{
				"schema_version": "aichat.structured_plan.v1",
				"domain":         "agent_management",
				"intent":         "agent.batch_delete_then_edit",
				"operations": []interface{}{
					map[string]interface{}{
						"status":    "completed",
						"action":    "delete",
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "delete_agents",
					},
					map[string]interface{}{
						"status":    "pending",
						"action":    "navigate",
						"skill_id":  skills.SkillConsoleNavigator,
						"tool_name": "navigate",
					},
					map[string]interface{}{
						"status":    "pending",
						"action":    "update",
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "update_agent_config",
					},
				},
			},
			"tool_result": map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "delete_agents",
				"result_summary": map[string]interface{}{
					"status":         "completed",
					"operation_type": "agent.delete.batch",
					"target_count":   2,
					"deleted_count":  2,
					"item_results": []interface{}{
						map[string]interface{}{"status": "succeeded", "agent_name": "Agent One"},
						map[string]interface{}{"status": "succeeded", "agent_name": "Agent Two"},
					},
				},
			},
		},
	}

	if answer, ok := FastPathFinalAnswerForCompletionEvidence(evidence); ok {
		t.Fatalf("FastPathFinalAnswerForCompletionEvidence() = (%q, true), want blocked until structured operations finish", answer)
	}
}

func TestFastPathCompletionEvidenceAllowsCompletedStructuredPlanOperation(t *testing.T) {
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "completed",
			"structured_plan": map[string]interface{}{
				"schema_version": "aichat.structured_plan.v1",
				"domain":         "agent_management",
				"intent":         "agent.batch_delete",
				"operations": []interface{}{
					map[string]interface{}{
						"status":    "completed",
						"action":    "delete",
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "delete_agents",
					},
				},
			},
			"tool_result": map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "delete_agents",
				"result_summary": map[string]interface{}{
					"status":         "completed",
					"operation_type": "agent.delete.batch",
					"target_count":   1,
					"deleted_count":  1,
					"item_results": []interface{}{
						map[string]interface{}{"status": "succeeded", "agent_name": "Agent One"},
					},
				},
			},
		},
	}

	answer, ok := FastPathFinalAnswerForCompletionEvidence(evidence)
	if !ok {
		t.Fatal("FastPathFinalAnswerForCompletionEvidence() ok = false, want completed structured operation to close")
	}
	if !strings.Contains(answer, "1") {
		t.Fatalf("answer = %q, want completed delete summary", answer)
	}
}

func TestFastPathCompletionEvidenceUsesIdentityUpdateAfterGetAgentRead(t *testing.T) {
	answer, ok := FastPathFinalAnswerForCompletionEvidence(map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"original_user_goal": "回归验证-Agent基础信息编辑-1782893696751：把名称改为 GOAL-CONFIG-AGENT-1782893696751-ID，描述改为 identity smoke 1782893696751，图标改成蓝色爱心。审批通过后请确认当前页面顶部名称、描述和图标已更新。",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_identity",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_identity",
				},
				map[string]interface{}{
					"id":        "tool:agent-management/get_agent",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "get_agent",
				},
			},
		},
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_identity",
				"status":    "success",
				"result": map[string]interface{}{
					"status":         "completed",
					"effect":         "updated",
					"agent_name":     "GOAL-CONFIG-AGENT-1782893696751-ID",
					"updated_fields": []interface{}{"name", "description", "icon"},
				},
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent",
				"status":    "success",
				"result": map[string]interface{}{
					"status":      "completed",
					"agent_name":  "GOAL-CONFIG-AGENT-1782893696751-ID",
					"description": "identity smoke 1782893696751",
					"icon":        "💙",
				},
			},
		},
	})

	if !ok {
		t.Fatal("FastPathFinalAnswerForCompletionEvidence() ok = false, want identity update verified by get_agent")
	}
	for _, want := range []string{
		"智能体「GOAL-CONFIG-AGENT-1782893696751-ID」",
		"名称",
		"描述",
		"图标",
		"已在更新后重新读取配置并完成确认",
	} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want %q", answer, want)
		}
	}
	if strings.Contains(answer, "1782893609487") || strings.Contains(answer, "上一轮") {
		t.Fatalf("answer = %q, want no stale history reference", answer)
	}
}

func TestFastPathCompletionEvidenceWaitsWhenConfigUpdateStillPendingAfterIdentity(t *testing.T) {
	_, ok := FastPathFinalAnswerForCompletionEvidence(map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":             "running",
			"original_user_goal": "Rename the current Agent, update the runtime config, then read config again to verify.",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/get_agent_config",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "get_agent_config",
				},
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_identity",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_identity",
				},
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_config",
					"status":    "pending",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_config",
				},
				map[string]interface{}{
					"id":        "tool:agent-management/get_agent_config#post_update",
					"phase":     "post_update_verification",
					"status":    "pending",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "get_agent_config",
				},
			},
			"step_status": map[string]interface{}{
				"tool:agent-management/get_agent_config":             "completed",
				"tool:agent-management/update_agent_identity":        "completed",
				"tool:agent-management/update_agent_config":          "pending",
				"tool:agent-management/get_agent_config#post_update": "pending",
			},
		},
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
				"result": map[string]interface{}{
					"status":     "completed",
					"agent_id":   "agent-1",
					"agent_name": "Support Agent",
				},
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_identity",
				"result": map[string]interface{}{
					"status":         "completed",
					"effect":         "updated",
					"agent_id":       "agent-1",
					"agent_name":     "Support Agent Edited",
					"updated_fields": []interface{}{"name", "description", "icon"},
				},
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent",
				"result": map[string]interface{}{
					"status":      "completed",
					"agent_id":    "agent-1",
					"agent_name":  "Support Agent Edited",
					"description": "identity updated but config still pending",
				},
			},
		},
	})
	if ok {
		t.Fatal("FastPathFinalAnswerForCompletionEvidence() ok = true, want false while update_agent_config remains pending")
	}
}

func TestFastPathPostUpdateReadMustFollowLatestAgentMutation(t *testing.T) {
	evidence := map[string]interface{}{
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_identity",
				"result": map[string]interface{}{
					"status":         "completed",
					"agent_name":     "Support Agent Edited",
					"updated_fields": []interface{}{"name"},
				},
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent",
				"result": map[string]interface{}{
					"status":     "completed",
					"agent_name": "Support Agent Edited",
				},
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
				"result": map[string]interface{}{
					"status":         "completed",
					"agent_name":     "Support Agent Edited",
					"updated_fields": []interface{}{"system_prompt"},
				},
			},
		},
	}
	if fastPathHasSuccessfulAgentConfigReadAfterUpdate(evidence) {
		t.Fatal("fastPathHasSuccessfulAgentConfigReadAfterUpdate() = true, want false without a read after the latest update_agent_config")
	}
}

func TestFastPathPostUpdateReadIgnoresApprovedMutationStatus(t *testing.T) {
	evidence := map[string]interface{}{
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "approved",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
				"result": map[string]interface{}{
					"status":         "approved",
					"agent_name":     "Support Agent",
					"updated_fields": []interface{}{"system_prompt"},
				},
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
				"result": map[string]interface{}{
					"status":     "completed",
					"agent_name": "Support Agent",
				},
			},
		},
	}
	if fastPathHasSuccessfulAgentConfigReadAfterUpdate(evidence) {
		t.Fatal("fastPathHasSuccessfulAgentConfigReadAfterUpdate() = true, want false when the only mutation status is approved")
	}
}

func TestFastPathTraceWithToolResultKeepsAgentBindingNames(t *testing.T) {
	trace := skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Status:   "success",
		Result: map[string]interface{}{
			"status":         "completed",
			"agent_name":     "Support Agent",
			"binding_kind":   "agent_skill",
			"change_action":  "bind",
			"resource_count": 1,
		},
	}
	toolResult := map[string]interface{}{
		"status":     "completed",
		"agent_name": "Support Agent",
		"binding_changes": []map[string]interface{}{
			{
				"field":                "enabled_skill_ids",
				"binding_kind":         "agent_skill",
				"change_action":        "bind",
				"resource_count":       1,
				"added_resource_count": 1,
				"resource_names":       []string{"Chart Generator"},
			},
		},
	}

	answer, ok := FastPathFinalAnswerForToolTrace(fastPathTraceWithToolResult(trace, toolResult))
	if !ok {
		t.Fatal("FastPathFinalAnswerForToolTrace() ok = false, want true for successful Agent config update")
	}
	for _, want := range []string{"Support Agent", "Chart Generator"} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want %q", answer, want)
		}
	}
}

func TestFastPathFinalAnswerForAgentConfigUpdateDoesNotClaimPreservedSkillBinding(t *testing.T) {
	answer, ok := FastPathFinalAnswerForToolTrace(skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Status:   "success",
		Result: map[string]interface{}{
			"status":     "completed",
			"agent_name": "Support Agent",
			"updated_fields": []interface{}{
				"enabled_skill_ids",
				"knowledge_dataset_ids",
				"database_bindings",
				"workflow_bindings",
			},
			"binding_changes": []interface{}{
				map[string]interface{}{
					"field":                "knowledge_dataset_ids",
					"binding_kind":         "knowledge_base",
					"change_action":        "bind",
					"resource_count":       1,
					"added_resource_count": 1,
					"resource_names":       []interface{}{"KB Two"},
				},
				map[string]interface{}{
					"field":                "database_bindings",
					"binding_kind":         "database_table",
					"change_action":        "bind",
					"resource_count":       1,
					"added_resource_count": 1,
					"resource_names":       []interface{}{"orders"},
				},
				map[string]interface{}{
					"field":                "workflow_bindings",
					"binding_kind":         "workflow",
					"change_action":        "bind",
					"resource_count":       1,
					"added_resource_count": 1,
					"resource_names":       []interface{}{"Approval Flow"},
				},
			},
		},
	})
	if !ok {
		t.Fatal("FastPathFinalAnswerForToolTrace() ok = false, want true")
	}
	for _, want := range []string{"Support Agent", "KB Two", "orders", "Approval Flow"} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want resource name %q", answer, want)
		}
	}
	for _, unwanted := range []string{"绑定 1 个技能", "更新技能", "skill"} {
		if strings.Contains(answer, unwanted) {
			t.Fatalf("answer = %q, want no preserved skill binding claim %q", answer, unwanted)
		}
	}
}

func TestFastPathFinalAnswerForCompletionEvidenceKeepsAgentBindingNames(t *testing.T) {
	resultSummary := map[string]interface{}{
		"status":     "completed",
		"agent_name": "Support Agent",
		"updated_fields": []interface{}{
			"enabled_skill_ids",
			"knowledge_dataset_ids",
			"database_bindings",
			"workflow_bindings",
		},
		"binding_changes": []interface{}{
			map[string]interface{}{
				"field":                "knowledge_dataset_ids",
				"binding_kind":         "knowledge_base",
				"change_action":        "bind",
				"resource_count":       1,
				"added_resource_count": 1,
				"resource_names":       []interface{}{"KB Two"},
			},
			map[string]interface{}{
				"field":                "database_bindings",
				"binding_kind":         "database_table",
				"change_action":        "bind",
				"resource_count":       1,
				"added_resource_count": 1,
				"resource_names":       []interface{}{"orders"},
			},
			map[string]interface{}{
				"field":                "workflow_bindings",
				"binding_kind":         "workflow",
				"change_action":        "bind",
				"resource_count":       1,
				"added_resource_count": 1,
				"resource_names":       []interface{}{"Approval Flow"},
			},
		},
	}
	answer, ok := FastPathFinalAnswerForCompletionEvidence(map[string]interface{}{
		"operation_result_summary": map[string]interface{}{
			"latest_tool_result": map[string]interface{}{
				"status":         "success",
				"skill_id":       skills.SkillAgentManagement,
				"tool_name":      "update_agent_config",
				"result_summary": resultSummary,
			},
		},
	})
	if !ok {
		t.Fatal("FastPathFinalAnswerForCompletionEvidence() ok = false, want true")
	}
	for _, want := range []string{"Support Agent", "KB Two", "orders", "Approval Flow"} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want resource name %q", answer, want)
		}
	}
	for _, unwanted := range []string{"绑定 1 个技能", "更新技能", "skill"} {
		if strings.Contains(answer, unwanted) {
			t.Fatalf("answer = %q, want no preserved skill binding claim %q", answer, unwanted)
		}
	}
}

func TestFastPathFinalAnswerForCompletionEvidenceUsesOperationPlanToolResult(t *testing.T) {
	resultSummary := map[string]interface{}{
		"status":     "completed",
		"agent_name": "Support Agent",
		"updated_fields": []interface{}{
			"enabled_skill_ids",
			"knowledge_dataset_ids",
			"database_bindings",
			"workflow_bindings",
		},
		"binding_changes": []interface{}{
			map[string]interface{}{
				"field":                "knowledge_dataset_ids",
				"binding_kind":         "knowledge_base",
				"change_action":        "bind",
				"resource_count":       1,
				"added_resource_count": 1,
				"resource_names":       []interface{}{"KB Two"},
			},
			map[string]interface{}{
				"field":                "database_bindings",
				"binding_kind":         "database_table",
				"change_action":        "bind",
				"resource_count":       1,
				"added_resource_count": 1,
				"resource_names":       []interface{}{"orders"},
			},
			map[string]interface{}{
				"field":                "workflow_bindings",
				"binding_kind":         "workflow",
				"change_action":        "bind",
				"resource_count":       1,
				"added_resource_count": 1,
				"resource_names":       []interface{}{"Approval Flow"},
			},
		},
	}
	answer, ok := FastPathFinalAnswerForCompletionEvidence(map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "completed",
			"tool_result": map[string]interface{}{
				"status":         "success",
				"skill_id":       skills.SkillAgentManagement,
				"tool_name":      "update_agent_config",
				"result_summary": resultSummary,
			},
		},
	})
	if !ok {
		t.Fatal("FastPathFinalAnswerForCompletionEvidence() ok = false, want true")
	}
	for _, want := range []string{"Support Agent", "KB Two", "orders", "Approval Flow"} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want resource name %q", answer, want)
		}
	}
	for _, unwanted := range []string{"skill", "Skill"} {
		if strings.Contains(answer, unwanted) {
			t.Fatalf("answer = %q, want no preserved skill binding claim %q", answer, unwanted)
		}
	}
}

func TestFastPathFinalAnswerForCompletionEvidenceAggregatesAgentIdentityAndConfig(t *testing.T) {
	answer, ok := FastPathFinalAnswerForCompletionEvidence(map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":             "completed",
			"original_user_goal": "rename current Agent and update its homepage title",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_identity",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_identity",
				},
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_config",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_config",
				},
			},
			"step_status": map[string]interface{}{
				"tool:agent-management/update_agent_identity": "completed",
				"tool:agent-management/update_agent_config":   "completed",
			},
		},
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_identity",
				"result": map[string]interface{}{
					"status":         "completed",
					"effect":         "updated",
					"agent_id":       "agent-1",
					"agent_name":     "Support Agent Edited",
					"updated_fields": []interface{}{"name", "description", "icon"},
				},
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
				"result": map[string]interface{}{
					"status":         "completed",
					"agent_id":       "agent-1",
					"agent_name":     "Support Agent Edited",
					"updated_fields": []interface{}{"home_title"},
					"home_title":     "Welcome Home",
				},
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
				"result": map[string]interface{}{
					"status":     "completed",
					"agent_id":   "agent-1",
					"agent_name": "Support Agent Edited",
					"config": map[string]interface{}{
						"home_title": "Welcome Home",
					},
				},
			},
		},
	})

	if !ok {
		t.Fatal("FastPathFinalAnswerForCompletionEvidence() ok = false, want aggregate answer for completed identity and config mutations")
	}
	for _, want := range []string{
		"Support Agent Edited",
		"\u57fa\u7840\u4fe1\u606f",
		"\u8fd0\u884c\u914d\u7f6e",
		"\u540d\u79f0",
		"\u9996\u9875\u6807\u9898",
		"Welcome Home",
	} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want %q", answer, want)
		}
	}
}

func TestFastPathFinalAnswerForCompletionEvidenceWaitsForRequestedPostUpdateConfigRead(t *testing.T) {
	resultSummary := map[string]interface{}{
		"status":     "completed",
		"agent_name": "Support Agent",
		"updated_fields": []interface{}{
			"enabled_skill_ids",
		},
		"binding_changes": []interface{}{
			map[string]interface{}{
				"field":                  "enabled_skill_ids",
				"binding_kind":           "agent_skill",
				"change_action":          "unbind",
				"resource_count":         1,
				"removed_resource_count": 1,
				"resource_names":         []interface{}{"Chart Generator"},
				"final_resource_count":   1,
				"final_resource_names":   []interface{}{"Architecture Diagram Generator"},
			},
		},
	}
	_, ok := FastPathFinalAnswerForCompletionEvidence(map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":             "running",
			"original_user_goal": "Unbind Chart Generator, then read config again after completion and verify the remaining bindings.",
			"tool_result": map[string]interface{}{
				"status":         "success",
				"skill_id":       skills.SkillAgentManagement,
				"tool_name":      "update_agent_config",
				"result_summary": resultSummary,
			},
		},
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
			},
		},
	})
	if ok {
		t.Fatal("FastPathFinalAnswerForCompletionEvidence() ok = true, want false until requested post-update get_agent_config succeeds")
	}
}

func TestFastPathFinalAnswerForCompletionEvidenceWaitsForPendingPostUpdateReadStep(t *testing.T) {
	resultSummary := map[string]interface{}{
		"status":     "completed",
		"agent_name": "Support Agent",
		"updated_fields": []interface{}{
			"knowledge_dataset_ids",
			"database_bindings",
		},
		"binding_changes": []interface{}{
			map[string]interface{}{
				"field":                  "knowledge_dataset_ids",
				"binding_kind":           "knowledge_base",
				"change_action":          "unbind",
				"resource_count":         1,
				"removed_resource_count": 1,
				"resource_names":         []interface{}{"KB Two"},
			},
			map[string]interface{}{
				"field":                  "database_bindings",
				"binding_kind":           "database_table",
				"change_action":          "unbind",
				"resource_count":         1,
				"removed_resource_count": 1,
				"resource_names":         []interface{}{"orders"},
			},
		},
	}
	_, ok := FastPathFinalAnswerForCompletionEvidence(map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "running",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_config",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_config",
				},
				map[string]interface{}{
					"id":        "tool:agent-management/get_agent_config#post_update",
					"status":    "pending",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "get_agent_config",
				},
			},
			"tool_result": map[string]interface{}{
				"status":         "success",
				"skill_id":       skills.SkillAgentManagement,
				"tool_name":      "update_agent_config",
				"result_summary": resultSummary,
			},
		},
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
				"result":    resultSummary,
			},
		},
	})
	if ok {
		t.Fatal("FastPathFinalAnswerForCompletionEvidence() ok = true, want false while post-update get_agent_config plan step is pending")
	}
}

func TestFastPathFinalAnswerForCompletionEvidenceDedupesExecutionLedgerBeforePostRead(t *testing.T) {
	preRead := map[string]interface{}{
		"kind":       "tool_call",
		"runtime_id": "tool_call:agent-management:get_agent_config::#1",
		"status":     "success",
		"skill_id":   skills.SkillAgentManagement,
		"tool_name":  "get_agent_config",
		"result": map[string]interface{}{
			"status":     "completed",
			"agent_id":   "agent-1",
			"agent_name": "Support Agent",
		},
	}
	update := map[string]interface{}{
		"kind":       "tool_call",
		"runtime_id": "tool_call:agent-management:update_agent_config::#1",
		"status":     "success",
		"skill_id":   skills.SkillAgentManagement,
		"tool_name":  "update_agent_config",
		"result": map[string]interface{}{
			"status":         "completed",
			"agent_id":       "agent-1",
			"agent_name":     "Support Agent",
			"updated_fields": []interface{}{"knowledge_dataset_ids"},
			"binding_changes": []interface{}{
				map[string]interface{}{
					"field":         "knowledge_dataset_ids",
					"binding_kind":  "knowledge_base",
					"change_action": "bind",
				},
			},
		},
	}
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"original_user_goal": "Bind a knowledge base. After completion read the agent config again.",
			"status":             "running",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/get_agent_config",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "get_agent_config",
				},
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_config",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_config",
				},
				map[string]interface{}{
					"id":                                "tool:agent-management/get_agent_config#post_update",
					"status":                            "pending",
					"skill_id":                          skills.SkillAgentManagement,
					"tool_name":                         "get_agent_config",
					"required_post_update_verification": true,
				},
			},
			"step_status": map[string]interface{}{
				"tool:agent-management/get_agent_config":             "completed",
				"tool:agent-management/update_agent_config":          "completed",
				"tool:agent-management/get_agent_config#post_update": "pending",
			},
		},
		"skill_invocations": []interface{}{preRead, update},
		"execution_ledger": map[string]interface{}{
			"skill_invocations": []interface{}{preRead, update},
		},
	}

	if answer, ok := FastPathFinalAnswerForCompletionEvidence(evidence); ok {
		t.Fatalf("FastPathFinalAnswerForCompletionEvidence() = (%q, true), want blocked until real post-update read", answer)
	}
}

func TestCompletionEvidenceForFastPathDedupesExistingCallsBeforePostUpdateRead(t *testing.T) {
	preReadResult := map[string]interface{}{
		"status":     "completed",
		"agent_id":   "agent-1",
		"agent_name": "Support Agent",
	}
	updateResult := map[string]interface{}{
		"status":         "completed",
		"agent_id":       "agent-1",
		"agent_name":     "Support Agent",
		"updated_fields": []interface{}{"knowledge_dataset_ids"},
		"binding_changes": []interface{}{
			map[string]interface{}{
				"field":         "knowledge_dataset_ids",
				"binding_kind":  "knowledge_base",
				"change_action": "bind",
			},
		},
	}
	evidence := completionEvidenceForFastPathWithSuccessfulToolCalls(RunRequest{
		CompletionEvidence: func() map[string]interface{} {
			return map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"original_user_goal": "Bind a knowledge base. After completion read the agent config again.",
					"steps": []interface{}{
						map[string]interface{}{
							"id":        "tool:agent-management/get_agent_config",
							"status":    "completed",
							"skill_id":  skills.SkillAgentManagement,
							"tool_name": "get_agent_config",
						},
						map[string]interface{}{
							"id":        "tool:agent-management/update_agent_config",
							"status":    "completed",
							"skill_id":  skills.SkillAgentManagement,
							"tool_name": "update_agent_config",
						},
						map[string]interface{}{
							"id":                                "tool:agent-management/get_agent_config#post_update",
							"status":                            "pending",
							"skill_id":                          skills.SkillAgentManagement,
							"tool_name":                         "get_agent_config",
							"required_post_update_verification": true,
						},
					},
				},
				"skill_invocations": []interface{}{
					map[string]interface{}{
						"kind":      "tool_call",
						"status":    "success",
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "get_agent_config",
						"result":    preReadResult,
					},
					map[string]interface{}{
						"kind":      "tool_call",
						"status":    "success",
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "update_agent_config",
						"result":    updateResult,
					},
				},
			}
		},
	}, []SkillToolCallRef{
		{SkillID: skills.SkillAgentManagement, ToolName: "get_agent_config", Result: preReadResult},
		{SkillID: skills.SkillAgentManagement, ToolName: "update_agent_config", Result: updateResult},
	})

	if got := len(evidenceSliceFromAny(evidence["skill_invocations"])); got != 2 {
		t.Fatalf("skill_invocations len = %d, want deduped 2; evidence=%#v", got, evidence["skill_invocations"])
	}
	if answer, ok := FastPathFinalAnswerForCompletionEvidence(evidence); ok {
		t.Fatalf("FastPathFinalAnswerForCompletionEvidence() = (%q, true), want blocked until actual post-update read", answer)
	}
}

func TestFastPathFinalAnswerWithEvidenceBlocksAgentConfigUpdateWhenConfigReadStepPending(t *testing.T) {
	_, ok := FastPathFinalAnswerForToolTraceWithEvidence(skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Status:   "success",
		Result: map[string]interface{}{
			"status":     "completed",
			"agent_name": "Support Agent",
			"updated_fields": []interface{}{
				"enabled_skill_ids",
			},
			"binding_changes": []interface{}{
				map[string]interface{}{
					"field":          "enabled_skill_ids",
					"binding_kind":   "agent_skill",
					"change_action":  "bind",
					"resource_count": 1,
					"resource_names": []interface{}{"Chart Generator"},
				},
			},
		},
	}, map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":             "running",
			"original_user_goal": "\u6211\u7684\u56de\u7b54\uff1a\n1. \u662f\u5426\u786e\u8ba4\u5c06 Skill\u300c\u56fe\u8868\u751f\u6210\u5668\u300d\u7ed1\u5b9a\u5230\u667a\u80fd\u4f53 Support Agent\uff1f: \u786e\u8ba4\uff0c\u6267\u884c\u7ed1\u5b9a",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/get_agent_config",
					"status":    "pending",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "get_agent_config",
				},
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_config",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_config",
				},
			},
			"step_status": map[string]interface{}{
				"tool:agent-management/get_agent_config":    "pending",
				"tool:agent-management/update_agent_config": "completed",
			},
		},
	})
	if ok {
		t.Fatal("FastPathFinalAnswerForToolTraceWithEvidence() ok = true, want false while get_agent_config verification step is pending")
	}
}

func TestFastPathFinalAnswerWithEvidenceDoesNotTreatModelDecidesPlanStepAsHardBlock(t *testing.T) {
	answer, ok := FastPathFinalAnswerForToolTraceWithEvidence(skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Status:   "success",
		Result: map[string]interface{}{
			"status":     "completed",
			"agent_name": "Support Agent",
			"updated_fields": []interface{}{
				"enabled_skill_ids",
			},
			"binding_changes": []interface{}{
				map[string]interface{}{
					"field":          "enabled_skill_ids",
					"binding_kind":   "agent_skill",
					"change_action":  "bind",
					"resource_count": 1,
					"resource_names": []interface{}{"File Generator"},
				},
			},
		},
	}, map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":           "running",
			"tool_choice_mode": operationPlanToolChoiceModelDecides,
			"planning_mode":    "phase_only_model_decides",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/get_agent_config",
					"status":    "pending",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "get_agent_config",
				},
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_config",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_config",
				},
			},
			"step_status": map[string]interface{}{
				"tool:agent-management/get_agent_config":    "pending",
				"tool:agent-management/update_agent_config": "completed",
			},
		},
	})
	if !ok {
		t.Fatal("FastPathFinalAnswerForToolTraceWithEvidence() ok = false, want model-decides plan hints not to hard-block an evidence-grounded answer")
	}
	if !strings.Contains(answer, "Support Agent") {
		t.Fatalf("answer = %q, want Agent name", answer)
	}
}

func TestFastPathFinalAnswerWithEvidenceWaitsForModelDecidesPostUpdateRead(t *testing.T) {
	_, ok := FastPathFinalAnswerForToolTraceWithEvidence(skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Status:   "success",
		Result: map[string]interface{}{
			"status":     "completed",
			"agent_name": "Support Agent",
			"updated_fields": []interface{}{
				"enabled_skill_ids",
			},
			"binding_changes": []interface{}{
				map[string]interface{}{
					"field":          "enabled_skill_ids",
					"binding_kind":   "agent_skill",
					"change_action":  "bind",
					"resource_count": 1,
					"resource_names": []interface{}{"File Generator"},
				},
			},
		},
	}, map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":           "running",
			"tool_choice_mode": operationPlanToolChoiceModelDecides,
			"planning_mode":    "phase_only_model_decides",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_config",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_config",
				},
				map[string]interface{}{
					"id":                                "tool:agent-management/get_agent_config#post_update",
					"status":                            "pending",
					"skill_id":                          skills.SkillAgentManagement,
					"tool_name":                         "get_agent_config",
					"phase":                             "post_update_verification",
					"required_post_update_verification": true,
				},
			},
			"step_status": map[string]interface{}{
				"tool:agent-management/update_agent_config":          "completed",
				"tool:agent-management/get_agent_config#post_update": "pending",
			},
		},
	})
	if ok {
		t.Fatal("FastPathFinalAnswerForToolTraceWithEvidence() ok = true, want false while model-decides post-update get_agent_config is pending")
	}
}

func TestFastPathFinalAnswerWithEvidenceWaitsForModelDecidesPendingAgentMutation(t *testing.T) {
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":              "running",
			"tool_choice_mode":    operationPlanToolChoiceModelDecides,
			"planning_mode":       "phase_only_model_decides",
			"pending_next_action": "Run tool:agent-management/create_agent",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/delete_agent",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "delete_agent",
				},
				map[string]interface{}{
					"id":        "tool:agent-management/create_agent",
					"status":    "pending",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "create_agent",
				},
			},
			"step_status": map[string]interface{}{
				"tool:agent-management/delete_agent": "completed",
				"tool:agent-management/create_agent": "pending",
			},
		},
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "delete_agent",
				"result": map[string]interface{}{
					"status":     "completed",
					"effect":     "deleted",
					"agent_id":   "agent-1",
					"agent_name": "Old Agent",
				},
			},
		},
	}
	trace := skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "delete_agent",
		Status:   "success",
		Result: map[string]interface{}{
			"status":     "completed",
			"effect":     "deleted",
			"agent_id":   "agent-1",
			"agent_name": "Old Agent",
		},
	}

	if answer, ok := FastPathFinalAnswerForToolTraceWithEvidence(trace, evidence); ok {
		t.Fatalf("FastPathFinalAnswerForToolTraceWithEvidence() = (%q, true), want false while model-decides create_agent is pending", answer)
	}
	if answer, ok := FastPathFinalAnswerForCompletionEvidence(evidence); ok {
		t.Fatalf("FastPathFinalAnswerForCompletionEvidence() = (%q, true), want false while model-decides create_agent is pending", answer)
	}
}

func TestFastPathFinalAnswerWithEvidenceWaitsForModelDecidesCapabilityGoalsAfterClientCreate(t *testing.T) {
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":           "running",
			"tool_choice_mode": operationPlanToolChoiceModelDecides,
			"planning_mode":    "phase_only_model_decides",
			"capability_goals": []interface{}{
				map[string]interface{}{
					"capability_id":          "agent.model_selection",
					"goal_action":            "update",
					"required_config_fields": []interface{}{"model_provider", "model"},
					"verify_by":              []interface{}{"read get_agent_config after updates"},
				},
				map[string]interface{}{
					"capability_id":   "agent.skill_backed_capability",
					"goal_action":     "enable",
					"candidate_tool":  "list_agent_skill_candidates",
					"candidate_query": "file generation",
					"required_binding_actions": map[string]interface{}{
						"enabled_skill_ids": "bind",
					},
					"verify_by": []interface{}{"read get_agent_config after updates"},
				},
			},
		},
		"client_actions": []interface{}{
			map[string]interface{}{
				"kind":      "client_action",
				"status":    "succeeded",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "create_agent",
				"result": map[string]interface{}{
					"agent_id":   "agent-new",
					"agent_name": "AICHAT-PHASE-SMOKE",
					"status":     "completed",
				},
			},
		},
	}

	if answer, ok := FastPathFinalAnswerForCompletionEvidence(evidence); ok {
		t.Fatalf("FastPathFinalAnswerForCompletionEvidence() = (%q, true), want false while model-decides capability goals still need configuration and verification", answer)
	}
}

func TestFastPathFinalAnswerWithEvidenceBlocksAgentIdentityUpdateWhenConfigReadStepPending(t *testing.T) {
	_, ok := FastPathFinalAnswerForToolTraceWithEvidence(skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_identity",
		Status:   "success",
		Result: map[string]interface{}{
			"status":     "completed",
			"effect":     "updated",
			"agent_name": "Support Agent",
			"agent_id":   "agent-1",
			"agent_icon": `{"icon":"ID","icon_background":"#0f766e"}`,
			"updated_fields": []interface{}{
				"name",
				"description",
				"icon_type",
				"icon",
				"icon_background",
			},
		},
	}, map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":             "running",
			"original_user_goal": "Rename the current Agent, then read/observe it again.",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/get_agent_config",
					"status":    "pending",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "get_agent_config",
				},
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_identity",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_identity",
				},
			},
			"step_status": map[string]interface{}{
				"tool:agent-management/get_agent_config":      "pending",
				"tool:agent-management/update_agent_identity": "completed",
			},
		},
	})
	if ok {
		t.Fatal("FastPathFinalAnswerForToolTraceWithEvidence() ok = true, want false while identity update verification read is pending")
	}
}

func TestFastPathFinalAnswerWithEvidenceBlocksAgentConfigUpdateWhenGoalRequiresChinesePostRead(t *testing.T) {
	_, ok := FastPathFinalAnswerForToolTraceWithEvidence(skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Status:   "success",
		Result: map[string]interface{}{
			"status":     "completed",
			"agent_name": "Support Agent",
			"updated_fields": []interface{}{
				"enabled_skill_ids",
			},
			"binding_changes": []interface{}{
				map[string]interface{}{
					"field":                  "enabled_skill_ids",
					"binding_kind":           "agent_skill",
					"change_action":          "unbind",
					"resource_count":         1,
					"removed_resource_count": 1,
					"resource_names":         []interface{}{"Chart Generator"},
				},
			},
		},
	}, map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":             "running",
			"original_user_goal": "\u66f4\u65b0\u5b8c\u6210\u540e\u5fc5\u987b\u518d\u6b21\u8bfb\u53d6\u8be5\u667a\u80fd\u4f53\u914d\u7f6e\u9a8c\u8bc1\uff0c\u5e76\u5728\u6700\u7ec8\u56de\u7b54\u91cc\u8bf4\u660e\u590d\u8bfb\u914d\u7f6e\u540e\u5b83\u662f\u5426\u4ecd\u5904\u4e8e\u5df2\u7ed1\u5b9a\u72b6\u6001\u3002",
		},
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
			},
		},
	})
	if ok {
		t.Fatal("FastPathFinalAnswerForToolTraceWithEvidence() ok = true, want false until the requested Chinese post-update config read succeeds")
	}
}

func TestFastPathFinalAnswerForCompletionEvidenceAllowsRequestedPostUpdateConfigReadWhenObserved(t *testing.T) {
	resultSummary := map[string]interface{}{
		"status":     "completed",
		"agent_name": "Support Agent",
		"updated_fields": []interface{}{
			"enabled_skill_ids",
		},
		"binding_changes": []interface{}{
			map[string]interface{}{
				"field":                  "enabled_skill_ids",
				"binding_kind":           "agent_skill",
				"change_action":          "unbind",
				"resource_count":         1,
				"removed_resource_count": 1,
				"resource_names":         []interface{}{"Chart Generator"},
				"final_resource_count":   1,
				"final_resource_names":   []interface{}{"Architecture Diagram Generator"},
			},
		},
	}
	answer, ok := FastPathFinalAnswerForCompletionEvidence(map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":             "completed",
			"original_user_goal": "Unbind Chart Generator, then read config again after completion and verify the remaining bindings.",
			"tool_result": map[string]interface{}{
				"status":         "success",
				"skill_id":       skills.SkillAgentManagement,
				"tool_name":      "update_agent_config",
				"result_summary": resultSummary,
			},
		},
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
			},
		},
	})
	if !ok {
		t.Fatal("FastPathFinalAnswerForCompletionEvidence() ok = false, want true after requested post-update get_agent_config succeeds")
	}
	for _, want := range []string{"Support Agent", "Chart Generator", "当前保留", "Architecture Diagram Generator"} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want %q", answer, want)
		}
	}
}

func TestFastPathFinalAnswerForCompletionEvidenceBlocksAgentConfigMismatchAfterPostRead(t *testing.T) {
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":             "completed",
			"original_user_goal": "Update Agent config and read config again after completion.",
			"steps": []interface{}{
				map[string]interface{}{
					"id":                      "tool:agent-management/update_agent_config",
					"skill_id":                skills.SkillAgentManagement,
					"tool_name":               "update_agent_config",
					"status":                  "completed",
					"expected_updated_fields": []interface{}{"suggested_questions"},
					"arguments": map[string]interface{}{
						"suggested_questions": []interface{}{"Question A", "Question B"},
					},
				},
			},
		},
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
				"arguments": map[string]interface{}{
					"expected_updated_fields": []interface{}{"suggested_questions"},
					"suggested_questions":     []interface{}{"Question A", "Question B"},
				},
				"result": map[string]interface{}{
					"status":         "completed",
					"agent_name":     "Support Agent",
					"updated_fields": []interface{}{"suggested_questions"},
					"config": map[string]interface{}{
						"suggested_questions": []interface{}{"Question A", "Question B"},
					},
				},
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
				"result": map[string]interface{}{
					"status":     "completed",
					"agent_name": "Support Agent",
					"config": map[string]interface{}{
						"suggested_questions": []interface{}{"Question A"},
					},
				},
			},
		},
	}

	if answer, ok := FastPathFinalAnswerForCompletionEvidence(evidence); ok {
		t.Fatalf("FastPathFinalAnswerForCompletionEvidence() = (%q, true), want blocked by post-read suggested_questions mismatch", answer)
	}
	if answer, ok := FastPathPreferredFinalAnswerForCompletionEvidence(evidence, "done"); ok {
		t.Fatalf("FastPathPreferredFinalAnswerForCompletionEvidence() = (%q, true), want blocked by post-read suggested_questions mismatch", answer)
	}
	trace := skills.SkillTrace{
		Kind:     "tool_call",
		Status:   "success",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Result: map[string]interface{}{
			"status":         "completed",
			"agent_name":     "Support Agent",
			"updated_fields": []interface{}{"suggested_questions"},
		},
	}
	if answer, ok := FastPathFinalAnswerForToolTraceWithEvidence(trace, evidence); ok {
		t.Fatalf("FastPathFinalAnswerForToolTraceWithEvidence() = (%q, true), want blocked by post-read suggested_questions mismatch", answer)
	}
}

func TestFastPathReadOnlyConfigDoesNotCloseWhenAgentMutationStepPending(t *testing.T) {
	evidence := map[string]interface{}{
		"operation_plan": pendingAgentSkillUnbindOperationPlanForTest(),
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
				"result": map[string]interface{}{
					"status":              "completed",
					"agent_id":            "agent-1",
					"model":               "gpt-4o",
					"model_provider":      "openai",
					"enabled_skill_count": 2,
				},
			},
		},
	}
	trace := skills.SkillTrace{
		Kind:     "tool_call",
		Status:   "success",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "get_agent_config",
		Result: map[string]interface{}{
			"status":              "completed",
			"agent_id":            "agent-1",
			"model":               "gpt-4o",
			"model_provider":      "openai",
			"enabled_skill_count": 2,
		},
	}

	if answer, ok := FastPathFinalAnswerForToolTraceWithEvidence(trace, evidence); ok {
		t.Fatalf("FastPathFinalAnswerForToolTraceWithEvidence() = (%q, true), want blocked by pending update_agent_config", answer)
	}
	if answer, ok := FastPathFinalAnswerForCompletionEvidence(evidence); ok {
		t.Fatalf("FastPathFinalAnswerForCompletionEvidence() = (%q, true), want blocked by pending update_agent_config", answer)
	}
}

func TestFastPathReadOnlyConfigDoesNotCloseMutationIntentWithScopedNoOtherChanges(t *testing.T) {
	evidence := map[string]interface{}{
		"original_user_goal": "Skill \u89e3\u7ed1\u56de\u5f52\uff1a\u8bf7\u53ea\u5904\u7406\u5f53\u524d\u667a\u80fd\u4f53\u7684 Skill \u89e3\u7ed1\u3002\u82e5 chart-generator \u5df2\u7ed1\u5b9a\uff0c\u8bf7\u53ea\u89e3\u7ed1 chart-generator\uff1b\u5fc5\u987b\u4fdd\u7559 architecture-diagram-generator\u3002\u4e0d\u8981\u4fee\u6539\u5176\u4ed6\u914d\u7f6e\u3002\u5ba1\u6279\u901a\u8fc7\u540e\u91cd\u65b0\u8bfb\u53d6\u914d\u7f6e\u3002",
	}
	trace := skills.SkillTrace{
		Kind:     "tool_call",
		Status:   "success",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "get_agent_config",
		Result: map[string]interface{}{
			"status":              "completed",
			"agent_id":            "agent-1",
			"model":               "gpt-4o",
			"model_provider":      "openai",
			"enabled_skill_count": 2,
		},
	}

	if answer, ok := FastPathFinalAnswerForToolTraceWithEvidence(trace, evidence); ok {
		t.Fatalf("FastPathFinalAnswerForToolTraceWithEvidence() = (%q, true), want blocked by positive unbind intent", answer)
	}
	if fastPathGoalRequestsReadOnlyAgentConfig(evidence) {
		t.Fatal("fastPathGoalRequestsReadOnlyAgentConfig() = true, want false for scoped no-other-config mutation request")
	}
}

func TestFastPathReadOnlyConfigAllowsExplicitReadOnlyNoMutation(t *testing.T) {
	evidence := map[string]interface{}{
		"original_user_goal": "\u53ea\u67e5\u770b\u5f53\u524d\u667a\u80fd\u4f53\u914d\u7f6e\uff0c\u4e0d\u8981\u4fee\u6539\u4efb\u4f55\u914d\u7f6e\u3002",
	}
	trace := skills.SkillTrace{
		Kind:     "tool_call",
		Status:   "success",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "get_agent_config",
		Result: map[string]interface{}{
			"status":         "completed",
			"agent_id":       "agent-1",
			"model":          "gpt-4o",
			"model_provider": "openai",
		},
	}

	if answer, ok := FastPathFinalAnswerForToolTraceWithEvidence(trace, evidence); !ok {
		t.Fatal("FastPathFinalAnswerForToolTraceWithEvidence() ok = false, want true for explicit read-only config request")
	} else if !strings.Contains(answer, "gpt-4o") {
		t.Fatalf("answer = %q, want model detail", answer)
	}
}

func TestCompletionEvidenceMergesCurrentMetadataOperationPlanBeforeFastPath(t *testing.T) {
	evidence := completionEvidenceForFastPathWithSuccessfulToolCalls(RunRequest{
		CompletionEvidence: func() map[string]interface{} {
			return map[string]interface{}{}
		},
		CurrentMetadata: func() map[string]interface{} {
			return map[string]interface{}{
				"operation_plan": pendingAgentSkillUnbindOperationPlanForTest(),
			}
		},
	}, []SkillToolCallRef{
		{
			SkillID:  skills.SkillAgentManagement,
			ToolName: "get_agent_config",
			Result: map[string]interface{}{
				"status":              "completed",
				"agent_id":            "agent-1",
				"model":               "gpt-4o",
				"model_provider":      "openai",
				"enabled_skill_count": 2,
			},
		},
	})

	if answer, ok := FastPathFinalAnswerForCompletionEvidence(evidence); ok {
		t.Fatalf("FastPathFinalAnswerForCompletionEvidence() = (%q, true), want blocked by CurrentMetadata pending update_agent_config", answer)
	}
}

func pendingAgentSkillUnbindOperationPlanForTest() map[string]interface{} {
	return map[string]interface{}{
		"status":              "running",
		"original_user_goal":  "Skill解绑回归：若 chart-generator 已绑定，请只解绑 chart-generator；必须保留 architecture-diagram-generator。请使用 update_agent_config 的 remove_enabled_skill_ids 一次性提交，审批通过后重新读取配置。",
		"pending_next_action": "Run tool:agent-management/update_agent_config",
		"steps": []interface{}{
			map[string]interface{}{
				"id":        "tool:agent-management/get_agent_config",
				"status":    "completed",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
			},
			map[string]interface{}{
				"id":        "tool:agent-management/update_agent_config",
				"status":    "pending",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
				"asset_target": map[string]interface{}{
					"effect":     "update",
					"asset_type": "agent",
				},
			},
			map[string]interface{}{
				"id":        "tool:agent-management/get_agent_config#post_update",
				"phase":     "post_update_verification",
				"status":    "pending",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
			},
		},
		"step_status": map[string]interface{}{
			"tool:agent-management/get_agent_config":             "completed",
			"tool:agent-management/update_agent_config":          "pending",
			"tool:agent-management/get_agent_config#post_update": "pending",
		},
	}
}

func TestFastPathPreferredFinalAnswerForCompletionEvidenceOverridesMisleadingPostReadText(t *testing.T) {
	resultSummary := map[string]interface{}{
		"status":     "completed",
		"agent_name": "Support Agent",
		"updated_fields": []interface{}{
			"enabled_skill_ids",
		},
		"binding_changes": []interface{}{
			map[string]interface{}{
				"field":                "enabled_skill_ids",
				"binding_kind":         "agent_skill",
				"change_action":        "bind",
				"resource_count":       1,
				"added_resource_count": 1,
				"final_resource_count": 1,
				"resource_names":       []interface{}{"Chart Generator"},
			},
		},
	}
	answer, ok := FastPathPreferredFinalAnswerForCompletionEvidence(map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":             "completed",
			"original_user_goal": "Bind Chart Generator, then read config again after completion and verify it is bound.",
			"tool_result": map[string]interface{}{
				"status":         "success",
				"skill_id":       skills.SkillAgentManagement,
				"tool_name":      "update_agent_config",
				"result_summary": resultSummary,
			},
		},
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
			},
		},
	}, "复读配置后发现 Chart Generator 已处于绑定状态，因此本次没有实际绑定。")
	if !ok {
		t.Fatal("FastPathPreferredFinalAnswerForCompletionEvidence() ok = false, want evidence-grounded override")
	}
	for _, want := range []string{"Support Agent", "Chart Generator", "绑定", "重新读取配置"} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want %q", answer, want)
		}
	}
	for _, unwanted := range []string{"没有实际绑定", "无需"} {
		if strings.Contains(answer, unwanted) {
			t.Fatalf("answer = %q, want no misleading no-op claim %q", answer, unwanted)
		}
	}
}

func TestFastPathFinalAnswerForCompletionEvidenceUsesOperationPlanToolResultDespiteStalePendingIdentity(t *testing.T) {
	resultSummary := map[string]interface{}{
		"status":     "completed",
		"agent_name": "Support Agent",
		"updated_fields": []interface{}{
			"enabled_skill_ids",
			"knowledge_dataset_ids",
			"database_bindings",
			"workflow_bindings",
		},
		"binding_changes": []interface{}{
			map[string]interface{}{
				"field":                  "knowledge_dataset_ids",
				"binding_kind":           "knowledge_base",
				"change_action":          "unbind",
				"resource_count":         1,
				"removed_resource_count": 1,
				"resource_names":         []interface{}{"KB Two"},
			},
			map[string]interface{}{
				"field":                  "database_bindings",
				"binding_kind":           "database_table",
				"change_action":          "unbind",
				"resource_count":         1,
				"removed_resource_count": 1,
				"resource_names":         []interface{}{"orders"},
			},
			map[string]interface{}{
				"field":                  "workflow_bindings",
				"binding_kind":           "workflow",
				"change_action":          "unbind",
				"resource_count":         1,
				"removed_resource_count": 1,
				"resource_names":         []interface{}{"Approval Flow"},
			},
		},
	}
	answer, ok := FastPathFinalAnswerForCompletionEvidence(map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":              "running",
			"original_user_goal":  "\u8bf7\u7ed1\u5b9a KB Two\u3001orders \u548c Approval Flow\uff0c\u8bf7\u4fdd\u7559\u5df2\u7ed1\u5b9a\u7684\u56fe\u8868\u751f\u6210\u5668 Skill\uff0c\u4e0d\u6539\u540d\u79f0\u3001\u63cf\u8ff0\u3001\u6a21\u578b\u3001\u9996\u9875\u6807\u9898\u3001\u5f00\u573a\u95ee\u9898\u6216\u5176\u4ed6\u914d\u7f6e\u3002",
			"pending_next_action": "Run tool:agent-management/update_agent_identity",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_identity",
					"status":    "pending",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_identity",
				},
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_config",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_config",
				},
			},
			"tool_result": map[string]interface{}{
				"status":         "success",
				"skill_id":       skills.SkillAgentManagement,
				"tool_name":      "update_agent_config",
				"result_summary": resultSummary,
			},
		},
		"operation_result_summary": map[string]interface{}{
			"pending_next_action": "Run tool:agent-management/update_agent_identity",
			"latest_tool_result": map[string]interface{}{
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
				"result_summary": map[string]interface{}{
					"status":                  "completed",
					"agent_name":              "Support Agent",
					"knowledge_dataset_count": 1,
					"database_binding_count":  1,
					"workflow_binding_count":  1,
				},
			},
		},
	})
	if !ok {
		t.Fatal("FastPathFinalAnswerForCompletionEvidence() ok = false, want true")
	}
	for _, want := range []string{"Support Agent", "KB Two", "orders", "Approval Flow"} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want resource name %q", answer, want)
		}
	}
	for _, unwanted := range []string{"skill", "Skill", "update_agent_identity"} {
		if strings.Contains(answer, unwanted) {
			t.Fatalf("answer = %q, want no stale pending identity or preserved skill claim %q", answer, unwanted)
		}
	}
}

func TestFastPathFinalAnswerForAgentConfigUpdateIgnoresPreservedSkillField(t *testing.T) {
	resultSummary := map[string]interface{}{
		"status":        "completed",
		"effect":        "updated",
		"agent_name":    "Support Agent",
		"binding_kind":  "multiple",
		"change_action": "bind",
		"updated_fields": []interface{}{
			"enabled_skill_ids",
			"knowledge_dataset_ids",
			"database_bindings",
			"workflow_bindings",
		},
		"binding_changes": []interface{}{
			map[string]interface{}{
				"field":                "knowledge_dataset_ids",
				"binding_kind":         "knowledge_base",
				"change_action":        "bind",
				"resource_count":       1,
				"added_resource_count": 1,
				"resource_names":       []interface{}{"KB Two"},
			},
			map[string]interface{}{
				"field":                "database_bindings",
				"binding_kind":         "database_table",
				"change_action":        "bind",
				"resource_count":       1,
				"added_resource_count": 1,
				"resource_names":       []interface{}{"orders"},
			},
			map[string]interface{}{
				"field":                "workflow_bindings",
				"binding_kind":         "workflow",
				"change_action":        "bind",
				"resource_count":       1,
				"added_resource_count": 1,
				"resource_names":       []interface{}{"Approval Flow"},
			},
		},
	}
	answer, ok := FastPathFinalAnswerForToolTrace(skills.SkillTrace{
		Kind:     "tool_call",
		Status:   "success",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Result:   resultSummary,
	})
	if !ok {
		t.Fatal("FastPathFinalAnswerForToolTrace() ok = false, want true")
	}
	for _, want := range []string{"Support Agent", "KB Two", "orders", "Approval Flow"} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want resource name %q", answer, want)
		}
	}
	for _, unwanted := range []string{"技能", "Skill", "skill", "chart-generator"} {
		if strings.Contains(answer, unwanted) {
			t.Fatalf("answer = %q, want no preserved skill binding claim %q", answer, unwanted)
		}
	}
}

func TestFastPathFinalAnswerForCompletionEvidenceWaitsWhenGoalRequestsPendingIdentity(t *testing.T) {
	resultSummary := map[string]interface{}{
		"status":     "completed",
		"agent_name": "Support Agent",
		"updated_fields": []interface{}{
			"knowledge_dataset_ids",
		},
		"binding_changes": []interface{}{
			map[string]interface{}{
				"field":                "knowledge_dataset_ids",
				"binding_kind":         "knowledge_base",
				"change_action":        "bind",
				"resource_count":       1,
				"added_resource_count": 1,
				"resource_names":       []interface{}{"KB Two"},
			},
		},
	}
	_, ok := FastPathFinalAnswerForCompletionEvidence(map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":              "running",
			"original_user_goal":  "Rename the agent to Support Bot and bind KB Two.",
			"pending_next_action": "Run tool:agent-management/update_agent_identity",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_identity",
					"status":    "pending",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_identity",
				},
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_config",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_config",
				},
			},
			"tool_result": map[string]interface{}{
				"status":         "success",
				"skill_id":       skills.SkillAgentManagement,
				"tool_name":      "update_agent_config",
				"result_summary": resultSummary,
			},
		},
	})
	if ok {
		t.Fatal("FastPathFinalAnswerForCompletionEvidence() ok = true, want false while requested identity update is still pending")
	}
}

func TestFastPathFinalAnswerForCompletionEvidenceWaitsWhenChineseGoalRequestsPendingIdentity(t *testing.T) {
	resultSummary := map[string]interface{}{
		"status":     "completed",
		"agent_name": "Support Agent",
		"updated_fields": []interface{}{
			"system_prompt",
			"home_title",
			"suggested_questions",
		},
	}
	_, ok := FastPathFinalAnswerForCompletionEvidence(map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "running",
			"original_user_goal": strings.Join([]string{
				"\u540d\u79f0\u6539\u4e3a\u300cGOAL-SMOKE-EDITED\u300d",
				"\u63cf\u8ff0\u6539\u4e3a\u300cAIChat smoke edit\u300d",
				"\u56fe\u6807\u6539\u6210\u300cGE\u300d",
				"\u7cfb\u7edf\u63d0\u793a\u8bcd\u6539\u4e3a\u300cbe concise\u300d",
				"\u5f00\u573a\u95ee\u9898\u8bbe\u7f6e\u4e3a 3 \u6761",
			}, "\uff1b"),
			"pending_next_action": "Run tool:agent-management/update_agent_identity",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_identity",
					"status":    "pending",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_identity",
				},
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_config",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_config",
				},
			},
			"tool_result": map[string]interface{}{
				"status":         "success",
				"skill_id":       skills.SkillAgentManagement,
				"tool_name":      "update_agent_config",
				"result_summary": resultSummary,
			},
		},
	})
	if ok {
		t.Fatal("FastPathFinalAnswerForCompletionEvidence() ok = true, want false while Chinese goal still requests pending identity update")
	}
}

func TestFastPathFinalAnswerForAgentModelUpdateIncludesProviderAndModel(t *testing.T) {
	answer, ok := FastPathFinalAnswerForToolTrace(skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Status:   "success",
		Result: map[string]interface{}{
			"status":         "completed",
			"agent_name":     "Support Agent",
			"updated_fields": []interface{}{"model_provider", "model"},
			"model_provider": "deepseek",
			"model":          "deepseek-v4-flash",
		},
	})

	if !ok {
		t.Fatal("FastPathFinalAnswerForToolTrace() ok = false, want true for successful Agent model update")
	}
	for _, want := range []string{"Support Agent", "deepseek", "deepseek-v4-flash"} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want %q", answer, want)
		}
	}
}

func TestFastPathFinalAnswerForAgentHomeAndSuggestedQuestionsIncludesValues(t *testing.T) {
	answer, ok := FastPathFinalAnswerForToolTrace(skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Status:   "success",
		Result: map[string]interface{}{
			"status":         "completed",
			"agent_name":     "Support Agent",
			"updated_fields": []interface{}{"system_prompt", "agent_memory_enabled", "file_upload_enabled", "home_title", "input_placeholder", "theme_color", "suggested_questions"},
			"config": map[string]interface{}{
				"system_prompt":        "\u4f60\u662f\u4e00\u4f4d\u7a33\u5b9a\u7684\u5ba2\u670d\u52a9\u624b\u3002",
				"agent_memory_enabled": true,
				"file_upload_enabled":  false,
				"home_title":           "\u0041\u0049\u0043\u0068\u0061\u0074 \u914d\u7f6e\u95ed\u73af\u5192\u70df 0630",
				"input_placeholder":    "\u8bf7\u8f93\u5165\u95ee\u9898",
				"theme_color":          "emerald",
				"suggested_questions": []interface{}{
					"\u914d\u7f6e\u662f\u5426\u5df2\u4fdd\u5b58\uff1f",
					"\u6a21\u578b\u662f\u5426\u53ef\u7528\uff1f",
					"\u8fd8\u9700\u8981\u6d4b\u8bd5\u4ec0\u4e48\uff1f",
				},
			},
		},
	})

	if !ok {
		t.Fatal("FastPathFinalAnswerForToolTrace() ok = false, want true for successful Agent home config update")
	}
	for _, want := range []string{
		"Support Agent",
		"\u7cfb\u7edf\u63d0\u793a\u8bcd\uff1a\u4f60\u662f\u4e00\u4f4d\u7a33\u5b9a\u7684\u5ba2\u670d\u52a9\u624b\u3002",
		"\u8bb0\u5fc6\uff1a\u5f00\u542f",
		"\u6587\u4ef6\u4e0a\u4f20\uff1a\u5173\u95ed",
		"\u9996\u9875\u6807\u9898\uff1a\u0041\u0049\u0043\u0068\u0061\u0074 \u914d\u7f6e\u95ed\u73af\u5192\u70df 0630",
		"\u8f93\u5165\u6846\u5360\u4f4d\u6587\u6848\uff1a\u8bf7\u8f93\u5165\u95ee\u9898",
		"\u4e3b\u9898\u8272\uff1aemerald",
		"\u5f00\u573a\u95ee\u9898\uff1a3 \u4e2a",
		"\u914d\u7f6e\u662f\u5426\u5df2\u4fdd\u5b58\uff1f",
		"\u6a21\u578b\u662f\u5426\u53ef\u7528\uff1f",
		"\u8fd8\u9700\u8981\u6d4b\u8bd5\u4ec0\u4e48\uff1f",
	} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want %q", answer, want)
		}
	}
}

func TestFastPathFinalAnswerForAgentConfigIncludesSatisfiedOnlyFields(t *testing.T) {
	answer, ok := FastPathFinalAnswerForToolTrace(skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Status:   "success",
		Result: map[string]interface{}{
			"status":            "completed",
			"agent_name":        "Support Agent",
			"updated_fields":    []interface{}{"home_title", "input_placeholder"},
			"satisfied_fields":  []interface{}{"home_title", "input_placeholder", "theme_color"},
			"home_title":        "Runtime Check",
			"input_placeholder": "\u8bf7\u8f93\u5165\u95ee\u9898",
			"theme_color":       "emerald",
		},
	})

	if !ok {
		t.Fatal("FastPathFinalAnswerForToolTrace() ok = false, want true for successful Agent config update")
	}
	for _, want := range []string{
		"Support Agent",
		"\u9996\u9875\u6807\u9898\uff1aRuntime Check",
		"\u8f93\u5165\u6846\u5360\u4f4d\u6587\u6848\uff1a\u8bf7\u8f93\u5165\u95ee\u9898",
		"\u4e3b\u9898\u8272\u5df2\u6ee1\u8db3\uff1aemerald",
	} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want %q", answer, want)
		}
	}
	if strings.Contains(answer, "\u4e3b\u9898\u8272\uff1aemerald") {
		t.Fatalf("answer = %q, want satisfied-only theme to avoid claiming an actual change", answer)
	}
}

func TestFastPathFinalAnswerForAgentConfigIncludesSatisfiedBindingFinalState(t *testing.T) {
	answer, ok := FastPathFinalAnswerForToolTrace(skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Status:   "success",
		Result: map[string]interface{}{
			"status":           "completed",
			"agent_name":       "Support Agent",
			"satisfied_fields": []interface{}{"enabled_skill_ids"},
			"binding_final_states": []interface{}{map[string]interface{}{
				"field":                "enabled_skill_ids",
				"binding_kind":         "agent_skill",
				"change_action":        "satisfied",
				"final_resource_count": 0,
			}},
		},
	})

	if !ok {
		t.Fatal("FastPathFinalAnswerForToolTrace() ok = false, want true for satisfied binding final state")
	}
	for _, want := range []string{
		"Support Agent",
		"\u6280\u80fd\u5df2\u6ee1\u8db3",
		"\u5f53\u524d\u672a\u7ed1\u5b9a\u6280\u80fd",
	} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want %q", answer, want)
		}
	}
}

func TestAgentReadOnlyConfigFastPathAnswerFromLocalSkillInvocations(t *testing.T) {
	answer, ok := agentReadOnlyConfigFastPathAnswerFromEvidence(map[string]interface{}{
		"user_request": "\u53ea\u8bfb\u68c0\u67e5\u5f53\u524d\u667a\u80fd\u4f53\u914d\u7f6e\uff1a\u53ea\u786e\u8ba4\u540d\u79f0\u3001\u63cf\u8ff0\u3001\u6a21\u578b/provider\u3001\u5f53\u524d\u5df2\u7ed1\u5b9a\u8d44\u6e90\u6570\u91cf\u3002\u4e0d\u8981\u5217\u5019\u9009\u8d44\u6e90\uff0c\u4e0d\u8981\u4fee\u6539\u4efb\u4f55\u914d\u7f6e\u3002",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
				"result": map[string]interface{}{
					"status": "completed",
					"config": map[string]interface{}{
						"model_provider":        "deepseek",
						"model":                 "deepseek-chat",
						"enabled_skill_ids":     []interface{}{"chart-generator"},
						"knowledge_dataset_ids": []interface{}{"kb-1"},
						"database_bindings":     []interface{}{map[string]interface{}{"database_id": "db-1", "table_ids": []interface{}{"table-1"}}},
						"workflow_bindings":     []interface{}{map[string]interface{}{"workflow_id": "workflow-1"}},
						"agent_memory_enabled":  true,
						"file_upload_enabled":   false,
						"input_placeholder":     "\u8bf7\u8f93\u5165\u95ee\u9898",
						"theme_color":           "emerald",
					},
				},
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent",
				"result": map[string]interface{}{
					"status":            "completed",
					"agent_name":        "ReadOnly Agent",
					"agent_description": "Current Agent description",
				},
			},
		},
	})

	if !ok {
		t.Fatal("agentReadOnlyConfigFastPathAnswerFromEvidence() ok = false, want true")
	}
	for _, want := range []string{
		"ReadOnly Agent",
		"Current Agent description",
		"deepseek/deepseek-chat",
		"\u8f93\u5165\u6846\u5360\u4f4d\u6587\u6848\uff1a\u8bf7\u8f93\u5165\u95ee\u9898",
		"\u4e3b\u9898\u8272\uff1aemerald",
		"\u6280\u80fd 1 \u4e2a",
		"\u77e5\u8bc6\u5e93 1 \u4e2a",
		"\u6570\u636e\u5e93\u8868 1 \u4e2a",
		"\u5de5\u4f5c\u6d41 1 \u4e2a",
	} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want %q", answer, want)
		}
	}
}

func TestAgentReadOnlyConfigFastPathUsesLatestUserRequestOverStaleMutationPlan(t *testing.T) {
	answer, ok := agentReadOnlyConfigFastPathAnswerFromEvidence(map[string]interface{}{
		"user_request": "\u53ea\u8bfb\u68c0\u67e5\u5f53\u524d\u667a\u80fd\u4f53\u914d\u7f6e\uff1a\u53ea\u786e\u8ba4\u540d\u79f0\u3001\u63cf\u8ff0\u3001\u6a21\u578b/provider\u3001\u5f53\u524d\u5df2\u7ed1\u5b9a\u8d44\u6e90\u6570\u91cf\u3002\u4e0d\u8981\u5217\u5019\u9009\u8d44\u6e90\uff0c\u4e0d\u8981\u4fee\u6539\u4efb\u4f55\u914d\u7f6e\u3002",
		"operation_plan": map[string]interface{}{
			"original_user_goal": "\u4fee\u6539\u5f53\u524d\u667a\u80fd\u4f53\u914d\u7f6e",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_config",
					"status":    "pending",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_config",
				},
			},
		},
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
				"result": map[string]interface{}{
					"status": "completed",
					"config": map[string]interface{}{
						"model_provider":      "deepseek",
						"model":               "deepseek-chat",
						"enabled_skill_count": 1,
					},
				},
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent",
				"result": map[string]interface{}{
					"status":     "completed",
					"agent_name": "ReadOnly Agent",
				},
			},
		},
	})

	if !ok {
		t.Fatal("agentReadOnlyConfigFastPathAnswerFromEvidence() ok = false, want stale mutation plan ignored for explicit read-only request")
	}
	if !strings.Contains(answer, "ReadOnly Agent") || !strings.Contains(answer, "deepseek/deepseek-chat") {
		t.Fatalf("answer = %q, want current read-only config summary", answer)
	}
}

func TestCompletionEvidenceAddsLatestUserRequestOverStaleEvidence(t *testing.T) {
	latestRequest := "复测只读配置闭环：请只读取当前 Agent 配置并回答当前首页标题、模型 provider/model、绑定的 Skill/知识库/数据库表/工作流数量。不要修改任何配置，不要发起审批，不要查询可用模型或候选资源。"
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: latestRequest}},
	})
	prepared.Query = latestRequest
	evidence := completionEvidenceForFastPathWithSuccessfulToolCalls(RunRequest{
		Prepared: prepared,
		CompletionEvidence: func() map[string]interface{} {
			return map[string]interface{}{
				"user_request":       "请修改当前智能体配置",
				"original_user_goal": "请修改当前智能体配置",
				"operation_plan": map[string]interface{}{
					"original_user_goal": "请修改当前智能体配置",
					"steps": []interface{}{
						map[string]interface{}{
							"id":        "tool:agent-management/update_agent_config",
							"status":    "pending",
							"skill_id":  skills.SkillAgentManagement,
							"tool_name": "update_agent_config",
						},
					},
				},
			}
		},
	}, []SkillToolCallRef{
		{
			SkillID:  skills.SkillAgentManagement,
			ToolName: "get_agent_config",
			Result: map[string]interface{}{
				"status": "completed",
				"config": map[string]interface{}{
					"model_provider":        "deepseek",
					"model":                 "deepseek-v4-flash",
					"home_title":            "AIChat leaf 恢复验证",
					"enabled_skill_ids":     []interface{}{},
					"knowledge_dataset_ids": []interface{}{},
					"database_bindings":     []interface{}{},
					"workflow_bindings":     []interface{}{},
				},
			},
		},
	})

	if got := stringFromAny(evidence["latest_user_request"]); got != latestRequest {
		t.Fatalf("latest_user_request = %q, want current user request", got)
	}
	answer, ok := agentReadOnlyConfigFastPathAnswerFromEvidence(evidence)
	if !ok {
		t.Fatal("agentReadOnlyConfigFastPathAnswerFromEvidence() ok = false, want latest read-only request to override stale mutation evidence")
	}
	if !strings.Contains(answer, "AIChat leaf 恢复验证") || !strings.Contains(answer, "deepseek/deepseek-v4-flash") {
		t.Fatalf("answer = %q, want read-only config summary from tool result", answer)
	}
}

func TestAgentReadOnlyConfigFastPathBeforeRedundantCandidateLookup(t *testing.T) {
	evidence := map[string]interface{}{
		"user_request": "Read-only check current Agent configuration: answer current homepage title, model provider/model, and bound resource counts. Do not modify any config.",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
				"result": map[string]interface{}{
					"status": "completed",
					"config": map[string]interface{}{
						"model_provider":        "deepseek",
						"model":                 "deepseek-v4-flash",
						"home_title":            "AIChat leaf recovery check",
						"enabled_skill_ids":     []interface{}{},
						"knowledge_dataset_ids": []interface{}{},
						"database_bindings":     []interface{}{},
						"workflow_bindings":     []interface{}{},
					},
				},
			},
		},
	}

	answer, ok := agentReadOnlyConfigFastPathAnswerBeforeRedundantLookup(
		skills.SkillAgentManagement,
		"list_available_models",
		evidence,
	)

	if !ok {
		t.Fatal("agentReadOnlyConfigFastPathAnswerBeforeRedundantLookup() ok = false, want true")
	}
	for _, want := range []string{"AIChat leaf recovery check", "deepseek/deepseek-v4-flash", "\u6280\u80fd 0 \u4e2a"} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want %q", answer, want)
		}
	}
}

func TestAgentReadOnlyConfigFastPathBeforeNegatedCandidateLookup(t *testing.T) {
	evidence := map[string]interface{}{
		"user_request": "只读取当前 Agent 配置并回答当前首页标题、模型 provider/model、绑定资源数量。不要修改任何配置，不要查询可用模型或候选资源。",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
				"result": map[string]interface{}{
					"status": "completed",
					"config": map[string]interface{}{
						"model_provider": "deepseek",
						"model":          "deepseek-v4-flash",
						"home_title":     "AIChat leaf recovery check",
					},
				},
			},
		},
	}

	answer, ok := agentReadOnlyConfigFastPathAnswerBeforeRedundantLookup(
		skills.SkillAgentManagement,
		"list_available_models",
		evidence,
	)

	if !ok {
		t.Fatal("agentReadOnlyConfigFastPathAnswerBeforeRedundantLookup() ok = false, want negated available-model lookup to fast-answer")
	}
	if !strings.Contains(answer, "AIChat leaf recovery check") {
		t.Fatalf("answer = %q, want config summary", answer)
	}
}

func TestAgentReadOnlyConfigFastPathBeforeRedundantCandidateLookupKeepsExplicitCandidateRequest(t *testing.T) {
	evidence := map[string]interface{}{
		"user_request": "Read-only check current Agent configuration and list available models. Do not modify any config.",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
				"result": map[string]interface{}{
					"status": "completed",
					"config": map[string]interface{}{
						"model_provider": "deepseek",
						"model":          "deepseek-v4-flash",
					},
				},
			},
		},
	}

	if answer, ok := agentReadOnlyConfigFastPathAnswerBeforeRedundantLookup(
		skills.SkillAgentManagement,
		"list_available_models",
		evidence,
	); ok {
		t.Fatalf("agentReadOnlyConfigFastPathAnswerBeforeRedundantLookup() = %q, true; want false for explicit available-model request", answer)
	}
}

func TestAgentReadOnlyConfigFastPathDoesNotCloseChineseBindableResourceRequest(t *testing.T) {
	request := "\u53ea\u8bfb\u67e5\u8be2\uff0c\u4e0d\u8981\u4fee\u6539\u914d\u7f6e\u3002\u8bf7\u5217\u51fa\u5f53\u524d\u667a\u80fd\u4f53\u53ef\u7ed1\u5b9a\u7684\u77e5\u8bc6\u5e93\u3001\u6570\u636e\u5e93\u8868\u3001\u5de5\u4f5c\u6d41\u5404\u524d 3 \u4e2a\uff0c\u5e76\u8bf4\u660e\u5f53\u524d\u5df2\u7ed1\u5b9a\u6570\u91cf\u3002"
	configResult := map[string]interface{}{
		"status": "completed",
		"config": map[string]interface{}{
			"model_provider":          "deepseek",
			"model":                   "deepseek-v4-flash",
			"knowledge_dataset_count": 0,
			"database_table_count":    0,
			"workflow_binding_count":  0,
		},
	}
	evidence := map[string]interface{}{
		"user_request": request,
		"operation_plan": map[string]interface{}{
			"original_user_goal": request,
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/list_agent_knowledge_candidates",
					"status":    "pending",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "list_agent_knowledge_candidates",
				},
				map[string]interface{}{
					"id":        "tool:agent-management/list_agent_database_candidates",
					"status":    "pending",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "list_agent_database_candidates",
				},
				map[string]interface{}{
					"id":        "tool:agent-management/list_agent_database_tables",
					"status":    "pending",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "list_agent_database_tables",
				},
				map[string]interface{}{
					"id":        "tool:agent-management/list_agent_workflow_binding_candidates",
					"status":    "pending",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "list_agent_workflow_binding_candidates",
				},
			},
		},
		"operation_result_summary": map[string]interface{}{
			"latest_tool_result": map[string]interface{}{
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "list_agent_database_tables",
			},
		},
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
				"result":    configResult,
			},
		},
	}
	trace := skills.SkillTrace{
		Kind:     "tool_call",
		Status:   "success",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "get_agent_config",
		Result:   configResult,
	}

	if answer, ok := FastPathFinalAnswerForToolTraceWithEvidence(trace, evidence); ok {
		t.Fatalf("FastPathFinalAnswerForToolTraceWithEvidence() = %q, true; want false until requested candidate tools run", answer)
	}
	if answer, ok := FastPathFinalAnswerForCompletionEvidence(evidence); ok {
		t.Fatalf("FastPathFinalAnswerForCompletionEvidence() = %q, true; want false until requested candidate tools run", answer)
	}
	if !fastPathGoalRequestsReadOnlyAgentConfig(evidence) {
		t.Fatal("fastPathGoalRequestsReadOnlyAgentConfig() = false, want bindable-resource read request treated as read-only config context")
	}
	if !fastPathGoalExplicitlyRequestsAgentCandidateLookup(evidence) {
		t.Fatal("fastPathGoalExplicitlyRequestsAgentCandidateLookup() = false, want Chinese bindable resource request detected")
	}
}

func TestFastPathCompletionEvidenceClosesReadOnlyAgentCandidateLookup(t *testing.T) {
	request := "\u5192\u70df\u9a8c\u8bc18a\uff1a\u53ea\u8bfb\u67e5\u770b\u667a\u80fd\u4f53\u5f53\u524d\u53ef\u7ed1\u5b9a\u7684 Skill\u3001\u77e5\u8bc6\u5e93\u3001\u6570\u636e\u5e93\u8868\u3001\u5de5\u4f5c\u6d41\u5019\u9009\u5404\u6709\u54ea\u4e9b\u3002\u4e0d\u8981\u4fee\u6539\u914d\u7f6e\uff0c\u4e0d\u8981\u53d1\u8d77\u5ba1\u6279\u3002"
	evidence := map[string]interface{}{
		"user_request": request,
		"operation_plan": map[string]interface{}{
			"original_user_goal": request,
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/list_agent_skill_candidates",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "list_agent_skill_candidates",
				},
				map[string]interface{}{
					"id":        "tool:agent-management/list_agent_knowledge_candidates",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "list_agent_knowledge_candidates",
				},
				map[string]interface{}{
					"id":        "tool:agent-management/list_agent_database_candidates",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "list_agent_database_candidates",
				},
				map[string]interface{}{
					"id":        "tool:agent-management/list_agent_database_tables",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "list_agent_database_tables",
				},
				map[string]interface{}{
					"id":        "tool:agent-management/list_agent_workflow_binding_candidates",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "list_agent_workflow_binding_candidates",
				},
			},
		},
		"skill_invocations": []interface{}{
			agentCandidateLookupInvocationForTest("list_agent_skill_candidates", 20, []map[string]interface{}{
				{"id": "calculator", "name": "\u8ba1\u7b97\u5668"},
				{"id": "chart-generator", "name": "\u56fe\u8868\u751f\u6210\u5668"},
			}),
			agentCandidateLookupInvocationForTest("list_agent_knowledge_candidates", 2, []map[string]interface{}{
				{"id": "kb-1", "name": "\u6d4b\u8bd5\u5e932"},
			}),
			agentCandidateLookupInvocationForTest("list_agent_database_candidates", 1, []map[string]interface{}{
				{"id": "db-1", "name": "\u6d4b\u8bd5\u5e931"},
			}),
			agentCandidateLookupInvocationForTest("list_agent_database_tables", 2, []map[string]interface{}{
				{"id": "db-1:table-1", "name": "test1"},
			}),
			agentCandidateLookupInvocationForTest("list_agent_workflow_binding_candidates", 1, []map[string]interface{}{
				{"id": "workflow-1", "name": "\u6d4b\u8bd5\u5de5\u4f5c\u6d41"},
			}),
		},
	}

	answer, ok := FastPathFinalAnswerForCompletionEvidence(evidence)
	if !ok {
		t.Fatal("FastPathFinalAnswerForCompletionEvidence() ok = false, want read-only candidate lookup closed")
	}
	for _, want := range []string{
		"\u672a\u4fee\u6539\u914d\u7f6e",
		"\u672a\u53d1\u8d77\u5ba1\u6279",
		"Skill\uff1a20 \u4e2a",
		"\u77e5\u8bc6\u5e93\uff1a2 \u4e2a",
		"\u6570\u636e\u5e93\uff1a1 \u4e2a",
		"\u6570\u636e\u5e93\u8868\uff1a2 \u4e2a",
		"\u5de5\u4f5c\u6d41\uff1a1 \u4e2a",
		"\u56fe\u8868\u751f\u6210\u5668",
		"test1",
	} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want substring %q", answer, want)
		}
	}
}

func TestFastPathCompletionEvidenceAnswersUnboundSkillBackedCapabilityStatus(t *testing.T) {
	request := "\u8fd9\u4e2a Agent \u80fd\u751f\u6210\u6587\u4ef6\u5417"
	evidence := map[string]interface{}{
		"user_request": request,
		"operation_plan": map[string]interface{}{
			"original_user_goal": request,
			"intent":             "inspect_agent_config",
			"capability_goals": []interface{}{
				map[string]interface{}{
					"capability_id":   "agent.skill_backed_capability",
					"candidate_tool":  "list_agent_skill_candidates",
					"candidate_query": "file generation",
					"required_binding_actions": map[string]interface{}{
						"enabled_skill_ids": "bind",
					},
				},
			},
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/get_agent_config",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "get_agent_config",
				},
				map[string]interface{}{
					"id":        "tool:agent-management/list_agent_skill_candidates",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "list_agent_skill_candidates",
				},
			},
		},
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
				"result": map[string]interface{}{
					"status":     "completed",
					"agent_name": "Novel Agent",
					"config": map[string]interface{}{
						"enabled_skill_ids": []interface{}{"calculator"},
					},
				},
			},
			agentCandidateLookupInvocationForTest("list_agent_skill_candidates", 1, []map[string]interface{}{
				{"id": "file-generator", "name": "File Generator"},
			}),
		},
	}

	answer, ok := FastPathFinalAnswerForCompletionEvidence(evidence)
	if !ok {
		t.Fatal("FastPathFinalAnswerForCompletionEvidence() ok = false, want skill-backed capability status answer")
	}
	for _, want := range []string{
		"Novel Agent",
		"\u5c1a\u672a\u5177\u5907",
		"File Generator",
		"file-generator",
		"\u672a\u4fee\u6539\u914d\u7f6e",
		"\u672a\u53d1\u8d77\u5ba1\u6279",
	} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want substring %q", answer, want)
		}
	}
	if strings.Contains(answer, "\u5df2\u5177\u5907\u8be5\u6280\u80fd\u578b\u80fd\u529b") {
		t.Fatalf("answer = %q, want no enabled capability claim", answer)
	}
}

func TestFastPathCompletionEvidenceAnswersBoundSkillBackedCapabilityStatus(t *testing.T) {
	request := "\u8fd9\u4e2a Agent \u80fd\u751f\u6210\u6587\u4ef6\u5417"
	evidence := map[string]interface{}{
		"user_request": request,
		"operation_plan": map[string]interface{}{
			"original_user_goal": request,
			"intent":             "inspect_agent_config",
			"capability_goals": []interface{}{
				map[string]interface{}{
					"capability_id":   "agent.skill_backed_capability",
					"candidate_tool":  "list_agent_skill_candidates",
					"candidate_query": "file generation",
					"required_binding_actions": map[string]interface{}{
						"enabled_skill_ids": "bind",
					},
				},
			},
		},
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
				"result": map[string]interface{}{
					"status":     "completed",
					"agent_name": "Writer Agent",
					"config": map[string]interface{}{
						"enabled_skill_ids": []interface{}{"file-generator"},
					},
				},
			},
			agentCandidateLookupInvocationForTest("list_agent_skill_candidates", 1, []map[string]interface{}{
				{"id": "file-generator", "name": "File Generator"},
			}),
		},
	}

	answer, ok := FastPathFinalAnswerForCompletionEvidence(evidence)
	if !ok {
		t.Fatal("FastPathFinalAnswerForCompletionEvidence() ok = false, want bound skill capability status answer")
	}
	for _, want := range []string{
		"Writer Agent",
		"\u5df2\u5177\u5907\u8be5\u6280\u80fd\u578b\u80fd\u529b",
		"File Generator",
		"file-generator",
		"\u672a\u4fee\u6539\u914d\u7f6e",
	} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want substring %q", answer, want)
		}
	}
}

func TestFastPathCompletionEvidenceAnswersSkillBackedStatusFromSummaryRefs(t *testing.T) {
	request := "\u8fd9\u4e2a Agent \u80fd\u751f\u6210\u6587\u4ef6\u5417"
	evidence := map[string]interface{}{
		"user_request": request,
		"operation_plan": map[string]interface{}{
			"original_user_goal": request,
			"intent":             "inspect_agent_config",
			"capability_goals": []interface{}{
				map[string]interface{}{
					"capability_id":   "agent.skill_backed_capability",
					"candidate_tool":  "list_agent_skill_candidates",
					"candidate_query": "file generation",
					"required_binding_actions": map[string]interface{}{
						"enabled_skill_ids": "bind",
					},
				},
			},
		},
		"execution_summary": map[string]interface{}{
			"tool_results": []interface{}{
				map[string]interface{}{
					"kind":      "tool_call",
					"status":    "success",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "get_agent_config",
					"result_summary": map[string]interface{}{
						"status":              "completed",
						"agent_name":          "Compressed Agent",
						"enabled_skill_count": 1,
						"enabled_skill_refs":  []interface{}{"file-generator"},
					},
				},
				map[string]interface{}{
					"kind":      "tool_call",
					"status":    "success",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "list_agent_skill_candidates",
					"result_summary": map[string]interface{}{
						"status": "completed",
						"candidate_samples": []interface{}{
							map[string]interface{}{"id": "file-generator", "name": "File Generator"},
						},
					},
				},
			},
		},
	}

	answer, ok := FastPathFinalAnswerForCompletionEvidence(evidence)
	if !ok {
		t.Fatal("FastPathFinalAnswerForCompletionEvidence() ok = false, want compressed summary capability status answer")
	}
	for _, want := range []string{
		"Compressed Agent",
		"\u5df2\u5177\u5907\u8be5\u6280\u80fd\u578b\u80fd\u529b",
		"File Generator",
		"file-generator",
		"\u672a\u4fee\u6539\u914d\u7f6e",
	} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want substring %q", answer, want)
		}
	}
}

func TestFastPathCompletionEvidenceAnswersBooleanAgentCapabilityStatus(t *testing.T) {
	request := "\u8fd9\u4e2a Agent \u80fd\u4e0a\u4f20\u6587\u4ef6\u5417\uff1f\u5b83\u6709\u8bb0\u5fc6\u5417\uff1f"
	evidence := map[string]interface{}{
		"user_request": request,
		"operation_plan": map[string]interface{}{
			"original_user_goal": request,
			"intent":             "inspect_agent_config",
			"capability_goals": []interface{}{
				map[string]interface{}{
					"capability_id":          "agent.accept_uploaded_files",
					"required_config_fields": []interface{}{"file_upload_enabled"},
				},
				map[string]interface{}{
					"capability_id":          "agent.memory",
					"required_config_fields": []interface{}{"agent_memory_enabled"},
				},
			},
		},
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
				"result": map[string]interface{}{
					"status":     "completed",
					"agent_name": "Config Agent",
					"config": map[string]interface{}{
						"file_upload_enabled":  true,
						"agent_memory_enabled": false,
					},
				},
			},
		},
	}

	answer, ok := FastPathFinalAnswerForCompletionEvidence(evidence)
	if !ok {
		t.Fatal("FastPathFinalAnswerForCompletionEvidence() ok = false, want boolean capability status answer")
	}
	for _, want := range []string{
		"Config Agent",
		"\u6587\u4ef6\u4e0a\u4f20\uff1a\u5df2\u5f00\u542f",
		"\u8bb0\u5fc6\uff1a\u672a\u5f00\u542f",
		"\u672a\u4fee\u6539\u914d\u7f6e",
		"\u672a\u53d1\u8d77\u5ba1\u6279",
	} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want substring %q", answer, want)
		}
	}
}

func TestFastPathCompletionEvidenceReportsMissingRequestedAgentCandidate(t *testing.T) {
	missingName := "NEVER-EXISTS-KB-1783071933871"
	request := "\u8bf7\u5c1d\u8bd5\u628a\u540d\u4e3a " + missingName + " \u7684\u77e5\u8bc6\u5e93\u7ed1\u5b9a\u5230\u667a\u80fd\u4f53 AICHAT-GOAL-EDIT\u3002\u8bf7\u5148\u641c\u7d22\u53ef\u7ed1\u5b9a\u77e5\u8bc6\u5e93\u5019\u9009\uff0c\u5982\u679c\u6ca1\u6709\u8fd9\u4e2a\u77e5\u8bc6\u5e93\uff0c\u4e0d\u8981\u53d1\u8d77\u5ba1\u6279\uff0c\u4e0d\u8981\u8c03\u7528 update_agent_config\uff0c\u76f4\u63a5\u544a\u8bc9\u6211\u627e\u4e0d\u5230\u3002"
	evidence := map[string]interface{}{
		"user_request": request,
		"operation_plan": map[string]interface{}{
			"original_user_goal": request,
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/list_agent_knowledge_candidates",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "list_agent_knowledge_candidates",
				},
			},
		},
		"skill_invocations": []interface{}{
			agentCandidateLookupInvocationForTest("list_agent_knowledge_candidates", 2, []map[string]interface{}{
				{"id": "kb-1", "name": "\u6d4b\u8bd5\u5e932"},
				{"id": "kb-2", "name": "\u6d4b\u8bd5\u5e93"},
			}),
		},
	}

	answer, ok := FastPathFinalAnswerForCompletionEvidence(evidence)
	if !ok {
		t.Fatal("FastPathFinalAnswerForCompletionEvidence() ok = false, want candidate miss closed without approval")
	}
	for _, want := range []string{
		"\u672a\u627e\u5230\u540d\u4e3a\u300c" + missingName + "\u300d\u7684\u77e5\u8bc6\u5e93",
		"\u672a\u4fee\u6539\u914d\u7f6e",
		"\u672a\u53d1\u8d77\u5ba1\u6279",
		"\u77e5\u8bc6\u5e93\uff1a2 \u4e2a",
		"\u6d4b\u8bd5\u5e932",
	} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want substring %q", answer, want)
		}
	}
	for _, forbidden := range []string{
		"\u5df2\u7ed1\u5b9a",
		"\u5df2\u66f4\u65b0",
		"\u5df2\u4fee\u6539",
	} {
		if strings.Contains(answer, forbidden) {
			t.Fatalf("answer = %q, want no misleading success substring %q", answer, forbidden)
		}
	}
}

func TestFastPathCompletionEvidenceWaitsForGenericBindableResourceSweep(t *testing.T) {
	request := "Read-only inspect the first Agent basic info, runtime config, editable items, and the current workspace bindable resources count. Do not modify, bind, unbind, create, or delete assets."
	evidence := map[string]interface{}{
		"user_request": request,
		"operation_plan": map[string]interface{}{
			"original_user_goal": request,
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/list_agent_knowledge_candidates",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "list_agent_knowledge_candidates",
				},
			},
		},
		"skill_invocations": []interface{}{
			agentCandidateLookupInvocationForTest("list_agent_knowledge_candidates", 2, []map[string]interface{}{
				{"id": "kb-1", "name": "Support KB"},
			}),
		},
	}

	required := fastPathRequiredAgentCandidateLookupTools(evidence)
	for _, want := range []string{
		"list_agent_skill_candidates",
		"list_agent_knowledge_candidates",
		"list_agent_database_candidates",
		"list_agent_database_tables",
		"list_agent_workflow_binding_candidates",
	} {
		found := false
		for _, got := range required {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("required tools = %#v, want %q for generic bindable resource sweep", required, want)
		}
	}
	if answer, ok := FastPathFinalAnswerForCompletionEvidence(evidence); ok {
		t.Fatalf("FastPathFinalAnswerForCompletionEvidence() = %q, true; want false until all generic bindable resource categories and config run", answer)
	}

	evidence["skill_invocations"] = []interface{}{
		map[string]interface{}{
			"kind":      "tool_call",
			"status":    "success",
			"skill_id":  skills.SkillAgentManagement,
			"tool_name": "get_agent_config",
			"result": map[string]interface{}{
				"status":     "completed",
				"agent_name": "Support Agent",
				"config": map[string]interface{}{
					"model_provider":          "deepseek",
					"model":                   "deepseek-chat",
					"home_title":              "Support home",
					"enabled_skill_count":     1,
					"knowledge_dataset_count": 0,
					"database_table_count":    2,
					"workflow_binding_count":  1,
				},
			},
		},
		agentCandidateLookupInvocationForTest("list_agent_skill_candidates", 3, []map[string]interface{}{
			{"id": "chart-generator", "name": "Chart Generator"},
		}),
		agentCandidateLookupInvocationForTest("list_agent_knowledge_candidates", 2, []map[string]interface{}{
			{"id": "kb-1", "name": "Support KB"},
		}),
		agentCandidateLookupInvocationForTest("list_agent_database_candidates", 1, []map[string]interface{}{
			{"id": "db-1", "name": "Support DB"},
		}),
		agentCandidateLookupInvocationForTest("list_agent_database_tables", 2, []map[string]interface{}{
			{"id": "db-1:table-1", "name": "tickets"},
		}),
		agentCandidateLookupInvocationForTest("list_agent_workflow_binding_candidates", 1, []map[string]interface{}{
			{"id": "workflow-1", "name": "Support Flow"},
		}),
	}

	answer, ok := FastPathFinalAnswerForCompletionEvidence(evidence)
	if !ok {
		t.Fatalf("FastPathFinalAnswerForCompletionEvidence() ok = false, want completed generic resource sweep with config evidence; explicit_candidate=%v config_summary=%v blocked_by_mutation=%v required=%#v results=%#v",
			fastPathGoalExplicitlyRequestsAgentCandidateLookup(evidence),
			fastPathGoalRequestsAgentConfigSummaryWithCandidates(evidence),
			fastPathReadOnlyAgentConfigBlockedByMutation(evidence),
			fastPathRequiredAgentCandidateLookupTools(evidence),
			fastPathSuccessfulAgentCandidateLookupResults(evidence),
		)
	}
	for _, want := range []string{
		"\u5f53\u524d\u667a\u80fd\u4f53\u914d\u7f6e",
		"\u53ef\u7f16\u8f91\u9879\u76ee",
		"Skill\uff1a3 \u4e2a",
		"\u77e5\u8bc6\u5e93\uff1a2 \u4e2a",
		"\u6570\u636e\u5e93\uff1a1 \u4e2a",
		"\u6570\u636e\u5e93\u8868\uff1a2 \u4e2a",
		"\u5de5\u4f5c\u6d41\uff1a1 \u4e2a",
		"Support Flow",
	} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want substring %q", answer, want)
		}
	}
}

func TestFastPathCompletionEvidenceDoesNotCloseCandidateLookupWithPendingAgentMutation(t *testing.T) {
	request := "\u628a\u56fe\u8868\u751f\u6210\u5668\u7ed1\u5b9a\u5230\u5f53\u524d\u667a\u80fd\u4f53"
	evidence := map[string]interface{}{
		"user_request": request,
		"operation_plan": map[string]interface{}{
			"original_user_goal": request,
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_config",
					"status":    "pending",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_config",
				},
			},
		},
		"skill_invocations": []interface{}{
			agentCandidateLookupInvocationForTest("list_agent_skill_candidates", 1, []map[string]interface{}{
				{"id": "chart-generator", "name": "\u56fe\u8868\u751f\u6210\u5668"},
			}),
		},
	}

	if answer, ok := FastPathFinalAnswerForCompletionEvidence(evidence); ok {
		t.Fatalf("FastPathFinalAnswerForCompletionEvidence() = %q, true; want false with pending mutation", answer)
	}
}

func TestFastPathCompletionEvidenceClosesReadOnlyCandidateLookupDespiteStaleMutationPlan(t *testing.T) {
	request := "\u5192\u70df\u9a8c\u8bc18c\uff1a\u53ea\u8bfb\u67e5\u770b\u667a\u80fd\u4f53\u5f53\u524d\u53ef\u7ed1\u5b9a\u7684 Skill\u3001\u77e5\u8bc6\u5e93\u3001\u6570\u636e\u5e93\u8868\u3001\u5de5\u4f5c\u6d41\u5019\u9009\u5404\u6709\u54ea\u4e9b\u3002\u4e0d\u8981\u4fee\u6539\u914d\u7f6e\uff0c\u4e0d\u8981\u53d1\u8d77\u5ba1\u6279\u3002"
	evidence := map[string]interface{}{
		"user_request": request,
		"operation_plan": map[string]interface{}{
			"original_user_goal": request,
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/list_agent_skill_candidates",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "list_agent_skill_candidates",
				},
				map[string]interface{}{
					"id":        "tool:agent-management/list_agent_knowledge_candidates",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "list_agent_knowledge_candidates",
				},
				map[string]interface{}{
					"id":        "tool:agent-management/list_agent_database_candidates",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "list_agent_database_candidates",
				},
				map[string]interface{}{
					"id":        "tool:agent-management/list_agent_database_tables",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "list_agent_database_tables",
				},
				map[string]interface{}{
					"id":        "tool:agent-management/list_agent_workflow_binding_candidates",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "list_agent_workflow_binding_candidates",
				},
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_config",
					"status":    "pending",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_config",
				},
			},
		},
		"skill_invocations": []interface{}{
			agentCandidateLookupInvocationForTest("list_agent_skill_candidates", 1, []map[string]interface{}{
				{"id": "chart-generator", "name": "\u56fe\u8868\u751f\u6210\u5668"},
			}),
			agentCandidateLookupGovernanceInvocationForTest("list_agent_skill_candidates"),
			agentCandidateLookupInvocationForTest("list_agent_knowledge_candidates", 1, []map[string]interface{}{
				{"id": "kb-1", "name": "\u6d4b\u8bd5\u5e932"},
			}),
			agentCandidateLookupGovernanceInvocationForTest("list_agent_knowledge_candidates"),
			agentCandidateLookupInvocationForTest("list_agent_database_candidates", 1, []map[string]interface{}{
				{"id": "db-1", "name": "\u6d4b\u8bd5\u5e931"},
			}),
			agentCandidateLookupGovernanceInvocationForTest("list_agent_database_candidates"),
			agentCandidateLookupInvocationForTest("list_agent_database_tables", 1, []map[string]interface{}{
				{"id": "db-1:table-1", "name": "test1"},
			}),
			agentCandidateLookupGovernanceInvocationForTest("list_agent_database_tables"),
			agentCandidateLookupInvocationForTest("list_agent_workflow_binding_candidates", 1, []map[string]interface{}{
				{"id": "workflow-1", "name": "\u6d4b\u8bd5\u5de5\u4f5c\u6d41"},
			}),
			agentCandidateLookupGovernanceInvocationForTest("list_agent_workflow_binding_candidates"),
		},
	}

	answer, ok := FastPathFinalAnswerForCompletionEvidence(evidence)
	if !ok {
		t.Fatal("FastPathFinalAnswerForCompletionEvidence() ok = false, want explicit read-only candidate lookup to ignore stale mutation plan")
	}
	for _, want := range []string{
		"\u672a\u4fee\u6539\u914d\u7f6e",
		"\u672a\u53d1\u8d77\u5ba1\u6279",
		"Skill\uff1a1 \u4e2a",
		"\u56fe\u8868\u751f\u6210\u5668",
	} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want substring %q", answer, want)
		}
	}
}

func agentCandidateLookupGovernanceInvocationForTest(toolName string) map[string]interface{} {
	return map[string]interface{}{
		"kind":      "tool_governance",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": toolName,
		"result": map[string]interface{}{
			"approval_event": nil,
		},
	}
}

func agentCandidateLookupInvocationForTest(toolName string, count int, samples []map[string]interface{}) map[string]interface{} {
	items := make([]interface{}, 0, len(samples))
	for _, sample := range samples {
		items = append(items, sample)
	}
	return map[string]interface{}{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": toolName,
		"result": map[string]interface{}{
			"status":            "completed",
			"count":             count,
			"candidates_count":  count,
			"candidate_samples": items,
		},
	}
}

func TestFastPathCompletionEvidencePrefersAgentDeleteToolCallOverGovernanceWrapper(t *testing.T) {
	answer, ok := FastPathFinalAnswerForCompletionEvidence(map[string]interface{}{
		"user_request": "delete the current test agent",
		"operation_plan": map[string]interface{}{
			"status": "running",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/delete_agents",
					"status":    "pending",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "delete_agents",
				},
				map[string]interface{}{
					"id":        "tool:agent-management/delete_agent",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "delete_agent",
				},
			},
			"step_status": map[string]interface{}{
				"tool:agent-management/delete_agents": "pending",
				"tool:agent-management/delete_agent":  "completed",
			},
			"tool_result": map[string]interface{}{
				"kind":      "client_action",
				"status":    "succeeded",
				"skill_id":  skills.SkillConsoleNavigator,
				"tool_name": "navigate",
			},
		},
		"execution_summary": map[string]interface{}{
			"tool_results": []interface{}{
				map[string]interface{}{
					"kind":      "tool_governance",
					"status":    "success",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "delete_agent",
				},
				map[string]interface{}{
					"kind":      "tool_call",
					"status":    "success",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "delete_agent",
					"result_summary": map[string]interface{}{
						"status":     "completed",
						"effect":     "deleted",
						"agent_id":   "agent-1",
						"agent_name": "Agent One",
					},
				},
			},
		},
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "delete_agent",
				"result": map[string]interface{}{
					"status":     "completed",
					"effect":     "deleted",
					"agent_id":   "agent-1",
					"agent_name": "Agent One",
				},
			},
			map[string]interface{}{
				"kind":      "tool_governance",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "delete_agent",
				"asset_operation_audit": map[string]interface{}{
					"assets": []interface{}{
						map[string]interface{}{"id": "agent-1", "name": "Agent One", "type": "agent"},
					},
				},
			},
		},
	})

	if !ok {
		t.Fatal("FastPathFinalAnswerForCompletionEvidence() ok = false, want delete tool result to close after navigation")
	}
	if !strings.Contains(answer, "Agent One") {
		t.Fatalf("answer = %q, want deleted agent name from tool call result", answer)
	}
	if strings.Contains(answer, "指定智能体") {
		t.Fatalf("answer = %q, want no generic deleted agent fallback", answer)
	}
}
