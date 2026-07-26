package pause

import "time"

type RunPause struct {
	ID                string     `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID          string     `gorm:"type:uuid;not null;index" json:"tenant_id"`
	AppID             string     `gorm:"type:uuid;not null;index" json:"app_id"`
	WorkflowRunID     string     `gorm:"type:varchar(255);not null;index" json:"workflow_run_id"`
	NodeID            string     `gorm:"type:varchar(255);not null" json:"node_id"`
	Reason            string     `gorm:"type:varchar(64);not null" json:"reason"`
	ConversationID    *string    `gorm:"type:uuid;index" json:"conversation_id,omitempty"`
	StateJSON         string     `gorm:"type:text;not null" json:"state_json"`
	CreatedAt         time.Time  `json:"created_at"`
	ResumedAt         *time.Time `json:"resumed_at"`
	Generation        int64      `gorm:"not null;default:1" json:"generation"`
	Status            string     `gorm:"type:varchar(32);not null;default:'paused';index" json:"status"`
	Revision          int64      `gorm:"not null;default:0" json:"revision"`
	ResumeExecutionID *string    `gorm:"type:uuid" json:"resume_execution_id,omitempty"`
	LeaseExpiresAt    *time.Time `gorm:"type:timestamptz" json:"lease_expires_at,omitempty"`
}

func (RunPause) TableName() string {
	return "workflow_run_pauses"
}

type RunPauseReason struct {
	ID                string     `gorm:"type:uuid;primaryKey" json:"id"`
	PauseID           string     `gorm:"type:uuid;not null;index" json:"pause_id"`
	Type              string     `gorm:"type:varchar(64);not null" json:"type"`
	NodeID            string     `gorm:"type:varchar(255);not null;default:''" json:"node_id"`
	FormID            string     `gorm:"type:varchar(255);not null;default:''" json:"form_id"`
	Status            string     `gorm:"type:varchar(32);not null;default:'pending';index" json:"status"`
	Revision          int64      `gorm:"not null;default:0" json:"revision"`
	SubmissionEventID *string    `gorm:"type:uuid" json:"submission_event_id,omitempty"`
	CompletedAt       *time.Time `gorm:"type:timestamptz" json:"completed_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

func (RunPauseReason) TableName() string {
	return "workflow_run_pause_reasons"
}

type RunEvent struct {
	ID              string    `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID        string    `gorm:"type:uuid;not null;index" json:"tenant_id"`
	AppID           string    `gorm:"type:uuid;not null;index" json:"app_id"`
	WorkflowRunID   string    `gorm:"type:varchar(255);not null;index" json:"workflow_run_id"`
	Sequence        int       `gorm:"not null;index" json:"sequence"`
	EventType       string    `gorm:"type:varchar(100);not null" json:"event_type"`
	EventData       string    `gorm:"type:text;not null" json:"event_data"`
	CreatedAt       time.Time `json:"created_at"`
	SchemaVersion   int       `gorm:"not null;default:1" json:"schema_version"`
	Category        string    `gorm:"type:varchar(32);not null;default:'execution';index" json:"category"`
	ExecutionID     *string   `gorm:"type:uuid;index" json:"execution_id,omitempty"`
	PauseID         *string   `gorm:"type:uuid;index" json:"pause_id,omitempty"`
	PauseGeneration *int64    `gorm:"type:bigint" json:"pause_generation,omitempty"`
	IdempotencyKey  *string   `gorm:"type:varchar(255)" json:"idempotency_key,omitempty"`
	OccurredAt      time.Time `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP" json:"occurred_at"`
}

func (RunEvent) TableName() string {
	return "workflow_run_events"
}

type RuntimeOutbox struct {
	ID             string     `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID       string     `gorm:"type:uuid;not null;index" json:"tenant_id"`
	WorkflowRunID  string     `gorm:"type:uuid;not null;index" json:"workflow_run_id"`
	PauseID        *string    `gorm:"type:uuid;index" json:"pause_id,omitempty"`
	Kind           string     `gorm:"type:varchar(64);not null;index" json:"kind"`
	IdempotencyKey string     `gorm:"type:varchar(255);not null;uniqueIndex" json:"idempotency_key"`
	PayloadJSON    string     `gorm:"type:text;not null" json:"payload_json"`
	Status         string     `gorm:"type:varchar(32);not null;default:'pending';index" json:"status"`
	Attempts       int        `gorm:"not null;default:0" json:"attempts"`
	NextAttemptAt  time.Time  `gorm:"type:timestamptz;not null;index" json:"next_attempt_at"`
	PublishedAt    *time.Time `gorm:"type:timestamptz" json:"published_at,omitempty"`
	LastError      *string    `gorm:"type:text" json:"last_error,omitempty"`
	CreatedAt      time.Time  `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (RuntimeOutbox) TableName() string {
	return "workflow_runtime_outbox"
}
