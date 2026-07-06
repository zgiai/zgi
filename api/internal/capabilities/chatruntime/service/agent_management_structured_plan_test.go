package service

import (
	"encoding/json"
	"strings"
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
	if strategy == nil {
		t.Fatal("strategy = nil, want Agent-management strategy")
	}
	if got := strategy.ToolChoiceMode; got != aiChatTurnToolChoiceModelDecides {
		t.Fatalf("ToolChoiceMode = %q, want %q; strategy=%#v", got, aiChatTurnToolChoiceModelDecides, strategy)
	}
	if strategy.StructuredPlan != nil || len(strategy.PlannedTools) != 0 {
		t.Fatalf("strategy has scripted tools: structured=%#v planned=%#v, want semantic model-decides strategy", strategy.StructuredPlan, strategy.PlannedTools)
	}
	for _, want := range []string{agentCapabilityKnowledgeBinding, agentCapabilityDatabaseBinding, agentCapabilityWorkflowBinding} {
		if !agentCapabilityGoalsContain(strategy.CapabilityGoals, want) {
			t.Fatalf("capability goals = %#v, missing %s", strategy.CapabilityGoals, want)
		}
	}
	operationPlan := operationPlanFromTurnStrategy("task-agent-binding-update", parts, strategy)
	if got := stringFromAny(operationPlan["planning_mode"]); got != "phase_only_model_decides" {
		t.Fatalf("planning_mode = %q, want phase_only_model_decides; plan=%#v", got, operationPlan)
	}
	if steps := mapSliceFromAny(operationPlan["steps"]); len(steps) != 0 {
		t.Fatalf("operation plan steps = %#v, want no scripted required tool sequence", steps)
	}
	for _, want := range []string{agentCapabilityKnowledgeBinding, agentCapabilityDatabaseBinding, agentCapabilityWorkflowBinding} {
		if !operationPlanCapabilityGoalsContainForTest(mapSliceFromAny(operationPlan["capability_goals"]), want) {
			t.Fatalf("operation plan capability_goals = %#v, missing %s", operationPlan["capability_goals"], want)
		}
	}
}

func TestAgentManagementStructuredPlanBindsCreateThenEditTarget(t *testing.T) {
	parts := &chatRequestParts{
		Query:     "create a new novelist agent, then set its model to gpt-4o",
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillAgentManagement},
	}
	strategy := attachAgentManagementStructuredPlan(parts, &AIChatTurnStrategy{
		Intent: "manage_agent_asset",
		PlannedTools: []AIChatTurnStrategyTool{
			{
				SkillID:  skills.SkillAgentManagement,
				ToolName: "create_agent",
				Arguments: map[string]string{
					"name": "Novelist",
				},
			},
			{
				SkillID:  skills.SkillAgentManagement,
				ToolName: "update_agent_config",
				Arguments: map[string]string{
					"model_provider": "openai",
					"model":          "gpt-4o",
				},
			},
		},
	}, parts.Query)
	if strategy == nil || strategy.StructuredPlan == nil {
		t.Fatalf("StructuredPlan = %#v, want create-and-edit plan", strategy)
	}
	tools := strategy.StructuredPlan.RequiredToolSequence
	if len(tools) != 2 {
		t.Fatalf("required tools = %#v, want create and update", tools)
	}
	if got := tools[0].OutputAlias; got != aiChatStructuredCreatedAgentsOutputAlias {
		t.Fatalf("create output alias = %q, want %q", got, aiChatStructuredCreatedAgentsOutputAlias)
	}
	if got := tools[1].ArgsBinding["agent_id"]; got != aiChatStructuredFirstCreatedAgentIDExpr {
		t.Fatalf("update args_binding.agent_id = %q, want %q", got, aiChatStructuredFirstCreatedAgentIDExpr)
	}

	operationPlan := operationPlanFromTurnStrategy("task-create-edit-binding", parts, strategy)
	structured := mapFromOperationContext(operationPlan["structured_plan"])
	requiredTools := mapSliceFromAny(structured["required_tool_sequence"])
	if len(requiredTools) != 2 {
		t.Fatalf("operation_plan structured tools = %#v, want two tools", requiredTools)
	}
	createTool := requiredTools[0]
	if got := stringFromAny(createTool["output_alias"]); got != aiChatStructuredCreatedAgentsOutputAlias {
		t.Fatalf("operation_plan create output_alias = %q, want %q", got, aiChatStructuredCreatedAgentsOutputAlias)
	}
	updateBinding := cleanStringAnyStringMap(mapFromOperationContext(requiredTools[1]["args_binding"]))
	if got := updateBinding["agent_id"]; got != aiChatStructuredFirstCreatedAgentIDExpr {
		t.Fatalf("operation_plan update args_binding.agent_id = %q, want %q", got, aiChatStructuredFirstCreatedAgentIDExpr)
	}
}

