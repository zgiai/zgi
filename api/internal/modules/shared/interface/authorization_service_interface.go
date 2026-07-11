package interfaces

import (
	"context"

	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

// AuthorizationService centralizes organization and workspace scope checks.
type AuthorizationService interface {
	RequireOrganizationMember(ctx context.Context, req OrganizationScopeRequest) (*OrganizationScope, error)
	CanUseOrganizationFeature(ctx context.Context, req OrganizationFeatureRequest) (bool, error)
	RequireWorkspacePermission(ctx context.Context, req WorkspaceScopeRequest) (*WorkspaceScope, error)
	ListWorkspaceIDsByPermission(ctx context.Context, req WorkspaceListPermissionRequest) ([]string, error)
}

type OrganizationScopeRequest struct {
	OrganizationID string
	AccountID      string
}

type OrganizationFeatureRequest struct {
	OrganizationID string
	AccountID      string
	Feature        string
}

type OrganizationScope struct {
	OrganizationID string
	AccountID      string
	Role           model.OrganizationRole
	IsAdmin        bool
}

type WorkspaceScopeRequest struct {
	OrganizationID  string
	WorkspaceID     string
	AccountID       string
	PermissionCodes []model.WorkspacePermissionCode
}

type WorkspaceScope struct {
	OrganizationScope
	WorkspaceID      string
	PermissionCodes  []model.WorkspacePermissionCode
	WorkspaceIsAdmin bool
}

type WorkspaceListPermissionRequest struct {
	OrganizationID string
	AccountID      string
	PermissionCode model.WorkspacePermissionCode
}
