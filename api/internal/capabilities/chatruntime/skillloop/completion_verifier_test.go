package skillloop

import (
	"encoding/json"
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestCompletionVerificationOperationPlanForPromptHidesModelDecidesCandidateTools(t *testing.T) {
	plan := map[string]interface{}{
		"tool_choice_mode": "model_decides",
		"capability_goals": []interface{}{
			map[string]interface{}{
				"capability_id":   "agent.skill_backed_capability",
				"candidate_tool":  "list_agent_skill_candidates",
				"candidate_query": "file generation",
			},
		},
		"strategy_state": map[string]interface{}{
			"capability_goals": []interface{}{
				map[string]interface{}{
					"capability_id":  "agent.model_selection",
					"candidate_tool": "list_available_models",
				},
			},
		},
	}

	promptPlan := completionVerificationOperationPlanForPrompt(plan)
	encoded, err := json.Marshal(promptPlan)
	if err != nil {
		t.Fatalf("json.Marshal(promptPlan) failed: %v", err)
	}
	if strings.Contains(string(encoded), "candidate_tool") {
		t.Fatalf("prompt plan leaked candidate_tool: %s", encoded)
	}
	if !strings.Contains(string(encoded), "candidate_query") {
		t.Fatalf("prompt plan lost semantic candidate query: %s", encoded)
	}
}

func TestCompletionVerificationFallbackAnswerUsesReadableChinese(t *testing.T) {
	answer := completionVerificationFallbackAnswer(completionVerificationDecision{
		Status:            completionVerificationStatusFailed,
		Reason:            "update_agent_config failed",
		MissingSteps:      []string{"agent-management/update_agent_config"},
		UnsupportedClaims: []string{"\u5df2\u5b8c\u6210"},
	}, "done")

	for _, fragment := range []string{
		completionVerificationFallbackFailed,
		"update_agent_config failed",
		"\u7f3a\u5c11\u7684\u5b8c\u6210\u8bc1\u636e\uff1a\u667a\u80fd\u4f53\u914d\u7f6e\u66f4\u65b0\u7ed3\u679c\u3002",
		"\u5019\u9009\u7b54\u590d\u4e2d\u6709\u672a\u88ab\u5de5\u5177\u7ed3\u679c\u652f\u6301\u7684\u8bf4\u6cd5\uff1a\u5df2\u5b8c\u6210\u3002",
	} {
		if !strings.Contains(answer, fragment) {
			t.Fatalf("answer = %q, want fragment %q", answer, fragment)
		}
	}
	for _, mojibake := range []string{"\u93b4", "\u6769", "\u7f02", "\u934a"} {
		if strings.Contains(answer, mojibake) {
			t.Fatalf("answer contains mojibake marker %q: %q", mojibake, answer)
		}
	}
}

func TestCompletionVerificationSystemMessageMapsAgentConfigNeedsActionToTools(t *testing.T) {
	message := completionVerificationSystemMessage(completionVerificationDecision{
		Status:       completionVerificationStatusNeedsAction,
		Reason:       "final answer lacks tool evidence",
		MissingSteps: []string{"Run tool:agent-management/update_agent_config", "Run tool:agent-management/get_agent_config (post-update verification)"},
	}, "\u5df2\u5b8c\u6210\u3002", 1)
	content := messageContent(message.Content)

	for _, fragment := range []string{
		"Required next tool: call agent-management/update_agent_config",
		"update only the missing fields",
		"call agent-management/get_agent_config",
		"Do not produce another final answer until the requested Agent configuration update succeeds",
	} {
		if !strings.Contains(content, fragment) {
			t.Fatalf("system message = %q, want fragment %q", content, fragment)
		}
	}
}

func TestCompletionVerificationFallbackAnswerMapsAgentConfigEvidenceLabels(t *testing.T) {
	answer := completionVerificationFallbackAnswer(completionVerificationDecision{
		Status:       completionVerificationStatusFailed,
		Reason:       "final answer lacks tool evidence",
		MissingSteps: []string{"Run tool:agent-management/update_agent_config", "Run tool:agent-management/get_agent_config (post-update verification)"},
	}, "\u5df2\u5b8c\u6210\u3002")

	if strings.Contains(answer, "agent-management/update_agent_config") ||
		strings.Contains(answer, "agent-management/get_agent_config") {
		t.Fatalf("answer leaks internal Agent config tool labels: %q", answer)
	}
	for _, fragment := range []string{
		"\u667a\u80fd\u4f53\u914d\u7f6e\u66f4\u65b0\u7ed3\u679c",
		"\u66f4\u65b0\u540e\u7684\u667a\u80fd\u4f53\u914d\u7f6e\u8bfb\u53d6\u7ed3\u679c",
	} {
		if !strings.Contains(answer, fragment) {
			t.Fatalf("answer = %q, want fragment %q", answer, fragment)
		}
	}
}

func TestCompletionVerificationFeedbackToolChoiceForAgentConfigNeedsAction(t *testing.T) {
	decision := completionVerificationDecision{
		Status:       completionVerificationStatusNeedsAction,
		MissingSteps: []string{"Run tool:agent-management/update_agent_config"},
	}
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{
		{Metadata: skills.SkillMetadata{ID: skills.SkillAgentManagement}},
	}}

	unloadedChoice := completionVerificationFeedbackToolChoice(decision, nil, resolved)
	if got := functionToolChoiceName(unloadedChoice); got != skills.MetaToolLoadSkill {
		t.Fatalf("unloaded tool choice = %q, want %s", got, skills.MetaToolLoadSkill)
	}

	loadedChoice := completionVerificationFeedbackToolChoice(decision, map[string]struct{}{skills.SkillAgentManagement: {}}, resolved)
	if got := functionToolChoiceName(loadedChoice); got != skills.MetaToolCallSkillTool {
		t.Fatalf("loaded tool choice = %q, want %s", got, skills.MetaToolCallSkillTool)
	}
}

func functionToolChoiceName(choice interface{}) string {
	root, ok := choice.(map[string]interface{})
	if !ok {
		return ""
	}
	fn, ok := root["function"].(map[string]interface{})
	if !ok {
		return ""
	}
	name, _ := fn["name"].(string)
	return name
}

func TestCompletionVerificationFallbackAnswerHidesInternalReason(t *testing.T) {
	answer := completionVerificationFallbackAnswer(completionVerificationDecision{
		Status:       completionVerificationStatusFailed,
		Reason:       "Operation plan still has a pending executable step: skill:console-navigator. Per verification contract, cannot pass.",
		MissingSteps: []string{"skill:console-navigator"},
	}, "done")

	if strings.Contains(answer, "Operation plan") ||
		strings.Contains(answer, "verification contract") ||
		strings.Contains(answer, "pending executable") {
		t.Fatalf("answer leaks internal verifier reason: %q", answer)
	}
	if !strings.Contains(answer, completionVerificationFallbackFailed) {
		t.Fatalf("answer = %q, want conservative failure prefix", answer)
	}
	if !strings.Contains(answer, "\u7f3a\u5c11\u7684\u5b8c\u6210\u8bc1\u636e\uff1askill:console-navigator\u3002") {
		t.Fatalf("answer = %q, want missing evidence", answer)
	}
}

func TestCompletionVerificationAlignLanguageDropsEnglishReplacementForChineseUser(t *testing.T) {
	decision := completionVerificationAlignLanguage(map[string]interface{}{
		"user_request": "\u5e2e\u6211\u5220\u9664\u8fd9\u4e2a\u9875\u9762\u7684\u524d\u56db\u4e2a\u667a\u80fd\u4f53",
	}, completionVerificationDecision{
		Status:              completionVerificationStatusFailed,
		Reason:              "candidate answer has unsupported claims",
		FinalAnswer:         "The plan only executed list_agents, so no agents were deleted.",
		FinalAnswerGuidance: "Ask the user to try again.",
	})

	if decision.FinalAnswer != "" {
		t.Fatalf("FinalAnswer = %q, want empty English replacement for Chinese request", decision.FinalAnswer)
	}
	if decision.FinalAnswerGuidance != "" {
		t.Fatalf("FinalAnswerGuidance = %q, want empty English guidance for Chinese request", decision.FinalAnswerGuidance)
	}
	if !strings.Contains(decision.Reason, "\u540e\u6821\u9a8c") {
		t.Fatalf("Reason = %q, want Chinese public reason", decision.Reason)
	}
	if decision.LanguageHint != "zh-Hans" {
		t.Fatalf("LanguageHint = %q, want zh-Hans", decision.LanguageHint)
	}
}

func TestCompletionVerificationDetectsInternalPlanLeak(t *testing.T) {
	answer := "\u867d\u7136\u7cfb\u7edf\u63d0\u793a\u7ee7\u7eed\u6267\u884c update_agent_config\uff0c\u4f46\u6211\u6ca1\u6709\u627e\u5230\u5019\u9009 Skill\u3002"
	if !completionVerificationCandidateAnswerLeaksInternalPlan(answer) {
		t.Fatalf("completionVerificationCandidateAnswerLeaksInternalPlan(%q) = false, want true", answer)
	}

	decision := completionVerificationInternalPlanLeakDecision(map[string]interface{}{
		"user_request": "\u8bf7\u7ed1\u5b9a Skill",
	})
	if got := decision.normalizedStatus(); got != completionVerificationStatusNeedsAction {
		t.Fatalf("decision status = %q, want needs_action", got)
	}
	if !strings.Contains(decision.FinalAnswerGuidance, "\u4e0d\u8981\u63d0\u5230\u7cfb\u7edf\u63d0\u793a") {
		t.Fatalf("FinalAnswerGuidance = %q, want Chinese guidance hiding internal prompts", decision.FinalAnswerGuidance)
	}
}

