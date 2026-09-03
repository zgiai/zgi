package skillloop

import (
	"context"
	"strings"
	"testing"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestToolFailureExplanationOnlyDescribesProviderRequestStateWhenObserved(t *testing.T) {
	unknown := buildToolFailureExplanationEvidence(skills.SkillTrace{
		Kind: "tool_call", Status: "error", Error: "provider failed", Arguments: map[string]interface{}{},
	}, nil, "failed")
	if unknown.ProviderRequestSent != nil {
		t.Fatalf("unknown provider request state = %#v, want omitted", unknown.ProviderRequestSent)
	}

	preflight := buildToolFailureExplanationEvidence(skills.SkillTrace{
		Kind: "tool_call", Status: "error", Error: "integration_invalid_input",
		Arguments: map[string]interface{}{
			"reason_code":           "action_arguments_schema_mismatch",
			"provider_request_sent": false,
		},
	}, nil, "failed")
	if preflight.ProviderRequestSent == nil || *preflight.ProviderRequestSent {
		t.Fatalf("preflight provider request state = %#v, want explicit false", preflight.ProviderRequestSent)
	}
}

func TestToolFailureExplanationUsesOnlyTrustworthyCompletedOperations(t *testing.T) {
	tests := []struct {
		name       string
		call       SkillToolCallRef
		wantCount  int
		wantStatus string
		wantAction string
	}{
		{
			name: "ordinary successful business tool",
			call: SkillToolCallRef{
				SkillID: "file-generator", ToolName: "generate_docx",
			},
			wantCount: 1, wantStatus: "success",
		},
		{
			name:      "provider-confirmed external action",
			call:      externalActionFailureExplanationCall("completed", true, true),
			wantCount: 1, wantStatus: "provider_confirmed_success", wantAction: "wecom.message.send",
		},
		{
			name:      "outcome unknown external action",
			call:      externalActionFailureExplanationCall("outcome_unknown", false, true),
			wantCount: 0,
		},
		{
			name:      "completed external action without confirmation",
			call:      externalActionFailureExplanationCall("completed", false, false),
			wantCount: 0,
		},
		{
			name:      "partial external result even when confirmation is malformed",
			call:      externalActionFailureExplanationCall("partially_succeeded", true, true),
			wantCount: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evidence := buildToolFailureExplanationEvidence(skills.SkillTrace{
				Kind: "tool_call", SkillID: "another-skill", ToolName: "unfinished_operation", Status: "error",
			}, []SkillToolCallRef{test.call}, "business operation failed")
			if got := len(evidence.CompletedOperations); got != test.wantCount {
				t.Fatalf("completed operations = %#v, want count %d", evidence.CompletedOperations, test.wantCount)
			}
			if evidence.PartialSuccess != (test.wantCount > 0) {
				t.Fatalf("partial_success = %v, completed=%#v", evidence.PartialSuccess, evidence.CompletedOperations)
			}
			if test.wantCount == 0 {
				return
			}
			completed := evidence.CompletedOperations[0]
			if completed.Status != test.wantStatus || completed.ActionID != test.wantAction {
				t.Fatalf("completed operation = %#v, want status=%q action=%q", completed, test.wantStatus, test.wantAction)
			}
		})
	}
}

