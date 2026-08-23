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
				"result": map[string]interface{}{"operation_status": "completed"},
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

func TestTerminalStateGuardBlocksExternalExecutionIntentAfterGuideWithoutExecution(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "列出钉钉角色",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind": "tool_call", "skill_id": "external-apps", "tool_name": "search_actions", "status": "success",
			},
			map[string]interface{}{
				"kind": "tool_call", "skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{"integration_id": "dingtalk", "action_id": "dingtalk.role.list", "availability": "ready"},
			},
		},
	}

	decision := terminalStateGuardEvaluate(evidence, "当前环境没有找到钉钉角色列表的业务函数。")
	if decision.Path != terminalStateGuardBlocked {
		t.Fatalf("guide without execution was not blocked: %#v", decision)
	}
	if len(decision.Blockers) != 1 || decision.Blockers[0] != "missing_protocol:external_action_execution" {
		t.Fatalf("blockers = %#v", decision.Blockers)
	}
}

func TestTerminalStateGuardAllowsExternalActionClarificationAfterGuide(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "查询钉钉通知状态",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind": "tool_call", "skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "dingtalk", "action_id": "dingtalk.message.delivery.get", "availability": "ready",
					"required_arguments": []interface{}{map[string]interface{}{"name": "message_ref", "type": "string"}},
				},
			},
		},
	}

	decision := terminalStateGuardEvaluate(evidence, "请提供要查询的消息引用，以便我读取通知状态。")
	if decision.Path != terminalStateGuardAccepted {
		t.Fatalf("required-input clarification was blocked: %#v", decision)
	}
}

func TestTerminalStateGuardRejectsClarificationForArgumentFreeExternalAction(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "list DingTalk roles",
		"skill_invocations": []interface{}{map[string]interface{}{
			"skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
			"result": map[string]interface{}{
				"integration_id": "dingtalk", "action_id": "dingtalk.role.list", "availability": "ready",
				"required_arguments": []interface{}{},
			},
		}},
	}

	decision := terminalStateGuardEvaluate(evidence, "Please provide more information before I list the roles.")
	if decision.Path != terminalStateGuardBlocked {
		t.Fatalf("argument-free action escaped through a clarification: %#v", decision)
	}
}

func TestTerminalStateGuardAllowsCapabilityDiscoveryWithoutExecution(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "钉钉支持哪些功能？",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind": "tool_call", "skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
			},
		},
	}

	decision := terminalStateGuardEvaluate(evidence, "钉钉支持成员、部门、角色和通知操作。")
	if decision.Path != terminalStateGuardAccepted {
		t.Fatalf("capability discovery was blocked: %#v", decision)
	}
}

func TestTerminalStateGuardDoesNotTreatDifferentExternalActionAsCompletion(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "list DingTalk roles",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{"integration_id": "dingtalk", "action_id": "dingtalk.role.list", "availability": "ready"},
			},
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
				"arguments": map[string]interface{}{"integration_id": "dingtalk", "action_id": "dingtalk.department.list"},
			},
		},
	}

	decision := terminalStateGuardEvaluate(evidence, "The requested function is unavailable.")
	if decision.Path != terminalStateGuardBlocked {
		t.Fatalf("different action execution satisfied the guide: %#v", decision)
	}
}

func TestTerminalStateGuardAcceptsMatchingExternalActionAfterGuide(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "list DingTalk roles",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{"integration_id": "dingtalk", "action_id": "dingtalk.role.list", "availability": "ready"},
			},
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
				"arguments": map[string]interface{}{"integration_id": "dingtalk", "action_id": "dingtalk.role.list"},
			},
		},
	}

	decision := terminalStateGuardEvaluate(evidence, "No roles were returned.")
	if decision.Path != terminalStateGuardAccepted {
		t.Fatalf("matching action execution was blocked: %#v", decision)
	}
}

func TestTerminalStateGuardBlocksNaturalExecutionRequestAfterGuideWithoutExecution(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "帮我看下钉钉里的角色列表",
		"skill_invocations": []interface{}{map[string]interface{}{
			"skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
			"result": map[string]interface{}{
				"integration_id": "dingtalk", "action_id": "dingtalk.role.list", "availability": "ready",
			},
		}},
	}

	decision := terminalStateGuardEvaluate(evidence, "当前没有可用的角色列表能力。")
	if decision.Path != terminalStateGuardBlocked {
		t.Fatalf("natural execution request escaped without execution: %#v", decision)
	}
}

func TestTerminalStateGuardAllowsActionGuideQuestionWithoutExecution(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "钉钉列出角色这个功能需要哪些参数？",
		"skill_invocations": []interface{}{map[string]interface{}{
			"skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
			"result": map[string]interface{}{
				"integration_id": "dingtalk", "action_id": "dingtalk.role.list", "availability": "ready",
			},
		}},
	}

	decision := terminalStateGuardEvaluate(evidence, "这个操作不需要业务参数，可以直接执行。")
	if decision.Path != terminalStateGuardAccepted {
		t.Fatalf("action guide question was incorrectly treated as execution intent: %#v", decision)
	}
}

