package skillloop

import "testing"

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
