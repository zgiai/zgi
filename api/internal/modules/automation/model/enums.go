package model

// AutomationTaskStatus represents the lifecycle state of an automation task.
type AutomationTaskStatus string

const (
	AutomationTaskStatusDraft     AutomationTaskStatus = "draft"
	AutomationTaskStatusActive    AutomationTaskStatus = "active"
	AutomationTaskStatusPaused    AutomationTaskStatus = "paused"
	AutomationTaskStatusCompleted AutomationTaskStatus = "completed"
	AutomationTaskStatusArchived  AutomationTaskStatus = "archived"
)

// AutomationTriggerType represents the trigger kind of an automation task.
type AutomationTriggerType string

const (
	AutomationTriggerTypeSchedule AutomationTriggerType = "schedule"
)

// AutomationScheduleType represents the stored schedule expression category.
type AutomationScheduleType string

const (
	AutomationScheduleTypeOnce AutomationScheduleType = "once"
	AutomationScheduleTypeCron AutomationScheduleType = "cron"
)

// AutomationSourceType represents how a task was created.
type AutomationSourceType string

const (
	AutomationSourceTypeManual   AutomationSourceType = "manual"
	AutomationSourceTypeWorkflow AutomationSourceType = "workflow"
	AutomationSourceTypeLLMTool  AutomationSourceType = "llm_tool"
)

// AutomationActionType represents the action executed when a task is triggered.
type AutomationActionType string

const (
	AutomationActionTypeSendNotification AutomationActionType = "send_notification"
	AutomationActionTypeRunWorkflow      AutomationActionType = "run_workflow"
)

// AutomationTaskRunStatus represents the status of a task run.
type AutomationTaskRunStatus string

const (
	AutomationTaskRunStatusQueued    AutomationTaskRunStatus = "queued"
	AutomationTaskRunStatusRunning   AutomationTaskRunStatus = "running"
	AutomationTaskRunStatusSucceeded AutomationTaskRunStatus = "succeeded"
	AutomationTaskRunStatusFailed    AutomationTaskRunStatus = "failed"
	AutomationTaskRunStatusCancelled AutomationTaskRunStatus = "cancelled"
)

// AutomationTriggerSource represents what caused a task run.
type AutomationTriggerSource string

const (
	AutomationTriggerSourceScheduler AutomationTriggerSource = "scheduler"
	AutomationTriggerSourceManualRun AutomationTriggerSource = "manual_run"
	AutomationTriggerSourceRetry     AutomationTriggerSource = "retry"
)

// AutomationActionRunStatus represents the status of an action execution.
type AutomationActionRunStatus string

const (
	AutomationActionRunStatusSucceeded AutomationActionRunStatus = "succeeded"
	AutomationActionRunStatusFailed    AutomationActionRunStatus = "failed"
)

// NotificationChannelType represents the notification delivery channel.
type NotificationChannelType string

const (
	NotificationChannelTypeEmail NotificationChannelType = "email"
	NotificationChannelTypeSMS   NotificationChannelType = "sms"
)
