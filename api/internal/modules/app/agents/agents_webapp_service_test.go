package agents

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/runtimeauth"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestAgentsService_UpdateWebAppStatus_AllowsManagerEditor(t *testing.T) {
	ctx := webAppStatusTestContext()
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	accountID := "99999999-9999-9999-9999-999999999999"
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:        agentID,
			TenantID:  uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			WebAppID:  uuid.MustParse("33333333-3333-3333-3333-333333333333"),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}
	orgService := &stubWebAppStatusOrganizationService{allowed: true}
	service := &agentsService{
		agentsRepo:        repo,
		accountService:    &stubWebAppStatusAccountService{isEditor: true},
		enterpriseService: orgService,
	}

	resp, err := service.UpdateWebAppStatus(ctx, agentID.String(), dto.UpdateWebAppStatusRequest{
		Status: string(AgentWebAppStatusInactive),
		Reason: "maintenance",
	})
	require.NoError(t, err)
	require.Equal(t, agentID.String(), resp.AgentID)
	require.Equal(t, "33333333-3333-3333-3333-333333333333", resp.WebAppID)
	require.Equal(t, "inactive", resp.WebAppStatus)
	require.Equal(t, agentRuntimeAccessManagePermissionCodes("AGENT"), orgService.lastPermissions)
	require.Equal(t, AgentWebAppStatusInactive, repo.lastStatus)
	require.Equal(t, "maintenance", repo.lastReason)
	require.Equal(t, accountID, repo.lastUpdatedBy)
}

func TestAgentsService_CreateWorkflowRequiresWorkflowCreatePermission(t *testing.T) {
	ctx := webAppStatusTestContext()
	orgService := &stubWebAppStatusOrganizationService{
		allowedPermissions: map[workspace_model.WorkspacePermissionCode]bool{
			workspace_model.WorkspacePermissionAgentCreate: true,
		},
	}
	service := &agentsService{enterpriseService: orgService}

	_, err := service.CreateAgent(ctx, "22222222-2222-2222-2222-222222222222", dto.CreateAgentRequest{
		Name:      "Workflow",
		AgentType: "WORKFLOW",
	}, "99999999-9999-9999-9999-999999999999")

	require.EqualError(t, err, "permission denied")
	require.True(t, orgService.checkCalled)
	require.Equal(t, []workspace_model.WorkspacePermissionCode{
		workspace_model.WorkspacePermissionWorkflowCreate,
	}, orgService.lastPermissions)
}

func TestAgentsService_UpdateWorkflowTenantRequiresMovePermission(t *testing.T) {
	ctx := webAppStatusTestContext()
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	orgService := &stubWebAppStatusOrganizationService{
		allowedPermissions: map[workspace_model.WorkspacePermissionCode]bool{
			workspace_model.WorkspacePermissionWorkflowUpdate: true,
		},
	}
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:         agentID,
			TenantID:   uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			AgentsType: "WORKFLOW",
		},
	}
	service := &agentsService{
		agentsRepo:        repo,
		enterpriseService: orgService,
	}

	_, err := service.UpdateAgent(ctx, agentID.String(), map[string]interface{}{
		"tenant_id": "33333333-3333-3333-3333-333333333333",
	})

	require.EqualError(t, err, "permission denied")
	require.True(t, orgService.checkCalled)
	require.Equal(t, []workspace_model.WorkspacePermissionCode{
		workspace_model.WorkspacePermissionWorkflowMove,
	}, orgService.lastPermissions)
	require.False(t, repo.updateAgentCalled)
}

func TestAgentsService_UpdateWebAppStatus_RejectsInvalidInputs(t *testing.T) {
	service := &agentsService{}

	_, err := service.UpdateWebAppStatus(webAppStatusTestContext(), "agent-1", dto.UpdateWebAppStatusRequest{
		Status: "archived",
	})
	require.ErrorIs(t, err, errInvalidWebAppStatus)

	_, err = service.UpdateWebAppStatus(webAppStatusTestContext(), "agent-1", dto.UpdateWebAppStatusRequest{
		Status: string(AgentWebAppStatusInactive),
		Reason: strings.Repeat("a", maxWebAppOfflineReasonLength+1),
	})
	require.ErrorIs(t, err, errWebAppOfflineReasonTooLong)
}

func TestAgentsService_UpdateWebAppStatus_RejectsMissingPermission(t *testing.T) {
	ctx := webAppStatusTestContext()
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:       agentID,
			TenantID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			WebAppID: uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		},
	}
	service := &agentsService{
		agentsRepo:        repo,
		accountService:    &stubWebAppStatusAccountService{isEditor: true},
		enterpriseService: &stubWebAppStatusOrganizationService{allowed: false},
	}

	_, err := service.UpdateWebAppStatus(ctx, agentID.String(), dto.UpdateWebAppStatusRequest{
		Status: string(AgentWebAppStatusInactive),
	})
	require.EqualError(t, err, "permission denied")
	require.False(t, repo.updateCalled)
}

func TestAgentsService_UpdateWebAppStatus_RejectsRuntimeConfigWithoutRuntimeAccessPermission(t *testing.T) {
	ctx := webAppStatusTestContext()
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:       agentID,
			TenantID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			WebAppID: uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		},
	}
	orgService := &stubWebAppStatusOrganizationService{
		allowedPermissions: map[workspace_model.WorkspacePermissionCode]bool{
			workspace_model.WorkspacePermissionAgentUpdate: true,
		},
	}
	service := &agentsService{
		agentsRepo:        repo,
		accountService:    &stubWebAppStatusAccountService{isEditor: true},
		enterpriseService: orgService,
	}

	_, err := service.UpdateWebAppStatus(ctx, agentID.String(), dto.UpdateWebAppStatusRequest{
		Status: string(AgentWebAppStatusInactive),
	})

	require.EqualError(t, err, "permission denied")
	require.Equal(t, agentRuntimeAccessManagePermissionCodes("AGENT"), orgService.lastPermissions)
	require.False(t, repo.updateCalled)
}

func TestAgentsService_UpdateWebAppStatus_RejectsSystemManagedAgent(t *testing.T) {
	ctx := webAppStatusTestContext()
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:       agentID,
			TenantID: uuid.Nil,
			WebAppID: uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		},
	}
	service := &agentsService{
		agentsRepo:     repo,
		accountService: &stubWebAppStatusAccountService{isEditor: true},
	}

	_, err := service.UpdateWebAppStatus(ctx, agentID.String(), dto.UpdateWebAppStatusRequest{
		Status: string(AgentWebAppStatusInactive),
	})
	require.EqualError(t, err, "agent not found")
	require.False(t, repo.updateCalled)
}

func TestAgentsService_GetPublishedAgentWebAppConfig_RejectsUnpublishedActiveWebApp(t *testing.T) {
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	webAppID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:           agentID,
			TenantID:     uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			WebAppID:     webAppID,
			AgentsType:   "AGENT",
			WebAppStatus: AgentWebAppStatusActive,
		},
	}
	service := &agentsService{agentsRepo: repo}

	_, err := service.GetPublishedAgentWebAppConfig(context.Background(), webAppID.String())
	require.ErrorIs(t, err, errAgentWebAppNotPublished)
}

