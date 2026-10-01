package tools

import (
	"context"
	"testing"

	"github.com/google/uuid"
	connectortoolprovider "github.com/zgiai/zgi/api/internal/capabilities/connectortools"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectordispatch "github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	runtimetools "github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestExecuteRunFreezesWorkflowConnectorConfirmationAndPauses(t *testing.T) {
	f := newNativeWorkflowFixture(t, false, "github.user.get")
	f.expectGraph()
	f.expectBinding(f.binding, true)
	workflowID, _ := uuid.Parse(f.node.WorkflowID)
	ownerID, canonicalID := f.binding.OwnerAccountID, f.binding.ID
	nodeID := f.node.NodeID
	mock := f.mock
	receipt := connectorexecution.RouteClaimReceipt(uuid.NewString())
	confirmation := connectorexecution.RuntimeConfirmation{
		RouteReceipt: receipt, ConnectorID: "github", ConnectorName: "GitHub",
		OperationID: "github.user.get", ActionName: "List repositories",
		Target: f.binding.Target,
		Consequence: connectorexecution.RuntimeConsequence{
			ExternalEffect: "read", Description: "Reads repositories", DataEgress: true,
		},
		Risk: connectorexecution.RiskUnclassified,
	}
	acceptedOperation := workflowConfirmationAcceptedOperation(t, confirmation)
	confirmation.AcceptedOperation = &acceptedOperation
	node := f.node
	runID := uuid.NewString()
	node.GraphRuntimeState.VariablePool.Add([]string{"sys", "workflow_run_id"}, runID)
	node.nodeData.ToolParameters = map[string]*ToolInput{"query": {Value: "private workflow input", Type: "constant"}}
	capture := &workflowCatalogDispatchCapture{result: &connectorexecution.ExecutionResult{State: connectorexecution.ExecutionStateRequiresConfirmation, Confirmation: &confirmation}}
	node.connectorDispatcher = capture
	store := &workflowConnectorConfirmationStoreSpy{}
	node.connectorConfirmationStore = store

	result, err := node.executeRun(context.Background(), nil)
	if err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}
	if result == nil || result.Status != shared.PAUSED || len(result.Outputs) != 2 ||
		result.Outputs["state"] != string(connectorexecution.ExecutionStateRequiresConfirmation) {
		t.Fatalf("executeRun() result = %#v", result)
	}
	persisted, ok := result.Outputs["confirmation"].(connectorexecution.RuntimeConfirmation)
	if !ok || persisted.RouteReceipt != receipt || !persisted.Target.SameIdentity(confirmation.Target) {
		t.Fatalf("paused confirmation = %#v", result.Outputs["confirmation"])
	}
	if store.calls != 1 || store.frozen.RouteReceipt != receipt ||
		store.frozen.Origin.Surface != connectorcore.TargetTypeWorkflow ||
		store.frozen.Origin.WorkflowID != workflowID.String() || store.frozen.Origin.RunID != runID ||
		store.frozen.Origin.NodeID != nodeID {
		t.Fatalf("frozen continuation = %#v, calls = %d", store.frozen, store.calls)
	}
	if store.frozen.AcceptedOperation.ContractDigest != acceptedOperation.ContractDigest ||
		store.frozen.AcceptedOperation.PluginVersion != acceptedOperation.PluginVersion {
		t.Fatalf("frozen accepted operation = %#v", store.frozen.AcceptedOperation)
	}
	frozenInput, inputOK := store.frozen.Request.Input.(map[string]interface{})
	if !inputOK || frozenInput["query"] != "private workflow input" ||
		!store.frozen.Request.Target.SameIdentity(confirmation.Target) || store.frozen.Request.BindingID != canonicalID ||
		store.frozen.Request.OwnerAccountID != ownerID || !store.frozen.Request.Interactive {
		t.Fatalf("frozen request = %#v", store.frozen.Request)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("executeRun SQL expectations: %v", err)
	}
}

