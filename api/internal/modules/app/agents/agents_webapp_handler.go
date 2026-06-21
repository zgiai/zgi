package agents

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

func (h *AgentsHandler) UpdateWebAppStatus(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	agentID := c.Param("agent_id")
	if agentID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	callerOrganizationID := util.GetOrganizationID(c)
	if callerOrganizationID == "" {
		response.Fail(c, response.ErrOrganizationNotFound)
		return
	}

	ctx := context.WithValue(c.Request.Context(), "account_id", accountID)
	ctx = context.WithValue(ctx, "tenant_id", callerOrganizationID)
	if err := h.appService.RequireAgentManageAccess(ctx, agentID, accountID); err != nil {
		h.failRuntime(c, err)
		return
	}

	var req dto.UpdateWebAppStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	result, err := h.appService.UpdateWebAppStatus(ctx, agentID, req)
	if err != nil {
		switch {
		case errors.Is(err, errInvalidWebAppStatus), errors.Is(err, errWebAppOfflineReasonTooLong):
			response.Fail(c, response.ErrInvalidParam)
		case err.Error() == "agent not found":
			response.SpecialFail(c, gin.H{"code": "404001", "message": "Agent not found"})
		case err.Error() == "permission denied":
			response.SpecialFail(c, gin.H{"code": "403001", "message": "Permission denied"})
		default:
			response.SpecialFail(c, gin.H{"code": "399001", "message": err.Error()})
		}
		return
	}

	response.Success(c, result)
}
