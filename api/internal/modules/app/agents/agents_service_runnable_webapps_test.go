package agents

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/runtimeauth"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

func TestAgentsService_GetRunnableWebApps_OrganizationAdminUsesAllNormalOrganizationWorkspaces(t *testing.T) {
	ctx := t.Context()
	orgID := "org-1"
	wsAlpha := "ws-alpha"
	wsBeta := "ws-beta"

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
		permissionWorkspaceIDs: []string{wsAlpha, wsBeta},
	}

	service := &agentsService{
		agentsRepo:        repo,
		tenantService:     tenantService,
		enterpriseService: orgService,
	}

	resp, err := service.GetRunnableWebApps(ctx, "account-1", dto.GetRunnableWebAppsRequest{})
	require.NoError(t, err)
	require.True(t, orgService.listWorkspaceIDsByPermissionCalled)
	require.False(t, orgService.getOrganizationWorkspacesListCalled)
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

func TestAgentsService_GetRunnableWebApps_FiltersOfflineParentStatus(t *testing.T) {
	ctx := t.Context()
	repo := &stubAgentsRepository{
		items: []runnableWebAppItem{
			{
				AgentID:      "agent-active",
				WorkspaceID:  "workspace-1",
				WebAppID:     "webapp-active",
				WebAppStatus: string(AgentWebAppStatusActive),
				AgentName:    "Active",
				AgentType:    "CONVERSATIONAL_WORKFLOW",
			},
			{
				AgentID:      "agent-inactive",
				WorkspaceID:  "workspace-1",
				WebAppID:     "webapp-inactive",
				WebAppStatus: string(AgentWebAppStatusInactive),
				AgentName:    "Inactive",
				AgentType:    "CONVERSATIONAL_WORKFLOW",
			},
		},
	}
	tenantService := &stubWorkspaceManagementService{
		currentOrganization: &workspace_model.OrganizationMember{
			OrganizationID: "org-1",
			AccountID:      "account-1",
			Role:           workspace_model.OrganizationRoleNormal,
		},
	}
	orgService := &stubOrganizationService{
		permissionWorkspaceIDs: []string{"workspace-1"},
	}
	service := &agentsService{
		agentsRepo:        repo,
		tenantService:     tenantService,
		enterpriseService: orgService,
	}

	resp, err := service.GetRunnableWebApps(ctx, "account-1", dto.GetRunnableWebAppsRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	require.Equal(t, "agent-active", resp.Items[0].AgentID)
}

func TestAgentsService_GetRunnableWebApps_OrganizationMemberWithoutWorkspaceViewReturnsEmpty(t *testing.T) {
	ctx := t.Context()
	orgID := "org-1"
	repo := &stubAgentsRepository{}
	tenantService := &stubWorkspaceManagementService{
		currentOrganization: &workspace_model.OrganizationMember{
			OrganizationID: orgID,
			AccountID:      "account-1",
			Role:           workspace_model.OrganizationRoleNormal,
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
	require.True(t, orgService.listWorkspaceIDsByPermissionCalled)
	require.False(t, orgService.getOrganizationWorkspacesListCalled)
	require.False(t, tenantService.getAccountWorkspaceJoinsCalled)
	require.False(t, tenantService.getWorkspacesByIDsCalled)
	require.False(t, repo.listRunnableWebAppsCalled)
	require.Equal(t, &dto.RunnableWebAppsResponse{Items: []dto.RunnableWebAppItem{}}, resp)
}

func TestAgentsService_GetRunnableWebApps_NoWorkspaceMemberCanSeeAccountGrantedAppCenterApp(t *testing.T) {
	ctx := t.Context()
	db, mock := newRunnableWebAppsMockDB(t)

	orgID := uuid.New()
	accountID := uuid.New()
	otherAccountID := uuid.New()
	workspaceID := uuid.New()
	visibleAgentID := uuid.New()
	hiddenAgentID := uuid.New()
	fallbackAgentID := uuid.New()
	visibleSurfaceID := uuid.New()
	hiddenSurfaceID := uuid.New()
	now := time.Now()
	orgIDString := orgID.String()

	repo := &stubAgentsRepository{
		items: []runnableWebAppItem{
			{
				AgentID:      visibleAgentID.String(),
				WorkspaceID:  workspaceID.String(),
				WebAppID:     "webapp-visible",
				WebAppStatus: string(AgentWebAppStatusActive),
				AgentName:    "Visible",
				AgentType:    "CONVERSATIONAL_WORKFLOW",
			},
			{
				AgentID:      hiddenAgentID.String(),
				WorkspaceID:  workspaceID.String(),
				WebAppID:     "webapp-hidden",
				WebAppStatus: string(AgentWebAppStatusActive),
				AgentName:    "Hidden",
				AgentType:    "CONVERSATIONAL_WORKFLOW",
			},
			{
				AgentID:      fallbackAgentID.String(),
				WorkspaceID:  workspaceID.String(),
				WebAppID:     "webapp-fallback",
				WebAppStatus: string(AgentWebAppStatusActive),
				AgentName:    "Fallback",
				AgentType:    "CONVERSATIONAL_WORKFLOW",
			},
		},
	}
	tenantService := &stubWorkspaceManagementService{
		currentOrganization: &workspace_model.OrganizationMember{
			OrganizationID: orgIDString,
			AccountID:      accountID.String(),
			Role:           workspace_model.OrganizationRoleNormal,
		},
	}
	orgService := &stubOrganizationService{
		permissionWorkspaceIDs: []string{},
		workspaces: []*workspace_model.Workspace{
			{ID: workspaceID.String(), Status: workspace_model.WorkspaceStatusNormal, OrganizationID: &orgIDString},
		},
	}
	service := &agentsService{
		db:                db,
		agentsRepo:        repo,
		tenantService:     tenantService,
		enterpriseService: orgService,
	}

	mock.ExpectQuery(`SELECT department_members\.department_id FROM "department_members" JOIN departments ON departments\.id = department_members\.department_id WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"department_id"}))
	mock.ExpectQuery(`SELECT "resource_id" FROM "published_runtime_surfaces" WHERE resource_type = .* AND surface = .* AND organization_id = .* AND resource_id IN`).
		WillReturnRows(sqlmock.NewRows([]string{"resource_id"}).
			AddRow(visibleAgentID.String()).
			AddRow(hiddenAgentID.String()))
	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surfaces" WHERE resource_type = .* AND surface = .* AND organization_id = .* AND resource_id IN`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"resource_type",
			"resource_id",
			"organization_id",
			"workspace_id",
			"surface",
			"enabled",
			"compatibility_source",
			"created_at",
			"updated_at",
			"deleted_at",
		}).
			AddRow(visibleSurfaceID.String(), string(runtimeauth.PublishedRuntimeResourceAgent), visibleAgentID.String(), orgID.String(), workspaceID.String(), string(runtimeauth.PublishedRuntimeSurfaceAppCenter), true, runtimeauth.PublishedRuntimeSourceGrant, now, now, nil).
			AddRow(hiddenSurfaceID.String(), string(runtimeauth.PublishedRuntimeResourceAgent), hiddenAgentID.String(), orgID.String(), workspaceID.String(), string(runtimeauth.PublishedRuntimeSurfaceAppCenter), true, runtimeauth.PublishedRuntimeSourceGrant, now, now, nil))
	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surface_grants" WHERE surface_id IN`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"surface_id",
			"subject_type",
			"subject_id",
			"enabled",
			"created_at",
			"updated_at",
			"deleted_at",
		}).
			AddRow(uuid.New().String(), visibleSurfaceID.String(), string(runtimeauth.PublishedRuntimeSubjectAccount), accountID.String(), true, now, now, nil).
			AddRow(uuid.New().String(), hiddenSurfaceID.String(), string(runtimeauth.PublishedRuntimeSubjectAccount), otherAccountID.String(), true, now, now, nil))

	resp, err := service.GetRunnableWebApps(ctx, accountID.String(), dto.GetRunnableWebAppsRequest{})
	require.NoError(t, err)
	require.True(t, orgService.listWorkspaceIDsByPermissionCalled)
	require.True(t, orgService.getOrganizationWorkspacesListCalled)
	require.Equal(t, []string{workspaceID.String()}, repo.lastWorkspaceIDs)
	require.Len(t, resp.Items, 1)
	require.Equal(t, visibleAgentID.String(), resp.Items[0].AgentID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentsService_GetRunnableWebApps_NoWorkspaceMemberCanSeeOrganizationGrantedAppCenterApp(t *testing.T) {
	ctx := t.Context()
	db, mock := newRunnableWebAppsMockDB(t)

	orgID := uuid.New()
	accountID := uuid.New()
	workspaceID := uuid.New()
	agentID := uuid.New()
	surfaceID := uuid.New()
	now := time.Now()
	orgIDString := orgID.String()

	repo := &stubAgentsRepository{
		items: []runnableWebAppItem{{
			AgentID:      agentID.String(),
			WorkspaceID:  workspaceID.String(),
			WebAppID:     "webapp-visible",
			WebAppStatus: string(AgentWebAppStatusActive),
			AgentName:    "Visible",
			AgentType:    "CONVERSATIONAL_WORKFLOW",
		}},
	}
	tenantService := &stubWorkspaceManagementService{
		currentOrganization: &workspace_model.OrganizationMember{
			OrganizationID: orgIDString,
			AccountID:      accountID.String(),
			Role:           workspace_model.OrganizationRoleNormal,
		},
	}
	orgService := &stubOrganizationService{
		permissionWorkspaceIDs: []string{},
		workspaces: []*workspace_model.Workspace{
			{ID: workspaceID.String(), Status: workspace_model.WorkspaceStatusNormal, OrganizationID: &orgIDString},
		},
	}
	service := &agentsService{
		db:                db,
		agentsRepo:        repo,
		tenantService:     tenantService,
		enterpriseService: orgService,
	}

	mock.ExpectQuery(`SELECT department_members\.department_id FROM "department_members" JOIN departments ON departments\.id = department_members\.department_id WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"department_id"}))
	mock.ExpectQuery(`SELECT "resource_id" FROM "published_runtime_surfaces" WHERE resource_type = .* AND surface = .* AND organization_id = .* AND resource_id IN`).
		WillReturnRows(sqlmock.NewRows([]string{"resource_id"}).AddRow(agentID.String()))
	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surfaces" WHERE resource_type = .* AND surface = .* AND organization_id = .* AND resource_id IN`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"resource_type",
			"resource_id",
			"organization_id",
			"workspace_id",
			"surface",
			"enabled",
			"compatibility_source",
			"created_at",
			"updated_at",
			"deleted_at",
		}).AddRow(surfaceID.String(), string(runtimeauth.PublishedRuntimeResourceAgent), agentID.String(), orgID.String(), workspaceID.String(), string(runtimeauth.PublishedRuntimeSurfaceAppCenter), true, runtimeauth.PublishedRuntimeSourceGrant, now, now, nil))
	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surface_grants" WHERE surface_id IN`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"surface_id",
			"subject_type",
			"subject_id",
			"enabled",
			"created_at",
			"updated_at",
			"deleted_at",
		}).AddRow(uuid.New().String(), surfaceID.String(), string(runtimeauth.PublishedRuntimeSubjectOrganization), orgID.String(), true, now, now, nil))

	resp, err := service.GetRunnableWebApps(ctx, accountID.String(), dto.GetRunnableWebAppsRequest{})
	require.NoError(t, err)
	require.True(t, orgService.listWorkspaceIDsByPermissionCalled)
	require.True(t, orgService.getOrganizationWorkspacesListCalled)
	require.Equal(t, []string{workspaceID.String()}, repo.lastWorkspaceIDs)
	require.Len(t, resp.Items, 1)
	require.Equal(t, agentID.String(), resp.Items[0].AgentID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentsService_GetRunnableWebApps_AllowsWorkspaceViewWithoutAgentView(t *testing.T) {
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
	}
	orgService := &stubOrganizationService{
		permissionWorkspaceIDs: []string{"ws-alpha", wsBeta},
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
		WorkspaceID: wsBeta,
	})
	require.NoError(t, err)
	require.True(t, orgService.listWorkspaceIDsByPermissionCalled)
	require.False(t, orgService.getOrganizationWorkspacesListCalled)
	require.False(t, tenantService.getAccountWorkspaceJoinsCalled)
	require.False(t, tenantService.getWorkspacesByIDsCalled)
	require.Equal(t, []string{"ws-alpha", wsBeta}, repo.lastWorkspaceIDs)
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

