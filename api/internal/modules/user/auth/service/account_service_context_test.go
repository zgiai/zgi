package service

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	shared_dto "github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	auth_repo "github.com/zgiai/zgi/api/internal/modules/user/auth/repository"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestUpdateAccountContextSwitchOrganizationLeavesWorkspaceEmpty(t *testing.T) {
	now := time.Now().UTC()
	oldOrganizationID := "org-old"
	oldWorkspaceID := "ws-old"
	newOrganizationID := "org-new"

	repo := &fakeAccountContextRepository{
		ctxModel: &auth_model.AccountContext{
			AccountID:             "acc-1",
			CurrentOrganizationID: &oldOrganizationID,
			CurrentWorkspaceID:    &oldWorkspaceID,
			CreatedAt:             now,
			UpdatedAt:             now,
		},
	}
	organizationService := &fakeOrganizationContextService{
		members: map[string]bool{newOrganizationID: true},
		admins:  map[string]bool{newOrganizationID: false},
	}

	svc := &AccountService{
		accountRepo:                repo,
		workspaceManagementService: &fakeWorkspaceContextService{},
		organizationService:        organizationService,
	}

	ctxModel, err := svc.UpdateAccountContext(context.Background(), "acc-1", &newOrganizationID, nil)
	require.NoError(t, err)
	require.NotNil(t, ctxModel.CurrentOrganizationID)
	require.Equal(t, newOrganizationID, *ctxModel.CurrentOrganizationID)
	require.Nil(t, ctxModel.CurrentWorkspaceID)
	require.NotNil(t, repo.updated)
	require.Nil(t, repo.updated.CurrentWorkspaceID)
}

func TestUpdateAccountContextSwitchOrganizationWithoutWorkspaceLeavesWorkspaceEmpty(t *testing.T) {
	now := time.Now().UTC()
	oldOrganizationID := "org-old"
	oldWorkspaceID := "ws-old"
	newOrganizationID := "org-new"

	repo := &fakeAccountContextRepository{
		ctxModel: &auth_model.AccountContext{
			AccountID:             "acc-1",
			CurrentOrganizationID: &oldOrganizationID,
			CurrentWorkspaceID:    &oldWorkspaceID,
			CreatedAt:             now,
			UpdatedAt:             now,
		},
	}
	workspaceService := &fakeWorkspaceContextService{
		workspaces: map[string]*workspace_model.Workspace{},
	}
	organizationService := &fakeOrganizationContextService{
		members: map[string]bool{newOrganizationID: true},
		admins:  map[string]bool{newOrganizationID: false},
	}

	svc := &AccountService{
		accountRepo:                repo,
		workspaceManagementService: workspaceService,
		organizationService:        organizationService,
	}

	ctxModel, err := svc.UpdateAccountContext(context.Background(), "acc-1", &newOrganizationID, nil)
	require.NoError(t, err)
	require.NotNil(t, ctxModel.CurrentOrganizationID)
	require.Equal(t, newOrganizationID, *ctxModel.CurrentOrganizationID)
	require.Nil(t, ctxModel.CurrentWorkspaceID)
	require.NotNil(t, repo.updated)
	require.Nil(t, repo.updated.CurrentWorkspaceID)
}

func TestUpdateAccountContextClearsWorkspaceWithinCurrentOrganization(t *testing.T) {
	now := time.Now().UTC()
	organizationID := "org-1"
	oldWorkspaceID := "ws-old"
	emptyWorkspaceID := ""

	repo := &fakeAccountContextRepository{
		ctxModel: &auth_model.AccountContext{
			AccountID:             "acc-1",
			CurrentOrganizationID: &organizationID,
			CurrentWorkspaceID:    &oldWorkspaceID,
			CreatedAt:             now,
			UpdatedAt:             now,
		},
	}
	organizationService := &fakeOrganizationContextService{
		members: map[string]bool{organizationID: true},
		admins:  map[string]bool{organizationID: false},
	}

	svc := &AccountService{
		accountRepo:                repo,
		workspaceManagementService: &fakeWorkspaceContextService{},
		organizationService:        organizationService,
	}

	ctxModel, err := svc.UpdateAccountContext(context.Background(), "acc-1", &organizationID, &emptyWorkspaceID)
	require.NoError(t, err)
	require.NotNil(t, ctxModel.CurrentOrganizationID)
	require.Equal(t, organizationID, *ctxModel.CurrentOrganizationID)
	require.Nil(t, ctxModel.CurrentWorkspaceID)
	require.NotNil(t, repo.updated)
	require.NotNil(t, repo.updated.CurrentOrganizationID)
	require.Equal(t, organizationID, *repo.updated.CurrentOrganizationID)
	require.Nil(t, repo.updated.CurrentWorkspaceID)
}

func TestUpdateAccountContextWorkspaceSelectionBackfillsOrganization(t *testing.T) {
	organizationID := "org-1"
	workspaceID := "ws-1"
	repo := &fakeAccountContextRepository{
		ctxModel: &auth_model.AccountContext{AccountID: "acc-1"},
	}
	workspaceService := &fakeWorkspaceContextService{
		workspaces: map[string]*workspace_model.Workspace{
			workspaceID: {
				ID:             workspaceID,
				Name:           "Workspace",
				Status:         workspace_model.WorkspaceStatusNormal,
				OrganizationID: &organizationID,
			},
		},
		joins: map[string]bool{
			workspaceID + ":acc-1": true,
		},
	}
	organizationService := &fakeOrganizationContextService{
		members: map[string]bool{organizationID: true},
		admins:  map[string]bool{organizationID: false},
	}

	svc := &AccountService{
		accountRepo:                repo,
		workspaceManagementService: workspaceService,
		organizationService:        organizationService,
	}

	ctxModel, err := svc.UpdateAccountContext(context.Background(), "acc-1", nil, &workspaceID)
	require.NoError(t, err)
	require.NotNil(t, ctxModel.CurrentOrganizationID)
	require.Equal(t, organizationID, *ctxModel.CurrentOrganizationID)
	require.NotNil(t, ctxModel.CurrentWorkspaceID)
	require.Equal(t, workspaceID, *ctxModel.CurrentWorkspaceID)
}

