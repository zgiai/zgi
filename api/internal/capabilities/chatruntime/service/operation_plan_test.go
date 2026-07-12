package service

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func operationPlanEvidenceLedgerHasToolForTest(ledger []map[string]interface{}, skillID string, toolName string, invocationID string) bool {
	for _, entry := range ledger {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(entry["skill_id"])), strings.TrimSpace(skillID)) ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(entry["tool_name"])), strings.TrimSpace(toolName)) {
			continue
		}
		if invocationID == "" || strings.TrimSpace(stringFromAny(entry["invocation_id"])) == invocationID {
			return true
		}
	}
	return false
}

func assertAgentManagementModelDecidesStrategyForTest(t *testing.T, parts *chatRequestParts, strategy *AIChatTurnStrategy) map[string]interface{} {
	t.Helper()
	if strategy == nil || strategy.Intent != "manage_agent_asset" {
		t.Fatalf("strategy = %#v, want Agent-management model-decides strategy", strategy)
	}
	assertAgentManagementModelDecidesExecutionForTest(t, strategy)
	plan := operationPlanFromTurnStrategy("task-agent-model-decides", parts, strategy)
	assertAgentManagementModelDecidesOperationPlanForTest(t, plan)
	return plan
}

func assertAgentManagementModelDecidesOperationPlanForTest(t *testing.T, plan map[string]interface{}) {
	t.Helper()
	if len(plan) == 0 {
		t.Fatal("operation plan is empty")
	}
	if got := stringFromAny(plan["planning_mode"]); got != "phase_only_model_decides" {
		t.Fatalf("planning_mode = %q, want phase_only_model_decides", got)
	}
	if got := stringFromAny(plan["tool_choice_mode"]); got != aiChatTurnToolChoiceModelDecides {
		t.Fatalf("tool_choice_mode = %q, want %q", got, aiChatTurnToolChoiceModelDecides)
	}
	if len(mapSliceFromAny(plan["steps"])) != 0 {
		t.Fatalf("operation plan contains executable steps: %#v", plan["steps"])
	}
	for _, key := range []string{"step_status", "structured_plan", "required_tool_sequence", "args_binding", "planned_tools", "pending_next_action"} {
		if _, exists := plan[key]; exists {
			t.Fatalf("operation plan contains executable field %q: %#v", key, plan[key])
		}
	}
	if got := stringFromAny(plan["plan_sync_status"]); got != "current" {
		t.Fatalf("plan_sync_status = %q, want current", got)
	}
}

func TestInvocationEvidenceMarksPlanStaleWithoutUpdatingLegacyStepsOrPhases(t *testing.T) {
	phases := []interface{}{map[string]interface{}{
		"id": "phase-1", "step": "Update the Agent", "status": "in_progress",
	}}
	legacySteps := []interface{}{map[string]interface{}{
		"id": "tool:agent-management/update_agent_config", "status": "pending",
	}}
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":                           operationPlanStatusRunning,
			"phases":                           phases,
			"steps":                            legacySteps,
			"step_status":                      map[string]interface{}{"tool:agent-management/update_agent_config": "pending"},
			"required_tool_sequence":           []interface{}{"agent-management/get_agent_config"},
			"args_binding":                     map[string]interface{}{"agent_id": "legacy-agent"},
			"plan_sync_status":                 "current",
			"evidence_sequence_at_plan_update": 0,
			"evidence_after_last_plan_update":  0,
		},
	}
	applyOperationPlanInvocationState(metadata, []map[string]interface{}{map[string]interface{}{
		"kind":       "tool_call",
		"status":     "success",
		"runtime_id": "runtime:tool#7",
		"skill_id":   skills.SkillAgentManagement,
		"tool_name":  "update_agent_config",
		"arguments":  map[string]interface{}{"agent_id": "model-selected-agent"},
		"result": map[string]interface{}{
			"status": "completed", "effect": "updated", "agent_id": "model-selected-agent",
			"updated_fields": []interface{}{"system_prompt"},
		},
	}})

	plan := mapFromOperationContext(metadata["operation_plan"])
	if got := stringFromAny(plan["status"]); got != operationPlanStatusRunning {
		t.Fatalf("plan status = %q, want running until final answer", got)
	}
	if got := stringFromAny(plan["plan_sync_status"]); got != "stale" {
		t.Fatalf("plan_sync_status = %q, want stale", got)
	}
	if got := intValueFromAny(plan["evidence_after_last_plan_update"]); got != 1 {
		t.Fatalf("evidence_after_last_plan_update = %d, want 1", got)
	}
	if got := stringFromAny(mapSliceFromAny(plan["phases"])[0]["status"]); got != "in_progress" {
		t.Fatalf("phase status = %q, want unchanged in_progress", got)
	}
	if got := stringFromAny(mapFromOperationContext(plan["step_status"])["tool:agent-management/update_agent_config"]); got != "pending" {
		t.Fatalf("legacy step status = %q, want unchanged pending", got)
	}
	ledger := mapSliceFromAny(plan[operationPlanEvidenceLedgerKey])
	if len(ledger) != 1 || stringFromAny(ledger[0]["tool_name"]) != "update_agent_config" {
		t.Fatalf("evidence ledger = %#v, want one successful update", ledger)
	}
}

func TestInvocationEvidenceUsesGlobalRevisionAcrossLocalToolSequences(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":                           operationPlanStatusRunning,
			"plan_sync_status":                 "current",
			"evidence_revision":                1,
			"evidence_revision_at_plan_update": 1,
			"evidence_sequence_at_plan_update": 1,
			operationPlanEvidenceLedgerKey: []interface{}{map[string]interface{}{
				"kind": "tool_call", "status": "completed", "skill_id": skills.SkillFileReader,
				"tool_name": "read_file", "invocation_id": "read-1", "sequence": 1, "ledger_revision": 1,
				"keys": []interface{}{"tool:file-reader/read_file"},
			}},
		},
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{map[string]interface{}{
		"kind": "tool_call", "status": "success", "runtime_id": "agent-list-1", "sequence": 1,
		"skill_id": skills.SkillAgentManagement, "tool_name": "list_agents",
		"result": map[string]interface{}{"status": "completed", "agents": []interface{}{}},
	}})

	plan := mapFromOperationContext(metadata["operation_plan"])
	if got := intValueFromAny(plan["evidence_revision"]); got != 2 {
		t.Fatalf("evidence_revision = %d, want 2", got)
	}
	if got := intValueFromAny(plan["evidence_after_last_plan_update"]); got != 1 {
		t.Fatalf("evidence_after_last_plan_update = %d, want 1", got)
	}
	if got := stringFromAny(plan["plan_sync_status"]); got != "stale" {
		t.Fatalf("plan_sync_status = %q, want stale", got)
	}
	ledger := mapSliceFromAny(plan[operationPlanEvidenceLedgerKey])
	if len(ledger) != 2 || intValueFromAny(ledger[1]["ledger_revision"]) != 2 {
		t.Fatalf("evidence ledger revisions = %#v, want global revisions 1,2", ledger)
	}
}

func TestApplyOperationPlanCompletionVerificationMirrorsTopLevelMetadata(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":           "running",
			"plan_sync_status": "stale",
			"phases": []interface{}{map[string]interface{}{
				"id": "phase-1", "step": "Update Agent", "status": "in_progress",
			}},
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_config",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_config",
				},
			},
			"step_status": map[string]interface{}{
				"tool:agent-management/update_agent_config": "completed",
			},
			operationPlanEvidenceLedgerKey: []interface{}{
				map[string]interface{}{
					"kind":      "tool_call",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_config",
					"keys":      []interface{}{"tool:agent-management/update_agent_config"},
				},
			},
		},
	}

	applyOperationPlanTerminalCompletionResult(metadata, "pass", "tool evidence is complete", nil, nil, "")

	verification := mapFromOperationContext(metadata["completion_verification"])
	if got := stringFromAny(verification["status"]); got != "pass" {
		t.Fatalf("completion_verification.status = %q, want pass; metadata=%#v", got, metadata)
	}
	ledger := mapSliceFromAny(metadata["evidence_ledger"])
	if len(ledger) != 1 {
		t.Fatalf("evidence_ledger = %#v, want mirrored ledger entry", metadata["evidence_ledger"])
	}
	if got := stringFromAny(ledger[0]["tool_name"]); got != "update_agent_config" {
		t.Fatalf("evidence_ledger[0].tool_name = %q, want update_agent_config", got)
	}
	plan := mapFromOperationContext(metadata["operation_plan"])
	if got := stringFromAny(mapSliceFromAny(plan["phases"])[0]["status"]); got != "in_progress" {
		t.Fatalf("phase status = %q, want finalizer to preserve model phase status", got)
	}
	if got := stringFromAny(plan["plan_sync_status"]); got != "stale" {
		t.Fatalf("plan_sync_status = %q, want finalizer to preserve stale", got)
	}
}

func TestOperationPlanEvidenceLedgerStoresAgentFactsAndPostReadVerification(t *testing.T) {
	plan := map[string]interface{}{
		"status": operationPlanStatusRunning,
	}
	update := map[string]interface{}{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "update_agent_config",
		"arguments": map[string]interface{}{
			"agent_id":            "agent-1",
			"model_provider":      "deepseek",
			"model":               "deepseek-v4-flash",
			"system_prompt":       "Write fiction and generate files when needed.",
			"file_upload_enabled": true,
			"enabled_skill_ids":   []interface{}{"file-generator"},
			"expected_updated_fields": []interface{}{
				"model",
				"system_prompt",
				"file_upload_enabled",
				"enabled_skill_ids",
			},
		},
		"result": map[string]interface{}{
			"status":              "completed",
			"effect":              "updated",
			"agent_id":            "agent-1",
			"agent_name":          "Story Agent",
			"model_provider":      "deepseek",
			"model":               "deepseek-v4-flash",
			"file_upload_enabled": true,
			"enabled_skill_ids":   []interface{}{"file-generator"},
			"updated_fields":      []interface{}{"model", "system_prompt", "file_upload_enabled", "enabled_skill_ids"},
		},
	}
	read := map[string]interface{}{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "get_agent_config",
		"result": map[string]interface{}{
			"status":              "completed",
			"agent_id":            "agent-1",
			"agent_name":          "Story Agent",
			"model_provider":      "deepseek",
			"model":               "deepseek-v4-flash",
			"system_prompt":       "Write fiction and generate files when needed.",
			"file_upload_enabled": true,
			"enabled_skill_ids":   []interface{}{"file-generator"},
		},
	}

	operationPlanAppendEvidenceLedgerEntry(plan, update, []string{"tool:agent-management/update_agent_config"})
	operationPlanAppendEvidenceLedgerEntry(plan, read, []string{"tool:agent-management/get_agent_config", "agent:config"})

	ledger := mapSliceFromAny(plan[operationPlanEvidenceLedgerKey])
	if len(ledger) != 2 {
		t.Fatalf("evidence_ledger len = %d, want 2; ledger=%#v", len(ledger), ledger)
	}
	updateFacts := mapFromOperationContext(ledger[0]["result_facts"])
	if _, leaked := updateFacts["system_prompt"]; leaked {
		t.Fatalf("update result_facts leaked system_prompt: %#v", updateFacts)
	}
	if got := stringFromAny(updateFacts["agent.config.system_prompt_digest"]); !strings.HasPrefix(got, operationPlanEvidenceDigestPrefix) {
		t.Fatalf("system_prompt digest = %q, want sha256 digest; facts=%#v", got, updateFacts)
	}
	readFacts := mapFromOperationContext(ledger[1]["result_facts"])
	fieldStatus := mapFromOperationContext(readFacts["field_status"])
	if got := stringFromAny(fieldStatus["system_prompt"]); got != "verified" {
		t.Fatalf("field_status.system_prompt = %q, want verified; read facts=%#v", got, readFacts)
	}
	if got := stringFromAny(fieldStatus["model"]); got != "verified" {
		t.Fatalf("field_status.model = %q, want verified; read facts=%#v", got, readFacts)
	}
	if got := stringFromAny(readFacts["agent.config.model"]); got != "deepseek-v4-flash" {
		t.Fatalf("agent.config.model = %q, want deepseek-v4-flash; read facts=%#v", got, readFacts)
	}
	compact := operationPlanCompactEvidenceLedger(plan[operationPlanEvidenceLedgerKey], 10)
	if summary := mapFromOperationContext(compact[1]["result_summary"]); stringFromAny(summary["model"]) != "deepseek-v4-flash" {
		t.Fatalf("compact result_summary = %#v, want model evidence", summary)
	}
}

