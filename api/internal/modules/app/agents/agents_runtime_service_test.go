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
	})
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