func TestCompletionVerificationAllowsAgentSystemPromptField(t *testing.T) {
	answer := "\u5f53\u524d\u667a\u80fd\u4f53\u7684\u53ef\u7f16\u8f91\u9879\u5305\u62ec\u540d\u79f0\u3001\u63cf\u8ff0\u3001\u56fe\u6807\u3001\u6a21\u578b\u548c\u7cfb\u7edf\u63d0\u793a\u8bcd\uff1b\u672c\u8f6e\u53ea\u8bfb\u68c0\u67e5\uff0c\u6ca1\u6709\u6267\u884c\u53d8\u66f4\u64cd\u4f5c\u3002"
	if completionVerificationCandidateAnswerLeaksInternalPlan(answer) {
		t.Fatalf("completionVerificationCandidateAnswerLeaksInternalPlan(%q) = true, want false", answer)
	}
}

func TestCompletionVerificationFallbackAnswerHidesInternalUnsupportedClaims(t *testing.T) {
	answer := completionVerificationFallbackAnswer(completionVerificationDecision{
		Status:            completionVerificationStatusNeedsAction,
		Reason:            "candidate answer exposed internal planning or system instruction wording",
		UnsupportedClaims: []string{"internal planning or system instruction wording leaked to the user"},
	}, "")

	for _, leaked := range []string{"internal planning", "system instruction", "candidate answer", "unsupported claim"} {
		if strings.Contains(strings.ToLower(answer), leaked) {
			t.Fatalf("answer leaks internal verifier claim %q: %q", leaked, answer)
		}
	}
	if !strings.Contains(answer, completionVerificationFallbackUnknown) {
		t.Fatalf("answer = %q, want neutral fallback", answer)
	}
}

func TestCompletionVerificationSystemMessageKeepsChineseRetryLanguage(t *testing.T) {
	message := completionVerificationSystemMessage(completionVerificationDecision{
		Status:       completionVerificationStatusNeedsAction,
		Reason:       "\u9700\u8981\u7ee7\u7eed\u786e\u8ba4\u5de5\u5177\u7ed3\u679c",
		LanguageHint: "zh-Hans",
	}, "\u6211\u5df2\u5b8c\u6210\u64cd\u4f5c\u3002", 1)
	content := messageContent(message.Content)

	if !strings.Contains(content, "Continue in Chinese") {
		t.Fatalf("system message = %q, want explicit Chinese retry language", content)
	}
	if strings.Contains(content, "do not answer in Chinese") {
		t.Fatalf("system message = %q, contains contradictory language guidance", content)
	}
}

func TestCompletionVerificationSystemMessageMapsResolvedFileDeleteToToolAction(t *testing.T) {
	message := completionVerificationSystemMessage(completionVerificationDecision{
		Status:       completionVerificationStatusNeedsAction,
		Reason:       "missing delete evidence",
		MissingSteps: []string{"Delete resolved file"},
	}, "已删除。", 1)
	content := messageContent(message.Content)

	for _, fragment := range []string{
		"Required next tool: call file-manager/delete_file",
		"resolved file_id",
		"Tool governance owns the approval card",
		"Do not produce another final answer until file-manager/delete_file succeeds",
	} {
		if !strings.Contains(content, fragment) {
			t.Fatalf("system message = %q, want fragment %q", content, fragment)
		}
	}
}

func TestCompletionVerificationFallbackAnswerMapsResolvedFileDeleteLabel(t *testing.T) {
	answer := completionVerificationFallbackAnswer(completionVerificationDecision{
		Status:       completionVerificationStatusFailed,
		Reason:       "final answer lacks tool evidence",
		MissingSteps: []string{"Delete resolved file"},
	}, "已删除。")

	if strings.Contains(answer, "Delete resolved file") {
		t.Fatalf("answer leaks internal step label: %q", answer)
	}
	if !strings.Contains(answer, "\u6587\u4ef6\u5220\u9664\u7ed3\u679c") {
		t.Fatalf("answer = %q, want public missing delete evidence label", answer)
	}
}

func TestCompletionVerificationContractTreatsPlanAsAdvisory(t *testing.T) {
	contract := completionVerificationContract()
	rawRules, ok := contract["rules"].([]string)
	if !ok {
		t.Fatalf("rules = %#v, want []string", contract["rules"])
	}
	rules := strings.Join(rawRules, "\n")
	if !strings.Contains(rules, "advisory strategy snapshots") {
		t.Fatalf("rules = %q, want operation plan advisory language", rules)
	}
	if !strings.Contains(rules, "execution_ledger") ||
		!strings.Contains(rules, "authoritative facts") {
		t.Fatalf("rules = %q, want ledger as authoritative fact source", rules)
	}
	if !strings.Contains(rules, "page_context") ||
		!strings.Contains(rules, "target_route_already_available") {
		t.Fatalf("rules = %q, want current page route evidence language", rules)
	}
	if !strings.Contains(rules, "operation as skipped, unnecessary, or not executed") {
		t.Fatalf("rules = %q, want mutation evidence wording guidance", rules)
	}
	if strings.Contains(rules, "operation_plan still has a pending executable tool step") {
		t.Fatalf("rules = %q, must not force pending plan steps as hard verifier failures", rules)
	}
}

func TestCompletionVerificationPromptTreatsEvidenceAsAuthoritative(t *testing.T) {
	request := completionVerificationRequest(&adapter.ChatRequest{}, `{"candidate_answer":"done"}`, false)
	if len(request.Messages) == 0 {
		t.Fatal("completionVerificationRequest returned no messages")
	}
	system := messageContent(request.Messages[0].Content)
	for _, fragment := range []string{
		"faithful to the provided evidence",
		"operation_plan and turn_strategy as advisory strategy snapshots only",
		"must not override successful or failed execution evidence",
		"Current page context, tool results, ledger evidence, client actions, and governance outcomes are authoritative",
		"target_route_already_available",
	} {
		if !strings.Contains(system, fragment) {
			t.Fatalf("system message = %q, want fragment %q", system, fragment)
		}
	}
	if strings.Contains(system, "faithful to the provided plan") {
		t.Fatalf("system message = %q, must not make plan the primary verifier authority", system)
	}
}

func TestCompletionVerificationApplyPlanOverrideRejectsPassForFailedPlan(t *testing.T) {
	decision := completionVerificationApplyPlanOverride(map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "failed",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:file-manager/save_file_to_management",
					"status":    "failed",
					"skill_id":  "file-manager",
					"tool_name": "save_file_to_management",
				},
			},
		},
		"execution_ledger": map[string]interface{}{
			"skill_invocations": []map[string]interface{}{
				{
					"kind":      "tool_call",
					"status":    "error",
					"skill_id":  "file-manager",
					"tool_name": "save_file_to_management",
					"result": map[string]interface{}{
						"status":     "error",
						"error":      "workspace permission denied",
						"error_code": "permission_denied",
					},
				},
			},
		},
	}, completionVerificationDecision{Status: completionVerificationStatusPass, Reason: "looks good"})

	if got := decision.normalizedStatus(); got != completionVerificationStatusFailed {
		t.Fatalf("decision status = %q, want failed; decision=%#v", got, decision)
	}
	if !strings.Contains(decision.Reason, "file-manager/save_file_to_management") {
		t.Fatalf("decision reason = %q, want failed plan step label", decision.Reason)
	}
	if len(decision.UnsupportedClaims) == 0 {
		t.Fatalf("unsupported claims = %#v, want pass rejection claim", decision.UnsupportedClaims)
	}
	if !strings.Contains(decision.FinalAnswer, completionVerificationFallbackFailed) ||
		!strings.Contains(decision.FinalAnswer, "file-manager/save_file_to_management") {
		t.Fatalf("final_answer = %q, want conservative failed-plan answer", decision.FinalAnswer)
	}
	if !strings.Contains(decision.FinalAnswer, "workspace permission denied") {
		t.Fatalf("final_answer = %q, want ledger failure detail", decision.FinalAnswer)
	}
}

func TestCompletionVerificationApplyPlanOverrideKeepsPassForPlanOnlyFailure(t *testing.T) {
	decision := completionVerificationApplyPlanOverride(map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "failed",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_config",
					"status":    "failed",
					"skill_id":  "agent-management",
					"tool_name": "update_agent_config",
				},
			},
		},
		"execution_ledger": map[string]interface{}{
			"skill_invocations": []interface{}{
				map[string]interface{}{
					"kind":      "tool_call",
					"status":    "success",
					"skill_id":  "agent-management",
					"tool_name": "update_agent_config",
					"result": map[string]interface{}{
						"status": "completed",
					},
				},
			},
		},
	}, completionVerificationDecision{Status: completionVerificationStatusPass, Reason: "candidate answer is supported by tool evidence"})

	if got := decision.normalizedStatus(); got != completionVerificationStatusPass {
		t.Fatalf("decision status = %q, want pass for stale plan-only failure; decision=%#v", got, decision)
	}
	if decision.FinalAnswer != "" {
		t.Fatalf("FinalAnswer = %q, want no forced failed-plan answer", decision.FinalAnswer)
	}
	if len(decision.UnsupportedClaims) != 0 {
		t.Fatalf("UnsupportedClaims = %#v, want no forced failed-plan claim", decision.UnsupportedClaims)
	}
}

func TestCompletionVerificationApplyPlanOverrideRequiresRequestedPostUpdateConfigRead(t *testing.T) {
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":             "running",
			"original_user_goal": "Bind Chart Generator, then read config again after completion and verify the binding state.",
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
			"tool_result": map[string]interface{}{
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
				"result_summary": map[string]interface{}{
					"status":     "completed",
					"agent_name": "Support Agent",
					"updated_fields": []interface{}{
						"enabled_skill_ids",
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
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
			},
		},
	}

	decision := completionVerificationApplyPlanOverride(evidence, completionVerificationDecision{
		Status: completionVerificationStatusPass,
		Reason: "candidate answer claims completion",
	})

	if got := decision.normalizedStatus(); got != completionVerificationStatusNeedsAction {
		t.Fatalf("decision status = %q, want needs_action; decision=%#v", got, decision)
	}
	if !strings.Contains(strings.Join(decision.MissingSteps, ","), "get_agent_config") {
		t.Fatalf("MissingSteps = %#v, want post-update get_agent_config", decision.MissingSteps)
	}
	if !strings.Contains(decision.FinalAnswerGuidance, "get_agent_config") {
		t.Fatalf("FinalAnswerGuidance = %q, want get_agent_config guidance", decision.FinalAnswerGuidance)
	}
}

