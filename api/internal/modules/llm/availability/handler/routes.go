package handler

import "github.com/gin-gonic/gin"

// RegisterAvailabilityRoutes registers model availability check routes
func RegisterAvailabilityRoutes(r *gin.RouterGroup, handler *AvailabilityHandler) {
	models := r.Group("/models")
	{
		// Single model availability check
		models.GET("/:id/check-availability", handler.CheckModelAvailability)

		// Batch availability check
		models.POST("/check-availability", handler.BatchCheckAvailability)
	}
}
