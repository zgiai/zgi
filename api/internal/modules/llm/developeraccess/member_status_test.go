package developeraccess

import (
	"context"
	"errors"
	"testing"

	apikeyrepo "github.com/zgiai/zgi/api/internal/modules/llm/apikey/repository"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

func TestDisabledOrganizationMemberLosesDeveloperAccess(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, organizationID, ownerID, memberID := seedDeveloperWorkspace(t, db)
	if err := db.Create(&workspacemodel.OrganizationMember{
		OrganizationID: organizationID, AccountID: ownerID,
		Role: workspacemodel.OrganizationRoleNormal, Status: workspacemodel.OrganizationMemberStatusActive,
	}).Error; err != nil {
		t.Fatal(err)
	}
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)
	ctx := context.Background()

	if _, err := service.GetMe(ctx, workspaceID, ownerID); err != nil {
		t.Fatalf("active workspace owner: %v", err)
	}
	if _, err := service.ListKeys(ctx, workspaceID, ownerID, KeyQuery{Scope: "all"}); err != nil {
		t.Fatalf("active workspace owner key list: %v", err)
	}
	var workspace workspacemodel.Workspace
	if err := db.First(&workspace, "id = ?", workspaceID).Error; err != nil {
		t.Fatal(err)
	}
	staleScope := &workspaceScope{Workspace: &workspace, CanManage: true}

	if err := db.Model(&workspacemodel.OrganizationMember{}).
		Where("organization_id = ? AND account_id = ?", organizationID, ownerID).
		Update("status", workspacemodel.OrganizationMemberStatusInactive).Error; err != nil {
		t.Fatal(err)
	}

	// The workspace owner row survives the disabled membership, which used to
	// keep developer access and key management reachable.
	if _, err := service.GetMe(ctx, workspaceID, ownerID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("GetMe error = %v, want forbidden", err)
	}
	if _, err := service.ListKeys(ctx, workspaceID, ownerID, KeyQuery{Scope: "all"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ListKeys error = %v, want forbidden", err)
	}
	tx := db.Begin()
	defer tx.Rollback()
	if _, err := service.lockScopeForUpdate(ctx, tx, staleScope, ownerID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("lockScopeForUpdate error = %v, want forbidden", err)
	}

	// A member without a members row is legacy data and keeps its access.
	if _, err := service.GetMe(ctx, workspaceID, memberID); err != nil {
		t.Fatalf("legacy workspace member: %v", err)
	}
}