func TestAgentsService_GetPublishedAgentWebAppConfig_RejectsPersistedDisabledWebApp(t *testing.T) {
	db, mock, cleanup := openAgentRuntimeSurfacesMockDBWithMock(t)
	defer cleanup()

	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	webAppID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	workspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:           agentID,
			TenantID:     workspaceID,
			WebAppID:     webAppID,
			AgentsType:   "AGENT",
			WebAppStatus: AgentWebAppStatusActive,
			EnableAPI:    true,
		},
	}
	expectAgentRuntimeSurfaceRows(mock, agentID, workspaceID, []agentRuntimeSurfaceExpectation{{
		surface: "webapp",
		enabled: false,
	}})
	service := &agentsService{agentsRepo: repo, db: db}

	_, err := service.GetPublishedAgentWebAppConfig(context.Background(), webAppID.String())
	require.ErrorIs(t, err, errAgentWebAppOffline)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentsService_GetPublishedAgentWebAppConfig_RejectsOfflineParentDespitePersistedEnabledWebAppSurface(t *testing.T) {
	db, mock, cleanup := openAgentRuntimeSurfacesMockDBWithMock(t)
	defer cleanup()

	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	webAppID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	workspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:           agentID,
			TenantID:     workspaceID,
			WebAppID:     webAppID,
			AgentsType:   "AGENT",
			WebAppStatus: AgentWebAppStatusInactive,
			EnableAPI:    true,
		},
	}
	service := &agentsService{agentsRepo: repo, db: db}

	_, err := service.GetPublishedAgentWebAppConfig(context.Background(), webAppID.String())
	require.ErrorIs(t, err, errAgentWebAppOffline)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentsService_GetPublishedAgentWebAppConfig_AllowsPersistedPrivateWebAppGrant(t *testing.T) {
	db, mock, cleanup := openAgentRuntimeSurfacesMockDBWithMock(t)
	defer cleanup()

	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	webAppID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	workspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	accountGrantID := uuid.MustParse("99999999-9999-9999-9999-999999999998")
	versionID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:           agentID,
			TenantID:     workspaceID,
			WebAppID:     webAppID,
			AgentsType:   "AGENT",
			WebAppStatus: AgentWebAppStatusActive,
			EnableAPI:    true,
		},
		latestVersion: &AgentPublishedVersion{
			AgentID:     agentID,
			WorkspaceID: workspaceID,
			Version:     "v1",
			VersionUUID: versionID,
			ConfigSnapshot: map[string]interface{}{
				"supports_vision": true,
			},
		},
	}
	expectAgentRuntimeSurfaceRowsWithGrant(mock, agentID, workspaceID, "webapp", true, "account", accountGrantID)
	service := &agentsService{agentsRepo: repo, db: db}

	got, err := service.GetPublishedAgentWebAppConfig(context.Background(), webAppID.String())
	require.NoError(t, err)
	require.Equal(t, webAppID.String(), got.WebAppID)
	require.Equal(t, versionID.String(), got.VersionUUID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentsService_GetWebAppRuntimeCapability_LoginRequiredForPrivateOrganizationGrant(t *testing.T) {
	db, mock, cleanup := openAgentRuntimeSurfacesMockDBWithMock(t)
	defer cleanup()

	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	webAppID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	workspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	organizationID := uuid.MustParse("88888888-8888-8888-8888-888888888888")
	versionID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:           agentID,
			TenantID:     workspaceID,
			WebAppID:     webAppID,
			AgentsType:   "AGENT",
			WebAppStatus: AgentWebAppStatusActive,
		},
		latestVersion: &AgentPublishedVersion{
			AgentID:     agentID,
			WorkspaceID: workspaceID,
			Version:     "v1",
			VersionUUID: versionID,
		},
	}
	expectAgentRuntimeSurfaceRowsWithGrantAndOrganization(mock, agentID, organizationID, workspaceID, "webapp", true, "organization", organizationID)
	service := &agentsService{agentsRepo: repo, db: db}

	got, err := service.GetWebAppRuntimeCapability(context.Background(), webAppID.String(), uuid.NewString(), false)
	require.NoError(t, err)
	require.False(t, got.Allowed)
	require.Equal(t, agentWebAppCapabilityReasonLoginRequired, got.Reason)
	require.False(t, got.PublicOnly)
	require.True(t, got.PrivateAudienceEnabled)
	require.Equal(t, []string{"public", "organization", "department", "workspace", "account"}, got.SupportedSubjectTypes)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentsService_GetWebAppRuntimeCapability_RejectsOfflineParent(t *testing.T) {
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	webAppID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	workspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:           agentID,
			TenantID:     workspaceID,
			WebAppID:     webAppID,
			AgentsType:   "AGENT",
			WebAppStatus: AgentWebAppStatusInactive,
		},
	}
	service := &agentsService{agentsRepo: repo}

	got, err := service.GetWebAppRuntimeCapability(context.Background(), webAppID.String(), uuid.NewString(), false)
	require.NoError(t, err)
	require.False(t, got.Allowed)
	require.Equal(t, string(runtimeauth.RuntimeAccessDeniedDisabledSurface), got.Reason)
	require.True(t, got.PublicOnly)
	require.False(t, got.PrivateAudienceEnabled)
	require.Equal(t, []string{"public", "organization", "department", "workspace", "account"}, got.SupportedSubjectTypes)
}

