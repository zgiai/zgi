package v1

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	connectorconfirmation "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeconfirmation"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
)

type connectorRuntimeConfirmationContinuerFunc func(context.Context, connectorconfirmation.Command, connectorconfirmation.Result) error

func (continuer connectorRuntimeConfirmationContinuerFunc) ResumeAfterConnectorConfirmation(
	ctx context.Context,
	command connectorconfirmation.Command,
	result connectorconfirmation.Result,
) error {
	return continuer(ctx, command, result)
}

// A channel reply (and the API) continues a DigitalHuman turn with the
// operation's result, as the console does, instead of only recording it.
func TestChannelConnectorConfirmerContinuesDigitalHumanTurn(t *testing.T) {
	service := &connectorRuntimeConfirmationServiceSpy{result: connectorconfirmation.Result{
		State: connectorconfirmation.StateCompleted, DecisionID: uuid.New(),
		Origin: connectorexecution.ConfirmationOrigin{Surface: connectorcore.TargetTypeDigitalHuman},
		ExecutionResult: &connectorexecution.ExecutionResult{
			State: connectorexecution.ExecutionStateCompleted,
			Value: map[string]any{"stdout": "active (running)"},
		},
	}}
	continuer := &connectorRuntimeConfirmationContinuerSpy{}
	confirmer := digitalHumanChannelConnectorConfirmer{service: service, recorder: service, digitalHuman: continuer}

	owner, receipt := uuid.New(), uuid.New()
	decision, err := confirmer.DecideChannelConnectorConfirmation(t.Context(), uuid.New(), owner, receipt, true)
	if err != nil {
		t.Fatalf("DecideChannelConnectorConfirmation() error = %v", err)
	}
	if !decision.Continued || decision.State != string(connectorconfirmation.StateCompleted) {
		t.Fatalf("decision = %+v, want a continued completed decision", decision)
	}
	if len(service.calls) != 1 || service.calls[0].OwnerAccountID != owner ||
		service.calls[0].RouteReceipt != connectorexecution.RouteClaimReceipt(receipt.String()) ||
		service.calls[0].Decision != connectorexecution.ConfirmationDecisionConfirmed {
		t.Fatalf("Continue calls = %#v", service.calls)
	}
	if len(service.recordCalls) != 0 {
		t.Fatalf("a DigitalHuman decision was only recorded: %#v", service.recordCalls)
	}
	if len(continuer.results) != 1 || continuer.results[0].ExecutionResult == nil {
		t.Fatalf("continuation did not receive the operation result: %#v", continuer.results)
	}
}

// A second reply to a decided confirmation finds the turn already claimed:
// it is a run conflict, so the channel stays quiet instead of failing.
func TestChannelConnectorConfirmerReportsAContinuedTurnAsConflict(t *testing.T) {
	service := &connectorRuntimeConfirmationServiceSpy{result: connectorconfirmation.Result{
		State:  connectorconfirmation.StateCompleted,
		Origin: connectorexecution.ConfirmationOrigin{Surface: connectorcore.TargetTypeDigitalHuman},
	}}
	claimed := false
	continuer := connectorRuntimeConfirmationContinuerFunc(func(context.Context, connectorconfirmation.Command, connectorconfirmation.Result) error {
		if claimed {
			return connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
		}
		claimed = true
		return nil
	})
	confirmer := digitalHumanChannelConnectorConfirmer{service: service, recorder: service, digitalHuman: continuer}
	if _, err := confirmer.DecideChannelConnectorConfirmation(t.Context(), uuid.New(), uuid.New(), uuid.New(), true); err != nil {
		t.Fatalf("first decision error = %v", err)
	}
	if _, err := confirmer.DecideChannelConnectorConfirmation(t.Context(), uuid.New(), uuid.New(), uuid.New(), true); !errors.Is(err, digitalhuman.ErrRunConflict) {
		t.Fatalf("repeated decision error = %v, want ErrRunConflict", err)
	}

	service.err = connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
	if _, err := confirmer.DecideChannelConnectorConfirmation(t.Context(), uuid.New(), uuid.New(), uuid.New(), false); !errors.Is(err, digitalhuman.ErrRunConflict) {
		t.Fatalf("contradicting decision error = %v, want ErrRunConflict", err)
	}
}
