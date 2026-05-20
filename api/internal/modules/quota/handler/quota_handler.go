package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/zgiai/zgi/api/internal/dto"
	quota_model "github.com/zgiai/zgi/api/internal/modules/quota/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
)

// QuotaHandler Quota management handler
type QuotaHandler struct {
	quotaService interfaces.QuotaService
}

// NewQuotaHandler Create quota handler instance
func NewQuotaHandler(quotaService interfaces.QuotaService) *QuotaHandler {
	return &QuotaHandler{
		quotaService: quotaService,
	}
}

// GetQuotaStatus Get quota status
func (h *QuotaHandler) GetQuotaStatus(c *gin.Context) {
	groupIDStr, exists := c.Get("group_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    "401",
			"message": "group_id not found in token",
		})
		return
	}

	groupID, err := uuid.Parse(groupIDStr.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "400",
			"message": "invalid group_id format",
		})
		return
	}

	status, err := h.quotaService.GetQuotaStatus(c.Request.Context(), groupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "500",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, status)
}

// GetUsageHistory Query usage history
func (h *QuotaHandler) GetUsageHistory(c *gin.Context) {
	groupIDStr, exists := c.Get("group_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    "401",
			"message": "group_id not found in token",
		})
		return
	}

	groupID, err := uuid.Parse(groupIDStr.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "400",
			"message": "invalid group_id format",
		})
		return
	}

	var filter dto.QuotaUsageHistoryFilterDTO
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "400",
			"message": err.Error(),
		})
		return
	}

	filter.GroupID = &groupID

	result, err := h.quotaService.GetUsageHistory(c.Request.Context(), &filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "500",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// CheckQuota Manually check quota
func (h *QuotaHandler) CheckQuota(c *gin.Context) {
	groupIDStr, exists := c.Get("group_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    "401",
			"message": "group_id not found in token",
		})
		return
	}

	groupID, err := uuid.Parse(groupIDStr.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "400",
			"message": "invalid group_id format",
		})
		return
	}

	var req struct {
		ResourceType quota_model.ResourceType `json:"resource_type" binding:"required"`
		Amount       int64                    `json:"amount" binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "400",
			"message": err.Error(),
		})
		return
	}

	canProceed, currentUsage, limit, err := h.quotaService.CheckQuota(
		c.Request.Context(),
		groupID,
		req.ResourceType,
		req.Amount,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "500",
			"message": err.Error(),
		})
		return
	}

	var remaining int64
	unlimited := limit == -1
	if !unlimited {
		remaining = limit - currentUsage
		if remaining < 0 {
			remaining = 0
		}
	}

	resp := dto.QuotaCheckResponseDTO{
		CanProceed:   canProceed,
		CurrentUsage: currentUsage,
		Limit:        limit,
		Remaining:    remaining,
		Unlimited:    unlimited,
	}

	c.JSON(http.StatusOK, resp)
}

// RegisterRoutes Register routes
func (h *QuotaHandler) RegisterRoutes(router *gin.RouterGroup) {
	router.GET("/status", h.GetQuotaStatus)
	router.GET("/history", h.GetUsageHistory)
	router.POST("/check", h.CheckQuota)
}
