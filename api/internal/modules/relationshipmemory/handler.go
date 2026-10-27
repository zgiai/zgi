package relationshipmemory

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	ErrorCodeRelationshipNotFound = "user_memory.relationship.not_found"
	ErrorCodeAccessDenied         = "user_memory.access.denied"
	ErrorCodeRevisionConflict     = "user_memory.revision.conflict"
	ErrorCodePolicyRejected       = "user_memory.policy.rejected"
	ErrorCodeUndoUnavailable      = "user_memory.undo.unavailable"
	ErrorCodeCapacityExceeded     = "user_memory.capacity.exceeded"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/user-memories")
	group.GET("/relationships", h.ListRelationships)
	group.GET("/relationships/:id", h.GetRelationship)
	group.PATCH("/relationships/:id/settings", h.UpdateSettings)
	group.PUT("/relationships/:id/items/:key", h.PutItem)
	group.DELETE("/relationships/:id/items/:key", h.DeleteItem)
	group.DELETE("/relationships/:id/items", h.ClearItems)
	group.DELETE("/relationships/:id", h.DeleteRelationship)
	group.GET("/relationships/:id/export", h.Export)
	group.POST("/relationships/:id/operations/:operation_id/undo", h.Undo)
	group.GET("/relationships/:id/history", h.History)
	group.POST("/relationships/:id/history/:version_id/revert", h.RevertVersion)
	group.GET("/relationships/:id/archive", h.Archive)
	group.GET("/relationships/:id/notes", h.Notes)
	group.DELETE("/relationships/:id/notes/:key", h.DeleteNote)
	group.PATCH("/relationships/:id/archive/:entry_id", h.UpdateArchiveEntry)
	group.DELETE("/relationships/:id/archive/:entry_id", h.DeleteArchiveEntry)
}

func (h *Handler) ListRelationships(c *gin.Context) {
	organizationID, accountID, ok := relationshipRequestScope(c)
	if !ok {
		return
	}
	req := ListRelationshipsRequest{OrganizationID: organizationID, AccountID: accountID, ConsumerType: strings.TrimSpace(c.Query("consumer_type")), Status: strings.TrimSpace(c.Query("status")), Page: queryInt(c, "page", 1), PageSize: queryInt(c, "page_size", 20)}
	if rawWorkspaceID := strings.TrimSpace(c.Query("workspace_id")); rawWorkspaceID != "" {
		workspaceID, err := uuid.Parse(rawWorkspaceID)
		if err != nil {
			writeMemoryError(c, ErrPolicyRejected)
			return
		}
		req.WorkspaceID = &workspaceID
	}
	result, err := h.service.ListAccountRelationships(c.Request.Context(), req)
	if err != nil {
		writeMemoryError(c, err)
		return
	}
	response.Success(c, result)
}

// relationshipDetailResponse adds whether automatic memory can run at all, so
// the person's switch is not shown as working when the assistant or the
// deployment has it off.
type relationshipDetailResponse struct {
	*RelationshipDetail
	AutoCaptureAvailable bool `json:"auto_capture_available"`
}

func (h *Handler) GetRelationship(c *gin.Context) {
	detail, ok := h.accountRelationship(c)
	if !ok {
		return
	}
	contract, err := h.managementContract(c, detail)
	if err != nil {
		writeMemoryError(c, err)
		return
	}
	response.Success(c, relationshipDetailResponse{
		RelationshipDetail:   detail,
		AutoCaptureAvailable: contract.Enabled && contract.AutoCaptureAllowed && AutomaticExtractionEnabled(),
	})
}

type updateSettingsPayload struct {
	MemoryEnabled      *bool `json:"memory_enabled"`
	AutoCaptureEnabled *bool `json:"auto_capture_enabled"`
	ExpectedRevision   int64 `json:"expected_revision" binding:"required"`
}

