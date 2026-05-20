package model

import (
	"time"

	"gorm.io/gorm"
)

// AutomationActionRun stores the execution result of one action inside a task run.
type AutomationActionRun struct {
	ID              string                    `gorm:"type:uuid;primaryKey" json:"id"`
	TaskRunID       string                    `gorm:"column:task_run_id;type:uuid;not null;index" json:"task_run_id"`
	TaskActionID    string                    `gorm:"column:task_action_id;type:uuid;not null;index" json:"task_action_id"`
	ActionType      AutomationActionType      `gorm:"column:action_type;type:varchar(32);not null" json:"action_type"`
	ChannelType     *NotificationChannelType  `gorm:"column:channel_type;type:varchar(32)" json:"channel_type,omitempty"`
	RequestPayload  map[string]interface{}    `gorm:"column:request_payload;type:jsonb;serializer:json" json:"request_payload,omitempty"`
	ResponsePayload map[string]interface{}    `gorm:"column:response_payload;type:jsonb;serializer:json" json:"response_payload,omitempty"`
	ErrorMessage    *string                   `gorm:"column:error_message;type:text" json:"error_message,omitempty"`
	Status          AutomationActionRunStatus `gorm:"type:varchar(32);not null" json:"status"`
	StartedAt       *time.Time                `gorm:"column:started_at" json:"started_at,omitempty"`
	FinishedAt      *time.Time                `gorm:"column:finished_at" json:"finished_at,omitempty"`
	CreatedAt       time.Time                 `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt       time.Time                 `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// TableName returns the database table name for AutomationActionRun.
func (AutomationActionRun) TableName() string {
	return "automation_action_runs"
}

// BeforeCreate initializes the primary key and timestamps before insert.
func (r *AutomationActionRun) BeforeCreate(tx *gorm.DB) error {
	ensureModelID(&r.ID)
	ensureModelTimestamps(&r.CreatedAt, &r.UpdatedAt)
	return nil
}

// BeforeUpdate refreshes the updated_at timestamp before update.
func (r *AutomationActionRun) BeforeUpdate(tx *gorm.DB) error {
	touchModelUpdatedAt(&r.UpdatedAt)
	return nil
}
