package handler

import "github.com/gin-gonic/gin"

// RegisterTenantCredentialRoutes registers tenant routes for credential management
func RegisterTenantCredentialRoutes(r *gin.RouterGroup, handler *TenantCredentialHandler) {
	g := r.Group("/tenant/credentials")
	g.POST("", handler.Create)
	g.GET("", handler.List)
	g.GET("/:id", handler.Get)
	g.PUT("/:id", handler.Update)
	g.DELETE("/:id", handler.Delete)
	g.POST("/:id/test", handler.Test)
}