func TestTerminalStateGuardDoesNotRepeatSuccessfulRedactedExternalWrite(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "给杨志航发送一条钉钉通知",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "dingtalk", "action_id": "dingtalk.message.send", "availability": "ready",
				},
			},
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
				"result_summary": map[string]interface{}{"operation_status": "completed"},
			},
		},
	}

	decision := terminalStateGuardEvaluate(evidence, "通知已发送。")
	if decision.Path != terminalStateGuardBlocked || decision.PendingExternalAction == nil {
		t.Fatalf("redacted execution was incorrectly attributed to an identified action: %#v", decision)
	}
	if decision.PendingExternalAction.RetryAllowed ||
		terminalStateGuardCanRetryPendingExternalAction(decision.PendingExternalAction, map[string]int{}) {
		t.Fatalf("redacted write could be repeated: %#v", decision.PendingExternalAction)
	}
}

func TestTerminalStateGuardDoesNotLetStaleNestedGuideOverrideCompletedWrite(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "给杨志航发送一条钉钉通知",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "dingtalk", "action_id": "dingtalk.message.send", "availability": "ready",
				},
			},
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
				"arguments": map[string]interface{}{
					"integration_id": "dingtalk", "action_id": "dingtalk.message.send",
				},
			},
		},
		"execution_ledger": map[string]interface{}{
			"skill_invocations": []interface{}{map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "dingtalk", "action_id": "dingtalk.message.send", "availability": "ready",
				},
			}},
		},
	}

	decision := terminalStateGuardEvaluate(evidence, "通知已发送。")
	if decision.Path != terminalStateGuardAccepted {
		t.Fatalf("stale nested guide caused a completed write to repeat: %#v", decision)
	}
}

func TestTerminalStateGuardDoesNotRepeatWriteAfterRedundantLaterGuide(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "给杨志航发送一条钉钉通知",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "dingtalk", "action_id": "dingtalk.message.send", "availability": "ready",
				},
			},
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
				"arguments": map[string]interface{}{
					"integration_id": "dingtalk", "action_id": "dingtalk.message.send",
				},
			},
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "dingtalk", "action_id": "dingtalk.message.send", "availability": "ready",
				},
			},
		},
	}

	decision := terminalStateGuardEvaluate(evidence, "通知已发送。")
	if decision.Path != terminalStateGuardAccepted {
		t.Fatalf("redundant guide caused an already completed write to repeat: %#v", decision)
	}
}

func TestTerminalStateGuardTracksLatestPendingActionAfterDifferentActionSucceeded(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "查询企业微信成员并给他发送消息",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"invocation_id": "guide-search", "skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.contact.search", "effect": "read", "can_execute": true,
				},
			},
			map[string]interface{}{
				"invocation_id": "execute-search", "skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.contact.search", "result_count": 1,
				},
			},
			map[string]interface{}{
				"invocation_id": "guide-send", "skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.message.send", "effect": "external_send", "can_execute": true,
				},
			},
		},
	}

	decision := terminalStateGuardEvaluate(evidence, "已经处理完成。")
	if decision.Path != terminalStateGuardBlocked || decision.PendingExternalAction == nil {
		t.Fatalf("latest pending action was not blocked independently: %#v", decision)
	}
	pending := decision.PendingExternalAction
	if pending.IntegrationID != "wecom" || pending.ActionID != "wecom.message.send" || pending.RetryKey != "guide:guide-send" {
		t.Fatalf("pending action = %#v, want WeCom send guide", pending)
	}
}

func TestTerminalStateGuardDoesNotLetEarlierUnkeyedExecutionSatisfyLaterActionGuide(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "查询企业微信成员并给他发送消息",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.contact.search", "can_execute": true,
				},
			},
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
				"result_summary": map[string]interface{}{"operation_status": "completed"},
			},
			map[string]interface{}{
				"invocation_id": "guide-send", "skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.message.send", "effect": "external_send", "can_execute": true,
				},
			},
		},
	}

	decision := terminalStateGuardEvaluate(evidence, "已经处理完成。")
	if decision.Path != terminalStateGuardBlocked || decision.PendingExternalAction == nil ||
		decision.PendingExternalAction.ActionID != "wecom.message.send" {
		t.Fatalf("earlier unkeyed execution suppressed later action: %#v", decision)
	}
}

func TestTerminalStateGuardDoesNotLetCompletedLaterActionHideEarlierPendingAction(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "查询企业微信成员并发送消息",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"invocation_id": "guide-search", "skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.contact.search", "can_execute": true,
				},
			},
			map[string]interface{}{
				"invocation_id": "guide-send", "skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.message.send", "can_execute": true,
					"preparation_hints": []interface{}{map[string]interface{}{"action_id": "wecom.contact.search"}},
				},
			},
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
				"arguments": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.message.send",
				},
			},
		},
	}

	decision := terminalStateGuardEvaluate(evidence, "已经完成。")
	if decision.Path != terminalStateGuardBlocked || decision.PendingExternalAction == nil ||
		decision.PendingExternalAction.ActionID != "wecom.contact.search" {
		t.Fatalf("later completed action hid an earlier pending action: %#v", decision)
	}
}

func TestTerminalStateGuardDistinguishesRepeatedActionByPlanPhase(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "先给甲发送消息，再给乙发送消息",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"invocation_id": "guide-send-a", "skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.message.send", "plan_phase_id": "send-a", "can_execute": true,
				},
			},
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
				"arguments": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.message.send", "plan_phase_id": "send-a",
				},
			},
			map[string]interface{}{
				"invocation_id": "guide-send-b", "skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.message.send", "plan_phase_id": "send-b", "can_execute": true,
				},
			},
		},
	}

	decision := terminalStateGuardEvaluate(evidence, "两条消息都已发送。")
	if decision.Path != terminalStateGuardBlocked || decision.PendingExternalAction == nil ||
		decision.PendingExternalAction.PlanPhaseID != "send-b" ||
		decision.PendingExternalAction.RetryKey != "guide:guide-send-b" {
		t.Fatalf("second phase was not tracked independently: %#v", decision)
	}
}