func TestToolFailureExplanationPromptSeparatesPartialSuccessFromFailure(t *testing.T) {
	failure := skills.SkillTrace{
		Kind: "tool_call", SkillID: skills.SkillExternalApps, ToolName: "execute_action", Status: "error",
		Arguments: map[string]interface{}{
			"integration_id": "wecom", "action_id": "wecom.calendar.event.create",
		},
	}
	partial := buildToolFailureExplanationEvidence(
		failure,
		[]SkillToolCallRef{externalActionFailureExplanationCall("completed", true, true)},
		"one required action failed",
	)
	prompt := toolFailureExplanationSystemPrompt(partial, "evidence-json")
	for _, required := range []string{
		"only partially completed",
		"completed_operations",
		"failed_operation",
		"Never say that the whole request failed",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("partial-success prompt does not contain %q:\n%s", required, prompt)
		}
	}
	if strings.Contains(prompt, "The requested operation could not be completed by the available business tool.") ||
		strings.Contains(prompt, "never claim that a file, asset, update, or other side effect succeeded") {
		t.Fatalf("partial-success prompt retained blanket failure guidance:\n%s", prompt)
	}
	if partial.FailedOperation.ActionID != "wecom.calendar.event.create" ||
		partial.CompletedOperations[0].ActionID != "wecom.message.send" {
		t.Fatalf("partial-success operations are not distinguishable: %#v", partial)
	}

	allFailed := buildToolFailureExplanationEvidence(failure, []SkillToolCallRef{
		externalActionFailureExplanationCall("outcome_unknown", false, true),
	}, "provider outcome unknown")
	allFailedPrompt := toolFailureExplanationSystemPrompt(allFailed, "evidence-json")
	if allFailed.PartialSuccess || len(allFailed.CompletedOperations) != 0 {
		t.Fatalf("outcome_unknown became partial success: %#v", allFailed)
	}
	for _, required := range []string{
		"The requested operation could not be completed by the available business tool.",
		"never claim that a file, asset, update, or other side effect succeeded",
	} {
		if !strings.Contains(allFailedPrompt, required) {
			t.Fatalf("ordinary failure prompt lost %q:\n%s", required, allFailedPrompt)
		}
	}
}

func TestToolFailureExplanationUsesHistoricalConfirmedServerOutcomeForPartialSuccess(t *testing.T) {
	sendOutcome, sendPhase, sendCandidate := failureExplanationCanonicalOutcome(
		"outcome-send", "phase-send", "epoch-send", "binding-send",
		"wecom", "wecom.message.send", "recipient_ref", "alice",
	)
	calendarOutcome, calendarPhase, calendarCandidate := failureExplanationCanonicalOutcome(
		"outcome-calendar", "phase-calendar", "epoch-calendar", "binding-calendar",
		"wecom", "wecom.calendar.event.create", "calendar_id", "calendar-1",
	)
	state := map[string]interface{}{
		runtimeStateNativeExternalActionCandidatesKey: []interface{}{sendCandidate, calendarCandidate},
		"operation_plan": map[string]interface{}{
			"outcomes": []interface{}{sendOutcome, calendarOutcome},
			"phases":   []interface{}{sendPhase, calendarPhase},
		},
		// This confirmation was persisted by an earlier turn; this failure path
		// receives no successfulToolCalls for the current continuation.
		"skill_invocations": []interface{}{
			failureExplanationExternalInvocation(
				"tool_call", "phase-send", "epoch-send", "binding-send",
				"wecom", "wecom.message.send", "recipient_ref", "alice", "completed", true,
			),
		},
	}
	failure := skills.SkillTrace{
		Kind: "tool_call", SkillID: skills.SkillExternalApps, ToolName: "execute_action", Status: "error",
		Arguments: map[string]interface{}{
			"integration_id": "wecom", "action_id": "wecom.calendar.event.create",
		},
	}
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{{
		Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "消息已发送，但日历事件创建失败。"}, FinishReason: "stop"}},
	}}}
	runner := &Runner{LLMClient: fakeLLM, AppContext: &llmclient.AppContext{}}
	toolSet := &skills.NativeToolSet{ExternalActionCandidates: []skills.NativeExternalActionCandidate{
		failureExplanationNativeCandidate(sendCandidate),
		failureExplanationNativeCandidate(calendarCandidate),
	}}
	prepared := NewPreparedChat("conv-partial-history", "msg-partial-history", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "发送消息并创建日历事件"}},
	})
	answer, _, err := runner.runToolFailureExplanation(context.Background(), RunRequest{
		NativeToolSet:        toolSet,
		RuntimeStateSnapshot: func() map[string]interface{} { return state },
	}, prepared, failure, nil, "calendar provider failed", 2)
	if err != nil || answer != "消息已发送，但日历事件创建失败。" {
		t.Fatalf("runToolFailureExplanation() answer=%q error=%v", answer, err)
	}
	if fakeLLM.appChatCalls != 1 || !runnerTestRequestContains(fakeLLM.appChatRequests[0], `"partial_success":true`) ||
		!runnerTestRequestContains(fakeLLM.appChatRequests[0], `"action_id":"wecom.message.send"`) {
		contents := make([]string, 0, len(fakeLLM.appChatRequests[0].Messages))
		for _, message := range fakeLLM.appChatRequests[0].Messages {
			contents = append(contents, messageContent(message.Content))
		}
		t.Fatalf("historical server Outcome was absent from partial-success evidence:\n%s", strings.Join(contents, "\n---\n"))
	}
}

