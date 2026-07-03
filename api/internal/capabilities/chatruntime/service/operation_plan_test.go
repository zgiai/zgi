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
	if target := mapFromOperationContext(plan["target_resource"]); target["page"] != "/console/files" {
		t.Fatalf("target_resource = %#v, want files page target", target)
	}
	if plan["risk_level"] != "low" || plan["approval_required"] != false {
		t.Fatalf("risk/approval = %v/%v, want low/no approval; plan=%#v", plan["risk_level"], plan["approval_required"], plan)
	}
	if criteria := stringSliceFromAny(plan["success_criteria"]); len(criteria) == 0 {
		t.Fatalf("success_criteria = %#v, want non-empty criteria", plan["success_criteria"])
	}
	if got := len(mapSliceFromAny(plan["completed_steps"])); got != 0 {
		t.Fatalf("completed_steps len = %d, want 0; plan=%#v", got, plan)
	}
	if got := len(mapSliceFromAny(plan["failed_steps"])); got != 0 {
		t.Fatalf("failed_steps len = %d, want 0; plan=%#v", got, plan)
	}
}

func TestOperationPlanIncludesCurrentPageEvidence(t *testing.T) {
	parts := consoleAgentsVisibleTargetsTestParts("delete the first two visible agents on this page")

	metadata := streamingMessageMetadataWithTaskID(parts, "task-page-evidence")
	plan := metadata["operation_plan"].(map[string]interface{})
	pageEvidence := mapFromOperationContext(plan["page_evidence"])
	if got := stringFromAny(pageEvidence["current_page"]); got != "/console/agents" {
		t.Fatalf("page_evidence.current_page = %q, want /console/agents; evidence=%#v", got, pageEvidence)
	}
	currentPageEvidence := mapFromOperationContext(plan["current_page_evidence"])
	if !reflect.DeepEqual(currentPageEvidence, pageEvidence) {
		t.Fatalf("current_page_evidence = %#v, want page_evidence %#v", currentPageEvidence, pageEvidence)
	}
	resources := mapSliceFromAny(pageEvidence["resources"])
	if len(resources) < 3 {
		t.Fatalf("page_evidence.resources = %#v, want page plus visible agents", pageEvidence["resources"])
	}
	if got := stringFromAny(resources[1]["title"]); got != "Visible Agent One" {
		t.Fatalf("page_evidence.resources[1].title = %q, want Visible Agent One; resources=%#v", got, resources)
	}
}

func TestOperationPlanProgressFieldsTrackCompletedAndFailedSteps(t *testing.T) {
	createStepID := operationPlanToolStepID(skills.SkillAgentManagement, "create_agent")
	updateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{
				map[string]interface{}{
					"id":        createStepID,
					"title":     "Create agent",
					"status":    operationPlanStepStatusPending,
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "create_agent",
					"asset_target": map[string]interface{}{
						"effect":     "create",
						"asset_type": "agent",
					},
				},
				map[string]interface{}{
					"id":        updateStepID,
					"title":     "Update agent config",
					"status":    operationPlanStepStatusPending,
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_config",
					"asset_target": map[string]interface{}{
						"effect":     "update",
						"asset_type": "agent",
					},
				},
			},
			"step_status": map[string]interface{}{
				createStepID: operationPlanStepStatusPending,
				updateStepID: operationPlanStepStatusPending,
			},
		},
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{
		{
			"kind":       "tool_call",
			"runtime_id": "create-call",
			"status":     "success",
			"skill_id":   skills.SkillAgentManagement,
			"tool_name":  "create_agent",
			"result": map[string]interface{}{
				"agent_id":   "agent-created",
				"agent_name": "Created Agent",
			},
		},
		{
			"kind":       "tool_call",
			"runtime_id": "update-call",
			"status":     "error",
			"skill_id":   skills.SkillAgentManagement,
			"tool_name":  "update_agent_config",
			"error":      "model provider is required",
		},
	})

	plan := metadata["operation_plan"].(map[string]interface{})
	completed := mapSliceFromAny(plan["completed_steps"])
	if len(completed) != 1 {
		t.Fatalf("completed_steps = %#v, want one completed step; plan=%#v", plan["completed_steps"], plan)
	}
	if got := stringFromAny(completed[0]["id"]); got != createStepID {
		t.Fatalf("completed_steps[0].id = %q, want %q", got, createStepID)
	}
	failed := mapSliceFromAny(plan["failed_steps"])
	if len(failed) != 1 {
		t.Fatalf("failed_steps = %#v, want one failed step; plan=%#v", plan["failed_steps"], plan)
	}
	if got := stringFromAny(failed[0]["id"]); got != updateStepID {
		t.Fatalf("failed_steps[0].id = %q, want %q", got, updateStepID)
	}
	if got := stringFromAny(failed[0]["error"]); got != "model provider is required" {
		t.Fatalf("failed_steps[0].error = %q, want tool error", got)
	}
	if got := plan["status"]; got != operationPlanStatusFailed {
		t.Fatalf("plan status = %#v, want failed; plan=%#v", got, plan)
	}
}

func assertStringSliceContains(t *testing.T, values []string, want string) {
	t.Helper()
	for _, value := range values {
		if value == want {
			return
		}
	}
	t.Fatalf("values = %#v, want to contain %q", values, want)
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
	if got := plan["risk_level"]; got != "medium" {
		t.Fatalf("operation_plan risk_level = %#v, want medium; plan=%#v", got, plan)
	}
	if got := plan["approval_required"]; got != true {
		t.Fatalf("operation_plan approval_required = %#v, want true; plan=%#v", got, plan)
	}
	assertStringSliceContains(t, stringSliceFromAny(plan["approval_actions"]), operationPlanToolStepID(skills.SkillAgentManagement, "create_agent"))
	if criteria := stringSliceFromAny(plan["success_criteria"]); len(criteria) == 0 {
		t.Fatalf("operation_plan success_criteria = %#v, want strategy criteria", plan["success_criteria"])
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

func TestRestrictResolvedSkillsForTurnStrategyKeepsOnlyPlannedSkillsVisible(t *testing.T) {
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
	want := []string{skills.SkillFileManager}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered skill ids = %#v, want planned skill ids %#v", got, want)
	}
}

func TestTurnStrategyAllowedSkillIDsKeepsPlannedToolsAfterRouteHint(t *testing.T) {
	strategy := &AIChatTurnStrategy{
		Intent:        "manage_agent_asset",
		RouteRequired: true,
		RequiredNextTool: &AIChatTurnStrategyTool{
			SkillID:  skills.SkillConsoleNavigator,
			ToolName: "navigate",
			Arguments: map[string]string{
				"href": "/console/agents/agent-1/agent",
			},
		},
		PlannedTools: []AIChatTurnStrategyTool{
			{SkillID: skills.SkillAgentManagement, ToolName: "update_agent_config"},
			{SkillID: skills.SkillAgentManagement, ToolName: "get_agent_config"},
		},
	}

	allowed := turnStrategyAllowedSkillIDs(strategy)
	for _, want := range []string{skills.SkillConsoleNavigator, skills.SkillAgentManagement} {
		if _, ok := allowed[want]; !ok {
			t.Fatalf("allowed skills = %#v, missing %s", allowed, want)
		}
	}
}

func TestOperationPlanToolExposureIncludesPlannedToolsAfterRouteHint(t *testing.T) {
	plan := map[string]interface{}{
		"status": operationPlanStatusRunning,
		"steps": []interface{}{
			map[string]interface{}{
				"id":          operationPlanRouteStepID("/console/agents/agent-1/agent", 1),
				"status":      operationPlanStepStatusPending,
				"skill_id":    skills.SkillConsoleNavigator,
				"tool_name":   "navigate",
				"target_page": "/console/agents/agent-1/agent",
			},
			map[string]interface{}{
				"id":        operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"),
				"status":    operationPlanStepStatusPending,
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
			},
		},
		"step_status": map[string]interface{}{
			operationPlanRouteStepID("/console/agents/agent-1/agent", 1):                operationPlanStepStatusPending,
			operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"): operationPlanStepStatusPending,
		},
	}
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{"operation_plan": plan}},
		parts: &chatRequestParts{
			Query:     "\u6253\u5f00\u5f53\u524d\u667a\u80fd\u4f53\u8be6\u60c5\u5e76\u4fee\u6539\u914d\u7f6e",
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillConsoleNavigator, skills.SkillAgentManagement, skills.SkillFileManager},
		},
	}
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{
		{Metadata: skills.SkillMetadata{ID: skills.SkillConsoleNavigator}},
		{Metadata: skills.SkillMetadata{ID: skills.SkillAgentManagement}},
		{Metadata: skills.SkillMetadata{ID: skills.SkillFileManager}},
	}}

	filtered := restrictResolvedSkillsForPreparedTurn(prepared, resolved)
	if got, want := filtered.SkillIDs(), []string{skills.SkillConsoleNavigator, skills.SkillAgentManagement}; !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered skill ids = %#v, want route plus following planned skill %#v", got, want)
	}
	guard := skillLoopPlanToolCallGuardWithResolved(prepared, resolved)
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
	}); blocked {
		t.Fatal("plan guard blocked a planned Agent config tool that follows a route hint")
	}
	if pending := operationPlanPendingExecutableSteps(plan, 8); len(pending) != 1 ||
		stringFromAny(pending[0]["skill_id"]) != skills.SkillConsoleNavigator {
		t.Fatalf("operationPlanPendingExecutableSteps = %#v, want route-first progress view unchanged", pending)
	}
}

func TestAgentManagementUnsupportedMVPRequestPlansExplanationOnly(t *testing.T) {
	parts := &chatRequestParts{
		Query: strings.Join([]string{
			"Please publish the current Agent and create an API Key for it.",
			"If this contextual AIChat MVP does not support that, do not modify config and do not request approval.",
		}, " "),
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
	if strategy.Intent != "explain_agent_management_limit" {
		t.Fatalf("strategy.Intent = %q, want explain_agent_management_limit; strategy=%#v", strategy.Intent, strategy)
	}
	if len(strategy.PlannedTools) > 0 || strategy.RequiredNextTool != nil {
		t.Fatalf("strategy planned tools = %#v required=%#v, want explanation-only strategy", strategy.PlannedTools, strategy.RequiredNextTool)
	}
	for _, skillID := range append(append([]string{}, strategy.PrimarySkills...), strategy.SupportingSkills...) {
		if skillID == skills.SkillAgentManagement || skillID == skills.SkillConsoleNavigator {
			t.Fatalf("strategy exposed skill %q for unsupported request: %#v", skillID, strategy)
		}
	}
	criteria := strings.Join(strategy.SuccessCriteria, "\n")
	for _, want := range []string{"cannot publish Agents", "manage API keys", "do not request governance approval"} {
		if !strings.Contains(criteria, want) {
			t.Fatalf("success criteria missing %q in:\n%s", want, criteria)
		}
	}

	metadata := streamingMessageMetadataWithTaskID(parts, "task-agent-unsupported")
	if plan := mapFromOperationContext(metadata["operation_plan"]); len(plan) > 0 {
		for _, unexpected := range []string{
			"create_agent",
			"get_agent_config",
			"list_available_models",
			"list_agent_skill_candidates",
			"list_agent_knowledge_candidates",
			"list_agent_database_candidates",
			"list_agent_database_tables",
			"list_agent_workflow_binding_candidates",
			"update_agent_config",
		} {
			if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, unexpected)); got != "" {
				t.Fatalf("%s step status = %#v, want absent; plan=%#v", unexpected, got, plan)
			}
		}
	}

	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{
		{Metadata: skills.SkillMetadata{ID: skills.SkillAgentManagement}},
		{Metadata: skills.SkillMetadata{ID: skills.SkillConsoleNavigator}},
	}}
	filtered := restrictResolvedSkillsForTurnStrategy(parts, resolved)
	wantSkills := []string{skills.SkillAgentManagement, skills.SkillConsoleNavigator}
	if got := filtered.SkillIDs(); !reflect.DeepEqual(got, wantSkills) {
		t.Fatalf("filtered skill ids = %#v, want skills unchanged because unsupported requests are handled by strategy guidance %#v", got, wantSkills)
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
	want := []string{skills.SkillAgentManagement}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered skill ids = %#v, want only Agent management for current Agent config edit %#v", got, want)
	}

	metadata := streamingMessageMetadataWithTaskID(parts, "task-agent-config")
	plan := metadata["operation_plan"].(map[string]interface{})
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillChartGenerator, "generate_chart")); got != "" {
		t.Fatalf("chart-generator step status = %#v, want absent; plan=%#v", got, plan)
	}
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "list_agent_skill_candidates")); got != operationPlanStepStatusPending {
		t.Fatalf("list_agent_skill_candidates step status = %#v, want pending; plan=%#v", got, plan)
	}
	candidateStep := operationPlanStepForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "list_agent_skill_candidates"))
	candidateArgs := mapFromOperationContext(candidateStep["arguments"])
	if got := stringFromAny(candidateArgs["query"]); got != "\u56fe\u8868\u751f\u6210\u5668" {
		t.Fatalf("list_agent_skill_candidates query = %#v, want 图表生成器; step=%#v", got, candidateStep)
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

func TestAgentBindingCandidateSelectionPlansMutationAfterCandidateReads(t *testing.T) {
	query := strings.Join([]string{
		"\u8bf7\u4e3a\u5f53\u524d\u6d4b\u8bd5\u667a\u80fd\u4f53 GOAL-CONFIG-SIDEBAR-1782988265670-EDITED \u505a\u4e00\u6b21\u8d44\u6e90\u7ed1\u5b9a\u5192\u70df\uff1a",
		"\u5148\u67e5\u8be2\u5f53\u524d\u5de5\u4f5c\u7a7a\u95f4\u5185\u53ef\u7ed1\u5b9a\u7684 Skill\u3001\u77e5\u8bc6\u5e93\u3001\u6570\u636e\u5e93\u8868\u3001\u5de5\u4f5c\u6d41\u5019\u9009\uff0c\u4e0d\u8981\u731c ID\uff1b",
		"\u5982\u679c\u67d0\u7c7b\u5b58\u5728\u5019\u9009\uff0c\u5c31\u5404\u9009\u62e9 1 \u4e2a\u8fdb\u884c\u7ed1\u5b9a\uff0c\u5982\u679c\u67d0\u7c7b\u6ca1\u6709\u5019\u9009\u5c31\u5982\u5b9e\u8bf4\u660e\u5e76\u8df3\u8fc7\u3002",
		"\u9700\u8981\u5ba1\u6279\u65f6\u8bf7\u4e00\u6b21\u6027\u53d1\u8d77\u5ba1\u6279\uff1b\u5ba1\u6279\u901a\u8fc7\u540e\u91cd\u65b0\u8bfb\u53d6\u914d\u7f6e\uff0c\u57fa\u4e8e\u5de5\u5177\u8fd4\u56de\u503c\u786e\u8ba4\u5b9e\u9645\u7ed1\u5b9a\u7ed3\u679c\u3002",
	}, "")
	if !agentBindingMutationRequested(query) {
		t.Fatalf("agentBindingMutationRequested(%q) = false, want true", query)
	}
	requiredTools := requiredAgentBindingMutationTools(query)
	for _, want := range []string{"replace_agent_knowledge_bindings", "replace_agent_database_bindings", "replace_agent_workflow_bindings"} {
		if !stringSliceContainsFold(requiredTools, want) {
			t.Fatalf("requiredAgentBindingMutationTools(%q) = %#v, missing %s", query, requiredTools, want)
		}
	}
	if !agentManagementSkillBindingRequested(query) {
		t.Fatalf("agentManagementSkillBindingRequested(%q) = false, want true", query)
	}
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
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	for _, want := range []string{
		"list_agent_skill_candidates",
		"list_agent_knowledge_candidates",
		"list_agent_database_candidates",
		"list_agent_database_tables",
		"list_agent_workflow_binding_candidates",
		"update_agent_config",
		"get_agent_config",
	} {
		if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, want) {
			t.Fatalf("PlannedTools = %#v, missing agent-management/%s", strategy.PlannedTools, want)
		}
	}
	updateArgs := aiChatTurnStrategyPlannedToolArgumentsForTest(strategy, skills.SkillAgentManagement, "update_agent_config")
	if actions := stringFromAny(updateArgs[operationPlanExpectedBindingActionsKey]); actions == "" {
		t.Fatalf("update_agent_config args = %#v, want expected binding actions", updateArgs)
	}

	metadata := streamingMessageMetadataWithTaskID(parts, "task-agent-bind-after-candidates")
	plan := metadata["operation_plan"].(map[string]interface{})
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")); got != operationPlanStepStatusPending {
		t.Fatalf("update_agent_config step status = %#v, want pending; plan=%#v", got, plan)
	}
	if got := operationPlanStepStatusForTest(plan, operationPlanPostUpdateAgentConfigReadStepID()); got != operationPlanStepStatusPending {
		t.Fatalf("post-update get_agent_config step status = %#v, want pending; plan=%#v", got, plan)
	}
	if approvals := stringSliceFromAny(plan["approval_actions"]); !stringSliceContainsFold(approvals, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")) {
		t.Fatalf("approval_actions = %#v, want update_agent_config approval", approvals)
	}
}

func TestSkillLoopPlanToolGuardBlocksOverbroadAgentCandidateSelectionUpdate(t *testing.T) {
	query := "Bind resources to this Agent: list bindable Skill, knowledge base, database table, and workflow candidates first; if a candidate exists, choose one candidate for each resource type and bind it."
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     query,
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillAgentManagement},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status":             operationPlanStatusRunning,
				"original_user_goal": query,
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
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopPlanToolCallGuard() = nil, want guard")
	}
	result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Arguments: map[string]interface{}{
			"add_enabled_skill_ids":     []interface{}{"calculator", "\u5185\u5bb9\u603b\u7ed3"},
			"add_knowledge_dataset_ids": []interface{}{"kb-1", "kb-2"},
			"add_database_bindings":     `[{"data_source_id":"db-1","table_ids":["table-1"]}]`,
			"add_workflow_bindings":     []interface{}{map[string]interface{}{"agent_id": "agent-1", "workflow_id": "workflow-1"}},
		},
	})
	if !blocked {
		t.Fatal("overbroad update_agent_config was allowed, want candidate selection guard to block before governance")
	}
	if !strings.Contains(result.SystemMessage, "at most one") {
		t.Fatalf("guard system message = %q, want one-candidate guidance", result.SystemMessage)
	}
	if !strings.Contains(result.SystemMessage, "display name") {
		t.Fatalf("guard system message = %q, want skill ID/display-name guidance", result.SystemMessage)
	}
}

func TestSkillLoopPlanToolGuardBlocksUnresolvedAgentSkillBindingBeforeApproval(t *testing.T) {
	query := "Try to bind a Skill named 不存在的冒烟技能-XYZ to the current Agent. If it cannot be found, do not request approval."
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     query,
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillAgentManagement},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status":             operationPlanStatusRunning,
				"original_user_goal": query,
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
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopPlanToolCallGuard() = nil, want guard")
	}
	result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Arguments: map[string]interface{}{
			"agent_id":              "agent-1",
			"add_enabled_skill_ids": []interface{}{"不存在的冒烟技能-XYZ"},
			"display_names": map[string]interface{}{
				"skills": map[string]interface{}{"不存在的冒烟技能-XYZ": "不存在的冒烟技能 Xyz"},
			},
		},
	})
	if !blocked {
		t.Fatal("unresolved display-name Skill binding was allowed, want guard to block before governance approval")
	}
	if !strings.Contains(result.SystemMessage, "no matching Skill was found") {
		t.Fatalf("guard system message = %q, want missing Skill guidance", result.SystemMessage)
	}
	if !strings.Contains(result.SystemMessage, "candidate.skill_id") {
		t.Fatalf("guard system message = %q, want candidate skill_id guidance", result.SystemMessage)
	}
}

func TestSkillLoopPlanToolGuardAllowsOneAgentCandidateSelectionUpdatePerType(t *testing.T) {
	query := "Bind resources to this Agent: list bindable Skill, knowledge base, database table, and workflow candidates first; if a candidate exists, choose one candidate for each resource type and bind it."
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     query,
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillAgentManagement},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status":             operationPlanStatusRunning,
				"original_user_goal": query,
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
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopPlanToolCallGuard() = nil, want guard")
	}
	if result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Arguments: map[string]interface{}{
			"add_enabled_skill_ids":     []interface{}{"calculator"},
			"add_knowledge_dataset_ids": []interface{}{"kb-1"},
			"add_database_bindings": []interface{}{map[string]interface{}{
				"data_source_id": "db-1",
				"table_ids":      []interface{}{"table-1"},
			}},
			"add_workflow_bindings": []interface{}{map[string]interface{}{"agent_id": "agent-1", "workflow_id": "workflow-1"}},
		},
	}); blocked {
		t.Fatalf("one-per-type update_agent_config was blocked: %#v", result)
	}
}

func TestSkillLoopPlanToolGuardBlocksRepeatedCompletedReadBeforePendingAgentBindingStep(t *testing.T) {
	query := strings.Join([]string{
		"\u8bf7\u4e3a\u5f53\u524d\u6d4b\u8bd5\u667a\u80fd\u4f53 GOAL-CONFIG-SIDEBAR-1782988265670-EDITED \u505a\u4e00\u6b21\u8d44\u6e90\u7ed1\u5b9a\u5192\u70df\uff1a",
		"\u5148\u67e5\u8be2\u5f53\u524d\u5de5\u4f5c\u7a7a\u95f4\u5185\u53ef\u7ed1\u5b9a\u7684 Skill\u3001\u77e5\u8bc6\u5e93\u3001\u6570\u636e\u5e93\u8868\u3001\u5de5\u4f5c\u6d41\u5019\u9009\uff0c\u4e0d\u8981\u731c ID\uff1b",
		"\u5982\u679c\u67d0\u7c7b\u5b58\u5728\u5019\u9009\uff0c\u5c31\u5404\u9009\u62e9 1 \u4e2a\u8fdb\u884c\u7ed1\u5b9a\u3002",
	}, "")
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
	metadata := streamingMessageMetadataWithTaskID(parts, "task-agent-bind-repeat-read")
	plan := metadata["operation_plan"].(map[string]interface{})
	steps := mapSliceFromAny(plan["steps"])
	stepStatus := mapFromOperationContext(plan["step_status"])
	for _, step := range steps {
		if !strings.EqualFold(stringFromAny(step["skill_id"]), skills.SkillAgentManagement) {
			continue
		}
		toolName := strings.TrimSpace(stringFromAny(step["tool_name"]))
		switch toolName {
		case "list_agent_skill_candidates",
			"list_agent_knowledge_candidates",
			"list_agent_database_candidates",
			"list_agent_database_tables",
			"list_agent_workflow_binding_candidates":
			step["status"] = operationPlanStepStatusCompleted
			stepStatus[stringFromAny(step["id"])] = operationPlanStepStatusCompleted
		}
	}
	applyOperationPlanProgress(plan, steps, stepStatus, "", "")
	prepared := &PreparedChat{
		parts:   parts,
		Message: &runtimemodel.Message{Metadata: metadata},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopPlanToolCallGuard() = nil, want guard")
	}
	result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "list_agent_database_tables",
	})
	if !blocked {
		t.Fatal("repeated list_agent_database_tables was allowed while get_agent_config/update_agent_config remained pending")
	}
	if result.SkillID != skills.SkillAgentManagement || result.ToolName != "get_agent_config" {
		t.Fatalf("guard result = %#v, want next pending get_agent_config", result)
	}
	if !result.Advisory {
		t.Fatalf("guard result Advisory = false, want true for repeated completed read/list step guidance")
	}
	if !strings.Contains(result.SystemMessage, "Do not repeat the completed read") {
		t.Fatalf("SystemMessage = %q, want repeated-read guidance", result.SystemMessage)
	}
}

func TestAgentReadOnlyBindingCandidatesDoesNotPlanConfigMutation(t *testing.T) {
	query := "\u56de\u5f52\u9a8c\u8bc1-\u53ea\u8bfb\u7ed1\u5b9a\u5019\u9009-1782900585971\uff1a\u8bf7\u53ea\u8bfb\u53d6\u5f53\u524d\u667a\u80fd\u4f53\u53ef\u7ed1\u5b9a\u77e5\u8bc6\u5e93\u3001\u6570\u636e\u5e93\u8868\u3001\u5de5\u4f5c\u6d41\u5019\u9009\uff1b\u4e0d\u8981\u4fee\u6539\u540d\u79f0\u3001\u63cf\u8ff0\u3001\u56fe\u6807\u3001\u6a21\u578b\u3001\u7cfb\u7edf\u63d0\u793a\u8bcd\u3001\u5f00\u573a\u95ee\u9898\uff0c\u4e5f\u4e0d\u8981\u7ed1\u5b9a\u6216\u89e3\u7ed1\u4efb\u4f55\u8d44\u6e90\u3002\u6700\u540e\u53ea\u6839\u636e\u5de5\u5177\u8fd4\u56de\u503c\u544a\u8bc9\u6211\u5019\u9009\u6570\u91cf\uff0c\u4e0d\u8981\u53d1\u8d77\u5ba1\u6279\u3002"
	if agentBindingMutationRequested(query) {
		t.Fatalf("agentBindingMutationRequested(%q) = true, want false for explicit read-only no-bind query", query)
	}
	if tools := requiredAgentBindingMutationTools(query); len(tools) != 0 {
		t.Fatalf("requiredAgentBindingMutationTools(%q) = %#v, want none", query, tools)
	}
	if agentManagementConfigUpdateRequested(query) {
		t.Fatalf("agentManagementConfigUpdateRequested(%q) = true, want false", query)
	}
	if !agentManagementReadOnlyGoalRejectsMutation(query) {
		t.Fatalf("agentManagementReadOnlyGoalRejectsMutation(%q) = false, want true", query)
	}
	parts := contextualConsoleAgentsManageCapabilityPartsForTest()
	parts.Query = query
	parts.RuntimeContext = "route=/console/agents/agent-1/agent"
	parts.SkillMode = skillModeAuto
	parts.SkillIDs = []string{
		skills.SkillAgentManagement,
		skills.SkillConsoleNavigator,
	}

	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	for _, want := range []string{"list_agent_knowledge_candidates", "list_agent_database_candidates", "list_agent_database_tables", "list_agent_workflow_binding_candidates"} {
		if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, want) {
			t.Fatalf("PlannedTools = %#v, missing read-only candidate tool agent-management/%s", strategy.PlannedTools, want)
		}
		args := aiChatTurnStrategyPlannedToolArgumentsForTest(strategy, skills.SkillAgentManagement, want)
		if args["agent_id"] != "agent-1" {
			t.Fatalf("PlannedTools = %#v, %s args agent_id = %q, want agent-1", strategy.PlannedTools, want, args["agent_id"])
		}
	}
	for _, unexpected := range []string{"get_agent_config", "update_agent_config", "update_agent_identity", "list_available_models", "replace_agent_knowledge_bindings", "replace_agent_database_bindings", "replace_agent_workflow_bindings"} {
		if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, unexpected) {
			t.Fatalf("PlannedTools = %#v, want no mutation/config tool agent-management/%s", strategy.PlannedTools, unexpected)
		}
	}

	plan := operationPlanFromTurnStrategy("task-agent-read-only-candidates", parts, strategy)
	if steps := mapSliceFromAny(plan["steps"]); len(steps) > 0 {
		for _, step := range steps {
			if stringFromAny(step["skill_id"]) != skills.SkillAgentManagement {
				continue
			}
			toolName := stringFromAny(step["tool_name"])
			if toolName == "" || toolName == "list_agent_database_tables" {
				continue
			}
			args := mapFromOperationContext(step["arguments"])
			if args["agent_id"] != "agent-1" {
				t.Fatalf("operation_plan step %s arguments = %#v, want agent-1; plan=%#v", toolName, args, plan)
			}
		}
	}
	for _, unexpected := range []string{"get_agent_config", "update_agent_config", "update_agent_identity", "list_available_models"} {
		if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, unexpected)); got != "" {
			t.Fatalf("%s step status = %#v, want absent; plan=%#v", unexpected, got, plan)
		}
	}
	if got := plan["approval_required"]; got != false {
		t.Fatalf("approval_required = %#v, want false; plan=%#v", got, plan)
	}
	if actions := stringSliceFromAny(plan["approval_actions"]); len(actions) != 0 {
		t.Fatalf("approval_actions = %#v, want none; plan=%#v", actions, plan)
	}

	metadata := map[string]interface{}{"operation_plan": plan}
	applyOperationPlanInvocationState(metadata, []map[string]interface{}{
		agentManagementToolInvocationForTest("list_agent_knowledge_candidates", "tool_call:agent-management:list_agent_knowledge_candidates::#1", map[string]interface{}{
			"status": "success",
			"items":  []interface{}{map[string]interface{}{"id": "kb-1", "name": "Support KB"}},
		}),
		agentManagementToolInvocationForTest("list_agent_database_candidates", "tool_call:agent-management:list_agent_database_candidates::#1", map[string]interface{}{
			"status": "success",
			"items":  []interface{}{map[string]interface{}{"id": "db-1", "name": "Support DB"}},
		}),
		agentManagementToolInvocationForTest("list_agent_database_tables", "tool_call:agent-management:list_agent_database_tables::#1", map[string]interface{}{
			"status": "success",
			"items":  []interface{}{map[string]interface{}{"id": "table-1", "name": "customers"}},
		}),
		agentManagementToolInvocationForTest("list_agent_workflow_binding_candidates", "tool_call:agent-management:list_agent_workflow_binding_candidates::#1", map[string]interface{}{
			"status": "success",
			"items":  []interface{}{map[string]interface{}{"id": "workflow-1", "name": "Support Flow"}},
		}),
	})
	metadata = preparedResultMetadata(metadata, nil)
	plan = metadata["operation_plan"].(map[string]interface{})
	if got := stringFromAny(plan["status"]); got != operationPlanStatusCompleted {
		t.Fatalf("operation_plan status = %q, want completed after read-only candidate evidence; plan=%#v", got, plan)
	}
	if got := stringFromAny(plan["pending_next_action"]); got != "none" {
		t.Fatalf("pending_next_action = %q, want none; plan=%#v", got, plan)
	}
}