func TestAgentManagementStructuredPlanBindsCreateThenCapabilityConfigTarget(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "创建一个小说创作智能体，让它能生成文件、能上传文件，并使用适合复杂推理的模型。",
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents",
		SkillMode:      skillModeAuto,
		SkillIDs:       []string{skills.SkillAgentManagement},
	}

	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("strategy = nil, want Agent-management model-decides strategy")
	}
	if strategy.StructuredPlan != nil || len(strategy.PlannedTools) != 0 {
		t.Fatalf("strategy has scripted tools: structured=%#v planned=%#v, want phase-only model-decides strategy", strategy.StructuredPlan, strategy.PlannedTools)
	}
	for _, want := range []string{agentCapabilitySkillBacked, agentCapabilityAcceptUploaded, agentCapabilityModelSelection} {
		if !agentCapabilityGoalsContain(strategy.CapabilityGoals, want) {
			t.Fatalf("capability goals = %#v, missing %s", strategy.CapabilityGoals, want)
		}
	}

	operationPlan := operationPlanFromTurnStrategy("task-create-agent-capability-config", parts, strategy)
	if operationPlan == nil {
		t.Fatal("operation plan = nil, want phase-only model-decides plan")
	}
	if got := stringFromAny(operationPlan["planning_mode"]); got != "phase_only_model_decides" {
		t.Fatalf("planning_mode = %q, want phase_only_model_decides; plan=%#v", got, operationPlan)
	}
	if steps := mapSliceFromAny(operationPlan["steps"]); len(steps) != 0 {
		t.Fatalf("operation plan steps = %#v, want no scripted tool steps for model-decides", steps)
	}
	for _, want := range []string{agentCapabilitySkillBacked, agentCapabilityAcceptUploaded, agentCapabilityModelSelection} {
		if !operationPlanCapabilityGoalsContainForTest(mapSliceFromAny(operationPlan["capability_goals"]), want) {
			t.Fatalf("operation plan capability_goals = %#v, missing %s", operationPlan["capability_goals"], want)
		}
	}
}

func TestAgentManagementStructuredPlanBindsCreateThenChineseCapabilityConfigTarget(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "\u521b\u5efa\u4e00\u4e2a\u5c0f\u8bf4\u521b\u4f5c\u667a\u80fd\u4f53\uff0c\u8ba9\u5b83\u80fd\u591f\u751f\u6210\u6587\u4ef6\u3001\u80fd\u591f\u4e0a\u4f20\u6587\u4ef6\uff0c\u5e76\u4f7f\u7528\u9002\u5408\u590d\u6742\u63a8\u7406\u7684\u6a21\u578b\u3002",
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents",
		SkillMode:      skillModeAuto,
		SkillIDs:       []string{skills.SkillAgentManagement},
	}

	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("strategy = nil, want Agent-management model-decides strategy")
	}
	if strategy.StructuredPlan != nil || len(strategy.PlannedTools) != 0 {
		t.Fatalf("strategy has scripted tools: structured=%#v planned=%#v, want phase-only model-decides strategy", strategy.StructuredPlan, strategy.PlannedTools)
	}
	for _, want := range []string{agentCapabilitySkillBacked, agentCapabilityAcceptUploaded, agentCapabilityModelSelection} {
		if !agentCapabilityGoalsContain(strategy.CapabilityGoals, want) {
			t.Fatalf("capability goals = %#v, missing %s", strategy.CapabilityGoals, want)
		}
	}

	operationPlan := operationPlanFromTurnStrategy("task-create-agent-chinese-capability-config", parts, strategy)
	if operationPlan == nil {
		t.Fatal("operation plan = nil, want phase-only model-decides plan")
	}
	if got := stringFromAny(operationPlan["planning_mode"]); got != "phase_only_model_decides" {
		t.Fatalf("planning_mode = %q, want phase_only_model_decides; plan=%#v", got, operationPlan)
	}
	if steps := mapSliceFromAny(operationPlan["steps"]); len(steps) != 0 {
		t.Fatalf("operation plan steps = %#v, want no scripted tool steps for model-decides", steps)
	}
}

