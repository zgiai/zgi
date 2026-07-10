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
}

func TestCompletionGateIgnoresKnowledgeBindingContractForSystemPromptEvidence(t *testing.T) {
	evidence := completionGateVerifiedSystemPromptEvidence("agent.knowledge_binding")

	decision := completionGateEvaluate(evidence, "已完成")
	if decision.Path != completionGateDeterministicPass {
		t.Fatalf("completionGateEvaluate().Path = %q, want %q; decision=%#v", decision.Path, completionGateDeterministicPass, decision)
	}
	if completionGateTestStringSliceContains(decision.MissingFacts, "missing_fact: agent.knowledge_binding") {
		t.Fatalf("MissingFacts = %#v, want no knowledge binding missing fact from advisory contract", decision.MissingFacts)
	}
}

func TestCompletionGateAllowsSystemPromptForRuntimeConfigAlternativeContract(t *testing.T) {
	evidence := map[string]interface{}{
		"user_request": "\u5230\u6587\u4ef6\u7ba1\u7406\u7eed\u5199\u7b2c\u4e00\u4e2a md\uff0c\u751f\u6210 PDF \u5e76\u5728\u8fd9\u4e2a\u667a\u80fd\u4f53\u91cc\u66f4\u65b0\u65b0\u7eed\u5199\u7684\u7ae0\u8282",
		"operation_plan": map[string]interface{}{
			"tool_choice_mode":    operationPlanToolChoiceModelDecides,
			"planning_mode":       "phase_only_model_decides",
			"status":              "running",
			"pending_next_action": "continue_from_phase_success_criteria",
			"task_contract": map[string]interface{}{
				"phases": []interface{}{
					"generate a PDF file with the continued content and save to File Management",
					"update the agent by binding the continued chapter content (e.g., as knowledge base document or runtime config)",
				},
				"completion_criteria": []interface{}{
					"PDF file generated and saved to File Management",
					"agent updated with continued chapter content",
				},
				"recommended_capabilities": []interface{}{
					"agent.knowledge_binding:bind",
					"agent.update_agent_runtime_config",
				},
			},
		},
		"evidence_ledger": []interface{}{
			map[string]interface{}{
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
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "completed",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
				"result_facts": map[string]interface{}{
					"status":               "completed",
					"effect":               "agent.system_prompt_update",
					"agent_id":             "agent-1",
					"agent_name":           "\u7075\u6f9c\u5b66\u9662\u8bf4\u4e66\u4eba",
					"updated_fields":       []interface{}{"system_prompt"},
					"satisfied_fields":     []interface{}{"system_prompt"},
					"system_prompt_digest": "sha256:prompt",
					"field_status": map[string]interface{}{
						"system_prompt": "updated",
					},
				},
			},
		},
	}

	if gaps := completionGateContractCoverageGaps(evidence); len(gaps) > 0 {
		t.Fatalf("completionGateContractCoverageGaps() = %#v, want no gaps for accepted runtime config alternative", gaps)
	}
	decision := completionGateEvaluate(evidence, "\u7cfb\u7edf\u63d0\u793a\u8bcd\u5df2\u66f4\u65b0\u3002")
	if decision.Path != completionGateDeterministicPass {
		t.Fatalf("completionGateEvaluate().Path = %q, want %q; decision=%#v", decision.Path, completionGateDeterministicPass, decision)
	}
	if completionGateTestStringSliceContains(decision.MissingFacts, "missing_fact: agent.knowledge_binding") {
		t.Fatalf("MissingFacts = %#v, want no knowledge binding gap", decision.MissingFacts)
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

func TestCompletionGateDoesNotAskUserFromLowConfidenceContractDriftAlone(t *testing.T) {
	evidence := completionGateVerifiedSystemPromptEvidence("agent.knowledge_binding")
	contract := evidenceMapFromAny(evidenceMapFromAny(evidence["operation_plan"])["task_contract"])
	contract["low_confidence"] = true

	decision := completionGateEvaluate(evidence, "已完成")
	if decision.Path != completionGateDeterministicPass {
		t.Fatalf("completionGateEvaluate().Path = %q, want %q; decision=%#v", decision.Path, completionGateDeterministicPass, decision)
	}
}

func TestCompletionGateDoesNotInferMissingWorkFromAdvisoryPlan(t *testing.T) {
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
	if decision.Path != completionGateDeterministicPass {
		t.Fatalf("completionGateEvaluate().Path = %q, want %q; decision=%#v", decision.Path, completionGateDeterministicPass, decision)
	}
	if decision.FinalAnswer != "The PDF file has been generated." {
		t.Fatalf("completionGateEvaluate().FinalAnswer = %q, want main model candidate unchanged", decision.FinalAnswer)
	}
	if len(decision.MissingFacts) != 0 {
		t.Fatalf("MissingFacts = %#v, want no contract-derived missing facts", decision.MissingFacts)
	}
}

func TestCompletionGateAuditsFailedOpenItemInsteadOfTreatingItAsPendingProtocol(t *testing.T) {
	decision := completionGateEvaluate(map[string]interface{}{
		"turn_state": map[string]interface{}{
			"open_items": []interface{}{map[string]interface{}{
				"status": "error",
				"reason": "failed_tool_call_needs_model_decision",
			}},
		},
		"operation_plan": map[string]interface{}{"status": "failed"},
	}, "The deletion failed because the file no longer exists.")

	if decision.Path != completionGateModelVerifier {
		t.Fatalf("completion gate path = %q, want model_verifier", decision.Path)
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
	if decision.Path != completionGateModelVerifier {
		t.Fatalf("completionGateEvaluate().Path = %q, want %q; decision=%#v", decision.Path, completionGateModelVerifier, decision)
	}
	if len(decision.MissingFacts) != 0 {
		t.Fatalf("MissingFacts = %#v, want no contract-derived managed PDF gap", decision.MissingFacts)
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
