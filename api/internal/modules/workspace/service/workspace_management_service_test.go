package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/modules/workspace/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGetWorkspaceMembersPaginatedReturnsHasMobile(t *testing.T) {
	t.Parallel()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`
CREATE TABLE accounts (
	id text primary key,
	name text not null,
	email text not null,
	avatar text,
	status text not null,
	last_login_at datetime,
	last_active_at datetime,
	created_at datetime not null,
	mobile_e164 text
)`).Error)
	require.NoError(t, db.Exec(`
CREATE TABLE workspace_members (
	workspace_id text not null,
	account_id text not null,
	role text not null,
	role_id text,
	permissions text not null default '[]',
	permission_source text not null default 'role_template',
	permission_template_role_id text,
	created_at datetime not null
)`).Error)

	now := time.Now().UTC()
	require.NoError(t, db.Exec(
		`INSERT INTO accounts (id, name, email, status, created_at, mobile_e164) VALUES (?, ?, ?, ?, ?, ?)`,
		"acc-with-mobile",
		"Mobile User",
		"mobile@example.com",
		"active",
		now,
		"+8613800138000",
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO accounts (id, name, email, status, created_at, mobile_e164) VALUES (?, ?, ?, ?, ?, ?)`,
		"acc-without-mobile",
		"No Mobile User",
		"nomobile@example.com",
		"active",
		now,
		"",
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO workspace_members (workspace_id, account_id, role, created_at) VALUES (?, ?, ?, ?)`,
		"ws-1",
		"acc-with-mobile",
		"member",
		now,
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO workspace_members (workspace_id, account_id, role, created_at) VALUES (?, ?, ?, ?)`,
		"ws-1",
		"acc-without-mobile",
		"member",
		now.Add(time.Second),
	).Error)

	svc := &WorkspaceManagementServiceImpl{db: db}

	members, total, err := svc.GetWorkspaceMembersPaginated(
		context.Background(),
		"ws-1",
		1,
		20,
		"",
		"",
	)
	require.NoError(t, err)
	require.EqualValues(t, 2, total)

	hasMobileByID := map[string]bool{}
	for _, member := range members {
		hasMobileByID[member.ID] = member.HasMobile
	}

	require.True(t, hasMobileByID["acc-with-mobile"])
	require.False(t, hasMobileByID["acc-without-mobile"])
}

func TestGetWorkspaceMembersPaginatedReturnsOrganizationDepartment(t *testing.T) {
	t.Parallel()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`
CREATE TABLE accounts (
	id text primary key,
	name text not null,
	email text not null,
	avatar text,
	status text not null,
	last_login_at datetime,
	last_active_at datetime,
	created_at datetime not null,
	mobile_e164 text
)`).Error)
	require.NoError(t, db.Exec(`
CREATE TABLE members (
	organization_id text not null,
	account_id text not null,
	name text,
	role text not null,
	status text not null
)`).Error)
	require.NoError(t, db.Exec(`
CREATE TABLE departments (
	id text primary key,
	group_id text not null,
	name text not null,
	status text not null,
	sort_order integer not null default 0,
	created_at datetime not null
)`).Error)
	require.NoError(t, db.Exec(`
CREATE TABLE department_members (
	department_id text not null,
	account_id text not null
)`).Error)
	require.NoError(t, db.Exec(`
CREATE TABLE workspace_members (
	workspace_id text not null,
	account_id text not null,
	role text not null,
	role_id text,
	permissions text not null default '[]',
	permission_source text not null default 'role_template',
	permission_template_role_id text,
	created_at datetime not null
)`).Error)

	now := time.Now().UTC()
	require.NoError(t, db.Exec(
		`INSERT INTO accounts (id, name, email, status, created_at) VALUES (?, ?, ?, ?, ?)`,
		"acc-1",
		"Account Name",
		"member@example.com",
		"active",
		now,
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO members (organization_id, account_id, name, role, status) VALUES (?, ?, ?, ?, ?)`,
		"org-1",
		"acc-1",
		"Member Name",
		"admin",
		"active",
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO departments (id, group_id, name, status, sort_order, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"dept-1",
		"org-1",
		"Platform",
		"active",
		1,
		now,
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO department_members (department_id, account_id) VALUES (?, ?)`,
		"dept-1",
		"acc-1",
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO workspace_members (workspace_id, account_id, role, created_at) VALUES (?, ?, ?, ?)`,
		"ws-1",
		"acc-1",
		"member",
		now,
	).Error)

	svc := &WorkspaceManagementServiceImpl{
		db: db,
		organizationService: workspaceManagementTestOrganizationService{
			organization: &model.Organization{ID: "org-1"},
		},
	}

	members, total, err := svc.GetWorkspaceMembersPaginated(
		context.Background(),
		"ws-1",
		1,
		20,
		"",
		"",
	)

	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, members, 1)
	require.Equal(t, "Member Name", members[0].Name)
	require.NotNil(t, members[0].DepartmentID)
	require.NotNil(t, members[0].DepartmentName)
	require.Equal(t, "dept-1", *members[0].DepartmentID)
	require.Equal(t, "Platform", *members[0].DepartmentName)
	require.Equal(t, "admin", members[0].OrganizationRole)
}