func TestAgentManagementStructuredPlanPreservesLookupArguments(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "Switch the current Agent to a model suited for complex reasoning, use_case=reasoning, and enable capability file generation.",
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents/agent-1/agent",
		SkillMode:      skillModeAuto,
		SkillIDs:       []string{skills.SkillAgentManagement},
	}

	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("strategy = nil, want Agent-management model-decides strategy")
	}
	if strategy.StructuredPlan != nil || len(strategy.PlannedTools) != 0 {
		t.Fatalf("strategy has scripted tools: structured=%#v planned=%#v, want phase-only model-decides strategy", strategy.StructuredPlan, strategy.PlannedTools)
	}
	var modelGoal AIChatAgentCapabilityGoal
	for _, goal := range strategy.CapabilityGoals {
		if goal.CapabilityID == agentCapabilityModelSelection {
			modelGoal = goal
			break
		}
	}
	if modelGoal.CapabilityID == "" {
		t.Fatalf("capability goals = %#v, want model selection goal", strategy.CapabilityGoals)
	}
	if got := modelGoal.CandidateTool; got != "list_available_models" {
		t.Fatalf("model capability candidate_tool = %q, want list_available_models; goal=%#v", got, modelGoal)
	}
	if got := modelGoal.CandidateUseCase; got != "reasoning" {
		t.Fatalf("model capability candidate_use_case = %q, want reasoning; goal=%#v", got, modelGoal)
	}
	var skillGoal AIChatAgentCapabilityGoal
	for _, goal := range strategy.CapabilityGoals {
		if goal.CapabilityID == agentCapabilitySkillBacked {
			skillGoal = goal
			break
		}
	}
	if got := skillGoal.CandidateQuery; got != "file generation" {
		t.Fatalf("skill-backed capability candidate_query = %q, want file generation; goal=%#v", got, skillGoal)
	}
}

func TestAgentManagementStructuredPlanUsesCapabilityGoalsForConfigContract(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "继续处理",
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents/agent-1/agent",
		SkillMode:      skillModeAuto,
		SkillIDs:       []string{skills.SkillAgentManagement},
	}
	strategy := attachAgentManagementStructuredPlan(parts, &AIChatTurnStrategy{
		Intent: "manage_agent_asset",
		PlannedTools: []AIChatTurnStrategyTool{
			{SkillID: skills.SkillAgentManagement, ToolName: "get_agent_config"},
			{
				SkillID:  skills.SkillAgentManagement,
				ToolName: "list_agent_skill_candidates",
				Arguments: map[string]string{
					"query": "file generation",
				},
			},
			{SkillID: skills.SkillAgentManagement, ToolName: "update_agent_config"},
		},
		CapabilityGoals: []AIChatAgentCapabilityGoal{{
			CapabilityID:         agentCapabilitySkillBacked,
			GoalAction:           agentCapabilityActionBind,
			DisplayName:          "file generation",
			RequiredConfigFields: []string{"enabled_skill_ids"},
			RequiredBindingActions: map[string]string{
				"enabled_skill_ids": "bind",
			},
			CandidateTool:  "list_agent_skill_candidates",
			CandidateQuery: "file generation",
			VerifyBy:       []string{"get_agent_config.enabled_skill_ids contains the selected Skill"},
		}},
	}, parts.Query)
	if strategy == nil || strategy.StructuredPlan == nil {
		t.Fatalf("StructuredPlan = %#v, want capability-driven Agent-management plan", strategy)
	}
	if got := strategy.StructuredPlan.Intent; got != "agent.update_bindings" {
		t.Fatalf("StructuredPlan.Intent = %q, want agent.update_bindings; plan=%#v", got, strategy.StructuredPlan)
	}
	var bindOperation *AIChatStructuredOperation
	for idx := range strategy.StructuredPlan.Operations {
		operation := &strategy.StructuredPlan.Operations[idx]
		if operation.Action == "bind" && operation.ResourceType == "skill" {
			bindOperation = operation
			break
		}
	}
	if bindOperation == nil {
		t.Fatalf("structured operations = %#v, want semantic skill bind operation from capability goal", strategy.StructuredPlan.Operations)
	}
	if !stringSliceContainsFold(bindOperation.Fields, "enabled_skill_ids") {
		t.Fatalf("bind operation fields = %#v, want enabled_skill_ids; operation=%#v", bindOperation.Fields, bindOperation)
	}
	if len(strategy.StructuredPlan.ValidationWarnings) != 0 {
		t.Fatalf("ValidationWarnings = %#v, want candidate lookup accepted from planned tools", strategy.StructuredPlan.ValidationWarnings)
	}

	plan := operationPlanFromTurnStrategy("task-capability-driven-bind", parts, strategy)
	updateStep := operationPlanStepForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"))
	fields := stringSliceFromAny(updateStep[operationPlanExpectedUpdatedFieldsKey])
	if !stringSliceContainsFold(fields, "enabled_skill_ids") {
		t.Fatalf("operation plan update fields = %#v, want enabled_skill_ids from capability goal; step=%#v plan=%#v", fields, updateStep, plan)
	}
	actions := operationPlanAgentConfigBindingActionsFromAny(updateStep[operationPlanExpectedBindingActionsKey])
	if got := actions["enabled_skill_ids"]; got != "bind" {
		t.Fatalf("operation plan binding actions = %#v, want enabled_skill_ids:bind from capability goal; step=%#v plan=%#v", actions, updateStep, plan)
	}
}

