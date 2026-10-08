package digitalhuman

import (
	"github.com/zgiai/zgi/api/internal/contracts/chatfile"
	"testing"
)

func TestFileUploadPolicyPublicationAndRestore(t *testing.T) {
	service, _ := newTestService(t)
	scope := testScope()
	detail := createPublishableDigitalHuman(t, service, scope, "Files")
	draft, err := service.UpdateDraft(t.Context(), scope, detail.Definition.ID, UpdateDraftRequest{ExpectedLockVersion: detail.Draft.LockVersion, FileUploadPolicy: &chatfile.Policy{AllowedFileTypes: []string{"document"}}})
	if err != nil {
		t.Fatal(err)
	}
	detail, err = service.Publish(t.Context(), scope, detail.Definition.ID, PublishRequest{ExpectedRevision: detail.Definition.Revision, ExpectedDraftLockVersion: draft.LockVersion})
	if err != nil {
		t.Fatal(err)
	}
	if detail.Published.FileUploadPolicy == detail.Draft.FileUploadPolicy {
		t.Fatal("publication shares mutable policy")
	}
	_, err = service.UpdateDraft(t.Context(), scope, detail.Definition.ID, UpdateDraftRequest{ExpectedLockVersion: detail.Draft.LockVersion, FileUploadPolicy: &chatfile.Policy{AllowedFileTypes: []string{"image"}}})
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := service.Get(t.Context(), scope, detail.Definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Published.FileUploadPolicy.AllowedFileTypes[0] != "document" || reloaded.Draft.FileUploadPolicy.AllowedFileTypes[0] != "image" {
		t.Fatal("draft changed publication or did not persist")
	}
	preview, err := service.PreviewVersionRestore(t.Context(), scope, detail.Definition.ID, detail.Published.ID)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := service.RestoreVersion(t.Context(), scope, detail.Definition.ID, RestoreVersionRequest{VersionID: detail.Published.ID, ExpectedRevision: reloaded.Definition.Revision, ExpectedDraftLockVersion: reloaded.Draft.LockVersion, ImpactToken: preview.ImpactToken})
	if err != nil {
		t.Fatal(err)
	}
	if restored.Draft.FileUploadPolicy.AllowedFileTypes[0] != "document" {
		t.Fatal("restore lost policy")
	}
}
