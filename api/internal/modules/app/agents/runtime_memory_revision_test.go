package agents

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
)

func TestAgentMemoryConfigRevisionIgnoresPersistenceMetadata(t *testing.T) {
	base := []dto.AgentMemorySlotConfig{{
		ID: "slot-v1", Key: "profile", Name: "Profile", Description: "Stable user profile",
		MaxChars: 1200, Enabled: true, SortOrder: 2,
		CreatedAt: 10, UpdatedAt: 20, CreatedAtUnix: 10, UpdatedAtUnix: 20,
		CreatedAtISO: "created-v1", UpdatedAtISO: "updated-v1",
		CreatedAtDisplay: "created display v1", UpdatedAtDisplay: "updated display v1",
	}}
	metadataOnlyChange := append([]dto.AgentMemorySlotConfig(nil), base...)
	metadataOnlyChange[0].ID = "slot-v2"
	metadataOnlyChange[0].CreatedAt = 30
	metadataOnlyChange[0].UpdatedAt = 40
	metadataOnlyChange[0].CreatedAtUnix = 30
	metadataOnlyChange[0].UpdatedAtUnix = 40
	metadataOnlyChange[0].CreatedAtISO = "created-v2"
	metadataOnlyChange[0].UpdatedAtISO = "updated-v2"
	metadataOnlyChange[0].CreatedAtDisplay = "created display v2"
	metadataOnlyChange[0].UpdatedAtDisplay = "updated display v2"

	baseRevision := agentMemoryConfigRevision(true, true, base)
	if got := agentMemoryConfigRevision(true, true, metadataOnlyChange); got != baseRevision {
		t.Fatalf("metadata-only revision = %q, want %q", got, baseRevision)
	}
}

func TestAgentMemoryConfigRevisionTracksRuntimeSemantics(t *testing.T) {
	base := []dto.AgentMemorySlotConfig{{
		Key: "profile", Name: "Profile", Description: "Stable user profile",
		MaxChars: 1200, Enabled: true, SortOrder: 2,
	}}
	baseRevision := agentMemoryConfigRevision(true, true, base)
	tests := []struct {
		name      string
		enabled   bool
		automatic bool
		mutate    func(*dto.AgentMemorySlotConfig)
	}{
		{name: "agent enabled", enabled: false, automatic: true},
		{name: "automatic maintenance", enabled: true, automatic: false},
		{name: "key", enabled: true, automatic: true, mutate: func(slot *dto.AgentMemorySlotConfig) { slot.Key = "preferences" }},
		{name: "name", enabled: true, automatic: true, mutate: func(slot *dto.AgentMemorySlotConfig) { slot.Name = "User profile" }},
		{name: "description", enabled: true, automatic: true, mutate: func(slot *dto.AgentMemorySlotConfig) { slot.Description = "Updated description" }},
		{name: "max chars", enabled: true, automatic: true, mutate: func(slot *dto.AgentMemorySlotConfig) { slot.MaxChars = 800 }},
		{name: "slot enabled", enabled: true, automatic: true, mutate: func(slot *dto.AgentMemorySlotConfig) { slot.Enabled = false }},
		{name: "sort order", enabled: true, automatic: true, mutate: func(slot *dto.AgentMemorySlotConfig) { slot.SortOrder = 3 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slots := append([]dto.AgentMemorySlotConfig(nil), base...)
			if tt.mutate != nil {
				tt.mutate(&slots[0])
			}
			if got := agentMemoryConfigRevision(tt.enabled, tt.automatic, slots); got == baseRevision {
				t.Fatalf("semantic change kept revision %q", got)
			}
		})
	}
}
