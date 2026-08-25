package handler

import (
	"net/http"
	"strconv"
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
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_REQUEST", "message": "INVALID_REQUEST"})
		return
	}
	result, err := h.service.CreateTask(c.Request.Context(), scope, req)
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) ListTasks(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	limit, err := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("limit", "20")))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_REQUEST", "message": "limit must be an integer"})
		return
	}
	result, err := h.service.ListTasks(c.Request.Context(), scope, imageservice.ListTasksQuery{
		Limit:  limit,
		Cursor: strings.TrimSpace(c.Query("cursor")),
		Search: strings.TrimSpace(c.Query("search")),
	})
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) GetTask(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	task, err := h.service.GetTask(c.Request.Context(), scope, strings.TrimSpace(c.Param("task_id")))
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, task)
}

func (h *Handler) CancelTask(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	task, err := h.service.CancelTask(c.Request.Context(), scope, strings.TrimSpace(c.Param("task_id")))
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, task)
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
	case "IMAGE_TASK_NOT_FOUND":
		status = http.StatusNotFound
	case "IMAGE_TASK_CONFLICT":
		status = http.StatusConflict
	}
	c.JSON(status, gin.H{"code": code, "message": publicErrorMessage(code)})
}

func publicErrorMessage(code string) string {
	if strings.TrimSpace(code) == "" {
		return "IMAGE_RUNTIME_FAILED"
	}
	return code
}