func TestAgentReadOnlyConfigAndBindableResourceSweepDoesNotPlanOrAllowMutation(t *testing.T) {
	query := "\u8bf7\u53ea\u8bfb\u68c0\u67e5\u5f53\u524d\u9875\u9762\u7b2c\u4e00\u4e2a\u667a\u80fd\u4f53\u7684\u914d\u7f6e\uff1a\u8bfb\u53d6\u5b83\u7684\u57fa\u7840\u4fe1\u606f\u3001\u8fd0\u884c\u914d\u7f6e\u3001\u53ef\u7f16\u8f91\u9879\u76ee\uff0c\u4ee5\u53ca\u5f53\u524d\u5de5\u4f5c\u7a7a\u95f4\u53ef\u7ed1\u5b9a\u8d44\u6e90\u7684\u5927\u81f4\u6570\u91cf\u3002\u4e0d\u8981\u4fee\u6539\u3001\u7ed1\u5b9a\u3001\u89e3\u7ed1\u3001\u521b\u5efa\u6216\u5220\u9664\u4efb\u4f55\u8d44\u4ea7\u3002\u6700\u540e\u8bf7\u660e\u786e\u8bf4\u660e\u6ca1\u6709\u6267\u884c\u4efb\u4f55\u53d8\u66f4\u64cd\u4f5c\u3002"
	if agentManagementCreateRequested(query) {
		t.Fatalf("agentManagementCreateRequested(%q) = true, want false for list-style negated create", query)
	}
	if agentManagementDeleteRequested(query) {
		t.Fatalf("agentManagementDeleteRequested(%q) = true, want false for list-style negated delete", query)
	}
	if agentManagementConfigUpdateRequested(query) {
		t.Fatalf("agentManagementConfigUpdateRequested(%q) = true, want false for read-only editable item request", query)
	}
	if tools := requiredAgentBindingMutationTools(query); len(tools) != 0 {
		t.Fatalf("requiredAgentBindingMutationTools(%q) = %#v, want none", query, tools)
	}
	if !agentManagementExplicitNoMutationRequested(query) {
		t.Fatalf("agentManagementExplicitNoMutationRequested(%q) = false, want true", query)
	}
	if !agentManagementReadOnlyGoalRejectsMutation(query) {
		t.Fatalf("agentManagementReadOnlyGoalRejectsMutation(%q) = false, want true", query)
	}

	parts := contextualConsoleAgentsManageCapabilityPartsForTest()
	parts.Query = query
	parts.RuntimeContext = "route=/console/agents"
	parts.SkillMode = skillModeAuto
	parts.SkillIDs = []string{skills.SkillAgentManagement, skills.SkillConsoleNavigator}
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	for _, want := range []string{"get_agent_config", "list_agent_skill_candidates", "list_agent_knowledge_candidates", "list_agent_database_candidates", "list_agent_database_tables", "list_agent_workflow_binding_candidates"} {
		if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, want) {
			t.Fatalf("PlannedTools = %#v, missing read-only tool agent-management/%s", strategy.PlannedTools, want)
		}
	}
	for _, unexpected := range []string{"create_agent", "delete_agent", "delete_agents", "update_agent_identity", "update_agent_config", "replace_agent_skill_bindings", "replace_agent_knowledge_bindings", "replace_agent_database_bindings", "replace_agent_workflow_bindings"} {
		if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, unexpected) {
			t.Fatalf("PlannedTools = %#v, want no mutation tool agent-management/%s", strategy.PlannedTools, unexpected)
		}
	}

	prepared := &PreparedChat{
		parts: parts,
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": operationPlanFromTurnStrategy("task-agent-readonly-sweep", parts, strategy),
		}},
	}
	guard := skillLoopPlanToolCallGuard(prepared)
	for _, toolName := range []string{"create_agent", "delete_agents", "update_agent_config"} {
		result, blocked := guard(skillloop.ToolCallGuardRequest{
			SkillID:  skills.SkillAgentManagement,
			ToolName: toolName,
		})
		if !blocked {
			t.Fatalf("%s was allowed for read-only Agent sweep, want blocked", toolName)
		}
		if !strings.Contains(result.SystemMessage, "read-only Agent request") {
			t.Fatalf("%s guard SystemMessage = %q, want read-only guidance", toolName, result.SystemMessage)
		}
	}
}

func TestAgentReadOnlyBindableCandidatesWithSkillDoesNotPlanConfigMutation(t *testing.T) {
	query := "\u5192\u70df\u9a8c\u8bc18f\uff1a\u53ea\u8bfb\u67e5\u770b\u667a\u80fd\u4f53\u300cGOAL-MATRIX-178302-1783027245043-EDITED\u300d\u5f53\u524d\u53ef\u7ed1\u5b9a\u7684 Skill\u3001\u77e5\u8bc6\u5e93\u3001\u6570\u636e\u5e93\u8868\u3001\u5de5\u4f5c\u6d41\u5019\u9009\u5404\u6709\u54ea\u4e9b\u3002\u4e0d\u8981\u4fee\u6539\u914d\u7f6e\uff0c\u4e0d\u8981\u53d1\u8d77\u5ba1\u6279\u3002"
	if agentBindingMutationRequested(query) {
		t.Fatalf("agentBindingMutationRequested(%q) = true, want false for read-only bindable candidate query", query)
	}
	if agentManagementSkillBindingRequested(query) {
		t.Fatalf("agentManagementSkillBindingRequested(%q) = true, want false for Skill candidate query", query)
	}
	if tools := requiredAgentBindingMutationTools(query); len(tools) != 0 {
		t.Fatalf("requiredAgentBindingMutationTools(%q) = %#v, want none", query, tools)
	}
	if agentManagementConfigUpdateRequested(query) {
		t.Fatalf("agentManagementConfigUpdateRequested(%q) = true, want false", query)
	}
	if !agentManagementReadOnlyGoalRejectsMutation(query) {
		t.Fatalf("agentManagementReadOnlyGoalRejectsMutation(%q) = false, want true", query)
	}

	parts := contextualConsoleAgentsManageCapabilityPartsForTest()
	parts.Query = query
	parts.RuntimeContext = "route=/console/agents/agent-1/agent"
	parts.SkillMode = skillModeAuto
	parts.SkillIDs = []string{skills.SkillAgentManagement, skills.SkillConsoleNavigator}

	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	for _, want := range []string{
		"list_agent_skill_candidates",
		"list_agent_knowledge_candidates",
		"list_agent_database_candidates",
		"list_agent_database_tables",
		"list_agent_workflow_binding_candidates",
	} {
		if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, want) {
			t.Fatalf("PlannedTools = %#v, missing read-only candidate tool agent-management/%s", strategy.PlannedTools, want)
		}
	}
	for _, unexpected := range []string{
		"update_agent_config",
		"update_agent_identity",
		"replace_agent_skill_bindings",
		"replace_agent_knowledge_bindings",
		"replace_agent_database_bindings",
		"replace_agent_workflow_bindings",
	} {
		if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, unexpected) {
			t.Fatalf("PlannedTools = %#v, want no mutation tool agent-management/%s", strategy.PlannedTools, unexpected)
		}
	}

	plan := operationPlanFromTurnStrategy("task-agent-read-only-bindable-candidates", parts, strategy)
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")); got != "" {
		t.Fatalf("update_agent_config step status = %q, want absent; plan=%#v", got, plan)
	}
	if got := plan["approval_required"]; got != false {
		t.Fatalf("approval_required = %#v, want false; plan=%#v", got, plan)
	}

	prepared := &PreparedChat{
		parts: parts,
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status":             operationPlanStatusRunning,
				"original_user_goal": query,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillAgentManagement, "list_agent_skill_candidates"),
						"status":    operationPlanStepStatusCompleted,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "list_agent_skill_candidates",
					},
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"),
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "update_agent_config",
					},
				},
				"step_status": map[string]interface{}{
					operationPlanToolStepID(skills.SkillAgentManagement, "list_agent_skill_candidates"): operationPlanStepStatusCompleted,
					operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"):         operationPlanStepStatusPending,
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
	guard := skillLoopPlanToolCallGuardWithResolved(prepared, resolved)
	result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillAgentManagement,
		ToolName:  "update_agent_config",
		Arguments: map[string]interface{}{"agent_id": "agent-1", "add_enabled_skill_ids": []interface{}{"chart-generator"}},
	})
	if !blocked {
		t.Fatal("update_agent_config was allowed for explicit read-only bindable candidate query")
	}
	if result.ToolName != "update_agent_config" || !strings.Contains(result.SystemMessage, "latest user request explicitly asks to read") {
		t.Fatalf("guard result = %#v, want latest read-only mutation block", result)
	}
}

func TestAgentReadOnlyBindableCandidatesCloseStaleMutationPlanAfterEvidence(t *testing.T) {
	query := "\u5192\u70df\u9a8c\u8bc18f\uff1a\u53ea\u8bfb\u67e5\u770b\u667a\u80fd\u4f53\u300cGOAL-MATRIX-178302-1783027245043-EDITED\u300d\u5f53\u524d\u53ef\u7ed1\u5b9a\u7684 Skill\u3001\u77e5\u8bc6\u5e93\u3001\u6570\u636e\u5e93\u8868\u3001\u5de5\u4f5c\u6d41\u5019\u9009\u5404\u6709\u54ea\u4e9b\u3002\u4e0d\u8981\u4fee\u6539\u914d\u7f6e\uff0c\u4e0d\u8981\u53d1\u8d77\u5ba1\u6279\u3002"
	candidateTools := []string{
		"list_agent_skill_candidates",
		"list_agent_knowledge_candidates",
		"list_agent_database_candidates",
		"list_agent_database_tables",
		"list_agent_workflow_binding_candidates",
	}
	steps := make([]interface{}, 0, len(candidateTools)+3)
	stepStatus := map[string]interface{}{}
	for _, toolName := range candidateTools {
		stepID := operationPlanToolStepID(skills.SkillAgentManagement, toolName)
		steps = append(steps, map[string]interface{}{
			"id":        stepID,
			"status":    operationPlanStepStatusPending,
			"skill_id":  skills.SkillAgentManagement,
			"tool_name": toolName,
		})
		stepStatus[stepID] = operationPlanStepStatusPending
	}
	for _, toolName := range []string{"get_agent_config", "update_agent_config"} {
		stepID := operationPlanToolStepID(skills.SkillAgentManagement, toolName)
		steps = append(steps, map[string]interface{}{
			"id":        stepID,
			"status":    operationPlanStepStatusPending,
			"skill_id":  skills.SkillAgentManagement,
			"tool_name": toolName,
		})
		stepStatus[stepID] = operationPlanStepStatusPending
	}
	steps = append(steps, map[string]interface{}{
		"id":     "observe",
		"status": operationPlanStepStatusPending,
	})
	stepStatus["observe"] = operationPlanStepStatusPending
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":              operationPlanStatusRunning,
			"original_user_goal":  query,
			"steps":               steps,
			"step_status":         stepStatus,
			"pending_next_action": "Run tool:agent-management/get_agent_config",
		},
	}

	invocations := make([]map[string]interface{}, 0, len(candidateTools))
	for _, toolName := range candidateTools {
		invocations = append(invocations, agentManagementToolInvocationForTest(toolName, "tool_call:agent-management:"+toolName+"::#1", map[string]interface{}{
			"status": "success",
			"count":  1,
			"items": []interface{}{
				map[string]interface{}{"id": toolName + "-1", "name": toolName + " result"},
			},
		}))
	}
	applyOperationPlanInvocationState(metadata, invocations)

	plan := metadata["operation_plan"].(map[string]interface{})
	if got := stringFromAny(plan["status"]); got != operationPlanStatusCompleted {
		t.Fatalf("operation_plan status = %q, want completed after read-only candidate evidence; plan=%#v", got, plan)
	}
	if got := stringFromAny(plan["pending_next_action"]); got != "none" {
		t.Fatalf("pending_next_action = %q, want none; plan=%#v", got, plan)
	}
	for _, toolName := range candidateTools {
		stepID := operationPlanToolStepID(skills.SkillAgentManagement, toolName)
		if got := operationPlanStepStatusForTest(plan, stepID); got != operationPlanStepStatusCompleted {
			t.Fatalf("%s status = %q, want completed; plan=%#v", toolName, got, plan)
		}
	}
	for _, stepID := range []string{
		operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config"),
		operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"),
		"observe",
	} {
		if got := operationPlanStepStatusForTest(plan, stepID); got != operationPlanStepStatusCompleted {
			t.Fatalf("%s status = %q, want completed closure; plan=%#v", stepID, got, plan)
		}
		if got := operationPlanStepFieldForTest(plan, stepID, "skipped_reason"); got != "covered_by_read_only_agent_candidate_lookup" {
			t.Fatalf("%s skipped_reason = %q, want read-only candidate closure; plan=%#v", stepID, got, plan)
		}
	}
	deviations := mapSliceFromAny(plan["deviations"])
	found := false
	for _, deviation := range deviations {
		if stringFromAny(deviation["skill_id"]) == skills.SkillAgentManagement &&
			stringFromAny(deviation["tool_name"]) == "update_agent_config" &&
			stringFromAny(deviation["reason"]) == "stale_mutation_plan_skipped_for_read_only_candidate_lookup" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("deviations = %#v, want skipped stale update_agent_config deviation", deviations)
	}
	state := mapFromOperationContext(plan["strategy_state"])
	if got := stringFromAny(state["status"]); got != operationPlanStatusCompleted {
		t.Fatalf("strategy_state.status = %q, want completed; plan=%#v", got, plan)
	}
}

func TestPreparedResultMetadataClosesReadOnlyCandidateLookupStaleMutationPlan(t *testing.T) {
	query := "\u5192\u70df\u9a8c\u8bc18f\uff1a\u53ea\u8bfb\u67e5\u770b\u667a\u80fd\u4f53\u300cGOAL-MATRIX-178302-1783027245043-EDITED\u300d\u5f53\u524d\u53ef\u7ed1\u5b9a\u7684 Skill\u3001\u77e5\u8bc6\u5e93\u3001\u6570\u636e\u5e93\u8868\u3001\u5de5\u4f5c\u6d41\u5019\u9009\u5404\u6709\u54ea\u4e9b\u3002\u4e0d\u8981\u4fee\u6539\u914d\u7f6e\uff0c\u4e0d\u8981\u53d1\u8d77\u5ba1\u6279\u3002"
	steps := []interface{}{}
	stepStatus := map[string]interface{}{}
	for _, toolName := range []string{
		"list_agent_skill_candidates",
		"list_agent_knowledge_candidates",
		"list_agent_database_candidates",
		"list_agent_database_tables",
		"list_agent_workflow_binding_candidates",
	} {
		stepID := operationPlanToolStepID(skills.SkillAgentManagement, toolName)
		steps = append(steps, map[string]interface{}{
			"id":        stepID,
			"status":    operationPlanStepStatusCompleted,
			"skill_id":  skills.SkillAgentManagement,
			"tool_name": toolName,
		})
		stepStatus[stepID] = operationPlanStepStatusCompleted
	}
	for _, toolName := range []string{"get_agent_config", "update_agent_config"} {
		stepID := operationPlanToolStepID(skills.SkillAgentManagement, toolName)
		steps = append(steps, map[string]interface{}{
			"id":        stepID,
			"status":    operationPlanStepStatusPending,
			"skill_id":  skills.SkillAgentManagement,
			"tool_name": toolName,
		})
		stepStatus[stepID] = operationPlanStepStatusPending
	}
	steps = append(steps, map[string]interface{}{"id": "observe", "status": operationPlanStepStatusPending})
	stepStatus["observe"] = operationPlanStepStatusPending
	metadata := preparedResultMetadata(map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":              operationPlanStatusRunning,
			"original_user_goal":  query,
			"steps":               steps,
			"step_status":         stepStatus,
			"pending_next_action": "Run tool:agent-management/get_agent_config",
		},
	}, nil)

	plan := metadata["operation_plan"].(map[string]interface{})
	if got := stringFromAny(plan["status"]); got != operationPlanStatusCompleted {
		t.Fatalf("operation_plan status = %q, want completed; plan=%#v", got, plan)
	}
	if got := stringFromAny(plan["pending_next_action"]); got != "none" {
		t.Fatalf("pending_next_action = %q, want none; plan=%#v", got, plan)
	}
	for _, stepID := range []string{
		operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config"),
		operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"),
		"observe",
	} {
		if got := operationPlanStepStatusForTest(plan, stepID); got != operationPlanStepStatusCompleted {
			t.Fatalf("%s status = %q, want completed closure; plan=%#v", stepID, got, plan)
		}
	}
}

func TestAgentConfigPlanTracksExpectedUpdatedFields(t *testing.T) {
	parts := &chatRequestParts{
		Query: strings.Join([]string{
			"请编辑当前智能体：",
			"名称改为 AICHAT-E2E-EDITED，描述和图标也一起更新。",
			"调用 list_available_models，use_case 用 text-chat，并把模型切换到返回列表里的 GPT 4o，provider/model 必须同步。",
			"系统提示词改为“你是端到端冒烟助手，请用中文简短回答”。",
			"首页标题改为 E2E Home，主题色改为 emerald，开场问题改为两条。",
			"不要绑定或解绑 Skill、知识库、数据库、工作流。",
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
	plan := operationPlanFromTurnStrategy("task-agent-config-expected-fields", parts, strategy)
	step := operationPlanStepForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"))
	if len(step) == 0 {
		t.Fatalf("update_agent_config step missing; plan=%#v", plan)
	}
	fields := stringSliceFromAny(step[operationPlanExpectedUpdatedFieldsKey])
	for _, want := range []string{"model", "system_prompt", "home_title", "theme_color", "suggested_questions"} {
		if !stringSliceContains(fields, want) {
			t.Fatalf("expected_updated_fields = %#v, missing %q; plan=%#v", fields, want, plan)
		}
	}
	args := mapFromOperationContext(step["arguments"])
	if got := strings.TrimSpace(stringFromAny(args["agent_id"])); got != "agent-1" {
		t.Fatalf("update_agent_config arguments.agent_id = %q, want agent-1; step=%#v", got, step)
	}
	for _, unexpected := range []string{"enabled_skill_ids", "knowledge_dataset_ids", "database_bindings", "workflow_bindings"} {
		if stringSliceContains(fields, unexpected) {
			t.Fatalf("expected_updated_fields = %#v, want no %q for explicit no-binding query; plan=%#v", fields, unexpected, plan)
		}
	}
}

func TestAgentDisplayConfigEditDoesNotPlanModelLookup(t *testing.T) {
	parts := &chatRequestParts{
		Query: strings.Join([]string{
			"运行配置编辑回归：请只处理当前智能体的运行与展示配置。",
			"先读取当前配置，然后用一次 update_agent_config 修改：system_prompt 为“你是 AIChat 配置闭环冒烟的测试智能体，只需用一句话回应测试请求。”",
			"home_title 改为“配置闭环测试首页”，input_placeholder 改为“请输入测试问题”，theme_color 改为 #16a34a，开场问题改为两条。",
			"不要修改模型/provider、名称、描述、图标、Skill、知识库、数据库、工作流。",
			"完成后重新读取配置验证，并最终回答实际变更的字段。",
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
	for _, unexpected := range []string{"list_available_models", "update_agent_identity"} {
		if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, unexpected) {
			t.Fatalf("PlannedTools = %#v, want no agent-management/%s for display config edit", strategy.PlannedTools, unexpected)
		}
	}
	for _, want := range []string{"get_agent_config", "update_agent_config"} {
		if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, want) {
			t.Fatalf("PlannedTools = %#v, missing agent-management/%s", strategy.PlannedTools, want)
		}
	}

	plan := operationPlanFromTurnStrategy("task-agent-display-config-no-model", parts, strategy)
	step := operationPlanStepForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"))
	if len(step) == 0 {
		t.Fatalf("update_agent_config step missing; plan=%#v", plan)
	}
	fields := stringSliceFromAny(step[operationPlanExpectedUpdatedFieldsKey])
	for _, want := range []string{"system_prompt", "home_title", "input_placeholder", "theme_color", "suggested_questions"} {
		if !stringSliceContains(fields, want) {
			t.Fatalf("expected_updated_fields = %#v, missing %q; plan=%#v", fields, want, plan)
		}
	}
	for _, unexpected := range []string{"model", "enabled_skill_ids", "knowledge_dataset_ids", "database_bindings", "workflow_bindings"} {
		if stringSliceContains(fields, unexpected) {
			t.Fatalf("expected_updated_fields = %#v, want no %q for display config edit; plan=%#v", fields, unexpected, plan)
		}
	}
}

func TestAgentDisplayConfigRetryWithSuggestedQuestionCurrentModelDoesNotPlanModelLookup(t *testing.T) {
	parts := &chatRequestParts{
		Query: strings.Join([]string{
			"Retry the runtime display config edit for Agent GOAL-CREATE-SMOKE-1782961316067-EDITED4.",
			"Set system_prompt to a short test prompt, home_title to Runtime Config 0702R2, input_placeholder to Ask 0702R2, theme_color to emerald.",
			"Set suggested questions to: Is 0702R2 config saved? and What is the current model?",
			"Do not modify model/provider, name, description, icon, Skill, knowledge base, database, or workflow.",
			"After approval, read the config again and answer with the actual changed fields.",
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
	if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "list_available_models") {
		t.Fatalf("PlannedTools = %#v, want no agent-management/list_available_models for negated display config retry", strategy.PlannedTools)
	}
	plan := operationPlanFromTurnStrategy("task-agent-display-config-retry-no-model", parts, strategy)
	step := operationPlanStepForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"))
	if len(step) == 0 {
		t.Fatalf("update_agent_config step missing; plan=%#v", plan)
	}
	fields := stringSliceFromAny(step[operationPlanExpectedUpdatedFieldsKey])
	for _, want := range []string{"system_prompt", "home_title", "input_placeholder", "theme_color", "suggested_questions"} {
		if !stringSliceContains(fields, want) {
			t.Fatalf("expected_updated_fields = %#v, missing %q; plan=%#v", fields, want, plan)
		}
	}
	if stringSliceContains(fields, "model") {
		t.Fatalf("expected_updated_fields = %#v, want no model field for negated display config retry; plan=%#v", fields, plan)
	}
}

func TestAgentConfigPlanTracksExpectedBindingActions(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "\u8bf7\u628a\u5f53\u524d\u667a\u80fd\u4f53\u7684\u77e5\u8bc6\u5e93\u548c\u6570\u636e\u8868\u90fd\u89e3\u7ed1\uff0c\u4e0d\u8981\u52a8 Skill \u6216\u5de5\u4f5c\u6d41\u3002",
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
	plan := operationPlanFromTurnStrategy("task-agent-binding-actions", parts, strategy)
	step := operationPlanStepForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"))
	if len(step) == 0 {
		t.Fatalf("update_agent_config step missing; plan=%#v", plan)
	}
	fields := stringSliceFromAny(step[operationPlanExpectedUpdatedFieldsKey])
	for _, want := range []string{"knowledge_dataset_ids", "database_bindings"} {
		if !stringSliceContains(fields, want) {
			t.Fatalf("expected_updated_fields = %#v, missing %q; plan=%#v", fields, want, plan)
		}
	}
	for _, unexpected := range []string{"enabled_skill_ids", "workflow_bindings"} {
		if stringSliceContains(fields, unexpected) {
			t.Fatalf("expected_updated_fields = %#v, want no %q; plan=%#v", fields, unexpected, plan)
		}
	}
	actions := operationPlanAgentConfigBindingActionsFromAny(step[operationPlanExpectedBindingActionsKey])
	for _, field := range []string{"knowledge_dataset_ids", "database_bindings"} {
		if actions[field] != "unbind" {
			t.Fatalf("expected_binding_actions[%s] = %#v, want unbind; actions=%#v plan=%#v", field, actions[field], actions, plan)
		}
	}
	for _, unexpected := range []string{"enabled_skill_ids", "workflow_bindings"} {
		if actions[unexpected] != "" {
			t.Fatalf("expected_binding_actions[%s] = %#v, want absent; actions=%#v plan=%#v", unexpected, actions[unexpected], actions, plan)
		}
	}
}

func TestAgentConfigUnbindExistingStateDoesNotPlanBindOrCandidateLookup(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "\u82e5\u5f53\u524d\u5df2\u7ed1\u5b9a\u77e5\u8bc6\u5e93\u3001\u6570\u636e\u8868\u6216\u5de5\u4f5c\u6d41\uff0c\u8bf7\u7528 update_agent_config \u7684 remove_knowledge_dataset_ids\u3001remove_database_bindings\u3001remove_workflow_bindings \u4e00\u6b21\u6027\u89e3\u7ed1\u5f53\u524d\u5df2\u7ed1\u5b9a\u7684\u77e5\u8bc6\u5e93\u3001\u6570\u636e\u8868\u548c\u5de5\u4f5c\u6d41\u3002\u4e0d\u8981\u4fee\u6539\u540d\u79f0\u3001\u63cf\u8ff0\u3001\u56fe\u6807\u3001\u6a21\u578b\u3001Skill\u3001\u7cfb\u7edf\u63d0\u793a\u8bcd\u3001\u9996\u9875\u6807\u9898\u6216\u5f00\u573a\u95ee\u9898\u3002\u5b8c\u6210\u540e\u5fc5\u987b\u91cd\u65b0\u8bfb\u53d6\u5f53\u524d\u914d\u7f6e\u3002",
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
	for _, want := range []string{"get_agent_config", "update_agent_config"} {
		if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, want) {
			t.Fatalf("PlannedTools = %#v, missing agent-management/%s", strategy.PlannedTools, want)
		}
	}
	for _, unexpected := range []string{
		"list_agent_knowledge_candidates",
		"list_agent_database_candidates",
		"list_agent_database_tables",
		"list_agent_workflow_binding_candidates",
		"list_agent_skill_candidates",
	} {
		if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, unexpected) {
			t.Fatalf("PlannedTools = %#v, want no %s for pure current-binding unbind", strategy.PlannedTools, unexpected)
		}
	}

	plan := operationPlanFromTurnStrategy("task-agent-existing-unbind", parts, strategy)
	step := operationPlanStepForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"))
	if len(step) == 0 {
		t.Fatalf("update_agent_config step missing; plan=%#v", plan)
	}
	actions := operationPlanAgentConfigBindingActionsFromAny(step[operationPlanExpectedBindingActionsKey])
	for _, field := range []string{"knowledge_dataset_ids", "database_bindings", "workflow_bindings"} {
		if actions[field] != "unbind" {
			t.Fatalf("expected_binding_actions[%s] = %#v, want unbind; actions=%#v plan=%#v", field, actions[field], actions, plan)
		}
	}
}

func TestAgentConfigCurrentBindingUnbindDoesNotPlanBindOrCandidateLookup(t *testing.T) {
	parts := &chatRequestParts{
		Query: strings.Join([]string{
			"\u53ea\u5904\u7406\u5f53\u524d\u667a\u80fd\u4f53\u5217\u8868\u7b2c\u4e00\u4e2a\u667a\u80fd\u4f53 GOAL-CREATE-SMOKE-1782961316067-EDITED4\u3002",
			"\u8bf7\u8bfb\u53d6\u914d\u7f6e\uff1b\u5982\u679c\u5f53\u524d\u7ed1\u5b9a\u4e86\u77e5\u8bc6\u5e93\u3001\u6570\u636e\u5e93\u8868\u3001\u5de5\u4f5c\u6d41\uff0c\u8bf7\u7533\u8bf7\u5ba1\u6279\u4e00\u6b21\u6027\u89e3\u7ed1\u8fd9\u4e9b\u8d44\u6e90\u3002",
			"\u6267\u884c\u540e\u57fa\u4e8e\u5de5\u5177\u8fd4\u56de\u548c\u6700\u7ec8\u7ed1\u5b9a\u72b6\u6001\u5982\u5b9e\u56de\u7b54\uff0c\u5fc5\u987b\u533a\u5206\u77e5\u8bc6\u5e93/\u6570\u636e\u5e93\u8868/\u5de5\u4f5c\u6d41\uff0c\u786e\u8ba4\u6700\u7ec8\u4e0d\u518d\u7ed1\u5b9a\u8fd9\u4e9b\u8d44\u6e90\u3002",
			"\u4e0d\u8981\u4fee\u6539\u540d\u79f0\u3001\u63cf\u8ff0\u3001\u56fe\u6807\u3001\u6a21\u578b\u3001\u7cfb\u7edf\u63d0\u793a\u8bcd\u3001\u5f00\u573a\u95ee\u9898\u3001Skill\u3002",
		}, ""),
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents",
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
	for _, want := range []string{"get_agent_config", "update_agent_config"} {
		if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, want) {
			t.Fatalf("PlannedTools = %#v, missing agent-management/%s", strategy.PlannedTools, want)
		}
	}
	for _, unexpected := range []string{
		"list_agent_knowledge_candidates",
		"list_agent_database_candidates",
		"list_agent_database_tables",
		"list_agent_workflow_binding_candidates",
	} {
		if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, unexpected) {
			t.Fatalf("PlannedTools = %#v, want no %s for current-binding unbind", strategy.PlannedTools, unexpected)
		}
	}

	plan := operationPlanFromTurnStrategy("task-agent-current-binding-unbind", parts, strategy)
	step := operationPlanStepForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"))
	if len(step) == 0 {
		t.Fatalf("update_agent_config step missing; plan=%#v", plan)
	}
	actions := operationPlanAgentConfigBindingActionsFromAny(step[operationPlanExpectedBindingActionsKey])
	for _, field := range []string{"knowledge_dataset_ids", "database_bindings", "workflow_bindings"} {
		if actions[field] != "unbind" {
			t.Fatalf("expected_binding_actions[%s] = %#v, want unbind; actions=%#v plan=%#v", field, actions[field], actions, plan)
		}
	}
}

func TestAgentConfigCurrentBindingUnbindAllResourcesDoesNotPlanCandidateLookup(t *testing.T) {
	query := "\u5e2e\u6211\u628a\u8fd9\u4e2a\u667a\u80fd\u4f53\u7ed1\u5b9a\u7684\u6570\u636e\u5e93/\u77e5\u8bc6\u5e93/\u5de5\u4f5c\u6d41/Skill \u90fd\u89e3\u7ed1\u3002\u9700\u8981\u5ba1\u6279\u65f6\u8bf7\u4e00\u6b21\u6027\u53d1\u8d77\u5ba1\u6279\uff1b\u5ba1\u6279\u901a\u8fc7\u540e\u91cd\u65b0\u8bfb\u53d6\u914d\u7f6e\uff0c\u57fa\u4e8e\u5de5\u5177\u8fd4\u56de\u503c\u786e\u8ba4\u5b9e\u9645\u89e3\u7ed1\u7ed3\u679c\u3002"
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
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	for _, want := range []string{"get_agent_config", "update_agent_config"} {
		if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, want) {
			t.Fatalf("PlannedTools = %#v, missing agent-management/%s", strategy.PlannedTools, want)
		}
	}
	for _, unexpected := range []string{
		"list_agent_skill_candidates",
		"list_agent_knowledge_candidates",
		"list_agent_database_candidates",
		"list_agent_database_tables",
		"list_agent_workflow_binding_candidates",
	} {
		if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, unexpected) {
			t.Fatalf("PlannedTools = %#v, want no %s for current-state unbind-all request", strategy.PlannedTools, unexpected)
		}
	}

	plan := operationPlanFromTurnStrategy("task-agent-current-binding-unbind-all", parts, strategy)
	step := operationPlanStepForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"))
	if len(step) == 0 {
		t.Fatalf("update_agent_config step missing; plan=%#v", plan)
	}
	actions := operationPlanAgentConfigBindingActionsFromAny(step[operationPlanExpectedBindingActionsKey])
	for _, field := range []string{"enabled_skill_ids", "knowledge_dataset_ids", "database_bindings", "workflow_bindings"} {
		if actions[field] != "unbind" {
			t.Fatalf("expected_binding_actions[%s] = %#v, want unbind; actions=%#v plan=%#v", field, actions[field], actions, plan)
		}
	}

	prepared := &PreparedChat{
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{"operation_plan": plan}},
		parts:   parts,
	}
	allowed := skillLoopAllowedPlannedTools(prepared)
	for _, unexpected := range []string{
		"list_agent_skill_candidates",
		"list_agent_knowledge_candidates",
		"list_agent_database_candidates",
		"list_agent_database_tables",
		"list_agent_workflow_binding_candidates",
	} {
		if _, ok := allowed[operationPlanToolStepID(skills.SkillAgentManagement, unexpected)]; ok {
			t.Fatalf("allowed planned tools = %#v, want no %s for current-state unbind-all request", allowed, unexpected)
		}
	}
}

