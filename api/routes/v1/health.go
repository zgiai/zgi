package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/consolesync"
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
			payload := gin.H{
				"status":  "ok",
				"version": "v1",
			}
			if snapshot, ok := consolesync.StatusSnapshot(c.Request.Context()); ok {
				payload["console_sync"] = snapshot
			}
			if config.ConnectorRetirementDeferred() {
				payload["connector_retirement"] = "deferred"
			}
			c.JSON(http.StatusOK, payload)
		})

		// MERGE-NOTE(codex-video-image-rollback): single added line; the branch does not
		// touch health.go, so this cannot conflict.
		consolesync.RegisterStatusRoute(health)
	}
}
