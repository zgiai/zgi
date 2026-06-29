package service

import (
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestStreamingMessageMetadataIncludesOperationPlan(t *testing.T) {
	parts := &chatRequestParts{
		Query:     "\u6253\u5f00\u6587\u4ef6\u7ba1\u7406",
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillConsoleNavigator},
	}

	metadata := streamingMessageMetadataWithTaskID(parts, "task-1")
	plan, ok := metadata["operation_plan"].(map[string]interface{})
	if !ok {
		t.Fatalf("operation_plan = %#v, want map", metadata["operation_plan"])
	}
	if plan["version"] != operationPlanVersion {
		t.Fatalf("operation_plan version = %#v, want %q", plan["version"], operationPlanVersion)
	}
	if plan["task_id"] != "task-1" {
		t.Fatalf("task_id = %#v, want task-1", plan["task_id"])
	}
	if plan["original_user_goal"] != parts.Query {
		t.Fatalf("original_user_goal = %#v, want query", plan["original_user_goal"])
	}
	if plan["intent"] != "navigate_console_page" {
		t.Fatalf("intent = %#v, want navigate_console_page", plan["intent"])
	}
	if _, ok := plan["steps"].([]interface{}); !ok {
		t.Fatalf("steps = %#v, want array", plan["steps"])
	}
	if _, ok := plan["step_status"].(map[string]interface{}); !ok {
		t.Fatalf("step_status = %#v, want map", plan["step_status"])
	}
}

func TestOperationPlanForCrossPageAgentCreateIncludesRouteAndCreate(t *testing.T) {
	parts := &chatRequestParts{
		Query:     "请导航到智能体页面，并在当前工作空间创建两个临时测试 Agent 草稿，名称分别为 AICHAT-A 和 AICHAT-B。描述都写为：AIChat smoke。",
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillConsoleNavigator, skills.SkillAgentManagement},
	}

	metadata := streamingMessageMetadataWithTaskID(parts, "task-agent-create")
	plan := metadata["operation_plan"].(map[string]interface{})
	if got := plan["intent"]; got != "manage_agent_asset" {
		t.Fatalf("operation_plan intent = %#v, want manage_agent_asset; plan=%#v", got, plan)
	}
	target := mapFromOperationContext(plan["asset_target"])
	if got := target["page"]; got != "/console/agents" {
		t.Fatalf("operation_plan target_page = %#v, want /console/agents; plan=%#v", got, plan)
	}
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillConsoleNavigator, "navigate")); got != operationPlanStepStatusPending {
		t.Fatalf("operation_plan steps = %#v, want console-navigator/navigate", plan["steps"])
	}
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "create_agent")); got != operationPlanStepStatusPending {
		t.Fatalf("operation_plan steps = %#v, want agent-management/create_agent", plan["steps"])
	}
	if got := operationPlanStepStatusForTest(plan, operationPlanRouteStepID("/console/workspace", 1)); got != "" {
		t.Fatalf("operation_plan unexpectedly routed current workspace phrase with status %q; plan=%#v", got, plan)
	}
}

func TestOperationPlanCompletionCriteriaKeepsEvidenceStepsAdvisory(t *testing.T) {
	criteria := operationPlanCompletionCriteria([]map[string]interface{}{
		{
			"id":        operationPlanRouteStepID("/console/files", 1),
			"title":     "Navigate to Files",
			"status":    operationPlanStepStatusPending,
			"skill_id":  skills.SkillConsoleNavigator,
			"tool_name": "navigate",
			"asset_target": map[string]interface{}{
				"page": "/console/files",
			},
		},
		{
			"id":        operationPlanToolStepID(skills.SkillAgentManagement, "list_agents"),
			"title":     "List agents",
			"status":    operationPlanStepStatusPending,
			"skill_id":  skills.SkillAgentManagement,
			"tool_name": "list_agents",
			"asset_target": map[string]interface{}{
				"effect":     "read",
				"asset_type": "agent",
			},
		},
		{
			"id":        operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"),
			"title":     "Update agent config",
			"status":    operationPlanStepStatusPending,
			"skill_id":  skills.SkillAgentManagement,
			"tool_name": "update_agent_config",
			"asset_target": map[string]interface{}{
				"effect":     "update",
				"asset_type": "agent",
			},
		},
	})
	joined := strings.Join(criteria, "\n")
	if strings.Contains(joined, "Complete required step") {
		t.Fatalf("completion criteria still uses hard-script language: %#v", criteria)
	}
	if strings.Contains(joined, "Navigate to Files") || strings.Contains(joined, "List agents") {
		t.Fatalf("completion criteria = %#v, want read/list/navigation steps advisory", criteria)
	}
	if !strings.Contains(joined, "Update agent config") ||
		!strings.Contains(joined, "Asset-changing step must have matching execution evidence") {
		t.Fatalf("completion criteria = %#v, want asset mutation evidence criterion", criteria)
	}
}

func TestContinuationOperationPlanCarriesPriorGoalAndPendingTool(t *testing.T) {
	parts := &chatRequestParts{
		Query:     "\u7ee7\u7eed",
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs: []string{
			skills.SkillConsoleNavigator,
			skills.SkillFileGenerator,
			skills.SkillFileManager,
		},
		RecentOperationPlans: []map[string]interface{}{{
			"version":            operationPlanVersion,
			"task_id":            "task-prior",
			"original_user_goal": "\u751f\u6210\u4e00\u4e2a SVG \u5e76\u4fdd\u5b58\u5230\u6587\u4ef6\u7ba1\u7406",
			"intent":             "save_generated_file_to_file_management",
			"status":             operationPlanStatusRunning,
			"asset_target": map[string]interface{}{
				"effect": "create",
				"page":   "/console/files",
			},
			"steps": []interface{}{
				map[string]interface{}{
					"id":        operationPlanToolStepID(skills.SkillFileGenerator, "generate_file"),
					"status":    operationPlanStepStatusCompleted,
					"skill_id":  skills.SkillFileGenerator,
					"tool_name": "generate_file",
				},
				map[string]interface{}{
					"id":        operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management"),
					"status":    operationPlanStepStatusPending,
					"skill_id":  skills.SkillFileManager,
					"tool_name": "save_file_to_management",
				},
			},
			"step_status": map[string]interface{}{
				operationPlanToolStepID(skills.SkillFileGenerator, "generate_file"):         operationPlanStepStatusCompleted,
				operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management"): operationPlanStepStatusPending,
			},
			"pending_next_action": "Save generated file to File Management",
		}},
	}

	metadata := streamingMessageMetadataWithTaskID(parts, "task-current")
	plan, ok := metadata["operation_plan"].(map[string]interface{})
	if !ok {
		t.Fatalf("operation_plan = %#v, want map", metadata["operation_plan"])
	}
	if got := plan["original_user_goal"]; got != parts.RecentOperationPlans[0]["original_user_goal"] {
		t.Fatalf("original_user_goal = %#v, want prior goal", got)
	}
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management")); got != operationPlanStepStatusPending {
		t.Fatalf("save step status = %q, want pending; plan=%#v", got, plan)
	}
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillFileGenerator, "generate_file")); got != "" {
		t.Fatalf("current continuation plan unexpectedly re-added generator step with status %q; plan=%#v", got, plan)
	}
}

func TestRestrictResolvedSkillsForTurnStrategyKeepsEnabledSkillsVisible(t *testing.T) {
	parts := &chatRequestParts{
		Query:     "\u7ee7\u7eed",
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs: []string{
			skills.SkillConsoleNavigator,
			skills.SkillFileGenerator,
			skills.SkillFileManager,
		},
		RecentOperationPlans: []map[string]interface{}{{
			"status": operationPlanStatusRunning,
			"steps": []interface{}{
				map[string]interface{}{
					"id":        operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management"),
					"status":    operationPlanStepStatusPending,
					"skill_id":  skills.SkillFileManager,
					"tool_name": "save_file_to_management",
				},
			},
			"step_status": map[string]interface{}{
				operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management"): operationPlanStepStatusPending,
			},
		}},
	}
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{
		{Metadata: skills.SkillMetadata{ID: skills.SkillConsoleNavigator}},
		{Metadata: skills.SkillMetadata{ID: skills.SkillFileGenerator}},
		{Metadata: skills.SkillMetadata{ID: skills.SkillFileManager}},
	}}

	filtered := restrictResolvedSkillsForTurnStrategy(parts, resolved)
	got := filtered.SkillIDs()
	want := []string{skills.SkillConsoleNavigator, skills.SkillFileGenerator, skills.SkillFileManager}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered skill ids = %#v, want all enabled resolved skills %#v", got, want)
	}
}

func TestRestrictResolvedSkillsForAgentConfigCandidateStaysOnAgentManagement(t *testing.T) {
	parts := &chatRequestParts{
		Query: strings.Join([]string{
			"\u8bf7\u7f16\u8f91\u5f53\u524d\u8fd9\u4e2a\u667a\u80fd\u4f53\u7684\u914d\u7f6e\uff1a",
			"1. \u540d\u79f0\u6539\u4e3a GOAL-CONFIG-AGENT-EDITED",
			"2. \u63cf\u8ff0\u6539\u4e3a edited config smoke",
			"3. \u56fe\u6807\u6587\u5b57\u6539\u4e3a GE",
			"4. \u7cfb\u7edf\u63d0\u793a\u8bcd\u6539\u4e3a\uff1a\u4f60\u662f post verification \u914d\u7f6e\u70df\u6d4b\u667a\u80fd\u4f53\uff0c\u53ea\u80fd\u57fa\u4e8e\u4e8b\u5b9e\u56de\u7b54\u3002",
			"5. \u5f00\u542f\u6587\u4ef6\u4e0a\u4f20\u548c\u8bb0\u5fc6\u3002",
			"6. \u9996\u9875\u6807\u9898\u6539\u4e3a \u914d\u7f6e\u70df\u6d4b\u667a\u80fd\u4f53\uff0c\u8f93\u5165\u6846\u5360\u4f4d\u6587\u6848\u6539\u4e3a \u8bf7\u8f93\u5165\u914d\u7f6e\u70df\u6d4b\u95ee\u9898\u3002",
			"7. \u8c03\u7528 list_available_models\uff0cuse_case \u7528 text-chat\u3002\u5982\u679c\u80fd\u627e\u5230 DeepSeek-Chat (V3)\uff0c\u628a\u6a21\u578b\u5207\u6362\u5230\u5b83\uff0c\u5e76\u786e\u4fdd provider \u4e00\u8d77\u5207\u6362\uff1b\u5426\u5219\u9009\u62e9\u5217\u8868\u91cc\u7b2c\u4e00\u4e2a\u53ef\u7528\u6a21\u578b\u5e76\u540c\u6b65 provider\u3002",
			"8. \u8c03\u7528 list_agent_skill_candidates\uff0cquery \u7528 \u56fe\u8868\u751f\u6210\u5668\uff0c\u5982\u679c\u627e\u5230\u5c31\u6dfb\u52a0\u8fd9\u4e2a skill\u3002",
			"9. \u4e0d\u8981\u7ed1\u5b9a\u77e5\u8bc6\u5e93\u3001\u6570\u636e\u5e93\u6216\u5de5\u4f5c\u6d41\u3002\u6240\u6709\u5de5\u5177\u8fd4\u56de\u6210\u529f\u540e\u518d\u56de\u7b54\uff1b\u5982\u679c\u67d0\u4e00\u6b65\u5931\u8d25\uff0c\u8bf7\u5982\u5b9e\u8bf4\u660e\uff0c\u4e0d\u8981\u8bf4\u5df2\u7ecf\u5b8c\u6210\u3002",
		}, "\n"),
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents/agent-1/agent",
		SkillMode:      skillModeAuto,
		SkillIDs: []string{
			skills.SkillAgentManagement,
			skills.SkillChartGenerator,
			skills.SkillConsoleNavigator,
			skills.SkillFileGenerator,
		},
	}
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	if strategy.Intent != "manage_agent_asset" {
		t.Fatalf("strategy.Intent = %q, want manage_agent_asset; strategy=%#v", strategy.Intent, strategy)
	}
	if strategy.TargetPage != "/console/agents/agent-1/agent" {
		t.Fatalf("strategy.TargetPage = %q, want current Agent detail page", strategy.TargetPage)
	}
	for _, want := range []string{"get_agent_config", "update_agent_config", "list_available_models", "list_agent_skill_candidates"} {
		if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, want) {
			t.Fatalf("PlannedTools = %#v, missing agent-management/%s", strategy.PlannedTools, want)
		}
	}
	for _, unexpected := range []struct {
		skillID  string
		toolName string
	}{
		{skills.SkillChartGenerator, "generate_chart"},
		{skills.SkillFileGenerator, "generate_file"},
		{skills.SkillAgentManagement, "replace_agent_skill_bindings"},
		{skills.SkillAgentManagement, "replace_agent_knowledge_bindings"},
		{skills.SkillAgentManagement, "replace_agent_database_bindings"},
		{skills.SkillAgentManagement, "replace_agent_workflow_bindings"},
	} {
		if aiChatTurnStrategyHasPlannedToolForTest(strategy, unexpected.skillID, unexpected.toolName) {
			t.Fatalf("PlannedTools = %#v, want no %s/%s", strategy.PlannedTools, unexpected.skillID, unexpected.toolName)
		}
	}

	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{
		{Metadata: skills.SkillMetadata{ID: skills.SkillAgentManagement}},
		{Metadata: skills.SkillMetadata{ID: skills.SkillChartGenerator}},
		{Metadata: skills.SkillMetadata{ID: skills.SkillConsoleNavigator}},
		{Metadata: skills.SkillMetadata{ID: skills.SkillFileGenerator}},
	}}
	filtered := restrictResolvedSkillsForTurnStrategy(parts, resolved)
	got := filtered.SkillIDs()
	want := []string{skills.SkillAgentManagement, skills.SkillChartGenerator, skills.SkillConsoleNavigator, skills.SkillFileGenerator}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered skill ids = %#v, want all enabled resolved skills %#v", got, want)
	}

	metadata := streamingMessageMetadataWithTaskID(parts, "task-agent-config")
	plan := metadata["operation_plan"].(map[string]interface{})
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillChartGenerator, "generate_chart")); got != "" {
		t.Fatalf("chart-generator step status = %#v, want absent; plan=%#v", got, plan)
	}
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "list_agent_skill_candidates")); got != operationPlanStepStatusPending {
		t.Fatalf("list_agent_skill_candidates step status = %#v, want pending; plan=%#v", got, plan)
	}
}

func TestAgentCreateIntentPlansCreateEvenFromAgentDetailPage(t *testing.T) {
	parts := &chatRequestParts{
		Query: strings.Join([]string{
			"请创建一个测试智能体，名称必须是 POSTVERIFY-AGENT-NEW。",
			"创建成功后请把描述修改为 post verifier agent edit smoke，图标设置为 P2，并导航到这个智能体的详情页。",
			"最终回答只有在工具返回 agent_id 且更新结果包含 updated_fields 后才可以说完成。",
		}, "\n"),
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents/current-agent/agent",
		SkillMode:      skillModeAuto,
		SkillIDs: []string{
			skills.SkillAgentManagement,
			skills.SkillConsoleNavigator,
		},
	}

	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	if strategy.Intent != "manage_agent_asset" {
		t.Fatalf("strategy.Intent = %q, want manage_agent_asset; strategy=%#v", strategy.Intent, strategy)
	}
	if strategy.TargetPage != "/console/agents" {
		t.Fatalf("strategy.TargetPage = %q, want Agent list for create-new request", strategy.TargetPage)
	}
	for _, want := range []string{"create_agent", "update_agent_identity"} {
		if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, want) {
			t.Fatalf("PlannedTools = %#v, missing agent-management/%s", strategy.PlannedTools, want)
		}
	}

	metadata := streamingMessageMetadataWithTaskID(parts, "task-agent-create-from-detail")
	plan := metadata["operation_plan"].(map[string]interface{})
	for _, want := range []string{"create_agent", "update_agent_identity"} {
		if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, want)); got != operationPlanStepStatusPending {
			t.Fatalf("operation plan step %s = %#v, want pending; plan=%#v", want, got, plan)
		}
	}
}

func TestAgentConfigNoopSkillInstructionDoesNotPlanSkillBindings(t *testing.T) {
	parts := &chatRequestParts{
		Query: strings.Join([]string{
			"\u8bf7\u7f16\u8f91\u5f53\u524d\u8fd9\u4e2a\u667a\u80fd\u4f53\u7684\u914d\u7f6e\uff1a",
			"1. \u540d\u79f0\u6539\u4e3a GOAL-CONFIG-AGENT-EDITED4",
			"2. \u5f00\u542f\u6587\u4ef6\u4e0a\u4f20\u548c\u8bb0\u5fc6\u3002",
			"3. \u8c03\u7528 list_available_models\uff0cuse_case \u7528 text-chat\uff0c\u5e76\u540c\u6b65\u66f4\u6362 provider/model\u3002",
			"4. \u4e0d\u8981\u6dfb\u52a0\u6216\u5220\u9664 skill\uff0c\u4e0d\u8981\u7ed1\u5b9a\u77e5\u8bc6\u5e93\u3001\u6570\u636e\u5e93\u6216\u5de5\u4f5c\u6d41\u3002",
		}, "\n"),
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents/agent-1/agent",
		SkillMode:      skillModeAuto,
		SkillIDs: []string{
			skills.SkillAgentManagement,
			skills.SkillConsoleNavigator,
		},
	}

	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	if strategy.Intent != "manage_agent_asset" {
		t.Fatalf("strategy.Intent = %q, want manage_agent_asset; strategy=%#v", strategy.Intent, strategy)
	}
	if strategy.TargetPage != "/console/agents/agent-1/agent" {
		t.Fatalf("strategy.TargetPage = %q, want current Agent detail page", strategy.TargetPage)
	}
	for _, want := range []string{"get_agent_config", "update_agent_identity", "list_available_models", "update_agent_config"} {
		if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, want) {
			t.Fatalf("PlannedTools = %#v, missing agent-management/%s", strategy.PlannedTools, want)
		}
	}
	for _, unexpected := range []string{"delete_agent", "delete_agents", "list_agent_skill_candidates", "replace_agent_skill_bindings", "replace_agent_knowledge_bindings", "replace_agent_database_bindings", "replace_agent_workflow_bindings"} {
		if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, unexpected) {
			t.Fatalf("PlannedTools = %#v, want no agent-management/%s", strategy.PlannedTools, unexpected)
		}
	}

	metadata := streamingMessageMetadataWithTaskID(parts, "task-agent-config-no-skill")
	plan := metadata["operation_plan"].(map[string]interface{})
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "list_agent_skill_candidates")); got != "" {
		t.Fatalf("list_agent_skill_candidates step status = %#v, want absent; plan=%#v", got, plan)
	}
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")); got != operationPlanStepStatusPending {
		t.Fatalf("update_agent_config step status = %#v, want pending; plan=%#v", got, plan)
	}
}

