package service

import (
	"context"
	"fmt"

	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

type crossDomainService struct {
	accountService    interfaces.AccountService
	tenantService     interfaces.WorkspaceManagementService
	enterpriseService interfaces.OrganizationService
}

func NewCrossDomainService(
	accountService interfaces.AccountService,
	tenantService interfaces.WorkspaceManagementService,
	enterpriseService interfaces.OrganizationService,
) interfaces.CrossDomainService {
	return &crossDomainService{
		accountService:    accountService,
		tenantService:     tenantService,
		enterpriseService: enterpriseService,
	}
}

func (s *crossDomainService) GetAccountWithTenantRole(ctx context.Context, email, workspaceID string) (*interfaces.AccountWithTenantRole, error) {
	account, err := s.accountService.GetAccountByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("failed to get account by email: %w", err)
	}

	joins, err := s.tenantService.GetAccountWorkspaceJoins(ctx, account.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get account tenant joins: %w", err)
	}

	for _, join := range joins {
		if join.WorkspaceID == workspaceID {
			return &interfaces.AccountWithTenantRole{
				Account: account,
				Role:    join.Role,
			}, nil
		}
	}

	return nil, fmt.Errorf("account not found in tenant")
}

func (s *crossDomainService) GetTenantsByAccountID(ctx context.Context, accountID string) ([]*workspace_model.Workspace, error) {
	joins, err := s.tenantService.GetAccountWorkspaceJoins(ctx, accountID)
	if err != nil {
		return nil, err
	}

	workspaceIDs := make([]string, len(joins))
	for i, join := range joins {
		workspaceIDs[i] = join.WorkspaceID
	}

	workspaces, err := s.tenantService.GetWorkspacesByIDs(ctx, workspaceIDs)
	if err != nil {
		return nil, err
	}

	return workspaces, nil
}

func (s *crossDomainService) GetAccountsByTenantID(ctx context.Context, workspaceID string) ([]*auth_model.Account, error) {
	joins, err := s.tenantService.GetWorkspaceAccountJoins(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	var accounts []*auth_model.Account
	for _, join := range joins {
		account, err := s.accountService.GetAccountByID(ctx, join.AccountID)
		if err != nil {
			continue // Skip failed account retrieval
		}
		accounts = append(accounts, account)
	}

	return accounts, nil
}

func (s *crossDomainService) GetUserGroupsForAccount(ctx context.Context, accountID, tenantID string) ([]*workspace_model.Organization, error) {
	resp, err := s.enterpriseService.GetUserOrganizationsByAccount(ctx, accountID, 1, 100)
	if err != nil {
		return nil, err
	}

	var groups []*workspace_model.Organization
	for _, groupWithRole := range resp.Data {
		group := &workspace_model.Organization{
			ID:        groupWithRole.ID,
			Name:      groupWithRole.Name,
			ShortName: groupWithRole.ShortName,
			Status:    groupWithRole.Status,
		}
		groups = append(groups, group)
	}

	return groups, nil
}

func (s *crossDomainService) GetAccountEnterpriseRole(ctx context.Context, accountID string) (*workspace_model.Organization, error) {
	resp, err := s.enterpriseService.GetCurrentOrganization(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, nil
	}

	enterpriseGroup := &workspace_model.Organization{
		ID:        resp.ID,
		Name:      resp.Name,
		ShortName: resp.ShortName,
		Status:    resp.Status,
	}

	return enterpriseGroup, nil
}

func (s *crossDomainService) GetAccountEnterpriseRoleByTenantID(ctx context.Context, accountID, workspaceID string) (*workspace_model.Organization, error) {
	group, err := s.enterpriseService.GetOrganizationByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return group, nil
}

func (s *crossDomainService) GetJoinWorkspaces(ctx context.Context, account *auth_model.Account) ([]*workspace_model.Workspace, error) {
	return s.tenantService.GetJoinWorkspaces(ctx, account)
}

func (s *crossDomainService) CheckDatasetPermissionForAccount(ctx context.Context, datasetID, accountID, tenantID string) (bool, error) {
	//
	return true, nil
}
