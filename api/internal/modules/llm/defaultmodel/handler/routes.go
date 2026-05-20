package handler

import "github.com/gin-gonic/gin"

func RegisterTenantDefaultModelRoutes(r *gin.RouterGroup, handler *Handler) {
	g := r.Group("/default-models")
	g.GET("", handler.List)
	g.PUT("/:use_case", handler.Upsert)
	g.DELETE("/:use_case", handler.Delete)
}