func TestTerminalStateGuardProjectedPlanDetectsSecondActionWithoutGuide(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "查询企业微信成员并给他发送消息",
		runtimeStateNativeExternalActionProjectionsKey: []interface{}{
			map[string]interface{}{"tool_name": "wecom_search_contact", "integration_id": "wecom", "action_id": "wecom.contact.search"},
			map[string]interface{}{"tool_name": "wecom_send_message", "integration_id": "wecom", "action_id": "wecom.message.send"},
		},
		"operation_plan": map[string]interface{}{"phases": []interface{}{
			map[string]interface{}{
				"id": "phase-search", "step": "search", "status": "in_progress", "required": true,
				"expected_action": map[string]interface{}{
					"skill_id": "external-apps", "tool_name": "execute_action", "projected_tool_name": "wecom_search_contact",
					planExpectedActionServerProjectionKey: "wecom_search_contact",
					"target":                              map[string]interface{}{"integration_id": "wecom", "action_id": "wecom.contact.search"},
				},
			},
			map[string]interface{}{
				"id": "phase-send", "step": "send", "status": "pending", "required": true,
				"expected_action": map[string]interface{}{
					"skill_id": "external-apps", "tool_name": "execute_action", "projected_tool_name": "wecom_send_message",
					planExpectedActionServerProjectionKey: "wecom_send_message",
					"target":                              map[string]interface{}{"integration_id": "wecom", "action_id": "wecom.message.send"},
				},
			},
		}},
		"skill_invocations": []interface{}{map[string]interface{}{
			"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
			"arguments": map[string]interface{}{
				"integration_id": "wecom", "action_id": "wecom.contact.search", "plan_phase_id": "phase-search",
			},
			"result": map[string]interface{}{"operation_status": "completed"},
		}},
	}

	decision := terminalStateGuardEvaluate(evidence, "查询和发送都已完成。")
	if decision.Path != terminalStateGuardBlocked || decision.PendingExternalAction == nil ||
		decision.PendingExternalAction.ActionID != "wecom.message.send" ||
		decision.PendingExternalAction.PlanPhaseID != "phase-send" ||
		!decision.PendingExternalAction.RetryAllowed {
		t.Fatalf("projected plan did not expose omitted send without guide: %#v", decision)
	}
}

func TestTerminalStateGuardProjectedPlanDistinguishesSameActionTargets(t *testing.T) {
	evidence := projectedExternalActionPlanTestState([]interface{}{
		projectedExternalActionPlanTestPhase("phase-alice", "in_progress", "recipient_ref", "alice"),
		projectedExternalActionPlanTestPhase("phase-bob", "pending", "recipient_ref", "bob"),
	})
	evidence["latest_user_request"] = "分别给 Alice 和 Bob 发送消息"
	evidence["skill_invocations"] = []interface{}{map[string]interface{}{
		"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
		"arguments": map[string]interface{}{
			"integration_id": "wecom", "action_id": "wecom.message.send", "plan_phase_id": "phase-alice",
		},
		"result": map[string]interface{}{"operation_status": "completed"},
	}}

	decision := terminalStateGuardEvaluate(evidence, "两条消息都已发送。")
	if decision.Path != terminalStateGuardBlocked || decision.PendingExternalAction == nil ||
		decision.PendingExternalAction.PlanPhaseID != "phase-bob" {
		t.Fatalf("same projected Action targets were not tracked independently: %#v", decision)
	}
}

func TestTerminalStateGuardProjectedExactAttemptRequiresConfirmedProviderStatus(t *testing.T) {
	tests := []struct {
		name      string
		status    string
		confirmed interface{}
	}{
		{name: "missing status"},
		{name: "unrecognized status", status: "provider_says_maybe"},
		{name: "completed but explicitly unconfirmed", status: "completed", confirmed: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evidence := projectedExternalActionPlanTestState([]interface{}{
				projectedExternalActionPlanTestPhase("phase-alice", "in_progress", "recipient_ref", "alice"),
			})
			evidence["latest_user_request"] = "给 Alice 发送企业微信消息"
			result := map[string]interface{}{
				"integration_id": "wecom", "action_id": "wecom.message.send",
			}
			if test.status != "" {
				result["operation_status"] = test.status
			}
			if test.confirmed != nil {
				result["provider_success_confirmed"] = test.confirmed
			}
			evidence["skill_invocations"] = []interface{}{map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
				"arguments": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.message.send", "plan_phase_id": "phase-alice",
					"operation_plan_target": map[string]interface{}{"recipient_ref": "alice"},
				},
				"result": result,
			}}

			decision := terminalStateGuardEvaluate(evidence, "消息已发送。")
			if decision.Path != terminalStateGuardAccepted || decision.FinalAnswer == "消息已发送。" ||
				!strings.Contains(decision.FinalAnswer, "不能视为已发送") {
				t.Fatalf("unconfirmed exact projected attempt escaped terminal guard: %#v", decision)
			}
			if terminalStateGuardCanStream(evidence) {
				t.Fatal("unconfirmed exact projected attempt allowed success streaming")
			}
		})
	}
}

