package skillloop

import (
	"context"
	"errors"
	"reflect"
	"testing"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestRunnerDirectProjectedReadAutoBindsWithoutModelPlan(t *testing.T) {
	runtime, resolved, actionTool := newProjectedExternalActionRunnerRuntime(t)
	toolSet := skills.NativeToolSet{ActiveSkillIDs: []string{skills.SkillExternalApps}, ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000}
	if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{
		projectedExternalActionRunnerProjectionFor("mail", "mail_search_messages", "mail.message.search"),
	}, skills.NativeToolProjectionOptions{}); added != 1 {
		t.Fatalf("AppendNativeToolProjections() added=%d, want 1", added)
	}
	alias := projectedExternalActionAliasForAction(t, toolSet, "mail", "mail.message.search")
	directCall := projectedExternalActionRunnerToolCall(t, "call-mail", alias, map[string]interface{}{"query": "recent"})
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{directCall}}, FinishReason: "tool_calls"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "已查到最近邮件。"}, FinishReason: "stop"}}},
	}}
	state := map[string]interface{}{"operation_plan": map[string]interface{}{}}
	autoBindings := 0
	runner := &Runner{
		LLMClient: fakeLLM, SkillRuntime: runtime, AppContext: &llmclient.AppContext{},
	}
	commitBinding := func(_ context.Context, trace skills.SkillTrace) error {
		if evidenceStringFromAny(trace.Arguments["source"]) != projectedActionAutoBindingTraceSource {
			return errors.New("unexpected binding trace")
		}
		autoBindings++
		state["operation_plan"] = map[string]interface{}{
			"outcomes": trace.Result["outcomes"],
			"phases":   trace.Result["plan"],
		}
		return nil
	}
	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared: NewPreparedChat("conv-auto-mail", "msg-auto-mail", "", "auto", &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "用邮箱外部连接查看最近收件箱"}},
		}),
		Resolved: resolved, NativeAgentLoop: true, NativeToolSet: &toolSet,
		RuntimeStateSnapshot:         func() map[string]interface{} { return state },
		CommitProjectedActionBinding: commitBinding,
	})
	if err != nil || answer != "已查到最近邮件。" {
		t.Fatalf("Run() answer=%q error=%v", answer, err)
	}
	if autoBindings != 1 || fakeLLM.appChatCalls != 2 {
		t.Fatalf("auto bindings=%d model calls=%d, want 1 and 2", autoBindings, fakeLLM.appChatCalls)
	}
	if got := actionTool.actionIDs(); !reflect.DeepEqual(got, []string{"mail.message.search"}) {
		t.Fatalf("provider Actions = %#v", got)
	}
	for _, call := range actionTool.calls {
		if _, leaked := call["plan_phase_id"]; leaked {
			t.Fatalf("server phase leaked to Provider: %#v", call)
		}
	}
}

func TestRunnerProjectedAutoBindingFailsClosedWhenSnapshotDoesNotRefresh(t *testing.T) {
	runtime, resolved, actionTool := newProjectedExternalActionRunnerRuntime(t)
	toolSet := skills.NativeToolSet{ActiveSkillIDs: []string{skills.SkillExternalApps}, ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000}
	if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{
		projectedExternalActionRunnerProjectionFor("mail", "mail_search_messages", "mail.message.search"),
	}, skills.NativeToolProjectionOptions{}); added != 1 {
		t.Fatalf("AppendNativeToolProjections() added=%d, want 1", added)
	}
	alias := projectedExternalActionAliasForAction(t, toolSet, "mail", "mail.message.search")
	directCall := projectedExternalActionRunnerToolCall(t, "call-mail", alias, map[string]interface{}{"query": "recent"})
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{directCall}}, FinishReason: "tool_calls"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "没有执行邮箱查询。"}, FinishReason: "stop"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "邮箱查询未执行。"}, FinishReason: "stop"}}},
	}}
	state := map[string]interface{}{"operation_plan": map[string]interface{}{}}
	commitCalls := 0
	runner := &Runner{LLMClient: fakeLLM, SkillRuntime: runtime, AppContext: &llmclient.AppContext{}}
	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared: NewPreparedChat("conv-auto-fail-closed", "msg-auto-fail-closed", "", "auto", &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "查看最近收件箱"}},
		}),
		Resolved: resolved, NativeAgentLoop: true, NativeToolSet: &toolSet,
		RuntimeStateSnapshot: func() map[string]interface{} { return state },
		CommitProjectedActionBinding: func(_ context.Context, trace skills.SkillTrace) error {
			commitCalls++
			// Simulate the old unsafe persistence order: the working snapshot was
			// mutated even though the durable write failed. Runner must still stop.
			state["operation_plan"] = map[string]interface{}{
				"outcomes": trace.Result["outcomes"],
				"phases":   trace.Result["plan"],
			}
			return errors.New("database unavailable")
		},
	})
	if !errors.Is(err, ErrProjectedActionBinding) {
		t.Fatalf("Run() error = %v, want ErrProjectedActionBinding", err)
	}
	if answer == "已查到最近邮件。" || len(actionTool.calls) != 0 || commitCalls != 1 {
		t.Fatalf("binding persistence failure reached Provider or claimed success: answer=%q calls=%#v", answer, actionTool.calls)
	}
}

