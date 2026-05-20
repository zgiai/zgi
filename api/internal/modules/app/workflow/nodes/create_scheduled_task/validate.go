package createscheduledtask

import (
	"fmt"
	"strings"

	"github.com/robfig/cron/v3"
	automationmodel "github.com/zgiai/ginext/internal/modules/automation/model"
	notificationsms "github.com/zgiai/ginext/internal/modules/notification/sms"
)

func validateNodeData(nodeData NodeData) error {
	if strings.TrimSpace(nodeData.Task.Name) == "" {
		return fmt.Errorf("task.name is required")
	}

	if err := validateSchedule(nodeData.Task.Schedule); err != nil {
		return err
	}

	if len(nodeData.Task.Actions) == 0 {
		return fmt.Errorf("task.actions must contain at least one action")
	}

	enabledActionCount := 0
	for index, action := range nodeData.Task.Actions {
		if !action.IsEnabled() {
			continue
		}
		enabledActionCount++
		if err := validateAction(index, action); err != nil {
			return err
		}
	}
	if enabledActionCount == 0 {
		return fmt.Errorf("task.actions must contain at least one enabled action")
	}

	return nil
}

func validateSchedule(schedule TaskSchedule) error {
	switch schedule.Type {
	case automationmodel.AutomationScheduleTypeOnce:
		if strings.TrimSpace(schedule.Once.RunAt) == "" {
			return fmt.Errorf("task.schedule.once.run_at is required")
		}
		switch schedule.Once.InputMode {
		case OnceInputModeFixed, OnceInputModeVariable:
			return nil
		default:
			return fmt.Errorf("task.schedule.once.input_mode must be one of %q or %q", OnceInputModeFixed, OnceInputModeVariable)
		}
	case automationmodel.AutomationScheduleTypeCron:
		cronExpr := strings.TrimSpace(schedule.Cron.Expr)
		if cronExpr == "" {
			return fmt.Errorf("task.schedule.cron.expr is required")
		}
		if _, err := cron.ParseStandard(cronExpr); err != nil {
			return fmt.Errorf("task.schedule.cron.expr must be a valid standard cron expression: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("task.schedule.type %q is not supported", schedule.Type)
	}
}

func validateAction(index int, action TaskActionData) error {
	switch action.ActionType {
	case automationmodel.AutomationActionTypeSendNotification:
		return validateNotificationAction(index, action)
	case automationmodel.AutomationActionTypeRunWorkflow:
		return validateRunWorkflowAction(index, action.Workflow)
	default:
		return fmt.Errorf("task.actions[%d].action_type %q is not supported", index, action.ActionType)
	}
}

func validateNotificationAction(index int, action TaskActionData) error {
	switch action.ChannelType {
	case automationmodel.NotificationChannelTypeEmail:
	case automationmodel.NotificationChannelTypeSMS:
	default:
		return fmt.Errorf("task.actions[%d].channel_type %q is not supported", index, action.ChannelType)
	}

	for _, recipient := range action.Notification.Recipients {
		if strings.TrimSpace(recipient) != "" {
			return validateNotificationContent(index, action)
		}
	}

	return fmt.Errorf("task.actions[%d].notification.recipients must contain at least one recipient", index)
}

func validateNotificationContent(index int, action TaskActionData) error {
	if action.ChannelType != automationmodel.NotificationChannelTypeSMS {
		return nil
	}

	template := strings.TrimSpace(action.Notification.Template)
	if template != "" && template != notificationsms.TemplatePendingActionNotification {
		return fmt.Errorf("task.actions[%d].notification.template %q is not supported", index, action.Notification.Template)
	}
	if strings.TrimSpace(action.Notification.NotificationTitle) == "" {
		return fmt.Errorf("task.actions[%d].notification.notification_title is required", index)
	}
	if strings.TrimSpace(action.Notification.LinkCode) == "" {
		return fmt.Errorf("task.actions[%d].notification.link_code is required", index)
	}
	return nil
}

func validateRunWorkflowAction(index int, workflow WorkflowActionData) error {
	if strings.TrimSpace(workflow.AgentID) == "" {
		return fmt.Errorf("task.actions[%d].workflow.agent_id is required", index)
	}
	switch strings.TrimSpace(workflow.VersionStrategy) {
	case "latest_published":
		return nil
	case "pinned":
		if strings.TrimSpace(workflow.VersionUUID) == "" {
			return fmt.Errorf("task.actions[%d].workflow.version_uuid is required when version_strategy is pinned", index)
		}
		return nil
	default:
		return fmt.Errorf("task.actions[%d].workflow.version_strategy %q is not supported", index, workflow.VersionStrategy)
	}
}
