package v1

import (
	"github.com/zgiai/zgi/api/internal/modules/datasource/handler"
	datasourceservice "github.com/zgiai/zgi/api/internal/modules/datasource/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"

	"github.com/gin-gonic/gin"
)

// DataSourceRouteDeps contains dependencies required by data source routes.
type DataSourceRouteDeps struct {
	DataSourceService   datasourceservice.DataSourceService
	AccountService      interfaces.AccountService
	OrganizationService interfaces.OrganizationService
}

// RegisterDataSourceRoutes registers data source routes
func RegisterDataSourceRoutes(router *gin.RouterGroup, deps DataSourceRouteDeps) {
	if deps.DataSourceService == nil {
		panic("data source routes require data source service")
	}
	if deps.AccountService == nil {
		panic("data source routes require account service")
	}
	if deps.OrganizationService == nil {
		panic("data source routes require organization service")
	}

	dataSourceHandler := handler.NewDataSourceHandler(
		deps.DataSourceService,
		deps.AccountService,
		deps.OrganizationService,
	)
	dataSourceHandler.RegisterRoutes(router)
}
