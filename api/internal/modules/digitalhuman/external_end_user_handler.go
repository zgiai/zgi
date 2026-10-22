package digitalhuman

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/relationshipmemory"
)

// EndUserNotificationCounts adds the unread count and read-all to an API end
// user's notifications.
type EndUserNotificationCounts interface {
	EndUserUnreadCount(ctx context.Context, organizationID uuid.UUID, subjectKind string, subjectID uuid.UUID) (int64, error)
	MarkAllEndUserNotificationsRead(ctx context.Context, organizationID uuid.UUID, subjectKind string, subjectID uuid.UUID) error
}

// EndUserMemoryItem is one thing the DigitalHuman remembers about an API end
// user, as that user may read it.
type EndUserMemoryItem struct {
	Key         string    `json:"key"`
	DisplayName string    `json:"display_name,omitempty"`
	Category    string    `json:"category,omitempty"`
	Content     string    `json:"content"`
	Revision    int64     `json:"revision"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// EndUserMemory is everything the DigitalHuman remembers about an API end
// user. MemoryEnabled is false when the user has no memory with it yet or it
// was turned off.
type EndUserMemory struct {
	MemoryEnabled bool                `json:"memory_enabled"`
	Items         []EndUserMemoryItem `json:"items"`
}

// registerAPIEndUserRoutes lets an API end user see and erase what the
// DigitalHuman remembers about them, read their unread count, and preview a
// task schedule before creating it.
func (h *ExternalHandler) registerAPIEndUserRoutes(group *gin.RouterGroup) {
	group.GET("/memory", h.GetAPIEndUserMemory)
	group.DELETE("/memory/items/:key", h.DeleteAPIEndUserMemoryItem)
	group.POST("/memory/clear", h.ClearAPIEndUserMemory)
	group.POST("/tasks/schedule-preview", h.PreviewAPITaskSchedule)
	if _, ok := h.notifications.(EndUserNotificationCounts); ok {
		group.GET("/notifications/unread-count", h.CountAPIUnreadNotifications)
		group.POST("/notifications/read-all", h.ReadAllAPINotifications)
	}
}

func (h *ExternalHandler) endUserMemory() *relationshipmemory.Service {
	if h.access == nil || h.access.db == nil {
		return nil
	}
	return relationshipmemory.NewService(h.access.db)
}

// endUserRelationship finds the end user's memory with this DigitalHuman.
// A user who never had memory with it has none to show or erase.
func (h *ExternalHandler) endUserRelationship(c *gin.Context, user string, operation string) (*relationshipmemory.Service, *relationshipmemory.UserMemoryRelationship, TaskAccessContext, bool) {
	access, ok := h.apiTaskScope(c, user)
	if !ok {
		return nil, nil, TaskAccessContext{}, false
	}
	service := h.endUserMemory()
	if service == nil || access.Owner.Subject.ID == uuid.Nil {
		h.writeApplicationError(c, ErrMemoryPolicyRejected, operation)
		return nil, nil, TaskAccessContext{}, false
	}
	relationship, err := service.FindRelationship(c.Request.Context(), access.WorkspaceID, access.Owner.Subject.ID, relationshipmemory.ConsumerDigitalHuman, access.DigitalHumanID)
	if errors.Is(err, relationshipmemory.ErrRelationshipNotFound) {
		return service, nil, access, true
	}
	if err != nil {
		h.writeApplicationError(c, err, operation)
		return nil, nil, TaskAccessContext{}, false
	}
	return service, relationship, access, true
}

// GetAPIEndUserMemory lists what the DigitalHuman remembers about the user.
func (h *ExternalHandler) GetAPIEndUserMemory(c *gin.Context) {
	const operation = "digital_human.external.memory.get"
	service, relationship, _, ok := h.endUserRelationship(c, userFromQuery(c), operation)
	if !ok {
		return
	}
	result := EndUserMemory{Items: []EndUserMemoryItem{}}
	if relationship == nil {
		writeExternalData(c, http.StatusOK, result)
		return
	}
	export, err := service.Export(c.Request.Context(), relationship.ID)
	if err != nil {
		h.writeApplicationError(c, endUserMemoryError(err), operation)
		return
	}
	result.MemoryEnabled = relationship.Status == relationshipmemory.RelationshipActive && relationship.MemoryEnabled
	for _, item := range export.Items {
		result.Items = append(result.Items, EndUserMemoryItem{Key: item.MemoryKey, DisplayName: item.DisplayName, Category: item.Category,
			Content: item.ContentPayload, Revision: item.Revision, UpdatedAt: item.UpdatedAt})
	}
	writeExternalData(c, http.StatusOK, result)
}

// DeleteAPIEndUserMemoryItem forgets one remembered item. The caller passes
// the revision it saw, so a newer change is not erased by mistake.
func (h *ExternalHandler) DeleteAPIEndUserMemoryItem(c *gin.Context) {
	const operation = "digital_human.external.memory.delete"
	service, relationship, _, ok := h.endUserRelationship(c, userFromQuery(c), operation)
	if !ok {
		return
	}
	revision, err := strconv.ParseInt(strings.TrimSpace(c.Query("expected_revision")), 10, 64)
	if err != nil || revision <= 0 {
		h.writeApplicationError(c, ErrInvalidRequest, operation)
		return
	}
	if relationship == nil {
		h.writeApplicationError(c, ErrMemoryNotFound, operation)
		return
	}
	if _, err := service.DeleteItem(c.Request.Context(), relationship.ID, c.Param("key"), revision, relationshipmemory.MutationMetadata{ActorType: "user", Source: "digital_human_api"}); err != nil {
		h.writeApplicationError(c, endUserMemoryError(err), operation)
		return
	}
	writeExternalData(c, http.StatusOK, gin.H{"deleted": true})
}

// ClearAPIEndUserMemory forgets everything remembered about the user.
func (h *ExternalHandler) ClearAPIEndUserMemory(c *gin.Context) {
	const operation = "digital_human.external.memory.clear"
	var request struct {
		User string `json:"user"`
	}
	if !h.base.decode(c, &request) {
		return
	}
	service, relationship, _, ok := h.endUserRelationship(c, request.User, operation)
	if !ok {
		return
	}
	if relationship != nil {
		if err := service.ClearItems(c.Request.Context(), relationship.ID, relationshipmemory.MutationMetadata{ActorType: "user", Source: "digital_human_api"}); err != nil {
			h.writeApplicationError(c, endUserMemoryError(err), operation)
			return
		}
	}
	writeExternalData(c, http.StatusOK, gin.H{"cleared": true})
}

func endUserMemoryError(err error) error {
	switch {
	case errors.Is(err, relationshipmemory.ErrRelationshipNotFound):
		return ErrMemoryNotFound
	case errors.Is(err, relationshipmemory.ErrRevisionConflict):
		return ErrMemoryConflict
	case errors.Is(err, relationshipmemory.ErrPolicyRejected), errors.Is(err, relationshipmemory.ErrAccessDenied):
		return ErrMemoryPolicyRejected
	default:
		return err
	}
}

// PreviewAPITaskSchedule returns when a schedule would next run, so clients
// need not validate cron expressions themselves.
func (h *ExternalHandler) PreviewAPITaskSchedule(c *gin.Context) {
	const operation = "digital_human.external.task.schedule_preview"
	var request struct {
		User string `json:"user"`
		TaskSchedulePreviewRequest
	}
	if !h.base.decode(c, &request) {
		return
	}
	access, ok := h.apiTaskScope(c, request.User)
	if !ok {
		return
	}
	result, err := h.tasks.PreviewTaskSchedule(c.Request.Context(), access, request.TaskSchedulePreviewRequest)
	if err != nil {
		h.writeTaskApplicationError(c, err, operation)
		return
	}
	writeExternalData(c, http.StatusOK, result)
}

// CountAPIUnreadNotifications returns how many notifications the user has not read.
func (h *ExternalHandler) CountAPIUnreadNotifications(c *gin.Context) {
	const operation = "digital_human.external.notification.unread_count"
	access, ok := h.apiTaskScope(c, userFromQuery(c))
	if !ok {
		return
	}
	counts, _ := h.notifications.(EndUserNotificationCounts)
	if counts == nil || access.Owner.Subject.ID == uuid.Nil {
		h.writeTaskApplicationError(c, ErrTaskPolicyRejected, operation)
		return
	}
	count, err := counts.EndUserUnreadCount(c.Request.Context(), access.OrganizationID, string(access.Owner.Subject.Kind), access.Owner.Subject.ID)
	if err != nil {
		h.writeTaskApplicationError(c, err, operation)
		return
	}
	writeExternalData(c, http.StatusOK, gin.H{"count": count})
}

// ReadAllAPINotifications marks every notification of the user read.
func (h *ExternalHandler) ReadAllAPINotifications(c *gin.Context) {
	const operation = "digital_human.external.notification.read_all"
	var request struct {
		User string `json:"user"`
	}
	if !h.base.decode(c, &request) {
		return
	}
	access, ok := h.apiTaskScope(c, request.User)
	if !ok {
		return
	}
	counts, _ := h.notifications.(EndUserNotificationCounts)
	if counts == nil || access.Owner.Subject.ID == uuid.Nil {
		h.writeTaskApplicationError(c, ErrTaskPolicyRejected, operation)
		return
	}
	if err := counts.MarkAllEndUserNotificationsRead(c.Request.Context(), access.OrganizationID, string(access.Owner.Subject.Kind), access.Owner.Subject.ID); err != nil {
		h.writeTaskApplicationError(c, err, operation)
		return
	}
	writeExternalData(c, http.StatusOK, gin.H{"read": true})
}
