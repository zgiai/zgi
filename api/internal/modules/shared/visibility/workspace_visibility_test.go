package visibility

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

func TestResolveVisibleWorkspaceIDs_FiltersOrgAdminByWorkspacePermission(t *testing.T) {
	ctx := t.Context()

	orgService := &stubOrganizationService{
		workspaces: []*workspace_model.Workspace{
			{ID: "ws-alpha", Status: workspace_model.WorkspaceStatusNormal},
			{ID: "ws-beta", Status: workspace_model.WorkspaceStatusNormal},
			{ID: "ws-archived", Status: workspace_model.WorkspaceStatusArchived},
		},
		permissionsByWorkspaceID: map[string]bool{
			"ws-alpha": true,
		},
	}

	workspaceIDs, err := ResolveVisibleWorkspaceIDs(
		ctx,
		orgService,
		"org-1",
		"account-1",
		"",
		workspace_model.WorkspacePermissionFileView,
	)
	require.NoError(t, err)
	require.Equal(t, []string{"ws-alpha"}, workspaceIDs)
	require.True(t, orgService.checkWorkspaceAnyPermissionCalled)
}

func TestResolveVisibleWorkspaceIDs_RequiresPermissionForFilteredWorkspace(t *testing.T) {
	ctx := t.Context()

	orgService := &stubOrganizationService{
		workspaces: []*workspace_model.Workspace{
			{ID: "ws-alpha", Status: workspace_model.WorkspaceStatusNormal},
			{ID: "ws-beta", Status: workspace_model.WorkspaceStatusNormal},
		},
	}

	workspaceIDs, err := ResolveVisibleWorkspaceIDs(
		ctx,
		orgService,
		"org-1",
		"account-1",
		"ws-beta",
		workspace_model.WorkspacePermissionFileView,
	)
	require.NoError(t, err)
	require.Empty(t, workspaceIDs)
	require.True(t, orgService.checkWorkspaceAnyPermissionCalled)
}

func TestResolveVisibleWorkspaceIDs_FiltersByAnyPermissionForNormalMember(t *testing.T) {
	ctx := t.Context()

	orgService := &stubOrganizationService{
		workspaces: []*workspace_model.Workspace{
			{ID: "ws-alpha", Status: workspace_model.WorkspaceStatusNormal},
			{ID: "ws-beta", Status: workspace_model.WorkspaceStatusNormal},
			{ID: "ws-gamma", Status: workspace_model.WorkspaceStatusNormal},
		},
		permissionsByWorkspaceID: map[string]bool{
			"ws-alpha": true,
			"ws-gamma": true,
		},
	}

	workspaceIDs, err := ResolveVisibleWorkspaceIDs(
		ctx,
		orgService,
		"org-1",
		"account-1",
		"",
		workspace_model.WorkspacePermissionAgentView,
		workspace_model.WorkspacePermissionAgentManage,
	)
	require.NoError(t, err)
	require.Equal(t, []string{"ws-alpha", "ws-gamma"}, workspaceIDs)
	require.True(t, orgService.checkWorkspaceAnyPermissionCalled)
}

func TestResolveVisibleWorkspaceIDs_ReturnsEmptyWhenFilteredWorkspaceOutsideOrganization(t *testing.T) {
	ctx := t.Context()

	orgService := &stubOrganizationService{
		workspaces: []*workspace_model.Workspace{
			{ID: "ws-alpha", Status: workspace_model.WorkspaceStatusNormal},
			{ID: "ws-beta", Status: workspace_model.WorkspaceStatusNormal},
		},
	}

	workspaceIDs, err := ResolveVisibleWorkspaceIDs(
		ctx,
		orgService,
		"org-1",
		"account-1",
		"ws-outside",
		workspace_model.WorkspacePermissionDatabaseView,
	)
	require.NoError(t, err)
	require.Empty(t, workspaceIDs)
	require.False(t, orgService.checkWorkspaceAnyPermissionCalled)
}

func TestResolveVisibleWorkspaceScope_UsesWorkspacePermissionOnly(t *testing.T) {
	ctx := t.Context()

	orgService := &stubOrganizationService{
		workspaces: []*workspace_model.Workspace{
			{ID: "ws-alpha", Status: workspace_model.WorkspaceStatusNormal},
			{ID: "ws-beta", Status: workspace_model.WorkspaceStatusNormal},
			{ID: "ws-archived", Status: workspace_model.WorkspaceStatusArchived},
		},
		permissionsByWorkspaceID: map[string]bool{
			"ws-beta": true,
		},
	}

	scope, err := ResolveVisibleWorkspaceScope(
		ctx,
		orgService,
		"org-1",
		"account-1",
		"",
		workspace_model.WorkspacePermissionFileView,
	)
	require.NoError(t, err)
	require.Equal(t, VisibleWorkspaceScope{
		WorkspaceIDs: []string{"ws-beta"},
	}, scope)
}

type stubOrganizationService struct {
	interfaces.OrganizationService

	workspaces                        []*workspace_model.Workspace
	permissionsByWorkspaceID          map[string]bool
	checkWorkspaceAnyPermissionCalled bool
}

func (s *stubOrganizationService) GetOrganizationWorkspacesList(_ context.Context, _ string) ([]*workspace_model.Workspace, error) {
	return append([]*workspace_model.Workspace(nil), s.workspaces...), nil
}

func (s *stubOrganizationService) CheckWorkspaceOrganizationAnyPermission(_ context.Context, _, workspaceID, _ string, _ ...workspace_model.WorkspacePermissionCode) (bool, error) {
	s.checkWorkspaceAnyPermissionCalled = true
	return s.permissionsByWorkspaceID[workspaceID], nil
}
