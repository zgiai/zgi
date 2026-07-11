package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/modules/workspace/repository"
)

func TestWorkspacePermissionFilterHonorsRequestedPermission(t *testing.T) {
	t.Parallel()

	fixture := newWorkspacePermissionFilterFixture(t)
	accountID := uuid.NewString()
	now := time.Now().UTC()

	memberWorkspaceID := fixture.addWorkspace("Member only", model.WorkspaceStatusNormal, now)
	adminWorkspaceID := fixture.addWorkspace("Admin", model.WorkspaceStatusNormal, now.Add(time.Second))
	archivedAdminWorkspaceID := fixture.addWorkspace("Archived admin", model.WorkspaceStatusArchived, now.Add(2*time.Second))

	fixture.addOrganizationMember(accountID, model.OrganizationRoleNormal)
	fixture.addWorkspaceMember(memberWorkspaceID, accountID, model.WorkspaceRoleNormal, nil)
	fixture.addWorkspaceMember(adminWorkspaceID, accountID, model.WorkspaceRoleAdmin, nil)
	fixture.addWorkspaceMember(archivedAdminWorkspaceID, accountID, model.WorkspaceRoleAdmin, nil)

	workspaces, err := fixture.service.GetAccessibleWorkspacesByPermission(
		context.Background(),
		accountID,
		fixture.organizationID,
		"create_database",
	)
	require.NoError(t, err)

	require.Equal(t, []string{adminWorkspaceID}, workspacePermissionResponseIDs(workspaces))
}

func TestWorkspacePermissionFilterAllowsFineCreatePermission(t *testing.T) {
	t.Parallel()

	fixture := newWorkspacePermissionFilterFixture(t)
	accountID := uuid.NewString()
	now := time.Now().UTC()

	createWorkspaceID := fixture.addWorkspace("Database create", model.WorkspaceStatusNormal, now)
	viewWorkspaceID := fixture.addWorkspace("Database view", model.WorkspaceStatusNormal, now.Add(time.Second))

	fixture.addOrganizationMember(accountID, model.OrganizationRoleNormal)
	fixture.addWorkspaceMemberWithPermissions(
		createWorkspaceID,
		accountID,
		model.WorkspaceRoleNormal,
		nil,
		[]string{string(model.WorkspacePermissionDatabaseCreate)},
		model.WorkspaceMemberPermissionSourceDirect,
	)
	fixture.addWorkspaceMemberWithPermissions(
		viewWorkspaceID,
		accountID,
		model.WorkspaceRoleNormal,
		nil,
		[]string{string(model.WorkspacePermissionDatabaseSchemaView)},
		model.WorkspaceMemberPermissionSourceDirect,
	)

	workspaces, err := fixture.service.GetAccessibleWorkspacesByPermission(
		context.Background(),
		accountID,
		fixture.organizationID,
		"create_database",
	)
	require.NoError(t, err)

	require.Equal(t, []string{createWorkspaceID}, workspacePermissionResponseIDs(workspaces))
}

func TestWorkspacePermissionFilterOrganizationAdminSeesNormalWorkspacesWithoutMembership(t *testing.T) {
	t.Parallel()

	fixture := newWorkspacePermissionFilterFixture(t)
	accountID := uuid.NewString()
	now := time.Now().UTC()

	firstWorkspaceID := fixture.addWorkspace("First", model.WorkspaceStatusNormal, now)
	secondWorkspaceID := fixture.addWorkspace("Second", model.WorkspaceStatusNormal, now.Add(time.Second))
	fixture.addWorkspace("Archived", model.WorkspaceStatusArchived, now.Add(2*time.Second))
	fixture.addOrganizationMember(accountID, model.OrganizationRoleAdmin)

	workspaces, err := fixture.service.GetAccessibleWorkspacesByPermission(
		context.Background(),
		accountID,
		fixture.organizationID,
		"create_agent",
	)
	require.NoError(t, err)

	require.Equal(t, []string{firstWorkspaceID, secondWorkspaceID}, workspacePermissionResponseIDs(workspaces))
}

