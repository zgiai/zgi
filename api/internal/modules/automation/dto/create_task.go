package dto

import automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"

// CreateTaskActionRequest defines one action to create alongside a task.
type CreateTaskActionRequest struct {
	ActionType  automationmodel.AutomationActionType `json:"action_type"`
	ActionOrder int                                  `json:"action_order,omitempty"`
	Enabled     *bool                                `json:"enabled,omitempty"`
	Config      map[string]interface{}               `json:"config"`
}

// CreateTaskRequest captures the data needed to persist a new automation task.
type CreateTaskRequest struct {
	TaskScope
	Name           string                                 `json:"name"`
	Description    *string                                `json:"description,omitempty"`
	ScheduleType   automationmodel.AutomationScheduleType `json:"schedule_type"`
	Timezone       string                                 `json:"timezone"`
	ScheduleConfig map[string]interface{}                 `json:"schedule_config"`
	SourceType     automationmodel.AutomationSourceType   `json:"source_type"`
	SourceRef      *string                                `json:"source_ref,omitempty"`
	SourceSnapshot map[string]interface{}                 `json:"source_snapshot,omitempty"`
	CreatedBy      string                                 `json:"created_by"`
	UpdatedBy      string                                 `json:"updated_by"`
	Actions        []CreateTaskActionRequest              `json:"actions"`
}

// CreateTaskResult returns the persisted task and its actions.
type CreateTaskResult struct {
	Task    *automationmodel.AutomationTask         `json:"task"`
	Actions []*automationmodel.AutomationTaskAction `json:"actions"`
}