func TestProjectedExternalActionAutoBindCreatesServerOutcome(t *testing.T) {
	toolSet := skills.NativeToolSet{ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000}
	if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{
		projectedExternalActionRunnerProjectionFor("mail", "mail_search_messages", "mail.message.search"),
	}, skills.NativeToolProjectionOptions{}); added != 1 {
		t.Fatalf("AppendNativeToolProjections() added=%d, want 1", added)
	}
	alias := projectedExternalActionAliasForAction(t, toolSet, "mail", "mail.message.search")
	call := projectedExternalActionRunnerToolCall(t, "call-mail", alias, map[string]interface{}{"query": "recent"})
	state := runtimeStateForRun(RunRequest{NativeToolSet: &toolSet})

	trace, phaseID := projectedExternalActionAutoBindCall(
		call,
		nativeExecutionCall(call, &toolSet),
		&toolSet,
		state,
		1,
		map[string]struct{}{},
	)
	if phaseID == "" || trace.Kind != "plan_update" || trace.Status != "success" {
		t.Fatalf("auto binding trace=%#v phaseID=%q", trace, phaseID)
	}
	if evidenceStringFromAny(trace.Arguments["source"]) != projectedActionAutoBindingTraceSource {
		t.Fatalf("trace source = %#v", trace.Arguments)
	}
	outcomes := evidenceMapsFromAny(trace.Result["outcomes"])
	phases := evidenceMapsFromAny(trace.Result["plan"])
	if len(outcomes) != 1 || len(phases) != 1 {
		t.Fatalf("outcomes=%#v phases=%#v, want one of each", outcomes, phases)
	}
	phase := phases[0]
	if evidenceStringFromAny(phase[operationPlanPhaseSourceKey]) != operationPlanPhaseSourceServerOutcome ||
		evidenceStringFromAny(phase[operationPlanServerProjectedLedgerEpochKey]) == "" ||
		evidenceStringFromAny(phase[operationPlanServerProjectedConnectionBindingKey]) == "" {
		t.Fatalf("server phase facts missing: %#v", phase)
	}
	expected := evidenceMapFromAny(phase["expected_action"])
	if evidenceStringFromAny(expected[planExpectedActionServerBindingFingerprintKey]) == "" ||
		evidenceStringFromAny(evidenceMapFromAny(expected["target"])["action_id"]) != "mail.message.search" {
		t.Fatalf("canonical expected_action = %#v", expected)
	}
}