func TestTerminalStateGuardProjectedPlanDoesNotTrustSkippedRequiredAction(t *testing.T) {
	skipped := projectedExternalActionPlanTestPhase("phase-bob", "skipped", "recipient_ref", "bob")
	evidence := projectedExternalActionPlanTestState([]interface{}{
		projectedExternalActionPlanTestPhase("phase-alice", "completed", "recipient_ref", "alice"),
		skipped,
	})
	evidence["latest_user_request"] = "分别给 Alice 和 Bob 发送消息"
	evidence["skill_invocations"] = []interface{}{map[string]interface{}{
		"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
		"arguments": map[string]interface{}{
			"integration_id": "wecom", "action_id": "wecom.message.send", "plan_phase_id": "phase-alice",
		},
		"result": map[string]interface{}{"operation_status": "completed"},
	}}

	decision := terminalStateGuardEvaluate(evidence, "两条消息都已发送。")
	if decision.Path != terminalStateGuardBlocked || decision.PendingExternalAction == nil ||
		decision.PendingExternalAction.PlanPhaseID != "phase-bob" || decision.PendingExternalAction.RetryAllowed {
		t.Fatalf("skipped required projected Action was trusted or replay-enabled: %#v", decision)
	}
}

func TestTerminalStateGuardOptionalProjectedPhaseIsNotAutoRetried(t *testing.T) {
	phase := projectedExternalActionPlanTestPhase("phase-optional", "pending", "recipient_ref", "alice")
	phase["outcome_id"] = "outcome-optional"
	delete(phase, "required")
	evidence := projectedExternalActionPlanTestState([]interface{}{phase})
	evidence[runtimeStateNativeExternalActionProjectionsKey].([]interface{})[0].(map[string]interface{})["intent_matched"] = true
	evidence["operation_plan"].(map[string]interface{})["outcomes"] = []interface{}{map[string]interface{}{
		"id": "outcome-optional", "required": false, "verification_mode": "runtime_effects",
	}}
	evidence["latest_user_request"] = "可选地给 Alice 发送消息"

	if instances := terminalStateGuardProjectedPlanInstances(evidence); len(instances) != 0 {
		t.Fatalf("optional projected phase created required retry instances: %#v", instances)
	}
	decision := terminalStateGuardEvaluate(evidence, "未执行可选发送操作。")
	if decision.Path != terminalStateGuardAccepted {
		t.Fatalf("optional projected phase blocked a truthful final answer: %#v", decision)
	}
}

func TestTerminalStateGuardBlocksMissingProjectedLedgerBeforeSuccessClaim(t *testing.T) {
	evidence := projectedExternalActionPlanTestState([]interface{}{map[string]interface{}{
		"id": "phase-send", "outcome_id": "outcome-send", "step": "Send message", "status": "in_progress",
	}})
	evidence[runtimeStateNativeExternalActionCandidatesKey] = []interface{}{map[string]interface{}{
		"integration_id": "wecom", "action_id": "wecom.message.send",
		"binding_fingerprint": "binding-wecom-message-send", "intent_matched": true,
		"intent_group": "message.send", "intent_tokens": []interface{}{"send", "message", "发送", "消息"},
		"effect": "external_send", "target_argument_paths": []interface{}{"recipient_ref"},
	}}
	evidence[runtimeStateNativeExternalActionProjectionsKey].([]interface{})[0].(map[string]interface{})["intent_matched"] = true
	evidence["operation_plan"].(map[string]interface{})["outcomes"] = []interface{}{map[string]interface{}{
		"id": "outcome-send", "required": true, "verification_mode": "runtime_effects",
	}}
	evidence["latest_user_request"] = "发送消息"

	decision := terminalStateGuardEvaluate(evidence, "消息已发送。")
	if decision.Path != terminalStateGuardBlocked || !terminalStateGuardRequiresProjectedPlanLedgerRetry(decision) {
		t.Fatalf("missing projected ledger success claim was accepted: %#v", decision)
	}
	if terminalStateGuardCanStream(evidence) {
		t.Fatal("missing projected ledger allowed provisional success streaming")
	}
}

func TestTerminalStateGuardBlocksIntentMatchedProjectionWithoutAnyPlanLedger(t *testing.T) {
	evidence := projectedExternalActionPlanTestState(nil)
	evidence[runtimeStateNativeExternalActionIntentMatchedKey] = true
	evidence[runtimeStateNativeExternalActionCandidatesKey] = []interface{}{map[string]interface{}{
		"integration_id": "wecom", "action_id": "wecom.message.send",
		"binding_fingerprint": "binding-wecom-message-send", "intent_matched": true,
		"intent_group": "message.send", "effect": "external_send",
	}}
	evidence["latest_user_request"] = "给 Alice 发送企业微信消息"

	decision := terminalStateGuardEvaluate(evidence, "消息已发送。")
	if decision.Path != terminalStateGuardBlocked || !terminalStateGuardRequiresProjectedPlanLedgerRetry(decision) {
		t.Fatalf("intent-matched Action without any plan ledger was accepted: %#v", decision)
	}
	if terminalStateGuardCanStream(evidence) {
		t.Fatal("intent-matched Action without any plan ledger allowed provisional streaming")
	}
}

