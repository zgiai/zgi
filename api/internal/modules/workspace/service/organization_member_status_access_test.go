package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	workspace_repo "github.com/zgiai/zgi/api/internal/modules/workspace/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type memberStatusWorkspaceManagementService struct {
	interfaces.WorkspaceManagementService
	db *gorm.DB
}

func (s memberStatusWorkspaceManagementService) GetByWorkspaceAndMember(ctx context.Context, workspaceID, accountID string) (*model.WorkspaceMember, error) {
	var join model.WorkspaceMember
	if err := s.db.WithContext(ctx).
		Where("workspace_id = ? AND account_id = ?", workspaceID, accountID).
		First(&join).Error; err != nil {
		return nil, err
	}
	return &join, nil
}

type memberStatusFixture struct {
	db             *gorm.DB
	service        *organizationService
	organizationID string
	workspaceID    string
	memberID       string
}

func newMemberStatusFixture(t *testing.T) *memberStatusFixture {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Organization{},
		&model.OrganizationMember{},
		&model.Workspace{},
		&model.WorkspaceMember{},
	))

	fixture := &memberStatusFixture{
		db:             db,
		organizationID: uuid.NewString(),
		workspaceID:    uuid.NewString(),
		memberID:       uuid.NewString(),
	}
	require.NoError(t, db.Create(&model.Organization{
		ID:     fixture.organizationID,
		Name:   "Organization",
		Status: model.OrganizationStatusActive,
	}).Error)
	require.NoError(t, db.Create(&model.Workspace{
		ID:             fixture.workspaceID,
		Name:           "Workspace",
		Status:         model.WorkspaceStatusNormal,
		OrganizationID: &fixture.organizationID,
	}).Error)
	require.NoError(t, db.Create(&model.OrganizationMember{
		OrganizationID: fixture.organizationID,
		AccountID:      fixture.memberID,
		Role:           model.OrganizationRoleNormal,
		Status:         model.OrganizationMemberStatusActive,
	}).Error)
	require.NoError(t, db.Create(&model.WorkspaceMember{
		ID:               uuid.NewString(),
		WorkspaceID:      fixture.workspaceID,
		AccountID:        fixture.memberID,
		Role:             model.WorkspaceRoleNormal,
		Permissions:      []string{string(model.WorkspacePermissionWorkspaceView), string(model.WorkspacePermissionAgentView)},
		PermissionSource: model.WorkspaceMemberPermissionSourceDirect,
	}).Error)

	fixture.service = &organizationService{
		db:                         db,
		organizationRepo:           workspace_repo.NewOrganizationRepository(db),
		workspaceRepo:              workspace_repo.NewWorkspaceRepository(db),
		workspaceManagementService: memberStatusWorkspaceManagementService{db: db},
	}
	return fixture
}

func (f *memberStatusFixture) setMemberStatus(t *testing.T, status model.OrganizationMemberStatus) {
	t.Helper()
	require.NoError(t, f.db.Model(&model.OrganizationMember{}).
		Where("organization_id = ? AND account_id = ?", f.organizationID, f.memberID).
		Update("status", status).Error)
}

func TestDisabledOrganizationMemberLosesWorkspaceAuthority(t *testing.T) {
	fixture := newMemberStatusFixture(t)
	ctx := context.Background()
	svc := fixture.service

	allowed, err := svc.CheckWorkspaceOrganizationAnyPermission(ctx, fixture.organizationID, fixture.workspaceID, fixture.memberID, model.WorkspacePermissionAgentView)
	require.NoError(t, err)
	require.True(t, allowed, "active member keeps its workspace permission")

	fixture.setMemberStatus(t, model.OrganizationMemberStatusInactive)

	allowed, err = svc.CheckWorkspaceOrganizationAnyPermission(ctx, fixture.organizationID, fixture.workspaceID, fixture.memberID, model.WorkspacePermissionAgentView)
	require.NoError(t, err)
	require.False(t, allowed)

	allowed, err = svc.CheckWorkspacePermission(ctx, fixture.organizationID, fixture.workspaceID, fixture.memberID, model.WorkspacePermissionAgentView)
	require.NoError(t, err)
	require.False(t, allowed)

	isMember, err := svc.IsOrganizationMember(ctx, fixture.organizationID, fixture.memberID)
	require.NoError(t, err)
	require.False(t, isMember)

	hasMembership, err := svc.HasOrganizationMembership(ctx, fixture.organizationID, fixture.memberID)
	require.NoError(t, err)
	require.True(t, hasMembership, "administrators can still look up the disabled member")

	workspaces, err := svc.GetUserAllWorkspacesInOrganization(ctx, fixture.organizationID, fixture.memberID)
	require.NoError(t, err)
	require.Empty(t, workspaces)

	permissions, err := svc.GetWorkspaceMemberPermissions(ctx, fixture.organizationID, fixture.workspaceID, fixture.memberID, fixture.memberID)
	require.NoError(t, err)
	require.Empty(t, permissions.Permissions)

	fixture.setMemberStatus(t, model.OrganizationMemberStatusActive)

	allowed, err = svc.CheckWorkspaceOrganizationAnyPermission(ctx, fixture.organizationID, fixture.workspaceID, fixture.memberID, model.WorkspacePermissionAgentView)
	require.NoError(t, err)
	require.True(t, allowed, "re-enabled member regains its stored permissions")
}

func TestDisabledOrganizationAdminLosesOrganizationBypass(t *testing.T) {
	fixture := newMemberStatusFixture(t)
	ctx := context.Background()
	adminID := uuid.NewString()
	require.NoError(t, fixture.db.Create(&model.OrganizationMember{
		OrganizationID: fixture.organizationID,
		AccountID:      adminID,
		Role:           model.OrganizationRoleAdmin,
		Status:         model.OrganizationMemberStatusInactive,
	}).Error)

	require.False(t, fixture.service.checkOrganizationAdmin(ctx, adminID, fixture.organizationID))

	allowed, err := fixture.service.CheckWorkspaceOrganizationAnyPermission(ctx, fixture.organizationID, fixture.workspaceID, adminID, model.WorkspacePermissionAgentView)
	require.NoError(t, err)
	require.False(t, allowed)

	isAdmin, err := fixture.service.IsOrganizationAdminOrOwner(ctx, fixture.organizationID, adminID)
	require.NoError(t, err)
	require.False(t, isAdmin)
}

func TestListUserOrganizationsExcludesDisabledMembership(t *testing.T) {
	fixture := newMemberStatusFixture(t)
	ctx := context.Background()
	repo := workspace_repo.NewOrganizationRepository(fixture.db)

	organizations, total, err := repo.ListUserOrganizationsForAccount(ctx, 1, 20, "", fixture.memberID)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, organizations, 1)

	fixture.setMemberStatus(t, model.OrganizationMemberStatusInactive)

	organizations, total, err = repo.ListUserOrganizationsForAccount(ctx, 1, 20, "", fixture.memberID)
	require.NoError(t, err)
	require.EqualValues(t, 0, total)
	require.Empty(t, organizations)

	_, err = repo.GetFirstJoinedOrganization(ctx, fixture.memberID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}
