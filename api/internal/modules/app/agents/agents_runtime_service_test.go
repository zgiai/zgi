package agents

import (
	"reflect"
	"testing"

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
