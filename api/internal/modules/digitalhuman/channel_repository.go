package digitalhuman

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	maxChannelDeliveryRunes = 20_000
	channelDeliveryIDName   = "digital-human-channel-delivery-v1"
)

type ChannelRoute struct {
	Installation *channelruntime.Installation
	Mapping      *channelruntime.ConversationMapping
	Binding      *channelruntime.Binding
}

type ChannelStore interface {
	CreateChannelInstallation(context.Context, *channelruntime.Installation) error
	ChannelInstallationByID(context.Context, uuid.UUID) (*channelruntime.Installation, error)
	ScopedChannelInstallationByID(context.Context, Scope, uuid.UUID) (*channelruntime.Installation, error)
	ListChannelInstallations(context.Context, Scope) ([]*ChannelInstallationView, error)
	UpdateChannelInstallation(context.Context, Scope, *channelruntime.Installation, int64) error
	DeleteChannelInstallation(context.Context, Scope, uuid.UUID, DeleteChannelInstallationRequest) error
	CreateChannelBinding(context.Context, *channelruntime.ConversationMapping, *channelruntime.Binding) error
	ListChannelBindings(context.Context, Scope, uuid.UUID) ([]*ChannelBindingView, error)
	ListDiscoveredChannelConversations(context.Context, Scope) ([]*ChannelConversationDiscovery, error)
	DeleteChannelBinding(context.Context, Scope, uuid.UUID, uuid.UUID) error
	ResolveChannelRoute(context.Context, uuid.UUID, string) (*ChannelRoute, error)
	EnsureChannelConversationID(context.Context, uuid.UUID, uuid.UUID, time.Time) (uuid.UUID, error)
	RecordInboundEvent(context.Context, *channelruntime.InboundEvent) (*channelruntime.InboundEvent, bool, error)
	MarkInboundEvent(context.Context, uuid.UUID, channelruntime.InboundEventStatus, string, *uuid.UUID, time.Time) error
	EnqueueRunChannelDelivery(context.Context, uuid.UUID, channelruntime.DeliveryKind, string, int64, string, time.Time) error
	RunChannelTransientTarget(context.Context, uuid.UUID) (*ChannelTransientTarget, error)
	ListInboundEvents(context.Context, Scope, uuid.UUID, ListQuery) ([]*channelruntime.InboundEvent, int64, error)
	ListChannelDeliveries(context.Context, Scope, uuid.UUID, ListQuery) ([]*channelruntime.Delivery, int64, error)
	ClaimDeliveryBatch(context.Context, int, time.Time) ([]*channelruntime.Delivery, error)
	SucceedDelivery(context.Context, uuid.UUID, *channelruntime.DeliveryReceipt, time.Time) error
	RetryDelivery(context.Context, uuid.UUID, time.Time, string, string) error
	FailDelivery(context.Context, uuid.UUID, channelruntime.DeliveryStatus, string, string, time.Time) error
	RequeueDelivery(context.Context, Scope, uuid.UUID, uuid.UUID, time.Time) error
	MarkStaleDeliveriesUnknown(context.Context, time.Time, time.Time) error
}

// ChannelTransientTarget contains only the routing identifiers required for a
// best-effort channel message. The message itself is never written to the
// durable channel outbox.
type ChannelTransientTarget struct {
	OrganizationID         uuid.UUID
	WorkspaceID            uuid.UUID
	ActorID                uuid.UUID
	InstallationID         uuid.UUID
	ConnectionID           uuid.UUID
	SubjectType            runtimeidentity.SubjectType
	SubjectID              uuid.UUID
	Provider               string
	ExternalConversationID string
	ExternalThreadID       string
}

type ChannelBindingView struct {
	Binding *channelruntime.Binding             `json:"binding"`
	Mapping *channelruntime.ConversationMapping `json:"mapping"`
}

func (r *GormRepository) CreateChannelInstallation(ctx context.Context, installation *channelruntime.Installation) error {
	if installation == nil || installation.ID == uuid.Nil || installation.OrganizationID == uuid.Nil || installation.WorkspaceID == uuid.Nil {
		return ErrInvalidRequest
	}
	if err := r.db.WithContext(ctx).Create(installation).Error; err != nil {
		if isUniqueViolation(err) {
			return ErrChannelConflict
		}
		return fmt.Errorf("create channel installation: %w", err)
	}
	return nil
}

func (r *GormRepository) ChannelInstallationByID(ctx context.Context, installationID uuid.UUID) (*channelruntime.Installation, error) {
	if installationID == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	var installation channelruntime.Installation
	err := r.db.WithContext(ctx).Where("id = ?", installationID).Take(&installation).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrChannelInstallationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get channel installation: %w", err)
	}
	return &installation, nil
}

func (r *GormRepository) ScopedChannelInstallationByID(ctx context.Context, scope Scope, installationID uuid.UUID) (*channelruntime.Installation, error) {
	if !validScopeBoundary(scope) || installationID == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	var installation channelruntime.Installation
	err := r.db.WithContext(ctx).Where(
		"id = ? AND organization_id = ? AND workspace_id = ?", installationID, scope.OrganizationID, scope.WorkspaceID,
	).Take(&installation).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrChannelInstallationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get scoped channel installation: %w", err)
	}
	return &installation, nil
}

