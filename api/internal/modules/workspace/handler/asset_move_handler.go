package handler

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_service "github.com/zgiai/zgi/api/internal/modules/workspace/service"
	helper "github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/response"
)

type WorkspaceAssetMoveHandler struct {
	accountService interfaces.AccountService
	service        *workspace_service.WorkspaceAssetMoveService
}

func NewWorkspaceAssetMoveHandler(accountService interfaces.AccountService, service *workspace_service.WorkspaceAssetMoveService) *WorkspaceAssetMoveHandler {
	return &WorkspaceAssetMoveHandler{accountService: accountService, service: service}
}

func (h *WorkspaceAssetMoveHandler) RegisterRoutes(router *gin.RouterGroup) {
	organization := router.Group("/organizations", middleware.JWTWithOrganizationAndService(h.accountService))
	{
		organization.POST("/current/assets/move/preview", h.Preview)
		organization.POST("/current/assets/move", h.Move)
	}
}

func (h *WorkspaceAssetMoveHandler) Preview(c *gin.Context) {
	var req dto.WorkspaceAssetMoveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	organizationID, accountID, ok := h.contextIDs(c)
	if !ok {
		return
	}
	preview, err := h.service.Preview(c.Request.Context(), organizationID, accountID, req)
	if err != nil {
		handleAssetMoveError(c, err, nil)
		return
	}
	response.Success(c, preview)
}

func (h *WorkspaceAssetMoveHandler) Move(c *gin.Context) {
	var req dto.WorkspaceAssetMoveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	organizationID, accountID, ok := h.contextIDs(c)
	if !ok {
		return
	}
	result, err := h.service.Move(c.Request.Context(), organizationID, accountID, req)
	if err != nil {
		handleAssetMoveError(c, err, result)
		return
	}
	response.Success(c, result)
}

func (h *WorkspaceAssetMoveHandler) contextIDs(c *gin.Context) (string, string, bool) {
	organizationID := helper.GetOrganizationID(c)
	accountID := helper.GetAccountID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return "", "", false
	}
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return "", "", false
	}
	return organizationID, accountID, true
}

func handleAssetMoveError(c *gin.Context, err error, result interface{}) {
	switch {
	case errors.Is(err, workspace_service.ErrAssetMovePermissionDenied):
		response.Fail(c, response.ErrPermissionDenied)
	case errors.Is(err, workspace_service.ErrAssetMoveInvalidRequest):
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
	case errors.Is(err, workspace_service.ErrAssetMoveBlocked):
		c.JSON(400, response.Response{
			Code:    "199001",
			Message: err.Error(),
			Data:    result,
		})
	default:
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
	}
}
