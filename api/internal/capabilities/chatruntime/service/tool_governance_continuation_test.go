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

	if toolGovernanceFrozenContinuationNeedsSkillLoop(prepared) {
		t.Fatal("toolGovernanceFrozenContinuationNeedsSkillLoop() = true, want false once no executable follow-up remains")
	}
}

func TestToolGovernanceFrozenContinuationNeedsSkillLoopForPendingPostUpdateRead(t *testing.T) {
	updateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")
	readStepID := operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config")
	prepared := &PreparedChat{
		parts: &chatRequestParts{},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"steps": []interface{}{
					map[string]interface{}{
						"id":        updateStepID,
						"status":    operationPlanStepStatusCompleted,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "update_agent_config",
					},
					map[string]interface{}{
						"id":                                readStepID,
						"status":                            operationPlanStepStatusPending,
						"skill_id":                          skills.SkillAgentManagement,
						"tool_name":                         "get_agent_config",
						"required_post_update_verification": true,
					},
				},
				"step_status": map[string]interface{}{
					updateStepID: operationPlanStepStatusCompleted,
					readStepID:   operationPlanStepStatusPending,
				},
			},
		}},
	}

	if !toolGovernanceFrozenContinuationNeedsSkillLoop(prepared) {
		t.Fatal("toolGovernanceFrozenContinuationNeedsSkillLoop() = false, want true for pending post-update verification read")
	}
}

func TestToolGovernanceFrozenContinuationNeedsSkillLoopForModelDecidesPendingAgentMutation(t *testing.T) {
	deleteStepID := operationPlanToolStepID(skills.SkillAgentManagement, "delete_agent")
	createStepID := operationPlanToolStepID(skills.SkillAgentManagement, "create_agent")
	prepared := &PreparedChat{
		parts: &chatRequestParts{},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status":           operationPlanStatusRunning,
				"tool_choice_mode": aiChatTurnToolChoiceModelDecides,
				"planning_mode":    "phase_only_model_decides",
				"steps": []interface{}{
					map[string]interface{}{
						"id":        deleteStepID,
						"status":    operationPlanStepStatusCompleted,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "delete_agent",
					},
					map[string]interface{}{
						"id":        createStepID,
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "create_agent",
					},
				},
				"step_status": map[string]interface{}{
					deleteStepID: operationPlanStepStatusCompleted,
					createStepID: operationPlanStepStatusPending,
				},
				"pending_next_action": "Run tool:agent-management/create_agent",
			},
		}},
	}

	if !toolGovernanceFrozenContinuationNeedsSkillLoop(prepared) {
		t.Fatal("toolGovernanceFrozenContinuationNeedsSkillLoop() = false, want model-decides pending Agent mutation to continue the skill loop")
	}

	answer, ok := toolGovernanceFrozenFastPathAnswer(prepared, skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "delete_agent",
		Status:   "success",
		Result: map[string]interface{}{
			"status":     "completed",
			"agent_name": "Old Agent",
		},
	})
	if ok {
		t.Fatalf("toolGovernanceFrozenFastPathAnswer() = (%q, true), want pending model-decides Agent mutation to block fast-path completion", answer)
	}
}

func TestToolGovernanceFrozenContinuationNeedsSkillLoopForModelDecidesOpenPhase(t *testing.T) {
	updateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_identity")
	prepared := &PreparedChat{
		parts: &chatRequestParts{},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status":           operationPlanStatusRunning,
				"tool_choice_mode": aiChatTurnToolChoiceModelDecides,
				"planning_mode":    "phase_only_model_decides",
				"phases": []interface{}{
					map[string]interface{}{"id": "understand_context", "status": operationPlanStepStatusCompleted},
					map[string]interface{}{"id": "act_with_tools", "status": operationPlanStepStatusPending},
					map[string]interface{}{"id": "verify_result", "status": operationPlanStepStatusPending},
				},
				"steps": []interface{}{
					map[string]interface{}{
						"id":        updateStepID,
						"status":    operationPlanStepStatusCompleted,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "update_agent_identity",
					},
				},
				"step_status": map[string]interface{}{
					updateStepID: operationPlanStepStatusCompleted,
				},
				"pending_next_action": "continue_from_phase_success_criteria",
			},
		}},
	}
	trace := skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_identity",
		Status:   "success",
		Result: map[string]interface{}{
			"status":         "completed",
			"agent_id":       "agent-1",
			"agent_name":     "Smoke Agent Edited",
			"updated_fields": []interface{}{"description"},
		},
	}

	if !toolGovernanceFrozenContinuationNeedsSkillLoop(prepared) {
		t.Fatal("toolGovernanceFrozenContinuationNeedsSkillLoop() = false, want model-decides open phase to continue the skill loop")
	}
	if answer, ok := toolGovernanceFrozenFastPathAnswer(prepared, trace); ok {
		t.Fatalf("toolGovernanceFrozenFastPathAnswer() = (%q, true), want open model-decides phase to block fast-path completion", answer)
	}
}