func (r *GormRepository) ListChannelInstallations(ctx context.Context, scope Scope) ([]*ChannelInstallationView, error) {
	if !validScopeBoundary(scope) {
		return nil, ErrInvalidRequest
	}
	installations := make([]*channelruntime.Installation, 0)
	if err := r.db.WithContext(ctx).Where(
		"organization_id = ? AND workspace_id = ?", scope.OrganizationID, scope.WorkspaceID,
	).Order("created_at ASC, id ASC").Find(&installations).Error; err != nil {
		return nil, fmt.Errorf("list channel installations: %w", err)
	}

	type usageRow struct {
		InstallationID   uuid.UUID `gorm:"column:installation_id"`
		DigitalHumanID   uuid.UUID `gorm:"column:digital_human_id"`
		DigitalHumanName string    `gorm:"column:digital_human_name"`
		BindingCount     int64     `gorm:"column:binding_count"`
	}
	usageRows := make([]usageRow, 0)
	if err := r.db.WithContext(ctx).
		Table("channel_bindings AS bindings").
		Select("bindings.installation_id, bindings.subject_id AS digital_human_id, COALESCE(digital_humans.name, '') AS digital_human_name, COUNT(*) AS binding_count").
		Joins("LEFT JOIN digital_humans ON digital_humans.id = bindings.subject_id AND digital_humans.organization_id = bindings.organization_id AND digital_humans.workspace_id = bindings.workspace_id AND digital_humans.deleted_at IS NULL").
		Where("bindings.organization_id = ? AND bindings.workspace_id = ? AND bindings.subject_type = ? AND bindings.deleted_at IS NULL", scope.OrganizationID, scope.WorkspaceID, runtimeidentity.SubjectTypeDigitalHuman).
		Group("bindings.installation_id, bindings.subject_id, digital_humans.name").
		Order("digital_human_name ASC, digital_human_id ASC").
		Scan(&usageRows).Error; err != nil {
		return nil, fmt.Errorf("list channel installation usage: %w", err)
	}

	items := make([]*ChannelInstallationView, 0, len(installations))
	itemsByID := make(map[uuid.UUID]*ChannelInstallationView, len(installations))
	for _, installation := range installations {
		item := &ChannelInstallationView{
			Installation: installation,
			UsedBy:       make([]*ChannelInstallationUsage, 0),
		}
		items = append(items, item)
		itemsByID[installation.ID] = item
	}
	for _, row := range usageRows {
		item := itemsByID[row.InstallationID]
		if item == nil {
			continue
		}
		item.BindingCount += row.BindingCount
		item.UsedBy = append(item.UsedBy, &ChannelInstallationUsage{
			DigitalHumanID: row.DigitalHumanID, DigitalHumanName: row.DigitalHumanName, BindingCount: row.BindingCount,
		})
	}
	return items, nil
}

func (r *GormRepository) UpdateChannelInstallation(ctx context.Context, scope Scope, installation *channelruntime.Installation, expectedRevision int64) error {
	if !validScope(scope) || installation == nil || installation.ID == uuid.Nil || expectedRevision < 1 {
		return ErrInvalidRequest
	}
	result := r.db.WithContext(ctx).Model(&channelruntime.Installation{}).Where(
		"id = ? AND organization_id = ? AND workspace_id = ? AND revision = ? AND deleted_at IS NULL",
		installation.ID, scope.OrganizationID, scope.WorkspaceID, expectedRevision,
	).Updates(map[string]any{
		"name": installation.Name, "external_tenant_id": installation.ExternalTenantID,
		"external_bot_id": installation.ExternalBotID, "status": installation.Status,
		"updated_by": scope.ActorID, "revision": gorm.Expr("revision + 1"), "updated_at": installation.UpdatedAt,
	})
	if result.Error != nil {
		return fmt.Errorf("update channel installation: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrChannelConflict
	}
	installation.Revision = expectedRevision + 1
	return nil
}

func (r *GormRepository) DeleteChannelInstallation(ctx context.Context, scope Scope, installationID uuid.UUID, request DeleteChannelInstallationRequest) error {
	if !validScope(scope) || installationID == uuid.Nil {
		return ErrInvalidRequest
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var installation channelruntime.Installation
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(
			"id = ? AND organization_id = ? AND workspace_id = ?", installationID, scope.OrganizationID, scope.WorkspaceID,
		).Take(&installation).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrChannelInstallationNotFound
		}
		if err != nil {
			return fmt.Errorf("lock channel installation before delete: %w", err)
		}

		const bindingFilter = "organization_id = ? AND workspace_id = ? AND installation_id = ? AND deleted_at IS NULL"
		bindingArgs := []any{scope.OrganizationID, scope.WorkspaceID, installationID}
		var count int64
		if err := tx.Model(&channelruntime.Binding{}).Where(bindingFilter, bindingArgs...).Count(&count).Error; err != nil {
			return fmt.Errorf("count channel bindings before installation delete: %w", err)
		}
		if count > 0 && !request.RemoveBindings {
			return ErrChannelInstallationInUse
		}
		if count > 0 {
			if err := tx.Where(bindingFilter, bindingArgs...).Delete(&channelruntime.Binding{}).Error; err != nil {
				return fmt.Errorf("delete channel installation bindings: %w", err)
			}
		}
		if err := tx.Delete(&installation).Error; err != nil {
			return fmt.Errorf("delete channel installation: %w", err)
		}
		return nil
	})
}