func TestUpdateMemberDirectPermissionsStoresExpandedDirectPermissions(t *testing.T) {
	t.Parallel()

	join := &model.WorkspaceMember{
		WorkspaceID:      "ws-1",
		AccountID:        "member-1",
		Role:             model.WorkspaceRoleNormal,
		PermissionSource: model.WorkspaceMemberPermissionSourceRoleTemplate,
		Permissions:      []string{"workspace.view"},
	}
	repo := &workspaceMemberDirectPermissionRepo{join: join}
	svc := &WorkspaceManagementServiceImpl{workspaceMemberRepo: repo}

	err := svc.UpdateMemberDirectPermissions(context.Background(), "ws-1", "member-1", []string{"agent.manage"})

	require.NoError(t, err)
	require.NotNil(t, repo.updated)
	require.Equal(t, model.WorkspaceMemberPermissionSourceDirect, repo.updated.PermissionSource)
	require.Contains(t, repo.updated.Permissions, "agent.create")
	require.NotContains(t, repo.updated.Permissions, "agent.manage")
	require.NotContains(t, repo.updated.Permissions, "workspace.manage")
}

func TestApplyWorkspaceMemberDirectPermissionSnapshotExpandsPermissions(t *testing.T) {
	t.Parallel()

	join := &model.WorkspaceMember{
		WorkspaceID:              "ws-1",
		AccountID:                "member-1",
		Role:                     model.WorkspaceRoleNormal,
		RoleID:                   stringPtr(model.WorkspaceBuiltinRoleViewerID),
		PermissionSource:         model.WorkspaceMemberPermissionSourceRoleTemplate,
		PermissionTemplateRoleID: stringPtr(model.WorkspaceBuiltinRoleViewerID),
		Permissions:              []string{"file.view"},
	}

	applyWorkspaceMemberDirectPermissionSnapshot(join, []string{"agent.manage", "agent.manage"})

	require.Equal(t, model.WorkspaceMemberPermissionSourceDirect, join.PermissionSource)
	require.Equal(t, stringPtr(model.WorkspaceBuiltinRoleViewerID), join.RoleID)
	require.Equal(t, stringPtr(model.WorkspaceBuiltinRoleViewerID), join.PermissionTemplateRoleID)
	require.Contains(t, join.Permissions, "agent.create")
	require.NotContains(t, join.Permissions, "agent.manage")
	require.NotContains(t, join.Permissions, "file.view")
	require.NotContains(t, join.Permissions, "workspace.manage")
}

func TestUpdateMemberDirectPermissionsRejectsOwner(t *testing.T) {
	t.Parallel()

	join := &model.WorkspaceMember{
		WorkspaceID: "ws-1",
		AccountID:   "owner-1",
		Role:        model.WorkspaceRoleOwner,
	}
	repo := &workspaceMemberDirectPermissionRepo{join: join}
	svc := &WorkspaceManagementServiceImpl{workspaceMemberRepo: repo}

	err := svc.UpdateMemberDirectPermissions(context.Background(), "ws-1", "owner-1", []string{"agent.view"})

	require.Error(t, err)
	require.Nil(t, repo.updated)
}

