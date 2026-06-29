package service

import (
	"context"
	"database/sql/driver"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	shared_dto "github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	workspace_repo "github.com/zgiai/zgi/api/internal/modules/workspace/repository"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type organizationPermissionTestAccountService struct {
	interfaces.AccountService
}

func (organizationPermissionTestAccountService) GetAccountByID(ctx context.Context, id string) (*auth_model.Account, error) {
	if id == "" {
		return nil, errors.New("account not found")
	}
	return &auth_model.Account{
		ID:     id,
		Name:   id,
		Email:  id + "@example.com",
		Status: auth_model.AccountStatusActive,
	}, nil
}

func TestUpdateMemberStatusCannotDisableOwner(t *testing.T) {
	t.Parallel()

	db, mock := newOrganizationPermissionRegressionMockDB(t)
	svc := newOrganizationPermissionRegressionService(db)
	now := time.Now().UTC()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "organizations" WHERE id = $1 ORDER BY "organizations"."id" LIMIT $2`)).
		WithArgs("org-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "status", "created_at", "updated_at"}).
			AddRow("org-1", "Org", model.OrganizationStatusActive, now, now))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "members" WHERE organization_id = $1 AND account_id = $2 ORDER BY "members"."organization_id" LIMIT $3`)).
		WithArgs("org-1", "owner-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"organization_id", "account_id", "role", "status", "created_at", "updated_at"}).
			AddRow("org-1", "owner-1", model.OrganizationRoleOwner, model.OrganizationMemberStatusActive, now, now))

	err := svc.UpdateMemberStatus(context.Background(), &shared_dto.UpdateOrganizationMemberStatusRequest{
		OrganizationID: "org-1",
		AccountID:      "owner-1",
		Status:         model.OrganizationMemberStatusInactive,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot disable")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteCustomWorkspaceRoleRejectsAssignedRole(t *testing.T) {
	t.Parallel()

	db, mock := newOrganizationPermissionRegressionMockDB(t)
	svc := newOrganizationPermissionRegressionService(db)
	now := time.Now().UTC()
	roleID := "role-1"

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "roles" WHERE id = $1 AND group_id = $2 ORDER BY "roles"."id" LIMIT $3`)).
		WithArgs(roleID, "org-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "group_id", "name", "description", "status", "permissions", "created_by", "created_at", "updated_at"}).
			AddRow(roleID, "org-1", "Custom", nil, model.WorkspaceCustomRoleStatusActive, "[]", "owner-1", now, now))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "workspace_members" WHERE workspace_id IN (SELECT id FROM "workspaces" WHERE organization_id = $1) AND role_id = $2`)).
		WithArgs("org-1", roleID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	err := svc.DeleteCustomWorkspaceRole(context.Background(), "org-1", roleID, "owner-1")

	require.ErrorIs(t, err, ErrWorkspaceRoleInUse)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetUserWorkspacesInOrganizationQualifiesWorkspaceIDSubquery(t *testing.T) {
	t.Parallel()

	db, mock := newOrganizationPermissionRegressionMockDB(t)
	svc := newOrganizationPermissionRegressionService(db)
	now := time.Now().UTC()

	subqueryPattern := `workspaces\.id IN \(SELECT workspaces\.id FROM "workspaces" JOIN organizations ON organizations\.id = workspaces\.organization_id`

	mock.ExpectQuery(`SELECT count\(\*\) FROM "workspaces" JOIN workspace_members ON workspaces\.id = workspace_members\.workspace_id WHERE `+subqueryPattern).
		WithArgs("org-1", model.OrganizationStatusActive, "member-1", model.WorkspaceStatusNormal).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	rows := sqlmock.NewRows([]string{
		"id",
		"name",
		"plan",
		"status",
		"organization_id",
		"created_at",
		"updated_at",
	}).AddRow("workspace-1", "Workspace", "basic", model.WorkspaceStatusNormal, "org-1", now, now)
	mock.ExpectQuery(`SELECT .* FROM "workspaces" JOIN workspace_members ON workspaces\.id = workspace_members\.workspace_id WHERE `+subqueryPattern).
		WithArgs("org-1", model.OrganizationStatusActive, "member-1", model.WorkspaceStatusNormal, 100).
		WillReturnRows(rows)

	result, err := svc.GetUserWorkspacesInOrganization(context.Background(), "org-1", "member-1", 1, 100)

	require.NoError(t, err)
	require.Equal(t, int64(1), result.Total)
	require.Len(t, result.Data, 1)
	require.Equal(t, "workspace-1", result.Data[0].ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetManagedAppWorkspacesUsesDirectCreatePermissions(t *testing.T) {
	t.Parallel()

	db, mock := newOrganizationPermissionRegressionMockDB(t)
	svc := newOrganizationPermissionRegressionService(db)
	now := time.Now().UTC()

	expectOrganizationByID(mock, "org-1", now)
	expectOrganizationMemberMissing(mock, "org-1", "member-1")
	expectPermissionWorkspaceQuery(mock, "org-1", "member-1", []workspacePermissionWorkspaceRow{
		{id: "workspace-app", name: "App", status: model.WorkspaceStatusNormal, createdAt: now.Add(time.Minute)},
		{id: "workspace-dataset", name: "Dataset", status: model.WorkspaceStatusNormal, createdAt: now},
	})
	expectPermissionJoinQuery(mock, []workspacePermissionJoinRow{
		{workspaceID: "workspace-app", accountID: "member-1", role: model.WorkspaceRoleNormal, permissions: `["agent.create"]`, source: model.WorkspaceMemberPermissionSourceDirect},
		{workspaceID: "workspace-dataset", accountID: "member-1", role: model.WorkspaceRoleNormal, permissions: `["knowledge_base.create"]`, source: model.WorkspaceMemberPermissionSourceDirect},
	}, "workspace-app", "workspace-dataset", "member-1")

	result, err := svc.GetManagedAppWorkspacesInOrganization(context.Background(), "org-1", "member-1", 1, 20)

	require.NoError(t, err)
	require.Equal(t, int64(1), result.Total)
	require.Len(t, result.Data, 1)
	require.Equal(t, "workspace-app", result.Data[0].ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCheckAnyWorkspaceCreateDatasetPermissionRequiresOrganizationAdminMembershipForBypass(t *testing.T) {
	t.Parallel()

	db, mock := newOrganizationPermissionRegressionMockDB(t)
	svc := newOrganizationPermissionRegressionService(db)
	now := time.Now().UTC()

	expectOrganizationByID(mock, "org-1", now)
	expectOrganizationMemberMissing(mock, "org-1", "org-admin")
	expectPermissionWorkspaceQuery(mock, "org-1", "org-admin", nil)

	allowed, err := svc.CheckAnyWorkspaceCreateDatasetPermission(context.Background(), "org-1", "org-admin")

	require.NoError(t, err)
	require.False(t, allowed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCheckAnyWorkspaceCreateDatasetPermissionAllowsOrganizationAdmin(t *testing.T) {
	t.Parallel()

	db, mock := newOrganizationPermissionRegressionMockDB(t)
	svc := newOrganizationPermissionRegressionService(db)
	now := time.Now().UTC()

	expectOrganizationByID(mock, "org-1", now)
	expectOrganizationMemberRole(mock, "org-1", "org-admin", model.OrganizationRoleAdmin, now)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspaces" WHERE organization_id = $1 AND status = $2 ORDER BY created_at DESC`)).
		WithArgs("org-1", model.WorkspaceStatusNormal).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "organization_id", "status", "created_at", "updated_at"}).
			AddRow("workspace-any", "Any", "org-1", model.WorkspaceStatusNormal, now, now))

	allowed, err := svc.CheckAnyWorkspaceCreateDatasetPermission(context.Background(), "org-1", "org-admin")

	require.NoError(t, err)
	require.True(t, allowed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCheckAnyManagedWorkspacePermissionRequiresOrganizationAdminMembershipForBypass(t *testing.T) {
	t.Parallel()

	db, mock := newOrganizationPermissionRegressionMockDB(t)
	svc := newOrganizationPermissionRegressionService(db)
	now := time.Now().UTC()

	expectOrganizationByID(mock, "org-1", now)
	expectOrganizationMemberMissing(mock, "org-1", "org-admin")
	expectPermissionWorkspaceQuery(mock, "org-1", "org-admin", nil)

	allowed, err := svc.CheckAnyManagedWorkspacePermission(context.Background(), "org-1", "org-admin")

	require.NoError(t, err)
	require.False(t, allowed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetUserAllWorkspacesInOrganizationDoesNotUseOrganizationAdminBypass(t *testing.T) {
	t.Parallel()

	db, mock := newOrganizationPermissionRegressionMockDB(t)
	svc := newOrganizationPermissionRegressionService(db)

	expectOrganizationMemberMissing(mock, "org-1", "org-admin")
	expectPermissionWorkspaceQuery(mock, "org-1", "org-admin", nil)

	workspaces, err := svc.GetUserAllWorkspacesInOrganization(context.Background(), "org-1", "org-admin")

	require.NoError(t, err)
	require.Empty(t, workspaces)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetVisibleOrganizationMemberAccountIDsLimitsNonAdminScope(t *testing.T) {
	t.Parallel()

	db, mock := newOrganizationPermissionRegressionMockDB(t)
	svc := newOrganizationPermissionRegressionService(db)
	now := time.Now().UTC()
	organizationID := "org-1"
	workspaceID := "workspace-visible"
	rootDepartmentID := "department-root"
	childDepartmentID := "department-child"
	otherDepartmentID := "department-other"

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT dm.department_id FROM department_members dm JOIN departments d ON d.id = dm.department_id WHERE d.group_id = $1 AND d.status = $2 AND dm.account_id = $3`)).
		WithArgs(organizationID, model.DepartmentStatusActive, "viewer").
		WillReturnRows(sqlmock.NewRows([]string{"department_id"}).AddRow(rootDepartmentID))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "departments" WHERE group_id = $1 AND status = $2`)).
		WithArgs(organizationID, model.DepartmentStatusActive).
		WillReturnRows(sqlmock.NewRows([]string{"id", "group_id", "parent_id", "name", "sort_order", "status", "created_at", "updated_at", "created_by"}).
			AddRow(rootDepartmentID, organizationID, nil, "Root", 0, model.DepartmentStatusActive, now, now, nil).
			AddRow(childDepartmentID, organizationID, rootDepartmentID, "Child", 0, model.DepartmentStatusActive, now, now, nil).
			AddRow(otherDepartmentID, organizationID, nil, "Other", 0, model.DepartmentStatusActive, now, now, nil))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT DISTINCT account_id FROM "department_members" WHERE department_id IN ($1,$2)`)).
		WithArgs(childDepartmentID, rootDepartmentID).
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).
			AddRow("viewer").
			AddRow("dept-peer").
			AddRow("child-peer"))

	expectOrganizationMemberRole(mock, organizationID, "viewer", model.OrganizationRoleNormal, now)
	expectPermissionWorkspaceQuery(mock, organizationID, "viewer", []workspacePermissionWorkspaceRow{
		{id: workspaceID, name: "Visible Workspace", status: model.WorkspaceStatusNormal, createdAt: now},
	})
	expectPermissionJoinQuery(mock, []workspacePermissionJoinRow{
		{workspaceID: workspaceID, accountID: "viewer", role: model.WorkspaceRoleNormal, permissions: `[]`, source: model.WorkspaceMemberPermissionSourceRoleTemplate},
	}, workspaceID, "viewer")
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT DISTINCT account_id FROM "workspace_members" WHERE workspace_id IN ($1)`)).
		WithArgs(workspaceID).
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).
			AddRow("viewer").
			AddRow("workspace-peer"))

	accountIDs, err := svc.getVisibleOrganizationMemberAccountIDs(context.Background(), organizationID, "viewer")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"viewer", "dept-peer", "child-peer", "workspace-peer"}, accountIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCheckAnyWorkspaceCreateDatasetPermissionUsesDirectCreatePermission(t *testing.T) {
	t.Parallel()

	db, mock := newOrganizationPermissionRegressionMockDB(t)
	svc := newOrganizationPermissionRegressionService(db)
	now := time.Now().UTC()

	expectOrganizationByID(mock, "org-1", now)
	expectOrganizationMemberMissing(mock, "org-1", "member-1")
	expectPermissionWorkspaceQuery(mock, "org-1", "member-1", []workspacePermissionWorkspaceRow{
		{id: "workspace-dataset", name: "Dataset", status: model.WorkspaceStatusNormal, createdAt: now},
	})
	expectPermissionJoinQuery(mock, []workspacePermissionJoinRow{
		{workspaceID: "workspace-dataset", accountID: "member-1", role: model.WorkspaceRoleNormal, permissions: `["knowledge_base.create"]`, source: model.WorkspaceMemberPermissionSourceDirect},
	}, "workspace-dataset", "member-1")

	allowed, err := svc.CheckAnyWorkspaceCreateDatasetPermission(context.Background(), "org-1", "member-1")

	require.NoError(t, err)
	require.True(t, allowed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListWorkspaceIDsByPermissionUsesMemberSnapshotOnly(t *testing.T) {
	t.Parallel()

	db, mock := newOrganizationPermissionRegressionMockDB(t)
	svc := newOrganizationPermissionRegressionService(db)
	now := time.Now().UTC()
	customRoleID := "10000000-0000-0000-0000-000000000001"

	expectOrganizationByID(mock, "org-1", now)
	expectOrganizationMemberMissing(mock, "org-1", "member-1")
	rows := sqlmock.NewRows([]string{
		"workspace_id",
		"role",
		"role_id",
		"permissions",
		"permission_source",
	}).
		AddRow("workspace-direct", model.WorkspaceRoleNormal, customRoleID, `["database.manage"]`, model.WorkspaceMemberPermissionSourceRoleTemplate).
		AddRow("workspace-template-only", model.WorkspaceRoleNormal, customRoleID, `[]`, model.WorkspaceMemberPermissionSourceRoleTemplate)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT workspace_members.workspace_id, workspace_members.role, workspace_members.role_id, workspace_members.permissions::text AS permissions, workspace_members.permission_source FROM "workspace_members" JOIN workspaces ON workspaces.id = workspace_members.workspace_id WHERE workspace_members.account_id = $1 AND workspaces.organization_id = $2 AND workspaces.status = $3`)).
		WithArgs("member-1", "org-1", model.WorkspaceStatusNormal).
		WillReturnRows(rows)

	workspaceIDs, err := svc.ListWorkspaceIDsByPermission(
		context.Background(),
		"org-1",
		"member-1",
		model.WorkspacePermissionDatabaseManage,
	)

	require.NoError(t, err)
	require.Equal(t, []string{"workspace-direct"}, workspaceIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetWorkspaceMemberPermissionsReturnsRawWorkspaceRole(t *testing.T) {
	t.Parallel()

	db, mock := newOrganizationPermissionRegressionMockDB(t)
	now := time.Now().UTC()
	adminRoleID := model.WorkspaceBuiltinRoleAdminID
	svc := &organizationService{
		organizationRepo: workspace_repo.NewOrganizationRepository(db),
		workspaceRepo:    workspace_repo.NewWorkspaceRepository(db),
		workspaceManagementService: &workspaceMemberPermissionsManagementService{
			join: &model.WorkspaceMember{
				WorkspaceID:              "ws-1",
				AccountID:                "admin-1",
				Role:                     model.WorkspaceRoleAdmin,
				RoleID:                   &adminRoleID,
				PermissionSource:         model.WorkspaceMemberPermissionSourceRoleTemplate,
				PermissionTemplateRoleID: &adminRoleID,
			},
		},
	}

	expectOrganizationByID(mock, "org-1", now)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "organization_id" FROM "workspaces" WHERE id = $1 ORDER BY "workspaces"."id" LIMIT $2`)).
		WithArgs("ws-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"organization_id"}).AddRow("org-1"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT role FROM "members" WHERE organization_id = $1 AND account_id = $2`)).
		WithArgs("org-1", "admin-1").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow(model.OrganizationRoleAdmin))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspaces" WHERE id = $1 ORDER BY "workspaces"."id" LIMIT $2`)).
		WithArgs("ws-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "organization_id", "status", "created_at", "updated_at"}).
			AddRow("ws-1", "Workspace", "org-1", model.WorkspaceStatusNormal, now, now))

	resp, err := svc.GetWorkspaceMemberPermissions(context.Background(), "org-1", "ws-1", "admin-1", "admin-1")

	require.NoError(t, err)
	require.Equal(t, string(model.WorkspaceRoleAdmin), resp.WorkspaceRole)
	require.Equal(t, adminRoleID, *resp.WorkspaceRoleID)
	require.NotEmpty(t, resp.WorkspaceRoleName)
	require.NotEqual(t, resp.WorkspaceRoleName, resp.WorkspaceRole)
	require.Contains(t, resp.Permissions, string(model.WorkspacePermissionAgentCreate))
	require.Contains(t, resp.Permissions, string(model.WorkspacePermissionKnowledgeBaseDocumentCreate))
	require.Contains(t, resp.Permissions, string(model.WorkspacePermissionDatabaseDelete))
	require.Contains(t, resp.Permissions, string(model.WorkspacePermissionFileUpload))
	requireDisplayableWorkspaceAssetPermissions(t, resp.Permissions)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetWorkspaceMemberPermissionsCanonicalizesLegacySnapshot(t *testing.T) {
	t.Parallel()

	db, mock := newOrganizationPermissionRegressionMockDB(t)
	now := time.Now().UTC()
	legacyPermissions := []string{
		"workspace.view",
		"dashboard.view",
		"prompt.optimize",
		"content_parse.playground",
		string(model.WorkspacePermissionAgentManage),
		string(model.WorkspacePermissionKnowledgeBaseManage),
		string(model.WorkspacePermissionDatabaseAIQuery),
		string(model.WorkspacePermissionFileUploadCreate),
		string(model.WorkspacePermissionFileMoveCreate),
	}
	svc := &organizationService{
		organizationRepo: workspace_repo.NewOrganizationRepository(db),
		workspaceRepo:    workspace_repo.NewWorkspaceRepository(db),
		workspaceManagementService: &workspaceMemberPermissionsManagementService{
			join: &model.WorkspaceMember{
				WorkspaceID:      "ws-1",
				AccountID:        "member-1",
				Role:             model.WorkspaceRoleNormal,
				Permissions:      legacyPermissions,
				PermissionSource: model.WorkspaceMemberPermissionSourceRoleTemplate,
			},
		},
	}

	expectOrganizationByID(mock, "org-1", now)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "organization_id" FROM "workspaces" WHERE id = $1 ORDER BY "workspaces"."id" LIMIT $2`)).
		WithArgs("ws-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"organization_id"}).AddRow("org-1"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT role FROM "members" WHERE organization_id = $1 AND account_id = $2`)).
		WithArgs("org-1", "member-1").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow(model.OrganizationRoleNormal))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspaces" WHERE id = $1 ORDER BY "workspaces"."id" LIMIT $2`)).
		WithArgs("ws-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "organization_id", "status", "created_at", "updated_at"}).
			AddRow("ws-1", "Workspace", "org-1", model.WorkspaceStatusNormal, now, now))

	resp, err := svc.GetWorkspaceMemberPermissions(context.Background(), "org-1", "ws-1", "member-1", "member-1")

	require.NoError(t, err)
	require.Contains(t, resp.Permissions, string(model.WorkspacePermissionAgentCreate))
	require.Contains(t, resp.Permissions, string(model.WorkspacePermissionKnowledgeBaseDocumentCreate))
	require.Contains(t, resp.Permissions, string(model.WorkspacePermissionDatabaseAIQueryRead))
	require.Contains(t, resp.Permissions, string(model.WorkspacePermissionFileUpload))
	require.Contains(t, resp.Permissions, string(model.WorkspacePermissionFileMove))
	require.NotContains(t, resp.Permissions, string(model.WorkspacePermissionAgentManage))
	require.NotContains(t, resp.Permissions, string(model.WorkspacePermissionKnowledgeBaseManage))
	require.NotContains(t, resp.Permissions, string(model.WorkspacePermissionDatabaseAIQuery))
	require.NotContains(t, resp.Permissions, string(model.WorkspacePermissionFileUploadCreate))
	require.NotContains(t, resp.Permissions, string(model.WorkspacePermissionFileMoveCreate))
	for _, permission := range resp.Permissions {
		require.False(t, strings.HasPrefix(permission, "workspace."), "workspace permission should be hidden: %s", permission)
		require.False(t, strings.HasPrefix(permission, "dashboard."), "dashboard permission should be hidden: %s", permission)
		require.False(t, strings.HasPrefix(permission, "prompt."), "prompt permission should be hidden: %s", permission)
		require.False(t, strings.HasPrefix(permission, "content_parse."), "content parse permission should be hidden: %s", permission)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetWorkspaceMemberPermissionsOrganizationAdminDoesNotRequireWorkspaceMembership(t *testing.T) {
	t.Parallel()

	db, mock := newOrganizationPermissionRegressionMockDB(t)
	now := time.Now().UTC()
	svc := &organizationService{
		organizationRepo: workspace_repo.NewOrganizationRepository(db),
		workspaceRepo:    workspace_repo.NewWorkspaceRepository(db),
		workspaceManagementService: &workspaceMemberPermissionsManagementService{
			join: nil,
		},
	}

	expectOrganizationByID(mock, "org-1", now)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "organization_id" FROM "workspaces" WHERE id = $1 ORDER BY "workspaces"."id" LIMIT $2`)).
		WithArgs("ws-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"organization_id"}).AddRow("org-1"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT role FROM "members" WHERE organization_id = $1 AND account_id = $2`)).
		WithArgs("org-1", "admin-1").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow(model.OrganizationRoleAdmin))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspaces" WHERE id = $1 ORDER BY "workspaces"."id" LIMIT $2`)).
		WithArgs("ws-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "organization_id", "status", "created_at", "updated_at"}).
			AddRow("ws-1", "Workspace", "org-1", model.WorkspaceStatusNormal, now, now))

	resp, err := svc.GetWorkspaceMemberPermissions(context.Background(), "org-1", "ws-1", "admin-1", "admin-1")

	require.NoError(t, err)
	require.Equal(t, string(model.OrganizationRoleAdmin), resp.OrganizationRole)
	require.Equal(t, string(model.WorkspaceRoleAdmin), resp.WorkspaceRole)
	require.Equal(t, model.WorkspaceBuiltinRoleAdminID, *resp.WorkspaceRoleID)
	require.Contains(t, resp.Permissions, string(model.WorkspacePermissionDatabaseDelete))
	requireDisplayableWorkspaceAssetPermissions(t, resp.Permissions)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyWorkspaceRoleTemplateRejectsBuiltinTemplateTargets(t *testing.T) {
	organizationID := "org-1"
	workspaceSvc := &applyTemplateWorkspaceManagementService{
		workspaces: map[string]*model.Workspace{
			"ws-1": {ID: "ws-1", OrganizationID: &organizationID},
			"ws-2": {ID: "ws-2", OrganizationID: &organizationID},
		},
	}
	svc := &organizationService{
		accountService:             organizationPermissionTestAccountService{},
		workspaceManagementService: workspaceSvc,
	}

	resp, err := svc.ApplyWorkspaceRoleTemplate(context.Background(), &shared_dto.ApplyWorkspaceRoleTemplateRequest{
		OrganizationID: organizationID,
		RoleID:         model.WorkspaceBuiltinRoleAdminID,
		OperatorID:     "operator-1",
		Members: []shared_dto.ApplyWorkspaceRoleTemplateTarget{
			{WorkspaceID: "ws-1", AccountID: "member-1"},
			{WorkspaceID: "ws-2", AccountID: "member-2"},
		},
	})

	require.ErrorIs(t, err, ErrCannotApplyOwnerRoleTemplate)
	require.Nil(t, resp)
	require.Empty(t, workspaceSvc.builtinUpdates)
}

func TestApplyWorkspaceRoleTemplateRejectsOwnerTemplate(t *testing.T) {
	svc := &organizationService{
		accountService:             organizationPermissionTestAccountService{},
		workspaceManagementService: &applyTemplateWorkspaceManagementService{},
	}

	_, err := svc.ApplyWorkspaceRoleTemplate(context.Background(), &shared_dto.ApplyWorkspaceRoleTemplateRequest{
		OrganizationID: "org-1",
		RoleID:         model.WorkspaceBuiltinRoleOwnerID,
		OperatorID:     "operator-1",
		Members: []shared_dto.ApplyWorkspaceRoleTemplateTarget{
			{WorkspaceID: "ws-1", AccountID: "member-1"},
		},
	})

	require.ErrorIs(t, err, ErrCannotApplyOwnerRoleTemplate)
}

func TestApplyWorkspaceRoleTemplateAppliesCustomTemplateTargets(t *testing.T) {
	db, mock := newOrganizationPermissionRegressionMockDB(t)
	organizationID := "org-1"
	roleID := "10000000-0000-0000-0000-000000000001"
	now := time.Now().UTC()
	workspaceSvc := &applyTemplateWorkspaceManagementService{
		workspaces: map[string]*model.Workspace{
			"ws-1": {ID: "ws-1", OrganizationID: &organizationID},
		},
	}
	svc := &organizationService{
		organizationRepo:           workspace_repo.NewOrganizationRepository(db),
		accountService:             organizationPermissionTestAccountService{},
		workspaceManagementService: workspaceSvc,
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "roles" WHERE id = $1 AND group_id = $2 AND status = $3 ORDER BY "roles"."id" LIMIT $4`)).
		WithArgs(roleID, organizationID, model.WorkspaceCustomRoleStatusActive, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "group_id", "name", "description", "status", "permissions", "created_by", "created_at", "updated_at"}).
			AddRow(roleID, organizationID, "Custom", nil, model.WorkspaceCustomRoleStatusActive, `["agent.view"]`, "operator-1", now, now))

	resp, err := svc.ApplyWorkspaceRoleTemplate(context.Background(), &shared_dto.ApplyWorkspaceRoleTemplateRequest{
		OrganizationID: organizationID,
		RoleID:         roleID,
		OperatorID:     "operator-1",
		Members: []shared_dto.ApplyWorkspaceRoleTemplateTarget{
			{WorkspaceID: "ws-1", AccountID: "member-1"},
		},
	})

	require.NoError(t, err)
	require.Equal(t, 1, resp.AppliedCount)
	require.Zero(t, resp.FailedCount)
	require.Len(t, workspaceSvc.customUpdates, 1)
	require.Equal(t, "ws-1", workspaceSvc.customUpdates[0].workspaceID)
	require.Equal(t, "member-1", workspaceSvc.customUpdates[0].accountID)
	require.Equal(t, roleID, workspaceSvc.customUpdates[0].roleID)
	require.Equal(t, "operator-1", workspaceSvc.customUpdates[0].operatorID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyWorkspaceRoleTemplateReportsWorkspacePermissionFailure(t *testing.T) {
	db, mock := newOrganizationPermissionRegressionMockDB(t)
	organizationID := "org-1"
	roleID := "10000000-0000-0000-0000-000000000001"
	now := time.Now().UTC()
	workspaceSvc := &applyTemplateWorkspaceManagementService{
		workspaces: map[string]*model.Workspace{
			"ws-1": {ID: "ws-1", OrganizationID: &organizationID},
			"ws-2": {ID: "ws-2", OrganizationID: &organizationID},
		},
		failByTargetKey: map[string]error{
			"ws-1:member-1": errors.New("no permission to update member role"),
		},
	}
	svc := &organizationService{
		organizationRepo:           workspace_repo.NewOrganizationRepository(db),
		accountService:             organizationPermissionTestAccountService{},
		workspaceManagementService: workspaceSvc,
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "roles" WHERE id = $1 AND group_id = $2 AND status = $3 ORDER BY "roles"."id" LIMIT $4`)).
		WithArgs(roleID, organizationID, model.WorkspaceCustomRoleStatusActive, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "group_id", "name", "description", "status", "permissions", "created_by", "created_at", "updated_at"}).
			AddRow(roleID, organizationID, "Custom", nil, model.WorkspaceCustomRoleStatusActive, `["agent.view"]`, "operator-1", now, now))

	resp, err := svc.ApplyWorkspaceRoleTemplate(context.Background(), &shared_dto.ApplyWorkspaceRoleTemplateRequest{
		OrganizationID: organizationID,
		RoleID:         roleID,
		OperatorID:     "operator-1",
		Members: []shared_dto.ApplyWorkspaceRoleTemplateTarget{
			{WorkspaceID: "ws-1", AccountID: "member-1"},
			{WorkspaceID: "ws-2", AccountID: "member-2"},
		},
	})

	require.NoError(t, err)
	require.Equal(t, 1, resp.AppliedCount)
	require.Equal(t, 1, resp.FailedCount)
	require.Len(t, resp.Results, 2)
	require.Equal(t, "failed", resp.Results[0].Status)
	require.Contains(t, resp.Results[0].Message, "no permission")
	require.Equal(t, "applied", resp.Results[1].Status)
	require.Len(t, workspaceSvc.customUpdates, 1)
	require.Equal(t, "ws-2", workspaceSvc.customUpdates[0].workspaceID)
	require.Equal(t, "member-2", workspaceSvc.customUpdates[0].accountID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func newOrganizationPermissionRegressionService(db *gorm.DB) *organizationService {
	return &organizationService{
		organizationRepo: workspace_repo.NewOrganizationRepository(db),
		accountService:   organizationPermissionTestAccountService{},
	}
}

type applyTemplateWorkspaceManagementService struct {
	interfaces.WorkspaceManagementService
	workspaces       map[string]*model.Workspace
	builtinUpdates   []applyTemplateBuiltinUpdate
	customUpdates    []applyTemplateCustomUpdate
	failByTargetKey  map[string]error
	missingWorkspace bool
}

type applyTemplateBuiltinUpdate struct {
	workspaceID string
	accountID   string
	role        string
	roleID      *string
	operatorID  string
}

type applyTemplateCustomUpdate struct {
	workspaceID string
	accountID   string
	roleID      string
	operatorID  string
}

type workspaceMemberPermissionsManagementService struct {
	interfaces.WorkspaceManagementService

	join *model.WorkspaceMember
}

func (s *workspaceMemberPermissionsManagementService) GetByWorkspaceAndMember(context.Context, string, string) (*model.WorkspaceMember, error) {
	if s.join == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return s.join, nil
}

func requireDisplayableWorkspaceAssetPermissions(t *testing.T, permissions []string) {
	t.Helper()

	for _, permission := range permissions {
		code := model.WorkspacePermissionCode(permission)
		require.False(t, model.IsWorkspaceCompatibilityPermission(code), "permission should not expose compatibility-only code: %s", permission)
		require.False(t, model.IsWorkspaceGovernancePermission(code), "permission should not expose workspace governance code: %s", permission)
		require.False(t, strings.HasPrefix(permission, "workspace."), "permission should not expose retired workspace code: %s", permission)
		require.False(t, strings.HasPrefix(permission, "dashboard."), "permission should not expose retired dashboard code: %s", permission)
		require.False(t, strings.HasPrefix(permission, "prompt."), "permission should not expose retired prompt code: %s", permission)
		require.False(t, strings.HasPrefix(permission, "content_parse."), "permission should not expose retired content parse code: %s", permission)
	}
}

func (s *applyTemplateWorkspaceManagementService) GetWorkspaceByID(ctx context.Context, id string) (*model.Workspace, error) {
	if s.missingWorkspace {
		return nil, errors.New("workspace not found")
	}
	workspace, ok := s.workspaces[id]
	if !ok {
		return nil, errors.New("workspace not found")
	}
	return workspace, nil
}

func (s *applyTemplateWorkspaceManagementService) UpdateMemberRoleAndRoleIDWithPermissionCheck(ctx context.Context, workspace *model.Workspace, member *auth_model.Account, newRole string, roleID *string, operator *auth_model.Account) error {
	if err := s.failureFor(workspace.ID, member.ID); err != nil {
		return err
	}
	var roleIDCopy *string
	if roleID != nil {
		copied := *roleID
		roleIDCopy = &copied
	}
	s.builtinUpdates = append(s.builtinUpdates, applyTemplateBuiltinUpdate{
		workspaceID: workspace.ID,
		accountID:   member.ID,
		role:        newRole,
		roleID:      roleIDCopy,
		operatorID:  operator.ID,
	})
	return nil
}

func (s *applyTemplateWorkspaceManagementService) UpdateMemberCustomRoleWithPermissionCheck(ctx context.Context, workspace *model.Workspace, member *auth_model.Account, roleID string, operator *auth_model.Account) error {
	if err := s.failureFor(workspace.ID, member.ID); err != nil {
		return err
	}
	s.customUpdates = append(s.customUpdates, applyTemplateCustomUpdate{
		workspaceID: workspace.ID,
		accountID:   member.ID,
		roleID:      roleID,
		operatorID:  operator.ID,
	})
	return nil
}

func (s *applyTemplateWorkspaceManagementService) failureFor(workspaceID, accountID string) error {
	if s.failByTargetKey == nil {
		return nil
	}
	return s.failByTargetKey[workspaceID+":"+accountID]
}

func newOrganizationPermissionRegressionMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	mock.MatchExpectationsInOrder(true)

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{SkipDefaultTransaction: true})
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	return db, mock
}

