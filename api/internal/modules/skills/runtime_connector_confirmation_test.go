package skills

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	toolprovider "github.com/zgiai/zgi/api/internal/capabilities/connectortools"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectordispatch "github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestConnectorConfirmationFreezesAuthoritativeInvocationBeforePendingResult(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		invokeFrom tools.ToolInvokeFrom
		targetType core.TargetType
	}{
		{name: "chat", invokeFrom: tools.ToolInvokeFromAIChat, targetType: core.TargetTypeChat},
		{name: "agent", invokeFrom: tools.ToolInvokeFromAgent, targetType: core.TargetTypeAgent},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			definition := connectorSkillTestDefinition()
			definition.Operations[0].InputSchema = map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{"type": "string"},
				},
				"required":             []interface{}{"query"},
				"additionalProperties": false,
			}
			operation := definition.Operations[0]
			router := &connectorConfirmationRouterSpy{
				receipt:  execution.RouteClaimReceipt(uuid.NewString()),
				accepted: mustConnectorSkillAcceptedOperation(t, definition, operation),
			}
			dispatcher, err := connectordispatch.New(router)
			if err != nil {
				t.Fatal(err)
			}
			provider, err := toolprovider.NewProvider(definition)
			if err != nil {
				t.Fatal(err)
			}
			manager := tools.NewToolManager(nil)
			if err := manager.RegisterProvider(provider); err != nil {
				t.Fatal(err)
			}
			runtime := NewRuntime(tools.NewToolEngine(manager), manager).WithConnectorDispatcher(dispatcher)
			resolver := &connectorIdentityResolverSpy{}
			store := &confirmationContinuationStoreSpy{}
			conversationID := uuid.NewString()
			agentID := uuid.NewString()
			execCtx := ExecutionContext{
				OrganizationID: uuid.NewString(), UserID: uuid.NewString(),
				ConversationID: conversationID, AppID: agentID, MessageID: uuid.NewString(),
				InvokeFrom: test.invokeFrom, ConnectorIdentityResolver: resolver, ConnectorActionResolver: newConnectorActionResolverForTest(),
				ConnectorConfirmationStore: store,
			}
			if test.invokeFrom == tools.ToolInvokeFromAgent {
				execCtx.RuntimeParameters = map[string]interface{}{"agent_runtime_source": "console"}
			}
			sentinel := "private-provider-input-sentinel"
			result, err := runtime.CallSkillTool(
				context.Background(), connectorSkillResolved(definition, operation, true),
				SkillConnectorRuntime, operation.ToolName, map[string]interface{}{"query": sentinel},
				execCtx, "call-confirmation-1",
			)
			if err != nil || result == nil || result.ConnectorConfirmation == nil {
				t.Fatalf("pending result = (%#v, %v)", result, err)
			}
			if result.Trace.Status != "pending_confirmation" || store.freezeCalls != 1 || router.calls != 1 {
				t.Fatalf("trace/store/router = %#v/%d/%d", result.Trace, store.freezeCalls, router.calls)
			}
			if strings.Contains(result.ToolMessage.Content.(string), sentinel) {
				t.Fatalf("model Tool message exposed frozen input: %s", result.ToolMessage.Content)
			}
			traceArguments := toJSONString(result.Trace.Arguments)
			if strings.Contains(traceArguments, sentinel) {
				t.Fatalf("Connector trace exposed frozen input: %s", traceArguments)
			}
			if result.Trace.Arguments["schema_bound_arguments_redacted"] != true ||
				result.Trace.Arguments["argument_count"] != 1 {
				t.Fatalf("Connector trace arguments = %#v", result.Trace.Arguments)
			}
			frozen := store.frozen
			if frozen.RouteReceipt != router.receipt || frozen.Request.Confirmation != nil ||
				frozen.Request.Input.(map[string]interface{})["query"] != sentinel || !frozen.Request.Interactive {
				t.Fatalf("frozen request = %#v", frozen)
			}
			if frozen.AcceptedOperation.ContractDigest != router.accepted.ContractDigest ||
				frozen.AcceptedOperation.PluginVersion != router.accepted.PluginVersion {
				t.Fatalf("frozen accepted operation = %#v", frozen.AcceptedOperation)
			}
			if frozen.Origin.Surface != test.targetType || frozen.Origin.MessageID != execCtx.MessageID {
				t.Fatalf("frozen origin = %#v", frozen.Origin)
			}
			if test.targetType == core.TargetTypeChat {
				if frozen.Origin.ConversationID != conversationID || frozen.Origin.AgentID != "" {
					t.Fatalf("chat origin = %#v", frozen.Origin)
				}
			} else if frozen.Origin.ConversationID != conversationID || frozen.Origin.AgentID != agentID {
				t.Fatalf("agent origin = %#v", frozen.Origin)
			}
		})
	}
}

