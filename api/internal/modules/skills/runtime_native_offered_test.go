package skills

import (
	"context"
	"reflect"
	"testing"
)

// activate_skills keeps offering an active skill, so its definition and the
// cached prompt prefix after it do not change when a skill is activated.
func TestNativeSkillSessionKeepsOfferingActiveSkills(t *testing.T) {
	resolved := &ResolvedSkills{Skills: []SkillDocument{
		{Metadata: SkillMetadata{ID: "ops", RuntimeType: SkillRuntimeTypePrompt}, Instructions: "Inspect servers."},
		{Metadata: SkillMetadata{ID: "writer", RuntimeType: SkillRuntimeTypePrompt}, Instructions: "Write reports."},
	}}
	catalog := BuildNativeSkillCatalog(resolved, nil, DefaultNativeSkillCatalogBudgetChars, 0, nil)
	session := NewNativeSkillSession(NewRuntime(nil, nil), resolved, catalog, NativeToolSetOptions{BudgetChars: 10000})
	before := NativeControlToolsForSession(resolved, session.ActiveSkillIDs(), session.OfferedSkillIDs(), session.SearchAvailable())

	if result := session.Activate(context.Background(), []string{"ops", "writer"}, "runtime_activation"); len(result.ActivatedSkillIDs) != 2 {
		t.Fatalf("activation = %#v", result)
	}
	if got := session.ExposedSkillIDs(); len(got) != 0 {
		t.Fatalf("exposed inactive skills = %#v, want none", got)
	}
	if got := session.OfferedSkillIDs(); !reflect.DeepEqual(got, []string{"ops", "writer"}) {
		t.Fatalf("offered skills = %#v, want both", got)
	}
	after := NativeControlToolsForSession(resolved, session.ActiveSkillIDs(), session.OfferedSkillIDs(), session.SearchAvailable())
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("control tools changed after activation:\nbefore %#v\nafter  %#v", before, after)
	}
}