func TestAgentsService_GetWebAppRuntimeCapability_AllowsAuthenticatedOrganizationGrant(t *testing.T) {
	db, mock, cleanup := openAgentRuntimeSurfacesMockDBWithMock(t)
	defer cleanup()

	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	webAppID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	workspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	organizationID := uuid.MustParse("88888888-8888-8888-8888-888888888888")
	accountID := uuid.MustParse("99999999-9999-9999-9999-999999999999")
	versionID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:           agentID,
			TenantID:     workspaceID,
			WebAppID:     webAppID,
			AgentsType:   "AGENT",
			WebAppStatus: AgentWebAppStatusActive,
		},
		latestVersion: &AgentPublishedVersion{
			AgentID:     agentID,
			WorkspaceID: workspaceID,
			Version:     "v1",
			VersionUUID: versionID,
		},
	}
	expectAgentRuntimeSurfaceRowsWithGrantAndOrganization(mock, agentID, organizationID, workspaceID, "webapp", true, "organization", organizationID)
	expectWebAppRuntimeAudience(mock, accountID, organizationID, nil, 1)
	service := &agentsService{agentsRepo: repo, db: db}

	got, err := service.GetWebAppRuntimeCapability(context.Background(), webAppID.String(), accountID.String(), true)
	require.NoError(t, err)
	require.True(t, got.Allowed)
	require.Equal(t, string(runtimeauth.RuntimeAccessAllowedOrganizationGrant), got.Reason)
	require.False(t, got.PublicOnly)
	require.True(t, got.PrivateAudienceEnabled)
	require.Equal(t, versionID.String(), got.VersionUUID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentsService_GetWebAppRuntimeCapability_AllowsAuthenticatedDepartmentGrant(t *testing.T) {
	db, mock, cleanup := openAgentRuntimeSurfacesMockDBWithMock(t)
	defer cleanup()

	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	webAppID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	workspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	organizationID := uuid.MustParse("88888888-8888-8888-8888-888888888888")
	accountID := uuid.MustParse("99999999-9999-9999-9999-999999999999")
	departmentID := uuid.MustParse("99999999-9999-9999-9999-999999999998")
	versionID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:           agentID,
			TenantID:     workspaceID,
			WebAppID:     webAppID,
			AgentsType:   "AGENT",
			WebAppStatus: AgentWebAppStatusActive,
		},
		latestVersion: &AgentPublishedVersion{
			AgentID:     agentID,
			WorkspaceID: workspaceID,
			Version:     "v1",
			VersionUUID: versionID,
		},
	}
	expectAgentRuntimeSurfaceRowsWithGrantAndOrganization(mock, agentID, organizationID, workspaceID, "webapp", true, "department", departmentID)
	expectWebAppRuntimeAudience(mock, accountID, organizationID, []uuid.UUID{departmentID}, 1)
	service := &agentsService{agentsRepo: repo, db: db}

	got, err := service.GetWebAppRuntimeCapability(context.Background(), webAppID.String(), accountID.String(), true)
	require.NoError(t, err)
	require.True(t, got.Allowed)
	require.Equal(t, string(runtimeauth.RuntimeAccessAllowedDepartmentGrant), got.Reason)
	require.False(t, got.PublicOnly)
	require.True(t, got.PrivateAudienceEnabled)
	require.Equal(t, versionID.String(), got.VersionUUID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentsService_GetAgentConfig_DeniedDoesNotCreateDraftConfig(t *testing.T) {
	ctx := webAppStatusTestContext()
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:         agentID,
			TenantID:   uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			AgentsType: "AGENT",
		},
	}
	service := &agentsService{
		agentsRepo:        repo,
		accountService:    &stubWebAppStatusAccountService{isEditor: true},
		enterpriseService: &stubWebAppStatusOrganizationService{allowed: false},
	}

	_, err := service.GetAgentConfig(ctx, agentID.String(), "99999999-9999-9999-9999-999999999999")
	require.EqualError(t, err, "permission denied")
	require.False(t, repo.createConfigCalled)
	require.False(t, repo.updateAgentCalled)
}

func TestAgentsService_GetAgentConfig_AuthorizedCreatesDraftConfig(t *testing.T) {
	ctx := webAppStatusTestContext()
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:         agentID,
			TenantID:   uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			AgentsType: "AGENT",
		},
	}
	service := &agentsService{
		agentsRepo:        repo,
		accountService:    &stubWebAppStatusAccountService{isEditor: true},
		enterpriseService: &stubWebAppStatusOrganizationService{allowed: true},
	}

	resp, err := service.GetAgentConfig(ctx, agentID.String(), "99999999-9999-9999-9999-999999999999")
	require.NoError(t, err)
	require.Equal(t, agentID.String(), resp.AgentID)
	require.True(t, repo.createConfigCalled)
	require.True(t, repo.updateAgentCalled)
	require.NotNil(t, repo.agent.AgentsModelConfigID)
}

func TestAgentsService_GetAgentConfig_AllowsRuntimeDetailReadPermissions(t *testing.T) {
	ctx := webAppStatusTestContext()
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	accountID := "99999999-9999-9999-9999-999999999999"
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:         agentID,
			TenantID:   uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			AgentsType: "AGENT",
		},
	}
	orgService := &stubWebAppStatusOrganizationService{
		allowedPermissions: map[workspace_model.WorkspacePermissionCode]bool{
			workspace_model.WorkspacePermissionAgentPublish: true,
		},
	}
	service := &agentsService{
		agentsRepo:        repo,
		accountService:    &stubWebAppStatusAccountService{isEditor: false},
		enterpriseService: orgService,
	}

	resp, err := service.GetAgentConfig(ctx, agentID.String(), accountID)
	require.NoError(t, err)
	require.Equal(t, agentID.String(), resp.AgentID)
	require.True(t, orgService.checkCalled)
	require.Equal(t, agentRuntimeConfigReadPermissionCodes("AGENT"), orgService.lastPermissions)
	require.Contains(t, orgService.lastPermissions, workspace_model.WorkspacePermissionAgentView)
	require.NotContains(t, orgService.lastPermissions, workspace_model.WorkspacePermissionAgentCreate)
	require.True(t, repo.createConfigCalled)
}

func TestAgentsService_GetAgentConfig_RejectsCreateOnlyPermission(t *testing.T) {
	ctx := webAppStatusTestContext()
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	accountID := "99999999-9999-9999-9999-999999999999"
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:         agentID,
			TenantID:   uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			AgentsType: "AGENT",
		},
	}
	orgService := &stubWebAppStatusOrganizationService{
		allowedPermissions: map[workspace_model.WorkspacePermissionCode]bool{
			workspace_model.WorkspacePermissionAgentCreate: true,
		},
	}
	service := &agentsService{
		agentsRepo:        repo,
		accountService:    &stubWebAppStatusAccountService{isEditor: false},
		enterpriseService: orgService,
	}

	_, err := service.GetAgentConfig(ctx, agentID.String(), accountID)
	require.EqualError(t, err, "permission denied")
	require.True(t, orgService.checkCalled)
	require.Equal(t, agentRuntimeConfigReadPermissionCodes("AGENT"), orgService.lastPermissions)
	require.NotContains(t, orgService.lastPermissions, workspace_model.WorkspacePermissionAgentCreate)
	require.False(t, repo.createConfigCalled)
}

func TestAgentsService_GetAgentDraftRuntimeConfig_StillRequiresAgentUpdate(t *testing.T) {
	ctx := webAppStatusTestContext()
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	accountID := "99999999-9999-9999-9999-999999999999"
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:         agentID,
			TenantID:   uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			AgentsType: "AGENT",
		},
	}
	orgService := &stubWebAppStatusOrganizationService{
		allowedPermissions: map[workspace_model.WorkspacePermissionCode]bool{
			workspace_model.WorkspacePermissionAgentPublish: true,
		},
	}
	service := &agentsService{
		agentsRepo:        repo,
		accountService:    &stubWebAppStatusAccountService{isEditor: false},
		enterpriseService: orgService,
	}

	_, err := service.GetAgentDraftRuntimeConfig(ctx, agentID.String(), accountID)
	require.EqualError(t, err, "permission denied")
	require.True(t, orgService.checkCalled)
	require.Equal(t, agentRuntimeConfigManagePermissionCodes("AGENT"), orgService.lastPermissions)
	require.False(t, repo.createConfigCalled)
}