func TestProjectedExternalActionCommittedBindingRequiresExactDurableFacts(t *testing.T) {
	toolSet := skills.NativeToolSet{ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000}
	if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{
		projectedExternalActionRunnerProjectionFor("mail", "mail_search_messages", "mail.message.search"),
	}, skills.NativeToolProjectionOptions{}); added != 1 {
		t.Fatalf("AppendNativeToolProjections() added=%d, want 1", added)
	}
	alias := projectedExternalActionAliasForAction(t, toolSet, "mail", "mail.message.search")
	call := projectedExternalActionRunnerToolCall(t, "call-mail", alias, map[string]interface{}{"query": "recent"})
	executionCall := nativeExecutionCall(call, &toolSet)

	committedState := func() (map[string]interface{}, skills.SkillTrace, string) {
		initial := runtimeStateForRun(RunRequest{NativeToolSet: &toolSet})
		trace, phaseID := projectedExternalActionAutoBindCall(call, executionCall, &toolSet, initial, 1, map[string]struct{}{})
		expectation := projectedExternalActionAutoBindingTraceSnapshot(trace)
		state := runtimeStateForRun(RunRequest{
			NativeToolSet: &toolSet,
			RuntimeStateSnapshot: func() map[string]interface{} {
				return map[string]interface{}{"operation_plan": map[string]interface{}{
					"outcomes": trace.Result["outcomes"],
					"phases":   trace.Result["plan"],
				}}
			},
		})
		return state, expectation, phaseID
	}
	state, trace, phaseID := committedState()
	if !projectedExternalActionCommittedBindingMatchesCall(call, executionCall, &toolSet, state, phaseID, trace) {
		t.Fatal("exact committed binding was rejected")
	}

	tests := []struct {
		name   string
		mutate func(map[string]interface{})
	}{
		{name: "source", mutate: func(phase map[string]interface{}) {
			phase[operationPlanPhaseSourceKey] = operationPlanPhaseSourceModelAdvisory
		}},
		{name: "fingerprint", mutate: func(phase map[string]interface{}) {
			evidenceMapFromAny(phase["expected_action"])[planExpectedActionServerBindingFingerprintKey] = "other-binding"
		}},
		{name: "epoch", mutate: func(phase map[string]interface{}) { phase[operationPlanServerProjectedLedgerEpochKey] = "other-epoch" }},
		{name: "connection", mutate: func(phase map[string]interface{}) {
			phase[operationPlanServerProjectedConnectionBindingKey] = "other-connection"
		}},
		{name: "target", mutate: func(phase map[string]interface{}) {
			evidenceMapFromAny(phase["expected_action"])["target_arguments"] = map[string]interface{}{"unexpected": "value"}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, trace, phaseID := committedState()
			phases := evidenceMapsFromAny(evidenceMapFromAny(state["operation_plan"])["phases"])
			tt.mutate(phases[0])
			if projectedExternalActionCommittedBindingMatchesCall(call, executionCall, &toolSet, state, phaseID, trace) {
				t.Fatalf("binding with tampered %s was accepted", tt.name)
			}
		})
	}
}

func TestProjectedExternalActionAutoBindAllowsUnmatchedExposedAction(t *testing.T) {
	projection := projectedExternalActionRunnerProjectionFor("mail", "mail_account_get", "mail.account.get")
	projection.Binding.IntentMatched = false
	projection.Binding.Pinned = false
	projection.Binding.Effect = "read"
	projection.Binding.PreparationActionIDs = nil
	projection.Binding.PreparationHints = nil
	toolSet := skills.NativeToolSet{ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000}
	if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{projection}, skills.NativeToolProjectionOptions{}); added != 1 {
		t.Fatalf("AppendNativeToolProjections() added=%d, want 1", added)
	}
	alias := projectedExternalActionAliasForAction(t, toolSet, "mail", "mail.account.get")
	call := projectedExternalActionRunnerToolCall(t, "call-account", alias, map[string]interface{}{})
	trace, phaseID := projectedExternalActionAutoBindCall(
		call,
		nativeExecutionCall(call, &toolSet),
		&toolSet,
		runtimeStateForRun(RunRequest{NativeToolSet: &toolSet}),
		1,
		map[string]struct{}{},
	)
	if phaseID == "" || trace.Kind != "plan_update" || trace.Status != "success" {
		t.Fatalf("unmatched exposed Action was not auto-bound: trace=%#v phaseID=%q", trace, phaseID)
	}
	phases := evidenceMapsFromAny(trace.Result["plan"])
	if len(phases) != 1 ||
		evidenceStringFromAny(phases[0][operationPlanPhaseSourceKey]) != operationPlanPhaseSourceServerOutcome {
		t.Fatalf("unmatched exposed Action did not receive one server Outcome phase: %#v", phases)
	}
	expected := evidenceMapFromAny(phases[0]["expected_action"])
	if got := evidenceStringFromAny(evidenceMapFromAny(expected["target"])["action_id"]); got != "mail.account.get" {
		t.Fatalf("auto-bound Action identity = %q, want mail.account.get; expected=%#v", got, expected)
	}
	if got := evidenceStringFromAny(expected[planExpectedActionServerBindingFingerprintKey]); got != projection.Binding.BindingFingerprint {
		t.Fatalf("auto-bound fingerprint = %q, want %q", got, projection.Binding.BindingFingerprint)
	}
}

