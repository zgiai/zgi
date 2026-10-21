package inbox

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

// Handler serves the signed-in account's inbox in its current organization.
type Handler struct {
	service   *Service
	projector *apptransport.Projector
}

// NewHandler builds the console inbox handler.
func NewHandler(service *Service, projector *apptransport.Projector) *Handler {
	if service == nil || projector == nil {
		panic("inbox handler requires service and projector")
	}
	return &Handler{service: service, projector: projector}
}

// RegisterRoutes registers /inbox routes on an authenticated group.
func (h *Handler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/inbox")
	group.GET("", h.list)
	group.GET("/unread-count", h.unreadCount)
	group.POST("/read", h.markRead)
	group.POST("/read-all", h.markAllRead)
}

func (h *Handler) recipient(c *gin.Context) (Recipient, bool) {
	organizationID, orgErr := uuid.Parse(strings.TrimSpace(util.GetOrganizationID(c)))
	accountID, accountErr := uuid.Parse(strings.TrimSpace(util.GetAccountID(c)))
	if orgErr != nil || accountErr != nil {
		h.writeError(c, apperror.New(appcatalog.CodeAuthenticationRequired, apperror.WithOperation("inbox.recipient")))
		return Recipient{}, false
	}
	return Recipient{OrganizationID: organizationID, Kind: RecipientAccount, ID: accountID}, true
}

func (h *Handler) list(c *gin.Context) {
	recipient, ok := h.recipient(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	result, err := h.service.List(c.Request.Context(), recipient, ListQuery{UnreadOnly: c.Query("unread_only") == "true", Page: page, PageSize: pageSize})
	if err != nil {
		h.writeError(c, inboxError(err, "inbox.list"))
		return
	}
	writeData(c, result)
}

func (h *Handler) unreadCount(c *gin.Context) {
	recipient, ok := h.recipient(c)
	if !ok {
		return
	}
	count, err := h.service.UnreadCount(c.Request.Context(), recipient)
	if err != nil {
		h.writeError(c, inboxError(err, "inbox.unread_count"))
		return
	}
	writeData(c, gin.H{"unread_count": count})
}

func (h *Handler) markRead(c *gin.Context) {
	recipient, ok := h.recipient(c)
	if !ok {
		return
	}
	var req struct {
		IDs        []uuid.UUID `json:"ids"`
		SourceType string      `json:"source_type"`
		SourceID   uuid.UUID   `json:"source_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		h.writeError(c, inboxError(ErrInvalidRequest, "inbox.read"))
		return
	}
	var err error
	if req.SourceType != "" {
		err = h.service.MarkSourceRead(c.Request.Context(), recipient, req.SourceType, req.SourceID)
	} else {
		err = h.service.MarkRead(c.Request.Context(), recipient, req.IDs)
	}
	if err != nil {
		h.writeError(c, inboxError(err, "inbox.read"))
		return
	}
	h.unreadCount(c)
}

func (h *Handler) markAllRead(c *gin.Context) {
	recipient, ok := h.recipient(c)
	if !ok {
		return
	}
	if err := h.service.MarkAllRead(c.Request.Context(), recipient); err != nil {
		h.writeError(c, inboxError(err, "inbox.read_all"))
		return
	}
	writeData(c, gin.H{"unread_count": 0})
}

func inboxError(err error, operation string) error {
	if errors.Is(err, ErrInvalidRequest) {
		return apperror.Wrap(err, appcatalog.CodeRequestInvalid, apperror.WithOperation(operation))
	}
	return apperror.Wrap(err, appcatalog.CodeInternal, apperror.WithOperation(operation))
}

func (h *Handler) writeError(c *gin.Context, err error) {
	locale := apptransport.LocaleFromAcceptLanguage(c.GetHeader("Accept-Language"))
	presentation := h.projector.Project(err, locale).Presentation
	c.JSON(presentation.HTTPStatus, gin.H{"code": presentation.Code.String(), "message": presentation.Message, "retryable": presentation.Retryable})
}

func writeData(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": "0", "message": "success", "data": data})
}
