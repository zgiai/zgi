package skillloop

import (
	"context"
	"strings"
	"testing"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestRecoverableSkillFailureBudgetSeparatesPlannerAndBusinessFailures(t *testing.T) {
	tests := []struct {
		name      string
		trace     skills.SkillTrace
		want      recoverableFailureBudgetClass
		wantClass retryFailureClass
	}{
		{
			name:  "planner feedback",
			trace: plannerFeedbackTrace("", skills.MetaToolUpdatePlan, nil),
			want:  recoverableFailureBudgetPlanner, wantClass: retryFailurePlannerSchema,
		},
		{
			name: "invalid plan snapshot",
			trace: skills.SkillTrace{
				Kind: "plan_update", ToolName: skills.MetaToolUpdatePlan, Status: "error",
			},
			want: recoverableFailureBudgetPlanner, wantClass: retryFailurePlannerSchema,
		},
		{
			name: "pre-provider schema validation",
			trace: skills.SkillTrace{
				Kind: "tool_call", SkillID: skills.SkillConnectorRuntime, ToolName: "perform_task", Status: "error",
				Arguments: map[string]interface{}{"provider_request_sent": false},
			},
			want: recoverableFailureBudgetPlanner, wantClass: retryFailureArgumentValidation,
		},
		{
			name: "local argument schema validation",
			trace: skills.SkillTrace{
				Kind: "tool_call", SkillID: skills.SkillConnectorRuntime, ToolName: "perform_task", Status: "error",
				Arguments: map[string]interface{}{"failure_category": "arguments"},
			},
			want: recoverableFailureBudgetPlanner, wantClass: retryFailureArgumentValidation,
		},
		{
			name: "provider business failure",
			trace: skills.SkillTrace{
				Kind: "tool_call", SkillID: skills.SkillConnectorRuntime, ToolName: "perform_task", Status: "error",
				Arguments: map[string]interface{}{"provider_request_sent": true, "provider_error_code": "provider.failed"},
			},
			want: recoverableFailureBudgetBusiness, wantClass: retryFailureProviderBusiness,
		},
		{
			name:  "terminal protocol correction",
			trace: skills.SkillTrace{Kind: "final_answer", Status: "error"},
			want:  recoverableFailureBudgetPlanner, wantClass: retryFailureTerminalProtocol,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := recoverableSkillFailureBudget(test.trace); got != test.want {
				t.Fatalf("recoverableSkillFailureBudget() = %q, want %q", got, test.want)
			}
			if got := recoverableSkillFailureClass(test.trace); got != test.wantClass {
				t.Fatalf("recoverableSkillFailureClass() = %q, want %q", got, test.wantClass)
			}
		})
	}
}

func TestInvalidSkillToolArgumentsArePreProviderValidationFeedback(t *testing.T) {
	step := invalidSkillToolArgumentsStep("call-1", "ordinary-skill", "perform_task", []interface{}{"bad"}, ErrInvalidInput)
	if !step.recoverable {
		t.Fatal("invalid arguments were not recoverable")
	}
	if sent, ok := step.trace.Arguments["provider_request_sent"].(bool); !ok || sent {
		t.Fatalf("provider_request_sent = %#v, want explicit false", step.trace.Arguments["provider_request_sent"])
	}
	if got := recoverableSkillFailureClass(step.trace); got != retryFailureArgumentValidation {
		t.Fatalf("failure class = %q, want %q", got, retryFailureArgumentValidation)
	}
	if got := recoverableSkillFailureBudget(step.trace); got != recoverableFailureBudgetPlanner {
		t.Fatalf("failure budget = %q, want planner/non-business", got)
	}
}

