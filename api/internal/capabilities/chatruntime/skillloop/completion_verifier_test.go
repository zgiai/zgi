package skillloop

import (
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

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
		"\u7f3a\u5c11\u7684\u5b8c\u6210\u8bc1\u636e\uff1aagent-management/update_agent_config\u3002",
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
