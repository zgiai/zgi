package createscheduledtask

import (
	"fmt"
	"strings"

	"github.com/robfig/cron/v3"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
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
	if template == "" {
		return fmt.Errorf("task.actions[%d].notification.template is required", index)
	}

	return nil
}

func (n *Node) validateSMSNotificationActions() error {
	for index, action := range n.nodeData.Task.Actions {
		if !action.IsEnabled() ||
			action.ActionType != automationmodel.AutomationActionTypeSendNotification ||
			action.ChannelType != automationmodel.NotificationChannelTypeSMS {
			continue
		}
		if err := n.validateSMSNotificationTemplateParams(index, action.Notification.Template, action.Notification.TemplateParams); err != nil {
			return err
		}
	}
	return nil
}

func (n *Node) validateSMSNotificationTemplateParams(index int, template string, params map[string]string) error {
	if n.notificationSMSService == nil {
		return fmt.Errorf("notification sms service is required for task.actions[%d]", index)
	}
	if err := n.notificationSMSService.ValidateTemplateParams(template, params); err != nil {
		return fmt.Errorf("task.actions[%d].notification.template_params: %w", index, err)
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
