package relationshipmemory

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
)

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
