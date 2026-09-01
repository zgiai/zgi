package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	workspace_repo "github.com/zgiai/zgi/api/internal/modules/workspace/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAcceptInviteByTokenCreatesTargetScopeTransactionally(t *testing.T) {
	db := newOrganizationInviteAcceptanceDB(t)
	organizationID := uuid.NewString()
	workspaceID := uuid.NewString()
	departmentID := uuid.NewString()
	accountID := uuid.NewString()
	require.NoError(t, db.Create(&model.Organization{ID: organizationID, Name: "Target", Status: model.OrganizationStatusActive}).Error)
	require.NoError(t, db.Create(&model.Workspace{ID: workspaceID, Name: "Target Workspace", OrganizationID: &organizationID, Status: model.WorkspaceStatusNormal}).Error)
	require.NoError(t, db.Create(&model.Department{ID: departmentID, OrganizationID: organizationID, Name: "Target Department", Status: model.DepartmentStatusActive}).Error)
	require.NoError(t, db.Create(&auth_model.Account{ID: accountID, Email: "invitee@example.com", Name: "Invitee", Status: auth_model.AccountStatusActive}).Error)
	link := &model.OrganizationInviteLink{
		OrganizationID:          organizationID,
		DepartmentID:            &departmentID,
		WorkspaceID:             &workspaceID,
		Token:                   "registration-invite",
		Status:                  "active",
		RequireApproval:         false,
		DefaultOrganizationRole: string(model.OrganizationRoleNormal),
		DefaultWorkspaceRole:    string(model.WorkspaceRoleMember),
		CreatedBy:               uuid.NewString(),
	}
	repository := workspace_repo.NewOrganizationRepository(db)
	require.NoError(t, repository.CreateInviteLink(t.Context(), link))
	accountService := &inviteAcceptanceAccountService{}

	service := &organizationService{
		db:                         db,
		organizationRepo:           repository,
		accountService:             accountService,
		workspaceManagementService: &inviteAcceptanceWorkspaceService{db: db},
	}
	validated, err := service.ValidateInviteLinkForRegistration(t.Context(), link.Token)
	require.NoError(t, err)
	require.NotNil(t, validated.WorkspaceID)
	require.Equal(t, workspaceID, *validated.WorkspaceID)

	accepted, err := service.AcceptInviteByToken(t.Context(), link.Token, accountID, nil)
	require.NoError(t, err)
	require.Equal(t, model.OrganizationJoinRequestStatusApproved, accepted.Status)
	require.Equal(t, int64(1), countRows(t, db, &model.OrganizationMember{}, "organization_id = ? AND account_id = ?", organizationID, accountID))
	require.Equal(t, int64(1), countRows(t, db, &model.DepartmentMember{}, "department_id = ? AND account_id = ?", departmentID, accountID))
	require.Equal(t, int64(1), countRows(t, db, &model.WorkspaceMember{}, "workspace_id = ? AND account_id = ?", workspaceID, accountID))

	var workspaceMember model.WorkspaceMember
	require.NoError(t, db.Where("workspace_id = ? AND account_id = ?", workspaceID, accountID).First(&workspaceMember).Error)
	require.True(t, workspaceMember.Current)
	var accountContext auth_model.AccountContext
	require.NoError(t, db.Where("account_id = ?", accountID).First(&accountContext).Error)
	require.Equal(t, organizationID, *accountContext.CurrentOrganizationID)
	require.Equal(t, workspaceID, *accountContext.CurrentWorkspaceID)
	require.Equal(t, []string{accountID}, accountService.invalidatedAccountIDs)

	// A retry after a lost response is idempotent and does not duplicate grants.
	accepted, err = service.AcceptInviteByToken(t.Context(), link.Token, accountID, nil)
	require.NoError(t, err)
	require.Equal(t, model.OrganizationJoinRequestStatusApproved, accepted.Status)
	require.Equal(t, int64(1), countRows(t, db, &model.OrganizationMember{}, "organization_id = ? AND account_id = ?", organizationID, accountID))
	require.Equal(t, int64(1), countRows(t, db, &model.WorkspaceMember{}, "workspace_id = ? AND account_id = ?", workspaceID, accountID))
	require.Equal(t, []string{accountID, accountID}, accountService.invalidatedAccountIDs)
}

