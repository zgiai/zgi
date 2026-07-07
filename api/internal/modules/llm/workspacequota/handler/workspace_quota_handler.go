package handler

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/modules/llm/workspacequota/dto"
	"github.com/zgiai/zgi/api/internal/modules/llm/workspacequota/service"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/response"
)

type workspaceQuotaPermissionChecker interface {
	CheckWorkspaceOrganizationAnyPermission(ctx context.Context, organizationID, workspaceID, accountID string, permissionCodes ...workspacemodel.WorkspacePermissionCode) (bool, error)
}

// WorkspaceQuotaHandler handles workspace quota management requests.
type WorkspaceQuotaHandler struct {
	workspaceQuotaService service.WorkspaceQuotaService
	permissionChecker     workspaceQuotaPermissionChecker
}

func NewWorkspaceQuotaHandler(workspaceQuotaService service.WorkspaceQuotaService, permissionChecker workspaceQuotaPermissionChecker) *WorkspaceQuotaHandler {
	return &WorkspaceQuotaHandler{
		workspaceQuotaService: workspaceQuotaService,
		permissionChecker:     permissionChecker,
	}
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
	orgID := organizationID.(string)
	if !h.requireWorkspaceQuotaReadPermission(c, orgID, workspaceID) {
		return
	}

	result, err := h.workspaceQuotaService.GetWorkspaceQuota(c.Request.Context(), orgID, workspaceID)
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

func (h *WorkspaceQuotaHandler) requireWorkspaceQuotaReadPermission(c *gin.Context, organizationID, workspaceID string) bool {
	if h.permissionChecker == nil {
		response.Fail(c, response.ErrPermissionDenied)
		return false
	}

	accountID, ok := getContextString(c, "account_id")
	if !ok || accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return false
	}

	allowed, err := h.permissionChecker.CheckWorkspaceOrganizationAnyPermission(
		c.Request.Context(),
		organizationID,
		workspaceID,
		accountID,
		workspacemodel.WorkspacePermissionWorkspaceView,
	)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return false
	}
	if !allowed {
		response.Fail(c, response.ErrPermissionDenied)
		return false
	}
	return true
}

func getContextString(c *gin.Context, key string) (string, bool) {
	value, ok := c.Get(key)
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	return text, ok
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
