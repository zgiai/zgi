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
				"result_summary": map[string]interface{}{"status": "completed"},
			},
		},
	}

	decision := terminalStateGuardEvaluate(evidence, "通知已发送。")
	if decision.Path != terminalStateGuardAccepted {
		t.Fatalf("redacted successful write was incorrectly scheduled for repetition: %#v", decision)
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
