package handler

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/llm/availability/dto"
	"github.com/zgiai/zgi/api/internal/modules/llm/availability/service"
	"github.com/zgiai/zgi/api/pkg/response"
)

// AvailabilityHandler handles model availability check requests
type AvailabilityHandler struct {
	service service.AvailabilityService
}

// NewAvailabilityHandler creates a new availability handler
func NewAvailabilityHandler(service service.AvailabilityService) *AvailabilityHandler {
	return &AvailabilityHandler{
		service: service,
	}
}

// CheckModelAvailability checks the availability of a specific model
// GET /console/api/llm/models/:model_id/check-availability
func (h *AvailabilityHandler) CheckModelAvailability(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}

	modelIDStr := c.Param("id")
	modelID, err := uuid.Parse(modelIDStr)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid model id")
		return
	}

	availability, err := h.service.CheckModelAvailability(c.Request.Context(), organizationID, modelID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, availability)
}

// BatchCheckAvailability checks the availability of multiple models
// POST /console/api/llm/models/check-availability
func (h *AvailabilityHandler) BatchCheckAvailability(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}

	var req dto.BatchCheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	results, err := h.service.BatchCheckAvailability(c.Request.Context(), organizationID, req.ModelIDs)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, dto.BatchCheckResponse{Results: results})
}

// getOrganizationID extracts tenant ID from gin context
func getOrganizationID(c *gin.Context) (uuid.UUID, error) {
	tenantIDStr := c.GetString("organization_id")
	if tenantIDStr == "" {
		return uuid.Nil, fmt.Errorf("tenant_id not found in context")
	}
	return uuid.Parse(tenantIDStr)
}
