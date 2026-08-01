package skillloop

import (
	"strings"
	"testing"
)

func TestTerminalStateGuardAcceptsMainModelAnswerWithoutPlanOrEvidenceJudgment(t *testing.T) {
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "running",
			"phases": []interface{}{map[string]interface{}{
				"id": "phase-1", "status": "pending",
			}},
		},
		"turn_state": map[string]interface{}{
			"open_items": []interface{}{map[string]interface{}{
				"status": "failed", "reason": "stale recovered tool error",
			}},
		},
	}

	decision := terminalStateGuardEvaluate(evidence, "任务已经处理完成。")
	if decision.Path != terminalStateGuardAccepted {
		t.Fatalf("terminalStateGuardEvaluate().Path = %q, want %q; decision=%#v", decision.Path, terminalStateGuardAccepted, decision)
	}
}

func TestTerminalStateGuardBlocksEmptyAnswer(t *testing.T) {
	decision := terminalStateGuardEvaluate(nil, "  ")
	if decision.Path != terminalStateGuardBlocked {
		t.Fatalf("terminalStateGuardEvaluate().Path = %q, want %q", decision.Path, terminalStateGuardBlocked)
	}
	if len(decision.Blockers) != 1 || decision.Blockers[0] != "missing_protocol:final_answer" {
		t.Fatalf("terminalStateGuardEvaluate().Blockers = %#v", decision.Blockers)
	}
}

func TestTerminalStateGuardBlocksActiveGovernanceOnly(t *testing.T) {
	evidence := map[string]interface{}{
		"tool_governance": []interface{}{
			map[string]interface{}{
				"correlation_id": "approval-1",
				"status":         "needs_approval",
			},
		},
	}
	if terminalStateGuardCanStream(evidence) {
		t.Fatal("terminalStateGuardCanStream() = true with active approval")
	}

	evidence["tool_governance"] = []interface{}{
		map[string]interface{}{
			"correlation_id": "approval-1",
			"status":         "needs_approval",
		},
		map[string]interface{}{
			"correlation_id":  "approval-1",
			"status":          "completed",
			"approval_status": "approved",
		},
	}
	decision := terminalStateGuardEvaluate(evidence, "已完成。")
	if decision.Path != terminalStateGuardAccepted {
		t.Fatalf("resolved approval still blocked terminal answer: %#v", decision)
	}
}

func TestTerminalStateGuardBlocksActiveClientActionOnly(t *testing.T) {
	evidence := map[string]interface{}{
		"client_actions": []interface{}{
			map[string]interface{}{"action_id": "route-1", "status": "waiting"},
			map[string]interface{}{"action_id": "route-1", "status": "succeeded"},
			map[string]interface{}{"action_id": "route-2", "status": "waiting_client_action"},
		},
	}
	decision := terminalStateGuardEvaluate(evidence, "已完成。")
	if decision.Path != terminalStateGuardBlocked {
		t.Fatalf("active client action did not block terminal answer: %#v", decision)
	}
	if len(decision.Blockers) != 1 || decision.Blockers[0] != "pending_protocol:client_action" {
		t.Fatalf("terminalStateGuardEvaluate().Blockers = %#v", decision.Blockers)
	}
}

func TestTerminalStateGuardReplacesUnsupportedExternalActionSuccessClaim(t *testing.T) {
	evidence := map[string]interface{}{
		"user_request": "给杨志航发送飞书消息",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind": "tool_call", "skill_id": "external-apps", "tool_name": "execute_action",
				"status": "error", "error": "integration audit failed",
			},
		},
	}
	decision := terminalStateGuardEvaluate(evidence, "已经成功发送给杨志航。")
	if decision.Path != terminalStateGuardAccepted {
		t.Fatalf("terminalStateGuardEvaluate().Path = %q, want accepted replacement", decision.Path)
	}
	if decision.FinalAnswer != "外部操作未完成。系统没有取得服务商的成功回执，因此不能视为已发送或已完成。请先检查连接状态和执行记录，确认后再重试。" {
		t.Fatalf("terminalStateGuardEvaluate().FinalAnswer = %q", decision.FinalAnswer)
	}
}

func TestTerminalStateGuardAllowsExternalActionAfterLaterSuccess(t *testing.T) {
	evidence := map[string]interface{}{
		"user_request": "send the message",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind": "tool_call", "skill_id": "external-apps", "tool_name": "execute_action", "status": "error",
			},
			map[string]interface{}{
				"kind": "tool_call", "skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
				"result": map[string]interface{}{"status": "completed"},
			},
		},
	}
	decision := terminalStateGuardEvaluate(evidence, "The message was sent.")
	if decision.FinalAnswer != "The message was sent." {
		t.Fatalf("terminalStateGuardEvaluate().FinalAnswer = %q, want successful candidate", decision.FinalAnswer)
	}
}

func TestTerminalStateGuardTreatsReceiptFailureAsFailureEvenWithProviderPayload(t *testing.T) {
	evidence := map[string]interface{}{
		"user_request": "发送消息",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "execute_action", "status": "error",
				"result": map[string]interface{}{"status": "completed"},
				"error":  "successful provider response was not durably receipted",
			},
		},
	}
	decision := terminalStateGuardEvaluate(evidence, "发送成功。")
	if !strings.Contains(decision.FinalAnswer, "外部操作未完成") {
		t.Fatalf("terminalStateGuardEvaluate().FinalAnswer = %q", decision.FinalAnswer)
	}
}