func TestOrganizationOnlyInviteDoesNotGrantWorkspaceAccess(t *testing.T) {
	db := newOrganizationInviteAcceptanceDB(t)
	organizationID := uuid.NewString()
	workspaceID := uuid.NewString()
	oldOrganizationID := uuid.NewString()
	oldWorkspaceID := uuid.NewString()
	accountID := uuid.NewString()
	require.NoError(t, db.Create(&model.Organization{ID: organizationID, Name: "Target", Status: model.OrganizationStatusActive}).Error)
	require.NoError(t, db.Create(&model.Organization{ID: oldOrganizationID, Name: "Previous", Status: model.OrganizationStatusActive}).Error)
	require.NoError(t, db.Create(&model.Workspace{ID: workspaceID, Name: "Not Invited", OrganizationID: &organizationID, Status: model.WorkspaceStatusNormal}).Error)
	require.NoError(t, db.Create(&model.Workspace{ID: oldWorkspaceID, Name: "Previous Workspace", OrganizationID: &oldOrganizationID, Status: model.WorkspaceStatusNormal}).Error)
	require.NoError(t, db.Create(&auth_model.Account{ID: accountID, Email: "org-only@example.com", Name: "Org Only", Status: auth_model.AccountStatusActive}).Error)
	require.NoError(t, db.Create(&model.OrganizationMember{
		OrganizationID: oldOrganizationID, AccountID: accountID, Role: model.OrganizationRoleNormal,
	}).Error)
	require.NoError(t, db.Create(&model.OrganizationMember{
		OrganizationID: organizationID, AccountID: accountID, Role: model.OrganizationRoleNormal,
	}).Error)
	oldWorkspaceMember := &model.WorkspaceMember{
		WorkspaceID: oldWorkspaceID, AccountID: accountID, Role: model.WorkspaceRoleMember, Current: true,
	}
	model.ApplyWorkspaceMemberDefaults(oldWorkspaceMember)
	require.NoError(t, db.Create(oldWorkspaceMember).Error)
	require.NoError(t, db.Create(&auth_model.AccountContext{
		AccountID: accountID, CurrentOrganizationID: &oldOrganizationID, CurrentWorkspaceID: &oldWorkspaceID,
	}).Error)
	link := &model.OrganizationInviteLink{
		OrganizationID:          organizationID,
		Token:                   "organization-only-invite",
		Status:                  "active",
		RequireApproval:         false,
		DefaultOrganizationRole: string(model.OrganizationRoleNormal),
		DefaultWorkspaceRole:    string(model.WorkspaceRoleMember),
		CreatedBy:               uuid.NewString(),
	}
	repository := workspace_repo.NewOrganizationRepository(db)
	require.NoError(t, repository.CreateInviteLink(t.Context(), link))
	service := &organizationService{
		db:                         db,
		organizationRepo:           repository,
		workspaceManagementService: &inviteAcceptanceWorkspaceService{db: db},
	}

	validated, err := service.ValidateInviteLinkForRegistration(t.Context(), link.Token)
	require.NoError(t, err)
	require.Nil(t, validated.WorkspaceID)
	_, err = service.AcceptInviteByToken(t.Context(), link.Token, accountID, nil)
	require.NoError(t, err)
	require.Zero(t, countRows(t, db, &model.WorkspaceMember{}, "workspace_id = ? AND account_id = ?", workspaceID, accountID))
	var accountContext auth_model.AccountContext
	require.NoError(t, db.Where("account_id = ?", accountID).First(&accountContext).Error)
	require.Equal(t, organizationID, *accountContext.CurrentOrganizationID)
	require.Nil(t, accountContext.CurrentWorkspaceID)
	var previousWorkspaceMember model.WorkspaceMember
	require.NoError(t, db.Where("workspace_id = ? AND account_id = ?", oldWorkspaceID, accountID).First(&previousWorkspaceMember).Error)
	require.False(t, previousWorkspaceMember.Current)
}

