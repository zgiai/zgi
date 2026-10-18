package skills

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	connectortools "github.com/zgiai/zgi/api/internal/capabilities/connectortools"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectordispatch "github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

// ConnectorConfirmationPending is the safe runtime-control projection handed
// to host loops after the complete provider input has been encrypted and
// frozen by the server. It contains no operation input or credential data.
type ConnectorConfirmationPending struct {
	Confirmation connectorexecution.RuntimeConfirmation
}

func blockedConnectorRuntimeMessages(messages []tools.ToolInvokeMessage) bool {
	if len(messages) != 1 {
		return false
	}
	metadata, typed := connectortools.ResultFromMessage(messages[0])
	return typed && metadata.State == connectorexecution.ExecutionStateBlocked &&
		metadata.DecisionID != uuid.Nil && metadata.Confirmation == nil
}

func freezeConnectorConfirmation(
	ctx context.Context,
	toolDef SkillToolDefinition,
	identity *connectordispatch.Identity,
	arguments map[string]interface{},
	execCtx ExecutionContext,
	messages []tools.ToolInvokeMessage,
) (*ConnectorConfirmationPending, bool, error) {
	if len(messages) != 1 {
		return nil, false, nil
	}
	metadata, typed := connectortools.ResultFromMessage(messages[0])
	if !typed || metadata.State != connectorexecution.ExecutionStateRequiresConfirmation {
		return nil, false, nil
	}
	if !toolDef.ConnectorNative || identity == nil || metadata.Confirmation == nil ||
		metadata.Confirmation.AcceptedOperation == nil || execCtx.ConnectorConfirmationStore == nil {
		return nil, true, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	confirmation := *metadata.Confirmation
	if !connectorConfirmationMatchesIdentity(confirmation, *identity) {
		return nil, true, core.NewError(core.ErrorCodeAccessDenied, false, nil)
	}
	origin, err := connectorConfirmationOrigin(execCtx, identity.Target)
	if err != nil {
		return nil, true, err
	}
	input, err := cloneConnectorConfirmationInput(arguments)
	if err != nil {
		return nil, true, err
	}
	request := connectorexecution.ExecutionRequest{
		ConversationID: execCtx.ConversationID,
		OrganizationID: identity.OrganizationID, OwnerAccountID: identity.OwnerAccountID,
		ActingPrincipalID: identity.ActingPrincipalID, BindingID: identity.BindingID,
		ConnectionID: identity.ConnectionID, ConnectorID: identity.ConnectorID,
		Target: identity.Target, OperationID: identity.OperationID,
		LogicalOperationID: identity.LogicalOperationID, CorrelationID: identity.CorrelationID,
		Interactive: identity.Interactive, Input: input,
	}
	acceptedOperation := *confirmation.AcceptedOperation
	continuation, err := execCtx.ConnectorConfirmationStore.Freeze(ctx, connectorexecution.FrozenConfirmationRequest{
		RouteReceipt: confirmation.RouteReceipt, Request: request, Origin: origin,
		AcceptedOperation: acceptedOperation, Preview: confirmation.Preview,
	})
	if err != nil {
		return nil, true, err
	}
	if continuation.State != connectorexecution.ConfirmationContinuationPending ||
		continuation.Frozen.RouteReceipt != confirmation.RouteReceipt {
		return nil, true, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	return &ConnectorConfirmationPending{Confirmation: confirmation}, true, nil
}

func connectorConfirmationMatchesIdentity(
	confirmation connectorexecution.RuntimeConfirmation,
	identity connectordispatch.Identity,
) bool {
	return identity.Interactive && confirmation.RouteReceipt != "" &&
		confirmation.ConnectorID == identity.ConnectorID &&
		confirmation.OperationID == identity.OperationID && confirmation.Target == identity.Target &&
		(confirmation.Risk == connectorexecution.RiskHigh || confirmation.Risk == connectorexecution.RiskUnclassified) &&
		strings.TrimSpace(confirmation.ConnectorName) != "" && strings.TrimSpace(confirmation.ActionName) != "" &&
		strings.TrimSpace(confirmation.Consequence.ExternalEffect) != ""
}

func connectorConfirmationOrigin(
	execCtx ExecutionContext,
	target core.TargetRef,
) (connectorexecution.ConfirmationOrigin, error) {
	origin := connectorexecution.ConfirmationOrigin{Surface: target.Type}
	switch target.Type {
	case core.TargetTypeChat:
		origin.ConversationID = strings.TrimSpace(execCtx.ConversationID)
		origin.MessageID = strings.TrimSpace(execCtx.MessageID)
	case core.TargetTypeAgent:
		origin.ConversationID = strings.TrimSpace(execCtx.ConversationID)
		origin.MessageID = strings.TrimSpace(execCtx.MessageID)
		origin.AgentID = strings.TrimSpace(execCtx.AppID)
	case core.TargetTypeDigitalHuman:
		// A DigitalHuman confirmation stays on its own surface: the host run and
		// the immutable target carry the identity, and the Agent/Workflow fields
		// stay empty so the frozen invocation cannot resume as a Chat or Agent
		// turn. The target binding instance is already pinned in the execution
		// request the host freezes.
		origin.RunID = strings.TrimSpace(execCtx.ConnectorRunID)
		origin.ConversationID = strings.TrimSpace(execCtx.ConversationID)
		origin.MessageID = strings.TrimSpace(execCtx.MessageID)
	case core.TargetTypeWorkflow:
		origin.WorkflowID = runtimeParameterString(execCtx.RuntimeParameters, "workflow_id")
		origin.RunID = runtimeParameterString(execCtx.RuntimeParameters, "workflow_run_id")
		origin.NodeID = runtimeParameterString(execCtx.RuntimeParameters, "node_id")
	default:
		return connectorexecution.ConfirmationOrigin{}, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	return origin, nil
}

func cloneConnectorConfirmationInput(arguments map[string]interface{}) (map[string]interface{}, error) {
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return nil, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	cloned := map[string]interface{}{}
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return nil, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	return cloned, nil
}
