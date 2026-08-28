package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestApprovedFrozenExternalActionFailureUsesDeterministicAnswer(t *testing.T) {
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{Query: "给杨志航发送飞书消息"},
	}
	frozen := toolgovernance.FrozenInvocation{
		SkillID:  skills.SkillExternalApps,
		ToolName: "execute_action",
		Effect:   toolgovernance.EffectExternalSend,
	}
	if !approvedFrozenExternalAction(frozen) {
		t.Fatal("approvedFrozenExternalAction() = false")
	}
	answer := approvedFrozenExternalActionFailureAnswer(prepared, frozen)
	if answer != "发送未完成。系统没有取得服务商的成功回执，因此不能视为已发送。请先检查连接状态和执行记录，确认后再重试。" {
		t.Fatalf("approvedFrozenExternalActionFailureAnswer() = %q", answer)
	}
}

func TestCompleteApprovedFrozenExternalActionFailurePersistsFailedEvidence(t *testing.T) {
	now := time.Now().UTC()
	messageID := uuid.New()
	conversationID := uuid.New()
	message := &runtimemodel.Message{
		ID: messageID, ConversationID: conversationID, Query: "给杨志航发送飞书消息",
		Status: runtimemodel.MessageStatusWaitingApproval, Metadata: map[string]interface{}{},
		CreatedAt: now, UpdatedAt: now,
	}
	conversation := &runtimemodel.Conversation{
		ID: conversationID, Status: runtimemodel.ConversationStatusNormal,
		RuntimeStatus:        runtimemodel.ConversationRuntimeStatusStreaming,
		CurrentLeafMessageID: &messageID, CreatedAt: now, UpdatedAt: now,
	}
	messageRepo := &toolGovernanceStreamMessageRepo{message: message}
	conversationRepo := &toolGovernanceStreamConversationRepo{conversation: conversation}
	svc := &service{
		repos:  &repository.Repositories{Message: messageRepo, Conversation: conversationRepo},
		events: newStreamEventStore(nil),
	}
	prepared := &PreparedChat{Conversation: conversation, Message: message, Continuation: true}
	frozen := toolgovernance.FrozenInvocation{
		SkillID: skills.SkillExternalApps, ToolName: "execute_action", Effect: toolgovernance.EffectExternalSend,
		Arguments: map[string]interface{}{"action_id": "feishu.message.send_user"},
	}
	result, err := svc.completeApprovedFrozenExternalActionFailure(context.Background(), prepared, frozen, func(StreamEvent) error { return nil })
	if err != nil {
		t.Fatalf("completeApprovedFrozenExternalActionFailure() error = %v", err)
	}
	if !messageRepo.updateCompletedCalled || result == nil || result.Status != runtimemodel.MessageStatusCompleted {
		t.Fatalf("completion result = %#v, updateCompleted=%v", result, messageRepo.updateCompletedCalled)
	}
	if result.Answer != "发送未完成。系统没有取得服务商的成功回执，因此不能视为已发送。请先检查连接状态和执行记录，确认后再重试。" {
		t.Fatalf("result answer = %q", result.Answer)
	}
	summary := mapFromOperationContext(result.Metadata["operation_result_summary"])
	if summary["status"] != "failed" || summary["provider_success_confirmed"] != false || summary["operation"] != "feishu.message.send_user" {
		t.Fatalf("operation result summary = %#v", summary)
	}
}

func TestCompleteApprovedFrozenExternalActionSuccessTerminatesSingleOperation(t *testing.T) {
	frozen := toolgovernance.FrozenInvocation{
		SkillID: skills.SkillExternalApps, ToolName: "execute_action",
		Arguments: map[string]interface{}{
			"integration_id": "feishu", "action_id": "feishu.calendar.event.create",
		},
	}
	invocation := &skills.ToolInvocationResult{Trace: skills.SkillTrace{
		Status: "success",
		Result: map[string]interface{}{
			"integration_id": "feishu", "action_id": "feishu.calendar.event.create",
			"operation_status": "completed", "provider_success_confirmed": true,
			"provider_result": map[string]interface{}{
				"event": map[string]interface{}{"event_id": "event-1", "summary": "未来七天日程"},
			},
		},
	}}

	if !approvedFrozenExternalActionProviderSuccess(frozen, invocation) {
		t.Fatal("approvedFrozenExternalActionProviderSuccess() = false")
	}
	metadata, terminal := completeApprovedFrozenExternalActionSuccess(map[string]interface{}{}, frozen, invocation)
	if !terminal {
		t.Fatal("completeApprovedFrozenExternalActionSuccess() terminal = false")
	}
	summary := mapFromOperationContext(metadata["operation_result_summary"])
	if summary["status"] != operationPlanStatusCompleted || summary["provider_success_confirmed"] != true || summary["success_count"] != 1 {
		t.Fatalf("operation result summary = %#v", summary)
	}
	latest := mapFromOperationContext(summary["latest_tool_result"])
	if latest["operation_status"] != "completed" {
		t.Fatalf("latest tool result = %#v", latest)
	}
}

