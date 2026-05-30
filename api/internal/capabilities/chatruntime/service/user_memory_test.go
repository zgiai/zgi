package service

import (
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
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