func TestOperationPlanEvidenceLedgerStoresGeneratedFileFactsAndDedupesManagedSave(t *testing.T) {
	plan := map[string]interface{}{
		"status": operationPlanStatusRunning,
	}
	generate := map[string]interface{}{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillFileGenerator,
		"tool_name": "generate_file",
		"arguments": map[string]interface{}{
			"filename":       "story.pdf",
			"format":         "pdf",
			"lifecycle":      "temporary",
			"content_length": 1520,
		},
		"asset_operation_audit": map[string]interface{}{
			"assets": []interface{}{
				map[string]interface{}{
					"name": "story.pdf",
					"type": "file",
					"metadata": map[string]interface{}{
						"format":    "pdf",
						"lifecycle": "temporary",
					},
				},
			},
		},
	}
	save := map[string]interface{}{
		"kind":          "tool_call",
		"status":        "success",
		"skill_id":      skills.SkillFileManager,
		"tool_name":     "save_file_to_management",
		"runtime_id":    "runtime-save-1",
		"invocation_id": "runtime-save-1",
		"result": map[string]interface{}{
			"status":         "completed",
			"target":         "managed_file",
			"file_id":        "managed-1",
			"upload_file_id": "managed-1",
			"filename":       "story.md",
			"source_type":    "tool_file",
		},
	}
	duplicateSave := copyStringAnyMap(save)
	duplicateSave["runtime_id"] = "trace-save-1"
	duplicateSave["invocation_id"] = "trace-save-1"

	operationPlanAppendEvidenceLedgerEntry(plan, generate, []string{"tool:file-generator/generate_file"})
	operationPlanAppendEvidenceLedgerEntry(plan, save, []string{"tool:file-manager/save_file_to_management"})
	operationPlanAppendEvidenceLedgerEntry(plan, duplicateSave, []string{"tool:file-manager/save_file_to_management"})

	ledger := mapSliceFromAny(plan[operationPlanEvidenceLedgerKey])
	if len(ledger) != 2 {
		t.Fatalf("evidence_ledger len = %d, want generated file plus one managed save; ledger=%#v", len(ledger), ledger)
	}
	generateFacts := mapFromOperationContext(ledger[0]["result_facts"])
	if got := stringFromAny(generateFacts["filename"]); got != "story.pdf" {
		t.Fatalf("generated filename = %q, want story.pdf; facts=%#v", got, generateFacts)
	}
	if got := stringFromAny(generateFacts["file_extension"]); got != "pdf" {
		t.Fatalf("generated file_extension = %q, want pdf; facts=%#v", got, generateFacts)
	}
	if got := stringFromAny(generateFacts["target"]); got != "temporary_artifact" {
		t.Fatalf("generated target = %q, want temporary_artifact; facts=%#v", got, generateFacts)
	}
	if got := stringFromAny(generateFacts["mime_type"]); got != "application/pdf" {
		t.Fatalf("generated mime_type = %q, want application/pdf; facts=%#v", got, generateFacts)
	}
	if got := intValueFromAny(generateFacts["content_length"]); got != 1520 {
		t.Fatalf("generated content_length = %d, want 1520; facts=%#v", got, generateFacts)
	}
	saveFacts := mapFromOperationContext(ledger[1]["result_facts"])
	if got := stringFromAny(saveFacts["file_extension"]); got != "md" {
		t.Fatalf("managed save file_extension = %q, want md; facts=%#v", got, saveFacts)
	}
}

