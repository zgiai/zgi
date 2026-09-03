package digitalhuman

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
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
	ScopedChannelInstallationByID(context.Context, Scope, uuid.UUID, uuid.UUID) (*channelruntime.Installation, error)
	ListChannelInstallations(context.Context, Scope, uuid.UUID) ([]*channelruntime.Installation, error)
	UpdateChannelInstallation(context.Context, Scope, *channelruntime.Installation, int64) error
	DeleteChannelInstallation(context.Context, Scope, uuid.UUID, uuid.UUID) error
	CreateChannelBinding(context.Context, *channelruntime.ConversationMapping, *channelruntime.Binding) error
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
	DismissDelivery(context.Context, Scope, uuid.UUID, uuid.UUID, time.Time) error
	MarkStaleDeliveriesUnknown(context.Context, time.Time, time.Time) error
}

// ChannelConnectionTestStore projects only the evidence needed to prove one
// real message round trip. It does not expose arbitrary message search through
// the authenticated connection-test API.
type ChannelConnectionTestStore interface {
	ChannelConnectionTestEvidence(context.Context, uuid.UUID, channelruntime.ConversationType, string, time.Time) (*channelruntime.InboundEvent, *channelruntime.Delivery, error)
}

// ChannelReceiverStore is the process-lifecycle persistence surface used by
// long-lived channel transports. The lease prevents two API replicas from
// opening competing receivers for the same installation.
type ChannelReceiverStore interface {
	ListActiveChannelInstallations(context.Context) ([]*channelruntime.Installation, error)
	TryAcquireChannelReceiverLease(context.Context, uuid.UUID) (*ChannelReceiverLease, bool, error)
}

// ChannelReceiverLease owns one database session lock. Check detects a lost
// session so the transport stops instead of continuing without ownership.
type ChannelReceiverLease struct {
	Release func()
	Check   func(context.Context) error
}

// ChannelCredentialStore is kept separate from ChannelStore so existing
// provider-reference installations and test stores do not need to implement
// credential persistence.
type ChannelCredentialStore interface {
	CreateChannelInstallationWithCredential(context.Context, *channelruntime.Installation, *ChannelInstallationCredential) error
	ChannelInstallationCredentialByID(context.Context, uuid.UUID, uuid.UUID) (*ChannelInstallationCredential, error)
	ReplaceChannelInstallationCredential(context.Context, Scope, *channelruntime.Installation, *ChannelInstallationCredential, int64) error
}

// ChannelContextStore is the narrow persistence surface consumed while
// preparing provider context for a channel invocation.
type ChannelContextStore interface {
	ChannelInstallationByID(context.Context, uuid.UUID) (*channelruntime.Installation, error)
	LastSuccessfulChannelDeliveryAt(context.Context, uuid.UUID, string, time.Time) (*time.Time, error)
	SuccessfulChannelDeliveryKinds(
		context.Context,
		uuid.UUID,
		string,
		uuid.UUID,
		[]string,
	) (map[string]channelruntime.DeliveryKind, error)
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

func (r *GormRepository) CreateChannelInstallationWithCredential(
	ctx context.Context,
	installation *channelruntime.Installation,
	credential *ChannelInstallationCredential,
) error {
	if installation == nil || credential == nil || installation.ID == uuid.Nil ||
		credential.InstallationID != installation.ID || installation.ConnectionID != installation.ID ||
		credential.OrganizationID != installation.OrganizationID || credential.WorkspaceID != installation.WorkspaceID ||
		!strings.EqualFold(credential.Provider, installation.Provider) {
		return ErrInvalidRequest
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(installation).Error; err != nil {
			if isUniqueViolation(err) {
				return ErrChannelConflict
			}
			return fmt.Errorf("create channel installation: %w", err)
		}
		if err := tx.Create(credential).Error; err != nil {
			return fmt.Errorf("create channel installation credential: %w", err)
		}
		return nil
	})
}

