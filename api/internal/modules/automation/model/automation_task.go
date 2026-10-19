package model

import (
	"time"

	"gorm.io/gorm"
)

// AutomationTask stores the persisted definition of a scheduled automation task.
type AutomationTask struct {
	ID                      string                   `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID          string                   `gorm:"column:organization_id;type:uuid;not null;index" json:"organization_id"`
	WorkspaceID             string                   `gorm:"column:workspace_id;type:uuid;not null;index" json:"workspace_id"`
	Name                    string                   `gorm:"type:varchar(255);not null" json:"name"`
	Description             *string                  `gorm:"type:text" json:"description,omitempty"`
	TaskKind                AutomationTaskKind       `gorm:"column:task_kind;type:varchar(32);not null;default:'automation'" json:"task_kind"`
	SubjectType             *string                  `gorm:"column:subject_type;type:varchar(32)" json:"subject_type,omitempty"`
	SubjectID               *string                  `gorm:"column:subject_id;type:uuid" json:"subject_id,omitempty"`
	OwnerKind               *AutomationTaskOwnerKind `gorm:"column:owner_kind;type:varchar(32)" json:"owner_kind,omitempty"`
	OwnerSubjectKind        *string                  `gorm:"column:owner_subject_kind;type:varchar(32)" json:"owner_subject_kind,omitempty"`
	OwnerSubjectID          *string                  `gorm:"column:owner_subject_id;type:uuid" json:"owner_subject_id,omitempty"`
	OwnerConversationID     *string                  `gorm:"column:owner_conversation_id;type:uuid" json:"owner_conversation_id,omitempty"`
	OriginSurface           *string                  `gorm:"column:origin_surface;type:varchar(32)" json:"origin_surface,omitempty"`
	Visibility              *string                  `gorm:"column:visibility;type:varchar(16)" json:"visibility,omitempty"`
	ConfirmedByType         *string                  `gorm:"column:confirmed_by_type;type:varchar(32)" json:"confirmed_by_type,omitempty"`
	ConfirmedByRef          *string                  `gorm:"column:confirmed_by_ref;type:varchar(255)" json:"confirmed_by_ref,omitempty"`
	ConfirmedAt             *time.Time               `gorm:"column:confirmed_at" json:"confirmed_at,omitempty"`
	WorkStatus              *AutomationWorkStatus    `gorm:"column:work_status;type:varchar(32)" json:"work_status,omitempty"`
	Priority                *AutomationTaskPriority  `gorm:"column:priority;type:varchar(16)" json:"priority,omitempty"`
	Goal                    *string                  `gorm:"column:goal;type:text" json:"goal,omitempty"`
	SuccessCriteria         []string                 `gorm:"column:success_criteria;type:jsonb;serializer:json;not null;default:'[]'" json:"success_criteria,omitempty"`
	ExecutionTimeoutSeconds int                      `gorm:"column:execution_timeout_seconds;not null;default:300" json:"execution_timeout_seconds"`
	MaxAttempts             int                      `gorm:"column:max_attempts;not null;default:1" json:"max_attempts"`
	Revision                int64                    `gorm:"not null;default:1" json:"revision"`
	DeliveryConfig          map[string]interface{}   `gorm:"column:delivery_config;type:jsonb;serializer:json" json:"delivery_config,omitempty"`
	SourceOperationID       *string                  `gorm:"column:source_operation_id;type:uuid" json:"source_operation_id,omitempty"`
	ProposalExpiresAt       *time.Time               `gorm:"column:proposal_expires_at" json:"proposal_expires_at,omitempty"`
	Status                  AutomationTaskStatus     `gorm:"type:varchar(32);not null" json:"status"`
	TriggerType             AutomationTriggerType    `gorm:"column:trigger_type;type:varchar(32);not null;default:'schedule'" json:"trigger_type"`
	ScheduleType            AutomationScheduleType   `gorm:"column:schedule_type;type:varchar(32);not null" json:"schedule_type"`
	Timezone                string                   `gorm:"type:varchar(64);not null" json:"timezone"`
	ScheduleConfig          map[string]interface{}   `gorm:"column:schedule_config;type:jsonb;serializer:json;not null;default:'{}'" json:"schedule_config"`
	NextRunAt               *time.Time               `gorm:"column:next_run_at" json:"next_run_at,omitempty"`
	LastRunAt               *time.Time               `gorm:"column:last_run_at" json:"last_run_at,omitempty"`
	LastRunStatus           *string                  `gorm:"column:last_run_status;type:varchar(32)" json:"last_run_status,omitempty"`
	SourceType              AutomationSourceType     `gorm:"column:source_type;type:varchar(32);not null" json:"source_type"`
	SourceRef               *string                  `gorm:"column:source_ref;type:varchar(255)" json:"source_ref,omitempty"`
	SourceSnapshot          map[string]interface{}   `gorm:"column:source_snapshot;type:jsonb;serializer:json" json:"source_snapshot,omitempty"`
	CreatedBy               string                   `gorm:"column:created_by;type:uuid;not null" json:"created_by"`
	UpdatedBy               string                   `gorm:"column:updated_by;type:uuid;not null" json:"updated_by"`
	CreatedAt               time.Time                `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt               time.Time                `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
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
