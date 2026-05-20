package model

import (
	"time"

	"gorm.io/gorm"
)

// AutomationTaskRun stores one scheduled or manual execution instance of a task.
type AutomationTaskRun struct {
	ID             string                  `gorm:"type:uuid;primaryKey" json:"id"`
	TaskID         string                  `gorm:"column:task_id;type:uuid;not null;index" json:"task_id"`
	TriggerSource  AutomationTriggerSource `gorm:"column:trigger_source;type:varchar(32);not null" json:"trigger_source"`
	ScheduledFor   time.Time               `gorm:"column:scheduled_for;not null" json:"scheduled_for"`
	StartedAt      *time.Time              `gorm:"column:started_at" json:"started_at,omitempty"`
	FinishedAt     *time.Time              `gorm:"column:finished_at" json:"finished_at,omitempty"`
	Status         AutomationTaskRunStatus `gorm:"type:varchar(32);not null" json:"status"`
	RuntimeContext map[string]interface{}  `gorm:"column:runtime_context;type:jsonb;serializer:json" json:"runtime_context,omitempty"`
	ErrorSummary   *string                 `gorm:"column:error_summary;type:text" json:"error_summary,omitempty"`
	CreatedAt      time.Time               `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt      time.Time               `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// TableName returns the database table name for AutomationTaskRun.
func (AutomationTaskRun) TableName() string {
	return "automation_task_runs"
}

// BeforeCreate initializes the primary key and timestamps before insert.
func (r *AutomationTaskRun) BeforeCreate(tx *gorm.DB) error {
	ensureModelID(&r.ID)
	ensureModelTimestamps(&r.CreatedAt, &r.UpdatedAt)
	return nil
}

// BeforeUpdate refreshes the updated_at timestamp before update.
func (r *AutomationTaskRun) BeforeUpdate(tx *gorm.DB) error {
	touchModelUpdatedAt(&r.UpdatedAt)
	return nil
}