func TestRetryFailureBudgetStateSeparatesAlternatingSchemaAndArgumentValidation(t *testing.T) {
	state := newRetryFailureBudgetState()
	rounds := defaultMaxPlannerSchemaFailureRounds - 1
	if argumentRounds := defaultMaxArgumentValidationFailureRounds - 1; argumentRounds < rounds {
		rounds = argumentRounds
	}
	for index := 0; index < rounds; index++ {
		schemaTrace := retryBudgetTestTrace(retryFailurePlannerSchema, index)
		state.recordCall(retryFailurePlannerSchema, "schema", schemaTrace)
		state.finishRound(
			map[retryFailureClass]bool{retryFailurePlannerSchema: true},
			map[retryFailureClass]bool{},
			false,
		)

		argumentTrace := retryBudgetTestTrace(retryFailureArgumentValidation, index)
		state.recordCall(retryFailureArgumentValidation, "arguments", argumentTrace)
		state.finishRound(
			map[retryFailureClass]bool{retryFailureArgumentValidation: true},
			map[retryFailureClass]bool{},
			false,
		)
	}

	if class, _, reached := state.limitReached(); reached {
		t.Fatalf("alternating independent failures exhausted %q budget", class)
	}
	schema := state.metricsFor(retryFailurePlannerSchema)
	arguments := state.metricsFor(retryFailureArgumentValidation)
	if schema.roundCount != rounds || schema.callCount != rounds {
		t.Fatalf("planner_schema metrics = %#v, want %d independent rounds/calls", schema, rounds)
	}
	if arguments.roundCount != rounds || arguments.callCount != rounds {
		t.Fatalf("argument_validation metrics = %#v, want %d independent rounds/calls", arguments, rounds)
	}
	if schema.categories["arguments"] != 0 || arguments.categories["schema"] != 0 {
		t.Fatalf("failure categories crossed budgets: schema=%#v arguments=%#v", schema.categories, arguments.categories)
	}

	state.recordCall(
		retryFailurePlannerSchema,
		"schema",
		retryBudgetTestTrace(retryFailurePlannerSchema, rounds),
	)
	state.finishRound(
		map[retryFailureClass]bool{retryFailurePlannerSchema: true},
		map[retryFailureClass]bool{},
		false,
	)
	if class, _, reached := state.limitReached(); !reached || class != retryFailurePlannerSchema {
		t.Fatalf("independent cumulative schema limit = (%q, %t), want planner_schema only", class, reached)
	}
	if arguments.roundCount != rounds || arguments.callCount != rounds {
		t.Fatalf("schema limit mutated argument budget: %#v", arguments)
	}
}

func TestRetryFailureBudgetStateSeparatesTerminalProtocolAndPlanner(t *testing.T) {
	state := newRetryFailureBudgetState()
	rounds := defaultMaxTerminalProtocolFailureRounds - 1
	if plannerRounds := defaultMaxPlannerSchemaFailureRounds - 1; plannerRounds < rounds {
		rounds = plannerRounds
	}
	for index := 0; index < rounds; index++ {
		state.recordStandaloneFailureForTest(
			retryFailureTerminalProtocol,
			"protocol",
			retryBudgetTestTrace(retryFailureTerminalProtocol, index),
		)
		state.recordCall(
			retryFailurePlannerSchema,
			"schema",
			retryBudgetTestTrace(retryFailurePlannerSchema, index),
		)
		state.finishRound(
			map[retryFailureClass]bool{retryFailurePlannerSchema: true},
			map[retryFailureClass]bool{},
			false,
		)
	}

	if class, _, reached := state.limitReached(); reached {
		t.Fatalf("terminal protocol and planner failures shared %q budget", class)
	}
	terminal := state.metricsFor(retryFailureTerminalProtocol)
	planner := state.metricsFor(retryFailurePlannerSchema)
	if terminal.roundCount != rounds || terminal.callCount != rounds {
		t.Fatalf("terminal_protocol metrics = %#v, want %d independent rounds/calls", terminal, rounds)
	}
	if planner.roundCount != rounds || planner.callCount != rounds {
		t.Fatalf("planner_schema metrics = %#v, want %d independent rounds/calls", planner, rounds)
	}

	state.recordStandaloneFailureForTest(
		retryFailureTerminalProtocol,
		"protocol",
		retryBudgetTestTrace(retryFailureTerminalProtocol, rounds),
	)
	if class, _, reached := state.limitReached(); !reached || class != retryFailureTerminalProtocol {
		t.Fatalf("independent cumulative terminal limit = (%q, %t), want terminal_protocol only", class, reached)
	}
	if planner.roundCount != rounds || planner.callCount != rounds {
		t.Fatalf("terminal limit mutated planner budget: %#v", planner)
	}
}

