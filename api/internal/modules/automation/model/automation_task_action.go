package model

import (
	"time"

	"gorm.io/gorm"
)

// AutomationTaskAction stores an action configuration bound to an automation task.
type AutomationTaskAction struct {
	ID          string                 `gorm:"type:uuid;primaryKey" json:"id"`
	TaskID      string                 `gorm:"column:task_id;type:uuid;not null;index" json:"task_id"`
	ActionType  AutomationActionType   `gorm:"column:action_type;type:varchar(32);not null" json:"action_type"`
	ActionOrder int                    `gorm:"column:action_order;not null;default:1" json:"action_order"`
	Enabled     bool                   `gorm:"not null;default:true" json:"enabled"`
	Config      map[string]interface{} `gorm:"type:jsonb;serializer:json;not null;default:'{}'" json:"config"`
	CreatedAt   time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt   time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// TableName returns the database table name for AutomationTaskAction.
func (AutomationTaskAction) TableName() string {
	return "automation_task_actions"
}

// BeforeCreate initializes the primary key and timestamps before insert.
func (a *AutomationTaskAction) BeforeCreate(tx *gorm.DB) error {
	ensureModelID(&a.ID)
	ensureModelTimestamps(&a.CreatedAt, &a.UpdatedAt)
	return nil
}

// BeforeUpdate refreshes the updated_at timestamp before update.
func (a *AutomationTaskAction) BeforeUpdate(tx *gorm.DB) error {
	touchModelUpdatedAt(&a.UpdatedAt)
	return nil
}