func (r *GormRepository) ChannelInstallationCredentialByID(
	ctx context.Context,
	organizationID uuid.UUID,
	installationID uuid.UUID,
) (*ChannelInstallationCredential, error) {
	if organizationID == uuid.Nil || installationID == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	var credential ChannelInstallationCredential
	err := r.db.WithContext(ctx).Where(
		"installation_id = ? AND organization_id = ?", installationID, organizationID,
	).Take(&credential).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrChannelCredentialNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get channel installation credential: %w", err)
	}
	return &credential, nil
}

func (r *GormRepository) ReplaceChannelInstallationCredential(
	ctx context.Context,
	scope Scope,
	installation *channelruntime.Installation,
	credential *ChannelInstallationCredential,
	expectedRevision int64,
) error {
	if !validScope(scope) || installation == nil || credential == nil || expectedRevision < 1 ||
		installation.ID == uuid.Nil || installation.ConnectionID != installation.ID ||
		!isDigitalHumanChannelInstallation(installation) ||
		credential.InstallationID != installation.ID || credential.OrganizationID != scope.OrganizationID ||
		credential.WorkspaceID != scope.WorkspaceID || !strings.EqualFold(credential.Provider, installation.Provider) {
		return ErrInvalidRequest
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&channelruntime.Installation{}).Where(
			"id = ? AND organization_id = ? AND workspace_id = ? AND default_subject_type = ? AND default_subject_id = ? AND revision = ? AND deleted_at IS NULL",
			installation.ID, scope.OrganizationID, scope.WorkspaceID,
			runtimeidentity.SubjectTypeDigitalHuman, *installation.DefaultSubjectID, expectedRevision,
		).Updates(map[string]any{
			"connection_id": installation.ID, "external_tenant_id": installation.ExternalTenantID,
			"external_bot_id": installation.ExternalBotID, "updated_by": scope.ActorID,
			"revision": gorm.Expr("revision + 1"), "updated_at": installation.UpdatedAt,
		})
		if result.Error != nil {
			return fmt.Errorf("update channel installation credential reference: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrChannelConflict
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "installation_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"encrypted_credentials", "credential_version", "updated_by", "updated_at",
			}),
		}).Create(credential).Error; err != nil {
			return fmt.Errorf("replace channel installation credential: %w", err)
		}
		installation.Revision = expectedRevision + 1
		return nil
	})
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

func (r *GormRepository) ScopedChannelInstallationByID(
	ctx context.Context,
	scope Scope,
	digitalHumanID uuid.UUID,
	installationID uuid.UUID,
) (*channelruntime.Installation, error) {
	if !validScopeBoundary(scope) || digitalHumanID == uuid.Nil || installationID == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	var installation channelruntime.Installation
	err := r.db.WithContext(ctx).Where(
		"id = ? AND organization_id = ? AND workspace_id = ? AND default_subject_type = ? AND default_subject_id = ?",
		installationID, scope.OrganizationID, scope.WorkspaceID,
		runtimeidentity.SubjectTypeDigitalHuman, digitalHumanID,
	).Take(&installation).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrChannelInstallationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get scoped channel installation: %w", err)
	}
	return &installation, nil
}

func (r *GormRepository) ListChannelInstallations(ctx context.Context, scope Scope, digitalHumanID uuid.UUID) ([]*channelruntime.Installation, error) {
	if !validScopeBoundary(scope) || digitalHumanID == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	installations := make([]*channelruntime.Installation, 0)
	if err := r.db.WithContext(ctx).Where(
		"organization_id = ? AND workspace_id = ? AND default_subject_type = ? AND default_subject_id = ?",
		scope.OrganizationID, scope.WorkspaceID, runtimeidentity.SubjectTypeDigitalHuman, digitalHumanID,
	).Order("created_at ASC, id ASC").Find(&installations).Error; err != nil {
		return nil, fmt.Errorf("list channel installations: %w", err)
	}

	return installations, nil
}

func (r *GormRepository) ListActiveChannelInstallations(ctx context.Context) ([]*channelruntime.Installation, error) {
	installations := make([]*channelruntime.Installation, 0)
	if err := r.db.WithContext(ctx).Where("status = ?", channelruntime.InstallationStatusActive).
		Order("created_at ASC, id ASC").Find(&installations).Error; err != nil {
		return nil, fmt.Errorf("list active channel installations: %w", err)
	}
	return installations, nil
}