func TestCompleteApprovedFrozenExternalActionSuccessKeepsDifferentPlannedAction(t *testing.T) {
	frozen := toolgovernance.FrozenInvocation{
		SkillID: skills.SkillExternalApps, ToolName: "execute_action",
		Arguments: map[string]interface{}{
			"integration_id": "feishu", "action_id": "feishu.calendar.event.create",
		},
	}
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": operationPlanStatusRunning,
			"phases": []interface{}{
				map[string]interface{}{
					"id": "phase-calendar", "status": "in_progress",
					"expected_action": map[string]interface{}{
						"skill_id": skills.SkillExternalApps, "tool_name": "execute_action",
						"target": map[string]interface{}{"integration_id": "feishu", "action_id": "feishu.calendar.event.create"},
					},
				},
				map[string]interface{}{
					"id": "phase-message", "status": operationPlanStepStatusPending,
					"expected_action": map[string]interface{}{
						"skill_id": skills.SkillExternalApps, "tool_name": "execute_action",
						"target": map[string]interface{}{"integration_id": "feishu", "action_id": "feishu.message.send_user"},
					},
				},
			},
		},
	}
	invocation := &skills.ToolInvocationResult{Trace: skills.SkillTrace{
		Status: "success", Result: map[string]interface{}{
			"action_id": "feishu.calendar.event.create", "operation_status": "completed",
		},
	}}

	next, terminal := completeApprovedFrozenExternalActionSuccess(metadata, frozen, invocation)
	if terminal {
		t.Fatal("completeApprovedFrozenExternalActionSuccess() terminal = true with a different pending action")
	}
	plan := mapFromOperationContext(next["operation_plan"])
	if plan["status"] != operationPlanStatusRunning {
		t.Fatalf("operation plan status = %#v", plan["status"])
	}
}

func TestApprovedProjectedGovernanceResumeNeverBroadCompletesRemainingNativePhase(t *testing.T) {
	frozen := projectedExternalActionCompletionFrozenInvocation("Alice")
	metadata := projectedExternalActionCompletionMetadata(false, "Alice", "epoch-1")
	plan := mapFromOperationContext(metadata["operation_plan"])
	phases := mapSliceFromAny(plan["phases"])
	phases = append(phases, map[string]interface{}{
		"id": "phase-native", "step": "calculate local result", "status": operationPlanStepStatusPending,
		"expected_action": map[string]interface{}{"skill_id": skills.SkillCalculator, "tool_name": "calculate"},
	})
	plan["phases"] = mapsToInterfaceSlice(phases)
	bound := bindPendingGovernedInvocationToOperationPlan(metadata, map[string]interface{}{
		"frozen_invocation": frozen, "plan_phase_id": "phase-send",
	})
	if !approvedFrozenInvocationHasServerProjectedPlan(bound, frozen) {
		t.Fatal("server-projected runtime binding was not detected")
	}
	invocation := &skills.ToolInvocationResult{Trace: skills.SkillTrace{
		Status: "success",
		Result: map[string]interface{}{
			"integration_id": "wecom", "action_id": "wecom.message.send",
			"operation_status": "completed", "provider_success_confirmed": true,
		},
	}}

	completed, terminal := completeApprovedFrozenInvocationSuccess(bound, frozen, invocation)
	if terminal {
		t.Fatal("projected approval resume became terminal with native work remaining")
	}
	completedPhases := mapSliceFromAny(mapFromOperationContext(completed["operation_plan"])["phases"])
	if got := stringFromAny(completedPhases[0]["status"]); got != operationPlanStepStatusCompleted {
		t.Fatalf("projected phase status = %q, want completed", got)
	}
	if got := stringFromAny(completedPhases[1]["status"]); got != "in_progress" {
		t.Fatalf("native phase status = %q, want in_progress", got)
	}

	repeated, repeatedTerminal := completeApprovedFrozenInvocationSuccess(completed, frozen, invocation)
	if repeatedTerminal {
		t.Fatal("repeated projected approval resume broadly completed remaining native work")
	}
	repeatedPhases := mapSliceFromAny(mapFromOperationContext(repeated["operation_plan"])["phases"])
	if got := stringFromAny(repeatedPhases[1]["status"]); got != "in_progress" {
		t.Fatalf("native phase status after repeated resume = %q, want still in_progress", got)
	}
	if got := stringFromAny(mapFromOperationContext(repeated["operation_plan"])["status"]); got == operationPlanStatusCompleted {
		t.Fatalf("operation plan status = %q, native work was broadly completed", got)
	}
	summary := mapFromOperationContext(repeated["operation_result_summary"])
	if summary["status"] == operationPlanStatusCompleted || summary["pending_next_action"] == "none" {
		t.Fatalf("nonterminal provider summary claimed plan completion: %#v", summary)
	}
}

