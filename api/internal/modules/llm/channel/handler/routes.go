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
	g.POST("", handler.CreateRoute)
	g.GET("", handler.ListRoutesAggregated)
	g.GET("/all", handler.ListRoutes)
	g.GET("/platform", handler.GetPlatformChannel)
	g.PUT("/platform", handler.UpdatePlatformChannelSettings)
	g.PATCH("/platform/:id", handler.UpdatePlatformChannel)
	g.GET("/platform/models", handler.ListPlatformChannelModels)
	g.GET("/providers", handler.GetAvailableProviders)
	g.POST("/select", handler.SelectRoute)
	g.GET("/by-model", handler.GetRoutesForModel)
	g.POST("/init", handler.InitTenantRoutes)
	g.POST("/draft/test/model", handler.TestDraftChannelModel)
	g.POST("/ollama/discover-models", handler.DiscoverOllamaModels)
	g.POST("/official/init", handler.InitOfficialChannel)
	g.POST("/batch/toggle", handler.BatchToggleRoutes)
	g.POST("/batch/delete", handler.BatchDeleteRoutes)
	g.GET("/:id", handler.GetRoute)
	g.PUT("/:id", handler.UpdateRoute)
	g.DELETE("/:id", handler.DeleteRoute)
	g.POST("/:id/toggle", handler.ToggleRoute)
	g.POST("/:id/test", handler.TestRoute)
	gAdmin := g.Group("")
	gAdmin.Use(AdjustChannelWalletAdminRequired())
	gAdmin.POST("/:id/wallet/adjust", handler.AdjustChannelWallet)
	g.POST("/:id/test/model", handler.TestChannelModel)
	g.POST("/:id/test/batch", handler.BatchTestChannelModels)
}
