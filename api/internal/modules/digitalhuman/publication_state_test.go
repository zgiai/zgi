package digitalhuman

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
)

// Resource bindings are not part of the configuration revision, so the
// publication state must compare them separately: a binding-only edit is a
// publishable change, and publishing it clears the pending state.
func TestDetailReportsBindingOnlyChangesAsUnpublished(t *testing.T) {
	service, _ := newTestService(t)
	scope := testScope()
	created := createPublishableDigitalHuman(t, service, scope, "binding publish")
	if !created.HasUnpublishedChanges {
		t.Fatal("a never-published DigitalHuman must report unpublished changes")
	}
	published, err := service.Publish(t.Context(), scope, created.Definition.ID, PublishRequest{ExpectedRevision: created.Definition.Revision, ExpectedDraftLockVersion: created.Draft.LockVersion})
	if err != nil {
		t.Fatal(err)
	}
	if published.HasUnpublishedChanges {
		t.Fatal("the draft right after publishing must not report pending changes")
	}
	current, err := service.Get(t.Context(), scope, created.Definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.HasUnpublishedChanges {
		t.Fatal("Get must not report pending changes right after publishing")
	}

	bound, err := service.ReplaceDraftResourceBindings(t.Context(), scope, created.Definition.ID, ReplaceResourceBindingsRequest{
		ExpectedDraftLockVersion: current.Draft.LockVersion,
		Items: []ResourceBindingInput{{
			Type: resourcebindings.BindingTypeKnowledgeDataset, ResourceID: "dataset-1", DisplayName: "Handbook",
			AccessMode: resourcebindings.AccessModeRead, ResourceRevision: "dataset:1",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err = service.Get(t.Context(), scope, created.Definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Draft.Revision != current.Published.Revision || !current.HasUnpublishedChanges {
		t.Fatalf("binding-only edit: revisions %q/%q, pending = %v; want equal revisions and a pending change",
			current.Draft.Revision, current.Published.Revision, current.HasUnpublishedChanges)
	}

	republished, err := service.Publish(t.Context(), scope, created.Definition.ID, PublishRequest{ExpectedRevision: current.Definition.Revision, ExpectedDraftLockVersion: bound.DraftLockVersion})
	if err != nil {
		t.Fatal(err)
	}
	current, err = service.Get(t.Context(), scope, created.Definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	if republished.HasUnpublishedChanges || current.HasUnpublishedChanges {
		t.Fatal("publishing the binding change must clear the pending state")
	}
	if _, err := service.ReplaceDraftResourceBindings(t.Context(), scope, created.Definition.ID, ReplaceResourceBindingsRequest{ExpectedDraftLockVersion: current.Draft.LockVersion}); err != nil {
		t.Fatal(err)
	}
	current, err = service.Get(t.Context(), scope, created.Definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !current.HasUnpublishedChanges {
		t.Fatal("removing a published binding must report a pending change")
	}
}
