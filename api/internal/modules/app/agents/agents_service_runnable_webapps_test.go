package agents

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

func TestAgentsService_GetRunnableWebApps_UsesAllNormalOrganizationWorkspaces(t *testing.T) {
	ctx := t.Context()

	repo := &stubAgentsRepository{
		items: []runnableWebAppItem{
			{
				AgentID:     "agent-1",
				WorkspaceID: "ws-alpha",
				WebAppID:    "webapp-1",
				AgentName:   "Alpha",
				AgentDesc:   "desc-alpha",
				AgentType:   "CONVERSATIONAL_WORKFLOW",
			},
			{
				AgentID:     "agent-2",
				WorkspaceID: "ws-beta",
				WebAppID:    "webapp-2",
				AgentName:   "Beta",
				AgentDesc:   "desc-beta",
				AgentType:   "CONVERSATIONAL_WORKFLOW",
			},
		},
	}
	tenantService := &stubWorkspaceManagementService{
		currentOrganization: &workspace_model.OrganizationMember{
			OrganizationID: "org-1",
			AccountID:      "account-1",
		},
	}
	orgService := &stubOrganizationService{
		permissionWorkspaceIDs: []string{"ws-alpha"},
		workspaces: []*workspace_model.Workspace{
			{ID: "ws-alpha", Status: workspace_model.WorkspaceStatusNormal},
			{ID: "ws-beta", Status: workspace_model.WorkspaceStatusNormal},
			{ID: "ws-archived", Status: workspace_model.WorkspaceStatusArchived},
		},
	}

	service := &agentsService{
		agentsRepo:        repo,
		tenantService:     tenantService,
		enterpriseService: orgService,
	}

	resp, err := service.GetRunnableWebApps(ctx, "account-1", dto.GetRunnableWebAppsRequest{})
	require.NoError(t, err)
	require.False(t, orgService.listWorkspaceIDsByPermissionCalled)
	require.True(t, orgService.getOrganizationWorkspacesListCalled)
	require.Equal(t, []string{"ws-alpha", "ws-beta"}, repo.lastWorkspaceIDs)
	require.Empty(t, repo.lastWorkspaceID)
	require.Equal(t, &dto.RunnableWebAppsResponse{
		Items: []dto.RunnableWebAppItem{
			{
				AgentID:      "agent-1",
				WorkspaceID:  "ws-alpha",
				WebAppID:     "webapp-1",
				WebAppStatus: "active",
				MetaData: dto.RunnableWebAppMetaData{
					Name:      "Alpha",
					Desc:      "desc-alpha",
					AgentType: "CONVERSATIONAL_WORKFLOW",
				},
			},
			{
				AgentID:      "agent-2",
				WorkspaceID:  "ws-beta",
				WebAppID:     "webapp-2",
				WebAppStatus: "active",
				MetaData: dto.RunnableWebAppMetaData{
					Name:      "Beta",
					Desc:      "desc-beta",
					AgentType: "CONVERSATIONAL_WORKFLOW",
				},
			},
		},
	}, resp)
}

func TestAgentsService_GetRunnableWebApps_AllowsWorkspaceInOrganizationWithoutAgentView(t *testing.T) {
	ctx := t.Context()

	repo := &stubAgentsRepository{
		items: []runnableWebAppItem{
			{
				AgentID:     "agent-2",
				WorkspaceID: "ws-beta",
				WebAppID:    "webapp-2",
				AgentName:   "Beta",
				AgentDesc:   "desc-beta",
				AgentType:   "CONVERSATIONAL_WORKFLOW",
			},
		},
	}
	tenantService := &stubWorkspaceManagementService{
		currentOrganization: &workspace_model.OrganizationMember{
			OrganizationID: "org-1",
			AccountID:      "account-1",
		},
	}
	orgService := &stubOrganizationService{
		permissionWorkspaceIDs: []string{"ws-alpha"},
		workspaces: []*workspace_model.Workspace{
			{ID: "ws-alpha", Status: workspace_model.WorkspaceStatusNormal},
			{ID: "ws-beta", Status: workspace_model.WorkspaceStatusNormal},
		},
	}

	service := &agentsService{
		agentsRepo:        repo,
		tenantService:     tenantService,
		enterpriseService: orgService,
	}

	resp, err := service.GetRunnableWebApps(ctx, "account-1", dto.GetRunnableWebAppsRequest{
		WorkspaceID: "ws-beta",
	})
	require.NoError(t, err)
	require.False(t, orgService.listWorkspaceIDsByPermissionCalled)
	require.True(t, orgService.getOrganizationWorkspacesListCalled)
	require.Equal(t, []string{"ws-alpha", "ws-beta"}, repo.lastWorkspaceIDs)
	require.Equal(t, "ws-beta", repo.lastWorkspaceID)
	require.Equal(t, &dto.RunnableWebAppsResponse{
		Items: []dto.RunnableWebAppItem{
			{
				AgentID:      "agent-2",
				WorkspaceID:  "ws-beta",
				WebAppID:     "webapp-2",
				WebAppStatus: "active",
				MetaData: dto.RunnableWebAppMetaData{
					Name:      "Beta",
					Desc:      "desc-beta",
					AgentType: "CONVERSATIONAL_WORKFLOW",
				},
			},
		},
	}, resp)
}