func TestAgentConfigReadOnlyQuestionPlansOnlyConfigRead(t *testing.T) {
	for _, query := range []string{
		"Tell me the current Agent model name and provider from its config. Do not modify any configuration.",
		"\u8bf7\u8bfb\u53d6\u5f53\u524d Agent \u914d\u7f6e\uff0c\u544a\u8bc9\u6211\u5f53\u524d\u4f7f\u7528\u7684\u6a21\u578b\u540d\u79f0\u548c provider\u3002\u53ea\u6839\u636e\u5de5\u5177\u8fd4\u56de\u503c\u56de\u7b54\uff0c\u4e0d\u8981\u4fee\u6539\u4efb\u4f55\u914d\u7f6e\u3002",
	} {
		parts := &chatRequestParts{
			Query:          query,
			Surface:        aiChatSurfaceContextualSidebar,
			RuntimeContext: "route=/console/agents/agent-1/agent",
			SkillMode:      skillModeAuto,
			SkillIDs: []string{
				skills.SkillAgentManagement,
				skills.SkillConsoleNavigator,
			},
		}

		strategy := contextualAIChatTurnStrategyFromParts(parts)
		if strategy == nil {
			t.Fatalf("contextualAIChatTurnStrategyFromParts(%q) = nil, want strategy", query)
		}
		if strategy.Intent != "manage_agent_asset" {
			t.Fatalf("strategy.Intent = %q, want manage_agent_asset; query=%q strategy=%#v", strategy.Intent, query, strategy)
		}
		if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "get_agent_config") {
			t.Fatalf("PlannedTools = %#v, missing agent-management/get_agent_config for query %q", strategy.PlannedTools, query)
		}
		for _, unexpected := range []string{"update_agent_identity", "list_available_models", "update_agent_config"} {
			if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, unexpected) {
				t.Fatalf("PlannedTools = %#v, want no agent-management/%s for read-only query %q", strategy.PlannedTools, unexpected, query)
			}
		}

		metadata := streamingMessageMetadataWithTaskID(parts, "task-agent-config-readonly")
		plan := metadata["operation_plan"].(map[string]interface{})
		if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config")); got != operationPlanStepStatusPending {
			t.Fatalf("get_agent_config step status = %#v, want pending; query=%q plan=%#v", got, query, plan)
		}
		for _, unexpected := range []string{"update_agent_identity", "list_available_models", "update_agent_config"} {
			if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, unexpected)); got != "" {
				t.Fatalf("%s step status = %#v, want absent; query=%q plan=%#v", unexpected, got, query, plan)
			}
		}
	}
}

func TestSkillLoopCompletionEvidenceFinalizesObservationFromLedger(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "\u8bf7\u8bfb\u53d6\u5f53\u524d Agent \u914d\u7f6e\uff0c\u4e0d\u8981\u4fee\u6539\u4efb\u4f55\u914d\u7f6e\u3002",
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents/agent-1/agent",
		SkillMode:      skillModeAuto,
		SkillIDs:       []string{skills.SkillAgentManagement, skills.SkillConsoleNavigator},
	}
	metadata := streamingMessageMetadataWithTaskID(parts, "task-agent-config-evidence")
	metadata["skill_invocations"] = []map[string]interface{}{
		{
			"kind":      "tool_call",
			"status":    "success",
			"skill_id":  skills.SkillAgentManagement,
			"tool_name": "get_agent_config",
		},
	}
	prepared := &PreparedChat{
		parts: parts,
		Message: &runtimemodel.Message{
			ID:       uuid.New(),
			Metadata: metadata,
		},
	}

	evidence := skillLoopCompletionEvidence(prepared)()
	plan := mapFromOperationContext(evidence["operation_plan"])
	if got := stringFromAny(plan["status"]); got != operationPlanStatusCompleted {
		t.Fatalf("evidence operation_plan.status = %q, want completed; plan=%#v", got, plan)
	}
	for _, wantCompleted := range []string{
		operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config"),
		"skill:" + skills.SkillAgentManagement,
		"observe",
	} {
		if got := operationPlanStepStatusForTest(plan, wantCompleted); got != operationPlanStepStatusCompleted {
			t.Fatalf("%s step status = %q, want completed; plan=%#v", wantCompleted, got, plan)
		}
	}
	if got := stringFromAny(plan["pending_next_action"]); got != "none" {
		t.Fatalf("pending_next_action = %q, want none; plan=%#v", got, plan)
	}
}

func TestSkillLoopCompletionEvidenceEmbedsOperationLedgerInExecutionLedger(t *testing.T) {
	parts := &chatRequestParts{
		Query:     "Use the selected page resource.",
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillConsoleNavigator},
		OperationLedger: map[string]interface{}{
			"version": operationLedgerVersion,
			"status":  operationLedgerStatusObserved,
			"resources": []map[string]interface{}{{
				"id":   "file-1",
				"type": "file",
				"name": "visible.md",
			}},
			"capabilities": []map[string]interface{}{{
				"id":   "file.read",
				"name": "read_file",
			}},
			"risk_summary": map[string]interface{}{
				"level": "low",
			},
		},
	}
	metadata := streamingMessageMetadataWithTaskID(parts, "task-operation-ledger")
	prepared := &PreparedChat{
		parts: parts,
		Message: &runtimemodel.Message{
			ID:       uuid.New(),
			Metadata: metadata,
		},
	}

	evidence := skillLoopCompletionEvidence(prepared)()
	if ledger := mapFromOperationContext(evidence["operation_ledger"]); len(ledger) == 0 {
		t.Fatalf("operation_ledger evidence = %#v, want map", evidence["operation_ledger"])
	}
	executionLedger := mapFromOperationContext(evidence["execution_ledger"])
	operationLedger := mapFromOperationContext(executionLedger["operation_ledger"])
	if len(operationLedger) == 0 {
		t.Fatalf("execution_ledger.operation_ledger = %#v, want operation ledger", executionLedger["operation_ledger"])
	}
	if operationLedger["version"] != operationLedgerVersion {
		t.Fatalf("operation ledger version = %#v, want %q", operationLedger["version"], operationLedgerVersion)
	}
	resources := mapSliceFromAny(operationLedger["resources"])
	if len(resources) != 1 || resources[0]["name"] != "visible.md" {
		t.Fatalf("operation ledger resources = %#v, want selected visible.md resource", operationLedger["resources"])
	}
	capabilities := mapSliceFromAny(operationLedger["capabilities"])
	if len(capabilities) != 1 || capabilities[0]["name"] != "read_file" {
		t.Fatalf("operation ledger capabilities = %#v, want read_file capability", operationLedger["capabilities"])
	}
}

func TestSkillLoopCompletionEvidenceBuildsExecutionSummaryForBatchAndDeviations(t *testing.T) {
	parts := &chatRequestParts{
		Query:     "\u5220\u9664\u524d\u4e24\u4e2a\u667a\u80fd\u4f53\uff0c\u5982\u679c\u6709\u5931\u8d25\u8981\u544a\u8bc9\u6211",
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillAgentManagement},
	}
	metadata := streamingMessageMetadataWithTaskID(parts, "task-batch-summary")
	recordOperationPlanToolDeviation(metadata, skills.SkillAgentManagement, "list_agents", "model_collected_unplanned_readonly_evidence")
	recordOperationPlanToolBlockedDeviation(metadata, skills.SkillFileManager, "delete_file", "model_attempted_unrelated_mutation")
	metadata["skill_invocations"] = []map[string]interface{}{
		{
			"kind":      "tool_call",
			"status":    "success",
			"skill_id":  skills.SkillAgentManagement,
			"tool_name": "delete_agents",
			"result": map[string]interface{}{
				"status":        "partial_failed",
				"target_count":  2,
				"deleted_count": 1,
				"failed_count":  1,
				"operation_group": map[string]interface{}{
					"id":            "agent.delete.batch:test",
					"type":          "batch",
					"operation":     "agent.delete",
					"asset_type":    "agent",
					"status":        "partial_failed",
					"target_count":  2,
					"success_count": 1,
					"failed_count":  1,
					"item_results": []interface{}{
						map[string]interface{}{"agent_id": "agent-ok", "agent_name": "Agent OK", "status": "succeeded"},
						map[string]interface{}{"agent_id": "agent-locked", "agent_name": "Agent Locked", "status": "failed", "error": "locked"},
					},
				},
			},
		},
	}
	prepared := &PreparedChat{
		parts: parts,
		Message: &runtimemodel.Message{
			ID:       uuid.New(),
			Metadata: metadata,
		},
	}

	evidence := skillLoopCompletionEvidence(prepared)()
	summary := mapFromOperationContext(evidence["execution_summary"])
	if len(summary) == 0 {
		t.Fatalf("execution_summary = %#v, want compact summary", evidence["execution_summary"])
	}
	planSummary := mapFromOperationContext(summary["operation_plan"])
	deviations := mapSliceFromAny(planSummary["deviations"])
	if len(deviations) != 1 {
		t.Fatalf("execution_summary.operation_plan.deviations = %#v, want one deviation", planSummary["deviations"])
	}
	if got := stringFromAny(deviations[0]["outcome"]); got != "allowed" {
		t.Fatalf("execution_summary.operation_plan.deviations[0].outcome = %q, want allowed", got)
	}
	blockedDeviations := mapSliceFromAny(planSummary["blocked_deviations"])
	if len(blockedDeviations) != 1 {
		t.Fatalf("execution_summary.operation_plan.blocked_deviations = %#v, want one blocked deviation", planSummary["blocked_deviations"])
	}
	if got := stringFromAny(blockedDeviations[0]["outcome"]); got != "blocked" {
		t.Fatalf("execution_summary.operation_plan.blocked_deviations[0].outcome = %q, want blocked", got)
	}
	groups := mapSliceFromAny(summary["operation_groups"])
	if len(groups) != 1 {
		t.Fatalf("execution_summary.operation_groups = %#v, want one batch group", summary["operation_groups"])
	}
	items := mapSliceFromAny(groups[0]["item_results"])
	if len(items) != 2 || items[1]["status"] != "failed" || items[1]["error"] != "locked" {
		t.Fatalf("operation group item_results = %#v, want failed item evidence", groups[0]["item_results"])
	}
	toolResults := mapSliceFromAny(summary["tool_results"])
	if len(toolResults) != 1 {
		t.Fatalf("execution_summary.tool_results = %#v, want one delete_agents result", summary["tool_results"])
	}
	toolSummary := mapFromOperationContext(toolResults[0]["result_summary"])
	groupSummary := mapFromOperationContext(toolSummary["operation_group"])
	if groupSummary["failed_count"] != 1 {
		t.Fatalf("tool result summary operation_group = %#v, want failed_count=1", groupSummary)
	}
	operationSummary := mapFromOperationContext(evidence["operation_result_summary"])
	if len(operationSummary) == 0 {
		t.Fatalf("operation_result_summary = %#v, want stable operation facts", evidence["operation_result_summary"])
	}
	if got := operationSummary["status"]; got != "partial_failed" {
		t.Fatalf("operation_result_summary.status = %#v, want partial_failed; summary=%#v", got, operationSummary)
	}
	if got := operationSummary["operation"]; got != "agent.delete" {
		t.Fatalf("operation_result_summary.operation = %#v, want agent.delete; summary=%#v", got, operationSummary)
	}
	if got := operationSummary["target_count"]; got != 2 {
		t.Fatalf("operation_result_summary.target_count = %#v, want 2; summary=%#v", got, operationSummary)
	}
	if got := operationSummary["failed_count"]; got != 1 {
		t.Fatalf("operation_result_summary.failed_count = %#v, want 1; summary=%#v", got, operationSummary)
	}
	if latest := mapFromOperationContext(operationSummary["latest_tool_result"]); latest["tool_name"] != "delete_agents" {
		t.Fatalf("operation_result_summary.latest_tool_result = %#v, want delete_agents", latest)
	}
	ledger := mapFromOperationContext(evidence["execution_ledger"])
	if got := mapFromOperationContext(ledger["summary"]); len(got) == 0 {
		t.Fatalf("execution_ledger.summary = %#v, want mirrored summary", ledger["summary"])
	}
	if got := mapFromOperationContext(ledger["operation_result_summary"]); len(got) == 0 {
		t.Fatalf("execution_ledger.operation_result_summary = %#v, want mirrored operation facts", ledger["operation_result_summary"])
	}
}

func TestSkillLoopCompletionEvidenceCarriesFailedManagedFileSaveLedger(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "\u751f\u6210\u4e00\u4e2a SVG \u5e76\u4fdd\u5b58\u5230\u6587\u4ef6\u7ba1\u7406",
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/files",
		SkillMode:      skillModeAuto,
		SkillIDs:       []string{skills.SkillFileGenerator, skills.SkillFileManager},
	}
	metadata := streamingMessageMetadataWithTaskID(parts, "task-managed-save-failure")
	metadata["skill_invocations"] = []map[string]interface{}{
		{
			"kind":      "tool_call",
			"status":    "error",
			"skill_id":  skills.SkillFileManager,
			"tool_name": "save_file_to_management",
			"arguments": map[string]interface{}{
				"tool_file_id": "tool-file-1",
				"filename":     "report.svg",
			},
			"error": "workspace permission denied",
			"result": map[string]interface{}{
				"status":       "error",
				"error":        "workspace permission denied",
				"error_code":   "permission_denied",
				"recoverable":  false,
				"tool_file_id": "tool-file-1",
				"filename":     "report.svg",
			},
		},
	}
	prepared := &PreparedChat{
		parts: parts,
		Message: &runtimemodel.Message{
			ID:       uuid.New(),
			Metadata: metadata,
		},
	}

	evidence := skillLoopCompletionEvidence(prepared)()
	plan := mapFromOperationContext(evidence["operation_plan"])
	if got := stringFromAny(plan["status"]); got != operationPlanStatusFailed {
		t.Fatalf("evidence operation_plan.status = %q, want failed; plan=%#v", got, plan)
	}
	saveStepID := operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management")
	if got := operationPlanStepStatusForTest(plan, saveStepID); got != operationPlanStepStatusFailed {
		t.Fatalf("%s step status = %q, want failed; plan=%#v", saveStepID, got, plan)
	}
	ledger := mapFromOperationContext(evidence["execution_ledger"])
	invocations := mapSliceFromAny(ledger["skill_invocations"])
	if len(invocations) != 1 {
		t.Fatalf("execution_ledger.skill_invocations = %#v, want one failed save call", ledger["skill_invocations"])
	}
	invocation := invocations[0]
	if invocation["skill_id"] != skills.SkillFileManager || invocation["tool_name"] != "save_file_to_management" {
		t.Fatalf("ledger invocation = %#v, want file-manager/save_file_to_management", invocation)
	}
	if got := stringFromAny(invocation["status"]); got != "error" {
		t.Fatalf("ledger invocation status = %q, want error; invocation=%#v", got, invocation)
	}
	if !strings.Contains(stringFromAny(invocation["error"]), "permission denied") {
		t.Fatalf("ledger invocation error = %#v, want permission failure", invocation["error"])
	}
	result := mapFromOperationContext(invocation["result"])
	if result["error_code"] != "permission_denied" || result["recoverable"] != false {
		t.Fatalf("ledger invocation result = %#v, want structured failure evidence", result)
	}
}

func TestSkillLoopPlanToolGuardRestrictsAgentConfigContinuationToPendingTool(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "请编辑当前这个智能体配置，不要添加或删除 skill",
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillAgentManagement, skills.SkillConsoleNavigator},
		},
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
	guard := skillLoopPlanToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopPlanToolCallGuard() = nil, want guard")
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillAgentManagement}); blocked {
		t.Fatal("load agent-management was blocked, want allowed because a pending agent-management tool exists")
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillAgentManagement, ToolName: "update_agent_config"}); blocked {
		t.Fatal("update_agent_config was blocked, want pending planned tool allowed")
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillAgentManagement, ToolName: "update_agent_identity"}); !blocked {
		t.Fatal("update_agent_identity was allowed, want completed step blocked")
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillConsoleNavigator,
		ToolName:  "navigate",
		Arguments: map[string]interface{}{"href": "/console/agents"},
	}); blocked {
		t.Fatal("console-navigator/navigate was blocked, want advisory plan to allow route context collection")
	}
}

func TestSkillLoopPlanToolGuardAllowsPlannedReadEvidenceReplay(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "继续编辑当前智能体配置",
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillAgentManagement, skills.SkillConsoleNavigator},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"steps": []interface{}{
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config"),
						"status":    operationPlanStepStatusCompleted,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "get_agent_config",
					},
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillAgentManagement, "list_available_models"),
						"status":    operationPlanStepStatusCompleted,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "list_available_models",
					},
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
					operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config"):      operationPlanStepStatusCompleted,
					operationPlanToolStepID(skills.SkillAgentManagement, "list_available_models"): operationPlanStepStatusCompleted,
					operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_identity"): operationPlanStepStatusCompleted,
					operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"):   operationPlanStepStatusPending,
				},
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if _, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillAgentManagement, ToolName: "get_agent_config"}); blocked {
		t.Fatal("get_agent_config was blocked, want planned read evidence replay allowed")
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillAgentManagement, ToolName: "list_available_models"}); blocked {
		t.Fatal("list_available_models was blocked, want planned read evidence replay allowed")
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillAgentManagement, ToolName: "update_agent_config"}); blocked {
		t.Fatal("update_agent_config was blocked, want pending mutation allowed")
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillAgentManagement, ToolName: "update_agent_identity"}); !blocked {
		t.Fatal("update_agent_identity was allowed, want completed mutation blocked")
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillConsoleNavigator,
		ToolName:  "navigate",
		Arguments: map[string]interface{}{"href": "/console/agents/agent-1/agent"},
	}); blocked {
		t.Fatal("console-navigator/navigate was blocked, want advisory plan to allow route context collection")
	}
}

func TestSkillLoopPlanToolGuardAllowsUnplannedEvidenceToolDeviation(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "change this agent model to a DeepSeek model",
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillAgentManagement},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status": operationPlanStatusRunning,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config"),
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "get_agent_config",
					},
				},
				"step_status": map[string]interface{}{
					operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config"): operationPlanStepStatusPending,
				},
				"original_user_goal": "change this agent model to a DeepSeek model",
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if _, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillAgentManagement, ToolName: "list_available_models"}); blocked {
		t.Fatal("list_available_models was blocked, want advisory plan to allow additional model evidence")
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	deviations := mapSliceFromAny(plan["deviations"])
	if len(deviations) != 1 {
		t.Fatalf("deviations = %#v, want one recorded evidence deviation", deviations)
	}
	if got := stringFromAny(deviations[0]["tool_name"]); got != "list_available_models" {
		t.Fatalf("deviation tool_name = %q, want list_available_models; plan=%#v", got, plan)
	}
	if got := stringFromAny(deviations[0]["outcome"]); got != "allowed" {
		t.Fatalf("deviation outcome = %q, want allowed; plan=%#v", got, plan)
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillAgentManagement, ToolName: "update_agent_identity"}); !blocked {
		t.Fatal("update_agent_identity was allowed, want unrelated mutation still blocked")
	}
	plan = mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	blockedDeviations := mapSliceFromAny(plan["blocked_deviations"])
	if len(blockedDeviations) != 1 {
		t.Fatalf("blocked_deviations = %#v, want one blocked mutation deviation", blockedDeviations)
	}
	if got := stringFromAny(blockedDeviations[0]["outcome"]); got != "blocked" {
		t.Fatalf("blocked deviation outcome = %q, want blocked; plan=%#v", got, plan)
	}
}

func TestSkillLoopPlanToolGuardAllowsUnplannedReadOnlyEvidenceDeviation(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "help me inspect this agent and its related data before editing it",
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillAgentManagement, skills.SkillInternalDatabase, skills.SkillFileManager, skills.SkillConsoleNavigator},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status": operationPlanStatusRunning,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"),
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "update_agent_config",
					},
				},
				"step_status": map[string]interface{}{
					operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"): operationPlanStepStatusPending,
				},
				"original_user_goal": "help me inspect this agent and its related data before editing it",
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if _, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillInternalDatabase, ToolName: "query_table_records"}); blocked {
		t.Fatal("query_table_records was blocked, want advisory plan to allow unplanned read-only evidence")
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	deviations := mapSliceFromAny(plan["deviations"])
	if len(deviations) != 1 {
		t.Fatalf("deviations = %#v, want one recorded read-only deviation", deviations)
	}
	if got := stringFromAny(deviations[0]["reason"]); got != "model_collected_unplanned_readonly_evidence" {
		t.Fatalf("deviation reason = %q, want read-only evidence reason; plan=%#v", got, plan)
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillFileManager, ToolName: "save_file_to_management"}); !blocked {
		t.Fatal("save_file_to_management was allowed, want unplanned asset mutation still blocked")
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillConsoleNavigator,
		ToolName:  "navigate",
		Arguments: map[string]interface{}{"href": "/console/agents"},
	}); blocked {
		t.Fatal("console-navigator/navigate was blocked, want advisory plan to allow route context collection")
	}
	plan = mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	deviations = mapSliceFromAny(plan["deviations"])
	if len(deviations) != 2 {
		t.Fatalf("deviations = %#v, want read-only and navigation deviations", deviations)
	}
}