func (r *GormRepository) CreateChannelBinding(ctx context.Context, mapping *channelruntime.ConversationMapping, binding *channelruntime.Binding) error {
	if mapping == nil || binding == nil || mapping.ID == uuid.Nil || binding.ID == uuid.Nil ||
		mapping.InstallationID == uuid.Nil || binding.InstallationID != mapping.InstallationID ||
		mapping.OrganizationID == uuid.Nil || mapping.WorkspaceID == uuid.Nil ||
		binding.OrganizationID != mapping.OrganizationID || binding.WorkspaceID != mapping.WorkspaceID {
		return ErrInvalidRequest
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var installation channelruntime.Installation
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(
			"id = ? AND organization_id = ? AND workspace_id = ? AND status = ?",
			mapping.InstallationID, mapping.OrganizationID, mapping.WorkspaceID, channelruntime.InstallationStatusActive,
		).Take(&installation).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrChannelNotReady
		}
		if err != nil {
			return fmt.Errorf("lock channel installation before binding: %w", err)
		}

		var existing channelruntime.ConversationMapping
		err = tx.Where(
			"installation_id = ? AND external_conversation_id = ?", mapping.InstallationID, mapping.ExternalConversationID,
		).Take(&existing).Error
		switch {
		case err == nil:
			if existing.OrganizationID != mapping.OrganizationID || existing.WorkspaceID != mapping.WorkspaceID || existing.ConversationType != mapping.ConversationType {
				return ErrChannelConflict
			}
			if existing.LocalConversationID == nil && mapping.LocalConversationID != nil {
				result := tx.Model(&channelruntime.ConversationMapping{}).Where(
					"id = ? AND local_conversation_id IS NULL", existing.ID,
				).Updates(map[string]any{
					"local_conversation_id": *mapping.LocalConversationID,
					"updated_at":            mapping.UpdatedAt,
				})
				if result.Error != nil {
					return fmt.Errorf("reserve rebound channel conversation: %w", result.Error)
				}
				if result.RowsAffected == 1 {
					existing.LocalConversationID = mapping.LocalConversationID
				}
			}
			*mapping = existing
			binding.ConversationMappingID = existing.ID
		case errors.Is(err, gorm.ErrRecordNotFound):
			if err := tx.Create(mapping).Error; err != nil {
				if isUniqueViolation(err) {
					return ErrChannelConflict
				}
				return fmt.Errorf("create external conversation mapping: %w", err)
			}
			binding.ConversationMappingID = mapping.ID
		default:
			return fmt.Errorf("load external conversation mapping: %w", err)
		}
		if err := tx.Create(binding).Error; err != nil {
			if isUniqueViolation(err) {
				return ErrChannelConflict
			}
			return fmt.Errorf("create channel binding: %w", err)
		}
		return nil
	})
}

func (r *GormRepository) ListChannelBindings(ctx context.Context, scope Scope, subjectID uuid.UUID) ([]*ChannelBindingView, error) {
	if !validScopeBoundary(scope) || subjectID == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	var bindings []*channelruntime.Binding
	if err := r.db.WithContext(ctx).Where(
		"organization_id = ? AND workspace_id = ? AND subject_type = ? AND subject_id = ?",
		scope.OrganizationID, scope.WorkspaceID, runtimeidentity.SubjectTypeDigitalHuman, subjectID,
	).Order("created_at ASC, id ASC").Find(&bindings).Error; err != nil {
		return nil, fmt.Errorf("list channel bindings: %w", err)
	}
	items := make([]*ChannelBindingView, 0, len(bindings))
	for _, binding := range bindings {
		var mapping channelruntime.ConversationMapping
		if err := r.db.WithContext(ctx).Where("id = ?", binding.ConversationMappingID).Take(&mapping).Error; err != nil {
			return nil, fmt.Errorf("load channel binding conversation: %w", err)
		}
		items = append(items, &ChannelBindingView{Binding: binding, Mapping: &mapping})
	}
	return items, nil
}

func (r *GormRepository) ListDiscoveredChannelConversations(ctx context.Context, scope Scope) ([]*ChannelConversationDiscovery, error) {
	if !validScopeBoundary(scope) {
		return nil, ErrInvalidRequest
	}
	items := make([]*ChannelConversationDiscovery, 0)
	err := r.db.WithContext(ctx).
		Table("channel_inbound_events AS events").
		Select(`events.installation_id,
			events.external_conversation_id,
			CASE WHEN events.event_type = 'channel.mention' THEN 'group' ELSE 'direct' END AS conversation_type,
			MAX(events.sender_external_id) AS sender_external_id,
			COUNT(*) AS event_count,
			MAX(events.received_at) AS last_event_at`).
		Joins("JOIN channel_installations AS installations ON installations.id = events.installation_id AND installations.deleted_at IS NULL").
		Where(`events.organization_id = ? AND events.workspace_id = ?
			AND installations.status = ?
			AND events.external_conversation_id <> ''
			AND NOT EXISTS (
				SELECT 1
				FROM external_conversation_mappings AS mappings
				JOIN channel_bindings AS bindings
				  ON bindings.conversation_mapping_id = mappings.id
				 AND bindings.deleted_at IS NULL
				WHERE mappings.installation_id = events.installation_id
				  AND mappings.external_conversation_id = events.external_conversation_id
			)`, scope.OrganizationID, scope.WorkspaceID, channelruntime.InstallationStatusActive).
		Group("events.installation_id, events.external_conversation_id, events.event_type").
		Order("last_event_at DESC, events.external_conversation_id ASC").
		Limit(100).
		Scan(&items).Error
	if err != nil {
		return nil, fmt.Errorf("list discovered channel conversations: %w", err)
	}
	return items, nil
}

func (r *GormRepository) DeleteChannelBinding(ctx context.Context, scope Scope, subjectID, bindingID uuid.UUID) error {
	if !validScope(scope) || subjectID == uuid.Nil || bindingID == uuid.Nil {
		return ErrInvalidRequest
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var binding channelruntime.Binding
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(
			"id = ? AND organization_id = ? AND workspace_id = ? AND subject_type = ? AND subject_id = ?",
			bindingID, scope.OrganizationID, scope.WorkspaceID, runtimeidentity.SubjectTypeDigitalHuman, subjectID,
		).Take(&binding).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrChannelBindingNotFound
		}
		if err != nil {
			return fmt.Errorf("lock channel binding before delete: %w", err)
		}
		if err := tx.Delete(&binding).Error; err != nil {
			return fmt.Errorf("delete channel binding: %w", err)
		}
		result := tx.Model(&channelruntime.ConversationMapping{}).Where(
			"id = ? AND installation_id = ? AND organization_id = ? AND workspace_id = ?",
			binding.ConversationMappingID, binding.InstallationID, scope.OrganizationID, scope.WorkspaceID,
		).Updates(map[string]any{
			"local_conversation_id": nil, "last_event_at": nil, "updated_at": time.Now().UTC(),
		})
		if result.Error != nil {
			return fmt.Errorf("reset external conversation mapping: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrChannelConflict
		}
		return nil
	})
}