func TestAgentsService_AgentMemoryEndpointsRequireManagePermission(t *testing.T) {
	ctx := webAppStatusTestContext()
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	workspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	accountID := "99999999-9999-9999-9999-999999999999"

	tests := []struct {
		name string
		call func(*agentsService) error
	}{
		{
			name: "list slots",
			call: func(service *agentsService) error {
				_, err := service.ListAgentMemorySlots(ctx, agentID.String(), accountID)
				return err
			},
		},
		{
			name: "replace slots",
			call: func(service *agentsService) error {
				_, err := service.ReplaceAgentMemorySlots(ctx, agentID.String(), accountID, []dto.AgentMemorySlotConfig{{Key: "profile", Enabled: true}})
				return err
			},
		},
		{
			name: "list values",
			call: func(service *agentsService) error {
				_, err := service.ListAgentMemoryValues(ctx, agentID.String(), accountID)
				return err
			},
		},
		{
			name: "update value",
			call: func(service *agentsService) error {
				_, err := service.UpdateAgentMemoryValue(ctx, agentID.String(), accountID, dto.UpdateAgentMemoryValueRequest{Key: "profile", Content: "Alice"})
				return err
			},
		},
		{
			name: "clear value",
			call: func(service *agentsService) error {
				_, err := service.ClearAgentMemoryValue(ctx, agentID.String(), accountID, "profile")
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orgService := &stubWebAppStatusOrganizationService{allowed: false}
			service := &agentsService{
				agentsRepo: &stubWebAppStatusRepository{
					agent: &Agent{
						ID:         agentID,
						TenantID:   workspaceID,
						AgentsType: "AGENT",
					},
				},
				accountService:    &stubWebAppStatusAccountService{isEditor: true},
				enterpriseService: orgService,
			}

			err := tt.call(service)
			require.EqualError(t, err, "permission denied")
			require.True(t, orgService.checkCalled)
			require.Equal(t, agentRuntimeConfigManagePermissionCodes("AGENT"), orgService.lastPermissions)
		})
	}
}

func TestAgentsService_AgentMemoryEndpointsAllowManagePermissionWithoutEditorRole(t *testing.T) {
	ctx := webAppStatusTestContext()
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	orgService := &stubWebAppStatusOrganizationService{allowed: true}
	service := &agentsService{
		agentsRepo: &stubWebAppStatusRepository{
			agent: &Agent{
				ID:         agentID,
				TenantID:   uuid.MustParse("22222222-2222-2222-2222-222222222222"),
				AgentsType: "AGENT",
			},
		},
		accountService:    &stubWebAppStatusAccountService{isEditor: false},
		enterpriseService: orgService,
	}

	_, err := service.ListAgentMemorySlots(ctx, agentID.String(), "99999999-9999-9999-9999-999999999999")
	require.NoError(t, err)
	require.True(t, orgService.checkCalled)
	require.Equal(t, agentRuntimeConfigManagePermissionCodes("AGENT"), orgService.lastPermissions)
}

func TestAgentsService_GetAgent_RejectsMissingWorkspaceViewPermission(t *testing.T) {
	ctx := webAppStatusTestContext()
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	orgService := &stubWebAppStatusOrganizationService{allowed: false}
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:       agentID,
			TenantID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		},
	}
	service := &agentsService{
		agentsRepo:        repo,
		enterpriseService: orgService,
	}

	_, err := service.GetAgent(ctx, agentID.String())
	require.EqualError(t, err, "permission denied")
	require.True(t, orgService.checkCalled)
	require.Equal(t, agentAssetVisiblePermissionCodes(), orgService.lastPermissions)
	require.Contains(t, orgService.lastPermissions, workspace_model.WorkspacePermissionAgentView)
	require.NotContains(t, orgService.lastPermissions, workspace_model.WorkspacePermissionAgentCreate)
	require.NotContains(t, orgService.lastPermissions, workspace_model.WorkspacePermissionWorkflowCreate)
	require.NotContains(t, orgService.lastPermissions, workspace_model.WorkspacePermissionWorkflowImport)
	require.NotContains(t, orgService.lastPermissions, workspace_model.WorkspacePermissionAgentManage)
}

func TestAgentsService_GetAgentRuntimeSurfaces_UsesWorkspaceViewAndLegacyFallback(t *testing.T) {
	ctx := webAppStatusTestContext()
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	workspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	orgService := &stubWebAppStatusOrganizationService{
		allowed:        true,
		organizationID: "88888888-8888-8888-8888-888888888888",
	}
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:           agentID,
			TenantID:     workspaceID,
			WebAppID:     uuid.MustParse("33333333-3333-3333-3333-333333333333"),
			WebAppStatus: AgentWebAppStatusInactive,
			EnableAPI:    true,
		},
	}
	service := &agentsService{
		agentsRepo:        repo,
		enterpriseService: orgService,
	}

	resp, err := service.GetAgentRuntimeSurfaces(ctx, agentID.String(), "99999999-9999-9999-9999-999999999999")
	require.NoError(t, err)
	require.Equal(t, agentID.String(), resp.AgentID)
	require.Equal(t, workspaceID.String(), resp.WorkspaceID)
	require.Equal(t, "88888888-8888-8888-8888-888888888888", resp.OrganizationID)
	require.True(t, orgService.checkCalled)
	require.Equal(t, agentAssetVisiblePermissionCodes(), orgService.lastPermissions)
	require.Contains(t, orgService.lastPermissions, workspace_model.WorkspacePermissionAgentView)
	require.NotContains(t, orgService.lastPermissions, workspace_model.WorkspacePermissionAgentCreate)
	require.NotContains(t, orgService.lastPermissions, workspace_model.WorkspacePermissionWorkflowCreate)
	require.NotContains(t, orgService.lastPermissions, workspace_model.WorkspacePermissionWorkflowImport)
	require.NotContains(t, orgService.lastPermissions, workspace_model.WorkspacePermissionAgentManage)

	surfaces := runtimeSurfaceTestMap(resp.Surfaces)
	require.Len(t, surfaces, 4)
	require.False(t, surfaces["webapp"].Enabled)
	require.False(t, surfaces["app_center"].Enabled)
	require.Len(t, surfaces["app_center"].Grants, 1)
	require.Equal(t, "workspace", surfaces["app_center"].Grants[0].SubjectType)
	require.NotNil(t, surfaces["app_center"].Grants[0].SubjectID)
	require.Equal(t, workspaceID.String(), *surfaces["app_center"].Grants[0].SubjectID)
	require.True(t, surfaces["api"].Enabled)
	require.True(t, surfaces["internal"].Enabled)
	_, hasBuiltinApp := surfaces["builtin_app"]
	require.False(t, hasBuiltinApp)
}