func TestCompletionVerificationApplyPlanOverrideUsesLatestPostUpdateAgentConfigRead(t *testing.T) {
	preRead := map[string]interface{}{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "get_agent_config",
		"result": map[string]interface{}{
			"status":              "completed",
			"agent_id":            "agent-1",
			"model_provider":      "openai",
			"model":               "gpt-4o",
			"file_upload_enabled": false,
		},
	}
	update := map[string]interface{}{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "update_agent_config",
		"arguments": map[string]interface{}{
			"expected_updated_fields": []interface{}{"model", "system_prompt", "file_upload_enabled", "enabled_skill_ids"},
			"expected_binding_actions": map[string]interface{}{
				"enabled_skill_ids": "bind",
			},
			"model_provider":        "deepseek",
			"model":                 "deepseek-v4-flash",
			"system_prompt":         "You are a professional fiction writing assistant. Your core capabilities include story planning�",
			"file_upload_enabled":   true,
			"add_enabled_skill_ids": "[\"file-generator\"]",
		},
		"result": map[string]interface{}{
			"status":         "completed",
			"agent_id":       "agent-1",
			"updated_fields": []interface{}{"model", "system_prompt", "file_upload_enabled", "enabled_skill_ids"},
		},
	}
	postRead := map[string]interface{}{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "get_agent_config",
		"result": map[string]interface{}{
			"status":              "completed",
			"agent_id":            "agent-1",
			"model_provider":      "deepseek",
			"model":               "deepseek-v4-flash",
			"system_prompt":       "You are a professional fiction writing assistant. Your core capabilities include story planning, file generation, and upload-aware drafting.",
			"file_upload_enabled": true,
			"enabled_skill_ids":   []string{"file-generator"},
		},
	}
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "completed",
			"steps": []interface{}{
				map[string]interface{}{
					"id":                       "tool:agent-management/update_agent_config",
					"status":                   "completed",
					"skill_id":                 skills.SkillAgentManagement,
					"tool_name":                "update_agent_config",
					"expected_updated_fields":  []interface{}{"model", "system_prompt", "file_upload_enabled", "enabled_skill_ids"},
					"expected_binding_actions": map[string]interface{}{"enabled_skill_ids": "bind"},
					"arguments": map[string]interface{}{
						"model_provider":        "deepseek",
						"model":                 "deepseek-v4-flash",
						"system_prompt":         "You are a professional fiction writing assistant. Your core capabilities include story planning�",
						"file_upload_enabled":   true,
						"add_enabled_skill_ids": "[\"file-generator\"]",
					},
				},
			},
		},
		"skill_invocations": []interface{}{preRead, update, postRead},
		"execution_summary": map[string]interface{}{
			"tool_results": []interface{}{postRead, update, preRead},
		},
	}

	decision := completionVerificationApplyPlanOverride(evidence, completionVerificationDecision{
		Status: completionVerificationStatusPass,
		Reason: "candidate answer claims completion",
	})

	if got := decision.normalizedStatus(); got != completionVerificationStatusPass {
		t.Fatalf("decision status = %q, want pass using latest post-update read; decision=%#v", got, decision)
	}
	if mismatches := completionVerificationAgentConfigMismatches(evidence); len(mismatches) > 0 {
		t.Fatalf("completionVerificationAgentConfigMismatches() = %#v, want none", mismatches)
	}

	reconciled := ReconcileCompletionVerificationResultWithEvidence(evidence, CompletionVerificationResult{
		Status:         completionVerificationStatusNeedsAction,
		Reason:         "requested Agent config state was not verified",
		MissingSteps:   []string{"agent-management/get_agent_config enabled_skill_ids missing bound target: file-generator"},
		NextActionHint: "agent-management/update_agent_config",
	})
	if got := reconciled.Status; got != completionVerificationStatusPass {
		t.Fatalf("reconciled status = %q, want pass; result=%#v", got, reconciled)
	}
	if len(reconciled.MissingSteps) != 0 || reconciled.NextActionHint != "" {
		t.Fatalf("reconciled = %#v, want cleared missing steps and next action", reconciled)
	}
}

func TestCompletionVerificationAgentConfigBindingUsesCandidateAliases(t *testing.T) {
	update := map[string]interface{}{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "update_agent_config",
		"arguments": map[string]interface{}{
			"expected_updated_fields": []interface{}{"model_provider", "model", "system_prompt", "file_upload_enabled", "enabled_skill_ids"},
			"expected_binding_actions": map[string]interface{}{
				"enabled_skill_ids": "bind",
			},
			"model_provider":        "deepseek",
			"model":                 "deepseek-v4-flash",
			"system_prompt":         "Write fiction and generate files when needed.",
			"file_upload_enabled":   true,
			"add_enabled_skill_ids": []interface{}{"文件生成器"},
		},
		"result": map[string]interface{}{
			"status":              "completed",
			"agent_id":            "agent-1",
			"updated_fields":      []interface{}{"model_provider", "model", "system_prompt", "file_upload_enabled", "enabled_skill_ids"},
			"satisfied_fields":    []interface{}{"model_provider", "model", "system_prompt", "file_upload_enabled", "enabled_skill_ids"},
			"enabled_skill_ids":   []string{"file-generator"},
			"file_upload_enabled": true,
		},
	}
	postRead := map[string]interface{}{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "get_agent_config",
		"result": map[string]interface{}{
			"status":              "completed",
			"agent_id":            "agent-1",
			"model_provider":      "deepseek",
			"model":               "deepseek-v4-flash",
			"system_prompt":       "Write fiction and generate files when needed.",
			"file_upload_enabled": true,
			"enabled_skill_ids":   []interface{}{"file-generator"},
		},
	}
	candidateLookup := map[string]interface{}{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "list_agent_skill_candidates",
		"result": map[string]interface{}{
			"status": "completed",
			"candidate_samples": []interface{}{
				map[string]interface{}{
					"id":       "file-generator",
					"name":     "文件生成器",
					"selected": true,
				},
			},
		},
	}
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":             "completed",
			"original_user_goal": "模型配置为 deepseek flash，提示词要能生成文件和上传文件，完成后重新读取配置确认。",
			"steps": []interface{}{
				map[string]interface{}{
					"id":                       "tool:agent-management/update_agent_config",
					"status":                   "completed",
					"skill_id":                 skills.SkillAgentManagement,
					"tool_name":                "update_agent_config",
					"expected_updated_fields":  []interface{}{"model_provider", "model", "system_prompt", "file_upload_enabled", "enabled_skill_ids"},
					"expected_binding_actions": map[string]interface{}{"enabled_skill_ids": "bind"},
					"arguments": map[string]interface{}{
						"model_provider":        "deepseek",
						"model":                 "deepseek-v4-flash",
						"system_prompt":         "Write fiction and generate files when needed.",
						"file_upload_enabled":   true,
						"add_enabled_skill_ids": []interface{}{"文件生成器"},
					},
				},
				map[string]interface{}{
					"id":                                "tool:agent-management/get_agent_config#post_update",
					"status":                            "completed",
					"skill_id":                          skills.SkillAgentManagement,
					"tool_name":                         "get_agent_config",
					"required_post_update_verification": true,
				},
			},
		},
		"skill_invocations": []interface{}{candidateLookup, update, postRead},
		"completion_verification": map[string]interface{}{
			"status":           completionVerificationStatusNeedsAction,
			"reason":           "requested Agent config state was not verified",
			"missing_steps":    []interface{}{"agent-management/get_agent_config enabled_skill_ids missing bound target: 文件生成器"},
			"next_action_hint": "agent-management/update_agent_config",
		},
	}

	if mismatches := completionVerificationAgentConfigMismatches(evidence); len(mismatches) > 0 {
		t.Fatalf("completionVerificationAgentConfigMismatches() = %#v, want none", mismatches)
	}
	answer, ok := FastPathFinalAnswerForCompletionEvidence(evidence)
	if !ok {
		t.Fatal("FastPathFinalAnswerForCompletionEvidence() ok = false, want verified fast-path answer")
	}
	if strings.Contains(answer, "不能确认") {
		t.Fatalf("answer = %q, want no unsupported uncertainty", answer)
	}
	reconciled := ReconcileCompletionVerificationResultWithEvidence(evidence, CompletionVerificationResult{
		Status:         completionVerificationStatusNeedsAction,
		Reason:         "requested Agent config state was not verified",
		MissingSteps:   []string{"agent-management/get_agent_config enabled_skill_ids missing bound target: 文件生成器"},
		NextActionHint: "agent-management/update_agent_config",
	})
	if got := reconciled.Status; got != completionVerificationStatusPass {
		t.Fatalf("reconciled status = %q, want pass; result=%#v", got, reconciled)
	}
}