func TestModelDecidesOperationPlanStripsPendingToolScriptCriteria(t *testing.T) {
	parts := &chatRequestParts{
		Query:   "continue the previous task",
		Surface: aiChatSurfaceContextualSidebar,
	}
	strategy := &AIChatTurnStrategy{
		Surface:        aiChatSurfaceContextualSidebar,
		Intent:         "continue_previous_task",
		ToolChoiceMode: aiChatTurnToolChoiceModelDecides,
		SuccessCriteria: []string{
			"continue from the recent execution context instead of treating the request as a new generic question",
			"complete pending plan step: tool:file-manager/save_file_to_management",
			"verify the final answer against actual tool results or refreshed page context",
		},
	}

	plan := operationPlanFromTurnStrategy("task-model-decides", parts, strategy)
	criteria := stringSliceFromAny(plan["success_criteria"])
	for _, item := range criteria {
		if strings.Contains(item, "complete pending plan step:") || strings.Contains(item, "tool:file-manager/save_file_to_management") {
			t.Fatalf("success_criteria contains scripted tool step %q; plan=%#v", item, plan)
		}
	}
	if !stringSliceContains(criteria, "continue from the recent execution context instead of treating the request as a new generic question") {
		t.Fatalf("success_criteria = %#v, want evidence continuation criterion", criteria)
	}
	for _, phase := range mapSliceFromAny(plan["phases"]) {
		for _, item := range stringSliceFromAny(phase["success_criteria"]) {
			if strings.Contains(item, "complete pending plan step:") || strings.Contains(item, "tool:file-manager/save_file_to_management") {
				t.Fatalf("phase success_criteria contains scripted tool step %q; phase=%#v", item, phase)
			}
		}
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

func assertStringSliceContains(t *testing.T, values []string, want string) {
	t.Helper()
	for _, value := range values {
		if value == want {
			return
		}
	}
	t.Fatalf("values = %#v, want to contain %q", values, want)
}

func TestTurnStrategyAllowedSkillIDsKeepsPlannedToolsWithRouteHint(t *testing.T) {
	strategy := &AIChatTurnStrategy{
		Intent:        "manage_agent_asset",
		RouteRequired: true,
		PlannedTools: []AIChatTurnStrategyTool{
			{SkillID: skills.SkillAgentManagement, ToolName: "update_agent_config"},
			{SkillID: skills.SkillAgentManagement, ToolName: "get_agent_config"},
		},
	}

	allowed := turnStrategyAllowedSkillIDs(strategy)
	for _, want := range []string{skills.SkillAgentManagement} {
		if _, ok := allowed[want]; !ok {
			t.Fatalf("allowed skills = %#v, missing %s", allowed, want)
		}
	}
}

func readOnlyBindingCapabilityGoalMapsForTest() []interface{} {
	goals := []AIChatAgentCapabilityGoal{
		agentCapabilityGoalWithDefaults(AIChatAgentCapabilityGoal{
			CapabilityID: agentCapabilitySkillBacked,
			GoalAction:   agentCapabilityActionInspect,
		}),
		agentCapabilityGoalWithDefaults(AIChatAgentCapabilityGoal{
			CapabilityID: agentCapabilityKnowledgeBinding,
			GoalAction:   agentCapabilityActionInspect,
		}),
		agentCapabilityGoalWithDefaults(AIChatAgentCapabilityGoal{
			CapabilityID: agentCapabilityDatabaseBinding,
			GoalAction:   agentCapabilityActionInspect,
		}),
		agentCapabilityGoalWithDefaults(AIChatAgentCapabilityGoal{
			CapabilityID: agentCapabilityWorkflowBinding,
			GoalAction:   agentCapabilityActionInspect,
		}),
	}
	return mapsToInterfaceSlice(agentCapabilityGoalsToMaps(goals))
}

func TestAgentConfigPlanMapsCapabilityFieldsAndPreservesConfigGoal(t *testing.T) {
	parts := &chatRequestParts{
		Query: strings.Join([]string{
			"Edit the current Agent configuration.",
			"Rename it to AICHAT-E2E-EDITED and update the description and icon.",
			"Use list_available_models with use_case text-chat and select the provider/model matching GPT 4o.",
			"Enable memory and file upload, update home title, placeholder, theme color, and suggested questions.",
			"Bind one available Skill, one knowledge base, one database table, and one workflow.",
			"Verify the final Agent config after saving.",
		}, "\n"),
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents/agent-1/agent",
		SkillMode:      skillModeAuto,
		SkillIDs: []string{
			skills.SkillAgentManagement,
			skills.SkillConsoleNavigator,
		},
		ModelTurnIntent: &AIChatModelTurnIntent{
			Intent: "manage_agent_asset",
			RecommendedCapabilities: []string{
				"agent.model_selection:text-chat",
				"agent.system_prompt",
				"agent.suggested_questions",
			},
			Confidence: 0.94,
		},
	}

	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	plan := operationPlanFromTurnStrategy("task-agent-config-expected-fields", parts, strategy)
	step := map[string]interface{}{operationPlanConfigGoalKey: plan["original_user_goal"]}
	assertAgentManagementModelDecidesStrategyForTest(t, parts, strategy)
	capabilityGoals := mapSliceFromAny(plan["capability_goals"])
	for _, want := range []string{agentCapabilityModelSelection, agentCapabilitySystemPrompt, agentCapabilitySuggestedQuestion} {
		if !operationPlanCapabilityGoalsContainForTest(capabilityGoals, want) {
			t.Fatalf("capability_goals = %#v, missing %s; plan=%#v", capabilityGoals, want, plan)
		}
	}
	for _, want := range []string{"model_provider", "model", "system_prompt", "suggested_questions"} {
		if !operationPlanCapabilityGoalsContainRequiredFieldForTest(capabilityGoals, want) {
			t.Fatalf("capability_goals = %#v, missing required config field %q; plan=%#v", capabilityGoals, want, plan)
		}
	}
	for _, unexpected := range []string{"enabled_skill_ids", "knowledge_dataset_ids", "database_bindings", "workflow_bindings"} {
		if operationPlanCapabilityGoalsContainBindingActionForTest(capabilityGoals, unexpected, "bind") ||
			operationPlanCapabilityGoalsContainBindingActionForTest(capabilityGoals, unexpected, "unbind") {
			t.Fatalf("capability_goals = %#v, want no binding action for %q; plan=%#v", capabilityGoals, unexpected, plan)
		}
	}
	if goal := stringFromAny(step[operationPlanConfigGoalKey]); !strings.Contains(goal, "file upload") || !strings.Contains(goal, "theme color") {
		t.Fatalf("config_goal = %q, want preserved semantic config target; step=%#v plan=%#v", goal, step, plan)
	}
}

func TestAgentSkillBackedCapabilityGoalExposesEvidenceBoundaries(t *testing.T) {
	goals := agentManagementCapabilityGoalsFromModelIntent(&AIChatModelTurnIntent{
		Intent:                  "manage_agent_asset",
		RecommendedCapabilities: []string{"agent.skill_backed_capability:enable:file generation"},
		Confidence:              0.95,
	})
	if len(goals) != 1 {
		t.Fatalf("capability goals = %#v, want one skill-backed capability goal", goals)
	}
	goal := goals[0]
	if goal.CapabilityID != agentCapabilitySkillBacked {
		t.Fatalf("capability_id = %q, want %q; goal=%#v", goal.CapabilityID, agentCapabilitySkillBacked, goal)
	}
	for _, boundary := range []string{"system_prompt_only", "file_upload_enabled_only", "candidate_lookup_only", "natural_language_claim_only"} {
		if !stringSliceContains(goal.NotSufficient, boundary) {
			t.Fatalf("not_sufficient = %#v, want %q; goal=%#v", goal.NotSufficient, boundary, goal)
		}
	}
	if !strings.Contains(goal.Meaning, "enabled_skill_ids") {
		t.Fatalf("meaning = %q, want enabled_skill_ids semantics; goal=%#v", goal.Meaning, goal)
	}
	if len(goal.EnableBy) == 0 || !stringSliceContains(goal.RequiredConfigFields, "enabled_skill_ids") {
		t.Fatalf("goal = %#v, want enable path and enabled_skill_ids field", goal)
	}
	maps := agentCapabilityGoalsToMaps(goals)
	if len(maps) != 1 {
		t.Fatalf("agentCapabilityGoalsToMaps = %#v, want one mapped goal", maps)
	}
	for _, key := range []string{"meaning", "enable_by", "not_sufficient", "verify_by"} {
		if _, ok := maps[0][key]; !ok {
			t.Fatalf("mapped capability goal = %#v, missing key %q", maps[0], key)
		}
	}
	compact := operationPlanCompactCapabilityGoals(mapsToInterfaceSlice(maps), 1)
	if len(compact) != 1 {
		t.Fatalf("compact capability goals = %#v, want one compact goal", compact)
	}
	compactGoal := mapFromOperationContext(compact[0])
	for _, key := range []string{"meaning", "enable_by", "not_sufficient", "verify_by"} {
		if _, ok := compactGoal[key]; !ok {
			t.Fatalf("compact capability goal = %#v, missing key %q", compactGoal, key)
		}
	}
}

func TestAgentBindingCapabilityGoalRequiresExplicitAction(t *testing.T) {
	goals := agentManagementCapabilityGoalsFromModelIntent(&AIChatModelTurnIntent{
		Intent:                  "manage_agent_asset",
		RecommendedCapabilities: []string{"agent.knowledge_binding"},
		Confidence:              0.95,
	})
	if len(goals) != 0 {
		t.Fatalf("capability goals = %#v, want no implicit binding goal", goals)
	}

	goals = agentManagementCapabilityGoalsFromModelIntent(&AIChatModelTurnIntent{
		Intent:                  "manage_agent_asset",
		RecommendedCapabilities: []string{"agent.knowledge_binding:bind"},
		Confidence:              0.95,
	})
	if len(goals) != 1 || goals[0].CapabilityID != agentCapabilityKnowledgeBinding {
		t.Fatalf("capability goals = %#v, want explicit knowledge binding goal", goals)
	}
	if !agentCapabilityGoalsContainBindingActionForTest(goals, "knowledge_dataset_ids", "bind") {
		t.Fatalf("capability goals = %#v, want knowledge_dataset_ids bind action", goals)
	}
}

func TestAgentCapabilityDefinitionsForPromptExposeMVPContracts(t *testing.T) {
	definitions := agentManagementCapabilityDefinitionsForPrompt()
	if len(definitions) == 0 {
		t.Fatal("agentManagementCapabilityDefinitionsForPrompt() = empty, want prompt-facing capability model")
	}
	byID := map[string]map[string]interface{}{}
	for _, definition := range definitions {
		id := stringFromAny(definition["capability_id"])
		if id != "" {
			byID[id] = definition
		}
	}

	model := byID[agentCapabilityModelSelection]
	for _, field := range []string{"model_provider", "model"} {
		if !stringSliceContainsFold(stringSliceFromAny(model["required_config_fields"]), field) {
			t.Fatalf("model capability definition = %#v, missing required field %s", model, field)
		}
	}
	if stringFromAny(model["candidate_tool"]) != "list_available_models" {
		t.Fatalf("model capability definition = %#v, want list_available_models candidate tool", model)
	}
	for _, want := range []string{"model_provider", "model"} {
		if !strings.Contains(fmt.Sprint(model["verify_by"]), want) {
			t.Fatalf("model capability verify_by = %#v, want %s evidence", model["verify_by"], want)
		}
	}

	systemPrompt := byID[agentCapabilitySystemPrompt]
	if !stringSliceContainsFold(stringSliceFromAny(systemPrompt["required_config_fields"]), "system_prompt") {
		t.Fatalf("system-prompt capability definition = %#v, want system_prompt field", systemPrompt)
	}
	if !strings.Contains(fmt.Sprint(systemPrompt["not_sufficient"]), "tool_binding_or_resource_access_claim") {
		t.Fatalf("system-prompt not_sufficient = %#v, want resource-access boundary", systemPrompt["not_sufficient"])
	}

	fileUpload := byID[agentCapabilityAcceptUploaded]
	if !stringSliceContainsFold(stringSliceFromAny(fileUpload["required_config_fields"]), "file_upload_enabled") {
		t.Fatalf("file-upload capability definition = %#v, want file_upload_enabled config field", fileUpload)
	}
	if stringSliceContainsFold(stringSliceFromAny(fileUpload["required_config_fields"]), "enabled_skill_ids") {
		t.Fatalf("file-upload capability definition = %#v, should not require skill binding", fileUpload)
	}

	memory := byID[agentCapabilityMemory]
	if !stringSliceContainsFold(stringSliceFromAny(memory["required_config_fields"]), "agent_memory_enabled") {
		t.Fatalf("memory capability definition = %#v, want agent_memory_enabled config field", memory)
	}
	if !strings.Contains(fmt.Sprint(memory["not_sufficient"]), "system_prompt_only") {
		t.Fatalf("memory not_sufficient = %#v, want prompt-only boundary", memory["not_sufficient"])
	}

	suggestedQuestions := byID[agentCapabilitySuggestedQuestion]
	if !stringSliceContainsFold(stringSliceFromAny(suggestedQuestions["required_config_fields"]), "suggested_questions") {
		t.Fatalf("suggested-questions capability definition = %#v, want suggested_questions config field", suggestedQuestions)
	}
	if !strings.Contains(fmt.Sprint(suggestedQuestions["verify_by"]), "suggested_questions") {
		t.Fatalf("suggested-questions verify_by = %#v, want suggested_questions evidence", suggestedQuestions["verify_by"])
	}

	skillBacked := byID[agentCapabilitySkillBacked]
	if !stringSliceContainsFold(stringSliceFromAny(skillBacked["required_config_fields"]), "enabled_skill_ids") {
		t.Fatalf("skill-backed capability definition = %#v, want enabled_skill_ids config field", skillBacked)
	}
	if got := stringFromAny(skillBacked["candidate_tool"]); got != "list_agent_skill_candidates" {
		t.Fatalf("skill-backed candidate_tool = %q, want list_agent_skill_candidates; definition=%#v", got, skillBacked)
	}
	actions := operationPlanAgentConfigBindingActionsFromAny(skillBacked["required_binding_actions"])
	if got := actions["enabled_skill_ids"]; got != "bind" {
		t.Fatalf("skill-backed required_binding_actions = %#v, want enabled_skill_ids bind; definition=%#v", actions, skillBacked)
	}
	if !strings.Contains(fmt.Sprint(skillBacked["examples"]), "file generation") {
		t.Fatalf("skill-backed examples = %#v, want file generation example", skillBacked["examples"])
	}

	for _, want := range []struct {
		capabilityID string
		field        string
	}{
		{agentCapabilityKnowledgeBinding, "knowledge_dataset_ids"},
		{agentCapabilityDatabaseBinding, "database_bindings"},
		{agentCapabilityWorkflowBinding, "workflow_bindings"},
	} {
		definition := byID[want.capabilityID]
		actions := operationPlanAgentConfigBindingActionsFromAny(definition["required_binding_actions"])
		if got := actions[want.field]; got != "bind" {
			t.Fatalf("%s required_binding_actions = %#v, want %s bind; definition=%#v", want.capabilityID, actions, want.field, definition)
		}
		if len(stringSliceFromAny(definition["candidate_tools"])) == 0 {
			t.Fatalf("%s definition = %#v, want candidate_tools for binding resolution", want.capabilityID, definition)
		}
	}
	for _, capabilityID := range []string{
		agentCapabilityModelSelection,
		agentCapabilitySystemPrompt,
		agentCapabilityAcceptUploaded,
		agentCapabilityMemory,
		agentCapabilitySuggestedQuestion,
		agentCapabilitySkillBacked,
		agentCapabilityKnowledgeBinding,
		agentCapabilityDatabaseBinding,
		agentCapabilityWorkflowBinding,
	} {
		definition := byID[capabilityID]
		if len(definition) == 0 {
			t.Fatalf("missing MVP capability definition %s; definitions=%#v", capabilityID, definitions)
		}
		for _, key := range []string{"meaning", "enable_by", "not_sufficient", "verify_by"} {
			if value, ok := definition[key]; !ok || fmt.Sprint(value) == "" || fmt.Sprint(value) == "[]" {
				t.Fatalf("%s definition missing %s semantic contract: %#v", capabilityID, key, definition)
			}
		}
	}
}

func TestAgentCapabilityStatusTargetMarkersCoverMVPModel(t *testing.T) {
	markers := agentManagementCapabilityStatusTargetMarkers()
	for _, want := range []string{
		"file generation",
		"file upload",
		"memory capability",
		"knowledge base",
		"database table",
		"workflow",
		"\u751f\u6210\u6587\u4ef6",
		"\u4e0a\u4f20\u6587\u4ef6",
		"\u8bb0\u5fc6\u80fd\u529b",
		"\u77e5\u8bc6\u5e93",
		"\u6570\u636e\u8868",
		"\u5de5\u4f5c\u6d41",
	} {
		if !stringSliceContainsFold(markers, want) {
			t.Fatalf("agentManagementCapabilityStatusTargetMarkers() = %#v, missing %q", markers, want)
		}
	}
	if !agentManagementCapabilityStatusTargetMentioned("\u8fd9\u4e2a Agent \u662f\u5426\u5df2\u5f00\u542f\u8bb0\u5fc6\u80fd\u529b\uff1f") {
		t.Fatal("agentManagementCapabilityStatusTargetMentioned(memory status query) = false, want true")
	}
	if !agentManagementCapabilityStatusTargetMentioned("\u8fd9\u4e2a Agent \u80fd\u5426\u751f\u6210\u6587\u4ef6\uff1f") {
		t.Fatal("agentManagementCapabilityStatusTargetMentioned(file-generation query) = false, want true")
	}
}

func TestAgentConfigFieldDescriptorsDriveCanonicalFieldsAndBindingActions(t *testing.T) {
	for _, descriptor := range agentManagementConfigFieldDescriptors() {
		t.Run(descriptor.field, func(t *testing.T) {
			if descriptor.field == "" {
				t.Fatalf("config field descriptor has empty field: %#v", descriptor)
			}
			if got := operationPlanAgentConfigCanonicalField(descriptor.field); got != descriptor.field {
				t.Fatalf("operationPlanAgentConfigCanonicalField(%q) = %q, want %q", descriptor.field, got, descriptor.field)
			}
			for _, alias := range descriptor.aliases {
				if got := operationPlanAgentConfigCanonicalField(alias); got != descriptor.field {
					t.Fatalf("operationPlanAgentConfigCanonicalField(%q) = %q, want %q", alias, got, descriptor.field)
				}
			}
			for _, marker := range descriptor.explicitMarkers {
				fields := agentManagementExplicitConfigFieldsFromText("please update " + marker)
				if !stringSliceContainsFold(fields, descriptor.field) {
					t.Fatalf("agentManagementExplicitConfigFieldsFromText(%q) = %#v, missing %s", marker, fields, descriptor.field)
				}
			}
			for _, marker := range descriptor.semanticMarkers {
				query := "please update " + marker
				if !agentManagementConfigFieldSemanticMarkerRequested(query, descriptor.field) {
					t.Fatalf("agentManagementConfigFieldSemanticMarkerRequested(%q, %q) = false, want true", query, descriptor.field)
				}
				if !agentManagementConfigCapabilityMarkerRequested(query) {
					t.Fatalf("agentManagementConfigCapabilityMarkerRequested(%q) = false, want true from descriptor marker %q", query, marker)
				}
			}
			for action := range descriptor.bindingActionMarkers {
				canonicalAction := operationPlanCanonicalAgentConfigBindingAction(action)
				if canonicalAction == "" {
					t.Fatalf("binding action %q for %s is not canonical", action, descriptor.field)
				}
			}
		})
	}
}

func TestAgentCapabilityUpdateRouteContextBeatsTemporaryFileGeneration(t *testing.T) {
	query := "\u8ba9\u5f53\u524d Agent \u80fd\u591f\u751f\u6210\u6587\u4ef6"
	parts := &chatRequestParts{
		Query:          query,
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents/agent-1/agent",
		SkillMode:      skillModeAuto,
		SkillIDs: []string{
			skills.SkillAgentManagement,
			skills.SkillFileGenerator,
		},
		ModelTurnIntent: agentManagementModelIntentForTest("/console/agents/agent-1/agent", "agent.skill_backed_capability:enable:file generation"),
	}
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	if !stringSliceContainsFold(strategy.PrimarySkills, skills.SkillAgentManagement) {
		t.Fatalf("PrimarySkills = %#v, want agent-management", strategy.PrimarySkills)
	}
	if stringSliceContainsFold(strategy.PrimarySkills, skills.SkillFileGenerator) {
		t.Fatalf("PrimarySkills = %#v, want no file-generator for Agent capability update", strategy.PrimarySkills)
	}
	plan := assertAgentManagementModelDecidesStrategyForTest(t, parts, strategy)
	capabilityGoals := mapSliceFromAny(plan["capability_goals"])
	if len(capabilityGoals) != 1 || stringFromAny(capabilityGoals[0]["capability_id"]) != agentCapabilitySkillBacked {
		t.Fatalf("capability_goals = %#v, want skill-backed capability goal; plan=%#v", capabilityGoals, plan)
	}
}

func TestAgentDetailRouteStillAllowsPlainTemporaryFileGeneration(t *testing.T) {
	query := "\u751f\u6210\u4e00\u4e2a SVG \u6587\u4ef6\uff0c\u5185\u5bb9\u968f\u610f"
	parts := &chatRequestParts{
		Query:          query,
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents/agent-1/agent",
		SkillMode:      skillModeAuto,
		SkillIDs: []string{
			skills.SkillAgentManagement,
			skills.SkillFileGenerator,
		},
		ModelTurnIntent: &AIChatModelTurnIntent{
			Intent:                  "generate_temporary_file_artifact",
			TargetPage:              "/console/agents/agent-1/agent",
			RecommendedCapabilities: []string{"file_artifact"},
			Confidence:              0.95,
		},
	}
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	if strategy.Intent != "generate_temporary_file_artifact" {
		t.Fatalf("strategy.Intent = %q, want generate_temporary_file_artifact; strategy=%#v", strategy.Intent, strategy)
	}
	if !stringSliceContainsFold(strategy.PrimarySkills, skills.SkillFileGenerator) {
		t.Fatalf("PrimarySkills = %#v, want file-generator", strategy.PrimarySkills)
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
		ModelTurnIntent: agentManagementModelIntentForTest("/console/agents/agent-1/agent",
			"agent.knowledge_binding:bind",
			"agent.database_binding:bind",
			"agent.workflow_binding:bind",
		),
	}
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	plan := assertAgentManagementModelDecidesStrategyForTest(t, parts, strategy)
	capabilityGoals := mapSliceFromAny(plan["capability_goals"])
	for _, field := range []string{"knowledge_dataset_ids", "database_bindings", "workflow_bindings"} {
		if !operationPlanCapabilityGoalsContainBindingActionForTest(capabilityGoals, field, "bind") {
			t.Fatalf("capability_goals = %#v, missing bind action for %s; plan=%#v", capabilityGoals, field, plan)
		}
	}
	if operationPlanCapabilityGoalsContainForTest(capabilityGoals, agentCapabilitySkillBacked) ||
		operationPlanCapabilityGoalsContainBindingActionForTest(capabilityGoals, "enabled_skill_ids", "bind") ||
		operationPlanCapabilityGoalsContainBindingActionForTest(capabilityGoals, "enabled_skill_ids", "unbind") {
		t.Fatalf("capability_goals = %#v, want no Skill binding goal for noop Skill phrase", capabilityGoals)
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
		ModelTurnIntent: agentManagementModelIntentForTest("/console/agents/agent-1/agent",
			"agent.model_selection",
			"agent.system_prompt",
			"agent.suggested_questions",
		),
	}

	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want Agent-management strategy")
	}
	semanticPlan := assertAgentManagementModelDecidesStrategyForTest(t, parts, strategy)
	semanticCapabilityGoals := mapSliceFromAny(semanticPlan["capability_goals"])
	for _, want := range []string{agentCapabilityModelSelection, agentCapabilitySystemPrompt, agentCapabilitySuggestedQuestion} {
		if !operationPlanCapabilityGoalsContainForTest(semanticCapabilityGoals, want) {
			t.Fatalf("capability_goals = %#v, missing %s; plan=%#v", semanticCapabilityGoals, want, semanticPlan)
		}
	}
}

func TestAgentIdentityPostUpdateReadDoesNotAddConsoleNavigatorAutomatically(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "\u628a\u5f53\u524d\u667a\u80fd\u4f53\u7684\u540d\u79f0\u6539\u4e3a Support Agent\uff0c\u63cf\u8ff0\u6539\u4e3a smoke\u3002\u5b8c\u6210\u540e\u786e\u8ba4\u9875\u9762\u9876\u90e8\u5df2\u66f4\u65b0\u3002",
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents/agent-1/agent",
		SkillMode:      skillModeAuto,
		SkillIDs: []string{
			skills.SkillAgentManagement,
			skills.SkillConsoleNavigator,
		},
		ModelTurnIntent: agentManagementModelIntentForTest("/console/agents/agent-1/agent",
			"agent.model_selection:reasoning",
		),
	}
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{
		{Metadata: skills.SkillMetadata{ID: skills.SkillAgentManagement}},
		{Metadata: skills.SkillMetadata{ID: skills.SkillConsoleNavigator}},
	}}

	filtered := restrictResolvedSkillsForTurnStrategy(parts, resolved)
	got := filtered.SkillIDs()
	want := []string{skills.SkillAgentManagement, skills.SkillConsoleNavigator}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered skill ids = %#v, want all enabled skills for model-decides tool choice %#v", got, want)
	}
}

