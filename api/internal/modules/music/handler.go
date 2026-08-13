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
	service *Service
}

func NewHandler(service *Service) *Handler {
	if service == nil {
		panic("music handler requires service")
	}
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/music")
	group.POST("/tasks", h.Create)
	group.GET("/tasks", h.List)
	group.GET("/tasks/:id", h.Get)
}

func (h *Handler) Create(c *gin.Context) {
	scope, ok := musicScope(c)
	if !ok {
		return
	}
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeMusicError(c, http.StatusBadRequest, "INVALID_REQUEST", messageJSONContentTypeRequired)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxCreateRequestBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	var request CreateRequest
	if err := decoder.Decode(&request); err != nil {
		writeMusicError(c, http.StatusBadRequest, "INVALID_REQUEST", messageInvalidGenerationRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeMusicError(c, http.StatusBadRequest, "INVALID_REQUEST", messageInvalidGenerationRequest)
		return
	}
	task, err := h.service.Create(c.Request.Context(), scope, request)
	if err != nil {
		handleMusicServiceError(c, err)
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
		handleMusicServiceError(c, err)
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
		handleMusicServiceError(c, err)
		return
	}
	result, err := h.service.List(c.Request.Context(), scope, request)
	if err != nil {
		handleMusicServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    "0",
		"message": "success",
		"data":    result,
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
	workspaceID, workspaceErr := uuid.Parse(strings.TrimSpace(util.GetWorkspaceID(c)))
	accountID, accountErr := uuid.Parse(strings.TrimSpace(util.GetAccountID(c)))
	if organizationErr != nil || workspaceErr != nil || accountErr != nil ||
		organizationID == uuid.Nil || workspaceID == uuid.Nil || accountID == uuid.Nil {
		writeMusicError(c, http.StatusUnauthorized, "UNAUTHORIZED", messageInvalidAuthContext)
		return Scope{}, false
	}
	return Scope{OrganizationID: organizationID, WorkspaceID: workspaceID, AccountID: accountID}, true
}

func handleMusicServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidRequest):
		writeMusicError(c, http.StatusBadRequest, "INVALID_REQUEST", messageInvalidGenerationRequest)
	case errors.Is(err, ErrModelUnavailable):
		writeMusicError(c, http.StatusBadRequest, "MODEL_UNAVAILABLE", messageModelUnavailable)
	case errors.Is(err, ErrTaskNotFound):
		writeMusicError(c, http.StatusNotFound, "TASK_NOT_FOUND", messageTaskNotFound)
	case errors.Is(err, ErrTaskConflict):
		writeMusicError(c, http.StatusConflict, "TASK_CONFLICT", messageTaskConflict)
	default:
		writeMusicError(c, http.StatusInternalServerError, "MUSIC_TASK_FAILED", messageTaskProcessingFailed)
	}
}

func writeMusicError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"code": code, "message": message})
}
