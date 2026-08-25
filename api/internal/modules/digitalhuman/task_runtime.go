package digitalhuman

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const (
	taskListToolName           = "list_digital_human_tasks"
	taskGetToolName            = "get_digital_human_task"
	taskManageToolName         = "manage_digital_human_task"
	taskListViewOpen           = "open"
	taskListViewCompleted      = "completed"
	taskListViewCanceled       = "canceled"
	taskListDefaultLimit       = 10
	taskListMaximumLimit       = 20
	taskHistoryDefaultLookback = 30 * 24 * time.Hour
)

type TaskRuntimePreparation struct {
	SystemPromptAddendum string
	Tools                []skillloop.RuntimeTool
}
type TaskRuntimeProvider interface {
	Prepare(context.Context, *Run, invocation.Event) (*TaskRuntimePreparation, error)
}

type DefaultTaskRuntimeProvider struct {
	service TaskHandlerService
	now     func() time.Time
}

func NewTaskRuntimeProvider(service TaskHandlerService) *DefaultTaskRuntimeProvider {
	if service == nil {
		panic("DigitalHuman task runtime requires service")
	}
	return &DefaultTaskRuntimeProvider{service: service, now: func() time.Time { return time.Now().UTC() }}
}

type taskRuntimeBoundary struct {
	source           string
	conversationID   string
	conversationType string
	replyTo          *invocation.ReplyAddress
	currentTaskID    uuid.UUID
	writable         bool
}

func (p *DefaultTaskRuntimeProvider) Prepare(ctx context.Context, run *Run, event invocation.Event) (*TaskRuntimePreparation, error) {
	if run == nil || event.Target.ID != run.DigitalHumanID {
		return nil, ErrInvalidRequest
	}
	boundary := taskBoundary(event)
	scope := runScope(run)
	if scope.ActorID == uuid.Nil {
		scope.ActorID = run.DigitalHumanID
	}
	switch event.Origin.Kind {
	case invocation.OriginChannel:
		installationID, err := uuid.Parse(strings.TrimSpace(event.Origin.InstallationID))
		if err != nil {
			return nil, nil
		}
		enabled, err := p.service.ChannelTaskManagementEnabled(ctx, scope, run.DigitalHumanID, installationID)
		if err != nil {
			return nil, err
		}
		boundary.writable = enabled
	case invocation.OriginManual:
		boundary.writable = true
	case invocation.OriginSchedule:
		boundary.writable = boundary.currentTaskID != uuid.Nil
	}
	if !boundary.writable {
		return &TaskRuntimePreparation{SystemPromptAddendum: "Task management is not enabled for this invocation. Do not claim that a task was created, scheduled, or changed."}, nil
	}
	return &TaskRuntimePreparation{
		SystemPromptAddendum: taskRuntimeGuidance(boundary),
		Tools:                []skillloop.RuntimeTool{p.listTool(run, boundary), p.getTool(run, boundary), p.manageTool(run, boundary)},
	}, nil
}

func taskRuntimeGuidance(boundary taskRuntimeBoundary) string {
	guidance := []string{
		"You can manage the visible todo list owned by this DigitalHuman.",
		"When the current user explicitly and unambiguously asks to create a task, reminder, or schedule, use create to commit it in one operation. Do not propose and then confirm the same explicit request.",
		"When ordinary discussion merely suggests possible future work, use propose and ask the user to confirm it. Never silently turn inferred work into a confirmed task.",
		"When the user confirms an earlier proposal, list proposed tasks from this conversation first. Confirm the single matching proposal using its returned ID and revision. If several proposals could match, ask which one instead of guessing.",
		"Task listing defaults to unfinished work. Read completed or canceled history only when the user asks about past work, a previous result is needed, a recurring task needs its prior outcome, or a duplicate check requires recent history.",
		"Historical task lists are summaries. Get one task by ID only after identifying the relevant record; do not load unrelated completed work into the conversation.",
		"Confirm, update, schedule, run, cancel, or change status only when the current user explicitly requests that operation.",
		"A task is operational state, not long-term memory. Do not store task contents merely because a task exists.",
		"Never claim a task mutation succeeded unless the task tool reports success. Use the returned revision for subsequent mutations.",
	}
	if boundary.currentTaskID != uuid.Nil {
		guidance = append(guidance, "This is a scheduled execution. You may only update the current task to waiting, blocked, or completed; do not create, confirm, reschedule, rerun, or cancel tasks from this run.")
	}
	if boundary.conversationID != "" {
		conversationScope := "current external conversation"
		switch boundary.conversationType {
		case "group":
			conversationScope = "current group conversation"
		case "direct":
			conversationScope = "current private conversation"
		}
		guidance = append(guidance,
			"Task visibility is restricted to the "+conversationScope+". Tasks created in other private chats, groups, or the console are intentionally hidden.",
			"When answering a task list or status question, explicitly describe the result as belonging to the "+conversationScope+". Never imply it is the DigitalHuman's complete todo list.",
		)
	}
	return strings.Join(guidance, "\n")
}