func TestApprovalRequiredInviteRemainsRetryableUntilApproval(t *testing.T) {
	db := newOrganizationInviteAcceptanceDB(t)
	organizationID := uuid.NewString()
	accountID := uuid.NewString()
	require.NoError(t, db.Create(&model.Organization{ID: organizationID, Name: "Target", Status: model.OrganizationStatusActive}).Error)
	require.NoError(t, db.Create(&auth_model.Account{ID: accountID, Email: "pending@example.com", Name: "Pending", Status: auth_model.AccountStatusActive}).Error)
	link := &model.OrganizationInviteLink{
		OrganizationID:          organizationID,
		Token:                   "pending-invite",
		Status:                  "active",
		RequireApproval:         true,
		DefaultOrganizationRole: string(model.OrganizationRoleNormal),
		DefaultWorkspaceRole:    string(model.WorkspaceRoleMember),
		CreatedBy:               uuid.NewString(),
	}
	repository := workspace_repo.NewOrganizationRepository(db)
	require.NoError(t, repository.CreateInviteLink(t.Context(), link))
	accountService := &inviteAcceptanceAccountService{}
	service := &organizationService{db: db, organizationRepo: repository, accountService: accountService}

	first, err := service.AcceptInviteByToken(t.Context(), link.Token, accountID, nil)
	require.NoError(t, err)
	require.Equal(t, model.OrganizationJoinRequestStatusPending, first.Status)
	second, err := service.AcceptInviteByToken(t.Context(), link.Token, accountID, nil)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, int64(1), countRows(t, db, &model.OrganizationJoinRequest{}, "group_id = ? AND account_id = ?", organizationID, accountID))
	require.Zero(t, countRows(t, db, &model.OrganizationMember{}, "organization_id = ? AND account_id = ?", organizationID, accountID))
	accountService.invalidatedAccountIDs = nil

	approved, err := service.ApproveDepartmentJoinRequest(t.Context(), organizationID, first.ID, uuid.NewString())
	require.NoError(t, err)
	require.Equal(t, model.OrganizationJoinRequestStatusApproved, approved.Status)
	require.Equal(t, int64(1), countRows(t, db, &model.OrganizationMember{}, "organization_id = ? AND account_id = ?", organizationID, accountID))
	require.Equal(t, []string{accountID}, accountService.invalidatedAccountIDs)
}

