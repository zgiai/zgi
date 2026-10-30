package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	shared_dto "github.com/zgiai/zgi/api/internal/dto"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	"github.com/zgiai/zgi/api/internal/modules/workspace/memberstatus"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	workspace_repo "github.com/zgiai/zgi/api/internal/modules/workspace/repository"
	"github.com/zgiai/zgi/api/pkg/apperror"
)

// addWorkspaceAdmin adds an account that manages the fixture workspace through
// its workspace role. withMembersRow false models legacy data that predates
// organization membership rows.
func (f *memberStatusFixture) addWorkspaceAdmin(t *testing.T, withMembersRow bool) string {
	t.Helper()
	accountID := uuid.NewString()
	if withMembersRow {
		require.NoError(t, f.db.Create(&model.OrganizationMember{
			OrganizationID: f.organizationID,
			AccountID:      accountID,
			Role:           model.OrganizationRoleNormal,
			Status:         model.OrganizationMemberStatusActive,
		}).Error)
	}
	join := &model.WorkspaceMember{
		ID:          uuid.NewString(),
		WorkspaceID: f.workspaceID,
		AccountID:   accountID,
		Role:        model.WorkspaceRoleAdmin,
	}
	model.ApplyWorkspaceMemberDefaults(join)
	require.NoError(t, f.db.Create(join).Error)
	return accountID
}

func (f *memberStatusFixture) setStatus(t *testing.T, accountID string, status model.OrganizationMemberStatus) {
	t.Helper()
	require.NoError(t, f.db.Model(&model.OrganizationMember{}).
		Where("organization_id = ? AND account_id = ?", f.organizationID, accountID).
		Update("status", status).Error)
}

func TestWorkspaceManagementPermissionRejectsDisabledOrganizationMember(t *testing.T) {
	fixture := newMemberStatusFixture(t)
	ctx := context.Background()
	svc := &WorkspaceManagementServiceImpl{
		db:                  fixture.db,
		workspaceRepo:       workspace_repo.NewWorkspaceRepository(fixture.db),
		workspaceMemberRepo: workspace_repo.NewWorkspaceMemberRepository(fixture.db),
	}
	adminID := fixture.addWorkspaceAdmin(t, true)
	legacyAdminID := fixture.addWorkspaceAdmin(t, false)

	require.True(t, svc.CheckPermission(ctx, fixture.workspaceID, adminID))
	role, err := svc.GetUserRole(ctx, adminID, fixture.workspaceID)
	require.NoError(t, err)
	require.NotNil(t, role)

	fixture.setStatus(t, adminID, model.OrganizationMemberStatusInactive)

	require.False(t, svc.CheckPermission(ctx, fixture.workspaceID, adminID))
	role, err = svc.GetUserRole(ctx, adminID, fixture.workspaceID)
	require.NoError(t, err)
	require.Nil(t, role, "a disabled member holds no workspace role")

	require.True(t, svc.CheckPermission(ctx, fixture.workspaceID, legacyAdminID), "legacy workspace-only access is kept")
	role, err = svc.GetUserRole(ctx, legacyAdminID, fixture.workspaceID)
	require.NoError(t, err)
	require.NotNil(t, role)

	fixture.setStatus(t, adminID, model.OrganizationMemberStatusActive)
	require.True(t, svc.CheckPermission(ctx, fixture.workspaceID, adminID))
}

func TestAccessibleWorkspacesFilterIsEmptyForDisabledMember(t *testing.T) {
	fixture := newMemberStatusFixture(t)
	ctx := context.Background()
	filter := NewWorkspacePermissionFilterService(
		workspace_repo.NewOrganizationRepository(fixture.db),
		workspace_repo.NewWorkspaceRepository(fixture.db),
		workspace_repo.NewWorkspaceMemberRepository(fixture.db),
	)
	adminID := fixture.addWorkspaceAdmin(t, true)
	legacyAdminID := fixture.addWorkspaceAdmin(t, false)

	workspaces, err := filter.GetAccessibleWorkspacesByPermission(ctx, adminID, fixture.organizationID, "create_agent")
	require.NoError(t, err)
	require.Len(t, workspaces, 1)

	fixture.setStatus(t, adminID, model.OrganizationMemberStatusInactive)

	workspaces, err = filter.GetAccessibleWorkspacesByPermission(ctx, adminID, fixture.organizationID, "create_agent")
	require.NoError(t, err)
	require.Empty(t, workspaces)

	workspaces, err = filter.GetAccessibleWorkspacesByPermission(ctx, legacyAdminID, fixture.organizationID, "create_agent")
	require.NoError(t, err)
	require.Len(t, workspaces, 1, "legacy workspace-only access is kept")
}

type memberStatusUpdateAccountService struct {
	interfaces.AccountService
}

