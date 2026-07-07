package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/response"
)

const adjustChannelWalletPermissionMessage = "only organization admin or owner can adjust private channel wallet"

// AdjustChannelWalletAdminRequired enforces admin/owner permission with route-specific error message.
func AdjustChannelWalletAdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !middleware.IsOrganizationAdminOrOwner(c) {
			response.FailWithMessage(c, response.ErrPermissionDenied, adjustChannelWalletPermissionMessage)
			c.Abort()
			return
		}
		c.Next()
	}
}

// RegisterTenantChannelRoutes registers tenant routes for channel management
func RegisterTenantChannelRoutes(r *gin.RouterGroup, handler *ChannelHandler) {
	g := r.Group("/channels")
	admin := g.Group("")
	admin.Use(middleware.EnterpriseAdminOrOwnerRequired())

	admin.POST("", handler.CreateRoute)
	g.GET("", handler.ListRoutesAggregated)
	g.GET("/all", handler.ListRoutes)
	g.GET("/platform", handler.GetPlatformChannel)
	admin.PUT("/platform", handler.UpdatePlatformChannelSettings)
	admin.PATCH("/platform/:id", handler.UpdatePlatformChannel)
	g.GET("/platform/models", handler.ListPlatformChannelModels)
	g.GET("/providers", handler.GetAvailableProviders)
	g.POST("/select", handler.SelectRoute)
	g.GET("/by-model", handler.GetRoutesForModel)
	admin.POST("/init", handler.InitTenantRoutes)
	admin.POST("/draft/discover-models", handler.DiscoverDraftChannelModels)
	admin.POST("/draft/test/model", handler.TestDraftChannelModel)
	admin.POST("/ollama/discover-models", handler.DiscoverOllamaModels)
	admin.POST("/official/init", handler.InitOfficialChannel)
	admin.POST("/batch/toggle", handler.BatchToggleRoutes)
	admin.POST("/batch/delete", handler.BatchDeleteRoutes)
	g.GET("/:id", handler.GetRoute)
	admin.PUT("/:id", handler.UpdateRoute)
	admin.DELETE("/:id", handler.DeleteRoute)
	admin.POST("/:id/toggle", handler.ToggleRoute)
	admin.POST("/:id/test", handler.TestRoute)
	gAdmin := g.Group("")
	gAdmin.Use(AdjustChannelWalletAdminRequired())
	gAdmin.POST("/:id/wallet/adjust", handler.AdjustChannelWallet)
	admin.POST("/:id/test/model", handler.TestChannelModel)
	admin.POST("/:id/test/batch", handler.BatchTestChannelModels)
}