func TestCompletionVerificationUsesUpdateConfigResultAsPostUpdateEvidence(t *testing.T) {
	update := map[string]interface{}{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "update_agent_config",
		"arguments": map[string]interface{}{
			"expected_updated_fields": []interface{}{"model_provider", "model", "system_prompt", "file_upload_enabled", "enabled_skill_ids"},
			"expected_binding_actions": map[string]interface{}{
				"enabled_skill_ids": "bind",
			},
			"model_provider":        "deepseek",
			"model":                 "deepseek-v4-flash",
			"system_prompt":         "Write fiction and generate files when needed.",
			"file_upload_enabled":   true,
			"add_enabled_skill_ids": []interface{}{"File Generator"},
		},
		"result": map[string]interface{}{
			"status":              "completed",
			"agent_id":            "agent-1",
			"agent_name":          "Novel Writer",
			"model_provider":      "deepseek",
			"model":               "deepseek-v4-flash",
			"system_prompt":       "Write fiction and generate files when needed.",
			"file_upload_enabled": true,
			"enabled_skill_ids":   []interface{}{"file-generator"},
			"updated_fields":      []interface{}{"model_provider", "model", "system_prompt", "file_upload_enabled", "enabled_skill_ids"},
			"satisfied_fields":    []interface{}{"model_provider", "model", "system_prompt", "file_upload_enabled", "enabled_skill_ids"},
		},
	}
	candidateLookup := map[string]interface{}{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "list_agent_skill_candidates",
		"result": map[string]interface{}{
			"status": "completed",
			"candidate_samples": []interface{}{
				map[string]interface{}{
					"id":       "file-generator",
					"name":     "File Generator",
					"selected": true,
				},
			},
		},
	}
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":             "running",
			"original_user_goal": "Create a Novel Writer agent, configure deepseek flash, enable file generation and file upload, then verify the result.",
			"steps": []interface{}{
				map[string]interface{}{
					"id":                       "tool:agent-management/update_agent_config",
					"status":                   "completed",
					"skill_id":                 skills.SkillAgentManagement,
					"tool_name":                "update_agent_config",
					"expected_updated_fields":  []interface{}{"model_provider", "model", "system_prompt", "file_upload_enabled", "enabled_skill_ids"},
					"expected_binding_actions": map[string]interface{}{"enabled_skill_ids": "bind"},
					"arguments": map[string]interface{}{
						"model_provider":        "deepseek",
						"model":                 "deepseek-v4-flash",
						"system_prompt":         "Write fiction and generate files when needed.",
						"file_upload_enabled":   true,
						"add_enabled_skill_ids": []interface{}{"File Generator"},
					},
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
		"skill_invocations": []interface{}{candidateLookup, update},
	}

	if fastPathCompletionEvidenceNeedsAgentConfigPostRead(evidence) {
		t.Fatal("fastPathCompletionEvidenceNeedsAgentConfigPostRead() = true, want false when update_agent_config returned verified config evidence")
	}
	if mismatches := completionVerificationAgentConfigMismatches(evidence); len(mismatches) > 0 {
		t.Fatalf("completionVerificationAgentConfigMismatches() = %#v, want none", mismatches)
	}
	decision := completionVerificationApplyPlanOverride(evidence, completionVerificationDecision{
		Status: completionVerificationStatusPass,
		Reason: "candidate answer claims the requested Agent config is complete",
	})
	if got := decision.normalizedStatus(); got != completionVerificationStatusPass {
		t.Fatalf("decision status = %q, want pass; decision=%#v", got, decision)
	}
}

func TestCompletionVerificationAgentConfigBindingUsesStringifiedMutationEvidence(t *testing.T) {
	update := map[string]interface{}{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "update_agent_config",
		"arguments": map[string]interface{}{
			"model_provider":        "deepseek",
			"model":                 "deepseek-v4-flash",
			"system_prompt":         "Write fiction and generate files when needed.",
			"file_upload_enabled":   true,
			"add_enabled_skill_ids": "[\"file-generator\"]",
		},
		"result": map[string]interface{}{
			"status":              "completed",
			"agent_id":            "agent-1",
			"agent_name":          "Novel Writer",
			"model_provider":      "deepseek",
			"model":               "deepseek-v4-flash",
			"system_prompt":       "Write fiction and generate files when needed.",
			"file_upload_enabled": true,
			"enabled_skill_ids":   []interface{}{"file-generator"},
			"satisfied_fields":    []interface{}{"model_provider", "model", "system_prompt", "file_upload_enabled", "enabled_skill_ids"},
		},
	}
	postRead := map[string]interface{}{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "get_agent_config",
		"arguments": map[string]interface{}{
			"agent_id": map[string]interface{}{"type": "string", "length": 36},
		},
		"result": map[string]interface{}{
			"status":              "completed",
			"agent_id":            "agent-1",
			"agent_name":          "Novel Writer",
			"model_provider":      "deepseek",
			"model":               "deepseek-v4-flash",
			"system_prompt":       "Write fiction and generate files when needed.",
			"file_upload_enabled": true,
			"enabled_skill_ids":   []interface{}{"file-generator"},
		},
	}
	candidateLookup := map[string]interface{}{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "list_agent_skill_candidates",
		"arguments": map[string]interface{}{
			"query":    map[string]interface{}{"type": "string", "length": 15},
			"agent_id": map[string]interface{}{"type": "string", "length": 36},
		},
		"result": map[string]interface{}{
			"status": "completed",
			"candidate_samples": []interface{}{
				map[string]interface{}{"id": "content-summary", "name": "Content Summary"},
				map[string]interface{}{"id": "file-generator", "name": "File Generator", "selected": true},
			},
		},
	}
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":              "completed",
			"pending_next_action": "none",
			"step_status": map[string]interface{}{
				"tool:agent-management/update_agent_config":          "completed",
				"tool:agent-management/get_agent_config#post_update": "completed",
			},
			"steps": []interface{}{
				map[string]interface{}{
					"id":                       "tool:agent-management/update_agent_config",
					"status":                   "completed",
					"skill_id":                 skills.SkillAgentManagement,
					"tool_name":                "update_agent_config",
					"expected_updated_fields":  []interface{}{"model_provider", "model", "system_prompt", "file_upload_enabled", "enabled_skill_ids"},
					"expected_binding_actions": "enabled_skill_ids:bind",
					"arguments": map[string]interface{}{
						"config_goal":              "create a writing agent with DeepSeek Flash, file upload, and file generation",
						"expected_updated_fields":  "model_provider,model,system_prompt,file_upload_enabled,enabled_skill_ids",
						"expected_binding_actions": "enabled_skill_ids:bind",
					},
				},
				map[string]interface{}{
					"id":                                "tool:agent-management/get_agent_config#post_update",
					"status":                            "completed",
					"skill_id":                          skills.SkillAgentManagement,
					"tool_name":                         "get_agent_config",
					"required_post_update_verification": true,
				},
			},
		},
		"skill_invocations": []interface{}{update, postRead, candidateLookup},
	}

	if mismatches := completionVerificationAgentConfigMismatches(evidence); len(mismatches) > 0 {
		t.Fatalf("completionVerificationAgentConfigMismatches() = %#v, want none", mismatches)
	}
	decision := completionVerificationApplyPlanOverride(evidence, completionVerificationDecision{
		Status: completionVerificationStatusPass,
	})
	if got := decision.normalizedStatus(); got != completionVerificationStatusPass {
		t.Fatalf("completionVerificationApplyPlanOverride status = %q, want pass; decision=%#v", got, decision)
	}
	if _, ok := FastPathFinalAnswerForCompletionEvidence(evidence); !ok {
		t.Fatal("FastPathFinalAnswerForCompletionEvidence() ok = false, want verified final answer")
	}
}

func TestCompletionVerificationApplyPlanOverrideRequiresRemainingAgentConfigFields(t *testing.T) {
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "running",
			"steps": []interface{}{
				map[string]interface{}{
					"id":                     "tool:agent-management/update_agent_config",
					"status":                 "pending",
					"skill_id":               skills.SkillAgentManagement,
					"tool_name":              "update_agent_config",
					"missing_updated_fields": []interface{}{"system_prompt", "home_title", "suggested_questions"},
				},
			},
			"step_status": map[string]interface{}{
				"tool:agent-management/update_agent_config": "pending",
			},
		},
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
				"result": map[string]interface{}{
					"status":         "completed",
					"updated_fields": []interface{}{"model"},
				},
			},
		},
	}

	decision := completionVerificationApplyPlanOverride(evidence, completionVerificationDecision{
		Status: completionVerificationStatusPass,
		Reason: "candidate answer claims completion",
	})

	if got := decision.normalizedStatus(); got != completionVerificationStatusNeedsAction {
		t.Fatalf("decision status = %q, want needs_action; decision=%#v", got, decision)
	}
	joinedMissing := strings.Join(decision.MissingSteps, ",")
	for _, want := range []string{"agent-management/update_agent_config", "system_prompt", "home_title", "suggested_questions"} {
		if !strings.Contains(joinedMissing, want) {
			t.Fatalf("MissingSteps = %#v, want fragment %q", decision.MissingSteps, want)
		}
	}
	if decision.NextActionHint != "agent-management/update_agent_config" {
		t.Fatalf("NextActionHint = %q, want agent-management/update_agent_config", decision.NextActionHint)
	}
	if decision.FinalAnswer != "" {
		t.Fatalf("FinalAnswer = %q, want empty needs-action answer", decision.FinalAnswer)
	}
	for _, want := range []string{"update_agent_config", "system_prompt", "home_title", "suggested_questions"} {
		if !strings.Contains(decision.FinalAnswerGuidance, want) {
			t.Fatalf("FinalAnswerGuidance = %q, want fragment %q", decision.FinalAnswerGuidance, want)
		}
	}
}

func TestCompletionVerificationApplyPlanOverrideRequiresBoundEnabledSkillInPostRead(t *testing.T) {
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "running",
			"steps": []interface{}{
				map[string]interface{}{
					"id":                       "tool:agent-management/update_agent_config",
					"status":                   "completed",
					"skill_id":                 skills.SkillAgentManagement,
					"tool_name":                "update_agent_config",
					"expected_updated_fields":  []interface{}{"enabled_skill_ids"},
					"expected_binding_actions": map[string]interface{}{"enabled_skill_ids": "bind"},
					"arguments": map[string]interface{}{
						"candidate_skill_id": "file-generator",
					},
				},
				map[string]interface{}{
					"id":                                "tool:agent-management/get_agent_config#post_update",
					"status":                            "completed",
					"skill_id":                          skills.SkillAgentManagement,
					"tool_name":                         "get_agent_config",
					"phase":                             "post_update_verification",
					"required_post_update_verification": true,
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
					"add_enabled_skill_ids": []interface{}{"file-generator"},
				},
				"result": map[string]interface{}{
					"status":         "completed",
					"updated_fields": []interface{}{"enabled_skill_ids"},
				},
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
				"result": map[string]interface{}{
					"status": "completed",
					"config": map[string]interface{}{
						"enabled_skill_ids": []interface{}{"chart-generator"},
					},
				},
			},
		},
	}

	decision := completionVerificationApplyPlanOverride(evidence, completionVerificationDecision{
		Status: completionVerificationStatusPass,
		Reason: "candidate answer claims completion",
	})

	if got := decision.normalizedStatus(); got != completionVerificationStatusNeedsAction {
		t.Fatalf("decision status = %q, want needs_action; decision=%#v", got, decision)
	}
	joinedMissing := strings.Join(decision.MissingSteps, ",")
	for _, want := range []string{"enabled_skill_ids", "file-generator"} {
		if !strings.Contains(joinedMissing, want) {
			t.Fatalf("MissingSteps = %#v, want fragment %q", decision.MissingSteps, want)
		}
	}
	if decision.NextActionHint != "agent-management/update_agent_config" {
		t.Fatalf("NextActionHint = %q, want agent-management/update_agent_config", decision.NextActionHint)
	}
	if !strings.Contains(decision.FinalAnswerGuidance, "file-generator") {
		t.Fatalf("FinalAnswerGuidance = %q, want missing skill guidance", decision.FinalAnswerGuidance)
	}
}