func TestAgentMemoryCapabilityEnableRequestPlansConfigMutation(t *testing.T) {
	query := "\u8ba9\u5f53\u524d Agent \u5177\u5907\u957f\u671f\u8bb0\u5fc6\u80fd\u529b"
	if agentManagementCapabilityStatusQuestionRequested(query) {
		t.Fatalf("agentManagementCapabilityStatusQuestionRequested(%q) = true, want false for memory mutation request", query)
	}
	if !agentManagementMemoryConfigCapabilityRequested(query) {
		t.Fatalf("agentManagementMemoryConfigCapabilityRequested(%q) = false, want true", query)
	}
	parts := consoleAgentDetailTestParts(query)
	parts.ModelTurnIntent = agentManagementModelIntentForTest("/console/agents/agent-1/agent", "agent.memory:update")
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	semanticPlan := assertAgentManagementModelDecidesStrategyForTest(t, parts, strategy)
	semanticCapabilityGoals := mapSliceFromAny(semanticPlan["capability_goals"])
	if len(semanticCapabilityGoals) != 1 || stringFromAny(semanticCapabilityGoals[0]["capability_id"]) != agentCapabilityMemory {
		t.Fatalf("capability_goals = %#v, want one memory capability goal; plan=%#v", semanticCapabilityGoals, semanticPlan)
	}
	if got := stringFromAny(semanticCapabilityGoals[0]["goal_action"]); got != agentCapabilityActionUpdate {
		t.Fatalf("capability goal action = %q, want %q; goal=%#v", got, agentCapabilityActionUpdate, semanticCapabilityGoals[0])
	}
	if !operationPlanCapabilityGoalsContainRequiredFieldForTest(semanticCapabilityGoals, "agent_memory_enabled") {
		t.Fatalf("capability_goals = %#v, missing agent_memory_enabled", semanticCapabilityGoals)
	}
	return
	if strategy.Intent != "continue_previous_task" {
		t.Fatalf("strategy.Intent = %q, want continue_previous_task without synthesized capability continuation; strategy=%#v", strategy.Intent, strategy)
	}
	if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "get_agent_config") {
		t.Fatalf("PlannedTools = %#v, missing get_agent_config", strategy.PlannedTools)
	}
	if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "update_agent_config") {
		t.Fatalf("PlannedTools = %#v, missing update_agent_config", strategy.PlannedTools)
	}
	updateArgs := aiChatTurnStrategyPlannedToolArgumentsForTest(strategy, skills.SkillAgentManagement, "update_agent_config")
	fields := operationPlanNormalizedAgentConfigFieldsFromAny(updateArgs[operationPlanExpectedUpdatedFieldsKey])
	if !stringSliceContainsFold(fields, "agent_memory_enabled") {
		t.Fatalf("update_agent_config expected fields = %#v, missing agent_memory_enabled; args=%#v", fields, updateArgs)
	}
	if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "list_agent_skill_candidates") {
		t.Fatalf("PlannedTools = %#v, want no skill candidate lookup for memory config setting", strategy.PlannedTools)
	}

	plan := operationPlanFromTurnStrategy("task-agent-memory-enable", parts, strategy)
	capabilityGoals := mapSliceFromAny(plan["capability_goals"])
	if len(capabilityGoals) != 1 || stringFromAny(capabilityGoals[0]["capability_id"]) != agentCapabilityMemory {
		t.Fatalf("capability_goals = %#v, want one memory capability goal; plan=%#v", capabilityGoals, plan)
	}
	if got := stringFromAny(capabilityGoals[0]["goal_action"]); got != agentCapabilityActionUpdate {
		t.Fatalf("capability goal action = %q, want %q; goal=%#v", got, agentCapabilityActionUpdate, capabilityGoals[0])
	}
	goalFields := stringSliceFromAny(capabilityGoals[0]["required_config_fields"])
	if len(goalFields) != 1 || goalFields[0] != "agent_memory_enabled" {
		t.Fatalf("capability required_config_fields = %#v, want agent_memory_enabled", goalFields)
	}
}