func (r *GormRepository) TryAcquireChannelReceiverLease(
	ctx context.Context,
	installationID uuid.UUID,
) (*ChannelReceiverLease, bool, error) {
	if installationID == uuid.Nil {
		return nil, false, ErrInvalidRequest
	}
	// PostgreSQL advisory locks are bound to one physical connection and are
	// released automatically if the API process exits. Other dialects are used
	// only by local tests and run a single receiver manager.
	if r.db.Dialector.Name() != "postgres" {
		return &ChannelReceiverLease{Release: func() {}}, true, nil
	}
	database, err := r.db.DB()
	if err != nil {
		return nil, false, fmt.Errorf("resolve channel receiver database: %w", err)
	}
	connection, err := database.Conn(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("reserve channel receiver connection: %w", err)
	}
	key := channelReceiverLeaseKey(installationID)
	var acquired bool
	if err := connection.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&acquired); err != nil {
		_ = connection.Close()
		return nil, false, fmt.Errorf("acquire channel receiver lease: %w", err)
	}
	if !acquired {
		_ = connection.Close()
		return nil, false, nil
	}
	return &ChannelReceiverLease{
		Release: func() { releaseChannelReceiverLease(connection, key) },
		Check:   connection.PingContext,
	}, true, nil
}

func channelReceiverLeaseKey(installationID uuid.UUID) int64 {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte("digital-human-channel-receiver:" + installationID.String()))
	return int64(hash.Sum64())
}

func releaseChannelReceiverLease(connection *sql.Conn, key int64) {
	if connection == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = connection.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", key)
	_ = connection.Close()
}

func (r *GormRepository) LastSuccessfulChannelDeliveryAt(
	ctx context.Context,
	installationID uuid.UUID,
	externalConversationID string,
	before time.Time,
) (*time.Time, error) {
	if installationID == uuid.Nil || strings.TrimSpace(externalConversationID) == "" || before.IsZero() {
		return nil, ErrInvalidRequest
	}
	var delivery channelruntime.Delivery
	err := r.db.WithContext(ctx).Select("delivered_at").Where(
		"installation_id = ? AND external_conversation_id = ? AND delivery_kind = ? AND status = ? AND delivered_at IS NOT NULL AND delivered_at <= ?",
		installationID, strings.TrimSpace(externalConversationID), channelruntime.DeliveryKindFinal, channelruntime.DeliverySucceeded, before,
	).Order("delivered_at DESC").Take(&delivery).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get last successful channel delivery: %w", err)
	}
	return delivery.DeliveredAt, nil
}

func (r *GormRepository) SuccessfulChannelDeliveryKinds(
	ctx context.Context,
	installationID uuid.UUID,
	externalConversationID string,
	subjectID uuid.UUID,
	providerMessageIDs []string,
) (map[string]channelruntime.DeliveryKind, error) {
	conversationID := strings.TrimSpace(externalConversationID)
	messageIDs := normalizedUniqueStrings(providerMessageIDs)
	if installationID == uuid.Nil || conversationID == "" || subjectID == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	if len(messageIDs) == 0 {
		return map[string]channelruntime.DeliveryKind{}, nil
	}

	type deliveryKindRow struct {
		ProviderMessageID string                      `gorm:"column:provider_message_id"`
		Kind              channelruntime.DeliveryKind `gorm:"column:delivery_kind"`
	}
	rows := make([]deliveryKindRow, 0, len(messageIDs))
	err := r.db.WithContext(ctx).Model(&channelruntime.Delivery{}).
		Select("provider_message_id, delivery_kind").
		Where(
			"installation_id = ? AND external_conversation_id = ? AND subject_type = ? AND subject_id = ? AND status = ? AND provider_message_id IN ?",
			installationID,
			conversationID,
			runtimeidentity.SubjectTypeDigitalHuman,
			subjectID,
			channelruntime.DeliverySucceeded,
			messageIDs,
		).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list successful channel delivery kinds: %w", err)
	}

	kinds := make(map[string]channelruntime.DeliveryKind, len(rows))
	for _, row := range rows {
		messageID := strings.TrimSpace(row.ProviderMessageID)
		if messageID != "" {
			kinds[messageID] = row.Kind
		}
	}
	return kinds, nil
}

