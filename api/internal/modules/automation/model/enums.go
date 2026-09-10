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

// AutomationTaskKind separates the original generic automation workbench from
// subject-owned task definitions that reuse the same durable scheduler.
type AutomationTaskKind string

const (
	AutomationTaskKindAutomation   AutomationTaskKind = "automation"
	AutomationTaskKindDigitalHuman AutomationTaskKind = "digital_human"
)

// AutomationTaskOwnerKind separates a DigitalHuman's shared work from work
// owned by one user subject or one external conversation.
type AutomationTaskOwnerKind string

const (
	AutomationTaskOwnerDigitalHuman AutomationTaskOwnerKind = "digital_human"
	AutomationTaskOwnerUserSubject  AutomationTaskOwnerKind = "user_subject"
	AutomationTaskOwnerConversation AutomationTaskOwnerKind = "conversation"
)

// AutomationWorkStatus is the user-facing lifecycle of a DigitalHuman task.
// It is intentionally separate from AutomationTaskStatus, which controls
// whether the scheduler may dispatch the definition.
type AutomationWorkStatus string

const (
	AutomationWorkStatusProposed   AutomationWorkStatus = "proposed"
	AutomationWorkStatusPending    AutomationWorkStatus = "pending"
	AutomationWorkStatusInProgress AutomationWorkStatus = "in_progress"
	AutomationWorkStatusWaiting    AutomationWorkStatus = "waiting"
	AutomationWorkStatusBlocked    AutomationWorkStatus = "blocked"
	AutomationWorkStatusCompleted  AutomationWorkStatus = "completed"
	AutomationWorkStatusCanceled   AutomationWorkStatus = "canceled"
)

// AutomationTaskPriority is the operator-visible urgency of a task.
type AutomationTaskPriority string

const (
	AutomationTaskPriorityLow    AutomationTaskPriority = "low"
	AutomationTaskPriorityNormal AutomationTaskPriority = "normal"
	AutomationTaskPriorityHigh   AutomationTaskPriority = "high"
	AutomationTaskPriorityUrgent AutomationTaskPriority = "urgent"
)

// AutomationTriggerType represents the trigger kind of an automation task.
type AutomationTriggerType string

const (
	AutomationTriggerTypeSchedule AutomationTriggerType = "schedule"
)

// AutomationScheduleType represents the stored schedule expression category.
type AutomationScheduleType string

const (
	AutomationScheduleTypeManual AutomationScheduleType = "manual"
	AutomationScheduleTypeOnce   AutomationScheduleType = "once"
	AutomationScheduleTypeCron   AutomationScheduleType = "cron"
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
	AutomationActionTypeRunDigitalHuman  AutomationActionType = "run_digital_human"
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
	AutomationActionRunStatusQueued    AutomationActionRunStatus = "queued"
	AutomationActionRunStatusRunning   AutomationActionRunStatus = "running"
	AutomationActionRunStatusSucceeded AutomationActionRunStatus = "succeeded"
	AutomationActionRunStatusFailed    AutomationActionRunStatus = "failed"
	AutomationActionRunStatusCancelled AutomationActionRunStatus = "cancelled"
)

// AutomationTaskActivityActor identifies who produced an auditable task
// mutation without turning an external sender into a platform account.
type AutomationTaskActivityActor string

const (
	AutomationTaskActivityActorAccount      AutomationTaskActivityActor = "account"
	AutomationTaskActivityActorDigitalHuman AutomationTaskActivityActor = "digital_human"
	AutomationTaskActivityActorExternal     AutomationTaskActivityActor = "external"
	AutomationTaskActivityActorSystem       AutomationTaskActivityActor = "system"
)

// NotificationChannelType represents the notification delivery channel.
type NotificationChannelType string

const (
	NotificationChannelTypeEmail NotificationChannelType = "email"
	NotificationChannelTypeSMS   NotificationChannelType = "sms"
)
