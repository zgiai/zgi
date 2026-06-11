package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
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
