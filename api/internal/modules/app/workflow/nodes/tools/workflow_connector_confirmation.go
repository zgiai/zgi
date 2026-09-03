package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	connectortools "github.com/zgiai/zgi/api/internal/capabilities/connectortools"
	workflowshared "github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectordispatch "github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	runtimetools "github.com/zgiai/zgi/api/internal/modules/tools"
)

// pauseForConnectorConfirmation consumes only the in-process typed Connector
// control marker. Provider JSON is never interpreted as a pause instruction.
// The complete operation input is frozen before the Workflow exposes the
// credential-free confirmation envelope to its durable pause projection.
func (n *Node) pauseForConnectorConfirmation(
	ctx context.Context,
	identity *connectordispatch.Identity,
	input map[string]interface{},
	runtimeParameters map[string]interface{},
	result *runtimetools.InvokeResult,
	invokeErr error,
) (*workflowshared.NodeRunResult, bool, error) {
	metadata, found, err := workflowResultMetadata(result)
	if err != nil || !found {
		return nil, found, err
	}
	if metadata.State == connectorexecution.ExecutionStateBlocked {
		if identity == nil || metadata.DecisionID == uuid.Nil ||
			!connectorcore.IsErrorCode(invokeErr, connectorcore.ErrorCodeConfirmationRequired) {
			return nil, true, connectorcore.NewError(connectorcore.ErrorCodeInternal, false, nil)
		}
		return &workflowshared.NodeRunResult{
			Status: workflowshared.FAILED,
			Outputs: map[string]interface{}{
				"state":       string(connectorexecution.ExecutionStateBlocked),
				"decision_id": metadata.DecisionID.String(),
			},
			Metadata: map[workflowshared.WorkflowNodeExecutionMetadataKey]interface{}{
				workflowshared.ToolInfo: map[string]interface{}{
					"provider_id": n.nodeData.ProviderID, "tool_name": n.nodeData.ToolName,
					"provider_type": n.nodeData.ProviderType, "system": n.nodeData.ProviderType,
				},
			},
		}, true, nil
	}
	if metadata.State != connectorexecution.ExecutionStateRequiresConfirmation {
		return nil, false, nil
	}
	if invokeErr != nil {
		return nil, true, connectorcore.NewError(connectorcore.ErrorCodeInternal, false, nil)
	}
	if identity == nil || metadata.Confirmation == nil ||
		metadata.Confirmation.AcceptedOperation == nil || n.connectorConfirmationStore == nil {
		return nil, true, connectorcore.NewError(connectorcore.ErrorCodeInternal, false, nil)
	}

	confirmation := *metadata.Confirmation
	if !workflowConnectorConfirmationMatchesIdentity(confirmation, *identity) {
		return nil, true, connectorcore.NewError(connectorcore.ErrorCodeAccessDenied, false, nil)
	}
	runID := workflowConnectorRuntimeString(runtimeParameters, "workflow_run_id")
	workflowID := workflowConnectorRuntimeString(runtimeParameters, "workflow_id")
	nodeID := workflowConnectorRuntimeString(runtimeParameters, "node_id")
	expectedWorkflowID := strings.TrimSpace(n.WorkflowID)
	if expectedWorkflowID == "" {
		expectedWorkflowID = identity.Target.ID
	}
	if runID == "" || workflowID != expectedWorkflowID || nodeID != identity.Target.BindingInstanceID {
		return nil, true, connectorcore.NewError(connectorcore.ErrorCodeAccessDenied, false, nil)
	}

	acceptedOperation := *confirmation.AcceptedOperation
	continuation, err := n.connectorConfirmationStore.Freeze(ctx, connectorexecution.FrozenConfirmationRequest{
		RouteReceipt: confirmation.RouteReceipt,
		Request: connectorexecution.ExecutionRequest{
			OrganizationID: identity.OrganizationID, OwnerAccountID: identity.OwnerAccountID,
			ActingPrincipalID: identity.ActingPrincipalID, BindingID: identity.BindingID,
			ConnectionID: identity.ConnectionID, ConnectorID: identity.ConnectorID,
			Target: identity.Target, OperationID: identity.OperationID,
			LogicalOperationID: identity.LogicalOperationID, CorrelationID: identity.CorrelationID,
			Interactive: identity.Interactive, Input: input,
		},
		Origin: connectorexecution.ConfirmationOrigin{
			Surface: connectorcore.TargetTypeWorkflow, WorkflowID: identity.Target.ID,
			RunID: runID, NodeID: identity.Target.BindingInstanceID,
		},
		AcceptedOperation: acceptedOperation,
	})
	if err != nil {
		return nil, true, err
	}
	if continuation.State != connectorexecution.ConfirmationContinuationPending ||
		continuation.Frozen.RouteReceipt != confirmation.RouteReceipt ||
		continuation.Frozen.Origin.Surface != connectorcore.TargetTypeWorkflow ||
		continuation.Frozen.Origin.WorkflowID != identity.Target.ID ||
		continuation.Frozen.Origin.RunID != runID ||
		continuation.Frozen.Origin.NodeID != identity.Target.BindingInstanceID {
		return nil, true, connectorcore.NewError(connectorcore.ErrorCodeInternal, false, nil)
	}

	return &workflowshared.NodeRunResult{
		Status: workflowshared.PAUSED,
		Outputs: map[string]interface{}{
			"state":        string(connectorexecution.ExecutionStateRequiresConfirmation),
			"confirmation": confirmation,
		},
		Metadata: map[workflowshared.WorkflowNodeExecutionMetadataKey]interface{}{
			workflowshared.ToolInfo: map[string]interface{}{
				"provider_id": n.nodeData.ProviderID, "tool_name": n.nodeData.ToolName,
				"provider_type": n.nodeData.ProviderType, "system": n.nodeData.ProviderType,
			},
		},
	}, true, nil
}

