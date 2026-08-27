package digitalhuman

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
)

// TaskWorkStatus is the operator-visible state of a DigitalHuman-owned task.
type TaskWorkStatus string

const (
	TaskWorkStatusProposed   TaskWorkStatus = "proposed"
	TaskWorkStatusPending    TaskWorkStatus = "pending"
	TaskWorkStatusInProgress TaskWorkStatus = "in_progress"
	TaskWorkStatusWaiting    TaskWorkStatus = "waiting"
	TaskWorkStatusBlocked    TaskWorkStatus = "blocked"
	TaskWorkStatusCompleted  TaskWorkStatus = "completed"
	TaskWorkStatusCanceled   TaskWorkStatus = "canceled"
)

// TaskPriority expresses user-visible urgency without affecting queue priority.
type TaskPriority string

const (
	TaskPriorityLow    TaskPriority = "low"
	TaskPriorityNormal TaskPriority = "normal"
	TaskPriorityHigh   TaskPriority = "high"
	TaskPriorityUrgent TaskPriority = "urgent"
)

// TaskScheduleType identifies whether and how a task is automatically run.
type TaskScheduleType string

const (
	TaskScheduleManual TaskScheduleType = "manual"
	TaskScheduleOnce   TaskScheduleType = "once"
	TaskScheduleCron   TaskScheduleType = "cron"
)

// TaskDeliveryMode controls where a committed task result is visible.
type TaskDeliveryMode string

const (
	TaskDeliveryTaskOnly           TaskDeliveryMode = "task_only"
	TaskDeliverySourceConversation TaskDeliveryMode = "source_conversation"
)

// TaskSchedule is the typed public schedule contract for DigitalHuman tasks.
type TaskSchedule struct {
	Type     TaskScheduleType `json:"type"`
	RunAt    *time.Time       `json:"run_at,omitempty"`
	Cron     string           `json:"cron,omitempty"`
	Timezone string           `json:"timezone,omitempty"`
}

// Task is the product-facing projection of a subject-owned automation task.
type Task struct {
	ID                      uuid.UUID        `json:"id"`
	DigitalHumanID          uuid.UUID        `json:"digital_human_id"`
	Name                    string           `json:"name"`
	Goal                    string           `json:"goal"`
	SuccessCriteria         []string         `json:"success_criteria"`
	WorkStatus              TaskWorkStatus   `json:"work_status"`
	LifecycleStatus         string           `json:"lifecycle_status"`
	Priority                TaskPriority     `json:"priority"`
	Schedule                TaskSchedule     `json:"schedule"`
	DeliveryMode            TaskDeliveryMode `json:"delivery_mode"`
	ExecutionTimeoutSeconds int              `json:"execution_timeout_seconds"`
	MaxAttempts             int              `json:"max_attempts"`
	NextRunAt               *time.Time       `json:"next_run_at,omitempty"`
	LastRunAt               *time.Time       `json:"last_run_at,omitempty"`
	LastRunStatus           string           `json:"last_run_status,omitempty"`
	Source                  string           `json:"source"`
	SourceConversationType  string           `json:"source_conversation_type,omitempty"`
	Revision                int64            `json:"revision"`
	ProposalExpiresAt       *time.Time       `json:"proposal_expires_at,omitempty"`
	CreatedAt               time.Time        `json:"created_at"`
	UpdatedAt               time.Time        `json:"updated_at"`
}

// TaskListQuery contains bounded task filters.
type TaskListQuery struct {
	Statuses             []TaskWorkStatus
	Priorities           []TaskPriority
	ScheduleType         TaskScheduleType
	Source               string
	Query                string
	UpdatedAfter         *time.Time
	UpdatedBefore        *time.Time
	RunFinishedAfter     *time.Time
	RunFinishedBefore    *time.Time
	Page                 int
	PageSize             int
	SourceConversationID string
}

// TaskListResult is a page of tasks and state summary counts.
type TaskListResult struct {
	Items              []*Task                  `json:"items"`
	Counts             map[TaskWorkStatus]int64 `json:"counts"`
	SuccessfulRunCount int64                    `json:"successful_run_count"`
	Total              int64                    `json:"total"`
	Page               int                      `json:"page"`
	PageSize           int                      `json:"page_size"`
	HasMore            bool                     `json:"has_more"`
}

// CreateTaskRequest contains only product fields; subject and delivery
// addresses are supplied by the service boundary.
type CreateTaskRequest struct {
	Name                    string           `json:"name"`
	Goal                    string           `json:"goal"`
	SuccessCriteria         []string         `json:"success_criteria,omitempty"`
	Priority                TaskPriority     `json:"priority,omitempty"`
	Schedule                TaskSchedule     `json:"schedule"`
	DeliveryMode            TaskDeliveryMode `json:"delivery_mode,omitempty"`
	ExecutionTimeoutSeconds int              `json:"execution_timeout_seconds,omitempty"`
	MaxAttempts             int              `json:"max_attempts,omitempty"`
}