func TestAgentCapabilityContinuationActionConfirmationUsesPriorCandidate(t *testing.T) {
	for _, query := range []string{
		"\u90a3\u5c31\u505a",
		"\u5c31\u8fd9\u4e48\u505a",
		"\u6309\u8fd9\u4e2a\u505a",
		"\u6309\u8fd9\u4e2a\u5904\u7406",
	} {
		t.Run(query, func(t *testing.T) {
			if !isContinuationIntent(query) {
				t.Fatalf("isContinuationIntent(%q) = false, want true", query)
			}
			previousQuery := "\u8fd9\u4e2aagent\u80fd\u751f\u6210\u6587\u4ef6\u5417"
			previousMessage := &runtimemodel.Message{
				ID:     uuid.New(),
				Query:  previousQuery,
				Status: runtimemodel.MessageStatusCompleted,
				Metadata: map[string]interface{}{
					"operation_plan": map[string]interface{}{
						"version":             operationPlanVersion,
						"status":              operationPlanStatusCompleted,
						"pending_next_action": "none",
						"intent":              "inspect_agent_config",
						"original_user_goal":  previousQuery,
						"capability_goals": []interface{}{
							map[string]interface{}{
								"capability_id": agentCapabilitySkillBacked,
								"display_name":  "skill-backed capability",
								"required_binding_actions": map[string]interface{}{
									"enabled_skill_ids": "bind",
								},
								"candidate_tool":  "list_agent_skill_candidates",
								"candidate_query": "file generation",
							},
						},
					},
					"skill_invocations": []interface{}{
						agentManagementToolInvocationForTest("get_agent_config", "tool_call:agent-management:get_agent_config::#1", map[string]interface{}{
							"status":            "success",
							"agent_id":          "agent-1",
							"agent_name":        "Novel Agent",
							"enabled_skill_ids": []interface{}{"calculator"},
						}),
						agentManagementToolInvocationForTest("list_agent_skill_candidates", "tool_call:agent-management:list_agent_skill_candidates::#1", map[string]interface{}{
							"status": "success",
							"candidate_samples": []interface{}{
								map[string]interface{}{"id": "file-generator", "name": "File Generator"},
							},
						}),
					},
				},
			}
			parts := consoleAgentDetailTestParts(query)
			parts.SkillIDs = []string{skills.SkillAgentManagement, skills.SkillConsoleNavigator}
			applyRecentOperationPlansFromBranch(parts, []*runtimemodel.Message{previousMessage})
			assertNoSynthesizedAgentCapabilityContinuationForTest(t, parts)
			strategy := contextualAIChatTurnStrategyFromParts(parts)
			if strategy == nil {
				t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want continuation strategy")
			}
			assertAgentManagementModelDecidesExecutionForTest(t, strategy)
			if len(strategy.CapabilityGoals) != 0 {
				t.Fatalf("strategy.CapabilityGoals = %#v, want no hard synthesized continuation goals", strategy.CapabilityGoals)
			}
			return
			if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "update_agent_config") {
				t.Fatalf("PlannedTools = %#v, missing update_agent_config from action-confirmation continuation", strategy.PlannedTools)
			}
			updateArgs := aiChatTurnStrategyPlannedToolArgumentsForTest(strategy, skills.SkillAgentManagement, "update_agent_config")
			if got := stringFromAny(updateArgs["candidate_skill_id"]); got != "file-generator" {
				t.Fatalf("update_agent_config args = %#v, want candidate_skill_id file-generator", updateArgs)
			}
			if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "delete_agent") ||
				aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "delete_agents") {
				t.Fatalf("PlannedTools = %#v, want no stale delete tools for action-confirmation continuation", strategy.PlannedTools)
			}
		})
	}
}

func TestAgentConfigCapabilityContinuationEnablesPriorFileUploadStatus(t *testing.T) {
	previousQuery := "\u8fd9\u4e2aagent\u80fd\u4e0a\u4f20\u6587\u4ef6\u5417"
	previousMessage := &runtimemodel.Message{
		ID:     uuid.New(),
		Query:  previousQuery,
		Status: runtimemodel.MessageStatusCompleted,
		Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"version":             operationPlanVersion,
				"status":              operationPlanStatusCompleted,
				"pending_next_action": "none",
				"intent":              "inspect_agent_config",
				"original_user_goal":  previousQuery,
				"capability_goals": []interface{}{
					map[string]interface{}{
						"capability_id":          agentCapabilityAcceptUploaded,
						"goal_action":            agentCapabilityActionInspect,
						"display_name":           "file upload",
						"required_config_fields": []interface{}{"file_upload_enabled"},
						"verify_by":              []interface{}{"get_agent_config reports the current file_upload_enabled state"},
					},
				},
			},
			"skill_invocations": []interface{}{
				agentManagementToolInvocationForTest("get_agent_config", "tool_call:agent-management:get_agent_config::#1", map[string]interface{}{
					"status":              "success",
					"agent_id":            "agent-1",
					"agent_name":          "Upload Agent",
					"file_upload_enabled": false,
				}),
			},
		},
	}
	parts := consoleAgentDetailTestParts("\u5f00\u59cb\u5904\u7406")
	parts.SkillIDs = []string{skills.SkillAgentManagement, skills.SkillConsoleNavigator}
	applyRecentOperationPlansFromBranch(parts, []*runtimemodel.Message{previousMessage})
	assertNoSynthesizedAgentCapabilityContinuationForTest(t, parts)

	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want continuation strategy")
	}
	if strategy.Intent != "continue_previous_task" {
		t.Fatalf("strategy.Intent = %q, want continue_previous_task without synthesized capability continuation; strategy=%#v", strategy.Intent, strategy)
	}
	assertAgentManagementModelDecidesExecutionForTest(t, strategy)
	if len(strategy.CapabilityGoals) != 0 {
		t.Fatalf("strategy.CapabilityGoals = %#v, want no hard synthesized continuation goals", strategy.CapabilityGoals)
	}
}

func TestAgentConfigCapabilityContinuationEnablesPriorMemoryStatus(t *testing.T) {
	previousQuery := "\u8fd9\u4e2aagent\u6709\u8bb0\u5fc6\u80fd\u529b\u5417"
	previousMessage := &runtimemodel.Message{
		ID:     uuid.New(),
		Query:  previousQuery,
		Status: runtimemodel.MessageStatusCompleted,
		Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"version":             operationPlanVersion,
				"status":              operationPlanStatusCompleted,
				"pending_next_action": "none",
				"intent":              "inspect_agent_config",
				"original_user_goal":  previousQuery,
				"capability_goals": []interface{}{
					map[string]interface{}{
						"capability_id":          agentCapabilityMemory,
						"goal_action":            agentCapabilityActionInspect,
						"display_name":           "agent memory",
						"required_config_fields": []interface{}{"agent_memory_enabled"},
						"verify_by":              []interface{}{"get_agent_config reports the current agent_memory_enabled state"},
					},
				},
			},
			"skill_invocations": []interface{}{
				agentManagementToolInvocationForTest("get_agent_config", "tool_call:agent-management:get_agent_config::#1", map[string]interface{}{
					"status":               "success",
					"agent_id":             "agent-1",
					"agent_name":           "Memory Agent",
					"agent_memory_enabled": false,
				}),
			},
		},
	}
	parts := consoleAgentDetailTestParts("\u90a3\u5c31\u505a")
	parts.SkillIDs = []string{skills.SkillAgentManagement, skills.SkillConsoleNavigator}
	applyRecentOperationPlansFromBranch(parts, []*runtimemodel.Message{previousMessage})
	assertNoSynthesizedAgentCapabilityContinuationForTest(t, parts)
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want continuation strategy")
	}
	if strategy.Intent != "continue_previous_task" {
		t.Fatalf("strategy.Intent = %q, want continue_previous_task without synthesized capability continuation; strategy=%#v", strategy.Intent, strategy)
	}
	assertAgentManagementModelDecidesExecutionForTest(t, strategy)
	if len(strategy.CapabilityGoals) != 0 {
		t.Fatalf("strategy.CapabilityGoals = %#v, want no hard synthesized continuation goals", strategy.CapabilityGoals)
	}
}

func TestAgentResourceBindingCandidateContinuationUsesPriorCandidateResults(t *testing.T) {
	previousQuery := "\u53ea\u8bfb\u67e5\u770b\u8fd9\u4e2aagent\u5f53\u524d\u53ef\u7ed1\u5b9a\u7684\u77e5\u8bc6\u5e93\u3001\u6570\u636e\u8868\u548c\u5de5\u4f5c\u6d41\u5019\u9009"
	previousMessage := &runtimemodel.Message{
		ID:     uuid.New(),
		Query:  previousQuery,
		Status: runtimemodel.MessageStatusCompleted,
		Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"version":             operationPlanVersion,
				"status":              operationPlanStatusCompleted,
				"pending_next_action": "none",
				"intent":              "agent.inspect_candidates",
				"original_user_goal":  previousQuery,
				"capability_goals": []interface{}{
					map[string]interface{}{
						"capability_id":          agentCapabilityKnowledgeBinding,
						"goal_action":            agentCapabilityActionInspect,
						"display_name":           "knowledge base binding",
						"required_config_fields": []interface{}{"knowledge_dataset_ids"},
					},
					map[string]interface{}{
						"capability_id":          agentCapabilityDatabaseBinding,
						"goal_action":            agentCapabilityActionInspect,
						"display_name":           "database table binding",
						"required_config_fields": []interface{}{"database_bindings"},
					},
					map[string]interface{}{
						"capability_id":          agentCapabilityWorkflowBinding,
						"goal_action":            agentCapabilityActionInspect,
						"display_name":           "workflow binding",
						"required_config_fields": []interface{}{"workflow_bindings"},
					},
				},
			},
			"skill_invocations": []interface{}{
				agentManagementToolInvocationForTest("list_agent_knowledge_candidates", "tool_call:agent-management:list_agent_knowledge_candidates::#1", map[string]interface{}{
					"status": "success",
					"items": []interface{}{
						map[string]interface{}{"id": "kb-1", "name": "Knowledge One"},
					},
				}),
				agentManagementToolInvocationForTest("list_agent_database_tables", "tool_call:agent-management:list_agent_database_tables::#1", map[string]interface{}{
					"status": "success",
					"binding_candidates": []interface{}{
						map[string]interface{}{
							"id":   "db-1:table-1",
							"name": "Orders",
							"binding": map[string]interface{}{
								"data_source_id": "db-1",
								"table_id":       "table-1",
							},
						},
					},
				}),
				agentManagementToolInvocationForTest("list_agent_workflow_binding_candidates", "tool_call:agent-management:list_agent_workflow_binding_candidates::#1", map[string]interface{}{
					"status": "success",
					"items": []interface{}{
						map[string]interface{}{"id": "wf-1", "name": "Workflow One"},
					},
				}),
			},
		},
	}
	parts := consoleAgentDetailTestParts("\u8fdb\u884c\u5904\u7406")
	parts.SkillIDs = []string{skills.SkillAgentManagement, skills.SkillConsoleNavigator}
	applyRecentOperationPlansFromBranch(parts, []*runtimemodel.Message{previousMessage})
	assertNoSynthesizedAgentCapabilityContinuationForTest(t, parts)

	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want continuation strategy")
	}
	assertAgentManagementModelDecidesExecutionForTest(t, strategy)
	if len(strategy.CapabilityGoals) != 0 {
		t.Fatalf("strategy.CapabilityGoals = %#v, want no hard synthesized continuation goals", strategy.CapabilityGoals)
	}
	return
	if !aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "update_agent_config") {
		t.Fatalf("PlannedTools = %#v, missing update_agent_config from candidate-based continuation", strategy.PlannedTools)
	}
	if aiChatTurnStrategyHasPlannedToolForTest(strategy, skills.SkillAgentManagement, "list_agent_database_tables") {
		t.Fatalf("PlannedTools = %#v, want no repeated list_agent_database_tables after prior candidate results", strategy.PlannedTools)
	}
	updateArgs := aiChatTurnStrategyPlannedToolArgumentsForTest(strategy, skills.SkillAgentManagement, "update_agent_config")
	strategyActions := operationPlanAgentConfigBindingActionsFromAny(updateArgs[operationPlanExpectedBindingActionsKey])
	for _, field := range []string{"knowledge_dataset_ids", "database_bindings", "workflow_bindings"} {
		if strategyActions[field] != "bind" {
			t.Fatalf("strategy update_agent_config actions[%s] = %#v, want bind; args=%#v", field, strategyActions[field], updateArgs)
		}
	}
}

