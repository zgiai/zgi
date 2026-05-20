package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RegisterHealthRoutes registers health check routes
func RegisterHealthRoutes(v1 *gin.RouterGroup) {
	health := v1.Group("/health")
	{
		health.GET("/ping", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"message": "pong",
				"version": "v1",
			})
		})

		health.GET("/status", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"status":  "ok",
				"version": "v1",
			})
		})
	}
}
