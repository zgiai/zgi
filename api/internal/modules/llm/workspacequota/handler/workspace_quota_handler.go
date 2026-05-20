package handler

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/internal/modules/llm/workspacequota/dto"
	"github.com/zgiai/ginext/internal/modules/llm/workspacequota/service"
	"github.com/zgiai/ginext/pkg/response"
)

// WorkspaceQuotaHandler handles workspace quota management requests.
type WorkspaceQuotaHandler struct {
	workspaceQuotaService service.WorkspaceQuotaService
}

func NewWorkspaceQuotaHandler(workspaceQuotaService service.WorkspaceQuotaService) *WorkspaceQuotaHandler {
	return &WorkspaceQuotaHandler{workspaceQuotaService: workspaceQuotaService}
}

func (h *WorkspaceQuotaHandler) ListWorkspaceQuotas(c *gin.Context) {
	organizationID, ok := c.Get("organization_id")
	if !ok {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var req dto.ListWorkspaceQuotaRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	result, err := h.workspaceQuotaService.ListWorkspaceQuotas(c.Request.Context(), organizationID.(string), &req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	response.Success(c, result)
}

func (h *WorkspaceQuotaHandler) GetWorkspaceQuota(c *gin.Context) {
	organizationID, ok := c.Get("organization_id")
	if !ok {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	workspaceID := c.Param("workspace_id")
	result, err := h.workspaceQuotaService.GetWorkspaceQuota(c.Request.Context(), organizationID.(string), workspaceID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	response.Success(c, result)
}

func (h *WorkspaceQuotaHandler) UpdateWorkspaceQuota(c *gin.Context) {
	organizationID, ok := c.Get("organization_id")
	if !ok {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	workspaceID := c.Param("workspace_id")
	var req dto.UpdateWorkspaceQuotaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	result, err := h.workspaceQuotaService.UpdateWorkspaceQuota(c.Request.Context(), organizationID.(string), workspaceID, &req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	response.Success(c, result)
}

func (h *WorkspaceQuotaHandler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrWorkspaceNotFound):
		response.FailWithMessage(c, response.ErrNotFound, err.Error())
	case errors.Is(err, service.ErrWorkspaceOrgMismatch):
		response.FailWithMessage(c, response.ErrUnauthorized, err.Error())
	case errors.Is(err, service.ErrInvalidOrganization),
		errors.Is(err, service.ErrInvalidWorkspaceID),
		errors.Is(err, service.ErrQuotaAmountRequired),
		errors.Is(err, service.ErrInvalidRemainQuota),
		errors.Is(err, service.ErrRemainExceedsLimit):
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
	default:
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
	}
}