func TestToolFailureExplanationPreparationSuccessDoesNotBecomePartialSuccess(t *testing.T) {
	sendOutcome, sendPhase, sendCandidate := failureExplanationCanonicalOutcome(
		"outcome-send", "phase-send", "epoch-send", "binding-send",
		"wecom", "wecom.message.send", "recipient_ref", "alice",
	)
	_, preparationPhase, searchCandidate := failureExplanationCanonicalOutcome(
		"outcome-preparation", "phase-search", "epoch-search", "binding-search",
		"wecom", "wecom.contact.search", "query", "Alice",
	)
	searchCandidate["effect"] = "read"
	sendCandidate["preparation_hints"] = []interface{}{map[string]interface{}{
		"action_key": "wecom:wecom.contact.search", "relation": "resolve_target",
		"target_arguments": []interface{}{"recipient_ref"}, "result_paths": []interface{}{"members[].recipient_ref"},
	}}
	preparationPhase[operationPlanPhaseSourceKey] = operationPlanPhaseSourceServerPreparation
	preparationPhase["required"] = true
	state := map[string]interface{}{
		runtimeStateNativeExternalActionCandidatesKey: []interface{}{sendCandidate, searchCandidate},
		"operation_plan": map[string]interface{}{
			"outcomes": []interface{}{sendOutcome},
			"phases":   []interface{}{sendPhase, preparationPhase},
		},
		"skill_invocations": []interface{}{
			failureExplanationExternalInvocation(
				"tool_call", "phase-search", "epoch-search", "binding-search",
				"wecom", "wecom.contact.search", "query", "Alice", "completed", true,
			),
		},
	}
	preparationCall := SkillToolCallRef{
		SkillID: skills.SkillExternalApps, ToolName: "execute_action",
		Arguments: map[string]interface{}{"integration_id": "wecom", "action_id": "wecom.contact.search"},
		Result: map[string]interface{}{
			"integration_id": "wecom", "action_id": "wecom.contact.search",
			"operation_status": "completed", "provider_success_confirmed": true,
		},
	}
	evidence := buildToolFailureExplanationEvidenceWithRuntimeState(skills.SkillTrace{
		Kind: "tool_call", SkillID: skills.SkillExternalApps, ToolName: "execute_action", Status: "error",
		Arguments: map[string]interface{}{"integration_id": "wecom", "action_id": "wecom.message.send"},
	}, []SkillToolCallRef{preparationCall}, "send failed", state)
	if evidence.PartialSuccess || len(evidence.CompletedOperations) != 0 {
		t.Fatalf("preparation query became a completed user Outcome: %#v", evidence)
	}
}

func TestToolFailureExplanationRejectsGovernanceAndUnknownOutcomeEvidence(t *testing.T) {
	outcome, phase, candidate := failureExplanationCanonicalOutcome(
		"outcome-send", "phase-send", "epoch-send", "binding-send",
		"wecom", "wecom.message.send", "recipient_ref", "alice",
	)
	state := map[string]interface{}{
		runtimeStateNativeExternalActionCandidatesKey: []interface{}{candidate},
		"operation_plan": map[string]interface{}{
			"outcomes": []interface{}{outcome}, "phases": []interface{}{phase},
		},
		"skill_invocations": []interface{}{
			failureExplanationExternalInvocation(
				"tool_governance", "phase-send", "epoch-send", "binding-send",
				"wecom", "wecom.message.send", "recipient_ref", "alice", "completed", true,
			),
			failureExplanationExternalInvocation(
				"tool_call", "phase-send", "epoch-send", "binding-send",
				"wecom", "wecom.message.send", "recipient_ref", "alice", "outcome_unknown", false,
			),
		},
	}
	evidence := buildToolFailureExplanationEvidenceWithRuntimeState(skills.SkillTrace{
		Kind: "tool_call", SkillID: skills.SkillExternalApps, ToolName: "execute_action", Status: "error",
	}, nil, "outcome unknown", state)
	if evidence.PartialSuccess || len(evidence.CompletedOperations) != 0 {
		t.Fatalf("governance/outcome_unknown became a completed server Outcome: %#v", evidence)
	}
}

