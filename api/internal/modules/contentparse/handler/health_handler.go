package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/internal/modules/contentparse/service"
	"github.com/zgiai/ginext/pkg/response"
)

type HealthHandler struct {
	service service.HealthService
}

func NewHealthHandler(service service.HealthService) *HealthHandler {
	return &HealthHandler{service: service}
}

func (h *HealthHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/providers/:id/health", h.ListHealthChecks)
	rg.GET("/providers/:id/health/latest", h.GetLatestHealthCheck)
}

func (h *HealthHandler) ListHealthChecks(c *gin.Context) {
	id, err := parseRequiredUUID(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid provider id")
		return
	}
	limit := 20
	if raw := c.Query("limit"); raw != "" {
		if parsed, parseErr := strconv.Atoi(raw); parseErr == nil && parsed > 0 {
			limit = parsed
		}
	}
	items, err := h.service.ListByProviderConfigID(c.Request.Context(), id, limit)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, items)
}

func (h *HealthHandler) GetLatestHealthCheck(c *gin.Context) {
	id, err := parseRequiredUUID(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid provider id")
		return
	}
	item, err := h.service.GetLatestByProviderConfigID(c.Request.Context(), id)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	if item == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "404001", "message": "health check not found"})
		return
	}
	response.Success(c, item)
}