func TestProjectedExternalActionAutoBindRejectsUnexposedBinding(t *testing.T) {
	projection := projectedExternalActionRunnerProjectionFor("mail", "mail_account_get", "mail.account.get")
	projection.Binding.IntentMatched = false
	projection.Binding.Pinned = false
	projection.Binding.Effect = "read"
	projection.Binding.PreparationActionIDs = nil
	projection.Binding.PreparationHints = nil
	toolSet := skills.NativeToolSet{ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000}
	if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{projection}, skills.NativeToolProjectionOptions{}); added != 1 {
		t.Fatalf("AppendNativeToolProjections() added=%d, want 1", added)
	}
	alias := projectedExternalActionAliasForAction(t, toolSet, "mail", "mail.account.get")
	// Simulate a stale or forged call that retained a binding after the schema
	// was no longer present in the request's provider-visible tool projection.
	toolSet.ProviderTools = nil
	call := projectedExternalActionRunnerToolCall(t, "call-unexposed", alias, map[string]interface{}{})
	trace, phaseID := projectedExternalActionAutoBindCall(
		call,
		nativeExecutionCall(call, &toolSet),
		&toolSet,
		runtimeStateForRun(RunRequest{NativeToolSet: &toolSet}),
		1,
		map[string]struct{}{},
	)
	if phaseID != "" || trace.Kind != "" {
		t.Fatalf("unexposed Action acquired authority: trace=%#v phaseID=%q", trace, phaseID)
	}
}

func TestProjectedExternalActionAutoBindPreparationRequiresCompleteHint(t *testing.T) {
	prepare := projectedExternalActionRunnerProjection("wecom_search_contact", "wecom.contact.search")
	prepare.Binding.IntentMatched = false
	prepare.Binding.Pinned = false
	root := projectedExternalActionRunnerProjection("wecom_send_message", "wecom.message.send")

	build := func(rootProjection skills.NativeToolProjection) skills.NativeToolSet {
		toolSet := skills.NativeToolSet{ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000}
		if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{prepare, rootProjection}, skills.NativeToolProjectionOptions{}); added != 2 {
			t.Fatalf("AppendNativeToolProjections() added=%d, want 2", added)
		}
		return toolSet
	}

	withoutHint := root
	withoutHint.Binding.PreparationHints = nil
	toolSet := build(withoutHint)
	alias := projectedExternalActionAliasForAction(t, toolSet, "wecom", "wecom.contact.search")
	call := projectedExternalActionRunnerToolCall(t, "call-prepare", alias, map[string]interface{}{"query": "Alice"})
	trace, phaseID := projectedExternalActionAutoBindCall(
		call, nativeExecutionCall(call, &toolSet), &toolSet,
		runtimeStateForRun(RunRequest{NativeToolSet: &toolSet}), 1, map[string]struct{}{},
	)
	if phaseID == "" || trace.Kind != "plan_update" {
		t.Fatalf("directly exposed Action was not bound to its own Outcome: trace=%#v phase=%q", trace, phaseID)
	}
	phase := evidenceMapsFromAny(trace.Result["plan"])[0]
	expected := evidenceMapFromAny(phase["expected_action"])
	if got := evidenceStringFromAny(evidenceMapFromAny(expected["target"])["action_id"]); got != "wecom.contact.search" {
		t.Fatalf("PreparationActionIDs alone granted parent authority: expected=%#v", expected)
	}

	toolSet = build(root)
	alias = projectedExternalActionAliasForAction(t, toolSet, "wecom", "wecom.contact.search")
	call = projectedExternalActionRunnerToolCall(t, "call-prepare", alias, map[string]interface{}{"query": "Alice"})
	trace, phaseID = projectedExternalActionAutoBindCall(
		call, nativeExecutionCall(call, &toolSet), &toolSet,
		runtimeStateForRun(RunRequest{NativeToolSet: &toolSet}), 1, map[string]struct{}{},
	)
	if phaseID == "" || trace.Kind != "plan_update" {
		t.Fatalf("valid PreparationHint was not bound: trace=%#v phase=%q", trace, phaseID)
	}
	phase = evidenceMapsFromAny(trace.Result["plan"])[0]
	expected = evidenceMapFromAny(phase["expected_action"])
	if got := evidenceStringFromAny(evidenceMapFromAny(expected["target"])["action_id"]); got != "wecom.message.send" {
		t.Fatalf("preparation bound its own Action instead of parent: expected=%#v", expected)
	}
}

