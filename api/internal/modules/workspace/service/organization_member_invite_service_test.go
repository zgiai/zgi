package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	shared_dto "github.com/zgiai/zgi/api/internal/dto"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	workspace_repo "github.com/zgiai/zgi/api/internal/modules/workspace/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestInviteMemberDefaultsCreateUsableWorkspaceContext(t *testing.T) {
	t.Parallel()

	workspaceID := uuid.New().String()
	accountID := uuid.New().String()
	organizationID := uuid.New().String()

	workspaceMember := newInviteWorkspaceMemberJoin(workspaceID, accountID, true)
	require.NotEmpty(t, workspaceMember.ID)
	require.NoError(t, uuid.Validate(workspaceMember.ID))
	require.Equal(t, workspaceID, workspaceMember.WorkspaceID)
	require.Equal(t, accountID, workspaceMember.AccountID)
	require.Equal(t, model.WorkspaceRoleNormal, workspaceMember.Role)
	require.NotNil(t, workspaceMember.RoleID)
	require.Equal(t, model.WorkspaceBuiltinRoleMemberID, *workspaceMember.RoleID)
	require.True(t, workspaceMember.Current)

	accountContext := newInviteAccountContext(accountID, organizationID, workspaceID)
	require.Equal(t, accountID, accountContext.AccountID)
	require.NotNil(t, accountContext.CurrentOrganizationID)
	require.Equal(t, organizationID, *accountContext.CurrentOrganizationID)
	require.NotNil(t, accountContext.CurrentWorkspaceID)
	require.Equal(t, workspaceID, *accountContext.CurrentWorkspaceID)
}

func TestWorkspaceMemberDefaultsNormalizeRoleID(t *testing.T) {
	t.Parallel()

	emptyRoleID := " "
	join := &model.WorkspaceMember{
		WorkspaceID: uuid.New().String(),
		AccountID:   uuid.New().String(),
		Role:        model.WorkspaceRoleAdmin,
		RoleID:      &emptyRoleID,
	}

	model.ApplyWorkspaceMemberDefaults(join)

	require.NotEmpty(t, join.ID)
	require.NoError(t, uuid.Validate(join.ID))
	require.NotNil(t, join.RoleID)
	require.Equal(t, model.WorkspaceBuiltinRoleAdminID, *join.RoleID)

	customRoleID := uuid.New().String()
	customJoin := &model.WorkspaceMember{
		WorkspaceID: uuid.New().String(),
		AccountID:   uuid.New().String(),
		Role:        model.WorkspaceRoleNormal,
		RoleID:      &customRoleID,
	}

	model.ApplyWorkspaceMemberDefaults(customJoin)

	require.Equal(t, customRoleID, *customJoin.RoleID)
}