func TestAgentsService_GetAgentRuntimeSurfaces_UsesCurrentAgentWorkspaceOverPersistedMetadata(t *testing.T) {
	ctx := webAppStatusTestContext()
	db, mock, cleanup := openAgentRuntimeSurfacesMockDBWithMock(t)
	defer cleanup()

	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	currentWorkspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	staleWorkspaceID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	expectAgentRuntimeSurfaceRows(mock, agentID, staleWorkspaceID, []agentRuntimeSurfaceExpectation{
		{surface: string(runtimeauth.PublishedRuntimeSurfaceWebApp), enabled: true},
	})

	service := &agentsService{
		db: db,
		agentsRepo: &stubWebAppStatusRepository{agent: &Agent{
			ID:           agentID,
			TenantID:     currentWorkspaceID,
			WebAppStatus: AgentWebAppStatusActive,
		}},
		enterpriseService: &stubWebAppStatusOrganizationService{
			allowed:        true,
			organizationID: "88888888-8888-8888-8888-888888888888",
		},
	}

	resp, err := service.GetAgentRuntimeSurfaces(ctx, agentID.String(), "99999999-9999-9999-9999-999999999999")
	require.NoError(t, err)
	require.Equal(t, currentWorkspaceID.String(), resp.WorkspaceID)
	require.Equal(t, "88888888-8888-8888-8888-888888888888", resp.OrganizationID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentsService_GetAgentRuntimeSurfaces_RejectsMissingWorkspaceViewPermission(t *testing.T) {
	ctx := webAppStatusTestContext()
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:       agentID,
			TenantID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		},
	}
	service := &agentsService{
		agentsRepo:        repo,
		enterpriseService: &stubWebAppStatusOrganizationService{allowed: false},
	}

	_, err := service.GetAgentRuntimeSurfaces(ctx, agentID.String(), "99999999-9999-9999-9999-999999999999")
	require.ErrorIs(t, err, runtimeservice.ErrPermissionDenied)
}

func TestAgentsService_UpdateAgentRuntimeSurfaces_RejectsInternalDisable(t *testing.T) {
	db, cleanup := openAgentRuntimeSurfacesMockDB(t)
	defer cleanup()

	ctx := webAppStatusTestContext()
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:       agentID,
			TenantID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		},
	}
	service := &agentsService{
		agentsRepo:        repo,
		accountService:    &stubWebAppStatusAccountService{isEditor: true},
		enterpriseService: &stubWebAppStatusOrganizationService{allowed: true, organizationID: "88888888-8888-8888-8888-888888888888"},
		db:                db,
	}

	_, err := service.UpdateAgentRuntimeSurfaces(ctx, agentID.String(), "99999999-9999-9999-9999-999999999999", dto.UpdateAgentRuntimeSurfacesRequest{
		Surfaces: []dto.UpdateAgentRuntimeSurfaceAuthorization{{
			Surface: "internal",
			Enabled: false,
		}},
	})
	require.ErrorIs(t, err, runtimeservice.ErrInvalidInput)
	require.Contains(t, err.Error(), "internal runtime surface cannot be disabled")
}

func TestAgentsService_UpdateAgentRuntimeSurfaces_RequiresRuntimeAccessPermission(t *testing.T) {
	db, cleanup := openAgentRuntimeSurfacesMockDB(t)
	defer cleanup()

	ctx := webAppStatusTestContext()
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:       agentID,
			TenantID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		},
	}
	orgService := &stubWebAppStatusOrganizationService{
		organizationID: "88888888-8888-8888-8888-888888888888",
		allowedPermissions: map[workspace_model.WorkspacePermissionCode]bool{
			workspace_model.WorkspacePermissionAgentUpdate: true,
		},
	}
	service := &agentsService{
		agentsRepo:        repo,
		accountService:    &stubWebAppStatusAccountService{isEditor: true},
		enterpriseService: orgService,
		db:                db,
	}

	_, err := service.UpdateAgentRuntimeSurfaces(ctx, agentID.String(), "99999999-9999-9999-9999-999999999999", dto.UpdateAgentRuntimeSurfacesRequest{
		Surfaces: []dto.UpdateAgentRuntimeSurfaceAuthorization{{
			Surface: "webapp",
			Enabled: true,
		}},
	})

	require.ErrorIs(t, err, runtimeservice.ErrPermissionDenied)
	require.Equal(t, agentRuntimeAccessManagePermissionCodes("AGENT"), orgService.lastPermissions)
}

func TestAgentsService_UpdateAgentRuntimeSurfaces_AllowsRuntimeAccessWithoutRuntimeConfigPermission(t *testing.T) {
	db, cleanup := openAgentRuntimeSurfacesMockDB(t)
	defer cleanup()

	ctx := webAppStatusTestContext()
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:       agentID,
			TenantID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		},
	}
	orgService := &stubWebAppStatusOrganizationService{
		organizationID: "88888888-8888-8888-8888-888888888888",
		allowedPermissions: map[workspace_model.WorkspacePermissionCode]bool{
			workspace_model.WorkspacePermissionAgentRuntimeAccessManage: true,
		},
	}
	service := &agentsService{
		agentsRepo:        repo,
		accountService:    &stubWebAppStatusAccountService{isEditor: true},
		enterpriseService: orgService,
		db:                db,
	}

	_, err := service.UpdateAgentRuntimeSurfaces(ctx, agentID.String(), "99999999-9999-9999-9999-999999999999", dto.UpdateAgentRuntimeSurfacesRequest{
		Surfaces: []dto.UpdateAgentRuntimeSurfaceAuthorization{{
			Surface: "internal",
			Enabled: false,
		}},
	})

	require.ErrorIs(t, err, runtimeservice.ErrInvalidInput)
	require.Equal(t, agentRuntimeAccessManagePermissionCodes("AGENT"), orgService.lastPermissions)
}

func TestAgentsService_UpdateAgentRuntimeSurfaces_IgnoresLegacyBuiltinSurfaceInput(t *testing.T) {
	db, cleanup := openAgentRuntimeSurfacesMockDB(t)
	defer cleanup()

	ctx := webAppStatusTestContext()
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:       agentID,
			TenantID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		},
	}
	service := &agentsService{
		agentsRepo:        repo,
		accountService:    &stubWebAppStatusAccountService{isEditor: true},
		enterpriseService: &stubWebAppStatusOrganizationService{allowed: true, organizationID: "88888888-8888-8888-8888-888888888888"},
		db:                db,
	}

	_, err := service.UpdateAgentRuntimeSurfaces(ctx, agentID.String(), "99999999-9999-9999-9999-999999999999", dto.UpdateAgentRuntimeSurfacesRequest{
		Surfaces: []dto.UpdateAgentRuntimeSurfaceAuthorization{{
			Surface: "builtin_app",
			Enabled: true,
		}},
	})
	require.ErrorIs(t, err, runtimeservice.ErrInvalidInput)
	require.Contains(t, err.Error(), "no supported agent runtime surface provided")
}

