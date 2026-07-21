package handler

import "github.com/gin-gonic/gin"

func (h *Handler) RegisterRoutes(router *gin.RouterGroup, generateMiddleware gin.HandlerFunc) {
	group := router.Group("/image-runtime")
	group.GET("/models", h.ListModels)
	group.POST("/generate", generateMiddleware, h.Generate)
}
