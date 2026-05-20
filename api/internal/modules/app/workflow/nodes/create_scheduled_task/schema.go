package createscheduledtask

import (
	"encoding/json"
	"fmt"
	"strings"

	automationmodel "github.com/zgiai/ginext/internal/modules/automation/model"
	automationaction "github.com/zgiai/ginext/internal/modules/automation/service/action"
)

const (
	defaultWorkflowScheduleTimezone = "Asia/Shanghai"
	defaultEmailBodyType            = "text/html"
	defaultSourceRef                = "workflow.node.create-scheduled-task"
)

type OnceInputMode string

const (
	OnceInputModeFixed    OnceInputMode = "fixed"
	OnceInputModeVariable OnceInputMode = "variable"
)

type NodeData struct {
	Type          string   `json:"type"`
	Title         string   `json:"title"`
	Desc          string   `json:"desc"`
	Task          TaskData `json:"task"`
	IsInLoop      bool     `json:"isInLoop"`
	IsInIteration bool     `json:"isInIteration"`
}

type TaskData struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Schedule    TaskSchedule     `json:"schedule"`
	Actions     []TaskActionData `json:"actions"`
}

type TaskSchedule struct {
	Type     automationmodel.AutomationScheduleType `json:"type"`
	Timezone string                                 `json:"timezone"`
	Once     OnceScheduleData                       `json:"once"`
	Cron     CronScheduleData                       `json:"cron"`
}

type OnceScheduleData struct {
	InputMode OnceInputMode `json:"input_mode"`
	RunAt     string        `json:"run_at"`
}

type CronScheduleData struct {
	Expr string `json:"expr"`
}

type TaskActionData struct {
	ClientID     string                                  `json:"client_id"`
	ActionType   automationmodel.AutomationActionType    `json:"action_type"`
	Enabled      *bool                                   `json:"enabled"`
	ChannelType  automationmodel.NotificationChannelType `json:"channel_type"`
	Notification NotificationData                        `json:"notification"`
	Workflow     WorkflowActionData                      `json:"workflow"`
}

type NotificationData struct {
	Recipients        []string `json:"recipients"`
	Subject           string   `json:"subject"`
	Body              string   `json:"body"`
	BodyType          string   `json:"body_type"`
	Template          string   `json:"template"`
	NotificationTitle string   `json:"notification_title"`
	LinkCode          string   `json:"link_code"`
}

type WorkflowActionData struct {
	AgentID         string                 `json:"agent_id"`
	WorkflowID      string                 `json:"workflow_id,omitempty"`
	VersionStrategy string                 `json:"version_strategy"`
	VersionUUID     string                 `json:"version_uuid,omitempty"`
	Inputs          map[string]interface{} `json:"inputs"`
	TimeoutSeconds  int                    `json:"timeout_seconds,omitempty"`
}

func parseNodeDataFromConfig(config map[string]any) (NodeData, string, error) {
	rawNodeID, ok := config["id"]
	if !ok {
		return NodeData{}, "", fmt.Errorf("node ID is required")
	}

	nodeID, ok := rawNodeID.(string)
	if !ok || strings.TrimSpace(nodeID) == "" {
		return NodeData{}, "", fmt.Errorf("node ID must be a non-empty string")
	}

	rawData, ok := config["data"]
	if !ok {
		return NodeData{}, "", fmt.Errorf("node data is required")
	}

	payload, err := json.Marshal(rawData)
	if err != nil {
		return NodeData{}, "", fmt.Errorf("failed to marshal node data: %w", err)
	}

	var nodeData NodeData
	if err := json.Unmarshal(payload, &nodeData); err != nil {
		return NodeData{}, "", fmt.Errorf("failed to unmarshal node data: %w", err)
	}

	nodeData.applyDefaults()
	if err := validateNodeData(nodeData); err != nil {
		return NodeData{}, "", err
	}

	return nodeData, nodeID, nil
}

func (d *NodeData) applyDefaults() {
	if strings.TrimSpace(d.Task.Schedule.Timezone) == "" {
		d.Task.Schedule.Timezone = defaultWorkflowScheduleTimezone
	}

	for index := range d.Task.Actions {
		if d.Task.Actions[index].Enabled == nil {
			enabled := true
			d.Task.Actions[index].Enabled = &enabled
		}
		if strings.TrimSpace(d.Task.Actions[index].Notification.BodyType) == "" {
			d.Task.Actions[index].Notification.BodyType = defaultEmailBodyType
		}
		if strings.TrimSpace(d.Task.Actions[index].Workflow.VersionStrategy) == "" {
			d.Task.Actions[index].Workflow.VersionStrategy = automationaction.WorkflowVersionStrategyLatestPublished
		}
	}
}

func (a TaskActionData) IsEnabled() bool {
	return a.Enabled == nil || *a.Enabled
}