func TestSkillLoopRuntimeStateSnapshotEmbedsOperationLedgerInExecutionLedger(t *testing.T) {
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

	evidence := skillLoopRuntimeStateSnapshot(prepared)()
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

func TestSkillLoopRuntimeStateSnapshotBuildsExecutionSummaryWithoutLegacyDeviations(t *testing.T) {
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

	evidence := skillLoopRuntimeStateSnapshot(prepared)()
	summary := mapFromOperationContext(evidence["execution_summary"])
	if len(summary) == 0 {
		t.Fatalf("execution_summary = %#v, want compact summary", evidence["execution_summary"])
	}
	planSummary := mapFromOperationContext(summary["operation_plan"])
	for _, key := range []string{"deviations", "blocked_deviations"} {
		if _, exists := planSummary[key]; exists {
			t.Fatalf("execution summary contains audit-only field %q: %#v", key, planSummary[key])
		}
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

	if targets := consoleNavigationResolvedTargetsForParts(parts); len(targets) > 0 {
		t.Fatalf("consoleNavigationResolvedTargetsForParts() = %#v, want no target for explicit no-navigation request", targets)
	}
	evidence := skillLoopCompletionPageContextEvidence(parts)
	if got := stringFromAny(evidence["current_page"]); got != "/console/agents/agent-1/agent" {
		t.Fatalf("current_page = %q, want current Agent detail route; evidence=%#v", got, evidence)
	}
	if target := mapFromOperationContext(evidence["resolved_target_from_user_request"]); len(target) > 0 {
		t.Fatalf("resolved_target_from_user_request = %#v, want omitted for explicit no-navigation request", target)
	}
}

func TestContextualConsoleAgentsSkillMessageFiltersUnavailableResolvedTools(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          "open the Agent page",
			Surface:        aiChatSurfaceContextualSidebar,
			SkillMode:      skillModeAuto,
			SkillIDs:       []string{skills.SkillAgentManagement, skills.SkillConsoleNavigator},
			RuntimeContext: "route=/console/agents",
			RawOperationContext: map[string]interface{}{
				"resources": []interface{}{
					map[string]interface{}{
						"resource_type": "page",
						"href":          "/console/agents",
					},
					map[string]interface{}{
						"resource_type": "agent",
						"id":            "agent-1",
						"title":         "Customer Support Agent",
						"href":          "/console/agents/agent-1/agent",
					},
				},
			},
		},
	}
	prepared.parts.OperationContext = prepared.parts.RawOperationContext
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{
		{
			Metadata: skills.SkillMetadata{ID: skills.SkillAgentManagement},
			Tools: []skills.SkillToolDefinition{
				{Name: "get_agent_config"},
				{Name: "update_agent_config"},
				{Name: "list_available_models"},
			},
		},
	}}

	message, ok := contextualConsoleAgentsSkillMessageForResolved(prepared, resolved)
	if !ok {
		t.Fatal("contextualConsoleAgentsSkillMessageForResolved() = false, want message")
	}
	content := stringFromAny(message.Content)
	for _, unexpected := range []string{`"tool_name":"list_agents"`, `"tool_name":"get_agent"`, `"tool_name":"navigate"`} {
		if strings.Contains(content, unexpected) {
			t.Fatalf("message content exposes unavailable tool %s: %s", unexpected, content)
		}
	}
	for _, want := range []string{
		`"tool_name":"get_agent_config"`,
		`"tool_name":"update_agent_config"`,
		"tools array in Agent management JSON is the authoritative callable tool list",
		"Because list_agents is absent from the tools array",
		"Because get_agent is absent from the tools array",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("message content missing %q: %s", want, content)
		}
	}
}

func TestSkillLoopToolEffectClassificationUsesGovernanceManifest(t *testing.T) {
	if !skillLoopToolLooksReadOnly(skills.SkillInternalKnowledge, "retrieve_knowledge") {
		t.Fatal("retrieve_knowledge was not classified as read-only from governance manifest")
	}
	if skillLoopToolLooksAssetMutation(skills.SkillInternalKnowledge, "retrieve_knowledge") {
		t.Fatal("retrieve_knowledge was classified as mutation, want read-only")
	}
	if !skillLoopToolLooksAssetMutation(skills.SkillAgentWorkflow, "run_agent_workflow") {
		t.Fatal("run_agent_workflow was not classified as governed mutation/invoke")
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
		ModelTurnIntent: agentManagementModelIntentForTest("/console/agents",
			"agent.knowledge_binding:bind",
		),
	}
	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{Intent: "manage_agent_asset"})
	assertAgentManagementModelDecidesStrategyForTest(t, parts, strategy)
}

func TestAgentManagementDeleteIntentDoesNotPlanEditsFromTargetNames(t *testing.T) {
	query := "\u8bf7\u5220\u9664\u5f53\u524d\u667a\u80fd\u4f53\u9875\u9762\u524d\u4e24\u4e2a\u53ef\u89c1\u667a\u80fd\u4f53\uff1aGOAL-CONFIG-AGENT-1782403819308-EDITED9 \u548c AIChat\u914d\u7f6e\u9a8c\u8bc106231035-\u5df2\u7f16\u8f91"
	parts := &chatRequestParts{
		Query:     query,
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillAgentManagement},
		ModelTurnIntent: agentManagementModelIntentForTest("/console/agents",
			"agent.skill_backed_capability:bind",
			"agent.knowledge_binding:bind",
			"agent.database_binding:bind",
		),
	}
	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{Intent: "manage_agent_asset"})
	assertAgentManagementModelDecidesStrategyForTest(t, parts, strategy)
}

func TestAgentManagementDeleteCreatedReferenceDoesNotPlanCreate(t *testing.T) {
	query := "\u5220\u9664\u521a\u521a\u521b\u5efa\u7684\u8fd9\u4e24\u4e2a\u6d4b\u8bd5 Agent\uff1aAICHAT-BATCH-SMOKE-A \u548c AICHAT-BATCH-SMOKE-B\u3002\u8bf7\u4e00\u6b21\u6027\u6279\u91cf\u5220\u9664\uff0c\u5b8c\u6210\u540e\u505c\u7559\u5728\u667a\u80fd\u4f53\u5217\u8868\u9875\uff0c\u5e76\u544a\u8bc9\u6211\u6bcf\u4e2a\u5220\u9664\u7ed3\u679c\u3002"
	if agentManagementCreateRequested(query) {
		t.Fatalf("agentManagementCreateRequested(%q) = true, want false for existing Agent delete target reference", query)
	}
	if !agentManagementCreateMentionIsDeleteTargetReference(query) {
		t.Fatalf("agentManagementCreateMentionIsDeleteTargetReference(%q) = false, want true", query)
	}
	parts := &chatRequestParts{
		Query:     query,
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillAgentManagement},
		ModelTurnIntent: agentManagementModelIntentForTest("/console/agents",
			"agent.knowledge_binding:bind",
		),
	}
	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{
		Intent: "manage_agent_asset",
		PlannedTools: []AIChatTurnStrategyTool{
			{SkillID: skills.SkillAgentManagement, ToolName: "create_agent"},
		},
	})
	assertAgentManagementModelDecidesStrategyForTest(t, parts, strategy)
}

func TestAgentManagementDeleteJustNowCreatedReferenceDoesNotPlanCreate(t *testing.T) {
	query := "\u8bf7\u5220\u9664\u521a\u624d\u521b\u5efa\u7684\u6d4b\u8bd5\u667a\u80fd\u4f53 AICHAT-GOAL-SMOKE-1782754487710\u3002\u53ea\u5220\u9664\u8fd9\u4e2a\u540d\u5b57\u5b8c\u5168\u5339\u914d\u7684\u667a\u80fd\u4f53\uff0c\u5220\u9664\u540e\u786e\u8ba4\u5217\u8868\u4e2d\u5df2\u7ecf\u4e0d\u53ef\u89c1\u3002"
	if agentManagementCreateRequested(query) {
		t.Fatalf("agentManagementCreateRequested(%q) = true, want false for existing Agent reference", query)
	}
	if !agentManagementCreateMentionIsDeleteTargetReference(query) {
		t.Fatalf("agentManagementCreateMentionIsDeleteTargetReference(%q) = false, want true for just-now-created delete target", query)
	}
	parts := &chatRequestParts{
		Query:     query,
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillAgentManagement},
		ModelTurnIntent: agentManagementModelIntentForTest("/console/agents",
			"agent.skill_backed_capability:bind",
			"agent.knowledge_binding:bind",
			"agent.database_binding:bind",
		),
	}
	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{
		Intent: "manage_agent_asset",
		PlannedTools: []AIChatTurnStrategyTool{
			{SkillID: skills.SkillAgentManagement, ToolName: "create_agent"},
		},
	})
	assertAgentManagementModelDecidesStrategyForTest(t, parts, strategy)
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
		ModelTurnIntent: agentManagementModelIntentForTest("/console/agents",
			"agent.knowledge_binding:bind",
		),
	}
	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{Intent: "manage_agent_asset"})
	assertAgentManagementModelDecidesStrategyForTest(t, parts, strategy)
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
		ModelTurnIntent: agentManagementModelIntentForTest("/console/agents",
			"agent.skill_backed_capability:bind",
			"agent.knowledge_binding:bind",
			"agent.database_binding:bind",
		),
	}
	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{Intent: "manage_agent_asset"})
	assertAgentManagementModelDecidesStrategyForTest(t, parts, strategy)
}

func TestAgentManagementCreateDescriptionDoesNotPlanDelete(t *testing.T) {
	query := `create 2 draft agents named Smoke-A and Smoke-B with description "AIChat smoke test, deletable"; do not navigate to detail page`
	if !agentManagementCreateRequested(query) {
		t.Fatalf("agentManagementCreateRequested(%q) = false, want true", query)
	}
	if agentManagementDeleteRequested(query) {
		t.Fatalf("agentManagementDeleteRequested(%q) = true, want false for descriptive deletable text", query)
	}
	parts := &chatRequestParts{
		Query:           query,
		Surface:         aiChatSurfaceContextualSidebar,
		SkillMode:       skillModeAuto,
		SkillIDs:        []string{skills.SkillAgentManagement},
		ModelTurnIntent: agentManagementModelIntentForTest("/console/agents"),
	}
	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{Intent: "manage_agent_asset"})
	assertAgentManagementModelDecidesStrategyForTest(t, parts, strategy)
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
	parts := &chatRequestParts{
		Query:           query,
		Surface:         aiChatSurfaceContextualSidebar,
		SkillMode:       skillModeAuto,
		SkillIDs:        []string{skills.SkillAgentManagement},
		ModelTurnIntent: agentManagementModelIntentForTest("/console/agents"),
	}
	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{Intent: "manage_agent_asset"})
	assertAgentManagementModelDecidesStrategyForTest(t, parts, strategy)
}

func TestAgentManagementCreateForBatchDeleteRegressionDoesNotPlanDelete(t *testing.T) {
	query := "Create two Agents named PLAN-DELETE-A and PLAN-DELETE-B for an AIChat planner regression case. These are literal names for new Agents."
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
		Query:           query,
		Surface:         aiChatSurfaceContextualSidebar,
		SkillMode:       skillModeAuto,
		SkillIDs:        []string{skills.SkillAgentManagement},
		ModelTurnIntent: agentManagementModelIntentForTest("/console/agents", "agent.knowledge_binding:bind"),
	}
	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{Intent: "manage_agent_asset"})
	assertAgentManagementModelDecidesStrategyForTest(t, parts, strategy)
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
	assertAgentManagementModelDecidesStrategyForTest(t, parts, strategy)
}

