package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

func TestAuthorizationServiceAllowsOrganizationFeatureForMemberWithoutWorkspace(t *testing.T) {
	t.Parallel()

	org := newAuthorizationFixture()
	org.members["org-1:acc-1"] = model.OrganizationRoleNormal
	svc := NewAuthorizationService(org)

	allowed, err := svc.CanUseOrganizationFeature(context.Background(), interfaces.OrganizationFeatureRequest{
		OrganizationID: "org-1",
		AccountID:      "acc-1",
		Feature:        "work.chat",
	})
	if err != nil {
		t.Fatalf("CanUseOrganizationFeature error = %v", err)
	}
	if !allowed {
		t.Fatalf("CanUseOrganizationFeature allowed = false, want true")
	}
}

func TestAuthorizationServiceDeniesOrganizationFeatureForNonMember(t *testing.T) {
	t.Parallel()

	svc := NewAuthorizationService(newAuthorizationFixture())

	allowed, err := svc.CanUseOrganizationFeature(context.Background(), interfaces.OrganizationFeatureRequest{
		OrganizationID: "org-1",
		AccountID:      "acc-1",
		Feature:        "work.chat",
	})
	if err != nil {
		t.Fatalf("CanUseOrganizationFeature error = %v", err)
	}
	if allowed {
		t.Fatalf("CanUseOrganizationFeature allowed = true, want false")
	}
}

func TestAuthorizationServiceDeniesInactiveOrganizationMember(t *testing.T) {
	t.Parallel()

	org := newAuthorizationFixture()
	key := "org-1:acc-1"
	org.members[key] = model.OrganizationRoleAdmin
	org.inactiveMembers[key] = true
	svc := NewAuthorizationService(org)

	allowed, err := svc.CanUseOrganizationFeature(context.Background(), interfaces.OrganizationFeatureRequest{
		OrganizationID: "org-1",
		AccountID:      "acc-1",
		Feature:        "work.chat",
	})
	if err != nil {
		t.Fatalf("CanUseOrganizationFeature error = %v", err)
	}
	if allowed {
		t.Fatalf("CanUseOrganizationFeature allowed = true, want false")
	}
}

func TestAuthorizationServiceRequiresWorkspacePermission(t *testing.T) {
	t.Parallel()

	org := newAuthorizationFixture()
	org.members["org-1:acc-1"] = model.OrganizationRoleNormal
	org.workspaces["org-1"] = []*model.Workspace{
		{ID: "ws-1", Status: model.WorkspaceStatusNormal},
	}
	org.workspacePermissions["org-1:ws-1:acc-1"] = []model.WorkspacePermissionCode{
		model.WorkspacePermissionDatabaseManage,
	}
	svc := NewAuthorizationService(org)

	scope, err := svc.RequireWorkspacePermission(context.Background(), interfaces.WorkspaceScopeRequest{
		OrganizationID:  "org-1",
		WorkspaceID:     "ws-1",
		AccountID:       "acc-1",
		PermissionCodes: []model.WorkspacePermissionCode{model.WorkspacePermissionDatabaseManage},
	})
	if err != nil {
		t.Fatalf("RequireWorkspacePermission error = %v", err)
	}
	if scope.WorkspaceID != "ws-1" {
		t.Fatalf("WorkspaceID = %q, want ws-1", scope.WorkspaceID)
	}
	if scope.WorkspaceIsAdmin {
		t.Fatalf("WorkspaceIsAdmin = true, want false")
	}
}

func TestAuthorizationServiceDeniesOrganizationAdminWithoutWorkspacePermission(t *testing.T) {
	t.Parallel()

	org := newAuthorizationFixture()
	org.members["org-1:admin-1"] = model.OrganizationRoleAdmin
	org.workspaces["org-1"] = []*model.Workspace{
		{ID: "ws-1", Status: model.WorkspaceStatusNormal},
	}
	svc := NewAuthorizationService(org)

	_, err := svc.RequireWorkspacePermission(context.Background(), interfaces.WorkspaceScopeRequest{
		OrganizationID:  "org-1",
		WorkspaceID:     "ws-1",
		AccountID:       "admin-1",
		PermissionCodes: []model.WorkspacePermissionCode{model.WorkspacePermissionAgentManage},
	})
	if !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("RequireWorkspacePermission error = %v, want ErrAuthorizationDenied", err)
	}
}

func TestAuthorizationServiceAllowsOrganizationAdminWithWorkspacePermission(t *testing.T) {
	t.Parallel()

	org := newAuthorizationFixture()
	org.members["org-1:admin-1"] = model.OrganizationRoleAdmin
	org.workspaces["org-1"] = []*model.Workspace{
		{ID: "ws-1", Status: model.WorkspaceStatusNormal},
	}
	org.workspacePermissions["org-1:ws-1:admin-1"] = []model.WorkspacePermissionCode{
		model.WorkspacePermissionAgentManage,
	}
	svc := NewAuthorizationService(org)

	scope, err := svc.RequireWorkspacePermission(context.Background(), interfaces.WorkspaceScopeRequest{
		OrganizationID:  "org-1",
		WorkspaceID:     "ws-1",
		AccountID:       "admin-1",
		PermissionCodes: []model.WorkspacePermissionCode{model.WorkspacePermissionAgentManage},
	})
	if err != nil {
		t.Fatalf("RequireWorkspacePermission error = %v", err)
	}
	if scope.WorkspaceID != "ws-1" {
		t.Fatalf("WorkspaceID = %q, want ws-1", scope.WorkspaceID)
	}
}

