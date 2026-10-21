package relationshipmemory

import (
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
)

func TestAgentEndUserIdentityPreservesRegisteredRuntimeWorkspaceIssuer(t *testing.T) {
	runtimeWorkspaceID := uuid.New()
	registered := &usersubject.Subject{
		ID:         uuid.New(),
		Kind:       usersubject.APIEndUser,
		IssuerType: "runtime_workspace_api",
		IssuerID:   &runtimeWorkspaceID,
	}

	kind, issuerType, issuerID, err := agentEndUserIdentity(registered, uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	if kind != usersubject.APIEndUser || issuerType != "runtime_workspace_api" || issuerID == nil || *issuerID != runtimeWorkspaceID {
		t.Fatalf("resolved identity = (%q, %q, %v), want registered Runtime Workspace identity", kind, issuerType, issuerID)
	}
}

func TestAgentEndUserIdentityDefaultsToLegacyAgentIssuer(t *testing.T) {
	agentID := uuid.New()

	kind, issuerType, issuerID, err := agentEndUserIdentity(nil, agentID)
	if err != nil {
		t.Fatal(err)
	}
	if kind != usersubject.APIEndUser || issuerType != "agent_api" || issuerID == nil || *issuerID != agentID {
		t.Fatalf("resolved identity = (%q, %q, %v), want legacy Agent API identity", kind, issuerType, issuerID)
	}
}

func TestAgentEndUserIdentityRejectsRegisteredAccount(t *testing.T) {
	accountID := uuid.New()
	registered := &usersubject.Subject{ID: accountID, Kind: usersubject.Account, AccountID: &accountID}

	if _, _, _, err := agentEndUserIdentity(registered, uuid.New()); err == nil {
		t.Fatal("registered account was accepted as an API end user")
	}
}

func TestNormalizePublishedAgentMemoryContractBackfillsLegacyRevision(t *testing.T) {
	allSlots := []agentmemory.RuntimeSlot{
		{Key: "profile", MaxChars: 1200, Enabled: true},
		{Key: "preference", MaxChars: 800, Enabled: false, SortOrder: 1},
	}
	contract := publishedAgentMemoryContract{
		Enabled:            true,
		AutoCaptureEnabled: false,
		Slots:              append([]agentmemory.RuntimeSlot(nil), allSlots...),
	}
	wantRevision := agentmemory.ConfigRevision(true, false, allSlots)

	normalizePublishedAgentMemoryContract(&contract)

	if contract.ConfigRevision != wantRevision {
		t.Fatalf("legacy config revision = %q, want %q", contract.ConfigRevision, wantRevision)
	}
	if len(contract.Slots) != 1 || contract.Slots[0].Key != "profile" {
		t.Fatalf("runtime slots = %#v, want only enabled profile slot", contract.Slots)
	}
}

func TestNormalizePublishedAgentMemoryContractPreservesStoredRevision(t *testing.T) {
	contract := publishedAgentMemoryContract{
		Enabled:        true,
		ConfigRevision: " published-v1 ",
		Slots:          []agentmemory.RuntimeSlot{{Key: "profile", Enabled: true}},
	}

	normalizePublishedAgentMemoryContract(&contract)

	if contract.ConfigRevision != "published-v1" {
		t.Fatalf("config revision = %q, want stored revision", contract.ConfigRevision)
	}
}
