package digitalhuman

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

const maxDigitalHumanRequestBytes = 256 * 1024

// HandlerService is the DigitalHuman application surface consumed by HTTP.
type HandlerService interface {
	Create(context.Context, Scope, CreateRequest) (*Detail, error)
	Get(context.Context, Scope, uuid.UUID) (*Detail, error)
	List(context.Context, Scope, ListQuery) (*ListResult, error)
	Update(context.Context, Scope, uuid.UUID, UpdateRequest) (*Detail, error)
	UpdateDraft(context.Context, Scope, uuid.UUID, UpdateDraftRequest) (*Version, error)
	Publish(context.Context, Scope, uuid.UUID, PublishRequest) (*Detail, error)
	PublishedVersions(context.Context, Scope, uuid.UUID, ListQuery) (*PublishedVersionsResult, error)
	PreviewVersionRestore(context.Context, Scope, uuid.UUID, uuid.UUID) (*VersionRestorePreview, error)
	RestoreVersion(context.Context, Scope, uuid.UUID, RestoreVersionRequest) (*Detail, error)
	DeleteImpact(context.Context, Scope, uuid.UUID) (*DeleteImpact, error)
	Delete(context.Context, Scope, uuid.UUID, int64) error
	CreatePersonaTemplate(context.Context, Scope, CreatePersonaTemplateRequest) (*PersonaTemplate, error)
	GetPersonaTemplate(context.Context, Scope, uuid.UUID) (*PersonaTemplate, error)
	ListPersonaTemplates(context.Context, Scope, ListQuery) (*PersonaTemplateListResult, error)
	UpdatePersonaTemplate(context.Context, Scope, uuid.UUID, UpdatePersonaTemplateRequest) (*PersonaTemplate, error)
	DeletePersonaTemplate(context.Context, Scope, uuid.UUID, int64) error
	ListDraftResourceBindings(context.Context, Scope, uuid.UUID) (*DraftResourceBindings, error)
	ReplaceDraftResourceBindings(context.Context, Scope, uuid.UUID, ReplaceResourceBindingsRequest) (*DraftResourceBindings, error)
	ListMemories(context.Context, Scope, uuid.UUID, MemoryListQuery) (*MemoryListResult, error)
	UpdateMemoryState(context.Context, Scope, uuid.UUID, UpdateMemoryStateRequest) (*MemoryState, error)
	ApplyMemoryProposal(context.Context, Scope, uuid.UUID, MemoryProposalRequest) (*MemoryMutationResult, error)
	UndoMemory(context.Context, Scope, uuid.UUID, uuid.UUID, MemoryUndoRequest) (*MemoryMutationResult, error)
}

// Handler exposes the DigitalHuman lifecycle API.
type Handler struct {
	service   HandlerService
	projector *apptransport.Projector
}

// NewHandler constructs a DigitalHuman HTTP handler.
func NewHandler(service HandlerService, projector *apptransport.Projector) *Handler {
	if service == nil {
		panic("digital human handler requires service")
	}
	if projector == nil {
		panic("digital human handler requires application error projector")
	}
	return &Handler{service: service, projector: projector}
}

// RegisterRoutes registers workspace-scoped lifecycle routes.
func (h *Handler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/digital-humans")
	group.POST("", h.Create)
	group.GET("", h.List)
	group.GET("/:id", h.Get)
	group.PATCH("/:id", h.Update)
	group.GET("/:id/draft", h.GetDraft)
	group.PATCH("/:id/draft", h.UpdateDraft)
	group.POST("/:id/publish", h.Publish)
	group.GET("/:id/versions", h.PublishedVersions)
	group.GET("/:id/versions/:version_id/restore-preview", h.PreviewVersionRestore)
	group.POST("/:id/versions/restore", h.RestoreVersion)
	group.GET("/:id/delete-impact", h.DeleteImpact)
	group.DELETE("/:id", h.Delete)
	group.GET("/:id/draft/bindings", h.ListDraftResourceBindings)
	group.PUT("/:id/draft/bindings", h.ReplaceDraftResourceBindings)
	group.GET("/:id/memory", h.ListMemories)
	group.PATCH("/:id/memory/state", h.UpdateMemoryState)
	group.POST("/:id/memory/proposals", h.ApplyMemoryProposal)
	group.POST("/:id/memory/operations/:operation_id/undo", h.UndoMemory)

	templates := router.Group("/digital-human-persona-templates")
	templates.POST("", h.CreatePersonaTemplate)
	templates.GET("", h.ListPersonaTemplates)
	templates.GET("/:id", h.GetPersonaTemplate)
	templates.PATCH("/:id", h.UpdatePersonaTemplate)
	templates.DELETE("/:id", h.DeletePersonaTemplate)
}

