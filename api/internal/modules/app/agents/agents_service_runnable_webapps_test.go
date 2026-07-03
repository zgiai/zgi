package agents

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

func TestAgentsService_GetRunnableWebApps_OrganizationAdminUsesAllNormalOrganizationWorkspaces(t *testing.T) {
	ctx := t.Context()
	orgID := "org-1"
	wsAlpha := "ws-alpha"
	wsBeta := "ws-beta"
	wsArchived := "ws-archived"

	repo := &stubAgentsRepository{
		items: []runnableWebAppItem{
			{
				AgentID:     "agent-1",
				WorkspaceID: wsAlpha,
				WebAppID:    "webapp-1",
				AgentName:   "Alpha",
				AgentDesc:   "desc-alpha",
				AgentType:   "CONVERSATIONAL_WORKFLOW",
			},
			{
				AgentID:     "agent-2",
				WorkspaceID: wsBeta,
				WebAppID:    "webapp-2",
				AgentName:   "Beta",
				AgentDesc:   "desc-beta",
				AgentType:   "CONVERSATIONAL_WORKFLOW",
			},
		},
	}
	tenantService := &stubWorkspaceManagementService{
		currentOrganization: &workspace_model.OrganizationMember{
			OrganizationID: orgID,
			AccountID:      "account-1",
			Role:           workspace_model.OrganizationRoleAdmin,
		},
	}
	orgService := &stubOrganizationService{
		permissionWorkspaceIDs: []string{"ws-alpha"},
		workspaces: []*workspace_model.Workspace{
			{ID: wsAlpha, Status: workspace_model.WorkspaceStatusNormal, OrganizationID: &orgID},
			{ID: wsBeta, Status: workspace_model.WorkspaceStatusNormal, OrganizationID: &orgID},
			{ID: wsArchived, Status: workspace_model.WorkspaceStatusArchived, OrganizationID: &orgID},
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
	require.Equal(t, []string{wsAlpha, wsBeta}, repo.lastWorkspaceIDs)
	require.Empty(t, repo.lastWorkspaceID)
	require.Equal(t, &dto.RunnableWebAppsResponse{
		Items: []dto.RunnableWebAppItem{
			{
				AgentID:      "agent-1",
				WorkspaceID:  wsAlpha,
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
				WorkspaceID:  wsBeta,
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

func TestAgentsService_GetRunnableWebApps_RegularMemberUsesJoinedNormalWorkspaces(t *testing.T) {
	ctx := t.Context()
	orgID := "org-1"
	wsAlpha := "ws-alpha"
	wsBeta := "ws-beta"
	wsOutside := "ws-outside"

	repo := &stubAgentsRepository{
		items: []runnableWebAppItem{
			{
				AgentID:     "agent-1",
				WorkspaceID: wsAlpha,
				WebAppID:    "webapp-1",
				AgentName:   "Alpha",
				AgentDesc:   "desc-alpha",
				AgentType:   "CONVERSATIONAL_WORKFLOW",
			},
		},
	}
	tenantService := &stubWorkspaceManagementService{
		currentOrganization: &workspace_model.OrganizationMember{
			OrganizationID: orgID,
			AccountID:      "account-1",
			Role:           workspace_model.OrganizationRoleNormal,
		},
		workspaceJoins: []*workspace_model.WorkspaceMember{
			{WorkspaceID: wsAlpha, AccountID: "account-1"},
			{WorkspaceID: wsBeta, AccountID: "account-1"},
			{WorkspaceID: wsOutside, AccountID: "account-1"},
		},
		workspacesByID: map[string]*workspace_model.Workspace{
			wsAlpha:   {ID: wsAlpha, Status: workspace_model.WorkspaceStatusNormal, OrganizationID: &orgID},
			wsBeta:    {ID: wsBeta, Status: workspace_model.WorkspaceStatusArchived, OrganizationID: &orgID},
			wsOutside: {ID: wsOutside, Status: workspace_model.WorkspaceStatusNormal, OrganizationID: testStringPtr("org-2")},
		},
	}
	orgService := &stubOrganizationService{}

	service := &agentsService{
		agentsRepo:        repo,
		tenantService:     tenantService,
		enterpriseService: orgService,
	}

	resp, err := service.GetRunnableWebApps(ctx, "account-1", dto.GetRunnableWebAppsRequest{})
	require.NoError(t, err)
	require.False(t, orgService.getOrganizationWorkspacesListCalled)
	require.Equal(t, []string{wsAlpha}, repo.lastWorkspaceIDs)
	require.Empty(t, repo.lastWorkspaceID)
	require.Equal(t, &dto.RunnableWebAppsResponse{
		Items: []dto.RunnableWebAppItem{
			{
				AgentID:      "agent-1",
				WorkspaceID:  wsAlpha,
				WebAppID:     "webapp-1",
				WebAppStatus: "active",
				MetaData: dto.RunnableWebAppMetaData{
					Name:      "Alpha",
					Desc:      "desc-alpha",
					AgentType: "CONVERSATIONAL_WORKFLOW",
				},
			},
		},
	}, resp)
}

func TestAgentsService_GetRunnableWebApps_AllowsJoinedWorkspaceWithoutAgentView(t *testing.T) {
	ctx := t.Context()
	orgID := "org-1"
	wsBeta := "ws-beta"

	repo := &stubAgentsRepository{
		items: []runnableWebAppItem{
			{
				AgentID:     "agent-2",
				WorkspaceID: wsBeta,
				WebAppID:    "webapp-2",
				AgentName:   "Beta",
				AgentDesc:   "desc-beta",
				AgentType:   "CONVERSATIONAL_WORKFLOW",
			},
		},
	}
	tenantService := &stubWorkspaceManagementService{
		currentOrganization: &workspace_model.OrganizationMember{
			OrganizationID: orgID,
			AccountID:      "account-1",
			Role:           workspace_model.OrganizationRoleNormal,
		},
		workspaceJoins: []*workspace_model.WorkspaceMember{
			{WorkspaceID: wsBeta, AccountID: "account-1"},
		},
		workspacesByID: map[string]*workspace_model.Workspace{
			wsBeta: {ID: wsBeta, Status: workspace_model.WorkspaceStatusNormal, OrganizationID: &orgID},
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
		WorkspaceID: wsBeta,
	})
	require.NoError(t, err)
	require.False(t, orgService.listWorkspaceIDsByPermissionCalled)
	require.False(t, orgService.getOrganizationWorkspacesListCalled)
	require.Equal(t, []string{wsBeta}, repo.lastWorkspaceIDs)
	require.Equal(t, wsBeta, repo.lastWorkspaceID)
	require.Equal(t, &dto.RunnableWebAppsResponse{
		Items: []dto.RunnableWebAppItem{
			{
				AgentID:      "agent-2",
				WorkspaceID:  wsBeta,
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

func TestAgentsService_GetRunnableWebApps_RegularMemberCannotRequestUnjoinedWorkspace(t *testing.T) {
	ctx := t.Context()
	orgID := "org-1"
	wsAlpha := "ws-alpha"
	wsBeta := "ws-beta"

	repo := &stubAgentsRepository{}
	tenantService := &stubWorkspaceManagementService{
		currentOrganization: &workspace_model.OrganizationMember{
			OrganizationID: orgID,
			AccountID:      "account-1",
			Role:           workspace_model.OrganizationRoleNormal,
		},
		workspaceJoins: []*workspace_model.WorkspaceMember{
			{WorkspaceID: wsAlpha, AccountID: "account-1"},
		},
		workspacesByID: map[string]*workspace_model.Workspace{
			wsAlpha: {ID: wsAlpha, Status: workspace_model.WorkspaceStatusNormal, OrganizationID: &orgID},
		},
	}

	service := &agentsService{
		agentsRepo:        repo,
		tenantService:     tenantService,
		enterpriseService: &stubOrganizationService{},
	}

	resp, err := service.GetRunnableWebApps(ctx, "account-1", dto.GetRunnableWebAppsRequest{
		WorkspaceID: wsBeta,
	})
	require.NoError(t, err)
	require.Equal(t, &dto.RunnableWebAppsResponse{Items: []dto.RunnableWebAppItem{}}, resp)
	require.False(t, repo.listRunnableWebAppsCalled)
}

func TestAgentsService_GetRunnableWebApps_ReturnsEmptyWhenWorkspaceOutsideOrganization(t *testing.T) {
	ctx := t.Context()

	repo := &stubAgentsRepository{}
	tenantService := &stubWorkspaceManagementService{
		currentOrganization: &workspace_model.OrganizationMember{
			OrganizationID: "org-1",
			AccountID:      "account-1",
			Role:           workspace_model.OrganizationRoleAdmin,
		},
	}
	orgID := "org-1"
	orgService := &stubOrganizationService{
		workspaces: []*workspace_model.Workspace{
			{ID: "ws-alpha", Status: workspace_model.WorkspaceStatusNormal, OrganizationID: &orgID},
			{ID: "ws-beta", Status: workspace_model.WorkspaceStatusNormal, OrganizationID: &orgID},
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
	workspaceJoins      []*workspace_model.WorkspaceMember
	workspacesByID      map[string]*workspace_model.Workspace
	err                 error
}

func (s *stubWorkspaceManagementService) GetCurrentOrganization(_ context.Context, _ string) (*workspace_model.OrganizationMember, error) {
	return s.currentOrganization, s.err
}

func (s *stubWorkspaceManagementService) GetAccountWorkspaceJoins(_ context.Context, _ string) ([]*workspace_model.WorkspaceMember, error) {
	return append([]*workspace_model.WorkspaceMember(nil), s.workspaceJoins...), nil
}

func (s *stubWorkspaceManagementService) GetWorkspacesByIDs(_ context.Context, workspaceIDs []string) ([]*workspace_model.Workspace, error) {
	workspaces := make([]*workspace_model.Workspace, 0, len(workspaceIDs))
	for _, workspaceID := range workspaceIDs {
		if workspace := s.workspacesByID[workspaceID]; workspace != nil {
			workspaces = append(workspaces, workspace)
		}
	}
	return workspaces, nil
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

func testStringPtr(value string) *string {
	return &value
}
