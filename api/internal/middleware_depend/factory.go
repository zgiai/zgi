package middleware_depend

import (
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
)

func CreateServiceProviderFromServices(
	accountService interfaces.AccountService,
	tenantService interfaces.WorkspaceManagementService,
	enterpriseService interface{},
	systemService interface{},
) ServiceProvider {
	return NewServiceProvider(
		accountService,
		tenantService,
		enterpriseService,
		systemService,
	)
}

type ProviderConfig struct {
	AccountService    interfaces.AccountService
	TenantService     interfaces.WorkspaceManagementService
	EnterpriseService interface{}
	SystemService     interface{}
}

func NewServiceProviderFromConfig(config *ProviderConfig) ServiceProvider {
	return NewServiceProvider(
		config.AccountService,
		config.TenantService,
		config.EnterpriseService,
		config.SystemService,
	)
}