func TestSkillLoopPlanToolGuardAllowsUnplannedNonMutatingToolDeviation(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "inspect this agent and calculate a small config value before editing",
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillAgentManagement, skills.SkillCalculator, skills.SkillFileGenerator},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status": operationPlanStatusRunning,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"),
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "update_agent_config",
					},
				},
				"step_status": map[string]interface{}{
					operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"): operationPlanStepStatusPending,
				},
				"original_user_goal": "inspect this agent and calculate a small config value before editing",
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if _, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillCalculator, ToolName: "evaluate_expression"}); blocked {
		t.Fatal("calculator/evaluate_expression was blocked, want advisory deviation for non-mutating helper tool")
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	deviations := mapSliceFromAny(plan["deviations"])
	if len(deviations) != 1 {
		t.Fatalf("deviations = %#v, want one non-mutating tool deviation", deviations)
	}
	if got := stringFromAny(deviations[0]["reason"]); got != "model_used_unplanned_non_mutating_tool" {
		t.Fatalf("deviation reason = %q, want non-mutating reason; plan=%#v", got, plan)
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillFileGenerator, ToolName: "generate_file"}); !blocked {
		t.Fatal("file-generator/generate_file was allowed, want unplanned generated asset creation still blocked")
	}
}

func TestSkillLoopPlanToolGuardAllowsGovernedReadToolWithMutationLikeName(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "run the bound workflow and then check the workflow run status",
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillAgentWorkflow},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status": operationPlanStatusRunning,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillAgentWorkflow, "run_agent_workflow"),
						"status":    operationPlanStepStatusCompleted,
						"skill_id":  skills.SkillAgentWorkflow,
						"tool_name": "run_agent_workflow",
					},
				},
				"step_status": map[string]interface{}{
					operationPlanToolStepID(skills.SkillAgentWorkflow, "run_agent_workflow"): operationPlanStepStatusCompleted,
				},
				"original_user_goal": "run the bound workflow and then check the workflow run status",
			},
		}},
	}
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata: skills.SkillMetadata{ID: skills.SkillAgentWorkflow},
		Tools: []skills.SkillToolDefinition{
			{
				Name: "get_workflow_run_status",
				Governance: &toolgovernance.Manifest{
					Effect:    toolgovernance.EffectRead,
					AssetType: "workflow_run",
					RiskLevel: toolgovernance.RiskLevelLow,
				},
			},
			{
				Name: "run_agent_workflow",
				Governance: &toolgovernance.Manifest{
					Effect:    toolgovernance.EffectInvoke,
					AssetType: "workflow",
					RiskLevel: toolgovernance.RiskLevelHigh,
				},
			},
		},
	}}}

	guard := skillLoopPlanToolCallGuardWithResolved(prepared, resolved)
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillAgentWorkflow,
		ToolName:  "get_workflow_run_status",
		Arguments: map[string]interface{}{"workflow_run_id": "run-1"},
		AttemptedToolCalls: []skillloop.SkillToolCallRef{{
			SkillID:   skills.SkillAgentWorkflow,
			ToolName:  "get_workflow_run_status",
			Arguments: map[string]interface{}{"workflow_run_id": "run-1"},
		}},
	}); blocked {
		t.Fatal("get_workflow_run_status was blocked, want manifest read tool allowed despite mutation-like name")
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	deviations := mapSliceFromAny(plan["deviations"])
	if len(deviations) != 1 {
		t.Fatalf("deviations = %#v, want manifest read deviation", deviations)
	}
	if got := stringFromAny(deviations[0]["reason"]); got != "model_collected_manifest_read_evidence" {
		t.Fatalf("deviation reason = %q, want manifest read reason; plan=%#v", got, plan)
	}

	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillAgentWorkflow,
		ToolName:  "run_agent_workflow",
		Arguments: map[string]interface{}{"binding_id": "binding-1"},
		AttemptedToolCalls: []skillloop.SkillToolCallRef{{
			SkillID:   skills.SkillAgentWorkflow,
			ToolName:  "run_agent_workflow",
			Arguments: map[string]interface{}{"binding_id": "binding-1"},
		}},
	}); !blocked {
		t.Fatal("duplicate run_agent_workflow was allowed, want invoke mutation duplicate protection preserved")
	}
}

func TestSkillLoopPlanToolGuardAllowsGovernedReadAfterCompletedPlan(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "summarize the workflow run result",
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillAgentWorkflow},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status": operationPlanStatusCompleted,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillAgentWorkflow, "run_agent_workflow"),
						"status":    operationPlanStepStatusCompleted,
						"skill_id":  skills.SkillAgentWorkflow,
						"tool_name": "run_agent_workflow",
					},
				},
				"step_status": map[string]interface{}{
					operationPlanToolStepID(skills.SkillAgentWorkflow, "run_agent_workflow"): operationPlanStepStatusCompleted,
				},
				"original_user_goal": "summarize the workflow run result",
			},
		}},
	}
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata: skills.SkillMetadata{ID: skills.SkillAgentWorkflow},
		Tools: []skills.SkillToolDefinition{
			{
				Name: "get_workflow_run_status",
				Governance: &toolgovernance.Manifest{
					Effect:    toolgovernance.EffectRead,
					AssetType: "workflow_run",
					RiskLevel: toolgovernance.RiskLevelLow,
				},
			},
			{
				Name: "run_agent_workflow",
				Governance: &toolgovernance.Manifest{
					Effect:    toolgovernance.EffectInvoke,
					AssetType: "workflow",
					RiskLevel: toolgovernance.RiskLevelHigh,
				},
			},
		},
	}}}

	guard := skillLoopPlanToolCallGuardWithResolved(prepared, resolved)
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillAgentWorkflow,
		ToolName:  "get_workflow_run_status",
		Arguments: map[string]interface{}{"workflow_run_id": "run-1"},
	}); blocked {
		t.Fatal("get_workflow_run_status was blocked after completed plan, want read-effect verification allowed")
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	deviations := mapSliceFromAny(plan["deviations"])
	if len(deviations) != 1 {
		t.Fatalf("deviations = %#v, want one read-effect verification deviation", deviations)
	}
	if got := stringFromAny(deviations[0]["reason"]); got != "model_collected_manifest_read_evidence" {
		t.Fatalf("deviation reason = %q, want manifest read reason; plan=%#v", got, plan)
	}

	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillAgentWorkflow,
		ToolName:  "run_agent_workflow",
		Arguments: map[string]interface{}{"binding_id": "binding-1"},
		AttemptedToolCalls: []skillloop.SkillToolCallRef{{
			SkillID:   skills.SkillAgentWorkflow,
			ToolName:  "run_agent_workflow",
			Arguments: map[string]interface{}{"binding_id": "binding-1"},
		}},
	}); !blocked {
		t.Fatal("run_agent_workflow was allowed after completed plan, want invoke mutation duplicate protection preserved")
	}
}

func TestSkillLoopPlanToolGuardAllowsArtifactGenerationWithinManagedFileGoal(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "create an svg file in File Management",
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillFileGenerator, skills.SkillFileManager},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status": operationPlanStatusRunning,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management"),
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillFileManager,
						"tool_name": "save_file_to_management",
					},
				},
				"step_status": map[string]interface{}{
					operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management"): operationPlanStepStatusPending,
				},
				"original_user_goal": "create an svg file in File Management",
			},
		}},
	}
	args := map[string]interface{}{"filename": "draft.svg", "format": "svg"}

	guard := skillLoopPlanToolCallGuard(prepared)
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillFileGenerator,
		ToolName:  "generate_file",
		Arguments: args,
	}); blocked {
		t.Fatal("file-generator/generate_file was blocked, want artifact generation allowed within managed file create goal")
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillFileGenerator, "generate_file")); got != operationPlanStepStatusPending {
		t.Fatalf("generate_file step status = %q, want pending amendment; plan=%#v", got, plan)
	}
	deviations := mapSliceFromAny(plan["deviations"])
	if len(deviations) != 1 {
		t.Fatalf("deviations = %#v, want artifact generation deviation", deviations)
	}
	if got := stringFromAny(deviations[0]["reason"]); got != "model_generated_temporary_artifact_within_user_goal" {
		t.Fatalf("deviation reason = %q, want artifact generation reason; plan=%#v", got, plan)
	}

	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillFileGenerator,
		ToolName:  "generate_file",
		Arguments: args,
		AttemptedToolCalls: []skillloop.SkillToolCallRef{{
			SkillID:   skills.SkillFileGenerator,
			ToolName:  "generate_file",
			Arguments: map[string]interface{}{"format": "svg", "filename": "draft.svg"},
		}},
	}); !blocked {
		t.Fatal("duplicate generate_file was allowed, want same-argument artifact generation retry blocked")
	}
}

func TestSkillLoopPlanToolGuardAllowsGovernedMutationDeviation(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "帮我把这个智能体调整成更适合客服接待",
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillAgentManagement},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status": operationPlanStatusRunning,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config"),
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "get_agent_config",
					},
				},
				"step_status": map[string]interface{}{
					operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config"): operationPlanStepStatusPending,
				},
				"original_user_goal": "帮我把这个智能体调整成更适合客服接待",
			},
		}},
	}
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata: skills.SkillMetadata{ID: skills.SkillAgentManagement},
		Tools: []skills.SkillToolDefinition{{
			Name: "update_agent_config",
			Governance: &toolgovernance.Manifest{
				Effect:    toolgovernance.EffectUpdate,
				AssetType: "agent",
				RiskLevel: toolgovernance.RiskLevelMedium,
			},
		}},
	}}}

	guard := skillLoopPlanToolCallGuardWithResolved(prepared, resolved)
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillAgentManagement,
		ToolName:  "update_agent_config",
		Arguments: map[string]interface{}{"agent_id": "agent-1", "home_title": "客服接待"},
	}); blocked {
		t.Fatal("governed update_agent_config was blocked, want runtime governance to own the mutation decision")
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")); got != operationPlanStepStatusPending {
		t.Fatalf("update_agent_config step status = %q, want pending amendment; plan=%#v", got, plan)
	}
	deviations := mapSliceFromAny(plan["deviations"])
	if len(deviations) != 1 {
		t.Fatalf("deviations = %#v, want governed mutation deviation", deviations)
	}
	if got := stringFromAny(deviations[0]["reason"]); got != "model_requested_governed_mutation_under_runtime_governance" {
		t.Fatalf("deviation reason = %q, want governed mutation reason; plan=%#v", got, plan)
	}
}

func TestSkillLoopPlanToolGuardBlocksDuplicateGovernedMutationDeviation(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "帮我把这个智能体调整成更适合客服接待",
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillAgentManagement},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status": operationPlanStatusRunning,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config"),
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "get_agent_config",
					},
				},
				"step_status": map[string]interface{}{
					operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config"): operationPlanStepStatusPending,
				},
			},
		}},
	}
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata: skills.SkillMetadata{ID: skills.SkillAgentManagement},
		Tools: []skills.SkillToolDefinition{{
			Name: "update_agent_config",
			Governance: &toolgovernance.Manifest{
				Effect:    toolgovernance.EffectUpdate,
				AssetType: "agent",
				RiskLevel: toolgovernance.RiskLevelMedium,
			},
		}},
	}}}
	args := map[string]interface{}{"agent_id": "agent-1", "home_title": "客服接待"}

	guard := skillLoopPlanToolCallGuardWithResolved(prepared, resolved)
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillAgentManagement,
		ToolName:  "update_agent_config",
		Arguments: args,
		AttemptedToolCalls: []skillloop.SkillToolCallRef{{
			SkillID:   skills.SkillAgentManagement,
			ToolName:  "update_agent_config",
			Arguments: map[string]interface{}{"home_title": "客服接待", "agent_id": "agent-1"},
		}},
	}); !blocked {
		t.Fatal("duplicate governed update_agent_config was allowed, want same-argument retry blocked")
	}
}

func TestSkillLoopPlanToolGuardAllowsRepeatedCreateAgentForExplicitMultiCreateGoal(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "create two draft agents named Alpha and Beta",
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillAgentManagement},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status": operationPlanStatusRunning,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillAgentManagement, "create_agent"),
						"status":    operationPlanStepStatusCompleted,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "create_agent",
					},
				},
				"step_status": map[string]interface{}{
					operationPlanToolStepID(skills.SkillAgentManagement, "create_agent"): operationPlanStepStatusCompleted,
				},
				"original_user_goal": "create two draft agents named Alpha and Beta",
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillAgentManagement,
		ToolName:  "create_agent",
		Arguments: map[string]interface{}{"name": "Beta"},
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{{
			SkillID:   skills.SkillAgentManagement,
			ToolName:  "create_agent",
			Arguments: map[string]interface{}{"name": "Alpha"},
			Result:    map[string]interface{}{"agent_id": "agent-alpha", "name": "Alpha"},
		}},
	}); blocked {
		t.Fatal("second create_agent was blocked, want repeated planned mutation allowed for explicit two-agent goal")
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	repeatStepID := operationPlanToolStepID(skills.SkillAgentManagement, "create_agent") + "#2"
	if got := operationPlanStepStatusForTest(plan, repeatStepID); got != operationPlanStepStatusPending {
		t.Fatalf("repeat create step status = %q, want pending; plan=%#v", got, plan)
	}
	deviations := mapSliceFromAny(plan["deviations"])
	if len(deviations) != 1 {
		t.Fatalf("deviations = %#v, want one repeated mutation deviation", deviations)
	}
	if got := stringFromAny(deviations[0]["reason"]); got != "model_repeated_planned_mutation_within_user_goal" {
		t.Fatalf("deviation reason = %q, want repeated mutation reason; plan=%#v", got, plan)
	}
}

func TestSkillLoopPlanToolGuardBlocksRepeatedCreateAgentForSameTarget(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "create two draft agents named Alpha and Beta",
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillAgentManagement},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status": operationPlanStatusRunning,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillAgentManagement, "create_agent"),
						"status":    operationPlanStepStatusCompleted,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "create_agent",
					},
				},
				"step_status": map[string]interface{}{
					operationPlanToolStepID(skills.SkillAgentManagement, "create_agent"): operationPlanStepStatusCompleted,
				},
				"original_user_goal": "create two draft agents named Alpha and Beta",
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillAgentManagement,
		ToolName:  "create_agent",
		Arguments: map[string]interface{}{"name": "Alpha"},
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{{
			SkillID:   skills.SkillAgentManagement,
			ToolName:  "create_agent",
			Arguments: map[string]interface{}{"name": "Alpha"},
			Result:    map[string]interface{}{"agent_id": "agent-alpha", "name": "Alpha"},
		}},
	}); !blocked {
		t.Fatal("duplicate create_agent with the same target was allowed, want blocked")
	}
}

func TestSkillLoopPlanToolGuardAllowsExplicitAgentDeletePlanAmendment(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "\u5e2e\u6211\u5220\u9664\u8fd9\u4e2a\u9875\u9762\u7684\u524d\u56db\u4e2a\u667a\u80fd\u4f53",
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillAgentManagement},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status": operationPlanStatusCompleted,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillAgentManagement, "list_agents"),
						"status":    operationPlanStepStatusCompleted,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "list_agents",
					},
				},
				"step_status": map[string]interface{}{
					operationPlanToolStepID(skills.SkillAgentManagement, "list_agents"): operationPlanStepStatusCompleted,
				},
				"original_user_goal": "\u5e2e\u6211\u5220\u9664\u8fd9\u4e2a\u9875\u9762\u7684\u524d\u56db\u4e2a\u667a\u80fd\u4f53",
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if _, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillAgentManagement, ToolName: "delete_agent"}); blocked {
		t.Fatal("delete_agent was blocked, want plan amendment allowed for explicit delete goal")
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	stepID := operationPlanToolStepID(skills.SkillAgentManagement, "delete_agent")
	stepStatus := mapFromOperationContext(plan["step_status"])
	if got := stringFromAny(stepStatus[stepID]); got != operationPlanStepStatusPending {
		t.Fatalf("step_status[%s] = %q, want pending after amendment; plan=%#v", stepID, got, plan)
	}
	if got := stringFromAny(plan["status"]); got != operationPlanStatusRunning {
		t.Fatalf("plan status = %q, want running after amendment; plan=%#v", got, plan)
	}
	if !operationPlanBoolValue(plan["amended"]) {
		t.Fatalf("plan amended = %#v, want true; plan=%#v", plan["amended"], plan)
	}
}

func TestSkillLoopPlanToolGuardBlocksMutationAfterOperationPlanCompleted(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "Summarize the approved Agent update result.",
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillAgentManagement},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status": operationPlanStatusCompleted,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_identity"),
						"status":    operationPlanStepStatusCompleted,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "update_agent_identity",
					},
				},
				"step_status": map[string]interface{}{
					operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_identity"): operationPlanStepStatusCompleted,
				},
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopPlanToolCallGuard() = nil, want completed-plan guard")
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillAgentManagement}); blocked {
		t.Fatal("load agent-management was blocked after operation plan completed, want enabled skill load allowed for follow-up reasoning")
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillAgentManagement, ToolName: "list_agents"}); blocked {
		t.Fatal("list_agents was blocked after operation plan completed, want read-only evidence tools allowed")
	}
	result, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillAgentManagement, ToolName: "update_agent_identity"})
	if !blocked {
		t.Fatal("update_agent_identity was allowed after operation plan completed")
	}
	if result.ToolName != "update_agent_identity" ||
		!strings.Contains(result.SystemMessage, "operation plan is advisory") ||
		!strings.Contains(result.SystemMessage, "current-goal match") {
		t.Fatalf("guard result = %#v, want completed-plan feedback for repeated Agent update", result)
	}
}

func TestSkillLoopPlanToolGuardDoesNotRestrictWithoutOperationPlan(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "save this generated file to file management",
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillFileGenerator, skills.SkillFileManager, skills.SkillConsoleNavigator},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"turn_strategy": map[string]interface{}{
				"intent": "save_generated_file_to_file_management",
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if _, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillConsoleNavigator, ToolName: "navigate"}); blocked {
		t.Fatal("console-navigator/navigate was blocked without operation_plan, want no plan-based restriction")
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillFileManager, ToolName: "save_file_to_management"}); blocked {
		t.Fatal("file-manager/save_file_to_management was blocked without operation_plan, want no plan-based restriction")
	}
}

func TestSkillLoopPlanToolGuardDoesNotRestrictNonExecutablePlan(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "保存不存在的临时文件，如果失败请如实说明",
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillFileManager, skills.SkillConsoleNavigator},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"intent":              "answer_or_explain_zgi_context",
				"status":              operationPlanStatusCompleted,
				"pending_next_action": "none",
				"steps": []interface{}{
					map[string]interface{}{
						"id":       "skill:" + skills.SkillConsoleNavigator,
						"role":     "supporting",
						"title":    "Use console-navigator",
						"status":   operationPlanStepStatusPending,
						"skill_id": skills.SkillConsoleNavigator,
					},
					map[string]interface{}{
						"id":     "observe",
						"title":  "Observe result",
						"status": operationPlanStepStatusCompleted,
					},
				},
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if _, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillFileManager, ToolName: "save_file_to_management"}); blocked {
		t.Fatal("file-manager/save_file_to_management was blocked by a non-executable plan, want no plan restriction")
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillConsoleNavigator, ToolName: "navigate"}); blocked {
		t.Fatal("console-navigator/navigate was blocked by a non-executable plan, want no plan restriction")
	}
}

func TestSkillLoopPlanToolGuardBlocksCompletedRouteRepeat(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "continue from the loaded files page",
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillConsoleNavigator, skills.SkillFileManager},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status": operationPlanStatusRunning,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        operationPlanRouteStepID("/console/files", 1),
						"status":    operationPlanStepStatusCompleted,
						"skill_id":  skills.SkillConsoleNavigator,
						"tool_name": "navigate",
						"asset_target": map[string]interface{}{
							"page": "/console/files",
						},
					},
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management"),
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillFileManager,
						"tool_name": "save_file_to_management",
					},
				},
				"step_status": map[string]interface{}{
					operationPlanRouteStepID("/console/files", 1):                               operationPlanStepStatusCompleted,
					operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management"): operationPlanStepStatusPending,
				},
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillConsoleNavigator,
		ToolName:  "navigate",
		Arguments: map[string]interface{}{"href": "/console/files"},
	}); !blocked {
		t.Fatal("console-navigator/navigate repeated completed route was allowed")
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillConsoleNavigator,
		ToolName:  "navigate",
		Arguments: map[string]interface{}{"href": "/console/agents"},
	}); blocked {
		t.Fatal("console-navigator/navigate to a different console route was blocked, want advisory deviation")
	}
}