func TestApprovedProjectedGovernanceResumeFailsClosedWithoutExactBindingProof(t *testing.T) {
	tests := []struct {
		name        string
		mutatePlan  func(map[string]interface{})
		mutateEvent func(map[string]interface{}, *toolgovernance.FrozenInvocation)
	}{
		{
			name: "missing phase id",
			mutateEvent: func(event map[string]interface{}, _ *toolgovernance.FrozenInvocation) {
				delete(event, "plan_phase_id")
			},
		},
		{
			name: "missing ledger epoch",
			mutatePlan: func(metadata map[string]interface{}) {
				plan := mapFromOperationContext(metadata["operation_plan"])
				phases := mapSliceFromAny(plan["phases"])
				delete(phases[0], operationPlanServerProjectedEpochKey)
				plan["phases"] = mapsToInterfaceSlice(phases)
				metadata["operation_plan"] = plan
			},
		},
		{
			name: "missing binding fingerprint",
			mutatePlan: func(metadata map[string]interface{}) {
				plan := mapFromOperationContext(metadata["operation_plan"])
				phases := mapSliceFromAny(plan["phases"])
				expected := mapFromOperationContext(phases[0]["expected_action"])
				delete(expected, operationPlanServerProjectedBindingKey)
				phases[0]["expected_action"] = expected
				plan["phases"] = mapsToInterfaceSlice(phases)
				metadata["operation_plan"] = plan
			},
		},
		{
			name: "missing connection",
			mutateEvent: func(_ map[string]interface{}, frozen *toolgovernance.FrozenInvocation) {
				delete(frozen.Arguments, "connection_id")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metadata := projectedExternalActionCompletionMetadata(true, "Alice", "epoch-1")
			if test.mutatePlan != nil {
				test.mutatePlan(metadata)
			}
			frozen := projectedExternalActionCompletionFrozenInvocation("Alice")
			event := map[string]interface{}{"frozen_invocation": frozen, "plan_phase_id": "phase-send"}
			if test.mutateEvent != nil {
				test.mutateEvent(event, &frozen)
				event["frozen_invocation"] = frozen
			}
			bound := bindPendingGovernedInvocationToOperationPlan(metadata, event)
			invocation := &skills.ToolInvocationResult{Trace: skills.SkillTrace{
				Status: "success",
				Result: map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.message.send",
					"operation_status": "completed", "provider_success_confirmed": true,
				},
			}}
			completed, terminal := completeApprovedFrozenInvocationSuccess(bound, frozen, invocation)
			if terminal {
				t.Fatalf("incomplete projected binding proof reached terminal state: %#v", completed)
			}
			plan := mapFromOperationContext(completed["operation_plan"])
			phase := mapSliceFromAny(plan["phases"])[0]
			outcome := mapSliceFromAny(plan[operationPlanOutcomesKey])[0]
			if stringFromAny(phase["status"]) == operationPlanStepStatusCompleted ||
				stringFromAny(outcome["status"]) == operationPlanStepStatusCompleted ||
				len(mapSliceFromAny(plan[operationPlanEffectLedgerKey])) != 0 {
				t.Fatalf("incomplete proof changed projected completion state: plan=%#v", plan)
			}
			summary := mapFromOperationContext(completed["operation_result_summary"])
			if summary["status"] == operationPlanStatusCompleted || summary["pending_next_action"] == "none" {
				t.Fatalf("incomplete proof produced terminal summary: %#v", summary)
			}
		})
	}
}