func TestWorkspacePermissionFilterOrganizationOwnerSeesNormalWorkspacesWithoutMembership(t *testing.T) {
	t.Parallel()

	fixture := newWorkspacePermissionFilterFixture(t)
	accountID := uuid.NewString()
	now := time.Now().UTC()

	firstWorkspaceID := fixture.addWorkspace("First", model.WorkspaceStatusNormal, now)
	secondWorkspaceID := fixture.addWorkspace("Second", model.WorkspaceStatusNormal, now.Add(time.Second))
	fixture.addWorkspace("Archived", model.WorkspaceStatusArchived, now.Add(2*time.Second))
	fixture.addOrganizationMember(accountID, model.OrganizationRoleOwner)

	workspaces, err := fixture.service.GetAccessibleWorkspacesByPermission(
		context.Background(),
		accountID,
		fixture.organizationID,
		"create_knowledge",
	)
	require.NoError(t, err)

	require.Equal(t, []string{firstWorkspaceID, secondWorkspaceID}, workspacePermissionResponseIDs(workspaces))
}

func TestWorkspacePermissionFilterDoesNotFallbackToCustomRoleTemplate(t *testing.T) {
	t.Parallel()

	fixture := newWorkspacePermissionFilterFixture(t)
	accountID := uuid.NewString()
	customRoleID := uuid.NewString()
	now := time.Now().UTC()

	directWorkspaceID := fixture.addWorkspace("Direct", model.WorkspaceStatusNormal, now)
	templateOnlyWorkspaceID := fixture.addWorkspace("Template only", model.WorkspaceStatusNormal, now.Add(time.Second))

	fixture.addOrganizationMember(accountID, model.OrganizationRoleNormal)
	fixture.addWorkspaceMemberWithPermissions(
		directWorkspaceID,
		accountID,
		model.WorkspaceRoleNormal,
		&customRoleID,
		[]string{string(model.WorkspacePermissionDatabaseManage)},
		model.WorkspaceMemberPermissionSourceRoleTemplate,
	)
	fixture.addWorkspaceMemberWithPermissions(
		templateOnlyWorkspaceID,
		accountID,
		model.WorkspaceRoleNormal,
		&customRoleID,
		nil,
		model.WorkspaceMemberPermissionSourceRoleTemplate,
	)

	workspaces, err := fixture.service.GetAccessibleWorkspacesByPermission(
		context.Background(),
		accountID,
		fixture.organizationID,
		"create_database",
	)
	require.NoError(t, err)

	require.Equal(t, []string{directWorkspaceID}, workspacePermissionResponseIDs(workspaces))
}

func TestWorkspacePermissionFilterRejectsUnknownPermissionType(t *testing.T) {
	t.Parallel()

	fixture := newWorkspacePermissionFilterFixture(t)

	_, err := fixture.service.GetAccessibleWorkspacesByPermission(
		context.Background(),
		uuid.NewString(),
		fixture.organizationID,
		"create_everything",
	)
	require.ErrorContains(t, err, "invalid permission type")
}

type workspacePermissionFilterFixture struct {
	organizationID string
	organization   *model.Organization
	orgMembers     map[string]*model.OrganizationMember
	workspaces     map[string]*model.Workspace
	workspaceOrder []*model.Workspace
	workspaceJoins map[string]*model.WorkspaceMember
	service        WorkspacePermissionFilterService
}

func newWorkspacePermissionFilterFixture(t *testing.T) *workspacePermissionFilterFixture {
	t.Helper()

	organizationID := uuid.NewString()
	fixture := &workspacePermissionFilterFixture{
		organizationID: organizationID,
		organization: &model.Organization{
			ID:     organizationID,
			Name:   "Acme",
			Status: model.OrganizationStatusActive,
		},
		orgMembers:     make(map[string]*model.OrganizationMember),
		workspaces:     make(map[string]*model.Workspace),
		workspaceJoins: make(map[string]*model.WorkspaceMember),
	}

	fixture.service = NewWorkspacePermissionFilterService(
		&workspacePermissionFilterOrganizationRepo{fixture: fixture},
		&workspacePermissionFilterWorkspaceRepo{fixture: fixture},
		&workspacePermissionFilterMemberRepo{fixture: fixture},
	)
	return fixture
}

