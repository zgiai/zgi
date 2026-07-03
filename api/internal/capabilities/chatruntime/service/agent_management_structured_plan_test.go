package service

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestAgentManagementStructuredPlanCapturesBindingUpdate(t *testing.T) {
	query := "\u8bf7\u5bf9\u521a\u521b\u5efa\u7684 GOAL-BIND-SMOKE-1783069834712 \u505a\u914d\u7f6e\u53d8\u66f4\uff1a\u5148\u67e5\u770b\u5f53\u524d\u914d\u7f6e\u548c\u53ef\u7ed1\u5b9a\u7684 Skill\u3001\u77e5\u8bc6\u5e93\u3001\u6570\u636e\u5e93\u8868\u3001\u5de5\u4f5c\u6d41\u5019\u9009\uff1b\u82e5\u6bcf\u7c7b\u6709\u53ef\u7528\u5019\u9009\uff0c\u8bf7\u5404\u7ed1\u5b9a 1 \u4e2a\u5230\u8fd9\u4e2a\u667a\u80fd\u4f53\u3002\u8bf7\u4f18\u5148\u7528 update_agent_config \u4e00\u6b21\u63d0\u4ea4\u8fd9\u4e9b\u7ed1\u5b9a\u3002"
	parts := &chatRequestParts{
		Query:     query,
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillAgentManagement},
	}

	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{Intent: "manage_agent_asset"})
	plan := strategy.StructuredPlan
	if plan == nil {
		t.Fatal("strategy.StructuredPlan = nil, want Agent-management structured plan")
	}
	if got := plan.SchemaVersion; got != aiChatStructuredPlanVersion {
		t.Fatalf("structured plan schema = %q, want %q", got, aiChatStructuredPlanVersion)
	}
	if got := plan.Intent; got != "agent.update_bindings" {
		t.Fatalf("structured plan intent = %q, want agent.update_bindings; plan=%#v", got, plan)
	}
	for _, want := range []string{
		"get_agent_config",
		"list_agent_skill_candidates",
		"list_agent_knowledge_candidates",
		"list_agent_database_candidates",
		"list_agent_database_tables",
		"list_agent_workflow_binding_candidates",
		"update_agent_config",
	} {
		if !structuredPlanHasTool(plan, want) {
			t.Fatalf("structured plan required tools = %#v, missing %s", plan.RequiredToolSequence, want)
		}
	}
	for _, want := range []struct {
		action       string
		resourceType string
	}{
		{action: "read", resourceType: "agent_config"},
		{action: "read_candidates", resourceType: "knowledge_base"},
		{action: "read_candidates", resourceType: "database_table"},
		{action: "read_candidates", resourceType: "workflow"},
		{action: "bind", resourceType: "skill"},
		{action: "bind", resourceType: "knowledge_base"},
		{action: "bind", resourceType: "database_table"},
		{action: "bind", resourceType: "workflow"},
	} {
		if !structuredPlanHasOperation(plan, want.action, want.resourceType) {
			t.Fatalf("structured plan operations = %#v, missing %s/%s", plan.Operations, want.action, want.resourceType)
		}
	}
	if len(plan.ValidationWarnings) != 0 {
		t.Fatalf("structured plan warnings = %#v, want none", plan.ValidationWarnings)
	}
}

func TestAgentManagementStructuredPlanDoesNotCreateForExistingReference(t *testing.T) {
	query := "\u8bf7\u5bf9\u521a\u521b\u5efa\u7684 GOAL-BIND-SMOKE-1783069834712 \u505a\u914d\u7f6e\u53d8\u66f4\uff0c\u67e5\u770b\u5f53\u524d\u914d\u7f6e\u540e\u7ed1\u5b9a\u4e00\u4e2a\u77e5\u8bc6\u5e93\u3002"
	parts := &chatRequestParts{
		Query:     query,
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillAgentManagement},
	}

	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{
		Intent: "manage_agent_asset",
		PlannedTools: []AIChatTurnStrategyTool{
			{SkillID: skills.SkillAgentManagement, ToolName: "create_agent"},
		},
	})
	plan := strategy.StructuredPlan
	if plan == nil {
		t.Fatal("strategy.StructuredPlan = nil, want Agent-management structured plan")
	}
	if structuredPlanHasTool(plan, "create_agent") {
		t.Fatalf("structured plan tools = %#v, want no stale create_agent", plan.RequiredToolSequence)
	}
	if structuredPlanHasOperation(plan, "create", "agent") {
		t.Fatalf("structured plan operations = %#v, want no Agent create operation", plan.Operations)
	}
	if !structuredPlanHasOperation(plan, "bind", "knowledge_base") {
		t.Fatalf("structured plan operations = %#v, missing knowledge_base bind operation", plan.Operations)
	}
}

func TestAgentManagementStructuredPlanIncludedInOperationPlanState(t *testing.T) {
	query := "\u5220\u9664\u5f53\u524d\u9875\u9762\u7684\u524d\u4e24\u4e2a\u667a\u80fd\u4f53"
	parts := &chatRequestParts{
		Query:     query,
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillAgentManagement},
	}
	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{Intent: "manage_agent_asset"})

	plan := operationPlanFromTurnStrategy("task-agent-structured-plan", parts, strategy)
	structured := mapFromOperationContext(plan["structured_plan"])
	if got := stringFromAny(structured["intent"]); got != "agent.batch_delete" {
		t.Fatalf("operation_plan.structured_plan.intent = %q, want agent.batch_delete; plan=%#v", got, plan)
	}
	state := mapFromOperationContext(plan["strategy_state"])
	stateStructured := mapFromOperationContext(state["structured_plan"])
	if got := stringFromAny(stateStructured["intent"]); got != "agent.batch_delete" {
		t.Fatalf("strategy_state.structured_plan.intent = %q, want agent.batch_delete; state=%#v", got, state)
	}
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "delete_agents")); got != operationPlanStepStatusPending {
		t.Fatalf("delete_agents status = %q, want pending; plan=%#v", got, plan)
	}
}

func structuredPlanHasTool(plan *AIChatStructuredPlan, toolName string) bool {
	if plan == nil {
		return false
	}
	return agentManagementStructuredHasTool(plan.RequiredToolSequence, toolName)
}

func structuredPlanHasOperation(plan *AIChatStructuredPlan, action string, resourceType string) bool {
	if plan == nil {
		return false
	}
	for _, operation := range plan.Operations {
		if operation.Action == action && operation.ResourceType == resourceType {
			return true
		}
	}
	return false
}