func TestRunnerAlternatingSchemaAndArgumentValidationFailuresDoNotShareLimit(t *testing.T) {
	responses := make([]*adapter.ChatResponse, 0, 2*(defaultMaxConsecutivePlannerSchemaFailureRounds-1)+1)
	for index := 0; index < defaultMaxConsecutivePlannerSchemaFailureRounds-1; index++ {
		responses = append(responses,
			runnerRetryBudgetToolResponse(adapter.ToolCall{
				ID:   "schema-" + evidenceStringFromAny(index),
				Type: "function",
				Function: adapter.FunctionCall{
					Name:      skills.MetaToolLoadSkill,
					Arguments: `{"skill_id":"test-skill"}`,
				},
			}),
			runnerRetryBudgetToolResponse(adapter.ToolCall{
				ID:   "arguments-" + evidenceStringFromAny(index),
				Type: "function",
				Function: adapter.FunctionCall{
					Name:      "unknown_business_action",
					Arguments: `[`,
				},
			}),
		)
	}
	responses = append(responses, &adapter.ChatResponse{Choices: []adapter.Choice{{Message: adapter.Message{
		Role: "assistant", Content: "已停止无效调用并返回安全结果。",
	}}}})

	client := fakeRunnerLLM(responses)
	var traces []skills.SkillTrace
	runner := &Runner{
		LLMClient: client, SkillRuntime: skills.NewRuntime(nil, nil), AppContext: &llmclient.AppContext{},
		OnTrace: func(_ []skills.SkillTrace, trace skills.SkillTrace) { traces = append(traces, trace) },
	}
	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared: NewPreparedChat("conv-independent-retry", "msg-independent-retry", "", "auto", &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "执行测试操作"}},
		}),
		Resolved:             runnerTestResolvedSkills(),
		NativeToolSet:        &skills.NativeToolSet{},
		RuntimeStateSnapshot: func() map[string]interface{} { return map[string]interface{}{} },
	})
	if err != nil || answer != "已停止无效调用并返回安全结果。" {
		t.Fatalf("Run() answer=%q error=%v", answer, err)
	}
	if client.appChatCalls != len(responses) {
		t.Fatalf("AppChat calls = %d, want %d independent retries plus final answer", client.appChatCalls, len(responses))
	}
	for _, trace := range traces {
		if trace.Kind == "tool_retry_guard" {
			t.Fatalf("alternating failure classes exhausted a shared budget: %#v", trace)
		}
	}
}