func TestGetAccountContextPreservesMissingWorkspaceInCurrentOrganization(t *testing.T) {
	organizationID := "org-1"
	repo := &fakeAccountContextRepository{
		ctxModel: &auth_model.AccountContext{
			AccountID:             "acc-1",
			CurrentOrganizationID: &organizationID,
		},
	}
	organizationService := &fakeOrganizationContextService{
		members: map[string]bool{organizationID: true},
		admins:  map[string]bool{organizationID: false},
	}

	svc := &AccountService{
		accountRepo:                repo,
		workspaceManagementService: &fakeWorkspaceContextService{},
		organizationService:        organizationService,
	}

	ctxModel, err := svc.GetAccountContext(context.Background(), "acc-1")
	require.NoError(t, err)
	require.NotNil(t, ctxModel.CurrentOrganizationID)
	require.Equal(t, organizationID, *ctxModel.CurrentOrganizationID)
	require.Nil(t, ctxModel.CurrentWorkspaceID)
	require.Nil(t, repo.updated)
}

func TestGetAccountContextBackfillsOrganizationFromCurrentWorkspace(t *testing.T) {
	organizationID := "org-1"
	workspaceID := "ws-1"
	repo := &fakeAccountContextRepository{
		ctxModel: &auth_model.AccountContext{
			AccountID:          "acc-1",
			CurrentWorkspaceID: &workspaceID,
		},
	}
	workspaceService := &fakeWorkspaceContextService{
		workspaces: map[string]*workspace_model.Workspace{
			workspaceID: {
				ID:             workspaceID,
				Name:           "Workspace",
				Status:         workspace_model.WorkspaceStatusNormal,
				OrganizationID: &organizationID,
			},
		},
		joins: map[string]bool{
			workspaceID + ":acc-1": true,
		},
	}
	organizationService := &fakeOrganizationContextService{
		members: map[string]bool{organizationID: true},
		admins:  map[string]bool{organizationID: false},
	}

	svc := &AccountService{
		accountRepo:                repo,
		workspaceManagementService: workspaceService,
		organizationService:        organizationService,
	}

	ctxModel, err := svc.GetAccountContext(context.Background(), "acc-1")
	require.NoError(t, err)
	require.NotNil(t, ctxModel.CurrentOrganizationID)
	require.Equal(t, organizationID, *ctxModel.CurrentOrganizationID)
	require.NotNil(t, ctxModel.CurrentWorkspaceID)
	require.Equal(t, workspaceID, *ctxModel.CurrentWorkspaceID)
	require.NotNil(t, repo.updated)
	require.Equal(t, organizationID, *repo.updated.CurrentOrganizationID)
}

func TestGetAccountContextPopulatesDefaultOrganizationWithoutWorkspace(t *testing.T) {
	organizationID := "org-1"
	repo := &fakeAccountContextRepository{
		ctxModel: &auth_model.AccountContext{
			AccountID: "acc-1",
		},
	}
	organizationService := &fakeOrganizationContextService{
		members: map[string]bool{organizationID: true},
		admins:  map[string]bool{organizationID: false},
		firstJoined: &workspace_model.Organization{
			ID:     organizationID,
			Name:   "Organization",
			Status: workspace_model.OrganizationStatusActive,
		},
	}

	svc := &AccountService{
		accountRepo:                repo,
		workspaceManagementService: &fakeWorkspaceContextService{},
		organizationService:        organizationService,
	}

	ctxModel, err := svc.GetAccountContext(context.Background(), "acc-1")
	require.NoError(t, err)
	require.NotNil(t, ctxModel.CurrentOrganizationID)
	require.Equal(t, organizationID, *ctxModel.CurrentOrganizationID)
	require.Nil(t, ctxModel.CurrentWorkspaceID)
	require.NotNil(t, repo.updated)
	require.Equal(t, organizationID, *repo.updated.CurrentOrganizationID)
	require.Nil(t, repo.updated.CurrentWorkspaceID)
}

func TestGetAccountContextLeavesWorkspaceEmptyWhenNoneAccessible(t *testing.T) {
	repo := &fakeAccountContextRepository{
		ctxModel: &auth_model.AccountContext{
			AccountID: "acc-1",
		},
	}

	svc := &AccountService{
		accountRepo:                repo,
		workspaceManagementService: &fakeWorkspaceContextService{},
		organizationService:        &fakeOrganizationContextService{},
	}

	ctxModel, err := svc.GetAccountContext(context.Background(), "acc-1")
	require.NoError(t, err)
	require.Nil(t, ctxModel.CurrentOrganizationID)
	require.Nil(t, ctxModel.CurrentWorkspaceID)
	require.Nil(t, repo.updated)
}