func TestAgentsService_UpdateAgentRuntimeSurfaces_RejectsNonPublicAPIGrantsBeforeSQL(t *testing.T) {
	ctx := webAppStatusTestContext()
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	workspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	departmentID := "99999999-9999-9999-9999-999999999997"

	tests := []struct {
		name      string
		surface   string
		subject   string
		subjectID *string
		want      string
	}{
		{
			name:      "api rejects department grant",
			surface:   "api",
			subject:   "department",
			subjectID: &departmentID,
			want:      "api runtime grants must use public subject",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, cleanup := openAgentRuntimeSurfacesMockDBWithMock(t)
			defer cleanup()

			repo := &stubWebAppStatusRepository{
				agent: &Agent{
					ID:       agentID,
					TenantID: workspaceID,
				},
			}
			service := &agentsService{
				agentsRepo:        repo,
				accountService:    &stubWebAppStatusAccountService{isEditor: true},
				enterpriseService: &stubWebAppStatusOrganizationService{allowed: true, organizationID: "88888888-8888-8888-8888-888888888888"},
				db:                db,
			}

			_, err := service.UpdateAgentRuntimeSurfaces(ctx, agentID.String(), "99999999-9999-9999-9999-999999999999", dto.UpdateAgentRuntimeSurfacesRequest{
				Surfaces: []dto.UpdateAgentRuntimeSurfaceAuthorization{{
					Surface: tt.surface,
					Enabled: true,
					Grants: []dto.UpdateAgentRuntimeSurfaceGrant{{
						SubjectType: tt.subject,
						SubjectID:   tt.subjectID,
					}},
				}},
			})

			require.ErrorIs(t, err, runtimeservice.ErrInvalidInput)
			require.Contains(t, err.Error(), tt.want)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAgentsService_UpdateAgentRuntimeSurfaces_RejectsWebAppAccountGrantOutsideOrganization(t *testing.T) {
	db, mock, cleanup := openAgentRuntimeSurfacesMockDBWithMock(t)
	defer cleanup()

	ctx := webAppStatusTestContext()
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	workspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	organizationID := uuid.MustParse("88888888-8888-8888-8888-888888888888")
	accountGrantID := uuid.MustParse("99999999-9999-9999-9999-999999999998")
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:       agentID,
			TenantID: workspaceID,
		},
	}
	service := &agentsService{
		agentsRepo:        repo,
		accountService:    &stubWebAppStatusAccountService{isEditor: true},
		enterpriseService: &stubWebAppStatusOrganizationService{allowed: true, organizationID: organizationID.String()},
		db:                db,
	}
	mock.ExpectQuery(`SELECT count\(\*\) FROM "members" WHERE organization_id = \$1 AND account_id = \$2 AND status = \$3`).
		WithArgs(organizationID.String(), accountGrantID.String(), workspace_model.OrganizationMemberStatusActive).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	_, err := service.UpdateAgentRuntimeSurfaces(ctx, agentID.String(), "99999999-9999-9999-9999-999999999999", dto.UpdateAgentRuntimeSurfacesRequest{
		Surfaces: []dto.UpdateAgentRuntimeSurfaceAuthorization{{
			Surface: "webapp",
			Enabled: true,
			Grants: []dto.UpdateAgentRuntimeSurfaceGrant{{
				SubjectType: "account",
				SubjectID:   stringPtr(accountGrantID.String()),
			}},
		}},
	})

	require.ErrorIs(t, err, runtimeservice.ErrInvalidInput)
	require.Contains(t, err.Error(), "runtime grant account is not in organization")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentsService_UpdateAgentRuntimeSurfaces_RejectsOrganizationGrantForNormalMember(t *testing.T) {
	db, mock, cleanup := openAgentRuntimeSurfacesMockDBWithMock(t)
	defer cleanup()

	ctx := webAppStatusTestContext()
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	workspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	organizationID := uuid.MustParse("88888888-8888-8888-8888-888888888888")
	operatorID := "99999999-9999-9999-9999-999999999999"
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:       agentID,
			TenantID: workspaceID,
		},
	}
	service := &agentsService{
		agentsRepo:        repo,
		accountService:    &stubWebAppStatusAccountService{isEditor: true},
		enterpriseService: &stubWebAppStatusOrganizationService{allowed: true, organizationID: organizationID.String()},
		db:                db,
	}
	mock.ExpectQuery(`SELECT \* FROM "members" WHERE organization_id = \$1 AND account_id = \$2 AND status = \$3 LIMIT \$4`).
		WithArgs(organizationID.String(), operatorID, workspace_model.OrganizationMemberStatusActive, 1).
		WillReturnRows(sqlmock.NewRows([]string{"organization_id", "account_id", "role", "name", "status", "created_at", "updated_at"}).
			AddRow(organizationID.String(), operatorID, workspace_model.OrganizationRoleNormal, nil, workspace_model.OrganizationMemberStatusActive, time.Now(), time.Now()))

	_, err := service.UpdateAgentRuntimeSurfaces(ctx, agentID.String(), operatorID, dto.UpdateAgentRuntimeSurfacesRequest{
		Surfaces: []dto.UpdateAgentRuntimeSurfaceAuthorization{{
			Surface: "webapp",
			Enabled: true,
			Grants: []dto.UpdateAgentRuntimeSurfaceGrant{{
				SubjectType: "organization",
			}},
		}},
	})

	require.ErrorIs(t, err, runtimeservice.ErrPermissionDenied)
	require.Contains(t, err.Error(), "only organization owners or admins can grant organization-wide access")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentsService_UpdateAgentRuntimeSurfaces_RejectsDepartmentGrantOutsideOrganization(t *testing.T) {
	db, mock, cleanup := openAgentRuntimeSurfacesMockDBWithMock(t)
	defer cleanup()

	ctx := webAppStatusTestContext()
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	workspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	organizationID := uuid.MustParse("88888888-8888-8888-8888-888888888888")
	departmentGrantID := uuid.MustParse("99999999-9999-9999-9999-999999999997")
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:       agentID,
			TenantID: workspaceID,
		},
	}
	service := &agentsService{
		agentsRepo:        repo,
		accountService:    &stubWebAppStatusAccountService{isEditor: true},
		enterpriseService: &stubWebAppStatusOrganizationService{allowed: true, organizationID: organizationID.String()},
		db:                db,
	}
	mock.ExpectQuery(`SELECT count\(\*\) FROM "departments" WHERE group_id = \$1 AND id = \$2 AND status = \$3`).
		WithArgs(organizationID.String(), departmentGrantID.String(), workspace_model.DepartmentStatusActive).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	_, err := service.UpdateAgentRuntimeSurfaces(ctx, agentID.String(), "99999999-9999-9999-9999-999999999999", dto.UpdateAgentRuntimeSurfacesRequest{
		Surfaces: []dto.UpdateAgentRuntimeSurfaceAuthorization{{
			Surface: "webapp",
			Enabled: true,
			Grants: []dto.UpdateAgentRuntimeSurfaceGrant{{
				SubjectType: "department",
				SubjectID:   stringPtr(departmentGrantID.String()),
			}},
		}},
	})

	require.ErrorIs(t, err, runtimeservice.ErrInvalidInput)
	require.Contains(t, err.Error(), "runtime grant department is not in organization")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentRuntimeAuthorizationFromUpdateRequest_AllowsWebAppAudienceGrants(t *testing.T) {
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	workspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	organizationID := uuid.MustParse("88888888-8888-8888-8888-888888888888")
	accountID := "99999999-9999-9999-9999-999999999999"
	departmentID := "99999999-9999-9999-9999-999999999998"

	auth, _, err := agentRuntimeAuthorizationFromUpdateRequest(agentID, workspaceID, organizationID, dto.UpdateAgentRuntimeSurfacesRequest{
		Surfaces: []dto.UpdateAgentRuntimeSurfaceAuthorization{{
			Surface: "webapp",
			Enabled: true,
			Grants: []dto.UpdateAgentRuntimeSurfaceGrant{
				{SubjectType: "organization"},
				{SubjectType: "account", SubjectID: &accountID},
				{SubjectType: "department", SubjectID: &departmentID},
			},
		}},
	})
	require.NoError(t, err)
	require.Len(t, auth.Surfaces, 1)
	grants := auth.Surfaces[0].Grants
	require.Len(t, grants, 3)
	require.Equal(t, runtimeauth.PublishedRuntimeSubjectOrganization, grants[0].SubjectType)
	require.NotNil(t, grants[0].SubjectID)
	require.Equal(t, organizationID, *grants[0].SubjectID)
	require.Equal(t, runtimeauth.PublishedRuntimeSubjectAccount, grants[1].SubjectType)
	require.Equal(t, accountID, grants[1].SubjectID.String())
	require.Equal(t, runtimeauth.PublishedRuntimeSubjectDepartment, grants[2].SubjectType)
	require.Equal(t, departmentID, grants[2].SubjectID.String())
}

