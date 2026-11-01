package v1

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
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

type channelPreviewStatusSpy struct {
	*connectorRuntimeConfirmationServiceSpy
	status connectorconfirmation.StatusResult
	query  connectorconfirmation.StatusQuery
}

func (spy *channelPreviewStatusSpy) ReadStatus(_ context.Context, query connectorconfirmation.StatusQuery) (connectorconfirmation.StatusResult, error) {
	spy.query = query
	return spy.status, nil
}

// The card asking about a confirmation shows the details it was frozen with,
// read through the console's owner and requester checks.
func TestChannelConnectorPreviewReadsTheFrozenDetailsOfAPendingConfirmation(t *testing.T) {
	org, owner, receipt := uuid.New(), uuid.New(), uuid.New()
	spy := &channelPreviewStatusSpy{
		connectorRuntimeConfirmationServiceSpy: &connectorRuntimeConfirmationServiceSpy{},
		status: connectorconfirmation.StatusResult{Resolution: "unresolved", Presentation: &connectorconfirmation.Presentation{
			Version: 1, Preview: &connectorexecution.ConfirmationPreview{
				AccountIdentity: "ops@example.com",
				Fields: []connectorexecution.ConfirmationPreviewField{
					{Label: "Command", LabelI18n: catalog.LocalizedText{"zh-Hans": "命令"}, Value: "uptime", Format: connectorexecution.ConfirmationPreviewFormatCommand},
					{Label: "Body", Value: "long", Truncated: true},
				},
			},
		}},
	}
	confirmer := digitalHumanChannelConnectorConfirmer{service: spy}
	fields, err := confirmer.ChannelConnectorConfirmationPreview(t.Context(), org, owner, receipt)
	if err != nil || len(fields) != 3 {
		t.Fatalf("fields = %#v, %v", fields, err)
	}
	if fields[0].Label != "使用账号" || fields[0].Value != "ops@example.com" ||
		fields[1].Label != "命令" || !fields[1].Code || fields[2].Label != "Body" || !fields[2].Truncated {
		t.Fatalf("fields = %#v", fields)
	}
	if spy.query.PresentationVersion != 1 || spy.query.OrganizationID != org || spy.query.OwnerAccountID != owner ||
		string(spy.query.RouteReceipt) != receipt.String() {
		t.Fatalf("status query = %#v", spy.query)
	}

	spy.status.Resolution = "resolved"
	if fields, err := confirmer.ChannelConnectorConfirmationPreview(t.Context(), org, owner, receipt); err != nil || fields != nil {
		t.Fatalf("a decided confirmation still showed details: %#v, %v", fields, err)
	}
}
