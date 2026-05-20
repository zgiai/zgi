package model

import (
	"time"

	"gorm.io/gorm"
)

// AutomationTask stores the persisted definition of a scheduled automation task.
type AutomationTask struct {
	ID             string                 `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID string                 `gorm:"column:organization_id;type:uuid;not null;index" json:"organization_id"`
	WorkspaceID    string                 `gorm:"column:workspace_id;type:uuid;not null;index" json:"workspace_id"`
	Name           string                 `gorm:"type:varchar(255);not null" json:"name"`
	Description    *string                `gorm:"type:text" json:"description,omitempty"`
	Status         AutomationTaskStatus   `gorm:"type:varchar(32);not null" json:"status"`
	TriggerType    AutomationTriggerType  `gorm:"column:trigger_type;type:varchar(32);not null;default:'schedule'" json:"trigger_type"`
	ScheduleType   AutomationScheduleType `gorm:"column:schedule_type;type:varchar(32);not null" json:"schedule_type"`
	Timezone       string                 `gorm:"type:varchar(64);not null" json:"timezone"`
	ScheduleConfig map[string]interface{} `gorm:"column:schedule_config;type:jsonb;serializer:json;not null;default:'{}'" json:"schedule_config"`
	NextRunAt      *time.Time             `gorm:"column:next_run_at" json:"next_run_at,omitempty"`
	LastRunAt      *time.Time             `gorm:"column:last_run_at" json:"last_run_at,omitempty"`
	LastRunStatus  *string                `gorm:"column:last_run_status;type:varchar(32)" json:"last_run_status,omitempty"`
	SourceType     AutomationSourceType   `gorm:"column:source_type;type:varchar(32);not null" json:"source_type"`
	SourceRef      *string                `gorm:"column:source_ref;type:varchar(255)" json:"source_ref,omitempty"`
	SourceSnapshot map[string]interface{} `gorm:"column:source_snapshot;type:jsonb;serializer:json" json:"source_snapshot,omitempty"`
	CreatedBy      string                 `gorm:"column:created_by;type:uuid;not null" json:"created_by"`
	UpdatedBy      string                 `gorm:"column:updated_by;type:uuid;not null" json:"updated_by"`
	CreatedAt      time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt      time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// TableName returns the database table name for AutomationTask.
func (AutomationTask) TableName() string {
	return "automation_tasks"
}

// BeforeCreate initializes the primary key and timestamps before insert.
func (t *AutomationTask) BeforeCreate(tx *gorm.DB) error {
	ensureModelID(&t.ID)
	ensureModelTimestamps(&t.CreatedAt, &t.UpdatedAt)
	return nil
}

// BeforeUpdate refreshes the updated_at timestamp before update.
func (t *AutomationTask) BeforeUpdate(tx *gorm.DB) error {
	touchModelUpdatedAt(&t.UpdatedAt)
	return nil
}
