package v1

import (
	"context"

	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorconfirmation "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeconfirmation"
)

// connectorRuntimeConfirmationSurfaceRecorder keeps host projection outside
// Connector core. The frozen server-side origin is the only dispatch input;
// callers cannot select a surface in the continuation request.
type connectorRuntimeConfirmationSurfaceRecorder struct {
	chatAndAgent ConnectorRuntimeConfirmationResolutionRecorder
	workflow     ConnectorRuntimeConfirmationResolutionRecorder
}

func (recorder connectorRuntimeConfirmationSurfaceRecorder) RecordConnectorConfirmationResolution(
	ctx context.Context,
	command connectorconfirmation.Command,
	result connectorconfirmation.Result,
) error {
	var target ConnectorRuntimeConfirmationResolutionRecorder
	switch result.Origin.Surface {
	case connectorcore.TargetTypeChat, connectorcore.TargetTypeAgent, connectorcore.TargetTypeDigitalHuman:
		target = recorder.chatAndAgent
	case connectorcore.TargetTypeWorkflow:
		target = recorder.workflow
	default:
		return connectorcore.NewError(connectorcore.ErrorCodeInternal, false, nil)
	}
	if target == nil {
		return connectorcore.NewError(connectorcore.ErrorCodeInternal, false, nil)
	}
	return target.RecordConnectorConfirmationResolution(ctx, command, result)
}