func TestProjectedExternalActionAutoBindSeparatesDifferentTargets(t *testing.T) {
	toolSet := skills.NativeToolSet{ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000}
	if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{
		projectedExternalActionRunnerProjection("wecom_send_message", "wecom.message.send"),
	}, skills.NativeToolProjectionOptions{}); added != 1 {
		t.Fatalf("AppendNativeToolProjections() added=%d, want 1", added)
	}
	alias := projectedExternalActionAliasForAction(t, toolSet, "wecom", "wecom.message.send")
	claimed := map[string]struct{}{}
	state := runtimeStateForRun(RunRequest{NativeToolSet: &toolSet})
	first := projectedExternalActionRunnerToolCall(t, "call-first", alias, map[string]interface{}{
		"recipient_ref": "alice", "content": "first",
	})
	firstTrace, firstPhase := projectedExternalActionAutoBindCall(first, nativeExecutionCall(first, &toolSet), &toolSet, state, 1, claimed)
	if firstPhase == "" {
		t.Fatal("first call did not acquire a phase")
	}
	state["operation_plan"] = map[string]interface{}{
		"outcomes": firstTrace.Result["outcomes"],
		"phases":   firstTrace.Result["plan"],
	}
	state = runtimeStateForRun(RunRequest{NativeToolSet: &toolSet, RuntimeStateSnapshot: func() map[string]interface{} { return state }})
	second := projectedExternalActionRunnerToolCall(t, "call-second", alias, map[string]interface{}{
		"recipient_ref": "bob", "content": "second",
	})
	secondTrace, secondPhase := projectedExternalActionAutoBindCall(second, nativeExecutionCall(second, &toolSet), &toolSet, state, 1, claimed)
	if secondPhase == "" || secondPhase == firstPhase || secondTrace.Kind != "plan_update" {
		t.Fatalf("different-target calls were collapsed: first=%q second=%q trace=%#v", firstPhase, secondPhase, secondTrace)
	}
	if got := len(evidenceMapsFromAny(secondTrace.Result["plan"])); got != 2 {
		t.Fatalf("phase count = %d, want 2", got)
	}
}

func TestProjectedExternalActionAutoBindPrefersUniqueActiveServerOutcomeInIntentGroup(t *testing.T) {
	toolSet := skills.NativeToolSet{ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000}
	if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{
		projectedExternalActionRunnerProjection("wecom_send_message", "wecom.message.send"),
	}, skills.NativeToolProjectionOptions{}); added != 1 {
		t.Fatalf("AppendNativeToolProjections() added=%d, want 1", added)
	}
	alias := projectedExternalActionAliasForAction(t, toolSet, "wecom", "wecom.message.send")
	state := projectedExternalActionAutoBindingServerOutcomeState(&toolSet,
		[]interface{}{
			map[string]interface{}{
				"id": "phase-first", "outcome_id": "outcome-first", "step": "Send first message", "status": "in_progress",
				operationPlanPhaseSourceKey: operationPlanPhaseSourceServerOutcome,
			},
			map[string]interface{}{
				"id": "phase-second", "outcome_id": "outcome-second", "step": "Send second message", "status": "pending",
				operationPlanPhaseSourceKey: operationPlanPhaseSourceServerOutcome,
			},
		},
		[]interface{}{
			projectedExternalActionAutoBindingServerOutcome("outcome-first", "Send first message"),
			projectedExternalActionAutoBindingServerOutcome("outcome-second", "Send second message"),
		},
	)
	call := projectedExternalActionRunnerToolCall(t, "call-first", alias, map[string]interface{}{
		"recipient_ref": "alice", "content": "first",
	})

	trace, phaseID := projectedExternalActionAutoBindCall(
		call, nativeExecutionCall(call, &toolSet), &toolSet, state, 1, map[string]struct{}{},
	)
	if phaseID != "phase-first" || trace.Kind != "plan_update" {
		t.Fatalf("auto binding trace=%#v phase=%q, want unique active phase-first", trace, phaseID)
	}
	phases := evidenceMapsFromAny(trace.Result["plan"])
	if len(phases) != 2 {
		t.Fatalf("phase count = %d, want existing two Outcomes without a synthetic third", len(phases))
	}
	if len(evidenceMapFromAny(phases[0]["expected_action"])) == 0 || len(evidenceMapFromAny(phases[1]["expected_action"])) != 0 {
		t.Fatalf("server Outcome bindings = %#v, want only active phase bound", phases)
	}
}

