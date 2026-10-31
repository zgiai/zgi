package skillloop

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/connectortools"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func nativeRunnerOperation() connectortools.OperationRef {
	return connectortools.OperationRef{ConnectorID: "wecom", OperationID: "wecom.message.send", BindingID: uuid.New(), ConnectionID: uuid.New(), Target: core.TargetRef{Type: core.TargetTypeChat, ID: uuid.NewString()}, LogicalOperationID: uuid.NewString(), ActionStepID: uuid.NewString(), PlanPhaseID: "phase-send"}
}
func nativeRunnerExpectedAction() map[string]any {
	return map[string]any{"skill_id": skills.SkillConnectorRuntime, "tool_name": "send_message", "connector_operation": map[string]any{"connector_id": "wecom", "operation_id": "wecom.message.send"}}
}
func nativeRunnerPhase(id, status string) map[string]any {
	return map[string]any{"id": id, "step": "Send message", "status": status, "expected_action": nativeRunnerExpectedAction()}
}

type nativeRunnerDispatcher struct {
	calls               int
	state               map[string]any
	binding, connection uuid.UUID
}

func (d *nativeRunnerDispatcher) ResolveConnectorIdentity(_ context.Context, r dispatch.IdentityResolutionRequest) (dispatch.Identity, error) {
	return dispatch.Identity{OrganizationID: r.OrganizationID, OwnerAccountID: r.OwnerAccountID, ActingPrincipalID: r.ActingPrincipalID, ConnectorID: r.ConnectorID, OperationID: r.OperationID, BindingID: d.binding, ConnectionID: d.connection, Target: r.Target, LogicalOperationID: r.LogicalOperationID, CorrelationID: r.CorrelationID, Interactive: r.Interactive}, nil
}
func (d *nativeRunnerDispatcher) Dispatch(_ context.Context, inv dispatch.Invocation) (execution.ExecutionResult, error) {
	d.calls++
	ref := connectortools.OperationRef{ConnectorID: inv.Identity.ConnectorID, OperationID: inv.Identity.OperationID, BindingID: inv.Identity.BindingID, ConnectionID: inv.Identity.ConnectionID, Target: inv.Identity.Target, LogicalOperationID: inv.Identity.LogicalOperationID, ActionStepID: inv.Identity.ActionStepID, PlanPhaseID: "phase-send"}
	outcomeID := uuid.New()
	record := nativeOutcomeRecord(ref, "completed", "successful")
	evidenceMapFromAny(record["result"])["outcome_id"] = outcomeID.String()
	d.state["skill_invocations"] = []any{record}
	return execution.ExecutionResult{State: execution.ExecutionStateCompleted, Outcome: core.OperationOutcome{ID: outcomeID, Status: core.OutcomeStatusSuccessful}, Value: map[string]any{"sent": true}}, nil
}

func TestRunnerNativeSuccessSurvivesLaterPlannerFeedbackLimit(t *testing.T) {
	state := map[string]any{"operation_plan": map[string]any{"phases": []any{nativeRunnerPhase("phase-send", "pending")}}}
	dispatcher := &nativeRunnerDispatcher{state: state, binding: uuid.New(), connection: uuid.New()}
	runtime := skills.NewRuntime(nil, nil).WithConnectorDispatcher(dispatcher)
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{Metadata: skills.SkillMetadata{ID: skills.SkillConnectorRuntime, TimeoutSeconds: 5}, Tools: []skills.SkillToolDefinition{{Name: "send_message", ProviderType: tools.ToolProviderTypeConnector, ProviderID: "wecom", InputSchema: map[string]any{"type": "object", "properties": map[string]any{}}, ConnectorNative: true, ConnectorOperationID: "wecom.message.send"}}}}}
	set := runtime.BuildNativeToolSet(context.Background(), resolved, skills.NativeToolSetOptions{BudgetChars: 10000})
	alias := ""
	for name, binding := range set.ToolBindings {
		if binding.ToolName == "send_message" {
			alias = name
		}
	}
	if alias == "" {
		t.Fatal("native operation was not projected")
	}
	responses := []*adapter.ChatResponse{runnerRetryBudgetToolResponse(adapter.ToolCall{ID: "send", Type: "function", Function: adapter.FunctionCall{Name: alias, Arguments: `{}`}})}
	for i := 0; i < defaultMaxConsecutivePlannerFeedbackRounds; i++ {
		responses = append(responses, runnerRetryBudgetToolResponse(adapter.ToolCall{ID: uuid.NewString(), Type: "function", Function: adapter.FunctionCall{Name: skills.MetaToolUpdatePlan, Arguments: `{}`}}))
	}
	responses = append(responses, &adapter.ChatResponse{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "消息已发送。"}, FinishReason: "stop"}}})
	client := fakeRunnerLLM(responses)
	runner := &Runner{LLMClient: client, SkillRuntime: runtime, AppContext: &llmclient.AppContext{}}
	ledger := dispatch.ActionLedger{}
	var completion TerminalCompletionResult
	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared: NewPreparedChat(uuid.NewString(), uuid.NewString(), "", "auto", &adapter.ChatRequest{Messages: []adapter.Message{{Role: "user", Content: "发送消息"}}}), Resolved: resolved, NativeToolSet: &set,
		ExecutionContext: skills.ExecutionContext{OrganizationID: uuid.NewString(), UserID: uuid.NewString(), ConversationID: uuid.NewString(), MessageID: uuid.NewString(), InvokeFrom: tools.ToolInvokeFromAIChat, ConnectorIdentityResolver: dispatcher, ConnectorActionResolver: func(_ context.Context, id dispatch.Identity, input any) (dispatch.Identity, error) {
			id.PlanPhaseID = "phase-send"
			step, _, err := dispatch.ResolveActionPlanStep(&ledger, []dispatch.PlanPhaseRef{{ID: "phase-send", ConnectorID: id.ConnectorID, OperationID: id.OperationID}}, "phase-send", id)
			if err != nil {
				return dispatch.Identity{}, err
			}
			committed, _, err := dispatch.ResolveActionInstance(&ledger, step, id.CorrelationID, id, input)
			return committed, err
		}},
		RuntimeStateSnapshot: func() map[string]any { return state }, OnTerminalCompletion: func(result TerminalCompletionResult) { completion = result },
	})
	if err != nil || answer != "消息已发送。" || dispatcher.calls != 1 || completion.CompletionReason != connectorOutcomeRecoveryReason || client.appChatCalls != defaultMaxConsecutivePlannerFeedbackRounds+2 {
		t.Fatalf("answer=%q err=%v dispatches=%d completion=%#v model=%d", answer, err, dispatcher.calls, completion, client.appChatCalls)
	}
}
