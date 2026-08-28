package skillloop

import (
	"context"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestNormalizePlanSnapshotEnforcesStructuralProgressRules(t *testing.T) {
	if _, err := normalizePlanSnapshot([]interface{}{
		map[string]interface{}{"id": "phase-1", "step": "First", "status": "in_progress"},
		map[string]interface{}{"id": "phase-1", "step": "Second", "status": "pending"},
	}); err == nil {
		t.Fatal("normalizePlanSnapshot() error = nil, want duplicate explicit phase ID rejection")
	}

	_, err := normalizePlanSnapshot([]interface{}{
		map[string]interface{}{"id": "phase-1", "step": "First", "status": "in_progress"},
		map[string]interface{}{"id": "phase-2", "step": "Second", "status": "in_progress"},
	})
	if err == nil {
		t.Fatal("normalizePlanSnapshot() error = nil, want multiple in_progress rejection")
	}

	phasesWithoutEvidence, err := normalizePlanSnapshot([]interface{}{
		map[string]interface{}{"id": "phase-1", "step": "First", "status": "completed"},
		map[string]interface{}{"id": "phase-2", "step": "Optional cleanup", "status": "skipped"},
	})
	if err != nil || len(phasesWithoutEvidence) != 2 {
		t.Fatalf("normalizePlanSnapshot() = %#v, %v; want advisory phases accepted", phasesWithoutEvidence, err)
	}

	phases, err := normalizePlanSnapshot([]interface{}{
		map[string]interface{}{
			"id": "phase-1", "step": "Read file", "status": "completed", "evidence_refs": []interface{}{"file-reader/read_file"},
			"expected_action": map[string]interface{}{
				"skill_id": "file-reader", "tool_name": "read_file", "target": map[string]interface{}{"file_id": "file-1", "secret": "drop-me"},
			},
		},
		map[string]interface{}{"step": "Optional cleanup", "status": "skipped", "note": "Not requested"},
	})
	if err != nil {
		t.Fatalf("normalizePlanSnapshot() error = %v", err)
	}
	if phases[1]["id"] != "phase-amendment-1" {
		t.Fatalf("generated amendment ID = %#v, want phase-amendment-1", phases[1]["id"])
	}
	refs := evidenceStringSliceFromAny(phases[0]["evidence_refs"])
	if len(refs) != 1 || refs[0] != "tool:file-reader/read_file" {
		t.Fatalf("evidence_refs = %#v, want canonical tool ref", refs)
	}
	expectedAction := evidenceMapFromAny(phases[0]["expected_action"])
	if expectedAction["skill_id"] != "file-reader" || expectedAction["tool_name"] != "read_file" {
		t.Fatalf("expected_action = %#v", expectedAction)
	}
	target := evidenceMapFromAny(expectedAction["target"])
	if target["file_id"] != "file-1" || target["secret"] != nil {
		t.Fatalf("expected_action.target = %#v, want allowlisted target only", target)
	}
}

func TestHandleUpdatePlanCallProducesPersistablePlanTrace(t *testing.T) {
	step := (&Runner{}).handleUpdatePlanCall("call-plan", map[string]interface{}{
		"explanation": "file read completed",
		"plan": []interface{}{map[string]interface{}{
			"id": "phase-1", "step": "Read file", "status": "completed", "evidence_refs": []interface{}{"tool:file-reader/read_file"},
		}},
	}, successfulReadFileEvidence(), 2)
	if step.fatalErr != nil || step.trace.Kind != "plan_update" || step.trace.Status != "success" {
		t.Fatalf("handleUpdatePlanCall() step = %#v", step)
	}
	if step.trace.ToolName != skills.MetaToolUpdatePlan {
		t.Fatalf("trace.ToolName = %q, want %q", step.trace.ToolName, skills.MetaToolUpdatePlan)
	}
	if got, ok := step.trace.Arguments["round"].(int); !ok || got != 2 {
		t.Fatalf("trace round = %#v, want 2", step.trace.Arguments["round"])
	}
}

func TestHandleUpdatePlanCallAcceptsOutcomeContractWithoutCompatibilityPlan(t *testing.T) {
	step := (&Runner{}).handleUpdatePlanCall("call-outcomes", map[string]interface{}{
		"explanation": "the user changed the requested result",
		"outcomes": []interface{}{
			map[string]interface{}{
				"id": "outcome-file", "goal": "Save the generated file", "status": "pending",
				"capabilities": []interface{}{"managed_file"},
			},
			map[string]interface{}{
				"id": "outcome-agent", "goal": "Update the Agent prompt", "status": "pending",
				"depends_on": []interface{}{"outcome-file"}, "capabilities": []interface{}{"agent.system_prompt"},
			},
		},
	}, nil, 3)
	if step.fatalErr != nil || step.recoverable || step.trace.Status != "success" {
		t.Fatalf("handleUpdatePlanCall() = %#v, want successful outcome revision", step)
	}
	outcomes := evidenceMapsFromAny(step.trace.Result["outcomes"])
	if len(outcomes) != 2 || evidenceStringFromAny(outcomes[1]["id"]) != "outcome-agent" {
		t.Fatalf("trace outcomes = %#v, want two normalized outcomes", outcomes)
	}
	if plan := evidenceMapsFromAny(step.trace.Result["plan"]); len(plan) != 0 {
		t.Fatalf("compatibility plan = %#v, want omitted", plan)
	}
}

func TestHandleProgressiveSkillCallSkipsHiddenPlanUpdate(t *testing.T) {
	runner := &Runner{}
	state := map[string]interface{}{runtimeStateAllowPlanUpdateKey: false}
	for _, call := range []adapter.ToolCall{
		{
			ID: "direct-plan",
			Function: adapter.FunctionCall{
				Name:      skills.MetaToolUpdatePlan,
				Arguments: `{"plan":[{"id":"phase-1","step":"Read","status":"in_progress"}]}`,
			},
		},
		{
			ID: "nested-plan",
			Function: adapter.FunctionCall{
				Name:      skills.MetaToolCallSkillTool,
				Arguments: `{"tool_name":"update_plan","arguments":{"plan":[{"id":"phase-1","step":"Read","status":"in_progress"}]}}`,
			},
		},
	} {
		step := runner.handleProgressiveSkillCall(
			context.Background(), nil, &skills.ResolvedSkills{}, call, skills.ExecutionContext{},
			0, map[string]int{}, map[string]struct{}{}, state, 1, nil,
		)
		if step.recoverable || step.fatalErr != nil {
			t.Fatalf("handleProgressiveSkillCall(%s) = %#v, want non-failing advisory", call.ID, step)
		}
		if step.trace.Kind != "planner_feedback" || step.trace.Arguments["reason_code"] != "control_tool_not_required" {
			t.Fatalf("trace(%s) = %#v, want suppressed control-tool advisory", call.ID, step.trace)
		}
	}
}

func TestHandleUpdatePlanCallRecordsUnavailableEvidenceAsAuditWarning(t *testing.T) {
	step := (&Runner{}).handleUpdatePlanCall("call-plan", map[string]interface{}{
		"plan": []interface{}{map[string]interface{}{
			"id": "phase-1", "step": "Delete agent", "status": "completed", "evidence_refs": []interface{}{"agent-management/delete_agent"},
		}},
	}, successfulReadFileEvidence(), 2)
	if step.fatalErr != nil || step.recoverable || step.trace.Status != "success" {
		t.Fatalf("handleUpdatePlanCall() step = %#v, want successful advisory plan trace", step)
	}
	warnings := evidenceStringSliceFromAny(step.trace.Result["evidence_warnings"])
	if len(warnings) != 1 || warnings[0] != "unresolved_evidence_ref:tool:agent-management/delete_agent" {
		t.Fatalf("evidence_warnings = %#v", warnings)
	}
}

func TestHandleUpdatePlanCallRejectsNewTerminalProjectedPhaseWithoutExecution(t *testing.T) {
	for _, testCase := range []struct {
		status       string
		wantAccepted bool
	}{
		{status: "completed"},
		{status: "skipped"},
		{status: "pending", wantAccepted: true},
		{status: "in_progress", wantAccepted: true},
	} {
		t.Run(testCase.status, func(t *testing.T) {
			state := projectedExternalActionPlanTestState(nil)
			step := (&Runner{}).handleUpdatePlanCall("call-plan", map[string]interface{}{
				"plan": []interface{}{map[string]interface{}{
					"id": "phase-send", "step": "Send message", "status": testCase.status,
					"expected_action": projectedExternalActionPlanExpectedAction("alice"),
				}},
			}, state, 1)
			accepted := !step.recoverable && step.fatalErr == nil && step.trace.Status == "success"
			if accepted != testCase.wantAccepted {
				t.Fatalf("handleUpdatePlanCall(status=%q) = %#v; accepted=%v, want %v", testCase.status, step, accepted, testCase.wantAccepted)
			}
			if !testCase.wantAccepted && (!step.recoverable || step.trace.Status != "error") {
				t.Fatalf("terminal projected phase rejection was not recoverable: %#v", step)
			}
		})
	}
}

func TestHandleUpdatePlanCallRejectsShrinkingCompoundProjectedActionBaseline(t *testing.T) {
	state := projectedExternalActionPlanTestState([]interface{}{
		map[string]interface{}{"id": "phase-a", "step": "Send first message", "status": "in_progress", "required": true},
		map[string]interface{}{"id": "phase-b", "step": "Send second message", "status": "pending", "required": true},
	})
	step := (&Runner{}).handleUpdatePlanCall("call-plan", map[string]interface{}{
		"plan": []interface{}{map[string]interface{}{
			"id": "phase-a", "step": "Send first message", "status": "in_progress",
			"expected_action": projectedExternalActionPlanExpectedAction("alice"),
		}},
	}, state, 1)
	if !step.recoverable || step.trace.Status != "error" {
		t.Fatalf("handleUpdatePlanCall() = %#v, want recoverable baseline-shrink rejection", step)
	}
}

func TestHandleUpdatePlanCallRejectsSkippingOrReclassifyingRequiredProjectedPhase(t *testing.T) {
	state := projectedExternalActionPlanTestState([]interface{}{
		map[string]interface{}{"id": "phase-a", "step": "Send first message", "status": "in_progress", "required": true},
		map[string]interface{}{"id": "phase-b", "step": "Send second message", "status": "pending", "required": true},
	})
	for _, malicious := range []map[string]interface{}{
		{"id": "phase-b", "step": "Pretend second message is optional", "status": "skipped", "completion_mode": "non_tool"},
		{"id": "phase-b", "step": "Answer instead of sending", "status": "pending", "completion_mode": "final_answer"},
	} {
		step := (&Runner{}).handleUpdatePlanCall("call-plan", map[string]interface{}{
			"plan": []interface{}{
				map[string]interface{}{
					"id": "phase-a", "step": "Send first message", "status": "in_progress",
					"expected_action": projectedExternalActionPlanExpectedAction("alice"),
				},
				malicious,
			},
		}, state, 1)
		if !step.recoverable || step.trace.Status != "error" {
			t.Fatalf("handleUpdatePlanCall(%#v) = %#v, want recoverable required-phase rejection", malicious, step)
		}
	}
}

func TestHandleUpdatePlanCallAcceptsCompleteCanonicalProjectedActionLedger(t *testing.T) {
	state := projectedExternalActionPlanTestState([]interface{}{
		map[string]interface{}{"id": "phase-a", "step": "Send first message", "status": "in_progress", "required": true},
		map[string]interface{}{"id": "phase-b", "step": "Send second message", "status": "pending", "required": true},
	})
	step := (&Runner{}).handleUpdatePlanCall("call-plan", map[string]interface{}{
		"plan": []interface{}{
			map[string]interface{}{
				"id": "phase-a", "step": "Send first message", "status": "in_progress",
				"expected_action": projectedExternalActionPlanExpectedAction("alice"),
			},
			map[string]interface{}{
				"id": "phase-b", "step": "Send second message", "status": "pending",
				"expected_action": projectedExternalActionPlanExpectedAction("bob"),
			},
		},
	}, state, 1)
	if step.recoverable || step.fatalErr != nil || step.trace.Status != "success" {
		t.Fatalf("handleUpdatePlanCall() = %#v, want complete projected Action ledger accepted", step)
	}
}

func TestHandleUpdatePlanCallRejectsReplacingExistingProjectedActionTarget(t *testing.T) {
	baseline := projectedExternalActionPlanTestPhase("phase-send", "in_progress", "recipient_ref", "bob")
	baseline["step"] = "Send Bob a message"
	state := projectedExternalActionPlanTestState([]interface{}{baseline})
	step := (&Runner{}).handleUpdatePlanCall("call-plan", map[string]interface{}{
		"plan": []interface{}{map[string]interface{}{
			"id": "phase-send", "step": "Send someone else a message", "status": "in_progress",
			"expected_action": projectedExternalActionPlanExpectedAction("alice"),
		}},
	}, state, 1)
	if !step.recoverable || step.trace.Status != "error" {
		t.Fatalf("handleUpdatePlanCall() = %#v, want projected target replacement rejection", step)
	}
}

func TestHandleUpdatePlanCallAcceptsProjectedActionWithServerFinalAnswerOutcome(t *testing.T) {
	state := projectedExternalActionPlanTestState([]interface{}{
		map[string]interface{}{
			"id": "phase-send", "outcome_id": "outcome-send", "step": "Send message", "status": "pending",
		},
		map[string]interface{}{
			"id": "phase-explain", "outcome_id": "outcome-explain", "step": "Explain result", "status": "pending",
		},
	})
	state["operation_plan"].(map[string]interface{})["outcomes"] = []interface{}{
		map[string]interface{}{"id": "outcome-send", "required": true, "verification_mode": "runtime_effects"},
		map[string]interface{}{"id": "outcome-explain", "required": true, "verification_mode": "final_answer"},
	}
	step := (&Runner{}).handleUpdatePlanCall("call-plan", map[string]interface{}{
		"plan": []interface{}{
			map[string]interface{}{
				"id": "phase-send", "step": "Send message", "status": "in_progress",
				"expected_action": projectedExternalActionPlanExpectedAction("alice"),
			},
			map[string]interface{}{
				"id": "phase-explain", "step": "Explain result", "status": "pending", "completion_mode": "final_answer",
			},
		},
	}, state, 1)
	if step.recoverable || step.fatalErr != nil || step.trace.Status != "success" {
		t.Fatalf("handleUpdatePlanCall() = %#v, want mixed projected/final-answer outcome accepted", step)
	}
	phases := evidenceMapsFromAny(step.trace.Result["plan"])
	if evidenceStringFromAny(phases[1]["outcome_id"]) != "outcome-explain" {
		t.Fatalf("server outcome linkage was not preserved: %#v", phases)
	}
}

func TestHandleUpdatePlanCallWithAvailableProjectionsAcceptsOrdinaryMultiPhaseRevision(t *testing.T) {
	state := projectedExternalActionPlanTestState([]interface{}{
		map[string]interface{}{"id": "phase-file", "step": "Read file", "status": "in_progress", "required": true},
		map[string]interface{}{"id": "phase-agent", "step": "Update agent", "status": "pending", "required": true},
	})
	step := (&Runner{}).handleUpdatePlanCall("call-plan", map[string]interface{}{
		"plan": []interface{}{
			map[string]interface{}{"id": "phase-file", "step": "Read file", "status": "in_progress"},
			map[string]interface{}{"id": "phase-agent", "step": "Update agent", "status": "pending"},
		},
	}, state, 1)
	if step.recoverable || step.fatalErr != nil || step.trace.Status != "success" {
		t.Fatalf("handleUpdatePlanCall() = %#v, projections should not force expected_action onto an ordinary plan", step)
	}
}

func TestHandleUpdatePlanCallRejectsTwoStepFinalAnswerClassificationLaundering(t *testing.T) {
	state := projectedExternalActionPlanTestState([]interface{}{
		map[string]interface{}{"id": "phase-a", "outcome_id": "outcome-a", "step": "Send first", "status": "in_progress"},
		map[string]interface{}{"id": "phase-b", "outcome_id": "outcome-b", "step": "Send second", "status": "pending"},
	})
	state["operation_plan"].(map[string]interface{})["outcomes"] = []interface{}{
		map[string]interface{}{"id": "outcome-a", "required": true, "verification_mode": "runtime_effects"},
		map[string]interface{}{"id": "outcome-b", "required": true, "verification_mode": "runtime_effects"},
	}

	first := (&Runner{}).handleUpdatePlanCall("call-plan-1", map[string]interface{}{
		"plan": []interface{}{
			map[string]interface{}{"id": "phase-a", "step": "Send first", "status": "in_progress"},
			map[string]interface{}{"id": "phase-b", "step": "Pretend second is explanation", "status": "pending", "completion_mode": "final_answer"},
		},
	}, state, 1)
	if first.recoverable || first.fatalErr != nil || first.trace.Status != "success" {
		t.Fatalf("first ordinary revision = %#v, want accepted before projected-ledger mode", first)
	}
	state["operation_plan"] = map[string]interface{}{
		"phases":   first.trace.Result["plan"],
		"outcomes": state["operation_plan"].(map[string]interface{})["outcomes"],
	}

	second := (&Runner{}).handleUpdatePlanCall("call-plan-2", map[string]interface{}{
		"plan": []interface{}{
			map[string]interface{}{
				"id": "phase-a", "step": "Send first", "status": "in_progress",
				"expected_action": projectedExternalActionPlanExpectedAction("alice"),
			},
			map[string]interface{}{"id": "phase-b", "step": "Still pretend explanation", "status": "pending", "completion_mode": "final_answer"},
		},
	}, state, 2)
	if !second.recoverable || second.trace.Status != "error" {
		t.Fatalf("second projected revision = %#v, want laundering rejection", second)
	}
}

func projectedExternalActionPlanExpectedAction(recipient string) map[string]interface{} {
	return map[string]interface{}{
		"skill_id":                            skills.SkillExternalApps,
		"tool_name":                           "execute_action",
		"projected_tool_name":                 "wecom_send_message",
		planExpectedActionServerProjectionKey: "wecom_send_message",
		"target": map[string]interface{}{
			"integration_id": "wecom",
			"action_id":      "wecom.message.send",
		},
		"target_arguments": map[string]interface{}{"recipient_ref": recipient},
	}
}

func successfulReadFileEvidence() map[string]interface{} {
	return map[string]interface{}{
		"evidence_ledger": []interface{}{map[string]interface{}{
			"status": "completed", "skill_id": "file-reader", "tool_name": "read_file", "invocation_id": "runtime_id:read-1",
		}},
	}
}