func agentCapabilityGoalsContain(goals []AIChatAgentCapabilityGoal, capabilityID string) bool {
	for _, goal := range goals {
		if goal.CapabilityID == capabilityID {
			return true
		}
	}
	return false
}

func TestContextualSidebarStructuredPlanCoversNavigationTool(t *testing.T) {
	parts := &chatRequestParts{
		Query:     "open file management",
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillConsoleNavigator},
	}
	strategy := attachContextualSidebarStructuredPlan(parts, &AIChatTurnStrategy{
		Intent:     "navigate_console_page",
		TargetPage: "/console/files",
		PlannedTools: []AIChatTurnStrategyTool{{
			SkillID:  skills.SkillConsoleNavigator,
			ToolName: "navigate",
			Arguments: map[string]string{
				"href": "/console/files",
			},
		}},
	}, parts.Query)
	if strategy == nil || strategy.StructuredPlan == nil {
		t.Fatalf("StructuredPlan = %#v, want generic sidebar plan", strategy)
	}
	plan := strategy.StructuredPlan
	if got := plan.Domain; got != "console_navigation" {
		t.Fatalf("structured plan domain = %q, want console_navigation; plan=%#v", got, plan)
	}
	if len(plan.Operations) != 1 {
		t.Fatalf("structured plan operations = %#v, want one navigation operation", plan.Operations)
	}
	operation := plan.Operations[0]
	if operation.SkillID != skills.SkillConsoleNavigator ||
		operation.ToolName != "navigate" ||
		operation.Action != "navigate" ||
		operation.ResourceType != "page" {
		t.Fatalf("navigation operation = %#v, want console-navigator navigate page operation", operation)
	}
	operationPlan := operationPlanFromTurnStrategy("task-generic-sidebar-structured", parts, strategy)
	structured := mapFromOperationContext(operationPlan["structured_plan"])
	compact := operationPlanCompactStructuredPlanForPrompt(structured, 4)
	operations := mapSliceFromAny(compact["operations"])
	if len(operations) != 1 ||
		stringFromAny(operations[0]["action"]) != "navigate" ||
		stringFromAny(operations[0]["resource_type"]) != "page" {
		t.Fatalf("compact structured operations = %#v, want navigation phase without exact tool script", operations)
	}
	if stringFromAny(operations[0]["skill_id"]) != "" || stringFromAny(operations[0]["tool_name"]) != "" {
		t.Fatalf("compact structured operations = %#v, want no model-facing skill/tool prescription", operations)
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
	if strategy == nil {
		t.Fatal("strategy = nil, want Agent-management strategy")
	}
	if got := strategy.ToolChoiceMode; got != aiChatTurnToolChoiceModelDecides {
		t.Fatalf("ToolChoiceMode = %q, want %q; strategy=%#v", got, aiChatTurnToolChoiceModelDecides, strategy)
	}
	if strategy.StructuredPlan != nil || len(strategy.PlannedTools) != 0 {
		t.Fatalf("strategy has scripted tools: structured=%#v planned=%#v, want stale create_agent removed and model decides next tools", strategy.StructuredPlan, strategy.PlannedTools)
	}
	if !agentCapabilityGoalsContainBindingActionForTest(strategy.CapabilityGoals, "knowledge_dataset_ids", "bind") {
		t.Fatalf("capability goals = %#v, want knowledge bind goal for existing Agent reference", strategy.CapabilityGoals)
	}
	operationPlan := operationPlanFromTurnStrategy("task-existing-agent-binding", parts, strategy)
	if got := stringFromAny(operationPlan["planning_mode"]); got != "phase_only_model_decides" {
		t.Fatalf("planning_mode = %q, want phase_only_model_decides; plan=%#v", got, operationPlan)
	}
	if steps := mapSliceFromAny(operationPlan["steps"]); len(steps) != 0 {
		t.Fatalf("operation plan steps = %#v, want no stale create step", steps)
	}
	if !operationPlanCapabilityGoalsContainBindingActionForTest(mapSliceFromAny(operationPlan["capability_goals"]), "knowledge_dataset_ids", "bind") {
		t.Fatalf("operation plan capability_goals = %#v, want knowledge bind goal", operationPlan["capability_goals"])
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
	if got := stringFromAny(plan["planning_mode"]); got != "phase_only_model_decides" {
		t.Fatalf("planning_mode = %q, want phase_only_model_decides; plan=%#v", got, plan)
	}
	if structured := mapFromOperationContext(plan["structured_plan"]); len(structured) != 0 {
		t.Fatalf("operation_plan.structured_plan = %#v, want omitted for Agent-management model-decides flow", structured)
	}
	if steps := mapSliceFromAny(plan["steps"]); len(steps) != 0 {
		t.Fatalf("operation_plan.steps = %#v, want no scripted tool steps", steps)
	}
	if phases := mapSliceFromAny(plan["phases"]); len(phases) == 0 {
		t.Fatalf("operation_plan.phases = %#v, want phase checklist", plan["phases"])
	}
	state := mapFromOperationContext(plan["strategy_state"])
	if stateStructured := mapFromOperationContext(state["structured_plan"]); len(stateStructured) != 0 {
		t.Fatalf("strategy_state.structured_plan = %#v, want omitted for model-decides flow; state=%#v", stateStructured, state)
	}
	message, ok := contextualAIChatTurnStrategyMessage(&PreparedChat{parts: parts})
	if !ok {
		t.Fatal("contextualAIChatTurnStrategyMessage() missing, want phase plan guidance")
	}
	content, ok := message.Content.(string)
	if !ok {
		t.Fatalf("strategy message content type = %T, want string", message.Content)
	}
	for _, want := range []string{
		"planning_contract",
		"phase/status checklist",
		"final answers must be grounded in successful tool results",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("strategy message missing %q in:\n%s", want, content)
		}
	}
	for _, unexpected := range []string{
		"required_tool_sequence",
		"planned_tools",
		"required_next_tool",
	} {
		if strings.Contains(content, unexpected) {
			t.Fatalf("strategy message contains %q, want model-facing plan without exact tool script:\n%s", unexpected, content)
		}
	}
}