func TestCheckMemberPermissionRejectsGovernancePermissionsFromDirectSnapshot(t *testing.T) {
	t.Parallel()

	workspace := &model.Workspace{ID: "ws-1"}
	operator := &auth_model.Account{ID: "operator-1"}
	member := &auth_model.Account{ID: "member-1"}
	repo := &workspaceMemberDirectPermissionRepo{
		joins: map[string]*model.WorkspaceMember{
			"ws-1/operator-1": {
				WorkspaceID:      "ws-1",
				AccountID:        "operator-1",
				Role:             model.WorkspaceRoleNormal,
				PermissionSource: model.WorkspaceMemberPermissionSourceDirect,
				Permissions:      []string{string(model.WorkspacePermissionWorkspaceMemberManage)},
			},
			"ws-1/member-1": {
				WorkspaceID: "ws-1",
				AccountID:   "member-1",
				Role:        model.WorkspaceRoleNormal,
			},
		},
	}
	svc := &WorkspaceManagementServiceImpl{workspaceMemberRepo: repo}

	err := svc.CheckMemberPermission(context.Background(), workspace, operator, member, "remove")
	require.Error(t, err)
}

func TestCheckMemberPermissionAllowsAdminGovernanceEvenWithEmptySnapshot(t *testing.T) {
	t.Parallel()

	workspace := &model.Workspace{ID: "ws-1"}
	operator := &auth_model.Account{ID: "operator-1"}
	member := &auth_model.Account{ID: "member-1"}
	repo := &workspaceMemberDirectPermissionRepo{
		joins: map[string]*model.WorkspaceMember{
			"ws-1/operator-1": {
				WorkspaceID:      "ws-1",
				AccountID:        "operator-1",
				Role:             model.WorkspaceRoleAdmin,
				PermissionSource: model.WorkspaceMemberPermissionSourceRoleTemplate,
				Permissions:      []string{},
			},
			"ws-1/member-1": {
				WorkspaceID: "ws-1",
				AccountID:   "member-1",
				Role:        model.WorkspaceRoleNormal,
			},
		},
	}
	svc := &WorkspaceManagementServiceImpl{workspaceMemberRepo: repo}

	require.NoError(t, svc.CheckMemberPermission(context.Background(), workspace, operator, member, "remove"))
}

func TestCheckMemberPermissionRejectsPermissionManageFromDirectSnapshot(t *testing.T) {
	t.Parallel()

	workspace := &model.Workspace{ID: "ws-1"}
	operator := &auth_model.Account{ID: "operator-1"}
	member := &auth_model.Account{ID: "member-1"}
	repo := &workspaceMemberDirectPermissionRepo{
		joins: map[string]*model.WorkspaceMember{
			"ws-1/operator-1": {
				WorkspaceID:      "ws-1",
				AccountID:        "operator-1",
				Role:             model.WorkspaceRoleNormal,
				PermissionSource: model.WorkspaceMemberPermissionSourceDirect,
				Permissions:      []string{string(model.WorkspacePermissionWorkspacePermissionManage)},
			},
			"ws-1/member-1": {
				WorkspaceID: "ws-1",
				AccountID:   "member-1",
				Role:        model.WorkspaceRoleNormal,
			},
		},
	}
	svc := &WorkspaceManagementServiceImpl{workspaceMemberRepo: repo}

	err := svc.CheckMemberPermission(context.Background(), workspace, operator, member, "permission")
	require.Error(t, err)
}

