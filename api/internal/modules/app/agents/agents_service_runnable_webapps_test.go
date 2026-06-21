package agents

import (
	"context"
	"database/sql/driver"
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

func TestAgentsService_GetRunnableWebApps_OrganizationMemberWithoutWorkspaceUsesNormalOrganizationWorkspaces(t *testing.T) {
	ctx := t.Context()
	orgID := "org-1"
	wsAlpha := "ws-alpha"
	wsBeta := "ws-beta"
	wsArchived := "ws-archived"
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
		workspaces: []*workspace_model.Workspace{
			{ID: wsAlpha, Status: workspace_model.WorkspaceStatusNormal, OrganizationID: &orgID},
			{ID: wsBeta, Status: workspace_model.WorkspaceStatusNormal, OrganizationID: &orgID},
			{ID: wsArchived, Status: workspace_model.WorkspaceStatusArchived, OrganizationID: &orgID},
			{ID: wsOutside, Status: workspace_model.WorkspaceStatusNormal, OrganizationID: testStringPtr("org-2")},
		},
	}

	service := &agentsService{
		agentsRepo:        repo,
		tenantService:     tenantService,
		enterpriseService: orgService,
	}

	resp, err := service.GetRunnableWebApps(ctx, "account-1", dto.GetRunnableWebAppsRequest{})
	require.NoError(t, err)
	require.True(t, orgService.getOrganizationWorkspacesListCalled)
	require.False(t, tenantService.getAccountWorkspaceJoinsCalled)
	require.False(t, tenantService.getWorkspacesByIDsCalled)
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

func TestAgentsService_GetRunnableWebApps_AllowsOrganizationWorkspaceWithoutAgentView(t *testing.T) {
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
		permissionWorkspaceIDs: []string{"ws-alpha"},
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
	require.False(t, orgService.listWorkspaceIDsByPermissionCalled)
	require.True(t, orgService.getOrganizationWorkspacesListCalled)
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

func TestAgentsService_GetRunnableWebApps_OrganizationMemberCanRequestWorkspaceWithoutJoiningIt(t *testing.T) {
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
		workspaces: []*workspace_model.Workspace{
			{ID: wsAlpha, Status: workspace_model.WorkspaceStatusNormal, OrganizationID: &orgID},
			{ID: wsBeta, Status: workspace_model.WorkspaceStatusNormal, OrganizationID: &orgID},
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
	require.True(t, repo.listRunnableWebAppsCalled)
	require.True(t, orgService.getOrganizationWorkspacesListCalled)
	require.False(t, tenantService.getAccountWorkspaceJoinsCalled)
	require.False(t, tenantService.getWorkspacesByIDsCalled)
	require.Equal(t, []string{wsAlpha, wsBeta}, repo.lastWorkspaceIDs)
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

func TestAgentsService_GetRunnableWebApps_FiltersExplicitBuiltinAccountGrant(t *testing.T) {
	ctx := t.Context()
	db, mock, cleanup := openAgentRuntimeSurfacesMockDBWithMock(t)
	defer cleanup()

	orgID := uuid.New()
	accountID := uuid.New()
	otherAccountID := uuid.New()
	workspaceID := uuid.New()
	visibleAgentID := uuid.New()
	hiddenAgentID := uuid.New()
	hiddenSurfaceID := uuid.New()

	repo := &stubAgentsRepository{
		items: []runnableWebAppItem{
			{
				AgentID:      visibleAgentID.String(),
				WorkspaceID:  workspaceID.String(),
				WebAppID:     uuid.NewString(),
				WebAppStatus: string(AgentWebAppStatusActive),
				AgentName:    "Visible",
				AgentType:    "CONVERSATIONAL_WORKFLOW",
			},
			{
				AgentID:      hiddenAgentID.String(),
				WorkspaceID:  workspaceID.String(),
				WebAppID:     uuid.NewString(),
				WebAppStatus: string(AgentWebAppStatusActive),
				AgentName:    "Hidden",
				AgentType:    "CONVERSATIONAL_WORKFLOW",
			},
		},
	}
	tenantService := &stubWorkspaceManagementService{
		currentOrganization: &workspace_model.OrganizationMember{
			OrganizationID: orgID.String(),
			AccountID:      accountID.String(),
			Role:           workspace_model.OrganizationRoleNormal,
		},
	}
	orgService := &stubOrganizationService{
		workspaces: []*workspace_model.Workspace{
			{ID: workspaceID.String(), Status: workspace_model.WorkspaceStatusNormal, OrganizationID: testStringPtr(orgID.String())},
		},
	}
	expectRunnableAudienceDepartments(mock, accountID, orgID, nil)
	expectRunnableRuntimeCandidateRows(mock, orgID, []uuid.UUID{visibleAgentID, hiddenAgentID}, []runnableRuntimeCandidateSurfaceRow{{
		resourceID: hiddenAgentID,
		id:         hiddenSurfaceID,
		surface:    runtimeauth.PublishedRuntimeSurfaceBuiltinApp,
		enabled:    true,
		source:     runtimeauth.PublishedRuntimeSourceGrant,
	}}, []runnableRuntimeGrantRow{{
		surfaceID:   hiddenSurfaceID,
		subjectType: runtimeauth.PublishedRuntimeSubjectAccount,
		subjectID:   &otherAccountID,
		enabled:     true,
	}})

	service := &agentsService{
		agentsRepo:        repo,
		tenantService:     tenantService,
		enterpriseService: orgService,
		db:                db,
	}

	resp, err := service.GetRunnableWebApps(ctx, accountID.String(), dto.GetRunnableWebAppsRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	require.Equal(t, visibleAgentID.String(), resp.Items[0].AgentID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentsService_GetRunnableWebApps_AllowsExplicitBuiltinDepartmentGrant(t *testing.T) {
	ctx := t.Context()
	db, mock, cleanup := openAgentRuntimeSurfacesMockDBWithMock(t)
	defer cleanup()

	orgID := uuid.New()
	accountID := uuid.New()
	workspaceID := uuid.New()
	agentID := uuid.New()
	surfaceID := uuid.New()
	departmentID := uuid.New()

	repo := &stubAgentsRepository{
		items: []runnableWebAppItem{{
			AgentID:      agentID.String(),
			WorkspaceID:  workspaceID.String(),
			WebAppID:     uuid.NewString(),
			WebAppStatus: string(AgentWebAppStatusActive),
			AgentName:    "Department app",
			AgentType:    "CONVERSATIONAL_WORKFLOW",
		}},
	}
	tenantService := &stubWorkspaceManagementService{
		currentOrganization: &workspace_model.OrganizationMember{
			OrganizationID: orgID.String(),
			AccountID:      accountID.String(),
			Role:           workspace_model.OrganizationRoleNormal,
		},
	}
	orgService := &stubOrganizationService{
		workspaces: []*workspace_model.Workspace{
			{ID: workspaceID.String(), Status: workspace_model.WorkspaceStatusNormal, OrganizationID: testStringPtr(orgID.String())},
		},
	}
	expectRunnableAudienceDepartments(mock, accountID, orgID, []uuid.UUID{departmentID})
	expectRunnableRuntimeCandidateRows(mock, orgID, []uuid.UUID{agentID}, []runnableRuntimeCandidateSurfaceRow{{
		resourceID: agentID,
		id:         surfaceID,
		surface:    runtimeauth.PublishedRuntimeSurfaceBuiltinApp,
		enabled:    true,
		source:     runtimeauth.PublishedRuntimeSourceGrant,
	}}, []runnableRuntimeGrantRow{{
		surfaceID:   surfaceID,
		subjectType: runtimeauth.PublishedRuntimeSubjectDepartment,
		subjectID:   &departmentID,
		enabled:     true,
	}})
	service := &agentsService{
		agentsRepo:        repo,
		tenantService:     tenantService,
		enterpriseService: orgService,
		db:                db,
	}

	resp, err := service.GetRunnableWebApps(ctx, accountID.String(), dto.GetRunnableWebAppsRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	require.Equal(t, agentID.String(), resp.Items[0].AgentID)
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

func testStringPtr(value string) *string {
	return &value
}

type runnableRuntimeSurfaceRow struct {
	id      uuid.UUID
	surface runtimeauth.PublishedRuntimeSurface
	enabled bool
	source  string
}

type runnableRuntimeCandidateSurfaceRow struct {
	resourceID uuid.UUID
	id         uuid.UUID
	surface    runtimeauth.PublishedRuntimeSurface
	enabled    bool
	source     string
}

type runnableRuntimeGrantRow struct {
	surfaceID   uuid.UUID
	subjectType runtimeauth.PublishedRuntimeSubjectType
	subjectID   *uuid.UUID
	enabled     bool
}

func expectRunnableRuntimeCandidateRows(mock sqlmock.Sqlmock, organizationID uuid.UUID, resourceIDs []uuid.UUID, surfaces []runnableRuntimeCandidateSurfaceRow, grants []runnableRuntimeGrantRow) {
	rows := sqlmock.NewRows([]string{
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
	})
	now := time.Now().UTC().Truncate(time.Second)
	for _, surface := range surfaces {
		rows.AddRow(
			surface.id.String(),
			string(runtimeauth.PublishedRuntimeResourceAgent),
			surface.resourceID.String(),
			organizationID.String(),
			uuid.NewString(),
			string(surface.surface),
			surface.enabled,
			surface.source,
			now,
			now,
			nil,
		)
	}

	args := make([]driver.Value, 0, 3+len(resourceIDs))
	args = append(args, string(runtimeauth.PublishedRuntimeResourceAgent), string(runtimeauth.PublishedRuntimeSurfaceBuiltinApp), organizationID)
	for _, resourceID := range resourceIDs {
		args = append(args, resourceID)
	}
	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surfaces" WHERE resource_type = \$1 AND surface = \$2 AND organization_id = \$3 AND resource_id IN \(.+\) AND deleted_at IS NULL ORDER BY resource_id ASC`).
		WithArgs(args...).
		WillReturnRows(rows)
	if len(surfaces) == 0 {
		return
	}

	grantRows := sqlmock.NewRows([]string{
		"id",
		"surface_id",
		"subject_type",
		"subject_id",
		"enabled",
		"created_at",
		"updated_at",
		"deleted_at",
	})
	for _, grant := range grants {
		var subjectID interface{}
		if grant.subjectID != nil {
			subjectID = grant.subjectID.String()
		}
		grantRows.AddRow(
			uuid.NewString(),
			grant.surfaceID.String(),
			string(grant.subjectType),
			subjectID,
			grant.enabled,
			now,
			now,
			nil,
		)
	}

	grantArgs := make([]driver.Value, 0, len(surfaces))
	for _, surface := range surfaces {
		grantArgs = append(grantArgs, surface.id)
	}
	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surface_grants" WHERE surface_id IN \(.+\) AND deleted_at IS NULL ORDER BY subject_type ASC, subject_id ASC, created_at ASC`).
		WithArgs(grantArgs...).
		WillReturnRows(grantRows)
}

func expectRunnableRuntimeSurfaceRows(mock sqlmock.Sqlmock, agentID uuid.UUID, surfaces []runnableRuntimeSurfaceRow, grants []runnableRuntimeGrantRow) {
	rows := sqlmock.NewRows([]string{
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
	})
	now := time.Now().UTC().Truncate(time.Second)
	for _, surface := range surfaces {
		rows.AddRow(
			surface.id.String(),
			string(runtimeauth.PublishedRuntimeResourceAgent),
			agentID.String(),
			uuid.NewString(),
			uuid.NewString(),
			string(surface.surface),
			surface.enabled,
			surface.source,
			now,
			now,
			nil,
		)
	}
	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surfaces" WHERE resource_type = \$1 AND resource_id = \$2 AND deleted_at IS NULL ORDER BY surface ASC`).
		WithArgs(string(runtimeauth.PublishedRuntimeResourceAgent), agentID).
		WillReturnRows(rows)
	if len(surfaces) == 0 {
		return
	}

	grantRows := sqlmock.NewRows([]string{
		"id",
		"surface_id",
		"subject_type",
		"subject_id",
		"enabled",
		"created_at",
		"updated_at",
		"deleted_at",
	})
	for _, grant := range grants {
		var subjectID interface{}
		if grant.subjectID != nil {
			subjectID = grant.subjectID.String()
		}
		grantRows.AddRow(
			uuid.NewString(),
			grant.surfaceID.String(),
			string(grant.subjectType),
			subjectID,
			grant.enabled,
			now,
			now,
			nil,
		)
	}
	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surface_grants" WHERE surface_id IN \(.+\) AND deleted_at IS NULL ORDER BY subject_type ASC, subject_id ASC, created_at ASC`).
		WillReturnRows(grantRows)
}

func expectRunnableAudienceDepartments(mock sqlmock.Sqlmock, accountID, organizationID uuid.UUID, departmentIDs []uuid.UUID) {
	rows := sqlmock.NewRows([]string{"department_id"})
	for _, departmentID := range departmentIDs {
		rows.AddRow(departmentID.String())
	}
	mock.ExpectQuery(`SELECT department_members\.department_id FROM "department_members" JOIN departments ON departments\.id = department_members\.department_id WHERE department_members\.account_id = \$1 AND departments\.group_id = \$2 AND departments\.status = \$3`).
		WithArgs(accountID, organizationID, workspace_model.DepartmentStatusActive).
		WillReturnRows(rows)
}