func TestEnsureCurrentOrganizationIDPreservesOrganizationModeWithoutWorkspace(t *testing.T) {
	organizationID := "org-1"
	workspaceID := "ws-1"
	repo := &fakeAccountContextRepository{
		ctxModel: &auth_model.AccountContext{
			AccountID:             "acc-1",
			CurrentOrganizationID: &organizationID,
		},
	}
	workspaceService := &fakeWorkspaceContextService{
		workspaces: map[string]*workspace_model.Workspace{
			workspaceID: {
				ID:             workspaceID,
				Name:           "Workspace",
				Status:         workspace_model.WorkspaceStatusNormal,
				OrganizationID: &organizationID,
			},
		},
		joins: map[string]bool{
			workspaceID + ":acc-1": true,
		},
	}
	organizationService := &fakeOrganizationContextService{
		members: map[string]bool{organizationID: true},
	}

	svc := &AccountService{
		accountRepo:                repo,
		workspaceManagementService: workspaceService,
		organizationService:        organizationService,
	}

	resolvedOrganizationID, err := svc.EnsureCurrentOrganizationID(context.Background(), "acc-1")
	require.NoError(t, err)
	require.Equal(t, organizationID, resolvedOrganizationID)
	require.Nil(t, repo.ctxModel.CurrentWorkspaceID)
	require.Nil(t, repo.updated)
}

func TestGetAccountProfileReturnsOrganizationContextWithoutWorkspace(t *testing.T) {
	organizationID := "org-1"
	repo := &fakeAccountContextRepository{
		account: &auth_model.Account{
			ID:     "acc-1",
			Name:   "Account",
			Email:  "account@example.com",
			Status: auth_model.AccountStatusActive,
		},
		ctxModel: &auth_model.AccountContext{
			AccountID:             "acc-1",
			CurrentOrganizationID: &organizationID,
		},
	}
	organizationService := &fakeOrganizationContextService{
		members: map[string]bool{organizationID: true},
		roles: map[string]workspace_model.OrganizationRole{
			organizationID: workspace_model.OrganizationRoleNormal,
		},
	}

	svc := &AccountService{
		accountRepo:                repo,
		workspaceManagementService: &fakeWorkspaceContextService{},
		organizationService:        organizationService,
	}

	profile, err := svc.GetAccountProfile(context.Background(), "acc-1")
	require.NoError(t, err)
	require.Equal(t, "acc-1", profile.ID)
	require.NotNil(t, profile.CurrentOrganizationID)
	require.Equal(t, organizationID, *profile.CurrentOrganizationID)
	require.Nil(t, profile.CurrentWorkspaceID)
	require.Equal(t, string(workspace_model.OrganizationRoleNormal), profile.OrganizationRole)
	require.Equal(t, string(workspace_model.OrganizationRoleNormal), profile.GroupRole)
	require.Nil(t, repo.updated)
}

func TestGetAccountCapabilitiesOrganizationModeAllowsProductSurfacesOnly(t *testing.T) {
	organizationID := "org-1"
	repo := &fakeAccountContextRepository{
		ctxModel: &auth_model.AccountContext{
			AccountID:             "acc-1",
			CurrentOrganizationID: &organizationID,
		},
	}
	organizationService := &fakeOrganizationContextService{
		members: map[string]bool{organizationID: true},
		roles: map[string]workspace_model.OrganizationRole{
			organizationID: workspace_model.OrganizationRoleNormal,
		},
	}

	svc := &AccountService{
		accountRepo:                repo,
		workspaceManagementService: &fakeWorkspaceContextService{},
		organizationService:        organizationService,
	}

	capabilities, err := svc.GetAccountCapabilities(context.Background(), "acc-1")
	require.NoError(t, err)
	require.Equal(t, "organization", capabilities.Context.Mode)
	require.True(t, capabilities.Organization.IsMember)
	require.True(t, capabilities.Organization.ProductSurfaces.Chat)
	require.True(t, capabilities.Organization.ProductSurfaces.Image)
	require.True(t, capabilities.Organization.ProductSurfaces.App)
	require.True(t, capabilities.Organization.ProductSurfaces.Settings)
	require.False(t, capabilities.Organization.CanAccessDashboard)
	require.False(t, capabilities.Organization.CanManageModelConfig)
	require.True(t, capabilities.Routes.OrganizationScopeAllowed)
	require.False(t, capabilities.Routes.WorkspaceScopeAllowed)
	require.True(t, capabilities.Routes.WorkspaceRequired)
	require.False(t, capabilities.Workspace.Available)
	require.Empty(t, capabilities.Workspace.Permissions)
	require.Equal(t, "acc-1", capabilities.RuntimeAudience.AccountID)
	require.Equal(t, &organizationID, capabilities.RuntimeAudience.OrganizationID)
	require.ElementsMatch(t, []string{"organization", "account"}, capabilities.RuntimeAudience.SubjectTypes)
	require.Empty(t, capabilities.RuntimeAudience.WorkspaceIDs)
	assertAccountRuntimeSurfaceContract(t, capabilities.RuntimeSurfaces, true)
	assertAccountRuntimeResourceListContract(t, capabilities.RuntimeResourceLists, true)
}