func TestUpdateMemberRoleReappliesSameBuiltinTemplateSnapshot(t *testing.T) {
	t.Parallel()

	workspace := &model.Workspace{ID: "ws-1"}
	operator := &auth_model.Account{ID: "operator-1"}
	member := &auth_model.Account{ID: "member-1"}
	memberRoleID := model.WorkspaceBuiltinRoleMemberID
	repo := &workspaceMemberDirectPermissionRepo{
		joins: map[string]*model.WorkspaceMember{
			"ws-1/operator-1": {
				WorkspaceID:      "ws-1",
				AccountID:        "operator-1",
				Role:             model.WorkspaceRoleAdmin,
				PermissionSource: model.WorkspaceMemberPermissionSourceDirect,
				Permissions:      []string{string(model.WorkspacePermissionWorkspacePermissionManage)},
			},
			"ws-1/member-1": {
				WorkspaceID:              "ws-1",
				AccountID:                "member-1",
				Role:                     model.WorkspaceRoleNormal,
				RoleID:                   &memberRoleID,
				PermissionSource:         model.WorkspaceMemberPermissionSourceDirect,
				PermissionTemplateRoleID: &memberRoleID,
				Permissions:              []string{"agent.manage"},
			},
		},
	}
	svc := &WorkspaceManagementServiceImpl{workspaceMemberRepo: repo}

	err := svc.UpdateMemberRoleAndRoleIDWithPermissionCheck(
		context.Background(),
		workspace,
		member,
		string(model.WorkspaceRoleNormal),
		&memberRoleID,
		operator,
	)

	require.NoError(t, err)
	require.NotNil(t, repo.updated)
	require.Equal(t, model.WorkspaceMemberPermissionSourceRoleTemplate, repo.updated.PermissionSource)
	require.Equal(t, &memberRoleID, repo.updated.RoleID)
	require.Equal(t, &memberRoleID, repo.updated.PermissionTemplateRoleID)
	require.Contains(t, repo.updated.Permissions, string(model.WorkspacePermissionFileUpload))
	require.NotContains(t, repo.updated.Permissions, string(model.WorkspacePermissionWorkspaceView))
	require.NotContains(t, repo.updated.Permissions, "agent.manage")
}

func TestGetAccessibleWorkspaceIDsReturnsDirectMembershipsOnly(t *testing.T) {
	t.Parallel()

	repo := &workspaceMemberDirectPermissionRepo{
		joins: map[string]*model.WorkspaceMember{
			"ws-direct-1/account-1": {
				WorkspaceID: "ws-direct-1",
				AccountID:   "account-1",
				Role:        model.WorkspaceRoleNormal,
			},
			"ws-direct-2/account-1": {
				WorkspaceID: "ws-direct-2",
				AccountID:   "account-1",
				Role:        model.WorkspaceRoleAdmin,
			},
			"ws-other/account-2": {
				WorkspaceID: "ws-other",
				AccountID:   "account-2",
				Role:        model.WorkspaceRoleOwner,
			},
		},
	}
	svc := &WorkspaceManagementServiceImpl{workspaceMemberRepo: repo}

	workspaceIDs, err := svc.GetAccessibleWorkspaceIDs(context.Background(), "account-1")

	require.NoError(t, err)
	require.ElementsMatch(t, []string{"ws-direct-1", "ws-direct-2"}, workspaceIDs)
}

func TestTransferOwnerByOrganizationAdminDemotesActualOwner(t *testing.T) {
	t.Parallel()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Workspace{}, &model.WorkspaceMember{}))

	orgID := "org-1"
	workspaceID := "ws-1"
	oldOwnerID := "owner-1"
	newOwnerID := "member-1"
	operatorID := "org-admin-1"
	require.NoError(t, db.Create(&model.Workspace{
		ID:             workspaceID,
		Name:           "Workspace",
		Status:         model.WorkspaceStatusNormal,
		Plan:           "basic",
		OrganizationID: &orgID,
	}).Error)
	require.NoError(t, db.Create(&model.WorkspaceMember{
		ID:          "join-owner",
		WorkspaceID: workspaceID,
		AccountID:   oldOwnerID,
		Role:        model.WorkspaceRoleOwner,
	}).Error)
	require.NoError(t, db.Create(&model.WorkspaceMember{
		ID:          "join-member",
		WorkspaceID: workspaceID,
		AccountID:   newOwnerID,
		Role:        model.WorkspaceRoleNormal,
	}).Error)

	svc := &WorkspaceManagementServiceImpl{
		db:                  db,
		workspaceRepo:       repository.NewWorkspaceRepository(db),
		workspaceMemberRepo: repository.NewWorkspaceMemberRepository(db),
		organizationService: workspaceManagementTestOrganizationService{
			adminAccounts: map[string]bool{operatorID: true},
		},
	}

	err = svc.TransferOwner(context.Background(), workspaceID, operatorID, newOwnerID)

	require.NoError(t, err)
	var oldOwnerJoin model.WorkspaceMember
	require.NoError(t, db.Where("workspace_id = ? AND account_id = ?", workspaceID, oldOwnerID).First(&oldOwnerJoin).Error)
	require.Equal(t, model.WorkspaceRoleAdmin, oldOwnerJoin.Role)
	require.Equal(t, model.WorkspaceMemberPermissionSourceRoleTemplate, oldOwnerJoin.PermissionSource)
	require.NotNil(t, oldOwnerJoin.RoleID)
	require.Equal(t, model.WorkspaceBuiltinRoleAdminID, *oldOwnerJoin.RoleID)

	var newOwnerJoin model.WorkspaceMember
	require.NoError(t, db.Where("workspace_id = ? AND account_id = ?", workspaceID, newOwnerID).First(&newOwnerJoin).Error)
	require.Equal(t, model.WorkspaceRoleOwner, newOwnerJoin.Role)
	require.Equal(t, model.WorkspaceMemberPermissionSourceOwner, newOwnerJoin.PermissionSource)

	var operatorJoinCount int64
	require.NoError(t, db.Model(&model.WorkspaceMember{}).
		Where("workspace_id = ? AND account_id = ?", workspaceID, operatorID).
		Count(&operatorJoinCount).Error)
	require.Zero(t, operatorJoinCount)
}

