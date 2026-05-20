package dto

import automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"

// UpdateTaskActionRequest defines one action to persist during task updates.
type UpdateTaskActionRequest struct {
	ActionType  automationmodel.AutomationActionType `json:"action_type"`
	ActionOrder int                                  `json:"action_order,omitempty"`
	Enabled     *bool                                `json:"enabled,omitempty"`
	Config      map[string]interface{}               `json:"config"`
}

// UpdateTaskRequest captures editable task definition fields for MVP updates.
type UpdateTaskRequest struct {
	Name           string                                 `json:"name"`
	Description    *string                                `json:"description,omitempty"`
	ScheduleType   automationmodel.AutomationScheduleType `json:"schedule_type"`
	Timezone       string                                 `json:"timezone"`
	ScheduleConfig map[string]interface{}                 `json:"schedule_config"`
	Actions        []UpdateTaskActionRequest              `json:"actions"`
	UpdatedBy      string                                 `json:"updated_by"`
}
