package interfaces

import (
	"context"

	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

type CrossDomainService interface {
	GetAccountWithTenantRole(ctx context.Context, email, tenantID string) (*AccountWithTenantRole, error)

	GetTenantsByAccountID(ctx context.Context, accountID string) ([]*workspace_model.Workspace, error)

	GetAccountsByTenantID(ctx context.Context, tenantID string) ([]*auth_model.Account, error)

	GetUserGroupsForAccount(ctx context.Context, accountID, tenantID string) ([]*workspace_model.Organization, error)

	GetAccountEnterpriseRole(ctx context.Context, accountID string) (*workspace_model.Organization, error)

	GetAccountEnterpriseRoleByTenantID(ctx context.Context, accountID, tenantID string) (*workspace_model.Organization, error)

	GetJoinWorkspaces(ctx context.Context, account *auth_model.Account) ([]*workspace_model.Workspace, error)

	CheckDatasetPermissionForAccount(ctx context.Context, accountID, datasetID, permission string) (bool, error)
}

type AccountWithTenantRole struct {
	Account *auth_model.Account                 `json:"account"`
	Role    workspace_model.WorkspaceMemberRole `json:"role"`
}