func TestGetAccountCapabilitiesOrganizationAdminCanManageModelConfig(t *testing.T) {
	organizationID := "org-1"
	repo := &fakeAccountContextRepository{
		ctxModel: &auth_model.AccountContext{
			AccountID:             "acc-1",
			CurrentOrganizationID: &organizationID,
		},
	}
	organizationService := &fakeOrganizationContextService{
		members: map[string]bool{organizationID: true},
		roles: map[string]workspace_model.OrganizationRole{
			organizationID: workspace_model.OrganizationRoleAdmin,
		},
	}

	svc := &AccountService{
		accountRepo:                repo,
		workspaceManagementService: &fakeWorkspaceContextService{},
		organizationService:        organizationService,
	}

	capabilities, err := svc.GetAccountCapabilities(context.Background(), "acc-1")
	require.NoError(t, err)
	require.True(t, capabilities.Organization.IsAdmin)
	require.True(t, capabilities.Organization.CanAccessDashboard)
	require.True(t, capabilities.Organization.CanManageModelConfig)
}

func TestGetAccountCapabilitiesWithoutOrganizationKeepsRuntimeSurfaceContractDisabled(t *testing.T) {
	repo := &fakeAccountContextRepository{
		ctxModel: &auth_model.AccountContext{
			AccountID: "acc-1",
		},
	}

	svc := &AccountService{
		accountRepo:                repo,
		workspaceManagementService: &fakeWorkspaceContextService{},
	}

	capabilities, err := svc.GetAccountCapabilities(context.Background(), "acc-1")
	require.NoError(t, err)
	require.Equal(t, "none", capabilities.Context.Mode)
	require.False(t, capabilities.Routes.OrganizationScopeAllowed)
	require.False(t, capabilities.Routes.WorkspaceScopeAllowed)
	require.True(t, capabilities.Routes.WorkspaceRequired)
	require.Empty(t, capabilities.RuntimeAudience.SubjectTypes)
	assertAccountRuntimeSurfaceContract(t, capabilities.RuntimeSurfaces, false)
	assertAccountRuntimeResourceListContract(t, capabilities.RuntimeResourceLists, false)
}

