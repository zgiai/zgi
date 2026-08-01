package service

import (
	"context"
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
