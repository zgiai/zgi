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
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
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

// TaskService exposes the task domain to the conversation continuation
// adapter without coupling ChatRuntime to DigitalHuman internals.
func (p *DefaultTaskRuntimeProvider) TaskService() TaskHandlerService {
	if p == nil {
		return nil
	}
	return p.service
}

type taskRuntimeBoundary struct {
	source           string
	conversationID   string
	conversationType string
	replyTo          *invocation.ReplyAddress
	currentTaskID    uuid.UUID
	access           TaskAccessContext
	writable         bool
}

func (p *DefaultTaskRuntimeProvider) Prepare(ctx context.Context, run *Run, event invocation.Event) (*TaskRuntimePreparation, error) {
	if run == nil || event.Target.ID != run.DigitalHumanID {
		return nil, ErrInvalidRequest
	}
	boundary := taskBoundary(event)
	switch event.Origin.Kind {
	case invocation.OriginChannel:
		boundary.writable = true
	case invocation.OriginManual:
		boundary.writable = true
	case invocation.OriginAPI:
		// API and authenticated WebApp runs use the current UserSubject as
		// their private task namespace. Anonymous WebApp runs have no subject
		// and are rejected by taskAccessForRun below.
		boundary.writable = run.UserSubjectID != nil && *run.UserSubjectID != uuid.Nil
	case invocation.OriginSchedule:
		boundary.writable = boundary.currentTaskID != uuid.Nil
	}
	if !boundary.writable {
		return &TaskRuntimePreparation{SystemPromptAddendum: "Task management is not enabled for this invocation. Do not claim that a task was created, scheduled, or changed."}, nil
	}
	access, err := taskAccessForRun(run, event, boundary)
	if err != nil {
		return nil, err
	}
	boundary.access = access
	return &TaskRuntimePreparation{
		SystemPromptAddendum: taskRuntimeGuidance(boundary),
		Tools:                []skillloop.RuntimeTool{p.listTool(run, boundary), p.getTool(run, boundary), p.manageTool(run, boundary)},
	}, nil
}

func taskRuntimeGuidance(boundary taskRuntimeBoundary) string {
	guidance := []string{
		"You can manage only the task namespace made visible to this invocation.",
		"Every conversational task creation requires a separate user approval. When the current user explicitly or implicitly asks for a task, reminder, or schedule, create a proposed task and ask the user to confirm it; never commit a new task in the same turn.",
		"The create and propose actions both create a proposal. Treat create as a compatibility alias for propose, not as an approval bypass. This approval rule applies to console, API, WebApp, and channel conversations.",
		"When the user confirms an earlier proposal, list proposed tasks from the current task namespace first; for a channel, prefer the current conversation. Confirm the single matching proposal using its returned ID and revision. If several proposals could match, ask which one instead of guessing.",
		"When one request changes a paused task's schedule and also asks to restore, activate, or resume it, submit one update action with both schedule and lifecycle_status=active. Do not split that request into separate schedule and resume operations.",
		"When you change a task's schedule, check its goal and success criteria for times, dates, or frequencies the change makes wrong. Include corrected goal and success_criteria in the same update so the approval shows one consistent task; leave them out only when they do not mention the schedule.",
		"Task listing defaults to unfinished work. Read completed or canceled history only when the user asks about past work, a previous result is needed, a recurring task needs its prior outcome, or a duplicate check requires recent history.",
		"Historical task lists are summaries. Get one task by ID only after identifying the relevant record; do not load unrelated completed work into the conversation.",
		"Confirm, update, schedule, run, cancel, pause, resume, retry, archive, or change status only when the current user explicitly requests that operation.",
		"Use pause and resume for the task's lifecycle. Do not use set_status to resume a paused task: set_status changes only the work status and does not reactivate scheduling.",
		"A task is operational state, not long-term memory. Do not store task contents merely because a task exists.",
		"Never claim a task was created, scheduled, or changed unless the task tool reports success. A successful proposal is not an approved or active task; tell the user that approval is still required. Use the returned revision for subsequent mutations.",
	}
	if boundary.currentTaskID != uuid.Nil {
		guidance = append(guidance, "This is a scheduled execution. The scheduler owns successful completion: do not mark the task completed. You may only update the current task to waiting or blocked when the work genuinely cannot finish; do not create, confirm, reschedule, rerun, or cancel tasks from this run.")
	} else if TaskVisibilityEnabled() && boundary.access.Viewer.Mode == TaskViewMember {
		guidance = append(guidance,
			"Visible tasks are the tasks the current user requested plus tasks shared with the workspace. Tasks with can_change false are read-only: explain that only their requester or a task manager can change them, and never propose a change to them.",
			"Open tasks requested by others appear only in other_open_work as time, status, and priority. Use them to reason about workload and schedule conflicts, but never guess, ask about, or reveal their content or requester.",
		)
	} else if boundary.access.Owner.Kind == TaskOwnerUserSubject {
		guidance = append(guidance,
			"Task visibility is restricted to the current signed-in user. Shared DigitalHuman tasks and other users' private tasks are intentionally hidden.",
			"When answering a task list or status question, describe the result as the current user's tasks, not the DigitalHuman's complete shared todo list.",
		)
	}
	if boundary.conversationID != "" && (!TaskVisibilityEnabled() || boundary.access.Viewer.Mode == TaskViewChannel) {
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
		if taskOccupancyEnabled(boundary) && boundary.access.Viewer.Mode == TaskViewChannel {
			guidance = append(guidance, "Other open work of this DigitalHuman appears only in other_open_work as time, status, and priority. Use it to reason about workload and schedule conflicts, but never guess or reveal its content or requester.")
		}
	}
	return strings.Join(guidance, "\n")
}

