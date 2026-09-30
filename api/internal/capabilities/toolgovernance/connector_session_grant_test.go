package toolgovernance

import (
	"testing"
	"time"
)

func TestMatchSessionGrantScopesConnectorConnectionAndBinding(t *testing.T) {
	expires := time.Now().Add(time.Hour)
	manifest := Manifest{
		ToolID: "qqmail.message.send", SkillID: "connector-runtime", Effect: EffectExternalSend,
		AssetType: "chat", RiskLevel: RiskLevelHigh, DataEgress: true,
		ExternalDestination: "mail", SensitiveDataAllowed: true,
	}
	request := Request{
		Manifest: manifest, ConversationID: "conversation-1", OrganizationID: "org-1", UserID: "user-1",
		SkillID: "connector-runtime", ProviderType: "connector", ProviderID: "qqmail",
		ConnectionID: "connection-1", BindingID: "binding-1", Assets: []AssetRef{{ID: "chat-1", Type: "chat"}},
		SessionGrants: []SessionGrant{{
			ConversationID: "conversation-1", OrganizationID: "org-1", UserID: "user-1",
			SkillID: "connector-runtime", ProviderType: "connector", ProviderID: "qqmail",
			ConnectionID: "connection-1", BindingID: "binding-1", ToolID: "qqmail.message.send",
			Effect: EffectExternalSend, AssetType: "chat", DataEgress: true,
			ExternalDestination: "mail", SensitiveDataAllowed: true, RiskLevel: RiskLevelHigh,
			Assets: []AssetRef{{ID: "chat-1", Type: "chat"}}, ExpiresAt: expires,
		}},
	}
	if _, ok := MatchSessionGrant(request, manifest, request.Assets); !ok {
		t.Fatal("matching connector session grant was rejected")
	}
	request.ConnectionID = "connection-2"
	if _, ok := MatchSessionGrant(request, manifest, request.Assets); ok {
		t.Fatal("grant for a different connection was accepted")
	}
}