func TestToolGovernanceFrozenContinuationSkipsVerifiedModelDecidesPhase(t *testing.T) {
	updateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_identity")
	prepared := &PreparedChat{
		parts: &chatRequestParts{},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status":           operationPlanStatusCompleted,
				"tool_choice_mode": aiChatTurnToolChoiceModelDecides,
				"planning_mode":    "phase_only_model_decides",
				"completion_verification": map[string]interface{}{
					"status": "pass",
				},
				"phases": []interface{}{
					map[string]interface{}{"id": "act_with_tools", "status": operationPlanStepStatusPending},
					map[string]interface{}{"id": "verify_result", "status": operationPlanStepStatusPending},
				},
				"steps": []interface{}{
					map[string]interface{}{
						"id":        updateStepID,
						"status":    operationPlanStepStatusCompleted,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "update_agent_identity",
					},
				},
				"step_status": map[string]interface{}{
					updateStepID: operationPlanStepStatusCompleted,
				},
				"pending_next_action": "none",
			},
		}},
	}

	if toolGovernanceFrozenContinuationNeedsSkillLoop(prepared) {
		t.Fatal("toolGovernanceFrozenContinuationNeedsSkillLoop() = true, want verified model-decides phase to finish")
	}
}

func TestToolGovernanceFrozenFastPathWaitsForModelDecidesPostUpdateRead(t *testing.T) {
	updateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")
	readStepID := operationPlanPostUpdateAgentConfigReadStepID()
	prepared := &PreparedChat{
		parts: &chatRequestParts{Query: "update the agent config and verify it"},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status":           operationPlanStatusRunning,
				"tool_choice_mode": aiChatTurnToolChoiceModelDecides,
				"planning_mode":    "phase_only_model_decides",
				"steps": []interface{}{
					map[string]interface{}{
						"id":        updateStepID,
						"status":    operationPlanStepStatusCompleted,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "update_agent_config",
					},
					map[string]interface{}{
						"id":                                readStepID,
						"status":                            operationPlanStepStatusPending,
						"skill_id":                          skills.SkillAgentManagement,
						"tool_name":                         "get_agent_config",
						"phase":                             "post_update_verification",
						"required_post_update_verification": true,
					},
				},
				"step_status": map[string]interface{}{
					updateStepID: operationPlanStepStatusCompleted,
					readStepID:   operationPlanStepStatusPending,
				},
				"pending_next_action": "Run tool:agent-management/get_agent_config",
			},
		}},
	}
	trace := skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Status:   "success",
		Result: map[string]interface{}{
			"status":         "completed",
			"agent_name":     "Novel Master",
			"updated_fields": []interface{}{"model", "system_prompt", "file_upload_enabled"},
		},
	}

	if !toolGovernanceFrozenContinuationNeedsSkillLoop(prepared) {
		t.Fatal("toolGovernanceFrozenContinuationNeedsSkillLoop() = false, want model-decides pending post-update read to continue the skill loop")
	}
	if answer, ok := toolGovernanceFrozenFastPathAnswer(prepared, trace); ok {
		t.Fatalf("toolGovernanceFrozenFastPathAnswer() = (%q, true), want post-update read to block fast-path completion", answer)
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
						"id":                      updateStepID,
						"status":                  operationPlanStepStatusPending,
						"skill_id":                skills.SkillAgentManagement,
						"tool_name":               "update_agent_config",
						"expected_updated_fields": []interface{}{"home_title", "input_placeholder", "theme_color"},
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

func TestToolGovernanceFrozenFastPathWaitsForPendingRouteFollowup(t *testing.T) {
	deleteStepID := operationPlanToolStepID(skills.SkillAgentManagement, "delete_agents")
	routeStepID := operationPlanRouteStepID("/console/agents/agent-4/agent", 1)
	prepared := &PreparedChat{
		parts: &chatRequestParts{Query: "delete then open the first remaining agent"},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status": operationPlanStatusRunning,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        deleteStepID,
						"status":    operationPlanStepStatusCompleted,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "delete_agents",
					},
					map[string]interface{}{
						"id":        routeStepID,
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillConsoleNavigator,
						"tool_name": "navigate",
						"wait_for":  deleteStepID,
						"asset_target": map[string]interface{}{
							"page": "/console/agents/agent-4/agent",
						},
					},
				},
				"step_status": map[string]interface{}{
					deleteStepID: operationPlanStepStatusCompleted,
					routeStepID:  operationPlanStepStatusPending,
				},
				"pending_next_action": operationPlanToolStepTitle(skills.SkillConsoleNavigator, "navigate"),
			},
		}},
	}

	answer, ok := toolGovernanceFrozenFastPathAnswer(prepared, skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "delete_agents",
		Status:   "success",
		Result: map[string]interface{}{
			"status":        "completed",
			"target_count":  3,
			"deleted_count": 3,
			"failed_count":  0,
		},
	})

	if ok {
		t.Fatalf("toolGovernanceFrozenFastPathAnswer() = (%q, true), want pending route step to keep skill loop running", answer)
	}
}