func taskBoundary(event invocation.Event) taskRuntimeBoundary {
	result := taskRuntimeBoundary{source: "console", replyTo: event.ReplyTo}
	// Manual/API/WebApp runs carry the canonical ChatRuntime conversation in
	// the signed invocation context. Capture it once so task delivery can
	// return to the same conversation after a scheduled run completes.
	if raw, ok := event.Context["conversation_id"].(string); ok {
		result.conversationID = strings.TrimSpace(raw)
	}
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
	if event.Origin.Kind == invocation.OriginAPI {
		result.source = "external-api"
		if event.Context != nil {
			if surface, ok := event.Context["surface"].(string); ok && strings.TrimSpace(surface) == SurfaceWebApp {
				result.source = "webapp"
			}
		}
	}
	return result
}

func taskAccessForRun(run *Run, event invocation.Event, boundary taskRuntimeBoundary) (TaskAccessContext, error) {
	if run == nil || run.OrganizationID == uuid.Nil || run.WorkspaceID == uuid.Nil || run.DigitalHumanID == uuid.Nil {
		return TaskAccessContext{}, ErrInvalidRequest
	}
	access := TaskAccessContext{
		OrganizationID: run.OrganizationID,
		WorkspaceID:    run.WorkspaceID,
		DigitalHumanID: run.DigitalHumanID,
		Principal:      event.Sender,
		Surface:        run.Surface,
	}
	switch event.Origin.Kind {
	case invocation.OriginManual:
		if run.UserSubjectID == nil || *run.UserSubjectID == uuid.Nil {
			return TaskAccessContext{}, ErrTaskPolicyRejected
		}
		access.Owner = UserTaskOwner(usersubject.Ref{Kind: usersubject.Kind(run.UserSubjectKind), ID: *run.UserSubjectID})
		access.Surface = SurfaceConsole
		access.Viewer = TaskViewer{Mode: TaskViewMember}
	case invocation.OriginAPI:
		if run.UserSubjectID == nil || *run.UserSubjectID == uuid.Nil {
			return TaskAccessContext{}, ErrTaskPolicyRejected
		}
		kind := usersubject.Kind(run.UserSubjectKind)
		if kind != usersubject.Account && kind != usersubject.APIEndUser {
			return TaskAccessContext{}, ErrTaskPolicyRejected
		}
		if run.Surface != SurfaceAPI && run.Surface != SurfaceWebApp {
			return TaskAccessContext{}, ErrTaskPolicyRejected
		}
		access.Owner = UserTaskOwner(usersubject.Ref{Kind: kind, ID: *run.UserSubjectID})
		access.Surface = run.Surface
		access.Viewer = TaskViewer{Mode: TaskViewExternal}
	case invocation.OriginChannel:
		conversationID, err := ChannelTaskConversationID(run.OrganizationID, run.WorkspaceID, run.DigitalHumanID, event.Origin.Provider, event.Origin.InstallationID, boundary.conversationID)
		if err != nil {
			return TaskAccessContext{}, err
		}
		access.Owner = ConversationTaskOwner(conversationID)
		access.Surface = SurfaceChannel
		access.Viewer = TaskViewer{Mode: TaskViewChannel}
	case invocation.OriginSchedule:
		if boundary.currentTaskID == uuid.Nil || event.Sender.Type != invocation.PrincipalSystem {
			return TaskAccessContext{}, ErrTaskPolicyRejected
		}
		currentTaskID := boundary.currentTaskID
		access.CurrentTaskID = &currentTaskID
		access.Surface = SurfaceTask
	default:
		return TaskAccessContext{}, ErrTaskPolicyRejected
	}
	if access.Principal.Type == "" || strings.TrimSpace(access.Principal.ID) == "" {
		return TaskAccessContext{}, ErrTaskPolicyRejected
	}
	return access, nil
}