func TestAgentConfigTrailingUnbindAllDoesNotTreatBoundSkillAsBind(t *testing.T) {
	query := "\u8bf7\u628a\u8fd9\u4e2a\u667a\u80fd\u4f53\u521a\u521a\u7ed1\u5b9a\u7684 Skill\u3001\u77e5\u8bc6\u5e93\u3001\u6570\u636e\u8868\u3001\u5de5\u4f5c\u6d41\u5168\u90e8\u89e3\u7ed1\u3002\u4e0d\u8981\u731c ID\uff1b\u5148\u8bfb\u53d6\u5f53\u524d\u914d\u7f6e\uff0c\u4f7f\u7528\u5f53\u524d\u7ed1\u5b9a\u7ed3\u679c\u53d1\u8d77\u4e00\u6b21\u6027\u89e3\u7ed1\u5ba1\u6279\u3002"
	actions := agentManagementExpectedConfigBindingActions(query)
	for _, field := range []string{"enabled_skill_ids", "knowledge_dataset_ids", "database_bindings", "workflow_bindings"} {
		if actions[field] != "unbind" {
			t.Fatalf("expected_binding_actions[%s] = %#v, want unbind; actions=%#v", field, actions[field], actions)
		}
	}

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
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	plan := operationPlanFromTurnStrategy("task-agent-trailing-unbind-all", parts, strategy)
	step := operationPlanStepForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"))
	if len(step) == 0 {
		t.Fatalf("update_agent_config step missing; plan=%#v", plan)
	}
	planActions := operationPlanAgentConfigBindingActionsFromAny(step[operationPlanExpectedBindingActionsKey])
	for _, field := range []string{"enabled_skill_ids", "knowledge_dataset_ids", "database_bindings", "workflow_bindings"} {
		if planActions[field] != "unbind" {
			t.Fatalf("plan expected_binding_actions[%s] = %#v, want unbind; actions=%#v plan=%#v", field, planActions[field], planActions, plan)
		}
	}
}

func TestAgentManagementGuidanceSurfacesExpectedUnbindPlanFields(t *testing.T) {
	query := "\u8bf7\u628a\u8fd9\u4e2a\u667a\u80fd\u4f53\u521a\u521a\u7ed1\u5b9a\u7684 Skill\u3001\u77e5\u8bc6\u5e93\u3001\u6570\u636e\u8868\u3001\u5de5\u4f5c\u6d41\u5168\u90e8\u89e3\u7ed1\u3002\u4e0d\u8981\u731c ID\uff1b\u5148\u8bfb\u53d6\u5f53\u524d\u914d\u7f6e\uff0c\u4f7f\u7528\u5f53\u524d\u7ed1\u5b9a\u7ed3\u679c\u53d1\u8d77\u4e00\u6b21\u6027\u89e3\u7ed1\u5ba1\u6279\u3002"
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
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	plan := operationPlanFromTurnStrategy("task-agent-guidance-unbind-all", parts, strategy)
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{"operation_plan": plan}},
		parts:   parts,
	}
	message, ok := contextualConsoleAgentsSkillMessage(prepared)
	if !ok {
		t.Fatal("contextualConsoleAgentsSkillMessage() ok = false, want guidance")
	}
	content := stringFromAny(message.Content)

	for _, want := range []string{
		"Active Agent config plan expects update_agent_config",
		"enabled_skill_ids=unbind",
		"knowledge_dataset_ids=unbind",
		"database_bindings=unbind",
		"workflow_bindings=unbind",
		"remove_enabled_skill_ids",
		"remove_knowledge_dataset_ids",
		"remove_database_bindings",
		"remove_workflow_bindings",
		"Do not replace a governed mutation with a natural-language confirmation",
		"Do not finish after a partial binding edit",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("agent guidance missing %q\ncontent:\n%s", want, content)
		}
	}
}

func TestAgentBindingCandidateReadDoesNotLeakNoopSkillScope(t *testing.T) {
	query := strings.Join([]string{
		"\u8bf7\u53ea\u5904\u7406\u5f53\u524d\u667a\u80fd\u4f53\u7684\u8d44\u6e90\u7ed1\u5b9a\u3002",
		"\u5148\u8bfb\u53d6\u5f53\u524d\u914d\u7f6e\uff0c\u7136\u540e\u5206\u522b\u67e5\u8be2\u5f53\u524d\u5de5\u4f5c\u7a7a\u95f4\u53ef\u7ed1\u5b9a\u7684\u77e5\u8bc6\u5e93\u3001\u6570\u636e\u5e93\u3001\u6570\u636e\u5e93\u8868\u3001\u5de5\u4f5c\u6d41\u5019\u9009\u3002",
		"\u82e5\u6709\u53ef\u7528\u77e5\u8bc6\u5e93\uff0c\u8bf7\u7ed1\u5b9a\u7b2c\u4e00\u4e2a\u672a\u7ed1\u5b9a\u77e5\u8bc6\u5e93\uff1b\u82e5\u6709\u53ef\u7528\u6570\u636e\u5e93\u8868\uff0c\u8bf7\u7ed1\u5b9a\u7b2c\u4e00\u4e2a\u672a\u7ed1\u5b9a\u6570\u636e\u5e93\u8868\u4e3a\u53ea\u8bfb\uff1b\u82e5\u6709\u53ef\u7528\u5de5\u4f5c\u6d41\uff0c\u8bf7\u7ed1\u5b9a\u7b2c\u4e00\u4e2a\u672a\u7ed1\u5b9a\u5de5\u4f5c\u6d41\u3002",
		"\u8bf7\u7528 update_agent_config \u4e00\u6b21\u6027\u63d0\u4ea4\u53ef\u6267\u884c\u7684\u7ed1\u5b9a\u53d8\u66f4\uff0c\u4e0d\u8981\u66ff\u6362\u6216\u6e05\u7a7a\u73b0\u6709\u7ed1\u5b9a\uff0c\u4e0d\u8981\u4fee\u6539\u540d\u79f0\u3001\u63cf\u8ff0\u3001\u56fe\u6807\u3001\u6a21\u578b\u3001Skill\u3001\u7cfb\u7edf\u63d0\u793a\u8bcd\u3001\u9996\u9875\u6807\u9898\u6216\u5f00\u573a\u95ee\u9898\u3002",
		"\u5b8c\u6210\u540e\u5fc5\u987b\u91cd\u65b0\u8bfb\u53d6\u5f53\u524d\u914d\u7f6e\u3002",
	}, "")
	if agentManagementSkillBindingRequested(query) {
		t.Fatalf("agentManagementSkillBindingRequested(%q) = true, want false for noop Skill phrase", query)
	}
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
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	for _, want := range []string{
		"get_agent_config",
		"list_agent_knowledge_candidates",
		"list_agent_database_candidates",
		"list_agent_database_tables",
		"list_agent_workflow_binding_candidates",
		"update_agent_config",
	} {
		if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, want) {
			t.Fatalf("PlannedTools = %#v, missing agent-management/%s", strategy.PlannedTools, want)
		}
	}
	if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "list_agent_skill_candidates") {
		t.Fatalf("PlannedTools = %#v, want no Skill candidate lookup for noop Skill phrase", strategy.PlannedTools)
	}
	plan := operationPlanFromTurnStrategy("task-agent-bind-no-skill-candidates", parts, strategy)
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "list_agent_skill_candidates")); got != "" {
		t.Fatalf("list_agent_skill_candidates step status = %#v, want absent; plan=%#v", got, plan)
	}
}

func TestAgentConfigPreserveOtherSectionsOnlyPlansRuntimeConfigPatch(t *testing.T) {
	parts := &chatRequestParts{
		Query: strings.Join([]string{
			"\u914d\u7f6e\u7f16\u8f91\u590d\u6d4b\uff1a\u8bf7\u53ea\u4fee\u6539\u5f53\u524d\u667a\u80fd\u4f53\u7cfb\u7edf\u63d0\u793a\u8bcd\u4e3a\u201c\u4f60\u662f\u4e00\u4e2a AIChat Agent \u914d\u7f6e\u95ed\u73af\u9a8c\u8bc1\u52a9\u624b\u201d\uff0c",
			"\u9996\u9875\u6807\u9898\u4e3a\u201cAIChat \u914d\u7f6e\u95ed\u73af\u590d\u6d4b 0630-E\u201d\uff0c",
			"\u5f00\u573a\u95ee\u9898\u6539\u4e3a\u4e09\u6761\uff1a[\u201c\u5e2e\u6211\u603b\u7ed3\u5f53\u524d\u914d\u7f6e\u201d, \u201c\u5e2e\u6211\u68c0\u67e5\u7ed1\u5b9a\u8d44\u6e90\u201d, \u201c\u5e2e\u6211\u751f\u6210\u4e00\u4e2a\u6d4b\u8bd5\u95ee\u9898\u201d]\u3002",
			"\u8bf7\u4fdd\u7559\u540d\u79f0\u3001\u63cf\u8ff0\u3001\u56fe\u6807\u3001\u6a21\u578b\u3001Skill\u3001\u77e5\u8bc6\u5e93\u3001\u6570\u636e\u5e93\u3001\u5de5\u4f5c\u6d41\u6216\u5176\u4ed6\u914d\u7f6e\u3002",
			"\u9700\u8981\u5ba1\u6279\u65f6\u6211\u4f1a\u540c\u610f\uff1b\u5b8c\u6210\u540e\u8bf7\u57fa\u4e8e\u5de5\u5177\u8fd4\u56de\u503c\u548c\u9875\u9762\u53ef\u89c1\u7ed3\u679c\u8bf4\u660e\u5b9e\u9645\u4fee\u6539\u5b57\u6bb5\uff0c\u4e0d\u8981\u8bf4\u4fee\u6539\u4e86\u6a21\u578b\u6216\u7ed1\u5b9a\u8d44\u6e90\u3002",
		}, ""),
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
	for _, want := range []string{"get_agent_config", "update_agent_config"} {
		if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, want) {
			t.Fatalf("PlannedTools = %#v, missing agent-management/%s", strategy.PlannedTools, want)
		}
	}
	for _, unexpected := range []string{
		"update_agent_identity",
		"list_available_models",
		"list_agent_skill_candidates",
		"list_agent_knowledge_candidates",
		"list_agent_database_candidates",
		"list_agent_database_tables",
		"list_agent_workflow_binding_candidates",
		"replace_agent_skill_bindings",
		"replace_agent_knowledge_bindings",
		"replace_agent_database_bindings",
		"replace_agent_workflow_bindings",
	} {
		if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, unexpected) {
			t.Fatalf("PlannedTools = %#v, want no agent-management/%s", strategy.PlannedTools, unexpected)
		}
	}

	plan := streamingMessageMetadataWithTaskID(parts, "task-agent-config-preserve-other-sections")["operation_plan"].(map[string]interface{})
	for _, want := range []string{"get_agent_config", "update_agent_config"} {
		if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, want)); got != operationPlanStepStatusPending {
			t.Fatalf("operation plan step %s = %#v, want pending; plan=%#v", want, got, plan)
		}
	}
	for _, unexpected := range []string{"update_agent_identity", "list_available_models", "list_agent_skill_candidates", "list_agent_knowledge_candidates", "list_agent_database_candidates", "list_agent_database_tables", "list_agent_workflow_binding_candidates"} {
		if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, unexpected)); got != "" {
			t.Fatalf("operation plan step %s = %#v, want absent; plan=%#v", unexpected, got, plan)
		}
	}
	if actions := stringSliceFromAny(plan["approval_actions"]); !reflect.DeepEqual(actions, []string{operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")}) {
		t.Fatalf("approval_actions = %#v, want only update_agent_config", actions)
	}
}

func TestOperationPlanAgentConfigExpectedFieldsRequireCumulativeCoverage(t *testing.T) {
	updateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{map[string]interface{}{
				"id":                                  updateStepID,
				"status":                              operationPlanStepStatusPending,
				"skill_id":                            skills.SkillAgentManagement,
				"tool_name":                           "update_agent_config",
				operationPlanExpectedUpdatedFieldsKey: []interface{}{"model", "system_prompt", "home_title"},
			}},
			"step_status": map[string]interface{}{
				updateStepID: operationPlanStepStatusPending,
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
			"status":         "completed",
			"agent_id":       "agent-1",
			"updated_fields": []interface{}{"system_prompt", "home_title"},
			"config": map[string]interface{}{
				"model_provider": "deepseek",
				"model":          "deepseek-chat",
			},
		},
	}})

	plan := metadata["operation_plan"].(map[string]interface{})
	if got := plan["status"]; got != operationPlanStatusRunning {
		t.Fatalf("plan status = %#v, want running after partial config update; plan=%#v", got, plan)
	}
	if got := operationPlanStepStatusForTest(plan, updateStepID); got != operationPlanStepStatusPending {
		t.Fatalf("update_agent_config status = %#v, want pending after partial update; plan=%#v", got, plan)
	}
	step := operationPlanStepForTest(plan, updateStepID)
	if missing := stringSliceFromAny(step["missing_updated_fields"]); !reflect.DeepEqual(missing, []string{"model"}) {
		t.Fatalf("missing_updated_fields = %#v, want [model]; plan=%#v", missing, plan)
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "update_agent_config",
		"result": map[string]interface{}{
			"status":         "completed",
			"agent_id":       "agent-1",
			"updated_fields": []interface{}{"model_provider", "model"},
		},
	}})

	plan = metadata["operation_plan"].(map[string]interface{})
	if got := operationPlanStepStatusForTest(plan, updateStepID); got != operationPlanStepStatusCompleted {
		t.Fatalf("update_agent_config status = %#v, want completed after cumulative model coverage; plan=%#v", got, plan)
	}
	step = operationPlanStepForTest(plan, updateStepID)
	if missing := stringSliceFromAny(step["missing_updated_fields"]); len(missing) > 0 {
		t.Fatalf("missing_updated_fields = %#v, want cleared; plan=%#v", missing, plan)
	}
	if fields := stringSliceFromAny(step["completed_updated_fields"]); !stringSliceContains(fields, "model") || !stringSliceContains(fields, "system_prompt") || !stringSliceContains(fields, "home_title") {
		t.Fatalf("completed_updated_fields = %#v, want cumulative model/system_prompt/home_title; plan=%#v", fields, plan)
	}
}

func TestOperationPlanAgentConfigSatisfiedFieldsCloseAlreadyCurrentValues(t *testing.T) {
	updateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{map[string]interface{}{
				"id":                                  updateStepID,
				"status":                              operationPlanStepStatusPending,
				"skill_id":                            skills.SkillAgentManagement,
				"tool_name":                           "update_agent_config",
				operationPlanExpectedUpdatedFieldsKey: []interface{}{"home_title", "theme_color", "suggested_questions"},
			}},
			"step_status": map[string]interface{}{
				updateStepID: operationPlanStepStatusPending,
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
			"status":           "completed",
			"agent_id":         "agent-1",
			"updated_fields":   []interface{}{"home_title", "suggested_questions"},
			"satisfied_fields": []interface{}{"home_title", "theme_color", "suggested_questions"},
			"theme_color":      "emerald",
		},
	}})

	plan := metadata["operation_plan"].(map[string]interface{})
	if got := operationPlanStepStatusForTest(plan, updateStepID); got != operationPlanStepStatusCompleted {
		t.Fatalf("update_agent_config status = %#v, want completed from satisfied fields; plan=%#v", got, plan)
	}
	step := operationPlanStepForTest(plan, updateStepID)
	if missing := stringSliceFromAny(step["missing_updated_fields"]); len(missing) > 0 {
		t.Fatalf("missing_updated_fields = %#v, want cleared; plan=%#v", missing, plan)
	}
	if fields := stringSliceFromAny(step["completed_updated_fields"]); !stringSliceContains(fields, "theme_color") {
		t.Fatalf("completed_updated_fields = %#v, want theme_color satisfied evidence; plan=%#v", fields, plan)
	}
}

func TestOperationPlanEmptySkillCandidateClosesPureSkillBindingUpdate(t *testing.T) {
	candidateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "list_agent_skill_candidates")
	updateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{
				map[string]interface{}{
					"id":        candidateStepID,
					"status":    operationPlanStepStatusPending,
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "list_agent_skill_candidates",
					"arguments": map[string]interface{}{
						"agent_id": "agent-1",
						"query":    "missing-skill",
					},
				},
				map[string]interface{}{
					"id":                                   updateStepID,
					"status":                               operationPlanStepStatusPending,
					"skill_id":                             skills.SkillAgentManagement,
					"tool_name":                            "update_agent_config",
					operationPlanExpectedUpdatedFieldsKey:  []interface{}{"enabled_skill_ids"},
					operationPlanExpectedBindingActionsKey: "enabled_skill_ids:bind",
				},
			},
			"step_status": map[string]interface{}{
				candidateStepID: operationPlanStepStatusPending,
				updateStepID:    operationPlanStepStatusPending,
			},
			"status": operationPlanStatusRunning,
		},
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "list_agent_skill_candidates",
		"arguments": map[string]interface{}{
			"agent_id": "agent-1",
			"query":    "missing-skill",
		},
		"result": map[string]interface{}{
			"status":   "completed",
			"agent_id": "agent-1",
			"query":    "missing-skill",
			"count":    0,
			"skills":   []interface{}{},
		},
	}})

	plan := metadata["operation_plan"].(map[string]interface{})
	if got := operationPlanStepStatusForTest(plan, candidateStepID); got != operationPlanStepStatusCompleted {
		t.Fatalf("candidate step status = %#v, want completed; plan=%#v", got, plan)
	}
	if got := operationPlanStepStatusForTest(plan, updateStepID); got != operationPlanStepStatusCompleted {
		t.Fatalf("update step status = %#v, want completed no-op; plan=%#v", got, plan)
	}
	updateStep := operationPlanStepForTest(plan, updateStepID)
	if got := stringFromAny(updateStep["skipped_reason"]); got != "agent_skill_candidate_not_found" {
		t.Fatalf("update skipped_reason = %#v, want agent_skill_candidate_not_found; step=%#v", got, updateStep)
	}
	if got := plan["status"]; got != operationPlanStatusCompleted {
		t.Fatalf("plan status = %#v, want completed no-op; plan=%#v", got, plan)
	}
	targetResolution := mapFromOperationContext(plan["target_resolution"])
	if got := stringFromAny(targetResolution["query"]); got != "missing-skill" {
		t.Fatalf("target_resolution.query = %#v, want missing-skill; target_resolution=%#v", got, targetResolution)
	}
}

func TestOperationPlanEmptySkillCandidateDoesNotCloseMixedConfigUpdate(t *testing.T) {
	candidateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "list_agent_skill_candidates")
	updateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{
				map[string]interface{}{
					"id":        candidateStepID,
					"status":    operationPlanStepStatusPending,
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "list_agent_skill_candidates",
				},
				map[string]interface{}{
					"id":                                   updateStepID,
					"status":                               operationPlanStepStatusPending,
					"skill_id":                             skills.SkillAgentManagement,
					"tool_name":                            "update_agent_config",
					operationPlanExpectedUpdatedFieldsKey:  []interface{}{"system_prompt", "enabled_skill_ids"},
					operationPlanExpectedBindingActionsKey: "enabled_skill_ids:bind",
				},
			},
			"step_status": map[string]interface{}{
				candidateStepID: operationPlanStepStatusPending,
				updateStepID:    operationPlanStepStatusPending,
			},
			"status": operationPlanStatusRunning,
		},
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "list_agent_skill_candidates",
		"result": map[string]interface{}{
			"status": "completed",
			"count":  0,
			"skills": []interface{}{},
		},
	}})

	plan := metadata["operation_plan"].(map[string]interface{})
	if got := operationPlanStepStatusForTest(plan, updateStepID); got != operationPlanStepStatusPending {
		t.Fatalf("mixed update step status = %#v, want still pending for non-skill fields; plan=%#v", got, plan)
	}
}

func TestOperationPlanAgentConfigFinalBindingStateClosesSatisfiedUnbind(t *testing.T) {
	updateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{map[string]interface{}{
				"id":                                   updateStepID,
				"status":                               operationPlanStepStatusPending,
				"skill_id":                             skills.SkillAgentManagement,
				"tool_name":                            "update_agent_config",
				operationPlanExpectedUpdatedFieldsKey:  []interface{}{"enabled_skill_ids"},
				operationPlanExpectedBindingActionsKey: "enabled_skill_ids:unbind",
			}},
			"step_status": map[string]interface{}{
				updateStepID: operationPlanStepStatusPending,
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
			"status":           "completed",
			"agent_id":         "agent-1",
			"satisfied_fields": []interface{}{"enabled_skill_ids"},
			"binding_final_states": []interface{}{map[string]interface{}{
				"field":                "enabled_skill_ids",
				"binding_kind":         "agent_skill",
				"change_action":        "satisfied",
				"final_resource_count": 0,
			}},
		},
	}})

	plan := metadata["operation_plan"].(map[string]interface{})
	if got := operationPlanStepStatusForTest(plan, updateStepID); got != operationPlanStepStatusCompleted {
		t.Fatalf("update_agent_config status = %#v, want completed from satisfied final binding state; plan=%#v", got, plan)
	}
	step := operationPlanStepForTest(plan, updateStepID)
	if missing := stringSliceFromAny(step["missing_updated_fields"]); len(missing) > 0 {
		t.Fatalf("missing_updated_fields = %#v, want cleared; plan=%#v", missing, plan)
	}
	actions := operationPlanAgentConfigBindingActionsFromAny(step["completed_binding_actions"])
	if actions["enabled_skill_ids"] != "unbind" {
		t.Fatalf("completed_binding_actions = %#v, want enabled_skill_ids:unbind; plan=%#v", actions, plan)
	}
}

func TestOperationPlanAgentConfigResourceFinalBindingStatesCloseSatisfiedUnbind(t *testing.T) {
	updateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{map[string]interface{}{
				"id":                                  updateStepID,
				"status":                              operationPlanStepStatusPending,
				"skill_id":                            skills.SkillAgentManagement,
				"tool_name":                           "update_agent_config",
				operationPlanExpectedUpdatedFieldsKey: []interface{}{"knowledge_dataset_ids", "database_bindings", "workflow_bindings"},
				operationPlanExpectedBindingActionsKey: "knowledge_dataset_ids:unbind," +
					"database_bindings:unbind,workflow_bindings:unbind",
			}},
			"step_status": map[string]interface{}{
				updateStepID: operationPlanStepStatusPending,
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
			"status":           "completed",
			"agent_id":         "agent-1",
			"satisfied_fields": []interface{}{"knowledge_dataset_ids", "database_bindings", "workflow_bindings"},
			"binding_final_states": []interface{}{
				map[string]interface{}{
					"field":                "knowledge_dataset_ids",
					"binding_kind":         "knowledge_base",
					"change_action":        "satisfied",
					"final_resource_count": 0,
				},
				map[string]interface{}{
					"field":                "database_bindings",
					"binding_kind":         "database_table",
					"change_action":        "satisfied",
					"final_resource_count": 0,
				},
				map[string]interface{}{
					"field":                "workflow_bindings",
					"binding_kind":         "workflow",
					"change_action":        "satisfied",
					"final_resource_count": 0,
				},
			},
		},
	}})

	plan := metadata["operation_plan"].(map[string]interface{})
	if got := operationPlanStepStatusForTest(plan, updateStepID); got != operationPlanStepStatusCompleted {
		t.Fatalf("update_agent_config status = %#v, want completed from resource final binding states; plan=%#v", got, plan)
	}
	step := operationPlanStepForTest(plan, updateStepID)
	if missing := stringSliceFromAny(step["missing_updated_fields"]); len(missing) > 0 {
		t.Fatalf("missing_updated_fields = %#v, want cleared; plan=%#v", missing, plan)
	}
	actions := operationPlanAgentConfigBindingActionsFromAny(step["completed_binding_actions"])
	for _, field := range []string{"knowledge_dataset_ids", "database_bindings", "workflow_bindings"} {
		if actions[field] != "unbind" {
			t.Fatalf("completed_binding_actions[%s] = %#v, want unbind; actions=%#v plan=%#v", field, actions[field], actions, plan)
		}
	}
}

func TestOperationPlanAgentConfigBindingActionMismatchStaysPending(t *testing.T) {
	updateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{map[string]interface{}{
				"id":                                   updateStepID,
				"status":                               operationPlanStepStatusPending,
				"skill_id":                             skills.SkillAgentManagement,
				"tool_name":                            "update_agent_config",
				operationPlanExpectedUpdatedFieldsKey:  []interface{}{"knowledge_dataset_ids"},
				operationPlanExpectedBindingActionsKey: "knowledge_dataset_ids:unbind",
			}},
			"step_status": map[string]interface{}{
				updateStepID: operationPlanStepStatusPending,
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
			"status":         "completed",
			"agent_id":       "agent-1",
			"updated_fields": []interface{}{"knowledge_dataset_ids"},
			"binding_changes": []interface{}{map[string]interface{}{
				"field":         "knowledge_dataset_ids",
				"binding_kind":  "knowledge_base",
				"change_action": "bind",
			}},
		},
	}})

	plan := metadata["operation_plan"].(map[string]interface{})
	if got := operationPlanStepStatusForTest(plan, updateStepID); got != operationPlanStepStatusPending {
		t.Fatalf("update_agent_config status = %#v, want pending after binding action mismatch; plan=%#v", got, plan)
	}
	step := operationPlanStepForTest(plan, updateStepID)
	if missing := stringSliceFromAny(step["missing_updated_fields"]); !reflect.DeepEqual(missing, []string{"knowledge_dataset_ids"}) {
		t.Fatalf("missing_updated_fields = %#v, want [knowledge_dataset_ids]; plan=%#v", missing, plan)
	}
	if mismatches := stringSliceFromAny(step["binding_action_mismatch"]); len(mismatches) == 0 {
		t.Fatalf("binding_action_mismatch = %#v, want mismatch evidence; plan=%#v", mismatches, plan)
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "update_agent_config",
		"result": map[string]interface{}{
			"status":         "completed",
			"agent_id":       "agent-1",
			"updated_fields": []interface{}{"knowledge_dataset_ids"},
			"binding_changes": []interface{}{map[string]interface{}{
				"field":                  "knowledge_dataset_ids",
				"binding_kind":           "knowledge_base",
				"change_action":          "unbind",
				"removed_resource_count": 1,
			}},
		},
	}})

	plan = metadata["operation_plan"].(map[string]interface{})
	if got := operationPlanStepStatusForTest(plan, updateStepID); got != operationPlanStepStatusCompleted {
		t.Fatalf("update_agent_config status = %#v, want completed after matching unbind evidence; plan=%#v", got, plan)
	}
	step = operationPlanStepForTest(plan, updateStepID)
	if mismatches := stringSliceFromAny(step["binding_action_mismatch"]); len(mismatches) > 0 {
		t.Fatalf("binding_action_mismatch = %#v, want cleared; plan=%#v", mismatches, plan)
	}
	actions := operationPlanAgentConfigBindingActionsFromAny(step["completed_binding_actions"])
	if actions["knowledge_dataset_ids"] != "unbind" {
		t.Fatalf("completed_binding_actions = %#v, want knowledge_dataset_ids:unbind; plan=%#v", actions, plan)
	}
}

