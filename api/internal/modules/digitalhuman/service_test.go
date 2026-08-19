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
	if !strings.HasPrefix(publishedDetail.Published.Revision, versionRevisionPrefix) || len(publishedDetail.Published.IdentitySnapshot) == 0 {
		t.Fatalf("published revision/snapshot = %q / %s", publishedDetail.Published.Revision, publishedDetail.Published.IdentitySnapshot)
	}
	if publishedDetail.Draft.ID == publishedDetail.Published.ID || publishedDetail.Draft.State != VersionStateDraft || publishedDetail.Draft.LockVersion != 1 {
		t.Fatalf("next draft = %#v", publishedDetail.Draft)
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

func TestServiceRejectsDuplicateWorkspaceName(t *testing.T) {
	service, _ := newTestService(t)
	scope := testScope()
	if _, err := service.Create(t.Context(), scope, CreateRequest{Name: "Atlas"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(t.Context(), scope, CreateRequest{Name: " atlas "}); !apperror.IsCode(err, AppCodeNameConflict) {
		t.Fatalf("Create(duplicate) error = %v, want name conflict", err)
	}
}

func TestPublishedVersionsAreNewestFirst(t *testing.T) {
	service, _ := newTestService(t)
	scope := testScope()
	detail := createPublishableDigitalHuman(t, service, scope, "Echo")
	first, err := service.Publish(t.Context(), scope, detail.Definition.ID, PublishRequest{
		ExpectedRevision:         detail.Definition.Revision,
		ExpectedDraftLockVersion: detail.Draft.LockVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Publish(t.Context(), scope, detail.Definition.ID, PublishRequest{
		ExpectedRevision:         first.Definition.Revision,
		ExpectedDraftLockVersion: first.Draft.LockVersion,
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
}

func TestReusableRoleAndBindingsRemainPinnedAcrossDigitalHumans(t *testing.T) {
	service, repo := newTestService(t)
	scope := testScope()
	role, err := service.CreateRoleTemplate(t.Context(), scope, CreateRoleTemplateRequest{
		Name: "Growth researcher", Goals: []string{"Find qualified opportunities"},
		Responsibilities: []string{"Research evidence before recommending actions"},
	})
	if err != nil {
		t.Fatal(err)
	}
	role, err = service.PublishRoleTemplate(t.Context(), scope, role.Template.ID, PublishRoleTemplateRequest{
		ExpectedRevision: role.Template.Revision, ExpectedDraftLockVersion: role.Draft.LockVersion,
	})
	if err != nil {
		t.Fatal(err)
	}

	first := createPublishableDigitalHuman(t, service, scope, "Atlas")
	second := createPublishableDigitalHuman(t, service, scope, "Nova")
	for _, detail := range []*Detail{first, second} {
		assigned, assignErr := service.ReplaceDraftRoleAssignments(t.Context(), scope, detail.Definition.ID, ReplaceRoleAssignmentsRequest{
			ExpectedDraftLockVersion: detail.Draft.LockVersion,
			Items:                    []RoleAssignmentInput{{RoleTemplateVersionID: role.Published.ID, Parameters: map[string]any{"region": "APAC"}}},
		})
		if assignErr != nil {
			t.Fatal(assignErr)
		}
		detail.Draft.LockVersion = assigned.DraftLockVersion
	}

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

	publishedAssignments, err := repo.ListRoleAssignments(t.Context(), scope, first.Definition.ID, first.Published.ID)
	if err != nil || len(publishedAssignments) != 1 || publishedAssignments[0].RoleTemplateVersionID != role.Published.ID {
		t.Fatalf("published assignments = %#v, %v", publishedAssignments, err)
	}
	nextDraftAssignments, err := repo.ListRoleAssignments(t.Context(), scope, first.Definition.ID, first.Draft.ID)
	if err != nil || len(nextDraftAssignments) != 1 || nextDraftAssignments[0].ID == publishedAssignments[0].ID {
		t.Fatalf("next draft assignments = %#v, %v", nextDraftAssignments, err)
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
		&Definition{}, &Version{}, &RoleTemplate{}, &RoleTemplateVersion{}, &RoleAssignment{}, &ResourceBinding{},
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
		Name: name, Persona: "A careful research partner", Rules: []string{"Cite evidence"}, DefaultModel: "gpt-5",
		MemoryPolicy: map[string]any{"enabled": false}, TriggerPolicy: map[string]any{"manual": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	return detail
}