func TestApproveInviteRevalidatesActiveTargetInsideTransaction(t *testing.T) {
	tests := []struct {
		name       string
		deactivate func(*gorm.DB, string, string, string) error
		wantError  string
	}{
		{
			name: "organization",
			deactivate: func(db *gorm.DB, organizationID, _, _ string) error {
				return db.Model(&model.Organization{}).
					Where("id = ?", organizationID).
					Update("status", model.OrganizationStatusInactive).Error
			},
			wantError: "invited organization is unavailable",
		},
		{
			name: "department",
			deactivate: func(db *gorm.DB, _, departmentID, _ string) error {
				return db.Model(&model.Department{}).
					Where("id = ?", departmentID).
					Update("status", model.DepartmentStatusArchived).Error
			},
			wantError: "invited department is unavailable",
		},
		{
			name: "workspace",
			deactivate: func(db *gorm.DB, _, _, workspaceID string) error {
				return db.Model(&model.Workspace{}).
					Where("id = ?", workspaceID).
					Update("status", model.WorkspaceStatusArchived).Error
			},
			wantError: "invited workspace is unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newOrganizationInviteAcceptanceDB(t)
			organizationID := uuid.NewString()
			departmentID := uuid.NewString()
			workspaceID := uuid.NewString()
			accountID := uuid.NewString()
			require.NoError(t, db.Create(&model.Organization{
				ID: organizationID, Name: "Target", Status: model.OrganizationStatusActive,
			}).Error)
			require.NoError(t, db.Create(&model.Department{
				ID: departmentID, OrganizationID: organizationID, Name: "Target Department", Status: model.DepartmentStatusActive,
			}).Error)
			require.NoError(t, db.Create(&model.Workspace{
				ID: workspaceID, Name: "Target Workspace", OrganizationID: &organizationID, Status: model.WorkspaceStatusNormal,
			}).Error)
			require.NoError(t, db.Create(&auth_model.Account{
				ID: accountID, Email: tt.name + "-inactive@example.com", Name: "Invitee", Status: auth_model.AccountStatusActive,
			}).Error)
			link := &model.OrganizationInviteLink{
				OrganizationID: organizationID, DepartmentID: &departmentID, WorkspaceID: &workspaceID,
				Token: "pending-" + tt.name, Status: "active", RequireApproval: true,
				DefaultOrganizationRole: string(model.OrganizationRoleNormal),
				DefaultWorkspaceRole:    string(model.WorkspaceRoleMember), CreatedBy: uuid.NewString(),
			}
			repository := workspace_repo.NewOrganizationRepository(db)
			require.NoError(t, repository.CreateInviteLink(t.Context(), link))
			accountService := &inviteAcceptanceAccountService{}
			service := &organizationService{
				db: db, organizationRepo: repository, accountService: accountService,
				workspaceManagementService: &inviteAcceptanceWorkspaceService{db: db},
			}

			pending, err := service.AcceptInviteByToken(t.Context(), link.Token, accountID, nil)
			require.NoError(t, err)
			require.Equal(t, model.OrganizationJoinRequestStatusPending, pending.Status)
			accountService.invalidatedAccountIDs = nil
			require.NoError(t, tt.deactivate(db, organizationID, departmentID, workspaceID))

			_, err = service.ApproveDepartmentJoinRequest(t.Context(), organizationID, pending.ID, uuid.NewString())

			require.ErrorContains(t, err, tt.wantError)
			var persisted model.OrganizationJoinRequest
			require.NoError(t, db.Where("id = ?", pending.ID).First(&persisted).Error)
			require.Equal(t, model.OrganizationJoinRequestStatusPending, persisted.Status)
			require.Zero(t, countRows(t, db, &model.OrganizationMember{}, "organization_id = ? AND account_id = ?", organizationID, accountID))
			require.Zero(t, countRows(t, db, &model.DepartmentMember{}, "department_id = ? AND account_id = ?", departmentID, accountID))
			require.Zero(t, countRows(t, db, &model.WorkspaceMember{}, "workspace_id = ? AND account_id = ?", workspaceID, accountID))
			require.Empty(t, accountService.invalidatedAccountIDs)
		})
	}
}

func TestExistingOrganizationMemberWorkspaceInviteStillRequiresApproval(t *testing.T) {
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
		Token: "existing-member-workspace-invite", Status: "active", RequireApproval: true,
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
	require.Zero(t, countRows(t, db, &model.DepartmentMember{}, "department_id = ? AND account_id = ?", departmentID, accountID))
	require.Zero(t, countRows(t, db, &model.WorkspaceMember{}, "workspace_id = ? AND account_id = ?", workspaceID, accountID))

	retry, err := service.AcceptInviteByToken(t.Context(), link.Token, accountID, nil)
	require.NoError(t, err)
	require.Equal(t, pending.ID, retry.ID)

	_, err = service.ApproveDepartmentJoinRequest(t.Context(), organizationID, pending.ID, uuid.NewString())
	require.NoError(t, err)
	require.Equal(t, int64(1), countRows(t, db, &model.DepartmentMember{}, "department_id = ? AND account_id = ?", departmentID, accountID))
	require.Equal(t, int64(1), countRows(t, db, &model.WorkspaceMember{}, "workspace_id = ? AND account_id = ?", workspaceID, accountID))
	var accountContext auth_model.AccountContext
	require.NoError(t, db.Where("account_id = ?", accountID).First(&accountContext).Error)
	require.Equal(t, organizationID, *accountContext.CurrentOrganizationID)
	require.Equal(t, workspaceID, *accountContext.CurrentWorkspaceID)
}

