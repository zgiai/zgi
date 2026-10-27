package orgaudit

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

// Handler serves an organization's audit log.
type Handler struct {
	service   *Service
	projector *apptransport.Projector
}

// NewHandler builds the audit log handler.
func NewHandler(service *Service, projector *apptransport.Projector) *Handler {
	if service == nil || projector == nil {
		panic("org audit handler requires a service and a projector")
	}
	return &Handler{service: service, projector: projector}
}

// RegisterRoutes adds GET /organizations/:organization_id/audit-logs to an
// authenticated organizations group.
func (h *Handler) RegisterRoutes(organizations *gin.RouterGroup) {
	organizations.GET("/:organization_id/audit-logs", h.List)
}

// List returns a page of records. Query: page, limit, actor_account_id,
// target_account_id, action, result, from and to (RFC 3339 or Unix seconds).
func (h *Handler) List(c *gin.Context) {
	accountID := util.GetAccountID(c)
	if accountID == "" {
		h.writeError(c, apperror.New(appcatalog.CodeAuthenticationRequired, apperror.WithOperation("org_audit.list")))
		return
	}
	filter, err := parseFilter(c)
	if err != nil {
		h.writeError(c, appError(err, "org_audit.list"))
		return
	}
	page, err := h.service.List(c.Request.Context(), accountID, filter)
	if err != nil {
		h.writeError(c, appError(err, "org_audit.list"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "0", "message": "success", "data": page})
}

func parseFilter(c *gin.Context) (Filter, error) {
	organizationID, ok := parseUUID(c.Param("organization_id"))
	if !ok {
		return Filter{}, ErrFilterInvalid
	}
	filter := Filter{OrganizationID: organizationID}
	var err error
	if filter.Page, err = optionalInt(c.Query("page")); err != nil {
		return Filter{}, err
	}
	if filter.Limit, err = optionalInt(c.Query("limit")); err != nil {
		return Filter{}, err
	}
	if filter.ActorAccountID, err = optionalAccount(c.Query("actor_account_id")); err != nil {
		return Filter{}, err
	}
	if filter.TargetAccountID, err = optionalAccount(c.Query("target_account_id")); err != nil {
		return Filter{}, err
	}
	filter.Action = Action(strings.TrimSpace(c.Query("action")))
	filter.Result = Result(strings.TrimSpace(c.Query("result")))
	if filter.From, err = optionalTime(c.Query("from")); err != nil {
		return Filter{}, err
	}
	if filter.To, err = optionalTime(c.Query("to")); err != nil {
		return Filter{}, err
	}
	return filter, nil
}

func optionalInt(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, ErrFilterInvalid
	}
	return parsed, nil
}

func optionalAccount(value string) (*uuid.UUID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, ok := parseUUID(value)
	if !ok {
		return nil, ErrFilterInvalid
	}
	return &parsed, nil
}

func optionalTime(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		parsed := time.Unix(seconds, 0).UTC()
		return &parsed, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, ErrFilterInvalid
	}
	return &parsed, nil
}

func (h *Handler) writeError(c *gin.Context, err error) {
	locale := apptransport.LocaleFromAcceptLanguage(c.GetHeader("Accept-Language"))
	presentation := h.projector.Project(err, locale).Presentation
	c.JSON(presentation.HTTPStatus, gin.H{"code": presentation.Code.String(), "message": presentation.Message, "retryable": presentation.Retryable})
}