func TestCompletionVerificationApplyPlanOverridePassesBoundEnabledSkillInPostRead(t *testing.T) {
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "running",
			"steps": []interface{}{
				map[string]interface{}{
					"id":                       "tool:agent-management/update_agent_config",
					"status":                   "completed",
					"skill_id":                 skills.SkillAgentManagement,
					"tool_name":                "update_agent_config",
					"expected_updated_fields":  []interface{}{"enabled_skill_ids"},
					"expected_binding_actions": map[string]interface{}{"enabled_skill_ids": "bind"},
					"arguments": map[string]interface{}{
						"candidate_skill_id": "file-generator",
					},
				},
				map[string]interface{}{
					"id":                                "tool:agent-management/get_agent_config#post_update",
					"status":                            "completed",
					"skill_id":                          skills.SkillAgentManagement,
					"tool_name":                         "get_agent_config",
					"phase":                             "post_update_verification",
					"required_post_update_verification": true,
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
					"add_enabled_skill_ids": []interface{}{"file-generator"},
				},
				"result": map[string]interface{}{
					"status":         "completed",
					"updated_fields": []interface{}{"enabled_skill_ids"},
				},
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
				"result": map[string]interface{}{
					"status": "completed",
					"config": map[string]interface{}{
						"enabled_skill_ids": []interface{}{"chart-generator", "file-generator"},
					},
				},
			},
		},
	}

	decision := completionVerificationApplyPlanOverride(evidence, completionVerificationDecision{
		Status: completionVerificationStatusPass,
		Reason: "candidate answer is supported by post-update config",
	})

	if got := decision.normalizedStatus(); got != completionVerificationStatusPass {
		t.Fatalf("decision status = %q, want pass; decision=%#v", got, decision)
	}
	if len(decision.MissingSteps) != 0 {
		t.Fatalf("MissingSteps = %#v, want none", decision.MissingSteps)
	}
}

func TestCompletionVerificationApplyPlanOverridePassesBoundEnabledSkillFromInvocationArgumentsAfterStaleRead(t *testing.T) {
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "running",
			"steps": []interface{}{
				map[string]interface{}{
					"id":                       "tool:agent-management/update_agent_config",
					"status":                   "completed",
					"skill_id":                 skills.SkillAgentManagement,
					"tool_name":                "update_agent_config",
					"expected_updated_fields":  []interface{}{"enabled_skill_ids"},
					"expected_binding_actions": map[string]interface{}{"enabled_skill_ids": "bind"},
					"arguments": map[string]interface{}{
						"expected_binding_actions": "enabled_skill_ids:bind",
					},
				},
				map[string]interface{}{
					"id":                                "tool:agent-management/get_agent_config#post_update",
					"status":                            "completed",
					"skill_id":                          skills.SkillAgentManagement,
					"tool_name":                         "get_agent_config",
					"phase":                             "post_update_verification",
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
				"result": map[string]interface{}{
					"status":              "completed",
					"enabled_skill_ids":   []interface{}{},
					"file_upload_enabled": false,
				},
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
				"arguments": map[string]interface{}{
					"add_enabled_skill_ids": "[\"file-generator\"]",
				},
				"result": map[string]interface{}{
					"status":            "completed",
					"updated_fields":    []interface{}{"enabled_skill_ids"},
					"enabled_skill_ids": []interface{}{"file-generator"},
				},
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
				"result": map[string]interface{}{
					"status":            "completed",
					"enabled_skill_ids": []interface{}{"file-generator"},
				},
			},
		},
	}

	decision := completionVerificationApplyPlanOverride(evidence, completionVerificationDecision{
		Status: completionVerificationStatusPass,
		Reason: "candidate answer is supported by post-update config",
	})

	if got := decision.normalizedStatus(); got != completionVerificationStatusPass {
		t.Fatalf("decision status = %q, want pass; decision=%#v", got, decision)
	}
	if len(decision.MissingSteps) != 0 {
		t.Fatalf("MissingSteps = %#v, want none", decision.MissingSteps)
	}
}

func TestCompletionVerificationApplyPlanOverrideRequiresModelPairInPostRead(t *testing.T) {
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "running",
			"steps": []interface{}{
				map[string]interface{}{
					"id":                      "tool:agent-management/update_agent_config",
					"status":                  "completed",
					"skill_id":                skills.SkillAgentManagement,
					"tool_name":               "update_agent_config",
					"expected_updated_fields": []interface{}{"model_provider", "model"},
					"arguments": map[string]interface{}{
						"model_provider": "openai",
						"model":          "gpt-4o",
					},
				},
				map[string]interface{}{
					"id":                                "tool:agent-management/get_agent_config#post_update",
					"status":                            "completed",
					"skill_id":                          skills.SkillAgentManagement,
					"tool_name":                         "get_agent_config",
					"phase":                             "post_update_verification",
					"required_post_update_verification": true,
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
					"model_provider": "openai",
					"model":          "gpt-4o",
				},
				"result": map[string]interface{}{"status": "completed", "updated_fields": []interface{}{"model_provider", "model"}},
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
				"result": map[string]interface{}{
					"status": "completed",
					"config": map[string]interface{}{
						"model_provider": "deepseek",
						"model":          "deepseek-chat",
					},
				},
			},
		},
	}

	decision := completionVerificationApplyPlanOverride(evidence, completionVerificationDecision{
		Status: completionVerificationStatusPass,
		Reason: "candidate answer claims the model was replaced",
	})

	if got := decision.normalizedStatus(); got != completionVerificationStatusNeedsAction {
		t.Fatalf("decision status = %q, want needs_action; decision=%#v", got, decision)
	}
	joinedMissing := strings.Join(decision.MissingSteps, ",")
	for _, want := range []string{"model_provider", "openai", "model", "gpt-4o"} {
		if !strings.Contains(joinedMissing, want) {
			t.Fatalf("MissingSteps = %#v, want fragment %q", decision.MissingSteps, want)
		}
	}
}

func TestCompletionVerificationApplyPlanOverrideUsesConfigGoalModelHint(t *testing.T) {
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":             "completed",
			"original_user_goal": "创建智能体，模型配置为deepseek flash，写好提示词需要让agent能生成文件和上传文件。",
			"steps": []interface{}{
				map[string]interface{}{
					"id":                      "tool:agent-management/update_agent_config",
					"status":                  "completed",
					"skill_id":                skills.SkillAgentManagement,
					"tool_name":               "update_agent_config",
					"expected_updated_fields": []interface{}{"model_provider", "model", "system_prompt", "file_upload_enabled", "enabled_skill_ids"},
					"arguments": map[string]interface{}{
						"model_provider": "deepseek",
						"model":          "deepseek-chat",
						"config_goal":    "模型配置为deepseek flash，写好提示词需要让agent能生成文件和上传文件。",
					},
				},
				map[string]interface{}{
					"id":                                "tool:agent-management/get_agent_config#post_update",
					"status":                            "completed",
					"skill_id":                          skills.SkillAgentManagement,
					"tool_name":                         "get_agent_config",
					"phase":                             "post_update_verification",
					"required_post_update_verification": true,
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
					"model_provider":          "deepseek",
					"model":                   "deepseek-chat",
					"config_goal":             "模型配置为deepseek flash，写好提示词需要让agent能生成文件和上传文件。",
					"expected_updated_fields": "model_provider,model,system_prompt,file_upload_enabled,enabled_skill_ids",
				},
				"result": map[string]interface{}{
					"status":              "completed",
					"model_provider":      "deepseek",
					"model":               "deepseek-chat",
					"updated_fields":      []interface{}{"model_provider", "model", "system_prompt", "file_upload_enabled", "enabled_skill_ids"},
					"enabled_skill_ids":   []interface{}{"file-generator"},
					"file_upload_enabled": true,
				},
			},
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
						"enabled_skill_ids":   []interface{}{"file-generator"},
						"file_upload_enabled": true,
					},
				},
			},
		},
	}

	decision := completionVerificationApplyPlanOverride(evidence, completionVerificationDecision{
		Status: completionVerificationStatusPass,
		Reason: "candidate answer claims the requested Agent config is complete",
	})

	if got := decision.normalizedStatus(); got != completionVerificationStatusNeedsAction {
		t.Fatalf("decision status = %q, want needs_action; decision=%#v", got, decision)
	}
	joinedMissing := strings.Join(decision.MissingSteps, ",")
	for _, want := range []string{"model mismatch", "deepseek flash"} {
		if !strings.Contains(joinedMissing, want) {
			t.Fatalf("MissingSteps = %#v, want fragment %q", decision.MissingSteps, want)
		}
	}
	if decision.NextActionHint != "agent-management/update_agent_config" {
		t.Fatalf("NextActionHint = %q, want agent-management/update_agent_config", decision.NextActionHint)
	}
}