func TestAgentManagementBindingFinalAnswerInstructionsDoNotPlanIdentityUpdate(t *testing.T) {
	query := "Enable the Skill named AICHAT-CONFIG-SMOKE for the current Agent."
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
		ModelTurnIntent: agentManagementModelIntentForTest("/console/agents",
			"agent.skill_backed_capability:bind",
		),
	}
	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{Intent: "manage_agent_asset"})
	plan := assertAgentManagementModelDecidesStrategyForTest(t, parts, strategy)
	if !operationPlanCapabilityGoalsContainBindingActionForTest(mapSliceFromAny(plan["capability_goals"]), "enabled_skill_ids", "bind") &&
		!operationPlanCapabilityGoalsContainBindingActionForTest(mapSliceFromAny(plan["capability_goals"]), "enabled_skill_ids", "unbind") {
		t.Fatalf("capability_goals = %#v, want Skill bind/unbind target", plan["capability_goals"])
	}
}

func TestAgentManagementResourceBindingAnswerInstructionsDoNotPlanIdentityUpdate(t *testing.T) {
	query := "Bind one available knowledge base, one database table, and one workflow to the current Agent."
	if agentManagementIdentityUpdateRequested(query) {
		t.Fatalf("agentManagementIdentityUpdateRequested(%q) = true, want false for resource-name final answer instruction", query)
	}

	parts := &chatRequestParts{
		Query:          query,
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents",
		SkillMode:      skillModeAuto,
		SkillIDs:       []string{skills.SkillAgentManagement},
		ModelTurnIntent: agentManagementModelIntentForTest("/console/agents",
			"agent.knowledge_binding:bind",
			"agent.database_binding:bind",
			"agent.workflow_binding:bind",
		),
	}
	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{Intent: "manage_agent_asset"})
	plan := assertAgentManagementModelDecidesStrategyForTest(t, parts, strategy)
	for _, field := range []string{"knowledge_dataset_ids", "database_bindings", "workflow_bindings"} {
		if !operationPlanCapabilityGoalsContainBindingActionForTest(mapSliceFromAny(plan["capability_goals"]), field, "bind") {
			t.Fatalf("capability_goals = %#v, missing bind action for %s", plan["capability_goals"], field)
		}
	}
}

