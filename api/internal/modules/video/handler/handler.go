package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	videoservice "github.com/zgiai/zgi/api/internal/modules/video/service"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

type Handler struct {
	service videoservice.Service
}

func NewHandler(service videoservice.Service) *Handler {
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
	var req videoservice.GenerateRequest
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
	result, err := h.service.ListTasks(c.Request.Context(), scope, videoservice.ListTasksQuery{
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
	taskID := strings.TrimSpace(c.Param("task_id"))
	task, err := h.service.GetTask(c.Request.Context(), scope, taskID)
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, task)
}

func (h *Handler) DeleteTask(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	taskID := strings.TrimSpace(c.Param("task_id"))
	if err := h.service.DeleteTask(c.Request.Context(), scope, taskID); err != nil {
		fail(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func (h *Handler) scope(c *gin.Context) (videoservice.Scope, bool) {
	accountID, err := uuid.Parse(strings.TrimSpace(util.GetAccountID(c)))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "UNAUTHORIZED", "message": "account id is required"})
		return videoservice.Scope{}, false
	}
	organizationID, err := uuid.Parse(strings.TrimSpace(util.GetOrganizationID(c)))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "UNAUTHORIZED", "message": "organization id is required"})
		return videoservice.Scope{}, false
	}
	var workspaceID *uuid.UUID
	if raw := strings.TrimSpace(util.GetWorkspaceID(c)); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_WORKSPACE", "message": "workspace id is invalid"})
			return videoservice.Scope{}, false
		}
		workspaceID = &parsed
	}
	return videoservice.Scope{OrganizationID: organizationID, AccountID: accountID, WorkspaceID: workspaceID}, true
}

func fail(c *gin.Context, err error) {
	code := videoservice.ErrorCode(err)
	status := http.StatusBadRequest
	switch code {
	case "UPSTREAM_FAILED", "VIDEO_RUNTIME_UNAVAILABLE", "VIDEO_RUNTIME_FAILED":
		status = http.StatusBadGateway
	case "VIDEO_TASK_NOT_FOUND":
		status = http.StatusNotFound
	}
	c.JSON(status, gin.H{"code": code, "message": err.Error()})
}