func TestSkillLoopPlanToolGuardBlocksRouteAlreadyLoadedByClientActionContext(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "保存到文件管理",
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillConsoleNavigator, skills.SkillFileManager},
			OperationContext: map[string]interface{}{
				"client_action_continuation": map[string]interface{}{
					"action_type": "route_navigation",
					"status":      clientActionStatusSucceeded,
					"href":        "/console/files",
					"result": map[string]interface{}{
						"observed_path": "/console/files",
					},
				},
			},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status": operationPlanStatusRunning,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillConsoleNavigator, "navigate"),
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillConsoleNavigator,
						"tool_name": "navigate",
					},
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management"),
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillFileManager,
						"tool_name": "save_file_to_management",
					},
				},
				"step_status": map[string]interface{}{
					operationPlanToolStepID(skills.SkillConsoleNavigator, "navigate"):           operationPlanStepStatusPending,
					operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management"): operationPlanStepStatusPending,
				},
				"original_user_goal": "保存到文件管理",
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillConsoleNavigator,
		ToolName:  "navigate",
		Arguments: map[string]interface{}{"href": "/console/files"},
	})
	if !blocked {
		t.Fatal("console-navigator/navigate repeated loaded client-action route was allowed")
	}
	if !strings.Contains(result.SystemMessage, "Do not call console-navigator/navigate again") {
		t.Fatalf("SystemMessage = %q, want repeated-navigation guidance", result.SystemMessage)
	}
}

func TestSkillLoopPlanToolGuardBlocksRouteAlreadyLoadedByMessageMetadata(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "打开文件管理",
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillConsoleNavigator},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"client_actions": []interface{}{
				map[string]interface{}{
					"action_type": "route_navigation",
					"status":      clientActionStatusSucceeded,
					"href":        "/console/files",
				},
			},
			"operation_plan": map[string]interface{}{
				"status": operationPlanStatusRunning,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillConsoleNavigator, "navigate"),
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillConsoleNavigator,
						"tool_name": "navigate",
					},
				},
				"step_status": map[string]interface{}{
					operationPlanToolStepID(skills.SkillConsoleNavigator, "navigate"): operationPlanStepStatusPending,
				},
				"original_user_goal": "打开文件管理",
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillConsoleNavigator,
		ToolName:  "navigate",
		Arguments: map[string]interface{}{"href": "/console/files"},
	}); !blocked {
		t.Fatal("console-navigator/navigate repeated metadata route was allowed")
	}
}

func TestSkillLoopPlanToolGuardBlocksRouteAlreadyPendingByMessageMetadata(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "save the generated file to file management",
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillConsoleNavigator, skills.SkillFileManager},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"client_action_continuation": map[string]interface{}{
				"action_id":   "route_navigation:/console/files",
				"action_type": "route_navigation",
				"status":      clientActionStatusWaiting,
				"skill_id":    skills.SkillConsoleNavigator,
				"tool_name":   "navigate",
				"href":        "/console/files",
			},
			"operation_plan": map[string]interface{}{
				"status": operationPlanStatusRunning,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillConsoleNavigator, "navigate"),
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillConsoleNavigator,
						"tool_name": "navigate",
					},
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management"),
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillFileManager,
						"tool_name": "save_file_to_management",
					},
				},
				"step_status": map[string]interface{}{
					operationPlanToolStepID(skills.SkillConsoleNavigator, "navigate"):           operationPlanStepStatusPending,
					operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management"): operationPlanStepStatusPending,
				},
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillConsoleNavigator,
		ToolName:  "navigate",
		Arguments: map[string]interface{}{"href": "/console/files"},
	})
	if !blocked {
		t.Fatal("console-navigator/navigate repeated pending metadata route was allowed")
	}
	if !strings.Contains(result.SystemMessage, "already loaded or already pending") {
		t.Fatalf("SystemMessage = %q, want pending-route guidance", result.SystemMessage)
	}
}

func TestSkillLoopPlanToolGuardAllowsSpecificRouteAfterCompletedParentRoute(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "open this agent detail page",
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillConsoleNavigator, skills.SkillAgentManagement},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status": operationPlanStatusRunning,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        operationPlanRouteStepID("/console/agents", 1),
						"status":    operationPlanStepStatusCompleted,
						"skill_id":  skills.SkillConsoleNavigator,
						"tool_name": "navigate",
						"asset_target": map[string]interface{}{
							"page": "/console/agents",
						},
					},
				},
				"step_status": map[string]interface{}{
					operationPlanRouteStepID("/console/agents", 1): operationPlanStepStatusCompleted,
				},
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillConsoleNavigator,
		ToolName:  "navigate",
		Arguments: map[string]interface{}{"href": "/console/agents/agent-1/agent"},
	}); blocked {
		t.Fatal("specific agent detail route was blocked by completed parent route")
	}
}

func TestRestrictResolvedSkillsForPreparedTurnKeepsSkillsVisibleWithPendingPlan(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          "继续处理配置，必要时导航到智能体页面",
			Surface:        aiChatSurfaceContextualSidebar,
			SkillMode:      skillModeAuto,
			SkillIDs:       []string{skills.SkillAgentManagement, skills.SkillConsoleNavigator},
			RuntimeContext: "route=/console/agents/agent-1/agent",
		},
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
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{
		{Metadata: skills.SkillMetadata{ID: skills.SkillAgentManagement}},
		{Metadata: skills.SkillMetadata{ID: skills.SkillConsoleNavigator}},
	}}

	filtered := restrictResolvedSkillsForPreparedTurn(prepared, resolved)
	got := filtered.SkillIDs()
	want := []string{skills.SkillAgentManagement, skills.SkillConsoleNavigator}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered skill ids = %#v, want all enabled resolved skills %#v", got, want)
	}

	resolved = &skills.ResolvedSkills{Skills: []skills.SkillDocument{
		{Metadata: skills.SkillMetadata{ID: skills.SkillConsoleNavigator}},
	}}
	filtered = restrictResolvedSkillsForPreparedTurn(prepared, resolved)
	if got := filtered.SkillIDs(); !reflect.DeepEqual(got, []string{skills.SkillConsoleNavigator}) {
		t.Fatalf("filtered skill ids = %#v, want existing resolved fallback skill to remain visible", got)
	}
}

func TestContextualConsoleAgentsSkillMessageOmitsNavigatorWhenPlanDoesNotAllowNavigation(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          "请编辑当前智能体配置，不要添加或删除 skill",
			Surface:        aiChatSurfaceContextualSidebar,
			SkillMode:      skillModeAuto,
			SkillIDs:       []string{skills.SkillAgentManagement, skills.SkillConsoleNavigator},
			RuntimeContext: "route=/console/agents/agent-1/agent",
			RawOperationContext: map[string]interface{}{
				"resources": []interface{}{
					map[string]interface{}{
						"resource_type": "page",
						"href":          "/console/agents/agent-1/agent",
					},
					map[string]interface{}{
						"resource_type": "agent",
						"id":            "agent-1",
						"title":         "目标智能体",
						"href":          "/console/agents/agent-1/agent",
						"selected":      true,
					},
				},
			},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"steps": []interface{}{
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"),
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "update_agent_config",
					},
				},
			},
		}},
	}
	prepared.parts.OperationContext = prepared.parts.RawOperationContext

	message, ok := contextualConsoleAgentsSkillMessage(prepared)
	if !ok {
		t.Fatal("contextualConsoleAgentsSkillMessage() = false, want message")
	}
	content := stringFromAny(message.Content)
	if strings.Contains(content, `"skill_id":"`+skills.SkillConsoleNavigator+`"`) {
		t.Fatalf("message content exposes console-navigator tool: %s", content)
	}
	if !strings.Contains(content, "Avoid console-navigator/navigate") ||
		!strings.Contains(content, "current goal cannot proceed from current page evidence") {
		t.Fatalf("message content missing soft navigation guidance: %s", content)
	}
}

func TestOperationPlanUpdatesFromSkillInvocation(t *testing.T) {
	parts := &chatRequestParts{
		Query:     "\u751f\u6210\u4e00\u4e2a\u4e34\u65f6 SVG \u6587\u4ef6",
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillFileGenerator},
	}
	metadata := streamingMessageMetadataWithTaskID(parts, "task-2")

	metadata = mergeSkillInvocationMetadata(metadata, []map[string]interface{}{{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillFileGenerator,
		"tool_name": "generate_file",
		"message":   "generated temporary.svg",
	}})
	plan := metadata["operation_plan"].(map[string]interface{})
	if plan["tool_result"] == nil {
		t.Fatalf("operation_plan tool_result missing in %#v", plan)
	}
	stepStatus := plan["step_status"].(map[string]interface{})
	if stepStatus["skill:"+skills.SkillFileGenerator] != operationPlanStepStatusCompleted {
		t.Fatalf("step_status = %#v, want file-generator completed", stepStatus)
	}
}

func TestOperationPlanToolResultIncludesAgentConfigEvidence(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{map[string]interface{}{
				"id":        operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"),
				"status":    operationPlanStepStatusPending,
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
			}},
			"step_status": map[string]interface{}{
				operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"): operationPlanStepStatusPending,
			},
			"status": operationPlanStatusRunning,
		},
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "update_agent_config",
		"result": map[string]interface{}{
			"status":                 "completed",
			"effect":                 "updated",
			"agent_id":               "agent-1",
			"workspace_id":           "workspace-1",
			"updated_fields":         []string{"model_provider", "model", "home_title"},
			"model_provider":         "deepseek",
			"model":                  "deepseek-chat",
			"agent_memory_enabled":   true,
			"enabled_skill_count":    2,
			"system_prompt":          "full prompt should not be copied into operation plan result",
			"knowledge_dataset_ids":  []string{"kb-1"},
			"database_bindings":      []string{"table-1"},
			"workflow_bindings":      []string{"workflow-1"},
			"suggested_questions":    []string{"question"},
			"model_parameters":       map[string]interface{}{"temperature": 0.7},
			"agent_memory_slot_text": "full memory detail should not be copied",
		},
	}})

	plan := metadata["operation_plan"].(map[string]interface{})
	if got := plan["status"]; got != operationPlanStatusCompleted {
		t.Fatalf("plan status = %#v, want completed; plan=%#v", got, plan)
	}
	toolResult := mapFromOperationContext(plan["tool_result"])
	summary := mapFromOperationContext(toolResult["result_summary"])
	if summary["agent_id"] != "agent-1" || summary["workspace_id"] != "workspace-1" {
		t.Fatalf("result_summary identity = %#v, want agent/workspace evidence", summary)
	}
	if summary["model_provider"] != "deepseek" || summary["model"] != "deepseek-chat" {
		t.Fatalf("result_summary model = %#v/%#v, want provider and model evidence; summary=%#v", summary["model_provider"], summary["model"], summary)
	}
	fields, ok := summary["updated_fields"].([]string)
	if !ok || len(fields) != 3 || fields[0] != "model_provider" || fields[1] != "model" || fields[2] != "home_title" {
		t.Fatalf("updated_fields = %#v, want exact updated field evidence", summary["updated_fields"])
	}
	if _, ok := summary["system_prompt"]; ok {
		t.Fatalf("result_summary should not copy full system prompt: %#v", summary)
	}
	if _, ok := summary["knowledge_dataset_ids"]; ok {
		t.Fatalf("result_summary should not copy raw binding ids: %#v", summary)
	}
}

func TestOperationPlanToolResultIncludesAgentBindingEvidence(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{map[string]interface{}{
				"id":        operationPlanToolStepID(skills.SkillAgentManagement, "replace_agent_database_bindings"),
				"status":    operationPlanStepStatusPending,
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "replace_agent_database_bindings",
			}},
			"step_status": map[string]interface{}{
				operationPlanToolStepID(skills.SkillAgentManagement, "replace_agent_database_bindings"): operationPlanStepStatusPending,
			},
			"status": operationPlanStatusRunning,
		},
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "replace_agent_database_bindings",
		"result": map[string]interface{}{
			"status":         "completed",
			"effect":         "updated",
			"agent_id":       "agent-1",
			"agent_name":     "Support Agent",
			"binding_kind":   "database_table",
			"resource_count": 2,
			"resource_names": []string{"CRM.customers", "CRM.orders"},
			"bindings":       []string{"raw-binding-id"},
		},
	}})

	plan := metadata["operation_plan"].(map[string]interface{})
	toolResult := mapFromOperationContext(plan["tool_result"])
	summary := mapFromOperationContext(toolResult["result_summary"])
	if summary["binding_kind"] != "database_table" || summary["resource_count"] != 2 {
		t.Fatalf("binding summary = %#v, want binding kind and resource count", summary)
	}
	names, ok := summary["resource_names"].([]string)
	if !ok || len(names) != 2 || names[0] != "CRM.customers" || names[1] != "CRM.orders" {
		t.Fatalf("resource_names = %#v, want visible resource names", summary["resource_names"])
	}
	if _, ok := summary["bindings"]; ok {
		t.Fatalf("result_summary should not copy raw binding payload: %#v", summary)
	}
}

func TestOperationPlanToolResultDerivesAgentBindingCountsFromConfig(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{map[string]interface{}{
				"id":        operationPlanToolStepID(skills.SkillAgentManagement, "replace_agent_skill_bindings"),
				"status":    operationPlanStepStatusPending,
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "replace_agent_skill_bindings",
			}},
			"step_status": map[string]interface{}{
				operationPlanToolStepID(skills.SkillAgentManagement, "replace_agent_skill_bindings"): operationPlanStepStatusPending,
			},
			"status": operationPlanStatusRunning,
		},
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "replace_agent_skill_bindings",
		"result": map[string]interface{}{
			"status":       "completed",
			"effect":       "updated",
			"agent_id":     "agent-1",
			"workspace_id": "workspace-1",
			"config": map[string]interface{}{
				"enabled_skill_ids": []interface{}{"chart-generator", "file-reader"},
			},
		},
	}})

	plan := metadata["operation_plan"].(map[string]interface{})
	if got := plan["status"]; got != operationPlanStatusCompleted {
		t.Fatalf("plan status = %#v, want completed with config binding evidence; plan=%#v", got, plan)
	}
	toolResult := mapFromOperationContext(plan["tool_result"])
	summary := mapFromOperationContext(toolResult["result_summary"])
	if summary["enabled_skill_count"] != 2 {
		t.Fatalf("enabled_skill_count = %#v, want 2; summary=%#v", summary["enabled_skill_count"], summary)
	}
	if _, ok := summary["config"]; ok {
		t.Fatalf("result_summary should not copy full config: %#v", summary)
	}
}

func TestOperationPlanToolResultDerivesAgentResourceBindingCountsFromConfig(t *testing.T) {
	tests := []struct {
		name        string
		toolName    string
		configKey   string
		countKey    string
		configValue interface{}
		wantCount   int
	}{
		{
			name:        "knowledge datasets",
			toolName:    "replace_agent_knowledge_bindings",
			configKey:   "knowledge_dataset_ids",
			countKey:    "knowledge_dataset_count",
			configValue: []interface{}{"dataset-1", "dataset-2"},
			wantCount:   2,
		},
		{
			name:      "database bindings",
			toolName:  "replace_agent_database_bindings",
			configKey: "database_bindings",
			countKey:  "database_binding_count",
			configValue: []interface{}{
				map[string]interface{}{"data_source_id": "db-1", "table_ids": []interface{}{"table-1"}},
			},
			wantCount: 1,
		},
		{
			name:      "workflow bindings",
			toolName:  "replace_agent_workflow_bindings",
			configKey: "workflow_bindings",
			countKey:  "workflow_binding_count",
			configValue: []interface{}{
				map[string]interface{}{"binding_id": "workflow-1", "label": "Approval Flow"},
				map[string]interface{}{"binding_id": "workflow-2", "label": "Refund Flow"},
			},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stepID := operationPlanToolStepID(skills.SkillAgentManagement, tt.toolName)
			metadata := map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"steps": []interface{}{map[string]interface{}{
						"id":        stepID,
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": tt.toolName,
					}},
					"step_status": map[string]interface{}{
						stepID: operationPlanStepStatusPending,
					},
					"status": operationPlanStatusRunning,
				},
			}

			applyOperationPlanInvocationState(metadata, []map[string]interface{}{{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": tt.toolName,
				"result": map[string]interface{}{
					"status":       "completed",
					"effect":       "updated",
					"agent_id":     "agent-1",
					"workspace_id": "workspace-1",
					"config": map[string]interface{}{
						tt.configKey: tt.configValue,
					},
				},
			}})

			plan := metadata["operation_plan"].(map[string]interface{})
			if got := plan["status"]; got != operationPlanStatusCompleted {
				t.Fatalf("plan status = %#v, want completed with %s evidence; plan=%#v", got, tt.countKey, plan)
			}
			toolResult := mapFromOperationContext(plan["tool_result"])
			summary := mapFromOperationContext(toolResult["result_summary"])
			if summary[tt.countKey] != tt.wantCount {
				t.Fatalf("%s = %#v, want %d; summary=%#v", tt.countKey, summary[tt.countKey], tt.wantCount, summary)
			}
			if _, ok := summary["config"]; ok {
				t.Fatalf("result_summary should not copy full config: %#v", summary)
			}
		})
	}
}

func TestOperationPlanToolResultIncludesManagedFileEvidence(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{map[string]interface{}{
				"id":        operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management"),
				"status":    operationPlanStepStatusPending,
				"skill_id":  skills.SkillFileManager,
				"tool_name": "save_file_to_management",
			}},
			"step_status": map[string]interface{}{
				operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management"): operationPlanStepStatusPending,
			},
			"status": operationPlanStatusRunning,
		},
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillFileManager,
		"tool_name": "save_file_to_management",
		"result": map[string]interface{}{
			"status":          "completed",
			"target":          "managed_file",
			"file_id":         "managed-file-1",
			"upload_file_id":  "managed-file-1",
			"source_file_id":  "tool-file-1",
			"filename":        "report.svg",
			"workspace_id":    "workspace-1",
			"mime_type":       "image/svg+xml",
			"size":            2048,
			"source_type":     "tool_file",
			"download_url":    "/console/api/files/managed-file-1/download",
			"source_file_url": "should-not-copy",
		},
	}})

	plan := metadata["operation_plan"].(map[string]interface{})
	if got := plan["status"]; got != operationPlanStatusCompleted {
		t.Fatalf("plan status = %#v, want completed; plan=%#v", got, plan)
	}
	toolResult := mapFromOperationContext(plan["tool_result"])
	summary := mapFromOperationContext(toolResult["result_summary"])
	for _, key := range []string{"file_id", "upload_file_id", "source_file_id", "filename", "workspace_id", "mime_type", "size", "source_type"} {
		if _, ok := summary[key]; !ok {
			t.Fatalf("result_summary missing %s in %#v", key, summary)
		}
	}
	if summary["file_id"] != "managed-file-1" || summary["upload_file_id"] != "managed-file-1" {
		t.Fatalf("managed file evidence = %#v, want managed-file-1 ids", summary)
	}
	if summary["source_file_id"] != "tool-file-1" || summary["filename"] != "report.svg" {
		t.Fatalf("source/name evidence = %#v, want tool-file-1/report.svg", summary)
	}
	if _, ok := summary["download_url"]; ok {
		t.Fatalf("result_summary should not copy download_url: %#v", summary)
	}
	if _, ok := summary["source_file_url"]; ok {
		t.Fatalf("result_summary should not copy source_file_url: %#v", summary)
	}
}

func TestOperationPlanDoesNotCompleteManagedFileSaveWithoutIdentity(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{map[string]interface{}{
				"id":        operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management"),
				"status":    operationPlanStepStatusPending,
				"skill_id":  skills.SkillFileManager,
				"tool_name": "save_file_to_management",
			}},
			"step_status": map[string]interface{}{
				operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management"): operationPlanStepStatusPending,
			},
			"status": operationPlanStatusRunning,
		},
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillFileManager,
		"tool_name": "save_file_to_management",
		"result": map[string]interface{}{
			"status":   "completed",
			"target":   "managed_file",
			"filename": "report.svg",
		},
	}})

	plan := metadata["operation_plan"].(map[string]interface{})
	if got := plan["status"]; got != operationPlanStatusRunning {
		t.Fatalf("plan status = %#v, want running without managed file identity; plan=%#v", got, plan)
	}
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management")); got != operationPlanStepStatusPending {
		t.Fatalf("save step status = %#v, want pending without managed file identity; plan=%#v", got, plan)
	}
}

