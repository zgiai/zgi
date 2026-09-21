package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	videoservice "github.com/zgiai/zgi/api/internal/modules/video/service"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/apperror"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"github.com/zgiai/zgi/api/pkg/response"
)

type Handler struct {
	service        videoservice.Service
	errorProjector *apptransport.Projector
}

func NewHandler(service videoservice.Service, errorProjector *apptransport.Projector) *Handler {
	if errorProjector == nil {
		panic("video runtime handler requires application error projector")
	}
	return &Handler{service: service, errorProjector: errorProjector}
}

func (h *Handler) ListModels(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	models, err := h.service.ListModels(c.Request.Context(), scope)
	if err != nil {
		h.fail(c, err)
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
		h.fail(c, apperror.Wrap(err, videoservice.AppCodeVideoRequestInvalid, apperror.WithOperation("video.generate.bind")))
		return
	}
	result, err := h.service.Generate(c.Request.Context(), scope, req)
	if err != nil {
		h.fail(c, err)
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
		h.fail(c, apperror.Wrap(errors.New("limit must be an integer"), videoservice.AppCodeVideoRequestInvalid, apperror.WithOperation("video.tasks.limit")))
		return
	}
	result, err := h.service.ListTasks(c.Request.Context(), scope, videoservice.ListTasksQuery{
		Limit:  limit,
		Cursor: strings.TrimSpace(c.Query("cursor")),
		Search: strings.TrimSpace(c.Query("search")),
	})
	if err != nil {
		h.fail(c, err)
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
		h.fail(c, err)
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
		h.fail(c, err)
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

// fail projects the video error through the application catalog. The original
// error is only used for classification and logging; its text never reaches the
// client.
func (h *Handler) fail(c *gin.Context, err error) {
	appErr := videoApplicationError(err)
	locale := apptransport.LocaleFromAcceptLanguage(c.GetHeader("Accept-Language"))
	result := h.errorProjector.Project(appErr, locale)

	body := gin.H{
		"code":      result.Presentation.Code.String(),
		"message":   result.Presentation.Message,
		"retryable": result.Presentation.Retryable,
	}
	if result.ErrorID != "" {
		body["error_id"] = result.ErrorID
	}
	if requestID := requestIDFromContext(c); requestID != "" {
		body["request_id"] = requestID
	}
	if result.Presentation.HTTPStatus == http.StatusTooManyRequests {
		c.Header("Retry-After", "60")
	}
	c.JSON(result.Presentation.HTTPStatus, body)
}

// videoApplicationError maps the legacy service sentinels to cataloged codes.
// Errors that already carry an application error pass through unchanged.
func videoApplicationError(err error) error {
	if appErr, ok := apperror.As(err); ok {
		return appErr
	}
	switch videoservice.ErrorCode(err) {
	case "PROMPT_REQUIRED", "PROMPT_TOO_LONG", "MODEL_NOT_AVAILABLE", "INVALID_CURSOR", "SEARCH_TOO_LONG", "BILLING_CONTEXT_REQUIRED":
		return apperror.Wrap(err, videoservice.AppCodeVideoRequestInvalid, apperror.WithOperation("video.request"))
	case "VIDEO_TASK_NOT_FOUND":
		return apperror.Wrap(err, videoservice.AppCodeVideoTaskNotFound, apperror.WithOperation("video.task"))
	case "VIDEO_RUNTIME_UNAVAILABLE":
		return apperror.Wrap(err, videoservice.AppCodeVideoRuntimeUnavailable, apperror.WithOperation("video.runtime"))
	case "UPSTREAM_FAILED":
		return apperror.Wrap(err, videoservice.AppCodeVideoTaskFailed, apperror.WithOperation("video.upstream"))
	default:
		return apperror.Wrap(err, videoservice.AppCodeVideoTaskFailed, apperror.WithOperation("video.runtime"))
	}
}

func requestIDFromContext(c *gin.Context) string {
	if requestID := strings.TrimSpace(c.GetString("request_id")); requestID != "" {
		return requestID
	}
	return strings.TrimSpace(c.Writer.Header().Get("X-Request-ID"))
}
