package digitalhuman

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Workflow rows published before the parent was keyed by the workflow Agent id
// keep an empty parent. Runtime re-verification asks with the workflow Agent id
// as parent and must still find them, but only when the metadata names the
// same workflow Agent.
func TestVerifyAcceptsPublishedWorkflowBindingWithLegacyEmptyParent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&Definition{}, &Version{}, &ResourceBinding{}); err != nil {
		t.Fatal(err)
	}
	scope := testScope()
	now := time.Now().UTC()
	definition := &Definition{ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, Name: "Atlas", CreatedAt: now, UpdatedAt: now}
	version := &Version{ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, DigitalHumanID: definition.ID, State: VersionStatePublished, IdentityName: "Atlas", CreatedAt: now, UpdatedAt: now}
	if err := db.Create(definition).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(version).Error; err != nil {
		t.Fatal(err)
	}
	agentID, otherAgentID := uuid.NewString(), uuid.NewString()
	seed := func(resourceID, metadataAgentID string) {
		raw, err := safeJSON(map[string]any{"workflow_agent_id": metadataAgentID, "workflow_id": "workflow", "version_uuid": "version"})
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&ResourceBinding{
			ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
			DigitalHumanID: definition.ID, DigitalHumanVersionID: version.ID,
			BindingType: resourcebindings.BindingTypeWorkflow, ResourceID: resourceID, ParentResourceID: "",
			DisplayName: "Approval", AccessMode: resourcebindings.AccessModeExecute, ResourceRevision: "sha256:rev", BindingRevision: "rev",
			Metadata: raw, AuthorizedBy: &scope.ActorID, AuthorizedAt: &now, CreatedAt: now, UpdatedAt: now,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	seed(agentID, agentID)
	mismatchedID := uuid.NewString()
	seed(mismatchedID, otherAgentID)

	repo := NewRepository(db)
	ref := resourcebindings.SnapshotRef{
		Subject: runtimeidentity.SubjectRef{Type: runtimeidentity.SubjectTypeDigitalHuman, ID: definition.ID, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID},
		Phase:   resourcebindings.PhasePublished, VersionID: &version.ID,
	}
	verify := func(resourceID, parent string) bool {
		decision, err := repo.Verify(context.Background(), ref, resourcebindings.Match{
			Type: resourcebindings.BindingTypeWorkflow, ResourceID: resourceID, ParentResourceID: parent, AccessMode: resourcebindings.AccessModeExecute,
		})
		if err != nil {
			t.Fatal(err)
		}
		return decision.Allowed
	}
	if !verify(agentID, agentID) {
		t.Fatal("legacy workflow binding with empty parent was not verified for its workflow Agent")
	}
	if verify(mismatchedID, mismatchedID) || verify(agentID, otherAgentID) {
		t.Fatal("legacy workflow binding was verified for a different workflow Agent")
	}
	if !verify(agentID, "") {
		t.Fatal("an exact empty-parent lookup must keep working")
	}
}
