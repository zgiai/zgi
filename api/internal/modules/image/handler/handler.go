package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	imageservice "github.com/zgiai/zgi/api/internal/modules/image/service"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/apperror"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"github.com/zgiai/zgi/api/pkg/response"
)

type Handler struct {
	service        imageservice.Service
	errorProjector *apptransport.Projector
}

func NewHandler(service imageservice.Service, errorProjector *apptransport.Projector) *Handler {
	if errorProjector == nil {
		panic("image runtime handler requires application error projector")
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
	var req imageservice.GenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_REQUEST", "message": "INVALID_REQUEST"})
		return
	}
	result, err := h.service.CreateTask(c.Request.Context(), scope, req)
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
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_REQUEST", "message": "limit must be an integer"})
		return
	}
	result, err := h.service.ListTasks(c.Request.Context(), scope, imageservice.ListTasksQuery{
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
	task, err := h.service.GetTask(c.Request.Context(), scope, strings.TrimSpace(c.Param("task_id")))
	if err != nil {
		h.fail(c, err)
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
		h.fail(c, err)
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

func (h *Handler) fail(c *gin.Context, err error) {
	appErr := imageApplicationError(err)
	if appErr != nil && h.errorProjector != nil {
		locale := apptransport.LocaleFromAcceptLanguage(c.GetHeader("Accept-Language"))
		result := h.errorProjector.ProjectWithRequestID(appErr, locale, imageRequestIDFromContext(c))
		body := gin.H{
			"code":      result.Presentation.Code.String(),
			"message":   result.Presentation.Message,
			"retryable": result.Presentation.Retryable,
		}
		if result.ErrorID != "" {
			body["error_id"] = result.ErrorID
		}
		if requestID := imageRequestIDFromContext(c); requestID != "" {
			body["request_id"] = requestID
		}
		if result.Presentation.HTTPStatus == http.StatusTooManyRequests {
			c.Header("Retry-After", "60")
		}
		c.JSON(result.Presentation.HTTPStatus, body)
		return
	}
	code := imageservice.ErrorCode(err)
	status := http.StatusBadRequest
	switch code {
	case "UPSTREAM_FAILED", "IMAGE_SAVE_FAILED", "IMAGE_RUNTIME_FAILED":
		status = http.StatusBadGateway
	case "CONVERSATION_NOT_ACCESSIBLE":
		status = http.StatusForbidden
	}
	c.JSON(status, gin.H{"code": code, "message": publicErrorMessage(code)})
}

// imageApplicationError maps the legacy image service sentinels to cataloged
// codes. Errors that already carry an application error pass through unchanged,
// and unregistered conditions return nil so the legacy response is preserved.
func imageApplicationError(err error) error {
	if appErr, ok := apperror.As(err); ok {
		return appErr
	}
	switch imageservice.ErrorCode(err) {
	case "PROMPT_REQUIRED", "PROMPT_TOO_LONG", "MODEL_NOT_AVAILABLE", "IMAGE_PARAMETER_NOT_SUPPORTED",
		"UNSUPPORTED_SIZE", "UNSUPPORTED_COUNT", "IMAGE_GENERATION_MODE_INVALID",
		"IMAGE_MAX_IMAGES_REQUIRED", "IMAGE_MAX_IMAGES_NOT_ALLOWED", "IMAGE_MAX_IMAGES_OUT_OF_RANGE",
		"BILLING_CONTEXT_REQUIRED", "REFERENCE_IMAGE_REQUIRED", "REFERENCE_IMAGE_INVALID",
		"REFERENCE_IMAGE_UNSUPPORTED":
		return apperror.Wrap(err, imageservice.AppCodeRequestInvalid, apperror.WithOperation("image.request"))
	case "IMAGE_TASK_TIMEOUT":
		return apperror.Wrap(err, imageservice.AppCodeTaskTimeout, apperror.WithOperation("image.task"))
	case "UPSTREAM_FAILED":
		return apperror.Wrap(err, imageservice.AppCodeUpstreamFailed, apperror.WithOperation("image.upstream"))
	case "IMAGE_SAVE_FAILED":
		return apperror.Wrap(err, imageservice.AppCodeAssetSaveFailed, apperror.WithOperation("image.asset"))
	case "CONVERSATION_NOT_ACCESSIBLE":
		return nil
	default:
		return apperror.Wrap(err, imageservice.AppCodeRuntimeFailed, apperror.WithOperation("image.runtime"))
	}
}

func imageRequestIDFromContext(c *gin.Context) string {
	if requestID := strings.TrimSpace(c.GetString("request_id")); requestID != "" {
		return requestID
	}
	return strings.TrimSpace(c.Writer.Header().Get("X-Request-ID"))
}

func publicErrorMessage(code string) string {
	if strings.TrimSpace(code) == "" {
		return "IMAGE_RUNTIME_FAILED"
	}
	return code
}
