package service

import (
	"context"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	auth_repo "github.com/zgiai/zgi/api/internal/modules/user/auth/repository"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestUpdateAccountContextSwitchOrganizationResolvesAccessibleWorkspace(t *testing.T) {
	db, mock := newAccountContextMockDB(t)
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
		workspaces: map[string]*workspace_model.Workspace{
			oldWorkspaceID: {
				ID:             oldWorkspaceID,
				Name:           "Old workspace",
				Status:         workspace_model.WorkspaceStatusNormal,
				OrganizationID: &oldOrganizationID,
			},
		},
	}
	organizationService := &fakeOrganizationContextService{
		members: map[string]bool{newOrganizationID: true},
		admins:  map[string]bool{newOrganizationID: false},
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT workspaces.* FROM "workspaces" JOIN workspace_members ON workspaces.id = workspace_members.workspace_id WHERE (workspaces.organization_id = $1 AND workspaces.status = $2) AND workspace_members.account_id = $3 ORDER BY workspaces.created_at DESC LIMIT $4`)).
		WithArgs(newOrganizationID, string(workspace_model.WorkspaceStatusNormal), "acc-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "status", "organization_id"}).
			AddRow("ws-new", "New workspace", string(workspace_model.WorkspaceStatusNormal), newOrganizationID))

	svc := &AccountService{
		accountRepo:                repo,
		db:                         db,
		workspaceManagementService: workspaceService,
		organizationService:        organizationService,
	}

	ctxModel, err := svc.UpdateAccountContext(context.Background(), "acc-1", &newOrganizationID, nil)
	require.NoError(t, err)
	require.NotNil(t, ctxModel.CurrentOrganizationID)
	require.Equal(t, newOrganizationID, *ctxModel.CurrentOrganizationID)
	require.NotNil(t, ctxModel.CurrentWorkspaceID)
	require.Equal(t, "ws-new", *ctxModel.CurrentWorkspaceID)
	require.NotNil(t, repo.updated)
	require.Equal(t, "ws-new", *repo.updated.CurrentWorkspaceID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateAccountContextSwitchOrganizationWithoutWorkspaceLeavesWorkspaceEmpty(t *testing.T) {
	db, mock := newAccountContextMockDB(t)
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

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT workspaces.* FROM "workspaces" JOIN workspace_members ON workspaces.id = workspace_members.workspace_id WHERE (workspaces.organization_id = $1 AND workspaces.status = $2) AND workspace_members.account_id = $3 ORDER BY workspaces.created_at DESC LIMIT $4`)).
		WithArgs(newOrganizationID, string(workspace_model.WorkspaceStatusNormal), "acc-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "status", "organization_id"}))

	svc := &AccountService{
		accountRepo:                repo,
		db:                         db,
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
	require.NoError(t, mock.ExpectationsWereMet())
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

func TestGetAccountContextResolvesMissingWorkspaceInCurrentOrganization(t *testing.T) {
	db, mock := newAccountContextMockDB(t)
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

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT workspaces.* FROM "workspaces" JOIN workspace_members ON workspaces.id = workspace_members.workspace_id WHERE (workspaces.organization_id = $1 AND workspaces.status = $2) AND workspace_members.account_id = $3 ORDER BY workspaces.created_at DESC LIMIT $4`)).
		WithArgs(organizationID, string(workspace_model.WorkspaceStatusNormal), "acc-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "status", "organization_id"}).
			AddRow("ws-1", "Workspace", string(workspace_model.WorkspaceStatusNormal), organizationID))

	svc := &AccountService{
		accountRepo:                repo,
		db:                         db,
		workspaceManagementService: &fakeWorkspaceContextService{},
		organizationService:        organizationService,
	}

	ctxModel, err := svc.GetAccountContext(context.Background(), "acc-1")
	require.NoError(t, err)
	require.NotNil(t, ctxModel.CurrentWorkspaceID)
	require.Equal(t, "ws-1", *ctxModel.CurrentWorkspaceID)
	require.NotNil(t, repo.updated)
	require.Equal(t, "ws-1", *repo.updated.CurrentWorkspaceID)
	require.NoError(t, mock.ExpectationsWereMet())
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

func TestGetAccountContextResolvesAnyAccessibleWorkspaceWhenContextEmpty(t *testing.T) {
	db, mock := newAccountContextMockDB(t)
	organizationID := "org-1"
	repo := &fakeAccountContextRepository{
		ctxModel: &auth_model.AccountContext{
			AccountID: "acc-1",
		},
	}
	organizationService := &fakeOrganizationContextService{
		members: map[string]bool{organizationID: true},
		admins:  map[string]bool{organizationID: false},
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT workspaces.* FROM "workspaces" JOIN members AS organization_members ON organization_members.organization_id = workspaces.organization_id LEFT JOIN workspace_members ON workspaces.id = workspace_members.workspace_id AND workspace_members.account_id = organization_members.account_id WHERE organization_members.account_id = $1 AND workspaces.status = $2 AND workspaces.organization_id IS NOT NULL AND ((organization_members.role IN ($3,$4) OR workspace_members.account_id IS NOT NULL)) ORDER BY COALESCE(workspace_members.current, false) DESC, workspaces.created_at DESC LIMIT $5`)).
		WithArgs("acc-1", string(workspace_model.WorkspaceStatusNormal), workspace_model.OrganizationRoleOwner, workspace_model.OrganizationRoleAdmin, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "status", "organization_id"}).
			AddRow("ws-1", "Workspace", string(workspace_model.WorkspaceStatusNormal), organizationID))

	svc := &AccountService{
		accountRepo:                repo,
		db:                         db,
		workspaceManagementService: &fakeWorkspaceContextService{},
		organizationService:        organizationService,
	}

	ctxModel, err := svc.GetAccountContext(context.Background(), "acc-1")
	require.NoError(t, err)
	require.NotNil(t, ctxModel.CurrentOrganizationID)
	require.Equal(t, organizationID, *ctxModel.CurrentOrganizationID)
	require.NotNil(t, ctxModel.CurrentWorkspaceID)
	require.Equal(t, "ws-1", *ctxModel.CurrentWorkspaceID)
	require.NotNil(t, repo.updated)
	require.Equal(t, organizationID, *repo.updated.CurrentOrganizationID)
	require.Equal(t, "ws-1", *repo.updated.CurrentWorkspaceID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetAccountContextLeavesWorkspaceEmptyWhenNoneAccessible(t *testing.T) {
	db, mock := newAccountContextMockDB(t)
	repo := &fakeAccountContextRepository{
		ctxModel: &auth_model.AccountContext{
			AccountID: "acc-1",
		},
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT workspaces.* FROM "workspaces" JOIN members AS organization_members ON organization_members.organization_id = workspaces.organization_id LEFT JOIN workspace_members ON workspaces.id = workspace_members.workspace_id AND workspace_members.account_id = organization_members.account_id WHERE organization_members.account_id = $1 AND workspaces.status = $2 AND workspaces.organization_id IS NOT NULL AND ((organization_members.role IN ($3,$4) OR workspace_members.account_id IS NOT NULL)) ORDER BY COALESCE(workspace_members.current, false) DESC, workspaces.created_at DESC LIMIT $5`)).
		WithArgs("acc-1", string(workspace_model.WorkspaceStatusNormal), workspace_model.OrganizationRoleOwner, workspace_model.OrganizationRoleAdmin, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "status", "organization_id"}))

	svc := &AccountService{
		accountRepo:                repo,
		db:                         db,
		workspaceManagementService: &fakeWorkspaceContextService{},
		organizationService:        &fakeOrganizationContextService{},
	}

	ctxModel, err := svc.GetAccountContext(context.Background(), "acc-1")
	require.NoError(t, err)
	require.Nil(t, ctxModel.CurrentOrganizationID)
	require.Nil(t, ctxModel.CurrentWorkspaceID)
	require.Nil(t, repo.updated)
	require.NoError(t, mock.ExpectationsWereMet())
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

func TestGetAccountProfileCachesRepeatedReads(t *testing.T) {
	organizationID := "org-1"
	repo := &fakeAccountContextRepository{
		account: &auth_model.Account{
			ID:     "acc-1",
			Name:   "Alice",
			Email:  "alice@example.com",
			Status: auth_model.AccountStatusActive,
		},
		ctxModel: &auth_model.AccountContext{
			AccountID:             "acc-1",
			CurrentOrganizationID: &organizationID,
		},
	}
	orgService := &fakeOrganizationContextService{
		members: map[string]bool{
			organizationID: true,
		},
		roles: map[string]workspace_model.OrganizationRole{
			organizationID: workspace_model.OrganizationRoleAdmin,
		},
	}
	svc := &AccountService{
		accountRepo:         repo,
		organizationService: orgService,
		profileCache:        make(map[string]*accountProfileCacheEntry),
	}

	first, err := svc.GetAccountProfile(context.Background(), "acc-1")
	require.NoError(t, err)
	require.Equal(t, "Alice", first.Name)
	require.Equal(t, "admin", first.OrganizationRole)

	first.Name = "mutated"
	second, err := svc.GetAccountProfile(context.Background(), "acc-1")
	require.NoError(t, err)
	require.Equal(t, "Alice", second.Name)
	require.Equal(t, 1, repo.getAccountCalls)
	require.Equal(t, 1, repo.getContextCalls)
	require.Equal(t, 1, orgService.roleCalls)
}

func TestGetAccountProfileInvalidatesAfterContextUpdate(t *testing.T) {
	oldOrganizationID := "org-old"
	newOrganizationID := "org-new"
	newWorkspaceID := "ws-new"
	repo := &fakeAccountContextRepository{
		account: &auth_model.Account{
			ID:     "acc-1",
			Name:   "Alice",
			Email:  "alice@example.com",
			Status: auth_model.AccountStatusActive,
		},
		ctxModel: &auth_model.AccountContext{
			AccountID:             "acc-1",
			CurrentOrganizationID: &oldOrganizationID,
		},
	}
	workspaceService := &fakeWorkspaceContextService{
		workspaces: map[string]*workspace_model.Workspace{
			newWorkspaceID: {
				ID:             newWorkspaceID,
				Name:           "New workspace",
				Status:         workspace_model.WorkspaceStatusNormal,
				OrganizationID: &newOrganizationID,
			},
		},
		joins: map[string]bool{
			newWorkspaceID + ":acc-1": true,
		},
	}
	orgService := &fakeOrganizationContextService{
		members: map[string]bool{
			oldOrganizationID: true,
			newOrganizationID: true,
		},
		roles: map[string]workspace_model.OrganizationRole{
			oldOrganizationID: workspace_model.OrganizationRoleNormal,
			newOrganizationID: workspace_model.OrganizationRoleOwner,
		},
	}
	svc := &AccountService{
		accountRepo:                repo,
		workspaceManagementService: workspaceService,
		organizationService:        orgService,
		profileCache:               make(map[string]*accountProfileCacheEntry),
	}

	first, err := svc.GetAccountProfile(context.Background(), "acc-1")
	require.NoError(t, err)
	require.Equal(t, oldOrganizationID, *first.CurrentOrganizationID)
	require.Equal(t, "normal", first.OrganizationRole)

	_, err = svc.UpdateAccountContext(context.Background(), "acc-1", &newOrganizationID, &newWorkspaceID)
	require.NoError(t, err)

	second, err := svc.GetAccountProfile(context.Background(), "acc-1")
	require.NoError(t, err)
	require.Equal(t, newOrganizationID, *second.CurrentOrganizationID)
	require.Equal(t, newWorkspaceID, *second.CurrentWorkspaceID)
	require.Equal(t, "owner", second.OrganizationRole)
	require.Equal(t, 2, repo.getAccountCalls)
}

func TestSetCachedAccountProfileBoundsCacheSize(t *testing.T) {
	now := time.Now()
	svc := &AccountService{
		profileCache:           make(map[string]*accountProfileCacheEntry),
		profileCacheGeneration: make(map[string]uint64),
	}
	for i := 0; i < accountProfileCacheMaxEntries; i++ {
		accountID := "acc-" + strconv.Itoa(i)
		updatedAt := now
		if i == 0 {
			updatedAt = now.Add(-accountProfileCacheTTL)
		}
		svc.profileCache[accountID] = &accountProfileCacheEntry{
			profile: &dto.AccountProfileResponse{
				ID: accountID,
			},
			updatedAt: updatedAt,
		}
	}

	svc.setCachedAccountProfileIfCurrent("acc-new", &dto.AccountProfileResponse{ID: "acc-new"}, 0)

	require.Len(t, svc.profileCache, accountProfileCacheMaxEntries)
	require.NotContains(t, svc.profileCache, "acc-0")
	require.Contains(t, svc.profileCache, "acc-new")
}

type fakeAccountContextRepository struct {
	auth_repo.AccountRepository
	account         *auth_model.Account
	ctxModel        *auth_model.AccountContext
	created         *auth_model.AccountContext
	updated         *auth_model.AccountContext
	getAccountCalls int
	getContextCalls int
}

func (f *fakeAccountContextRepository) GetAccount(ctx context.Context, accountID string) (*auth_model.Account, error) {
	f.getAccountCalls++
	if f.account == nil || f.account.ID != accountID {
		return nil, gorm.ErrRecordNotFound
	}
	copyAccount := *f.account
	if f.account.Extensions != nil {
		copyAccount.Extensions = make(auth_model.JSONMap, len(f.account.Extensions))
		for key, value := range f.account.Extensions {
			copyAccount.Extensions[key] = value
		}
	}
	return &copyAccount, nil
}

func (f *fakeAccountContextRepository) GetAccountContextByAccountID(ctx context.Context, accountID string) (*auth_model.AccountContext, error) {
	f.getContextCalls++
	return f.ctxModel, nil
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
	members     map[string]bool
	admins      map[string]bool
	roles       map[string]workspace_model.OrganizationRole
	roleCalls   int
	firstOwned  *workspace_model.Organization
	firstJoined *workspace_model.Organization
}

func (f *fakeOrganizationContextService) IsOrganizationMember(ctx context.Context, organizationID, accountID string) (bool, error) {
	return f.members[organizationID], nil
}

func (f *fakeOrganizationContextService) IsOrganizationAdminOrOwner(ctx context.Context, organizationID, accountID string) (bool, error) {
	return f.admins[organizationID], nil
}

func (f *fakeOrganizationContextService) GetUserOrganizationRole(ctx context.Context, organizationID, accountID string) (workspace_model.OrganizationRole, error) {
	f.roleCalls++
	if role, ok := f.roles[organizationID]; ok {
		return role, nil
	}
	return workspace_model.OrganizationRoleNormal, nil
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

func newAccountContextMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{SkipDefaultTransaction: true})
	require.NoError(t, err)

	return db, mock
}