func TestTransferOwnerNormalizesHistoricalOwnersWhenTargetAlreadyOwner(t *testing.T) {
	t.Parallel()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Workspace{}, &model.WorkspaceMember{}))

	orgID := "org-1"
	workspaceID := "ws-1"
	oldOwnerID := "owner-1"
	targetOwnerID := "owner-2"
	normalMemberID := "member-1"
	operatorID := "org-admin-1"
	require.NoError(t, db.Create(&model.Workspace{
		ID:             workspaceID,
		Name:           "Workspace",
		Status:         model.WorkspaceStatusNormal,
		Plan:           "basic",
		OrganizationID: &orgID,
	}).Error)
	require.NoError(t, db.Create(&model.WorkspaceMember{
		ID:          "join-owner-1",
		WorkspaceID: workspaceID,
		AccountID:   oldOwnerID,
		Role:        model.WorkspaceRoleOwner,
	}).Error)
	require.NoError(t, db.Create(&model.WorkspaceMember{
		ID:          "join-owner-2",
		WorkspaceID: workspaceID,
		AccountID:   targetOwnerID,
		Role:        model.WorkspaceRoleOwner,
	}).Error)
	require.NoError(t, db.Create(&model.WorkspaceMember{
		ID:          "join-member",
		WorkspaceID: workspaceID,
		AccountID:   normalMemberID,
		Role:        model.WorkspaceRoleNormal,
	}).Error)

	svc := &WorkspaceManagementServiceImpl{
		db:                  db,
		workspaceRepo:       repository.NewWorkspaceRepository(db),
		workspaceMemberRepo: repository.NewWorkspaceMemberRepository(db),
		organizationService: workspaceManagementTestOrganizationService{
			adminAccounts: map[string]bool{operatorID: true},
		},
	}

	err = svc.TransferOwner(context.Background(), workspaceID, operatorID, targetOwnerID)

	require.NoError(t, err)
	var ownerCount int64
	require.NoError(t, db.Model(&model.WorkspaceMember{}).
		Where("workspace_id = ? AND role = ?", workspaceID, model.WorkspaceRoleOwner).
		Count(&ownerCount).Error)
	require.EqualValues(t, 1, ownerCount)

	var oldOwnerJoin model.WorkspaceMember
	require.NoError(t, db.Where("workspace_id = ? AND account_id = ?", workspaceID, oldOwnerID).First(&oldOwnerJoin).Error)
	require.Equal(t, model.WorkspaceRoleAdmin, oldOwnerJoin.Role)
	require.Equal(t, model.WorkspaceMemberPermissionSourceRoleTemplate, oldOwnerJoin.PermissionSource)

	var targetOwnerJoin model.WorkspaceMember
	require.NoError(t, db.Where("workspace_id = ? AND account_id = ?", workspaceID, targetOwnerID).First(&targetOwnerJoin).Error)
	require.Equal(t, model.WorkspaceRoleOwner, targetOwnerJoin.Role)
	require.Equal(t, model.WorkspaceMemberPermissionSourceOwner, targetOwnerJoin.PermissionSource)

	var normalMemberJoin model.WorkspaceMember
	require.NoError(t, db.Where("workspace_id = ? AND account_id = ?", workspaceID, normalMemberID).First(&normalMemberJoin).Error)
	require.Equal(t, model.WorkspaceRoleNormal, normalMemberJoin.Role)
}