func TestTerminalStateGuardAllowsCapabilityDiscoveryWithoutMatchedMutationPlanLedger(t *testing.T) {
	tests := []struct {
		name          string
		request       string
		intentMatched bool
		effect        string
	}{
		{name: "unmatched mutation candidate", request: "企业微信支持哪些功能？", effect: "external_send"},
		{name: "intent-matched read candidate", request: "请介绍企业微信可用操作", intentMatched: true, effect: "read"},
		{name: "english read discovery", request: "How to use the available actions?", intentMatched: true, effect: "read"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evidence := projectedExternalActionPlanTestState(nil)
			if tt.intentMatched {
				evidence[runtimeStateNativeExternalActionIntentMatchedKey] = true
			}
			evidence[runtimeStateNativeExternalActionCandidatesKey] = []interface{}{map[string]interface{}{
				"integration_id": "wecom", "action_id": "wecom.message.send",
				"binding_fingerprint": "binding-wecom-message-send", "intent_matched": tt.intentMatched,
				"intent_group": "message.send", "effect": tt.effect,
			}}
			evidence["latest_user_request"] = tt.request

			decision := terminalStateGuardEvaluate(evidence, "当前支持发送消息。")
			if decision.Path != terminalStateGuardAccepted {
				t.Fatalf("capability discovery was blocked by the empty-ledger guard: %#v", decision)
			}
			if !terminalStateGuardCanStream(evidence) {
				t.Fatal("capability discovery without a plan ledger unexpectedly blocked streaming")
			}
		})
	}
}

func TestTerminalStateGuardDoesNotTreatMixedDiscoveryAndExecutionAsDiscoveryOnly(t *testing.T) {
	for _, request := range []string{
		"告诉我企业微信支持哪些功能，然后给 Alice 发送消息",
		"企业微信支持哪些功能？给 Alice 发送消息",
		"企业微信支持哪些功能？之后给 Alice 发送消息",
		"企业微信支持哪些功能？最后给 Alice 发送消息",
		"企业微信支持哪些功能？另外给 Alice 发送消息",
		"Tell me which actions are available and send Alice a message",
		"Which actions are available? Send Alice a message",
		"Which actions are available? Finally send Alice a message",
		"Tell me which actions are available? Message Alice hello.",
		"企业微信支持哪些功能？给 Alice 消息：你好。",
		"企业微信支持哪些功能；Alice 消息“你好”",
	} {
		t.Run(request, func(t *testing.T) {
			evidence := projectedExternalActionPlanTestState(nil)
			evidence[runtimeStateNativeExternalActionIntentMatchedKey] = true
			evidence[runtimeStateNativeExternalActionCandidatesKey] = []interface{}{map[string]interface{}{
				"integration_id": "wecom", "action_id": "wecom.message.send",
				"binding_fingerprint": "binding-wecom-message-send", "intent_matched": true,
				"intent_group": "message.send", "effect": "external_send",
			}}
			evidence["latest_user_request"] = request

			decision := terminalStateGuardEvaluate(evidence, "消息已发送。")
			if decision.Path != terminalStateGuardBlocked || !terminalStateGuardRequiresProjectedPlanLedgerRetry(decision) {
				t.Fatalf("mixed discovery/execution request bypassed the plan ledger: %#v", decision)
			}
			if terminalStateGuardCanStream(evidence) {
				t.Fatal("mixed discovery/execution request allowed provisional success streaming")
			}
		})
	}
}

func TestTerminalStateGuardAllowsServerFinalAnswerOnlyOutcomeWithoutProjectedLedger(t *testing.T) {
	evidence := projectedExternalActionPlanTestState([]interface{}{map[string]interface{}{
		"id": "phase-explain", "outcome_id": "outcome-explain", "step": "Explain", "status": "in_progress",
	}})
	evidence[runtimeStateNativeExternalActionProjectionsKey].([]interface{})[0].(map[string]interface{})["intent_matched"] = true
	evidence["operation_plan"].(map[string]interface{})["outcomes"] = []interface{}{map[string]interface{}{
		"id": "outcome-explain", "required": true, "verification_mode": "final_answer",
	}}
	evidence["latest_user_request"] = "说明支持的功能"

	decision := terminalStateGuardEvaluate(evidence, "当前支持发送消息。")
	if decision.Path != terminalStateGuardAccepted {
		t.Fatalf("server final-answer-only outcome was blocked: %#v", decision)
	}
}

func TestTerminalStateGuardDoesNotRequireExternalLedgerForUnmatchedInternalRuntimeOutcome(t *testing.T) {
	evidence := projectedExternalActionPlanTestState([]interface{}{map[string]interface{}{
		"id": "phase-file", "outcome_id": "outcome-file", "step": "Generate file", "status": "in_progress",
	}})
	evidence["operation_plan"].(map[string]interface{})["outcomes"] = []interface{}{map[string]interface{}{
		"id": "outcome-file", "required": true, "verification_mode": "runtime_effects",
	}}
	evidence["latest_user_request"] = "生成本地报告文件"

	decision := terminalStateGuardEvaluate(evidence, "报告文件已生成。")
	if decision.Path != terminalStateGuardAccepted {
		t.Fatalf("unmatched external projections blocked an internal runtime outcome: %#v", decision)
	}
}

