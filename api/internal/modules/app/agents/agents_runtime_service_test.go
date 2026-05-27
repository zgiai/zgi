package agents

import (
	"reflect"
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestNormalizeAgentEnabledSkillIDsRemovesRuntimeManagedSkills(t *testing.T) {
	got := normalizeAgentEnabledSkillIDs([]string{
		skills.SkillAgentKnowledge,
		skills.SkillUserMemory,
		skills.SkillCalculator,
		skills.SkillCalculator,
		"  time  ",
		"",
	})
	want := []string{skills.SkillCalculator, skills.SkillTime}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeAgentEnabledSkillIDs() = %#v, want %#v", got, want)
	}
}

func TestAgentMemoryReplaceRequestPreservesInvalidRowsForValidation(t *testing.T) {
	req := agentMemoryReplaceRequestFromConfig([]dto.AgentMemorySlotConfig{
		{Key: "profile", Enabled: true},
		{Key: "", Enabled: true},
		{Key: "profile", Enabled: true},
	}, true)
	if len(req.Slots) != 3 {
		t.Fatalf("len(req.Slots) = %d, want 3", len(req.Slots))
	}
	if req.Slots[1].Key != "" {
		t.Fatalf("empty key row was changed to %q", req.Slots[1].Key)
	}
	if req.Slots[2].Key != "profile" {
		t.Fatalf("duplicate key row was changed to %q", req.Slots[2].Key)
	}
}

func TestAgentMemoryReplaceRequestCanDropHistoricalIDsForRollback(t *testing.T) {
	req := agentMemoryReplaceRequestFromConfig([]dto.AgentMemorySlotConfig{
		{ID: "stale-slot-id", Key: "profile", Enabled: true},
	}, false)
	if len(req.Slots) != 1 {
		t.Fatalf("len(req.Slots) = %d, want 1", len(req.Slots))
	}
	if req.Slots[0].ID != "" {
		t.Fatalf("rollback request preserved historical id %q", req.Slots[0].ID)
	}
	if req.Slots[0].Key != "profile" {
		t.Fatalf("rollback request key = %q, want profile", req.Slots[0].Key)
	}
}

func TestAgentMemorySnapshotSlotsDoNotPersistVolatileIDs(t *testing.T) {
	slots := agentMemorySnapshotSlots([]dto.AgentMemorySlotConfig{
		{
			ID:          "draft-slot-id",
			Key:         "profile",
			Description: "User profile",
			MaxChars:    4000,
			Enabled:     true,
			SortOrder:   3,
			CreatedAt:   123,
			UpdatedAt:   456,
		},
	})
	if len(slots) != 1 {
		t.Fatalf("len(slots) = %d, want 1", len(slots))
	}
	if slots[0].ID != "" {
		t.Fatalf("snapshot preserved slot id %q", slots[0].ID)
	}
	if slots[0].CreatedAt != 0 || slots[0].UpdatedAt != 0 {
		t.Fatalf("snapshot preserved timestamps created=%d updated=%d", slots[0].CreatedAt, slots[0].UpdatedAt)
	}
	if slots[0].MaxChars != 2000 {
		t.Fatalf("snapshot max chars = %d, want 2000", slots[0].MaxChars)
	}
}
