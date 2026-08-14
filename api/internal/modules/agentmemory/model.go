package agentmemory

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	UserScopeAccount = "account"
	UserScopeEndUser = "end_user"

	SourceKindLegacy    = "legacy"
	SourceKindExplicit  = "explicit"
	SourceKindAutomatic = "automatic"
	SourceKindManager   = "manager"

	EventActionSlotCreate  = "slot_create"
	EventActionSlotUpdate  = "slot_update"
	EventActionSlotDisable = "slot_disable"
	EventActionSlotDelete  = "slot_delete"
	EventActionValueUpdate = "value_update"
	EventActionValueClear  = "value_clear"
	EventActionValueUndo   = "value_undo"
	EventActionValuesClear = "values_clear"

	EventActorOrganizer = "organizer"
	EventActorUser      = "user"
	EventActorModel     = "model"
	EventActorSystem    = "system"

	EventSourceAPI   = "api"
	EventSourceAgent = "agent"

	ExtractionJobPending   = "pending"
	ExtractionJobQueued    = "queued"
	ExtractionJobRunning   = "running"
	ExtractionJobCompleted = "completed"
	ExtractionJobFailed    = "failed"
	ExtractionJobExhausted = "exhausted"
	ExtractionJobCancelled = "cancelled"
)

type AgentMemorySlot struct {
	ID          uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	WorkspaceID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_agent_memory_slots_agent_key,priority:1;index:idx_agent_memory_slots_agent_sort,priority:1" json:"workspace_id"`
	AgentID     uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_agent_memory_slots_agent_key,priority:2;index:idx_agent_memory_slots_agent_sort,priority:2" json:"agent_id"`
	Key         string    `gorm:"type:varchar(64);not null;uniqueIndex:idx_agent_memory_slots_agent_key,priority:3" json:"key"`
	Name        string    `gorm:"type:varchar(80);not null;default:''" json:"name"`
	Description string    `gorm:"type:text;not null;default:''" json:"description"`
	MaxChars    int       `gorm:"not null;default:1000" json:"max_chars"`
	Enabled     bool      `gorm:"not null;default:true;index" json:"enabled"`
	SortOrder   int       `gorm:"not null;default:0;index:idx_agent_memory_slots_agent_sort,priority:3" json:"sort_order"`
	CreatedBy   uuid.UUID `gorm:"type:uuid" json:"created_by"`
	UpdatedBy   uuid.UUID `gorm:"type:uuid" json:"updated_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (AgentMemorySlot) TableName() string {
	return "agent_memory_slots"
}

func (s *AgentMemorySlot) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

type AgentMemoryValue struct {
	ID                   uuid.UUID  `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	WorkspaceID          uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_agent_memory_values_scope,priority:1;index:idx_agent_memory_values_agent_user,priority:1" json:"workspace_id"`
	AgentID              uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_agent_memory_values_scope,priority:2;index:idx_agent_memory_values_agent_user,priority:2" json:"agent_id"`
	SlotKey              string     `gorm:"type:varchar(64);not null;uniqueIndex:idx_agent_memory_values_scope,priority:3;index" json:"slot_key"`
	UserScope            string     `gorm:"type:varchar(32);not null;uniqueIndex:idx_agent_memory_values_scope,priority:4;index:idx_agent_memory_values_agent_user,priority:3" json:"user_scope"`
	UserID               uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_agent_memory_values_scope,priority:5;index:idx_agent_memory_values_agent_user,priority:4" json:"user_id"`
	Content              string     `gorm:"type:text;not null;default:''" json:"content"`
	Revision             int64      `gorm:"not null;default:1" json:"revision"`
	SourceKind           string     `gorm:"type:varchar(32);not null;default:'legacy'" json:"source_kind"`
	SourceConversationID *uuid.UUID `gorm:"type:uuid" json:"source_conversation_id,omitempty"`
	SourceMessageID      *uuid.UUID `gorm:"type:uuid" json:"source_message_id,omitempty"`
	SourceCompletedAt    *time.Time `json:"source_completed_at,omitempty"`
	ExtractorVersion     string     `gorm:"type:varchar(64);not null;default:''" json:"extractor_version,omitempty"`
	LastOperationID      *uuid.UUID `gorm:"type:uuid" json:"last_operation_id,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

func (AgentMemoryValue) TableName() string {
	return "agent_memory_values"
}

func (v *AgentMemoryValue) BeforeCreate(tx *gorm.DB) error {
	if v.ID == uuid.Nil {
		v.ID = uuid.New()
	}
	return nil
}

type AgentMemoryEvent struct {
	ID                   uuid.UUID      `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	OperationID          *uuid.UUID     `gorm:"type:uuid;uniqueIndex:idx_agent_memory_events_operation" json:"operation_id,omitempty"`
	WorkspaceID          uuid.UUID      `gorm:"type:uuid;not null;index:idx_agent_memory_events_agent_created,priority:1" json:"workspace_id"`
	AgentID              uuid.UUID      `gorm:"type:uuid;not null;index:idx_agent_memory_events_agent_created,priority:2" json:"agent_id"`
	SlotKey              string         `gorm:"type:varchar(64);not null;default:'';index" json:"slot_key,omitempty"`
	UserScope            string         `gorm:"type:varchar(32);index" json:"user_scope,omitempty"`
	UserID               *uuid.UUID     `gorm:"type:uuid;index" json:"user_id,omitempty"`
	Action               string         `gorm:"type:varchar(32);not null;index" json:"action"`
	ActorType            string         `gorm:"type:varchar(32);not null;default:'system';index" json:"actor_type"`
	Source               string         `gorm:"type:varchar(32);not null;default:'api';index" json:"source"`
	SourceConversationID *uuid.UUID     `gorm:"type:uuid;index" json:"source_conversation_id,omitempty"`
	SourceMessageID      *uuid.UUID     `gorm:"type:uuid;index" json:"source_message_id,omitempty"`
	BeforeSnapshot       datatypes.JSON `gorm:"type:jsonb" json:"before_snapshot,omitempty"`
	AfterSnapshot        datatypes.JSON `gorm:"type:jsonb" json:"after_snapshot,omitempty"`
	BeforeRevision       *int64         `json:"before_revision,omitempty"`
	AfterRevision        *int64         `json:"after_revision,omitempty"`
	Result               string         `gorm:"type:varchar(32);not null;default:'success'" json:"result"`
	CreatedAt            time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP;index:idx_agent_memory_events_agent_created,priority:3" json:"created_at"`
}