func TestTerminalStateGuardRequiresExternalLedgerFromServerOutcomeCapabilityWithoutLexicalIntent(t *testing.T) {
	evidence := projectedExternalActionPlanTestState([]interface{}{
		map[string]interface{}{
			"id": "phase-send", "outcome_id": "outcome-send", "step": "Continue operation", "status": "in_progress",
		},
		map[string]interface{}{
			"id": "phase-file", "outcome_id": "outcome-file", "step": "Write local file", "status": "pending",
		},
	})
	evidence[runtimeStateNativeExternalActionCandidatesKey] = []interface{}{map[string]interface{}{
		"integration_id": "wecom", "action_id": "wecom.message.send",
		"binding_fingerprint": "binding-wecom-message-send", "intent_matched": false,
		"intent_group": "message.send", "effect": "external_send",
		"target_argument_paths": []interface{}{"recipient_ref"},
	}}
	evidence["operation_plan"].(map[string]interface{})["outcomes"] = []interface{}{
		map[string]interface{}{
			"id": "outcome-send", "required": true, "verification_mode": "runtime_effects",
			"capabilities": []interface{}{"external-apps"},
		},
		map[string]interface{}{
			"id": "outcome-file", "required": true, "verification_mode": "runtime_effects",
			"capabilities": []interface{}{"managed_file"},
		},
	}
	evidence["latest_user_request"] = "继续"

	required := projectedExternalActionRequiredPhaseIDs(evidence)
	if _, ok := required["phase-send"]; !ok {
		t.Fatalf("server external capability phase is not required: %#v", required)
	}
	if _, ok := required["phase-file"]; ok {
		t.Fatalf("native managed-file phase was incorrectly external-bound: %#v", required)
	}
	decision := terminalStateGuardEvaluate(evidence, "操作已完成。")
	if decision.Path != terminalStateGuardBlocked || !terminalStateGuardRequiresProjectedPlanLedgerRetry(decision) {
		t.Fatalf("server external capability without ledger was accepted: %#v", decision)
	}
}

func TestTerminalStateGuardUnkeyedExecutionCannotCompleteEitherIdentifiedGuide(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "查询企业微信成员并发送消息",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.contact.search", "can_execute": true,
				},
			},
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.message.send", "can_execute": true,
				},
			},
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
				"result_summary": map[string]interface{}{"operation_status": "completed"},
			},
		},
	}

	decision := terminalStateGuardEvaluate(evidence, "已经全部完成。")
	if decision.Path != terminalStateGuardBlocked || decision.PendingExternalAction == nil {
		t.Fatalf("unkeyed execution completed an identified guide: %#v", decision)
	}
	if decision.PendingExternalAction.RetryAllowed ||
		terminalStateGuardCanRetryPendingExternalAction(decision.PendingExternalAction, map[string]int{}) {
		t.Fatalf("ambiguous external write could be replayed: %#v", decision.PendingExternalAction)
	}
}

func TestTerminalStateGuardAcceptsTruthfulNonExecutionAfterEmptyPrerequisite(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "查询企业微信成员并给他发送消息",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.contact.search", "can_execute": true,
				},
			},
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
				"arguments": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.contact.search",
				},
				"result": map[string]interface{}{"result_count": 0, "operation_status": "completed"},
			},
			map[string]interface{}{
				"invocation_id": "guide-send", "skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.message.send", "can_execute": true,
					"preparation_hints": []interface{}{map[string]interface{}{"action_id": "wecom.contact.search"}},
				},
			},
		},
	}

	decision := terminalStateGuardEvaluate(evidence, "没有找到目标成员，因此未发送消息。")
	if decision.Path != terminalStateGuardAccepted || decision.FinalAnswer != "没有找到目标成员，因此未发送消息。" {
		t.Fatalf("truthful non-execution after empty prerequisite was rejected: %#v", decision)
	}
}

func TestTerminalStateGuardEmptyPrerequisiteWaivesOnlyDependentWrite(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "查询成员，给他发送消息，再列出部门",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.contact.search", "can_execute": true,
				},
			},
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
				"arguments": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.contact.search",
				},
				"result": map[string]interface{}{"result_count": 0, "operation_status": "completed"},
			},
			map[string]interface{}{
				"invocation_id": "guide-send", "skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.message.send", "effect": "external_send", "can_execute": true,
					"preparation_hints": []interface{}{map[string]interface{}{"action_id": "wecom.contact.search"}},
				},
			},
			map[string]interface{}{
				"invocation_id": "guide-departments", "skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.department.list", "effect": "read", "can_execute": true,
				},
			},
		},
	}

	decision := terminalStateGuardEvaluate(evidence, "没有找到目标成员，因此未发送消息。")
	if decision.Path != terminalStateGuardBlocked || decision.PendingExternalAction == nil ||
		decision.PendingExternalAction.ActionID != "wecom.department.list" {
		t.Fatalf("empty prerequisite waived an unrelated pending action: %#v", decision)
	}
}

func TestTerminalStateGuardEmptyPrerequisiteDoesNotWaiveUnrelatedWrite(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "查询企业微信成员、发送消息并创建日历事件",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
				"arguments": map[string]interface{}{"integration_id": "wecom", "action_id": "wecom.contact.search"},
				"result":    map[string]interface{}{"result_count": 0, "operation_status": "completed"},
			},
			map[string]interface{}{
				"invocation_id": "guide-send", "skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.message.send", "effect": "external_send", "can_execute": true,
					"preparation_hints": []interface{}{map[string]interface{}{"action_id": "wecom.contact.search"}},
				},
			},
			map[string]interface{}{
				"invocation_id": "guide-create", "skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "calendar", "action_id": "calendar.event.create", "effect": "create", "can_execute": true,
				},
			},
		},
	}

	decision := terminalStateGuardEvaluate(evidence, "没有找到目标成员，因此未发送消息。")
	if decision.Path != terminalStateGuardBlocked || decision.PendingExternalAction == nil ||
		decision.PendingExternalAction.ActionID != "calendar.event.create" {
		t.Fatalf("empty member search waived an unrelated create action: %#v", decision)
	}
}