func TestGetAccountCapabilitiesRuntimeAudienceIncludesActiveDepartments(t *testing.T) {
	organizationID := "org-1"
	repo := &fakeAccountContextRepository{
		ctxModel: &auth_model.AccountContext{
			AccountID:             "acc-1",
			CurrentOrganizationID: &organizationID,
		},
	}
	organizationService := &fakeOrganizationContextService{
		members: map[string]bool{organizationID: true},
		roles: map[string]workspace_model.OrganizationRole{
			organizationID: workspace_model.OrganizationRoleNormal,
		},
	}
	db, mock := openAccountContextMockDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT department_members.department_id FROM "department_members" JOIN departments ON departments.id = department_members.department_id WHERE department_members.account_id = $1 AND departments.group_id = $2 AND departments.status = $3`)).
		WithArgs("acc-1", organizationID, string(workspace_model.DepartmentStatusActive)).
		WillReturnRows(sqlmock.NewRows([]string{"department_id"}).
			AddRow("dept-1").
			AddRow("dept-2"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT workspace_members.workspace_id FROM "workspace_members" JOIN workspaces ON workspaces.id = workspace_members.workspace_id WHERE workspace_members.account_id = $1 AND workspaces.organization_id = $2 AND workspaces.status = $3`)).
		WithArgs("acc-1", organizationID, string(workspace_model.WorkspaceStatusNormal)).
		WillReturnRows(sqlmock.NewRows([]string{"workspace_id"}).
			AddRow("ws-1").
			AddRow("ws-2"))

	svc := &AccountService{
		accountRepo:                repo,
		db:                         db,
		workspaceManagementService: &fakeWorkspaceContextService{},
		organizationService:        organizationService,
	}

	capabilities, err := svc.GetAccountCapabilities(context.Background(), "acc-1")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"organization", "account", "department", "workspace"}, capabilities.RuntimeAudience.SubjectTypes)
	require.ElementsMatch(t, []string{"dept-1", "dept-2"}, capabilities.RuntimeAudience.DepartmentIDs)
	require.ElementsMatch(t, []string{"ws-1", "ws-2"}, capabilities.RuntimeAudience.WorkspaceIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetAccountCapabilitiesWorkspaceModeUsesWorkspaceMembership(t *testing.T) {
	organizationID := "org-1"
	workspaceID := "ws-1"
	repo := &fakeAccountContextRepository{
		ctxModel: &auth_model.AccountContext{
			AccountID:             "acc-1",
			CurrentOrganizationID: &organizationID,
			CurrentWorkspaceID:    &workspaceID,
		},
	}
	workspaceService := &fakeWorkspaceContextService{
		workspaces: map[string]*workspace_model.Workspace{
			workspaceID: {
				ID:             workspaceID,
				Name:           "Workspace",
				Status:         workspace_model.WorkspaceStatusNormal,
				OrganizationID: &organizationID,
			},
		},
		joins: map[string]bool{
			workspaceID + ":acc-1": true,
		},
	}
	organizationService := &fakeOrganizationContextService{
		members: map[string]bool{organizationID: true},
		roles: map[string]workspace_model.OrganizationRole{
			organizationID: workspace_model.OrganizationRoleNormal,
		},
		workspacePermissions: map[string]*shared_dto.WorkspaceMemberPermissionsResponse{
			organizationID + ":" + workspaceID + ":acc-1": {
				OrganizationID: organizationID,
				WorkspaceID:    workspaceID,
				AccountID:      "acc-1",
				Permissions: []string{
					string(workspace_model.WorkspacePermissionAgentView),
				},
			},
		},
	}

	svc := &AccountService{
		accountRepo:                repo,
		workspaceManagementService: workspaceService,
		organizationService:        organizationService,
	}

	capabilities, err := svc.GetAccountCapabilities(context.Background(), "acc-1")
	require.NoError(t, err)
	require.Equal(t, "workspace", capabilities.Context.Mode)
	require.True(t, capabilities.Routes.OrganizationScopeAllowed)
	require.True(t, capabilities.Routes.WorkspaceScopeAllowed)
	require.False(t, capabilities.Routes.WorkspaceRequired)
	require.True(t, capabilities.Workspace.Available)
	require.True(t, capabilities.Workspace.CanView)
	require.NotContains(t, capabilities.Workspace.Permissions, string(workspace_model.WorkspacePermissionWorkspaceView))
}

func TestGetAccountCapabilitiesWorkspaceModeAllowsEmptyPermissionSnapshot(t *testing.T) {
	organizationID := "org-1"
	workspaceID := "ws-1"
	repo := &fakeAccountContextRepository{
		ctxModel: &auth_model.AccountContext{
			AccountID:             "acc-1",
			CurrentOrganizationID: &organizationID,
			CurrentWorkspaceID:    &workspaceID,
		},
	}
	workspaceService := &fakeWorkspaceContextService{
		workspaces: map[string]*workspace_model.Workspace{
			workspaceID: {
				ID:             workspaceID,
				Name:           "Workspace",
				Status:         workspace_model.WorkspaceStatusNormal,
				OrganizationID: &organizationID,
			},
		},
		joins: map[string]bool{
			workspaceID + ":acc-1": true,
		},
	}
	organizationService := &fakeOrganizationContextService{
		members: map[string]bool{organizationID: true},
		roles: map[string]workspace_model.OrganizationRole{
			organizationID: workspace_model.OrganizationRoleNormal,
		},
		workspacePermissions: map[string]*shared_dto.WorkspaceMemberPermissionsResponse{
			organizationID + ":" + workspaceID + ":acc-1": {
				OrganizationID: organizationID,
				WorkspaceID:    workspaceID,
				AccountID:      "acc-1",
				Permissions:    []string{},
			},
		},
	}

	svc := &AccountService{
		accountRepo:                repo,
		workspaceManagementService: workspaceService,
		organizationService:        organizationService,
	}

	capabilities, err := svc.GetAccountCapabilities(context.Background(), "acc-1")
	require.NoError(t, err)
	require.True(t, capabilities.Routes.WorkspaceScopeAllowed)
	require.False(t, capabilities.Routes.WorkspaceRequired)
	require.True(t, capabilities.Workspace.Available)
	require.True(t, capabilities.Workspace.CanView)
	require.Empty(t, capabilities.Workspace.Permissions)
}

func TestGetAccountCapabilitiesKeepsWorkspaceForOrganizationAdminWithoutWorkspaceMembership(t *testing.T) {
	organizationID := "org-1"
	workspaceID := "ws-1"
	repo := &fakeAccountContextRepository{
		ctxModel: &auth_model.AccountContext{
			AccountID:             "acc-1",
			CurrentOrganizationID: &organizationID,
			CurrentWorkspaceID:    &workspaceID,
		},
	}
	workspaceService := &fakeWorkspaceContextService{
		workspaces: map[string]*workspace_model.Workspace{
			workspaceID: {
				ID:             workspaceID,
				Name:           "Stale",
				Status:         workspace_model.WorkspaceStatusNormal,
				OrganizationID: &organizationID,
			},
		},
		joins: map[string]bool{},
	}
	organizationService := &fakeOrganizationContextService{
		members: map[string]bool{organizationID: true},
		admins:  map[string]bool{organizationID: true},
		roles: map[string]workspace_model.OrganizationRole{
			organizationID: workspace_model.OrganizationRoleAdmin,
		},
	}

	svc := &AccountService{
		accountRepo:                repo,
		workspaceManagementService: workspaceService,
		organizationService:        organizationService,
	}

	capabilities, err := svc.GetAccountCapabilities(context.Background(), "acc-1")
	require.NoError(t, err)
	require.Equal(t, "workspace", capabilities.Context.Mode)
	require.True(t, capabilities.Organization.IsAdmin)
	require.True(t, capabilities.Routes.OrganizationScopeAllowed)
	require.True(t, capabilities.Routes.WorkspaceScopeAllowed)
	require.False(t, capabilities.Routes.WorkspaceRequired)
	require.True(t, capabilities.Workspace.Available)
	require.True(t, capabilities.Workspace.CanView)
	require.Equal(t, string(workspace_model.WorkspaceRoleAdmin), capabilities.Workspace.Role)
	require.Contains(t, capabilities.Workspace.Permissions, string(workspace_model.WorkspacePermissionDatabaseDelete))
	require.NotNil(t, capabilities.Context.CurrentWorkspaceID)
	require.Equal(t, workspaceID, *capabilities.Context.CurrentWorkspaceID)
	require.Nil(t, repo.updated)
}

func TestGetAccountCapabilitiesOrganizationAdminOverridesWorkspaceMembershipPermissions(t *testing.T) {
	organizationID := "org-1"
	workspaceID := "ws-1"
	repo := &fakeAccountContextRepository{
		ctxModel: &auth_model.AccountContext{
			AccountID:             "acc-1",
			CurrentOrganizationID: &organizationID,
			CurrentWorkspaceID:    &workspaceID,
		},
	}
	workspaceService := &fakeWorkspaceContextService{
		workspaces: map[string]*workspace_model.Workspace{
			workspaceID: {
				ID:             workspaceID,
				Name:           "Workspace",
				Status:         workspace_model.WorkspaceStatusNormal,
				OrganizationID: &organizationID,
			},
		},
		joins: map[string]bool{
			workspaceID + ":acc-1": true,
		},
	}
	organizationService := &fakeOrganizationContextService{
		members: map[string]bool{organizationID: true},
		admins:  map[string]bool{organizationID: true},
		roles: map[string]workspace_model.OrganizationRole{
			organizationID: workspace_model.OrganizationRoleAdmin,
		},
		workspacePermissions: map[string]*shared_dto.WorkspaceMemberPermissionsResponse{
			organizationID + ":" + workspaceID + ":acc-1": {
				OrganizationID: organizationID,
				WorkspaceID:    workspaceID,
				AccountID:      "acc-1",
				Permissions:    []string{string(workspace_model.WorkspacePermissionAgentView)},
			},
		},
	}

	svc := &AccountService{
		accountRepo:                repo,
		workspaceManagementService: workspaceService,
		organizationService:        organizationService,
	}

	capabilities, err := svc.GetAccountCapabilities(context.Background(), "acc-1")
	require.NoError(t, err)
	require.True(t, capabilities.Organization.IsAdmin)
	require.True(t, capabilities.Workspace.Available)
	require.True(t, capabilities.Workspace.CanView)
	require.True(t, capabilities.Routes.WorkspaceScopeAllowed)
	require.False(t, capabilities.Routes.WorkspaceRequired)
	require.Contains(t, capabilities.Workspace.Permissions, string(workspace_model.WorkspacePermissionDatabaseDelete))
}

func TestEnsureAccountContextForWorkspaceCreatesTargetWhenContextMissing(t *testing.T) {
	organizationID := "org-1"
	workspaceID := "ws-1"
	repo := &fakeAccountContextRepository{}
	workspaceService := &fakeWorkspaceContextService{
		workspaces: map[string]*workspace_model.Workspace{
			workspaceID: {
				ID:             workspaceID,
				Name:           "Workspace",
				Status:         workspace_model.WorkspaceStatusNormal,
				OrganizationID: &organizationID,
			},
		},
		joins: map[string]bool{
			workspaceID + ":acc-1": true,
		},
	}
	organizationService := &fakeOrganizationContextService{
		members: map[string]bool{organizationID: true},
	}

	svc := &AccountService{
		accountRepo:                repo,
		workspaceManagementService: workspaceService,
		organizationService:        organizationService,
	}

	ctxModel, selectedTarget, err := svc.EnsureAccountContextForWorkspace(context.Background(), "acc-1", organizationID, workspaceID)
	require.NoError(t, err)
	require.True(t, selectedTarget)
	require.NotNil(t, ctxModel.CurrentOrganizationID)
	require.Equal(t, organizationID, *ctxModel.CurrentOrganizationID)
	require.NotNil(t, ctxModel.CurrentWorkspaceID)
	require.Equal(t, workspaceID, *ctxModel.CurrentWorkspaceID)
	require.NotNil(t, repo.created)
}

func TestEnsureAccountContextForWorkspacePreservesValidCurrentWorkspace(t *testing.T) {
	currentOrganizationID := "org-current"
	currentWorkspaceID := "ws-current"
	targetOrganizationID := "org-target"
	targetWorkspaceID := "ws-target"
	repo := &fakeAccountContextRepository{
		ctxModel: &auth_model.AccountContext{
			AccountID:             "acc-1",
			CurrentOrganizationID: &currentOrganizationID,
			CurrentWorkspaceID:    &currentWorkspaceID,
		},
	}
	workspaceService := &fakeWorkspaceContextService{
		workspaces: map[string]*workspace_model.Workspace{
			currentWorkspaceID: {
				ID:             currentWorkspaceID,
				Name:           "Current",
				Status:         workspace_model.WorkspaceStatusNormal,
				OrganizationID: &currentOrganizationID,
			},
			targetWorkspaceID: {
				ID:             targetWorkspaceID,
				Name:           "Target",
				Status:         workspace_model.WorkspaceStatusNormal,
				OrganizationID: &targetOrganizationID,
			},
		},
		joins: map[string]bool{
			currentWorkspaceID + ":acc-1": true,
			targetWorkspaceID + ":acc-1":  true,
		},
	}
	organizationService := &fakeOrganizationContextService{
		members: map[string]bool{
			currentOrganizationID: true,
			targetOrganizationID:  true,
		},
	}

	svc := &AccountService{
		accountRepo:                repo,
		workspaceManagementService: workspaceService,
		organizationService:        organizationService,
	}

	ctxModel, selectedTarget, err := svc.EnsureAccountContextForWorkspace(context.Background(), "acc-1", targetOrganizationID, targetWorkspaceID)
	require.NoError(t, err)
	require.False(t, selectedTarget)
	require.NotNil(t, ctxModel.CurrentOrganizationID)
	require.Equal(t, currentOrganizationID, *ctxModel.CurrentOrganizationID)
	require.NotNil(t, ctxModel.CurrentWorkspaceID)
	require.Equal(t, currentWorkspaceID, *ctxModel.CurrentWorkspaceID)
	require.Nil(t, repo.created)
	require.Nil(t, repo.updated)
}

func TestEnsureAccountContextForWorkspaceBackfillsOrganizationForValidCurrentWorkspace(t *testing.T) {
	currentOrganizationID := "org-current"
	currentWorkspaceID := "ws-current"
	targetOrganizationID := "org-target"
	targetWorkspaceID := "ws-target"
	repo := &fakeAccountContextRepository{
		ctxModel: &auth_model.AccountContext{
			AccountID:          "acc-1",
			CurrentWorkspaceID: &currentWorkspaceID,
		},
	}
	workspaceService := &fakeWorkspaceContextService{
		workspaces: map[string]*workspace_model.Workspace{
			currentWorkspaceID: {
				ID:             currentWorkspaceID,
				Name:           "Current",
				Status:         workspace_model.WorkspaceStatusNormal,
				OrganizationID: &currentOrganizationID,
			},
			targetWorkspaceID: {
				ID:             targetWorkspaceID,
				Name:           "Target",
				Status:         workspace_model.WorkspaceStatusNormal,
				OrganizationID: &targetOrganizationID,
			},
		},
		joins: map[string]bool{
			currentWorkspaceID + ":acc-1": true,
			targetWorkspaceID + ":acc-1":  true,
		},
	}
	organizationService := &fakeOrganizationContextService{
		members: map[string]bool{
			currentOrganizationID: true,
			targetOrganizationID:  true,
		},
	}

	svc := &AccountService{
		accountRepo:                repo,
		workspaceManagementService: workspaceService,
		organizationService:        organizationService,
	}

	ctxModel, selectedTarget, err := svc.EnsureAccountContextForWorkspace(context.Background(), "acc-1", targetOrganizationID, targetWorkspaceID)
	require.NoError(t, err)
	require.False(t, selectedTarget)
	require.NotNil(t, ctxModel.CurrentOrganizationID)
	require.Equal(t, currentOrganizationID, *ctxModel.CurrentOrganizationID)
	require.NotNil(t, ctxModel.CurrentWorkspaceID)
	require.Equal(t, currentWorkspaceID, *ctxModel.CurrentWorkspaceID)
	require.NotNil(t, repo.updated)
	require.Equal(t, currentOrganizationID, *repo.updated.CurrentOrganizationID)
	require.Equal(t, currentWorkspaceID, *repo.updated.CurrentWorkspaceID)
}

func TestEnsureAccountContextForWorkspaceSwitchesInvalidCurrentWorkspaceToTarget(t *testing.T) {
	oldOrganizationID := "org-old"
	oldWorkspaceID := "ws-old"
	targetOrganizationID := "org-target"
	targetWorkspaceID := "ws-target"
	repo := &fakeAccountContextRepository{
		ctxModel: &auth_model.AccountContext{
			AccountID:             "acc-1",
			CurrentOrganizationID: &oldOrganizationID,
			CurrentWorkspaceID:    &oldWorkspaceID,
		},
	}
	workspaceService := &fakeWorkspaceContextService{
		workspaces: map[string]*workspace_model.Workspace{
			targetWorkspaceID: {
				ID:             targetWorkspaceID,
				Name:           "Target",
				Status:         workspace_model.WorkspaceStatusNormal,
				OrganizationID: &targetOrganizationID,
			},
		},
		joins: map[string]bool{
			targetWorkspaceID + ":acc-1": true,
		},
	}
	organizationService := &fakeOrganizationContextService{
		members: map[string]bool{
			oldOrganizationID:    true,
			targetOrganizationID: true,
		},
	}

	svc := &AccountService{
		accountRepo:                repo,
		workspaceManagementService: workspaceService,
		organizationService:        organizationService,
	}

	ctxModel, selectedTarget, err := svc.EnsureAccountContextForWorkspace(context.Background(), "acc-1", targetOrganizationID, targetWorkspaceID)
	require.NoError(t, err)
	require.True(t, selectedTarget)
	require.NotNil(t, ctxModel.CurrentOrganizationID)
	require.Equal(t, targetOrganizationID, *ctxModel.CurrentOrganizationID)
	require.NotNil(t, ctxModel.CurrentWorkspaceID)
	require.Equal(t, targetWorkspaceID, *ctxModel.CurrentWorkspaceID)
	require.NotNil(t, repo.updated)
	require.Equal(t, targetOrganizationID, *repo.updated.CurrentOrganizationID)
	require.Equal(t, targetWorkspaceID, *repo.updated.CurrentWorkspaceID)
}

type fakeAccountContextRepository struct {
	auth_repo.AccountRepository
	account  *auth_model.Account
	ctxModel *auth_model.AccountContext
	created  *auth_model.AccountContext
	updated  *auth_model.AccountContext
}

func openAccountContextMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	require.NoError(t, err)
	return db, mock
}

