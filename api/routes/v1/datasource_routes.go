package v1

import (
	"github.com/zgiai/ginext/internal/container"
	"github.com/zgiai/ginext/internal/modules/datasource/handler"

	"github.com/gin-gonic/gin"
)

// RegisterDataSourceRoutes registers data source routes
func RegisterDataSourceRoutes(router *gin.RouterGroup, container *container.ServiceContainer) {
	dataSourceHandler := handler.NewDataSourceHandler(
		container.GetDataSourceService(),
		container.GetAccountService(),
		container.GetOrganizationService(),
	)
	dataSourceHandler.RegisterRoutes(router)
}