func TestOperationPlanDoesNotCompleteAgentConfigWithoutUpdatedFields(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{map[string]interface{}{
				"id":        operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"),
				"status":    operationPlanStepStatusPending,
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
			}},
			"step_status": map[string]interface{}{
				operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"): operationPlanStepStatusPending,
			},
			"status": operationPlanStatusRunning,
		},
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "update_agent_config",
		"result": map[string]interface{}{
			"status":   "completed",
			"effect":   "updated",
			"agent_id": "agent-1",
			"model":    "deepseek-chat",
		},
	}})

	plan := metadata["operation_plan"].(map[string]interface{})
	if got := plan["status"]; got != operationPlanStatusRunning {
		t.Fatalf("plan status = %#v, want running without updated_fields; plan=%#v", got, plan)
	}
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")); got != operationPlanStepStatusPending {
		t.Fatalf("update_agent_config status = %#v, want pending without updated_fields; plan=%#v", got, plan)
	}
}

func TestOperationPlanFailsReadFileWhenContentExtractionFails(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{map[string]interface{}{
				"id":        operationPlanToolStepID(skills.SkillFileReader, "read_file"),
				"status":    operationPlanStepStatusPending,
				"skill_id":  skills.SkillFileReader,
				"tool_name": "read_file",
			}},
			"step_status": map[string]interface{}{
				operationPlanToolStepID(skills.SkillFileReader, "read_file"): operationPlanStepStatusPending,
			},
			"status": operationPlanStatusRunning,
		},
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillFileReader,
		"tool_name": "read_file",
		"result": map[string]interface{}{
			"status":            "completed",
			"content_status":    "error",
			"content_error":     "file content extraction returned no result",
			"content":           "",
			"content_chars":     0,
			"content_truncated": false,
			"file": map[string]interface{}{
				"id":   "file-1",
				"name": "broken.pdf",
			},
		},
	}})

	plan := metadata["operation_plan"].(map[string]interface{})
	if got := plan["status"]; got != operationPlanStatusFailed {
		t.Fatalf("plan status = %#v, want failed for read_file content error; plan=%#v", got, plan)
	}
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillFileReader, "read_file")); got != operationPlanStepStatusFailed {
		t.Fatalf("read_file status = %#v, want failed for content error; plan=%#v", got, plan)
	}
}

func TestOperationPlanToolResultIncludesNestedFileIdentityEvidence(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{map[string]interface{}{
				"id":        operationPlanToolStepID(skills.SkillFileReader, "read_file"),
				"status":    operationPlanStepStatusPending,
				"skill_id":  skills.SkillFileReader,
				"tool_name": "read_file",
			}},
			"step_status": map[string]interface{}{
				operationPlanToolStepID(skills.SkillFileReader, "read_file"): operationPlanStepStatusPending,
			},
			"status": operationPlanStatusRunning,
		},
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillFileReader,
		"tool_name": "read_file",
		"result": map[string]interface{}{
			"status":            "completed",
			"content_status":    "extracted",
			"content":           "hello",
			"content_chars":     5,
			"content_truncated": false,
			"file": map[string]interface{}{
				"id":           "file-1",
				"name":         "notes.md",
				"workspace_id": "workspace-1",
				"mime_type":    "text/markdown",
				"size":         64,
			},
		},
	}})

	plan := metadata["operation_plan"].(map[string]interface{})
	if got := plan["status"]; got != operationPlanStatusCompleted {
		t.Fatalf("plan status = %#v, want completed with read content evidence; plan=%#v", got, plan)
	}
	toolResult := mapFromOperationContext(plan["tool_result"])
	summary := mapFromOperationContext(toolResult["result_summary"])
	if summary["file_id"] != "file-1" || summary["filename"] != "notes.md" || summary["workspace_id"] != "workspace-1" {
		t.Fatalf("nested file identity summary = %#v, want file/name/workspace evidence", summary)
	}
}

func TestOperationPlanAgentManagementStepsIncludeAssetTargets(t *testing.T) {
	plan := operationPlanFromTurnStrategy("task-agent-targets", &chatRequestParts{
		Query:   "update the current agent model and database bindings",
		Surface: aiChatSurfaceContextualSidebar,
	}, &AIChatTurnStrategy{
		Intent: "manage_agent_asset",
		PlannedTools: []AIChatTurnStrategyTool{
			{SkillID: skills.SkillAgentManagement, ToolName: "update_agent_config"},
			{SkillID: skills.SkillAgentManagement, ToolName: "replace_agent_database_bindings"},
			{SkillID: skills.SkillAgentManagement, ToolName: "delete_agent"},
		},
	})

	if got := operationPlanStepAssetTargetForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"), "asset_type"); got != "agent" {
		t.Fatalf("update_agent_config asset_type = %#v, want agent; plan=%#v", got, plan)
	}
	if got := operationPlanStepAssetTargetForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "replace_agent_database_bindings"), "asset_type"); got != "database_table" {
		t.Fatalf("replace_agent_database_bindings asset_type = %#v, want database_table; plan=%#v", got, plan)
	}
	if got := operationPlanStepAssetTargetForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "delete_agent"), "effect"); got != "delete" {
		t.Fatalf("delete_agent effect = %#v, want delete; plan=%#v", got, plan)
	}
}

func TestAgentManagementDeleteIntentAllowsSeparatedChineseDeleteVerb(t *testing.T) {
	query := "\u5e2e\u6211\u5220\u9664\u8fd9\u4e2a\u9875\u9762\u7684\u524d\u56db\u4e2a\u667a\u80fd\u4f53"
	if !agentManagementDeleteRequested(query) {
		t.Fatalf("agentManagementDeleteRequested(%q) = false, want true", query)
	}
	if !agentManagementBatchDeleteRequested(query) {
		t.Fatalf("agentManagementBatchDeleteRequested(%q) = false, want true", query)
	}
	parts := &chatRequestParts{
		Query:     query,
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillAgentManagement},
	}
	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{Intent: "manage_agent_asset"})
	if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "delete_agents") {
		t.Fatalf("planned_tools = %#v, missing delete_agents", strategy.PlannedTools)
	}
	plan := operationPlanFromTurnStrategy("task-agent-delete", parts, strategy)
	stepID := operationPlanToolStepID(skills.SkillAgentManagement, "delete_agents")
	stepStatus := mapFromOperationContext(plan["step_status"])
	if got := stringFromAny(stepStatus[stepID]); got != operationPlanStepStatusPending {
		t.Fatalf("step_status[%s] = %q, want pending; plan=%#v", stepID, got, plan)
	}
	if got := operationPlanStepAssetTargetForTest(plan, stepID, "operation_mode"); got != "batch" {
		t.Fatalf("delete_agents operation_mode = %#v, want batch; plan=%#v", got, plan)
	}
}

func TestAgentManagementDeleteIntentDoesNotPlanEditsFromTargetNames(t *testing.T) {
	query := "\u8bf7\u5220\u9664\u5f53\u524d\u667a\u80fd\u4f53\u9875\u9762\u524d\u4e24\u4e2a\u53ef\u89c1\u667a\u80fd\u4f53\uff1aGOAL-CONFIG-AGENT-1782403819308-EDITED9 \u548c AIChat\u914d\u7f6e\u9a8c\u8bc106231035-\u5df2\u7f16\u8f91"
	parts := &chatRequestParts{
		Query:     query,
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillAgentManagement},
	}
	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{Intent: "manage_agent_asset"})
	if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "delete_agents") {
		t.Fatalf("planned_tools = %#v, missing delete_agents", strategy.PlannedTools)
	}
	for _, unexpected := range []string{"get_agent_config", "update_agent_identity", "update_agent_config"} {
		if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, unexpected) {
			t.Fatalf("planned_tools = %#v, want no %s from target-name keywords", strategy.PlannedTools, unexpected)
		}
	}
}

func TestAgentManagementDeleteCreatedReferenceDoesNotPlanCreate(t *testing.T) {
	query := "\u5220\u9664\u521a\u521a\u521b\u5efa\u7684\u8fd9\u4e24\u4e2a\u6d4b\u8bd5 Agent\uff1aAICHAT-BATCH-SMOKE-A \u548c AICHAT-BATCH-SMOKE-B\u3002\u8bf7\u4e00\u6b21\u6027\u6279\u91cf\u5220\u9664\uff0c\u5b8c\u6210\u540e\u505c\u7559\u5728\u667a\u80fd\u4f53\u5217\u8868\u9875\uff0c\u5e76\u544a\u8bc9\u6211\u6bcf\u4e2a\u5220\u9664\u7ed3\u679c\u3002"
	if !agentManagementCreateRequested(query) {
		t.Fatalf("agentManagementCreateRequested(%q) = false before reference filter, want raw marker detected", query)
	}
	if !agentManagementCreateMentionIsDeleteTargetReference(query) {
		t.Fatalf("agentManagementCreateMentionIsDeleteTargetReference(%q) = false, want true", query)
	}
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
	if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "create_agent") {
		t.Fatalf("planned_tools = %#v, want stale create_agent removed for delete target reference", strategy.PlannedTools)
	}
	if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "delete_agents") {
		t.Fatalf("planned_tools = %#v, missing delete_agents", strategy.PlannedTools)
	}
	plan := operationPlanFromTurnStrategy("task-agent-delete-created-reference", parts, strategy)
	if _, ok := mapFromOperationContext(plan["step_status"])[operationPlanToolStepID(skills.SkillAgentManagement, "create_agent")]; ok {
		t.Fatalf("plan = %#v, want no create_agent step", plan)
	}
}

func TestAgentManagementDeleteThenCreateStillPlansCreate(t *testing.T) {
	query := "\u5220\u9664\u521a\u521a\u521b\u5efa\u7684\u8fd9\u4e24\u4e2a Agent\uff0c\u7136\u540e\u518d\u521b\u5efa\u4e00\u4e2a\u65b0\u7684 Agent"
	if agentManagementCreateMentionIsDeleteTargetReference(query) {
		t.Fatalf("agentManagementCreateMentionIsDeleteTargetReference(%q) = true, want false when user asks to create again", query)
	}
	parts := &chatRequestParts{
		Query:     query,
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillAgentManagement},
	}
	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{Intent: "manage_agent_asset"})
	if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "create_agent") {
		t.Fatalf("planned_tools = %#v, missing create_agent for explicit follow-up create", strategy.PlannedTools)
	}
	if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "delete_agents") {
		t.Fatalf("planned_tools = %#v, missing delete_agents", strategy.PlannedTools)
	}
}

func TestAgentManagementCreateDescriptionDoesNotPlanDelete(t *testing.T) {
	query := `create 2 draft agents named Smoke-A and Smoke-B with description "AIChat smoke test, deletable"; do not navigate to detail page`
	if !agentManagementCreateRequested(query) {
		t.Fatalf("agentManagementCreateRequested(%q) = false, want true", query)
	}
	if agentManagementDeleteRequested(query) {
		t.Fatalf("agentManagementDeleteRequested(%q) = true, want false for descriptive deletable text", query)
	}
	if wantsCreatedAgentDetailNavigation(query) {
		t.Fatalf("wantsCreatedAgentDetailNavigation(%q) = true, want false for explicit no-navigation request", query)
	}
	parts := &chatRequestParts{
		Query:     query,
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillAgentManagement},
	}
	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{Intent: "manage_agent_asset"})
	if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "create_agent") {
		t.Fatalf("planned_tools = %#v, missing create_agent", strategy.PlannedTools)
	}
	if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "delete_agents") ||
		aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "delete_agent") {
		t.Fatalf("planned_tools = %#v, want no delete tool for descriptive deletable text", strategy.PlannedTools)
	}
}

func TestAgentManagementCreateQuotedChineseDescriptionDoesNotPlanDelete(t *testing.T) {
	query := "\u5192\u70df\u51c6\u5907\uff1a\u8bf7\u521b\u5efa\u4e24\u4e2a\u8349\u7a3f\u667a\u80fd\u4f53\uff0c\u540d\u79f0\u5206\u522b\u4e3a PLAN-A \u548c PLAN-B\uff0c\u63cf\u8ff0\u90fd\u5199\u201cAIChat \u6279\u91cf\u5220\u9664\u56de\u5f52\u6d4b\u8bd5\u201d\u3002\u4e0d\u8981\u5bfc\u822a\u5230\u8be6\u60c5\u9875\u3002\u5b8c\u6210\u540e\u544a\u8bc9\u6211\u521b\u5efa\u7ed3\u679c\u3002"
	if !agentManagementCreateRequested(query) {
		t.Fatalf("agentManagementCreateRequested(%q) = false, want true", query)
	}
	if agentManagementDeleteRequested(query) {
		t.Fatalf("agentManagementDeleteRequested(%q) = true, want false for quoted description payload", query)
	}
	if agentManagementBatchDeleteRequested(query) {
		t.Fatalf("agentManagementBatchDeleteRequested(%q) = true, want false for quoted description payload", query)
	}
	if wantsCreatedAgentDetailNavigation(query) {
		t.Fatalf("wantsCreatedAgentDetailNavigation(%q) = true, want false for explicit no-navigation request", query)
	}

	parts := &chatRequestParts{
		Query:     query,
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillAgentManagement},
	}
	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{Intent: "manage_agent_asset"})
	if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "create_agent") {
		t.Fatalf("planned_tools = %#v, missing create_agent", strategy.PlannedTools)
	}
	for _, unexpected := range []string{"delete_agent", "delete_agents"} {
		if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, unexpected) {
			t.Fatalf("planned_tools = %#v, want no %s for quoted description payload", strategy.PlannedTools, unexpected)
		}
	}

	plan := operationPlanFromTurnStrategy("task-agent-create-quoted-description", parts, strategy)
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "create_agent")); got != operationPlanStepStatusPending {
		t.Fatalf("create_agent step status = %q, want pending; plan=%#v", got, plan)
	}
	for _, unexpected := range []string{"delete_agent", "delete_agents"} {
		if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, unexpected)); got != "" {
			t.Fatalf("%s step status = %q, want absent; plan=%#v", unexpected, got, plan)
		}
	}
}

func TestAgentManagementDeleteQuotedTargetStillWorks(t *testing.T) {
	query := "\u8bf7\u5220\u9664\u540d\u4e3a\u201cPLAN-A\u201d\u7684\u667a\u80fd\u4f53"
	if !agentManagementDeleteRequested(query) {
		t.Fatalf("agentManagementDeleteRequested(%q) = false, want true", query)
	}
	if agentManagementBatchDeleteRequested(query) {
		t.Fatalf("agentManagementBatchDeleteRequested(%q) = true, want false for single target", query)
	}
}

func TestWantsCreatedAgentDetailNavigationHonorsChineseNegation(t *testing.T) {
	query := "\u521b\u5efa 2 \u4e2a\u667a\u80fd\u4f53\uff0c\u4e0d\u8981\u5bfc\u822a\u5230\u8be6\u60c5\u9875"
	if wantsCreatedAgentDetailNavigation(query) {
		t.Fatalf("wantsCreatedAgentDetailNavigation(%q) = true, want false", query)
	}
}

func TestAgentManagementVisiblePageTargetsDoNotPlanListOrNavigate(t *testing.T) {
	parts := consoleAgentsVisibleTargetsTestParts("delete the first two visible agents on this page")
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	if strategy.Intent != "manage_agent_asset" {
		t.Fatalf("strategy.Intent = %q, want manage_agent_asset; strategy=%#v", strategy.Intent, strategy)
	}
	if strategy.RouteRequired {
		t.Fatalf("strategy.RouteRequired = true, want false; strategy=%#v", strategy)
	}
	if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "delete_agents") {
		t.Fatalf("PlannedTools = %#v, missing agent-management/delete_agents", strategy.PlannedTools)
	}
	for _, unexpected := range []struct {
		skillID  string
		toolName string
	}{
		{skills.SkillAgentManagement, "list_agents"},
		{skills.SkillConsoleNavigator, "navigate"},
	} {
		if aiChatTurnStrategyHasPlannedToolForTest(strategy, unexpected.skillID, unexpected.toolName) {
			t.Fatalf("PlannedTools = %#v, want no %s/%s", strategy.PlannedTools, unexpected.skillID, unexpected.toolName)
		}
	}
	if !aiChatTurnStrategyAvoidContainsForTest(strategy, "avoid redundant agent-management/list_agents") {
		t.Fatalf("strategy.Avoid = %#v, want visible target list_agents avoidance", strategy.Avoid)
	}
	criteria := strings.Join(strategy.SuccessCriteria, "\n")
	if strings.Contains(criteria, "publish, bind, and invoke are not attempted") {
		t.Fatalf("strategy.SuccessCriteria contains stale binding prohibition: %#v", strategy.SuccessCriteria)
	}
	if !strings.Contains(criteria, "binding and unbinding edits use supported draft config binding lists") {
		t.Fatalf("strategy.SuccessCriteria = %#v, want supported binding/unbinding guidance", strategy.SuccessCriteria)
	}
}

func TestSkillLoopPlanGuardAllowsListAgentsForResolvedVisibleAgentTargets(t *testing.T) {
	parts := consoleAgentsVisibleTargetsTestParts("delete the first two visible agents on this page")
	prepared := &PreparedChat{
		parts: parts,
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": operationPlanFromTurnStrategy("task-visible-agent-delete", parts, &AIChatTurnStrategy{
				Intent: "manage_agent_asset",
				PlannedTools: []AIChatTurnStrategyTool{
					{SkillID: skills.SkillAgentManagement, ToolName: "delete_agents"},
				},
			}),
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopPlanToolCallGuard() = nil, want guard")
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillAgentManagement, ToolName: "delete_agents"}); blocked {
		t.Fatal("delete_agents was blocked, want planned batch deletion allowed")
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillAgentManagement, ToolName: "list_agents"}); blocked {
		t.Fatal("list_agents was blocked, want soft planner guidance without hard guard interception")
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "list_agents")); got != operationPlanStepStatusPending {
		t.Fatalf("list_agents amended step status = %#v, want pending; plan=%#v", got, plan)
	}
}

func TestContextualConsoleAgentsSkillMessagePrioritizesVisibleTargets(t *testing.T) {
	parts := consoleAgentsVisibleTargetsTestParts("delete the first two visible agents on this page")
	prepared := &PreparedChat{
		parts: parts,
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": operationPlanFromTurnStrategy("task-visible-agent-delete", parts, contextualAIChatTurnStrategyFromParts(parts)),
		}},
	}

	message, ok := contextualConsoleAgentsSkillMessage(prepared)
	if !ok {
		t.Fatal("contextualConsoleAgentsSkillMessage() = false, want message")
	}
	content := stringFromAny(message.Content)
	for _, want := range []string{
		"visible_agents as authoritative resolved targets",
		"do not call list_agents only to rediscover the same visible targets",
		"read current config or list exact candidates only when the needed current binding set or candidate IDs/names are not already present",
		"Do not navigate after deleting Agents from the list page",
		"list or search Agents when visible page context is missing or insufficient",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("message content missing %q in:\n%s", want, content)
		}
	}
	for _, stale := range []string{
		"list or search visible Agents",
		"call it only for the planned href before the final answer",
	} {
		if strings.Contains(content, stale) {
			t.Fatalf("message content contains stale hard planner guidance %q in:\n%s", stale, content)
		}
	}
	if strings.Contains(content, "Binding edits must be done in three steps") {
		t.Fatalf("message content contains stale unconditional list-first binding guidance:\n%s", content)
	}
}