func TestApprovedProjectedGovernanceResumeKeepsUnboundModelReconciliationPhase(t *testing.T) {
	frozen := projectedExternalActionCompletionFrozenInvocation("Alice")
	metadata := projectedExternalActionCompletionMetadata(false, "Alice", "epoch-1")
	plan := mapFromOperationContext(metadata["operation_plan"])
	phases := mapSliceFromAny(plan["phases"])
	phases = append(phases, map[string]interface{}{
		"id": "phase-model", "step": "reconcile remaining native work", "status": operationPlanStepStatusPending,
		"verification_mode": "model_reconciliation",
	})
	plan["phases"] = mapsToInterfaceSlice(phases)
	bound := bindPendingGovernedInvocationToOperationPlan(metadata, map[string]interface{}{
		"frozen_invocation": frozen, "plan_phase_id": "phase-send",
	})
	invocation := &skills.ToolInvocationResult{Trace: skills.SkillTrace{
		Status: "success", Result: map[string]interface{}{
			"integration_id": "wecom", "action_id": "wecom.message.send",
			"operation_status": "completed", "provider_success_confirmed": true,
		},
	}}
	completed, terminal := completeApprovedFrozenInvocationSuccess(bound, frozen, invocation)
	if terminal {
		t.Fatal("unbound model-reconciliation phase was broadly completed")
	}
	completedPhases := mapSliceFromAny(mapFromOperationContext(completed["operation_plan"])["phases"])
	if got := stringFromAny(completedPhases[1]["status"]); got != "in_progress" {
		t.Fatalf("model-reconciliation phase status = %q, want in_progress", got)
	}
}

func TestApprovedProjectedGovernanceUnstructuredRepeatStaysTerminalWithoutDuplicateEvidence(t *testing.T) {
	frozen := projectedExternalActionCompletionFrozenInvocation("Alice")
	metadata := projectedExternalActionCompletionMetadata(false, "Alice", "epoch-1")
	bound := bindPendingGovernedInvocationToOperationPlan(metadata, map[string]interface{}{
		"frozen_invocation": frozen, "plan_phase_id": "phase-send",
	})
	invocation := &skills.ToolInvocationResult{Trace: skills.SkillTrace{
		Status: "success", Result: map[string]interface{}{
			"integration_id": "wecom", "action_id": "wecom.message.send",
			"operation_status": "completed", "provider_success_confirmed": true,
		},
	}}
	completed, terminal := completeApprovedFrozenInvocationSuccess(bound, frozen, invocation)
	if !terminal {
		t.Fatal("first exact unstructured projected resume was not terminal")
	}
	firstPlan := mapFromOperationContext(completed["operation_plan"])
	firstAttempts := len(mapSliceFromAny(firstPlan[operationPlanActionAttemptsKey]))
	firstEffects := len(mapSliceFromAny(firstPlan[operationPlanEffectLedgerKey]))

	repeated, repeatedTerminal := completeApprovedFrozenInvocationSuccess(completed, frozen, invocation)
	if !repeatedTerminal {
		t.Fatal("repeated exact unstructured projected resume lost terminal state")
	}
	repeatedPlan := mapFromOperationContext(repeated["operation_plan"])
	phase := mapSliceFromAny(repeatedPlan["phases"])[0]
	if got := stringFromAny(phase["status"]); got != operationPlanStepStatusCompleted {
		t.Fatalf("phase status after repeat = %q, want completed", got)
	}
	if got := len(mapSliceFromAny(repeatedPlan[operationPlanActionAttemptsKey])); got != firstAttempts {
		t.Fatalf("attempt count after repeat = %d, want %d", got, firstAttempts)
	}
	if got := len(mapSliceFromAny(repeatedPlan[operationPlanEffectLedgerKey])); got != firstEffects {
		t.Fatalf("effect count after repeat = %d, want %d", got, firstEffects)
	}
	summary := mapFromOperationContext(repeated["operation_result_summary"])
	if summary["status"] != operationPlanStatusCompleted || summary["pending_next_action"] != "none" {
		t.Fatalf("terminal summary was downgraded after repeat: %#v", summary)
	}
}