func (h *Handler) UpdateSettings(c *gin.Context) {
	detail, ok := h.accountRelationship(c)
	if !ok {
		return
	}
	if !detail.CanManage {
		writeMemoryError(c, ErrAccessDenied)
		return
	}
	var payload updateSettingsPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		writeMemoryError(c, ErrPolicyRejected)
		return
	}
	contract, err := h.managementContract(c, detail)
	if err != nil {
		writeMemoryError(c, err)
		return
	}
	if payload.MemoryEnabled != nil && *payload.MemoryEnabled && !contract.Enabled {
		writeMemoryError(c, fmt.Errorf("%w: relationship memory is not enabled by the published consumer configuration", ErrPolicyRejected))
		return
	}
	if payload.AutoCaptureEnabled != nil && *payload.AutoCaptureEnabled && !contract.AutoCaptureAllowed {
		writeMemoryError(c, fmt.Errorf("%w: automatic capture is not allowed by the published consumer configuration", ErrPolicyRejected))
		return
	}
	actorID := detail.SubjectID
	result, err := h.service.UpdateSettings(c.Request.Context(), detail.ID, UpdateSettingsRequest{MemoryEnabled: payload.MemoryEnabled, AutoCaptureEnabled: payload.AutoCaptureEnabled, ExpectedRevision: payload.ExpectedRevision, ActorType: "user", ActorID: &actorID, Source: "profile"})
	if err != nil {
		writeMemoryError(c, err)
		return
	}
	response.Success(c, result)
}

type putItemPayload struct {
	DisplayName      string `json:"display_name"`
	Category         string `json:"category"`
	Content          string `json:"content" binding:"required"`
	ExpectedRevision int64  `json:"expected_revision"`
}