func TestConnectorConfirmationWithoutContinuationStoreFailsClosed(t *testing.T) {
	t.Parallel()

	definition := connectorSkillTestDefinition()
	operation := definition.Operations[0]
	router := &connectorConfirmationRouterSpy{
		receipt:  execution.RouteClaimReceipt(uuid.NewString()),
		accepted: mustConnectorSkillAcceptedOperation(t, definition, operation),
	}
	dispatcher, err := connectordispatch.New(router)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := toolprovider.NewProvider(definition)
	if err != nil {
		t.Fatal(err)
	}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(provider); err != nil {
		t.Fatal(err)
	}
	_, err = NewRuntime(tools.NewToolEngine(manager), manager).WithConnectorDispatcher(dispatcher).CallSkillTool(
		context.Background(), connectorSkillResolved(definition, operation, true),
		SkillConnectorRuntime, operation.ToolName, map[string]interface{}{}, ExecutionContext{
			OrganizationID: uuid.NewString(), UserID: uuid.NewString(), ConversationID: uuid.NewString(),
			MessageID: uuid.NewString(), InvokeFrom: tools.ToolInvokeFromAIChat,
			ConnectorIdentityResolver: &connectorIdentityResolverSpy{}, ConnectorActionResolver: newConnectorActionResolverForTest(),
		}, "call-no-confirmation-store",
	)
	if !core.IsErrorCode(err, core.ErrorCodeInternal) || router.calls != 1 {
		t.Fatalf("missing continuation store error = %v, router calls = %d", err, router.calls)
	}
}

func TestUnattendedConnectorBlockedTerminalKeepsSafeHostProjection(t *testing.T) {
	t.Parallel()

	for _, invokeFrom := range []tools.ToolInvokeFrom{
		tools.ToolInvokeFromAIChat, tools.ToolInvokeFromAgent,
	} {
		invokeFrom := invokeFrom
		t.Run(string(invokeFrom), func(t *testing.T) {
			t.Parallel()

			definition := connectorSkillTestDefinition()
			operation := definition.Operations[0]
			decisionID := uuid.New()
			secret := "blocked-provider-private-value"
			router := &connectorBlockedRouterSpy{decisionID: decisionID, privateValue: secret}
			dispatcher, err := connectordispatch.New(router)
			if err != nil {
				t.Fatal(err)
			}
			provider, err := toolprovider.NewProvider(definition)
			if err != nil {
				t.Fatal(err)
			}
			manager := tools.NewToolManager(nil)
			if err := manager.RegisterProvider(provider); err != nil {
				t.Fatal(err)
			}
			execCtx := ExecutionContext{
				OrganizationID: uuid.NewString(), UserID: uuid.NewString(),
				ConversationID: uuid.NewString(), AppID: uuid.NewString(), MessageID: uuid.NewString(),
				InvokeFrom: invokeFrom, ConnectorIdentityResolver: &connectorIdentityResolverSpy{}, ConnectorActionResolver: newConnectorActionResolverForTest(),
			}
			result, invokeErr := NewRuntime(tools.NewToolEngine(manager), manager).WithConnectorDispatcher(dispatcher).CallSkillTool(
				context.Background(), connectorSkillResolved(definition, operation, true),
				SkillConnectorRuntime, operation.ToolName, map[string]interface{}{}, execCtx, "call-blocked",
			)
			if !core.IsErrorCode(invokeErr, core.ErrorCodeConfirmationRequired) || result == nil ||
				result.Trace.Status != "error" || result.Trace.ErrorCode != string(core.ErrorCodeConfirmationRequired) ||
				len(result.Messages) != 1 {
				t.Fatalf("blocked host result = (%#v, %v)", result, invokeErr)
			}
			metadata, ok := toolprovider.ResultFromMessage(result.Messages[0])
			if !ok || metadata.State != execution.ExecutionStateBlocked || metadata.DecisionID != decisionID {
				t.Fatalf("blocked host metadata = (%#v, %t)", metadata, ok)
			}
			serialized, marshalErr := json.Marshal(result.Messages)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if strings.Contains(string(serialized), secret) {
				t.Fatalf("blocked host projection exposed provider value: %s", serialized)
			}
			if router.calls != 1 {
				t.Fatalf("router calls = %d, want exactly one", router.calls)
			}
		})
	}
}

