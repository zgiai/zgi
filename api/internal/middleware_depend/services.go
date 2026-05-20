package middleware_depend

import (
	"context"

	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"

	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
)

type ServiceProvider interface {
	GetAccountService() AccountServiceProvider
	GetTenantService() TenantServiceProvider
	GetOrganizationService() EnterpriseServiceProvider
	GetSystemService() SystemServiceProvider
}

type AccountServiceProvider interface {
	GetAccountByID(ctx context.Context, accountID string) (*auth_model.Account, error)
	LoadUser(ctx context.Context, userID string) (*auth_model.Account, error)
	ExistsByEmail(ctx context.Context, email string) bool

	LoadLoggedInAccount(ctx context.Context, accountID string) (*auth_model.Account, error)
}

type TenantServiceProvider interface {
	GetWorkspaceByID(ctx context.Context, id string) (*model.Workspace, error)
	GetCurrentWorkspace(ctx context.Context, accountID string) (interface{}, error)
}

type EnterpriseServiceProvider interface {
	CheckLicense(ctx context.Context) (bool, error)
	GetLicenseInfo(ctx context.Context) (interface{}, error)
	HasValidLicense(ctx context.Context) bool
}

type SystemServiceProvider interface {
	IsSetupRequired() bool
	GetSetupInfo(ctx context.Context) (interface{}, error)
	IsSystemInitialized(ctx context.Context) bool
}

type DefaultServiceProvider struct {
	accountService    AccountServiceProvider
	tenantService     TenantServiceProvider
	enterpriseService EnterpriseServiceProvider
	systemService     SystemServiceProvider
}

func NewServiceProvider(
	accountService interfaces.AccountService,
	tenantService interfaces.WorkspaceManagementService,
	enterpriseService interface{},
	systemService interface{},
) ServiceProvider {
	return &DefaultServiceProvider{
		accountService:    &AccountServiceAdapter{accountService: accountService},
		tenantService:     &TenantServiceAdapter{tenantService: tenantService},
		enterpriseService: &EnterpriseServiceAdapter{enterpriseService: enterpriseService},
		systemService:     &SystemServiceAdapter{systemService: systemService},
	}
}

func (p *DefaultServiceProvider) GetAccountService() AccountServiceProvider {
	return p.accountService
}

func (p *DefaultServiceProvider) GetTenantService() TenantServiceProvider {
	return p.tenantService
}

func (p *DefaultServiceProvider) GetOrganizationService() EnterpriseServiceProvider {
	return p.enterpriseService
}

func (p *DefaultServiceProvider) GetSystemService() SystemServiceProvider {
	return p.systemService
}

type AccountServiceAdapter struct {
	accountService interfaces.AccountService
}

func (a *AccountServiceAdapter) GetAccountByID(ctx context.Context, accountID string) (*auth_model.Account, error) {
	return a.accountService.GetAccountByID(ctx, accountID)
}

func (a *AccountServiceAdapter) LoadUser(ctx context.Context, userID string) (*auth_model.Account, error) {
	return a.accountService.LoadUser(ctx, userID)
}

func (a *AccountServiceAdapter) ExistsByEmail(ctx context.Context, email string) bool {
	return a.accountService.ExistsByEmail(ctx, email)
}

func (a *AccountServiceAdapter) LoadLoggedInAccount(ctx context.Context, accountID string) (*auth_model.Account, error) {
	return a.accountService.LoadLoggedInAccount(ctx, accountID)
}

type TenantServiceAdapter struct {
	tenantService interfaces.WorkspaceManagementService
}

func (t *TenantServiceAdapter) GetWorkspaceByID(ctx context.Context, id string) (*model.Workspace, error) {
	return t.tenantService.GetWorkspaceByID(ctx, id)
}

func (t *TenantServiceAdapter) GetCurrentWorkspace(ctx context.Context, accountID string) (interface{}, error) {
	return t.tenantService.GetCurrentWorkspace(ctx, accountID)
}

type EnterpriseServiceAdapter struct {
	enterpriseService interface{}
}

func (e *EnterpriseServiceAdapter) CheckLicense(ctx context.Context) (bool, error) {
	return true, nil
}

func (e *EnterpriseServiceAdapter) GetLicenseInfo(ctx context.Context) (interface{}, error) {
	return nil, nil
}

func (e *EnterpriseServiceAdapter) HasValidLicense(ctx context.Context) bool {
	return true
}

type SystemServiceAdapter struct {
	systemService interface{}
}

func (s *SystemServiceAdapter) IsSetupRequired() bool {
	return false
}

func (s *SystemServiceAdapter) GetSetupInfo(ctx context.Context) (interface{}, error) {
	return nil, nil
}

func (s *SystemServiceAdapter) IsSystemInitialized(ctx context.Context) bool {
	return true
}
