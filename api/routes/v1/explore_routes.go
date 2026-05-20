package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/internal/modules/explore/handler"
	"github.com/zgiai/ginext/internal/modules/explore/service"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
)

// RegisterExploreRoutes registers explore-related routes
func RegisterExploreRoutes(router *gin.RouterGroup, accountService interfaces.AccountService) {
	recommendedAppService := service.NewRecommendedAppService()
	recommendedAppHandler := handler.NewRecommendedAppHandler(recommendedAppService, accountService)
	recommendedAppHandler.RegisterRoutes(router)
}
