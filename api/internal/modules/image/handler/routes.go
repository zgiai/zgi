package handler

import "github.com/gin-gonic/gin"

func (h *Handler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/image-runtime")
	group.GET("/models", h.ListModels)
	group.GET("/tasks", h.ListTasks)
	group.GET("/tasks/:task_id", h.GetTask)
	group.POST("/tasks/:task_id/cancel", h.CancelTask)
	group.POST("/generate", h.Generate)
}