func (r *GormRepository) ResolveChannelRoute(ctx context.Context, installationID uuid.UUID, externalConversationID string) (*ChannelRoute, error) {
	if installationID == uuid.Nil || strings.TrimSpace(externalConversationID) == "" {
		return nil, ErrInvalidRequest
	}
	installation, err := r.ChannelInstallationByID(ctx, installationID)
	if err != nil {
		return nil, err
	}
	if installation.Status != channelruntime.InstallationStatusActive {
		return nil, ErrChannelNotReady
	}
	var mapping channelruntime.ConversationMapping
	err = r.db.WithContext(ctx).Where(
		"installation_id = ? AND external_conversation_id = ?", installationID, strings.TrimSpace(externalConversationID),
	).Take(&mapping).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &ChannelRoute{Installation: installation}, ErrChannelBindingNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("resolve external conversation mapping: %w", err)
	}
	var binding channelruntime.Binding
	err = r.db.WithContext(ctx).Where(
		"conversation_mapping_id = ? AND installation_id = ? AND enabled = ?", mapping.ID, installationID, true,
	).Take(&binding).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &ChannelRoute{Installation: installation, Mapping: &mapping}, ErrChannelBindingNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("resolve channel binding: %w", err)
	}
	return &ChannelRoute{Installation: installation, Mapping: &mapping, Binding: &binding}, nil
}

func (r *GormRepository) EnsureChannelConversationID(ctx context.Context, mappingID, suggestedID uuid.UUID, eventAt time.Time) (uuid.UUID, error) {
	if mappingID == uuid.Nil || suggestedID == uuid.Nil {
		return uuid.Nil, ErrInvalidRequest
	}
	if eventAt.IsZero() {
		eventAt = time.Now().UTC()
	}
	result := r.db.WithContext(ctx).Model(&channelruntime.ConversationMapping{}).Where("id = ?", mappingID).Updates(map[string]any{
		"local_conversation_id": gorm.Expr("COALESCE(local_conversation_id, ?)", suggestedID),
		"last_event_at": gorm.Expr(
			"CASE WHEN last_event_at IS NULL OR last_event_at < ? THEN ? ELSE last_event_at END",
			eventAt, eventAt,
		),
		"updated_at": time.Now().UTC(),
	})
	if result.Error != nil {
		return uuid.Nil, fmt.Errorf("reserve channel conversation: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return uuid.Nil, ErrChannelBindingNotFound
	}
	var mapping channelruntime.ConversationMapping
	if err := r.db.WithContext(ctx).Select("local_conversation_id").Where("id = ?", mappingID).Take(&mapping).Error; err != nil {
		return uuid.Nil, fmt.Errorf("load reserved channel conversation: %w", err)
	}
	if mapping.LocalConversationID == nil || *mapping.LocalConversationID == uuid.Nil {
		return uuid.Nil, ErrChannelConflict
	}
	return *mapping.LocalConversationID, nil
}

func (r *GormRepository) RecordInboundEvent(ctx context.Context, event *channelruntime.InboundEvent) (*channelruntime.InboundEvent, bool, error) {
	if event == nil || event.ID == uuid.Nil || event.InstallationID == uuid.Nil || strings.TrimSpace(event.ProviderEventID) == "" {
		return nil, false, ErrInvalidRequest
	}
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(event)
	if result.Error != nil {
		return nil, false, fmt.Errorf("record channel inbound event: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return event, true, nil
	}
	var existing channelruntime.InboundEvent
	err := r.db.WithContext(ctx).Where(
		"installation_id = ? AND provider_event_id = ?", event.InstallationID, event.ProviderEventID,
	).Take(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) && strings.TrimSpace(event.ProviderMessageID) != "" {
		err = r.db.WithContext(ctx).Where(
			"installation_id = ? AND provider_message_id = ?", event.InstallationID, event.ProviderMessageID,
		).Take(&existing).Error
	}
	if err != nil {
		return nil, false, fmt.Errorf("load duplicate channel inbound event: %w", err)
	}
	return &existing, false, nil
}

func (r *GormRepository) MarkInboundEvent(ctx context.Context, eventID uuid.UUID, status channelruntime.InboundEventStatus, reason string, runID *uuid.UUID, now time.Time) error {
	updates := map[string]any{
		"status": status, "reason_code": strings.TrimSpace(reason), "processed_at": now, "updated_at": now,
	}
	if runID != nil && *runID != uuid.Nil {
		updates["invocation_run_id"] = *runID
	}
	allowedStatuses := []channelruntime.InboundEventStatus{channelruntime.InboundEventReceived}
	if status == channelruntime.InboundEventAccepted {
		allowedStatuses = append(allowedStatuses, channelruntime.InboundEventFailed)
	}
	result := r.db.WithContext(ctx).Model(&channelruntime.InboundEvent{}).Where(
		"id = ? AND status IN ?", eventID, allowedStatuses,
	).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("mark channel inbound event: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		var current channelruntime.InboundEvent
		if err := r.db.WithContext(ctx).Select("id", "status", "invocation_run_id").Where("id = ?", eventID).Take(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrChannelBindingNotFound
			}
			return err
		}
		if current.Status == status && (runID == nil || current.InvocationRunID != nil && *current.InvocationRunID == *runID) {
			return nil
		}
		return ErrChannelConflict
	}
	return nil
}

func (r *GormRepository) ListInboundEvents(ctx context.Context, scope Scope, subjectID uuid.UUID, query ListQuery) ([]*channelruntime.InboundEvent, int64, error) {
	if !validScopeBoundary(scope) || subjectID == uuid.Nil {
		return nil, 0, ErrInvalidRequest
	}
	db := r.db.WithContext(ctx).Model(&channelruntime.InboundEvent{}).
		Joins("JOIN channel_bindings ON channel_bindings.id = channel_inbound_events.binding_id").
		Where(
			"channel_inbound_events.organization_id = ? AND channel_inbound_events.workspace_id = ? AND channel_bindings.subject_type = ? AND channel_bindings.subject_id = ?",
			scope.OrganizationID, scope.WorkspaceID, runtimeidentity.SubjectTypeDigitalHuman, subjectID,
		)
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count channel inbound events: %w", err)
	}
	items := make([]*channelruntime.InboundEvent, 0, query.PageSize)
	if err := db.Order("channel_inbound_events.created_at DESC, channel_inbound_events.id DESC").
		Offset((query.Page - 1) * query.PageSize).Limit(query.PageSize).Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("list channel inbound events: %w", err)
	}
	return items, total, nil
}

