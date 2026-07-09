package skillloop

import (
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestCompletionGateDeterministicPassForVerifiedAgentSystemPrompt(t *testing.T) {
	evidence := completionGateVerifiedSystemPromptEvidence("agent.system_prompt_update")

	decision := completionGateEvaluate(evidence, "已完成")
	if decision.Path != completionGateDeterministicPass {
		t.Fatalf("completionGateEvaluate().Path = %q, want %q; decision=%#v", decision.Path, completionGateDeterministicPass, decision)
	}
	if strings.TrimSpace(decision.FinalAnswer) == "" {
		t.Fatalf("completionGateEvaluate().FinalAnswer is empty; decision=%#v", decision)
	}
	if completionVerificationShouldRun(evidence, nil, nil, 6) {
		t.Fatal("completionVerificationShouldRun() = true, want false when deterministic ledger coverage is sufficient")
	}
}

func TestCompletionGateBlocksKnowledgeBindingContractSystemPromptEvidenceDrift(t *testing.T) {
	evidence := completionGateVerifiedSystemPromptEvidence("agent.knowledge_binding")

	decision := completionGateEvaluate(evidence, "已完成")
	if decision.Path != completionGateNeedsAction {
		t.Fatalf("completionGateEvaluate().Path = %q, want %q; decision=%#v", decision.Path, completionGateNeedsAction, decision)
	}
	if !completionGateTestStringSliceContains(decision.MissingFacts, "missing_fact: agent.knowledge_binding") {
		t.Fatalf("MissingFacts = %#v, want knowledge binding missing fact", decision.MissingFacts)
	}
}

func completionGateTestStringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestCompletionGateAsksUserForLowConfidenceContractDrift(t *testing.T) {
	evidence := completionGateVerifiedSystemPromptEvidence("agent.knowledge_binding")
	contract := evidenceMapFromAny(evidenceMapFromAny(evidence["operation_plan"])["task_contract"])
	contract["low_confidence"] = true

	decision := completionGateEvaluate(evidence, "已完成")
	if decision.Path != completionGateAskUser {
		t.Fatalf("completionGateEvaluate().Path = %q, want %q; decision=%#v", decision.Path, completionGateAskUser, decision)
	}
	if strings.TrimSpace(decision.FinalAnswer) == "" {
		t.Fatalf("FinalAnswer is empty for ask_user decision: %#v", decision)
	}
}

func TestCompletionGateBlocksPartialFileWorkflowWhenAgentAndManagedPDFMissing(t *testing.T) {
	evidence := map[string]interface{}{
		"user_request": "rewrite the first md, generate a pdf in file management, then update this Agent with the new chapter",
		"operation_plan": map[string]interface{}{
			"tool_choice_mode": operationPlanToolChoiceModelDecides,
			"planning_mode":    "phase_only_model_decides",
			"status":           "running",
			"task_contract": map[string]interface{}{
				"intended_effect": "agent.system_prompt_update",
			},
			"phases": []interface{}{
				map[string]interface{}{"id": "read_first_md_file_content", "title": "read first md file content"},
				map[string]interface{}{"id": "generate_fancy_pdf_from_content", "title": "generate fancy pdf from content"},
				map[string]interface{}{"id": "save_pdf_to_file_management", "title": "save pdf to file management"},
				map[string]interface{}{"id": "update_agent_with_new_chapter_content", "title": "update Agent with new chapter content"},
			},
		},
		"evidence_ledger": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "completed",
				"skill_id":  skills.SkillFileReader,
				"tool_name": "read_file",
				"result_facts": map[string]interface{}{
					"status":         "completed",
					"file_extension": "md",
					"content_chars":  1493,
				},
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "completed",
				"skill_id":  skills.SkillFileGenerator,
				"tool_name": "generate_pdf",
				"result_facts": map[string]interface{}{
					"status":         "success",
					"target":         "temporary_artifact",
					"file_extension": "pdf",
				},
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "completed",
				"skill_id":  skills.SkillFileManager,
				"tool_name": "save_file_to_management",
				"result_facts": map[string]interface{}{
					"status":         "completed",
					"target":         "managed_file",
					"file_extension": "md",
					"file_id":        "managed-md",
					"filename":       "chapter.md",
				},
			},
		},
	}

	decision := completionGateEvaluate(evidence, "The PDF file has been generated.")
	if decision.Path != completionGateNeedsAction {
		t.Fatalf("completionGateEvaluate().Path = %q, want %q; decision=%#v", decision.Path, completionGateNeedsAction, decision)
	}
	for _, want := range []string{
		"missing_fact: agent.config.system_prompt_verified",
		"missing_fact: file.management.saved_pdf",
	} {
		if !completionGateTestStringSliceContains(decision.MissingFacts, want) {
			t.Fatalf("MissingFacts = %#v, want %q", decision.MissingFacts, want)
		}
	}
}

