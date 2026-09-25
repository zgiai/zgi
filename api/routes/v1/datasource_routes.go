package v1

import (
	"github.com/zgiai/zgi/api/internal/modules/datasource/handler"
	"github.com/zgiai/zgi/api/internal/modules/datasource/indexing"
	datasourceservice "github.com/zgiai/zgi/api/internal/modules/datasource/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/apperror/catalog"

	"github.com/gin-gonic/gin"
)

// DataSourceRouteDeps contains dependencies required by data source routes.
type DataSourceRouteDeps struct {
	SchemaIndexes           *indexing.Service
	ApplicationErrorCatalog *catalog.Catalog
	DataSourceService       datasourceservice.DataSourceService
	AccountService          interfaces.AccountService
	OrganizationService     interfaces.OrganizationService
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
	if deps.SchemaIndexes != nil {
		h := handler.NewSchemaIndexHandler(deps.SchemaIndexes, deps.ApplicationErrorCatalog)
		auth := router.Group("", middleware.JWTWithOrganizationAndService(deps.AccountService))
		auth.GET("/data-dbs/:id/schema-index", h.Status)
		auth.GET("/data-dbs/:id/schema-index/runs", h.History)
		auth.POST("/data-dbs/:id/schema-index", h.Start)
	}
}
