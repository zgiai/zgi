package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/middleware"
)

func RegisterTenantDefaultModelRoutes(r *gin.RouterGroup, handler *Handler) {
	g := r.Group("/default-models")
	admin := g.Group("")
	admin.Use(middleware.EnterpriseAdminOrOwnerRequired())

	g.GET("", handler.List)
	admin.PUT("/:use_case", handler.Upsert)
	admin.DELETE("/:use_case", handler.Delete)
}
