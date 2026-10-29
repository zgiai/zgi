package skills

import (
	"reflect"
	"strings"
	"testing"
)

func catalogTestSkills(ids ...string) *ResolvedSkills {
	resolved := &ResolvedSkills{}
	for _, id := range ids {
		resolved.Skills = append(resolved.Skills, SkillDocument{
			Metadata:     SkillMetadata{ID: id, Description: "Does " + id + " work in detail.", RuntimeType: SkillRuntimeTypePrompt},
			Instructions: "Use " + id + ".",
		})
	}
	return resolved
}

// A directory that shows every candidate does not depend on how the
// question ranked them, so it stays the same from turn to turn (ADR 0032).
func TestNativeSkillCatalogThatFitsIsTheSameForEveryQuestion(t *testing.T) {
	resolved := catalogTestSkills("writer", "ops", "calendar")
	first := BuildNativeSkillCatalog(resolved, []string{"writer", "ops"}, DefaultNativeSkillCatalogBudgetChars, 0, nil)
	second := BuildNativeSkillCatalog(resolved, []string{"calendar"}, DefaultNativeSkillCatalogBudgetChars, 0, nil)
	if !first.Stable || !second.Stable {
		t.Fatalf("stable = %v, %v, want both", first.Stable, second.Stable)
	}
	if first.Message.Content != second.Message.Content || !reflect.DeepEqual(first.CandidateSkillIDs, []string{"calendar", "ops", "writer"}) {
		t.Fatalf("catalogs differ or are not in ID order: %#v / %#v", first.CandidateSkillIDs, second.CandidateSkillIDs)
	}
}

func TestNativeSkillCatalogThatOmitsCandidatesFollowsThePriority(t *testing.T) {
	resolved := catalogTestSkills("writer", "ops", "calendar")
	// Room for about one candidate: the priority decides which one is shown.
	full := BuildNativeSkillCatalog(resolved, nil, DefaultNativeSkillCatalogBudgetChars, 0, nil)
	budget := len([]rune(stringFromNativeMessage(full.Message.Content))) - 150
	catalog := BuildNativeSkillCatalog(resolved, []string{"writer"}, budget, 0, nil)
	if catalog.Stable || catalog.OmittedCount == 0 {
		t.Fatalf("catalog = stable %v, omitted %d; want an unstable catalog that omits candidates", catalog.Stable, catalog.OmittedCount)
	}
	if len(catalog.ExposedSkillIDs) == 0 || catalog.ExposedSkillIDs[0] != "writer" ||
		!strings.Contains(stringFromNativeMessage(catalog.Message.Content), "omitted") {
		t.Fatalf("exposed = %#v, want the priority candidate first", catalog.ExposedSkillIDs)
	}
}