func TestToolGovernanceFrozenSimpleConfigFastPathWaitsForPendingIdentityUpdate(t *testing.T) {
	identityStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_identity")
	configStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")
	prepared := &PreparedChat{
		parts: &chatRequestParts{Query: "update agent identity and config"},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status": operationPlanStatusRunning,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        identityStepID,
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "update_agent_identity",
					},
					map[string]interface{}{
						"id":        configStepID,
						"status":    operationPlanStepStatusCompleted,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "update_agent_config",
						operationPlanExpectedUpdatedFieldsKey: []interface{}{
							"system_prompt",
							"home_title",
							"suggested_questions",
						},
					},
				},
				"step_status": map[string]interface{}{
					identityStepID: operationPlanStepStatusPending,
					configStepID:   operationPlanStepStatusCompleted,
				},
				"pending_next_action": operationPlanToolStepTitle(skills.SkillAgentManagement, "update_agent_identity"),
			},
		}},
	}
	trace := skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Status:   "success",
		Result: map[string]interface{}{
			"status":     "completed",
			"agent_id":   "agent-1",
			"agent_name": "Support Agent",
			"updated_fields": []interface{}{
				"system_prompt",
				"home_title",
				"suggested_questions",
			},
		},
	}

	answer, ok := toolGovernanceFrozenSimpleAgentConfigFastPathAnswer(prepared, trace)
	if ok {
		t.Fatalf("toolGovernanceFrozenSimpleAgentConfigFastPathAnswer() = (%q, true), want pending identity update to keep skill loop running", answer)
	}
}

func TestToolGovernanceFrozenFastPathAggregatesCompletedIdentityAndConfig(t *testing.T) {
	identityStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_identity")
	configStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")
	prepared := &PreparedChat{
		parts: &chatRequestParts{Query: "update agent identity and homepage title"},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status": operationPlanStatusCompleted,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        identityStepID,
						"status":    operationPlanStepStatusCompleted,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "update_agent_identity",
					},
					map[string]interface{}{
						"id":                                  configStepID,
						"status":                              operationPlanStepStatusCompleted,
						"skill_id":                            skills.SkillAgentManagement,
						"tool_name":                           "update_agent_config",
						operationPlanExpectedUpdatedFieldsKey: []interface{}{"home_title"},
					},
				},
				"step_status": map[string]interface{}{
					identityStepID: operationPlanStepStatusCompleted,
					configStepID:   operationPlanStepStatusCompleted,
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
			},
		}},
	}
	trace := skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Status:   "success",
		Result: map[string]interface{}{
			"status":         "completed",
			"agent_id":       "agent-1",
			"agent_name":     "Support Agent Edited",
			"updated_fields": []interface{}{"home_title"},
			"home_title":     "Welcome Home",
		},
	}
	prepared.Message.Metadata = mergeFrozenContinuationToolTraceMetadata(prepared.Message.Metadata, trace)
	prepared.Message.Metadata = preparedResultMetadata(prepared.Message.Metadata, nil)

	answer, ok := toolGovernanceFrozenFastPathAnswer(prepared, trace)
	if !ok {
		t.Fatal("toolGovernanceFrozenFastPathAnswer() ok = false, want aggregate answer")
	}
	for _, want := range []string{
		"Support Agent Edited",
		"\u57fa\u7840\u4fe1\u606f",
		"\u8fd0\u884c\u914d\u7f6e",
		"Welcome Home",
	} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want %q", answer, want)
		}
	}
}

