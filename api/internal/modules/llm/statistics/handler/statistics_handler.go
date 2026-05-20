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
	if errors.Is(err, service.ErrInvalidTimestamp) || errors.Is(err, service.ErrInvalidTimestampRange) {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	response.FailWithMessage(c, response.ErrSystemError, err.Error())
}
