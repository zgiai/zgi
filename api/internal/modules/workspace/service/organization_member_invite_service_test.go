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

func TestDirectAddOrganizationMemberRollsBackWhenWorkspaceAddFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&auth_model.Account{},
		&auth_model.AccountContext{},
		&model.OrganizationMember{},
		&model.Workspace{},
		&model.WorkspaceMember{},
		&model.Department{},
		&model.DepartmentMember{},
	))

	now := time.Now()
	organizationID := uuid.New().String()
	ownerID := uuid.New().String()
	workspaceID := uuid.New().String()
	departmentID := uuid.New().String()

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

func countRows(t *testing.T, db *gorm.DB, modelValue interface{}, query interface{}, args ...interface{}) int64 {
	t.Helper()

	var count int64
	require.NoError(t, db.Model(modelValue).Where(query, args...).Count(&count).Error)
	return count
}