func TestAgentManagementModelDecidesPromptHidesExactToolScript(t *testing.T) {
	query := "\u5220\u6389\u9875\u9762\u4e2d\u7684\u7b2c\u4e00\u4e2a\u667a\u80fd\u4f53\uff0c\u7136\u540e\u521b\u5efa\u4e00\u4e2a\u65b0\u7684\u667a\u80fd\u4f53\uff0c\u53d6\u540d\u53eb\u5c0f\u8bf4\u521b\u4f5c\u5927\u5e08\uff0c\u6a21\u578b\u914d\u7f6e\u4e3adeepseek flash\uff0c\u5199\u597d\u63d0\u793a\u8bcd\u9700\u8981\u8ba9agent\u80fd\u751f\u6210\u6587\u4ef6\u548c\u4e0a\u4f20\u6587\u4ef6\u3002"
	parts := &chatRequestParts{
		Query:     query,
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillAgentManagement, skills.SkillConsoleNavigator, skills.SkillFileReader},
	}
	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{
		Intent:         "manage_agent_asset",
		TargetPage:     "/console/agents",
		AssetEffect:    "update",
		AssetRisk:      "medium",
		Approval:       "required",
		ToolChoiceMode: aiChatTurnToolChoiceModelDecides,
	})

	if len(strategy.PlannedTools) != 0 {
		t.Fatalf("strategy.PlannedTools = %#v, want no internal scripted plan for model-decides", strategy.PlannedTools)
	}
	if strategy.StructuredPlan != nil {
		t.Fatalf("strategy.StructuredPlan = %#v, want nil for model-decides phase-only plan", strategy.StructuredPlan)
	}
	if !agentCapabilityGoalListHasCapability(strategy.CapabilityGoals, agentCapabilitySkillBacked) {
		t.Fatalf("strategy capability goals = %#v, missing file generation skill-backed goal", strategy.CapabilityGoals)
	}
	if !agentCapabilityGoalListHasCapability(strategy.CapabilityGoals, agentCapabilityAcceptUploaded) {
		t.Fatalf("strategy capability goals = %#v, missing file upload goal", strategy.CapabilityGoals)
	}

	view := aiChatTurnStrategyPromptView(strategy)
	structured := mapFromOperationContext(view["structured_plan"])
	if len(structured) != 0 {
		t.Fatalf("prompt view structured_plan = %#v, want omitted for model-decides", structured)
	}
	for _, unexpected := range []string{"planned_tools", "required_next_tool", "required_tool_sequence", "candidate_tool", `"tool_name"`, `"skill_id"`} {
		if strings.Contains(string(mustMarshalJSONForTest(t, view)), unexpected) {
			t.Fatalf("prompt view contains %q, want semantic plan without exact tool script: %#v", unexpected, view)
		}
	}

	operationPlan := operationPlanFromTurnStrategy("task-model-decides-prompt", parts, strategy)
	if got := stringFromAny(operationPlan["planning_mode"]); got != "phase_only_model_decides" {
		t.Fatalf("planning_mode = %q, want phase_only_model_decides; plan=%#v", got, operationPlan)
	}
	if steps := mapSliceFromAny(operationPlan["steps"]); len(steps) != 0 {
		t.Fatalf("operation plan steps = %#v, want no scripted steps for model-decides", steps)
	}
	summary := skillLoopCompletionPlanSummary(operationPlan)
	if strings.Contains(string(mustMarshalJSONForTest(t, summary)), "candidate_tool") {
		t.Fatalf("completion summary leaked candidate_tool, want model-facing summary without exact candidate tool: %#v", summary)
	}
}