func TestCompleteSkillLoopSafetyTerminationSummarizesProviderConfirmedSuccess(t *testing.T) {
	state := nativeRunnerTestState("successful")
	toolSet := &skills.NativeToolSet{ToolBindings: map[string]skills.NativeToolBinding{
		"wecom_send_message": {
			SkillID: skills.SkillConnectorRuntime, ToolName: "send_message", ConnectorOperation: &skills.NativeConnectorOperation{ConnectorID: "wecom", OperationID: "wecom.message.send"},
		},
	}}
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{{
		Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "消息已发送给 Alice。"}}},
	}}}
	runner := &Runner{LLMClient: fakeLLM, AppContext: &llmclient.AppContext{}}
	prepared := NewPreparedChat("conv-recovered", "msg-recovered", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "给 Alice 发送企业微信消息"}},
	})
	var completion TerminalCompletionResult
	req := RunRequest{
		Prepared:             prepared,
		NativeToolSet:        toolSet,
		RuntimeStateSnapshot: func() map[string]interface{} { return state },
		OnTerminalCompletion: func(result TerminalCompletionResult) { completion = result },
	}
	failure := plannerFeedbackTrace("", skills.MetaToolUpdatePlan, nil)
	answer, _, err := runner.completeSkillLoopSafetyTermination(
		context.Background(),
		req,
		prepared,
		prepared.LLMRequest.Messages,
		nil,
		failure,
		nil,
		"planner feedback limit reached after provider success",
		6,
	)
	if err != nil {
		t.Fatalf("completeSkillLoopSafetyTermination() error = %v", err)
	}
	if answer != "消息已发送给 Alice。" {
		t.Fatalf("answer = %q, want model-generated success summary", answer)
	}
	if completion.Status != "pass" || completion.CompletionReason != connectorOutcomeRecoveryReason {
		t.Fatalf("completion = %#v, want provider-confirmed pass", completion)
	}
	if fakeLLM.appChatCalls != 1 {
		t.Fatalf("AppChat calls = %d, want one no-tool success summary", fakeLLM.appChatCalls)
	}
	request := fakeLLM.appChatRequests[0]
	if len(request.Tools) != 0 || request.ToolChoice != nil {
		t.Fatalf("success summary tools=%#v choice=%#v, want no tools", request.Tools, request.ToolChoice)
	}
	content := runnerTestMessageText(request.Messages)
	if strings.Contains(content, "The requested operation could not be completed") ||
		!strings.Contains(content, "native Connector execution") {
		t.Fatalf("success summary request used the wrong terminal path:\n%s", content)
	}

	// The same successful call is not the whole task while the model's
	// checklist still has steps open (ADR 0049).
	fakeLLM = &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{{
		Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "消息已发送，日程尚未创建。"}}},
	}}}
	runner = &Runner{LLMClient: fakeLLM, AppContext: &llmclient.AppContext{}, planSteps: []map[string]interface{}{
		{"step": "发送消息", "status": "completed"},
		{"step": "创建日程", "status": "pending"},
	}}
	completion = TerminalCompletionResult{}
	_, _, _ = runner.completeSkillLoopSafetyTermination(context.Background(), req, prepared, prepared.LLMRequest.Messages, nil, failure, nil, "planner feedback limit reached", 6)
	if completion.CompletionReason == connectorOutcomeRecoveryReason {
		t.Fatalf("completion = %#v, want no connector success while a step is open", completion)
	}
	for _, request := range fakeLLM.appChatRequests {
		if strings.Contains(runnerTestMessageText(request.Messages), "native Connector execution") {
			t.Fatal("an unfinished checklist took the connector success path")
		}
	}
}

