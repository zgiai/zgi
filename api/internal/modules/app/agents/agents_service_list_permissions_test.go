package agents

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

func TestAgentsService_GetAgentsListWithPermissions_ReturnsEmptyWhenWorkspacePermissionMissing(t *testing.T) {
	ctx := t.Context()

	repo := &stubAgentsListRepository{}
	tenantService := &stubWorkspaceManagementServiceForAgentsList{
		currentOrganization: &workspace_model.OrganizationMember{
			OrganizationID: "org-1",
			AccountID:      "account-1",
			Role:           workspace_model.OrganizationRoleNormal,
		},
		workspaceIDsByOrganization: []string{"ws-alpha", "ws-beta"},
	}
	orgService := &stubOrganizationServiceForAgentsList{
		workspaces: []*workspace_model.Workspace{
			{ID: "ws-alpha", Status: workspace_model.WorkspaceStatusNormal},
			{ID: "ws-beta", Status: workspace_model.WorkspaceStatusNormal},
		},
		permissionsByWorkspaceID: map[string]bool{
			"ws-alpha": true,
			"ws-beta":  false,
		},
	}

	service := &agentsService{
		agentsRepo:                repo,
		tenantService:             tenantService,
		enterpriseService:         orgService,
		resourcePermissionService: &stubResourcePermissionService{},
	}

	resp, err := service.GetAgentsListWithPermissions(ctx, "account-1", dto.GetAgentsListRequest{
		Page:        1,
		Limit:       20,
		WorkspaceID: "ws-beta",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Empty(t, resp.Data)
	require.Zero(t, resp.Total)
	require.False(t, resp.HasMore)
	require.False(t, repo.getPaginatedAgentsMultipleTenantsCalled)
	require.Equal(t, agentAssetVisiblePermissionCodes(), orgService.lastPermissions)
	require.Contains(t, orgService.lastPermissions, workspace_model.WorkspacePermissionAgentView)
	require.NotContains(t, orgService.lastPermissions, workspace_model.WorkspacePermissionAgentCreate)
	require.NotContains(t, orgService.lastPermissions, workspace_model.WorkspacePermissionWorkflowCreate)
	require.NotContains(t, orgService.lastPermissions, workspace_model.WorkspacePermissionWorkflowImport)
	require.NotContains(t, orgService.lastPermissions, workspace_model.WorkspacePermissionAgentManage)
}

func TestAgentsService_GetAgentsListWithPermissions_FiltersAgentAssetKind(t *testing.T) {
	ctx := t.Context()

	repo := &stubAgentsListRepository{}
	tenantService := &stubWorkspaceManagementServiceForAgentsList{
		currentOrganization: &workspace_model.OrganizationMember{
			OrganizationID: "org-1",
			AccountID:      "account-1",
			Role:           workspace_model.OrganizationRoleNormal,
		},
		workspaceIDsByOrganization: []string{"ws-alpha"},
	}
	orgService := &stubOrganizationServiceForAgentsList{
		workspaces: []*workspace_model.Workspace{
			{ID: "ws-alpha", Status: workspace_model.WorkspaceStatusNormal},
		},
		permissionsByWorkspaceID: map[string]bool{
			"ws-alpha": true,
		},
	}

	service := &agentsService{
		agentsRepo:                repo,
		tenantService:             tenantService,
		enterpriseService:         orgService,
		resourcePermissionService: &stubResourcePermissionService{},
	}

	resp, err := service.GetAgentsListWithPermissions(ctx, "account-1", dto.GetAgentsListRequest{
		Page:      1,
		Limit:     20,
		AssetKind: "agent",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.True(t, repo.getPaginatedAgentsMultipleTenantsCalled)
	require.Equal(t, agentVisiblePermissionCodes(), orgService.lastPermissions)
	require.NotContains(t, orgService.lastPermissions, workspace_model.WorkspacePermissionAgentCreate)
	require.Equal(t, []string{"AGENT"}, repo.lastFilter.AgentTypes)
}

func TestAgentsService_GetAgentsListWithPermissions_FiltersWorkflowAssetKind(t *testing.T) {
	ctx := t.Context()

	repo := &stubAgentsListRepository{}
	tenantService := &stubWorkspaceManagementServiceForAgentsList{
		currentOrganization: &workspace_model.OrganizationMember{
			OrganizationID: "org-1",
			AccountID:      "account-1",
			Role:           workspace_model.OrganizationRoleNormal,
		},
		workspaceIDsByOrganization: []string{"ws-alpha"},
	}
	orgService := &stubOrganizationServiceForAgentsList{
		workspaces: []*workspace_model.Workspace{
			{ID: "ws-alpha", Status: workspace_model.WorkspaceStatusNormal},
		},
		permissionsByWorkspaceID: map[string]bool{
			"ws-alpha": true,
		},
	}

	service := &agentsService{
		agentsRepo:                repo,
		tenantService:             tenantService,
		enterpriseService:         orgService,
		resourcePermissionService: &stubResourcePermissionService{},
	}

	resp, err := service.GetAgentsListWithPermissions(ctx, "account-1", dto.GetAgentsListRequest{
		Page:      1,
		Limit:     20,
		AssetKind: "workflow",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.True(t, repo.getPaginatedAgentsMultipleTenantsCalled)
	require.Equal(t, workflowVisiblePermissionCodes(), orgService.lastPermissions)
	require.NotContains(t, orgService.lastPermissions, workspace_model.WorkspacePermissionWorkflowCreate)
	require.NotContains(t, orgService.lastPermissions, workspace_model.WorkspacePermissionWorkflowImport)
	require.Equal(
		t,
		[]string{"WORKFLOW", "CONVERSATIONAL_WORKFLOW", "CONVERSATIONAL_AGENT"},
		repo.lastFilter.AgentTypes,
	)
}

func TestAgentsService_GetAgentsListWithPermissions_RejectsInvalidAssetKind(t *testing.T) {
	ctx := t.Context()

	repo := &stubAgentsListRepository{}
	tenantService := &stubWorkspaceManagementServiceForAgentsList{
		currentOrganization: &workspace_model.OrganizationMember{
			OrganizationID: "org-1",
			AccountID:      "account-1",
			Role:           workspace_model.OrganizationRoleNormal,
		},
	}

	service := &agentsService{
		agentsRepo:                repo,
		tenantService:             tenantService,
		resourcePermissionService: &stubResourcePermissionService{},
	}

	resp, err := service.GetAgentsListWithPermissions(ctx, "account-1", dto.GetAgentsListRequest{
		Page:      1,
		Limit:     20,
		AssetKind: "dataset",
	})
	require.ErrorIs(t, err, errInvalidAgentListAssetKind)
	require.Nil(t, resp)
	require.False(t, repo.getPaginatedAgentsMultipleTenantsCalled)
}

func TestAgentsServiceBuildPermissionContextOrgAdminUsesWorkspaceMemberships(t *testing.T) {
	ctx := t.Context()
	tenantService := &stubWorkspaceManagementServiceForAgentsList{
		currentOrganization: &workspace_model.OrganizationMember{
			OrganizationID: "org-1",
			AccountID:      "account-1",
			Role:           workspace_model.OrganizationRoleAdmin,
		},
		workspaceIDsByOrganization: []string{"ws-admin", "ws-member", "ws-outside"},
		userWorkspaceMemberships: []interfaces.WorkspaceMembership{
			{WorkspaceID: "ws-member", Role: workspace_model.WorkspaceRoleMember},
			{WorkspaceID: "ws-admin", Role: workspace_model.WorkspaceRoleAdmin},
			{WorkspaceID: "ws-other-org", Role: workspace_model.WorkspaceRoleAdmin},
		},
	}
	service := &agentsService{tenantService: tenantService}

	permissionContext, err := service.buildPermissionContext(ctx, "account-1")
	require.NoError(t, err)

	require.Equal(t, []string{"ws-admin", "ws-member", "ws-outside"}, permissionContext.OrganizationDeptIDs)
	require.Equal(t, []string{"ws-member", "ws-admin"}, permissionContext.ValidDepartmentIDs)
	require.Equal(t, []string{"ws-admin"}, permissionContext.AdminDepartmentIDs)
	require.Equal(t, []string{"ws-member"}, permissionContext.NormalDepartmentIDs)
}

type stubAgentsListRepository struct {
	AgentsRepository

	getPaginatedAgentsMultipleTenantsCalled bool
	lastFilter                              AgentsFilter
}

func (s *stubAgentsListRepository) GetPaginatedAgentsMultipleTenants(_ context.Context, _ []string, filter AgentsFilter, _ int, _ int) ([]Agent, int64, error) {
	s.getPaginatedAgentsMultipleTenantsCalled = true
	s.lastFilter = filter
	return []Agent{
		{
			ID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			TenantID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			Name:     "agent-beta",
		},
	}, 1, nil
}

func (s *stubAgentsListRepository) HasPublishedAgentVersion(_ context.Context, _ string) (bool, error) {
	return true, nil
}

func (s *stubAgentsListRepository) HasPublishedWorkflow(_ context.Context, _ string) (bool, error) {
	return true, nil
}

type stubWorkspaceManagementServiceForAgentsList struct {
	interfaces.WorkspaceManagementService

	currentOrganization        *workspace_model.OrganizationMember
	workspaceIDsByOrganization []string
	userWorkspaceMemberships   []interfaces.WorkspaceMembership
}

func (s *stubWorkspaceManagementServiceForAgentsList) GetCurrentOrganization(_ context.Context, _ string) (*workspace_model.OrganizationMember, error) {
	return s.currentOrganization, nil
}

func (s *stubWorkspaceManagementServiceForAgentsList) GetWorkspaceIDsByOrganizationID(_ context.Context, _ string) ([]string, error) {
	return append([]string(nil), s.workspaceIDsByOrganization...), nil
}

func (s *stubWorkspaceManagementServiceForAgentsList) GetUserWorkspaceMemberships(_ context.Context, _ string) ([]interfaces.WorkspaceMembership, error) {
	return append([]interfaces.WorkspaceMembership(nil), s.userWorkspaceMemberships...), nil
}

type stubOrganizationServiceForAgentsList struct {
	interfaces.OrganizationService

	workspaces               []*workspace_model.Workspace
	permissionsByWorkspaceID map[string]bool
	lastPermissions          []workspace_model.WorkspacePermissionCode
}

func (s *stubOrganizationServiceForAgentsList) GetOrganizationWorkspacesList(_ context.Context, _ string) ([]*workspace_model.Workspace, error) {
	return append([]*workspace_model.Workspace(nil), s.workspaces...), nil
}

func (s *stubOrganizationServiceForAgentsList) IsOrganizationAdminOrOwner(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}

func (s *stubOrganizationServiceForAgentsList) ListWorkspaceIDsByPermission(_ context.Context, _, _ string, _ workspace_model.WorkspacePermissionCode) ([]string, error) {
	workspaceIDs := make([]string, 0, len(s.permissionsByWorkspaceID))
	for workspaceID, allowed := range s.permissionsByWorkspaceID {
		if allowed {
			workspaceIDs = append(workspaceIDs, workspaceID)
		}
	}
	return workspaceIDs, nil
}

func (s *stubOrganizationServiceForAgentsList) CheckWorkspaceOrganizationAnyPermission(_ context.Context, _, workspaceID, _ string, permissions ...workspace_model.WorkspacePermissionCode) (bool, error) {
	s.lastPermissions = append([]workspace_model.WorkspacePermissionCode(nil), permissions...)
	return s.permissionsByWorkspaceID[workspaceID], nil
}

type stubResourcePermissionService struct {
	interfaces.ResourcePermissionService
}

func (s *stubResourcePermissionService) CheckBatchResourceEditPermission(_ context.Context, _ interfaces.BatchResourcePermissionParams) (map[string]bool, error) {
	return map[string]bool{}, nil
}