func TestProjectedExternalActionAutoBindRejectsAmbiguousServerOutcomesInIntentGroup(t *testing.T) {
	for _, statuses := range []struct {
		name   string
		first  string
		second string
	}{
		{name: "multiple pending", first: "pending", second: "pending"},
		{name: "multiple active", first: "in_progress", second: "running"},
	} {
		t.Run(statuses.name, func(t *testing.T) {
			toolSet := skills.NativeToolSet{ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000}
			if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{
				projectedExternalActionRunnerProjection("wecom_send_message", "wecom.message.send"),
			}, skills.NativeToolProjectionOptions{}); added != 1 {
				t.Fatalf("AppendNativeToolProjections() added=%d, want 1", added)
			}
			alias := projectedExternalActionAliasForAction(t, toolSet, "wecom", "wecom.message.send")
			state := projectedExternalActionAutoBindingServerOutcomeState(&toolSet,
				[]interface{}{
					map[string]interface{}{
						"id": "phase-first", "outcome_id": "outcome-first", "step": "Send first message", "status": statuses.first,
						operationPlanPhaseSourceKey: operationPlanPhaseSourceServerOutcome,
					},
					map[string]interface{}{
						"id": "phase-second", "outcome_id": "outcome-second", "step": "Send second message", "status": statuses.second,
						operationPlanPhaseSourceKey: operationPlanPhaseSourceServerOutcome,
					},
				},
				[]interface{}{
					projectedExternalActionAutoBindingServerOutcome("outcome-first", "Send first message"),
					projectedExternalActionAutoBindingServerOutcome("outcome-second", "Send second message"),
				},
			)
			call := projectedExternalActionRunnerToolCall(t, "call-send", alias, map[string]interface{}{
				"recipient_ref": "alice", "content": "hello",
			})

			trace, phaseID := projectedExternalActionAutoBindCall(
				call, nativeExecutionCall(call, &toolSet), &toolSet, state, 1, map[string]struct{}{},
			)
			if phaseID != "" || trace.Kind != "" {
				t.Fatalf("ambiguous server Outcomes acquired a binding: trace=%#v phase=%q", trace, phaseID)
			}
		})
	}
}

func projectedExternalActionAutoBindingServerOutcomeState(
	toolSet *skills.NativeToolSet,
	phases []interface{},
	outcomes []interface{},
) map[string]interface{} {
	return runtimeStateForRun(RunRequest{
		NativeToolSet: toolSet,
		RuntimeStateSnapshot: func() map[string]interface{} {
			return map[string]interface{}{"operation_plan": map[string]interface{}{
				"phases": phases, "outcomes": outcomes,
			}}
		},
	})
}

func projectedExternalActionAutoBindingServerOutcome(id string, goal string) map[string]interface{} {
	return map[string]interface{}{
		"id": id, "goal": goal, "title": goal, "status": "pending", "required": true,
		"verification_mode": "runtime_effects", "capabilities": []interface{}{skills.SkillExternalApps},
	}
}

func TestProjectedExternalActionAutoBindDoesNotMintDuplicateConfirmedMutation(t *testing.T) {
	toolSet := skills.NativeToolSet{ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000}
	if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{
		projectedExternalActionRunnerProjection("wecom_send_message", "wecom.message.send"),
	}, skills.NativeToolProjectionOptions{}); added != 1 {
		t.Fatalf("AppendNativeToolProjections() added=%d, want 1", added)
	}
	alias := projectedExternalActionAliasForAction(t, toolSet, "wecom", "wecom.message.send")
	call := projectedExternalActionRunnerToolCall(t, "call-repeat", alias, map[string]interface{}{
		"recipient_ref": "alice", "content": "repeat",
	})
	state := runtimeStateForRun(RunRequest{NativeToolSet: &toolSet, RuntimeStateSnapshot: func() map[string]interface{} {
		return map[string]interface{}{
			"operation_plan": map[string]interface{}{"phases": []interface{}{}},
			"skill_invocations": []interface{}{map[string]interface{}{
				"kind": "tool_call", "status": "success", "skill_id": skills.SkillExternalApps, "tool_name": "execute_action",
				"arguments": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.message.send",
					"operation_plan_target": map[string]interface{}{"recipient_ref": "alice"},
				},
				"result": map[string]interface{}{"operation_status": "completed", "provider_success_confirmed": true},
			}},
		}
	}})
	trace, phaseID := projectedExternalActionAutoBindCall(call, nativeExecutionCall(call, &toolSet), &toolSet, state, 2, map[string]struct{}{})
	if phaseID != "" || trace.Kind != "" {
		t.Fatalf("confirmed mutation received a new operation phase: trace=%#v phase=%q", trace, phaseID)
	}
}