func TestRejectedInviteRequestCanBeSubmittedAgain(t *testing.T) {
	db := newOrganizationInviteAcceptanceDB(t)
	organizationID := uuid.NewString()
	accountID := uuid.NewString()
	require.NoError(t, db.Create(&model.Organization{
		ID: organizationID, Name: "Target", Status: model.OrganizationStatusActive,
	}).Error)
	require.NoError(t, db.Create(&auth_model.Account{
		ID: accountID, Email: "retry-rejected@example.com", Name: "Retry", Status: auth_model.AccountStatusActive,
	}).Error)
	link := &model.OrganizationInviteLink{
		OrganizationID:          organizationID,
		Token:                   "retry-rejected-invite",
		Status:                  "active",
		RequireApproval:         true,
		DefaultOrganizationRole: string(model.OrganizationRoleNormal),
		DefaultWorkspaceRole:    string(model.WorkspaceRoleMember),
		CreatedBy:               uuid.NewString(),
	}
	repository := workspace_repo.NewOrganizationRepository(db)
	require.NoError(t, repository.CreateInviteLink(t.Context(), link))
	rejected := &model.OrganizationJoinRequest{
		OrganizationID:          organizationID,
		InviteLinkID:            &link.ID,
		AccountID:               accountID,
		DefaultOrganizationRole: string(model.OrganizationRoleNormal),
		DefaultWorkspaceRole:    string(model.WorkspaceRoleMember),
		Status:                  model.OrganizationJoinRequestStatusRejected,
	}
	require.NoError(t, db.Create(rejected).Error)
	service := &organizationService{db: db, organizationRepo: repository}

	retried, err := service.AcceptInviteByToken(t.Context(), link.Token, accountID, nil)

	require.NoError(t, err)
	require.Equal(t, model.OrganizationJoinRequestStatusPending, retried.Status)
	require.NotEqual(t, rejected.ID, retried.ID)
	require.Equal(t, int64(2), countRows(t, db, &model.OrganizationJoinRequest{}, "group_id = ? AND invite_link_id = ? AND account_id = ?", organizationID, link.ID, accountID))
}

func TestApprovedInviteDoesNotHideConflictingDepartmentMembership(t *testing.T) {
	db := newOrganizationInviteAcceptanceDB(t)
	organizationID := uuid.NewString()
	existingDepartmentID := uuid.NewString()
	invitedDepartmentID := uuid.NewString()
	accountID := uuid.NewString()
	require.NoError(t, db.Create(&model.Organization{
		ID: organizationID, Name: "Target", Status: model.OrganizationStatusActive,
	}).Error)
	for _, department := range []model.Department{
		{ID: existingDepartmentID, OrganizationID: organizationID, Name: "Existing", Status: model.DepartmentStatusActive},
		{ID: invitedDepartmentID, OrganizationID: organizationID, Name: "Invited", Status: model.DepartmentStatusActive},
	} {
		require.NoError(t, db.Create(&department).Error)
	}
	require.NoError(t, db.Create(&auth_model.Account{
		ID: accountID, Email: "department-conflict@example.com", Name: "Conflict", Status: auth_model.AccountStatusActive,
	}).Error)
	require.NoError(t, db.Create(&model.OrganizationMember{
		OrganizationID: organizationID, AccountID: accountID, Role: model.OrganizationRoleNormal,
	}).Error)
	require.NoError(t, db.Create(&model.DepartmentMember{
		DepartmentID: existingDepartmentID, AccountID: accountID,
	}).Error)
	link := &model.OrganizationInviteLink{
		OrganizationID:          organizationID,
		DepartmentID:            &invitedDepartmentID,
		Token:                   "department-conflict-invite",
		Status:                  "active",
		RequireApproval:         false,
		DefaultOrganizationRole: string(model.OrganizationRoleNormal),
		DefaultWorkspaceRole:    string(model.WorkspaceRoleMember),
		CreatedBy:               uuid.NewString(),
	}
	repository := workspace_repo.NewOrganizationRepository(db)
	require.NoError(t, repository.CreateInviteLink(t.Context(), link))
	service := &organizationService{db: db, organizationRepo: repository}

	_, err := service.AcceptInviteByToken(t.Context(), link.Token, accountID, nil)

	require.ErrorIs(t, err, ErrMemberAlreadyInDept)
	require.Zero(t, countRows(t, db, &model.OrganizationJoinRequest{}, "group_id = ? AND invite_link_id = ? AND account_id = ?", organizationID, link.ID, accountID))
	require.Zero(t, countRows(t, db, &model.DepartmentMember{}, "department_id = ? AND account_id = ?", invitedDepartmentID, accountID))
}

func newOrganizationInviteAcceptanceDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&auth_model.Account{},
		&auth_model.AccountContext{},
		&model.Organization{},
		&model.OrganizationMember{},
		&model.Workspace{},
		&model.WorkspaceMember{},
	))
	require.NoError(t, db.Exec(`
		CREATE TABLE departments (
			id TEXT PRIMARY KEY,
			group_id TEXT NOT NULL,
			parent_id TEXT,
			name TEXT NOT NULL,
			sort_order INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'active',
			created_at DATETIME,
			updated_at DATETIME,
			created_by TEXT
		)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE department_members (
			id TEXT PRIMARY KEY,
			department_id TEXT NOT NULL,
			account_id TEXT NOT NULL,
			created_at DATETIME
		)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE organization_invite_links (
			id TEXT PRIMARY KEY,
			group_id TEXT NOT NULL,
			department_id TEXT,
			tenant_id TEXT,
			token TEXT NOT NULL UNIQUE,
			status TEXT NOT NULL,
			require_approval INTEGER NOT NULL DEFAULT 1,
			default_group_role TEXT NOT NULL DEFAULT 'normal',
			default_tenant_role TEXT NOT NULL DEFAULT 'normal',
			expires_at DATETIME,
			created_by TEXT NOT NULL,
			created_at DATETIME,
			updated_at DATETIME
		)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE organization_join_requests (
			id TEXT PRIMARY KEY,
			group_id TEXT NOT NULL,
			invite_link_id TEXT,
			account_id TEXT NOT NULL,
			department_id TEXT,
			tenant_id TEXT,
			default_group_role TEXT NOT NULL,
			default_tenant_role TEXT NOT NULL,
			name TEXT,
			status TEXT NOT NULL,
			reason TEXT,
			reviewer_id TEXT,
			created_at DATETIME,
			reviewed_at DATETIME
		)
	`).Error)
	return db
}

type inviteAcceptanceWorkspaceService struct {
	interfaces.WorkspaceManagementService
	db *gorm.DB
}

type inviteAcceptanceAccountService struct {
	interfaces.AccountService
	invalidatedAccountIDs []string
}

func (s *inviteAcceptanceAccountService) InvalidateAccountProfileCache(accountID string) {
	s.invalidatedAccountIDs = append(s.invalidatedAccountIDs, accountID)
}

func (s *inviteAcceptanceWorkspaceService) WithTx(tx *gorm.DB) interfaces.WorkspaceManagementService {
	return &inviteAcceptanceWorkspaceService{db: tx}
}

func (s *inviteAcceptanceWorkspaceService) AddMember(ctx context.Context, req *interfaces.AddMemberRequest) error {
	join := &model.WorkspaceMember{
		WorkspaceID: req.WorkspaceID,
		AccountID:   req.AccountID,
		Role:        req.Role,
	}
	model.ApplyWorkspaceMemberDefaults(join)
	return s.db.WithContext(ctx).Create(join).Error
}