func TestValidateOrganizationPasswordResetRole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		operatorRole model.OrganizationRole
		targetRole   model.OrganizationRole
		wantErr      error
	}{
		{
			name:         "owner can reset admin",
			operatorRole: model.OrganizationRoleOwner,
			targetRole:   model.OrganizationRoleAdmin,
		},
		{
			name:         "owner can reset normal member",
			operatorRole: model.OrganizationRoleOwner,
			targetRole:   model.OrganizationRoleNormal,
		},
		{
			name:         "owner cannot reset owner",
			operatorRole: model.OrganizationRoleOwner,
			targetRole:   model.OrganizationRoleOwner,
			wantErr:      ErrOrganizationOwnerPasswordReset,
		},
		{
			name:         "admin can reset normal member",
			operatorRole: model.OrganizationRoleAdmin,
			targetRole:   model.OrganizationRoleNormal,
		},
		{
			name:         "admin cannot reset admin",
			operatorRole: model.OrganizationRoleAdmin,
			targetRole:   model.OrganizationRoleAdmin,
			wantErr:      ErrOrganizationAdminPasswordReset,
		},
		{
			name:         "admin cannot reset owner",
			operatorRole: model.OrganizationRoleAdmin,
			targetRole:   model.OrganizationRoleOwner,
			wantErr:      ErrOrganizationOwnerPasswordReset,
		},
		{
			name:         "normal member cannot reset password",
			operatorRole: model.OrganizationRoleNormal,
			targetRole:   model.OrganizationRoleNormal,
			wantErr:      ErrOrganizationInvitePermissionDenied,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateOrganizationPasswordResetRole(tt.operatorRole, tt.targetRole)
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestDirectAddOrganizationMemberRollsBackWhenWorkspaceAddFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&auth_model.Account{},
		&auth_model.AccountContext{},
		&model.Organization{},
		&model.OrganizationMember{},
		&model.Workspace{},
		&model.WorkspaceMember{},
	))
	require.NoError(t, createDirectAddInviteDepartmentTables(db))

	now := time.Now()
	organizationID := uuid.New().String()
	ownerID := uuid.New().String()
	workspaceID := uuid.New().String()
	departmentID := uuid.New().String()
	require.NoError(t, db.Create(&model.Organization{ID: organizationID, Name: "Organization", Status: model.OrganizationStatusActive}).Error)

	require.NoError(t, db.Create(&auth_model.Account{
		ID:            ownerID,
		Name:          "Owner",
		Email:         "owner@example.com",
		Status:        auth_model.AccountStatusActive,
		InitializedAt: &now,
		LastActiveAt:  &now,
	}).Error)
	require.NoError(t, db.Create(&model.OrganizationMember{
		OrganizationID: organizationID,
		AccountID:      ownerID,
		Role:           model.OrganizationRoleOwner,
	}).Error)
	require.NoError(t, db.Create(&model.Workspace{
		ID:             workspaceID,
		Name:           "Workspace",
		Status:         model.WorkspaceStatusNormal,
		OrganizationID: &organizationID,
	}).Error)
	require.NoError(t, db.Create(&model.Department{
		ID:             departmentID,
		OrganizationID: organizationID,
		Name:           "Department",
		Status:         model.DepartmentStatusActive,
	}).Error)

	svc := &organizationService{
		db:                         db,
		workspaceManagementService: nil,
	}

	_, err = svc.DirectAddOrganizationMember(ctx, &shared_dto.DirectAddOrganizationMemberRequest{
		OrganizationID:    organizationID,
		OperatorAccountID: ownerID,
		WorkspaceID:       workspaceID,
		Email:             "alice@example.com",
		Name:              "Alice",
		DepartmentID:      &departmentID,
	})

	require.Error(t, err)
	require.Zero(t, countRows(t, db, &auth_model.Account{}, "LOWER(email) = ?", "alice@example.com"))
	require.Zero(t, countRows(t, db, &model.OrganizationMember{}, "organization_id = ? AND account_id <> ?", organizationID, ownerID))
	require.Zero(t, countRows(t, db, &model.DepartmentMember{}, "department_id = ?", departmentID))
	require.Zero(t, countRows(t, db, &model.WorkspaceMember{}, "workspace_id = ?", workspaceID))
	require.Zero(t, countRows(t, db, &auth_model.AccountContext{}, "1 = 1"))
}