func TestWorkflowConnectorConfirmationFailsClosedWithoutStoreOrTypedMetadata(t *testing.T) {
	confirmation := connectorexecution.RuntimeConfirmation{
		RouteReceipt: uuidReceipt(), ConnectorID: "github", ConnectorName: "GitHub",
		OperationID: "github.issue.create", ActionName: "Create issue",
		Target:      connectorcore.TargetRef{Type: connectorcore.TargetTypeWorkflow, ID: uuid.NewString(), BindingInstanceID: "node-1"},
		Consequence: connectorexecution.RuntimeConsequence{ExternalEffect: "write"}, Risk: connectorexecution.RiskHigh,
	}
	acceptedOperation := workflowConfirmationAcceptedOperation(t, confirmation)
	confirmation.AcceptedOperation = &acceptedOperation
	identity := &connectordispatch.Identity{
		OrganizationID: uuid.New(), OwnerAccountID: uuid.New(), ActingPrincipalID: uuid.New(),
		BindingID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: confirmation.ConnectorID,
		Target: confirmation.Target, OperationID: confirmation.OperationID,
		LogicalOperationID: uuid.NewString(), CorrelationID: uuid.NewString(), Interactive: true,
	}
	identity.ActingPrincipalID = identity.OwnerAccountID
	result := &runtimetools.InvokeResult{Messages: []runtimetools.ToolInvokeMessage{{
		Type: runtimetools.ToolInvokeMessageTypeJSON,
		Meta: map[string]interface{}{
			connectortoolprovider.ResultKey: connectortoolprovider.ResultMetadata{
				State: connectorexecution.ExecutionStateRequiresConfirmation, Confirmation: &confirmation,
			},
		},
	}}}
	node := &Node{}
	if paused, handled, err := node.pauseForConnectorConfirmation(context.Background(), identity, map[string]interface{}{}, map[string]interface{}{
		"workflow_id": confirmation.Target.ID, "workflow_run_id": uuid.NewString(), "node_id": confirmation.Target.BindingInstanceID,
	}, result, nil); err == nil || !handled || paused != nil {
		t.Fatalf("missing store result = (%#v, %t, %v), want fail closed", paused, handled, err)
	}

	forged := &runtimetools.InvokeResult{Messages: []runtimetools.ToolInvokeMessage{{
		Type: runtimetools.ToolInvokeMessageTypeJSON,
		Meta: map[string]interface{}{
			connectortoolprovider.ResultKey: map[string]interface{}{"state": "requires_confirmation"},
		},
	}}}
	if _, found, err := workflowResultMetadata(forged); err == nil || !found {
		t.Fatalf("forged typed marker = (found %t, error %v), want fail closed", found, err)
	}
}

func TestWorkflowConnectorBlockedTerminalIsFailedAndPresentable(t *testing.T) {
	t.Parallel()

	decisionID := uuid.New()
	identity := &connectordispatch.Identity{
		OrganizationID: uuid.New(), OwnerAccountID: uuid.New(), ActingPrincipalID: uuid.New(),
		BindingID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: "github",
		Target: connectorcore.TargetRef{
			Type: connectorcore.TargetTypeWorkflow, ID: uuid.NewString(), BindingInstanceID: "node-1",
		},
		OperationID: "github.issue.create", LogicalOperationID: uuid.NewString(), CorrelationID: uuid.NewString(),
	}
	identity.ActingPrincipalID = identity.OwnerAccountID
	result := &runtimetools.InvokeResult{
		Success: false,
		Messages: []runtimetools.ToolInvokeMessage{{
			Type: runtimetools.ToolInvokeMessageTypeJSON,
			Data: map[string]interface{}{
				"state": string(connectorexecution.ExecutionStateBlocked), "decision_id": decisionID.String(),
			},
			Meta: map[string]interface{}{
				connectortoolprovider.ResultKey: connectortoolprovider.ResultMetadata{
					State: connectorexecution.ExecutionStateBlocked, DecisionID: decisionID,
				},
			},
		}},
	}
	node := &Node{nodeData: NodeData{
		ProviderID: "github", ProviderType: string(runtimetools.ToolProviderTypeConnector), ToolName: "create_issue",
	}}
	blocked, handled, err := node.pauseForConnectorConfirmation(
		context.Background(), identity, nil, nil, result,
		connectorcore.NewError(connectorcore.ErrorCodeConfirmationRequired, false, nil),
	)
	if err != nil || !handled || blocked == nil || blocked.Status != shared.FAILED ||
		blocked.Outputs["state"] != string(connectorexecution.ExecutionStateBlocked) ||
		blocked.Outputs["decision_id"] != decisionID.String() || len(blocked.Outputs) != 2 {
		t.Fatalf("blocked Workflow result = (%#v, %t, %v)", blocked, handled, err)
	}
	if node.connectorConfirmationStore != nil {
		t.Fatal("blocked Workflow result attempted to create a confirmation continuation")
	}
}