func TestAgentsService_GetRunnableWebApps_OrganizationMemberCannotRequestWorkspaceWithoutWorkspaceView(t *testing.T) {
	ctx := t.Context()
	orgID := "org-1"
	wsAlpha := "ws-alpha"
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
	}
	orgService := &stubOrganizationService{
		permissionWorkspaceIDs: []string{wsAlpha},
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
	require.False(t, repo.listRunnableWebAppsCalled)
	require.True(t, orgService.listWorkspaceIDsByPermissionCalled)
	require.False(t, orgService.getOrganizationWorkspacesListCalled)
	require.False(t, tenantService.getAccountWorkspaceJoinsCalled)
	require.False(t, tenantService.getWorkspacesByIDsCalled)
	require.Equal(t, &dto.RunnableWebAppsResponse{Items: []dto.RunnableWebAppItem{}}, resp)
}

func TestAgentsService_GetRunnableWebApps_IgnoresLegacyAgentBuiltinAccountGrant(t *testing.T) {
	ctx := t.Context()

	orgID := "org-1"
	accountID := "account-1"
	workspaceID := "ws-alpha"

	repo := &stubAgentsRepository{
		items: []runnableWebAppItem{
			{
				AgentID:      "agent-visible",
				WorkspaceID:  workspaceID,
				WebAppID:     "webapp-visible",
				WebAppStatus: string(AgentWebAppStatusActive),
				AgentName:    "Visible",
				AgentType:    "CONVERSATIONAL_WORKFLOW",
			},
			{
				AgentID:      "agent-previously-hidden",
				WorkspaceID:  workspaceID,
				WebAppID:     "webapp-previously-hidden",
				WebAppStatus: string(AgentWebAppStatusActive),
				AgentName:    "Previously hidden",
				AgentType:    "CONVERSATIONAL_WORKFLOW",
			},
		},
	}
	tenantService := &stubWorkspaceManagementService{
		currentOrganization: &workspace_model.OrganizationMember{
			OrganizationID: orgID,
			AccountID:      accountID,
			Role:           workspace_model.OrganizationRoleNormal,
		},
	}
	orgService := &stubOrganizationService{
		permissionWorkspaceIDs: []string{workspaceID},
	}

	service := &agentsService{
		agentsRepo:        repo,
		tenantService:     tenantService,
		enterpriseService: orgService,
	}

	resp, err := service.GetRunnableWebApps(ctx, accountID, dto.GetRunnableWebAppsRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Items, 2)
	require.Equal(t, "agent-visible", resp.Items[0].AgentID)
	require.Equal(t, "agent-previously-hidden", resp.Items[1].AgentID)
}

