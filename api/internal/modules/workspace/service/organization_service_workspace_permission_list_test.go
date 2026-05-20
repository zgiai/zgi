package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zgiai/ginext/internal/modules/workspace/model"
	workspace_repo "github.com/zgiai/ginext/internal/modules/workspace/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestOrganizationService_ListWorkspaceIDsByPermission_OrgAdminGetsAllNormalWorkspaces(t *testing.T) {
	ctx := t.Context()
	db := setupOrganizationPermissionTestDB(t)
	orgID := "org-1"

	seedOrganizationPermissionBaseData(t, db, orgID)
	require.NoError(t, db.Create(&model.OrganizationMember{
		OrganizationID: orgID,
		AccountID:      "org-admin",
		Role:           model.OrganizationRoleAdmin,
		Status:         model.OrganizationMemberStatusActive,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}).Error)

	svc := &organizationService{
		organizationRepo: workspace_repo.NewOrganizationRepository(db),
		workspaceRepo:    workspace_repo.NewWorkspaceRepository(db),
	}

	workspaceIDs, err := svc.ListWorkspaceIDsByPermission(ctx, orgID, "org-admin", model.WorkspacePermissionAgentView)
	require.NoError(t, err)
	require.Equal(t, []string{"ws-alpha", "ws-beta", "ws-no-view"}, workspaceIDs)
}

func TestOrganizationService_ListWorkspaceIDsByPermission_FiltersCustomAndBuiltinRoles(t *testing.T) {
	ctx := t.Context()
	db := setupOrganizationPermissionTestDB(t)
	orgID := "org-1"

	seedOrganizationPermissionBaseData(t, db, orgID)
	require.NoError(t, db.Create(&model.OrganizationMember{
		OrganizationID: orgID,
		AccountID:      "member-1",
		Role:           model.OrganizationRoleNormal,
		Status:         model.OrganizationMemberStatusActive,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}).Error)

	noViewRoleID := "role-no-view"
	viewRoleID := "role-with-view"
	require.NoError(t, db.Create(&model.WorkspaceCustomRole{
		ID:             noViewRoleID,
		OrganizationID: orgID,
		Name:           "No View",
		Status:         model.WorkspaceCustomRoleStatusActive,
		Permissions:    []string{string(model.WorkspacePermissionDatabaseView)},
		CreatedBy:      "member-1",
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}).Error)
	require.NoError(t, db.Create(&model.WorkspaceCustomRole{
		ID:             viewRoleID,
		OrganizationID: orgID,
		Name:           "View Agent",
		Status:         model.WorkspaceCustomRoleStatusActive,
		Permissions:    []string{string(model.WorkspacePermissionAgentView)},
		CreatedBy:      "member-1",
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}).Error)

	require.NoError(t, db.Create(&model.WorkspaceMember{
		ID:          "join-1",
		WorkspaceID: "ws-alpha",
		AccountID:   "member-1",
		Role:        model.WorkspaceRoleNormal,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}).Error)
	require.NoError(t, db.Create(&model.WorkspaceMember{
		ID:          "join-2",
		WorkspaceID: "ws-no-view",
		AccountID:   "member-1",
		Role:        model.WorkspaceRoleNormal,
		RoleID:      &noViewRoleID,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}).Error)
	require.NoError(t, db.Create(&model.WorkspaceMember{
		ID:          "join-3",
		WorkspaceID: "ws-beta",
		AccountID:   "member-1",
		Role:        model.WorkspaceRoleNormal,
		RoleID:      &viewRoleID,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}).Error)
	require.NoError(t, db.Create(&model.WorkspaceMember{
		ID:          "join-4",
		WorkspaceID: "ws-other-org",
		AccountID:   "member-1",
		Role:        model.WorkspaceRoleNormal,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}).Error)

	svc := &organizationService{
		organizationRepo: workspace_repo.NewOrganizationRepository(db),
		workspaceRepo:    workspace_repo.NewWorkspaceRepository(db),
	}

	workspaceIDs, err := svc.ListWorkspaceIDsByPermission(ctx, orgID, "member-1", model.WorkspacePermissionAgentView)
	require.NoError(t, err)
	require.Equal(t, []string{"ws-alpha", "ws-beta"}, workspaceIDs)
}

