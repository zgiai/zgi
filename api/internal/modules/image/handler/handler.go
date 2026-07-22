package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	imageservice "github.com/zgiai/zgi/api/internal/modules/image/service"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

type Handler struct {
	service imageservice.Service
}

func NewHandler(service imageservice.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) ListModels(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	models, err := h.service.ListModels(c.Request.Context(), scope)
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, models)
}

func (h *Handler) Generate(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	var req imageservice.GenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_REQUEST", "message": err.Error()})
		return
	}
	result, err := h.service.Generate(c.Request.Context(), scope, req)
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) scope(c *gin.Context) (imageservice.Scope, bool) {
	accountID, err := uuid.Parse(strings.TrimSpace(util.GetAccountID(c)))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "UNAUTHORIZED", "message": "account id is required"})
		return imageservice.Scope{}, false
	}
	organizationID, err := uuid.Parse(strings.TrimSpace(util.GetOrganizationID(c)))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "UNAUTHORIZED", "message": "organization id is required"})
		return imageservice.Scope{}, false
	}
	var workspaceID *uuid.UUID
	if raw := strings.TrimSpace(util.GetWorkspaceID(c)); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_WORKSPACE", "message": "workspace id is invalid"})
			return imageservice.Scope{}, false
		}
		workspaceID = &parsed
	}
	return imageservice.Scope{OrganizationID: organizationID, AccountID: accountID, WorkspaceID: workspaceID}, true
}

func fail(c *gin.Context, err error) {
	code := imageservice.ErrorCode(err)
	status := http.StatusBadRequest
	switch code {
	case "UPSTREAM_FAILED", "IMAGE_SAVE_FAILED", "IMAGE_RUNTIME_FAILED":
		status = http.StatusBadGateway
	case "CONVERSATION_NOT_ACCESSIBLE":
		status = http.StatusForbidden
	}
	c.JSON(status, gin.H{"code": code, "message": err.Error()})
}