func TestCompletionVerificationApplyPlanOverrideRequiresBooleanConfigInPostRead(t *testing.T) {
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "running",
			"steps": []interface{}{
				map[string]interface{}{
					"id":                      "tool:agent-management/update_agent_config",
					"status":                  "completed",
					"skill_id":                skills.SkillAgentManagement,
					"tool_name":               "update_agent_config",
					"expected_updated_fields": []interface{}{"agent_memory_enabled", "file_upload_enabled"},
					"arguments": map[string]interface{}{
						"agent_memory_enabled": true,
						"file_upload_enabled":  true,
					},
				},
				map[string]interface{}{
					"id":                                "tool:agent-management/get_agent_config#post_update",
					"status":                            "completed",
					"skill_id":                          skills.SkillAgentManagement,
					"tool_name":                         "get_agent_config",
					"phase":                             "post_update_verification",
					"required_post_update_verification": true,
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
					"agent_memory_enabled": true,
					"file_upload_enabled":  true,
				},
				"result": map[string]interface{}{"status": "completed", "updated_fields": []interface{}{"agent_memory_enabled", "file_upload_enabled"}},
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
				"result": map[string]interface{}{
					"status": "completed",
					"config": map[string]interface{}{
						"agent_memory_enabled": true,
						"file_upload_enabled":  false,
					},
				},
			},
		},
	}

	decision := completionVerificationApplyPlanOverride(evidence, completionVerificationDecision{
		Status: completionVerificationStatusPass,
		Reason: "candidate answer claims upload and memory are enabled",
	})

	if got := decision.normalizedStatus(); got != completionVerificationStatusNeedsAction {
		t.Fatalf("decision status = %q, want needs_action; decision=%#v", got, decision)
	}
	joinedMissing := strings.Join(decision.MissingSteps, ",")
	if !strings.Contains(joinedMissing, "file_upload_enabled") {
		t.Fatalf("MissingSteps = %#v, want file_upload_enabled mismatch", decision.MissingSteps)
	}
	if strings.Contains(joinedMissing, "agent_memory_enabled") {
		t.Fatalf("MissingSteps = %#v, want no agent_memory_enabled mismatch when post-read confirms it", decision.MissingSteps)
	}
}

func TestCompletionVerificationApplyPlanOverrideRequiresSuggestedQuestionsInPostRead(t *testing.T) {
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "running",
			"steps": []interface{}{
				map[string]interface{}{
					"id":                      "tool:agent-management/update_agent_config",
					"status":                  "completed",
					"skill_id":                skills.SkillAgentManagement,
					"tool_name":               "update_agent_config",
					"expected_updated_fields": []interface{}{"suggested_questions"},
					"arguments": map[string]interface{}{
						"suggested_questions": []interface{}{"hello", "status"},
					},
				},
				map[string]interface{}{
					"id":                                "tool:agent-management/get_agent_config#post_update",
					"status":                            "completed",
					"skill_id":                          skills.SkillAgentManagement,
					"tool_name":                         "get_agent_config",
					"phase":                             "post_update_verification",
					"required_post_update_verification": true,
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
					"suggested_questions": []interface{}{"hello", "status"},
				},
				"result": map[string]interface{}{"status": "completed", "updated_fields": []interface{}{"suggested_questions"}},
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
				"result": map[string]interface{}{
					"status": "completed",
					"config": map[string]interface{}{
						"suggested_questions": []interface{}{"hello"},
					},
				},
			},
		},
	}

	decision := completionVerificationApplyPlanOverride(evidence, completionVerificationDecision{
		Status: completionVerificationStatusPass,
		Reason: "candidate answer claims suggested questions were updated",
	})

	if got := decision.normalizedStatus(); got != completionVerificationStatusNeedsAction {
		t.Fatalf("decision status = %q, want needs_action; decision=%#v", got, decision)
	}
	joinedMissing := strings.Join(decision.MissingSteps, ",")
	if !strings.Contains(joinedMissing, "suggested_questions mismatch") {
		t.Fatalf("MissingSteps = %#v, want suggested_questions mismatch", decision.MissingSteps)
	}
	if !strings.Contains(decision.FinalAnswerGuidance, "update_agent_config") ||
		!strings.Contains(decision.FinalAnswerGuidance, "get_agent_config") {
		t.Fatalf("FinalAnswerGuidance = %q, want retry and post-read guidance", decision.FinalAnswerGuidance)
	}
}

func TestCompletionVerificationApplyPlanOverrideRequiresResourceBindingsInPostRead(t *testing.T) {
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "running",
			"steps": []interface{}{
				map[string]interface{}{
					"id":                       "tool:agent-management/update_agent_config",
					"status":                   "completed",
					"skill_id":                 skills.SkillAgentManagement,
					"tool_name":                "update_agent_config",
					"expected_updated_fields":  []interface{}{"knowledge_dataset_ids", "database_bindings", "workflow_bindings"},
					"expected_binding_actions": "knowledge_dataset_ids:bind,database_bindings:bind,workflow_bindings:bind",
					"arguments": map[string]interface{}{
						"add_knowledge_dataset_ids": []interface{}{"kb-1"},
						"add_database_bindings":     `[{"data_source_id":"db-1","table_ids":["table-1"]}]`,
						"add_workflow_bindings":     []interface{}{map[string]interface{}{"workflow_id": "workflow-1"}},
					},
				},
				map[string]interface{}{
					"id":                                "tool:agent-management/get_agent_config#post_update",
					"status":                            "completed",
					"skill_id":                          skills.SkillAgentManagement,
					"tool_name":                         "get_agent_config",
					"phase":                             "post_update_verification",
					"required_post_update_verification": true,
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
					"add_knowledge_dataset_ids": []interface{}{"kb-1"},
					"add_database_bindings": []interface{}{map[string]interface{}{
						"data_source_id": "db-1",
						"table_ids":      []interface{}{"table-1"},
					}},
					"add_workflow_bindings": []interface{}{map[string]interface{}{"workflow_id": "workflow-1"}},
				},
				"result": map[string]interface{}{"status": "completed", "updated_fields": []interface{}{"knowledge_dataset_ids", "database_bindings", "workflow_bindings"}},
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
				"result": map[string]interface{}{
					"status": "completed",
					"config": map[string]interface{}{
						"knowledge_dataset_ids": []interface{}{"kb-1"},
						"database_bindings": []interface{}{map[string]interface{}{
							"data_source_id": "db-1",
							"table_ids":      []interface{}{"table-1"},
						}},
						"workflow_bindings": []interface{}{},
					},
				},
			},
		},
	}

	decision := completionVerificationApplyPlanOverride(evidence, completionVerificationDecision{
		Status: completionVerificationStatusPass,
		Reason: "candidate answer claims all resources are bound",
	})

	if got := decision.normalizedStatus(); got != completionVerificationStatusNeedsAction {
		t.Fatalf("decision status = %q, want needs_action; decision=%#v", got, decision)
	}
	joinedMissing := strings.Join(decision.MissingSteps, ",")
	if !strings.Contains(joinedMissing, "workflow-1") {
		t.Fatalf("MissingSteps = %#v, want missing workflow binding", decision.MissingSteps)
	}
	for _, unexpected := range []string{"kb-1", "table-1"} {
		if strings.Contains(joinedMissing, unexpected) {
			t.Fatalf("MissingSteps = %#v, did not expect confirmed target %q", decision.MissingSteps, unexpected)
		}
	}
}

func TestCompletionVerificationApplyPlanOverrideDetectsExtraTargetsAfterReplace(t *testing.T) {
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "running",
			"steps": []interface{}{
				map[string]interface{}{
					"id":                       "tool:agent-management/update_agent_config",
					"status":                   "completed",
					"skill_id":                 skills.SkillAgentManagement,
					"tool_name":                "update_agent_config",
					"expected_updated_fields":  []interface{}{"enabled_skill_ids", "database_bindings"},
					"expected_binding_actions": map[string]interface{}{"enabled_skill_ids": "replace", "database_bindings": "replace"},
					"arguments": map[string]interface{}{
						"enabled_skill_ids": []interface{}{"file-generator"},
						"database_bindings": []interface{}{map[string]interface{}{
							"data_source_id": "db-1",
							"table_ids":      []interface{}{"table-1"},
						}},
					},
				},
				map[string]interface{}{
					"id":                                "tool:agent-management/get_agent_config#post_update",
					"status":                            "completed",
					"skill_id":                          skills.SkillAgentManagement,
					"tool_name":                         "get_agent_config",
					"phase":                             "post_update_verification",
					"required_post_update_verification": true,
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
					"enabled_skill_ids": []interface{}{"file-generator"},
					"database_bindings": []interface{}{map[string]interface{}{
						"data_source_id": "db-1",
						"table_ids":      []interface{}{"table-1"},
					}},
				},
				"result": map[string]interface{}{"status": "completed", "updated_fields": []interface{}{"enabled_skill_ids", "database_bindings"}},
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
				"result": map[string]interface{}{
					"status": "completed",
					"config": map[string]interface{}{
						"enabled_skill_ids": []interface{}{"file-generator", "chart-generator"},
						"database_bindings": []interface{}{map[string]interface{}{
							"data_source_id": "db-1",
							"table_ids":      []interface{}{"table-1", "table-2"},
						}},
					},
				},
			},
		},
	}

	decision := completionVerificationApplyPlanOverride(evidence, completionVerificationDecision{
		Status: completionVerificationStatusPass,
		Reason: "candidate answer claims replacement succeeded",
	})

	if got := decision.normalizedStatus(); got != completionVerificationStatusNeedsAction {
		t.Fatalf("decision status = %q, want needs_action; decision=%#v", got, decision)
	}
	joinedMissing := strings.Join(decision.MissingSteps, ",")
	for _, want := range []string{"unexpected target after replace", "chart-generator", "table-2"} {
		if !strings.Contains(joinedMissing, want) {
			t.Fatalf("MissingSteps = %#v, want fragment %q", decision.MissingSteps, want)
		}
	}
}