func TestCompletionGateRequiresManagedPDFSaveNotTemporaryArtifact(t *testing.T) {
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"tool_choice_mode": operationPlanToolChoiceModelDecides,
			"planning_mode":    "phase_only_model_decides",
			"status":           "running",
			"phases": []interface{}{
				map[string]interface{}{"id": "save_pdf_to_file_management", "title": "save pdf to file management"},
			},
		},
		"evidence_ledger": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "completed",
				"skill_id":  skills.SkillFileGenerator,
				"tool_name": "generate_pdf",
				"result_facts": map[string]interface{}{
					"status":         "success",
					"target":         "temporary_artifact",
					"file_extension": "pdf",
				},
			},
		},
	}

	decision := completionGateEvaluate(evidence, "")
	if decision.Path != completionGateNeedsAction {
		t.Fatalf("completionGateEvaluate().Path = %q, want %q; decision=%#v", decision.Path, completionGateNeedsAction, decision)
	}
	if !completionGateTestStringSliceContains(decision.MissingFacts, "missing_fact: file.management.saved_pdf") {
		t.Fatalf("MissingFacts = %#v, want managed PDF missing fact", decision.MissingFacts)
	}

	managedPDFEvidence := map[string]interface{}{
		"kind":      "tool_call",
		"status":    "completed",
		"skill_id":  skills.SkillFileManager,
		"tool_name": "save_file_to_management",
		"result_facts": map[string]interface{}{
			"status":         "completed",
			"target":         "managed_file",
			"file_extension": "pdf",
			"file_id":        "managed-pdf",
			"filename":       "chapter.pdf",
		},
	}
	evidence["evidence_ledger"] = append(evidenceSliceFromAny(evidence["evidence_ledger"]), managedPDFEvidence)
	evidence["operation_result_summary"] = map[string]interface{}{
		"status":             "completed",
		"plan_status":        "completed",
		"latest_tool_status": "success",
		"latest_tool_result": managedPDFEvidence,
	}

	decision = completionGateEvaluate(evidence, "")
	if decision.Path == completionGateNeedsAction {
		t.Fatalf("completionGateEvaluate().Path = %q after managed PDF evidence; decision=%#v", decision.Path, decision)
	}
	if gaps := completionGateContractCoverageGaps(evidence); len(gaps) > 0 {
		t.Fatalf("completionGateContractCoverageGaps() = %#v, want no gaps after managed PDF evidence", gaps)
	}
}

func TestCompletionGateDoesNotTreatRecommendedSystemPromptAsMutationContract(t *testing.T) {
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"tool_choice_mode": operationPlanToolChoiceModelDecides,
			"planning_mode":    "phase_only_model_decides",
			"status":           "running",
			"task_contract": map[string]interface{}{
				"task_type":                "inspect_agent_runtime",
				"recommended_capabilities": []interface{}{"agent.system_prompt"},
			},
		},
		"evidence_ledger": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "completed",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
				"result_facts": map[string]interface{}{
					"status":                "completed",
					"effect":                "agent.config_read",
					"agent_id":              "agent-1",
					"system_prompt_present": true,
				},
			},
		},
	}

	if gaps := completionGateContractCoverageGaps(evidence); len(gaps) > 0 {
		t.Fatalf("completionGateContractCoverageGaps() = %#v, want no mutation gap for read-only recommended capability", gaps)
	}
}

func TestCompletionVerificationPassReasonDropsStaleFailureText(t *testing.T) {
	reason := completionVerificationReconciledPassReason(
		completionVerificationStatusPass,
		"最终答案后校验发现当前回答缺少工具结果支持",
		"latest evidence ledger verifies requested Agent configuration",
	)
	if strings.Contains(reason, "缺少工具结果支持") {
		t.Fatalf("reason = %q, want stale failure text removed", reason)
	}
}

func completionGateVerifiedSystemPromptEvidence(intendedEffect string) map[string]interface{} {
	return map[string]interface{}{
		"user_request": "更新智能体配置并重新读取配置验证",
		"operation_plan": map[string]interface{}{
			"tool_choice_mode":    operationPlanToolChoiceModelDecides,
			"planning_mode":       "phase_only_model_decides",
			"status":              "running",
			"original_user_goal":  "更新智能体 config，然后 read config again",
			"success_criteria":    []interface{}{"update Agent config", "read config again"},
			"pending_next_action": "none",
			"task_contract": map[string]interface{}{
				"intended_effect": intendedEffect,
			},
		},
		"evidence_ledger": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
				"sequence":  2,
				"result_facts": map[string]interface{}{
					"status":               "completed",
					"effect":               "agent.system_prompt_update",
					"agent_id":             "agent-1",
					"agent_name":           "灵澜学院说书人",
					"updated_fields":       []interface{}{"system_prompt"},
					"system_prompt_digest": "sha256:prompt",
				},
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
				"sequence":  3,
				"result_facts": map[string]interface{}{
					"status":               "completed",
					"effect":               "agent.config_read",
					"agent_id":             "agent-1",
					"agent_name":           "灵澜学院说书人",
					"system_prompt_digest": "sha256:prompt",
					"field_status": map[string]interface{}{
						"system_prompt": "verified",
					},
					"verified_fields": []interface{}{"system_prompt"},
				},
			},
		},
	}
}