func TestOperationPlanRecordsAgentBatchOperationGroup(t *testing.T) {
	parts := &chatRequestParts{
		Query:     "\u5e2e\u6211\u5220\u9664\u8fd9\u4e2a\u9875\u9762\u7684\u524d\u4e24\u4e2a\u667a\u80fd\u4f53",
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillAgentManagement},
	}
	strategy := &AIChatTurnStrategy{
		Intent: "manage_agent_asset",
		PlannedTools: []AIChatTurnStrategyTool{
			{SkillID: skills.SkillAgentManagement, ToolName: "delete_agents"},
		},
	}
	metadata := map[string]interface{}{
		"operation_plan": operationPlanFromTurnStrategy("task-agent-batch-delete", parts, strategy),
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "delete_agents",
		"result": map[string]interface{}{
			"status":        "partial_failed",
			"target_count":  2,
			"deleted_count": 1,
			"failed_count":  1,
			"item_results": []interface{}{
				map[string]interface{}{"agent_id": "agent-1", "agent_name": "Agent One", "status": "succeeded"},
				map[string]interface{}{"agent_id": "agent-2", "agent_name": "Agent Two", "status": "failed", "error": "locked"},
			},
			"operation_group": map[string]interface{}{
				"id":            "agent.delete.batch:test",
				"type":          "batch",
				"operation":     "agent.delete",
				"asset_type":    "agent",
				"status":        "partial_failed",
				"target_count":  2,
				"success_count": 1,
				"failed_count":  1,
				"targets": []interface{}{
					map[string]interface{}{"agent_id": "agent-1", "name": "Agent One", "type": "agent"},
					map[string]interface{}{"agent_id": "agent-2", "name": "Agent Two", "type": "agent"},
				},
				"item_results": []interface{}{
					map[string]interface{}{"agent_id": "agent-1", "agent_name": "Agent One", "status": "succeeded"},
					map[string]interface{}{"agent_id": "agent-2", "agent_name": "Agent Two", "status": "failed", "error": "locked"},
				},
			},
		},
	}})

	plan := mapFromOperationContext(metadata["operation_plan"])
	if plan["status"] != operationPlanStatusCompleted {
		t.Fatalf("plan status = %#v, want completed with partial item facts; plan=%#v", plan["status"], plan)
	}
	group := mapFromOperationContext(plan["operation_group"])
	if group["id"] != "agent.delete.batch:test" || group["failed_count"] != 1 {
		t.Fatalf("operation_group = %#v, want compact batch facts", group)
	}
	targetSet := mapSliceFromAny(plan["target_set"])
	if len(targetSet) != 2 || targetSet[0]["name"] != "Agent One" {
		t.Fatalf("target_set = %#v, want two named targets", targetSet)
	}
	itemSteps := mapSliceFromAny(plan["item_steps"])
	if len(itemSteps) != 2 || itemSteps[1]["status"] != "failed" {
		t.Fatalf("item_steps = %#v, want per-target failed status", itemSteps)
	}
	stepID := operationPlanToolStepID(skills.SkillAgentManagement, "delete_agents")
	step := operationPlanStepForTest(plan, stepID)
	if got := mapSliceFromAny(step["target_set"]); len(got) != 2 {
		t.Fatalf("step target_set = %#v, want two targets; step=%#v", got, step)
	}
	if got := mapSliceFromAny(step["item_steps"]); len(got) != 2 {
		t.Fatalf("step item_steps = %#v, want two item steps; step=%#v", got, step)
	}
}

func TestOperationPlanBatchDeleteCompletesSingleDeleteStep(t *testing.T) {
	parts := &chatRequestParts{
		Query:     "delete the first two visible agents",
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillAgentManagement},
	}
	strategy := &AIChatTurnStrategy{
		Intent: "manage_agent_asset",
		PlannedTools: []AIChatTurnStrategyTool{
			{SkillID: skills.SkillAgentManagement, ToolName: "delete_agent"},
		},
	}
	metadata := map[string]interface{}{
		"operation_plan": operationPlanFromTurnStrategy("task-agent-delete", parts, strategy),
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "delete_agents",
		"result": map[string]interface{}{
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
	}})

	plan := mapFromOperationContext(metadata["operation_plan"])
	deleteStepID := operationPlanToolStepID(skills.SkillAgentManagement, "delete_agent")
	deleteStep := operationPlanStepForTest(plan, deleteStepID)
	if got := stringFromAny(deleteStep["status"]); got != operationPlanStepStatusCompleted {
		t.Fatalf("delete_agent step status = %q, want completed from batch delete evidence; plan=%#v", got, plan)
	}
	stepStatus := mapFromOperationContext(plan["step_status"])
	if got := stringFromAny(stepStatus[deleteStepID]); got != operationPlanStepStatusCompleted {
		t.Fatalf("step_status[%s] = %q, want completed; plan=%#v", deleteStepID, got, plan)
	}
	if got := stringFromAny(plan["pending_next_action"]); strings.Contains(got, "delete_agent") {
		t.Fatalf("pending_next_action = %q, want no stale delete_agent after batch evidence; plan=%#v", got, plan)
	}
}

func TestOperationPlanBatchDeleteRequiresEvidenceForEveryTarget(t *testing.T) {
	parts := &chatRequestParts{
		Query:     "delete the first two visible agents",
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillAgentManagement},
	}
	strategy := &AIChatTurnStrategy{
		Intent: "manage_agent_asset",
		PlannedTools: []AIChatTurnStrategyTool{
			{SkillID: skills.SkillAgentManagement, ToolName: "delete_agents"},
		},
	}
	metadata := map[string]interface{}{
		"operation_plan": operationPlanFromTurnStrategy("task-agent-incomplete-batch-delete", parts, strategy),
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "delete_agents",
		"result": map[string]interface{}{
			"status":        "running",
			"target_count":  2,
			"deleted_count": 1,
			"failed_count":  0,
			"item_results": []interface{}{
				map[string]interface{}{"agent_id": "agent-1", "agent_name": "Agent One", "status": "succeeded"},
			},
			"operation_group": map[string]interface{}{
				"id":            "agent.delete.batch:test",
				"type":          "batch",
				"operation":     "agent.delete",
				"asset_type":    "agent",
				"status":        "running",
				"target_count":  2,
				"success_count": 1,
				"failed_count":  0,
				"item_results": []interface{}{
					map[string]interface{}{"agent_id": "agent-1", "agent_name": "Agent One", "status": "succeeded"},
				},
			},
		},
	}})

	plan := mapFromOperationContext(metadata["operation_plan"])
	if got := stringFromAny(plan["status"]); got == operationPlanStatusCompleted {
		t.Fatalf("plan status = %q, want running while batch evidence covers only one of two targets; plan=%#v", got, plan)
	}
	stepID := operationPlanToolStepID(skills.SkillAgentManagement, "delete_agents")
	step := operationPlanStepForTest(plan, stepID)
	if got := stringFromAny(step["status"]); got == operationPlanStepStatusCompleted {
		t.Fatalf("delete_agents step status = %q, want pending until every target has an item result; step=%#v", got, step)
	}
}

func TestOperationPlanUpdatesFromGeneratedArtifactMetadata(t *testing.T) {
	parts := &chatRequestParts{
		Query:     "\u751f\u6210\u4e00\u4e2a\u4e34\u65f6 SVG \u6587\u4ef6",
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillFileGenerator},
	}
	metadata := streamingMessageMetadataWithTaskID(parts, "task-artifact")

	metadata = mergeGeneratedArtifactMetadata(metadata, map[string]interface{}{
		"file_id":   "tool-file-1",
		"filename":  "temporary.svg",
		"extension": ".svg",
		"target":    "temporary_artifact",
		"skill_id":  skills.SkillFileGenerator,
		"tool_name": "generate_file",
	})

	plan := metadata["operation_plan"].(map[string]interface{})
	assetState := plan["asset_state"].(map[string]interface{})
	if assetState["temporary_count"] != 1 {
		t.Fatalf("asset_state = %#v, want one temporary artifact", assetState)
	}
	if assetState["generated_count"] != 1 || assetState["logical_asset_count"] != 1 || assetState["lifecycle_record_count"] != 1 {
		t.Fatalf("asset_state = %#v, want one logical artifact and one lifecycle record", assetState)
	}
	stepStatus := plan["step_status"].(map[string]interface{})
	if stepStatus["skill:"+skills.SkillFileGenerator] != operationPlanStepStatusCompleted {
		t.Fatalf("step_status = %#v, want file-generator completed", stepStatus)
	}
	if stepStatus["observe"] != operationPlanStepStatusCompleted {
		t.Fatalf("step_status = %#v, want observe completed", stepStatus)
	}
	if plan["status"] != operationPlanStatusCompleted {
		t.Fatalf("plan status = %#v, want completed", plan["status"])
	}
}

func TestOperationPlanSupportingSkillDoesNotBlockTemporaryArtifactCompletion(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{
				map[string]interface{}{
					"id":        operationPlanToolStepID(skills.SkillFileGenerator, "generate_file"),
					"title":     "Generate temporary file",
					"status":    operationPlanStepStatusPending,
					"skill_id":  skills.SkillFileGenerator,
					"tool_name": "generate_file",
				},
				map[string]interface{}{
					"id":       "skill:" + skills.SkillFileGenerator,
					"title":    "Use file-generator",
					"status":   operationPlanStepStatusPending,
					"skill_id": skills.SkillFileGenerator,
					"role":     "primary",
				},
				map[string]interface{}{
					"id":       "skill:" + skills.SkillConsoleNavigator,
					"title":    "Use console-navigator",
					"status":   operationPlanStepStatusPending,
					"skill_id": skills.SkillConsoleNavigator,
					"role":     "supporting",
				},
				map[string]interface{}{
					"id":     "observe",
					"title":  "Observe result",
					"status": operationPlanStepStatusPending,
				},
			},
			"step_status": map[string]interface{}{
				operationPlanToolStepID(skills.SkillFileGenerator, "generate_file"): operationPlanStepStatusPending,
				"skill:" + skills.SkillFileGenerator:                                operationPlanStepStatusPending,
				"skill:" + skills.SkillConsoleNavigator:                             operationPlanStepStatusPending,
				"observe":                                                           operationPlanStepStatusPending,
			},
			"status":              operationPlanStatusRunning,
			"pending_next_action": "Generate temporary file",
		},
	}

	metadata = mergeGeneratedArtifactMetadata(metadata, map[string]interface{}{
		"file_id":   "tool-file-1",
		"filename":  "temporary.svg",
		"extension": ".svg",
		"target":    "temporary_artifact",
		"skill_id":  skills.SkillFileGenerator,
		"tool_name": "generate_file",
	})

	plan := metadata["operation_plan"].(map[string]interface{})
	if plan["status"] != operationPlanStatusCompleted {
		t.Fatalf("plan status = %#v, want completed", plan["status"])
	}
	if plan["pending_next_action"] != "none" {
		t.Fatalf("pending_next_action = %#v, want none", plan["pending_next_action"])
	}
	if operationPlanHasIncompleteWork(plan) {
		t.Fatalf("operationPlanHasIncompleteWork(%#v) = true, want false", plan)
	}
	stepStatus := plan["step_status"].(map[string]interface{})
	if stepStatus["skill:"+skills.SkillConsoleNavigator] != operationPlanStepStatusPending {
		t.Fatalf("supporting navigator status = %#v, want still pending as non-blocking evidence", stepStatus["skill:"+skills.SkillConsoleNavigator])
	}
}

func TestPreparedResultMetadataCompletesObserveOnlyOperationPlan(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{
				map[string]interface{}{
					"id":       "skill:file-reader",
					"title":    "Use file-reader",
					"status":   operationPlanStepStatusPending,
					"skill_id": "file-reader",
					"role":     "primary",
				},
				map[string]interface{}{
					"id":     "observe",
					"title":  "Observe result",
					"status": operationPlanStepStatusPending,
				},
			},
			"step_status": map[string]interface{}{
				"skill:file-reader": operationPlanStepStatusPending,
				"observe":           operationPlanStepStatusPending,
			},
			"status":              operationPlanStatusRunning,
			"pending_next_action": "Use file-reader",
		},
	}

	metadata = preparedResultMetadata(metadata, nil)

	plan := metadata["operation_plan"].(map[string]interface{})
	if plan["status"] != operationPlanStatusCompleted {
		t.Fatalf("plan status = %#v, want completed", plan["status"])
	}
	if plan["pending_next_action"] != "none" {
		t.Fatalf("pending_next_action = %#v, want none", plan["pending_next_action"])
	}
	if operationPlanHasIncompleteWork(plan) {
		t.Fatalf("operationPlanHasIncompleteWork(%#v) = true, want false", plan)
	}
	for _, id := range []string{"skill:file-reader", "observe"} {
		if got := operationPlanStepStatusForTest(plan, id); got != operationPlanStepStatusCompleted {
			t.Fatalf("step %s status = %#v, want completed; plan=%#v", id, got, plan)
		}
	}
}

func TestPreparedResultMetadataCompletesObserveAfterToolResult(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{
				map[string]interface{}{
					"id":        operationPlanToolStepID("file-reader", "read_file"),
					"title":     "Read file",
					"status":    operationPlanStepStatusCompleted,
					"skill_id":  "file-reader",
					"tool_name": "read_file",
				},
				map[string]interface{}{
					"id":       "skill:file-reader",
					"title":    "Use file-reader",
					"status":   operationPlanStepStatusPending,
					"skill_id": "file-reader",
					"role":     "primary",
				},
				map[string]interface{}{
					"id":     "observe",
					"title":  "Observe result",
					"status": operationPlanStepStatusPending,
				},
			},
			"step_status": map[string]interface{}{
				operationPlanToolStepID("file-reader", "read_file"): operationPlanStepStatusCompleted,
				"skill:file-reader": operationPlanStepStatusPending,
				"observe":           operationPlanStepStatusPending,
			},
			"status":              operationPlanStatusRunning,
			"pending_next_action": "Use file-reader",
		},
	}

	metadata = preparedResultMetadata(metadata, nil)

	plan := metadata["operation_plan"].(map[string]interface{})
	if plan["status"] != operationPlanStatusCompleted {
		t.Fatalf("plan status = %#v, want completed", plan["status"])
	}
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID("file-reader", "read_file")); got != operationPlanStepStatusCompleted {
		t.Fatalf("tool step status = %#v, want completed", got)
	}
	for _, id := range []string{"skill:file-reader", "observe"} {
		if got := operationPlanStepStatusForTest(plan, id); got != operationPlanStepStatusCompleted {
			t.Fatalf("step %s status = %#v, want completed; plan=%#v", id, got, plan)
		}
	}
	if operationPlanHasIncompleteWork(plan) {
		t.Fatalf("operationPlanHasIncompleteWork(%#v) = true, want false", plan)
	}
}

func TestPreparedResultMetadataKeepsPendingToolOperationPlanRunning(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{
				map[string]interface{}{
					"id":        operationPlanToolStepID(skills.SkillFileManager, "delete_file"),
					"title":     "Delete resolved file",
					"status":    operationPlanStepStatusPending,
					"skill_id":  skills.SkillFileManager,
					"tool_name": "delete_file",
				},
				map[string]interface{}{
					"id":     "observe",
					"title":  "Observe result",
					"status": operationPlanStepStatusPending,
				},
			},
			"step_status": map[string]interface{}{
				operationPlanToolStepID(skills.SkillFileManager, "delete_file"): operationPlanStepStatusPending,
				"observe": operationPlanStepStatusPending,
			},
			"status":              operationPlanStatusRunning,
			"pending_next_action": "Delete resolved file",
		},
	}

	metadata = preparedResultMetadata(metadata, nil)

	plan := metadata["operation_plan"].(map[string]interface{})
	if plan["status"] != operationPlanStatusRunning {
		t.Fatalf("plan status = %#v, want running", plan["status"])
	}
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillFileManager, "delete_file")); got != operationPlanStepStatusPending {
		t.Fatalf("delete step status = %#v, want pending", got)
	}
	if !operationPlanHasIncompleteWork(plan) {
		t.Fatalf("operationPlanHasIncompleteWork(%#v) = false, want true", plan)
	}
}

func TestPreparedResultMetadataKeepsWaitForContinueRunning(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{
				map[string]interface{}{
					"id":       "wait:continue",
					"title":    "Wait for user continue",
					"status":   operationPlanStepStatusPending,
					"wait_for": "continue",
				},
				map[string]interface{}{
					"id":     "observe",
					"title":  "Observe result",
					"status": operationPlanStepStatusPending,
				},
			},
			"step_status": map[string]interface{}{
				"wait:continue": operationPlanStepStatusPending,
				"observe":       operationPlanStepStatusPending,
			},
			"status":              operationPlanStatusRunning,
			"pending_next_action": "Wait for user continue",
		},
	}

	metadata = preparedResultMetadata(metadata, nil)

	plan := metadata["operation_plan"].(map[string]interface{})
	if plan["status"] != operationPlanStatusRunning {
		t.Fatalf("plan status = %#v, want running", plan["status"])
	}
	if got := operationPlanStepStatusForTest(plan, "wait:continue"); got != operationPlanStepStatusPending {
		t.Fatalf("wait step status = %#v, want pending", got)
	}
	if !operationPlanHasIncompleteWork(plan) {
		t.Fatalf("operationPlanHasIncompleteWork(%#v) = false, want true", plan)
	}
}

func TestOperationPlanArtifactStateDeduplicatesSavedManagedFile(t *testing.T) {
	parts := &chatRequestParts{
		Query:     "\u751f\u6210 SVG \u5e76\u4fdd\u5b58\u5230\u6587\u4ef6\u7ba1\u7406",
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillFileGenerator, skills.SkillFileManager},
	}
	metadata := streamingMessageMetadataWithTaskID(parts, "task-managed-artifact")

	metadata = mergeGeneratedArtifactMetadata(metadata, map[string]interface{}{
		"file_id":   "tool-file-1",
		"filename":  "saved.svg",
		"extension": ".svg",
		"target":    "temporary_artifact",
		"skill_id":  skills.SkillFileGenerator,
		"tool_name": "generate_file",
		"size":      128,
	})
	metadata = mergeGeneratedArtifactMetadata(metadata, map[string]interface{}{
		"file_id":             "managed-file-1",
		"upload_file_id":      "managed-file-1",
		"source_tool_file_id": "tool-file-1",
		"filename":            "saved.svg",
		"extension":           "svg",
		"target":              "managed_file",
		"skill_id":            skills.SkillFileManager,
		"tool_name":           "save_file_to_management",
		"size":                128,
	})

	plan := metadata["operation_plan"].(map[string]interface{})
	assetState := plan["asset_state"].(map[string]interface{})
	if assetState["generated_count"] != 1 || assetState["logical_asset_count"] != 1 {
		t.Fatalf("asset_state = %#v, want one logical asset after save", assetState)
	}
	if assetState["lifecycle_record_count"] != 2 {
		t.Fatalf("asset_state = %#v, want temporary and managed lifecycle records", assetState)
	}
	if assetState["temporary_count"] != 1 || assetState["managed_count"] != 1 {
		t.Fatalf("asset_state = %#v, want one temporary and one managed record", assetState)
	}
	stepStatus := plan["step_status"].(map[string]interface{})
	if stepStatus["skill:"+skills.SkillFileGenerator] != operationPlanStepStatusCompleted {
		t.Fatalf("step_status = %#v, want file-generator completed", stepStatus)
	}
	if stepStatus["skill:"+skills.SkillFileManager] != operationPlanStepStatusCompleted {
		t.Fatalf("step_status = %#v, want file-manager completed", stepStatus)
	}
}

