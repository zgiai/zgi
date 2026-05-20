package definition

import (
	"fmt"

	automationdto "github.com/zgiai/zgi/api/internal/modules/automation/dto"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
)

func validateCreateTaskRequest(req automationdto.CreateTaskRequest) error {
	if req.OrganizationID == "" {
		return fmt.Errorf("organization_id is required")
	}
	if req.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if req.Name == "" {
		return fmt.Errorf("task name is required")
	}
	if req.CreatedBy == "" || req.UpdatedBy == "" {
		return fmt.Errorf("created_by and updated_by are required")
	}
	if req.SourceType == "" {
		return fmt.Errorf("source_type is required")
	}
	if req.ScheduleType == "" {
		return fmt.Errorf("schedule_type is required")
	}
	if req.ScheduleType == automationmodel.AutomationScheduleTypeCron && req.Timezone == "" {
		return fmt.Errorf("timezone is required for cron schedule")
	}
	if err := validateSchedule(req.ScheduleType, req.ScheduleConfig); err != nil {
		return err
	}
	if len(req.Actions) == 0 {
		return fmt.Errorf("at least one automation action is required")
	}

	for _, action := range req.Actions {
		if err := validateAutomationActionConfig(action.ActionType, action.Config); err != nil {
			return err
		}
	}

	return nil
}

func validateUpdateTaskRequest(req automationdto.UpdateTaskRequest) error {
	if req.Name == "" {
		return fmt.Errorf("task name is required")
	}
	if req.UpdatedBy == "" {
		return fmt.Errorf("updated_by is required")
	}
	if req.ScheduleType == "" {
		return fmt.Errorf("schedule_type is required")
	}
	if req.ScheduleType == automationmodel.AutomationScheduleTypeCron && req.Timezone == "" {
		return fmt.Errorf("timezone is required for cron schedule")
	}
	if err := validateSchedule(req.ScheduleType, req.ScheduleConfig); err != nil {
		return err
	}
	if len(req.Actions) == 0 {
		return fmt.Errorf("at least one automation action is required")
	}

	for _, action := range req.Actions {
		if err := validateAutomationActionConfig(action.ActionType, action.Config); err != nil {
			return err
		}
	}

	return nil
}

func validateSchedule(scheduleType automationmodel.AutomationScheduleType, config map[string]interface{}) error {
	switch scheduleType {
	case automationmodel.AutomationScheduleTypeOnce:
		_, err := parseOnceRunAt(config)
		return err
	case automationmodel.AutomationScheduleTypeCron:
		_, err := parseCronSchedule(config)
		return err
	default:
		return fmt.Errorf("unsupported automation schedule type: %s", scheduleType)
	}
}

func validateAutomationActionConfig(actionType automationmodel.AutomationActionType, config map[string]interface{}) error {
	if config == nil {
		return fmt.Errorf("automation action config is required")
	}

	switch actionType {
	case automationmodel.AutomationActionTypeSendNotification:
		return nil
	case automationmodel.AutomationActionTypeRunWorkflow:
		return automationaction.ValidateRunWorkflowConfig(config)
	default:
		return fmt.Errorf("unsupported automation action type: %s", actionType)
	}
}