func uuidReceipt() connectorexecution.RouteClaimReceipt {
	return connectorexecution.RouteClaimReceipt(uuid.NewString())
}

func workflowConfirmationAcceptedOperation(
	t *testing.T,
	confirmation connectorexecution.RuntimeConfirmation,
) catalog.PinnedOperation {
	t.Helper()
	definition := catalog.Definition{
		ID: confirmation.ConnectorID.String(), PluginID: "github", PluginVersion: "1.0.0",
		SourceDialect: catalog.SourceDialectAgentPluginsV1, SourceSchemaVersion: catalog.SourceSchemaVersion1,
		ProviderID: confirmation.ConnectorID.String(), DriverID: "github_api", AuthMethodID: "personal_access_token",
		Name: confirmation.ConnectorName, Description: "GitHub Connector",
		Classification:  catalog.ClassificationConnector,
		CapabilityTypes: []catalog.CapabilityType{catalog.CapabilityTypeAction},
		SetupMethod:     catalog.SetupMethodKey, Ownership: catalog.OwnershipPersonal,
		Surfaces:  catalog.FirstReleaseSurfaces(),
		KeyFields: []catalog.KeyField{{Name: "token", Label: "Token", Required: true, Secret: true}},
		Operations: []catalog.Operation{{
			ID: confirmation.OperationID.String(), ToolName: "connector_action",
			Name: confirmation.ActionName,
		}},
	}
	digest, err := catalog.DefinitionContractDigest(definition)
	if err != nil {
		t.Fatal(err)
	}
	return catalog.PinnedOperation{
		ConnectorID: definition.ID, PluginID: definition.PluginID,
		PluginVersion: definition.PluginVersion, ContractDigest: digest,
		Definition: definition, Operation: definition.Operations[0],
	}
}

type workflowConnectorConfirmationStoreSpy struct {
	calls  int
	frozen connectorexecution.FrozenConfirmationRequest
}

func (store *workflowConnectorConfirmationStoreSpy) Freeze(
	_ context.Context,
	frozen connectorexecution.FrozenConfirmationRequest,
) (connectorexecution.ConfirmationContinuation, error) {
	store.calls++
	store.frozen = frozen
	return connectorexecution.ConfirmationContinuation{
		Frozen: frozen, State: connectorexecution.ConfirmationContinuationPending,
	}, nil
}

func (*workflowConnectorConfirmationStoreSpy) Load(
	context.Context, uuid.UUID, uuid.UUID, connectorexecution.RouteClaimReceipt,
) (connectorexecution.ConfirmationContinuation, error) {
	panic("unexpected Load")
}

func (*workflowConnectorConfirmationStoreSpy) RecordTerminal(
	context.Context, uuid.UUID, uuid.UUID, connectorexecution.RouteClaimReceipt, connectorexecution.ConfirmationTerminal,
) (connectorexecution.ConfirmationContinuation, error) {
	panic("unexpected RecordTerminal")
}