func TestDirectAddOrganizationMemberAllowsMissingWorkspace(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&auth_model.Account{},
		&auth_model.AccountContext{},
		&model.Organization{},
		&model.OrganizationMember{},
		&model.Workspace{},
		&model.WorkspaceMember{},
	))
	require.NoError(t, createDirectAddInviteDepartmentTables(db))

	now := time.Now()
	organizationID := uuid.New().String()
	ownerID := uuid.New().String()
	departmentID := uuid.New().String()
	require.NoError(t, db.Create(&model.Organization{ID: organizationID, Name: "Organization", Status: model.OrganizationStatusActive}).Error)

	require.NoError(t, db.Create(&auth_model.Account{
		ID:            ownerID,
		Name:          "Owner",
		Email:         "owner@example.com",
		Status:        auth_model.AccountStatusActive,
		InitializedAt: &now,
		LastActiveAt:  &now,
	}).Error)
	require.NoError(t, db.Create(&model.OrganizationMember{
		OrganizationID: organizationID,
		AccountID:      ownerID,
		Role:           model.OrganizationRoleOwner,
	}).Error)
	require.NoError(t, db.Create(&model.Department{
		ID:             departmentID,
		OrganizationID: organizationID,
		Name:           "Department",
		Status:         model.DepartmentStatusActive,
	}).Error)

	svc := &organizationService{db: db}

	resp, err := svc.DirectAddOrganizationMember(ctx, &shared_dto.DirectAddOrganizationMemberRequest{
		OrganizationID:    organizationID,
		OperatorAccountID: ownerID,
		Email:             "alice@example.com",
		Name:              "Alice",
		DepartmentID:      &departmentID,
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Nil(t, resp.Workspace)
	require.NotNil(t, resp.Department)
	require.Equal(t, departmentID, resp.Department.ID)
	require.Equal(t, int64(1), countRows(t, db, &auth_model.Account{}, "LOWER(email) = ?", "alice@example.com"))
	require.Equal(t, int64(1), countRows(t, db, &model.OrganizationMember{}, "organization_id = ? AND account_id = ?", organizationID, resp.AccountID))
	require.Equal(t, int64(1), countRows(t, db, &model.DepartmentMember{}, "department_id = ? AND account_id = ?", departmentID, resp.AccountID))
	require.Zero(t, countRows(t, db, &model.WorkspaceMember{}, "account_id = ?", resp.AccountID))

	var accountContext auth_model.AccountContext
	require.NoError(t, db.Where("account_id = ?", resp.AccountID).First(&accountContext).Error)
	require.NotNil(t, accountContext.CurrentOrganizationID)
	require.Equal(t, organizationID, *accountContext.CurrentOrganizationID)
	require.Nil(t, accountContext.CurrentWorkspaceID)
}

func TestInviteCurrentOrganizationMemberAllowsMissingWorkspace(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&auth_model.Account{},
		&auth_model.AccountContext{},
		&model.Organization{},
		&model.OrganizationMember{},
		&model.Workspace{},
		&model.WorkspaceMember{},
	))
	require.NoError(t, createDirectAddInviteDepartmentTables(db))

	now := time.Now()
	organizationID := uuid.New().String()
	ownerID := uuid.New().String()
	require.NoError(t, db.Create(&model.Organization{ID: organizationID, Name: "Organization", Status: model.OrganizationStatusActive}).Error)

	require.NoError(t, db.Create(&auth_model.Account{
		ID:            ownerID,
		Name:          "Owner",
		Email:         "owner@example.com",
		Status:        auth_model.AccountStatusActive,
		InitializedAt: &now,
		LastActiveAt:  &now,
	}).Error)
	require.NoError(t, db.Create(&model.OrganizationMember{
		OrganizationID: organizationID,
		AccountID:      ownerID,
		Role:           model.OrganizationRoleOwner,
	}).Error)

	svc := &organizationService{db: db}

	resp, err := svc.InviteCurrentOrganizationMember(ctx, &shared_dto.InviteCurrentOrganizationMemberRequest{
		OrganizationID:    organizationID,
		OperatorAccountID: ownerID,
		Email:             "bob@example.com",
		Name:              "Bob",
		Password:          "password-123",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Nil(t, resp.Workspace)
	require.Nil(t, resp.Department)
	require.True(t, resp.CreatedAccount)
	require.Equal(t, int64(1), countRows(t, db, &auth_model.Account{}, "LOWER(email) = ?", "bob@example.com"))
	require.Equal(t, int64(1), countRows(t, db, &model.OrganizationMember{}, "organization_id = ? AND account_id = ?", organizationID, resp.AccountID))
	require.Zero(t, countRows(t, db, &model.WorkspaceMember{}, "account_id = ?", resp.AccountID))

	var accountContext auth_model.AccountContext
	require.NoError(t, db.Where("account_id = ?", resp.AccountID).First(&accountContext).Error)
	require.NotNil(t, accountContext.CurrentOrganizationID)
	require.Equal(t, organizationID, *accountContext.CurrentOrganizationID)
	require.Nil(t, accountContext.CurrentWorkspaceID)
}

func TestUpdateMemberInfoNormalizesAndRejectsDuplicateNames(t *testing.T) {
	t.Parallel()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Organization{}, &model.OrganizationMember{}))
	organizationID := uuid.NewString()
	firstAccountID := uuid.NewString()
	secondAccountID := uuid.NewString()
	require.NoError(t, db.Create(&model.Organization{
		ID: organizationID, Name: "Organization", Status: model.OrganizationStatusActive,
	}).Error)
	existingName := "Alice"
	require.NoError(t, db.Create(&model.OrganizationMember{
		OrganizationID: organizationID, AccountID: firstAccountID, Role: model.OrganizationRoleNormal, Name: &existingName,
	}).Error)
	require.NoError(t, db.Model(&model.OrganizationMember{}).
		Where("organization_id = ? AND account_id = ?", organizationID, firstAccountID).
		UpdateColumn("name", "\t\u00a0Alice\u00a0\t").Error)
	require.NoError(t, db.Create(&model.OrganizationMember{
		OrganizationID: organizationID, AccountID: secondAccountID, Role: model.OrganizationRoleNormal,
	}).Error)

	repository := workspace_repo.NewOrganizationRepository(db)
	exists, err := repository.ExistsMemberByName(t.Context(), organizationID, "\u00a0Alice\t", secondAccountID)
	require.NoError(t, err)
	require.True(t, exists)
	svc := &organizationService{db: db, organizationRepo: repository}
	duplicateName := "  Alice  "
	err = svc.UpdateMemberInfo(t.Context(), &shared_dto.UpdateOrganizationMemberRequest{
		OrganizationID: organizationID,
		AccountID:      secondAccountID,
		Name:           &duplicateName,
	})
	require.ErrorIs(t, err, ErrMemberNameExists)

	availableName := "  Bob  "
	require.NoError(t, svc.UpdateMemberInfo(t.Context(), &shared_dto.UpdateOrganizationMemberRequest{
		OrganizationID: organizationID,
		AccountID:      secondAccountID,
		Name:           &availableName,
	}))
	var updated model.OrganizationMember
	require.NoError(t, db.Where("organization_id = ? AND account_id = ?", organizationID, secondAccountID).First(&updated).Error)
	require.NotNil(t, updated.Name)
	require.Equal(t, "Bob", *updated.Name)
}