func TestTransferOwnerByWorkspaceOwnerDemotesSelfToAdmin(t *testing.T) {
	t.Parallel()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.WorkspaceMember{}))

	workspaceID := "ws-1"
	oldOwnerID := "owner-1"
	newOwnerID := "member-1"
	require.NoError(t, db.Create(&model.WorkspaceMember{
		ID:          "join-owner",
		WorkspaceID: workspaceID,
		AccountID:   oldOwnerID,
		Role:        model.WorkspaceRoleOwner,
	}).Error)
	require.NoError(t, db.Create(&model.WorkspaceMember{
		ID:          "join-member",
		WorkspaceID: workspaceID,
		AccountID:   newOwnerID,
		Role:        model.WorkspaceRoleNormal,
	}).Error)

	svc := &WorkspaceManagementServiceImpl{
		db:                  db,
		workspaceMemberRepo: repository.NewWorkspaceMemberRepository(db),
	}

	err = svc.TransferOwner(context.Background(), workspaceID, oldOwnerID, newOwnerID)

	require.NoError(t, err)
	var oldOwnerJoin model.WorkspaceMember
	require.NoError(t, db.Where("workspace_id = ? AND account_id = ?", workspaceID, oldOwnerID).First(&oldOwnerJoin).Error)
	require.Equal(t, model.WorkspaceRoleAdmin, oldOwnerJoin.Role)

	var newOwnerJoin model.WorkspaceMember
	require.NoError(t, db.Where("workspace_id = ? AND account_id = ?", workspaceID, newOwnerID).First(&newOwnerJoin).Error)
	require.Equal(t, model.WorkspaceRoleOwner, newOwnerJoin.Role)
}

type workspaceMemberDirectPermissionRepo struct {
	repository.WorkspaceMemberRepository
	join    *model.WorkspaceMember
	joins   map[string]*model.WorkspaceMember
	updated *model.WorkspaceMember
}

func (r *workspaceMemberDirectPermissionRepo) GetByWorkspaceAndMember(ctx context.Context, workspaceID, memberID string) (*model.WorkspaceMember, error) {
	if r.joins != nil {
		if join := r.joins[workspaceID+"/"+memberID]; join != nil {
			return join, nil
		}
		return nil, nil
	}
	if r.join == nil || r.join.WorkspaceID != workspaceID || r.join.AccountID != memberID {
		return nil, gorm.ErrRecordNotFound
	}
	return r.join, nil
}

func (r *workspaceMemberDirectPermissionRepo) GetJoinsByMemberID(ctx context.Context, memberID string) ([]*model.WorkspaceMember, error) {
	joins := make([]*model.WorkspaceMember, 0)
	for _, join := range r.joins {
		if join != nil && join.AccountID == memberID {
			joins = append(joins, join)
		}
	}
	if r.join != nil && r.join.AccountID == memberID {
		joins = append(joins, r.join)
	}
	return joins, nil
}

func (r *workspaceMemberDirectPermissionRepo) Update(ctx context.Context, join *model.WorkspaceMember) error {
	if join == nil {
		return errors.New("join is nil")
	}
	clone := *join
	clone.Permissions = append([]string(nil), join.Permissions...)
	r.updated = &clone
	return nil
}

func stringPtr(value string) *string {
	return &value
}

type workspaceManagementTestOrganizationService struct {
	interfaces.OrganizationService
	organization  *model.Organization
	adminAccounts map[string]bool
}

func (s workspaceManagementTestOrganizationService) GetOrganizationByWorkspaceID(context.Context, string) (*model.Organization, error) {
	return s.organization, nil
}

func (s workspaceManagementTestOrganizationService) IsOrganizationAdminOrOwner(_ context.Context, _ string, accountID string) (bool, error) {
	return s.adminAccounts[accountID], nil
}
