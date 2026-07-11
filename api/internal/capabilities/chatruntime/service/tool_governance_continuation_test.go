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

func TestToolGovernanceContinuationPlanStateSummaryIncludesReadFileEvidenceFacts(t *testing.T) {
	readStepID := operationPlanToolStepID(skills.SkillFileReader, "read_file")
	createStepID := operationPlanToolStepID(skills.SkillAgentManagement, "create_agent")
	message := &runtimemodel.Message{Metadata: map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": operationPlanStatusRunning,
			"steps": []interface{}{
				map[string]interface{}{
					"id":        readStepID,
					"status":    operationPlanStepStatusCompleted,
					"skill_id":  skills.SkillFileReader,
					"tool_name": "read_file",
				},
				map[string]interface{}{
					"id":        createStepID,
					"status":    operationPlanStepStatusPending,
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "create_agent",
				},
			},
			"step_status": map[string]interface{}{
				readStepID:   operationPlanStepStatusCompleted,
				createStepID: operationPlanStepStatusPending,
			},
			operationPlanEvidenceLedgerKey: []interface{}{
				map[string]interface{}{
					"keys":      []interface{}{"file:read"},
					"skill_id":  skills.SkillFileReader,
					"tool_name": "read_file",
					"kind":      "tool_call",
					"status":    operationPlanStepStatusCompleted,
					"result_facts": map[string]interface{}{
						"file_name":             "新建 文本文档.txt",
						"content_status":        "extracted",
						"content_value_preview": "测试代码111",
						"content_value_source":  "read_file.content",
					},
				},
			},
		},
	}}

	summary := toolGovernanceContinuationPlanStateSummary(message)
	ledger := mapSliceFromAny(summary["evidence_ledger"])
	for _, entry := range ledger {
		facts := mapFromOperationContext(entry["result_facts"])
		if stringFromAny(facts["content_value_preview"]) == "测试代码111" {
			return
		}
	}
	t.Fatalf("evidence_ledger = %#v, want read_file content_value_preview fact", ledger)
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

func TestMergeFrozenContinuationToolTraceMetadataClosesAgentConfigPlan(t *testing.T) {
	readStepID := operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config")
	updateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")
	knowledgeStepID := operationPlanToolStepID(skills.SkillAgentManagement, "list_agent_knowledge_candidates")
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": operationPlanStatusRunning,
			"steps": []interface{}{
				map[string]interface{}{
					"id":        readStepID,
					"status":    operationPlanStepStatusCompleted,
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "get_agent_config",
				},
				map[string]interface{}{
					"id":                                   updateStepID,
					"status":                               operationPlanStepStatusPending,
					"skill_id":                             skills.SkillAgentManagement,
					"tool_name":                            "update_agent_config",
					operationPlanExpectedUpdatedFieldsKey:  []interface{}{"knowledge_dataset_ids", "database_bindings", "workflow_bindings"},
					operationPlanExpectedBindingActionsKey: "knowledge_dataset_ids:unbind,database_bindings:unbind,workflow_bindings:unbind",
				},
				map[string]interface{}{
					"id":        knowledgeStepID,
					"status":    operationPlanStepStatusPending,
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "list_agent_knowledge_candidates",
				},
				map[string]interface{}{
					"id":     "observe",
					"title":  "Observe result",
					"status": operationPlanStepStatusPending,
				},
			},
			"step_status": map[string]interface{}{
				readStepID:      operationPlanStepStatusCompleted,
				updateStepID:    operationPlanStepStatusPending,
				knowledgeStepID: operationPlanStepStatusPending,
				"observe":       operationPlanStepStatusPending,
			},
			"pending_next_action": operationPlanToolStepTitle(skills.SkillAgentManagement, "update_agent_config"),
		},
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":       "tool_call",
				"skill_id":   skills.SkillAgentManagement,
				"tool_name":  "update_agent_config",
				"status":     "approved",
				"runtime_id": "tool_call:agent-management:update_agent_config::#1",
			},
			map[string]interface{}{
				"kind":       "tool_governance",
				"skill_id":   skills.SkillAgentManagement,
				"tool_name":  "update_agent_config",
				"status":     "success",
				"runtime_id": "tool_governance:corr-config",
			},
		},
	}
	trace := skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Status:   "success",
		Result: map[string]interface{}{
			"status":           "completed",
			"effect":           "updated",
			"agent_id":         "agent-1",
			"agent_name":       "Support Agent",
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
	}

	metadata = mergeFrozenContinuationToolTraceMetadata(metadata, trace)
	metadata = preparedResultMetadata(metadata, nil)

	invocations := skillInvocationsFromMetadata(metadata["skill_invocations"])
	var toolCall map[string]interface{}
	for _, invocation := range invocations {
		if strings.EqualFold(stringFromAny(invocation["kind"]), "tool_call") &&
			strings.EqualFold(stringFromAny(invocation["skill_id"]), skills.SkillAgentManagement) &&
			strings.EqualFold(stringFromAny(invocation["tool_name"]), "update_agent_config") {
			toolCall = invocation
		}
	}
	if len(toolCall) == 0 {
		t.Fatalf("skill_invocations = %#v, want merged update_agent_config tool_call", invocations)
	}
	if got := stringFromAny(toolCall["runtime_id"]); got != "tool_call:agent-management:update_agent_config::#1" {
		t.Fatalf("tool_call runtime_id = %q, want existing approved runtime id; invocation=%#v", got, toolCall)
	}
	if result := mapFromOperationContext(toolCall["result"]); len(result) == 0 || stringFromAny(result["agent_name"]) != "Support Agent" {
		t.Fatalf("tool_call result = %#v, want continuation tool result", toolCall["result"])
	}

	plan := mapFromOperationContext(metadata["operation_plan"])
	if got := stringFromAny(plan["status"]); got != operationPlanStatusCompleted {
		t.Fatalf("plan status = %q, want completed; plan=%#v", got, plan)
	}
	if got := operationPlanStepStatusForTest(plan, updateStepID); got != operationPlanStepStatusCompleted {
		t.Fatalf("update step status = %q, want completed; plan=%#v", got, plan)
	}
	for _, covered := range []string{knowledgeStepID, "observe"} {
		if got := operationPlanStepStatusForTest(plan, covered); got != operationPlanStepStatusCompleted {
			t.Fatalf("%s status = %q, want covered completed; plan=%#v", covered, got, plan)
		}
	}
	summary := mapFromOperationContext(metadata["operation_result_summary"])
	latest := mapFromOperationContext(summary["latest_tool_result"])
	if stringFromAny(latest["tool_name"]) != "update_agent_config" {
		t.Fatalf("operation_result_summary.latest_tool_result = %#v, want update_agent_config", latest)
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