func TestRunnerPlannerFeedbackUsesIndependentConsecutiveLimit(t *testing.T) {
	responses := make([]*adapter.ChatResponse, 0, defaultMaxConsecutivePlannerFeedbackRounds+1)
	for range defaultMaxConsecutivePlannerFeedbackRounds {
		responses = append(responses, &adapter.ChatResponse{Choices: []adapter.Choice{{Message: adapter.Message{
			Role: "assistant",
			ToolCalls: []adapter.ToolCall{{
				ID:   "legacy-load",
				Type: "function",
				Function: adapter.FunctionCall{
					Name:      skills.MetaToolLoadSkill,
					Arguments: `{"skill_id":"test-skill"}`,
				},
			}},
		}}}})
	}
	responses = append(responses, &adapter.ChatResponse{Choices: []adapter.Choice{{Message: adapter.Message{
		Role:    "assistant",
		Content: "当前规划未能形成可执行步骤，请稍后重试。",
	}}}})
	fakeLLM := &runnerTestLLMClient{appChatResponses: responses}
	var traces []skills.SkillTrace
	runner := &Runner{
		LLMClient: fakeLLM, SkillRuntime: skills.NewRuntime(nil, nil), AppContext: &llmclient.AppContext{},
		OnTrace: func(_ []skills.SkillTrace, trace skills.SkillTrace) { traces = append(traces, trace) },
	}
	prepared := NewPreparedChat("conv-planner-limit", "msg-planner-limit", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "执行测试操作"}},
	})
	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared: prepared, Resolved: runnerTestResolvedSkills(),
		NativeToolSet: &skills.NativeToolSet{}, RuntimeStateSnapshot: func() map[string]interface{} { return map[string]interface{}{} },
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "当前规划未能形成可执行步骤，请稍后重试。" {
		t.Fatalf("answer = %q", answer)
	}
	if fakeLLM.appChatCalls != defaultMaxConsecutivePlannerFeedbackRounds+1 {
		t.Fatalf("AppChat calls = %d, want %d planner rounds plus explanation", fakeLLM.appChatCalls, defaultMaxConsecutivePlannerFeedbackRounds+1)
	}
	var guard skills.SkillTrace
	for _, trace := range traces {
		if trace.Kind == "tool_retry_guard" {
			guard = trace
		}
	}
	if guard.Arguments["reason_code"] != "planner_feedback_limit_reached" {
		t.Fatalf("retry guard = %#v, want planner feedback limit", guard)
	}
	if categories := evidenceMapFromAny(guard.Arguments["failure_categories"]); len(categories) != 0 {
		t.Fatalf("planner feedback polluted business categories: %#v", categories)
	}
	classes := evidenceMapFromAny(guard.Arguments["retry_failure_class_counts"])
	if got := numericValue(classes[string(retryFailurePlannerSchema)]); got != defaultMaxConsecutivePlannerFeedbackRounds {
		t.Fatalf("planner_schema count = %d, want %d; classes=%#v", got, defaultMaxConsecutivePlannerFeedbackRounds, classes)
	}
	if got := numericValue(classes[string(retryFailureProviderBusiness)]); got != 0 {
		t.Fatalf("provider_business count = %d, want 0; classes=%#v", got, classes)
	}
}

func fakeRunnerLLM(responses []*adapter.ChatResponse) *runnerTestLLMClient {
	return &runnerTestLLMClient{appChatResponses: responses}
}

func retryBudgetTestTrace(class retryFailureClass, index int) skills.SkillTrace {
	return skills.SkillTrace{
		Kind:     string(class),
		ToolName: string(class),
		Status:   "error",
		Arguments: map[string]interface{}{
			"reason_code": string(class) + "_" + evidenceStringFromAny(index),
		},
	}
}

func runnerRetryBudgetToolResponse(call adapter.ToolCall) *adapter.ChatResponse {
	return &adapter.ChatResponse{Choices: []adapter.Choice{{
		Message:      adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{call}},
		FinishReason: "tool_calls",
	}}}
}

func nativeRunnerTestState(status string) map[string]interface{} {
	ref := nativeRunnerOperation()
	return map[string]interface{}{
		"operation_plan":    map[string]interface{}{"phases": []interface{}{nativeRunnerPhase(ref.PlanPhaseID, "in_progress")}},
		"skill_invocations": []interface{}{nativeOutcomeRecord(ref, "completed", status)},
	}
}

func mapsToInterfacesForRunnerRetryTest(values []map[string]interface{}) []interface{} {
	out := make([]interface{}, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func runnerTestMessageText(messages []adapter.Message) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		parts = append(parts, messageContent(message.Content))
	}
	return strings.Join(parts, "\n")
}

// recordStandaloneFailureForTest records one failed call as a round of its own.
func (s *retryFailureBudgetState) recordStandaloneFailureForTest(class retryFailureClass, category string, trace skills.SkillTrace) {
	s.recordCall(class, category, trace)
	s.finishRound(map[retryFailureClass]bool{class: true}, map[retryFailureClass]bool{}, false)
}