func TestAgentRuntimeAuthorizationFromUpdateRequest_DefaultsEnabledWebAppToPublicGrant(t *testing.T) {
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	workspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	organizationID := uuid.MustParse("88888888-8888-8888-8888-888888888888")

	auth, _, err := agentRuntimeAuthorizationFromUpdateRequest(agentID, workspaceID, organizationID, dto.UpdateAgentRuntimeSurfacesRequest{
		Surfaces: []dto.UpdateAgentRuntimeSurfaceAuthorization{{
			Surface: "webapp",
			Enabled: true,
		}},
	})
	require.NoError(t, err)
	require.Len(t, auth.Surfaces, 1)
	require.Len(t, auth.Surfaces[0].Grants, 1)
	grant := auth.Surfaces[0].Grants[0]
	require.Equal(t, runtimeauth.PublishedRuntimeSubjectPublic, grant.SubjectType)
	require.Nil(t, grant.SubjectID)
	require.True(t, grant.Enabled)
}

func TestAgentRuntimeAuthorizationFromUpdateRequest_DefaultsEnabledAppCenterToOwningWorkspaceGrant(t *testing.T) {
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	workspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	organizationID := uuid.MustParse("88888888-8888-8888-8888-888888888888")

	auth, _, err := agentRuntimeAuthorizationFromUpdateRequest(agentID, workspaceID, organizationID, dto.UpdateAgentRuntimeSurfacesRequest{
		Surfaces: []dto.UpdateAgentRuntimeSurfaceAuthorization{{
			Surface: "app_center",
			Enabled: true,
		}},
	})
	require.NoError(t, err)
	require.Len(t, auth.Surfaces, 1)
	require.Len(t, auth.Surfaces[0].Grants, 1)
	grant := auth.Surfaces[0].Grants[0]
	require.Equal(t, runtimeauth.PublishedRuntimeSubjectWorkspace, grant.SubjectType)
	require.NotNil(t, grant.SubjectID)
	require.Equal(t, workspaceID, *grant.SubjectID)
	require.True(t, grant.Enabled)
}

func TestAgentRuntimeAuthorizationFromUpdateRequest_RejectsInvalidSurfaceGrantSubjects(t *testing.T) {
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	workspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	organizationID := uuid.MustParse("88888888-8888-8888-8888-888888888888")

	tests := []struct {
		name      string
		surface   string
		subject   string
		subjectID *string
		want      string
	}{
		{
			name:      "webapp rejects internal grant",
			surface:   "webapp",
			subject:   "internal",
			subjectID: nil,
			want:      "webapp runtime grants must target public, organization, account, department, or workspace",
		},
		{
			name:    "api rejects organization grant",
			surface: "api",
			subject: "organization",
			want:    "api runtime grants must use public subject",
		},
		{
			name:    "app center rejects public grant",
			surface: "app_center",
			subject: "public",
			want:    "app center grants must target organization, account, department, or workspace",
		},
		{
			name:      "builtin surface is ignored as unsupported for ordinary agent",
			surface:   "builtin_app",
			subject:   "public",
			subjectID: nil,
			want:      "no supported agent runtime surface provided",
		},
		{
			name:    "internal rejects public grant",
			surface: "internal",
			subject: "public",
			want:    "internal runtime grants must use internal subject",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := agentRuntimeAuthorizationFromUpdateRequest(agentID, workspaceID, organizationID, dto.UpdateAgentRuntimeSurfacesRequest{
				Surfaces: []dto.UpdateAgentRuntimeSurfaceAuthorization{{
					Surface: tt.surface,
					Enabled: true,
					Grants: []dto.UpdateAgentRuntimeSurfaceGrant{{
						SubjectType: tt.subject,
						SubjectID:   tt.subjectID,
					}},
				}},
			})

			require.ErrorIs(t, err, runtimeservice.ErrInvalidInput)
			require.Contains(t, err.Error(), tt.want)
		})
	}
}

func webAppStatusTestContext() context.Context {
	ctx := context.WithValue(context.Background(), "account_id", "99999999-9999-9999-9999-999999999999")
	return context.WithValue(ctx, "tenant_id", "88888888-8888-8888-8888-888888888888")
}

func openAgentRuntimeSurfacesMockDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()

	db, _, cleanup := openAgentRuntimeSurfacesMockDBWithMock(t)
	return db, cleanup
}

func openAgentRuntimeSurfacesMockDBWithMock(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("open gorm: %v", err)
	}
	return db, mock, func() {
		_ = sqlDB.Close()
	}
}

type agentRuntimeSurfaceExpectation struct {
	surface string
	enabled bool
}

func expectAgentRuntimeSurfaceRows(mock sqlmock.Sqlmock, agentID, workspaceID uuid.UUID, surfaces []agentRuntimeSurfaceExpectation) {
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
			uuid.New().String(),
			"agent",
			agentID.String(),
			uuid.New().String(),
			workspaceID.String(),
			surface.surface,
			surface.enabled,
			"grant",
			now,
			now,
			nil,
		)
	}
	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surfaces" WHERE resource_type = \$1 AND resource_id = \$2 AND deleted_at IS NULL ORDER BY surface ASC`).
		WithArgs("agent", agentID).
		WillReturnRows(rows)
	if len(surfaces) == 0 {
		return
	}
	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surface_grants" WHERE surface_id IN \(.+\) AND deleted_at IS NULL ORDER BY subject_type ASC, subject_id ASC, created_at ASC`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"surface_id",
			"subject_type",
			"subject_id",
			"enabled",
			"created_at",
			"updated_at",
			"deleted_at",
		}))
}

func expectAgentRuntimeSurfaceRowsWithGrant(mock sqlmock.Sqlmock, agentID, workspaceID uuid.UUID, surfaceName string, enabled bool, subjectType string, subjectID uuid.UUID) {
	expectAgentRuntimeSurfaceRowsWithGrantAndOrganization(mock, agentID, uuid.New(), workspaceID, surfaceName, enabled, subjectType, subjectID)
}

