package service

import (
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func TestUserMemoryAccountIDFallsBackToConversation(t *testing.T) {
	accountID := uuid.New()
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{AccountID: accountID},
	}

	if got := userMemoryAccountID(prepared); got != accountID {
		t.Fatalf("userMemoryAccountID() = %s, want conversation account %s", got, accountID)
	}
}

func TestUserMemoryAccountIDPrefersScope(t *testing.T) {
	scopeAccountID := uuid.New()
	conversationAccountID := uuid.New()
	prepared := &PreparedChat{
		Scope:        Scope{AccountID: scopeAccountID},
		Conversation: &runtimemodel.Conversation{AccountID: conversationAccountID},
	}

	if got := userMemoryAccountID(prepared); got != scopeAccountID {
		t.Fatalf("userMemoryAccountID() = %s, want scope account %s", got, scopeAccountID)
	}
}

func TestCloneChatRequestPartsKeepsConnectorHostCoordinates(t *testing.T) {
	parts := &chatRequestParts{
		ConnectorTargetType:      connectorcore.TargetTypeDigitalHuman,
		ConnectorTargetScope:     "published",
		ConnectorTargetVersionID: uuid.NewString(),
		ConnectorRunID:           uuid.NewString(),
		ConnectorInteractive:     true,
	}

	clone := cloneChatRequestPartsForUserMemoryPreflight(parts)
	if clone == nil {
		t.Fatal("cloneChatRequestPartsForUserMemoryPreflight() = nil")
	}
	if clone.ConnectorTargetType != parts.ConnectorTargetType || clone.ConnectorTargetScope != parts.ConnectorTargetScope ||
		clone.ConnectorTargetVersionID != parts.ConnectorTargetVersionID || clone.ConnectorRunID != parts.ConnectorRunID ||
		clone.ConnectorInteractive != parts.ConnectorInteractive {
		t.Fatalf("clone lost Connector host coordinates: %#v", clone)
	}
}
