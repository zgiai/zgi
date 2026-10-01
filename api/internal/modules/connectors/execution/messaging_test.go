package execution

import (
	"context"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"testing"
)

func TestMessagingUsesNativeRiskConfirmationAndUnknownReceipt(t *testing.T) {
	defs, _ := catalog.MessagingDefinitions()
	registry, _ := catalog.New(defs...)
	resolver, err := NewTrustedRiskResolver(registry, messagingRiskPolicies())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range defs {
		for _, op := range d.Operations {
			owner := uuid.New()
			risk, err := resolver.ResolveRisk(context.Background(), RiskRequest{OrganizationID: uuid.New(), OwnerAccountID: owner, ActingPrincipalID: owner, ConnectionID: uuid.New(), ConnectorID: core.Identifier(d.ID), OperationID: core.Identifier(op.ID), Target: core.TargetRef{Type: core.TargetTypeChat, ID: uuid.NewString()}})
			want := RiskHigh
			if op.Effect == catalog.OperationEffectRead {
				want = RiskTrustedLow
			}
			if err != nil || risk != want {
				t.Fatal("risk", op.ID, err)
			}
			if want != RiskHigh {
				continue
			}
			for _, surface := range []core.TargetType{core.TargetTypeChat, core.TargetTypeAgent, core.TargetTypeWorkflow} {
				t.Run(op.ID+"/"+string(surface), func(t *testing.T) {
					makeHarness := func() *executionHarness {
						h := feishuExecutionHarness(t, surface, "feishu.message.send")
						r := &h.pinned.Request
						r.ConnectorID = core.Identifier(d.ID)
						r.OperationID = core.Identifier(op.ID)
						r.Input = map[string]any{"context_handle": "scoped-handle", "text": "approved message"}
						if op.ID == "dingtalk.messages.send_direct" {
							r.Input = map[string]any{"user_ids": []any{"test-user"}, "text": "approved message"}
						} else if op.ID == "dingtalk.messages.send_group" {
							r.Input = map[string]any{"conversation_id": "test-conversation", "text": "approved message"}
						}
						h.bindings.binding.ConnectorID = r.ConnectorID
						h.bindings.binding.Constraint = core.OwnerConstraint(r.OwnerAccountID, []core.Identifier{r.OperationID})
						c := &h.connections.connection
						c.ConnectorID = r.ConnectorID
						c.ProviderID = core.Identifier(d.ProviderID)
						c.DriverID = core.Identifier(d.DriverID)
						c.AuthMethodID = core.Identifier(d.AuthMethodID)
						c.UsableOperationIDs = []core.Identifier{r.OperationID}
						c.Constraint = core.OwnerConstraint(r.OwnerAccountID, c.UsableOperationIDs)
						h.service.catalog = &definitionCatalogStub{definition: d}
						h.risk.classification = RiskHigh
						return h
					}
					for _, gate := range []string{"owner", "binding", "operation", "unattended"} {
						h := makeHarness()
						switch gate {
						case "owner":
							h.pinned.Request.ActingPrincipalID = uuid.New()
						case "binding":
							h.bindings.err = core.NewError(core.ErrorCodeNotFound, false, nil)
						case "operation":
							h.connections.connection.UsableOperationIDs = nil
						case "unattended":
							h.pinned.Request.Interactive = false
						}
						if _, err := h.service.InvokeConnector(context.Background(), h.pinned); err == nil || h.provider.calls != 0 {
							t.Fatal("gate", gate)
						}
					}
					h := makeHarness()
					resume := frozenConfirmationRequest(t, h, ConfirmationDecisionConfirmed)
					h.provider.result = ProviderInvocationResult{Status: core.OutcomeStatusUnknown, FailureCode: core.ErrorCodeOutcomeUnknown}
					h.provider.err = core.NewError(core.ErrorCodeOutcomeUnknown, false, nil)
					_, _ = h.service.InvokeConnector(context.Background(), resume)
					_, err := h.service.InvokeConnector(context.Background(), resume)
					if err == nil || h.provider.calls != 1 {
						t.Fatal("unknown send replayed")
					}
				})
			}
		}
	}
}