func (r *GormRepository) ListChannelDeliveries(ctx context.Context, scope Scope, subjectID uuid.UUID, query ListQuery) ([]*channelruntime.Delivery, int64, error) {
	if !validScopeBoundary(scope) || subjectID == uuid.Nil {
		return nil, 0, ErrInvalidRequest
	}
	db := r.db.WithContext(ctx).Model(&channelruntime.Delivery{}).Where(
		"organization_id = ? AND workspace_id = ? AND subject_type = ? AND subject_id = ?",
		scope.OrganizationID, scope.WorkspaceID, runtimeidentity.SubjectTypeDigitalHuman, subjectID,
	)
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count channel deliveries: %w", err)
	}
	items := make([]*channelruntime.Delivery, 0, query.PageSize)
	if err := db.Order("created_at DESC, id DESC").Offset((query.Page - 1) * query.PageSize).Limit(query.PageSize).Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("list channel deliveries: %w", err)
	}
	return items, total, nil
}

func (r *GormRepository) ClaimDeliveryBatch(ctx context.Context, limit int, now time.Time) ([]*channelruntime.Delivery, error) {
	items := make([]*channelruntime.Delivery, 0, limit)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where(
			"status = ? AND available_at <= ?", channelruntime.DeliveryPending, now,
		).Order("created_at ASC").Limit(limit).Find(&items).Error; err != nil {
			return err
		}
		for _, item := range items {
			result := tx.Model(&channelruntime.Delivery{}).Where("id = ? AND status = ?", item.ID, channelruntime.DeliveryPending).
				Updates(map[string]any{
					"status": channelruntime.DeliveryProcessing, "claimed_at": now,
					"attempt_count": gorm.Expr("attempt_count + 1"), "updated_at": now,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrChannelDeliveryConflict
			}
			item.Status, item.ClaimedAt, item.AttemptCount = channelruntime.DeliveryProcessing, &now, item.AttemptCount+1
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("claim channel delivery batch: %w", err)
	}
	return items, nil
}

func (r *GormRepository) SucceedDelivery(ctx context.Context, deliveryID uuid.UUID, receipt *channelruntime.DeliveryReceipt, now time.Time) error {
	if deliveryID == uuid.Nil || receipt == nil || strings.TrimSpace(receipt.ProviderMessageID) == "" {
		return ErrInvalidRequest
	}
	result := r.db.WithContext(ctx).Model(&channelruntime.Delivery{}).Where(
		"id = ? AND status = ?", deliveryID, channelruntime.DeliveryProcessing,
	).Updates(map[string]any{
		"status": channelruntime.DeliverySucceeded, "delivered_at": now, "claimed_at": nil,
		"provider_message_id": strings.TrimSpace(receipt.ProviderMessageID),
		"provider_request_id": strings.TrimSpace(receipt.ProviderRequestID),
		"error_code":          "", "error_message": "", "updated_at": now,
	})
	if result.Error != nil {
		return fmt.Errorf("complete channel delivery: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrChannelDeliveryConflict
	}
	return nil
}

func (r *GormRepository) RetryDelivery(ctx context.Context, deliveryID uuid.UUID, availableAt time.Time, code, message string) error {
	result := r.db.WithContext(ctx).Model(&channelruntime.Delivery{}).Where(
		"id = ? AND status = ?", deliveryID, channelruntime.DeliveryProcessing,
	).Updates(map[string]any{
		"status": channelruntime.DeliveryPending, "available_at": availableAt, "claimed_at": nil,
		"error_code": strings.TrimSpace(code), "error_message": strings.TrimSpace(message), "updated_at": time.Now().UTC(),
	})
	if result.Error != nil {
		return fmt.Errorf("retry channel delivery: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrChannelDeliveryConflict
	}
	return nil
}

func (r *GormRepository) FailDelivery(ctx context.Context, deliveryID uuid.UUID, status channelruntime.DeliveryStatus, code, message string, now time.Time) error {
	if status != channelruntime.DeliveryFailed && status != channelruntime.DeliveryUnknown {
		return ErrInvalidRequest
	}
	result := r.db.WithContext(ctx).Model(&channelruntime.Delivery{}).Where(
		"id = ? AND status = ?", deliveryID, channelruntime.DeliveryProcessing,
	).Updates(map[string]any{
		"status": status, "claimed_at": nil, "error_code": strings.TrimSpace(code),
		"error_message": strings.TrimSpace(message), "updated_at": now,
	})
	if result.Error != nil {
		return fmt.Errorf("fail channel delivery: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrChannelDeliveryConflict
	}
	return nil
}

func (r *GormRepository) RequeueDelivery(ctx context.Context, scope Scope, subjectID, deliveryID uuid.UUID, now time.Time) error {
	if !validScope(scope) || subjectID == uuid.Nil || deliveryID == uuid.Nil {
		return ErrInvalidRequest
	}
	result := r.db.WithContext(ctx).Model(&channelruntime.Delivery{}).Where(
		"id = ? AND organization_id = ? AND workspace_id = ? AND subject_type = ? AND subject_id = ? AND status = ?",
		deliveryID, scope.OrganizationID, scope.WorkspaceID, runtimeidentity.SubjectTypeDigitalHuman, subjectID, channelruntime.DeliveryFailed,
	).Updates(map[string]any{
		"status": channelruntime.DeliveryPending, "available_at": now, "claimed_at": nil,
		"error_code": "", "error_message": "", "updated_at": now,
	})
	if result.Error != nil {
		return fmt.Errorf("requeue channel delivery: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrChannelDeliveryConflict
	}
	return nil
}

func (r *GormRepository) MarkStaleDeliveriesUnknown(ctx context.Context, claimedBefore, now time.Time) error {
	return r.db.WithContext(ctx).Model(&channelruntime.Delivery{}).Where(
		"status = ? AND claimed_at < ?", channelruntime.DeliveryProcessing, claimedBefore,
	).Updates(map[string]any{
		"status": channelruntime.DeliveryUnknown, "claimed_at": nil,
		"error_code":    "channel.delivery.interrupted",
		"error_message": "Delivery was interrupted after provider execution may have started.", "updated_at": now,
	}).Error
}

func (r *GormRepository) CompleteRunWithDelivery(ctx context.Context, runID uuid.UUID, summary string, trace datatypes.JSON, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run Run
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", runID).Take(&run).Error; err != nil {
			return err
		}
		if run.Status == RunStatusCanceling || run.CancelRequestedAt != nil {
			return tx.Model(&Run{}).Where("id = ?", runID).Updates(map[string]any{
				"status": RunStatusCanceled, "result_summary": summary, "execution_trace": trace,
				"completed_at": now, "updated_at": now,
			}).Error
		}
		if run.Status != RunStatusRunning {
			return ErrRunConflict
		}
		if err := tx.Model(&Run{}).Where("id = ?", runID).Updates(map[string]any{
			"status": RunStatusSucceeded, "result_summary": summary, "execution_trace": trace,
			"completed_at": now, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		return createRunDelivery(ctx, tx, &run, summary, trace, now)
	})
}

func createRunDelivery(ctx context.Context, tx *gorm.DB, run *Run, summary string, trace datatypes.JSON, now time.Time) error {
	if run == nil {
		return nil
	}
	if err := tx.WithContext(ctx).Where(
		"run_id = ? AND delivery_kind IN ? AND status = ?",
		run.ID,
		[]channelruntime.DeliveryKind{channelruntime.DeliveryKindAcknowledgement, channelruntime.DeliveryKindProgress},
		channelruntime.DeliveryPending,
	).Delete(&channelruntime.Delivery{}).Error; err != nil {
		return fmt.Errorf("discard superseded channel progress deliveries: %w", err)
	}
	return createRunChannelDelivery(
		ctx, tx, run, channelruntime.DeliveryKindFinal, "final", math.MaxInt64,
		summary, channelDeliveryArtifacts(trace), now,
	)
}

func (r *GormRepository) EnqueueRunChannelDelivery(
	ctx context.Context,
	runID uuid.UUID,
	kind channelruntime.DeliveryKind,
	deliveryKey string,
	sequence int64,
	text string,
	now time.Time,
) error {
	deliveryKey = strings.TrimSpace(deliveryKey)
	text = boundedRunes(strings.TrimSpace(text), maxChannelDeliveryRunes)
	if runID == uuid.Nil || (kind != channelruntime.DeliveryKindAcknowledgement && kind != channelruntime.DeliveryKindProgress) ||
		deliveryKey == "" || utf8.RuneCountInString(deliveryKey) > 128 || sequence < 0 || sequence == math.MaxInt64 || text == "" {
		return ErrInvalidRequest
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run Run
		if err := tx.Where("id = ?", runID).Take(&run).Error; err != nil {
			return err
		}
		switch run.Status {
		case RunStatusQueued, RunStatusRunning, RunStatusCanceling:
		default:
			return nil
		}
		return createRunChannelDelivery(ctx, tx, &run, kind, deliveryKey, sequence, text, nil, now)
	})
}

func (r *GormRepository) RunChannelTransientTarget(ctx context.Context, runID uuid.UUID) (*ChannelTransientTarget, error) {
	if runID == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	var run Run
	if err := r.db.WithContext(ctx).Where("id = ?", runID).Take(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRunNotFound
		}
		return nil, fmt.Errorf("load DigitalHuman run for transient channel delivery: %w", err)
	}
	if run.Status != RunStatusQueued && run.Status != RunStatusRunning {
		return nil, nil
	}
	var event invocation.Event
	if json.Unmarshal(run.InvocationEvent, &event) != nil || event.ReplyTo == nil || event.Origin.Kind != invocation.OriginChannel {
		return nil, nil
	}
	inboundEventID, ok := contextUUID(event.Context, "channel_inbound_event_id")
	if !ok {
		return nil, fmt.Errorf("channel invocation is missing inbound event identity")
	}
	installationID, err := uuid.Parse(strings.TrimSpace(event.Origin.InstallationID))
	if err != nil || installationID == uuid.Nil {
		return nil, fmt.Errorf("channel invocation installation identity is invalid")
	}
	var inbound channelruntime.InboundEvent
	if err := r.db.WithContext(ctx).Where(
		"id = ? AND installation_id = ? AND organization_id = ? AND workspace_id = ?",
		inboundEventID, installationID, run.OrganizationID, run.WorkspaceID,
	).Take(&inbound).Error; err != nil {
		return nil, fmt.Errorf("load channel inbound event for transient delivery: %w", err)
	}
	if inbound.InvocationRunID != nil && *inbound.InvocationRunID != run.ID {
		return nil, fmt.Errorf("channel inbound event is linked to another run")
	}
	if inbound.BindingID == nil || *inbound.BindingID == uuid.Nil {
		return nil, fmt.Errorf("channel inbound event is missing binding identity")
	}
	var binding channelruntime.Binding
	if err := r.db.WithContext(ctx).Where(
		"id = ? AND installation_id = ? AND organization_id = ? AND workspace_id = ? AND subject_type = ? AND subject_id = ? AND enabled = ?",
		*inbound.BindingID, installationID, run.OrganizationID, run.WorkspaceID,
		runtimeidentity.SubjectTypeDigitalHuman, run.DigitalHumanID, true,
	).Take(&binding).Error; err != nil {
		return nil, fmt.Errorf("load channel binding for transient delivery: %w", err)
	}
	var installation channelruntime.Installation
	if err := r.db.WithContext(ctx).Where(
		"id = ? AND organization_id = ? AND workspace_id = ? AND status = ?",
		installationID, run.OrganizationID, run.WorkspaceID, channelruntime.InstallationStatusActive,
	).Take(&installation).Error; err != nil {
		return nil, fmt.Errorf("load channel installation for transient delivery: %w", err)
	}
	return &ChannelTransientTarget{
		OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID,
		ActorID: installation.CreatedBy, InstallationID: installation.ID, ConnectionID: installation.ConnectionID,
		SubjectType: binding.SubjectType, SubjectID: binding.SubjectID,
		Provider:               strings.ToLower(strings.TrimSpace(installation.Provider)),
		ExternalConversationID: strings.TrimSpace(event.ReplyTo.ConversationID),
		ExternalThreadID:       strings.TrimSpace(event.ReplyTo.ThreadID),
	}, nil
}

func createRunChannelDelivery(
	ctx context.Context,
	tx *gorm.DB,
	run *Run,
	kind channelruntime.DeliveryKind,
	deliveryKey string,
	sequence int64,
	text string,
	artifacts []channelDeliveryArtifact,
	now time.Time,
) error {
	var event invocation.Event
	if run == nil || json.Unmarshal(run.InvocationEvent, &event) != nil || event.ReplyTo == nil || event.Origin.Kind != invocation.OriginChannel {
		return nil
	}
	inboundEventID, ok := contextUUID(event.Context, "channel_inbound_event_id")
	if !ok {
		return fmt.Errorf("channel invocation is missing inbound event identity")
	}
	installationID, err := uuid.Parse(strings.TrimSpace(event.Origin.InstallationID))
	if err != nil || installationID == uuid.Nil {
		return fmt.Errorf("channel invocation installation identity is invalid")
	}
	var inbound channelruntime.InboundEvent
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where(
		"id = ? AND installation_id = ? AND organization_id = ? AND workspace_id = ?",
		inboundEventID, installationID, run.OrganizationID, run.WorkspaceID,
	).Take(&inbound).Error; err != nil {
		return fmt.Errorf("load channel inbound event for delivery: %w", err)
	}
	if inbound.InvocationRunID != nil && *inbound.InvocationRunID != run.ID {
		return fmt.Errorf("channel inbound event is linked to another run")
	}
	if inbound.InvocationRunID == nil || inbound.Status != channelruntime.InboundEventAccepted {
		if err := tx.WithContext(ctx).Model(&channelruntime.InboundEvent{}).Where("id = ?", inbound.ID).Updates(map[string]any{
			"status": channelruntime.InboundEventAccepted, "reason_code": "", "invocation_run_id": run.ID,
			"processed_at": now, "updated_at": now,
		}).Error; err != nil {
			return fmt.Errorf("repair channel inbound run linkage: %w", err)
		}
		inbound.InvocationRunID = &run.ID
	}
	if inbound.BindingID == nil || *inbound.BindingID == uuid.Nil {
		return fmt.Errorf("channel inbound event is missing binding identity")
	}
	var binding channelruntime.Binding
	if err := tx.WithContext(ctx).Where(
		"id = ? AND installation_id = ? AND organization_id = ? AND workspace_id = ? AND subject_type = ? AND subject_id = ?",
		*inbound.BindingID, installationID, run.OrganizationID, run.WorkspaceID, runtimeidentity.SubjectTypeDigitalHuman, run.DigitalHumanID,
	).Take(&binding).Error; err != nil {
		return fmt.Errorf("load channel binding for delivery: %w", err)
	}
	var installation channelruntime.Installation
	if err := tx.WithContext(ctx).Where(
		"id = ? AND organization_id = ? AND workspace_id = ? AND status = ?",
		installationID, run.OrganizationID, run.WorkspaceID, channelruntime.InstallationStatusActive,
	).Take(&installation).Error; err != nil {
		return fmt.Errorf("load channel installation for delivery: %w", err)
	}
	text = boundedRunes(strings.TrimSpace(text), maxChannelDeliveryRunes)
	if text == "" && len(artifacts) == 0 {
		return nil
	}
	payload, digest, err := channelDeliveryPayloadWithKind(kind, text, artifacts...)
	if err != nil {
		return err
	}
	delivery := &channelruntime.Delivery{
		ID: channelDeliveryStageID(run.ID, kind, deliveryKey), OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID,
		InstallationID: installation.ID, BindingID: binding.ID, InboundEventID: inbound.ID, RunID: run.ID,
		Kind: kind, DeliveryKey: deliveryKey, Sequence: sequence,
		Provider: strings.ToLower(strings.TrimSpace(installation.Provider)), ConnectionID: installation.ConnectionID,
		ActorID: installation.CreatedBy, SubjectType: binding.SubjectType, SubjectID: binding.SubjectID,
		ExternalConversationID: strings.TrimSpace(event.ReplyTo.ConversationID), ExternalThreadID: strings.TrimSpace(event.ReplyTo.ThreadID),
		Payload: payload, PayloadDigest: digest, Status: channelruntime.DeliveryPending, AvailableAt: now,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}}, DoNothing: true,
	}).Create(delivery).Error; err != nil {
		return fmt.Errorf("create channel delivery intent: %w", err)
	}
	return nil
}

