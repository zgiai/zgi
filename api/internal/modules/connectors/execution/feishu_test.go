package execution

import (
	"context"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"testing"
)

func TestFeishuRiskRegistryCoversOnlyImplementedOperations(t *testing.T) {
	d, err := catalog.FeishuDefinition()
	if err != nil {
		t.Fatal(err)
	}
	defs, _ := catalog.New(d)
	policies := feishuRiskPolicies()
	if len(policies) != len(d.Operations) {
		t.Fatal("unclassified operation")
	}
	r, err := NewTrustedRiskResolver(defs, policies)
	if err != nil {
		t.Fatal(err)
	}
	owner := uuid.New()
	for _, op := range d.Operations {
		got, err := r.ResolveRisk(context.Background(), RiskRequest{OrganizationID: uuid.New(), OwnerAccountID: owner, ActingPrincipalID: owner, ConnectionID: uuid.New(), ConnectorID: "feishu", OperationID: core.Identifier(op.ID), Target: core.TargetRef{Type: core.TargetTypeChat, ID: "chat"}})
		want := RiskHigh
		if op.Effect == catalog.OperationEffectRead {
			want = RiskTrustedLow
		}
		if err != nil || got != want {
			t.Fatalf("%s %s %v", op.ID, got, err)
		}
	}
}
func feishuExecutionHarness(t *testing.T, surface core.TargetType, operation string) *executionHarness {
	t.Helper()
	h := newExecutionHarness(t)
	d, err := catalog.FeishuDefinition()
	if err != nil {
		t.Fatal(err)
	}
	request := &h.pinned.Request
	request.ConnectorID = "feishu"
	request.OperationID = core.Identifier(operation)
	request.Target = core.TargetRef{Type: surface, ID: uuid.NewString()}
	if surface != core.TargetTypeChat {
		request.Target.Scope = core.ScopeDraft
	}
	if surface == core.TargetTypeWorkflow {
		request.Target.BindingInstanceID = "fixture-node"
	}
	request.Input = map[string]any{}
	if operation == "feishu.message.send" {
		request.Input = map[string]any{"receive_id": "ou_user", "receive_id_type": "open_id", "text": "hello"}
	}
	if operation == "feishu.contact.search" {
		request.Input = map[string]any{"query": "张三"}
	}
	h.bindings.binding.ConnectorID = "feishu"
	h.bindings.binding.Target = request.Target
	h.bindings.binding.Constraint = core.OwnerConstraint(request.OwnerAccountID, []core.Identifier{request.OperationID})
	c := &h.connections.connection
	c.ConnectorID = "feishu"
	c.ProviderID = "feishu"
	c.DriverID = "feishu-openapi-v1"
	c.AuthMethodID = "feishu.app_credentials"
	c.UsableOperationIDs = []core.Identifier{request.OperationID}
	c.Constraint = core.OwnerConstraint(request.OwnerAccountID, c.UsableOperationIDs)
	h.service.catalog = &definitionCatalogStub{definition: d}
	return h
}

func TestFeishuNameSearchUsesExistingNativeGatesOnAllSurfaces(t *testing.T) {
	for _, surface := range []core.TargetType{core.TargetTypeChat, core.TargetTypeAgent, core.TargetTypeWorkflow} {
		t.Run(string(surface), func(t *testing.T) {
			for _, state := range []string{"ready", "nonowner", "unbound", "unverified"} {
				t.Run(state, func(t *testing.T) {
					h := feishuExecutionHarness(t, surface, "feishu.contact.search")
					switch state {
					case "nonowner":
						h.pinned.Request.ActingPrincipalID = uuid.New()
					case "unbound":
						h.bindings.err = core.NewError(core.ErrorCodeNotFound, false, nil)
					case "unverified":
						h.connections.connection.UsableOperationIDs = nil
					}
					res, err := h.service.InvokeConnector(context.Background(), h.pinned)
					if state == "ready" {
						if err != nil || res.Outcome.Status != core.OutcomeStatusSuccessful || h.provider.calls != 1 {
							t.Fatalf("search not available through native execution: %v", err)
						}
					} else if err == nil || h.provider.calls != 0 || h.credentials.calls != 0 {
						t.Fatal("search bypassed owner, binding or verified-grant gate")
					}
				})
			}
		})
	}
}
func TestFeishuAllSurfacesUseSharedOwnerBindingAndConfirmationGates(t *testing.T) {
	for _, surface := range []core.TargetType{core.TargetTypeChat, core.TargetTypeAgent, core.TargetTypeWorkflow} {
		t.Run(string(surface), func(t *testing.T) {
			h := feishuExecutionHarness(t, surface, "feishu.bot.get")
			res, err := h.service.InvokeConnector(context.Background(), h.pinned)
			if err != nil || res.Outcome.Status != core.OutcomeStatusSuccessful || h.provider.calls != 1 {
				t.Fatalf("read %v", err)
			}
			for _, denial := range []string{"nonowner", "unbound", "unverified", "unattended"} {
				t.Run(denial, func(t *testing.T) {
					h := feishuExecutionHarness(t, surface, "feishu.message.send")
					h.risk.classification = RiskHigh
					switch denial {
					case "nonowner":
						h.pinned.Request.ActingPrincipalID = uuid.New()
					case "unbound":
						h.bindings.err = core.NewError(core.ErrorCodeNotFound, false, nil)
					case "unverified":
						h.connections.connection.UsableOperationIDs = nil
					case "unattended":
						h.pinned.Request.Interactive = false
					}
					_, err := h.service.InvokeConnector(context.Background(), h.pinned)
					if err == nil || h.provider.calls != 0 || h.credentials.calls != 0 {
						t.Fatalf("gate %v calls %d", err, h.provider.calls)
					}
				})
			}
			h = feishuExecutionHarness(t, surface, "feishu.message.send")
			resume := frozenConfirmationRequest(t, h, ConfirmationDecisionConfirmed)
			h.provider.result = ProviderInvocationResult{Status: core.OutcomeStatusUnknown, FailureCode: core.ErrorCodeOutcomeUnknown}
			h.provider.err = core.NewError(core.ErrorCodeOutcomeUnknown, false, nil)
			_, _ = h.service.InvokeConnector(context.Background(), resume)
			_, err = h.service.InvokeConnector(context.Background(), resume)
			if err == nil || h.provider.calls != 1 {
				t.Fatal("unknown replay submitted twice")
			}
		})
	}
}