func assertAccountRuntimeSurfaceContract(t *testing.T, surfaces map[string]shared_dto.AccountRuntimeSurfaceCapability, enabled bool) {
	t.Helper()

	require.Len(t, surfaces, 5)
	tests := []struct {
		surface           string
		mode              string
		grantSubjectTypes []string
	}{
		{
			surface:           "webapp",
			mode:              "published_resource",
			grantSubjectTypes: []string{"public", "organization", "department", "workspace", "account"},
		},
		{
			surface:           "api",
			mode:              "api_key",
			grantSubjectTypes: []string{"public"},
		},
		{
			surface:           "app_center",
			mode:              "runtime_grant",
			grantSubjectTypes: []string{"organization", "department", "workspace", "account"},
		},
		{
			surface:           "builtin_app",
			mode:              "runtime_grant",
			grantSubjectTypes: []string{"organization", "department", "workspace", "account"},
		},
		{
			surface:           "internal",
			mode:              "internal_runtime",
			grantSubjectTypes: []string{"internal"},
		},
	}

	for _, tt := range tests {
		surface, ok := surfaces[tt.surface]
		require.Truef(t, ok, "runtime surface %s missing", tt.surface)
		require.Equalf(t, enabled, surface.Enabled, "runtime surface %s enabled", tt.surface)
		require.Equalf(t, tt.mode, surface.Mode, "runtime surface %s mode", tt.surface)
		require.ElementsMatchf(t, tt.grantSubjectTypes, surface.GrantSubjectTypes, "runtime surface %s grant subject types", tt.surface)
	}
}