func TestCurrentAgentSkillDisablePlansConfigBindingUpdate(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "\u8bf7\u628a\u5f53\u524d\u667a\u80fd\u4f53\u7684\u56fe\u8868\u751f\u6210\u5668 Skill \u89e3\u7ed1/\u505c\u7528\u3002\u53ea\u505a\u8fd9\u4e2a\u914d\u7f6e\u53d8\u66f4\uff0c\u5ba1\u6279\u540e\u8bf7\u57fa\u4e8e\u5de5\u5177\u8fd4\u56de\u503c\u548c\u9875\u9762\u53ef\u89c1\u7ed3\u679c\u786e\u8ba4\uff0c\u5e76\u5728\u6700\u7ec8\u56de\u590d\u91cc\u5199\u51fa\u5177\u4f53 Skill \u540d\u79f0\uff0c\u4e0d\u8981\u5207\u6362\u9875\u9762\u3002",
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
	if strategy.RouteRequired {
		t.Fatalf("strategy.RouteRequired = true, want false for current Agent detail config edit; strategy=%#v", strategy)
	}
	for _, want := range []string{"get_agent_config", "list_agent_skill_candidates", "update_agent_config"} {
		if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, want) {
			t.Fatalf("PlannedTools = %#v, missing agent-management/%s", strategy.PlannedTools, want)
		}
	}
	for _, unexpected := range []struct {
		skillID  string
		toolName string
	}{
		{skills.SkillConsoleNavigator, "navigate"},
		{skills.SkillChartGenerator, "generate_chart"},
		{skills.SkillFileGenerator, "generate_file"},
	} {
		if aiChatTurnStrategyHasPlannedToolForTest(strategy, unexpected.skillID, unexpected.toolName) {
			t.Fatalf("PlannedTools = %#v, want no %s/%s", strategy.PlannedTools, unexpected.skillID, unexpected.toolName)
		}
	}

	metadata := streamingMessageMetadataWithTaskID(parts, "task-agent-disable-skill")
	plan := metadata["operation_plan"].(map[string]interface{})
	for _, want := range []string{"get_agent_config", "list_agent_skill_candidates", "update_agent_config"} {
		if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, want)); got != operationPlanStepStatusPending {
			t.Fatalf("operation plan step %s = %#v, want pending; plan=%#v", want, got, plan)
		}
	}
	if got := stringFromAny(plan["pending_next_action"]); !strings.Contains(got, "agent-management/get_agent_config") {
		t.Fatalf("pending_next_action = %q, want first Agent config step; plan=%#v", got, plan)
	}
}

func TestAgentConfigUpdatePlanRequiresPostUpdateConfigReadWhenRequested(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "\u8bf7\u628a\u56fe\u8868\u751f\u6210\u5668 Skill \u7ed1\u5b9a\u5230\u5f53\u524d\u667a\u80fd\u4f53\u3002\u66f4\u65b0\u5b8c\u6210\u540e\u5fc5\u987b\u518d\u6b21\u8bfb\u53d6\u8be5\u667a\u80fd\u4f53\u914d\u7f6e\u9a8c\u8bc1\uff0c\u5e76\u5728\u6700\u7ec8\u56de\u7b54\u91cc\u8bf4\u660e\u590d\u8bfb\u914d\u7f6e\u540e\u5b83\u662f\u5426\u4ecd\u5904\u4e8e\u5df2\u7ed1\u5b9a\u72b6\u6001\u3002",
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents/agent-1/agent",
		SkillMode:      skillModeAuto,
		SkillIDs: []string{
			skills.SkillAgentManagement,
			skills.SkillConsoleNavigator,
		},
	}

	metadata := streamingMessageMetadataWithTaskID(parts, "task-agent-post-read")
	plan := metadata["operation_plan"].(map[string]interface{})
	preReadStepID := operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config")
	updateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")
	postReadStepID := operationPlanPostUpdateAgentConfigReadStepID()

	for _, want := range []string{preReadStepID, updateStepID, postReadStepID} {
		if got := operationPlanStepStatusForTest(plan, want); got != operationPlanStepStatusPending {
			t.Fatalf("operation plan step %s = %q, want pending; plan=%#v", want, got, plan)
		}
	}
	if got := operationPlanStepFieldForTest(plan, postReadStepID, "wait_for"); got != updateStepID {
		t.Fatalf("post read wait_for = %q, want %q", got, updateStepID)
	}
	if got := operationPlanStepFieldForTest(plan, postReadStepID, "phase"); got != "post_update_verification" {
		t.Fatalf("post read phase = %q, want post_update_verification", got)
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{
		{
			"kind":       "tool_call",
			"status":     "success",
			"skill_id":   skills.SkillAgentManagement,
			"tool_name":  "get_agent_config",
			"runtime_id": "tool_call:agent-management:get_agent_config::#1",
			"result": map[string]interface{}{
				"status":     "success",
				"agent_id":   "agent-1",
				"agent_name": "Support Agent",
			},
		},
	})
	plan = metadata["operation_plan"].(map[string]interface{})
	if got := operationPlanStepStatusForTest(plan, preReadStepID); got != operationPlanStepStatusCompleted {
		t.Fatalf("pre read status = %q, want completed; plan=%#v", got, plan)
	}
	if got := operationPlanStepStatusForTest(plan, postReadStepID); got != operationPlanStepStatusPending {
		t.Fatalf("post read status after initial read = %q, want pending; plan=%#v", got, plan)
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{
		{
			"kind":       "tool_call",
			"status":     "success",
			"skill_id":   skills.SkillAgentManagement,
			"tool_name":  "list_agent_skill_candidates",
			"runtime_id": "tool_call:agent-management:list_agent_skill_candidates::#1",
			"result": map[string]interface{}{
				"status": "success",
				"items": []interface{}{
					map[string]interface{}{"id": "chart-generator", "name": "\u56fe\u8868\u751f\u6210\u5668"},
				},
			},
		},
		{
			"kind":       "tool_call",
			"status":     "success",
			"skill_id":   skills.SkillAgentManagement,
			"tool_name":  "update_agent_config",
			"runtime_id": "tool_call:agent-management:update_agent_config::#2",
			"result": map[string]interface{}{
				"status":              "completed",
				"agent_id":            "agent-1",
				"agent_name":          "Support Agent",
				"updated_fields":      []interface{}{"enabled_skill_ids"},
				"resource_names":      []interface{}{"\u56fe\u8868\u751f\u6210\u5668"},
				"change_action":       "bind",
				"resource_count":      1,
				"binding_kind":        "agent_skill",
				"enabled_skill_count": 1,
			},
		},
	})
	plan = metadata["operation_plan"].(map[string]interface{})
	if got := operationPlanStepStatusForTest(plan, updateStepID); got != operationPlanStepStatusCompleted {
		t.Fatalf("update status = %q, want completed; plan=%#v", got, plan)
	}
	if got := operationPlanStepStatusForTest(plan, postReadStepID); got != operationPlanStepStatusPending {
		t.Fatalf("post read status after update = %q, want pending; plan=%#v", got, plan)
	}
	if got := stringFromAny(plan["pending_next_action"]); !strings.Contains(got, "agent-management/get_agent_config") {
		t.Fatalf("pending_next_action = %q, want post-update get_agent_config; plan=%#v", got, plan)
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{
		{
			"kind":       "tool_call",
			"status":     "success",
			"skill_id":   skills.SkillAgentManagement,
			"tool_name":  "get_agent_config",
			"runtime_id": "tool_call:agent-management:get_agent_config::#1",
			"result": map[string]interface{}{
				"status":     "success",
				"agent_id":   "agent-1",
				"agent_name": "Support Agent",
			},
		},
		{
			"kind":       "tool_call",
			"status":     "success",
			"skill_id":   skills.SkillAgentManagement,
			"tool_name":  "list_agent_skill_candidates",
			"runtime_id": "tool_call:agent-management:list_agent_skill_candidates::#1",
			"result": map[string]interface{}{
				"status": "success",
				"items": []interface{}{
					map[string]interface{}{"id": "chart-generator", "name": "\u56fe\u8868\u751f\u6210\u5668"},
				},
			},
		},
		{
			"kind":       "tool_call",
			"status":     "success",
			"skill_id":   skills.SkillAgentManagement,
			"tool_name":  "update_agent_config",
			"runtime_id": "tool_call:agent-management:update_agent_config::#2",
			"result": map[string]interface{}{
				"status":              "completed",
				"agent_id":            "agent-1",
				"agent_name":          "Support Agent",
				"updated_fields":      []interface{}{"enabled_skill_ids"},
				"resource_names":      []interface{}{"\u56fe\u8868\u751f\u6210\u5668"},
				"change_action":       "bind",
				"resource_count":      1,
				"binding_kind":        "agent_skill",
				"enabled_skill_count": 1,
			},
		},
	})
	plan = metadata["operation_plan"].(map[string]interface{})
	if got := operationPlanStepStatusForTest(plan, postReadStepID); got != operationPlanStepStatusPending {
		t.Fatalf("post read status after replaying old read = %q, want pending; plan=%#v", got, plan)
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{
		{
			"kind":       "tool_call",
			"status":     "success",
			"skill_id":   skills.SkillAgentManagement,
			"tool_name":  "get_agent_config",
			"runtime_id": "tool_call:agent-management:get_agent_config::#2",
			"result": map[string]interface{}{
				"status":            "success",
				"agent_id":          "agent-1",
				"agent_name":        "Support Agent",
				"enabled_skill_ids": []interface{}{"chart-generator"},
			},
		},
		{
			"kind":      "client_action",
			"status":    "succeeded",
			"skill_id":  skills.SkillAgentManagement,
			"tool_name": "observe_agent_config",
			"result": map[string]interface{}{
				"observed_path": "/console/agents/agent-1/agent",
			},
		},
	})
	plan = metadata["operation_plan"].(map[string]interface{})
	if got := operationPlanStepStatusForTest(plan, postReadStepID); got != operationPlanStepStatusCompleted {
		t.Fatalf("post read status = %q, want completed; plan=%#v", got, plan)
	}
	if got := stringFromAny(plan["status"]); got != operationPlanStatusCompleted {
		t.Fatalf("operation_plan status = %q, want completed; plan=%#v", got, plan)
	}
}

func TestAgentResourceBindingUpdateConfigPostReadClosesPlan(t *testing.T) {
	tests := []struct {
		name         string
		query        string
		action       string
		resultFields []interface{}
	}{
		{
			name: "bind knowledge database and workflow",
			query: "Bind the first available knowledge base, database table, and workflow to the current agent. " +
				"Do not change name, description, icon, model, system prompt, or opening questions. " +
				"After completion read the agent config again and answer only after verification.",
			action:       "bind",
			resultFields: []interface{}{"knowledge_dataset_ids", "database_bindings", "workflow_bindings"},
		},
		{
			name: "unbind knowledge database and workflow",
			query: "Unbind the current knowledge base, database table, and workflow from the current agent. " +
				"Do not change name, description, icon, model, system prompt, or opening questions. " +
				"After completion read the agent config again and answer only after verification.",
			action:       "unbind",
			resultFields: []interface{}{"knowledge_dataset_ids", "database_bindings", "workflow_bindings"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := &chatRequestParts{
				Query:          tt.query,
				Surface:        aiChatSurfaceContextualSidebar,
				RuntimeContext: "route=/console/agents/agent-1/agent",
				SkillMode:      skillModeAuto,
				SkillIDs: []string{
					skills.SkillAgentManagement,
					skills.SkillConsoleNavigator,
				},
			}

			metadata := streamingMessageMetadataWithTaskID(parts, "task-agent-resource-bindings")
			plan := metadata["operation_plan"].(map[string]interface{})
			updateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")
			postReadStepID := operationPlanPostUpdateAgentConfigReadStepID()

			if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_identity")); got != "" {
				t.Fatalf("update_agent_identity step status = %q, want absent; plan=%#v", got, plan)
			}
			wantPendingTools := []string{
				"get_agent_config",
				"update_agent_config",
			}
			candidateTools := []string{
				"list_agent_knowledge_candidates",
				"list_agent_database_candidates",
				"list_agent_database_tables",
				"list_agent_workflow_binding_candidates",
			}
			if tt.action == "bind" {
				wantPendingTools = append(wantPendingTools, candidateTools...)
			}
			for _, want := range wantPendingTools {
				if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, want)); got != operationPlanStepStatusPending {
					t.Fatalf("%s status = %q, want pending; plan=%#v", want, got, plan)
				}
			}
			if tt.action == "unbind" {
				for _, unexpected := range candidateTools {
					if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, unexpected)); got != "" {
						t.Fatalf("%s status = %q, want absent for pure unbind; plan=%#v", unexpected, got, plan)
					}
				}
			}
			if got := operationPlanStepStatusForTest(plan, postReadStepID); got != operationPlanStepStatusPending {
				t.Fatalf("post-read status = %q, want pending; plan=%#v", got, plan)
			}

			invocations := []map[string]interface{}{
				agentManagementToolInvocationForTest("get_agent_config", "tool_call:agent-management:get_agent_config::#1", map[string]interface{}{
					"status":            "success",
					"agent_id":          "agent-1",
					"agent_name":        "Support Agent",
					"database_bindings": []interface{}{},
					"workflow_bindings": []interface{}{},
				}),
			}
			if tt.action == "bind" {
				invocations = append(invocations,
					agentManagementToolInvocationForTest("list_agent_knowledge_candidates", "tool_call:agent-management:list_agent_knowledge_candidates::#1", map[string]interface{}{
						"status": "success",
						"items":  []interface{}{map[string]interface{}{"id": "kb-1", "name": "Support KB"}},
					}),
					agentManagementToolInvocationForTest("list_agent_database_candidates", "tool_call:agent-management:list_agent_database_candidates::#1", map[string]interface{}{
						"status": "success",
						"items":  []interface{}{map[string]interface{}{"id": "db-1", "name": "Support DB"}},
					}),
					agentManagementToolInvocationForTest("list_agent_database_tables", "tool_call:agent-management:list_agent_database_tables::#1", map[string]interface{}{
						"status": "success",
						"items":  []interface{}{map[string]interface{}{"id": "table-1", "name": "customers"}},
					}),
					agentManagementToolInvocationForTest("list_agent_workflow_binding_candidates", "tool_call:agent-management:list_agent_workflow_binding_candidates::#1", map[string]interface{}{
						"status": "success",
						"items":  []interface{}{map[string]interface{}{"id": "workflow-1", "name": "Support Flow"}},
					}),
				)
			}
			invocations = append(invocations,
				agentManagementToolInvocationForTest("update_agent_config", "tool_call:agent-management:update_agent_config::#1", map[string]interface{}{
					"status":         "completed",
					"agent_id":       "agent-1",
					"agent_name":     "Support Agent",
					"updated_fields": tt.resultFields,
					"binding_changes": []interface{}{
						map[string]interface{}{"field": "knowledge_dataset_ids", "binding_kind": "knowledge_base", "change_action": tt.action, "resource_count": 1},
						map[string]interface{}{"field": "database_bindings", "binding_kind": "database_table", "change_action": tt.action, "resource_count": 1},
						map[string]interface{}{"field": "workflow_bindings", "binding_kind": "workflow", "change_action": tt.action, "resource_count": 1},
					},
				}),
			)
			applyOperationPlanInvocationState(metadata, invocations)
			plan = metadata["operation_plan"].(map[string]interface{})
			if got := operationPlanStepStatusForTest(plan, updateStepID); got != operationPlanStepStatusCompleted {
				t.Fatalf("update_agent_config status = %q, want completed; plan=%#v", got, plan)
			}
			if got := operationPlanStepStatusForTest(plan, postReadStepID); got != operationPlanStepStatusPending {
				t.Fatalf("post-read status after update = %q, want pending; plan=%#v", got, plan)
			}

			postReadResult := map[string]interface{}{
				"status":                "success",
				"agent_id":              "agent-1",
				"agent_name":            "Support Agent",
				"knowledge_dataset_ids": []interface{}{},
				"database_bindings":     []interface{}{},
				"workflow_bindings":     []interface{}{},
			}
			if tt.action == "bind" {
				postReadResult["knowledge_dataset_ids"] = []interface{}{"kb-1"}
				postReadResult["database_bindings"] = []interface{}{
					map[string]interface{}{"data_source_id": "db-1", "table_ids": []interface{}{"table-1"}},
				}
				postReadResult["workflow_bindings"] = []interface{}{map[string]interface{}{"binding_id": "workflow-1"}}
			}
			applyOperationPlanInvocationState(metadata, []map[string]interface{}{
				agentManagementToolInvocationForTest("get_agent_config", "tool_call:agent-management:get_agent_config::#2", map[string]interface{}{
					"status":                postReadResult["status"],
					"agent_id":              postReadResult["agent_id"],
					"agent_name":            postReadResult["agent_name"],
					"knowledge_dataset_ids": postReadResult["knowledge_dataset_ids"],
					"database_bindings":     postReadResult["database_bindings"],
					"workflow_bindings":     postReadResult["workflow_bindings"],
				}),
			})
			plan = metadata["operation_plan"].(map[string]interface{})
			if got := operationPlanStepStatusForTest(plan, postReadStepID); got != operationPlanStepStatusCompleted {
				t.Fatalf("post-read status = %q, want completed; plan=%#v", got, plan)
			}
			if got := stringFromAny(plan["status"]); got != operationPlanStatusCompleted {
				t.Fatalf("operation_plan status = %q, want completed; plan=%#v", got, plan)
			}
			if got := stringFromAny(plan["pending_next_action"]); got != "none" {
				t.Fatalf("pending_next_action = %q, want none; plan=%#v", got, plan)
			}
		})
	}
}

func agentManagementToolInvocationForTest(toolName string, runtimeID string, result map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"kind":       "tool_call",
		"status":     "success",
		"skill_id":   skills.SkillAgentManagement,
		"tool_name":  toolName,
		"runtime_id": runtimeID,
		"result":     result,
	}
}

func TestAgentBindingFinalAnswerGuardAcceptsUnifiedUpdateAgentConfig(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          "帮我把这个智能体绑定的数据库/知识库/工作流都解绑",
			Surface:        aiChatSurfaceContextualSidebar,
			RuntimeContext: "route=/console/agents/agent-1/agent",
			SkillMode:      skillModeAuto,
			SkillIDs: []string{
				skills.SkillAgentManagement,
				skills.SkillConsoleNavigator,
			},
			OperationContext: map[string]interface{}{
				"resources": []interface{}{
					map[string]interface{}{
						"agent_id": "agent-1",
						"type":     "agent",
						"id":       "agent-1",
						"name":     "Support Agent",
						"href":     "/console/agents/agent-1/agent",
						"selected": true,
					},
				},
			},
		},
	}
	guard := skillLoopAgentManagementFinalAnswerGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopAgentManagementFinalAnswerGuard() = nil, want guard")
	}

	blockedResult, blocked := guard(skillloop.FinalAnswerGuardRequest{
		Answer: "已经解绑完成。",
	})
	if !blocked {
		t.Fatal("guard without mutation = not blocked, want update_agent_config requirement")
	}
	if blockedResult.ToolName != "update_agent_config" {
		t.Fatalf("blocked tool = %q, want update_agent_config", blockedResult.ToolName)
	}

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "已经解绑完成。",
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{{
			SkillID:  skills.SkillAgentManagement,
			ToolName: "update_agent_config",
			Result: map[string]interface{}{
				"agent_id": "agent-1",
				"binding_changes": []interface{}{
					map[string]interface{}{
						"field":                  "knowledge_dataset_ids",
						"binding_kind":           "knowledge_base",
						"change_action":          "unbind",
						"removed_resource_count": 1,
					},
					map[string]interface{}{
						"field":                  "database_bindings",
						"binding_kind":           "database_table",
						"change_action":          "unbind",
						"removed_resource_count": 1,
					},
					map[string]interface{}{
						"field":                  "workflow_bindings",
						"binding_kind":           "workflow",
						"change_action":          "unbind",
						"removed_resource_count": 1,
					},
				},
			},
		}},
	})
	if blocked {
		t.Fatal("guard blocked unified update_agent_config, want it accepted as binding mutation evidence")
	}
}

func TestAgentConfigReadOnlyQuestionPlansOnlyConfigRead(t *testing.T) {
	for _, query := range []string{
		"Tell me the current Agent model name and provider from its config. Do not modify any configuration.",
		"\u8bf7\u8bfb\u53d6\u5f53\u524d Agent \u914d\u7f6e\uff0c\u544a\u8bc9\u6211\u5f53\u524d\u4f7f\u7528\u7684\u6a21\u578b\u540d\u79f0\u548c provider\u3002\u53ea\u6839\u636e\u5de5\u5177\u8fd4\u56de\u503c\u56de\u7b54\uff0c\u4e0d\u8981\u4fee\u6539\u4efb\u4f55\u914d\u7f6e\u3002",
		"\u8bf7\u67e5\u770b\u5f53\u524d\u667a\u80fd\u4f53\u7ed1\u5b9a\u4e86\u54ea\u4e9b\u77e5\u8bc6\u5e93\u3001\u6570\u636e\u5e93\u8868\u3001\u5de5\u4f5c\u6d41\u548c Skill\uff0c\u53ea\u8bfb\u53d6\uff0c\u4e0d\u8981\u4fee\u6539\u4efb\u4f55\u914d\u7f6e\u3002",
		"\u8fd9\u4e2a\u667a\u80fd\u4f53\u542f\u7528\u4e86\u54ea\u4e9b Skill\uff1f\u53ea\u544a\u8bc9\u6211\uff0c\u4e0d\u8981\u4fee\u6539\u914d\u7f6e\u3002",
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
		for _, unexpected := range []string{"update_agent_identity", "list_available_models", "list_agent_skill_candidates", "list_agent_knowledge_candidates", "list_agent_database_candidates", "list_agent_workflow_binding_candidates", "update_agent_config"} {
			if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, unexpected) {
				t.Fatalf("PlannedTools = %#v, want no agent-management/%s for read-only query %q", strategy.PlannedTools, unexpected, query)
			}
		}

		metadata := streamingMessageMetadataWithTaskID(parts, "task-agent-config-readonly")
		plan := metadata["operation_plan"].(map[string]interface{})
		if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config")); got != operationPlanStepStatusPending {
			t.Fatalf("get_agent_config step status = %#v, want pending; query=%q plan=%#v", got, query, plan)
		}
		for _, unexpected := range []string{"update_agent_identity", "list_available_models", "list_agent_skill_candidates", "list_agent_knowledge_candidates", "list_agent_database_candidates", "list_agent_workflow_binding_candidates", "update_agent_config"} {
			if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, unexpected)); got != "" {
				t.Fatalf("%s step status = %#v, want absent; query=%q plan=%#v", unexpected, got, query, plan)
			}
		}
	}
}

func TestAgentManagementCapabilityQuestionUsesPageContextWithoutTools(t *testing.T) {
	query := "\u53ea\u8bfb\u9a8c\u8bc1\uff1a\u8bf7\u57fa\u4e8e\u5f53\u524d\u9875\u9762\u4e0a\u4e0b\u6587\u56de\u7b54\u5f53\u524d\u9875\u9762\u524d\u4e24\u4e2a\u53ef\u89c1\u667a\u80fd\u4f53\u540d\u79f0\uff0c\u5e76\u8bf4\u660e\u4f60\u4f5c\u4e3a\u4fa7\u680f\u52a9\u624b\u5f53\u524d\u80fd\u5e2e\u52a9\u6211\u505a\u54ea\u4e9b Agent \u7ba1\u7406\u64cd\u4f5c\u3002\u4e0d\u8981\u521b\u5efa\u3001\u4fee\u6539\u3001\u5220\u9664\u6216\u5bfc\u822a\u3002"
	parts := &chatRequestParts{
		Query:          query,
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents",
		SkillMode:      skillModeAuto,
		SkillIDs: []string{
			skills.SkillAgentManagement,
			skills.SkillConsoleNavigator,
		},
	}

	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want explanation strategy")
	}
	if strategy.Intent != "explain_agent_management_capabilities" {
		t.Fatalf("strategy.Intent = %q, want explain_agent_management_capabilities; strategy=%#v", strategy.Intent, strategy)
	}
	if len(strategy.PrimarySkills) > 0 || len(strategy.SupportingSkills) > 0 || len(strategy.PlannedTools) > 0 || strategy.RequiredNextTool != nil {
		t.Fatalf("strategy planned tool use = primary %#v supporting %#v tools %#v required %#v, want none", strategy.PrimarySkills, strategy.SupportingSkills, strategy.PlannedTools, strategy.RequiredNextTool)
	}
	message, ok := contextualAIChatTurnStrategyMessage(&PreparedChat{parts: parts})
	if !ok {
		t.Fatal("contextualAIChatTurnStrategyMessage() missing, want capability guidance")
	}
	messageContent, ok := message.Content.(string)
	if !ok {
		t.Fatalf("strategy message content type = %T, want string", message.Content)
	}
	for _, want := range []string{"model provider/model pair", "bind or unbind Agent skills", "knowledge bases", "database tables", "workflows"} {
		if !strings.Contains(messageContent, want) {
			t.Fatalf("strategy message missing %q in:\n%s", want, messageContent)
		}
	}
	metadata := streamingMessageMetadataWithTaskID(parts, "task-agent-capability-explanation")
	plan := mapFromOperationContext(metadata["operation_plan"])
	if len(plan) == 0 {
		t.Fatal("operation_plan absent, want explanation-only strategy state")
	}
	if plan["intent"] != "explain_agent_management_capabilities" {
		t.Fatalf("operation_plan intent = %#v, want explain_agent_management_capabilities; plan=%#v", plan["intent"], plan)
	}
	if plan["risk_level"] != "low" || plan["approval_required"] != false {
		t.Fatalf("operation_plan risk/approval = %#v/%#v, want low/no approval; plan=%#v", plan["risk_level"], plan["approval_required"], plan)
	}
	for _, unexpected := range []string{"list_agents", "get_agent_config", "list_available_models", "list_agent_skill_candidates", "list_agent_knowledge_candidates", "list_agent_database_candidates", "list_agent_database_tables", "list_agent_workflow_binding_candidates", "update_agent_config"} {
		if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, unexpected)); got != "" {
			t.Fatalf("%s step status = %#v, want absent for explanation-only capability question; plan=%#v", unexpected, got, plan)
		}
	}
}

func TestAgentManagementEditPromptWithCapabilityTextDoesNotBecomeExplanation(t *testing.T) {
	query := "Edit current Agent GOAL-CONFIG: first list available models, choose DeepSeek-Chat (V3) and set provider to deepseek; rename it to GOAL-CONFIG-EDITED; update description and icon; set system prompt; set opening questions to introduce yourself, what can you do, and reset test data; update home title; then re-read config."
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
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want Agent-management strategy")
	}
	if strategy.Intent == "explain_agent_management_capabilities" {
		t.Fatalf("strategy.Intent = %q, want Agent edit intent; strategy=%#v", strategy.Intent, strategy)
	}
	if strategy.Intent != "manage_agent_asset" {
		t.Fatalf("strategy.Intent = %q, want manage_agent_asset; strategy=%#v", strategy.Intent, strategy)
	}
	for _, want := range []string{"list_available_models", "update_agent_identity", "update_agent_config"} {
		if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, want) {
			t.Fatalf("PlannedTools = %#v, missing agent-management/%s", strategy.PlannedTools, want)
		}
	}

	metadata := streamingMessageMetadataWithTaskID(parts, "task-agent-config-edit-with-capability-text")
	plan := mapFromOperationContext(metadata["operation_plan"])
	if len(plan) == 0 {
		t.Fatal("operation_plan absent, want Agent edit strategy state")
	}
	if plan["intent"] != "manage_agent_asset" {
		t.Fatalf("operation_plan intent = %#v, want manage_agent_asset; plan=%#v", plan["intent"], plan)
	}
	for _, want := range []string{"list_available_models", "update_agent_identity", "update_agent_config"} {
		if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, want)); got != operationPlanStepStatusPending {
			t.Fatalf("%s step status = %#v, want pending; plan=%#v", want, got, plan)
		}
	}
}

func TestSkillLoopPlanToolGuardBlocksAgentManagementForCapabilityExplanation(t *testing.T) {
	query := "\u53ea\u8bfb\u9a8c\u8bc1\uff1a\u8bf4\u660e\u4f60\u4f5c\u4e3a\u4fa7\u680f\u52a9\u624b\u5f53\u524d\u80fd\u5e2e\u52a9\u6211\u505a\u54ea\u4e9b Agent \u7ba1\u7406\u64cd\u4f5c\u3002\u4e0d\u8981\u521b\u5efa\u3001\u4fee\u6539\u3001\u5220\u9664\u6216\u5bfc\u822a\u3002"
	parts := &chatRequestParts{
		Query:          query,
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents",
		SkillMode:      skillModeAuto,
		SkillIDs: []string{
			skills.SkillAgentManagement,
			skills.SkillConsoleNavigator,
		},
	}
	prepared := &PreparedChat{
		parts: parts,
		Message: &runtimemodel.Message{
			Metadata: streamingMessageMetadataWithTaskID(parts, "task-agent-capability-explanation"),
		},
	}
	guard := skillLoopPlanToolCallGuardWithResolved(prepared, nil)
	result, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillAgentManagement})
	if !blocked {
		t.Fatal("load agent-management was allowed, want blocked with direct-answer guidance")
	}
	if !strings.Contains(result.SystemMessage, "Answer directly from the injected current page context") {
		t.Fatalf("guard system message = %q, want direct page-context answer guidance", result.SystemMessage)
	}
	result, blocked = guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillAgentManagement, ToolName: "list_agents"})
	if !blocked {
		t.Fatal("list_agents was allowed, want blocked for capability explanation")
	}
	if !strings.Contains(result.Message, "not needed") {
		t.Fatalf("guard message = %q, want not-needed feedback", result.Message)
	}
}