func TestAuthorizationServiceFailsClosedForMissingOrArchivedWorkspace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		workspaces []*model.Workspace
	}{
		{name: "missing", workspaces: []*model.Workspace{{ID: "ws-other", Status: model.WorkspaceStatusNormal}}},
		{name: "archived", workspaces: []*model.Workspace{{ID: "ws-1", Status: model.WorkspaceStatusArchived}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			org := newAuthorizationFixture()
			org.members["org-1:acc-1"] = model.OrganizationRoleAdmin
			org.workspaces["org-1"] = tt.workspaces
			svc := NewAuthorizationService(org)

			_, err := svc.RequireWorkspacePermission(context.Background(), interfaces.WorkspaceScopeRequest{
				OrganizationID:  "org-1",
				WorkspaceID:     "ws-1",
				AccountID:       "acc-1",
				PermissionCodes: []model.WorkspacePermissionCode{model.WorkspacePermissionAgentManage},
			})
			if !errors.Is(err, ErrAuthorizationDenied) {
				t.Fatalf("RequireWorkspacePermission error = %v, want ErrAuthorizationDenied", err)
			}
		})
	}
}

func TestAuthorizationServiceListsWorkspaceIDsByPermission(t *testing.T) {
	t.Parallel()

	org := newAuthorizationFixture()
	org.members["org-1:acc-1"] = model.OrganizationRoleNormal
	org.listWorkspaceIDs = []string{"ws-1", "ws-2"}
	svc := NewAuthorizationService(org)

	ids, err := svc.ListWorkspaceIDsByPermission(context.Background(), interfaces.WorkspaceListPermissionRequest{
		OrganizationID: "org-1",
		AccountID:      "acc-1",
		PermissionCode: model.WorkspacePermissionFileView,
	})
	if err != nil {
		t.Fatalf("ListWorkspaceIDsByPermission error = %v", err)
	}
	if !reflect.DeepEqual(ids, []string{"ws-1", "ws-2"}) {
		t.Fatalf("workspace ids = %#v, want %#v", ids, []string{"ws-1", "ws-2"})
	}
	if org.lastListPermission != model.WorkspacePermissionFileView {
		t.Fatalf("lastListPermission = %q, want %q", org.lastListPermission, model.WorkspacePermissionFileView)
	}
}

type authorizationFixture struct {
	interfaces.OrganizationService
	members              map[string]model.OrganizationRole
	inactiveMembers      map[string]bool
	workspaces           map[string][]*model.Workspace
	workspacePermissions map[string][]model.WorkspacePermissionCode
	listWorkspaceIDs     []string
	lastListPermission   model.WorkspacePermissionCode
}

func newAuthorizationFixture() *authorizationFixture {
	return &authorizationFixture{
		members:              map[string]model.OrganizationRole{},
		inactiveMembers:      map[string]bool{},
		workspaces:           map[string][]*model.Workspace{},
		workspacePermissions: map[string][]model.WorkspacePermissionCode{},
	}
}

func (f *authorizationFixture) IsOrganizationMember(ctx context.Context, organizationID, accountID string) (bool, error) {
	_, ok := f.members[organizationID+":"+accountID]
	return ok, nil
}

func (f *authorizationFixture) GetOrganizationMemberByAccountID(ctx context.Context, organizationID, accountID string) (*dto.OrganizationMemberWithExtensionResponse, error) {
	key := organizationID + ":" + accountID
	role, ok := f.members[key]
	if !ok {
		return nil, nil
	}
	status := model.OrganizationMemberStatusActive
	if f.inactiveMembers[key] {
		status = model.OrganizationMemberStatusInactive
	}
	return &dto.OrganizationMemberWithExtensionResponse{
		ID:               accountID,
		Status:           string(status),
		OrganizationRole: role,
	}, nil
}

func (f *authorizationFixture) GetUserOrganizationRole(ctx context.Context, organizationID, accountID string) (model.OrganizationRole, error) {
	role, ok := f.members[organizationID+":"+accountID]
	if !ok {
		return "", nil
	}
	return role, nil
}

func (f *authorizationFixture) GetOrganizationWorkspacesList(ctx context.Context, organizationID string) ([]*model.Workspace, error) {
	workspaces := f.workspaces[organizationID]
	result := make([]*model.Workspace, len(workspaces))
	copy(result, workspaces)
	return result, nil
}

func (f *authorizationFixture) CheckWorkspaceOrganizationAnyPermission(
	ctx context.Context,
	organizationID, workspaceID, accountID string,
	permissionCodes ...model.WorkspacePermissionCode,
) (bool, error) {
	grants := f.workspacePermissions[organizationID+":"+workspaceID+":"+accountID]
	for _, grant := range grants {
		for _, permissionCode := range permissionCodes {
			if grant == permissionCode {
				return true, nil
			}
		}
	}
	return false, nil
}

func (f *authorizationFixture) ListWorkspaceIDsByPermission(
	ctx context.Context,
	organizationID, accountID string,
	permissionCode model.WorkspacePermissionCode,
) ([]string, error) {
	f.lastListPermission = permissionCode
	ids := make([]string, len(f.listWorkspaceIDs))
	copy(ids, f.listWorkspaceIDs)
	return ids, nil
}
