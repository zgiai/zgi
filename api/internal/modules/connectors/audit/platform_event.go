package audit

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PlatformActorKind distinguishes an automatic platform lifecycle transition
// from an explicit maintainer action without inventing an owner scope for a
// plugin package. Member-owned lifecycle decisions continue to use
// connector_lifecycle_decisions.
type PlatformActorKind string

const (
	PlatformActorSystem             PlatformActorKind = "system"
	PlatformActorPlatformMaintainer PlatformActorKind = "platform_maintainer"
)

func (kind PlatformActorKind) Valid() bool {
	return kind == PlatformActorSystem || kind == PlatformActorPlatformMaintainer
}

// PlatformActor is safe audit identity supplied by the production composition
// root. ID is optional because scheduled startup admission has no account.
type PlatformActor struct {
	Kind PlatformActorKind
	ID   string
}

type platformActorContextKey struct{}

func WithPlatformActor(ctx context.Context, actor PlatformActor) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, platformActorContextKey{}, actor)
}

func PlatformActorFromContext(ctx context.Context) (PlatformActor, bool) {
	if ctx == nil {
		return PlatformActor{}, false
	}
	actor, ok := ctx.Value(platformActorContextKey{}).(PlatformActor)
	if !ok || !validPlatformActor(actor) {
		return PlatformActor{}, false
	}
	return actor, true
}

// PlatformLifecycleEvent is immutable, non-secret evidence for lifecycle
// changes that do not belong to one organization/Connection owner, currently
// Connector plugin admission and availability transitions.
type PlatformLifecycleEvent struct {
	ID            uuid.UUID
	SubjectType   string
	SubjectID     string
	Actor         PlatformActor
	Decision      string
	Status        core.DecisionStatus
	CorrelationID string
	Details       map[string]any
	CreatedAt     time.Time
}

func (event PlatformLifecycleEvent) Validate() error {
	if event.ID == uuid.Nil || event.SubjectType != "connector_plugin" ||
		!validRequiredText(event.SubjectID, 128) || !validPlatformActor(event.Actor) ||
		!validRequiredText(event.Decision, 64) || event.Status != core.DecisionStatusSuccessful ||
		!validRequiredText(event.CorrelationID, 128) || event.CreatedAt.IsZero() || event.Details == nil {
		return invalidInputError()
	}
	return nil
}

type platformLifecycleEventRecord struct {
	ID            uuid.UUID      `gorm:"column:id;type:uuid;primaryKey"`
	SubjectType   string         `gorm:"column:subject_type;type:varchar(32);not null"`
	SubjectID     string         `gorm:"column:subject_id;type:varchar(128);not null"`
	ActorKind     string         `gorm:"column:actor_kind;type:varchar(32);not null"`
	ActorID       *string        `gorm:"column:actor_id;type:varchar(128)"`
	Decision      string         `gorm:"column:decision;type:varchar(64);not null"`
	Status        string         `gorm:"column:status;type:varchar(32);not null"`
	CorrelationID string         `gorm:"column:correlation_id;type:varchar(128);not null"`
	Details       datatypes.JSON `gorm:"column:details;type:jsonb;not null"`
	CreatedAt     time.Time      `gorm:"column:created_at;not null"`
}

func (platformLifecycleEventRecord) TableName() string {
	return "connector_platform_lifecycle_events"
}

// CreatePlatformEvent inserts or verifies an identical deterministic replay.
// Calling it on a GormRepository constructed with the caller's transaction
// makes the authoritative transition and its audit event atomic.
func (repository *GormRepository) CreatePlatformEvent(ctx context.Context, event PlatformLifecycleEvent) error {
	if repository == nil || repository.db == nil {
		return internalError()
	}
	record, err := newPlatformLifecycleEventRecord(event)
	if err != nil {
		return err
	}
	result := repository.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
	if result.Error != nil {
		return internalError()
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var existing platformLifecycleEventRecord
	err = repository.db.WithContext(ctx).
		Where("subject_type = ? AND subject_id = ? AND correlation_id = ?",
			record.SubjectType, record.SubjectID, record.CorrelationID).
		Take(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && !samePlatformLifecycleEvent(existing, record) {
		return conflictError()
	}
	if err != nil {
		return internalError()
	}
	return nil
}

func newPlatformLifecycleEventRecord(event PlatformLifecycleEvent) (platformLifecycleEventRecord, error) {
	if err := event.Validate(); err != nil {
		return platformLifecycleEventRecord{}, err
	}
	details, err := encodeSafeDetails(event.Details)
	if err != nil {
		return platformLifecycleEventRecord{}, err
	}
	return platformLifecycleEventRecord{
		ID: event.ID, SubjectType: event.SubjectType, SubjectID: event.SubjectID,
		ActorKind: string(event.Actor.Kind), ActorID: optionalString(event.Actor.ID),
		Decision: event.Decision, Status: string(event.Status), CorrelationID: event.CorrelationID,
		Details: details, CreatedAt: event.CreatedAt.UTC(),
	}, nil
}

func validPlatformActor(actor PlatformActor) bool {
	trimmedID := strings.TrimSpace(actor.ID)
	if !actor.Kind.Valid() || !validOptionalOpaqueText(trimmedID, 128) || actor.ID != trimmedID {
		return false
	}
	return actor.Kind != PlatformActorPlatformMaintainer || trimmedID != ""
}

func samePlatformLifecycleEvent(left, right platformLifecycleEventRecord) bool {
	left.ID, right.ID = uuid.Nil, uuid.Nil
	// Correlation is the idempotency identity. A retry may reconstruct its local
	// UUID/timestamp, but it cannot change actor, transition, or evidence; the
	// original immutable row retains its first ID and CreatedAt.
	left.CreatedAt, right.CreatedAt = time.Time{}, time.Time{}
	leftDetails, rightDetails := left.Details, right.Details
	left.Details, right.Details = nil, nil
	if !reflect.DeepEqual(left, right) {
		return false
	}
	var leftValue, rightValue any
	return json.Unmarshal(leftDetails, &leftValue) == nil && json.Unmarshal(rightDetails, &rightValue) == nil &&
		reflect.DeepEqual(leftValue, rightValue)
}
