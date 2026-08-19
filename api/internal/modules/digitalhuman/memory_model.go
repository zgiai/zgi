package digitalhuman

import (
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
	"gorm.io/datatypes"
)

type MemoryStatus string

const (
	MemoryStatusActive      MemoryStatus = "active"
	MemoryStatusInvalidated MemoryStatus = "invalidated"
	MemoryStatusArchived    MemoryStatus = "archived"
)

type MemoryVisibility string

const (
	MemoryVisibilityWorkspace  MemoryVisibility = "workspace"
	MemoryVisibilityRestricted MemoryVisibility = "restricted"
)

// MemoryState is the subject-wide stale-write fence. No actor identifier is
// part of this state because memory is owned solely by the DigitalHuman.
type MemoryState struct {
	ID                    uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID        uuid.UUID  `gorm:"type:uuid;not null" json:"organization_id"`
	WorkspaceID           uuid.UUID  `gorm:"type:uuid;not null" json:"workspace_id"`
	DigitalHumanID        uuid.UUID  `gorm:"type:uuid;not null" json:"digital_human_id"`
	MemoryEpoch           int64      `gorm:"not null" json:"memory_epoch"`
	ConfigRevision        string     `gorm:"type:varchar(71);not null" json:"config_revision"`
	AutoExtractionEnabled bool       `gorm:"not null" json:"auto_extraction_enabled"`
	ExtractionCutoffAt    *time.Time `json:"extraction_cutoff_at,omitempty"`
	CreatedAt             time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt             time.Time  `gorm:"not null" json:"updated_at"`
}

func (MemoryState) TableName() string { return "digital_human_memory_states" }

