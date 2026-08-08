package handler

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/modules/llm/statistics/dto"
	"github.com/zgiai/zgi/api/internal/modules/llm/statistics/service"
	"github.com/zgiai/zgi/api/pkg/response"
)

// StatisticsHandler handles HTTP requests for statistics operations
type StatisticsHandler struct {
	statisticsService service.StatisticsService
}

func (h *StatisticsHandler) GetInvocationContentSettings(c *gin.Context) {
	organizationID, exists := c.Get("organization_id")
	if !exists {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	result, err := h.statisticsService.GetInvocationContentSettings(c.Request.Context(), organizationID.(string))
	if err != nil {
		handleStatisticsError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *StatisticsHandler) UpdateInvocationContentSettings(c *gin.Context) {
	organizationID, exists := c.Get("organization_id")
	if !exists {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	var req dto.UpdateInvocationContentSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	result, err := h.statisticsService.UpdateInvocationContentSettings(c.Request.Context(), organizationID.(string), &req)
	if err != nil {
		handleStatisticsError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *StatisticsHandler) GetInvocationContent(c *gin.Context) {
	organizationID, exists := c.Get("organization_id")
	if !exists {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	result, err := h.statisticsService.GetInvocationContent(c.Request.Context(), organizationID.(string), accountID, c.Param("invocation_id"))
	if err != nil {
		if errors.Is(err, service.ErrInvocationContentNotFound) {
			response.FailWithMessage(c, response.ErrNotFound, err.Error())
			return
		}
		handleStatisticsError(c, err)
		return
	}
	response.Success(c, result)
}

// NewStatisticsHandler creates a new statistics handler
func NewStatisticsHandler(statisticsService service.StatisticsService) *StatisticsHandler {
	return &StatisticsHandler{
		statisticsService: statisticsService,
	}
}

// GetModelUsage gets token/point usage grouped from settled usage bills.
func (h *StatisticsHandler) GetModelUsage(c *gin.Context) {
	organizationID, exists := c.Get("organization_id")
	if !exists {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var req dto.ModelUsageRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	result, err := h.statisticsService.GetModelUsage(c.Request.Context(), organizationID.(string), &req)
	if err != nil {
		handleStatisticsError(c, err)
		return
	}

	response.Success(c, result)
}

// GetInvocationLog returns business-safe logical Gateway calls. Prompt and
// response content are intentionally not part of this contract.
func (h *StatisticsHandler) GetInvocationLog(c *gin.Context) {
	organizationID, exists := c.Get("organization_id")
	if !exists {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var req dto.InvocationLogRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	result, err := h.statisticsService.GetInvocationLog(c.Request.Context(), organizationID.(string), &req)
	if err != nil {
		handleStatisticsError(c, err)
		return
	}
	response.Success(c, result)
}

// GetWorkspaceQuota gets current workspace quota snapshot.
func (h *StatisticsHandler) GetWorkspaceQuota(c *gin.Context) {
	organizationID, exists := c.Get("organization_id")
	if !exists {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var req dto.WorkspaceQuotaRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	result, err := h.statisticsService.GetWorkspaceQuota(c.Request.Context(), organizationID.(string), &req)
	if err != nil {
		handleStatisticsError(c, err)
		return
	}

	response.Success(c, result)
}

func handleStatisticsError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrInvocationContentUnavailable) {
		response.FailWithMessage(c, response.ErrActionNotAllowed, err.Error())
		return
	}
	if service.IsValidationError(err) {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	response.FailWithMessage(c, response.ErrSystemError, err.Error())
}