func channelDeliveryID(runID uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(runID, []byte(channelDeliveryIDName))
}

func channelDeliveryStageID(runID uuid.UUID, kind channelruntime.DeliveryKind, deliveryKey string) uuid.UUID {
	if kind == channelruntime.DeliveryKindFinal && deliveryKey == "final" {
		return channelDeliveryID(runID)
	}
	return uuid.NewSHA1(runID, []byte(channelDeliveryIDName+":"+string(kind)+":"+deliveryKey))
}

func (r *GormRepository) LinkChannelRunConversation(ctx context.Context, runID, conversationID uuid.UUID, now time.Time) error {
	var run Run
	if err := r.db.WithContext(ctx).Select("invocation_event", "organization_id", "workspace_id", "digital_human_id").Where("id = ?", runID).Take(&run).Error; err != nil {
		return fmt.Errorf("load channel run conversation identity: %w", err)
	}
	var invocationEvent invocation.Event
	if json.Unmarshal(run.InvocationEvent, &invocationEvent) != nil || invocationEvent.Origin.Kind != invocation.OriginChannel {
		return nil
	}
	inboundEventID, ok := contextUUID(invocationEvent.Context, "channel_inbound_event_id")
	if !ok {
		return nil
	}
	var inbound channelruntime.InboundEvent
	err := r.db.WithContext(ctx).Where(
		"id = ? AND organization_id = ? AND workspace_id = ?",
		inboundEventID, run.OrganizationID, run.WorkspaceID,
	).Take(&inbound).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || inbound.BindingID == nil {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load channel event conversation mapping: %w", err)
	}
	if inbound.InvocationRunID != nil && *inbound.InvocationRunID != runID {
		return fmt.Errorf("channel inbound event is linked to another run")
	}
	if inbound.InvocationRunID == nil {
		result := r.db.WithContext(ctx).Model(&channelruntime.InboundEvent{}).Where(
			"id = ? AND invocation_run_id IS NULL", inbound.ID,
		).Updates(map[string]any{
			"status": channelruntime.InboundEventAccepted, "reason_code": "", "invocation_run_id": runID,
			"processed_at": now, "updated_at": now,
		})
		if result.Error != nil {
			return fmt.Errorf("link channel inbound event to run: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			var linked channelruntime.InboundEvent
			if err := r.db.WithContext(ctx).Select("invocation_run_id").Where("id = ?", inbound.ID).Take(&linked).Error; err != nil {
				return fmt.Errorf("verify channel inbound run linkage: %w", err)
			}
			if linked.InvocationRunID == nil || *linked.InvocationRunID != runID {
				return ErrChannelConflict
			}
		}
	}
	var binding channelruntime.Binding
	if err := r.db.WithContext(ctx).Where(
		"id = ? AND organization_id = ? AND workspace_id = ? AND subject_type = ? AND subject_id = ?",
		*inbound.BindingID, run.OrganizationID, run.WorkspaceID, runtimeidentity.SubjectTypeDigitalHuman, run.DigitalHumanID,
	).Take(&binding).Error; err != nil {
		return fmt.Errorf("load channel binding conversation mapping: %w", err)
	}
	return r.db.WithContext(ctx).Model(&channelruntime.ConversationMapping{}).Where(
		"id = ? AND organization_id = ? AND workspace_id = ? AND installation_id = ? AND (local_conversation_id IS NULL OR local_conversation_id = ?)",
		binding.ConversationMappingID, run.OrganizationID, run.WorkspaceID, binding.InstallationID, conversationID,
	).Updates(map[string]any{"local_conversation_id": conversationID, "last_event_at": now, "updated_at": now}).Error
}

func contextUUID(context map[string]any, key string) (uuid.UUID, bool) {
	if context == nil {
		return uuid.Nil, false
	}
	value, ok := context[key].(string)
	if !ok {
		return uuid.Nil, false
	}
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	return parsed, err == nil && parsed != uuid.Nil
}

func channelDeliveryPayload(text string, artifacts ...channelDeliveryArtifact) (datatypes.JSON, string, error) {
	return channelDeliveryPayloadWithKind(channelruntime.DeliveryKindFinal, text, artifacts...)
}

func channelDeliveryPayloadWithKind(kind channelruntime.DeliveryKind, text string, artifacts ...channelDeliveryArtifact) (datatypes.JSON, string, error) {
	if len(artifacts) > maxChannelDeliveryArtifacts {
		artifacts = artifacts[:maxChannelDeliveryArtifacts]
	}
	payload, err := json.Marshal(channelDeliveryPayloadData{Kind: kind, Text: text, Artifacts: artifacts})
	if err != nil {
		return nil, "", fmt.Errorf("encode channel delivery payload: %w", err)
	}
	digest := sha256.Sum256(payload)
	return datatypes.JSON(payload), "sha256:" + hex.EncodeToString(digest[:]), nil
}

func boundedRunes(value string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:limit]))
}
