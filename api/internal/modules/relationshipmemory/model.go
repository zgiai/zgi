package relationshipmemory

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	ConsumerAgent        = "agent"
	ConsumerDigitalHuman = "digital_human"

	RelationshipActive   = "active"
	RelationshipArchived = "archived"

	ContentEncodingPlain = "plain"

	SourceLegacy    = "legacy"
	SourceExplicit  = "explicit"
	SourceAutomatic = "automatic"
	SourceManager   = "manager"
)

type UserMemoryRelationship struct {
	ID                   uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID       uuid.UUID  `gorm:"type:uuid;not null;index" json:"organization_id"`
	WorkspaceID          uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_user_memory_relationship_scope,priority:1;index" json:"workspace_id"`
	SubjectID            uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_user_memory_relationship_scope,priority:2;index" json:"subject_id"`
	ConsumerType         string     `gorm:"type:varchar(32);not null;uniqueIndex:idx_user_memory_relationship_scope,priority:3;index" json:"consumer_type"`
	ConsumerID           uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_user_memory_relationship_scope,priority:4;index" json:"consumer_id"`
	ConsumerNameSnapshot string     `gorm:"type:varchar(255);not null;default:''" json:"consumer_name_snapshot"`
	Status               string     `gorm:"type:varchar(24);not null;default:'active';index" json:"status"`
	MemoryEnabled        bool       `gorm:"not null;default:true" json:"memory_enabled"`
	AutoCaptureEnabled   bool       `gorm:"not null;default:false" json:"auto_capture_enabled"`
	SettingsRevision     int64      `gorm:"not null;default:1" json:"settings_revision"`
	MemoryEpoch          int64      `gorm:"not null;default:0" json:"memory_epoch"`
	ExtractionCutoffAt   *time.Time `json:"-"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	ArchivedAt           *time.Time `json:"archived_at,omitempty"`
}

func (UserMemoryRelationship) TableName() string { return "user_memory_relationships" }
func (r *UserMemoryRelationship) BeforeCreate(*gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	if r.Status == "" {
		r.Status = RelationshipActive
	}
	if r.SettingsRevision == 0 {
		r.SettingsRevision = 1
	}
	return nil
}

type UserMemoryItem struct {
	ID                   uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	RelationshipID       uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_user_memory_item_key,priority:1;index" json:"relationship_id"`
	MemoryKey            string     `gorm:"type:varchar(64);not null;uniqueIndex:idx_user_memory_item_key,priority:2" json:"key"`
	DefinitionID         *uuid.UUID `gorm:"type:uuid" json:"definition_id,omitempty"`
	DisplayName          string     `gorm:"type:varchar(80);not null;default:''" json:"display_name"`
	Category             string     `gorm:"type:varchar(32);not null;default:''" json:"category,omitempty"`
	ContentPayload       string     `gorm:"type:text;not null;default:''" json:"content"`
	ContentEncoding      string     `gorm:"type:varchar(24);not null;default:'plain'" json:"content_encoding"`
	ContentKeyID         *string    `gorm:"type:varchar(128)" json:"-"`
	Revision             int64      `gorm:"not null;default:1" json:"revision"`
	SourceKind           string     `gorm:"type:varchar(32);not null;default:'explicit'" json:"source_kind"`
	SourceConversationID *uuid.UUID `gorm:"type:uuid" json:"source_conversation_id,omitempty"`
	SourceMessageID      *uuid.UUID `gorm:"type:uuid" json:"source_message_id,omitempty"`
	SourceCompletedAt    *time.Time `json:"source_completed_at,omitempty"`
	ExtractorVersion     string     `gorm:"type:varchar(64);not null;default:''" json:"extractor_version,omitempty"`
	LastOperationID      *uuid.UUID `gorm:"type:uuid" json:"last_operation_id,omitempty"`
	UndoableUntil        *time.Time `gorm:"-" json:"undoable_until,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

func (UserMemoryItem) TableName() string { return "user_memory_items" }
func (i *UserMemoryItem) BeforeCreate(*gorm.DB) error {
	if i.ID == uuid.Nil {
		i.ID = uuid.New()
	}
	if i.ContentEncoding == "" {
		i.ContentEncoding = ContentEncodingPlain
	}
	if i.Revision == 0 {
		i.Revision = 1
	}
	return nil
}

type UserMemoryEvent struct {
	ID                   uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OperationID          *uuid.UUID `gorm:"type:uuid;uniqueIndex,where:operation_id IS NOT NULL" json:"operation_id,omitempty"`
	RelationshipID       uuid.UUID  `gorm:"type:uuid;not null;index" json:"relationship_id"`
	MemoryKey            string     `gorm:"type:varchar(64);not null;default:''" json:"key,omitempty"`
	Action               string     `gorm:"type:varchar(32);not null;index" json:"action"`
	ActorType            string     `gorm:"type:varchar(32);not null;default:'system'" json:"actor_type"`
	ActorID              *uuid.UUID `gorm:"type:uuid" json:"actor_id,omitempty"`
	Source               string     `gorm:"type:varchar(32);not null;default:'api'" json:"source"`
	SourceConversationID *uuid.UUID `gorm:"type:uuid" json:"source_conversation_id,omitempty"`
	SourceMessageID      *uuid.UUID `gorm:"type:uuid" json:"source_message_id,omitempty"`
	BeforeRevision       *int64     `json:"before_revision,omitempty"`
	AfterRevision        *int64     `json:"after_revision,omitempty"`
	Result               string     `gorm:"type:varchar(32);not null;default:'success'" json:"result"`
	CreatedAt            time.Time  `json:"created_at"`
}

func (UserMemoryEvent) TableName() string { return "user_memory_events" }
func (e *UserMemoryEvent) BeforeCreate(*gorm.DB) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return nil
}

type UserMemoryUndoRecord struct {
	OperationID               uuid.UUID  `gorm:"type:uuid;primaryKey" json:"operation_id"`
	RelationshipID            uuid.UUID  `gorm:"type:uuid;not null;index" json:"relationship_id"`
	MemoryKey                 string     `gorm:"type:varchar(64);not null" json:"key"`
	PreviousExists            bool       `gorm:"not null;default:false" json:"previous_exists"`
	PreviousItemID            *uuid.UUID `gorm:"type:uuid" json:"-"`
	PreviousDisplayName       string     `gorm:"type:varchar(80);not null;default:''" json:"-"`
	PreviousCategory          string     `gorm:"type:varchar(32);not null;default:''" json:"-"`
	PreviousContentPayload    string     `gorm:"type:text;not null;default:''" json:"-"`
	PreviousContentEncoding   string     `gorm:"type:varchar(24);not null;default:'plain'" json:"-"`
	PreviousContentKeyID      *string    `gorm:"type:varchar(128)" json:"-"`
	PreviousRevision          int64      `gorm:"not null;default:0" json:"previous_revision"`
	PreviousSourceKind        string     `gorm:"type:varchar(32);not null;default:''" json:"previous_source_kind"`
	PreviousConversationID    *uuid.UUID `gorm:"type:uuid" json:"previous_conversation_id,omitempty"`
	PreviousMessageID         *uuid.UUID `gorm:"type:uuid" json:"previous_message_id,omitempty"`
	PreviousSourceCompletedAt *time.Time `json:"previous_source_completed_at,omitempty"`
	PreviousExtractorVersion  string     `gorm:"type:varchar(64);not null;default:''" json:"previous_extractor_version"`
	ResultingRevision         int64      `gorm:"not null" json:"resulting_revision"`
	ExpiresAt                 time.Time  `gorm:"not null;index" json:"expires_at"`
	CreatedAt                 time.Time  `json:"created_at"`
}

func (UserMemoryUndoRecord) TableName() string { return "user_memory_undo_records" }

type UserMemoryExtractionJob struct {
	ID                 uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	RelationshipID     uuid.UUID      `gorm:"type:uuid;not null;index" json:"relationship_id"`
	ConversationID     uuid.UUID      `gorm:"type:uuid;not null" json:"conversation_id"`
	MessageWatermarkID uuid.UUID      `gorm:"type:uuid;not null" json:"message_watermark_id"`
	MemoryEpoch        int64          `gorm:"not null;default:0" json:"memory_epoch"`
	ConfigRevision     string         `gorm:"type:varchar(64);not null;default:''" json:"config_revision"`
	DefinitionSnapshot datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"definition_snapshot"`
	ExtractorVersion   string         `gorm:"type:varchar(64);not null" json:"extractor_version"`
	IdempotencyKey     string         `gorm:"type:varchar(128);not null;uniqueIndex" json:"idempotency_key"`
	Status             string         `gorm:"type:varchar(24);not null;default:'pending';index" json:"status"`
	AttemptCount       int            `gorm:"not null;default:0" json:"attempt_count"`
	ErrorCode          string         `gorm:"type:varchar(64);not null;default:''" json:"error_code,omitempty"`
	ScheduledAt        time.Time      `gorm:"not null;index" json:"scheduled_at"`
	ForceAt            time.Time      `gorm:"not null" json:"force_at"`
	StartedAt          *time.Time     `json:"started_at,omitempty"`
	FinishedAt         *time.Time     `json:"finished_at,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

func (UserMemoryExtractionJob) TableName() string { return "user_memory_extraction_jobs" }
func (j *UserMemoryExtractionJob) BeforeCreate(*gorm.DB) error {
	if j.ID == uuid.Nil {
		j.ID = uuid.New()
	}
	return nil
}