func TestApprovedProjectedGovernanceNonSuccessProviderStatesPreservePlanExactly(t *testing.T) {
	for _, structured := range []bool{false, true} {
		for _, testCase := range []struct {
			name      string
			status    string
			confirmed interface{}
		}{
			{name: "failed_safe", status: "failed_safe", confirmed: true},
			{name: "partially_succeeded", status: "partially_succeeded", confirmed: true},
			{name: "outcome_unknown", status: "outcome_unknown", confirmed: true},
			{name: "executing", status: "executing", confirmed: true},
			{name: "missing_status", confirmed: true},
			{name: "unrecognized_status", status: "provider_says_maybe", confirmed: true},
			{name: "completed_but_unconfirmed", status: "completed", confirmed: false},
			{name: "completed_with_malformed_confirmation", status: "completed", confirmed: "false"},
		} {
			name := testCase.name + "/unstructured"
			if structured {
				name = testCase.name + "/structured"
			}
			t.Run(name, func(t *testing.T) {
				frozen := projectedExternalActionCompletionFrozenInvocation("Alice")
				metadata := projectedExternalActionCompletionMetadata(structured, "Alice", "epoch-1")
				bound := bindPendingGovernedInvocationToOperationPlan(metadata, map[string]interface{}{
					"frozen_invocation": frozen, "plan_phase_id": "phase-send",
				})
				before, err := json.Marshal(bound)
				if err != nil {
					t.Fatal(err)
				}
				invocation := &skills.ToolInvocationResult{Trace: skills.SkillTrace{
					Status: "success",
					Result: map[string]interface{}{
						"integration_id": "wecom", "action_id": "wecom.message.send",
						"operation_status": testCase.status,
						// A contradictory compacted flag must never override the
						// authoritative operation status.
						"provider_success_confirmed": testCase.confirmed,
					},
				}}
				if testCase.status == "" {
					delete(invocation.Trace.Result, "operation_status")
				}
				if approvedFrozenExternalActionProviderSuccess(frozen, invocation) {
					t.Fatalf("operation_status=%q confirmed=%v was accepted as provider success", testCase.status, testCase.confirmed)
				}
				after, terminal := completeApprovedFrozenInvocationSuccess(bound, frozen, invocation)
				if terminal {
					t.Fatalf("operation_status=%q confirmed=%v reached terminal state", testCase.status, testCase.confirmed)
				}
				encodedAfter, err := json.Marshal(after)
				if err != nil {
					t.Fatal(err)
				}
				if string(encodedAfter) != string(before) {
					t.Fatalf("operation_status=%q confirmed=%v mutated metadata\nbefore=%s\nafter=%s", testCase.status, testCase.confirmed, before, encodedAfter)
				}
				plan := mapFromOperationContext(after["operation_plan"])
				phase := mapSliceFromAny(plan["phases"])[0]
				if stringFromAny(phase["status"]) == operationPlanStepStatusCompleted ||
					len(mapSliceFromAny(plan[operationPlanActionAttemptsKey])) != 0 ||
					len(mapSliceFromAny(plan[operationPlanEffectLedgerKey])) != 0 {
					t.Fatalf("operation_status=%q confirmed=%v synthesized completion evidence: %#v", testCase.status, testCase.confirmed, plan)
				}
				if structured {
					outcome := mapSliceFromAny(plan[operationPlanOutcomesKey])[0]
					if stringFromAny(outcome["status"]) == operationPlanStepStatusCompleted {
						t.Fatalf("operation_status=%q confirmed=%v completed structured outcome: %#v", testCase.status, testCase.confirmed, outcome)
					}
				}
			})
		}
	}
}