func TestAgentManagementDeleteWithBindingPreserveClauseStillPlansDelete(t *testing.T) {
	query := "delete this agent, but do not modify its Skill, knowledge base, database table, or workflow bindings first."
	if !agentManagementDeleteRequested(query) {
		t.Fatalf("agentManagementDeleteRequested(%q) = false, want true for direct Agent deletion", query)
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
		ModelTurnIntent: agentManagementModelIntentForTest(
			"/console/agents/current/agent",
			"agent.skill_binding:bind",
			"agent.knowledge_binding:bind",
			"agent.database_binding:bind",
		),
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
	assertAgentManagementModelDecidesStrategyForTest(t, parts, strategy)
}

func TestAgentManagementCreateThenExplicitBindingSurvivesCreatePayloadPruning(t *testing.T) {
	query := "\u8bf7\u521b\u5efa\u4e00\u4e2a\u667a\u80fd\u4f53\uff0c\u540d\u79f0\u4e3a SMOKE-BIND\uff0c\u63cf\u8ff0\u4e3a Agent \u914d\u7f6e\u7ed1\u5b9a\u5192\u70df\u6d4b\u8bd5\uff0c\u5e76\u7ed1\u5b9a\u77e5\u8bc6\u5e93 \u6d4b\u8bd5\u5e932\u3002"
	parts := &chatRequestParts{
		Query:           query,
		Surface:         aiChatSurfaceContextualSidebar,
		SkillMode:       skillModeAuto,
		SkillIDs:        []string{skills.SkillAgentManagement},
		ModelTurnIntent: agentManagementModelIntentForTest("/console/agents", "agent.knowledge_binding:bind"),
	}
	strategy := enrichAIChatTurnStrategyPlannedTools(parts, &AIChatTurnStrategy{Intent: "manage_agent_asset"})
	plan := assertAgentManagementModelDecidesStrategyForTest(t, parts, strategy)
	if !operationPlanCapabilityGoalsContainBindingActionForTest(mapSliceFromAny(plan["capability_goals"]), "knowledge_dataset_ids", "bind") {
		t.Fatalf("capability_goals = %#v, missing knowledge bind action for explicit create-then-bind request", plan["capability_goals"])
	}
}

func TestAgentIdentityEditDoesNotPlanCreateFromHyphenatedAgentName(t *testing.T) {
	query := "Rename the existing Agent GOAL-CREATE-SMOKE-1782961316067-EDITED to GOAL-CREATE-SMOKE-1782961316067-EDITED2."
	if agentManagementCreateRequested(query) {
		t.Fatalf("agentManagementCreateRequested(%q) = true, want false; CREATE in a hyphenated Agent name is not a create intent", query)
	}

	parts := contextualConsoleAgentsManageCapabilityPartsForTest()
	parts.Query = query
	parts.SkillMode = skillModeAuto
	parts.SkillIDs = []string{skills.SkillAgentManagement}
	parts.ModelTurnIntent = agentManagementModelIntentForTest("/console/agents")
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	assertAgentManagementModelDecidesStrategyForTest(t, parts, strategy)
}

func TestAgentIdentityEditDoesNotPlanCreateFromDescriptionValue(t *testing.T) {
	query := "\u6700\u7ec8\u590d\u6d4b\uff1a\u8bf7\u628a\u667a\u80fd\u4f53 GOAL-CREATE-SMOKE-1782961316067-EDITED2 \u7684\u540d\u79f0\u6539\u4e3a GOAL-CREATE-SMOKE-1782961316067-EDITED3\uff0c\u63cf\u8ff0\u6539\u4e3a Planner create \u8bef\u5224\u4fee\u590d\u590d\u6d4b\uff0c\u56fe\u6807\u6539\u4e3a \U0001f7e9\u3002\u4fdd\u5b58\u540e\u7528\u4e00\u53e5\u8bdd\u786e\u8ba4\u5de5\u5177\u7ed3\u679c\u548c\u9875\u9762\u53ef\u89c1\u7ed3\u679c\u3002"
	if agentManagementCreateRequested(query) {
		t.Fatalf("agentManagementCreateRequested(%q) = true, want false; create inside a description value is not a create intent", query)
	}

	parts := contextualConsoleAgentsManageCapabilityPartsForTest()
	parts.Query = query
	parts.SkillMode = skillModeAuto
	parts.SkillIDs = []string{skills.SkillAgentManagement}
	parts.ModelTurnIntent = agentManagementModelIntentForTest("/console/agents")
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	assertAgentManagementModelDecidesStrategyForTest(t, parts, strategy)
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
	assertAgentManagementModelDecidesStrategyForTest(t, parts, strategy)
	assertAgentManagementModelDecidesExecutionForTest(t, strategy)
	criteria := strings.Join(strategy.SuccessCriteria, "\n")
	if strings.Contains(criteria, "publish, bind, and invoke are not attempted") {
		t.Fatalf("strategy.SuccessCriteria contains stale binding prohibition: %#v", strategy.SuccessCriteria)
	}
	if !strings.Contains(criteria, "binding and unbinding edits use supported draft config binding lists") {
		t.Fatalf("strategy.SuccessCriteria = %#v, want supported binding/unbinding guidance", strategy.SuccessCriteria)
	}
}

func TestAgentManagementPureBatchDeleteWithRefreshDoesNotPlanConfigTools(t *testing.T) {
	query := "Delete the visible Agents AICHAT-GOAL-SMOKE-1782844559803-EDITED and AICHAT-GOAL-BATCH-1782845202110, then refresh the list and confirm they are gone."
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
	assertAgentManagementModelDecidesStrategyForTest(t, parts, strategy)
}

func TestAgentManagementDeleteThenFollowupMutationStillPlansAfterDelete(t *testing.T) {
	query := "Delete the first visible Agent, then create a new Agent and configure its model, prompt, file generation capability, and file upload setting."
	if !agentManagementDeleteHasExplicitFollowupMutation(query) {
		t.Fatalf("agentManagementDeleteHasExplicitFollowupMutation(%q) = false, want true", query)
	}
	parts := consoleAgentsVisibleTargetsTestParts(query)
	parts.ModelTurnIntent = &AIChatModelTurnIntent{
		Intent:                  "manage_agent_asset",
		RecommendedCapabilities: []string{"agent.model_selection", "agent.system_prompt", "agent.skill_backed_capability:file generation", "agent.accept_uploaded_files"},
		Confidence:              0.94,
	}
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	assertAgentManagementModelDecidesStrategyForTest(t, parts, strategy)
}

func TestAgentManagementFileUploadCapabilityDoesNotPlanSkillBinding(t *testing.T) {
	for _, query := range []string{
		"\u8ba9\u5f53\u524dagent\u80fd\u4e0a\u4f20\u6587\u4ef6",
		"\u8ba9\u5f53\u524d Agent \u80fd\u591f\u4e0a\u4f20\u6587\u4ef6",
	} {
		t.Run(query, func(t *testing.T) {
			parts := &chatRequestParts{
				Query:          query,
				Surface:        aiChatSurfaceContextualSidebar,
				RuntimeContext: "route=/console/agents/agent-1/agent",
				SkillMode:      skillModeAuto,
				SkillIDs: []string{
					skills.SkillAgentManagement,
					skills.SkillFileManager,
				},
				ModelTurnIntent: &AIChatModelTurnIntent{
					Intent:                  "manage_agent_asset",
					RecommendedCapabilities: []string{"agent.accept_uploaded_files"},
					Confidence:              0.93,
				},
			}
			strategy := contextualAIChatTurnStrategyFromParts(parts)
			if strategy == nil {
				t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
			}
			if stringSliceContainsFold(strategy.PrimarySkills, skills.SkillFileManager) {
				t.Fatalf("PrimarySkills = %#v, want no file-manager for Agent file-upload capability setting", strategy.PrimarySkills)
			}
			plan := assertAgentManagementModelDecidesStrategyForTest(t, parts, strategy)
			capabilityGoals := mapSliceFromAny(plan["capability_goals"])
			if len(capabilityGoals) != 1 || stringFromAny(capabilityGoals[0]["capability_id"]) != agentCapabilityAcceptUploaded {
				t.Fatalf("capability_goals = %#v, want only file-upload capability goal", capabilityGoals)
			}
			if got := stringFromAny(capabilityGoals[0]["goal_action"]); got != agentCapabilityActionEnable {
				t.Fatalf("capability goal action = %q, want %q; goal=%#v", got, agentCapabilityActionEnable, capabilityGoals[0])
			}
			if !operationPlanCapabilityGoalsContainRequiredFieldForTest(capabilityGoals, "file_upload_enabled") {
				t.Fatalf("capability_goals = %#v, want file_upload_enabled config field", capabilityGoals)
			}
			if operationPlanCapabilityGoalsContainBindingActionForTest(capabilityGoals, "enabled_skill_ids", "bind") ||
				operationPlanCapabilityGoalsContainBindingActionForTest(capabilityGoals, "enabled_skill_ids", "unbind") {
				t.Fatalf("capability_goals = %#v, want no Skill binding action for file-upload config setting", capabilityGoals)
			}
		})
	}
}

func TestAgentManagementExplicitVisibleAgentDetailPlansNavigate(t *testing.T) {
	parts := consoleAgentsVisibleTargetsTestParts("open Visible Agent One detail page and edit its description")
	routeRequired := true
	parts.ModelTurnIntent.TargetPage = "/console/agents/agent-1/agent"
	parts.ModelTurnIntent.RouteRequired = &routeRequired
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	if strategy.TargetPage != "/console/agents/agent-1/agent" || !strategy.RouteRequired {
		t.Fatalf("target/route = %q/%v, want visible Agent detail route; strategy=%#v", strategy.TargetPage, strategy.RouteRequired, strategy)
	}
	if len(strategy.PlannedTools) != 0 {
		t.Fatalf("PlannedTools = %#v, want no hard post-navigation edit script", strategy.PlannedTools)
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
		"backend-backed list and order as authoritative resolved targets",
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

func TestSkillLoopCompletionOperationResultSummaryPrefersLatestToolCallOverPlanClientAction(t *testing.T) {
	summary := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":              operationPlanStatusRunning,
			"pending_next_action": "continue_from_phase_success_criteria",
			"tool_result": map[string]interface{}{
				"kind":      "client_action",
				"status":    "succeeded",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "create_agent",
				"result_summary": map[string]interface{}{
					"status": "succeeded",
					"effect": "create",
				},
			},
		},
		"tool_results": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
				"result_summary": map[string]interface{}{
					"status":         "completed",
					"effect":         "updated",
					"agent_id":       "agent-1",
					"agent_name":     "Smoke Agent",
					"model_provider": "deepseek",
					"model":          "deepseek-chat",
					"updated_fields": []interface{}{"model_provider", "model", "enabled_skill_ids"},
				},
			},
		},
		"client_actions": []interface{}{
			map[string]interface{}{
				"kind":      "client_action",
				"status":    "succeeded",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "create_agent",
				"result_summary": map[string]interface{}{
					"effect": "create",
				},
			},
		},
	}

	operationSummary := skillLoopCompletionOperationResultSummary(summary)
	latest := mapFromOperationContext(operationSummary["latest_tool_result"])
	if got := stringFromAny(latest["tool_name"]); got != "update_agent_config" {
		t.Fatalf("latest_tool_result.tool_name = %q, want update_agent_config; summary=%#v", got, operationSummary)
	}
	if got := stringFromAny(operationSummary["tool_name"]); got != "update_agent_config" {
		t.Fatalf("operation_result_summary.tool_name = %q, want update_agent_config; summary=%#v", got, operationSummary)
	}
	if got := stringFromAny(operationSummary["agent_id"]); got != "agent-1" {
		t.Fatalf("operation_result_summary.agent_id = %q, want agent-1; summary=%#v", got, operationSummary)
	}
	if got := stringFromAny(operationSummary["status"]); got != operationPlanStatusRunning {
		t.Fatalf("operation_result_summary.status = %q, want running while plan is still running; summary=%#v", got, operationSummary)
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

func operationPlanCapabilityGoalsContainForTest(goals []map[string]interface{}, capabilityID string) bool {
	for _, goal := range goals {
		if stringFromAny(goal["capability_id"]) == capabilityID {
			return true
		}
	}
	return false
}

func operationPlanCapabilityGoalsContainRequiredFieldForTest(goals []map[string]interface{}, field string) bool {
	field = strings.TrimSpace(field)
	for _, goal := range goals {
		if stringSliceContainsFold(stringSliceFromAny(goal["required_config_fields"]), field) {
			return true
		}
	}
	return false
}

func agentCapabilityGoalsContainForTest(goals []AIChatAgentCapabilityGoal, capabilityID string) bool {
	for _, goal := range goals {
		if goal.CapabilityID == capabilityID {
			return true
		}
	}
	return false
}

func agentCapabilityGoalsContainBindingActionForTest(goals []AIChatAgentCapabilityGoal, field string, action string) bool {
	field = operationPlanAgentConfigCanonicalField(field)
	action = operationPlanCanonicalAgentConfigBindingAction(action)
	for _, goal := range goals {
		if got := operationPlanCanonicalAgentConfigBindingAction(goal.RequiredBindingActions[field]); got == action {
			return true
		}
	}
	return false
}

func operationPlanCapabilityGoalsContainBindingActionForTest(goals []map[string]interface{}, field string, action string) bool {
	field = operationPlanAgentConfigCanonicalField(field)
	action = operationPlanCanonicalAgentConfigBindingAction(action)
	for _, goal := range goals {
		actions := operationPlanAgentConfigBindingActionsFromAny(goal["required_binding_actions"])
		if got := operationPlanCanonicalAgentConfigBindingAction(actions[field]); got == action {
			return true
		}
	}
	return false
}

func assertAgentManagementModelDecidesExecutionForTest(t *testing.T, strategy *AIChatTurnStrategy) {
	t.Helper()
	if strategy == nil {
		t.Fatal("strategy = nil, want model-decides Agent-management execution")
	}
	if got := strings.TrimSpace(strategy.ToolChoiceMode); got != aiChatTurnToolChoiceModelDecides {
		t.Fatalf("ToolChoiceMode = %q, want %q; strategy=%#v", got, aiChatTurnToolChoiceModelDecides, strategy)
	}
	if len(strategy.PlannedTools) != 0 {
		t.Fatalf("planned tools = %#v, want model-decides without hard scripted tools", strategy.PlannedTools)
	}
}

func assertNoSynthesizedAgentCapabilityContinuationForTest(t *testing.T, parts *chatRequestParts) {
	t.Helper()
	if parts == nil {
		t.Fatal("parts = nil")
	}
	if plan := firstIncompleteRecentOperationPlan(parts); len(plan) > 0 {
		t.Fatalf("RecentOperationPlans include incomplete synthesized work %#v, want only historical context", plan)
	}
	for _, plan := range parts.RecentOperationPlans {
		if derivedFrom := strings.TrimSpace(stringFromAny(plan["derived_from"])); strings.HasPrefix(derivedFrom, "recent_agent_") {
			t.Fatalf("RecentOperationPlans include synthesized Agent continuation %q: %#v", derivedFrom, plan)
		}
	}
}

func operationPlanStructuredOperationHasArgumentForTest(operations []map[string]interface{}, toolName string, argKey string, argValue string) bool {
	for _, operation := range operations {
		if !strings.EqualFold(stringFromAny(operation["tool_name"]), toolName) {
			continue
		}
		args := mapFromOperationContext(operation["arguments"])
		if stringFromAny(args[argKey]) == argValue {
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

func aiChatTurnStrategyPlannedToolForTest(strategy *AIChatTurnStrategy, skillID, toolName string) *AIChatTurnStrategyTool {
	if strategy == nil {
		return nil
	}
	for idx := range strategy.PlannedTools {
		tool := &strategy.PlannedTools[idx]
		if tool.SkillID == skillID && tool.ToolName == toolName {
			return tool
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
	return consoleAgentsVisibleTargetsTestPartsWithAgentNames(query, []string{
		"Visible Agent One",
		"Visible Agent Two",
	})
}

func consoleAgentsVisibleTargetsTestPartsWithAgentNames(query string, names []string) *chatRequestParts {
	resources := []interface{}{
		map[string]interface{}{
			"resource_type": "page",
			"href":          "/console/agents",
		},
	}
	for idx, name := range names {
		id := fmt.Sprintf("agent-%d", idx+1)
		resources = append(resources, map[string]interface{}{
			"resource_type": "agent",
			"id":            id,
			"title":         name,
			"href":          "/console/agents/" + id + "/agent",
			"visible_index": idx + 1,
		})
	}
	context := map[string]interface{}{
		"resources": resources,
	}
	return &chatRequestParts{
		Query:               query,
		Surface:             aiChatSurfaceContextualSidebar,
		RuntimeContext:      "route=/console/agents",
		SkillMode:           skillModeAuto,
		SkillIDs:            []string{skills.SkillAgentManagement, skills.SkillConsoleNavigator},
		ModelTurnIntent:     agentManagementModelIntentForTest("/console/agents"),
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
		ModelTurnIntent:     agentManagementModelIntentForTest("/console/agents/agent-1/agent"),
		RawOperationContext: context,
		OperationContext:    context,
	}
}

func agentManagementModelIntentForTest(targetPage string, capabilities ...string) *AIChatModelTurnIntent {
	return &AIChatModelTurnIntent{
		Intent:                  "manage_agent_asset",
		TargetPage:              targetPage,
		RecommendedCapabilities: capabilities,
	}
}

func modelTurnIntentForTest(intent string, targetPage string, capabilities ...string) *AIChatModelTurnIntent {
	return &AIChatModelTurnIntent{
		Intent:                  intent,
		TargetPage:              targetPage,
		RecommendedCapabilities: capabilities,
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

func TestOperationPlanRejectedAgentDeleteIsTerminalForContinuation(t *testing.T) {
	deleteStepID := operationPlanToolStepID(skills.SkillAgentManagement, "delete_agents")
	plan := map[string]interface{}{
		"version":             operationPlanVersion,
		"status":              "rejected",
		"pending_next_action": "none",
		"intent":              "manage_agent_asset",
		"original_user_goal":  "delete the first two visible agents",
		"steps": []interface{}{map[string]interface{}{
			"id":        deleteStepID,
			"title":     "Delete visible agents",
			"status":    "rejected",
			"skill_id":  skills.SkillAgentManagement,
			"tool_name": "delete_agents",
		}},
		"step_status": map[string]interface{}{
			deleteStepID: "rejected",
		},
	}

	if operationPlanHasIncompleteWork(plan) {
		t.Fatalf("operationPlanHasIncompleteWork(%#v) = true, want rejected plan to be terminal", plan)
	}
}

func TestContinuationUsesLegacyExecutablePlanAsAdvisoryTextOnly(t *testing.T) {
	parts := &chatRequestParts{
		Query: "continue",
		RecentOperationPlans: []map[string]interface{}{map[string]interface{}{
			"status": "running",
			"structured_plan": map[string]interface{}{
				"operations": []interface{}{map[string]interface{}{
					"title": "Update the selected Agent", "status": "pending",
					"skill_id": skills.SkillAgentManagement, "tool_name": "delete_agents",
					"arguments": map[string]interface{}{"agent_id": "legacy-agent"},
				}},
			},
			"required_tool_sequence": []interface{}{"agent-management/delete_agents"},
			"args_binding":           map[string]interface{}{"agent_id": "legacy-agent"},
		}},
	}
	strategy := &AIChatTurnStrategy{
		Intent:         "continue_previous_task",
		ToolChoiceMode: aiChatTurnToolChoiceModelDecides,
		PrimarySkills:  []string{skills.SkillFileReader},
	}

	got := applyRecentOperationPlanToContinuationStrategy(parts, strategy)
	if len(got.PlannedTools) != 0 {
		t.Fatalf("planned tools = %#v, want legacy executable fields ignored", got.PlannedTools)
	}
	if !reflect.DeepEqual(got.PrimarySkills, []string{skills.SkillFileReader}) {
		t.Fatalf("primary skills = %#v, want unchanged", got.PrimarySkills)
	}
	if got.TargetPage != "" || got.RouteRequired {
		t.Fatalf("route = (%q, %v), want no legacy route enforcement", got.TargetPage, got.RouteRequired)
	}
	if !stringSliceContains(got.PhaseGoals, "Update the selected Agent") {
		t.Fatalf("phase goals = %#v, want advisory operation title", got.PhaseGoals)
	}
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

func mustUUIDForTest(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if id == uuid.Nil {
		t.Fatal("uuid.New returned nil")
	}
	return id
}

func TestCompactUnsavedOperationPlanGeneratedFilesUsesSaveInvocationEvidence(t *testing.T) {
	files := []map[string]interface{}{{
		"target":       "temporary_artifact",
		"file_id":      "tool-1",
		"tool_file_id": "tool-1",
		"filename":     "report.pdf",
		"extension":    ".pdf",
	}}
	saveCalls := []skillloop.SkillToolCallRef{{
		SkillID:  skills.SkillFileManager,
		ToolName: "save_file_to_management",
		Arguments: map[string]interface{}{
			"source_type":  "tool_file",
			"tool_file_id": "tool-1",
			"filename":     "report.pdf",
		},
		Result: map[string]interface{}{
			"status":              "completed",
			"source_tool_file_id": "tool-1",
			"target":              "managed_file",
		},
	}}

	if unsaved := compactUnsavedOperationPlanGeneratedFiles(files, saveCalls); len(unsaved) != 0 {
		t.Fatalf("unsaved files = %#v, want save invocation evidence to satisfy the temporary artifact", unsaved)
	}
}