func taskBoundary(event invocation.Event) taskRuntimeBoundary {
	result := taskRuntimeBoundary{source: "console", replyTo: event.ReplyTo}
	if event.Origin.Kind == invocation.OriginChannel {
		result.source = "channel:" + event.Origin.Provider
		if event.ReplyTo != nil {
			result.conversationID = event.ReplyTo.ConversationID
		}
		if raw, ok := event.Context["conversation_type"].(string); ok {
			switch strings.TrimSpace(raw) {
			case "direct", "group":
				result.conversationType = strings.TrimSpace(raw)
			}
		}
	}
	if event.Origin.Kind == invocation.OriginSchedule {
		result.source = "schedule"
		if raw, ok := event.Context["task_id"].(string); ok {
			result.currentTaskID, _ = uuid.Parse(raw)
		}
	}
	return result
}

func (p *DefaultTaskRuntimeProvider) listTool(run *Run, boundary taskRuntimeBoundary) skillloop.RuntimeTool {
	definition := adapter.Tool{Type: "function", Function: adapter.Function{Name: taskListToolName, Description: taskListToolDescription(boundary), Parameters: map[string]any{"type": "object", "properties": map[string]any{
		"view":          map[string]any{"type": "string", "enum": []string{taskListViewOpen, taskListViewCompleted, taskListViewCanceled}, "description": "Task collection to read. Defaults to open."},
		"query":         map[string]any{"type": "string", "description": "Optional task name or goal search."},
		"history_since": map[string]any{"type": "string", "format": "date-time", "description": "Earliest history update time. Only valid for completed or canceled views; defaults to 30 days ago."},
		"history_until": map[string]any{"type": "string", "format": "date-time", "description": "Latest history update time. Only valid for completed or canceled views."},
		"limit":         map[string]any{"type": "integer", "minimum": 1, "maximum": taskListMaximumLimit, "description": "Maximum summaries to return. Defaults to 10."},
	}, "additionalProperties": false}}}
	handler := func(ctx context.Context, call adapter.ToolCall) skillloop.RuntimeToolResult {
		var args struct {
			View         string `json:"view"`
			Query        string `json:"query"`
			HistorySince string `json:"history_since"`
			HistoryUntil string `json:"history_until"`
			Limit        int    `json:"limit"`
		}
		if decodeStrict(call.Function.Arguments, &args) != nil {
			return taskToolFailure("invalid_arguments", true)
		}
		if boundary.currentTaskID != uuid.Nil {
			item, err := p.service.GetTask(ctx, runScopeWithActor(run), run.DigitalHumanID, boundary.currentTaskID, "")
			if err != nil {
				return taskToolFailure("task_not_found", false)
			}
			return taskToolSuccess(call, map[string]any{"view": taskListViewOpen, "scope": taskListScope(boundary), "items": []taskListToolItem{newTaskListToolItem(item)}, "total": 1, "has_more": false})
		}
		query, view, err := p.taskListQuery(args.View, args.Query, args.HistorySince, args.HistoryUntil, args.Limit, boundary.conversationID)
		if err != nil {
			return taskToolFailure("invalid_arguments", true)
		}
		result, err := p.service.ListTasks(ctx, runScopeWithActor(run), run.DigitalHumanID, query)
		if err != nil {
			return taskToolFailure(taskErrorCode(err), false)
		}
		items := make([]taskListToolItem, 0, len(result.Items))
		for _, item := range result.Items {
			items = append(items, newTaskListToolItem(item))
		}
		return taskToolSuccess(call, map[string]any{"view": view, "scope": taskListScope(boundary), "items": items, "total": result.Total, "has_more": result.HasMore})
	}
	return skillloop.RuntimeTool{Definition: definition, SkillID: skills.SkillDigitalHumanTasks, Handler: handler}
}