func (r *GormRepository) UpdateChannelInstallation(ctx context.Context, scope Scope, installation *channelruntime.Installation, expectedRevision int64) error {
	if !validScope(scope) || installation == nil || installation.ID == uuid.Nil || expectedRevision < 1 ||
		!isDigitalHumanChannelInstallation(installation) {
		return ErrInvalidRequest
	}
	result := r.db.WithContext(ctx).Model(&channelruntime.Installation{}).Where(
		"id = ? AND organization_id = ? AND workspace_id = ? AND default_subject_type = ? AND default_subject_id = ? AND revision = ? AND deleted_at IS NULL",
		installation.ID, scope.OrganizationID, scope.WorkspaceID,
		runtimeidentity.SubjectTypeDigitalHuman, *installation.DefaultSubjectID, expectedRevision,
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

func (r *GormRepository) DeleteChannelInstallation(ctx context.Context, scope Scope, digitalHumanID, installationID uuid.UUID) error {
	if !validScope(scope) || digitalHumanID == uuid.Nil || installationID == uuid.Nil {
		return ErrInvalidRequest
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var installation channelruntime.Installation
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(
			"id = ? AND organization_id = ? AND workspace_id = ? AND default_subject_type = ? AND default_subject_id = ?",
			installationID, scope.OrganizationID, scope.WorkspaceID,
			runtimeidentity.SubjectTypeDigitalHuman, digitalHumanID,
		).Take(&installation).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrChannelInstallationNotFound
		}
		if err != nil {
			return fmt.Errorf("lock channel installation before delete: %w", err)
		}

		const bindingFilter = "organization_id = ? AND workspace_id = ? AND installation_id = ? AND deleted_at IS NULL"
		bindingArgs := []any{scope.OrganizationID, scope.WorkspaceID, installationID}
		if err := tx.Where(bindingFilter, bindingArgs...).Delete(&channelruntime.Binding{}).Error; err != nil {
			return fmt.Errorf("delete channel installation routes: %w", err)
		}
		if err := tx.Delete(&installation).Error; err != nil {
			return fmt.Errorf("delete channel installation: %w", err)
		}
		return nil
	})
}

func isDigitalHumanChannelInstallation(installation *channelruntime.Installation) bool {
	return installation != nil &&
		installation.DefaultSubjectType != nil &&
		*installation.DefaultSubjectType == runtimeidentity.SubjectTypeDigitalHuman &&
		installation.DefaultSubjectID != nil &&
		*installation.DefaultSubjectID != uuid.Nil
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

func (r *GormRepository) ChannelInboundEventByID(ctx context.Context, organizationID, workspaceID, installationID, eventID uuid.UUID) (*channelruntime.InboundEvent, error) {
	if organizationID == uuid.Nil || workspaceID == uuid.Nil || installationID == uuid.Nil || eventID == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	var event channelruntime.InboundEvent
	err := r.db.WithContext(ctx).Where(
		"id = ? AND organization_id = ? AND workspace_id = ? AND installation_id = ?",
		eventID, organizationID, workspaceID, installationID,
	).Take(&event).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrChannelBindingNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load channel inbound event: %w", err)
	}
	return &event, nil
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

func (r *GormRepository) ChannelConnectionTestEvidence(
	ctx context.Context,
	installationID uuid.UUID,
	conversationType channelruntime.ConversationType,
	testMessage string,
	startedAt time.Time,
) (*channelruntime.InboundEvent, *channelruntime.Delivery, error) {
	if installationID == uuid.Nil ||
		(conversationType != channelruntime.ConversationTypeDirect && conversationType != channelruntime.ConversationTypeGroup) ||
		strings.TrimSpace(testMessage) == "" || startedAt.IsZero() {
		return nil, nil, ErrInvalidRequest
	}
	var event channelruntime.InboundEvent
	query := r.db.WithContext(ctx).Where(
		"installation_id = ? AND conversation_type = ? AND content_preview = ? AND received_at >= ?",
		installationID, conversationType, strings.TrimSpace(testMessage), startedAt.UTC(),
	)
	if conversationType == channelruntime.ConversationTypeGroup {
		query = query.Where("event_type = ?", "channel.mention")
	}
	err := query.Order("received_at DESC, id DESC").Take(&event).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("load channel connection test inbound event: %w", err)
	}

	var delivery channelruntime.Delivery
	err = r.db.WithContext(ctx).Where(
		"inbound_event_id = ? AND delivery_kind = ?", event.ID, channelruntime.DeliveryKindFinal,
	).Order("created_at DESC, id DESC").Take(&delivery).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &event, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("load channel connection test delivery: %w", err)
	}
	return &event, &delivery, nil
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

