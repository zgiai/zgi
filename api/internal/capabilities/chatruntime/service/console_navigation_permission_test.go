package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin/consolenavigation"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

type consoleNavigationPermissionService struct {
	allowed         map[workspacemodel.WorkspacePermissionCode]bool
	checks          []workspacemodel.WorkspacePermissionCode
	organizationIDs []string
	workspaceIDs    []string
	accountIDs      []string
}

func (s *consoleNavigationPermissionService) CheckWorkspacePermission(
	_ context.Context,
	organizationID string,
	workspaceID string,
	accountID string,
	code workspacemodel.WorkspacePermissionCode,
) (bool, error) {
	s.checks = append(s.checks, code)
	s.organizationIDs = append(s.organizationIDs, organizationID)
	s.workspaceIDs = append(s.workspaceIDs, workspaceID)
	s.accountIDs = append(s.accountIDs, accountID)
	return s.allowed[code], nil
}

func TestConsoleNavigationRouteAuthorizerUsesPreparedScopeAndAnyPermission(t *testing.T) {
	organizationID := uuid.New()
	workspaceID := uuid.New()
	accountID := uuid.New()
	permissions := &consoleNavigationPermissionService{
		allowed: map[workspacemodel.WorkspacePermissionCode]bool{
			workspacemodel.WorkspacePermissionWorkflowUpdate: true,
		},
	}
	authorizer := (&service{workspacePerms: permissions}).consoleNavigationRouteAuthorizer(
		&PreparedChat{Scope: Scope{
			OrganizationID: organizationID,
			WorkspaceID:    &workspaceID,
			AccountID:      accountID,
		}},
	)
	if authorizer == nil {
		t.Fatal("consoleNavigationRouteAuthorizer() = nil, want authorizer")
	}

	err := authorizer(t.Context(), consolenavigation.RouteAuthorizationRequest{
		Href:        "/console/workflows/workflow-1",
		WorkspaceID: workspaceID.String(),
		PermissionCodes: []workspacemodel.WorkspacePermissionCode{
			workspacemodel.WorkspacePermissionWorkflowView,
			workspacemodel.WorkspacePermissionWorkflowUpdate,
		},
	})
	if err != nil {
		t.Fatalf("authorizer returned error: %v", err)
	}
	if len(permissions.checks) != 2 ||
		permissions.organizationIDs[0] != organizationID.String() ||
		permissions.workspaceIDs[0] != workspaceID.String() ||
		permissions.accountIDs[0] != accountID.String() {
		t.Fatalf("permission checks did not use prepared scope: %#v", permissions)
	}
}

func TestConsoleNavigationRouteAuthorizerUsesWorkspaceViewForUnscopedWorkspacePage(t *testing.T) {
	workspaceID := uuid.New()
	permissions := &consoleNavigationPermissionService{
		allowed: map[workspacemodel.WorkspacePermissionCode]bool{
			workspacemodel.WorkspacePermissionWorkspaceView: true,
		},
	}
	authorizer := (&service{workspacePerms: permissions}).consoleNavigationRouteAuthorizer(
		&PreparedChat{Scope: Scope{
			OrganizationID: uuid.New(),
			WorkspaceID:    &workspaceID,
			AccountID:      uuid.New(),
		}},
	)

	if err := authorizer(t.Context(), consolenavigation.RouteAuthorizationRequest{
		Href:        "/console/files",
		WorkspaceID: workspaceID.String(),
	}); err != nil {
		t.Fatalf("authorizer returned error: %v", err)
	}
	if len(permissions.checks) != 1 ||
		permissions.checks[0] != workspacemodel.WorkspacePermissionWorkspaceView {
		t.Fatalf("permission checks = %#v, want workspace.view", permissions.checks)
	}
}

func TestConsoleNavigationRouteAuthorizerRejectsWorkspaceMismatchBeforePermissionCheck(t *testing.T) {
	workspaceID := uuid.New()
	permissions := &consoleNavigationPermissionService{allowed: map[workspacemodel.WorkspacePermissionCode]bool{}}
	authorizer := (&service{workspacePerms: permissions}).consoleNavigationRouteAuthorizer(
		&PreparedChat{Scope: Scope{
			OrganizationID: uuid.New(),
			WorkspaceID:    &workspaceID,
			AccountID:      uuid.New(),
		}},
	)

	err := authorizer(t.Context(), consolenavigation.RouteAuthorizationRequest{
		Href:        "/console/files",
		WorkspaceID: uuid.NewString(),
	})
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("authorizer error = %v, want ErrPermissionDenied", err)
	}
	if len(permissions.checks) != 0 {
		t.Fatalf("permission checks = %#v, want none for workspace mismatch", permissions.checks)
	}
}