type taskListToolScope struct {
	Visibility       string `json:"visibility"`
	ConversationType string `json:"conversation_type,omitempty"`
}

func taskListToolDescription(boundary taskRuntimeBoundary) string {
	description := "List this DigitalHuman's visible tasks. Omit view to return unfinished work. Use completed or canceled only when relevant history is needed."
	if boundary.conversationID != "" {
		description += " This channel call returns only tasks from the current conversation, not the DigitalHuman's complete todo list."
	}
	return description
}

func taskListScope(boundary taskRuntimeBoundary) taskListToolScope {
	if boundary.currentTaskID != uuid.Nil {
		return taskListToolScope{Visibility: "current_task"}
	}
	if boundary.conversationID != "" {
		return taskListToolScope{Visibility: "current_conversation", ConversationType: boundary.conversationType}
	}
	return taskListToolScope{Visibility: "all_digital_human_tasks"}
}

type taskListToolItem struct {
	ID            uuid.UUID      `json:"id"`
	Name          string         `json:"name"`
	Goal          string         `json:"goal"`
	WorkStatus    TaskWorkStatus `json:"work_status"`
	Priority      TaskPriority   `json:"priority"`
	NextRunAt     *time.Time     `json:"next_run_at,omitempty"`
	LastRunAt     *time.Time     `json:"last_run_at,omitempty"`
	LastRunStatus string         `json:"last_run_status,omitempty"`
	Revision      int64          `json:"revision"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

func newTaskListToolItem(task *Task) taskListToolItem {
	return taskListToolItem{ID: task.ID, Name: task.Name, Goal: task.Goal, WorkStatus: task.WorkStatus, Priority: task.Priority, NextRunAt: task.NextRunAt, LastRunAt: task.LastRunAt, LastRunStatus: task.LastRunStatus, Revision: task.Revision, UpdatedAt: task.UpdatedAt}
}

func (p *DefaultTaskRuntimeProvider) taskListQuery(view, keyword, historySince, historyUntil string, limit int, conversationID string) (TaskListQuery, string, error) {
	view = strings.TrimSpace(view)
	if view == "" {
		view = taskListViewOpen
	}
	statuses, historical, ok := taskListStatuses(view)
	if !ok {
		return TaskListQuery{}, "", ErrInvalidRequest
	}
	if limit == 0 {
		limit = taskListDefaultLimit
	}
	if limit < 1 || limit > taskListMaximumLimit {
		return TaskListQuery{}, "", ErrInvalidRequest
	}
	query := TaskListQuery{Statuses: statuses, Query: keyword, Page: 1, PageSize: limit, SourceConversationID: conversationID}
	if !historical {
		if strings.TrimSpace(historySince) != "" || strings.TrimSpace(historyUntil) != "" {
			return TaskListQuery{}, "", ErrInvalidRequest
		}
		return query, view, nil
	}
	var err error
	query.UpdatedAfter, err = parseTaskHistoryTime(historySince)
	if err != nil {
		return TaskListQuery{}, "", ErrInvalidRequest
	}
	if query.UpdatedAfter == nil {
		value := p.now().UTC().Add(-taskHistoryDefaultLookback)
		query.UpdatedAfter = &value
	}
	query.UpdatedBefore, err = parseTaskHistoryTime(historyUntil)
	if err != nil || query.UpdatedBefore != nil && query.UpdatedBefore.Before(*query.UpdatedAfter) {
		return TaskListQuery{}, "", ErrInvalidRequest
	}
	return query, view, nil
}

func taskListStatuses(view string) ([]TaskWorkStatus, bool, bool) {
	switch view {
	case taskListViewOpen:
		return []TaskWorkStatus{TaskWorkStatusProposed, TaskWorkStatusPending, TaskWorkStatusInProgress, TaskWorkStatusWaiting, TaskWorkStatusBlocked}, false, true
	case taskListViewCompleted:
		return []TaskWorkStatus{TaskWorkStatusCompleted}, true, true
	case taskListViewCanceled:
		return []TaskWorkStatus{TaskWorkStatusCanceled}, true, true
	default:
		return nil, false, false
	}
}

func parseTaskHistoryTime(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	value = value.UTC()
	return &value, nil
}

func (p *DefaultTaskRuntimeProvider) getTool(run *Run, boundary taskRuntimeBoundary) skillloop.RuntimeTool {
	definition := adapter.Tool{Type: "function", Function: adapter.Function{Name: taskGetToolName, Description: "Get one visible task and its current revision.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"task_id": map[string]any{"type": "string", "format": "uuid"}}, "required": []string{"task_id"}, "additionalProperties": false}}}
	handler := func(ctx context.Context, call adapter.ToolCall) skillloop.RuntimeToolResult {
		var args struct {
			TaskID uuid.UUID `json:"task_id"`
		}
		if decodeStrict(call.Function.Arguments, &args) != nil || args.TaskID == uuid.Nil || (boundary.currentTaskID != uuid.Nil && args.TaskID != boundary.currentTaskID) {
			return taskToolFailure("invalid_arguments", true)
		}
		result, err := p.service.GetTask(ctx, runScopeWithActor(run), run.DigitalHumanID, args.TaskID, boundary.conversationID)
		if err != nil {
			return taskToolFailure(taskErrorCode(err), false)
		}
		return taskToolSuccess(call, result)
	}
	return skillloop.RuntimeTool{Definition: definition, SkillID: skills.SkillDigitalHumanTasks, Handler: handler}
}

type manageTaskArguments struct {
	Action                  string           `json:"action"`
	TaskID                  *uuid.UUID       `json:"task_id"`
	ExpectedRevision        int64            `json:"expected_revision"`
	Name                    string           `json:"name"`
	Goal                    string           `json:"goal"`
	SuccessCriteria         []string         `json:"success_criteria"`
	Priority                TaskPriority     `json:"priority"`
	Status                  TaskWorkStatus   `json:"status"`
	Schedule                *TaskSchedule    `json:"schedule"`
	DeliveryMode            TaskDeliveryMode `json:"delivery_mode"`
	ExecutionTimeoutSeconds int              `json:"execution_timeout_seconds"`
	MaxAttempts             int              `json:"max_attempts"`
}

func (p *DefaultTaskRuntimeProvider) manageTool(run *Run, boundary taskRuntimeBoundary) skillloop.RuntimeTool {
	definition := adapter.Tool{Type: "function", Function: adapter.Function{Name: taskManageToolName, Description: "Create an explicitly requested task in one operation, propose inferred work for confirmation, or manage an existing task. Existing-task actions require its ID and current revision.", Parameters: map[string]any{"type": "object", "properties": map[string]any{
		"action": map[string]any{"type": "string", "enum": []string{"create", "propose", "confirm", "decline", "update", "set_status", "schedule", "run_now", "cancel"}}, "task_id": map[string]any{"type": "string", "format": "uuid"}, "expected_revision": map[string]any{"type": "integer", "minimum": 0}, "name": map[string]any{"type": "string"}, "goal": map[string]any{"type": "string"}, "success_criteria": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "priority": map[string]any{"type": "string", "enum": []string{"low", "normal", "high", "urgent"}}, "status": map[string]any{"type": "string", "enum": taskStatusValues()}, "schedule": map[string]any{"type": "object", "properties": map[string]any{"type": map[string]any{"type": "string", "enum": []string{"manual", "once", "cron"}}, "run_at": map[string]any{"type": "string", "format": "date-time"}, "cron": map[string]any{"type": "string"}, "timezone": map[string]any{"type": "string"}}, "required": []string{"type"}, "additionalProperties": false}, "delivery_mode": map[string]any{"type": "string", "enum": []string{"task_only", "source_conversation"}}, "execution_timeout_seconds": map[string]any{"type": "integer", "minimum": 30, "maximum": 1800}, "max_attempts": map[string]any{"type": "integer", "minimum": 1, "maximum": 3},
	}, "required": []string{"action"}, "additionalProperties": false}}}
	handler := func(ctx context.Context, call adapter.ToolCall) skillloop.RuntimeToolResult {
		var args manageTaskArguments
		if decodeStrict(call.Function.Arguments, &args) != nil {
			return taskToolFailure("invalid_arguments", true)
		}
		mutation := TaskMutationContext{ActorType: "digital_human", ActorRef: run.DigitalHumanID.String(), OperationID: taskToolOperationID(run.ID, call), SourceRunID: &run.ID, Source: boundary.source, SourceConversationID: boundary.conversationID, SourceConversationType: boundary.conversationType, ReplyTo: boundary.replyTo}
		if boundary.currentTaskID != uuid.Nil && !scheduledTaskMutationAllowed(args, boundary.currentTaskID) {
			return taskToolFailure("digital_human.task.policy_rejected", false)
		}
		if args.Action == "create" || args.Action == "propose" {
			mutation.Proposed = args.Action == "propose"
			result, err := p.service.CreateTask(ctx, runScopeWithActor(run), run.DigitalHumanID, CreateTaskRequest{Name: args.Name, Goal: args.Goal, SuccessCriteria: args.SuccessCriteria, Priority: args.Priority, Schedule: valueSchedule(args.Schedule), DeliveryMode: args.DeliveryMode, ExecutionTimeoutSeconds: args.ExecutionTimeoutSeconds, MaxAttempts: args.MaxAttempts}, mutation)
			if err != nil {
				return taskToolFailure(taskErrorCode(err), errors.Is(err, ErrInvalidRequest))
			}
			status := "committed"
			if mutation.Proposed {
				status = "proposed"
			}
			return taskToolSuccess(call, map[string]any{"status": status, "task": result})
		}
		if args.TaskID == nil || *args.TaskID == uuid.Nil || args.ExpectedRevision < 1 || (boundary.currentTaskID != uuid.Nil && *args.TaskID != boundary.currentTaskID) {
			return taskToolFailure("invalid_arguments", true)
		}
		var result *Task
		var err error
		switch args.Action {
		case "confirm":
			result, err = p.service.ApplyTaskAction(ctx, runScopeWithActor(run), run.DigitalHumanID, *args.TaskID, TaskActionConfirm, ApplyTaskActionRequest{ExpectedRevision: args.ExpectedRevision}, mutation)
		case "decline":
			result, err = p.service.ApplyTaskAction(ctx, runScopeWithActor(run), run.DigitalHumanID, *args.TaskID, TaskActionDecline, ApplyTaskActionRequest{ExpectedRevision: args.ExpectedRevision}, mutation)
		case "run_now":
			result, err = p.service.ApplyTaskAction(ctx, runScopeWithActor(run), run.DigitalHumanID, *args.TaskID, TaskActionRunNow, ApplyTaskActionRequest{ExpectedRevision: args.ExpectedRevision}, mutation)
		case "cancel":
			result, err = p.service.ApplyTaskAction(ctx, runScopeWithActor(run), run.DigitalHumanID, *args.TaskID, TaskActionCancel, ApplyTaskActionRequest{ExpectedRevision: args.ExpectedRevision}, mutation)
		case "set_status":
			result, err = p.service.SetTaskWorkStatus(ctx, runScopeWithActor(run), run.DigitalHumanID, *args.TaskID, args.Status, args.ExpectedRevision, mutation)
		case "schedule":
			if args.Schedule == nil {
				return taskToolFailure("invalid_arguments", true)
			}
			result, err = p.service.UpdateTask(ctx, runScopeWithActor(run), run.DigitalHumanID, *args.TaskID, UpdateTaskRequest{ExpectedRevision: args.ExpectedRevision, Schedule: args.Schedule}, mutation)
		case "update":
			result, err = p.service.UpdateTask(ctx, runScopeWithActor(run), run.DigitalHumanID, *args.TaskID, updateRequest(args), mutation)
		default:
			return taskToolFailure("invalid_arguments", true)
		}
		if err != nil {
			return taskToolFailure(taskErrorCode(err), false)
		}
		return taskToolSuccess(call, map[string]any{"status": "committed", "task": result})
	}
	return skillloop.RuntimeTool{Definition: definition, SkillID: skills.SkillDigitalHumanTasks, Handler: handler}
}

func scheduledTaskMutationAllowed(args manageTaskArguments, currentTaskID uuid.UUID) bool {
	if args.Action != "set_status" || args.TaskID == nil || *args.TaskID != currentTaskID {
		return false
	}
	switch args.Status {
	case TaskWorkStatusWaiting, TaskWorkStatusBlocked, TaskWorkStatusCompleted:
		return true
	default:
		return false
	}
}

func updateRequest(args manageTaskArguments) UpdateTaskRequest {
	req := UpdateTaskRequest{ExpectedRevision: args.ExpectedRevision}
	if strings.TrimSpace(args.Name) != "" {
		req.Name = &args.Name
	}
	if strings.TrimSpace(args.Goal) != "" {
		req.Goal = &args.Goal
	}
	if args.SuccessCriteria != nil {
		req.SuccessCriteria = &args.SuccessCriteria
	}
	if args.Priority != "" {
		req.Priority = &args.Priority
	}
	if args.DeliveryMode != "" {
		req.DeliveryMode = &args.DeliveryMode
	}
	if args.ExecutionTimeoutSeconds > 0 {
		req.ExecutionTimeoutSeconds = &args.ExecutionTimeoutSeconds
	}
	if args.MaxAttempts > 0 {
		req.MaxAttempts = &args.MaxAttempts
	}
	return req
}
func valueSchedule(value *TaskSchedule) TaskSchedule {
	if value == nil {
		return TaskSchedule{Type: TaskScheduleManual, Timezone: "UTC"}
	}
	return *value
}
func runScopeWithActor(run *Run) Scope {
	scope := runScope(run)
	if scope.ActorID == uuid.Nil {
		scope.ActorID = run.DigitalHumanID
	}
	return scope
}
func taskStatusValues() []string {
	return []string{"proposed", "pending", "in_progress", "waiting", "blocked", "completed", "canceled"}
}
func decodeStrict(raw string, target any) error {
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ErrInvalidRequest
	}
	return nil
}
func taskToolOperationID(runID uuid.UUID, call adapter.ToolCall) uuid.UUID {
	identity := strings.TrimSpace(call.ID)
	if identity == "" {
		identity = call.Function.Name + "\x00" + call.Function.Arguments
	}
	return uuid.NewSHA1(runID, []byte(identity))
}
func taskToolSuccess(call adapter.ToolCall, result any) skillloop.RuntimeToolResult {
	return skillloop.RuntimeToolResult{Status: "success", Arguments: map[string]any{"tool": call.Function.Name}, Result: map[string]any{"data": result}}
}
func taskToolFailure(code string, recoverable bool) skillloop.RuntimeToolResult {
	modelResult := map[string]any{"status": "rejected", "error_code": code, "retry_allowed": recoverable, "next_action": taskToolNextAction(code)}
	return skillloop.RuntimeToolResult{
		Status: "failed", Arguments: map[string]any{},
		Result:      map[string]any{"status": "rejected", "error_code": code, "retry_allowed": recoverable},
		ModelResult: modelResult, Error: ErrTaskPolicyRejected, Recoverable: recoverable,
	}
}

func taskToolNextAction(code string) string {
	switch code {
	case "invalid_arguments":
		return "Correct the task fields and retry once. Existing-task actions require task_id and expected_revision; use create for a new explicit request."
	case "digital_human.task.not_found":
		return "List tasks in the current conversation and use an existing task ID."
	case "digital_human.task.conflict":
		return "Get the task again and retry with its latest revision."
	case "digital_human.task.policy_rejected":
		return "Do not retry this mutation; explain the task boundary or ask the user for an explicit instruction."
	default:
		return "Do not claim success. Explain that the task change could not be completed."
	}
}
func taskErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrTaskNotFound):
		return "digital_human.task.not_found"
	case errors.Is(err, ErrTaskConflict):
		return "digital_human.task.conflict"
	case errors.Is(err, ErrTaskPolicyRejected):
		return "digital_human.task.policy_rejected"
	default:
		return "digital_human.task.failed"
	}
}