func TestOperationPlanProducerSkillDoesNotBlockManagedSaveCompletion(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"intent": "save_generated_file_to_file_management",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management"),
					"title":     "Save generated file to File Management",
					"status":    operationPlanStepStatusPending,
					"skill_id":  skills.SkillFileManager,
					"tool_name": "save_file_to_management",
				},
				map[string]interface{}{
					"id":       "skill:" + skills.SkillFileManager,
					"title":    "Use file-manager",
					"status":   operationPlanStepStatusPending,
					"skill_id": skills.SkillFileManager,
					"role":     "primary",
				},
				map[string]interface{}{
					"id":       "skill:" + skills.SkillFileGenerator,
					"title":    "Use file-generator",
					"status":   operationPlanStepStatusPending,
					"skill_id": skills.SkillFileGenerator,
					"role":     "primary",
				},
				map[string]interface{}{
					"id":     "observe",
					"title":  "Observe result",
					"status": operationPlanStepStatusPending,
				},
			},
			"step_status": map[string]interface{}{
				operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management"): operationPlanStepStatusPending,
				"skill:" + skills.SkillFileManager:                                          operationPlanStepStatusPending,
				"skill:" + skills.SkillFileGenerator:                                        operationPlanStepStatusPending,
				"observe":                                                                   operationPlanStepStatusPending,
			},
			"status":              operationPlanStatusRunning,
			"pending_next_action": "Save generated file to File Management",
		},
	}

	metadata = mergeGeneratedArtifactMetadata(metadata, map[string]interface{}{
		"file_id":             "managed-file-1",
		"upload_file_id":      "managed-file-1",
		"source_tool_file_id": "tool-file-1",
		"filename":            "saved.svg",
		"extension":           "svg",
		"target":              "managed_file",
		"skill_id":            skills.SkillFileManager,
		"tool_name":           "save_file_to_management",
		"size":                128,
	})

	plan := metadata["operation_plan"].(map[string]interface{})
	if plan["status"] != operationPlanStatusCompleted {
		t.Fatalf("plan status = %#v, want completed", plan["status"])
	}
	if plan["pending_next_action"] != "none" {
		t.Fatalf("pending_next_action = %#v, want none", plan["pending_next_action"])
	}
	if operationPlanHasIncompleteWork(plan) {
		t.Fatalf("operationPlanHasIncompleteWork(%#v) = true, want false", plan)
	}
	stepStatus := plan["step_status"].(map[string]interface{})
	if stepStatus["skill:"+skills.SkillFileGenerator] != operationPlanStepStatusCompleted {
		t.Fatalf("producer skill status = %#v, want completed once the artifact is saved", stepStatus["skill:"+skills.SkillFileGenerator])
	}
}

func TestOperationPlanManagedCreateStaysRunningUntilAllGeneratedFilesAreSaved(t *testing.T) {
	parts := &chatRequestParts{
		Query:     "\u751f\u6210 txt \u548c svg \u5e76\u4fdd\u5b58\u5230\u6587\u4ef6\u7ba1\u7406",
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillFileGenerator, skills.SkillFileManager},
	}
	metadata := streamingMessageMetadataWithTaskID(parts, "task-managed-partial")

	metadata = mergeGeneratedArtifactMetadata(metadata, map[string]interface{}{
		"file_id":   "tool-file-txt",
		"filename":  "partial.txt",
		"extension": ".txt",
		"target":    "temporary_artifact",
		"skill_id":  skills.SkillFileGenerator,
		"tool_name": "generate_file",
	})
	metadata = mergeGeneratedArtifactMetadata(metadata, map[string]interface{}{
		"file_id":   "tool-file-svg",
		"filename":  "partial.svg",
		"extension": ".svg",
		"target":    "temporary_artifact",
		"skill_id":  skills.SkillFileGenerator,
		"tool_name": "generate_file",
	})
	metadata = mergeGeneratedArtifactMetadata(metadata, map[string]interface{}{
		"file_id":             "managed-file-txt",
		"upload_file_id":      "managed-file-txt",
		"source_tool_file_id": "tool-file-txt",
		"filename":            "partial.txt",
		"extension":           "txt",
		"target":              "managed_file",
		"skill_id":            skills.SkillFileManager,
		"tool_name":           "save_file_to_management",
	})

	plan := metadata["operation_plan"].(map[string]interface{})
	if plan["status"] != operationPlanStatusRunning {
		t.Fatalf("plan status = %#v, want running while svg is unsaved", plan["status"])
	}
	if plan["pending_next_action"] != "save_remaining_generated_files_to_file_management" {
		t.Fatalf("pending_next_action = %#v, want save remaining", plan["pending_next_action"])
	}
	assetState := plan["asset_state"].(map[string]interface{})
	if assetState["unsaved_count"] != 1 {
		t.Fatalf("asset_state = %#v, want one unsaved generated file", assetState)
	}
	stepStatus := plan["step_status"].(map[string]interface{})
	if stepStatus["skill:"+skills.SkillFileManager] != operationPlanStepStatusPending {
		t.Fatalf("step_status = %#v, want file-manager pending until all files are saved", stepStatus)
	}
	if stepStatus[operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management")] != operationPlanStepStatusPending {
		t.Fatalf("step_status = %#v, want save_file_to_management pending until all files are saved", stepStatus)
	}
	if got := operationPlanStepStatusForTest(plan, "skill:"+skills.SkillFileManager); got != operationPlanStepStatusPending {
		t.Fatalf("steps[file-manager].status = %#v, want pending", got)
	}
}

func TestOperationPlanIncludesPostRouteManagedCreateSupportingSkills(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "\u7ee7\u7eed\u6267\u884c\u7b2c\u4e8c\u9636\u6bb5\uff1a\u5230\u6587\u4ef6\u7ba1\u7406\u521b\u5efa\u5e76\u4fdd\u5b58\u4e24\u4e2a\u6587\u4ef6\uff0c\u6587\u4ef6\u540d\u5fc5\u987b\u5206\u522b\u662f smoke-continue.txt \u548c smoke-continue.svg\u3002",
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents",
		SkillMode:      skillModeAuto,
		SkillIDs: []string{
			skills.SkillConsoleNavigator,
			skills.SkillFileGenerator,
			skills.SkillFileManager,
		},
	}

	metadata := streamingMessageMetadataWithTaskID(parts, "task-managed-post-route")
	plan, ok := metadata["operation_plan"].(map[string]interface{})
	if !ok {
		t.Fatalf("operation_plan = %#v, want map", metadata["operation_plan"])
	}
	if plan["intent"] != "continue_previous_task" {
		t.Fatalf("intent = %#v, want continuation", plan["intent"])
	}
	stepStatus := plan["step_status"].(map[string]interface{})
	for _, want := range []string{
		"tool:" + skills.SkillConsoleNavigator + "/navigate",
		"route:/console/files",
		"skill:" + skills.SkillFileGenerator,
		"skill:" + skills.SkillFileManager,
		"observe",
	} {
		if stepStatus[want] != operationPlanStepStatusPending {
			t.Fatalf("step_status = %#v, want %s pending", stepStatus, want)
		}
	}
	generatorIndex := operationPlanStepIndexForTest(plan, "skill:"+skills.SkillFileGenerator)
	managerIndex := operationPlanStepIndexForTest(plan, "skill:"+skills.SkillFileManager)
	if generatorIndex < 0 || managerIndex < 0 || generatorIndex > managerIndex {
		t.Fatalf("step order generator=%d manager=%d in %#v, want generator before manager", generatorIndex, managerIndex, plan["steps"])
	}
	if role := operationPlanStepFieldForTest(plan, "skill:"+skills.SkillFileGenerator, "role"); role != "supporting" {
		t.Fatalf("file-generator role = %#v, want supporting", role)
	}
}

func TestOperationPlanIncludesManagedCreateToolSteps(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "go to File Management, generate an svg file and save it to file management",
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents",
		SkillMode:      skillModeAuto,
		SkillIDs: []string{
			skills.SkillConsoleNavigator,
			skills.SkillFileGenerator,
			skills.SkillFileManager,
		},
	}

	metadata := streamingMessageMetadataWithTaskID(parts, "task-managed-tools")
	plan := metadata["operation_plan"].(map[string]interface{})
	stepStatus := plan["step_status"].(map[string]interface{})
	generateStepID := operationPlanToolStepID(skills.SkillFileGenerator, "generate_file")
	saveStepID := operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management")
	for _, want := range []string{generateStepID, saveStepID} {
		if stepStatus[want] != operationPlanStepStatusPending {
			t.Fatalf("step_status = %#v, want %s pending", stepStatus, want)
		}
	}
	routeIndex := operationPlanStepIndexForTest(plan, "route:/console/files")
	generateIndex := operationPlanStepIndexForTest(plan, generateStepID)
	saveIndex := operationPlanStepIndexForTest(plan, saveStepID)
	if routeIndex < 0 || generateIndex < 0 || saveIndex < 0 || !(routeIndex < generateIndex && generateIndex < saveIndex) {
		t.Fatalf("step order route=%d generate=%d save=%d in %#v, want route before generate before save", routeIndex, generateIndex, saveIndex, plan["steps"])
	}
}

func TestOperationPlanIncludesDeleteToolForManagedCreateAndDeleteGoal(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "go to File Management, generate an svg file and save it to file management, then delete the third file",
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents",
		SkillMode:      skillModeAuto,
		SkillIDs: []string{
			skills.SkillConsoleNavigator,
			skills.SkillFileGenerator,
			skills.SkillFileManager,
		},
	}

	metadata := streamingMessageMetadataWithTaskID(parts, "task-managed-delete-tools")
	plan := metadata["operation_plan"].(map[string]interface{})
	stepStatus := plan["step_status"].(map[string]interface{})
	saveStepID := operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management")
	deleteStepID := operationPlanToolStepID(skills.SkillFileManager, "delete_file")
	for _, want := range []string{saveStepID, deleteStepID} {
		if stepStatus[want] != operationPlanStepStatusPending {
			t.Fatalf("step_status = %#v, want %s pending", stepStatus, want)
		}
	}
	saveIndex := operationPlanStepIndexForTest(plan, saveStepID)
	deleteIndex := operationPlanStepIndexForTest(plan, deleteStepID)
	if saveIndex < 0 || deleteIndex < 0 || saveIndex > deleteIndex {
		t.Fatalf("step order save=%d delete=%d in %#v, want save before delete", saveIndex, deleteIndex, plan["steps"])
	}
}

func TestOperationPlanIncludesToolStepsForChineseStagedManagedCreateAndDeleteGoal(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "\u5bfc\u822a\u5230\u6587\u4ef6\u7ba1\u7406\uff0c\u521b\u5efa\u5e76\u4fdd\u5b58\u4e24\u4e2a\u6587\u4ef6\u5230\u6587\u4ef6\u7ba1\u7406\uff0c\u6587\u4ef6\u540d\u5206\u522b\u662f SMOKE-COMPLEX.txt \u548c SMOKE-COMPLEX.svg\uff1b\u5237\u65b0/\u89c2\u5bdf\u786e\u8ba4\u5b83\u4eec\u53ef\u89c1\uff1b\u7136\u540e\u51bb\u7ed3\u6587\u4ef6\u5217\u8868\u5f53\u524d\u7b2c\u4e09\u4e2a\u6587\u4ef6\u4f5c\u4e3a\u5220\u9664\u76ee\u6807\uff0c\u53ea\u6709\u5f53\u7b2c\u4e09\u4e2a\u6587\u4ef6\u540d\u4ee5 SMOKE- \u5f00\u5934\u65f6\u624d\u8fdb\u5165\u5220\u9664\u5ba1\u6279\u5e76\u5220\u9664\uff0c\u5426\u5219\u505c\u6b62\u5e76\u8bf4\u660e\u539f\u56e0\u3002",
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents",
		SkillMode:      skillModeAuto,
		SkillIDs: []string{
			skills.SkillConsoleNavigator,
			skills.SkillFileGenerator,
			skills.SkillFileManager,
		},
	}

	metadata := streamingMessageMetadataWithTaskID(parts, "task-chinese-managed-create-delete")
	plan := metadata["operation_plan"].(map[string]interface{})
	stepStatus := plan["step_status"].(map[string]interface{})
	generateStepID := operationPlanToolStepID(skills.SkillFileGenerator, "generate_file")
	saveStepID := operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management")
	deleteStepID := operationPlanToolStepID(skills.SkillFileManager, "delete_file")
	if !isFileDeleteIntent(parts.Query) {
		t.Fatalf("isFileDeleteIntent(%q) = false, want true", parts.Query)
	}
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillFileManager, "delete_file") {
		t.Fatalf("strategy planned_tools = %#v, missing file-manager/delete_file", strategy.PlannedTools)
	}
	for _, want := range []string{generateStepID, saveStepID, deleteStepID} {
		if stepStatus[want] != operationPlanStepStatusPending {
			t.Fatalf("step_status = %#v, want %s pending", stepStatus, want)
		}
	}
	routeIndex := operationPlanStepIndexForTest(plan, "route:/console/files")
	generateIndex := operationPlanStepIndexForTest(plan, generateStepID)
	saveIndex := operationPlanStepIndexForTest(plan, saveStepID)
	deleteIndex := operationPlanStepIndexForTest(plan, deleteStepID)
	if routeIndex < 0 || generateIndex < 0 || saveIndex < 0 || deleteIndex < 0 ||
		!(routeIndex < generateIndex && generateIndex < saveIndex && saveIndex < deleteIndex) {
		t.Fatalf("step order route=%d generate=%d save=%d delete=%d in %#v, want route before generate before save before delete", routeIndex, generateIndex, saveIndex, deleteIndex, plan["steps"])
	}
}

func TestIsFileDeleteIntentAllowsConditionalChineseDeleteGoal(t *testing.T) {
	query := "\u51bb\u7ed3\u6587\u4ef6\u5217\u8868\u5f53\u524d\u7b2c\u4e09\u4e2a\u6587\u4ef6\u4f5c\u4e3a\u5220\u9664\u76ee\u6807\uff0c\u53ea\u6709\u5f53\u7b2c\u4e09\u4e2a\u6587\u4ef6\u540d\u4ee5 SMOKE- \u5f00\u5934\u65f6\u624d\u8fdb\u5165\u5220\u9664\u5ba1\u6279\u5e76\u5220\u9664\uff0c\u5426\u5219\u505c\u6b62\u5e76\u8bf4\u660e\u539f\u56e0\u3002"
	if !isFileDeleteIntent(query) {
		t.Fatalf("isFileDeleteIntent(%q) = false, want true", query)
	}
}

func TestFileMutationNegationDoesNotTreatFenbieAsBieNegation(t *testing.T) {
	query := "\u6587\u4ef6\u540d\u5206\u522b\u662f SMOKE-COMPLEX.txt \u548c SMOKE-COMPLEX.svg\uff0c\u7136\u540e\u5220\u9664\u7b2c\u4e09\u4e2a\u6587\u4ef6"
	if !isFileDeleteIntent(query) {
		t.Fatalf("isFileDeleteIntent(%q) = false, want true; \u5206\u522b should not be treated as \u522b negation", query)
	}
	negated := "\u522b\u5220\u9664\u7b2c\u4e09\u4e2a\u6587\u4ef6"
	if isFileDeleteIntent(negated) {
		t.Fatalf("isFileDeleteIntent(%q) = true, want false for explicit \u522b\u5220\u9664", negated)
	}
}

func TestNavigationStrategyIgnoresNegatedAssetMutationConstraint(t *testing.T) {
	query := "\u8bf7\u5bfc\u822a\u5230\u667a\u80fd\u4f53\u5217\u8868\u9875\u9762\uff0c\u7b49\u9875\u9762\u4e0a\u4e0b\u6587\u52a0\u8f7d\u5b8c\u6210\u540e\uff0c\u53ea\u6839\u636e\u65b0\u9875\u9762\u4e0a\u4e0b\u6587\u56de\u7b54\u5f53\u524d\u9875\u9762\u6807\u9898\u6216\u6a21\u5757\u540d\u79f0\u3002\u4e0d\u8981\u521b\u5efa\u3001\u7f16\u8f91\u6216\u5220\u9664\u4efb\u4f55\u8d44\u4ea7\u3002"
	if isFileDeleteIntent(query) {
		t.Fatalf("isFileDeleteIntent(%q) = true, want false for negated asset deletion constraint", query)
	}
	parts := &chatRequestParts{
		Query:          query,
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/files",
		SkillMode:      skillModeAuto,
		SkillIDs: []string{
			skills.SkillConsoleNavigator,
			skills.SkillFileManager,
			skills.SkillFileGenerator,
			skills.SkillAgentManagement,
		},
	}
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	if strategy.Intent != "navigate_console_page" {
		t.Fatalf("Intent = %q, want navigate_console_page", strategy.Intent)
	}
	if strategy.TargetPage != "/console/agents" || !strategy.RouteRequired {
		t.Fatalf("target/route = %q/%v, want /console/agents/true", strategy.TargetPage, strategy.RouteRequired)
	}
	if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillFileManager, "delete_file") {
		t.Fatalf("planned_tools = %#v, want no delete_file for pure navigation", strategy.PlannedTools)
	}

	metadata := streamingMessageMetadataWithTaskID(parts, "task-nav-negated-assets")
	plan := metadata["operation_plan"].(map[string]interface{})
	if plan["intent"] != "navigate_console_page" {
		t.Fatalf("operation plan intent = %#v, want navigate_console_page", plan["intent"])
	}
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillFileManager, "delete_file")); got != "" {
		t.Fatalf("delete step status = %q, want no delete_file step in %#v", got, plan["steps"])
	}
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillConsoleNavigator, "navigate")); got != operationPlanStepStatusPending {
		t.Fatalf("navigate step status = %q, want pending", got)
	}
}

func aiChatTurnStrategyHasPlannedToolForTest(strategy *AIChatTurnStrategy, skillID, toolName string) bool {
	if strategy == nil {
		return false
	}
	for _, tool := range strategy.PlannedTools {
		if tool.SkillID == skillID && tool.ToolName == toolName {
			return true
		}
	}
	return false
}

func aiChatTurnStrategyAvoidContainsForTest(strategy *AIChatTurnStrategy, fragment string) bool {
	if strategy == nil {
		return false
	}
	for _, item := range strategy.Avoid {
		if strings.Contains(item, fragment) {
			return true
		}
	}
	return false
}

func consoleAgentsVisibleTargetsTestParts(query string) *chatRequestParts {
	context := map[string]interface{}{
		"resources": []interface{}{
			map[string]interface{}{
				"resource_type": "page",
				"href":          "/console/agents",
			},
			map[string]interface{}{
				"resource_type": "agent",
				"id":            "agent-1",
				"title":         "Visible Agent One",
				"href":          "/console/agents/agent-1/agent",
				"visible_index": 1,
			},
			map[string]interface{}{
				"resource_type": "agent",
				"id":            "agent-2",
				"title":         "Visible Agent Two",
				"href":          "/console/agents/agent-2/agent",
				"visible_index": 2,
			},
		},
	}
	return &chatRequestParts{
		Query:               query,
		Surface:             aiChatSurfaceContextualSidebar,
		RuntimeContext:      "route=/console/agents",
		SkillMode:           skillModeAuto,
		SkillIDs:            []string{skills.SkillAgentManagement, skills.SkillConsoleNavigator},
		RawOperationContext: context,
		OperationContext:    context,
	}
}

func TestOperationPlanIncludesDeleteToolForDeleteOnlyGoal(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "delete the third file",
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/files",
		SkillMode:      skillModeAuto,
		SkillIDs:       []string{skills.SkillFileManager},
	}

	metadata := streamingMessageMetadataWithTaskID(parts, "task-delete-tool")
	plan := metadata["operation_plan"].(map[string]interface{})
	stepStatus := plan["step_status"].(map[string]interface{})
	deleteStepID := operationPlanToolStepID(skills.SkillFileManager, "delete_file")
	if stepStatus[deleteStepID] != operationPlanStepStatusPending {
		t.Fatalf("step_status = %#v, want delete_file pending", stepStatus)
	}
	if plan["intent"] != "delete_visible_file" {
		t.Fatalf("intent = %#v, want delete_visible_file", plan["intent"])
	}
}

func TestOperationPlanTaskIDFollowsReplacementMessageID(t *testing.T) {
	parts := &chatRequestParts{
		Query:     "\u6253\u5f00\u6587\u4ef6\u7ba1\u7406",
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillConsoleNavigator},
	}
	source := newStreamingMessage(mustUUIDForTest(t), nil, parts)
	source.Metadata = streamingMessageMetadataWithTaskID(parts, "source-task")

	replacement := replacementRootMessage(source, parts)
	plan := replacement.Metadata["operation_plan"].(map[string]interface{})
	if plan["task_id"] != source.ID.String() {
		t.Fatalf("replacement task_id = %#v, want %s", plan["task_id"], source.ID.String())
	}
}