// UpdateTaskRequest updates a task using optimistic concurrency.
type UpdateTaskRequest struct {
	ExpectedRevision        int64             `json:"expected_revision"`
	Name                    *string           `json:"name,omitempty"`
	Goal                    *string           `json:"goal,omitempty"`
	SuccessCriteria         *[]string         `json:"success_criteria,omitempty"`
	Priority                *TaskPriority     `json:"priority,omitempty"`
	Schedule                *TaskSchedule     `json:"schedule,omitempty"`
	DeliveryMode            *TaskDeliveryMode `json:"delivery_mode,omitempty"`
	ExecutionTimeoutSeconds *int              `json:"execution_timeout_seconds,omitempty"`
	MaxAttempts             *int              `json:"max_attempts,omitempty"`
}

// TaskAction identifies an explicit lifecycle operation.
type TaskAction string

const (
	TaskActionConfirm TaskAction = "confirm"
	TaskActionDecline TaskAction = "decline"
	TaskActionRunNow  TaskAction = "run_now"
	TaskActionPause   TaskAction = "pause"
	TaskActionResume  TaskAction = "resume"
	TaskActionRetry   TaskAction = "retry"
	TaskActionCancel  TaskAction = "cancel"
	TaskActionArchive TaskAction = "archive"
)

// ApplyTaskActionRequest applies one explicit task lifecycle transition.
type ApplyTaskActionRequest struct {
	ExpectedRevision int64 `json:"expected_revision"`
}

// TaskRun exposes one scheduler execution and its linked DigitalHuman run.
type TaskRun struct {
	ID                uuid.UUID  `json:"id"`
	TaskID            uuid.UUID  `json:"task_id"`
	DigitalHumanRunID *uuid.UUID `json:"digital_human_run_id,omitempty"`
	TriggerSource     string     `json:"trigger_source"`
	Attempt           int        `json:"attempt"`
	Status            string     `json:"status"`
	ErrorSummary      string     `json:"error_summary,omitempty"`
	ScheduledFor      time.Time  `json:"scheduled_for"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	FinishedAt        *time.Time `json:"finished_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

// TaskRunListResult is a bounded page of task executions.
type TaskRunListResult struct {
	Items    []*TaskRun `json:"items"`
	Total    int64      `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
	HasMore  bool       `json:"has_more"`
}

// TaskActivity is one audit entry in the task timeline.
type TaskActivity struct {
	ID        uuid.UUID      `json:"id"`
	ActorType string         `json:"actor_type"`
	ActorRef  string         `json:"actor_ref"`
	Action    string         `json:"action"`
	Summary   string         `json:"summary"`
	Changes   map[string]any `json:"changes,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// TaskActivityListResult is a bounded page of task mutations.
type TaskActivityListResult struct {
	Items    []*TaskActivity `json:"items"`
	Total    int64           `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
	HasMore  bool            `json:"has_more"`
}

// ChannelTaskPolicy is an explicit installation-level grant for task tools.
type ChannelTaskPolicy struct {
	InstallationID uuid.UUID `json:"installation_id"`
	Enabled        bool      `json:"enabled"`
	Revision       int64     `json:"revision"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// UpdateChannelTaskPolicyRequest uses CAS to avoid overwriting another editor.
type UpdateChannelTaskPolicyRequest struct {
	Enabled          bool  `json:"enabled"`
	ExpectedRevision int64 `json:"expected_revision"`
}

// TaskMutationContext describes the trusted server-side source of a task
// mutation. HTTP and runtime adapters construct it; model arguments cannot.
type TaskMutationContext struct {
	ActorType              string
	ActorRef               string
	OperationID            uuid.UUID
	SourceRunID            *uuid.UUID
	Source                 string
	SourceConversationID   string
	SourceConversationType string
	ReplyTo                *invocation.ReplyAddress
	Proposed               bool
}

// TaskHandlerService is the DigitalHuman task application boundary shared by
// HTTP, runtime tools, and the scheduler integration.
type TaskHandlerService interface {
	ListTasks(context.Context, Scope, uuid.UUID, TaskListQuery) (*TaskListResult, error)
	CreateTask(context.Context, Scope, uuid.UUID, CreateTaskRequest, TaskMutationContext) (*Task, error)
	GetTask(context.Context, Scope, uuid.UUID, uuid.UUID, string) (*Task, error)
	UpdateTask(context.Context, Scope, uuid.UUID, uuid.UUID, UpdateTaskRequest, TaskMutationContext) (*Task, error)
	ApplyTaskAction(context.Context, Scope, uuid.UUID, uuid.UUID, TaskAction, ApplyTaskActionRequest, TaskMutationContext) (*Task, error)
	SetTaskWorkStatus(context.Context, Scope, uuid.UUID, uuid.UUID, TaskWorkStatus, int64, TaskMutationContext) (*Task, error)
	ListTaskRuns(context.Context, Scope, uuid.UUID, uuid.UUID, int, int, string) (*TaskRunListResult, error)
	ListTaskActivities(context.Context, Scope, uuid.UUID, uuid.UUID, int, int, string) (*TaskActivityListResult, error)
	GetChannelTaskPolicy(context.Context, Scope, uuid.UUID, uuid.UUID) (*ChannelTaskPolicy, error)
	UpdateChannelTaskPolicy(context.Context, Scope, uuid.UUID, uuid.UUID, UpdateChannelTaskPolicyRequest) (*ChannelTaskPolicy, error)
	ChannelTaskManagementEnabled(context.Context, Scope, uuid.UUID, uuid.UUID) (bool, error)
}