func expectAgentRuntimeSurfaceRowsWithGrantAndOrganization(mock sqlmock.Sqlmock, agentID, organizationID, workspaceID uuid.UUID, surfaceName string, enabled bool, subjectType string, subjectID uuid.UUID) {
	surfaceID := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)
	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surfaces" WHERE resource_type = \$1 AND resource_id = \$2 AND deleted_at IS NULL ORDER BY surface ASC`).
		WithArgs("agent", agentID).
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
		}).AddRow(
			surfaceID.String(),
			"agent",
			agentID.String(),
			organizationID.String(),
			workspaceID.String(),
			surfaceName,
			enabled,
			"grant",
			now,
			now,
			nil,
		))
	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surface_grants" WHERE surface_id IN \(.+\) AND deleted_at IS NULL ORDER BY subject_type ASC, subject_id ASC, created_at ASC`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"surface_id",
			"subject_type",
			"subject_id",
			"enabled",
			"created_at",
			"updated_at",
			"deleted_at",
		}).AddRow(
			uuid.NewString(),
			surfaceID.String(),
			subjectType,
			subjectID.String(),
			true,
			now,
			now,
			nil,
		))
}

func expectWebAppRuntimeAudience(mock sqlmock.Sqlmock, accountID, organizationID uuid.UUID, departmentIDs []uuid.UUID, memberCount int64) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "members" WHERE organization_id = $1 AND account_id = $2 AND status = $3`)).
		WithArgs(organizationID.String(), accountID.String(), workspace_model.OrganizationMemberStatusActive).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(memberCount))
	if memberCount == 0 {
		return
	}
	rows := sqlmock.NewRows([]string{"department_id"})
	for _, departmentID := range departmentIDs {
		rows.AddRow(departmentID.String())
	}
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT department_members.department_id FROM "department_members" JOIN departments ON departments.id = department_members.department_id WHERE department_members.account_id = $1 AND departments.group_id = $2 AND departments.status = $3`)).
		WithArgs(accountID, organizationID, workspace_model.DepartmentStatusActive).
		WillReturnRows(rows)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT workspace_members.workspace_id FROM "workspace_members" JOIN workspaces ON workspaces.id = workspace_members.workspace_id WHERE workspace_members.account_id = $1 AND workspaces.organization_id = $2 AND workspaces.status = $3`)).
		WithArgs(accountID, organizationID, workspace_model.WorkspaceStatusNormal).
		WillReturnRows(sqlmock.NewRows([]string{"workspace_id"}))
}

func runtimeSurfaceTestMap(surfaces []dto.AgentRuntimeSurfaceAuthorization) map[string]dto.AgentRuntimeSurfaceAuthorization {
	out := make(map[string]dto.AgentRuntimeSurfaceAuthorization, len(surfaces))
	for _, surface := range surfaces {
		out[surface.Surface] = surface
	}
	return out
}

type stubWebAppStatusRepository struct {
	AgentsRepository

	agent              *Agent
	updateCalled       bool
	updateAgentCalled  bool
	createConfigCalled bool
	lastStatus         AgentWebAppStatus
	lastReason         string
	lastUpdatedBy      string
	config             *AgentsConfig
	latestVersion      *AgentPublishedVersion
}

func (s *stubWebAppStatusRepository) GetByID(_ context.Context, id string) (*Agent, error) {
	if s.agent == nil || s.agent.ID.String() != id {
		return nil, errors.New("agent not found")
	}
	return s.agent, nil
}

func (s *stubWebAppStatusRepository) GetByWebAppID(_ context.Context, webAppID string) (*Agent, error) {
	if s.agent == nil || s.agent.WebAppID.String() != webAppID {
		return nil, errors.New("agent not found")
	}
	return s.agent, nil
}

func (s *stubWebAppStatusRepository) Update(_ context.Context, ag *Agent) error {
	s.updateAgentCalled = true
	s.agent = ag
	return nil
}

func (s *stubWebAppStatusRepository) UpdateWebAppStatus(_ context.Context, _ string, status AgentWebAppStatus, reason string, updatedBy string) error {
	s.updateCalled = true
	s.lastStatus = status
	s.lastReason = reason
	s.lastUpdatedBy = updatedBy
	s.agent.WebAppStatus = status
	s.agent.WebAppOfflineReason = reason
	s.agent.UpdatedAt = time.Now()
	return nil
}

func (s *stubWebAppStatusRepository) CreateAgentsConfig(_ context.Context, cfg *AgentsConfig) error {
	s.createConfigCalled = true
	cfg.ID = uuid.New()
	s.config = cfg
	return nil
}

func (s *stubWebAppStatusRepository) GetAgentsConfigByID(_ context.Context, id string) (*AgentsConfig, error) {
	if s.config == nil || s.config.ID.String() != id {
		return nil, nil
	}
	return s.config, nil
}

func (s *stubWebAppStatusRepository) GetAgentsConfigByAgentID(_ context.Context, agentID string) (*AgentsConfig, error) {
	if s.config == nil || s.config.AgentsID.String() != agentID {
		return nil, nil
	}
	return s.config, nil
}

func (s *stubWebAppStatusRepository) GetLatestAgentPublishedVersion(context.Context, string) (*AgentPublishedVersion, error) {
	return s.latestVersion, nil
}

type stubWebAppStatusAccountService struct {
	interfaces.AccountService

	isEditor bool
	err      error
}

func (s *stubWebAppStatusAccountService) IsEditor(_ context.Context, _ string) (bool, error) {
	return s.isEditor, s.err
}

type stubWebAppStatusOrganizationService struct {
	interfaces.OrganizationService

	allowed            bool
	allowedPermissions map[workspace_model.WorkspacePermissionCode]bool
	err                error
	checkCalled        bool
	lastPermission     workspace_model.WorkspacePermissionCode
	lastPermissions    []workspace_model.WorkspacePermissionCode
	organizationID     string
}

func (s *stubWebAppStatusOrganizationService) CheckWorkspacePermission(_ context.Context, _, _, _ string, permission workspace_model.WorkspacePermissionCode) (bool, error) {
	s.checkCalled = true
	s.lastPermission = permission
	s.lastPermissions = []workspace_model.WorkspacePermissionCode{permission}
	return s.allows(permission), s.err
}

func (s *stubWebAppStatusOrganizationService) CheckWorkspaceOrganizationAnyPermission(_ context.Context, _, _, _ string, permissions ...workspace_model.WorkspacePermissionCode) (bool, error) {
	s.checkCalled = true
	s.lastPermissions = append([]workspace_model.WorkspacePermissionCode(nil), permissions...)
	if len(permissions) > 0 {
		s.lastPermission = permissions[0]
	}
	return s.allows(permissions...), s.err
}

func (s *stubWebAppStatusOrganizationService) allows(permissions ...workspace_model.WorkspacePermissionCode) bool {
	if len(s.allowedPermissions) == 0 {
		return s.allowed
	}
	for _, permission := range permissions {
		if s.allowedPermissions[permission] {
			return true
		}
	}
	return false
}

func (s *stubWebAppStatusOrganizationService) GetOrganizationByWorkspaceID(_ context.Context, workspaceID string) (*workspace_model.Organization, error) {
	organizationID := strings.TrimSpace(s.organizationID)
	if organizationID == "" {
		organizationID = workspaceID
	}
	return &workspace_model.Organization{ID: organizationID, Status: workspace_model.OrganizationStatusActive}, nil
}