type workspacePermissionWorkspaceRow struct {
	id        string
	name      string
	status    model.WorkspaceStatus
	createdAt time.Time
}

type workspacePermissionJoinRow struct {
	workspaceID string
	accountID   string
	role        model.WorkspaceMemberRole
	permissions string
	source      model.WorkspaceMemberPermissionSource
}

func expectOrganizationByID(mock sqlmock.Sqlmock, organizationID string, now time.Time) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "organizations" WHERE id = $1 ORDER BY "organizations"."id" LIMIT $2`)).
		WithArgs(organizationID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "status", "created_at", "updated_at"}).
			AddRow(organizationID, "Org", model.OrganizationStatusActive, now, now))
}

func expectOrganizationMemberMissing(mock sqlmock.Sqlmock, organizationID, accountID string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "members" WHERE organization_id = $1 AND account_id = $2 ORDER BY "members"."organization_id" LIMIT $3`)).
		WithArgs(organizationID, accountID, 1).
		WillReturnError(gorm.ErrRecordNotFound)
}

func expectOrganizationMemberRole(mock sqlmock.Sqlmock, organizationID, accountID string, role model.OrganizationRole, now time.Time) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "members" WHERE organization_id = $1 AND account_id = $2 ORDER BY "members"."organization_id" LIMIT $3`)).
		WithArgs(organizationID, accountID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"organization_id", "account_id", "role", "status", "created_at", "updated_at"}).
			AddRow(organizationID, accountID, role, model.OrganizationMemberStatusActive, now, now))
}

func expectPermissionWorkspaceQuery(mock sqlmock.Sqlmock, organizationID, accountID string, workspaces []workspacePermissionWorkspaceRow) {
	rows := sqlmock.NewRows([]string{"id", "name", "organization_id", "status", "created_at", "updated_at"})
	for _, workspace := range workspaces {
		status := workspace.status
		if status == "" {
			status = model.WorkspaceStatusNormal
		}
		rows.AddRow(workspace.id, workspace.name, organizationID, status, workspace.createdAt, workspace.createdAt)
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT DISTINCT workspaces.* FROM "workspaces" JOIN workspace_members ON workspaces.id = workspace_members.workspace_id WHERE (workspaces.organization_id = $1 AND workspace_members.account_id = $2) AND workspaces.status = $3 ORDER BY workspaces.created_at DESC`)).
		WithArgs(organizationID, accountID, model.WorkspaceStatusNormal).
		WillReturnRows(rows)
}

func expectPermissionJoinQuery(mock sqlmock.Sqlmock, joins []workspacePermissionJoinRow, args ...driver.Value) {
	rows := sqlmock.NewRows([]string{
		"id",
		"workspace_id",
		"account_id",
		"role",
		"role_id",
		"permissions",
		"permission_source",
		"permission_template_role_id",
		"current",
		"created_at",
		"updated_at",
	})
	now := time.Now().UTC()
	for _, join := range joins {
		rows.AddRow(
			join.workspaceID+":"+join.accountID,
			join.workspaceID,
			join.accountID,
			join.role,
			nil,
			[]byte(join.permissions),
			join.source,
			nil,
			false,
			now,
			now,
		)
	}

	mock.ExpectQuery(`SELECT \* FROM "workspace_members" WHERE workspace_id IN \(.+\) AND account_id = \$\d+`).
		WithArgs(args...).
		WillReturnRows(rows)
}
