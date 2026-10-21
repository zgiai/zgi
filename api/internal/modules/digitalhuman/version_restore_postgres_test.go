package digitalhuman

import (
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"github.com/zgiai/zgi/api/internal/contracts/chatfile"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestDigitalHumanVersionRestorePostgres(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("ZGI_MIGRATION_TEST_DSN"))
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run DigitalHuman version restore integration")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })

	organizationID, workspaceID, actorID := uuid.New(), uuid.New(), uuid.New()
	if err := tx.Exec("INSERT INTO accounts (id, name, email) VALUES (?, ?, ?)", actorID, "Restore actor", "restore-"+actorID.String()+"@example.invalid").Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("INSERT INTO organizations (id, name) VALUES (?, ?)", organizationID, "DigitalHuman restore integration").Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("INSERT INTO workspaces (id, organization_id, name) VALUES (?, ?, ?)", workspaceID, organizationID, "Restore integration").Error; err != nil {
		t.Fatal(err)
	}

	scope := Scope{OrganizationID: organizationID, WorkspaceID: workspaceID, ActorID: actorID}
	service := NewService(NewRepository(tx))
	detail := createPublishableDigitalHuman(t, service, scope, "Restore integration")
	policyDraft, err := service.UpdateDraft(t.Context(), scope, detail.Definition.ID, UpdateDraftRequest{ExpectedLockVersion: detail.Draft.LockVersion, FileUploadPolicy: &chatfile.Policy{AllowedFileTypes: []string{"document"}}})
	if err != nil {
		t.Fatal(err)
	}
	detail.Draft = policyDraft
	bindings, err := service.ReplaceDraftResourceBindings(t.Context(), scope, detail.Definition.ID, ReplaceResourceBindingsRequest{
		ExpectedDraftLockVersion: detail.Draft.LockVersion,
		Items:                    []ResourceBindingInput{{Type: resourcebindings.BindingTypeSkill, ResourceID: "time", DisplayName: "Time", AccessMode: resourcebindings.AccessModeExecute, ResourceRevision: "time:v1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Publish(t.Context(), scope, detail.Definition.ID, PublishRequest{ExpectedRevision: detail.Definition.Revision, ExpectedDraftLockVersion: bindings.DraftLockVersion})
	if err != nil {
		t.Fatal(err)
	}
	changedPersona := "A changed draft persona"
	changed, err := service.UpdateDraft(t.Context(), scope, detail.Definition.ID, UpdateDraftRequest{ExpectedLockVersion: first.Draft.LockVersion, Persona: &changedPersona, FileUploadPolicy: &chatfile.Policy{AllowedFileTypes: []string{"image"}}})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.PreviewVersionRestore(t.Context(), scope, detail.Definition.ID, first.Published.ID)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := service.RestoreVersion(t.Context(), scope, detail.Definition.ID, RestoreVersionRequest{
		VersionID: first.Published.ID, ExpectedRevision: first.Definition.Revision,
		ExpectedDraftLockVersion: changed.LockVersion, ImpactToken: preview.ImpactToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	if restored.Published == nil || restored.Published.ID != first.Published.ID || restored.Draft.Persona != first.Published.Persona {
		t.Fatalf("restored detail = %#v", restored)
	}
	if restored.Draft.FileUploadPolicy == nil || len(restored.Draft.FileUploadPolicy.AllowedFileTypes) != 1 || restored.Draft.FileUploadPolicy.AllowedFileTypes[0] != "document" {
		t.Fatal("PostgreSQL version restore lost the published upload policy")
	}
}