func TestAgentIdentityOnlyEditDoesNotPlanConfigToolsFromNegatedExclusions(t *testing.T) {
	query := "\u53ea\u4fee\u6539\u5f53\u524d\u667a\u80fd\u4f53\u63cf\u8ff0\u4e3a AIChat smoke description\uff0c\u4e0d\u6539\u540d\u79f0\u3001\u6a21\u578b\u3001Skill\u3001\u77e5\u8bc6\u5e93\u3001\u6570\u636e\u5e93\u3001\u5de5\u4f5c\u6d41\u6216\u5176\u4ed6\u914d\u7f6e\u3002"
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
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	if strategy.Intent != "manage_agent_asset" {
		t.Fatalf("strategy.Intent = %q, want manage_agent_asset; strategy=%#v", strategy.Intent, strategy)
	}
	if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "update_agent_identity") {
		t.Fatalf("PlannedTools = %#v, missing agent-management/update_agent_identity", strategy.PlannedTools)
	}
	for _, unexpected := range []string{
		"get_agent_config",
		"list_available_models",
		"list_agent_skill_candidates",
		"list_agent_knowledge_candidates",
		"list_agent_database_candidates",
		"list_agent_database_tables",
		"list_agent_workflow_binding_candidates",
		"update_agent_config",
	} {
		if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, unexpected) {
			t.Fatalf("PlannedTools = %#v, want no agent-management/%s for identity-only query", strategy.PlannedTools, unexpected)
		}
	}

	metadata := streamingMessageMetadataWithTaskID(parts, "task-agent-identity-only")
	plan := metadata["operation_plan"].(map[string]interface{})
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_identity")); got != operationPlanStepStatusPending {
		t.Fatalf("update_agent_identity step status = %#v, want pending; plan=%#v", got, plan)
	}
	for _, unexpected := range []string{
		"get_agent_config",
		"list_available_models",
		"list_agent_skill_candidates",
		"list_agent_knowledge_candidates",
		"list_agent_database_candidates",
		"list_agent_database_tables",
		"list_agent_workflow_binding_candidates",
		"update_agent_config",
	} {
		if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, unexpected)); got != "" {
			t.Fatalf("%s step status = %#v, want absent; plan=%#v", unexpected, got, plan)
		}
	}
}

func TestAgentIdentityEditPlansPostUpdateReadWhenUserRequestsObservation(t *testing.T) {
	query := "\u8bf7\u4fee\u6539\u5f53\u524d\u9875\u9762\u8fd9\u4e2a\u667a\u80fd\u4f53\u7684\u57fa\u7840\u4fe1\u606f\uff1a\u540d\u79f0\u6539\u4e3a Support Agent\uff0c\u63cf\u8ff0\u6539\u4e3a smoke\uff0c\u56fe\u6807\u6539\u4e3a\u7d2b\u8272\u661f\u661f\u3002\u5b8c\u6210\u540e\u786e\u8ba4\u9875\u9762\u9876\u90e8\u5df2\u66f4\u65b0\u3002"
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
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "update_agent_identity") {
		t.Fatalf("PlannedTools = %#v, missing agent-management/update_agent_identity", strategy.PlannedTools)
	}
	if !aiChatTurnStrategyHasPlannedToolStepIDForTest(strategy, operationPlanPostUpdateAgentIdentityReadStepID()) {
		t.Fatalf("PlannedTools = %#v, missing post-update get_agent step", strategy.PlannedTools)
	}

	metadata := streamingMessageMetadataWithTaskID(parts, "task-agent-identity-post-read")
	plan := metadata["operation_plan"].(map[string]interface{})
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_identity")); got != operationPlanStepStatusPending {
		t.Fatalf("update_agent_identity step status = %#v, want pending; plan=%#v", got, plan)
	}
	if got := operationPlanStepStatusForTest(plan, operationPlanPostUpdateAgentIdentityReadStepID()); got != operationPlanStepStatusPending {
		t.Fatalf("post-update get_agent step status = %#v, want pending; plan=%#v", got, plan)
	}
	if got := operationPlanStepFieldForTest(plan, operationPlanPostUpdateAgentIdentityReadStepID(), "wait_for"); got != operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_identity") {
		t.Fatalf("post-update get_agent wait_for = %q, want update_agent_identity; plan=%#v", got, plan)
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{
		{
			"kind":       "tool_call",
			"status":     "success",
			"skill_id":   skills.SkillAgentManagement,
			"tool_name":  "update_agent_identity",
			"runtime_id": "tool_call:agent-management:update_agent_identity::#1",
			"result": map[string]interface{}{
				"status":         "completed",
				"agent_id":       "agent-1",
				"agent_name":     "Support Agent",
				"updated_fields": []interface{}{"name", "description", "icon"},
			},
		},
		{
			"kind":       "tool_call",
			"status":     "success",
			"skill_id":   skills.SkillAgentManagement,
			"tool_name":  "get_agent",
			"runtime_id": "tool_call:agent-management:get_agent::#1",
			"result": map[string]interface{}{
				"status":     "success",
				"agent_id":   "agent-1",
				"agent_name": "Support Agent",
			},
		},
	})
	plan = metadata["operation_plan"].(map[string]interface{})
	if got := operationPlanStepStatusForTest(plan, operationPlanPostUpdateAgentIdentityReadStepID()); got != operationPlanStepStatusCompleted {
		t.Fatalf("post-update get_agent status = %#v, want completed after identity update and reread; plan=%#v", got, plan)
	}
	if got := stringFromAny(plan["pending_next_action"]); got != "" && got != "none" {
		t.Fatalf("pending_next_action = %q, want none after identity update and get_agent reread; plan=%#v", got, plan)
	}
	criteriaText := strings.Join(stringSliceFromAny(plan["completion_criteria"]), "\n")
	if strings.Contains(criteriaText, "Asset-changing step must have matching execution evidence before claiming completion: Run tool:agent-management/get_agent") {
		t.Fatalf("completion_criteria = %q, want get_agent described as verification read, not asset-changing", criteriaText)
	}
	if !strings.Contains(criteriaText, "Verification read step must have matching execution evidence") {
		t.Fatalf("completion_criteria = %q, want verification read evidence criterion", criteriaText)
	}
}

func TestAgentIdentityPostUpdateReadKeepsConsoleNavigatorAvailable(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "\u628a\u5f53\u524d\u667a\u80fd\u4f53\u7684\u540d\u79f0\u6539\u4e3a Support Agent\uff0c\u63cf\u8ff0\u6539\u4e3a smoke\u3002\u5b8c\u6210\u540e\u786e\u8ba4\u9875\u9762\u9876\u90e8\u5df2\u66f4\u65b0\u3002",
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents/agent-1/agent",
		SkillMode:      skillModeAuto,
		SkillIDs: []string{
			skills.SkillAgentManagement,
			skills.SkillConsoleNavigator,
		},
	}
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{
		{Metadata: skills.SkillMetadata{ID: skills.SkillAgentManagement}},
		{Metadata: skills.SkillMetadata{ID: skills.SkillConsoleNavigator}},
	}}

	filtered := restrictResolvedSkillsForTurnStrategy(parts, resolved)
	got := filtered.SkillIDs()
	want := []string{skills.SkillAgentManagement, skills.SkillConsoleNavigator}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered skill ids = %#v, want Agent management plus console navigator for post-update page confirmation %#v", got, want)
	}
}

func TestAgentModelOnlyEditDoesNotPlanIdentityFromNegatedExclusions(t *testing.T) {
	query := "\u8bf7\u628a\u5f53\u524d\u667a\u80fd\u4f53\u7684\u6a21\u578b\u5207\u6362\u4e3a DeepSeek-V4-Flash\u3002\u5fc5\u987b\u5148\u4f7f\u7528\u53ef\u7528\u6a21\u578b\u5217\u8868\u4e2d\u7684 provider/model \u540c\u6b65\u66f4\u65b0\uff1b\u53ea\u6539\u6a21\u578b\u914d\u7f6e\uff0c\u4e0d\u6539\u540d\u79f0\u3001\u63cf\u8ff0\u3001Skill\u3001\u77e5\u8bc6\u5e93\u3001\u6570\u636e\u5e93\u3001\u5de5\u4f5c\u6d41\u3002"
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
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	if strategy.Intent != "manage_agent_asset" {
		t.Fatalf("strategy.Intent = %q, want manage_agent_asset; strategy=%#v", strategy.Intent, strategy)
	}
	for _, want := range []string{"get_agent_config", "list_available_models", "update_agent_config"} {
		if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, want) {
			t.Fatalf("PlannedTools = %#v, missing agent-management/%s", strategy.PlannedTools, want)
		}
	}
	if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "update_agent_identity") {
		t.Fatalf("PlannedTools = %#v, want no agent-management/update_agent_identity for model-only query", strategy.PlannedTools)
	}

	metadata := streamingMessageMetadataWithTaskID(parts, "task-agent-model-only")
	plan := metadata["operation_plan"].(map[string]interface{})
	for _, want := range []string{"get_agent_config", "list_available_models", "update_agent_config"} {
		if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, want)); got != operationPlanStepStatusPending {
			t.Fatalf("%s step status = %#v, want pending; plan=%#v", want, got, plan)
		}
	}
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_identity")); got != "" {
		t.Fatalf("update_agent_identity step status = %#v, want absent; plan=%#v", got, plan)
	}
}

func TestAgentModelOnlyEditDoesNotPlanIdentityFromRealSmokePrompt(t *testing.T) {
	query := "冒烟复测-model-provider-1782872182423：请把当前页面这个智能体的模型切换为可用模型列表里的 DeepSeek-Chat (V3)；如果列表里没有这个模型，就选择第一个 use_case=text-chat 的可用模型。必须先查询可用模型列表，并用同一个列表项里的 provider 和 model 成对更新。只修改模型/provider，不要修改名称、描述、图标、提示词、知识库、数据库、工作流或 Skill。完成后重新读取配置验证，并最终回答实际设置的 provider/model。"
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
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	for _, want := range []string{"get_agent_config", "list_available_models", "update_agent_config"} {
		if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, want) {
			t.Fatalf("PlannedTools = %#v, missing agent-management/%s", strategy.PlannedTools, want)
		}
	}
	if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "update_agent_identity") {
		t.Fatalf("PlannedTools = %#v, want no agent-management/update_agent_identity for model-only smoke prompt; identity=%v name=%v description=%v icon=%v secondary=%q",
			strategy.PlannedTools,
			agentManagementIdentityUpdateRequested(query),
			containsPositiveAgentManagementResourceMarker(query, []string{"name", "\u540d\u79f0", "\u540d\u5b57"}),
			containsPositiveAgentManagementResourceMarker(query, []string{"description", "\u63cf\u8ff0"}),
			containsPositiveAgentManagementResourceMarker(query, []string{"icon", "\u56fe\u6807"}),
			agentManagementSecondaryIntentQuery(query),
		)
	}

	metadata := streamingMessageMetadataWithTaskID(parts, "task-agent-model-only-real-smoke")
	plan := metadata["operation_plan"].(map[string]interface{})
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_identity")); got != "" {
		t.Fatalf("update_agent_identity step status = %#v, want absent; plan=%#v", got, plan)
	}
}

func TestAgentBroadEditableSmokePromptPlansCurrentAgentEditLoop(t *testing.T) {
	query := "修改这个智能体所有你能修改的地方，本轮作为侧栏能力的冒烟测试，请尽可能进行操作。"
	if !agentManagementBroadEditableConfigRequested(query) {
		t.Fatalf("agentManagementBroadEditableConfigRequested(%q) = false, want true", query)
	}
	if !agentManagementIdentityUpdateRequested(query) {
		t.Fatalf("agentManagementIdentityUpdateRequested(%q) = false, want true", query)
	}
	if !agentManagementConfigUpdateRequested(query) {
		t.Fatalf("agentManagementConfigUpdateRequested(%q) = false, want true", query)
	}

	parts := consoleAgentDetailTestParts(query)
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	if strategy.RouteRequired {
		t.Fatalf("strategy.RouteRequired = true, want false on current Agent detail page; strategy=%#v", strategy)
	}
	for _, want := range []string{"get_agent_config", "update_agent_identity", "update_agent_config"} {
		if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, want) {
			t.Fatalf("PlannedTools = %#v, missing agent-management/%s", strategy.PlannedTools, want)
		}
		args := aiChatTurnStrategyPlannedToolArgumentsForTest(strategy, skills.SkillAgentManagement, want)
		if args["agent_id"] != "agent-1" {
			t.Fatalf("PlannedTools = %#v, %s args agent_id = %q, want agent-1", strategy.PlannedTools, want, args["agent_id"])
		}
	}
	for _, unexpected := range []struct {
		skillID  string
		toolName string
	}{
		{skills.SkillAgentManagement, "list_agents"},
		{skills.SkillConsoleNavigator, "navigate"},
	} {
		if aiChatTurnStrategyHasPlannedToolForTest(strategy, unexpected.skillID, unexpected.toolName) {
			t.Fatalf("PlannedTools = %#v, want no %s/%s for current Agent broad edit smoke", strategy.PlannedTools, unexpected.skillID, unexpected.toolName)
		}
	}

	plan := operationPlanFromTurnStrategy("task-agent-broad-edit", parts, strategy)
	if plan == nil {
		t.Fatal("operationPlanFromTurnStrategy() = nil, want plan")
	}
	target := mapFromOperationContext(plan["asset_target"])
	if target["page"] != "/console/agents/agent-1/agent" {
		t.Fatalf("asset_target = %#v, want current Agent detail page", target)
	}
	if actions := stringSliceFromAny(plan["approval_actions"]); !stringSliceContainsFold(actions, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")) {
		t.Fatalf("approval_actions = %#v, want update_agent_config approval", actions)
	}
}

func TestAgentBroadEditableSmokePromptHonorsNoModifyNegation(t *testing.T) {
	query := "查看这个智能体所有你能修改的地方，但不要修改任何配置。"
	if agentManagementBroadEditableConfigRequested(query) {
		t.Fatalf("agentManagementBroadEditableConfigRequested(%q) = true, want false", query)
	}
	if agentManagementConfigUpdateRequested(query) {
		t.Fatalf("agentManagementConfigUpdateRequested(%q) = true, want false", query)
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

func TestOperationPlanFromTurnStrategyInitializesStrategyState(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "Update the current Agent model and verify the saved configuration.",
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents/agent-1/agent",
		SkillMode:      skillModeAuto,
		SkillIDs: []string{
			skills.SkillAgentManagement,
			skills.SkillConsoleNavigator,
		},
	}
	strategy := &AIChatTurnStrategy{
		Surface:         aiChatSurfaceContextualSidebar,
		CurrentPage:     "/console/agents/agent-1/agent",
		Intent:          "manage_agent_asset",
		TargetPage:      "/console/agents/agent-1/agent",
		AssetEffect:     "update",
		AssetRisk:       "medium",
		Approval:        "agent-management mutations are governed",
		SuccessCriteria: []string{"update_agent_config succeeds", "post-update config read confirms saved model"},
		PlannedTools: []AIChatTurnStrategyTool{
			{
				SkillID:   skills.SkillAgentManagement,
				ToolName:  "get_agent_config",
				Arguments: map[string]string{"agent_id": "agent-1"},
			},
			{
				SkillID:  skills.SkillAgentManagement,
				ToolName: "update_agent_config",
				Arguments: map[string]string{
					"agent_id":                            "agent-1",
					operationPlanExpectedUpdatedFieldsKey: "model",
				},
			},
		},
		ObservationPoints: []string{"page shows updated model"},
	}

	plan := operationPlanFromTurnStrategy("task-strategy-state", parts, strategy)
	if len(plan) == 0 {
		t.Fatal("operationPlanFromTurnStrategy() = nil, want plan")
	}
	state := mapFromOperationContext(plan["strategy_state"])
	if got := state["schema_version"]; got != "operation_plan.strategy_state.v1" {
		t.Fatalf("strategy_state.schema_version = %#v, want operation_plan.strategy_state.v1; state=%#v", got, state)
	}
	if got := state["user_goal"]; got != parts.Query {
		t.Fatalf("strategy_state.user_goal = %#v, want original query; state=%#v", got, state)
	}
	if got := state["intent"]; got != "manage_agent_asset" {
		t.Fatalf("strategy_state.intent = %#v, want manage_agent_asset; state=%#v", got, state)
	}
	if got := state["risk_level"]; got != "medium" {
		t.Fatalf("strategy_state.risk_level = %#v, want medium; state=%#v", got, state)
	}
	if got := state["approval_required"]; got != true {
		t.Fatalf("strategy_state.approval_required = %#v, want true; state=%#v", got, state)
	}
	assertStringSliceContains(t, stringSliceFromAny(state["approval_actions"]), operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"))
	assertStringSliceContains(t, stringSliceFromAny(state["success_criteria"]), "post-update config read confirms saved model")
	if target := mapFromOperationContext(state["target_resource"]); target["effect"] != "update" || target["page"] != "/console/agents/agent-1/agent" {
		t.Fatalf("strategy_state.target_resource = %#v, want update Agent page target", target)
	}
	if evidence := mapFromOperationContext(state["current_page_evidence"]); evidence["current_page"] != "/console/agents/agent-1/agent" {
		t.Fatalf("strategy_state.current_page_evidence = %#v, want current Agent page", evidence)
	}
	if steps := mapSliceFromAny(state["plan_steps"]); len(steps) < 2 {
		t.Fatalf("strategy_state.plan_steps = %#v, want planned read and update steps", state["plan_steps"])
	}
	if got := intValueFromAny(state["completed_step_count"]); got != 0 {
		t.Fatalf("strategy_state.completed_step_count = %#v, want 0; state=%#v", state["completed_step_count"], state)
	}
	if got := intValueFromAny(state["failed_step_count"]); got != 0 {
		t.Fatalf("strategy_state.failed_step_count = %#v, want 0; state=%#v", state["failed_step_count"], state)
	}
}

func TestSkillLoopCompletionPlanSummaryCarriesStrategyState(t *testing.T) {
	plan := map[string]interface{}{
		"status":              operationPlanStatusRunning,
		"intent":              "manage_agent_asset",
		"pending_next_action": "verify updated agent configuration",
		"original_user_goal":  "update the current agent model and knowledge binding",
		"risk_level":          "medium",
		"approval":            "agent-management mutations are governed",
		"approval_required":   true,
		"approval_actions": []interface{}{
			"tool:agent-management/update_agent_config",
		},
		"success_criteria": []interface{}{
			"update_agent_config succeeds",
			"page observation confirms the updated model and binding",
		},
		"completion_criteria": []interface{}{
			"final answer reports only observed configuration changes",
		},
		"asset_target": map[string]interface{}{
			"type": "agent",
			"name": "客服智能体",
		},
		"step_status": map[string]interface{}{
			"tool:agent-management/update_agent_config": operationPlanStepStatusCompleted,
			"observe": operationPlanStepStatusPending,
		},
		"page_evidence": map[string]interface{}{
			"current_page": "/console/agents/agent-1/agent",
			"resources": []interface{}{
				map[string]interface{}{
					"resource_type": "agent",
					"id":            "agent-1",
					"title":         "Support Agent",
				},
			},
		},
		"completed_steps": []interface{}{
			map[string]interface{}{
				"id":        "tool:agent-management/update_agent_config",
				"status":    operationPlanStepStatusCompleted,
				"title":     "update agent config",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
			},
		},
		"failed_steps": []interface{}{
			map[string]interface{}{
				"id":        "observe",
				"status":    operationPlanStepStatusFailed,
				"title":     "observe page evidence",
				"error":     "page observation timed out",
				"skill_id":  skills.SkillConsoleNavigator,
				"tool_name": "navigate",
			},
		},
		"asset_state": map[string]interface{}{
			"agent_name": "客服智能体",
		},
		"steps": []interface{}{
			map[string]interface{}{
				"id":        "tool:agent-management/update_agent_config",
				"title":     "update agent config",
				"status":    operationPlanStepStatusCompleted,
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
				"asset_target": map[string]interface{}{
					"type": "agent",
					"name": "客服智能体",
				},
			},
			map[string]interface{}{
				"id":     "observe",
				"title":  "observe page evidence",
				"status": operationPlanStepStatusPending,
			},
		},
		"strategy_state": map[string]interface{}{
			"schema_version":          "operation_plan.strategy_state.v1",
			"user_goal":               "update the current agent model and knowledge binding",
			"pending_next_action":     "verify updated agent configuration",
			"plan_deviation_count":    1,
			"blocked_deviation_count": 0,
			"last_plan_deviation": map[string]interface{}{
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "list_agent_knowledge_candidates",
				"reason":    "model_collected_additional_evidence_within_user_goal",
				"outcome":   "allowed",
			},
		},
	}

	summary := skillLoopCompletionPlanSummary(plan)
	if got := summary["intent"]; got != "manage_agent_asset" {
		t.Fatalf("plan summary intent = %#v, want manage_agent_asset; summary=%#v", got, summary)
	}
	if got := summary["risk_level"]; got != "medium" {
		t.Fatalf("plan summary risk_level = %#v, want medium; summary=%#v", got, summary)
	}
	if got := summary["approval_required"]; got != true {
		t.Fatalf("plan summary approval_required = %#v, want true; summary=%#v", got, summary)
	}
	assertStringSliceContains(t, stringSliceFromAny(summary["approval_actions"]), "tool:agent-management/update_agent_config")
	assertStringSliceContains(t, stringSliceFromAny(summary["success_criteria"]), "page observation confirms the updated model and binding")
	assertStringSliceContains(t, stringSliceFromAny(summary["completion_criteria"]), "final answer reports only observed configuration changes")
	if target := mapFromOperationContext(summary["asset_target"]); target["name"] != "客服智能体" {
		t.Fatalf("plan summary asset_target = %#v, want target agent", target)
	}
	if stepStatus := mapFromOperationContext(summary["step_status"]); stepStatus["observe"] != operationPlanStepStatusPending {
		t.Fatalf("plan summary step_status = %#v, want observe pending", stepStatus)
	}
	if assetState := mapFromOperationContext(summary["asset_state"]); assetState["agent_name"] != "客服智能体" {
		t.Fatalf("plan summary asset_state = %#v, want current agent evidence", assetState)
	}
	if pageEvidence := mapFromOperationContext(summary["page_evidence"]); pageEvidence["current_page"] != "/console/agents/agent-1/agent" {
		t.Fatalf("plan summary page_evidence = %#v, want current page evidence", pageEvidence)
	}
	strategyState := mapFromOperationContext(summary["strategy_state"])
	if got := strategyState["user_goal"]; got != "update the current agent model and knowledge binding" {
		t.Fatalf("plan summary strategy_state.user_goal = %#v, want original goal; summary=%#v", got, summary)
	}
	if got := intValueFromAny(strategyState["plan_deviation_count"]); got != 1 {
		t.Fatalf("plan summary strategy_state.plan_deviation_count = %#v, want 1; state=%#v", strategyState["plan_deviation_count"], strategyState)
	}
	if deviation := mapFromOperationContext(strategyState["last_plan_deviation"]); deviation["tool_name"] != "list_agent_knowledge_candidates" {
		t.Fatalf("plan summary strategy_state.last_plan_deviation = %#v, want knowledge candidate deviation", deviation)
	}
	completedSteps := mapSliceFromAny(summary["completed_steps"])
	if len(completedSteps) != 1 || stringFromAny(completedSteps[0]["tool_name"]) != "update_agent_config" {
		t.Fatalf("plan summary completed_steps = %#v, want completed update step", summary["completed_steps"])
	}
	failedSteps := mapSliceFromAny(summary["failed_steps"])
	if len(failedSteps) != 1 || stringFromAny(failedSteps[0]["error"]) != "page observation timed out" {
		t.Fatalf("plan summary failed_steps = %#v, want failed observe error", summary["failed_steps"])
	}
	steps := mapSliceFromAny(summary["steps"])
	if len(steps) != 2 {
		t.Fatalf("plan summary steps = %#v, want two compact steps", summary["steps"])
	}
	if got := stringFromAny(steps[0]["tool_name"]); got != "update_agent_config" {
		t.Fatalf("plan summary first step tool_name = %q, want update_agent_config; steps=%#v", got, steps)
	}
}

func TestSkillLoopCompletionEvidenceRefreshesOperationPlanPageEvidence(t *testing.T) {
	parts := consoleAgentsVisibleTargetsTestParts("show me the current agent list")
	message := &runtimemodel.Message{
		Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status":       operationPlanStatusRunning,
				"current_page": "/console/files",
				"steps": []interface{}{map[string]interface{}{
					"id":     "observe",
					"status": operationPlanStepStatusPending,
				}},
				"step_status": map[string]interface{}{
					"observe": operationPlanStepStatusPending,
				},
			},
		},
	}
	prepared := &PreparedChat{
		Message: message,
		parts:   parts,
	}

	evidence := skillLoopCompletionEvidence(prepared)()
	plan := mapFromOperationContext(evidence["operation_plan"])
	if got := stringFromAny(plan["current_page"]); got != "/console/agents" {
		t.Fatalf("operation_plan.current_page = %q, want refreshed /console/agents; plan=%#v", got, plan)
	}
	pageEvidence := mapFromOperationContext(plan["page_evidence"])
	if got := stringFromAny(pageEvidence["current_page"]); got != "/console/agents" {
		t.Fatalf("operation_plan.page_evidence = %#v, want refreshed page evidence", pageEvidence)
	}
	resources := mapSliceFromAny(pageEvidence["resources"])
	if len(resources) < 2 || stringFromAny(resources[1]["title"]) != "Visible Agent One" {
		t.Fatalf("operation_plan.page_evidence.resources = %#v, want visible Agent evidence", pageEvidence["resources"])
	}
}

func TestSkillLoopCompletionPageContextEvidenceHonorsNoNavigationRequest(t *testing.T) {
	context := map[string]interface{}{
		"resources": []interface{}{
			map[string]interface{}{
				"resource_type": "page",
				"href":          "/console/agents/agent-1/agent",
			},
			map[string]interface{}{
				"resource_type": "agent",
				"id":            "agent-1",
				"title":         "Visible Agent One",
				"href":          "/console/agents/agent-1/agent",
				"selected":      true,
			},
		},
	}
	parts := &chatRequestParts{
		Query:               "\u8bf7\u4fee\u6539\u5f53\u524d\u667a\u80fd\u4f53\u7684\u63cf\u8ff0\uff0c\u4e0d\u8981\u5207\u6362\u5230\u5176\u4ed6\u9875\u9762",
		Surface:             aiChatSurfaceContextualSidebar,
		RuntimeContext:      "route=/console/agents/agent-1/agent",
		SkillMode:           skillModeAuto,
		SkillIDs:            []string{skills.SkillAgentManagement, skills.SkillConsoleNavigator},
		RawOperationContext: context,
		OperationContext:    context,
	}

	if target, ok := resolveConsoleNavigationTargetForParts(parts); ok {
		t.Fatalf("resolveConsoleNavigationTargetForParts() = %#v, want no target for explicit no-navigation request", target)
	}
	evidence := skillLoopCompletionPageContextEvidence(parts)
	if got := stringFromAny(evidence["current_page"]); got != "/console/agents/agent-1/agent" {
		t.Fatalf("current_page = %q, want current Agent detail route; evidence=%#v", got, evidence)
	}
	if target := mapFromOperationContext(evidence["resolved_target_from_user_request"]); len(target) > 0 {
		t.Fatalf("resolved_target_from_user_request = %#v, want omitted for explicit no-navigation request", target)
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

func TestSkillLoopPlanToolGuardBlocksSecondAgentIdentityMutationFromRuntimeSuccess(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "请修改当前智能体名称，完成后重新读取配置",
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillAgentManagement},
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
						"id":        operationPlanPostUpdateAgentConfigReadStepID(),
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "get_agent_config",
						"wait_for":  operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_identity"),
					},
				},
				"step_status": map[string]interface{}{
					operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_identity"): operationPlanStepStatusCompleted,
					operationPlanPostUpdateAgentConfigReadStepID():                                operationPlanStepStatusPending,
				},
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopPlanToolCallGuard() = nil, want guard")
	}
	successful := []skillloop.SkillToolCallRef{{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_identity",
		Arguments: map[string]interface{}{
			"name": "Agent One",
		},
		Result: map[string]interface{}{
			"status":         "completed",
			"updated_fields": []interface{}{"name"},
		},
	}}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:             skills.SkillAgentManagement,
		ToolName:            "update_agent_identity",
		SuccessfulToolCalls: successful,
		Arguments: map[string]interface{}{
			"name": "Agent Two",
		},
	}); !blocked {
		t.Fatal("second update_agent_identity was allowed after a successful identity update in the same turn")
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:             skills.SkillAgentManagement,
		ToolName:            "get_agent_config",
		SuccessfulToolCalls: successful,
	}); blocked {
		t.Fatal("get_agent_config was blocked, want post-update verification read allowed")
	}
}

func TestSkillLoopPlanToolGuardBlocksAgentIdentityUpdateAlreadyCoveredByCreate(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "create a browser smoke Agent named GOAL Browser Agent with icon test tube",
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
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopPlanToolCallGuard() = nil, want guard")
	}
	successfulCreate := []skillloop.SkillToolCallRef{{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "create_agent",
		Arguments: map[string]interface{}{
			"name":            "GOAL Browser Agent",
			"description":     "browser smoke create",
			"icon_type":       "text",
			"icon":            "TEST",
			"icon_background": "#0f766e",
		},
		Result: map[string]interface{}{
			"status":                "completed",
			"agent_id":              "agent-1",
			"agent_name":            "GOAL Browser Agent",
			"agent_description":     "browser smoke create",
			"agent_icon_type":       "text",
			"agent_icon":            "TEST",
			"agent_icon_background": "#0f766e",
		},
	}}

	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:             skills.SkillAgentManagement,
		ToolName:            "update_agent_identity",
		SuccessfulToolCalls: successfulCreate,
		Arguments: map[string]interface{}{
			"agent_id":        "agent-1",
			"name":            "GOAL Browser Agent",
			"description":     "browser smoke create",
			"icon_type":       "text",
			"icon":            "TEST",
			"icon_background": "#0f766e",
		},
	}); !blocked {
		t.Fatal("update_agent_identity repeated the create_agent identity fields, want blocked")
	}
}

func TestSkillLoopPlanToolGuardBlocksAgentIdentityUpdateRepeatingCreateGoalAfterContinuation(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "请在当前智能体列表中创建一个测试智能体，名称 GOAL-PLAN-CLOSURE-123，描述“Planner闭环冒烟创建 123”，图标用 🧭。创建成功后打开它的编辑详情页，并简短说明结果。",
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
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopPlanToolCallGuard() = nil, want guard")
	}
	result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_identity",
		Arguments: map[string]interface{}{
			"agent_id":    "agent-1",
			"name":        "GOAL-PLAN-CLOSURE-123",
			"description": "Planner闭环冒烟创建 123",
			"icon_type":   "text",
			"icon":        "🧭",
		},
	})
	if !blocked {
		t.Fatal("redundant update_agent_identity after completed create was allowed")
	}
	if !strings.Contains(result.SystemMessage, "create_agent step already covers") {
		t.Fatalf("SystemMessage = %q, want create_agent coverage guidance", result.SystemMessage)
	}
}

