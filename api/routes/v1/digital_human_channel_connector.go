package v1

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
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

// ChannelConnectorConfirmationPreview reads the details a pending
// confirmation was frozen with, through the same owner and requester checks
// as the console's status read, for the card that asks about it.
func (confirmer digitalHumanChannelConnectorConfirmer) ChannelConnectorConfirmationPreview(
	ctx context.Context,
	organizationID, ownerAccountID, receipt uuid.UUID,
) ([]channelruntime.DeliveryCardField, error) {
	status, err := confirmer.service.ReadStatus(ctx, connectorconfirmation.StatusQuery{
		PresentationVersion: 1, OrganizationID: organizationID, OwnerAccountID: ownerAccountID,
		RouteReceipt: connectorexecution.RouteClaimReceipt(receipt.String()),
	})
	if err != nil || status.Resolution != "unresolved" || status.Presentation == nil || status.Presentation.Preview == nil {
		return nil, err
	}
	preview := status.Presentation.Preview
	fields := make([]channelruntime.DeliveryCardField, 0, len(preview.Fields)+1)
	if account := strings.TrimSpace(preview.AccountIdentity); account != "" {
		fields = append(fields, channelruntime.DeliveryCardField{Label: "使用账号", Value: account})
	}
	for _, field := range preview.Fields {
		label := strings.TrimSpace(field.LabelI18n["zh-Hans"])
		if label == "" {
			label = strings.TrimSpace(field.Label)
		}
		fields = append(fields, channelruntime.DeliveryCardField{
			Label: label, Value: field.Value, Truncated: field.Truncated,
			Code: field.Format == connectorexecution.ConfirmationPreviewFormatCommand,
		})
	}
	return fields, nil
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