func TestTerminalStateGuardProjectedUnrelatedEmptyReadDoesNotWaiveCreate(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "创建日历事件",
		runtimeStateNativeExternalActionProjectionsKey: []interface{}{map[string]interface{}{
			"tool_name": "calendar_create_event", "integration_id": "calendar", "action_id": "calendar.event.create",
			"effect": "create", "preparation_action_keys": []interface{}{"calendar:calendar.event.lookup"},
		}},
		"operation_plan": map[string]interface{}{"phases": []interface{}{map[string]interface{}{
			"id": "phase-create", "step": "Create event", "status": "in_progress", "required": true,
			"expected_action": map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "execute_action",
				"projected_tool_name": "calendar_create_event", planExpectedActionServerProjectionKey: "calendar_create_event",
				"target": map[string]interface{}{"integration_id": "calendar", "action_id": "calendar.event.create"},
			},
		}}},
		"skill_invocations": []interface{}{map[string]interface{}{
			"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
			"arguments": map[string]interface{}{"integration_id": "wecom", "action_id": "wecom.contact.search"},
			"result":    map[string]interface{}{"result_count": 0, "operation_status": "completed"},
		}},
	}

	decision := terminalStateGuardEvaluate(evidence, "没有找到联系人，因此未创建事件。")
	if decision.Path != terminalStateGuardBlocked || decision.PendingExternalAction == nil ||
		decision.PendingExternalAction.ActionID != "calendar.event.create" {
		t.Fatalf("unrelated empty read waived projected create: %#v", decision)
	}
}

func TestTerminalStateGuardProjectedDeclaredEmptyPrerequisiteWaivesSend(t *testing.T) {
	evidence := projectedExternalActionPlanTestState([]interface{}{
		projectedExternalActionPlanTestPhase("phase-send", "in_progress", "recipient_ref", "missing"),
	})
	evidence[runtimeStateNativeExternalActionProjectionsKey] = []interface{}{map[string]interface{}{
		"tool_name": "wecom_send_message", "integration_id": "wecom", "action_id": "wecom.message.send",
		"effect": "external_send", "target_argument_paths": []interface{}{"recipient_ref"},
		"preparation_action_keys": []interface{}{"wecom:wecom.contact.search"},
	}}
	evidence["latest_user_request"] = "查找联系人并发送消息"
	evidence["skill_invocations"] = []interface{}{map[string]interface{}{
		"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
		"arguments": map[string]interface{}{
			"integration_id": "wecom", "action_id": "wecom.contact.search", "plan_phase_id": "phase-send",
		},
		"result": map[string]interface{}{"result_count": 0, "operation_status": "completed"},
	}}

	decision := terminalStateGuardEvaluate(evidence, "没有找到联系人，因此未发送消息。")
	if decision.Path != terminalStateGuardAccepted || decision.FinalAnswer != "没有找到联系人，因此未发送消息。" {
		t.Fatalf("declared projected prerequisite was not honored: %#v", decision)
	}
}

func TestTerminalStateGuardWaiverDoesNotHideDifferentUnknownAction(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "查询成员，发送消息，并创建日历事件",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.contact.search", "can_execute": true,
				},
			},
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
				"arguments": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.contact.search",
				},
				"result": map[string]interface{}{"result_count": 0, "operation_status": "completed"},
			},
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.message.send", "effect": "external_send", "can_execute": true,
					"preparation_hints": []interface{}{map[string]interface{}{"action_id": "wecom.contact.search"}},
				},
			},
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.calendar.event.create", "effect": "write", "can_execute": true,
				},
			},
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
				"arguments": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.calendar.event.create",
				},
				"result": map[string]interface{}{"operation_status": "outcome_unknown"},
			},
		},
	}
	candidate := "查询已完成，但未发送消息；日历事件已创建。"

	decision := terminalStateGuardEvaluate(evidence, candidate)
	if decision.Path != terminalStateGuardAccepted || decision.FinalAnswer == candidate ||
		!strings.Contains(decision.FinalAnswer, "不能视为已发送") {
		t.Fatalf("dependent-action waiver hid a different unknown action: %#v", decision)
	}
}

func TestTerminalStateGuardCategorySpecificSuccessAllowsTruthfulMixedAnswer(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "查询成员并发送消息",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.contact.search", "can_execute": true,
				},
			},
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
				"arguments": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.contact.search",
				},
				"result": map[string]interface{}{"result_count": 0, "operation_status": "completed"},
			},
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.message.send", "effect": "external_send", "can_execute": true,
					"preparation_hints": []interface{}{map[string]interface{}{"action_id": "wecom.contact.search"}},
				},
			},
		},
	}
	candidate := "查询已完成，但没有找到目标，因此未发送消息。"

	decision := terminalStateGuardEvaluate(evidence, candidate)
	if decision.Path != terminalStateGuardAccepted || decision.FinalAnswer != candidate {
		t.Fatalf("read success marker rejected a truthful send non-execution answer: %#v", decision)
	}
}

func TestTerminalStateGuardRejectsContradictoryNonExecutionClaim(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "查询企业微信成员并发送消息",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
				"arguments": map[string]interface{}{"integration_id": "wecom", "action_id": "wecom.contact.search"},
				"result":    map[string]interface{}{"result_count": 0, "operation_status": "completed"},
			},
			map[string]interface{}{
				"invocation_id": "guide-send", "skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.message.send", "effect": "external_send", "can_execute": true,
				},
			},
		},
	}

	decision := terminalStateGuardEvaluate(evidence, "并非未发送，实际已经发送。")
	if decision.Path != terminalStateGuardBlocked || decision.PendingExternalAction == nil {
		t.Fatalf("contradictory non-execution claim was accepted: %#v", decision)
	}
}