func assertAccountRuntimeResourceListContract(t *testing.T, lists map[string]shared_dto.AccountRuntimeResourceListCapability, enabled bool) {
	t.Helper()

	require.Len(t, lists, 2)
	tests := []struct {
		key          string
		resourceType string
		surface      string
		mode         string
		endpoint     string
	}{
		{
			key:          "app_center",
			resourceType: "agent",
			surface:      "app_center",
			mode:         "runtimeauth_candidate_filter",
			endpoint:     "/console/api/agents/runnable-webapps",
		},
		{
			key:          "built_in_workflows",
			resourceType: "builtin_workflow",
			surface:      "builtin_app",
			mode:         "runtimeauth_candidate_filter",
			endpoint:     "/console/api/built-in-workflows",
		},
	}

	for _, tt := range tests {
		resourceList, ok := lists[tt.key]
		require.Truef(t, ok, "runtime resource list %s missing", tt.key)
		require.Equalf(t, enabled, resourceList.Enabled, "runtime resource list %s enabled", tt.key)
		require.Equalf(t, tt.resourceType, resourceList.ResourceType, "runtime resource list %s resource type", tt.key)
		require.Equalf(t, tt.surface, resourceList.Surface, "runtime resource list %s surface", tt.key)
		require.Equalf(t, tt.mode, resourceList.Mode, "runtime resource list %s mode", tt.key)
		require.Equalf(t, tt.endpoint, resourceList.Endpoint, "runtime resource list %s endpoint", tt.key)
	}
}