// ChannelTaskConversationID derives the non-sensitive task owner shared by
// invocations from exactly one provider conversation.
func ChannelTaskConversationID(organizationID, workspaceID, digitalHumanID uuid.UUID, provider, installationID, conversationID string) (uuid.UUID, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	installationID = strings.TrimSpace(installationID)
	conversationID = strings.TrimSpace(conversationID)
	if organizationID == uuid.Nil || workspaceID == uuid.Nil || digitalHumanID == uuid.Nil || provider == "" || installationID == "" || conversationID == "" {
		return uuid.Nil, ErrInvalidRequest
	}
	seed := strings.Join([]string{"zgi", "digital-human-task-conversation", organizationID.String(), workspaceID.String(), digitalHumanID.String(), provider, installationID, conversationID}, "\x00")
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(seed)), nil
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
			item, err := p.service.GetTask(ctx, boundary.access, boundary.currentTaskID)
			if err != nil {
				return taskToolFailure("task_not_found", false)
			}
			return taskToolSuccess(call, map[string]any{"view": taskListViewOpen, "scope": taskListScope(boundary), "items": []taskListToolItem{newTaskListToolItem(item)}, "total": 1, "has_more": false})
		}
		query, view, err := p.taskListQuery(args.View, args.Query, args.HistorySince, args.HistoryUntil, args.Limit)
		if err != nil {
			return taskToolFailure("invalid_arguments", true)
		}
		result, err := p.service.ListTasks(ctx, boundary.access, query)
		if err != nil {
			return taskToolFailure(taskErrorCode(err), false)
		}
		items := make([]taskListToolItem, 0, len(result.Items))
		for _, item := range result.Items {
			items = append(items, newTaskListToolItem(item))
		}
		payload := map[string]any{"view": view, "scope": taskListScope(boundary), "items": items, "total": result.Total, "has_more": result.HasMore}
		if view == taskListViewOpen && taskOccupancyEnabled(boundary) {
			if reader, ok := p.service.(TaskOccupancyReader); ok {
				occupancy, err := reader.ListTaskOccupancy(ctx, boundary.access, 0)
				if err != nil {
					return taskToolFailure(taskErrorCode(err), false)
				}
				payload["other_open_work"] = occupancy
			}
		}
		return taskToolSuccess(call, payload)
	}
	return skillloop.RuntimeTool{Definition: definition, SkillID: skills.SkillDigitalHumanTasks, Handler: handler}
}

type taskListToolScope struct {
	Visibility       string `json:"visibility"`
	ConversationType string `json:"conversation_type,omitempty"`
}

func taskListToolDescription(boundary taskRuntimeBoundary) string {
	description := "List this DigitalHuman's visible tasks. Omit view to return unfinished work. Use completed or canceled only when relevant history is needed."
	if boundary.conversationID != "" && (!TaskVisibilityEnabled() || boundary.access.Viewer.Mode == TaskViewChannel) {
		description += " This channel call returns only tasks from the current conversation, not the DigitalHuman's complete todo list."
	}
	return description
}