func TestTerminalStateGuardRetryExhaustionProducesSafeTerminalAnswer(t *testing.T) {
	decision := terminalStateGuardSafeExternalNonExecutionDecision(
		map[string]interface{}{"latest_user_request": "发送企业微信消息"},
		"并非未发送，实际已经发送。",
	)
	if decision.Path != terminalStateGuardAccepted || decision.FinalAnswer == "并非未发送，实际已经发送。" ||
		!strings.Contains(decision.FinalAnswer, "未完成") || !strings.Contains(decision.FinalAnswer, "不能视为已发送") {
		t.Fatalf("retry exhaustion did not produce a safe terminal answer: %#v", decision)
	}
}

func TestTerminalStateGuardDoesNotRetryAttemptedExternalWriteWithUnknownOutcome(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "给成员发送企业微信消息",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.message.send", "effect": "external_send", "can_execute": true,
				},
			},
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.message.send", "operation_status": "outcome_unknown", "retry_safe": false,
				},
			},
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.message.send", "effect": "external_send", "can_execute": true,
				},
			},
		},
	}

	decision := terminalStateGuardEvaluate(evidence, "消息已经发送。")
	if decision.Path != terminalStateGuardAccepted || decision.PendingExternalAction != nil {
		t.Fatalf("unknown write outcome was scheduled for replay: %#v", decision)
	}
	if !strings.Contains(decision.FinalAnswer, "不能视为已发送") {
		t.Fatalf("unknown write outcome was not replaced safely: %#v", decision)
	}
}

func TestTerminalStateGuardKeepsUnconfirmedOutcomePerAction(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "发送消息并创建日历事件",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
				"arguments": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.message.send",
				},
				"result": map[string]interface{}{"operation_status": "outcome_unknown"},
			},
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
				"arguments": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.calendar.event.create",
				},
				"result": map[string]interface{}{"operation_status": "completed"},
			},
		},
	}

	decision := terminalStateGuardEvaluate(evidence, "消息已发送，日历事件已创建。")
	if decision.Path != terminalStateGuardAccepted || !strings.Contains(decision.FinalAnswer, "不能视为已发送") {
		t.Fatalf("later successful action hid an earlier unknown outcome: %#v", decision)
	}
	if terminalStateGuardCanStream(evidence) {
		t.Fatal("terminal stream was allowed while one action outcome remained unknown")
	}
}

func TestTerminalStateGuardLaterSuccessOverridesEarlierFailureForSameAction(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "发送企业微信消息",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
				"arguments": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.message.send",
				},
				"result": map[string]interface{}{"operation_status": "failed_safe"},
			},
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
				"arguments": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.message.send",
				},
				"result": map[string]interface{}{"operation_status": "completed"},
			},
		},
	}

	decision := terminalStateGuardEvaluate(evidence, "消息已发送。")
	if decision.Path != terminalStateGuardAccepted || decision.FinalAnswer != "消息已发送。" {
		t.Fatalf("later success did not supersede the same action's earlier failure: %#v", decision)
	}
	if !terminalStateGuardCanStream(evidence) {
		t.Fatal("terminal stream remained blocked after the same action later completed")
	}
}

func TestTerminalStateGuardDoesNotClaimPartialExternalWriteCompleted(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "批量发送企业微信消息",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.message.send", "can_execute": true,
				},
			},
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
				"arguments": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.message.send",
				},
				"result": map[string]interface{}{"operation_status": "partially_succeeded", "retry_safe": false},
			},
		},
	}

	decision := terminalStateGuardEvaluate(evidence, "全部消息均已发送。")
	if decision.Path != terminalStateGuardAccepted || decision.PendingExternalAction != nil ||
		!strings.Contains(decision.FinalAnswer, "不能视为已发送") {
		t.Fatalf("partial external write was treated as complete or replayable: %#v", decision)
	}
}

func TestTerminalStateGuardDoesNotForceDisabledExternalGuide(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "发送企业微信消息",
		"skill_invocations": []interface{}{map[string]interface{}{
			"skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
			"result": map[string]interface{}{
				"integration_id": "wecom", "action_id": "wecom.message.send",
				"availability": "scope_upgrade_required", "can_execute": false,
			},
		}},
	}

	decision := terminalStateGuardEvaluate(evidence, "当前授权范围不支持发送消息。")
	if decision.Path != terminalStateGuardAccepted || decision.PendingExternalAction != nil {
		t.Fatalf("disabled guide was incorrectly forced to execute: %#v", decision)
	}
}

func TestTerminalStateGuardAllowsNaturalRequiredArgumentQuestion(t *testing.T) {
	evidence := map[string]interface{}{
		"latest_user_request": "查询钉钉通知状态",
		"skill_invocations": []interface{}{map[string]interface{}{
			"skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
			"result": map[string]interface{}{
				"integration_id": "dingtalk", "action_id": "dingtalk.message.delivery.get", "availability": "ready",
				"input_schema": map[string]interface{}{"required": []interface{}{"message_ref"}},
			},
		}},
	}

	decision := terminalStateGuardEvaluate(evidence, "你要查询哪一条通知？")
	if decision.Path != terminalStateGuardAccepted {
		t.Fatalf("natural required-argument question was blocked: %#v", decision)
	}
}