func TestAgentsService_GetRunnableWebApps_ReturnsEmptyWhenWorkspaceOutsideOrganization(t *testing.T) {
	ctx := t.Context()

	repo := &stubAgentsRepository{}
	tenantService := &stubWorkspaceManagementService{
		currentOrganization: &workspace_model.OrganizationMember{
			OrganizationID: "org-1",
			AccountID:      "account-1",
		},
	}
	orgService := &stubOrganizationService{
		workspaces: []*workspace_model.Workspace{
			{ID: "ws-alpha", Status: workspace_model.WorkspaceStatusNormal},
			{ID: "ws-beta", Status: workspace_model.WorkspaceStatusNormal},
		},
	}

	service := &agentsService{
		agentsRepo:        repo,
		tenantService:     tenantService,
		enterpriseService: orgService,
	}

	resp, err := service.GetRunnableWebApps(ctx, "account-1", dto.GetRunnableWebAppsRequest{
		WorkspaceID: "ws-outside",
	})
	require.NoError(t, err)
	require.Equal(t, &dto.RunnableWebAppsResponse{Items: []dto.RunnableWebAppItem{}}, resp)
	require.False(t, repo.listRunnableWebAppsCalled)
}

func TestAgentsService_GetRunnableWebApps_ReturnsErrorWhenCurrentOrganizationMissing(t *testing.T) {
	ctx := t.Context()

	service := &agentsService{
		agentsRepo: &stubAgentsRepository{},
		tenantService: &stubWorkspaceManagementService{
			currentOrganization: nil,
		},
		enterpriseService: &stubOrganizationService{},
	}

	resp, err := service.GetRunnableWebApps(ctx, "account-1", dto.GetRunnableWebAppsRequest{})
	require.ErrorIs(t, err, errCurrentOrganizationNotFound)
	require.Nil(t, resp)
}

type stubAgentsRepository struct {
	AgentsRepository

	items                     []runnableWebAppItem
	err                       error
	lastWorkspaceIDs          []string
	lastWorkspaceID           string
	listRunnableWebAppsCalled bool
}

func (s *stubAgentsRepository) ListRunnableWebApps(_ context.Context, workspaceIDs []string, workspaceID string) ([]runnableWebAppItem, error) {
	s.listRunnableWebAppsCalled = true
	s.lastWorkspaceIDs = append([]string(nil), workspaceIDs...)
	s.lastWorkspaceID = workspaceID
	if s.err != nil {
		return nil, s.err
	}
	return s.items, nil
}

type stubWorkspaceManagementService struct {
	interfaces.WorkspaceManagementService

	currentOrganization *workspace_model.OrganizationMember
	err                 error
}

func (s *stubWorkspaceManagementService) GetCurrentOrganization(_ context.Context, _ string) (*workspace_model.OrganizationMember, error) {
	return s.currentOrganization, s.err
}

type stubOrganizationService struct {
	interfaces.OrganizationService

	permissionWorkspaceIDs              []string
	workspaces                          []*workspace_model.Workspace
	listWorkspaceIDsByPermissionCalled  bool
	getOrganizationWorkspacesListCalled bool
	listWorkspaceIDsByPermissionErr     error
	getOrganizationWorkspacesListErr    error
}

func (s *stubOrganizationService) ListWorkspaceIDsByPermission(_ context.Context, _, _ string, _ workspace_model.WorkspacePermissionCode) ([]string, error) {
	s.listWorkspaceIDsByPermissionCalled = true
	if s.listWorkspaceIDsByPermissionErr != nil {
		return nil, s.listWorkspaceIDsByPermissionErr
	}
	return append([]string(nil), s.permissionWorkspaceIDs...), nil
}

func (s *stubOrganizationService) GetOrganizationWorkspacesList(_ context.Context, _ string) ([]*workspace_model.Workspace, error) {
	s.getOrganizationWorkspacesListCalled = true
	if s.getOrganizationWorkspacesListErr != nil {
		return nil, s.getOrganizationWorkspacesListErr
	}
	return append([]*workspace_model.Workspace(nil), s.workspaces...), nil
}
