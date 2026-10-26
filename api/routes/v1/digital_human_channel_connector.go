package v1

import (
	"context"

	"github.com/google/uuid"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	connectorconfirmation "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeconfirmation"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
)

// digitalHumanChannelConnectorConfirmer decides a connector confirmation from
// a channel reply through the same continuation and host projection as the
// console's confirmation card.
type digitalHumanChannelConnectorConfirmer struct {
	service  ConnectorRuntimeConfirmationService
	recorder ConnectorRuntimeConfirmationResolutionRecorder
}

func (confirmer digitalHumanChannelConnectorConfirmer) DecideChannelConnectorConfirmation(
	ctx context.Context,
	organizationID, ownerAccountID, receipt uuid.UUID,
	confirm bool,
) (digitalhuman.ChannelConnectorDecision, error) {
	decision := connectorexecution.ConfirmationDecisionCancelled
	if confirm {
		decision = connectorexecution.ConfirmationDecisionConfirmed
	}
	command := connectorconfirmation.Command{
		OrganizationID: organizationID,
		OwnerAccountID: ownerAccountID,
		RouteReceipt:   connectorexecution.RouteClaimReceipt(receipt.String()),
		Decision:       decision,
	}
	result, err := confirmer.service.Continue(ctx, command)
	if err != nil {
		return digitalhuman.ChannelConnectorDecision{}, err
	}
	resolution := result
	resolution.ExecutionResult = nil
	if err := confirmer.recorder.RecordConnectorConfirmationResolution(ctx, command, resolution); err != nil {
		return digitalhuman.ChannelConnectorDecision{}, err
	}
	outcome := digitalhuman.ChannelConnectorDecision{State: string(result.State), ErrorCode: string(result.ErrorCode)}
	if result.Presentation != nil {
		outcome.Title, outcome.Summary = result.Presentation.Title, result.Presentation.Summary
	}
	return outcome, nil
}
