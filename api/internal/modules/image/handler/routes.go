package handler

import "github.com/gin-gonic/gin"

func (h *Handler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/image-runtime")
	group.GET("/models", h.ListModels)
	group.POST("/generate", h.Generate)
}