func workflowResultMetadata(
	result *runtimetools.InvokeResult,
) (connectortools.ResultMetadata, bool, error) {
	if result == nil {
		return connectortools.ResultMetadata{}, true,
			connectorcore.NewError(connectorcore.ErrorCodeInternal, false, nil)
	}
	var metadata connectortools.ResultMetadata
	found := false
	for _, message := range result.Messages {
		_, hasMarker := message.Meta[connectortools.ResultKey]
		if !hasMarker {
			continue
		}
		parsed, valid := connectortools.ResultFromMessage(message)
		if !valid || found {
			return connectortools.ResultMetadata{}, true,
				connectorcore.NewError(connectorcore.ErrorCodeInternal, false, nil)
		}
		metadata = parsed
		found = true
	}
	if found && metadata.State == connectorexecution.ExecutionStateRequiresConfirmation && len(result.Messages) != 1 {
		return connectortools.ResultMetadata{}, true,
			connectorcore.NewError(connectorcore.ErrorCodeInternal, false, nil)
	}
	return metadata, found, nil
}

func workflowConnectorConfirmationMatchesIdentity(
	confirmation connectorexecution.RuntimeConfirmation,
	identity connectordispatch.Identity,
) bool {
	return identity.Interactive && confirmation.RouteReceipt != "" &&
		confirmation.ConnectorID == identity.ConnectorID && confirmation.OperationID == identity.OperationID &&
		confirmation.Target == identity.Target &&
		(confirmation.Risk == connectorexecution.RiskHigh || confirmation.Risk == connectorexecution.RiskUnclassified) &&
		strings.TrimSpace(confirmation.ConnectorName) != "" && strings.TrimSpace(confirmation.ActionName) != "" &&
		strings.TrimSpace(confirmation.Consequence.ExternalEffect) != ""
}

func cloneWorkflowConnectorInput(input map[string]interface{}) (map[string]interface{}, error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return nil, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	cloned := map[string]interface{}{}
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return nil, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	return cloned, nil
}

func workflowConnectorRuntimeString(values map[string]interface{}, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}
