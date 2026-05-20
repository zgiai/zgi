package service

import (
	"context"

	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	auth_repo "github.com/zgiai/zgi/api/internal/modules/user/auth/repository"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

type AccountTenantService struct {
	accountRepo   auth_repo.AccountRepository
	tenantService interfaces.WorkspaceManagementService
}

func NewAccountTenantService(
	accountRepo auth_repo.AccountRepository,
	tenantService interfaces.WorkspaceManagementService,
) *AccountTenantService {
	return &AccountTenantService{
		accountRepo:   accountRepo,
		tenantService: tenantService,
	}
}

func (s *AccountTenantService) GetAccountWorkspaces(ctx context.Context, accountID string) ([]*workspace_model.Workspace, error) {
	return s.tenantService.GetAccountWorkspaces(ctx, accountID)
}

func (s *AccountTenantService) GetTenantAccounts(ctx context.Context, tenantID string) ([]*auth_model.Account, error) {
	members, err := s.tenantService.GetWorkspaceMembers(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	accounts := make([]*auth_model.Account, 0, len(members))
	for _, member := range members {
		account, err := s.accountRepo.GetAccountById(ctx, member.ID)
		if err != nil {
			continue
		}
		accounts = append(accounts, account)
	}

	return accounts, nil
}

func (s *AccountTenantService) JoinTenantAccount(ctx context.Context, tenantID, accountID, role string) error {
	return s.tenantService.CreateWorkspaceMember(ctx, tenantID, accountID, role)
}

func (s *AccountTenantService) LeaveTenantAccount(ctx context.Context, tenantID, accountID string) error {
	return s.tenantService.RemoveMember(ctx, tenantID, accountID)
}