func TestCompletionVerificationApplyPlanOverrideIgnoresWorkflowBindingIDDuringReplace(t *testing.T) {
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "running",
			"steps": []interface{}{
				map[string]interface{}{
					"id":                       "tool:agent-management/update_agent_config",
					"status":                   "completed",
					"skill_id":                 skills.SkillAgentManagement,
					"tool_name":                "update_agent_config",
					"expected_updated_fields":  []interface{}{"workflow_bindings"},
					"expected_binding_actions": map[string]interface{}{"workflow_bindings": "replace"},
					"arguments": map[string]interface{}{
						"workflow_bindings": []interface{}{map[string]interface{}{"workflow_id": "workflow-1"}},
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
					"workflow_bindings": []interface{}{map[string]interface{}{"workflow_id": "workflow-1"}},
				},
				"result": map[string]interface{}{"status": "completed", "updated_fields": []interface{}{"workflow_bindings"}},
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
				"result": map[string]interface{}{
					"status": "completed",
					"config": map[string]interface{}{
						"workflow_bindings": []interface{}{map[string]interface{}{
							"binding_id":  "binding-1",
							"workflow_id": "workflow-1",
						}},
					},
				},
			},
		},
	}

	decision := completionVerificationApplyPlanOverride(evidence, completionVerificationDecision{
		Status: completionVerificationStatusPass,
		Reason: "candidate answer claims workflow replacement succeeded",
	})

	if got := decision.normalizedStatus(); got != completionVerificationStatusPass {
		t.Fatalf("decision status = %q, want pass; decision=%#v", got, decision)
	}
	joinedMissing := strings.Join(decision.MissingSteps, ",")
	if strings.Contains(joinedMissing, "binding-1") {
		t.Fatalf("MissingSteps = %#v, should not treat workflow binding_id as an extra target", decision.MissingSteps)
	}
}

func TestCompletionVerificationApplyPlanOverrideIgnoresUnrelatedFailedEvidence(t *testing.T) {
	decision := completionVerificationApplyPlanOverride(map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "failed",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_config",
					"status":    "failed",
					"skill_id":  "agent-management",
					"tool_name": "update_agent_config",
				},
			},
		},
		"execution_ledger": map[string]interface{}{
			"skill_invocations": []interface{}{
				map[string]interface{}{
					"kind":      "tool_call",
					"status":    "error",
					"skill_id":  "file-reader",
					"tool_name": "read_file",
					"result": map[string]interface{}{
						"status": "error",
						"error":  "file no longer exists",
					},
				},
			},
		},
	}, completionVerificationDecision{Status: completionVerificationStatusPass, Reason: "candidate answer is supported by update evidence"})

	if got := decision.normalizedStatus(); got != completionVerificationStatusPass {
		t.Fatalf("decision status = %q, want pass when failure evidence is unrelated to failed plan step; decision=%#v", got, decision)
	}
	if decision.FinalAnswer != "" {
		t.Fatalf("FinalAnswer = %q, want no unrelated failed-plan answer", decision.FinalAnswer)
	}
}

func TestCompletionVerificationApplyPlanOnlySofteningIgnoresPlanOnlyFailedStatus(t *testing.T) {
	decision := completionVerificationApplyPlanOnlySoftening(map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "failed",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_config",
					"status":    "failed",
					"skill_id":  "agent-management",
					"tool_name": "update_agent_config",
				},
			},
		},
		"execution_ledger": map[string]interface{}{
			"skill_invocations": []interface{}{
				map[string]interface{}{
					"kind":      "tool_call",
					"status":    "success",
					"skill_id":  "agent-management",
					"tool_name": "update_agent_config",
					"result": map[string]interface{}{
						"status": "completed",
					},
				},
			},
		},
	}, completionVerificationDecision{
		Status:       completionVerificationStatusNeedsAction,
		Reason:       "operation plan failed before the tool evidence was reconciled",
		MissingSteps: []string{"agent-management/update_agent_config"},
	})

	if got := decision.normalizedStatus(); got != completionVerificationStatusPass {
		t.Fatalf("decision status = %q, want pass after plan-only softening; decision=%#v", got, decision)
	}
	if len(decision.MissingSteps) != 0 {
		t.Fatalf("MissingSteps = %#v, want cleared stale plan-only missing steps", decision.MissingSteps)
	}
}

func TestCompletionVerificationApplyPlanOnlySofteningKeepsRequestedPostUpdateConfigRead(t *testing.T) {
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":             "running",
			"original_user_goal": "Update the Agent config, then read config again after completion.",
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
	}

	decision := completionVerificationApplyPlanOnlySoftening(evidence, completionVerificationDecision{
		Status:       completionVerificationStatusNeedsAction,
		Reason:       "operation_plan still has pending step",
		MissingSteps: []string{"agent-management/get_agent_config"},
	})

	if got := decision.normalizedStatus(); got != completionVerificationStatusNeedsAction {
		t.Fatalf("decision status = %q, want needs_action; decision=%#v", got, decision)
	}
	if len(decision.MissingSteps) == 0 {
		t.Fatalf("MissingSteps cleared unexpectedly; decision=%#v", decision)
	}
}

func TestCompletionVerificationApplyPlanOnlySofteningKeepsRemainingAgentConfigFields(t *testing.T) {
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "running",
			"steps": []interface{}{
				map[string]interface{}{
					"id":                     "tool:agent-management/update_agent_config",
					"status":                 "pending",
					"skill_id":               skills.SkillAgentManagement,
					"tool_name":              "update_agent_config",
					"missing_updated_fields": []interface{}{"system_prompt"},
				},
			},
		},
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
			},
		},
	}

	decision := completionVerificationApplyPlanOnlySoftening(evidence, completionVerificationDecision{
		Status:       completionVerificationStatusNeedsAction,
		Reason:       "operation_plan still has pending step",
		MissingSteps: []string{"agent-management/update_agent_config"},
	})

	if got := decision.normalizedStatus(); got != completionVerificationStatusNeedsAction {
		t.Fatalf("decision status = %q, want needs_action; decision=%#v", got, decision)
	}
	if len(decision.MissingSteps) == 0 {
		t.Fatalf("MissingSteps cleared unexpectedly; decision=%#v", decision)
	}
}

func TestCompletionVerificationApplyPlanOnlySofteningPassesStalePendingRoutePlan(t *testing.T) {
	decision := completionVerificationApplyPlanOnlySoftening(map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "running",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "route:/console/files",
					"status":    "pending",
					"skill_id":  "console-navigator",
					"tool_name": "navigate",
				},
			},
		},
		"client_actions": []interface{}{
			map[string]interface{}{
				"kind":        "client_action",
				"status":      "succeeded",
				"skill_id":    "console-navigator",
				"tool_name":   "navigate",
				"action_type": "route_navigation",
				"result": map[string]interface{}{
					"event_type":         "route_loaded",
					"href":               "/console/files",
					"page_context_ready": true,
				},
			},
		},
	}, completionVerificationDecision{
		Status:         completionVerificationStatusNeedsAction,
		Reason:         "operation plan still has a pending executable step",
		MissingSteps:   []string{"console-navigator/navigate"},
		NextActionHint: "Call the required_next_tool again.",
	})

	if got := decision.normalizedStatus(); got != completionVerificationStatusPass {
		t.Fatalf("decision status = %q, want pass; decision=%#v", got, decision)
	}
	if len(decision.MissingSteps) != 0 {
		t.Fatalf("missing steps = %#v, want cleared plan-only missing steps", decision.MissingSteps)
	}
}

func TestCompletionVerificationPendingExecutablePlanStepSkipsSuccessfulStatuses(t *testing.T) {
	for _, tc := range []struct {
		name       string
		planStatus string
		stepStatus string
	}{
		{name: "plan success", planStatus: "success", stepStatus: "pending"},
		{name: "plan succeeded", planStatus: "succeeded", stepStatus: "pending"},
		{name: "step success", planStatus: "running", stepStatus: "success"},
		{name: "step succeeded", planStatus: "running", stepStatus: "succeeded"},
		{name: "step skipped", planStatus: "running", stepStatus: "skipped"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := completionVerificationPendingExecutablePlanStep(map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"status": tc.planStatus,
					"steps": []interface{}{
						map[string]interface{}{
							"id":        "tool:agent-management/update_agent_config",
							"status":    tc.stepStatus,
							"skill_id":  skills.SkillAgentManagement,
							"tool_name": "update_agent_config",
						},
					},
				},
			})
			if ok {
				t.Fatalf("pending executable step = true, want false for plan status %q step status %q", tc.planStatus, tc.stepStatus)
			}
		})
	}
}

func TestCompletionVerificationApplyPlanOnlySofteningKeepsRealMissingToolEvidence(t *testing.T) {
	decision := completionVerificationApplyPlanOnlySoftening(map[string]interface{}{
		"generated_files": []interface{}{
			map[string]interface{}{
				"filename":     "draft.md",
				"tool_file_id": "tool-file-1",
			},
		},
	}, completionVerificationDecision{
		Status:       completionVerificationStatusNeedsAction,
		Reason:       "operation plan still has a pending executable step",
		MissingSteps: []string{"file-manager/save_file_to_management"},
	})

	if got := decision.normalizedStatus(); got != completionVerificationStatusNeedsAction {
		t.Fatalf("decision status = %q, want needs_action; decision=%#v", got, decision)
	}
}

func TestCompletionVerificationApplyPlanOnlySofteningKeepsPendingManagedFileSave(t *testing.T) {
	decision := completionVerificationApplyPlanOnlySoftening(map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":              "running",
			"pending_next_action": "save_remaining_generated_files_to_file_management",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:file-manager/save_file_to_management",
					"status":    "pending",
					"skill_id":  skills.SkillFileManager,
					"tool_name": "save_file_to_management",
				},
			},
		},
		"generated_files": []interface{}{
			map[string]interface{}{
				"filename":     "draft.svg",
				"tool_file_id": "tool-file-1",
				"target":       "temporary_artifact",
			},
		},
	}, completionVerificationDecision{
		Status:       completionVerificationStatusNeedsAction,
		Reason:       "operation plan still has a pending executable step",
		MissingSteps: []string{"operation plan pending save step"},
	})

	if got := decision.normalizedStatus(); got != completionVerificationStatusNeedsAction {
		t.Fatalf("decision status = %q, want needs_action for unsaved generated file; decision=%#v", got, decision)
	}
	if len(decision.MissingSteps) == 0 {
		t.Fatalf("missing steps = %#v, want save step retained", decision.MissingSteps)
	}
}

