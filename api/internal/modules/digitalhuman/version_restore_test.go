package digitalhuman

import (
	"reflect"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"gorm.io/datatypes"
)

func TestVersionChangedFields(t *testing.T) {
	current := &Version{
		IdentityName: "Atlas", Persona: "Current", Goals: datatypes.JSON(`[]`), Rules: datatypes.JSON(`["safe"]`),
		DefaultModel: "model-a", MemoryPolicy: datatypes.JSON(`{"enabled":true}`), TriggerPolicy: datatypes.JSON(`{"manual":true}`),
	}
	target := &Version{
		IdentityName: "Atlas", Persona: "Target", Goals: datatypes.JSON(`["research"]`), Rules: datatypes.JSON(`["safe"]`),
		DefaultModel: "model-b", MemoryPolicy: datatypes.JSON(`{"enabled":true}`), TriggerPolicy: datatypes.JSON(`{"manual":false}`),
	}
	want := []string{"persona", "goals", "default_model", "trigger_policy"}
	if got := versionChangedFields(current, target); !reflect.DeepEqual(got, want) {
		t.Fatalf("versionChangedFields() = %#v, want %#v", got, want)
	}
}

func TestVersionBindingChanges(t *testing.T) {
	current := []*ResourceBinding{
		{BindingType: resourcebindings.BindingTypeSkill, ResourceID: "same", AccessMode: resourcebindings.AccessModeExecute, BindingRevision: "same"},
		{BindingType: resourcebindings.BindingTypeWorkflow, ResourceID: "updated", AccessMode: resourcebindings.AccessModeExecute, BindingRevision: "old"},
		{BindingType: resourcebindings.BindingTypeKnowledgeDataset, ResourceID: "removed", AccessMode: resourcebindings.AccessModeRead, BindingRevision: "old"},
	}
	target := []*ResourceBinding{
		{BindingType: resourcebindings.BindingTypeSkill, ResourceID: "same", AccessMode: resourcebindings.AccessModeExecute, BindingRevision: "same"},
		{BindingType: resourcebindings.BindingTypeWorkflow, ResourceID: "updated", AccessMode: resourcebindings.AccessModeExecute, BindingRevision: "new"},
		{BindingType: resourcebindings.BindingTypeDatabase, ResourceID: "added", AccessMode: resourcebindings.AccessModeRead, BindingRevision: "new"},
	}
	changes := versionBindingChanges(current, target)
	if len(changes) != 3 {
		t.Fatalf("versionBindingChanges() = %#v", changes)
	}
	counts := map[string]int{}
	for _, change := range changes {
		counts[change.Change]++
	}
	if counts[restoreBindingAdded] != 1 || counts[restoreBindingRemoved] != 1 || counts[restoreBindingUpdated] != 1 {
		t.Fatalf("versionBindingChanges() counts = %#v", counts)
	}
}
