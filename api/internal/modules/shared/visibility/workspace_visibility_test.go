package visibility

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/ginext/internal/modules/workspace/model"
)

func TestResolveVisibleWorkspaceIDs_ReturnsAllNormalWorkspacesForOrgAdmin(t *testing.T) {
	ctx := t.Context()

	orgService := &stubOrganizationService{
		isAdmin: true,
		workspaces: []*workspace_model.Workspace{
			{ID: "ws-alpha", Status: workspace_model.WorkspaceStatusNormal},
			{ID: "ws-beta", Status: workspace_model.WorkspaceStatusNormal},
			{ID: "ws-archived", Status: workspace_model.WorkspaceStatusArchived},
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
	require.Equal(t, []string{"ws-alpha", "ws-beta"}, workspaceIDs)
	require.False(t, orgService.checkWorkspaceAnyPermissionCalled)
}

func TestResolveVisibleWorkspaceIDs_ReturnsFilteredWorkspaceForOrgAdmin(t *testing.T) {
	ctx := t.Context()

	orgService := &stubOrganizationService{
		isAdmin: true,
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
	require.Equal(t, []string{"ws-beta"}, workspaceIDs)
	require.False(t, orgService.checkWorkspaceAnyPermissionCalled)
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

func TestResolveVisibleWorkspaceScope_AdminAllowsOrganizationScopedResources(t *testing.T) {
	ctx := t.Context()

	orgService := &stubOrganizationService{
		isAdmin: true,
		workspaces: []*workspace_model.Workspace{
			{ID: "ws-alpha", Status: workspace_model.WorkspaceStatusNormal},
			{ID: "ws-beta", Status: workspace_model.WorkspaceStatusNormal},
			{ID: "ws-archived", Status: workspace_model.WorkspaceStatusArchived},
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
		WorkspaceIDs:            []string{"ws-alpha", "ws-beta"},
		AllowOrganizationScoped: true,
	}, scope)
}

type stubOrganizationService struct {
	interfaces.OrganizationService

	isAdmin                         bool
	workspaces                      []*workspace_model.Workspace
	permissionsByWorkspaceID        map[string]bool
	checkWorkspaceAnyPermissionCalled bool
}

func (s *stubOrganizationService) GetOrganizationWorkspacesList(_ context.Context, _ string) ([]*workspace_model.Workspace, error) {
	return append([]*workspace_model.Workspace(nil), s.workspaces...), nil
}

func (s *stubOrganizationService) IsOrganizationAdminOrOwner(_ context.Context, _, _ string) (bool, error) {
	return s.isAdmin, nil
}

func (s *stubOrganizationService) CheckWorkspaceOrganizationAnyPermission(_ context.Context, _, workspaceID, _ string, _ ...workspace_model.WorkspacePermissionCode) (bool, error) {
	s.checkWorkspaceAnyPermissionCalled = true
	return s.permissionsByWorkspaceID[workspaceID], nil
}