// ListMemories returns operator-visible subject-owned memory.
func (h *Handler) ListMemories(c *gin.Context) {
	scope, id, ok := h.scopeAndID(c)
	if !ok {
		return
	}
	query, err := parseMemoryListQuery(c)
	if err != nil {
		h.writeError(c, applicationError(err, "digital_human.memory.list"))
		return
	}
	result, err := h.service.ListMemories(c.Request.Context(), scope, id, query)
	if err != nil {
		h.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

// UpdateMemoryState changes the automatic extraction fuse with an epoch CAS.
func (h *Handler) UpdateMemoryState(c *gin.Context) {
	scope, id, ok := h.scopeAndID(c)
	if !ok {
		return
	}
	var request UpdateMemoryStateRequest
	if !h.decode(c, &request) {
		return
	}
	result, err := h.service.UpdateMemoryState(c.Request.Context(), scope, id, request)
	if err != nil {
		h.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

// ApplyMemoryProposal applies an explicit, permission-checked memory change.
func (h *Handler) ApplyMemoryProposal(c *gin.Context) {
	scope, id, ok := h.scopeAndID(c)
	if !ok {
		return
	}
	var request MemoryProposalRequest
	if !h.decode(c, &request) {
		return
	}
	result, err := h.service.ApplyMemoryProposal(c.Request.Context(), scope, id, request)
	if err != nil {
		h.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

// UndoMemory restores a still-current memory mutation within the undo window.
func (h *Handler) UndoMemory(c *gin.Context) {
	scope, id, ok := h.scopeAndID(c)
	if !ok {
		return
	}
	operationID, err := uuid.Parse(strings.TrimSpace(c.Param("operation_id")))
	if err != nil || operationID == uuid.Nil {
		h.writeError(c, applicationError(ErrInvalidRequest, "digital_human.memory.undo"))
		return
	}
	var request MemoryUndoRequest
	if !h.decode(c, &request) {
		return
	}
	result, err := h.service.UndoMemory(c.Request.Context(), scope, id, operationID, request)
	if err != nil {
		h.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

// Create handles DigitalHuman creation.
func (h *Handler) Create(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	var request CreateRequest
	if !h.decode(c, &request) {
		return
	}
	result, err := h.service.Create(c.Request.Context(), scope, request)
	if err != nil {
		h.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusCreated, result)
}

// List handles workspace-scoped pagination.
func (h *Handler) List(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	query, err := parseListQuery(c)
	if err != nil {
		h.writeError(c, applicationError(err, "digital_human.list"))
		return
	}
	result, err := h.service.List(c.Request.Context(), scope, query)
	if err != nil {
		h.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

// Get handles DigitalHuman detail retrieval.
func (h *Handler) Get(c *gin.Context) {
	h.withDetail(c, func(ctx context.Context, scope Scope, id uuid.UUID) (any, error) {
		return h.service.Get(ctx, scope, id)
	})
}

// GetDraft returns only the mutable draft.
func (h *Handler) GetDraft(c *gin.Context) {
	h.withDetail(c, func(ctx context.Context, scope Scope, id uuid.UUID) (any, error) {
		detail, err := h.service.Get(ctx, scope, id)
		if err != nil {
			return nil, err
		}
		return detail.Draft, nil
	})
}

// Update handles aggregate metadata updates.
func (h *Handler) Update(c *gin.Context) {
	scope, id, ok := h.scopeAndID(c)
	if !ok {
		return
	}
	var request UpdateRequest
	if !h.decode(c, &request) {
		return
	}
	result, err := h.service.Update(c.Request.Context(), scope, id, request)
	if err != nil {
		h.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

// UpdateDraft handles mutable identity configuration updates.
func (h *Handler) UpdateDraft(c *gin.Context) {
	scope, id, ok := h.scopeAndID(c)
	if !ok {
		return
	}
	var request UpdateDraftRequest
	if !h.decode(c, &request) {
		return
	}
	result, err := h.service.UpdateDraft(c.Request.Context(), scope, id, request)
	if err != nil {
		h.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

// Publish handles atomic draft publication.
func (h *Handler) Publish(c *gin.Context) {
	scope, id, ok := h.scopeAndID(c)
	if !ok {
		return
	}
	var request PublishRequest
	if !h.decode(c, &request) {
		return
	}
	result, err := h.service.Publish(c.Request.Context(), scope, id, request)
	if err != nil {
		h.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

// PublishedVersions handles immutable version history pagination.
func (h *Handler) PublishedVersions(c *gin.Context) {
	scope, id, ok := h.scopeAndID(c)
	if !ok {
		return
	}
	query, err := parseListQuery(c)
	if err != nil {
		h.writeError(c, applicationError(err, "digital_human.versions.list"))
		return
	}
	result, err := h.service.PublishedVersions(c.Request.Context(), scope, id, query)
	if err != nil {
		h.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

// PreviewVersionRestore returns the exact draft and binding impact before restoration.
func (h *Handler) PreviewVersionRestore(c *gin.Context) {
	scope, id, ok := h.scopeAndID(c)
	if !ok {
		return
	}
	versionID, err := uuid.Parse(strings.TrimSpace(c.Param("version_id")))
	if err != nil || versionID == uuid.Nil {
		h.writeError(c, applicationError(ErrInvalidRequest, "digital_human.version.restore_preview"))
		return
	}
	result, err := h.service.PreviewVersionRestore(c.Request.Context(), scope, id, versionID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

// RestoreVersion copies one immutable publication into the current draft.
func (h *Handler) RestoreVersion(c *gin.Context) {
	scope, id, ok := h.scopeAndID(c)
	if !ok {
		return
	}
	var request RestoreVersionRequest
	if !h.decode(c, &request) {
		return
	}
	result, err := h.service.RestoreVersion(c.Request.Context(), scope, id, request)
	if err != nil {
		h.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

// DeleteImpact handles deletion preflight.
func (h *Handler) DeleteImpact(c *gin.Context) {
	h.withDetail(c, func(ctx context.Context, scope Scope, id uuid.UUID) (any, error) {
		return h.service.DeleteImpact(ctx, scope, id)
	})
}

// Delete handles revision-bound soft deletion.
func (h *Handler) Delete(c *gin.Context) {
	scope, id, ok := h.scopeAndID(c)
	if !ok {
		return
	}
	expectedRevision, err := strconv.ParseInt(strings.TrimSpace(c.Query("expected_revision")), 10, 64)
	if err != nil || expectedRevision < 1 {
		h.writeError(c, applicationError(ErrInvalidRequest, "digital_human.delete"))
		return
	}
	if err := h.service.Delete(c.Request.Context(), scope, id, expectedRevision); err != nil {
		h.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, gin.H{"deleted": true})
}

func (h *Handler) CreatePersonaTemplate(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	var request CreatePersonaTemplateRequest
	if !h.decode(c, &request) {
		return
	}
	result, err := h.service.CreatePersonaTemplate(c.Request.Context(), scope, request)
	if err != nil {
		h.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusCreated, result)
}

func (h *Handler) ListPersonaTemplates(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	query, err := parseListQuery(c)
	if err != nil {
		h.writeError(c, applicationError(err, "digital_human.persona_template.list"))
		return
	}
	result, err := h.service.ListPersonaTemplates(c.Request.Context(), scope, query)
	if err != nil {
		h.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

func (h *Handler) GetPersonaTemplate(c *gin.Context) {
	h.withScopedID(c, func(ctx context.Context, scope Scope, id uuid.UUID) (any, error) {
		return h.service.GetPersonaTemplate(ctx, scope, id)
	})
}

func (h *Handler) UpdatePersonaTemplate(c *gin.Context) {
	h.withRequest(c, func(ctx context.Context, scope Scope, id uuid.UUID) (any, error) {
		var request UpdatePersonaTemplateRequest
		if !h.decode(c, &request) {
			return nil, nil
		}
		return h.service.UpdatePersonaTemplate(ctx, scope, id, request)
	})
}

func (h *Handler) DeletePersonaTemplate(c *gin.Context) {
	scope, id, ok := h.scopeAndID(c)
	if !ok {
		return
	}
	expectedRevision, err := strconv.ParseInt(strings.TrimSpace(c.Query("expected_revision")), 10, 64)
	if err != nil || expectedRevision < 1 {
		h.writeError(c, applicationError(ErrInvalidRequest, "digital_human.persona_template.delete"))
		return
	}
	if err := h.service.DeletePersonaTemplate(c.Request.Context(), scope, id, expectedRevision); err != nil {
		h.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, gin.H{"deleted": true})
}

func (h *Handler) ListDraftResourceBindings(c *gin.Context) {
	h.withScopedID(c, func(ctx context.Context, scope Scope, id uuid.UUID) (any, error) {
		return h.service.ListDraftResourceBindings(ctx, scope, id)
	})
}

func (h *Handler) ReplaceDraftResourceBindings(c *gin.Context) {
	h.withDigitalHumanRequest(c, func(ctx context.Context, scope Scope, id uuid.UUID) (any, error) {
		var request ReplaceResourceBindingsRequest
		if !h.decode(c, &request) {
			return nil, nil
		}
		return h.service.ReplaceDraftResourceBindings(ctx, scope, id, request)
	})
}

func (h *Handler) withDetail(c *gin.Context, load func(context.Context, Scope, uuid.UUID) (any, error)) {
	h.withScopedID(c, load)
}

func (h *Handler) withScopedID(c *gin.Context, load func(context.Context, Scope, uuid.UUID) (any, error)) {
	scope, id, ok := h.scopeAndID(c)
	if !ok {
		return
	}
	result, err := load(c.Request.Context(), scope, id)
	if err != nil {
		h.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

func (h *Handler) withDigitalHumanRequest(c *gin.Context, action func(context.Context, Scope, uuid.UUID) (any, error)) {
	h.withRequest(c, action)
}

func (h *Handler) withRequest(c *gin.Context, action func(context.Context, Scope, uuid.UUID) (any, error)) {
	scope, id, ok := h.scopeAndID(c)
	if !ok {
		return
	}
	result, err := action(c.Request.Context(), scope, id)
	if c.IsAborted() || result == nil && err == nil {
		return
	}
	if err != nil {
		h.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

func (h *Handler) scopeAndID(c *gin.Context) (Scope, uuid.UUID, bool) {
	scope, ok := h.scope(c)
	if !ok {
		return Scope{}, uuid.Nil, false
	}
	id, err := uuid.Parse(strings.TrimSpace(c.Param("id")))
	if err != nil || id == uuid.Nil {
		h.writeError(c, applicationError(ErrInvalidRequest, "digital_human.request"))
		return Scope{}, uuid.Nil, false
	}
	return scope, id, true
}

func (h *Handler) scope(c *gin.Context) (Scope, bool) {
	organizationID, organizationErr := uuid.Parse(strings.TrimSpace(util.GetOrganizationID(c)))
	workspaceID, workspaceErr := uuid.Parse(strings.TrimSpace(util.GetWorkspaceID(c)))
	actorID, actorErr := uuid.Parse(strings.TrimSpace(util.GetAccountID(c)))
	if organizationErr != nil || workspaceErr != nil || actorErr != nil || organizationID == uuid.Nil || workspaceID == uuid.Nil || actorID == uuid.Nil {
		h.writeError(c, apperror.New(appcatalog.CodeAuthenticationRequired, apperror.WithOperation("digital_human.scope")))
		return Scope{}, false
	}
	return Scope{OrganizationID: organizationID, WorkspaceID: workspaceID, ActorID: actorID}, true
}

func (h *Handler) decode(c *gin.Context, target any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxDigitalHumanRequestBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		h.writeError(c, applicationError(ErrInvalidRequest, "digital_human.request.decode"))
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		h.writeError(c, applicationError(ErrInvalidRequest, "digital_human.request.decode"))
		return false
	}
	return true
}

func (h *Handler) writeError(c *gin.Context, err error) {
	locale := apptransport.LocaleFromAcceptLanguage(c.GetHeader("Accept-Language"))
	presentation := h.projector.Project(err, locale).Presentation
	c.JSON(presentation.HTTPStatus, gin.H{
		"code":      presentation.Code.String(),
		"message":   presentation.Message,
		"retryable": presentation.Retryable,
	})
}

func parseListQuery(c *gin.Context) (ListQuery, error) {
	query := ListQuery{Search: c.Query("search")}
	if raw := strings.TrimSpace(c.Query("page")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return ListQuery{}, ErrInvalidRequest
		}
		query.Page = value
	}
	if raw := strings.TrimSpace(c.Query("page_size")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return ListQuery{}, ErrInvalidRequest
		}
		query.PageSize = value
	}
	return query, nil
}

func parseMemoryListQuery(c *gin.Context) (MemoryListQuery, error) {
	base, err := parseListQuery(c)
	if err != nil {
		return MemoryListQuery{}, err
	}
	return MemoryListQuery{
		Page: base.Page, PageSize: base.PageSize, Search: base.Search,
		Class:     memoryruntime.MemoryClass(c.Query("memory_class")),
		Type:      memoryruntime.MemoryType(c.Query("memory_type")),
		ScopeType: memoryruntime.MemoryScopeType(c.Query("scope_type")),
		ScopeID:   c.Query("scope_id"), Status: MemoryStatus(c.Query("status")), Source: c.Query("source"),
	}, nil
}

func writeDigitalHumanData(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"code": "0", "message": "success", "data": data})
}