func TestOperationPlanKeepsRepeatedRouteSteps(t *testing.T) {
	strategy := &AIChatTurnStrategy{
		Intent: "navigate_console_page",
		RemainingRouteSequence: []AIChatTurnStrategyRouteStep{
			{Href: "/console", Label: "Home", Status: "next"},
			{Href: "/console/files", Label: "Files", Status: "pending"},
			{Href: "/console/agents", Label: "Agents", Status: "pending"},
			{Href: "/console/db", Label: "Database", Status: "pending"},
			{Href: "/console/files", Label: "Files", Status: "pending"},
		},
	}
	plan := operationPlanFromTurnStrategy("task-routes", &chatRequestParts{
		Query:   "\u5148\u5230\u9996\u9875\uff0c\u518d\u5230\u6587\u4ef6\u3001\u667a\u80fd\u4f53\u3001\u6570\u636e\u5e93\uff0c\u6700\u540e\u56de\u5230\u6587\u4ef6",
		Surface: aiChatSurfaceContextualSidebar,
	}, strategy)
	if plan == nil {
		t.Fatal("operationPlanFromTurnStrategy() = nil, want plan")
	}
	wantPages := []string{"/console", "/console/files", "/console/agents", "/console/db", "/console/files"}
	if got := operationPlanRoutePagesForTest(plan); len(got) != len(wantPages) {
		t.Fatalf("route pages = %#v, want %#v", got, wantPages)
	} else {
		for idx := range wantPages {
			if got[idx] != wantPages[idx] {
				t.Fatalf("route pages = %#v, want %#v", got, wantPages)
			}
		}
	}
	stepStatus := plan["step_status"].(map[string]interface{})
	for _, id := range []string{"route:/console/files", "route:/console/files#2"} {
		if stepStatus[id] != operationPlanStepStatusPending {
			t.Fatalf("step_status[%s] = %#v, want pending in %#v", id, stepStatus[id], stepStatus)
		}
		if got := operationPlanStepStatusForTest(plan, id); got != operationPlanStepStatusPending {
			t.Fatalf("steps[%s].status = %#v, want pending", id, got)
		}
	}
	if plan["pending_next_action"] != "Home" {
		t.Fatalf("pending_next_action = %#v, want first route title", plan["pending_next_action"])
	}
}

func TestOperationPlanRouteToolCallWaitsForClientActionCompletion(t *testing.T) {
	plan := operationPlanFromTurnStrategy("task-route-tool", &chatRequestParts{
		Query:   "\u6253\u5f00\u6587\u4ef6\u7ba1\u7406",
		Surface: aiChatSurfaceContextualSidebar,
	}, &AIChatTurnStrategy{
		Intent: "navigate_console_page",
		RequiredNextTool: &AIChatTurnStrategyTool{
			SkillID:  skills.SkillConsoleNavigator,
			ToolName: "navigate",
			Arguments: map[string]string{
				"href": "/console/files",
			},
		},
		RemainingRouteSequence: []AIChatTurnStrategyRouteStep{{Href: "/console/files", Label: "Files", Status: "next"}},
	})
	metadata := map[string]interface{}{"operation_plan": plan}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{{
		"kind":       "tool_call",
		"runtime_id": "tool:route-files",
		"status":     "success",
		"skill_id":   skills.SkillConsoleNavigator,
		"tool_name":  "navigate",
		"arguments":  map[string]interface{}{"href": "/console/files"},
	}})

	updated := metadata["operation_plan"].(map[string]interface{})
	if got := operationPlanStepStatusForTest(updated, "tool:console-navigator/navigate"); got != operationPlanStepStatusCompleted {
		t.Fatalf("tool step status = %#v, want completed", got)
	}
	if got := operationPlanStepStatusForTest(updated, "route:/console/files"); got != operationPlanStepStatusPending {
		t.Fatalf("route step status = %#v, want pending until client action", got)
	}
}

func TestOperationPlanCompletesRepeatedRouteStepsSequentiallyFromClientActions(t *testing.T) {
	plan := operationPlanFromTurnStrategy("task-repeated-routes", &chatRequestParts{
		Query:   "\u8fde\u7eed\u5bfc\u822a\u5e76\u6700\u540e\u56de\u5230\u6587\u4ef6",
		Surface: aiChatSurfaceContextualSidebar,
	}, &AIChatTurnStrategy{
		Intent: "navigate_console_page",
		RemainingRouteSequence: []AIChatTurnStrategyRouteStep{
			{Href: "/console/files", Label: "Files", Status: "next"},
			{Href: "/console/agents", Label: "Agents", Status: "pending"},
			{Href: "/console/files", Label: "Files", Status: "pending"},
		},
	})
	metadata := map[string]interface{}{"operation_plan": plan}
	firstLoaded := map[string]interface{}{
		"kind":      "client_action",
		"action_id": "route-files-first",
		"status":    "succeeded",
		"skill_id":  skills.SkillConsoleNavigator,
		"tool_name": "navigate",
		"href":      "/console/files",
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{firstLoaded})
	updated := metadata["operation_plan"].(map[string]interface{})
	if got := operationPlanStepStatusForTest(updated, "route:/console/files"); got != operationPlanStepStatusCompleted {
		t.Fatalf("first files route status = %#v, want completed", got)
	}
	if got := operationPlanStepStatusForTest(updated, "route:/console/files#2"); got != operationPlanStepStatusPending {
		t.Fatalf("second files route status = %#v, want pending after first route_loaded", got)
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{firstLoaded})
	updated = metadata["operation_plan"].(map[string]interface{})
	if got := operationPlanStepStatusForTest(updated, "route:/console/files#2"); got != operationPlanStepStatusPending {
		t.Fatalf("second files route status = %#v, want pending after replaying same route_loaded", got)
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{
		firstLoaded,
		{
			"kind":      "client_action",
			"action_id": "route-files-final",
			"status":    "succeeded",
			"skill_id":  skills.SkillConsoleNavigator,
			"tool_name": "navigate",
			"href":      "/console/files",
		},
	})
	updated = metadata["operation_plan"].(map[string]interface{})
	if got := operationPlanStepStatusForTest(updated, "route:/console/files#2"); got != operationPlanStepStatusCompleted {
		t.Fatalf("second files route status = %#v, want completed after distinct final route_loaded", got)
	}
}

func TestOperationPlanStatusFromGuardrailFailure(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{map[string]interface{}{
				"id":       "skill:" + skills.SkillFileManager,
				"status":   operationPlanStepStatusPending,
				"skill_id": skills.SkillFileManager,
			}},
			"step_status": map[string]interface{}{"skill:" + skills.SkillFileManager: operationPlanStepStatusPending},
		},
	}
	applyOperationPlanInvocationState(metadata, []map[string]interface{}{{
		"kind":     "guardrail",
		"status":   "blocked",
		"skill_id": skills.SkillFileManager,
		"message":  "blocked duplicate deletion",
	}})
	plan := metadata["operation_plan"].(map[string]interface{})
	if plan["status"] != operationPlanStatusFailed {
		t.Fatalf("plan status = %#v, want failed", plan["status"])
	}
}

func TestOperationPlanPendingNextActionStopsAfterRejectedSaveBeforeDelete(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "go to File Management, generate an svg file and save it to file management, then delete the third file",
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/files",
		SkillMode:      skillModeAuto,
		SkillIDs:       []string{skills.SkillFileGenerator, skills.SkillFileManager},
	}
	metadata := streamingMessageMetadataWithTaskID(parts, "task-rejected-save-before-delete")

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{{
		"kind":       "tool_call",
		"runtime_id": "runtime_id:tool_call:file-manager:save_file_to_management::#1",
		"status":     "rejected",
		"skill_id":   skills.SkillFileManager,
		"tool_name":  "save_file_to_management",
	}})

	plan := metadata["operation_plan"].(map[string]interface{})
	if plan["status"] != operationPlanStatusFailed {
		t.Fatalf("plan status = %#v, want failed", plan["status"])
	}
	if plan["pending_next_action"] != "none" {
		t.Fatalf("pending_next_action = %#v, want none after rejected save", plan["pending_next_action"])
	}
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillFileManager, "delete_file")); got != operationPlanStepStatusPending {
		t.Fatalf("delete step status = %#v, want pending but not next action", got)
	}
}

func TestOperationPlanNavigationGuardrailDoesNotOverwriteCompletedRoutes(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "route:/console",
					"status":    operationPlanStepStatusPending,
					"skill_id":  skills.SkillConsoleNavigator,
					"tool_name": "navigate",
					"asset_target": map[string]interface{}{
						"page": "/console",
					},
				},
				map[string]interface{}{
					"id":        "route:/console/files",
					"status":    operationPlanStepStatusPending,
					"skill_id":  skills.SkillConsoleNavigator,
					"tool_name": "navigate",
					"asset_target": map[string]interface{}{
						"page": "/console/files",
					},
				},
			},
			"step_status": map[string]interface{}{
				"route:/console":       operationPlanStepStatusPending,
				"route:/console/files": operationPlanStepStatusPending,
			},
		},
	}
	applyOperationPlanInvocationState(metadata, []map[string]interface{}{
		{
			"kind":      "client_action",
			"action_id": "route-home",
			"status":    "success",
			"skill_id":  skills.SkillConsoleNavigator,
			"tool_name": "navigate",
			"arguments": map[string]interface{}{"href": "/console"},
		},
		{
			"kind":      "client_action",
			"action_id": "route-files",
			"status":    "success",
			"skill_id":  skills.SkillConsoleNavigator,
			"tool_name": "navigate",
			"arguments": map[string]interface{}{"href": "/console/files"},
		},
		{
			"kind":      "guardrail",
			"status":    "blocked",
			"skill_id":  skills.SkillConsoleNavigator,
			"tool_name": "navigate",
			"arguments": map[string]interface{}{"href": "/console/files"},
			"message":   "continue planning",
		},
	})

	plan := metadata["operation_plan"].(map[string]interface{})
	if plan["status"] != operationPlanStatusCompleted {
		t.Fatalf("plan status = %#v, want completed after route successes", plan["status"])
	}
	stepStatus := plan["step_status"].(map[string]interface{})
	for _, id := range []string{"route:/console", "route:/console/files"} {
		if stepStatus[id] != operationPlanStepStatusCompleted {
			t.Fatalf("step_status = %#v, want %s completed", stepStatus, id)
		}
		if got := operationPlanStepStatusForTest(plan, id); got != operationPlanStepStatusCompleted {
			t.Fatalf("steps[%s].status = %#v, want completed", id, got)
		}
	}
}

func TestOperationPlanContinuePlanningGuardrailDoesNotFailPendingSave(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{map[string]interface{}{
				"id":       "skill:" + skills.SkillFileManager,
				"status":   operationPlanStepStatusPending,
				"skill_id": skills.SkillFileManager,
			}},
			"step_status": map[string]interface{}{
				"skill:" + skills.SkillFileManager: operationPlanStepStatusPending,
			},
		},
	}
	applyOperationPlanInvocationState(metadata, []map[string]interface{}{{
		"kind":      "guardrail",
		"status":    "blocked",
		"skill_id":  skills.SkillFileManager,
		"tool_name": "save_file_to_management",
		"arguments": map[string]interface{}{
			"next_step":    "continue_planning",
			"blocked_tool": "file-generator/generate_file",
		},
	}})

	plan := metadata["operation_plan"].(map[string]interface{})
	if plan["status"] == operationPlanStatusFailed {
		t.Fatalf("plan status = %#v, want running for continue_planning guardrail", plan["status"])
	}
	stepStatus := plan["step_status"].(map[string]interface{})
	if stepStatus["skill:"+skills.SkillFileManager] != operationPlanStepStatusPending {
		t.Fatalf("step_status = %#v, want file-manager still pending", stepStatus)
	}
}

func TestOperationPlanUpdatesFromClientActionMetadata(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{map[string]interface{}{
				"id":        "tool:console-navigator/navigate",
				"status":    operationPlanStepStatusPending,
				"skill_id":  skills.SkillConsoleNavigator,
				"tool_name": "navigate",
			}, map[string]interface{}{
				"id":     "observe",
				"status": operationPlanStepStatusPending,
			}},
			"step_status": map[string]interface{}{
				"tool:console-navigator/navigate": operationPlanStepStatusPending,
				"observe":                         operationPlanStepStatusPending,
			},
		},
		"skill_invocations": []interface{}{map[string]interface{}{
			"kind":      "client_action",
			"action_id": "route-1",
			"status":    "waiting_client_action",
			"skill_id":  skills.SkillConsoleNavigator,
			"tool_name": "navigate",
		}},
	}
	metadata = mergeClientActionMetadata(metadata, map[string]interface{}{
		"kind":      "client_action",
		"action_id": "route-1",
		"status":    "succeeded",
		"skill_id":  skills.SkillConsoleNavigator,
		"tool_name": "navigate",
		"href":      "/console/files",
	})

	plan := metadata["operation_plan"].(map[string]interface{})
	stepStatus := plan["step_status"].(map[string]interface{})
	if stepStatus["tool:console-navigator/navigate"] != operationPlanStepStatusCompleted {
		t.Fatalf("step_status = %#v, want navigate completed", stepStatus)
	}
	if stepStatus["observe"] != operationPlanStepStatusCompleted {
		t.Fatalf("step_status = %#v, want observe completed", stepStatus)
	}
	for _, id := range []string{"tool:console-navigator/navigate", "observe"} {
		if got := operationPlanStepStatusForTest(plan, id); got != operationPlanStepStatusCompleted {
			t.Fatalf("steps[%s].status = %#v, want completed", id, got)
		}
	}
	if plan["status"] != operationPlanStatusCompleted {
		t.Fatalf("plan status = %#v, want completed", plan["status"])
	}
	if plan["current_page"] != "/console/files" {
		t.Fatalf("current_page = %#v, want /console/files", plan["current_page"])
	}
}

func TestOperationPlanClientActionPrefersObservedRoute(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{map[string]interface{}{
				"id":        "tool:console-navigator/navigate",
				"status":    operationPlanStepStatusPending,
				"skill_id":  skills.SkillConsoleNavigator,
				"tool_name": "navigate",
			}},
			"step_status": map[string]interface{}{
				"tool:console-navigator/navigate": operationPlanStepStatusPending,
			},
		},
		"skill_invocations": []interface{}{map[string]interface{}{
			"kind":      "client_action",
			"action_id": "route-redirected",
			"status":    "waiting_client_action",
			"skill_id":  skills.SkillConsoleNavigator,
			"tool_name": "navigate",
			"href":      "/console/agents",
		}},
	}
	metadata = mergeClientActionMetadata(metadata, map[string]interface{}{
		"kind":      "client_action",
		"action_id": "route-redirected",
		"status":    "succeeded",
		"skill_id":  skills.SkillConsoleNavigator,
		"tool_name": "navigate",
		"href":      "/console/agents",
		"result": map[string]interface{}{
			"observed_path": "/console/agents/agent-1/agent",
		},
	})

	plan := metadata["operation_plan"].(map[string]interface{})
	if plan["current_page"] != "/console/agents/agent-1/agent" {
		t.Fatalf("current_page = %#v, want observed route", plan["current_page"])
	}
}

func operationPlanStepStatusForTest(plan map[string]interface{}, id string) string {
	for _, step := range mapSliceFromAny(plan["steps"]) {
		if stringFromAny(step["id"]) == id {
			return stringFromAny(step["status"])
		}
	}
	return ""
}

func operationPlanStepFieldForTest(plan map[string]interface{}, id string, field string) string {
	for _, step := range mapSliceFromAny(plan["steps"]) {
		if stringFromAny(step["id"]) == id {
			return stringFromAny(step[field])
		}
	}
	return ""
}

func operationPlanStepAssetTargetForTest(plan map[string]interface{}, id string, field string) string {
	for _, step := range mapSliceFromAny(plan["steps"]) {
		if stringFromAny(step["id"]) != id {
			continue
		}
		target := mapFromOperationContext(step["asset_target"])
		return stringFromAny(target[field])
	}
	return ""
}

func operationPlanStepIndexForTest(plan map[string]interface{}, id string) int {
	for idx, step := range mapSliceFromAny(plan["steps"]) {
		if stringFromAny(step["id"]) == id {
			return idx
		}
	}
	return -1
}

func operationPlanRoutePagesForTest(plan map[string]interface{}) []string {
	steps := mapSliceFromAny(plan["steps"])
	pages := make([]string, 0, len(steps))
	for _, step := range steps {
		if !operationPlanStepIsRoute(step) {
			continue
		}
		pages = append(pages, operationPlanStepTargetPage(step))
	}
	return pages
}

func mustUUIDForTest(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if id == uuid.Nil {
		t.Fatal("uuid.New returned nil")
	}
	return id
}

func TestEnsureOperationPlanInvocationStepAppendsFailedToolAndStopsPending(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"version":             operationPlanVersion,
			"intent":              "answer_or_explain_zgi_context",
			"status":              operationPlanStatusCompleted,
			"pending_next_action": "none",
			"steps": []interface{}{
				map[string]interface{}{
					"id":       "skill:console-navigator",
					"title":    "Use console-navigator",
					"status":   operationPlanStepStatusPending,
					"skill_id": skills.SkillConsoleNavigator,
					"role":     "supporting",
				},
				map[string]interface{}{
					"id":     "observe",
					"title":  "Observe result",
					"status": operationPlanStepStatusCompleted,
				},
			},
			"step_status": map[string]interface{}{
				"skill:console-navigator": operationPlanStepStatusPending,
				"observe":                 operationPlanStepStatusCompleted,
			},
		},
	}

	ensureOperationPlanInvocationStep(metadata, map[string]interface{}{
		"kind":      "tool_call",
		"skill_id":  skills.SkillFileManager,
		"tool_name": "save_file_to_management",
		"status":    "error",
		"error":     "failed to load generated file metadata: tool file not found",
		"arguments": map[string]interface{}{
			"filename":     "nonexistent-postverify-check.md",
			"source_type":  "tool_file",
			"tool_file_id": "00000000-0000-0000-0000-000000000000",
		},
	})

	plan := mapFromOperationContext(metadata["operation_plan"])
	if got := stringFromAny(plan["status"]); got != operationPlanStatusFailed {
		t.Fatalf("operation_plan status = %q, want failed", got)
	}
	if got := stringFromAny(plan["pending_next_action"]); got != "none" {
		t.Fatalf("pending_next_action = %q, want none", got)
	}
	stepID := operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management")
	stepStatus := mapFromOperationContext(plan["step_status"])
	if got := stringFromAny(stepStatus[stepID]); got != operationPlanStepStatusFailed {
		t.Fatalf("step_status[%s] = %q, want failed", stepID, got)
	}
	if pending := operationPlanPendingExecutableSteps(plan, 4); len(pending) != 0 {
		t.Fatalf("pending executable steps = %#v, want none after failed tool", pending)
	}
}

func TestFinalAnswerGuardBatchAgentDeleteUsesItemResults(t *testing.T) {
	calls := []skillloop.SkillToolCallRef{{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "delete_agents",
		Arguments: map[string]interface{}{
			"agents": []interface{}{
				map[string]interface{}{"agent_id": "agent-ok", "agent_name": "ok"},
				map[string]interface{}{"agent_id": "agent-failed", "agent_name": "failed"},
			},
		},
		Result: map[string]interface{}{
			"operation_group": map[string]interface{}{
				"item_results": []interface{}{
					map[string]interface{}{"agent_id": "agent-ok", "status": "succeeded"},
					map[string]interface{}{"agent_id": "agent-failed", "status": "failed"},
				},
			},
		},
	}}

	if !finalAnswerGuardHasAgentDeleteCall(calls, "agent-ok") {
		t.Fatal("batch delete guard did not match succeeded target")
	}
	if finalAnswerGuardHasAgentDeleteCall(calls, "agent-failed") {
		t.Fatal("batch delete guard matched failed target")
	}

	legacyCalls := []skillloop.SkillToolCallRef{{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "delete_agents",
		Arguments: map[string]interface{}{
			"agents": []interface{}{
				map[string]interface{}{"agent_id": "agent-legacy", "agent_name": "legacy"},
			},
		},
	}}
	if !finalAnswerGuardHasAgentDeleteCall(legacyCalls, "agent-legacy") {
		t.Fatal("batch delete guard did not fall back to frozen arguments without item evidence")
	}
}
