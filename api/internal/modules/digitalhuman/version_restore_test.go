package digitalhuman

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
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

func TestVersionBindingSnapshotsExcludeCredentialReferences(t *testing.T) {
	bindingID := uuid.New()
	snapshots, err := versionBindingSnapshots([]*ResourceBinding{{
		ID:               bindingID,
		BindingType:      resourcebindings.BindingTypeIntegrationConnection,
		ResourceID:       "connection-1",
		ParentResourceID: "feishu",
		DisplayName:      "Project group",
		AccessMode:       resourcebindings.AccessModeExecute,
		ResourceRevision: "resource:v1",
		BindingRevision:  "binding:v1",
		CredentialRef:    "vault/private-reference",
		Metadata:         datatypes.JSON(`{"allowed_action_ids":["send_message"]}`),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 1 || snapshots[0].ID != bindingID || snapshots[0].ResourceID != "connection-1" {
		t.Fatalf("versionBindingSnapshots() = %#v", snapshots)
	}
	encoded, err := json.Marshal(snapshots)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "vault/private-reference") || strings.Contains(string(encoded), "credential_ref") {
		t.Fatalf("version binding preview leaked credential reference: %s", encoded)
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
