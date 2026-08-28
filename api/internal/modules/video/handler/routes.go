package handler

import "github.com/gin-gonic/gin"

func (h *Handler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/video-runtime")
	group.GET("/models", h.ListModels)
	group.GET("/tasks", h.ListTasks)
	group.GET("/tasks/:task_id", h.GetTask)
	group.DELETE("/tasks/:task_id", h.DeleteTask)
	group.POST("/generate", h.Generate)
}