// taskOccupancyEnabled reports whether the list tool reports hidden open work.
// External end users never receive it.
func taskOccupancyEnabled(boundary taskRuntimeBoundary) bool {
	if !TaskVisibilityEnabled() || boundary.currentTaskID != uuid.Nil {
		return false
	}
	mode := boundary.access.Viewer.Mode
	return mode == TaskViewMember || mode == TaskViewChannel
}

func taskListScope(boundary taskRuntimeBoundary) taskListToolScope {
	if boundary.currentTaskID != uuid.Nil {
		return taskListToolScope{Visibility: "current_task"}
	}
	if TaskVisibilityEnabled() && boundary.access.Viewer.Mode == TaskViewMember {
		return taskListToolScope{Visibility: "current_user_and_shared"}
	}
	if boundary.conversationID != "" {
		return taskListToolScope{Visibility: "current_conversation", ConversationType: boundary.conversationType}
	}
	return taskListToolScope{Visibility: "current_user"}
}

// taskReadOnlyErrorCode reports a change request for a task the current user
// can see but did not request.
const taskReadOnlyErrorCode = "digital_human.task.read_only"

type taskListToolItem struct {
	ID            uuid.UUID      `json:"id"`
	CanChange     bool           `json:"can_change"`
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
	return taskListToolItem{ID: task.ID, CanChange: task.CanChange, Name: task.Name, Goal: task.Goal, WorkStatus: task.WorkStatus, Priority: task.Priority, NextRunAt: task.NextRunAt, LastRunAt: task.LastRunAt, LastRunStatus: task.LastRunStatus, Revision: task.Revision, UpdatedAt: task.UpdatedAt}
}

func (p *DefaultTaskRuntimeProvider) taskListQuery(view, keyword, historySince, historyUntil string, limit int) (TaskListQuery, string, error) {
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
	query := TaskListQuery{Statuses: statuses, Query: keyword, Page: 1, PageSize: limit}
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
		result, err := p.service.GetTask(ctx, boundary.access, args.TaskID)
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
	LifecycleStatus         string           `json:"lifecycle_status"`
}