// AgentMemorySubjectState invalidates background work after a permanent delete.
type AgentMemorySubjectState struct {
	ID          uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	WorkspaceID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_agent_memory_subject_scope,priority:1" json:"workspace_id"`
	AgentID     uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_agent_memory_subject_scope,priority:2" json:"agent_id"`
	UserScope   string    `gorm:"type:varchar(32);not null;uniqueIndex:idx_agent_memory_subject_scope,priority:3" json:"user_scope"`
	UserID      uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_agent_memory_subject_scope,priority:4" json:"user_id"`
	MemoryEpoch int64     `gorm:"not null;default:0" json:"memory_epoch"`
	// ExtractionCutoffAt prevents future jobs from re-reading messages that
	// predate an explicit deletion, including jobs scheduled after the delete.
	ExtractionCutoffAt *time.Time `json:"extraction_cutoff_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func (AgentMemorySubjectState) TableName() string { return "agent_memory_subject_states" }

func (s *AgentMemorySubjectState) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

// AgentMemoryExtractionJob is a durable outbox record. It stores references only.
type AgentMemoryExtractionJob struct {
	ID                 uuid.UUID  `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	WorkspaceID        uuid.UUID  `gorm:"type:uuid;not null;index:idx_agent_memory_jobs_due,priority:1" json:"workspace_id"`
	AgentID            uuid.UUID  `gorm:"type:uuid;not null;index" json:"agent_id"`
	UserScope          string     `gorm:"type:varchar(32);not null" json:"user_scope"`
	UserID             uuid.UUID  `gorm:"type:uuid;not null" json:"user_id"`
	ConversationID     uuid.UUID  `gorm:"type:uuid;not null" json:"conversation_id"`
	MessageWatermarkID uuid.UUID  `gorm:"type:uuid;not null" json:"message_watermark_id"`
	MemoryEpoch        int64      `gorm:"not null;default:0" json:"memory_epoch"`
	ExtractorVersion   string     `gorm:"type:varchar(64);not null" json:"extractor_version"`
	IdempotencyKey     string     `gorm:"type:varchar(128);not null;uniqueIndex" json:"idempotency_key"`
	Status             string     `gorm:"type:varchar(24);not null;default:'pending';index:idx_agent_memory_jobs_due,priority:2" json:"status"`
	AttemptCount       int        `gorm:"not null;default:0" json:"attempt_count"`
	ErrorCode          string     `gorm:"type:varchar(64);not null;default:''" json:"error_code,omitempty"`
	ScheduledAt        time.Time  `gorm:"not null;index:idx_agent_memory_jobs_due,priority:3" json:"scheduled_at"`
	ForceAt            time.Time  `gorm:"not null" json:"force_at"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	FinishedAt         *time.Time `json:"finished_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func (AgentMemoryExtractionJob) TableName() string { return "agent_memory_extraction_jobs" }

func (j *AgentMemoryExtractionJob) BeforeCreate(tx *gorm.DB) error {
	if j.ID == uuid.Nil {
		j.ID = uuid.New()
	}
	return nil
}

// AgentMemoryUndoRecord keeps the previous value only for the 24-hour undo window.
type AgentMemoryUndoRecord struct {
	OperationID               uuid.UUID  `gorm:"type:uuid;primaryKey" json:"operation_id"`
	WorkspaceID               uuid.UUID  `gorm:"type:uuid;not null;index:idx_agent_memory_undo_scope,priority:1" json:"workspace_id"`
	AgentID                   uuid.UUID  `gorm:"type:uuid;not null;index:idx_agent_memory_undo_scope,priority:2" json:"agent_id"`
	UserScope                 string     `gorm:"type:varchar(32);not null;index:idx_agent_memory_undo_scope,priority:3" json:"user_scope"`
	UserID                    uuid.UUID  `gorm:"type:uuid;not null;index:idx_agent_memory_undo_scope,priority:4" json:"user_id"`
	SlotKey                   string     `gorm:"type:varchar(64);not null" json:"slot_key"`
	PreviousExists            bool       `gorm:"not null;default:false" json:"previous_exists"`
	PreviousContent           string     `gorm:"type:text;not null;default:''" json:"-"`
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

func (AgentMemoryUndoRecord) TableName() string { return "agent_memory_undo_records" }

func (AgentMemoryEvent) TableName() string {
	return "agent_memory_events"
}

func (e *AgentMemoryEvent) BeforeCreate(tx *gorm.DB) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return nil
}
