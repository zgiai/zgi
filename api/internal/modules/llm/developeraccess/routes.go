package developeraccess

import "github.com/gin-gonic/gin"

func RegisterRoutes(r *gin.RouterGroup, h *Handler) {
	workspaces := r.Group("/workspaces/:workspace_id")
	workspaces.GET("/developer-access/me", h.GetMe)
	workspaces.GET("/developer-access/policy", h.GetPolicy)
	workspaces.PUT("/developer-access/policy", h.PutPolicy)
	workspaces.GET("/developer-access/audit", h.ListAudit)

	requests := workspaces.Group("/access-requests")
	requests.GET("", h.ListRequests)
	requests.POST("", h.CreateRequest)
	requests.POST("/:request_id/cancel", h.CancelRequest)
	requests.POST("/:request_id/approve", h.ApproveRequest)
	requests.POST("/:request_id/reject", h.RejectRequest)

	keys := workspaces.Group("/api-keys")
	keys.GET("", h.ListKeys)
	keys.POST("", h.CreateKey)
	keys.PATCH("/:key_id", h.UpdateKey)
	keys.POST("/:key_id/disable", h.DisableKey)
	keys.POST("/:key_id/enable", h.EnableKey)
	keys.POST("/:key_id/revoke", h.RevokeKey)
	keys.POST("/:key_id/rotate", h.RotateKey)
}