func (p *DefaultTaskRuntimeProvider) manageTool(run *Run, boundary taskRuntimeBoundary) skillloop.RuntimeTool {
	description := "Create a task proposal that requires separate user approval, or manage an existing task. The create and propose actions are aliases. Existing-task actions require its ID and current revision. Use pause or resume to change whether a task is scheduled; use set_status only for the work status (waiting or blocked during a scheduled run)."
	definition := adapter.Tool{Type: "function", Function: adapter.Function{Name: taskManageToolName, Description: description, Parameters: map[string]any{"type": "object", "properties": map[string]any{
		"action": map[string]any{"type": "string", "enum": []string{"create", "propose", "confirm", "decline", "update", "set_status", "schedule", "run_now", "pause", "resume", "retry", "cancel", "archive"}}, "task_id": map[string]any{"type": "string", "format": "uuid"}, "expected_revision": map[string]any{"type": "integer", "minimum": 0}, "name": map[string]any{"type": "string"}, "goal": map[string]any{"type": "string"}, "success_criteria": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "priority": map[string]any{"type": "string", "enum": []string{"low", "normal", "high", "urgent"}}, "status": map[string]any{"type": "string", "enum": taskStatusValues()}, "lifecycle_status": map[string]any{"type": "string", "enum": []string{"paused", "active", "archived"}, "description": "When updating a paused task together with its schedule, set active to resume it in the same approved operation."}, "schedule": map[string]any{"type": "object", "properties": map[string]any{"type": map[string]any{"type": "string", "enum": []string{"manual", "once", "cron"}}, "run_at": map[string]any{"type": "string", "format": "date-time"}, "cron": map[string]any{"type": "string"}, "timezone": map[string]any{"type": "string"}}, "required": []string{"type"}, "additionalProperties": false}, "delivery_mode": map[string]any{"type": "string", "enum": []string{"task_only", "source_conversation"}}, "execution_timeout_seconds": map[string]any{"type": "integer", "minimum": 30, "maximum": 1800}, "max_attempts": map[string]any{"type": "integer", "minimum": 1, "maximum": 3},
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
			// Conversational creation always enters the approval state. Explicit
			// user intent is not enough to bypass confirmation because the same
			// model-facing tool is used by channels and external surfaces.
			mutation.Proposed = true
			if args.DeliveryMode == TaskDeliverySourceConversation &&
				boundary.replyTo == nil && !validTaskConversationID(boundary.conversationID) {
				// A source conversation is a durable ChatRuntime conversation for
				// console/API/WebApp runs, or a ReplyAddress for channels. If neither
				// is present, fall back safely to the task center instead of storing an
				// undeliverable target.
				args.DeliveryMode = TaskDeliveryTaskOnly
			}
			result, err := p.service.CreateTask(ctx, boundary.access, CreateTaskRequest{Name: args.Name, Goal: args.Goal, SuccessCriteria: args.SuccessCriteria, Priority: args.Priority, Schedule: valueSchedule(args.Schedule), DeliveryMode: args.DeliveryMode, ExecutionTimeoutSeconds: args.ExecutionTimeoutSeconds, MaxAttempts: args.MaxAttempts}, mutation)
			if err != nil {
				return taskToolFailure(taskErrorCode(err), errors.Is(err, ErrInvalidRequest))
			}
			return taskToolSuccessWithApproval(call, map[string]any{"status": "proposed", "approval_required": true, "task": result}, taskApprovalPayload(run, boundary, result, args, mutation.OperationID, "create"))
		}
		if args.TaskID == nil || *args.TaskID == uuid.Nil || args.ExpectedRevision < 1 || (boundary.currentTaskID != uuid.Nil && *args.TaskID != boundary.currentTaskID) {
			return taskToolFailure("invalid_arguments", true)
		}
		// Scheduled runs are system-owned and are the one mutation path that
		// cannot wait for a human approval. They may only move their current
		// task to a recoverable waiting/blocked state.
		if boundary.currentTaskID == uuid.Nil {
			result, err := p.service.GetTask(ctx, boundary.access, *args.TaskID)
			if err != nil {
				return taskToolFailure(taskErrorCode(err), false)
			}
			// A visible task requested by someone else is read-only. Never ask
			// the user to approve a change the approval could not apply.
			if !result.CanChange {
				return taskToolFailure(taskReadOnlyErrorCode, false)
			}
			approvalAction := taskApprovalActionForTask(args, result)
			return taskToolSuccessWithApproval(call,
				map[string]any{"status": "approval_required", "approval_required": true, "task": result},
				taskApprovalPayload(run, boundary, result, args, mutation.OperationID, approvalAction),
			)
		}
		var result *Task
		var err error
		switch args.Action {
		case "confirm":
			result, err = p.service.ApplyTaskAction(ctx, boundary.access, *args.TaskID, TaskActionConfirm, ApplyTaskActionRequest{ExpectedRevision: args.ExpectedRevision}, mutation)
		case "decline":
			result, err = p.service.ApplyTaskAction(ctx, boundary.access, *args.TaskID, TaskActionDecline, ApplyTaskActionRequest{ExpectedRevision: args.ExpectedRevision}, mutation)
		case "run_now":
			result, err = p.service.ApplyTaskAction(ctx, boundary.access, *args.TaskID, TaskActionRunNow, ApplyTaskActionRequest{ExpectedRevision: args.ExpectedRevision}, mutation)
		case "pause":
			result, err = p.service.ApplyTaskAction(ctx, boundary.access, *args.TaskID, TaskActionPause, ApplyTaskActionRequest{ExpectedRevision: args.ExpectedRevision}, mutation)
		case "resume":
			result, err = p.service.ApplyTaskAction(ctx, boundary.access, *args.TaskID, TaskActionResume, ApplyTaskActionRequest{ExpectedRevision: args.ExpectedRevision}, mutation)
		case "retry":
			result, err = p.service.ApplyTaskAction(ctx, boundary.access, *args.TaskID, TaskActionRetry, ApplyTaskActionRequest{ExpectedRevision: args.ExpectedRevision}, mutation)
		case "cancel":
			result, err = p.service.ApplyTaskAction(ctx, boundary.access, *args.TaskID, TaskActionCancel, ApplyTaskActionRequest{ExpectedRevision: args.ExpectedRevision}, mutation)
		case "archive":
			result, err = p.service.ApplyTaskAction(ctx, boundary.access, *args.TaskID, TaskActionArchive, ApplyTaskActionRequest{ExpectedRevision: args.ExpectedRevision}, mutation)
		case "set_status":
			result, err = p.service.SetTaskWorkStatus(ctx, boundary.access, *args.TaskID, args.Status, args.ExpectedRevision, mutation)
		case "schedule":
			if args.Schedule == nil {
				return taskToolFailure("invalid_arguments", true)
			}
			result, err = p.service.UpdateTask(ctx, boundary.access, *args.TaskID, UpdateTaskRequest{ExpectedRevision: args.ExpectedRevision, Schedule: args.Schedule}, mutation)
		case "update":
			result, err = p.service.UpdateTask(ctx, boundary.access, *args.TaskID, updateRequest(args), mutation)
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

// taskApprovalActionForTask keeps old model behavior compatible with the
// explicit lifecycle contract. A paused task cannot become runnable by merely
// changing its work status; pending/in-progress requests on a paused task are
// therefore treated as a resume proposal.
func taskApprovalActionForTask(args manageTaskArguments, task *Task) string {
	action := strings.ToLower(strings.TrimSpace(args.Action))
	if action == "set_status" && task != nil && task.LifecycleStatus == "paused" {
		switch args.Status {
		case TaskWorkStatusPending, TaskWorkStatusInProgress:
			return "resume"
		}
	}
	return action
}

func scheduledTaskMutationAllowed(args manageTaskArguments, currentTaskID uuid.UUID) bool {
	if args.Action != "set_status" || args.TaskID == nil || *args.TaskID != currentTaskID {
		return false
	}
	switch args.Status {
	case TaskWorkStatusWaiting, TaskWorkStatusBlocked:
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
	if args.Schedule != nil {
		req.Schedule = args.Schedule
	}
	return req
}
func valueSchedule(value *TaskSchedule) TaskSchedule {
	if value == nil {
		return TaskSchedule{Type: TaskScheduleManual, Timezone: "UTC"}
	}
	return *value
}

func validTaskConversationID(value string) bool {
	id, err := uuid.Parse(strings.TrimSpace(value))
	return err == nil && id != uuid.Nil
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

func taskToolSuccessWithApproval(call adapter.ToolCall, result any, approval map[string]interface{}) skillloop.RuntimeToolResult {
	return skillloop.RuntimeToolResult{
		Status:          "success",
		Arguments:       map[string]any{"tool": call.Function.Name},
		Result:          map[string]any{"data": result},
		PendingApproval: approval,
	}
}

func taskApprovalPayload(run *Run, boundary taskRuntimeBoundary, task *Task, args manageTaskArguments, operationID uuid.UUID, action string) map[string]interface{} {
	token := uuid.NewString()
	workflowRunID := "digital-human-task:" + task.ID.String() + ":" + token
	fields := []map[string]interface{}{}
	if action == "create" || action == "update" {
		fields = append(fields,
			map[string]interface{}{"key": "name", "label": "任务名称", "type": "text", "required": true},
			map[string]interface{}{"key": "goal", "label": "任务目标", "type": "textarea", "required": true},
			map[string]interface{}{"key": "supplement", "label": "补充要求", "type": "textarea", "required": false},
		)
	}
	defaults := map[string]interface{}{"name": task.Name, "goal": task.Goal}
	requestedInputs := taskApprovalRequestedInputs(args, action)
	augmentTaskApprovalRequestedInputs(run, task, action, requestedInputs)
	// Resuming a scheduler-blocked task changes both dimensions of state:
	// lifecycle paused -> active and work blocked -> pending. Keep the latter
	// in the approval preview so the user can see exactly what will be fixed.
	if strings.EqualFold(strings.TrimSpace(stringValue(requestedInputs["lifecycle_status"])), "active") &&
		task.WorkStatus == TaskWorkStatusBlocked {
		requestedInputs["work_status"] = TaskWorkStatusPending
	}
	for key, value := range requestedInputs {
		defaults[key] = value
	}
	if _, ok := defaults["schedule"]; !ok {
		defaults["schedule"] = task.Schedule
	}
	taskBefore, taskAfter, changes := taskApprovalDiff(task, requestedInputs, action)
	return map[string]interface{}{
		"kind":                "digital_human_task",
		"workflow_run_id":     workflowRunID,
		"approval_token":      token,
		"ui_approval_allowed": true,
		"task_id":             task.ID.String(),
		"task_name":           task.Name,
		"task_revision":       task.Revision,
		"task_action":         action,
		"operation_id":        operationID.String(),
		"source":              boundary.source,
		"owner_kind":          string(boundary.access.Owner.Kind),
		"owner_conversation_id": func() string {
			if boundary.access.Owner.Kind == TaskOwnerConversation {
				return boundary.access.Owner.ConversationID.String()
			}
			return ""
		}(),
		"task":             task,
		"requested_inputs": requestedInputs,
		"task_approval": map[string]interface{}{
			"operation":          taskApprovalOperation(action),
			"task_before":        taskBefore,
			"task_after":         taskAfter,
			"changes":            changes,
			"supplement_allowed": action == "create" || action == "update",
		},
		"approval_form": map[string]interface{}{
			"id":                      "digital-human-task-" + token,
			"token":                   token,
			"node_id":                 "digital_human_task",
			"node_title":              "任务审批",
			"content":                 "请确认这项任务操作，确认后才会生效。",
			"fields":                  fields,
			"actions":                 taskApprovalFormActions(action == "create" || action == "update"),
			"resolved_default_values": defaults,
			"task_approval": map[string]interface{}{
				"operation":          taskApprovalOperation(action),
				"task_before":        taskBefore,
				"task_after":         taskAfter,
				"changes":            changes,
				"supplement_allowed": action == "create" || action == "update",
			},
		},
	}
}

func taskApprovalFormActions(supplementAllowed bool) []map[string]interface{} {
	actions := []map[string]interface{}{
		{"id": "approve", "label": "确认并执行", "style": "primary"},
	}
	if supplementAllowed {
		actions = append(actions, map[string]interface{}{
			"id": "revise", "label": "补充要求并重新生成", "style": "secondary",
		})
	}
	actions = append(actions, map[string]interface{}{"id": "decline", "label": "取消", "style": "secondary"})
	return actions
}

func taskApprovalOperation(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "create", "propose":
		return "create"
	case "update", "schedule":
		return "update"
	case "cancel":
		return "cancel"
	case "run_now":
		return "run_now"
	case "pause":
		return "pause"
	case "resume":
		return "resume"
	case "retry":
		return "retry"
	case "archive":
		return "archive"
	case "set_status":
		return "status"
	case "confirm":
		return "confirm"
	case "decline":
		return "decline"
	default:
		return "action"
	}
}

func taskApprovalDiff(task *Task, requestedInputs map[string]interface{}, action string) (map[string]interface{}, map[string]interface{}, []map[string]interface{}) {
	if task == nil {
		return nil, nil, nil
	}
	before := taskApprovalTaskMap(task)
	after := make(map[string]interface{}, len(before))
	for key, value := range before {
		after[key] = value
	}
	if strings.EqualFold(taskApprovalOperation(action), "create") {
		return nil, after, nil
	}
	for key, value := range requestedInputs {
		if key == "supplement" || value == nil {
			continue
		}
		after[key] = value
	}
	// The stored next run belongs to the old schedule. The scheduler computes
	// the new one when the change is applied, so the preview must not present
	// the stale value as the outcome.
	nextRunRecalculated := false
	if _, requested := requestedInputs["next_run_at"]; !requested {
		beforeSchedule, _ := json.Marshal(before["schedule"])
		afterSchedule, _ := json.Marshal(after["schedule"])
		if string(beforeSchedule) != string(afterSchedule) {
			delete(after, "next_run_at")
			nextRunRecalculated = true
		}
	}
	changes := make([]map[string]interface{}, 0)
	labels := map[string]string{
		"name": "任务名称", "goal": "任务目标", "success_criteria": "成功标准",
		"priority": "优先级", "schedule": "执行计划", "delivery_mode": "结果去向",
		"execution_timeout_seconds": "超时时间", "max_attempts": "最大尝试次数",
		"work_status": "工作状态", "lifecycle_status": "运行状态", "next_run_at": "下次执行时间",
	}
	for key, label := range labels {
		if key == "next_run_at" && nextRunRecalculated {
			continue
		}
		beforeValue := before[key]
		afterValue := after[key]
		beforeJSON, _ := json.Marshal(beforeValue)
		afterJSON, _ := json.Marshal(afterValue)
		if string(beforeJSON) == string(afterJSON) {
			continue
		}
		changes = append(changes, map[string]interface{}{
			"field": key, "label": label, "before": beforeValue, "after": afterValue,
		})
	}
	return before, after, changes
}

func taskApprovalTaskMap(task *Task) map[string]interface{} {
	encoded, err := json.Marshal(task)
	if err != nil {
		return map[string]interface{}{}
	}
	result := map[string]interface{}{}
	if err := json.Unmarshal(encoded, &result); err != nil {
		return map[string]interface{}{}
	}
	return result
}

// taskApprovalRequestedInputs preserves the model's proposed mutation in the
// durable continuation. The task itself is intentionally not mutated for
// update/schedule/status actions until the user approves the continuation.
func taskApprovalRequestedInputs(args manageTaskArguments, action string) map[string]interface{} {
	inputs := map[string]interface{}{}
	switch action {
	case "create", "update":
		if strings.TrimSpace(args.Name) != "" {
			inputs["name"] = strings.TrimSpace(args.Name)
		}
		if strings.TrimSpace(args.Goal) != "" {
			inputs["goal"] = strings.TrimSpace(args.Goal)
		}
		if args.SuccessCriteria != nil {
			inputs["success_criteria"] = args.SuccessCriteria
		}
		if args.Priority != "" {
			inputs["priority"] = args.Priority
		}
		if args.Schedule != nil {
			inputs["schedule"] = args.Schedule
		}
		if args.DeliveryMode != "" {
			inputs["delivery_mode"] = args.DeliveryMode
		}
		if args.ExecutionTimeoutSeconds > 0 {
			inputs["execution_timeout_seconds"] = args.ExecutionTimeoutSeconds
		}
		if args.MaxAttempts > 0 {
			inputs["max_attempts"] = args.MaxAttempts
		}
		if strings.TrimSpace(args.LifecycleStatus) != "" {
			inputs["lifecycle_status"] = strings.TrimSpace(args.LifecycleStatus)
		}
	case "schedule":
		if args.Schedule != nil {
			inputs["schedule"] = args.Schedule
		}
	case "set_status":
		if args.Status != "" {
			inputs["work_status"] = args.Status
			// Keep the legacy key in the continuation so approvals created by
			// older clients remain readable by the apply path.
			inputs["status"] = args.Status
		}
	case "pause":
		inputs["lifecycle_status"] = "paused"
	case "resume":
		inputs["lifecycle_status"] = "active"
	case "retry":
		inputs["lifecycle_status"] = "active"
		inputs["work_status"] = string(TaskWorkStatusInProgress)
	case "archive":
		inputs["lifecycle_status"] = "archived"
	}
	return inputs
}

// augmentTaskApprovalRequestedInputs turns the common compound request
// "change the schedule and restore this paused task" into one approval plan.
// The model can provide lifecycle_status explicitly; the query fallback keeps
// older model snapshots from splitting the same user intent into two approvals.
func augmentTaskApprovalRequestedInputs(run *Run, task *Task, action string, inputs map[string]interface{}) {
	if run == nil || task == nil || inputs == nil || task.LifecycleStatus != "paused" {
		return
	}
	if action != "schedule" && action != "update" {
		return
	}
	if lifecycle := strings.TrimSpace(stringValue(inputs["lifecycle_status"])); lifecycle != "" {
		return
	}
	query := strings.ToLower(strings.TrimSpace(run.Query))
	if containsTaskResumeIntent(query) {
		inputs["lifecycle_status"] = "active"
	}
}

func containsTaskResumeIntent(query string) bool {
	for _, phrase := range []string{"恢复", "重新启动", "重新启用", "resume", "reactivate", "activate"} {
		if strings.Contains(query, phrase) &&
			!strings.Contains(query, "不要"+phrase) &&
			!strings.Contains(query, "不"+phrase) &&
			!strings.Contains(query, "别"+phrase) {
			return true
		}
	}
	return false
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
