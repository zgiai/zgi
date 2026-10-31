package v1

import (
	"context"

	"github.com/google/uuid"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	connectorconfirmation "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeconfirmation"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
)

// digitalHumanChannelConnectorConfirmer decides a connector confirmation from
// a channel reply or an API call through the same continuation and host
// projection as the console's confirmation card: the paused turn continues
// with the operation's result, and its answer ends the run.
type digitalHumanChannelConnectorConfirmer struct {
	service      ConnectorRuntimeConfirmationService
	recorder     ConnectorRuntimeConfirmationResolutionRecorder
	digitalHuman ConnectorRuntimeConfirmationContinuer
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
	// The person has decided: the operation and its record must not depend on
	// the inbound request staying open, as on the console.
	ctx = context.WithoutCancel(ctx)
	result, err := confirmer.service.Continue(ctx, command)
	if err != nil {
		return digitalhuman.ChannelConnectorDecision{}, channelConnectorDecisionError(err)
	}
	continued, err := projectConnectorConfirmation(ctx, confirmer.digitalHuman, confirmer.recorder, command, result)
	if err != nil {
		return digitalhuman.ChannelConnectorDecision{}, channelConnectorDecisionError(err)
	}
	outcome := digitalhuman.ChannelConnectorDecision{State: string(result.State), ErrorCode: string(result.ErrorCode), Continued: continued}
	if result.Presentation != nil {
		outcome.Title, outcome.Summary = result.Presentation.Title, result.Presentation.Summary
	}
	return outcome, nil
}

// channelConnectorDecisionError reports a confirmation another reply already
// decided, or a turn already continued, as a run conflict: the first decision
// executed the operation once and its continuation delivers the answer.
func channelConnectorDecisionError(err error) error {
	if connectorcore.IsErrorCode(err, connectorcore.ErrorCodeConflict) {
		return digitalhuman.ErrRunConflict
	}
	return err
}
