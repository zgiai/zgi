package music

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/apperror"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

const (
	maxCreateRequestBytes           = 64 * 1024
	messageJSONContentTypeRequired  = "Request content type must be application/json"
	messageInvalidGenerationRequest = "Invalid music generation request"
	messageInvalidTaskID            = "Invalid music task ID"
	messageInvalidAuthContext       = "Invalid authentication context"
	messageModelUnavailable         = "Music model is unavailable"
	messageTaskNotFound             = "Music task not found"
	messageTaskConflict             = "Music request ID is already in use"
	messageTaskProcessingFailed     = "Failed to process music task"
)

type Handler struct {
	service        *Service
	errorProjector *apptransport.Projector
}

func NewHandler(service *Service, errorProjector *apptransport.Projector) *Handler {
	if service == nil {
		panic("music handler requires service")
	}
	if errorProjector == nil {
		panic("music handler requires application error projector")
	}
	return &Handler{service: service, errorProjector: errorProjector}
}

func (h *Handler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/music")
	group.POST("/tasks", h.Create)
	group.GET("/tasks", h.List)
	group.GET("/tasks/:id", h.Get)
	group.DELETE("/tasks/:id", h.Delete)
}

func (h *Handler) Create(c *gin.Context) {
	scope, ok := musicScope(c)
	if !ok {
		return
	}
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		h.writeApplicationError(c, apperror.Wrap(errors.New(messageJSONContentTypeRequired), AppCodeRequestInvalid, apperror.WithOperation("music.request")))
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxCreateRequestBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	var request CreateRequest
	if err := decoder.Decode(&request); err != nil {
		h.writeApplicationError(c, apperror.Wrap(errors.New(messageInvalidGenerationRequest), AppCodeRequestInvalid, apperror.WithOperation("music.request")))
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		h.writeApplicationError(c, apperror.Wrap(errors.New(messageInvalidGenerationRequest), AppCodeRequestInvalid, apperror.WithOperation("music.request")))
		return
	}
	task, err := h.service.Create(c.Request.Context(), scope, request)
	if err != nil {
		h.handleMusicServiceError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{
		"code":    "0",
		"message": "success",
		"data":    taskView(task),
	})
}

func (h *Handler) Get(c *gin.Context) {
	scope, ok := musicScope(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(strings.TrimSpace(c.Param("id")))
	if err != nil {
		writeMusicError(c, http.StatusBadRequest, "INVALID_TASK_ID", messageInvalidTaskID)
		return
	}
	view, err := h.service.Get(c.Request.Context(), scope, id)
	if err != nil {
		h.handleMusicServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    "0",
		"message": "success",
		"data":    view,
	})
}

func (h *Handler) List(c *gin.Context) {
	scope, ok := musicScope(c)
	if !ok {
		return
	}
	request, err := parseListRequest(c)
	if err != nil {
		h.handleMusicServiceError(c, err)
		return
	}
	result, err := h.service.List(c.Request.Context(), scope, request)
	if err != nil {
		h.handleMusicServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    "0",
		"message": "success",
		"data":    result,
	})
}

func (h *Handler) Delete(c *gin.Context) {
	scope, ok := musicScope(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(strings.TrimSpace(c.Param("id")))
	if err != nil {
		writeMusicError(c, http.StatusBadRequest, "INVALID_TASK_ID", messageInvalidTaskID)
		return
	}
	if err := h.service.Delete(c.Request.Context(), scope, id); err != nil {
		h.handleMusicServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    "0",
		"message": "success",
		"data":    gin.H{"deleted": true},
	})
}

func parseListRequest(c *gin.Context) (ListRequest, error) {
	request := ListRequest{Search: c.Query("search")}
	if raw := c.Query("page"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return ListRequest{}, ErrInvalidRequest
		}
		request.Page = value
	}
	if raw := c.Query("page_size"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return ListRequest{}, ErrInvalidRequest
		}
		request.PageSize = value
	}
	return request, nil
}

func musicScope(c *gin.Context) (Scope, bool) {
	organizationID, organizationErr := uuid.Parse(strings.TrimSpace(util.GetOrganizationID(c)))
	accountID, accountErr := uuid.Parse(strings.TrimSpace(util.GetAccountID(c)))
	if organizationErr != nil || accountErr != nil || organizationID == uuid.Nil || accountID == uuid.Nil {
		writeMusicError(c, http.StatusUnauthorized, "UNAUTHORIZED", messageInvalidAuthContext)
		return Scope{}, false
	}
	var workspaceID *uuid.UUID
	if raw := strings.TrimSpace(util.GetWorkspaceID(c)); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil || parsed == uuid.Nil {
			writeMusicError(c, http.StatusUnauthorized, "UNAUTHORIZED", messageInvalidAuthContext)
			return Scope{}, false
		}
		workspaceID = &parsed
	}
	return Scope{OrganizationID: organizationID, WorkspaceID: workspaceID, AccountID: accountID}, true
}

func (h *Handler) handleMusicServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidRequest):
		h.writeApplicationError(c, apperror.Wrap(err, AppCodeRequestInvalid, apperror.WithOperation("music.request")))
	case errors.Is(err, ErrModelUnavailable):
		h.writeApplicationError(c, apperror.Wrap(err, AppCodeModelUnavailable, apperror.WithOperation("music.model")))
	case errors.Is(err, ErrTaskNotFound):
		h.writeApplicationError(c, apperror.Wrap(err, AppCodeTaskNotFound, apperror.WithOperation("music.task.get")))
	case errors.Is(err, ErrTaskConflict):
		h.writeApplicationError(c, apperror.Wrap(err, AppCodeTaskConflict, apperror.WithOperation("music.task.create")))
	case apperror.IsCode(err, AppCodeTaskNotDeletable):
		h.writeApplicationError(c, err)
	default:
		h.writeApplicationError(c, apperror.Wrap(err, AppCodeTaskFailed, apperror.WithOperation("music.task")))
	}
}

func (h *Handler) writeApplicationError(c *gin.Context, err error) {
	locale := apptransport.LocaleFromAcceptLanguage(c.GetHeader("Accept-Language"))
	result := h.errorProjector.Project(err, locale)
	body := gin.H{
		"code":      result.Presentation.Code.String(),
		"message":   result.Presentation.Message,
		"retryable": result.Presentation.Retryable,
	}
	if result.ErrorID != "" {
		body["error_id"] = result.ErrorID
	}
	if requestID := strings.TrimSpace(c.GetString("request_id")); requestID != "" {
		body["request_id"] = requestID
	} else if requestID := strings.TrimSpace(c.Writer.Header().Get("X-Request-ID")); requestID != "" {
		body["request_id"] = requestID
	}
	if result.Presentation.HTTPStatus == http.StatusTooManyRequests {
		c.Header("Retry-After", "60")
	}
	c.JSON(result.Presentation.HTTPStatus, body)
}

func writeMusicError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"code": code, "message": message})
}
