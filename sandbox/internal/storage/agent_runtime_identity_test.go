package storage

import (
	"reflect"
	"testing"

	runtime "github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

func TestExecutionIdentityMigrationRoundTripAndImmutability(t *testing.T) {
	_, stores := admissionStores(t, runtime.AdmissionLimits{Bindings: 8, BindingsPerOrg: 8, Jobs: 8, JobsPerOrg: 8, JobsPerBinding: 1, JobTimeoutSeconds: 60})
	store := stores[0]
	legacy := admissionBinding("legacy-console", "organization")
	created, _, err := store.ReserveBinding(t.Context(), legacy)
	if err != nil || created.Identity != nil {
		t.Fatalf("legacy identity = %+v, %v", created, err)
	}
	binding := admissionBinding("authenticated-webapp", "organization")
	binding.Identity = &runtime.ExecutionIdentity{Version: 1, WorkspaceID: "resource-workspace", ApplicationType: "agent",
		ApplicationID: binding.AgentID, Entry: "webapp", SubjectKind: "account", ConversationID: binding.ConversationID, PublicationRef: "version-1"}
	created, _, err = store.ReserveBinding(t.Context(), binding)
	if err != nil || !reflect.DeepEqual(created.Identity, binding.Identity) {
		t.Fatalf("persist identity = %+v, %v", created, err)
	}
	loaded, err := stores[1].Binding(t.Context(), binding.ID)
	if err != nil || !reflect.DeepEqual(loaded.Identity, binding.Identity) {
		t.Fatalf("read across stores = %+v, %v", loaded, err)
	}
	if _, err := store.db.ExecContext(t.Context(), `UPDATE agent_runtime_bindings SET execution_identity='null'::jsonb WHERE id=$1`, binding.ID); err == nil {
		t.Fatal("persisted identity can be downgraded")
	}
	if err := store.applyMigrations(t.Context()); err != nil {
		t.Fatalf("repeat migration = %v", err)
	}
	loaded, err = store.Binding(t.Context(), legacy.ID)
	if err != nil || loaded.Identity != nil || loaded.RequestHash != legacy.RequestHash || loaded.WorkspaceID != legacy.WorkspaceID {
		t.Fatalf("legacy migration changed ownership or workspace: %+v, %v", loaded, err)
	}
}