func externalActionFailureExplanationCall(operationStatus string, providerConfirmed bool, includeConfirmation bool) SkillToolCallRef {
	result := map[string]interface{}{
		"integration_id": "wecom", "action_id": "wecom.message.send", "operation_status": operationStatus,
	}
	if includeConfirmation {
		result["provider_success_confirmed"] = providerConfirmed
	}
	return SkillToolCallRef{
		SkillID: skills.SkillExternalApps, ToolName: "execute_action",
		Arguments: map[string]interface{}{"integration_id": "wecom", "action_id": "wecom.message.send"},
		Result:    result,
	}
}

func failureExplanationCanonicalOutcome(
	outcomeID string,
	phaseID string,
	ledgerEpoch string,
	bindingFingerprint string,
	integrationID string,
	actionID string,
	targetPath string,
	targetValue string,
) (map[string]interface{}, map[string]interface{}, map[string]interface{}) {
	connectionBinding := skills.NativeExternalActionConnectionBindingHash("connection-" + integrationID)
	target := map[string]interface{}{targetPath: targetValue}
	outcome := map[string]interface{}{
		"id": outcomeID, "required": true, "verification_mode": "runtime_effects",
		"capabilities": []interface{}{skills.SkillExternalApps},
	}
	phase := map[string]interface{}{
		"id": phaseID, "outcome_id": outcomeID, "status": "in_progress", "required": true,
		operationPlanPhaseSourceKey:                      operationPlanPhaseSourceServerOutcome,
		operationPlanServerProjectedLedgerEpochKey:       ledgerEpoch,
		operationPlanServerProjectedConnectionBindingKey: connectionBinding,
		"expected_action": map[string]interface{}{
			"skill_id": skills.SkillExternalApps, "tool_name": "execute_action",
			planExpectedActionServerBindingFingerprintKey: bindingFingerprint,
			"target":           map[string]interface{}{"integration_id": integrationID, "action_id": actionID},
			"target_arguments": target,
		},
	}
	candidate := map[string]interface{}{
		"integration_id": integrationID, "action_id": actionID,
		"binding_fingerprint":                            bindingFingerprint,
		operationPlanServerProjectedConnectionBindingKey: connectionBinding,
		"target_argument_paths":                          []interface{}{targetPath},
		"intent_matched":                                 true,
		"effect":                                         "external_send",
	}
	return outcome, phase, candidate
}

func failureExplanationExternalInvocation(
	kind string,
	phaseID string,
	ledgerEpoch string,
	bindingFingerprint string,
	integrationID string,
	actionID string,
	targetPath string,
	targetValue string,
	operationStatus string,
	providerConfirmed bool,
) map[string]interface{} {
	return map[string]interface{}{
		"kind": kind, "status": "success", "skill_id": skills.SkillExternalApps, "tool_name": "execute_action",
		"arguments": map[string]interface{}{
			"plan_phase_id": phaseID,
			operationPlanServerProjectedLedgerEpochKey:       ledgerEpoch,
			planExpectedActionServerBindingFingerprintKey:    bindingFingerprint,
			operationPlanServerProjectedConnectionBindingKey: skills.NativeExternalActionConnectionBindingHash("connection-" + integrationID),
			"operation_plan_target":                          map[string]interface{}{targetPath: targetValue},
		},
		"result": map[string]interface{}{
			"integration_id": integrationID, "action_id": actionID,
			"operation_status": operationStatus, "provider_success_confirmed": providerConfirmed,
		},
	}
}

func failureExplanationNativeCandidate(candidate map[string]interface{}) skills.NativeExternalActionCandidate {
	return skills.NativeExternalActionCandidate{
		IntegrationID:       evidenceStringFromAny(candidate["integration_id"]),
		ActionID:            evidenceStringFromAny(candidate["action_id"]),
		BindingFingerprint:  evidenceStringFromAny(candidate["binding_fingerprint"]),
		ConnectionBinding:   evidenceStringFromAny(candidate[operationPlanServerProjectedConnectionBindingKey]),
		Effect:              evidenceStringFromAny(candidate["effect"]),
		TargetArgumentPaths: evidenceStringSliceFromAny(candidate["target_argument_paths"]),
		IntentMatched:       true,
	}
}
