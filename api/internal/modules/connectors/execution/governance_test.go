package execution

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func TestConnectorSessionGrantMatchesResolvedConnectionAndBinding(t *testing.T) {
	connectionID := uuid.New()
	bindingID := uuid.New()
	target := core.TargetRef{Type: core.TargetTypeChat, ID: uuid.NewString()}
	definition := catalog.Definition{ID: "qqmail"}
	operation := catalog.Operation{
		ID: "qqmail.message.send", SkillID: "connector-runtime",
		Effect: catalog.OperationEffectExternalSend, DataEgress: true,
		ExternalDestination: "mail", SensitiveDataAllowed: true,
	}
	request := ExecutionRequest{
		ConversationID: target.ID, OrganizationID: uuid.New(), OwnerAccountID: uuid.New(),
		BindingID: bindingID, ConnectionID: connectionID, ConnectorID: "qqmail", Target: target,
		OperationID: core.Identifier(operation.ID), SessionGrants: []toolgovernance.SessionGrant{{
			ConversationID: target.ID, OrganizationID: "", UserID: "",
			SkillID: "connector-runtime", ProviderType: "connector", ProviderID: "qqmail",
			ConnectionID: connectionID.String(), BindingID: bindingID.String(),
			ToolID: operation.ID, Effect: toolgovernance.EffectExternalSend, AssetType: "chat",
			DataEgress: true, ExternalDestination: "mail", SensitiveDataAllowed: true,
			RiskLevel: toolgovernance.RiskLevelHigh, ExpiresAt: time.Now().Add(time.Hour),
			Assets: []toolgovernance.AssetRef{{ID: target.ID, Type: "chat"}},
		}},
	}
	request.SessionGrants[0].OrganizationID = request.OrganizationID.String()
	request.SessionGrants[0].UserID = request.OwnerAccountID.String()
	if _, ok := connectorSessionGrantMatches(request, definition, operation, RiskHigh); !ok {
		t.Fatal("matching connector session grant was rejected")
	}
	request.BindingID = uuid.New()
	if _, ok := connectorSessionGrantMatches(request, definition, operation, RiskHigh); ok {
		t.Fatal("grant for a different binding was accepted")
	}
}