func TestApprovedProjectedGovernanceStructuredRepeatStaysTerminalWithoutDuplicateEvidence(t *testing.T) {
	frozen := projectedExternalActionCompletionFrozenInvocation("Alice")
	metadata := projectedExternalActionCompletionMetadata(true, "Alice", "epoch-1")
	bound := bindPendingGovernedInvocationToOperationPlan(metadata, map[string]interface{}{
		"frozen_invocation": frozen, "plan_phase_id": "phase-send",
	})
	invocation := &skills.ToolInvocationResult{Trace: skills.SkillTrace{
		Status: "success", Result: map[string]interface{}{
			"integration_id": "wecom", "action_id": "wecom.message.send",
			"operation_status": "completed", "provider_success_confirmed": true,
		},
	}}
	completed, terminal := completeApprovedFrozenInvocationSuccess(bound, frozen, invocation)
	if !terminal {
		t.Fatal("first exact structured projected resume was not terminal")
	}
	firstPlan := mapFromOperationContext(completed["operation_plan"])
	firstAttempts := len(mapSliceFromAny(firstPlan[operationPlanActionAttemptsKey]))
	firstEffects := len(mapSliceFromAny(firstPlan[operationPlanEffectLedgerKey]))

	repeated, repeatedTerminal := completeApprovedFrozenInvocationSuccess(completed, frozen, invocation)
	if !repeatedTerminal {
		t.Fatal("repeated exact structured projected resume lost terminal state")
	}
	repeatedPlan := mapFromOperationContext(repeated["operation_plan"])
	if got := len(mapSliceFromAny(repeatedPlan[operationPlanActionAttemptsKey])); got != firstAttempts {
		t.Fatalf("attempt count after repeat = %d, want %d", got, firstAttempts)
	}
	if got := len(mapSliceFromAny(repeatedPlan[operationPlanEffectLedgerKey])); got != firstEffects {
		t.Fatalf("effect count after repeat = %d, want %d", got, firstEffects)
	}
	phase := mapSliceFromAny(repeatedPlan["phases"])[0]
	outcome := mapSliceFromAny(repeatedPlan[operationPlanOutcomesKey])[0]
	if stringFromAny(phase["status"]) != operationPlanStepStatusCompleted ||
		stringFromAny(outcome["status"]) != operationPlanStepStatusCompleted {
		t.Fatalf("structured completion was downgraded: phase=%#v outcome=%#v", phase, outcome)
	}
}

func TestApprovedOrdinaryGovernedSuccessDoesNotWriteExternalProviderSummary(t *testing.T) {
	frozen := toolgovernance.NewFrozenInvocation(toolgovernance.FrozenInvocationRequest{
		CorrelationID: "calculator-correlation", SkillID: skills.SkillCalculator, ToolName: "calculate",
		Arguments: map[string]interface{}{"operation": "add", "left": 1, "right": 2}, Now: time.Now(),
	})
	metadata := map[string]interface{}{"operation_plan": map[string]interface{}{
		"status": operationPlanStatusRunning,
		"phases": []interface{}{map[string]interface{}{
			"id": "phase-calculate", "step": "calculate", "status": "in_progress",
			"expected_action": map[string]interface{}{"skill_id": skills.SkillCalculator, "tool_name": "calculate"},
		}},
	}}
	bound := bindPendingGovernedInvocationToOperationPlan(metadata, map[string]interface{}{
		"frozen_invocation": frozen, "plan_phase_id": "phase-calculate",
	})
	completed, terminal := completeApprovedFrozenInvocationSuccess(bound, frozen, &skills.ToolInvocationResult{Trace: skills.SkillTrace{
		Status: "success", Result: map[string]interface{}{"value": 3},
	}})
	if !terminal {
		t.Fatal("ordinary exact governed invocation did not complete its only phase")
	}
	phase := mapSliceFromAny(mapFromOperationContext(completed["operation_plan"])["phases"])[0]
	if stringFromAny(phase["status"]) != operationPlanStepStatusCompleted {
		t.Fatalf("ordinary governed phase = %#v", phase)
	}
	if summary := mapFromOperationContext(completed["operation_result_summary"]); summary["provider_success_confirmed"] != nil || summary["operation_status"] != nil {
		t.Fatalf("ordinary governed invocation received external provider summary: %#v", summary)
	}
}

func TestApprovedExternalActionMultipleUnboundPhasesRemainOpen(t *testing.T) {
	frozen := toolgovernance.FrozenInvocation{
		SkillID: skills.SkillExternalApps, ToolName: "execute_action",
		Arguments: map[string]interface{}{"action_id": "feishu.calendar.event.create"},
	}
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"phases": []interface{}{
				map[string]interface{}{"id": "phase-1", "status": "in_progress"},
				map[string]interface{}{"id": "phase-2", "status": operationPlanStepStatusPending},
			},
		},
	}
	if !approvedExternalActionHasExplicitRemainingWork(metadata, frozen) {
		t.Fatal("approvedExternalActionHasExplicitRemainingWork() = false")
	}
}

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