type connectorConfirmationRouterSpy struct {
	receipt  execution.RouteClaimReceipt
	accepted catalog.PinnedOperation
	request  execution.ExecutionRequest
	calls    int
}

type connectorBlockedRouterSpy struct {
	calls        int
	decisionID   uuid.UUID
	privateValue string
}

func (spy *connectorBlockedRouterSpy) Route(
	context.Context,
	execution.ExecutionRequest,
) (execution.ExecutionResult, error) {
	spy.calls++
	return execution.ExecutionResult{
		State: execution.ExecutionStateBlocked, DecisionID: spy.decisionID, Value: spy.privateValue,
	}, core.NewError(core.ErrorCodeConfirmationRequired, false, nil)
}

func (spy *connectorConfirmationRouterSpy) Route(
	_ context.Context,
	request execution.ExecutionRequest,
) (execution.ExecutionResult, error) {
	spy.calls++
	spy.request = request
	return execution.ExecutionResult{
		State: execution.ExecutionStateRequiresConfirmation,
		Confirmation: &execution.RuntimeConfirmation{
			RouteReceipt: spy.receipt,
			ConnectorID:  request.ConnectorID, ConnectorName: "Test Connector",
			OperationID: request.OperationID, ActionName: "Write test item",
			Target:      request.Target,
			Consequence: execution.RuntimeConsequence{ExternalEffect: "create", Description: "Creates a test item"},
			Risk:        execution.RiskHigh, AcceptedOperation: &spy.accepted,
		},
	}, nil
}

func mustConnectorSkillAcceptedOperation(
	t *testing.T,
	definition catalog.Definition,
	operation catalog.Operation,
) catalog.PinnedOperation {
	t.Helper()
	digest, err := catalog.DefinitionContractDigest(definition)
	if err != nil {
		t.Fatal(err)
	}
	return catalog.PinnedOperation{
		ConnectorID: definition.ID, PluginID: definition.PluginID,
		PluginVersion: definition.PluginVersion, PluginDigest: definition.PluginDigest,
		ContractDigest: digest, Definition: definition, Operation: operation,
	}
}

type confirmationContinuationStoreSpy struct {
	freezeCalls int
	frozen      execution.FrozenConfirmationRequest
}

func (spy *confirmationContinuationStoreSpy) Freeze(
	_ context.Context,
	frozen execution.FrozenConfirmationRequest,
) (execution.ConfirmationContinuation, error) {
	spy.freezeCalls++
	spy.frozen = frozen
	return execution.ConfirmationContinuation{
		Frozen: frozen,
		State:  execution.ConfirmationContinuationPending,
	}, nil
}

func (*confirmationContinuationStoreSpy) Load(
	context.Context,
	uuid.UUID,
	uuid.UUID,
	execution.RouteClaimReceipt,
) (execution.ConfirmationContinuation, error) {
	panic("unexpected Load")
}

func (*confirmationContinuationStoreSpy) RecordTerminal(
	context.Context,
	uuid.UUID,
	uuid.UUID,
	execution.RouteClaimReceipt,
	execution.ConfirmationTerminal,
) (execution.ConfirmationContinuation, error) {
	panic("unexpected RecordTerminal")
}