func (r *GormRepository) DismissDelivery(ctx context.Context, scope Scope, subjectID, deliveryID uuid.UUID, now time.Time) error {
	if !validScope(scope) || subjectID == uuid.Nil || deliveryID == uuid.Nil || now.IsZero() {
		return ErrInvalidRequest
	}
	result := r.db.WithContext(ctx).Model(&channelruntime.Delivery{}).Where(
		"id = ? AND organization_id = ? AND workspace_id = ? AND subject_type = ? AND subject_id = ? AND status IN ?",
		deliveryID, scope.OrganizationID, scope.WorkspaceID, runtimeidentity.SubjectTypeDigitalHuman, subjectID,
		[]channelruntime.DeliveryStatus{channelruntime.DeliveryFailed, channelruntime.DeliveryUnknown},
	).Updates(map[string]any{
		"status": channelruntime.DeliveryDismissed, "claimed_at": nil,
		"dismissed_at": now, "dismissed_by": scope.ActorID, "updated_at": now,
	})
	if result.Error != nil {
		return fmt.Errorf("dismiss channel delivery: %w", result.Error)
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
	if run == nil || json.Unmarshal(run.InvocationEvent, &event) != nil || event.ReplyTo == nil {
		return nil
	}
	installationIdentity := strings.TrimSpace(event.ReplyTo.InstallationID)
	if installationIdentity == "" {
		installationIdentity = strings.TrimSpace(event.Origin.InstallationID)
	}
	installationID, err := uuid.Parse(installationIdentity)
	if err != nil || installationID == uuid.Nil {
		return fmt.Errorf("channel reply installation identity is invalid")
	}
	var binding channelruntime.Binding
	var inboundEventID *uuid.UUID
	if event.Origin.Kind == invocation.OriginChannel {
		inboundID, ok := contextUUID(event.Context, "channel_inbound_event_id")
		if !ok {
			return fmt.Errorf("channel invocation is missing inbound event identity")
		}
		var inbound channelruntime.InboundEvent
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND installation_id = ? AND organization_id = ? AND workspace_id = ?", inboundID, installationID, run.OrganizationID, run.WorkspaceID).Take(&inbound).Error; err != nil {
			return fmt.Errorf("load channel inbound event for delivery: %w", err)
		}
		if inbound.InvocationRunID != nil && *inbound.InvocationRunID != run.ID {
			return fmt.Errorf("channel inbound event is linked to another run")
		}
		if inbound.InvocationRunID == nil || inbound.Status != channelruntime.InboundEventAccepted {
			if err := tx.WithContext(ctx).Model(&channelruntime.InboundEvent{}).Where("id = ?", inbound.ID).Updates(map[string]any{"status": channelruntime.InboundEventAccepted, "reason_code": "", "invocation_run_id": run.ID, "processed_at": now, "updated_at": now}).Error; err != nil {
				return fmt.Errorf("repair channel inbound run linkage: %w", err)
			}
		}
		if inbound.BindingID == nil || *inbound.BindingID == uuid.Nil {
			return fmt.Errorf("channel inbound event is missing binding identity")
		}
		if err := tx.WithContext(ctx).Where("id = ? AND installation_id = ? AND organization_id = ? AND workspace_id = ? AND subject_type = ? AND subject_id = ?", *inbound.BindingID, installationID, run.OrganizationID, run.WorkspaceID, runtimeidentity.SubjectTypeDigitalHuman, run.DigitalHumanID).Take(&binding).Error; err != nil {
			return fmt.Errorf("load channel binding for delivery: %w", err)
		}
		inboundEventID = &inbound.ID
		if kind == channelruntime.DeliveryKindFinal {
			latestInboundEventID, err := applyConsumedChannelRuntimeInputs(ctx, tx, run, installationID, binding.ID, &event, now)
			if err != nil {
				return err
			}
			if latestInboundEventID != nil {
				inboundEventID = latestInboundEventID
			}
		}
	} else {
		if err := tx.WithContext(ctx).Where("installation_id = ? AND organization_id = ? AND workspace_id = ? AND subject_type = ? AND subject_id = ? AND enabled = ?", installationID, run.OrganizationID, run.WorkspaceID, runtimeidentity.SubjectTypeDigitalHuman, run.DigitalHumanID, true).Order("updated_at DESC").Take(&binding).Error; err != nil {
			return fmt.Errorf("load scheduled channel binding for delivery: %w", err)
		}
	}
	var installation channelruntime.Installation
	if err := tx.WithContext(ctx).Where(
		"id = ? AND organization_id = ? AND workspace_id = ? AND status = ?",
		installationID, run.OrganizationID, run.WorkspaceID, channelruntime.InstallationStatusActive,
	).Take(&installation).Error; err != nil {
		return fmt.Errorf("load channel installation for delivery: %w", err)
	}
	if provider := strings.ToLower(strings.TrimSpace(event.ReplyTo.Provider)); provider == "" || provider != strings.ToLower(strings.TrimSpace(installation.Provider)) {
		return fmt.Errorf("channel reply provider does not match installation")
	}
	text = boundedRunes(strings.TrimSpace(text), maxChannelDeliveryRunes)
	if text == "" && len(artifacts) == 0 {
		return nil
	}
	mentions := channelDeliveryMentions(event, kind)
	payload, digest, err := channelDeliveryPayloadWithMentions(kind, text, mentions, artifacts...)
	if err != nil {
		return err
	}
	delivery := &channelruntime.Delivery{
		ID: channelDeliveryStageID(run.ID, kind, deliveryKey), OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID,
		InstallationID: installation.ID, BindingID: binding.ID, InboundEventID: inboundEventID, RunID: run.ID,
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

// applyConsumedChannelRuntimeInputs advances the final reply target to the
// latest channel message that was actually consumed by the current model run.
// The stored invocation remains immutable; only this delivery intent uses the
// newer sender identity. A callback may have briefly created its own durable
// run before the earlier ChatRuntime message became visible; consumption is the
// authoritative ownership signal, so the inbound audit row is reassigned here.
func applyConsumedChannelRuntimeInputs(
	ctx context.Context,
	tx *gorm.DB,
	run *Run,
	installationID, bindingID uuid.UUID,
	event *invocation.Event,
	now time.Time,
) (*uuid.UUID, error) {
	if run == nil || run.MessageID == nil || *run.MessageID == uuid.Nil || event == nil || event.ReplyTo == nil {
		return nil, nil
	}
	var inputs []runtimemodel.RuntimeInput
	if err := tx.WithContext(ctx).Where(
		"target_message_id = ? AND consumed_message_id = ? AND status = ? AND consumption_mode = ?",
		*run.MessageID,
		*run.MessageID,
		runtimemodel.RuntimeInputStatusConsumed,
		runtimemodel.RuntimeInputConsumptionCurrentRun,
	).Order("sequence ASC").Find(&inputs).Error; err != nil {
		return nil, fmt.Errorf("load consumed channel runtime inputs: %w", err)
	}
	if len(inputs) == 0 {
		return nil, nil
	}

	clientInputIDs := make([]uuid.UUID, 0, len(inputs))
	for _, input := range inputs {
		if input.ClientInputID != uuid.Nil {
			clientInputIDs = append(clientInputIDs, input.ClientInputID)
		}
	}
	if len(clientInputIDs) == 0 {
		return nil, nil
	}
	var inboundEvents []channelruntime.InboundEvent
	if err := tx.WithContext(ctx).Where(
		"id IN ? AND installation_id = ? AND organization_id = ? AND workspace_id = ? AND binding_id = ?",
		clientInputIDs,
		installationID,
		run.OrganizationID,
		run.WorkspaceID,
		bindingID,
	).Find(&inboundEvents).Error; err != nil {
		return nil, fmt.Errorf("load channel runtime input events: %w", err)
	}
	if len(inboundEvents) == 0 {
		return nil, nil
	}
	inboundByID := make(map[uuid.UUID]*channelruntime.InboundEvent, len(inboundEvents))
	for index := range inboundEvents {
		inbound := &inboundEvents[index]
		inboundByID[inbound.ID] = inbound
	}
	var latest *channelruntime.InboundEvent
	for _, inputID := range clientInputIDs {
		inbound := inboundByID[inputID]
		if inbound == nil {
			continue
		}
		if strings.TrimSpace(inbound.ExternalConversationID) != strings.TrimSpace(event.ReplyTo.ConversationID) {
			return nil, fmt.Errorf("channel runtime input conversation does not match delivery target")
		}
		latest = inbound
	}
	if latest == nil {
		return nil, nil
	}
	if err := tx.WithContext(ctx).Model(&channelruntime.InboundEvent{}).Where(
		"id IN ? AND installation_id = ? AND organization_id = ? AND workspace_id = ? AND binding_id = ?",
		clientInputIDs,
		installationID,
		run.OrganizationID,
		run.WorkspaceID,
		bindingID,
	).Updates(map[string]any{
		"invocation_run_id": run.ID,
		"processed_at":      now,
		"updated_at":        now,
	}).Error; err != nil {
		return nil, fmt.Errorf("link channel runtime input events to run: %w", err)
	}

	event.Sender = invocation.PrincipalRef{Type: invocation.PrincipalExternalUser, ID: latest.SenderExternalID}
	if event.Context == nil {
		event.Context = map[string]any{}
	}
	event.Context["provider_message_id"] = latest.ProviderMessageID
	event.Context["sender_display_name"] = stableChannelAuthorAlias(installationID, latest.ExternalConversationID, latest.SenderExternalID)
	event.ReplyTo.ConversationID = latest.ExternalConversationID
	if latest.ConversationType == channelruntime.ConversationTypeGroup {
		event.ReplyTo.RecipientID = strings.TrimSpace(latest.SenderExternalID)
	} else {
		event.ReplyTo.RecipientID = ""
	}
	latestID := latest.ID
	return &latestID, nil
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
	return channelDeliveryPayloadWithMentions(kind, text, nil, artifacts...)
}

func channelDeliveryPayloadWithMentions(
	kind channelruntime.DeliveryKind,
	text string,
	mentions []channelruntime.DeliveryMention,
	artifacts ...channelDeliveryArtifact,
) (datatypes.JSON, string, error) {
	if len(artifacts) > maxChannelDeliveryArtifacts {
		artifacts = artifacts[:maxChannelDeliveryArtifacts]
	}
	if len(mentions) > maxChannelDeliveryMentions {
		mentions = mentions[:maxChannelDeliveryMentions]
	}
	payload, err := json.Marshal(channelDeliveryPayloadData{Kind: kind, Text: text, Mentions: mentions, Artifacts: artifacts})
	if err != nil {
		return nil, "", fmt.Errorf("encode channel delivery payload: %w", err)
	}
	digest := sha256.Sum256(payload)
	return datatypes.JSON(payload), "sha256:" + hex.EncodeToString(digest[:]), nil
}

func channelDeliveryMentions(event invocation.Event, kind channelruntime.DeliveryKind) []channelruntime.DeliveryMention {
	if kind != channelruntime.DeliveryKindFinal || event.ReplyTo == nil {
		return nil
	}
	externalID := strings.TrimSpace(event.ReplyTo.RecipientID)
	if externalID == "" {
		return nil
	}
	displayName := ""
	if event.Context != nil {
		if value, ok := event.Context["sender_display_name"].(string); ok {
			displayName = boundedRunes(strings.TrimSpace(value), maxChannelMentionNameRunes)
		}
	}
	return []channelruntime.DeliveryMention{{ExternalID: boundedRunes(externalID, maxChannelMentionIDRunes), DisplayName: displayName}}
}

func boundedRunes(value string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:limit]))
}
