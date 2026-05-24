package createscheduledtask

import (
	"fmt"
	"strings"

	automationdto "github.com/zgiai/zgi/api/internal/modules/automation/dto"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	notificationsms "github.com/zgiai/zgi/api/internal/modules/notification/sms"
)

func (n *Node) buildCreateTaskRequest() (automationdto.CreateTaskRequest, error) {
	taskName := strings.TrimSpace(n.resolveTemplateVariables(n.nodeData.Task.Name))
	if taskName == "" {
		return automationdto.CreateTaskRequest{}, fmt.Errorf("task.name is required")
	}

	scheduleType, timezone, scheduleConfig, err := n.compileSchedule()
	if err != nil {
		return automationdto.CreateTaskRequest{}, err
	}

	actions, err := n.compileActions()
	if err != nil {
		return automationdto.CreateTaskRequest{}, err
	}

	description := strings.TrimSpace(n.resolveTemplateVariables(n.nodeData.Task.Description))
	workspaceID := n.resolveWorkspaceID()
	organizationID := n.resolveOrganizationID()
	sourceRef := defaultSourceRef

	sourceSnapshot := map[string]interface{}{
		"workflow_id":         n.WorkflowID,
		"app_id":              n.APPID,
		"node_id":             n.NodeID,
		"invoke_from":         n.InvokeFrom,
		"organization_id":     organizationID,
		"workspace_id":        workspaceID,
		"schedule_type":       scheduleType,
		"created_from":        "workflow_node",
		"workflow_run_id":     n.lookupSystemString("workflow_run_id"),
		"workflow_call_depth": n.WorkflowCallDepth,
	}

	return automationdto.CreateTaskRequest{
		TaskScope: automationdto.TaskScope{
			OrganizationID: organizationID,
			WorkspaceID:    workspaceID,
		},
		Name:           taskName,
		Description:    stringPtr(description),
		ScheduleType:   scheduleType,
		Timezone:       timezone,
		ScheduleConfig: scheduleConfig,
		SourceType:     automationmodel.AutomationSourceTypeWorkflow,
		SourceRef:      &sourceRef,
		SourceSnapshot: sourceSnapshot,
		CreatedBy:      n.UserID,
		UpdatedBy:      n.UserID,
		Actions:        actions,
	}, nil
}

func (n *Node) compileSchedule() (automationmodel.AutomationScheduleType, string, map[string]interface{}, error) {
	schedule := n.nodeData.Task.Schedule

	switch schedule.Type {
	case automationmodel.AutomationScheduleTypeOnce:
		runAt, err := n.resolveOnceRunAt(schedule.Once)
		if err != nil {
			return "", "", nil, err
		}
		return schedule.Type, schedule.Timezone, map[string]interface{}{
			"run_at": runAt,
		}, nil
	case automationmodel.AutomationScheduleTypeCron:
		cronExpr := strings.TrimSpace(n.resolveTemplateVariables(schedule.Cron.Expr))
		if cronExpr == "" {
			return "", "", nil, fmt.Errorf("task.schedule.cron.expr is required")
		}
		return schedule.Type, schedule.Timezone, map[string]interface{}{
			"cron_expr": cronExpr,
		}, nil
	default:
		return "", "", nil, fmt.Errorf("task.schedule.type %q is not supported", schedule.Type)
	}
}

func (n *Node) compileActions() ([]automationdto.CreateTaskActionRequest, error) {
	actions := make([]automationdto.CreateTaskActionRequest, 0, len(n.nodeData.Task.Actions))

	for index, action := range n.nodeData.Task.Actions {
		if !action.IsEnabled() {
			continue
		}
		compiled, err := n.compileAction(index, action, len(actions)+1)
		if err != nil {
			return nil, err
		}
		actions = append(actions, compiled)
	}

	if len(actions) == 0 {
		return nil, fmt.Errorf("task.actions must contain at least one enabled action")
	}

	return actions, nil
}

func (n *Node) compileAction(index int, action TaskActionData, actionOrder int) (automationdto.CreateTaskActionRequest, error) {
	switch action.ActionType {
	case automationmodel.AutomationActionTypeSendNotification:
		return n.compileNotificationAction(index, action, actionOrder)
	case automationmodel.AutomationActionTypeRunWorkflow:
		return n.compileRunWorkflowAction(action, actionOrder), nil
	default:
		return automationdto.CreateTaskActionRequest{}, fmt.Errorf("task.actions[%d].action_type %q is not supported", index, action.ActionType)
	}
}