func TestOrganizationMemberRoleUpdateDoesNotOverwriteConcurrentName(t *testing.T) {
	t.Parallel()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.OrganizationMember{}))
	organizationID := uuid.NewString()
	accountID := uuid.NewString()
	paddedName := "  Alice  "
	require.NoError(t, db.Create(&model.OrganizationMember{
		OrganizationID: organizationID,
		AccountID:      accountID,
		Role:           model.OrganizationRoleNormal,
		Name:           &paddedName,
	}).Error)
	var stale model.OrganizationMember
	require.NoError(t, db.Where("organization_id = ? AND account_id = ?", organizationID, accountID).First(&stale).Error)
	require.NotNil(t, stale.Name)
	require.Equal(t, "Alice", *stale.Name)
	require.NoError(t, db.Model(&model.OrganizationMember{}).
		Where("organization_id = ? AND account_id = ?", organizationID, accountID).
		UpdateColumn("name", "Bob").Error)

	stale.Role = model.OrganizationRoleAdmin
	repository := workspace_repo.NewOrganizationRepository(db)
	require.NoError(t, repository.UpdateAccountJoin(t.Context(), &stale))
	var updated model.OrganizationMember
	require.NoError(t, db.Where("organization_id = ? AND account_id = ?", organizationID, accountID).First(&updated).Error)
	require.Equal(t, model.OrganizationRoleAdmin, updated.Role)
	require.NotNil(t, updated.Name)
	require.Equal(t, "Bob", *updated.Name)
}

func createDirectAddInviteDepartmentTables(db *gorm.DB) error {
	if err := db.Exec(`
CREATE TABLE departments (
	id text primary key,
	group_id text not null,
	parent_id text,
	name text not null,
	sort_order integer not null default 0,
	status text not null default 'active',
	created_at datetime,
	updated_at datetime,
	created_by text
)`).Error; err != nil {
		return err
	}

	return db.Exec(`
CREATE TABLE department_members (
	id text primary key,
	department_id text not null,
	account_id text not null,
	created_at datetime
)`).Error
}

func countRows(t *testing.T, db *gorm.DB, modelValue interface{}, query interface{}, args ...interface{}) int64 {
	t.Helper()

	var count int64
	require.NoError(t, db.Model(modelValue).Where(query, args...).Count(&count).Error)
	return count
}
