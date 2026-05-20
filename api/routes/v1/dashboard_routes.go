package v1

import (
	"github.com/gin-gonic/gin"

	"github.com/zgiai/zgi/api/internal/container"
	"github.com/zgiai/zgi/api/internal/modules/llm"
	systemHandler "github.com/zgiai/zgi/api/internal/modules/system/handler"
	"github.com/zgiai/zgi/api/internal/modules/system/service"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/database"
)

// RegisterDashboardRoutes registers dashboard related routes
func RegisterDashboardRoutes(v1 *gin.RouterGroup, serviceContainer *container.ServiceContainer, llmModule *llm.LLMModule) {
	db := database.GetDB()

	// Create dashboard service and handler
	var availableModels service.AvailableModelsLister
	if llmModule != nil && llmModule.LLMModelModule != nil {
		availableModels = llmModule.LLMModelModule.AvailableModelsSvc
	}
	dashboardService := service.NewDashboardServiceWithAvailableModels(db, availableModels)
	enterpriseService := serviceContainer.GetOrganizationService()
	dashboardHandler := systemHandler.NewDashboardHandler(dashboardService, enterpriseService)

	// Get services for middleware
	accountService := serviceContainer.GetAccountServiceAdapter()

	// Dashboard routes - requires authentication and tenant context
	dashboard := v1.Group("/dashboard")
	dashboard.Use(middleware.JWTWithOrganizationAndService(accountService))
	{
		dashboard.GET("/stats", dashboardHandler.GetDashboardStats)
		dashboard.GET("/recent-work", dashboardHandler.GetRecentWork)
	}
}