func TestSkillLoopPlanToolGuardBlocksRepeatedSingleAgentCreateAfterCompletedCreate(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "请在当前智能体列表中创建一个测试智能体，名称 GOAL-PLAN-CLOSURE-123，描述“Planner闭环冒烟创建 123”，图标用 🧭。创建成功后打开它的编辑详情页。",
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
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopPlanToolCallGuard() = nil, want guard")
	}
	result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "create_agent",
		Arguments: map[string]interface{}{
			"name":        "GOAL-PLAN-CLOSURE-123",
			"description": "Planner闭环冒烟创建 123",
			"icon_type":   "text",
			"icon":        "🧭",
		},
	})
	if !blocked {
		t.Fatal("repeated create_agent after completed single-Agent create was allowed")
	}
	if !strings.Contains(result.SystemMessage, "Do not call create_agent again") {
		t.Fatalf("SystemMessage = %q, want repeated create guidance", result.SystemMessage)
	}
}

func TestSkillLoopPlanToolGuardBlocksAgentConfigUpdateNotRequestedAfterCreate(t *testing.T) {
	query := "\u8bf7\u5728\u5f53\u524d\u667a\u80fd\u4f53\u5217\u8868\u4e2d\u521b\u5efa\u4e00\u4e2a\u6d4b\u8bd5\u667a\u80fd\u4f53\uff0c\u540d\u79f0 GOAL-CLOSURE-SMOKE-1\uff0c\u63cf\u8ff0\u201cPlanner\u95ed\u73af\u9a8c\u8bc1 1\u201d\uff0c\u56fe\u6807\u7528 \U0001f9ed\u3002\u521b\u5efa\u6210\u529f\u540e\u6253\u5f00\u5b83\u7684\u7f16\u8f91\u8be6\u60c5\u9875\uff0c\u5e76\u53ea\u57fa\u4e8e\u5de5\u5177\u8fd4\u56de\u503c\u548c\u9875\u9762\u8bc1\u636e\u7b80\u77ed\u8bf4\u660e\u7ed3\u679c\u3002"
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     query,
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
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"),
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "update_agent_config",
					},
				},
				"step_status": map[string]interface{}{
					operationPlanToolStepID(skills.SkillAgentManagement, "create_agent"):        operationPlanStepStatusCompleted,
					operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"): operationPlanStepStatusPending,
				},
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopPlanToolCallGuard() = nil, want guard")
	}
	result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Arguments: map[string]interface{}{
			"agent_id":            "agent-1",
			"suggested_questions": []interface{}{"\u914d\u7f6e\u662f\u5426\u5df2\u4fdd\u5b58\uff1f"},
		},
	})
	if !blocked {
		t.Fatal("unrequested update_agent_config after create was allowed")
	}
	if !strings.Contains(result.SystemMessage, "did not ask to change runtime configuration") {
		t.Fatalf("SystemMessage = %q, want create-only config guidance", result.SystemMessage)
	}
}

func TestSkillLoopPlanToolGuardAllowsAgentConfigUpdateAfterCreateWhenUserAskedPostCreateConfig(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "create an Agent named Agent One, then set its opening questions to hello and status",
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
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopPlanToolCallGuard() = nil, want guard")
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Arguments: map[string]interface{}{
			"agent_id":            "agent-1",
			"suggested_questions": []interface{}{"hello", "status"},
		},
	}); blocked {
		t.Fatal("explicit post-create config edit was blocked, want governance to handle it")
	}
}

func TestSkillLoopPlanToolGuardAllowsAgentIdentityUpdateAfterCreateWhenUserAskedPostCreateEdit(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "create an Agent named Agent One, then change its description to updated description",
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
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopPlanToolCallGuard() = nil, want guard")
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_identity",
		Arguments: map[string]interface{}{
			"agent_id":    "agent-1",
			"description": "updated description",
		},
	}); blocked {
		t.Fatal("explicit post-create identity edit was blocked, want governance to handle it")
	}
}

func TestSkillLoopPlanToolGuardAllowsAgentIdentityUpdateAfterCreateWhenFieldDiffers(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "create an Agent, then change its description",
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
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopPlanToolCallGuard() = nil, want guard")
	}
	successfulCreate := []skillloop.SkillToolCallRef{{
		SkillID:   skills.SkillAgentManagement,
		ToolName:  "create_agent",
		Arguments: map[string]interface{}{"name": "Agent One", "description": "draft"},
		Result: map[string]interface{}{
			"status":            "completed",
			"agent_id":          "agent-1",
			"agent_name":        "Agent One",
			"agent_description": "draft",
		},
	}}

	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:             skills.SkillAgentManagement,
		ToolName:            "update_agent_identity",
		SuccessfulToolCalls: successfulCreate,
		Arguments: map[string]interface{}{
			"agent_id":    "agent-1",
			"description": "updated description",
		},
	}); blocked {
		t.Fatal("update_agent_identity with a real post-create field change was blocked, want governance to handle it")
	}
}

func TestSkillLoopPlanToolGuardBlocksEmptyAgentIdentityUpdate(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "create an Agent and open its detail page",
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
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopPlanToolCallGuard() = nil, want guard")
	}
	result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_identity",
		Arguments: map[string]interface{}{
			"agent_id": "agent-1",
		},
	})
	if !blocked {
		t.Fatal("empty update_agent_identity was allowed, want blocked before governance")
	}
	if result.ToolName != "update_agent_identity" {
		t.Fatalf("guard result tool = %q, want update_agent_identity", result.ToolName)
	}
	if !strings.Contains(result.SystemMessage, "would not change the Agent") {
		t.Fatalf("guard system message = %q, want no-op guidance", result.SystemMessage)
	}
}

func TestSkillLoopPlanToolGuardBlocksUnrequestedAgentIconBackgroundUpdate(t *testing.T) {
	prepared := preparedAgentIdentityUpdateGuardTestChat("change the current Agent name to Smoke, description to edited, and icon to rocket")

	guard := skillLoopPlanToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopPlanToolCallGuard() = nil, want guard")
	}
	result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_identity",
		Arguments: map[string]interface{}{
			"agent_id":        "agent-1",
			"name":            "Smoke",
			"description":     "edited",
			"icon_type":       "text",
			"icon":            "🚀",
			"icon_background": "#0f766e",
		},
	})
	if !blocked {
		t.Fatal("update_agent_identity with unrequested icon_background was allowed")
	}
	if !strings.Contains(result.SystemMessage, "without icon_background") {
		t.Fatalf("guard system message = %q, want retry guidance without icon_background", result.SystemMessage)
	}
}

func TestSkillLoopPlanToolGuardAllowsRequestedAgentIconBackgroundUpdate(t *testing.T) {
	prepared := preparedAgentIdentityUpdateGuardTestChat("change the current Agent icon to rocket and icon background color to #0f766e")

	guard := skillLoopPlanToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopPlanToolCallGuard() = nil, want guard")
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_identity",
		Arguments: map[string]interface{}{
			"agent_id":        "agent-1",
			"icon_type":       "text",
			"icon":            "🚀",
			"icon_background": "#0f766e",
		},
	}); blocked {
		t.Fatal("requested icon_background update was blocked")
	}
}

func preparedAgentIdentityUpdateGuardTestChat(query string) *PreparedChat {
	return &PreparedChat{
		parts: &chatRequestParts{
			Query:     query,
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillAgentManagement},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status":             operationPlanStatusRunning,
				"original_user_goal": query,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_identity"),
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "update_agent_identity",
					},
				},
				"step_status": map[string]interface{}{
					operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_identity"): operationPlanStepStatusPending,
				},
			},
		}},
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
	if result, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillAgentManagement, ToolName: "list_available_models"}); blocked {
		t.Fatalf("list_available_models was blocked, want planned read evidence replay allowed; result=%#v", result)
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

func TestSkillLoopPlanToolGuardBlocksReadOnlyAgentConfigCandidateLookup(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "read-only check the current agent configuration: name, description, model/provider, and current bound resource counts; do not list candidates or modify config",
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
				"original_user_goal": "read-only check the current agent configuration: name, description, model/provider, and current bound resource counts; do not list candidates or modify config",
			},
		}},
	}

	goal := operationPlanAmendmentGoal(prepared)
	if !agentManagementExplicitReadOnlyConfigCheck(goal) {
		t.Fatalf("agentManagementExplicitReadOnlyConfigCheck(%q) = false, want true", goal)
	}
	if !agentManagementCandidateLookupExplicitlyNegated(goal) {
		t.Fatalf("agentManagementCandidateLookupExplicitlyNegated(%q) = false, want true", goal)
	}
	if !skillLoopShouldBlockReadOnlyAgentCandidateLookup(prepared, skills.SkillAgentManagement, "list_agent_database_tables") {
		t.Fatalf("skillLoopShouldBlockReadOnlyAgentCandidateLookup(%q) = false, want true", goal)
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if _, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillAgentManagement, ToolName: "get_agent_config"}); blocked {
		t.Fatal("get_agent_config was blocked, want the config read allowed")
	}
	result, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillAgentManagement, ToolName: "list_agent_database_tables"})
	if !blocked {
		t.Fatal("list_agent_database_tables was allowed, want read-only config guard to block candidate lookup")
	}
	if !strings.Contains(result.SystemMessage, "read-only current Agent configuration check") {
		t.Fatalf("guard system message = %q, want read-only config guidance", result.SystemMessage)
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillAgentManagement, ToolName: "list_available_models"}); !blocked {
		t.Fatal("list_available_models was allowed, want read-only config guard to block model candidate lookup")
	}
}

func TestSkillLoopPlanToolGuardAllowsExplicitAgentCandidateLookup(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "check the current agent configuration and list available bindable knowledge bases",
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
				"original_user_goal": "check the current agent configuration and list available bindable knowledge bases",
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if _, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillAgentManagement, ToolName: "list_agent_knowledge_candidates"}); blocked {
		t.Fatal("list_agent_knowledge_candidates was blocked, want explicit bindable resource query allowed")
	}
}

func TestSkillLoopPlanToolGuardAllowsExplicitWorkflowCandidateLookup(t *testing.T) {
	query := "\u53ea\u8bfb\u67e5\u8be2\uff0c\u4e0d\u8981\u4fee\u6539\u914d\u7f6e\u3002\u8bf7\u5217\u51fa\u5f53\u524d\u667a\u80fd\u4f53\u53ef\u7ed1\u5b9a\u7684\u77e5\u8bc6\u5e93\u3001\u6570\u636e\u5e93\u8868\u3001\u5de5\u4f5c\u6d41\u5404\u524d 3 \u4e2a\uff0c\u5e76\u8bf4\u660e\u5f53\u524d\u5df2\u7ed1\u5b9a\u6570\u91cf\u3002"
	if skillLoopToolNameLooksAssetMutation("list_agent_workflow_binding_candidates") {
		t.Fatal("list_agent_workflow_binding_candidates was classified as mutation, want read-only candidate lookup")
	}
	if !skillLoopToolNameLooksReadOnly("list_agent_workflow_binding_candidates") {
		t.Fatal("list_agent_workflow_binding_candidates was not classified as read-only")
	}
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     query,
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
				"original_user_goal": query,
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if result, blocked := guard(skillloop.ToolCallGuardRequest{SkillID: skills.SkillAgentManagement, ToolName: "list_agent_workflow_binding_candidates"}); blocked {
		t.Fatalf("list_agent_workflow_binding_candidates was blocked: %#v", result)
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
	state := mapFromOperationContext(plan["strategy_state"])
	if got := intValueFromAny(state["blocked_deviation_count"]); got != 1 {
		t.Fatalf("strategy_state.blocked_deviation_count = %#v, want 1; state=%#v", state["blocked_deviation_count"], state)
	}
	if latest := mapFromOperationContext(state["last_blocked_deviation"]); latest["tool_name"] != "update_agent_identity" {
		t.Fatalf("strategy_state.last_blocked_deviation = %#v, want update_agent_identity", latest)
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

func TestSkillLoopPlanToolGuardBlocksGovernedMutationForReadOnlyCandidateGoal(t *testing.T) {
	query := "\u56de\u5f52\u9a8c\u8bc1-\u53ea\u8bfb\u7ed1\u5b9a\u5019\u9009\uff1a\u8bf7\u53ea\u8bfb\u53d6\u5f53\u524d\u667a\u80fd\u4f53\u53ef\u7ed1\u5b9a\u77e5\u8bc6\u5e93\u3001\u6570\u636e\u5e93\u8868\u3001\u5de5\u4f5c\u6d41\u5019\u9009\uff1b\u4e0d\u8981\u4fee\u6539\u540d\u79f0\u3001\u63cf\u8ff0\u3001\u56fe\u6807\u3001\u6a21\u578b\u3001\u7cfb\u7edf\u63d0\u793a\u8bcd\u3001\u5f00\u573a\u95ee\u9898\uff0c\u4e5f\u4e0d\u8981\u7ed1\u5b9a\u6216\u89e3\u7ed1\u4efb\u4f55\u8d44\u6e90\u3002\u6700\u540e\u53ea\u6839\u636e\u5de5\u5177\u8fd4\u56de\u503c\u544a\u8bc9\u6211\u5019\u9009\u6570\u91cf\uff0c\u4e0d\u8981\u53d1\u8d77\u5ba1\u6279\u3002"
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     query,
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillAgentManagement},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status":             operationPlanStatusRunning,
				"original_user_goal": query,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillAgentManagement, "list_agent_knowledge_candidates"),
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "list_agent_knowledge_candidates",
					},
				},
				"step_status": map[string]interface{}{
					operationPlanToolStepID(skills.SkillAgentManagement, "list_agent_knowledge_candidates"): operationPlanStepStatusPending,
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

	guard := skillLoopPlanToolCallGuardWithResolved(prepared, resolved)
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillAgentManagement,
		ToolName:  "update_agent_config",
		Arguments: map[string]interface{}{"agent_id": "agent-1", "knowledge_dataset_ids": []interface{}{"kb-1"}},
	}); !blocked {
		t.Fatal("governed update_agent_config was allowed for explicit read-only candidate goal, want blocked")
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")); got != "" {
		t.Fatalf("update_agent_config step status = %q, want absent; plan=%#v", got, plan)
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

func TestSkillLoopPlanToolGuardAllowsRepeatedCreateAgentFromNamedTargets(t *testing.T) {
	query := "\u8bf7\u521b\u5efa\u4e34\u65f6\u667a\u80fd\u4f53\uff0c\u540d\u79f0\u5206\u522b\u4e3a Alpha \u548c Beta"
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     query,
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
				"original_user_goal": query,
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
		t.Fatal("second create_agent was blocked, want named target list to allow the missing target")
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	repeatStepID := operationPlanToolStepID(skills.SkillAgentManagement, "create_agent") + "#2"
	if got := operationPlanStepStatusForTest(plan, repeatStepID); got != operationPlanStepStatusPending {
		t.Fatalf("repeat create step status = %q, want pending; plan=%#v", got, plan)
	}
}

func TestApplyOperationPlanInvocationStateKeepsRepeatedCreatePendingAfterFirstCreate(t *testing.T) {
	createStepID := operationPlanToolStepID(skills.SkillAgentManagement, "create_agent")
	repeatStepID := createStepID + "#2"
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": operationPlanStatusRunning,
			"steps": []interface{}{
				map[string]interface{}{
					"id":                 createStepID,
					"status":             operationPlanStepStatusCompleted,
					"skill_id":           skills.SkillAgentManagement,
					"tool_name":          "create_agent",
					"last_invocation_id": "runtime_id:tool_governance:first-create",
				},
				map[string]interface{}{
					"id":        repeatStepID,
					"status":    operationPlanStepStatusPending,
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "create_agent",
					"repeat_of": createStepID,
				},
			},
			"step_status": map[string]interface{}{
				createStepID: operationPlanStepStatusCompleted,
				repeatStepID: operationPlanStepStatusPending,
			},
			"original_user_goal": "create two draft agents named Alpha and Beta",
		},
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{
		{
			"kind":       "tool_call",
			"skill_id":   skills.SkillAgentManagement,
			"tool_name":  "create_agent",
			"status":     "approved",
			"runtime_id": "tool_call:agent-management:create_agent::#1",
		},
		{
			"kind":       "tool_governance",
			"skill_id":   skills.SkillAgentManagement,
			"tool_name":  "create_agent",
			"status":     "needs_approval",
			"runtime_id": "tool_governance:first-create",
		},
		{
			"kind":       "tool_call",
			"skill_id":   skills.SkillAgentManagement,
			"tool_name":  "create_agent",
			"status":     "success",
			"runtime_id": "tool_call:agent-management:create_agent::#2",
			"result": map[string]interface{}{
				"agent_id":   "agent-alpha",
				"agent_name": "Alpha",
				"href":       "/console/agents/agent-alpha/agent",
			},
		},
		{
			"kind":       "tool_governance",
			"skill_id":   skills.SkillAgentManagement,
			"tool_name":  "create_agent",
			"status":     "success",
			"runtime_id": "tool_governance:first-create",
		},
		{
			"kind":       "tool_call",
			"skill_id":   skills.SkillAgentManagement,
			"tool_name":  "create_agent",
			"status":     "waiting_approval",
			"runtime_id": "tool_call:agent-management:create_agent::#3",
		},
		{
			"kind":       "tool_governance",
			"skill_id":   skills.SkillAgentManagement,
			"tool_name":  "create_agent",
			"status":     "needs_approval",
			"runtime_id": "tool_governance:second-create",
		},
	})

	plan := mapFromOperationContext(metadata["operation_plan"])
	if got := stringFromAny(plan["status"]); got != operationPlanStatusRunning {
		t.Fatalf("plan status = %q, want running; plan=%#v", got, plan)
	}
	if got := stringFromAny(plan["pending_next_action"]); got == "" || got == "none" {
		t.Fatalf("pending_next_action = %q, want pending repeated create; plan=%#v", got, plan)
	}
	if got := operationPlanStepStatusForTest(plan, createStepID); got != operationPlanStepStatusCompleted {
		t.Fatalf("base create status = %q, want completed; plan=%#v", got, plan)
	}
	if got := operationPlanStepStatusForTest(plan, repeatStepID); got != operationPlanStepStatusPending {
		t.Fatalf("repeat create status = %q, want pending; plan=%#v", got, plan)
	}
	repeatStep := operationPlanStepForTest(plan, repeatStepID)
	if got := stringFromAny(repeatStep["last_invocation_kind"]); got != "tool_governance" {
		t.Fatalf("repeat create last invocation kind = %q, want tool_governance; step=%#v", got, repeatStep)
	}
}

func TestSkillLoopPlanToolGuardBlocksRepeatedCreateAgentOutsideNamedTargets(t *testing.T) {
	query := "\u8bf7\u521b\u5efa\u4e34\u65f6\u667a\u80fd\u4f53\uff0c\u540d\u79f0\u5206\u522b\u4e3a Alpha \u548c Beta"
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     query,
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
				"original_user_goal": query,
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillAgentManagement,
		ToolName:  "create_agent",
		Arguments: map[string]interface{}{"name": "Gamma"},
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{{
			SkillID:   skills.SkillAgentManagement,
			ToolName:  "create_agent",
			Arguments: map[string]interface{}{"name": "Alpha"},
			Result:    map[string]interface{}{"agent_id": "agent-alpha", "name": "Alpha"},
		}},
	}); !blocked {
		t.Fatal("unrequested create_agent target was allowed, want named target list to constrain repeated creates")
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

func TestSkillLoopPlanToolGuardBlocksRepeatedCreateAgentFromMetadataTarget(t *testing.T) {
	query := "create two draft agents named Alpha and Beta"
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     query,
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
				"original_user_goal": query,
			},
			"skill_invocations": []interface{}{
				map[string]interface{}{
					"kind":      "tool_call",
					"status":    "success",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "create_agent",
					"result": map[string]interface{}{
						"agent_id": "agent-alpha",
						"agent": map[string]interface{}{
							"id":   "agent-alpha",
							"name": "Alpha",
						},
					},
				},
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillAgentManagement,
		ToolName:  "create_agent",
		Arguments: map[string]interface{}{"name": "Alpha", "description": "changed summary"},
	}); !blocked {
		t.Fatal("duplicate create_agent target from metadata was allowed, want blocked")
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillAgentManagement,
		ToolName:  "create_agent",
		Arguments: map[string]interface{}{"name": "Beta"},
	}); blocked {
		t.Fatal("missing create_agent target was blocked, want allowed for Beta")
	}
}

func TestSkillLoopPlanToolGuardBlocksRepeatedCreateAgentFromProgressTarget(t *testing.T) {
	query := "create two draft agents named Alpha and Beta"
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     query,
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
				"original_user_goal": query,
			},
			"agent_create_progress": map[string]interface{}{
				"operation":         "agent.create",
				"status":            "partial",
				"requested_count":   2,
				"completed_targets": []interface{}{"Alpha"},
				"missing_targets":   []interface{}{"Beta"},
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillAgentManagement,
		ToolName:  "create_agent",
		Arguments: map[string]interface{}{"name": "Alpha"},
	}); !blocked {
		t.Fatal("duplicate create_agent target from progress was allowed, want blocked")
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillAgentManagement,
		ToolName:  "create_agent",
		Arguments: map[string]interface{}{"name": "Beta"},
	}); blocked {
		t.Fatal("missing create_agent progress target was blocked, want allowed for Beta")
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

func TestSkillLoopPlanToolGuardAllowsSameGoalConfigAmendmentAfterIdentityStepCompleted(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "Edit this Agent name and model provider.",
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
				"original_user_goal": "Edit this Agent name and model provider.",
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopPlanToolCallGuard() = nil, want guard")
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillAgentManagement,
		ToolName:  "update_agent_config",
		Arguments: map[string]interface{}{"agent_id": "agent-1", "model_provider": "openai", "model": "gpt-4o"},
	}); blocked {
		t.Fatal("update_agent_config was blocked, want same-goal config amendment allowed after identity edit")
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	stepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")
	if got := operationPlanStepStatusForTest(plan, stepID); got != operationPlanStepStatusPending {
		t.Fatalf("update_agent_config step status = %q, want pending amendment; plan=%#v", got, plan)
	}
	if got := stringFromAny(plan["status"]); got != operationPlanStatusRunning {
		t.Fatalf("plan status = %q, want running after amendment; plan=%#v", got, plan)
	}
	deviations := mapSliceFromAny(plan["deviations"])
	if len(deviations) != 1 || stringFromAny(deviations[0]["reason"]) != "model_amended_operation_plan_within_user_goal" {
		t.Fatalf("deviations = %#v, want one same-goal amendment", deviations)
	}
}

func TestSkillLoopPlanToolGuardAllowsBindingConfigAmendmentAfterReadOnlyPlanCompleted(t *testing.T) {
	const goal = "Unbind the currently enabled Agent skill, knowledge base, database table, and workflow from this Agent. Keep every other binding unchanged."
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     goal,
			Surface:   aiChatSurfaceContextualSidebar,
			SkillMode: skillModeAuto,
			SkillIDs:  []string{skills.SkillAgentManagement},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status": operationPlanStatusCompleted,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config"),
						"status":    operationPlanStepStatusCompleted,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "get_agent_config",
					},
				},
				"step_status": map[string]interface{}{
					operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config"): operationPlanStepStatusCompleted,
				},
				"original_user_goal": goal,
			},
		}},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopPlanToolCallGuard() = nil, want guard")
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Arguments: map[string]interface{}{
			"agent_id":                     "agent-1",
			"remove_enabled_skill_ids":     []interface{}{"chart-generator"},
			"remove_knowledge_dataset_ids": []interface{}{"kb-1"},
			"remove_database_bindings":     []interface{}{map[string]interface{}{"data_source_id": "db-1", "table_ids": []interface{}{"table-1"}}},
			"remove_workflow_bindings":     []interface{}{map[string]interface{}{"binding_id": "workflow-binding-1"}},
		},
	}); blocked {
		t.Fatal("update_agent_config was blocked, want binding/unbinding config amendment allowed after read-only plan")
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	stepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")
	if got := operationPlanStepStatusForTest(plan, stepID); got != operationPlanStepStatusPending {
		t.Fatalf("update_agent_config step status = %q, want pending amendment; plan=%#v", got, plan)
	}
	if got := stringFromAny(plan["status"]); got != operationPlanStatusRunning {
		t.Fatalf("plan status = %q, want running after amendment; plan=%#v", got, plan)
	}
	deviations := mapSliceFromAny(plan["deviations"])
	if len(deviations) != 1 || stringFromAny(deviations[0]["reason"]) != "model_amended_operation_plan_within_user_goal" {
		t.Fatalf("deviations = %#v, want one same-goal amendment", deviations)
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

func TestRestrictResolvedSkillsForPreparedTurnKeepsPendingPlanSkillsVisible(t *testing.T) {
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
	want := []string{skills.SkillAgentManagement}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered skill ids = %#v, want pending operation-plan skills %#v", got, want)
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
		!strings.Contains(content, "current goal cannot proceed from current page evidence") ||
		!strings.Contains(content, "do not say you need to navigate") ||
		!strings.Contains(content, "Do not call list_agents only to verify that same single Agent") {
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

func TestAgentManagementDeleteJustNowCreatedReferenceDoesNotPlanCreate(t *testing.T) {
	query := "\u8bf7\u5220\u9664\u521a\u624d\u521b\u5efa\u7684\u6d4b\u8bd5\u667a\u80fd\u4f53 AICHAT-GOAL-SMOKE-1782754487710\u3002\u53ea\u5220\u9664\u8fd9\u4e2a\u540d\u5b57\u5b8c\u5168\u5339\u914d\u7684\u667a\u80fd\u4f53\uff0c\u5220\u9664\u540e\u786e\u8ba4\u5217\u8868\u4e2d\u5df2\u7ecf\u4e0d\u53ef\u89c1\u3002"
	if !agentManagementCreateRequested(query) {
		t.Fatalf("agentManagementCreateRequested(%q) = false before reference filter, want raw marker detected", query)
	}
	if !agentManagementCreateMentionIsDeleteTargetReference(query) {
		t.Fatalf("agentManagementCreateMentionIsDeleteTargetReference(%q) = false, want true for just-now-created delete target", query)
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
		t.Fatalf("planned_tools = %#v, want stale create_agent removed for just-now-created delete target", strategy.PlannedTools)
	}
	if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "delete_agent") {
		t.Fatalf("planned_tools = %#v, missing delete_agent", strategy.PlannedTools)
	}
	plan := operationPlanFromTurnStrategy("task-agent-delete-just-now-created-reference", parts, strategy)
	if _, ok := mapFromOperationContext(plan["step_status"])[operationPlanToolStepID(skills.SkillAgentManagement, "create_agent")]; ok {
		t.Fatalf("plan = %#v, want no create_agent step", plan)
	}
	if got := stringFromAny(plan["pending_next_action"]); strings.Contains(got, "create_agent") {
		t.Fatalf("pending_next_action = %q, want no stale create_agent", got)
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

func TestAgentManagementCreateThenBatchDeletePlansBothInOrder(t *testing.T) {
	query := "\u8bf7\u521b\u5efa\u4e24\u4e2a\u4e34\u65f6\u667a\u80fd\u4f53\uff0c\u540d\u79f0\u5206\u522b\u4e3a PLAN-A \u548c PLAN-B\u3002\u63cf\u8ff0\u90fd\u5199\u201cAIChat \u6279\u91cf\u5220\u9664\u5192\u70df\uff0c\u53ef\u4ee5\u5220\u9664\u201d\u3002\u521b\u5efa\u6210\u529f\u540e\u56de\u5230\u667a\u80fd\u4f53\u5217\u8868\uff0c\u5e76\u4e00\u6b21\u6027\u6279\u91cf\u5220\u9664\u8fd9\u4e24\u4e2a\u65b0\u5efa\u667a\u80fd\u4f53\u3002"
	if !agentManagementCreateRequested(query) {
		t.Fatalf("agentManagementCreateRequested(%q) = false, want true", query)
	}
	if !agentManagementDeleteRequested(query) {
		t.Fatalf("agentManagementDeleteRequested(%q) = false, want true for explicit delete outside quoted description", query)
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
	for _, want := range []string{"create_agent", "delete_agents"} {
		if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, want) {
			t.Fatalf("planned_tools = %#v, missing agent-management/%s", strategy.PlannedTools, want)
		}
	}
	plan := operationPlanFromTurnStrategy("task-agent-create-then-delete", parts, strategy)
	createStepID := operationPlanToolStepID(skills.SkillAgentManagement, "create_agent")
	deleteStepID := operationPlanToolStepID(skills.SkillAgentManagement, "delete_agents")
	for _, want := range []string{createStepID, deleteStepID} {
		if got := operationPlanStepStatusForTest(plan, want); got != operationPlanStepStatusPending {
			t.Fatalf("%s step status = %q, want pending; plan=%#v", want, got, plan)
		}
	}
	createIndex := operationPlanStepIndexForTest(plan, createStepID)
	deleteIndex := operationPlanStepIndexForTest(plan, deleteStepID)
	if createIndex < 0 || deleteIndex < 0 || createIndex > deleteIndex {
		t.Fatalf("step order create=%d delete=%d in %#v, want create before delete", createIndex, deleteIndex, plan["steps"])
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

func TestAgentManagementCreateForBatchDeleteRegressionDoesNotPlanDelete(t *testing.T) {
	query := "请在当前智能体列表页创建两个用于批量删除回归的临时智能体，名称分别为 PLAN-DELETE-A 和 PLAN-DELETE-B，描述都写“AIChat planner 批量删除回归对象”，图标分别用 PA 和 PB。只创建这两个，不要进入详情页，不要修改任何配置。需要审批时我会同意。完成后请确认列表里能看到这两个名字。"
	if !agentManagementCreateRequested(query) {
		t.Fatalf("agentManagementCreateRequested(%q) = false, want true", query)
	}
	if agentManagementDeleteRequested(query) {
		t.Fatalf("agentManagementDeleteRequested(%q) = true, want false for delete-regression purpose text", query)
	}
	if agentManagementBatchDeleteRequested(query) {
		t.Fatalf("agentManagementBatchDeleteRequested(%q) = true, want false for delete-regression purpose text", query)
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
	for _, unexpected := range []string{"delete_agent", "delete_agents", "get_agent_config", "update_agent_config"} {
		if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, unexpected) {
			t.Fatalf("planned_tools = %#v, want no %s for create-only delete-regression setup", strategy.PlannedTools, unexpected)
		}
	}
}

func TestAgentManagementCreateWithInlineIdentityFieldsDoesNotPlanFollowupIdentity(t *testing.T) {
	query := "\u521b\u5efa Agent \u5192\u70df AICHAT-GOAL-AGENT-1\uff1a\u8bf7\u5728\u5f53\u524d\u667a\u80fd\u4f53\u5217\u8868\u9875\u521b\u5efa\u4e00\u4e2a\u6d4b\u8bd5\u667a\u80fd\u4f53\uff0c\u540d\u79f0\u4e3a\u201cAICHAT-GOAL-AGENT-1\u201d\uff0c\u63cf\u8ff0\u4e3a\u201cAIChat \u521b\u5efa\u95ed\u73af\u5192\u70df\u6d4b\u8bd5\uff0c\u53ef\u5220\u9664\u201d\uff0c\u56fe\u6807\u4f7f\u7528\u4e00\u4e2a\u5bb9\u6613\u8bc6\u522b\u7684\u9ed8\u8ba4\u5f69\u8272\u56fe\u6807\u3002\u521b\u5efa\u6210\u529f\u540e\u8bf7\u8fdb\u5165\u8fd9\u4e2a\u65b0\u667a\u80fd\u4f53\u7684\u8be6\u60c5/\u7f16\u8f91\u9875\uff0c\u5e76\u57fa\u4e8e\u5de5\u5177\u8fd4\u56de\u503c\u548c\u9875\u9762\u53ef\u89c1\u7ed3\u679c\u8bf4\u660e\u521b\u5efa\u7684\u667a\u80fd\u4f53\u540d\u79f0\u3002"
	if !agentManagementCreateRequested(query) {
		t.Fatalf("agentManagementCreateRequested(%q) = false, want true", query)
	}
	if agentManagementIdentityUpdateRequested(query) {
		t.Fatalf("agentManagementIdentityUpdateRequested(%q) = true, want false for inline create fields", query)
	}

	parts := &chatRequestParts{
		Query:          query,
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents",
		SkillMode:      skillModeAuto,
		SkillIDs:       []string{skills.SkillAgentManagement, skills.SkillConsoleNavigator},
	}
	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{Intent: "manage_agent_asset"})
	if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "create_agent") {
		t.Fatalf("planned_tools = %#v, missing create_agent", strategy.PlannedTools)
	}
	for _, unexpected := range []string{"update_agent_identity", "get_agent_config", "update_agent_config"} {
		if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, unexpected) {
			t.Fatalf("planned_tools = %#v, want no %s for inline create fields", strategy.PlannedTools, unexpected)
		}
	}

	plan := operationPlanFromTurnStrategy("task-agent-create-inline-identity", parts, strategy)
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "create_agent")); got != operationPlanStepStatusPending {
		t.Fatalf("create_agent step status = %q, want pending; plan=%#v", got, plan)
	}
	for _, unexpected := range []string{"update_agent_identity", "get_agent_config", "update_agent_config"} {
		if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, unexpected)); got != "" {
			t.Fatalf("%s step status = %q, want absent; plan=%#v", unexpected, got, plan)
		}
	}
}