func (h *Handler) PutItem(c *gin.Context) {
	detail, ok := h.accountRelationship(c)
	if !ok {
		return
	}
	if !detail.CanManage {
		writeMemoryError(c, ErrAccessDenied)
		return
	}
	// The DigitalHuman's notes on the person are its own; the person may
	// delete them but not edit them (ADR 0015).
	if IsNoteKey(c.Param("key")) {
		writeMemoryError(c, ErrAccessDenied)
		return
	}
	var payload putItemPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		writeMemoryError(c, ErrPolicyRejected)
		return
	}
	contract, err := h.managementContract(c, detail)
	if err != nil {
		writeMemoryError(c, err)
		return
	}
	actorID := detail.SubjectID
	result, err := h.service.PutItem(c.Request.Context(), detail.ID, PutItemRequest{Key: c.Param("key"), DisplayName: payload.DisplayName, Category: payload.Category, Content: payload.Content, ExpectedRevision: payload.ExpectedRevision, Contract: contract, Metadata: MutationMetadata{ActorType: "user", ActorID: &actorID, Source: "profile", SourceKind: SourceManager}})
	if err != nil {
		writeMemoryError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) DeleteItem(c *gin.Context) {
	detail, ok := h.accountRelationship(c)
	if !ok {
		return
	}
	if !detail.CanManage {
		writeMemoryError(c, ErrAccessDenied)
		return
	}
	expectedRevision, err := strconv.ParseInt(c.Query("expected_revision"), 10, 64)
	if err != nil {
		writeMemoryError(c, ErrPolicyRejected)
		return
	}
	actorID := detail.SubjectID
	result, err := h.service.DeleteItem(c.Request.Context(), detail.ID, c.Param("key"), expectedRevision, MutationMetadata{ActorType: "user", ActorID: &actorID, Source: "profile", SourceKind: SourceManager})
	if err != nil {
		writeMemoryError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) ClearItems(c *gin.Context) {
	detail, ok := h.accountRelationship(c)
	if !ok {
		return
	}
	if !detail.CanManage {
		writeMemoryError(c, ErrAccessDenied)
		return
	}
	actorID := detail.SubjectID
	if err := h.service.ClearItems(c.Request.Context(), detail.ID, MutationMetadata{ActorType: "user", ActorID: &actorID, Source: "profile", SourceKind: SourceManager}); err != nil {
		writeMemoryError(c, err)
		return
	}
	response.Success(c, gin.H{"result": "success"})
}

func (h *Handler) DeleteRelationship(c *gin.Context) {
	detail, ok := h.accountRelationship(c)
	if !ok {
		return
	}
	if err := h.service.PermanentlyDeleteRelationship(c.Request.Context(), detail.ID); err != nil {
		writeMemoryError(c, err)
		return
	}
	response.Success(c, gin.H{"result": "success"})
}

func (h *Handler) Export(c *gin.Context) {
	detail, ok := h.accountRelationship(c)
	if !ok {
		return
	}
	result, err := h.service.Export(c.Request.Context(), detail.ID)
	if err != nil {
		writeMemoryError(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="user-memory-`+detail.ID.String()+`.json"`)
	response.Success(c, result)
}

func (h *Handler) Undo(c *gin.Context) {
	detail, ok := h.accountRelationship(c)
	if !ok {
		return
	}
	if !detail.CanManage {
		writeMemoryError(c, ErrAccessDenied)
		return
	}
	operationID, err := uuid.Parse(c.Param("operation_id"))
	if err != nil {
		writeMemoryError(c, ErrUndoUnavailable)
		return
	}
	actorID := detail.SubjectID
	result, err := h.service.Undo(c.Request.Context(), detail.ID, operationID, MutationMetadata{ActorType: "user", ActorID: &actorID, Source: "profile", SourceKind: SourceManager})
	if err != nil {
		writeMemoryError(c, err)
		return
	}
	response.Success(c, gin.H{"operation_id": operationID, "item": result})
}

// History lists the relationship's memory changes, newest first.
func (h *Handler) History(c *gin.Context) {
	detail, ok := h.accountRelationship(c)
	if !ok {
		return
	}
	request, ok := historyRequestFromQuery(c)
	if !ok {
		writeMemoryError(c, ErrPolicyRejected)
		return
	}
	page, err := h.service.History(c.Request.Context(), detail.ID, request)
	if err != nil {
		writeMemoryError(c, err)
		return
	}
	response.Success(c, page)
}

// RevertVersion undoes the latest change to one memory file.
func (h *Handler) RevertVersion(c *gin.Context) {
	detail, ok := h.accountRelationship(c)
	if !ok {
		return
	}
	if !detail.CanManage {
		writeMemoryError(c, ErrAccessDenied)
		return
	}
	versionID, err := uuid.Parse(c.Param("version_id"))
	if err != nil {
		writeMemoryError(c, ErrUndoUnavailable)
		return
	}
	contract, err := h.managementContract(c, detail)
	if err != nil {
		writeMemoryError(c, err)
		return
	}
	actorID := detail.SubjectID
	item, err := h.service.RevertVersion(c.Request.Context(), detail.ID, versionID, contract, MutationMetadata{ActorType: "user", ActorID: &actorID, Source: "profile", SourceKind: SourceManager})
	if err != nil {
		writeMemoryError(c, err)
		return
	}
	response.Success(c, gin.H{"version_id": versionID, "item": item})
}

// Notes lists what a DigitalHuman keeps on the person (ADR 0014
// /user/notes/), read-only. Other consumers keep no notes.
func (h *Handler) Notes(c *gin.Context) {
	detail, ok := h.accountRelationship(c)
	if !ok {
		return
	}
	notes := []InstanceNote{}
	if detail.ConsumerType == ConsumerDigitalHuman {
		var err error
		if notes, err = h.service.InstanceNotes(c.Request.Context(), detail.WorkspaceID, detail.ConsumerID, detail.SubjectID); err != nil {
			writeMemoryError(c, err)
			return
		}
	}
	response.Success(c, gin.H{"notes": notes})
}

// DeleteNote removes one of the DigitalHuman's notes on the person. The
// person may delete notes but not edit them (ADR 0015).
func (h *Handler) DeleteNote(c *gin.Context) {
	detail, ok := h.accountRelationship(c)
	if !ok {
		return
	}
	if !detail.CanManage {
		writeMemoryError(c, ErrAccessDenied)
		return
	}
	expectedRevision, err := strconv.ParseInt(c.Query("expected_revision"), 10, 64)
	if err != nil || !ValidNoteName(c.Param("key")) {
		writeMemoryError(c, ErrPolicyRejected)
		return
	}
	actorID := detail.SubjectID
	result, err := h.service.DeleteItem(c.Request.Context(), detail.ID, NoteKey(c.Param("key")), expectedRevision, MutationMetadata{ActorType: "user", ActorID: &actorID, Source: "profile", SourceKind: SourceManager})
	if err != nil {
		writeMemoryError(c, err)
		return
	}
	response.Success(c, result)
}

// Archive lists the relationship's archive (ADR 0014), newest first.
func (h *Handler) Archive(c *gin.Context) {
	detail, ok := h.accountRelationship(c)
	if !ok {
		return
	}
	entries, err := h.service.ArchiveEntries(c.Request.Context(), detail.ID, 0)
	if err != nil {
		writeMemoryError(c, err)
		return
	}
	response.Success(c, gin.H{"entries": entries})
}

type archiveEntryRequest struct {
	Content string `json:"content" binding:"required"`
}

// UpdateArchiveEntry replaces one archive entry's text.
func (h *Handler) UpdateArchiveEntry(c *gin.Context) {
	detail, entryID, ok := h.manageArchiveEntry(c)
	if !ok {
		return
	}
	var request archiveEntryRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeMemoryError(c, ErrPolicyRejected)
		return
	}
	actorID := detail.SubjectID
	entry, err := h.service.UpdateArchiveEntry(c.Request.Context(), detail.ID, entryID, request.Content, MutationMetadata{ActorType: "user", ActorID: &actorID, Source: "profile", SourceKind: SourceManager})
	if err != nil {
		writeMemoryError(c, err)
		return
	}
	response.Success(c, entry)
}

// DeleteArchiveEntry removes one archive entry.
func (h *Handler) DeleteArchiveEntry(c *gin.Context) {
	detail, entryID, ok := h.manageArchiveEntry(c)
	if !ok {
		return
	}
	actorID := detail.SubjectID
	if err := h.service.DeleteArchiveEntry(c.Request.Context(), detail.ID, entryID, MutationMetadata{ActorType: "user", ActorID: &actorID, Source: "profile", SourceKind: SourceManager}); err != nil {
		writeMemoryError(c, err)
		return
	}
	response.Success(c, gin.H{"entry_id": entryID})
}

func (h *Handler) manageArchiveEntry(c *gin.Context) (*RelationshipDetail, uuid.UUID, bool) {
	detail, ok := h.accountRelationship(c)
	if !ok {
		return nil, uuid.Nil, false
	}
	if !detail.CanManage {
		writeMemoryError(c, ErrAccessDenied)
		return nil, uuid.Nil, false
	}
	entryID, err := uuid.Parse(c.Param("entry_id"))
	if err != nil {
		writeMemoryError(c, ErrRelationshipNotFound)
		return nil, uuid.Nil, false
	}
	return detail, entryID, true
}

// historyRequestFromQuery reads key, limit, and before (RFC 3339) from the
// query string.
func historyRequestFromQuery(c *gin.Context) (HistoryRequest, bool) {
	request := HistoryRequest{Key: strings.TrimSpace(c.Query("key"))}
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 0 {
			return request, false
		}
		request.Limit = limit
	}
	if raw := strings.TrimSpace(c.Query("before")); raw != "" {
		before, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return request, false
		}
		request.Before = &before
	}
	return request, true
}

// HistoryRequestFromQuery is historyRequestFromQuery for other modules'
// handlers that expose the same history.
func HistoryRequestFromQuery(c *gin.Context) (HistoryRequest, bool) {
	return historyRequestFromQuery(c)
}

func (h *Handler) accountRelationship(c *gin.Context) (*RelationshipDetail, bool) {
	organizationID, accountID, ok := relationshipRequestScope(c)
	if !ok {
		return nil, false
	}
	relationshipID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeMemoryError(c, ErrRelationshipNotFound)
		return nil, false
	}
	detail, err := h.service.GetAccountRelationship(c.Request.Context(), organizationID, accountID, relationshipID)
	if err != nil {
		writeMemoryError(c, err)
		return nil, false
	}
	return detail, true
}

func (h *Handler) managementContract(c *gin.Context, detail *RelationshipDetail) (ConsumerMemoryContract, error) {
	if detail.ConsumerType == ConsumerDigitalHuman {
		var row struct{ Revision string }
		err := h.service.db.WithContext(c.Request.Context()).Table("digital_humans AS consumer").
			Select("published.revision").
			Joins("JOIN digital_human_versions AS published ON published.id = consumer.published_version_id AND published.state = ?", "published").
			Where("consumer.id = ? AND consumer.workspace_id = ? AND consumer.deleted_at IS NULL", detail.ConsumerID, detail.WorkspaceID).
			Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ConsumerMemoryContract{DefinitionMode: "files"}, nil
		}
		if err != nil {
			return ConsumerMemoryContract{}, err
		}
		// A person's memory with a DigitalHuman is files, as with an Agent (ADR 0019).
		return ConsumerMemoryContract{Enabled: true, DefinitionMode: "files", AutoCaptureAllowed: true, ConfigRevision: row.Revision, RenderBudgetChars: defaultRenderBudget}, nil
	}
	var row struct{ ConfigSnapshot datatypes.JSON }
	err := h.service.db.WithContext(c.Request.Context()).Table("agent_published_versions").
		Select("config_snapshot").
		Where("workspace_id = ? AND agent_id = ? AND deleted_at IS NULL", detail.WorkspaceID, detail.ConsumerID).
		Order("created_at DESC").Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ConsumerMemoryContract{DefinitionMode: "files"}, nil
	}
	if err != nil {
		return ConsumerMemoryContract{}, err
	}
	var snapshot struct {
		Enabled            bool `json:"agent_memory_enabled"`
		AutoCaptureAllowed bool `json:"agent_memory_auto_extraction_enabled"`
	}
	if err := json.Unmarshal(row.ConfigSnapshot, &snapshot); err != nil {
		return ConsumerMemoryContract{}, fmt.Errorf("%w: invalid published Agent memory configuration", ErrPolicyRejected)
	}
	return ConsumerMemoryContract{Enabled: snapshot.Enabled, DefinitionMode: "files", AutoCaptureAllowed: snapshot.AutoCaptureAllowed, RenderBudgetChars: defaultRenderBudget}, nil
}

func relationshipRequestScope(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	organizationID, organizationErr := uuid.Parse(strings.TrimSpace(util.GetOrganizationID(c)))
	accountID, accountErr := uuid.Parse(strings.TrimSpace(util.GetAccountID(c)))
	if organizationErr != nil || accountErr != nil {
		writeMemoryError(c, ErrAccessDenied)
		return uuid.Nil, uuid.Nil, false
	}
	return organizationID, accountID, true
}

func queryInt(c *gin.Context, key string, fallback int) int {
	value, err := strconv.Atoi(c.Query(key))
	if err != nil {
		return fallback
	}
	return value
}

func writeMemoryError(c *gin.Context, err error) {
	status, code, message := http.StatusInternalServerError, "internal_error", "Internal server error"
	switch {
	case errors.Is(err, ErrRelationshipNotFound):
		status, code, message = http.StatusNotFound, ErrorCodeRelationshipNotFound, ErrRelationshipNotFound.Error()
	case errors.Is(err, ErrAccessDenied):
		status, code, message = http.StatusForbidden, ErrorCodeAccessDenied, ErrAccessDenied.Error()
	case errors.Is(err, ErrRevisionConflict):
		status, code, message = http.StatusConflict, ErrorCodeRevisionConflict, ErrRevisionConflict.Error()
	case errors.Is(err, ErrPolicyRejected), errors.Is(err, usersubject.ErrInvalidUser):
		status, code, message = http.StatusBadRequest, ErrorCodePolicyRejected, err.Error()
	case errors.Is(err, ErrUndoUnavailable):
		status, code, message = http.StatusConflict, ErrorCodeUndoUnavailable, ErrUndoUnavailable.Error()
	case errors.Is(err, ErrCapacityExceeded):
		status, code, message = http.StatusConflict, ErrorCodeCapacityExceeded, ErrCapacityExceeded.Error()
	}
	c.JSON(status, gin.H{"code": code, "message": message})
}