func (n *Node) compileNotificationAction(index int, action TaskActionData, actionOrder int) (automationdto.CreateTaskActionRequest, error) {
	recipients := make([]string, 0, len(action.Notification.Recipients))
	for _, recipient := range action.Notification.Recipients {
		resolved := strings.TrimSpace(n.resolveTemplateVariables(recipient))
		if resolved != "" {
			recipients = append(recipients, resolved)
		}
	}
	if len(recipients) == 0 {
		return automationdto.CreateTaskActionRequest{}, fmt.Errorf("task.actions[%d].notification.recipients must resolve to at least one recipient", index)
	}

	if action.ChannelType == automationmodel.NotificationChannelTypeSMS {
		return n.compileSMSNotificationAction(index, action, actionOrder, recipients)
	}

	return automationdto.CreateTaskActionRequest{
		ActionType:  action.ActionType,
		ActionOrder: actionOrder,
		Enabled:     boolPtr(true),
		Config: map[string]interface{}{
			"channel_type": action.ChannelType,
			"to":           recipients,
			"subject":      strings.TrimSpace(n.resolveTemplateVariables(action.Notification.Subject)),
			"body":         n.resolveTemplateVariables(action.Notification.Body),
			"body_type":    resolveBodyType(action.Notification.BodyType),
		},
	}, nil
}

func (n *Node) compileSMSNotificationAction(index int, action TaskActionData, actionOrder int, recipients []string) (automationdto.CreateTaskActionRequest, error) {
	template := strings.TrimSpace(action.Notification.Template)
	templateParams := n.resolveNotificationTemplateParams(action.Notification)
	if err := n.validateSMSNotificationTemplateParams(index, template, templateParams); err != nil {
		return automationdto.CreateTaskActionRequest{}, err
	}

	return automationdto.CreateTaskActionRequest{
		ActionType:  action.ActionType,
		ActionOrder: actionOrder,
		Enabled:     boolPtr(true),
		Config: map[string]interface{}{
			"channel_type":    action.ChannelType,
			"to":              recipients,
			"template":        template,
			"template_params": templateParams,
		},
	}, nil
}

func (n *Node) resolveNotificationTemplateParams(notification NotificationData) map[string]string {
	resolved := make(map[string]string, len(notification.TemplateParams))
	for key, value := range notification.TemplateParams {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(n.resolveTemplateVariables(value))
		if key != "" && value != "" {
			resolved[key] = value
		}
	}
	return notificationsms.NormalizeTemplateParams(resolved)
}

func (n *Node) compileRunWorkflowAction(action TaskActionData, actionOrder int) automationdto.CreateTaskActionRequest {
	config := map[string]interface{}{
		"workflow_ref": map[string]interface{}{
			"agent_id":         strings.TrimSpace(n.resolveTemplateVariables(action.Workflow.AgentID)),
			"workflow_id":      strings.TrimSpace(n.resolveTemplateVariables(action.Workflow.WorkflowID)),
			"version_strategy": strings.TrimSpace(action.Workflow.VersionStrategy),
			"version_uuid":     strings.TrimSpace(n.resolveTemplateVariables(action.Workflow.VersionUUID)),
		},
		"inputs": n.resolveWorkflowInputs(action.Workflow.Inputs),
	}
	if action.Workflow.TimeoutSeconds > 0 {
		config["execution"] = map[string]interface{}{
			"timeout_seconds": action.Workflow.TimeoutSeconds,
			"created_by_role": "account",
		}
	}

	return automationdto.CreateTaskActionRequest{
		ActionType:  automationmodel.AutomationActionTypeRunWorkflow,
		ActionOrder: actionOrder,
		Enabled:     boolPtr(true),
		Config:      config,
	}
}

func (n *Node) resolveWorkflowInputs(inputs map[string]interface{}) map[string]interface{} {
	resolved := make(map[string]interface{}, len(inputs))
	for key, value := range inputs {
		resolved[key] = n.resolveWorkflowInputValue(value)
	}
	return resolved
}

func (n *Node) resolveWorkflowInputValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case string:
		return n.resolveTemplateVariables(typed)
	case map[string]interface{}:
		return n.resolveWorkflowInputs(typed)
	case []interface{}:
		values := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			values = append(values, n.resolveWorkflowInputValue(item))
		}
		return values
	default:
		return value
	}
}

func resolveBodyType(bodyType string) string {
	bodyType = strings.TrimSpace(bodyType)
	if bodyType == "" {
		return defaultEmailBodyType
	}
	return bodyType
}

func boolPtr(value bool) *bool {
	return &value
}

func stringPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}