func (f *fakeAccountContextRepository) GetAccountContextByAccountID(ctx context.Context, accountID string) (*auth_model.AccountContext, error) {
	return f.ctxModel, nil
}

func (f *fakeAccountContextRepository) GetAccount(ctx context.Context, id string) (*auth_model.Account, error) {
	return f.account, nil
}

func (f *fakeAccountContextRepository) CreateAccountContext(ctx context.Context, ctxModel *auth_model.AccountContext) error {
	copyModel := *ctxModel
	f.created = &copyModel
	return nil
}

func (f *fakeAccountContextRepository) UpdateAccountContext(ctx context.Context, ctxModel *auth_model.AccountContext) error {
	copyModel := *ctxModel
	f.updated = &copyModel
	return nil
}

type fakeOrganizationContextService struct {
	interfaces.OrganizationService
	members              map[string]bool
	admins               map[string]bool
	roles                map[string]workspace_model.OrganizationRole
	workspacePermissions map[string]*shared_dto.WorkspaceMemberPermissionsResponse
	firstOwned           *workspace_model.Organization
	firstJoined          *workspace_model.Organization
}

func (f *fakeOrganizationContextService) IsOrganizationMember(ctx context.Context, organizationID, accountID string) (bool, error) {
	return f.members[organizationID], nil
}

func (f *fakeOrganizationContextService) GetUserOrganizationRole(ctx context.Context, organizationID, accountID string) (workspace_model.OrganizationRole, error) {
	if role, ok := f.roles[organizationID]; ok {
		return role, nil
	}
	if f.admins[organizationID] {
		return workspace_model.OrganizationRoleAdmin, nil
	}
	if f.members[organizationID] {
		return workspace_model.OrganizationRoleNormal, nil
	}
	return workspace_model.OrganizationRoleNormal, nil
}

func (f *fakeOrganizationContextService) IsOrganizationAdminOrOwner(ctx context.Context, organizationID, accountID string) (bool, error) {
	return f.admins[organizationID], nil
}

func (f *fakeOrganizationContextService) GetWorkspaceMemberPermissions(ctx context.Context, organizationID, workspaceID, accountID, targetAccountID string) (*shared_dto.WorkspaceMemberPermissionsResponse, error) {
	if f.workspacePermissions == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return f.workspacePermissions[organizationID+":"+workspaceID+":"+targetAccountID], nil
}

func (f *fakeOrganizationContextService) GetFirstOwnedOrganization(ctx context.Context, accountID string) (*workspace_model.Organization, error) {
	return f.firstOwned, nil
}

func (f *fakeOrganizationContextService) GetFirstJoinedOrganization(ctx context.Context, accountID string) (*workspace_model.Organization, error) {
	return f.firstJoined, nil
}

type fakeWorkspaceContextService struct {
	interfaces.WorkspaceManagementService
	workspaces map[string]*workspace_model.Workspace
	joins      map[string]bool
}

func (f *fakeWorkspaceContextService) GetWorkspaceByID(ctx context.Context, id string) (*workspace_model.Workspace, error) {
	workspace, ok := f.workspaces[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return workspace, nil
}

func (f *fakeWorkspaceContextService) GetByWorkspaceAndMember(ctx context.Context, workspaceID, accountID string) (*workspace_model.WorkspaceMember, error) {
	if f.joins[workspaceID+":"+accountID] {
		return &workspace_model.WorkspaceMember{WorkspaceID: workspaceID, AccountID: accountID}, nil
	}
	return nil, gorm.ErrRecordNotFound
}
