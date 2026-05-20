package pause

import "time"

type RunPause struct {
	ID             string     `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID       string     `gorm:"type:uuid;not null;index" json:"tenant_id"`
	AppID          string     `gorm:"type:uuid;not null;index" json:"app_id"`
	WorkflowRunID  string     `gorm:"type:varchar(255);not null;index" json:"workflow_run_id"`
	NodeID         string     `gorm:"type:varchar(255);not null" json:"node_id"`
	Reason         string     `gorm:"type:varchar(64);not null" json:"reason"`
	ConversationID *string    `gorm:"type:uuid;index" json:"conversation_id,omitempty"`
	StateJSON      string     `gorm:"type:text;not null" json:"state_json"`
	CreatedAt      time.Time  `json:"created_at"`
	ResumedAt      *time.Time `json:"resumed_at"`
}

func (RunPause) TableName() string {
	return "workflow_run_pauses"
}

type RunPauseReason struct {
	ID        string    `gorm:"type:uuid;primaryKey" json:"id"`
	PauseID   string    `gorm:"type:uuid;not null;index" json:"pause_id"`
	Type      string    `gorm:"type:varchar(64);not null" json:"type"`
	NodeID    string    `gorm:"type:varchar(255);not null;default:''" json:"node_id"`
	FormID    string    `gorm:"type:varchar(255);not null;default:''" json:"form_id"`
	CreatedAt time.Time `json:"created_at"`
}

func (RunPauseReason) TableName() string {
	return "workflow_run_pause_reasons"
}

type RunEvent struct {
	ID            string    `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID      string    `gorm:"type:uuid;not null;index" json:"tenant_id"`
	AppID         string    `gorm:"type:uuid;not null;index" json:"app_id"`
	WorkflowRunID string    `gorm:"type:varchar(255);not null;index" json:"workflow_run_id"`
	Sequence      int       `gorm:"not null;index" json:"sequence"`
	EventType     string    `gorm:"type:varchar(100);not null" json:"event_type"`
	EventData     string    `gorm:"type:text;not null" json:"event_data"`
	CreatedAt     time.Time `json:"created_at"`
}

func (RunEvent) TableName() string {
	return "workflow_run_events"
}