func TestAgentManagementStructuredPlanStatusFollowsToolResult(t *testing.T) {
	parts := &chatRequestParts{
		Query:     "\u5220\u9664\u5f53\u524d\u9875\u9762\u7684\u524d\u4e24\u4e2a\u667a\u80fd\u4f53",
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillAgentManagement},
	}
	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{Intent: "manage_agent_asset"})
	metadata := map[string]interface{}{
		"operation_plan": operationPlanFromTurnStrategy("task-agent-structured-plan-status", parts, strategy),
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{{
		"kind":       "tool_call",
		"status":     "success",
		"runtime_id": "tool#1",
		"skill_id":   skills.SkillAgentManagement,
		"tool_name":  "delete_agents",
		"result": map[string]interface{}{
			"status":        "completed",
			"target_count":  2,
			"deleted_count": 2,
			"failed_count":  0,
			"operation_group": map[string]interface{}{
				"id":            "agent.delete.batch:structured-plan",
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
	}})

	plan := mapFromOperationContext(metadata["operation_plan"])
	if structured := mapFromOperationContext(plan["structured_plan"]); len(structured) != 0 {
		t.Fatalf("structured_plan = %#v, want omitted for Agent-management model-decides flow", structured)
	}
	ledger := mapSliceFromAny(plan[operationPlanEvidenceLedgerKey])
	if !operationPlanEvidenceLedgerHasToolForTest(ledger, skills.SkillAgentManagement, "delete_agents", "runtime_id:tool#1") {
		t.Fatalf("evidence_ledger = %#v, want completed delete_agents tool evidence", ledger)
	}
	summary := skillLoopCompletionPlanSummary(plan)
	if summaryStructured := mapFromOperationContext(summary["structured_plan"]); len(summaryStructured) != 0 {
		t.Fatalf("summary.structured_plan = %#v, want omitted for model-decides summary", summaryStructured)
	}
}

func TestAgentManagementStructuredPlanStatusFollowsToolFailure(t *testing.T) {
	parts := &chatRequestParts{
		Query:     "\u5220\u9664\u5f53\u524d\u9875\u9762\u7684\u524d\u4e24\u4e2a\u667a\u80fd\u4f53",
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillAgentManagement},
	}
	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{Intent: "manage_agent_asset"})
	metadata := map[string]interface{}{
		"operation_plan": operationPlanFromTurnStrategy("task-agent-structured-plan-failure", parts, strategy),
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{{
		"kind":      "tool_call",
		"status":    "error",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "delete_agents",
		"error":     "delete denied by governance",
	}})

	plan := mapFromOperationContext(metadata["operation_plan"])
	if structured := mapFromOperationContext(plan["structured_plan"]); len(structured) != 0 {
		t.Fatalf("structured_plan = %#v, want omitted for Agent-management model-decides flow", structured)
	}
	if ledger := mapSliceFromAny(plan[operationPlanEvidenceLedgerKey]); operationPlanEvidenceLedgerHasToolForTest(ledger, skills.SkillAgentManagement, "delete_agents", "") {
		t.Fatalf("evidence_ledger = %#v, want no completed evidence for failed delete_agents call", ledger)
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

func agentCapabilityGoalListHasCapability(goals []AIChatAgentCapabilityGoal, capabilityID string) bool {
	for _, goal := range goals {
		if strings.EqualFold(strings.TrimSpace(goal.CapabilityID), strings.TrimSpace(capabilityID)) {
			return true
		}
	}
	return false
}

func mustMarshalJSONForTest(t testing.TB, value interface{}) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal(%T) failed: %v", value, err)
	}
	return encoded
}