func TestAgentManagementCreateOpenEditDetailDoesNotPlanConfigUpdate(t *testing.T) {
	query := "\u8bf7\u5728\u5f53\u524d\u667a\u80fd\u4f53\u5217\u8868\u4e2d\u521b\u5efa\u4e00\u4e2a\u6d4b\u8bd5\u667a\u80fd\u4f53\uff0c\u540d\u79f0 GOAL-CLOSURE-SMOKE-1\uff0c\u63cf\u8ff0\u201cPlanner\u95ed\u73af\u9a8c\u8bc1 1\u201d\uff0c\u56fe\u6807\u7528 \U0001f9ed\u3002\u521b\u5efa\u6210\u529f\u540e\u6253\u5f00\u5b83\u7684\u7f16\u8f91\u8be6\u60c5\u9875\uff0c\u5e76\u53ea\u57fa\u4e8e\u5de5\u5177\u8fd4\u56de\u503c\u548c\u9875\u9762\u8bc1\u636e\u7b80\u77ed\u8bf4\u660e\u7ed3\u679c\u3002"
	if !agentManagementCreateRequested(query) {
		t.Fatalf("agentManagementCreateRequested(%q) = false, want true", query)
	}
	if !wantsCreatedAgentDetailNavigation(query) {
		t.Fatalf("wantsCreatedAgentDetailNavigation(%q) = false, want true", query)
	}
	if agentManagementConfigUpdateRequested(query) {
		t.Fatalf("agentManagementConfigUpdateRequested(%q) = true, want false for create plus edit-detail navigation", query)
	}
	if agentManagementPostCreateConfigEditRequested(query) {
		t.Fatalf("agentManagementPostCreateConfigEditRequested(%q) = true, want false for create plus edit-detail navigation", query)
	}

	parts := &chatRequestParts{
		Query:          query,
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents",
		SkillMode:      skillModeAuto,
		SkillIDs:       []string{skills.SkillAgentManagement, skills.SkillConsoleNavigator},
	}
	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{
		Intent: "manage_agent_asset",
		PlannedTools: []AIChatTurnStrategyTool{
			{SkillID: skills.SkillAgentManagement, ToolName: "create_agent"},
			{SkillID: skills.SkillAgentManagement, ToolName: "get_agent_config"},
			{SkillID: skills.SkillAgentManagement, ToolName: "update_agent_config"},
		},
	})
	if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "create_agent") {
		t.Fatalf("planned_tools = %#v, missing create_agent", strategy.PlannedTools)
	}
	for _, unexpected := range []string{"get_agent_config", "update_agent_config"} {
		if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, unexpected) {
			t.Fatalf("planned_tools = %#v, want no %s for create plus edit-detail navigation", strategy.PlannedTools, unexpected)
		}
	}

	plan := operationPlanFromTurnStrategy("task-agent-create-open-edit-detail", parts, strategy)
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "create_agent")); got != operationPlanStepStatusPending {
		t.Fatalf("create_agent step status = %q, want pending; plan=%#v", got, plan)
	}
	for _, unexpected := range []string{"get_agent_config", "update_agent_config"} {
		if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, unexpected)); got != "" {
			t.Fatalf("%s step status = %q, want absent; plan=%#v", unexpected, got, plan)
		}
	}
}

func TestAgentManagementBindingFinalAnswerInstructionsDoNotPlanIdentityUpdate(t *testing.T) {
	query := "针对当前列表中的智能体 AICHAT-CONFIG-SMOKE，检查 Skill「图表生成器」当前是否绑定；如果已绑定就解绑，如果未绑定就绑定。执行变更后必须重新读取该智能体配置，再最终回答：本次实际进行了绑定还是解绑、目标 Skill 名称、复读配置是否完成。"
	if agentManagementIdentityUpdateRequested(query) {
		t.Fatalf("agentManagementIdentityUpdateRequested(%q) = true, want false for final-answer resource-name instruction", query)
	}
	if !agentManagementSkillBindingRequested(query) {
		t.Fatalf("agentManagementSkillBindingRequested(%q) = false, want true", query)
	}

	parts := &chatRequestParts{
		Query:          query,
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents",
		SkillMode:      skillModeAuto,
		SkillIDs:       []string{skills.SkillAgentManagement},
	}
	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{Intent: "manage_agent_asset"})
	if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "update_agent_identity") {
		t.Fatalf("planned_tools = %#v, want no update_agent_identity from final-answer instruction", strategy.PlannedTools)
	}
	for _, want := range []string{"get_agent_config", "list_agent_skill_candidates", "update_agent_config"} {
		if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, want) {
			t.Fatalf("planned_tools = %#v, missing %s", strategy.PlannedTools, want)
		}
	}

	plan := operationPlanFromTurnStrategy("task-agent-binding-final-answer", parts, strategy)
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_identity")); got != "" {
		t.Fatalf("update_agent_identity step status = %q, want absent; plan=%#v", got, plan)
	}
}

func TestAgentManagementResourceBindingAnswerInstructionsDoNotPlanIdentityUpdate(t *testing.T) {
	query := "请读取配置和可绑定资源；如果知识库、数据库表、工作流当前未绑定，请分别选择当前工作空间第一个可用知识库、一个可用数据库表、一个可用工作流并申请审批一次性绑定。执行后基于工具返回和最终绑定状态如实回答，必须写清楚绑定的是知识库/数据库表/工作流各自的名称。不要修改名称、描述、图标、模型、系统提示词、开场问题、Skill。"
	if agentManagementIdentityUpdateRequested(query) {
		t.Fatalf("agentManagementIdentityUpdateRequested(%q) = true, want false for resource-name final answer instruction", query)
	}

	parts := &chatRequestParts{
		Query:          query,
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents",
		SkillMode:      skillModeAuto,
		SkillIDs:       []string{skills.SkillAgentManagement},
	}
	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{Intent: "manage_agent_asset"})
	if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "update_agent_identity") {
		t.Fatalf("planned_tools = %#v, want no update_agent_identity from resource-name answer instruction", strategy.PlannedTools)
	}
	for _, want := range []string{
		"get_agent_config",
		"list_agent_knowledge_candidates",
		"list_agent_database_candidates",
		"list_agent_database_tables",
		"list_agent_workflow_binding_candidates",
		"update_agent_config",
	} {
		if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, want) {
			t.Fatalf("planned_tools = %#v, missing %s", strategy.PlannedTools, want)
		}
	}

	plan := operationPlanFromTurnStrategy("task-agent-resource-binding-final-answer", parts, strategy)
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_identity")); got != "" {
		t.Fatalf("update_agent_identity step status = %q, want absent; plan=%#v", got, plan)
	}
}

func TestAgentManagementUnbindRemoveFieldsDoesNotPlanAgentDelete(t *testing.T) {
	query := "unbind the four resources from this agent: Skill Architecture, knowledge base Support KB, database table CRM.customers, and workflow Support Flow. First read current config, then use one update_agent_config remove_* fields patch to unbind them. Do not modify name, description, icon, model, system prompt, or opening questions. After completion read config again."
	if agentManagementDeleteRequested(query) {
		t.Fatalf("agentManagementDeleteRequested(%q) = true, want false for binding remove_* patch", query)
	}
	if !agentManagementConfigUpdateRequested(query) {
		t.Fatalf("agentManagementConfigUpdateRequested(%q) = false, want update_agent_config", query)
	}

	parts := &chatRequestParts{
		Query:          query,
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents/agent-1/agent",
		SkillMode:      skillModeAuto,
		SkillIDs:       []string{skills.SkillAgentManagement},
		RawOperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{
					"resource_type": "agent",
					"resource_id":   "agent-1",
					"title":         "Support Agent",
					"selected":      true,
					"can_edit":      true,
				},
			},
		},
	}
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	for _, unexpected := range []string{"delete_agent", "delete_agents"} {
		if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, unexpected) {
			t.Fatalf("planned_tools = %#v, want no %s for binding unbind request", strategy.PlannedTools, unexpected)
		}
	}
	for _, want := range []string{"get_agent_config", "update_agent_config"} {
		if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, want) {
			t.Fatalf("planned_tools = %#v, missing %s", strategy.PlannedTools, want)
		}
	}

	plan := operationPlanFromTurnStrategy("task-agent-unbind-remove-fields", parts, strategy)
	for _, unexpected := range []string{"delete_agent", "delete_agents"} {
		if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, unexpected)); got != "" {
			t.Fatalf("%s step status = %q, want absent; plan=%#v", unexpected, got, plan)
		}
	}
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")); got != operationPlanStepStatusPending {
		t.Fatalf("update_agent_config step status = %q, want pending; plan=%#v", got, plan)
	}
	if got := operationPlanStepStatusForTest(plan, operationPlanPostUpdateAgentConfigReadStepID()); got != operationPlanStepStatusPending {
		t.Fatalf("post-update get_agent_config step status = %q, want pending; plan=%#v", got, plan)
	}
}

func TestAgentManagementDeleteWithBindingPreserveClauseStillPlansDelete(t *testing.T) {
	query := "delete this agent, but do not modify its Skill, knowledge base, database table, or workflow bindings first."
	if !agentManagementDeleteRequested(query) {
		t.Fatalf("agentManagementDeleteRequested(%q) = false, want true for direct Agent deletion", query)
	}
}

func TestOperationPlanAgentConfigPostReadClosesCoveredReadOnlySteps(t *testing.T) {
	updateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")
	candidateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "list_agent_skill_candidates")
	postReadStepID := operationPlanPostUpdateAgentConfigReadStepID()
	steps := []map[string]interface{}{
		{"id": "observe", "status": operationPlanStepStatusPending},
		{
			"id":        updateStepID,
			"status":    operationPlanStepStatusCompleted,
			"skill_id":  skills.SkillAgentManagement,
			"tool_name": "update_agent_config",
		},
		{
			"id":        candidateStepID,
			"status":    operationPlanStepStatusPending,
			"skill_id":  skills.SkillAgentManagement,
			"tool_name": "list_agent_skill_candidates",
		},
		{
			"id":                                postReadStepID,
			"status":                            operationPlanStepStatusCompleted,
			"skill_id":                          skills.SkillAgentManagement,
			"tool_name":                         "get_agent_config",
			"phase":                             "post_update_verification",
			"required_post_update_verification": true,
		},
	}
	stepStatus := map[string]interface{}{
		"observe":       operationPlanStepStatusPending,
		updateStepID:    operationPlanStepStatusCompleted,
		candidateStepID: operationPlanStepStatusPending,
		postReadStepID:  operationPlanStepStatusCompleted,
	}
	plan := map[string]interface{}{"steps": mapsToInterfaceSlice(steps), "step_status": stepStatus}

	applyOperationPlanProgress(plan, steps, stepStatus, "", "")

	if got := plan["status"]; got != operationPlanStatusCompleted {
		t.Fatalf("status = %#v, want completed; plan=%#v", got, plan)
	}
	if got := plan["pending_next_action"]; got != "none" {
		t.Fatalf("pending_next_action = %#v, want none; plan=%#v", got, plan)
	}
	if got := stepStatus["observe"]; got != operationPlanStepStatusCompleted {
		t.Fatalf("observe step_status = %#v, want completed", got)
	}
	if got := stepStatus[candidateStepID]; got != operationPlanStepStatusCompleted {
		t.Fatalf("candidate step_status = %#v, want completed", got)
	}
	if reason := strings.TrimSpace(stringFromAny(steps[2]["skipped_reason"])); reason != "covered_by_post_update_agent_config_read" {
		t.Fatalf("candidate skipped_reason = %q", reason)
	}
}

func TestOperationPlanCompletedAgentMutationClosesAdvisoryNavigationAndReads(t *testing.T) {
	updateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")
	readStepID := operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config")
	routeStepID := operationPlanRouteStepID("/console/agents", 1)
	navigateStepID := operationPlanToolStepID(skills.SkillConsoleNavigator, "navigate")
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":       operationPlanStatusRunning,
			"current_page": "/console/agents",
			"page_evidence": map[string]interface{}{
				"current_page":                   "/console/agents",
				"target_route_already_available": true,
			},
			"steps": []interface{}{
				map[string]interface{}{
					"id":        navigateStepID,
					"status":    operationPlanStepStatusPending,
					"skill_id":  skills.SkillConsoleNavigator,
					"tool_name": "navigate",
					"asset_target": map[string]interface{}{
						"page": "/console/agents/agent-1/agent",
					},
				},
				map[string]interface{}{
					"id":        routeStepID,
					"status":    operationPlanStepStatusPending,
					"skill_id":  skills.SkillConsoleNavigator,
					"tool_name": "navigate",
					"asset_target": map[string]interface{}{
						"page": "/console/agents",
					},
				},
				map[string]interface{}{
					"id":        readStepID,
					"status":    operationPlanStepStatusPending,
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "get_agent_config",
					"asset_target": map[string]interface{}{
						"effect":     "read",
						"asset_type": "agent",
					},
				},
				map[string]interface{}{
					"id":                                  updateStepID,
					"status":                              operationPlanStepStatusPending,
					"skill_id":                            skills.SkillAgentManagement,
					"tool_name":                           "update_agent_config",
					operationPlanExpectedUpdatedFieldsKey: []interface{}{"model"},
					"asset_target": map[string]interface{}{
						"effect":     "update",
						"asset_type": "agent",
					},
				},
				map[string]interface{}{
					"id":       "skill:" + skills.SkillConsoleNavigator,
					"status":   operationPlanStepStatusPending,
					"skill_id": skills.SkillConsoleNavigator,
					"role":     "primary",
				},
				map[string]interface{}{
					"id":       "skill:" + skills.SkillAgentManagement,
					"status":   operationPlanStepStatusPending,
					"skill_id": skills.SkillAgentManagement,
					"role":     "primary",
				},
				map[string]interface{}{
					"id":     "observe",
					"status": operationPlanStepStatusPending,
				},
			},
			"step_status": map[string]interface{}{
				navigateStepID:                          operationPlanStepStatusPending,
				routeStepID:                             operationPlanStepStatusPending,
				readStepID:                              operationPlanStepStatusPending,
				updateStepID:                            operationPlanStepStatusPending,
				"skill:" + skills.SkillConsoleNavigator: operationPlanStepStatusPending,
				"skill:" + skills.SkillAgentManagement:  operationPlanStepStatusPending,
				"observe":                               operationPlanStepStatusPending,
			},
		},
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "update_agent_config",
		"result": map[string]interface{}{
			"status":         "completed",
			"agent_id":       "agent-1",
			"agent_name":     "Support Agent",
			"updated_fields": []interface{}{"model_provider", "model"},
			"model_provider": "deepseek",
			"model":          "deepseek-chat",
		},
	}})

	plan := metadata["operation_plan"].(map[string]interface{})
	if got := stringFromAny(plan["status"]); got != operationPlanStatusCompleted {
		t.Fatalf("operation_plan status = %q, want completed; plan=%#v", got, plan)
	}
	if got := stringFromAny(plan["pending_next_action"]); got != "none" {
		t.Fatalf("pending_next_action = %q, want none; plan=%#v", got, plan)
	}
	for _, id := range []string{
		navigateStepID,
		routeStepID,
		readStepID,
		updateStepID,
		"skill:" + skills.SkillConsoleNavigator,
		"skill:" + skills.SkillAgentManagement,
		"observe",
	} {
		if got := operationPlanStepStatusForTest(plan, id); got != operationPlanStepStatusCompleted {
			t.Fatalf("steps[%s].status = %q, want completed; plan=%#v", id, got, plan)
		}
	}
	for _, id := range []string{navigateStepID, routeStepID, readStepID, "observe"} {
		if got := operationPlanStepFieldForTest(plan, id, "skipped_reason"); got != "covered_by_completed_agent_mutation_result" {
			t.Fatalf("steps[%s].skipped_reason = %q, want covered_by_completed_agent_mutation_result", id, got)
		}
	}
	if deviations := mapSliceFromAny(plan["deviations"]); len(deviations) == 0 {
		t.Fatalf("deviations = %#v, want covered exploration deviation", plan["deviations"])
	}
}

func TestOperationPlanAgentIdentityPostReadClosesCoveredReadOnlySteps(t *testing.T) {
	updateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_identity")
	readStepID := operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config")
	postReadStepID := operationPlanPostUpdateAgentConfigReadStepID()
	steps := []map[string]interface{}{
		{"id": "observe", "status": operationPlanStepStatusPending},
		{
			"id":        readStepID,
			"status":    operationPlanStepStatusPending,
			"skill_id":  skills.SkillAgentManagement,
			"tool_name": "get_agent_config",
		},
		{
			"id":        updateStepID,
			"status":    operationPlanStepStatusCompleted,
			"skill_id":  skills.SkillAgentManagement,
			"tool_name": "update_agent_identity",
		},
		{
			"id":                                postReadStepID,
			"status":                            operationPlanStepStatusCompleted,
			"skill_id":                          skills.SkillAgentManagement,
			"tool_name":                         "get_agent_config",
			"phase":                             "post_update_verification",
			"required_post_update_verification": true,
		},
	}
	stepStatus := map[string]interface{}{
		"observe":                operationPlanStepStatusPending,
		readStepID:               operationPlanStepStatusPending,
		updateStepID:             operationPlanStepStatusCompleted,
		postReadStepID:           operationPlanStepStatusCompleted,
		"skill:agent-management": operationPlanStepStatusCompleted,
	}
	plan := map[string]interface{}{"steps": mapsToInterfaceSlice(steps), "step_status": stepStatus}

	applyOperationPlanProgress(plan, steps, stepStatus, "", "")

	if got := plan["status"]; got != operationPlanStatusCompleted {
		t.Fatalf("status = %#v, want completed; plan=%#v", got, plan)
	}
	if got := plan["pending_next_action"]; got != "none" {
		t.Fatalf("pending_next_action = %#v, want none; plan=%#v", got, plan)
	}
	for _, id := range []string{"observe", readStepID} {
		if got := stepStatus[id]; got != operationPlanStepStatusCompleted {
			t.Fatalf("step_status[%s] = %#v, want completed", id, got)
		}
	}
}

func TestOperationPlanPostUpdateReadCanShareInvocationWithNormalConfigRead(t *testing.T) {
	updateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_identity")
	readStepID := operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config")
	postReadStepID := operationPlanPostUpdateAgentConfigReadStepID()
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{
				map[string]interface{}{
					"id":        readStepID,
					"status":    operationPlanStepStatusPending,
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "get_agent_config",
				},
				map[string]interface{}{
					"id":        updateStepID,
					"status":    operationPlanStepStatusCompleted,
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_identity",
				},
				map[string]interface{}{
					"id":                                postReadStepID,
					"status":                            operationPlanStepStatusPending,
					"skill_id":                          skills.SkillAgentManagement,
					"tool_name":                         "get_agent_config",
					"wait_for":                          updateStepID,
					"phase":                             "post_update_verification",
					"required_post_update_verification": true,
				},
			},
			"step_status": map[string]interface{}{
				readStepID:     operationPlanStepStatusPending,
				updateStepID:   operationPlanStepStatusCompleted,
				postReadStepID: operationPlanStepStatusPending,
			},
		},
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{
		{
			"kind":       "tool_call",
			"status":     "success",
			"skill_id":   skills.SkillAgentManagement,
			"tool_name":  "get_agent_config",
			"runtime_id": "tool_call:agent-management:get_agent_config::#1",
			"result": map[string]interface{}{
				"status":     "success",
				"agent_id":   "agent-1",
				"agent_name": "Support Agent",
			},
		},
	})

	plan := metadata["operation_plan"].(map[string]interface{})
	for _, id := range []string{readStepID, postReadStepID} {
		if got := operationPlanStepStatusForTest(plan, id); got != operationPlanStepStatusCompleted {
			t.Fatalf("step %s status = %q, want completed; plan=%#v", id, got, plan)
		}
	}
	if got := plan["pending_next_action"]; got != "none" {
		t.Fatalf("pending_next_action = %#v, want none; plan=%#v", got, plan)
	}
}

func TestAgentManagementCreateWithDefaultConfigPrunesOverBroadPlannerTools(t *testing.T) {
	query := "\u8bf7\u521b\u5efa\u4e00\u4e2a\u6d4b\u8bd5\u667a\u80fd\u4f53\uff0c\u540d\u79f0\u4e3a AICHAT-GOAL-BIND-SMOKE-1782757787864\uff0c\u63cf\u8ff0\u4e3a Agent \u914d\u7f6e\u7ed1\u5b9a\u5192\u70df\u6d4b\u8bd5\uff0c\u53ef\u4f7f\u7528\u9ed8\u8ba4\u6a21\u578b\u548c\u914d\u7f6e\u3002\u521b\u5efa\u540e\u8bf7\u786e\u8ba4\u5b83\u5728\u5217\u8868\u4e2d\u53ef\u89c1\u3002"
	if !agentManagementCreateRequested(query) {
		t.Fatalf("agentManagementCreateRequested(%q) = false, want true", query)
	}
	if agentManagementConfigReadRequested(query) {
		t.Fatalf("agentManagementConfigReadRequested(%q) = true, want false for default config reference", query)
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
			{SkillID: skills.SkillAgentManagement, ToolName: "get_agent_config"},
			{SkillID: skills.SkillAgentManagement, ToolName: "update_agent_identity"},
			{SkillID: skills.SkillAgentManagement, ToolName: "list_available_models"},
			{SkillID: skills.SkillAgentManagement, ToolName: "update_agent_config"},
		},
	})
	if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "create_agent") {
		t.Fatalf("planned_tools = %#v, missing create_agent", strategy.PlannedTools)
	}
	for _, unexpected := range []string{"get_agent_config", "update_agent_identity", "list_available_models", "update_agent_config"} {
		if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, unexpected) {
			t.Fatalf("planned_tools = %#v, want no over-broad %s for create-only default config request", strategy.PlannedTools, unexpected)
		}
	}

	plan := operationPlanFromTurnStrategy("task-agent-create-default-config", parts, strategy)
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "create_agent")); got != operationPlanStepStatusPending {
		t.Fatalf("create_agent step status = %q, want pending; plan=%#v", got, plan)
	}
	for _, unexpected := range []string{"get_agent_config", "update_agent_identity", "list_available_models", "update_agent_config"} {
		if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, unexpected)); got != "" {
			t.Fatalf("%s step status = %q, want absent; plan=%#v", unexpected, got, plan)
		}
	}

	_, ok := skillloop.FastPathFinalAnswerForCompletionEvidence(map[string]interface{}{
		"operation_plan": plan,
		"client_actions": []interface{}{
			map[string]interface{}{
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "create_agent",
				"status":    "succeeded",
			},
		},
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "create_agent",
				"status":    "success",
				"result": map[string]interface{}{
					"status":     "completed",
					"agent_id":   "agent-1",
					"agent_name": "AICHAT-GOAL-BIND-SMOKE-1782757787864",
				},
			},
		},
	})
	if !ok {
		t.Fatal("FastPathFinalAnswerForCompletionEvidence() ok = false, want create result to close after observation")
	}
}

func TestAgentManagementCreateThenExplicitBindingSurvivesCreatePayloadPruning(t *testing.T) {
	query := "\u8bf7\u521b\u5efa\u4e00\u4e2a\u667a\u80fd\u4f53\uff0c\u540d\u79f0\u4e3a SMOKE-BIND\uff0c\u63cf\u8ff0\u4e3a Agent \u914d\u7f6e\u7ed1\u5b9a\u5192\u70df\u6d4b\u8bd5\uff0c\u5e76\u7ed1\u5b9a\u77e5\u8bc6\u5e93 \u6d4b\u8bd5\u5e932\u3002"
	parts := &chatRequestParts{
		Query:     query,
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillAgentManagement},
	}
	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{Intent: "manage_agent_asset"})
	for _, want := range []string{"create_agent", "get_agent_config", "list_agent_knowledge_candidates", "update_agent_config"} {
		if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, want) {
			t.Fatalf("planned_tools = %#v, missing %s for explicit create-then-bind request", strategy.PlannedTools, want)
		}
	}
}

func TestAgentManagementBindingNoopIsResourceScoped(t *testing.T) {
	query := "只绑定当前工作空间第一个可用 Skill、一个知识库、一个数据库表；不要绑定工作流，workflow_bindings 保持为空。"
	parts := &chatRequestParts{
		Query:     query,
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillAgentManagement},
	}
	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{Intent: "manage_agent_asset"})
	for _, want := range []string{
		"get_agent_config",
		"list_agent_skill_candidates",
		"list_agent_knowledge_candidates",
		"list_agent_database_candidates",
		"list_agent_database_tables",
		"update_agent_config",
	} {
		if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, want) {
			t.Fatalf("planned_tools = %#v, missing %s for scoped binding request", strategy.PlannedTools, want)
		}
	}
	if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "list_agent_workflow_binding_candidates") {
		t.Fatalf("planned_tools = %#v, want workflow no-op not to plan workflow candidates", strategy.PlannedTools)
	}
	requiredTools := requiredAgentBindingMutationTools(query)
	if stringSliceContainsFold(requiredTools, "replace_agent_workflow_bindings") {
		t.Fatalf("requiredAgentBindingMutationTools(%q) = %#v, want no workflow mutation", query, requiredTools)
	}
	for _, want := range []string{"replace_agent_knowledge_bindings", "replace_agent_database_bindings"} {
		if !stringSliceContainsFold(requiredTools, want) {
			t.Fatalf("requiredAgentBindingMutationTools(%q) = %#v, missing %s", query, requiredTools, want)
		}
	}
}

func TestAgentSkillBindingPromptDoesNotPlanCreateOrUnrelatedUnbinds(t *testing.T) {
	query := "Bind the first available Skill to the current agent using add_enabled_skill_ids. Do not modify knowledge, database, or workflow. Re-read config after the update."
	if agentManagementCreateRequested(query) {
		t.Fatalf("agentManagementCreateRequested(%q) = true, want false; add_enabled_skill_ids must not count as adding an Agent", query)
	}
	requiredTools := requiredAgentBindingMutationTools(query)
	for _, unwanted := range []string{
		"replace_agent_knowledge_bindings",
		"replace_agent_database_bindings",
		"replace_agent_workflow_bindings",
	} {
		if stringSliceContainsFold(requiredTools, unwanted) {
			t.Fatalf("requiredAgentBindingMutationTools(%q) = %#v, want no unrelated %s", query, requiredTools, unwanted)
		}
	}
	fields := agentManagementExpectedConfigUpdateFields(query)
	if len(fields) != 1 || fields[0] != "enabled_skill_ids" {
		t.Fatalf("agentManagementExpectedConfigUpdateFields(%q) = %#v, want only enabled_skill_ids", query, fields)
	}
	actions := agentManagementExpectedConfigBindingActions(query)
	if got := actions["enabled_skill_ids"]; got != "bind" {
		t.Fatalf("agentManagementExpectedConfigBindingActions(%q)[enabled_skill_ids] = %q, want bind; actions=%#v", query, got, actions)
	}
	for _, field := range []string{"knowledge_dataset_ids", "database_bindings", "workflow_bindings"} {
		if got := actions[field]; got != "" {
			t.Fatalf("agentManagementExpectedConfigBindingActions(%q)[%s] = %q, want empty; actions=%#v", query, field, got, actions)
		}
	}

	parts := contextualConsoleAgentsManageCapabilityPartsForTest()
	parts.Query = query
	parts.RuntimeContext = "route=/console/agents/agent-1/agent"
	parts.SkillMode = skillModeAuto
	parts.SkillIDs = []string{
		skills.SkillAgentManagement,
		skills.SkillConsoleNavigator,
	}
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "create_agent") {
		t.Fatalf("planned_tools = %#v, want no create_agent for skill binding prompt", strategy.PlannedTools)
	}
	for _, want := range []string{"get_agent_config", "list_agent_skill_candidates", "update_agent_config"} {
		if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, want) {
			t.Fatalf("planned_tools = %#v, missing %s", strategy.PlannedTools, want)
		}
	}
	for _, unwanted := range []string{"list_agent_knowledge_candidates", "list_agent_database_candidates", "list_agent_database_tables", "list_agent_workflow_binding_candidates"} {
		if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, unwanted) {
			t.Fatalf("planned_tools = %#v, want no unrelated %s", strategy.PlannedTools, unwanted)
		}
	}
}

