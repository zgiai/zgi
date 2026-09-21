package execution

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func TestFeishuTenantAndMembersUseNativeGatesOnEverySurface(t *testing.T) {
	for _, operation := range []string{"feishu.tenant.get", "feishu.chat.members.list"} {
		for _, surface := range []core.TargetType{core.TargetTypeChat, core.TargetTypeAgent, core.TargetTypeWorkflow} {
			for _, state := range []string{"ready", "unattended_read", "nonowner", "unbound", "unverified", "old_connection_constraint", "old_binding_constraint"} {
				t.Run(operation+"/"+string(surface)+"/"+state, func(t *testing.T) {
					h := feishuExecutionHarness(t, surface, operation)
					if operation == "feishu.chat.members.list" {
						h.pinned.Request.Input = map[string]any{"chat_id": "oc_fixture"}
					}
					switch state {
					case "unattended_read":
						h.pinned.Request.Interactive = false
					case "nonowner":
						h.pinned.Request.ActingPrincipalID = uuid.New()
					case "unbound":
						h.bindings.err = core.NewError(core.ErrorCodeNotFound, false, nil)
					case "unverified":
						h.connections.connection.UsableOperationIDs = nil
					case "old_connection_constraint":
						h.connections.connection.Constraint = core.OwnerConstraint(h.pinned.Request.OwnerAccountID, []core.Identifier{"feishu.bot.get"})
					case "old_binding_constraint":
						h.bindings.binding.Constraint = core.OwnerConstraint(h.pinned.Request.OwnerAccountID, []core.Identifier{"feishu.bot.get"})
					}
					result, err := h.service.InvokeConnector(context.Background(), h.pinned)
					if state == "ready" || state == "unattended_read" {
						if err != nil || result.Outcome.Status != core.OutcomeStatusSuccessful || h.provider.calls != 1 {
							t.Fatalf("authorized native read failed: err=%v calls=%d", err, h.provider.calls)
						}
					} else if err == nil || h.provider.calls != 0 || h.credentials.calls != 0 {
						t.Fatalf("native read bypassed %s: err=%v provider=%d credentials=%d", state, err, h.provider.calls, h.credentials.calls)
					}
				})
			}
		}
	}
}

func TestFeishuTenantAndMembersAreExplicitTrustedReads(t *testing.T) {
	definition, err := catalog.FeishuDefinition()
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := catalog.New(definition)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := NewTrustedRiskResolver(definitions, feishuRiskPolicies())
	if err != nil {
		t.Fatal(err)
	}
	owner := uuid.New()
	for _, operation := range []core.Identifier{"feishu.tenant.get", "feishu.chat.members.list"} {
		risk, err := resolver.ResolveRisk(t.Context(), RiskRequest{
			OrganizationID: uuid.New(), OwnerAccountID: owner, ActingPrincipalID: owner,
			ConnectionID: uuid.New(), ConnectorID: "feishu", OperationID: operation,
			Target: core.TargetRef{Type: core.TargetTypeChat, ID: "chat"},
		})
		if err != nil || risk != RiskTrustedLow {
			t.Fatalf("%s has no explicit trusted read policy: %v %v", operation, risk, err)
		}
	}
}