type DigitalHumanMemory struct {
	ID                uuid.UUID                     `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID    uuid.UUID                     `gorm:"type:uuid;not null" json:"organization_id"`
	WorkspaceID       uuid.UUID                     `gorm:"type:uuid;not null" json:"workspace_id"`
	DigitalHumanID    uuid.UUID                     `gorm:"type:uuid;not null" json:"digital_human_id"`
	MemoryKey         string                        `gorm:"type:varchar(160);not null" json:"memory_key"`
	MemoryClass       memoryruntime.MemoryClass     `gorm:"type:varchar(16);not null" json:"memory_class"`
	MemoryType        memoryruntime.MemoryType      `gorm:"type:varchar(24);not null" json:"memory_type"`
	ScopeType         memoryruntime.MemoryScopeType `gorm:"type:varchar(24);not null" json:"scope_type,omitempty"`
	ScopeID           string                        `gorm:"type:varchar(255);not null" json:"scope_id,omitempty"`
	Content           string                        `gorm:"type:text;not null" json:"content"`
	ContentDigest     string                        `gorm:"type:varchar(71);not null" json:"content_digest"`
	Visibility        MemoryVisibility              `gorm:"type:varchar(16);not null" json:"visibility"`
	Status            MemoryStatus                  `gorm:"type:varchar(16);not null" json:"status"`
	Confidence        float64                       `gorm:"not null" json:"confidence"`
	Revision          int64                         `gorm:"not null" json:"revision"`
	ConfigRevision    string                        `gorm:"type:varchar(71);not null" json:"config_revision"`
	MemoryEpoch       int64                         `gorm:"not null" json:"memory_epoch"`
	Evidence          datatypes.JSON                `gorm:"type:jsonb;not null" json:"evidence"`
	SourcePrincipalID string                        `gorm:"type:varchar(255);not null" json:"source_principal_id,omitempty"`
	LastOperationID   uuid.UUID                     `gorm:"type:uuid;not null" json:"last_operation_id"`
	SupersedesID      *uuid.UUID                    `gorm:"type:uuid" json:"supersedes_id,omitempty"`
	ValidFrom         *time.Time                    `json:"valid_from,omitempty"`
	ValidTo           *time.Time                    `json:"valid_to,omitempty"`
	CreatedAt         time.Time                     `gorm:"not null" json:"created_at"`
	UpdatedAt         time.Time                     `gorm:"not null" json:"updated_at"`
	UndoableUntil     *time.Time                    `gorm:"-" json:"undoable_until,omitempty"`
	UndoConsumed      bool                          `gorm:"-" json:"undo_consumed"`
}

func (DigitalHumanMemory) TableName() string { return "digital_human_memories" }

type MemoryEvent struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	OperationID       uuid.UUID      `gorm:"type:uuid;not null" json:"operation_id"`
	OrganizationID    uuid.UUID      `gorm:"type:uuid;not null" json:"organization_id"`
	WorkspaceID       uuid.UUID      `gorm:"type:uuid;not null" json:"workspace_id"`
	DigitalHumanID    uuid.UUID      `gorm:"type:uuid;not null" json:"digital_human_id"`
	MemoryID          *uuid.UUID     `gorm:"type:uuid" json:"memory_id,omitempty"`
	Action            string         `gorm:"type:varchar(24);not null" json:"action"`
	Result            string         `gorm:"type:varchar(24);not null" json:"result"`
	BeforeRevision    *int64         `json:"before_revision,omitempty"`
	AfterRevision     *int64         `json:"after_revision,omitempty"`
	BeforeSnapshot    datatypes.JSON `gorm:"type:jsonb" json:"before_snapshot,omitempty"`
	AfterSnapshot     datatypes.JSON `gorm:"type:jsonb" json:"after_snapshot,omitempty"`
	Evidence          datatypes.JSON `gorm:"type:jsonb;not null" json:"evidence"`
	SourcePrincipalID string         `gorm:"type:varchar(255);not null" json:"source_principal_id,omitempty"`
	RunID             *uuid.UUID     `gorm:"type:uuid" json:"run_id,omitempty"`
	ReasonCode        string         `gorm:"type:varchar(128);not null" json:"reason_code,omitempty"`
	CreatedAt         time.Time      `gorm:"not null" json:"created_at"`
}

func (MemoryEvent) TableName() string { return "digital_human_memory_events" }

type MemoryUndoRecord struct {
	OperationID        uuid.UUID      `gorm:"type:uuid;primaryKey"`
	OrganizationID     uuid.UUID      `gorm:"type:uuid;not null"`
	WorkspaceID        uuid.UUID      `gorm:"type:uuid;not null"`
	DigitalHumanID     uuid.UUID      `gorm:"type:uuid;not null"`
	MemoryID           uuid.UUID      `gorm:"type:uuid;not null"`
	PreviousSnapshot   datatypes.JSON `gorm:"type:jsonb"`
	SupersededSnapshot datatypes.JSON `gorm:"type:jsonb"`
	ResultingRevision  int64          `gorm:"not null"`
	ExpiresAt          time.Time      `gorm:"not null"`
	ConsumedAt         *time.Time
	CreatedAt          time.Time `gorm:"not null"`
}

func (MemoryUndoRecord) TableName() string { return "digital_human_memory_undo_records" }

type MemoryExtractionJobStatus string

const (
	MemoryExtractionJobPending   MemoryExtractionJobStatus = "pending"
	MemoryExtractionJobQueued    MemoryExtractionJobStatus = "queued"
	MemoryExtractionJobRunning   MemoryExtractionJobStatus = "running"
	MemoryExtractionJobSucceeded MemoryExtractionJobStatus = "succeeded"
	MemoryExtractionJobFailed    MemoryExtractionJobStatus = "failed"
	MemoryExtractionJobCanceled  MemoryExtractionJobStatus = "canceled"
)

type MemoryExtractionJob struct {
	ID               uuid.UUID                 `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID   uuid.UUID                 `gorm:"type:uuid;not null" json:"organization_id"`
	WorkspaceID      uuid.UUID                 `gorm:"type:uuid;not null" json:"workspace_id"`
	DigitalHumanID   uuid.UUID                 `gorm:"type:uuid;not null" json:"digital_human_id"`
	RunID            *uuid.UUID                `gorm:"type:uuid" json:"run_id,omitempty"`
	ConversationID   *uuid.UUID                `gorm:"type:uuid" json:"conversation_id,omitempty"`
	MessageID        *uuid.UUID                `gorm:"type:uuid" json:"message_id,omitempty"`
	ArtifactRefs     datatypes.JSON            `gorm:"type:jsonb;not null" json:"artifact_refs"`
	IdempotencyKey   string                    `gorm:"type:varchar(160);not null" json:"idempotency_key"`
	ConfigRevision   string                    `gorm:"type:varchar(71);not null" json:"config_revision"`
	MemoryEpoch      int64                     `gorm:"not null" json:"memory_epoch"`
	ExtractorVersion string                    `gorm:"type:varchar(64);not null" json:"extractor_version"`
	Status           MemoryExtractionJobStatus `gorm:"type:varchar(24);not null" json:"status"`
	AttemptCount     int                       `gorm:"not null" json:"attempt_count"`
	ErrorCode        string                    `gorm:"type:varchar(128);not null" json:"error_code,omitempty"`
	ScheduledAt      time.Time                 `gorm:"not null" json:"scheduled_at"`
	StartedAt        *time.Time                `json:"started_at,omitempty"`
	FinishedAt       *time.Time                `json:"finished_at,omitempty"`
	CreatedAt        time.Time                 `gorm:"not null" json:"created_at"`
	UpdatedAt        time.Time                 `gorm:"not null" json:"updated_at"`
}

func (MemoryExtractionJob) TableName() string { return "digital_human_memory_jobs" }

type ActionLedgerEntry struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID    uuid.UUID      `gorm:"type:uuid;not null" json:"organization_id"`
	WorkspaceID       uuid.UUID      `gorm:"type:uuid;not null" json:"workspace_id"`
	DigitalHumanID    uuid.UUID      `gorm:"type:uuid;not null" json:"digital_human_id"`
	RunID             *uuid.UUID     `gorm:"type:uuid" json:"run_id,omitempty"`
	ActionType        string         `gorm:"type:varchar(48);not null" json:"action_type"`
	Status            string         `gorm:"type:varchar(24);not null" json:"status"`
	MemoryOperationID *uuid.UUID     `gorm:"type:uuid" json:"memory_operation_id,omitempty"`
	MemoryID          *uuid.UUID     `gorm:"type:uuid" json:"memory_id,omitempty"`
	Detail            datatypes.JSON `gorm:"type:jsonb;not null" json:"detail"`
	CreatedAt         time.Time      `gorm:"not null" json:"created_at"`
}

func (ActionLedgerEntry) TableName() string { return "digital_human_action_ledger" }