func (memberStatusUpdateAccountService) GetAccountByID(_ context.Context, id string) (*auth_model.Account, error) {
	return &auth_model.Account{ID: id, Name: id, Status: auth_model.AccountStatusActive}, nil
}

func (memberStatusUpdateAccountService) InvalidateAccountProfileCache(string) {}

func TestDisablingMemberDeactivatesPersonalKeysWithoutReactivating(t *testing.T) {
	fixture := newMemberStatusFixture(t)
	ctx := context.Background()
	require.NoError(t, fixture.db.AutoMigrate(&apikeymodel.TenantAPIKey{}))
	fixture.service.accountService = memberStatusUpdateAccountService{}

	userPrincipal := "user"
	otherMemberID := uuid.NewString()
	otherOrganizationID := uuid.NewString()
	newKey := func(organizationID, principalID string, personal bool) string {
		key := &apikeymodel.TenantAPIKey{
			ID:             uuid.NewString(),
			OrganizationID: organizationID,
			KeyHash:        uuid.NewString(),
			Name:           "key",
			Status:         "active",
			WorkspaceID:    &fixture.workspaceID,
		}
		if personal {
			principal := principalID
			key.PrincipalType = &userPrincipal
			key.PrincipalID = &principal
		}
		require.NoError(t, fixture.db.Create(key).Error)
		return key.ID
	}
	memberKey := newKey(fixture.organizationID, fixture.memberID, true)
	otherMemberKey := newKey(fixture.organizationID, otherMemberID, true)
	otherOrganizationKey := newKey(otherOrganizationID, fixture.memberID, true)
	organizationKey := newKey(fixture.organizationID, "", false)
	keyStatus := func(id string) string {
		var key apikeymodel.TenantAPIKey
		require.NoError(t, fixture.db.Where("id = ?", id).First(&key).Error)
		return key.Status
	}

	require.NoError(t, fixture.service.UpdateMemberStatus(ctx, &shared_dto.UpdateOrganizationMemberStatusRequest{
		OrganizationID: fixture.organizationID,
		AccountID:      fixture.memberID,
		Status:         model.OrganizationMemberStatusInactive,
	}))

	require.Equal(t, "inactive", keyStatus(memberKey))
	require.Equal(t, "active", keyStatus(otherMemberKey))
	require.Equal(t, "active", keyStatus(otherOrganizationKey))
	require.Equal(t, "active", keyStatus(organizationKey))
	disabled, err := fixture.service.IsOrganizationMemberDisabled(ctx, fixture.organizationID, fixture.memberID)
	require.NoError(t, err)
	require.True(t, disabled)

	require.NoError(t, fixture.service.UpdateMemberStatus(ctx, &shared_dto.UpdateOrganizationMemberStatusRequest{
		OrganizationID: fixture.organizationID,
		AccountID:      fixture.memberID,
		Status:         model.OrganizationMemberStatusActive,
	}))
	require.Equal(t, "inactive", keyStatus(memberKey), "re-enabling leaves the member to turn keys back on")
}