func (f *workspacePermissionFilterFixture) addOrganizationMember(accountID string, role model.OrganizationRole) {
	f.orgMembers[accountID] = &model.OrganizationMember{
		OrganizationID: f.organizationID,
		AccountID:      accountID,
		Role:           role,
		Status:         model.OrganizationMemberStatusActive,
	}
}

func (f *workspacePermissionFilterFixture) addWorkspace(name string, status model.WorkspaceStatus, createdAt time.Time) string {
	workspaceID := uuid.NewString()
	workspace := &model.Workspace{
		ID:             workspaceID,
		Name:           name,
		Status:         status,
		OrganizationID: &f.organizationID,
		CreatedAt:      createdAt,
		UpdatedAt:      createdAt,
	}
	f.workspaces[workspaceID] = workspace
	f.workspaceOrder = append(f.workspaceOrder, workspace)
	return workspaceID
}

func (f *workspacePermissionFilterFixture) addWorkspaceMember(
	workspaceID string,
	accountID string,
	role model.WorkspaceMemberRole,
	roleID *string,
) {
	f.addWorkspaceMemberWithPermissions(workspaceID, accountID, role, roleID, nil, "")
}

func (f *workspacePermissionFilterFixture) addWorkspaceMemberWithPermissions(
	workspaceID string,
	accountID string,
	role model.WorkspaceMemberRole,
	roleID *string,
	permissions []string,
	source model.WorkspaceMemberPermissionSource,
) {
	f.workspaceJoins[workspaceMemberKey(workspaceID, accountID)] = &model.WorkspaceMember{
		ID:               uuid.NewString(),
		WorkspaceID:      workspaceID,
		AccountID:        accountID,
		Role:             role,
		RoleID:           roleID,
		Permissions:      permissions,
		PermissionSource: source,
	}
}

type workspacePermissionFilterOrganizationRepo struct {
	repository.OrganizationRepository
	fixture *workspacePermissionFilterFixture
}

func (r *workspacePermissionFilterOrganizationRepo) GetByID(ctx context.Context, id string) (*model.Organization, error) {
	if id != r.fixture.organizationID {
		return nil, nil
	}
	return r.fixture.organization, nil
}

func (r *workspacePermissionFilterOrganizationRepo) GetAccountJoin(ctx context.Context, organizationID, accountID string) (*model.OrganizationMember, error) {
	if organizationID != r.fixture.organizationID {
		return nil, nil
	}
	return r.fixture.orgMembers[accountID], nil
}

func (r *workspacePermissionFilterOrganizationRepo) GetWorkspacesByOrganizationID(ctx context.Context, organizationID string) ([]*model.Workspace, error) {
	if organizationID != r.fixture.organizationID {
		return nil, nil
	}

	workspaces := make([]*model.Workspace, len(r.fixture.workspaceOrder))
	copy(workspaces, r.fixture.workspaceOrder)
	return workspaces, nil
}

type workspacePermissionFilterWorkspaceRepo struct {
	repository.WorkspaceRepository
	fixture *workspacePermissionFilterFixture
}

func (r *workspacePermissionFilterWorkspaceRepo) GetByIDs(ctx context.Context, ids []string) ([]*model.Workspace, error) {
	workspaces := make([]*model.Workspace, 0, len(ids))
	for _, id := range ids {
		if workspace := r.fixture.workspaces[id]; workspace != nil {
			workspaces = append(workspaces, workspace)
		}
	}
	return workspaces, nil
}

type workspacePermissionFilterMemberRepo struct {
	repository.WorkspaceMemberRepository
	fixture *workspacePermissionFilterFixture
}

func (r *workspacePermissionFilterMemberRepo) GetByWorkspaceAndMember(ctx context.Context, workspaceID, memberID string) (*model.WorkspaceMember, error) {
	return r.fixture.workspaceJoins[workspaceMemberKey(workspaceID, memberID)], nil
}

func workspaceMemberKey(workspaceID, accountID string) string {
	return workspaceID + ":" + accountID
}

func workspacePermissionResponseIDs(workspaces []*WorkspacePermissionResponse) []string {
	ids := make([]string, 0, len(workspaces))
	for _, workspace := range workspaces {
		ids = append(ids, workspace.ID)
	}
	return ids
}