func TestCompletionVerificationApplyPlanOnlySofteningAllowsCompletedManagedFileSave(t *testing.T) {
	decision := completionVerificationApplyPlanOnlySoftening(map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":              "running",
			"pending_next_action": "save_remaining_generated_files_to_file_management",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:file-manager/save_file_to_management",
					"status":    "pending",
					"skill_id":  skills.SkillFileManager,
					"tool_name": "save_file_to_management",
				},
			},
		},
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillFileManager,
				"tool_name": "save_file_to_management",
				"result": map[string]interface{}{
					"status":       "completed",
					"target":       "managed_file",
					"file_id":      "file-1",
					"filename":     "draft.svg",
					"tool_file_id": "tool-file-1",
				},
			},
		},
	}, completionVerificationDecision{
		Status:       completionVerificationStatusNeedsAction,
		Reason:       "operation plan still has a pending executable step",
		MissingSteps: []string{"operation plan pending save step"},
	})

	if got := decision.normalizedStatus(); got != completionVerificationStatusPass {
		t.Fatalf("decision status = %q, want pass once managed file save evidence exists; decision=%#v", got, decision)
	}
	if len(decision.MissingSteps) != 0 {
		t.Fatalf("missing steps = %#v, want cleared plan-only save step", decision.MissingSteps)
	}
}

func TestCompletionVerificationEvidenceInvocationsIncludesTopLevelClientActions(t *testing.T) {
	invocations := completionVerificationEvidenceInvocations(map[string]interface{}{
		"client_actions": []interface{}{
			map[string]interface{}{
				"kind":        "client_action",
				"status":      "failed",
				"skill_id":    "console-navigator",
				"tool_name":   "navigate",
				"action_type": "route_navigation",
				"result": map[string]interface{}{
					"error": "route did not finish loading",
				},
			},
		},
	})

	if len(invocations) != 1 {
		t.Fatalf("invocations = %#v, want one top-level client action", invocations)
	}
	if !completionVerificationInvocationFailed(invocations[0]) {
		t.Fatalf("top-level client action was not treated as failed evidence: %#v", invocations[0])
	}
}

func TestCompletionVerificationDetectsBatchItemFailureInResultSummary(t *testing.T) {
	invocation := map[string]interface{}{
		"status":    "success",
		"skill_id":  "agent-management",
		"tool_name": "delete_agents",
		"result_summary": map[string]interface{}{
			"status": "partial_failed",
			"operation_group": map[string]interface{}{
				"item_results": []interface{}{
					map[string]interface{}{"agent_id": "agent-ok", "agent_name": "Agent OK", "status": "succeeded"},
					map[string]interface{}{"agent_id": "agent-locked", "agent_name": "Agent Locked", "status": "failed", "error": "locked"},
				},
			},
		},
	}

	if completionVerificationInvocationSucceeded(invocation) {
		t.Fatalf("batch invocation = %#v, want failed item to prevent success classification", invocation)
	}
	if !completionVerificationInvocationFailed(invocation) {
		t.Fatalf("batch invocation = %#v, want failed item to be failed evidence", invocation)
	}
	if detail := completionVerificationInvocationFailureDetail(invocation); !strings.Contains(detail, "Agent Locked") || !strings.Contains(detail, "locked") {
		t.Fatalf("failure detail = %q, want failed item name and reason", detail)
	}
}

func TestCompletionVerificationEvidenceInvocationsIncludesExecutionSummary(t *testing.T) {
	invocations := completionVerificationEvidenceInvocations(map[string]interface{}{
		"execution_summary": map[string]interface{}{
			"tool_results": []interface{}{
				map[string]interface{}{
					"status":    "success",
					"skill_id":  "agent-management",
					"tool_name": "delete_agents",
					"result_summary": map[string]interface{}{
						"status": "completed",
					},
				},
			},
		},
	})

	if len(invocations) != 1 {
		t.Fatalf("invocations = %#v, want one execution summary tool result", invocations)
	}
	if !completionVerificationInvocationSucceeded(invocations[0]) {
		t.Fatalf("execution summary invocation was not treated as successful evidence: %#v", invocations[0])
	}
}

func TestCompletionVerificationEvidenceInvocationsIncludesOperationResultSummary(t *testing.T) {
	invocations := completionVerificationEvidenceInvocations(map[string]interface{}{
		"operation_result_summary": map[string]interface{}{
			"status":    "partial_failed",
			"skill_id":  "agent-management",
			"tool_name": "delete_agents",
			"operation_group": map[string]interface{}{
				"operation":     "agent.delete",
				"asset_type":    "agent",
				"target_count":  2,
				"success_count": 1,
				"failed_count":  1,
				"item_results": []interface{}{
					map[string]interface{}{"agent_name": "Agent OK", "status": "succeeded"},
					map[string]interface{}{"agent_name": "Agent Locked", "status": "failed", "error": "locked"},
				},
			},
		},
	})

	if len(invocations) != 1 {
		t.Fatalf("invocations = %#v, want one operation summary invocation", invocations)
	}
	if completionVerificationInvocationSucceeded(invocations[0]) {
		t.Fatalf("operation summary invocation = %#v, want failed item to prevent success classification", invocations[0])
	}
	if !completionVerificationInvocationFailed(invocations[0]) {
		t.Fatalf("operation summary invocation = %#v, want failed item to be failed evidence", invocations[0])
	}
	if detail := completionVerificationInvocationFailureDetail(invocations[0]); !strings.Contains(detail, "1 item") {
		t.Fatalf("failure detail = %q, want failed count evidence", detail)
	}
}

func TestCompletionVerificationFailedPlanAnswerUsesClientActionError(t *testing.T) {
	decision := completionVerificationApplyPlanOverride(map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "failed",
			"steps": []map[string]interface{}{
				{
					"id":     "route:/console/files",
					"title":  "文件管理",
					"status": "failed",
				},
			},
		},
		"execution_ledger": map[string]interface{}{
			"client_actions": []map[string]interface{}{
				{
					"kind":        "client_action",
					"status":      "failed",
					"action_type": "route_navigation",
					"result": map[string]interface{}{
						"status": "failed",
						"error":  "route did not finish loading",
					},
				},
			},
		},
	}, completionVerificationDecision{Status: completionVerificationStatusPass})

	if got := decision.normalizedStatus(); got != completionVerificationStatusFailed {
		t.Fatalf("decision status = %q, want failed; decision=%#v", got, decision)
	}
	if !strings.Contains(decision.FinalAnswer, "文件管理") {
		t.Fatalf("final_answer = %q, want failed route step label", decision.FinalAnswer)
	}
	if !strings.Contains(decision.FinalAnswer, "route did not finish loading") {
		t.Fatalf("final_answer = %q, want client action failure detail", decision.FinalAnswer)
	}
}

func TestCompletionVerificationShouldRunForLedgerOnlyEvidence(t *testing.T) {
	tests := []struct {
		name     string
		evidence map[string]interface{}
		want     bool
	}{
		{
			name: "top-level operation ledger",
			evidence: map[string]interface{}{
				"operation_ledger": map[string]interface{}{
					"version": "operation_ledger.v1",
					"status":  "observed",
					"resources": []map[string]interface{}{{
						"name": "visible.md",
						"type": "file",
					}},
				},
			},
			want: true,
		},
		{
			name: "execution ledger operation ledger",
			evidence: map[string]interface{}{
				"execution_ledger": map[string]interface{}{
					"operation_ledger": map[string]interface{}{
						"version": "operation_ledger.v1",
						"status":  "observed",
						"capabilities": []map[string]interface{}{{
							"name": "read_file",
						}},
					},
				},
			},
			want: true,
		},
		{
			name: "top-level client action",
			evidence: map[string]interface{}{
				"client_actions": []map[string]interface{}{{
					"kind":   "client_action",
					"status": "succeeded",
				}},
			},
			want: true,
		},
		{
			name: "execution ledger client action",
			evidence: map[string]interface{}{
				"execution_ledger": map[string]interface{}{
					"client_actions": []interface{}{map[string]interface{}{
						"kind":   "client_action",
						"status": "failed",
					}},
				},
			},
			want: true,
		},
		{
			name: "empty execution ledger",
			evidence: map[string]interface{}{
				"execution_ledger": map[string]interface{}{},
			},
			want: false,
		},
		{
			name: "completed plan only does not require verifier",
			evidence: map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"status":              "completed",
					"pending_next_action": "none",
					"steps": []interface{}{
						map[string]interface{}{
							"id":     "observe",
							"status": "completed",
						},
					},
					"step_status": map[string]interface{}{
						"observe": "completed",
					},
				},
				"page_context": map[string]interface{}{
					"target_route_already_available": true,
					"route_evidence":                 "current_page_context_matches_target",
				},
			},
			want: false,
		},
		{
			name: "pending tool plan requires verifier",
			evidence: map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"status":              "running",
					"pending_next_action": "agent-management/delete_agents",
					"steps": []interface{}{
						map[string]interface{}{
							"id":        "tool:agent-management/delete_agents",
							"status":    "pending",
							"skill_id":  "agent-management",
							"tool_name": "delete_agents",
						},
					},
				},
			},
			want: true,
		},
		{
			name: "failed plan requires verifier",
			evidence: map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"status":              "failed",
					"pending_next_action": "none",
				},
			},
			want: true,
		},
		{
			name: "failed plan step requires verifier",
			evidence: map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"status":              "running",
					"pending_next_action": "none",
					"steps": []interface{}{
						map[string]interface{}{
							"id":        "tool:file-manager/save_file_to_management",
							"status":    "failed",
							"skill_id":  "file-manager",
							"tool_name": "save_file_to_management",
						},
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := completionVerificationShouldRun(tt.evidence, nil, nil, 0)
			if got != tt.want {
				t.Fatalf("completionVerificationShouldRun() = %v, want %v for evidence %#v", got, tt.want, tt.evidence)
			}
		})
	}
}