func TestAcceptInviteRefusesDisabledOrganizationMember(t *testing.T) {
	for _, scoped := range []bool{true, false} {
		name := "organization only"
		if scoped {
			name = "department and workspace"
		}
		t.Run(name, func(t *testing.T) {
			db := newOrganizationInviteAcceptanceDB(t)
			organizationID := uuid.NewString()
			workspaceID := uuid.NewString()
			departmentID := uuid.NewString()
			accountID := uuid.NewString()
			previousOrganizationID := uuid.NewString()
			require.NoError(t, db.Create(&model.Organization{ID: organizationID, Name: "Target", Status: model.OrganizationStatusActive}).Error)
			require.NoError(t, db.Create(&model.Workspace{ID: workspaceID, Name: "Target Workspace", OrganizationID: &organizationID, Status: model.WorkspaceStatusNormal}).Error)
			require.NoError(t, db.Create(&model.Department{ID: departmentID, OrganizationID: organizationID, Name: "Target Department", Status: model.DepartmentStatusActive}).Error)
			require.NoError(t, db.Create(&auth_model.Account{ID: accountID, Email: "disabled@example.com", Name: "Disabled", Status: auth_model.AccountStatusActive}).Error)
			require.NoError(t, db.Create(&model.OrganizationMember{
				OrganizationID: organizationID, AccountID: accountID, Role: model.OrganizationRoleNormal,
				Status: model.OrganizationMemberStatusInactive,
			}).Error)
			require.NoError(t, db.Create(&auth_model.AccountContext{AccountID: accountID, CurrentOrganizationID: &previousOrganizationID}).Error)
			link := &model.OrganizationInviteLink{
				OrganizationID: organizationID, Token: "disabled-member-invite", Status: "active",
				DefaultOrganizationRole: string(model.OrganizationRoleNormal),
				DefaultWorkspaceRole:    string(model.WorkspaceRoleMember), CreatedBy: uuid.NewString(),
			}
			if scoped {
				link.DepartmentID = &departmentID
				link.WorkspaceID = &workspaceID
			}
			repository := workspace_repo.NewOrganizationRepository(db)
			require.NoError(t, repository.CreateInviteLink(t.Context(), link))
			service := &organizationService{
				db: db, organizationRepo: repository,
				workspaceManagementService: &inviteAcceptanceWorkspaceService{db: db},
			}

			_, err := service.AcceptInviteByToken(t.Context(), link.Token, accountID, nil)

			require.ErrorIs(t, err, ErrOrganizationInviteMemberDisabled)
			require.True(t, apperror.IsCode(err, memberstatus.AppCodeMemberDisabled))
			require.Zero(t, countRows(t, db, &model.OrganizationJoinRequest{}, "account_id = ?", accountID))
			require.Zero(t, countRows(t, db, &model.DepartmentMember{}, "account_id = ?", accountID))
			require.Zero(t, countRows(t, db, &model.WorkspaceMember{}, "account_id = ?", accountID))
			var accountContext auth_model.AccountContext
			require.NoError(t, db.Where("account_id = ?", accountID).First(&accountContext).Error)
			require.Equal(t, previousOrganizationID, *accountContext.CurrentOrganizationID)
			var member model.OrganizationMember
			require.NoError(t, db.Where("organization_id = ? AND account_id = ?", organizationID, accountID).First(&member).Error)
			require.Equal(t, model.OrganizationMemberStatusInactive, member.Status)
		})
	}
}

func TestApprovingPendingInviteRefusesMemberDisabledAfterRequest(t *testing.T) {
	db := newOrganizationInviteAcceptanceDB(t)
	organizationID := uuid.NewString()
	workspaceID := uuid.NewString()
	departmentID := uuid.NewString()
	accountID := uuid.NewString()
	require.NoError(t, db.Create(&model.Organization{ID: organizationID, Name: "Target", Status: model.OrganizationStatusActive}).Error)
	require.NoError(t, db.Create(&model.Workspace{ID: workspaceID, Name: "Target Workspace", OrganizationID: &organizationID, Status: model.WorkspaceStatusNormal}).Error)
	require.NoError(t, db.Create(&model.Department{ID: departmentID, OrganizationID: organizationID, Name: "Target Department", Status: model.DepartmentStatusActive}).Error)
	require.NoError(t, db.Create(&auth_model.Account{ID: accountID, Email: "member@example.com", Name: "Member", Status: auth_model.AccountStatusActive}).Error)
	require.NoError(t, db.Create(&model.OrganizationMember{
		OrganizationID: organizationID, AccountID: accountID, Role: model.OrganizationRoleNormal,
	}).Error)
	link := &model.OrganizationInviteLink{
		OrganizationID: organizationID, DepartmentID: &departmentID, WorkspaceID: &workspaceID,
		Token: "pending-then-disabled", Status: "active", RequireApproval: true,
		DefaultOrganizationRole: string(model.OrganizationRoleNormal),
		DefaultWorkspaceRole:    string(model.WorkspaceRoleMember), CreatedBy: uuid.NewString(),
	}
	repository := workspace_repo.NewOrganizationRepository(db)
	require.NoError(t, repository.CreateInviteLink(t.Context(), link))
	service := &organizationService{
		db: db, organizationRepo: repository,
		workspaceManagementService: &inviteAcceptanceWorkspaceService{db: db},
	}
	pending, err := service.AcceptInviteByToken(t.Context(), link.Token, accountID, nil)
	require.NoError(t, err)
	require.Equal(t, model.OrganizationJoinRequestStatusPending, pending.Status)

	require.NoError(t, db.Model(&model.OrganizationMember{}).
		Where("organization_id = ? AND account_id = ?", organizationID, accountID).
		Update("status", model.OrganizationMemberStatusInactive).Error)

	_, err = service.ApproveDepartmentJoinRequest(t.Context(), organizationID, pending.ID, uuid.NewString())

	require.ErrorIs(t, err, ErrOrganizationInviteMemberDisabled)
	require.Zero(t, countRows(t, db, &model.DepartmentMember{}, "account_id = ?", accountID))
	require.Zero(t, countRows(t, db, &model.WorkspaceMember{}, "account_id = ?", accountID))
	var request model.OrganizationJoinRequest
	require.NoError(t, db.Where("id = ?", pending.ID).First(&request).Error)
	require.Equal(t, model.OrganizationJoinRequestStatusPending, request.Status, "the failed approval rolls back")
}
