package v1

import (
	"github.com/gin-gonic/gin"

	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	systemHandler "github.com/zgiai/zgi/api/internal/modules/system/handler"
	"github.com/zgiai/zgi/api/internal/modules/system/service"
	"github.com/zgiai/zgi/api/middleware"
	"gorm.io/gorm"
)

// DashboardRouteDeps contains dependencies required by dashboard routes.
type DashboardRouteDeps struct {
	DB                  *gorm.DB
	AccountService      interfaces.AccountService
	OrganizationService interfaces.OrganizationService
	AvailableModels     service.AvailableModelsLister
}

// RegisterDashboardRoutes registers dashboard related routes
func RegisterDashboardRoutes(v1 *gin.RouterGroup, deps DashboardRouteDeps) {
	if deps.DB == nil {
		panic("dashboard routes require db")
	}
	if deps.AccountService == nil {
		panic("dashboard routes require account service")
	}
	if deps.OrganizationService == nil {
		panic("dashboard routes require organization service")
	}
	if deps.AvailableModels == nil {
		panic("dashboard routes require available models service")
	}

	dashboardService := service.NewDashboardServiceWithAvailableModels(deps.DB, deps.AvailableModels)
	dashboardHandler := systemHandler.NewDashboardHandler(dashboardService, deps.OrganizationService)

	// Dashboard routes - requires authentication and tenant context
	dashboard := v1.Group("/dashboard")
	dashboard.Use(middleware.JWTWithOrganizationAndService(deps.AccountService))
	{
		dashboard.GET("/stats", dashboardHandler.GetDashboardStats)
		dashboard.GET("/recent-work", dashboardHandler.GetRecentWork)
	}
}