func TestAgentIdentityEditDoesNotPlanCreateFromHyphenatedAgentName(t *testing.T) {
	query := "请把智能体 GOAL-CREATE-SMOKE-1782961316067-EDITED 的名称改为 GOAL-CREATE-SMOKE-1782961316067-EDITED2，描述改为“AIChat fast-path 收束复测”，图标改为 🟦。"
	if agentManagementCreateRequested(query) {
		t.Fatalf("agentManagementCreateRequested(%q) = true, want false; CREATE in a hyphenated Agent name is not a create intent", query)
	}
	if !agentManagementIdentityUpdateRequested(query) {
		t.Fatalf("agentManagementIdentityUpdateRequested(%q) = false, want true", query)
	}

	parts := contextualConsoleAgentsManageCapabilityPartsForTest()
	parts.Query = query
	parts.SkillMode = skillModeAuto
	parts.SkillIDs = []string{skills.SkillAgentManagement}
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "create_agent") {
		t.Fatalf("planned_tools = %#v, want no create_agent for hyphenated Agent name edit", strategy.PlannedTools)
	}
	if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "update_agent_identity") {
		t.Fatalf("planned_tools = %#v, missing update_agent_identity", strategy.PlannedTools)
	}
}

func TestAgentIdentityEditDoesNotPlanCreateFromDescriptionValue(t *testing.T) {
	query := "\u6700\u7ec8\u590d\u6d4b\uff1a\u8bf7\u628a\u667a\u80fd\u4f53 GOAL-CREATE-SMOKE-1782961316067-EDITED2 \u7684\u540d\u79f0\u6539\u4e3a GOAL-CREATE-SMOKE-1782961316067-EDITED3\uff0c\u63cf\u8ff0\u6539\u4e3a Planner create \u8bef\u5224\u4fee\u590d\u590d\u6d4b\uff0c\u56fe\u6807\u6539\u4e3a \U0001f7e9\u3002\u4fdd\u5b58\u540e\u7528\u4e00\u53e5\u8bdd\u786e\u8ba4\u5de5\u5177\u7ed3\u679c\u548c\u9875\u9762\u53ef\u89c1\u7ed3\u679c\u3002"
	if agentManagementCreateRequested(query) {
		t.Fatalf("agentManagementCreateRequested(%q) = true, want false; create inside a description value is not a create intent", query)
	}
	if !agentManagementIdentityUpdateRequested(query) {
		t.Fatalf("agentManagementIdentityUpdateRequested(%q) = false, want true", query)
	}

	parts := contextualConsoleAgentsManageCapabilityPartsForTest()
	parts.Query = query
	parts.SkillMode = skillModeAuto
	parts.SkillIDs = []string{skills.SkillAgentManagement}
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "create_agent") {
		t.Fatalf("planned_tools = %#v, want no create_agent for identity edit description payload", strategy.PlannedTools)
	}
	if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "update_agent_identity") {
		t.Fatalf("planned_tools = %#v, missing update_agent_identity", strategy.PlannedTools)
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

func TestAgentManagementDeleteBeforeApprovalTextDoesNotImplyBatch(t *testing.T) {
	query := "\u8bf7\u5220\u9664\u5f53\u524d\u9875\u9762\u8fd9\u4e2a\u6d4b\u8bd5\u667a\u80fd\u4f53\uff0c\u6267\u884c\u524d\u6309\u6cbb\u7406\u6d41\u7a0b\u7533\u8bf7\u5ba1\u6279\uff1b\u5ba1\u6279\u901a\u8fc7\u540e\u8def\u7531\u56de\u667a\u80fd\u4f53\u5217\u8868\u9875\u3002"
	if !agentManagementDeleteRequested(query) {
		t.Fatalf("agentManagementDeleteRequested(%q) = false, want true", query)
	}
	if agentManagementBatchDeleteRequested(query) {
		t.Fatalf("agentManagementBatchDeleteRequested(%q) = true, want false; approval wording and current-page wording are not batch targets", query)
	}
}

func TestAgentManagementCurrentDetailDeletePlansSingleDeleteThenListNavigation(t *testing.T) {
	query := "\u8bf7\u5220\u9664\u5f53\u524d\u9875\u9762\u8fd9\u4e2a\u6d4b\u8bd5\u667a\u80fd\u4f53\uff0c\u4e0d\u8981\u5220\u9664\u4efb\u4f55\u5176\u4ed6\u667a\u80fd\u4f53\u3002\u6267\u884c\u524d\u6309\u6cbb\u7406\u6d41\u7a0b\u7533\u8bf7\u5ba1\u6279\uff1b\u5ba1\u6279\u901a\u8fc7\u540e\u8def\u7531\u56de\u667a\u80fd\u4f53\u5217\u8868\u9875\uff0c\u6700\u7ec8\u53ea\u7528\u4e2d\u6587\u8bf4\u660e\u5df2\u5220\u9664\u7684\u667a\u80fd\u4f53\u540d\u79f0\u3002"
	parts := consoleAgentDetailTestParts(query)
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "delete_agent") {
		t.Fatalf("PlannedTools = %#v, missing single delete_agent", strategy.PlannedTools)
	}
	if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "delete_agents") {
		t.Fatalf("PlannedTools = %#v, want no batch delete_agents for current detail single target", strategy.PlannedTools)
	}
	routeArgs := aiChatTurnStrategyPlannedToolArgumentsForTest(strategy, skills.SkillConsoleNavigator, "navigate")
	if routeArgs["href"] != "/console/agents" {
		t.Fatalf("navigate args = %#v, want href /console/agents", routeArgs)
	}

	metadata := streamingMessageMetadataWithTaskID(parts, "task-current-detail-delete")
	plan := metadata["operation_plan"].(map[string]interface{})
	deleteStepID := operationPlanToolStepID(skills.SkillAgentManagement, "delete_agent")
	routeStepID := operationPlanRouteStepID("/console/agents", 1)
	deleteIndex := operationPlanStepIndexForTest(plan, deleteStepID)
	routeIndex := operationPlanStepIndexForTest(plan, routeStepID)
	if deleteIndex < 0 || routeIndex < 0 || deleteIndex >= routeIndex {
		t.Fatalf("step order delete=%d route=%d in %#v, want delete before route", deleteIndex, routeIndex, plan["steps"])
	}
	routeStep := operationPlanStepForTest(plan, routeStepID)
	if got := stringFromAny(routeStep["wait_for"]); got != deleteStepID {
		t.Fatalf("route wait_for = %q, want %s; step=%#v", got, deleteStepID, routeStep)
	}
	if got := operationPlanStepTargetPage(routeStep); got != "/console/agents" {
		t.Fatalf("route target page = %q, want /console/agents; step=%#v", got, routeStep)
	}

	prepared := &PreparedChat{
		parts:   parts,
		Message: &runtimemodel.Message{Metadata: metadata},
	}
	allowedSkills := operationPlanAllowedSkillIDs(prepared)
	for _, want := range []string{skills.SkillAgentManagement, skills.SkillConsoleNavigator} {
		if _, ok := allowedSkills[want]; !ok {
			t.Fatalf("allowed skills = %#v, missing %s for planned delete plus post-delete navigation", allowedSkills, want)
		}
	}
	allowedTools := skillLoopAllowedPlannedTools(prepared)
	if !skillLoopToolAllowed(allowedTools, skills.SkillAgentManagement, "delete_agent") {
		t.Fatalf("allowed tools = %#v, want delete_agent ready before deletion", allowedTools)
	}
	if skillLoopToolAllowed(allowedTools, skills.SkillConsoleNavigator, "navigate") {
		t.Fatalf("allowed tools = %#v, want navigate blocked until delete_agent completes", allowedTools)
	}
	guard := skillLoopPlanToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopPlanToolCallGuard() = nil, want guard")
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillConsoleNavigator,
		ToolName:  "navigate",
		Arguments: map[string]interface{}{"href": "/console/agents"},
	}); !blocked {
		t.Fatal("console-navigator/navigate was allowed before delete_agent completed, want wait_for route guard")
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "delete_agent",
		"result": map[string]interface{}{
			"status":     "completed",
			"effect":     "deleted",
			"agent_id":   "agent-1",
			"agent_name": "Current Detail Agent",
		},
	}})
	allowedTools = skillLoopAllowedPlannedTools(prepared)
	if !skillLoopToolAllowed(allowedTools, skills.SkillConsoleNavigator, "navigate") {
		t.Fatalf("allowed tools = %#v, want navigate ready after delete_agent completes", allowedTools)
	}
	if _, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillConsoleNavigator,
		ToolName:  "navigate",
		Arguments: map[string]interface{}{"href": "/console/agents"},
	}); blocked {
		t.Fatal("console-navigator/navigate was blocked after delete_agent completed, want route guard released")
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

func TestAgentManagementPureBatchDeleteWithRefreshDoesNotPlanConfigTools(t *testing.T) {
	query := "请批量删除这两个本轮冒烟测试智能体：AICHAT-GOAL-SMOKE-1782844559803-EDITED 和 AICHAT-GOAL-BATCH-1782845202110。只删除这两个，不要删除其他智能体或工作流。请一次性批量删除，审批卡要列出这两个名称，删除后刷新并确认列表里看不到它们。需要审批时我会同意。"
	if !agentManagementDeleteRequested(query) {
		t.Fatalf("agentManagementDeleteRequested(%q) = false, want true", query)
	}
	if !agentManagementBatchDeleteRequested(query) {
		t.Fatalf("agentManagementBatchDeleteRequested(%q) = false, want true", query)
	}
	if agentManagementDeleteHasExplicitFollowupMutation(query) {
		t.Fatalf("agentManagementDeleteHasExplicitFollowupMutation(%q) = true, want false for refresh/confirmation only", query)
	}

	parts := consoleAgentsVisibleTargetsTestParts(query)
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "delete_agents") {
		t.Fatalf("PlannedTools = %#v, missing agent-management/delete_agents", strategy.PlannedTools)
	}
	for _, unexpected := range []string{
		"get_agent_config",
		"update_agent_identity",
		"update_agent_config",
		"list_available_models",
		"list_agent_skill_candidates",
		"list_agent_knowledge_candidates",
		"list_agent_database_candidates",
		"list_agent_database_tables",
		"list_agent_workflow_binding_candidates",
	} {
		if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, unexpected) {
			t.Fatalf("PlannedTools = %#v, want no stale %s for pure batch delete", strategy.PlannedTools, unexpected)
		}
	}
}

func TestAgentManagementDeleteThenFollowupMutationStillPlansAfterDelete(t *testing.T) {
	query := "删除当前页面前两个智能体，然后再创建一个新的智能体"
	if !agentManagementDeleteHasExplicitFollowupMutation(query) {
		t.Fatalf("agentManagementDeleteHasExplicitFollowupMutation(%q) = false, want true", query)
	}
	parts := consoleAgentsVisibleTargetsTestParts(query)
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "delete_agents") {
		t.Fatalf("PlannedTools = %#v, missing delete_agents", strategy.PlannedTools)
	}
	if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "create_agent") {
		t.Fatalf("PlannedTools = %#v, missing create_agent for explicit follow-up create", strategy.PlannedTools)
	}
}

func TestAgentManagementExplicitVisibleAgentDetailPlansNavigate(t *testing.T) {
	parts := consoleAgentsVisibleTargetsTestParts("open Visible Agent One detail page and edit its description")
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	if strategy.TargetPage != "/console/agents/agent-1/agent" || !strategy.RouteRequired {
		t.Fatalf("target/route = %q/%v, want visible Agent detail route; strategy=%#v", strategy.TargetPage, strategy.RouteRequired, strategy)
	}
	if strategy.RequiredNextTool == nil || strategy.RequiredNextTool.SkillID != skills.SkillConsoleNavigator ||
		strategy.RequiredNextTool.ToolName != "navigate" ||
		strategy.RequiredNextTool.Arguments["href"] != "/console/agents/agent-1/agent" {
		t.Fatalf("RequiredNextTool = %#v, want navigate to visible Agent detail", strategy.RequiredNextTool)
	}
	if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "update_agent_identity") {
		t.Fatalf("PlannedTools = %#v, missing agent-management/update_agent_identity", strategy.PlannedTools)
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
		"Do not call list_agents only to verify that same single Agent",
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

func aiChatTurnStrategyPlannedToolArgumentsForTest(strategy *AIChatTurnStrategy, skillID, toolName string) map[string]string {
	if strategy == nil {
		return nil
	}
	for _, tool := range strategy.PlannedTools {
		if tool.SkillID == skillID && tool.ToolName == toolName {
			return tool.Arguments
		}
	}
	return nil
}

func aiChatTurnStrategyHasPlannedToolStepIDForTest(strategy *AIChatTurnStrategy, stepID string) bool {
	if strategy == nil {
		return false
	}
	for _, tool := range strategy.PlannedTools {
		if strings.TrimSpace(tool.StepID) == stepID {
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

func consoleAgentDetailTestParts(query string) *chatRequestParts {
	context := map[string]interface{}{
		"resources": []interface{}{
			map[string]interface{}{
				"resource_type": "page",
				"href":          "/console/agents/agent-1/agent",
			},
			map[string]interface{}{
				"resource_type": "agent",
				"id":            "agent-1",
				"title":         "Current Detail Agent",
				"href":          "/console/agents/agent-1/agent",
				"selected":      true,
				"visible_index": 1,
			},
		},
	}
	return &chatRequestParts{
		Query:               query,
		Surface:             aiChatSurfaceContextualSidebar,
		RuntimeContext:      "route=/console/agents/agent-1/agent",
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

func TestOperationPlanRecordsUnplannedReadOnlyInvocationDeviation(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{map[string]interface{}{
				"id":        operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"),
				"status":    operationPlanStepStatusPending,
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
				"asset_target": map[string]interface{}{
					"effect":     "update",
					"asset_type": "agent",
				},
			}},
			"step_status": map[string]interface{}{
				operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"): operationPlanStepStatusPending,
			},
		},
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{{
		"kind":       "tool_call",
		"runtime_id": "tool:agent-candidates",
		"status":     "success",
		"skill_id":   skills.SkillAgentManagement,
		"tool_name":  "list_agent_skill_candidates",
		"result": map[string]interface{}{
			"status": "completed",
			"count":  1,
		},
	}})

	plan := metadata["operation_plan"].(map[string]interface{})
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")); got != operationPlanStepStatusPending {
		t.Fatalf("update_agent_config status = %#v, want pending; plan=%#v", got, plan)
	}
	deviations := mapSliceFromAny(plan["deviations"])
	if len(deviations) != 1 {
		t.Fatalf("deviations = %#v, want one read-only deviation", plan["deviations"])
	}
	deviation := deviations[0]
	if got := stringFromAny(deviation["tool_name"]); got != "list_agent_skill_candidates" {
		t.Fatalf("deviation tool_name = %#v, want list_agent_skill_candidates", got)
	}
	if got := stringFromAny(deviation["reason"]); got != "model_collected_unplanned_readonly_evidence" {
		t.Fatalf("deviation reason = %#v, want read-only evidence reason", got)
	}
	if got := stringFromAny(deviation["outcome"]); got != "allowed" {
		t.Fatalf("deviation outcome = %#v, want allowed", got)
	}
	if got := plan["status"]; got != operationPlanStatusRunning {
		t.Fatalf("plan status = %#v, want running; plan=%#v", got, plan)
	}
	state := mapFromOperationContext(plan["strategy_state"])
	if got := intValueFromAny(state["plan_deviation_count"]); got != 1 {
		t.Fatalf("strategy_state.plan_deviation_count = %#v, want 1; state=%#v", state["plan_deviation_count"], state)
	}
	if latest := mapFromOperationContext(state["last_plan_deviation"]); latest["tool_name"] != "list_agent_skill_candidates" {
		t.Fatalf("strategy_state.last_plan_deviation = %#v, want list_agent_skill_candidates", latest)
	}
	if steps := mapSliceFromAny(state["plan_steps"]); len(steps) != 1 {
		t.Fatalf("strategy_state.plan_steps = %#v, want original pending update step", state["plan_steps"])
	}
}

func TestOperationPlanRecordsUnplannedNavigationDeviation(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"steps": []interface{}{map[string]interface{}{
				"id":        operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"),
				"status":    operationPlanStepStatusPending,
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
				"asset_target": map[string]interface{}{
					"effect":     "update",
					"asset_type": "agent",
				},
			}},
			"step_status": map[string]interface{}{
				operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"): operationPlanStepStatusPending,
			},
		},
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{{
		"kind":      "client_action",
		"action_id": "route-files-unplanned",
		"status":    "succeeded",
		"skill_id":  skills.SkillConsoleNavigator,
		"tool_name": "navigate",
		"href":      "/console/files",
	}})

	plan := metadata["operation_plan"].(map[string]interface{})
	if got := plan["current_page"]; got != "/console/files" {
		t.Fatalf("current_page = %#v, want /console/files; plan=%#v", got, plan)
	}
	if got := operationPlanStepStatusForTest(plan, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")); got != operationPlanStepStatusPending {
		t.Fatalf("update_agent_config status = %#v, want pending; plan=%#v", got, plan)
	}
	deviations := mapSliceFromAny(plan["deviations"])
	if len(deviations) != 1 {
		t.Fatalf("deviations = %#v, want one navigation deviation", plan["deviations"])
	}
	deviation := deviations[0]
	if got := stringFromAny(deviation["tool_name"]); got != "navigate" {
		t.Fatalf("deviation tool_name = %#v, want navigate", got)
	}
	if got := stringFromAny(deviation["reason"]); got != "model_navigated_for_page_context_within_user_goal" {
		t.Fatalf("deviation reason = %#v, want navigation reason", got)
	}
	if got := stringFromAny(deviation["outcome"]); got != "allowed" {
		t.Fatalf("deviation outcome = %#v, want allowed", got)
	}
	state := mapFromOperationContext(plan["strategy_state"])
	if got := intValueFromAny(state["plan_deviation_count"]); got != 1 {
		t.Fatalf("strategy_state.plan_deviation_count = %#v, want 1; state=%#v", state["plan_deviation_count"], state)
	}
	if got := stringFromAny(state["current_page"]); got != "/console/files" {
		t.Fatalf("strategy_state.current_page = %#v, want /console/files; state=%#v", got, state)
	}
	if latest := mapFromOperationContext(state["last_plan_deviation"]); latest["tool_name"] != "navigate" {
		t.Fatalf("strategy_state.last_plan_deviation = %#v, want navigate", latest)
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

func stringSliceContains(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
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

func TestApplyOperationPlanCompletionVerificationResultFailsPendingExecutableStep(t *testing.T) {
	updateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"version":             operationPlanVersion,
			"status":              operationPlanStatusRunning,
			"pending_next_action": "Run tool:agent-management/update_agent_config",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_identity",
					"status":    operationPlanStepStatusCompleted,
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_identity",
				},
				map[string]interface{}{
					"id":        updateStepID,
					"status":    operationPlanStepStatusPending,
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_config",
				},
			},
			"step_status": map[string]interface{}{
				"tool:agent-management/update_agent_identity": operationPlanStepStatusCompleted,
				updateStepID: operationPlanStepStatusPending,
			},
		},
	}

	applyOperationPlanCompletionVerificationResult(
		metadata,
		"failed",
		"update_agent_config was not executed",
		[]string{"tool:agent-management/update_agent_config"},
		[]string{"configuration was updated"},
		"call update_agent_config",
	)

	plan := mapFromOperationContext(metadata["operation_plan"])
	if got := stringFromAny(plan["status"]); got != operationPlanStatusFailed {
		t.Fatalf("operation_plan status = %q, want failed; plan=%#v", got, plan)
	}
	if got := stringFromAny(plan["pending_next_action"]); got != "none" {
		t.Fatalf("pending_next_action = %q, want none; plan=%#v", got, plan)
	}
	if got := operationPlanStepStatusForTest(plan, updateStepID); got != operationPlanStepStatusFailed {
		t.Fatalf("%s status = %q, want failed; plan=%#v", updateStepID, got, plan)
	}
	state := mapFromOperationContext(plan["strategy_state"])
	if got := stringFromAny(state["status"]); got != operationPlanStatusFailed {
		t.Fatalf("strategy_state.status = %q, want failed; state=%#v", got, state)
	}
	if got := intValueFromAny(state["failed_step_count"]); got != 1 {
		t.Fatalf("strategy_state.failed_step_count = %#v, want 1; state=%#v", state["failed_step_count"], state)
	}
	verification := mapFromOperationContext(plan["completion_verification"])
	if got := stringFromAny(verification["status"]); got != "failed" {
		t.Fatalf("completion_verification.status = %q, want failed", got)
	}
	if missing := stringSliceFromAny(verification["missing_steps"]); len(missing) != 1 || missing[0] != "tool:agent-management/update_agent_config" {
		t.Fatalf("completion_verification.missing_steps = %#v, want update_agent_config", missing)
	}
	if pending := operationPlanPendingExecutableSteps(plan, 4); len(pending) != 0 {
		t.Fatalf("pending executable steps = %#v, want none after verifier terminal failure", pending)
	}
}

func TestApplyOperationPlanCompletionVerificationResultKeepsNeedsActionStepPending(t *testing.T) {
	updateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"version":             operationPlanVersion,
			"status":              operationPlanStatusRunning,
			"pending_next_action": "Run tool:agent-management/update_agent_config",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_identity",
					"status":    operationPlanStepStatusCompleted,
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_identity",
				},
				map[string]interface{}{
					"id":        updateStepID,
					"status":    operationPlanStepStatusPending,
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_config",
					"error":     "stale verifier error",
				},
			},
			"step_status": map[string]interface{}{
				"tool:agent-management/update_agent_identity": operationPlanStepStatusCompleted,
				updateStepID: operationPlanStepStatusPending,
			},
		},
	}

	applyOperationPlanCompletionVerificationResult(
		metadata,
		"needs_action",
		"missing update_agent_config evidence",
		[]string{"Run tool:agent-management/update_agent_config"},
		[]string{"configuration was updated"},
		"call update_agent_config",
	)

	plan := mapFromOperationContext(metadata["operation_plan"])
	if got := stringFromAny(plan["status"]); got != operationPlanStatusRunning {
		t.Fatalf("operation_plan status = %q, want running; plan=%#v", got, plan)
	}
	if got := stringFromAny(plan["pending_next_action"]); got != updateStepID {
		t.Fatalf("pending_next_action = %q, want update step; plan=%#v", got, plan)
	}
	if got := operationPlanStepStatusForTest(plan, updateStepID); got != operationPlanStepStatusPending {
		t.Fatalf("%s status = %q, want pending; plan=%#v", updateStepID, got, plan)
	}
	if got := operationPlanStepFieldForTest(plan, updateStepID, "error"); got != "" {
		t.Fatalf("%s error = %q, want cleared for needs_action", updateStepID, got)
	}
	verification := mapFromOperationContext(plan["completion_verification"])
	if got := stringFromAny(verification["status"]); got != "needs_action" {
		t.Fatalf("completion_verification.status = %q, want needs_action", got)
	}
	if pending := operationPlanPendingExecutableSteps(plan, 4); len(pending) != 1 {
		t.Fatalf("pending executable steps = %#v, want update_agent_config after needs_action", pending)
	}
}

func TestApplyOperationPlanCompletionVerificationResultKeepsCompletedPlanCompleted(t *testing.T) {
	updateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"version":             operationPlanVersion,
			"status":              operationPlanStatusCompleted,
			"pending_next_action": "none",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        updateStepID,
					"status":    operationPlanStepStatusCompleted,
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_config",
				},
			},
			"step_status": map[string]interface{}{
				updateStepID: operationPlanStepStatusCompleted,
			},
		},
	}

	applyOperationPlanCompletionVerificationResult(
		metadata,
		"failed",
		"candidate answer included unsupported extra wording",
		nil,
		[]string{"extra unsupported claim"},
		"",
	)

	plan := mapFromOperationContext(metadata["operation_plan"])
	if got := stringFromAny(plan["status"]); got != operationPlanStatusCompleted {
		t.Fatalf("operation_plan status = %q, want completed; plan=%#v", got, plan)
	}
	if got := stringFromAny(plan["pending_next_action"]); got != "none" {
		t.Fatalf("pending_next_action = %q, want none; plan=%#v", got, plan)
	}
	verification := mapFromOperationContext(plan["completion_verification"])
	if got := stringFromAny(verification["status"]); got != "failed" {
		t.Fatalf("completion_verification.status = %q, want recorded failed verifier", got)
	}
}

func TestEnsureOperationPlanInvocationStepRecordsUnplannedReadAsDeviation(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"version":             operationPlanVersion,
			"intent":              "manage_agent_asset",
			"status":              operationPlanStatusCompleted,
			"pending_next_action": "none",
			"steps": []interface{}{
				map[string]interface{}{
					"id":     "observe",
					"title":  "Observe result",
					"status": operationPlanStepStatusCompleted,
				},
			},
			"step_status": map[string]interface{}{
				"observe": operationPlanStepStatusCompleted,
			},
		},
	}

	ensureOperationPlanInvocationStep(metadata, map[string]interface{}{
		"kind":      "tool_call",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "get_agent_config",
		"status":    "success",
		"result": map[string]interface{}{
			"status":     "success",
			"agent_id":   "agent-1",
			"agent_name": "Support Agent",
		},
	})

	plan := mapFromOperationContext(metadata["operation_plan"])
	stepID := operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config")
	if got := operationPlanStepStatusForTest(plan, stepID); got != "" {
		t.Fatalf("%s step status = %q, want no appended blocking step; plan=%#v", stepID, got, plan)
	}
	if got := stringFromAny(plan["status"]); got != operationPlanStatusCompleted {
		t.Fatalf("operation_plan status = %q, want completed", got)
	}
	deviations := mapSliceFromAny(plan["deviations"])
	if len(deviations) != 1 {
		t.Fatalf("deviations = %#v, want one unplanned read deviation", deviations)
	}
	if got := stringFromAny(deviations[0]["reason"]); got != "model_collected_unplanned_readonly_evidence" {
		t.Fatalf("deviation reason = %q, want readonly evidence reason", got)
	}
	result := mapFromOperationContext(plan["tool_result"])
	if result["tool_name"] != "get_agent_config" || result["status"] != "success" {
		t.Fatalf("tool_result = %#v, want get_agent_config success summary", result)
	}
}

func TestEnsureOperationPlanInvocationStepRecordsUnplannedMutationAsAmendment(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"version":             operationPlanVersion,
			"intent":              "manage_agent_asset",
			"status":              operationPlanStatusCompleted,
			"pending_next_action": "none",
			"steps": []interface{}{
				map[string]interface{}{
					"id":     "observe",
					"title":  "Observe result",
					"status": operationPlanStepStatusCompleted,
				},
			},
			"step_status": map[string]interface{}{
				"observe": operationPlanStepStatusCompleted,
			},
		},
	}

	ensureOperationPlanInvocationStep(metadata, map[string]interface{}{
		"kind":      "tool_call",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "update_agent_config",
		"status":    "success",
		"result": map[string]interface{}{
			"status":         "success",
			"agent_id":       "agent-1",
			"agent_name":     "Support Agent",
			"updated_fields": []interface{}{"model", "model_provider"},
		},
	})

	plan := mapFromOperationContext(metadata["operation_plan"])
	stepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")
	if got := operationPlanStepStatusForTest(plan, stepID); got != operationPlanStepStatusCompleted {
		t.Fatalf("%s step status = %q, want completed; plan=%#v", stepID, got, plan)
	}
	if !operationPlanBoolValue(plan["amended"]) {
		t.Fatalf("operation_plan amended = %#v, want true; plan=%#v", plan["amended"], plan)
	}
	amendments := mapSliceFromAny(plan["amendments"])
	if len(amendments) != 1 {
		t.Fatalf("amendments = %#v, want one runtime amendment", amendments)
	}
	if got := stringFromAny(amendments[0]["step_id"]); got != stepID {
		t.Fatalf("amendment step_id = %q, want %q", got, stepID)
	}
	if got := stringFromAny(amendments[0]["reason"]); got != "runtime_recorded_unplanned_tool_step" {
		t.Fatalf("amendment reason = %q, want runtime_recorded_unplanned_tool_step", got)
	}
	deviations := mapSliceFromAny(plan["deviations"])
	if len(deviations) != 0 {
		t.Fatalf("deviations = %#v, want mutation recorded as amendment rather than exploratory deviation", deviations)
	}
	completed := mapSliceFromAny(plan["completed_steps"])
	found := false
	for _, record := range completed {
		if stringFromAny(record["id"]) == stepID && stringFromAny(record["reason"]) == "runtime_recorded_unplanned_tool_step" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("completed_steps = %#v, want amended runtime update step record", completed)
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
	if finalAnswerGuardHasAgentDeleteCall(legacyCalls, "agent-legacy") {
		t.Fatal("batch delete guard matched frozen arguments without item evidence")
	}
}
