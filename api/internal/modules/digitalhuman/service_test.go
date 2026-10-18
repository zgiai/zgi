package digitalhuman

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	"github.com/zgiai/zgi/api/pkg/apperror"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestServiceCreateUpdateAndScopeIsolation(t *testing.T) {
	service, _ := newTestService(t)
	scope := testScope()

	detail, err := service.Create(t.Context(), scope, CreateRequest{Name: "Atlas"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if detail.Definition.Status != StatusDraft || detail.Definition.Revision != 1 || detail.Draft.LockVersion != 1 {
		t.Fatalf("Create() detail = %#v", detail)
	}
	if detail.Definition.DraftVersionID == nil || *detail.Definition.DraftVersionID != detail.Draft.ID {
		t.Fatalf("draft pointer = %v, draft id = %s", detail.Definition.DraftVersionID, detail.Draft.ID)
	}

	updatedName := "Atlas Research"
	updated, err := service.Update(t.Context(), scope, detail.Definition.ID, UpdateRequest{
		ExpectedRevision: 1,
		Name:             &updatedName,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Definition.Name != updatedName || updated.Definition.Revision != 2 {
		t.Fatalf("Update() definition = %#v", updated.Definition)
	}
	if _, err := service.Update(t.Context(), scope, detail.Definition.ID, UpdateRequest{
		ExpectedRevision: 1,
		Name:             &updatedName,
	}); !apperror.IsCode(err, AppCodeRevisionConflict) || !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale Update() error = %v, want revision conflict", err)
	}

	wrongScope := scope
	wrongScope.WorkspaceID = uuid.New()
	if _, err := service.Get(t.Context(), wrongScope, detail.Definition.ID); !apperror.IsCode(err, AppCodeNotFound) {
		t.Fatalf("Get(other workspace) error = %v, want not found", err)
	}
}

func TestServicePublishCreatesImmutableSnapshotAndNextDraft(t *testing.T) {
	service, repo := newTestService(t)
	scope := testScope()
	detail := createPublishableDigitalHuman(t, service, scope, "Nova")

	publishedDetail, err := service.Publish(t.Context(), scope, detail.Definition.ID, PublishRequest{
		ExpectedRevision:         detail.Definition.Revision,
		ExpectedDraftLockVersion: detail.Draft.LockVersion,
		Name:                     "  Research-ready release  ",
		Description:              "  Adds production research behavior.  ",
	})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if publishedDetail.Definition.Status != StatusActive || publishedDetail.Definition.Revision != 2 {
		t.Fatalf("published definition = %#v", publishedDetail.Definition)
	}
	if publishedDetail.Published == nil || publishedDetail.Published.State != VersionStatePublished || publishedDetail.Published.VersionNumber != 1 {
		t.Fatalf("published version = %#v", publishedDetail.Published)
	}
	if publishedDetail.Published.Name != "Research-ready release" || publishedDetail.Published.Description != "Adds production research behavior." {
		t.Fatalf("published release metadata = %#v", publishedDetail.Published)
	}
	if !strings.HasPrefix(publishedDetail.Published.Revision, versionRevisionPrefix) || len(publishedDetail.Published.IdentitySnapshot) == 0 {
		t.Fatalf("published revision/snapshot = %q / %s", publishedDetail.Published.Revision, publishedDetail.Published.IdentitySnapshot)
	}
	if publishedDetail.Draft.ID == publishedDetail.Published.ID || publishedDetail.Draft.State != VersionStateDraft || publishedDetail.Draft.LockVersion != 1 {
		t.Fatalf("next draft = %#v", publishedDetail.Draft)
	}
	if publishedDetail.Draft.Name != "" || publishedDetail.Draft.Description != "" {
		t.Fatalf("next draft inherited release metadata = %#v", publishedDetail.Draft)
	}
	if publishedDetail.Draft.Revision != publishedDetail.Published.Revision {
		t.Fatalf("fresh draft revision = %q, published revision = %q", publishedDetail.Draft.Revision, publishedDetail.Published.Revision)
	}
	displayName := "Research workspace assistant"
	publishedDetail, err = service.Update(t.Context(), scope, detail.Definition.ID, UpdateRequest{
		ExpectedRevision: publishedDetail.Definition.Revision,
		Name:             &displayName,
	})
	if err != nil {
		t.Fatalf("Update(display metadata) error = %v", err)
	}
	if publishedDetail.Draft.Revision != publishedDetail.Published.Revision {
		t.Fatalf("display metadata changed runtime revision: draft=%q published=%q", publishedDetail.Draft.Revision, publishedDetail.Published.Revision)
	}
	publishedPersona := publishedDetail.Published.Persona
	publishedRevision := publishedDetail.Published.Revision
	changedPersona := "A changed draft persona"
	changedDraft, err := service.UpdateDraft(t.Context(), scope, detail.Definition.ID, UpdateDraftRequest{
		ExpectedLockVersion: publishedDetail.Draft.LockVersion,
		Persona:             &changedPersona,
	})
	if err != nil {
		t.Fatalf("UpdateDraft() error = %v", err)
	}
	if changedDraft.LockVersion != 2 || changedDraft.Persona != changedPersona {
		t.Fatalf("changed draft = %#v", changedDraft)
	}

	reloaded, err := service.Get(t.Context(), scope, detail.Definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Published.Persona != publishedPersona || reloaded.Published.Revision != publishedRevision {
		t.Fatalf("published version changed with draft: %#v", reloaded.Published)
	}
	if err := repo.UpdateDraftCAS(t.Context(), scope, reloaded.Published, reloaded.Published.LockVersion); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("UpdateDraftCAS(published) error = %v, want revision conflict", err)
	}
}

func TestServicePublishRejectsOversizedReleaseMetadata(t *testing.T) {
	service, _ := newTestService(t)
	scope := testScope()
	detail := createPublishableDigitalHuman(t, service, scope, "Release limits")

	tests := []PublishRequest{
		{
			ExpectedRevision:         detail.Definition.Revision,
			ExpectedDraftLockVersion: detail.Draft.LockVersion,
			Name:                     strings.Repeat("界", maxVersionNameRunes+1),
		},
		{
			ExpectedRevision:         detail.Definition.Revision,
			ExpectedDraftLockVersion: detail.Draft.LockVersion,
			Description:              strings.Repeat("界", maxVersionDescriptionRunes+1),
		},
	}
	for _, request := range tests {
		if _, err := service.Publish(t.Context(), scope, detail.Definition.ID, request); !apperror.IsCode(err, AppCodeRequestInvalid) {
			t.Fatalf("Publish(oversized metadata) error = %v, want request invalid", err)
		}
	}

	reloaded, err := service.Get(t.Context(), scope, detail.Definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Published != nil || reloaded.Draft.ID != detail.Draft.ID {
		t.Fatalf("oversized metadata partially published the draft: %#v", reloaded)
	}
}

func TestNormalizePublishRequest(t *testing.T) {
	normalized, err := normalizePublishRequest(PublishRequest{
		ExpectedRevision:         2,
		ExpectedDraftLockVersion: 3,
		Name:                     "  Research release  ",
		Description:              "  Production notes  ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Name != "Research release" || normalized.Description != "Production notes" {
		t.Fatalf("normalizePublishRequest() = %#v", normalized)
	}
	if _, err := normalizePublishRequest(PublishRequest{Name: strings.Repeat("界", maxVersionNameRunes+1)}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("normalizePublishRequest(oversized name) error = %v", err)
	}
	if _, err := normalizePublishRequest(PublishRequest{Description: strings.Repeat("界", maxVersionDescriptionRunes+1)}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("normalizePublishRequest(oversized description) error = %v", err)
	}
}

func TestServicePublishRejectsInvalidDraftWithoutPartialMutation(t *testing.T) {
	service, _ := newTestService(t)
	scope := testScope()
	detail, err := service.Create(t.Context(), scope, CreateRequest{Name: "Saga"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.Publish(t.Context(), scope, detail.Definition.ID, PublishRequest{
		ExpectedRevision:         detail.Definition.Revision,
		ExpectedDraftLockVersion: detail.Draft.LockVersion,
	}); !apperror.IsCode(err, AppCodePublishInvalid) {
		t.Fatalf("Publish(invalid) error = %v, want publish invalid", err)
	}
	reloaded, err := service.Get(t.Context(), scope, detail.Definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Definition.Status != StatusDraft || reloaded.Published != nil || reloaded.Draft.ID != detail.Draft.ID {
		t.Fatalf("invalid publish partially mutated state: %#v", reloaded)
	}
}

func TestServicePublishAllowsDraftWithoutRules(t *testing.T) {
	draft := &Version{
		IdentityName: "Rule optional",
		Persona:      "A concise operations partner",
		DefaultModel: "gpt-5",
		Rules:        []byte(`[]`),
	}
	if err := validatePublishable(draft); err != nil {
		t.Fatalf("validatePublishable() error = %v", err)
	}
}

func TestServiceDeleteImpactBlocksPublishedAndDeletesUnpublished(t *testing.T) {
	service, _ := newTestService(t)
	scope := testScope()
	published := createPublishableDigitalHuman(t, service, scope, "Iris")
	published, err := service.Publish(t.Context(), scope, published.Definition.ID, PublishRequest{
		ExpectedRevision:         published.Definition.Revision,
		ExpectedDraftLockVersion: published.Draft.LockVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	impact, err := service.DeleteImpact(t.Context(), scope, published.Definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	if impact.CanDelete || impact.PublishedVersionCount != 1 || len(impact.Blockers) != 1 {
		t.Fatalf("DeleteImpact() = %#v", impact)
	}
	if err := service.Delete(t.Context(), scope, published.Definition.ID, published.Definition.Revision); !apperror.IsCode(err, AppCodeDeleteBlocked) {
		t.Fatalf("Delete(published) error = %v, want blocked", err)
	}

	unpublished, err := service.Create(t.Context(), scope, CreateRequest{Name: "Bram"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(t.Context(), scope, unpublished.Definition.ID, unpublished.Definition.Revision); err != nil {
		t.Fatalf("Delete(unpublished) error = %v", err)
	}
	if _, err := service.Get(t.Context(), scope, unpublished.Definition.ID); !apperror.IsCode(err, AppCodeNotFound) {
		t.Fatalf("Get(deleted) error = %v, want not found", err)
	}
}

func TestServiceDeleteAllowsDisabledPublishedDigitalHuman(t *testing.T) {
	service, _ := newTestService(t)
	scope := testScope()
	draft := createPublishableDigitalHuman(t, service, scope, "Juno")
	published, err := service.Publish(t.Context(), scope, draft.Definition.ID, PublishRequest{
		ExpectedRevision:         draft.Definition.Revision,
		ExpectedDraftLockVersion: draft.Draft.LockVersion,
	})
	if err != nil {
		t.Fatal(err)
	}

	impact, err := service.DeleteImpact(t.Context(), scope, published.Definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	if impact.CanDelete || len(impact.Blockers) != 1 {
		t.Fatalf("DeleteImpact(online) = %#v, want blocked by published versions", impact)
	}

	disabled := StatusDisabled
	offline, err := service.Update(t.Context(), scope, published.Definition.ID, UpdateRequest{
		ExpectedRevision: published.Definition.Revision,
		Status:           &disabled,
	})
	if err != nil {
		t.Fatalf("Update(status=disabled) error = %v", err)
	}
	if offline.Definition.Status != StatusDisabled {
		t.Fatalf("status = %q, want disabled", offline.Definition.Status)
	}

	// Taking the digital human offline is the operator step that unlocks deletion;
	// the immutable published version is still reported.
	impact, err = service.DeleteImpact(t.Context(), scope, offline.Definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !impact.CanDelete || impact.PublishedVersionCount != 1 || len(impact.Blockers) != 0 {
		t.Fatalf("DeleteImpact(disabled) = %#v, want deletable", impact)
	}
	if err := service.Delete(t.Context(), scope, offline.Definition.ID, offline.Definition.Revision); err != nil {
		t.Fatalf("Delete(disabled) error = %v", err)
	}
	if _, err := service.Get(t.Context(), scope, offline.Definition.ID); !apperror.IsCode(err, AppCodeNotFound) {
		t.Fatalf("Get(deleted) error = %v, want not found", err)
	}
}

func TestServiceAllowsDuplicateWorkspaceNames(t *testing.T) {
	service, _ := newTestService(t)
	scope := testScope()
	first, err := service.Create(t.Context(), scope, CreateRequest{Name: "Atlas"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(t.Context(), scope, CreateRequest{Name: " atlas "}); err != nil {
		t.Fatalf("Create(trim/case duplicate) error = %v", err)
	}
	name := "ATLAS"
	if _, err := service.Update(t.Context(), scope, first.Definition.ID, UpdateRequest{
		ExpectedRevision: first.Definition.Revision,
		Name:             &name,
	}); err != nil {
		t.Fatalf("Update(duplicate name) error = %v", err)
	}
}

func TestPublishedVersionsAreNewestFirst(t *testing.T) {
	service, _ := newTestService(t)
	scope := testScope()
	detail := createPublishableDigitalHuman(t, service, scope, "Echo")
	first, err := service.Publish(t.Context(), scope, detail.Definition.ID, PublishRequest{
		ExpectedRevision:         detail.Definition.Revision,
		ExpectedDraftLockVersion: detail.Draft.LockVersion,
		Name:                     "First release",
		Description:              "Initial production version",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Publish(t.Context(), scope, detail.Definition.ID, PublishRequest{
		ExpectedRevision:         first.Definition.Revision,
		ExpectedDraftLockVersion: first.Draft.LockVersion,
		Name:                     "Second release",
		Description:              "Refined production version",
	})
	if err != nil {
		t.Fatal(err)
	}
	versions, err := service.PublishedVersions(t.Context(), scope, detail.Definition.ID, ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if versions.Total != 2 || len(versions.Items) != 2 || versions.Items[0].VersionNumber != 2 || versions.Items[1].VersionNumber != 1 {
		t.Fatalf("PublishedVersions() = %#v; latest detail = %#v", versions, second)
	}
	if versions.Items[0].Name != "Second release" || versions.Items[0].Description != "Refined production version" ||
		versions.Items[1].Name != "First release" || versions.Items[1].Description != "Initial production version" {
		t.Fatalf("PublishedVersions() release metadata = %#v", versions.Items)
	}
}

func TestRestorePublishedVersionReplacesOnlyCurrentDraft(t *testing.T) {
	service, _ := newTestService(t)
	scope := testScope()
	detail := createPublishableDigitalHuman(t, service, scope, "Saga")
	firstBindings, err := service.ReplaceDraftResourceBindings(t.Context(), scope, detail.Definition.ID, ReplaceResourceBindingsRequest{
		ExpectedDraftLockVersion: detail.Draft.LockVersion,
		Items: []ResourceBindingInput{{
			Type: resourcebindings.BindingTypeSkill, ResourceID: "skill.first", DisplayName: "First skill",
			AccessMode: resourcebindings.AccessModeExecute, ResourceRevision: "skill:first:v1",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	detail.Draft.LockVersion = firstBindings.DraftLockVersion
	first, err := service.Publish(t.Context(), scope, detail.Definition.ID, PublishRequest{
		ExpectedRevision: detail.Definition.Revision, ExpectedDraftLockVersion: detail.Draft.LockVersion,
	})
	if err != nil {
		t.Fatal(err)
	}

	secondPersona := "A second published persona"
	secondDraft, err := service.UpdateDraft(t.Context(), scope, detail.Definition.ID, UpdateDraftRequest{
		ExpectedLockVersion: first.Draft.LockVersion, Persona: &secondPersona,
	})
	if err != nil {
		t.Fatal(err)
	}
	secondBindings, err := service.ReplaceDraftResourceBindings(t.Context(), scope, detail.Definition.ID, ReplaceResourceBindingsRequest{
		ExpectedDraftLockVersion: secondDraft.LockVersion,
		Items: []ResourceBindingInput{{
			Type: resourcebindings.BindingTypeSkill, ResourceID: "skill.second", DisplayName: "Second skill",
			AccessMode: resourcebindings.AccessModeExecute, ResourceRevision: "skill:second:v1",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Publish(t.Context(), scope, detail.Definition.ID, PublishRequest{
		ExpectedRevision: first.Definition.Revision, ExpectedDraftLockVersion: secondBindings.DraftLockVersion,
	})
	if err != nil {
		t.Fatal(err)
	}

	changedPersona := "Unpublished work that will be replaced"
	changedDraft, err := service.UpdateDraft(t.Context(), scope, detail.Definition.ID, UpdateDraftRequest{
		ExpectedLockVersion: second.Draft.LockVersion, Persona: &changedPersona,
	})
	if err != nil {
		t.Fatal(err)
	}
	changedBindings, err := service.ReplaceDraftResourceBindings(t.Context(), scope, detail.Definition.ID, ReplaceResourceBindingsRequest{
		ExpectedDraftLockVersion: changedDraft.LockVersion,
		Items: []ResourceBindingInput{{
			Type: resourcebindings.BindingTypeSkill, ResourceID: "skill.draft", DisplayName: "Draft skill",
			AccessMode: resourcebindings.AccessModeExecute, ResourceRevision: "skill:draft:v1",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	preview, err := service.PreviewVersionRestore(t.Context(), scope, detail.Definition.ID, first.Published.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.IsCurrent || !preview.WillDiscardUnpublishedChanges || preview.ImpactToken == "" || len(preview.ChangedFields) == 0 || len(preview.BindingChanges) != 2 {
		t.Fatalf("PreviewVersionRestore() = %#v", preview)
	}
	if len(preview.Bindings) != 1 || preview.Bindings[0].ResourceID != "skill.first" {
		t.Fatalf("PreviewVersionRestore().Bindings = %#v", preview.Bindings)
	}
	restored, err := service.RestoreVersion(t.Context(), scope, detail.Definition.ID, RestoreVersionRequest{
		VersionID: first.Published.ID, ExpectedRevision: second.Definition.Revision,
		ExpectedDraftLockVersion: changedBindings.DraftLockVersion, ImpactToken: preview.ImpactToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	if restored.Published == nil || restored.Published.ID != second.Published.ID {
		t.Fatalf("active publication changed during restore: %#v", restored.Published)
	}
	if restored.Draft.ID != second.Draft.ID || restored.Draft.Persona != first.Published.Persona || restored.Draft.LockVersion != changedBindings.DraftLockVersion+1 {
		t.Fatalf("restored draft = %#v", restored.Draft)
	}
	bindings, err := service.ListDraftResourceBindings(t.Context(), scope, detail.Definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings.Items) != 1 || bindings.Items[0].ResourceID != "skill.first" {
		t.Fatalf("restored bindings = %#v", bindings.Items)
	}
}

func TestRestorePublishedVersionRejectsChangedImpactToken(t *testing.T) {
	service, _ := newTestService(t)
	scope := testScope()
	detail := createPublishableDigitalHuman(t, service, scope, "Iris")
	published, err := service.Publish(t.Context(), scope, detail.Definition.ID, PublishRequest{
		ExpectedRevision: detail.Definition.Revision, ExpectedDraftLockVersion: detail.Draft.LockVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.PreviewVersionRestore(t.Context(), scope, detail.Definition.ID, published.Published.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.RestoreVersion(t.Context(), scope, detail.Definition.ID, RestoreVersionRequest{
		VersionID: published.Published.ID, ExpectedRevision: published.Definition.Revision,
		ExpectedDraftLockVersion: published.Draft.LockVersion, ImpactToken: preview.ImpactToken + "changed",
	})
	if !errors.Is(err, ErrVersionRestoreImpactChanged) || !apperror.IsCode(err, AppCodeVersionRestoreImpactChanged) {
		t.Fatalf("RestoreVersion(stale impact) error = %v", err)
	}
}

func TestPersonaTemplatesAndBindingsRemainIndependent(t *testing.T) {
	service, repo := newTestService(t)
	scope := testScope()
	template, err := service.CreatePersonaTemplate(t.Context(), scope, CreatePersonaTemplateRequest{
		Name: "Nova", Persona: "Be evidence led", Goals: []string{"Find growth opportunities"}, Rules: []string{"Cite sources"},
	})
	if err != nil {
		t.Fatal(err)
	}
	changedPersona := "Be concise and evidence led"
	template, err = service.UpdatePersonaTemplate(t.Context(), scope, template.ID, UpdatePersonaTemplateRequest{
		ExpectedRevision: template.Revision,
		Persona:          &changedPersona,
	})
	if err != nil || template.Persona != changedPersona || template.Revision != 2 {
		t.Fatal(err)
	}
	if template.Name != "Nova" || string(template.Goals) != `["Find growth opportunities"]` {
		t.Fatalf("persona template identity = %#v", template)
	}

	first := createPublishableDigitalHuman(t, service, scope, "Atlas")
	bindings, err := service.ReplaceDraftResourceBindings(t.Context(), scope, first.Definition.ID, ReplaceResourceBindingsRequest{
		ExpectedDraftLockVersion: first.Draft.LockVersion,
		Items: []ResourceBindingInput{{
			Type: resourcebindings.BindingTypeSkill, ResourceID: "builtin.web.search", DisplayName: "Web search",
			AccessMode: resourcebindings.AccessModeExecute, ResourceRevision: "skill:v1",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	first.Draft.LockVersion = bindings.DraftLockVersion
	first, err = service.Publish(t.Context(), scope, first.Definition.ID, PublishRequest{
		ExpectedRevision: first.Definition.Revision, ExpectedDraftLockVersion: first.Draft.LockVersion,
	})
	if err != nil {
		t.Fatal(err)
	}

	versionID := first.Published.ID
	snapshot := resourcebindings.SnapshotRef{Subject: runtimeidentity.SubjectRef{
		Type: runtimeidentity.SubjectTypeDigitalHuman, ID: first.Definition.ID,
		OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
	}, Phase: resourcebindings.PhasePublished, VersionID: &versionID}
	decision, err := repo.Verify(t.Context(), snapshot, resourcebindings.Match{
		Type: resourcebindings.BindingTypeSkill, ResourceID: "builtin.web.search", AccessMode: resourcebindings.AccessModeExecute,
	})
	if err != nil || !decision.Allowed || decision.Binding.Revision == "" {
		t.Fatalf("Verify(published skill) = %#v, %v", decision, err)
	}
	denied, err := repo.Verify(t.Context(), snapshot, resourcebindings.Match{
		Type: resourcebindings.BindingTypeSkill, ResourceID: "builtin.unbound", AccessMode: resourcebindings.AccessModeExecute,
	})
	if err != nil || denied.Allowed || denied.Reason != "resource_not_bound" {
		t.Fatalf("Verify(unbound skill) = %#v, %v", denied, err)
	}
	if _, err := service.ReplaceDraftResourceBindings(t.Context(), scope, first.Definition.ID, ReplaceResourceBindingsRequest{
		ExpectedDraftLockVersion: first.Draft.LockVersion, Items: nil,
	}); err != nil {
		t.Fatal(err)
	}
	decision, err = repo.Verify(t.Context(), snapshot, resourcebindings.Match{
		Type: resourcebindings.BindingTypeSkill, ResourceID: "builtin.web.search", AccessMode: resourcebindings.AccessModeExecute,
	})
	if err != nil || !decision.Allowed {
		t.Fatalf("published snapshot changed with next draft: %#v, %v", decision, err)
	}
	if err := service.DeletePersonaTemplate(t.Context(), scope, template.ID, template.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetPersonaTemplate(t.Context(), scope, template.ID); !apperror.IsCode(err, AppCodePersonaTemplateNotFound) {
		t.Fatalf("GetPersonaTemplate(deleted) error = %v", err)
	}
}

func TestResourceBindingsRejectCredentialMaterial(t *testing.T) {
	service, _ := newTestService(t)
	scope := testScope()
	detail := createPublishableDigitalHuman(t, service, scope, "Iris")
	_, err := service.ReplaceDraftResourceBindings(t.Context(), scope, detail.Definition.ID, ReplaceResourceBindingsRequest{
		ExpectedDraftLockVersion: detail.Draft.LockVersion,
		Items: []ResourceBindingInput{{
			Type: resourcebindings.BindingTypeIntegrationConnection, ResourceID: "connection-1", ParentResourceID: "feishu",
			AccessMode: resourcebindings.AccessModeExecute, ResourceRevision: "connection:v1",
			Metadata: map[string]any{"access_token": "must-not-persist"},
		}},
	})
	if !apperror.IsCode(err, AppCodeResourceBindingInvalid) || !errors.Is(err, ErrResourceBindingInvalid) {
		t.Fatalf("ReplaceDraftResourceBindings(secret) error = %v", err)
	}
}

func newTestService(t *testing.T) (*Service, *GormRepository) {
	t.Helper()
	dsn := "file:" + uuid.NewString() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(
		&Definition{}, &Version{}, &PersonaTemplate{}, &ResourceBinding{},
	); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(db)
	return NewService(repo), repo
}

func testScope() Scope {
	return Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ActorID: uuid.New()}
}

func createPublishableDigitalHuman(t *testing.T, service *Service, scope Scope, name string) *Detail {
	t.Helper()
	detail, err := service.Create(t.Context(), scope, CreateRequest{
		Name: name + " workspace", IdentityName: name, Persona: "A careful research partner", Goals: []string{"Complete assigned research"}, Rules: []string{"Cite evidence"}, DefaultModel: "gpt-5",
		MemoryPolicy: map[string]any{"enabled": false}, TriggerPolicy: map[string]any{"manual": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	return detail
}