func TestToolGovernanceFrozenFastPathCoversAgentIdentityUpdate(t *testing.T) {
	updateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_identity")
	prepared := &PreparedChat{
		parts: &chatRequestParts{Query: "把当前智能体描述改成八次更新，不要切换页面"},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status": operationPlanStatusRunning,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        updateStepID,
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "update_agent_identity",
					},
					map[string]interface{}{
						"id":     "observe_current_page",
						"title":  "Observe current page result",
						"status": operationPlanStepStatusPending,
					},
				},
				"step_status": map[string]interface{}{
					updateStepID:           operationPlanStepStatusPending,
					"observe_current_page": operationPlanStepStatusPending,
				},
				"pending_next_action": operationPlanToolStepTitle(skills.SkillAgentManagement, "update_agent_identity"),
			},
		}},
	}
	trace := skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_identity",
		Status:   "success",
		Result: map[string]interface{}{
			"status":         "completed",
			"effect":         "updated",
			"agent_id":       "agent-1",
			"agent_name":     "客服智能体",
			"updated_fields": []interface{}{"description"},
		},
	}
	ensureOperationPlanInvocationStep(prepared.Message.Metadata, skillInvocationFromTrace(trace, 0))

	answer, ok := toolGovernanceFrozenFastPathAnswer(prepared, trace)
	if !ok {
		t.Fatalf("toolGovernanceFrozenFastPathAnswer() ok = false, want identity update result to finish governed turn; metadata=%#v", prepared.Message.Metadata)
	}
	for _, want := range []string{"智能体「客服智能体」", "描述"} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want %q", answer, want)
		}
	}
}

func TestToolGovernanceFrozenFastPathCoversConfigUpdateWithStalePreRead(t *testing.T) {
	readStepID := operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config")
	updateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")
	prepared := &PreparedChat{
		parts: &chatRequestParts{Query: "Change the current Agent home title, input placeholder, and theme color. Do not change other config."},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status": operationPlanStatusRunning,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        readStepID,
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "get_agent_config",
					},
					map[string]interface{}{
						"id":        updateStepID,
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "update_agent_config",
					},
					map[string]interface{}{
						"id":     "observe",
						"title":  "Observe result",
						"status": operationPlanStepStatusPending,
					},
				},
				"step_status": map[string]interface{}{
					readStepID:   operationPlanStepStatusPending,
					updateStepID: operationPlanStepStatusPending,
					"observe":    operationPlanStepStatusPending,
				},
				"pending_next_action": operationPlanToolStepTitle(skills.SkillAgentManagement, "update_agent_config"),
			},
		}},
	}
	trace := skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Status:   "success",
		Result: map[string]interface{}{
			"status":            "completed",
			"effect":            "updated",
			"agent_id":          "agent-1",
			"agent_name":        "Support Agent",
			"home_title":        "AIChat Display Smoke",
			"input_placeholder": "Ask me anything",
			"theme_color":       "emerald",
			"updated_fields": []interface{}{
				"home_title",
				"input_placeholder",
				"theme_color",
			},
		},
	}
	ensureOperationPlanInvocationStep(prepared.Message.Metadata, skillInvocationFromTrace(trace, 0))

	answer, ok := toolGovernanceFrozenFastPathAnswer(prepared, trace)
	if !ok {
		t.Fatalf("toolGovernanceFrozenFastPathAnswer() ok = false, want config update to close despite stale pre-read; metadata=%#v", prepared.Message.Metadata)
	}
	for _, want := range []string{"Support Agent", "AIChat Display Smoke", "Ask me anything", "emerald"} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want %q", answer, want)
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