func TestOrganizationService_CheckWorkspacePermission_RejectsOrganizationAdminForOtherOrganizationWorkspace(t *testing.T) {
	ctx := t.Context()
	db := setupOrganizationPermissionTestDB(t)
	orgID := "org-1"

	seedOrganizationPermissionBaseData(t, db, orgID)
	require.NoError(t, db.Create(&model.OrganizationMember{
		OrganizationID: orgID,
		AccountID:      "org-admin",
		Role:           model.OrganizationRoleAdmin,
		Status:         model.OrganizationMemberStatusActive,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}).Error)

	svc := newOrganizationPermissionTestService(db)

	ok, err := svc.CheckWorkspacePermission(ctx, orgID, "ws-other-org", "org-admin", model.WorkspacePermissionAgentView)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestOrganizationService_CheckWorkspacePermission_RejectsWorkspaceMemberForMismatchedOrganization(t *testing.T) {
	ctx := t.Context()
	db := setupOrganizationPermissionTestDB(t)
	orgID := "org-1"

	seedOrganizationPermissionBaseData(t, db, orgID)
	require.NoError(t, db.Create(&model.WorkspaceMember{
		ID:          "join-other-org",
		WorkspaceID: "ws-other-org",
		AccountID:   "member-1",
		Role:        model.WorkspaceRoleOwner,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}).Error)

	svc := newOrganizationPermissionTestService(db)

	ok, err := svc.CheckWorkspacePermission(ctx, orgID, "ws-other-org", "member-1", model.WorkspacePermissionAgentView)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestOrganizationService_CheckWorkspacePermission_AllowsWorkspaceMemberForMatchingOrganization(t *testing.T) {
	ctx := t.Context()
	db := setupOrganizationPermissionTestDB(t)
	orgID := "org-1"

	seedOrganizationPermissionBaseData(t, db, orgID)
	require.NoError(t, db.Create(&model.WorkspaceMember{
		ID:          "join-alpha",
		WorkspaceID: "ws-alpha",
		AccountID:   "member-1",
		Role:        model.WorkspaceRoleOwner,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}).Error)

	svc := newOrganizationPermissionTestService(db)

	ok, err := svc.CheckWorkspacePermission(ctx, orgID, "ws-alpha", "member-1", model.WorkspacePermissionAgentView)
	require.NoError(t, err)
	require.True(t, ok)
}

func newOrganizationPermissionTestService(db *gorm.DB) *organizationService {
	workspaceRepo := workspace_repo.NewWorkspaceRepository(db)
	workspaceMemberRepo := workspace_repo.NewWorkspaceMemberRepository(db)
	workspaceManagementService := NewWorkspaceManagementService(db, workspaceRepo, workspaceMemberRepo, nil, nil, nil)

	return &organizationService{
		organizationRepo:           workspace_repo.NewOrganizationRepository(db),
		workspaceRepo:              workspaceRepo,
		workspaceManagementService: workspaceManagementService,
	}
}

func setupOrganizationPermissionTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`
		CREATE TABLE organizations (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			short_name TEXT NULL,
			status TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE members (
			organization_id TEXT NOT NULL,
			account_id TEXT NOT NULL,
			role TEXT NOT NULL,
			name TEXT NULL,
			status TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			PRIMARY KEY (organization_id, account_id)
		)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE workspaces (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			encrypt_public_key TEXT NULL,
			plan TEXT NOT NULL DEFAULT 'basic',
			status TEXT NOT NULL,
			organization_id TEXT NULL,
			department_id TEXT NULL,
			api_key_id TEXT NULL,
			custom_config TEXT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE workspace_members (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			account_id TEXT NOT NULL,
			role TEXT NOT NULL,
			role_id TEXT NULL,
			current BOOLEAN NOT NULL DEFAULT FALSE,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			invited_by TEXT NULL,
			extensions TEXT NULL
		)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE roles (
			id TEXT PRIMARY KEY,
			group_id TEXT NOT NULL,
			name TEXT NOT NULL,
			description TEXT NULL,
			status TEXT NOT NULL,
			permissions TEXT NOT NULL,
			created_by TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)
	`).Error)

	return db
}

func seedOrganizationPermissionBaseData(t *testing.T, db *gorm.DB, orgID string) {
	t.Helper()

	otherOrgID := "org-2"
	now := time.Now().UTC()
	require.NoError(t, db.Create(&model.Organization{
		ID:        orgID,
		Name:      "Org 1",
		Status:    model.OrganizationStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&model.Organization{
		ID:        otherOrgID,
		Name:      "Org 2",
		Status:    model.OrganizationStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}).Error)

	require.NoError(t, db.Create(&model.Workspace{
		ID:             "ws-alpha",
		Name:           "Workspace Alpha",
		Status:         model.WorkspaceStatusNormal,
		OrganizationID: &orgID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}).Error)
	require.NoError(t, db.Create(&model.Workspace{
		ID:             "ws-beta",
		Name:           "Workspace Beta",
		Status:         model.WorkspaceStatusNormal,
		OrganizationID: &orgID,
		CreatedAt:      now.Add(time.Second),
		UpdatedAt:      now.Add(time.Second),
	}).Error)
	require.NoError(t, db.Create(&model.Workspace{
		ID:             "ws-no-view",
		Name:           "Workspace No View",
		Status:         model.WorkspaceStatusNormal,
		OrganizationID: &orgID,
		CreatedAt:      now.Add(2 * time.Second),
		UpdatedAt:      now.Add(2 * time.Second),
	}).Error)
	require.NoError(t, db.Create(&model.Workspace{
		ID:             "ws-archived",
		Name:           "Workspace Archived",
		Status:         model.WorkspaceStatusArchived,
		OrganizationID: &orgID,
		CreatedAt:      now.Add(3 * time.Second),
		UpdatedAt:      now.Add(3 * time.Second),
	}).Error)
	require.NoError(t, db.Create(&model.Workspace{
		ID:             "ws-other-org",
		Name:           "Workspace Other Org",
		Status:         model.WorkspaceStatusNormal,
		OrganizationID: &otherOrgID,
		CreatedAt:      now.Add(4 * time.Second),
		UpdatedAt:      now.Add(4 * time.Second),
	}).Error)
}