func TestAgentsService_GetRunnableWebApps_ReturnsWorkspaceVisibleAppWithoutRuntimeAuthGrant(t *testing.T) {
	ctx := t.Context()

	orgID := "org-1"
	accountID := "account-1"
	workspaceID := "ws-alpha"

	repo := &stubAgentsRepository{
		items: []runnableWebAppItem{{
			AgentID:      "agent-1",
			WorkspaceID:  workspaceID,
			WebAppID:     "webapp-1",
			WebAppStatus: string(AgentWebAppStatusActive),
			AgentName:    "Department app",
			AgentType:    "CONVERSATIONAL_WORKFLOW",
		}},
	}
	tenantService := &stubWorkspaceManagementService{
		currentOrganization: &workspace_model.OrganizationMember{
			OrganizationID: orgID,
			AccountID:      accountID,
			Role:           workspace_model.OrganizationRoleNormal,
		},
	}
	orgService := &stubOrganizationService{
		permissionWorkspaceIDs: []string{workspaceID},
	}
	service := &agentsService{
		agentsRepo:        repo,
		tenantService:     tenantService,
		enterpriseService: orgService,
	}

	resp, err := service.GetRunnableWebApps(ctx, accountID, dto.GetRunnableWebAppsRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	require.Equal(t, "agent-1", resp.Items[0].AgentID)
}

func TestAgentsService_GetRunnableWebApps_FiltersByAppCenterRuntimeGrant(t *testing.T) {
	ctx := t.Context()
	db, mock := newRunnableWebAppsMockDB(t)

	orgID := uuid.New()
	accountID := uuid.New()
	otherAccountID := uuid.New()
	workspaceID := uuid.New()
	otherWorkspaceID := uuid.New()
	visibleAgentID := uuid.New()
	hiddenAgentID := uuid.New()
	visibleSurfaceID := uuid.New()
	hiddenSurfaceID := uuid.New()
	now := time.Now()
	orgIDString := orgID.String()

	repo := &stubAgentsRepository{
		items: []runnableWebAppItem{
			{
				AgentID:      visibleAgentID.String(),
				WorkspaceID:  workspaceID.String(),
				WebAppID:     "webapp-visible",
				WebAppStatus: string(AgentWebAppStatusActive),
				AgentName:    "Visible",
				AgentType:    "CONVERSATIONAL_WORKFLOW",
			},
			{
				AgentID:      hiddenAgentID.String(),
				WorkspaceID:  otherWorkspaceID.String(),
				WebAppID:     "webapp-hidden",
				WebAppStatus: string(AgentWebAppStatusActive),
				AgentName:    "Hidden",
				AgentType:    "CONVERSATIONAL_WORKFLOW",
			},
		},
	}
	tenantService := &stubWorkspaceManagementService{
		currentOrganization: &workspace_model.OrganizationMember{
			OrganizationID: orgIDString,
			AccountID:      accountID.String(),
			Role:           workspace_model.OrganizationRoleNormal,
		},
	}
	orgService := &stubOrganizationService{
		permissionWorkspaceIDs: []string{workspaceID.String(), otherWorkspaceID.String()},
		workspaces: []*workspace_model.Workspace{
			{ID: workspaceID.String(), Status: workspace_model.WorkspaceStatusNormal, OrganizationID: &orgIDString},
			{ID: otherWorkspaceID.String(), Status: workspace_model.WorkspaceStatusNormal, OrganizationID: &orgIDString},
		},
	}
	service := &agentsService{
		db:                db,
		agentsRepo:        repo,
		tenantService:     tenantService,
		enterpriseService: orgService,
	}

	mock.ExpectQuery(`SELECT department_members\.department_id FROM "department_members" JOIN departments ON departments\.id = department_members\.department_id WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"department_id"}))
	mock.ExpectQuery(`SELECT "resource_id" FROM "published_runtime_surfaces" WHERE resource_type = .* AND surface = .* AND organization_id = .* AND resource_id IN`).
		WillReturnRows(sqlmock.NewRows([]string{"resource_id"}).
			AddRow(visibleAgentID.String()).
			AddRow(hiddenAgentID.String()))
	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surfaces" WHERE resource_type = .* AND surface = .* AND organization_id = .* AND resource_id IN`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"resource_type",
			"resource_id",
			"organization_id",
			"workspace_id",
			"surface",
			"enabled",
			"compatibility_source",
			"created_at",
			"updated_at",
			"deleted_at",
		}).
			AddRow(visibleSurfaceID.String(), string(runtimeauth.PublishedRuntimeResourceAgent), visibleAgentID.String(), orgID.String(), workspaceID.String(), string(runtimeauth.PublishedRuntimeSurfaceAppCenter), true, runtimeauth.PublishedRuntimeSourceGrant, now, now, nil).
			AddRow(hiddenSurfaceID.String(), string(runtimeauth.PublishedRuntimeResourceAgent), hiddenAgentID.String(), orgID.String(), otherWorkspaceID.String(), string(runtimeauth.PublishedRuntimeSurfaceAppCenter), true, runtimeauth.PublishedRuntimeSourceGrant, now, now, nil))
	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surface_grants" WHERE surface_id IN`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"surface_id",
			"subject_type",
			"subject_id",
			"enabled",
			"created_at",
			"updated_at",
			"deleted_at",
		}).
			AddRow(uuid.New().String(), visibleSurfaceID.String(), string(runtimeauth.PublishedRuntimeSubjectAccount), accountID.String(), true, now, now, nil).
			AddRow(uuid.New().String(), hiddenSurfaceID.String(), string(runtimeauth.PublishedRuntimeSubjectAccount), otherAccountID.String(), true, now, now, nil))

	resp, err := service.GetRunnableWebApps(ctx, accountID.String(), dto.GetRunnableWebAppsRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	require.Equal(t, visibleAgentID.String(), resp.Items[0].AgentID)
	require.NoError(t, mock.ExpectationsWereMet())
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
	orgService := &stubOrganizationService{
		permissionWorkspaceIDs: []string{"ws-alpha", "ws-beta"},
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

	getAccountWorkspaceJoinsCalled bool
	getWorkspacesByIDsCalled       bool
}

func (s *stubWorkspaceManagementService) GetCurrentOrganization(_ context.Context, _ string) (*workspace_model.OrganizationMember, error) {
	return s.currentOrganization, s.err
}

func (s *stubWorkspaceManagementService) GetAccountWorkspaceJoins(_ context.Context, _ string) ([]*workspace_model.WorkspaceMember, error) {
	s.getAccountWorkspaceJoinsCalled = true
	return append([]*workspace_model.WorkspaceMember(nil), s.workspaceJoins...), nil
}

func (s *stubWorkspaceManagementService) GetWorkspacesByIDs(_ context.Context, workspaceIDs []string) ([]*workspace_model.Workspace, error) {
	s.getWorkspacesByIDsCalled = true
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
