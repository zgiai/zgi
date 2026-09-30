package v1

import (
	"context"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/onboarding"
	"strings"
	"testing"
)

func (s *onboardingRouteFixture) Action(_ context.Context, c onboarding.ActionCommand) (onboarding.Snapshot, error) {
	s.org = c.OrganizationID
	s.owner = c.OwnerAccountID
	s.revision = c.ExpectedRevision
	return onboarding.Snapshot{ID: c.FlowID, State: onboarding.Awaiting, ProtocolVersion: 1, Step: "poll", Revision: c.ExpectedRevision + 1}, nil
}
func TestConnectorMessagingRoutesFreezeActorAndProtocol(t *testing.T) {
	s := &onboardingRouteFixture{}
	org, owner := uuid.New(), uuid.New()
	r := onboardingRouteRouter(t, s, org, owner)
	res := connectorRouteRequest(t, r, "POST", "/console/api/connectors/dingtalk/onboarding-flows", `{"intent":"register_bot","protocol_version":1}`, map[string]string{"Idempotency-Key": "fixture-key-0007"})
	if res.Code != 202 || s.command.ProtocolVersion != 1 || s.command.Intent != "register_bot" || s.command.OwnerAccountID != owner || s.command.OrganizationID != org {
		t.Fatal(res.Code, res.Body)
	}
	starts := s.starts
	for _, body := range []string{`{"intent":"register_bot","protocol_version":99}`, `{"intent":"register_bot","protocol_version":0}`, `{"intent":"register_bot","protocol_version":1,"source":"UNTRUSTED"}`, `{"intent":"register_bot","protocol_version":1,"owner_account_id":"fake"}`, `{"intent":"create_app","protocol_version":1}`, `{"intent":"register_bot","protocol_version":1,"region":"lark"}`} {
		res = connectorRouteRequest(t, r, "POST", "/console/api/connectors/dingtalk/onboarding-flows", body, nil)
		if res.Code != 400 || s.starts != starts {
			t.Fatal("untrusted protocol reached service")
		}
	}
	path := "/console/api/connectors/onboarding-flows/" + uuid.NewString() + "/actions"
	res = connectorRouteRequest(t, r, "POST", path, `{"action":"submit_code","expected_revision":7,"challenge_revision":2,"values":{"code":"123456"}}`, nil)
	if res.Code != 202 || s.revision != 7 || s.org != org || s.owner != owner || res.Header().Get("Cache-Control") != "no-store" || strings.Contains(res.Body.String(), "123456") {
		t.Fatal("unsafe challenge route", res.Code)
	}
	res = connectorRouteRequest(t, r, "POST", path, `{"action":"submit_code","expected_revision":7,"challenge_revision":2,"owner_account_id":"fake","values":{"code":"123456"}}`, nil)
	if res.Code != 400 {
		t.Fatal("spoofed action accepted")
	}
}
func TestMessagingCatalogDefaultsEnabledWithoutGrantingPermissions(t *testing.T) {
	d, _ := catalog.MessagingDefinition("dingtalk")
	handler := &connectorHandler{}
	connection := core.PersonalConnection{State: core.ReadinessReady, UsableOperationIDs: []core.Identifier{core.Identifier(d.Operations[0].ID)}, ProviderEvidence: &core.ProviderEvidence{Messaging: &core.MessagingEvidence{Operations: []core.OperationEvidence{{ID: d.Operations[0].ID, Availability: "conditional", Permission: "unknown"}}}}}
	out := handler.projectCatalog(d, []core.PersonalConnection{connection})
	if out.SetupAvailability == nil || !out.SetupAvailability.Enabled || !out.Capabilities[0].Usable || out.Capabilities[0].Availability != "conditional" || out.Capabilities[0].Permission != "unknown" || !handler.messagingMethod(d.ID, "qr_connect") {
		t.Fatal("default setup or conditional evidence changed")
	}
	for _, id := range []string{"dingtalk", "wecom", "weixin"} {
		definition, _ := catalog.MessagingDefinition(id)
		fresh := handler.projectCatalog(definition, nil)
		if fresh.SetupAvailability == nil || !fresh.SetupAvailability.Enabled || !handler.messagingMethod(id, "qr_connect") || !handler.messagingMethod(id, "qr_reauthorize") {
			t.Fatalf("%s requires deployment opt-in", id)
		}
		for _, capability := range fresh.Capabilities {
			if capability.Usable {
				t.Fatal("setup availability fabricated a verified connection")
			}
		}
		if handler.messagingMethod(id, "manual") != (id != "weixin") {
			t.Fatal("unsupported setup method")
		}
	}
}