func structuredPlanToolForTest(plan *AIChatStructuredPlan, toolName string) AIChatTurnStrategyTool {
	if plan == nil {
		return AIChatTurnStrategyTool{}
	}
	for _, tool := range plan.RequiredToolSequence {
		if strings.EqualFold(tool.ToolName, toolName) {
			return tool
		}
	}
	return AIChatTurnStrategyTool{}
}

func structuredPlanOperationForTool(plan *AIChatStructuredPlan, toolName string) AIChatStructuredOperation {
	if plan == nil {
		return AIChatStructuredOperation{}
	}
	for _, operation := range plan.Operations {
		if strings.EqualFold(operation.ToolName, toolName) {
			return operation
		}
	}
	return AIChatStructuredOperation{}
}

func structuredPlanOperationForActionAndResource(plan *AIChatStructuredPlan, action string, resourceType string) AIChatStructuredOperation {
	if plan == nil {
		return AIChatStructuredOperation{}
	}
	for _, operation := range plan.Operations {
		if strings.EqualFold(strings.TrimSpace(operation.Action), strings.TrimSpace(action)) &&
			strings.EqualFold(strings.TrimSpace(operation.ResourceType), strings.TrimSpace(resourceType)) {
			return operation
		}
	}
	return AIChatStructuredOperation{}
}

func operationPlanEvidenceLedgerHasToolForTest(ledger []map[string]interface{}, skillID string, toolName string, invocationID string) bool {
	for _, entry := range ledger {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(entry["skill_id"])), strings.TrimSpace(skillID)) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(entry["tool_name"])), strings.TrimSpace(toolName)) {
			continue
		}
		if invocationID != "" && strings.TrimSpace(stringFromAny(entry["invocation_id"])) != invocationID {
			continue
		}
		return true
	}
	return false
}

func structuredOperationForTest(plan map[string]interface{}, action string, resourceType string) map[string]interface{} {
	for _, operation := range mapSliceFromAny(plan["operations"]) {
		if stringFromAny(operation["action"]) == action && stringFromAny(operation["resource_type"]) == resourceType {
			return operation
		}
	}
	return nil
}
