package digitalhuman

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	"gorm.io/datatypes"
)

func TestExecutionConfigOnlyAcceptsApplicationOptIn(t *testing.T) {
	for _, raw := range []string{`{"enabled":true}`, `{"enabled":false}`} {
		var request CreateRequest
		if err := json.Unmarshal([]byte(`{"execution_runtime":`+raw+`}`), &request); err != nil {
			t.Fatal(err)
		}
		if request.ExecutionRuntime == nil {
			t.Fatal("execution setting missing")
		}
	}
	for _, raw := range []string{`{}`, `{"enabled":null}`, `{"enabled":"true"}`, `{"enabled":true,"profile_id":"other"}`, `{"enabled":true,"policy_id":"public"}`, `{"enabled":true,"home_snapshot_id":"private"}`} {
		var request UpdateDraftRequest
		if err := json.Unmarshal([]byte(`{"execution_runtime":`+raw+`}`), &request); err == nil {
			t.Fatalf("accepted runtime config=%s", raw)
		}
	}
}

func TestExecutionConfigPreservesLegacyRevisionAndFreezesPublication(t *testing.T) {
	version := &Version{IdentityName: "Atlas", Persona: "Careful", Goals: datatypes.JSON(`[]`), Rules: datatypes.JSON(`[]`),
		DefaultModel: "model", MemoryPolicy: datatypes.JSON(`{}`), TriggerPolicy: datatypes.JSON(`{}`), PresentationPolicy: datatypes.JSON(`{}`)}
	legacyJSON := `{"identity_name":"Atlas","persona":"Careful","goals":[],"rules":[],"default_model":"model","memory_policy":{},"trigger_policy":{},"presentation_policy":{}}`
	digest := sha256.Sum256([]byte(legacyJSON))
	want := "sha256:" + hex.EncodeToString(digest[:])
	for _, config := range []*ExecutionRuntimeConfig{nil, {Enabled: false}} {
		version.ExecutionRuntime = config
		if got, err := configRevision(version); err != nil || got != want {
			t.Fatalf("legacy revision=%s err=%v, want %s", got, err, want)
		}
	}

	service, _ := newTestService(t)
	scope := testScope()
	detail := createPublishableDigitalHuman(t, service, scope, "Linux")
	enabled := &ExecutionRuntimeConfig{Enabled: true}
	draft, err := service.UpdateDraft(t.Context(), scope, detail.Definition.ID, UpdateDraftRequest{ExpectedLockVersion: detail.Draft.LockVersion, ExecutionRuntime: enabled})
	if err != nil {
		t.Fatal(err)
	}
	if draft.ExecutionRuntime == enabled || draft.ExecutionRuntime == nil || !draft.ExecutionRuntime.Enabled {
		t.Fatal("draft did not copy opt-in")
	}
	enabled.Enabled = false
	detail, err = service.Publish(t.Context(), scope, detail.Definition.ID, PublishRequest{ExpectedRevision: detail.Definition.Revision, ExpectedDraftLockVersion: draft.LockVersion})
	if err != nil {
		t.Fatal(err)
	}
	if detail.Published.ExecutionRuntime == nil || detail.Draft.ExecutionRuntime == detail.Published.ExecutionRuntime {
		t.Fatal("publication and draft share runtime configuration")
	}
	if _, err := service.UpdateDraft(t.Context(), scope, detail.Definition.ID, UpdateDraftRequest{ExpectedLockVersion: detail.Draft.LockVersion, ExecutionRuntime: &ExecutionRuntimeConfig{Enabled: false}}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := service.Get(t.Context(), scope, detail.Definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Draft.ExecutionRuntime != nil || reloaded.Published.ExecutionRuntime == nil || !reloaded.Published.ExecutionRuntime.Enabled {
		t.Fatal("draft disable changed publication or failed to persist")
	}
	preview, err := service.PreviewVersionRestore(t.Context(), scope, detail.Definition.ID, detail.Published.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(preview.ChangedFields, "execution_runtime") {
		t.Fatalf("restore changes=%v", preview.ChangedFields)
	}
	restored, err := service.RestoreVersion(t.Context(), scope, detail.Definition.ID, RestoreVersionRequest{VersionID: detail.Published.ID,
		ExpectedRevision: reloaded.Definition.Revision, ExpectedDraftLockVersion: reloaded.Draft.LockVersion, ImpactToken: preview.ImpactToken})
	if err != nil {
		t.Fatal(err)
	}
	if restored.Draft.ExecutionRuntime == nil || !restored.Draft.ExecutionRuntime.Enabled {
		t.Fatal("restored draft lost Linux opt-in")
	}
}

func TestExecutionConfigCreateAndPartialUpdate(t *testing.T) {
	service, _ := newTestService(t)
	scope := testScope()
	config := &ExecutionRuntimeConfig{Enabled: true}
	detail, err := service.Create(t.Context(), scope, CreateRequest{Name: "Linux draft", ExecutionRuntime: config})
	if err != nil {
		t.Fatal(err)
	}
	config.Enabled = false
	reloaded, err := service.Get(t.Context(), scope, detail.Definition.ID)
	if err != nil || reloaded.Draft.ExecutionRuntime == nil || !reloaded.Draft.ExecutionRuntime.Enabled {
		t.Fatalf("created opt-in was not persisted independently: detail=%+v err=%v", reloaded, err)
	}
	persona := "Updated independently"
	updated, err := service.UpdateDraft(t.Context(), scope, detail.Definition.ID, UpdateDraftRequest{
		ExpectedLockVersion: reloaded.Draft.LockVersion, Persona: &persona,
	})
	if err != nil || updated.ExecutionRuntime == nil || !updated.ExecutionRuntime.Enabled {
		t.Fatalf("partial update lost opt-in: draft=%+v err=%v", updated, err)
	}
	if _, err := service.UpdateDraft(t.Context(), scope, detail.Definition.ID, UpdateDraftRequest{
		ExpectedLockVersion: reloaded.Draft.LockVersion, ExecutionRuntime: &ExecutionRuntimeConfig{Enabled: false},
	}); err == nil {
		t.Fatal("stale update disabled a newer draft")
	}
	reloaded, err = service.Get(t.Context(), scope, detail.Definition.ID)
	if err != nil || reloaded.Draft.ExecutionRuntime == nil || !reloaded.Draft.ExecutionRuntime.Enabled {
		t.Fatalf("failed update mutated stored configuration: detail=%+v err=%v", reloaded, err)
	}
}

func TestExecutionSnapshotPinsRuntimeConfigAndRetainsLegacyDigest(t *testing.T) {
	subject := runtimeidentity.SubjectRef{Type: runtimeidentity.SubjectTypeDigitalHuman, ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New()}
	store := &resolverStoreStub{definition: &Definition{Status: StatusActive}, version: &Version{ID: uuid.New(), IdentityName: "Atlas", Persona: "Careful", DefaultModel: "model", Rules: datatypes.JSON(`[]`), Revision: "sha256:version"}}
	resolver := NewResolver(store)
	now := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	resolver.now = func() time.Time { return now }
	event := invocation.Event{ID: "linux-snapshot", Type: invocation.EventTypeManual, Origin: invocation.Origin{Kind: invocation.OriginManual}, Target: subject,
		Sender: invocation.PrincipalRef{Type: invocation.PrincipalAccount, ID: uuid.NewString()}, OccurredAt: now}
	legacy, err := resolver.Resolve(t.Context(), event, nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(legacy.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var oldFields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &oldFields); err != nil {
		t.Fatal(err)
	}
	if _, ok := oldFields["execution_runtime"]; ok {
		t.Fatal("legacy snapshot gained a serialized field")
	}
	// Persisted v1 snapshots still round-trip through digest verification.
	var decoded ExecutionSnapshot
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if got, err := executionSnapshotRevision(decoded); err != nil || got != legacy.Snapshot.Revision {
		t.Fatalf("legacy snapshot digest=%s err=%v", got, err)
	}
	store.version.ExecutionRuntime = &ExecutionRuntimeConfig{Enabled: true}
	resolved, err := resolver.Resolve(t.Context(), event, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Snapshot.Revision == legacy.Snapshot.Revision || resolved.Snapshot.ExecutionRuntime == store.version.ExecutionRuntime {
		t.Fatal("snapshot did not freeze opt-in in its digest")
	}
	store.version.ExecutionRuntime.Enabled = false
	if !resolved.Snapshot.ExecutionRuntime.Enabled {
		t.Fatal("published setting mutation changed sealed snapshot")
	}
}

// New digital humans start with uploads off, so their WebApp does not take
// files until someone turns uploads on.
func TestNewDigitalHumanStartsWithUploadsOff(t *testing.T) {
	service, _ := newTestService(t)
	scope := testScope()
	detail, err := service.Create(t.Context(), scope, CreateRequest{Name: "Quiet draft"})
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := service.Get(t.Context(), scope, detail.Definition.ID)
	if err != nil || reloaded.Draft.FileUploadPolicy == nil || fileUploadEnabled(reloaded.Draft) {
		t.Fatalf("new draft upload policy = %+v, err %v", reloaded.Draft.FileUploadPolicy, err)
	}
}
