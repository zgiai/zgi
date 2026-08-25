package model

import (
	"time"

	"gorm.io/gorm"
)

// AutomationTaskActivity stores one replay-safe, operator-visible mutation of
// a task definition or its work lifecycle.
type AutomationTaskActivity struct {
	ID          string                      `gorm:"type:uuid;primaryKey" json:"id"`
	TaskID      string                      `gorm:"column:task_id;type:uuid;not null;index" json:"task_id"`
	OperationID string                      `gorm:"column:operation_id;type:uuid;not null;uniqueIndex" json:"operation_id"`
	ActorType   AutomationTaskActivityActor `gorm:"column:actor_type;type:varchar(32);not null" json:"actor_type"`
	ActorRef    string                      `gorm:"column:actor_ref;type:varchar(255);not null" json:"actor_ref"`
	Action      string                      `gorm:"type:varchar(64);not null" json:"action"`
	Summary     string                      `gorm:"type:text;not null" json:"summary"`
	Changes     map[string]interface{}      `gorm:"type:jsonb;serializer:json;not null;default:'{}'" json:"changes,omitempty"`
	SourceRunID *string                     `gorm:"column:source_run_id;type:uuid" json:"source_run_id,omitempty"`
	CreatedAt   time.Time                   `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
}

// TableName returns the task activity table name.
func (AutomationTaskActivity) TableName() string { return "automation_task_activities" }

// BeforeCreate initializes the activity identity and timestamp.
func (a *AutomationTaskActivity) BeforeCreate(tx *gorm.DB) error {
	ensureModelID(&a.ID)
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}
	return nil
}
