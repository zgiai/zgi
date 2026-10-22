package digitalhumantask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	automationdefinition "github.com/zgiai/zgi/api/internal/modules/automation/service/definition"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	subjectTypeDigitalHuman = "digital_human"
	defaultPageSize         = 20
	maxPageSize             = 100
	defaultTimeoutSeconds   = 300
	proposalTTL             = 7 * 24 * time.Hour
)

// Service owns the DigitalHuman product projection over the Automation scheduler.
type Service struct {
	db         *gorm.DB
	dispatcher RunDispatcher
	now        func() time.Time
	runner     automationaction.DigitalHumanRunnerProvider
	// notifyChannels checks the team channels a task reports to.
	notifyChannels digitalhuman.TaskNotifyChannels
}

func NewService(db *gorm.DB, dispatcher RunDispatcher, runner automationaction.DigitalHumanRunnerProvider) *Service {
	if db == nil || dispatcher == nil {
		panic("digital human task service requires db and run dispatcher")
	}
	return &Service{db: db, dispatcher: dispatcher, runner: runner, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) PreviewTaskSchedule(_ context.Context, access digitalhuman.TaskAccessContext, request digitalhuman.TaskSchedulePreviewRequest) (*digitalhuman.TaskSchedulePreview, error) {
	if err := validateBoundary(access); err != nil {
		return nil, err
	}
	if len(request.Timezone) > 128 || len(request.Cron) > 256 || len(request.LocalRunAt) > 32 {
		return nil, digitalhuman.ErrInvalidRequest
	}
	timezone := strings.TrimSpace(request.Timezone)
	if timezone == "" {
		timezone = "UTC"
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, digitalhuman.ErrInvalidRequest
	}
	schedule := digitalhuman.TaskSchedule{Type: request.Type, Cron: strings.TrimSpace(request.Cron), Timezone: timezone}
	if request.Type == digitalhuman.TaskScheduleOnce {
		local := strings.TrimSpace(request.LocalRunAt)
		runAt, err := time.ParseInLocation("2006-01-02T15:04", local, location)
		if err != nil || runAt.In(location).Format("2006-01-02T15:04") != local {
			return nil, digitalhuman.ErrInvalidRequest
		}
		for offset := -3 * time.Hour; offset <= 3*time.Hour; offset += 15 * time.Minute {
			if offset != 0 && runAt.Add(offset).In(location).Format("2006-01-02T15:04") == local {
				return nil, digitalhuman.ErrInvalidRequest
			}
		}
		schedule.RunAt = &runAt
	} else if request.Type != digitalhuman.TaskScheduleCron {
		return nil, digitalhuman.ErrInvalidRequest
	}
	_, _, _, next, err := compileSchedule(schedule)
	if err != nil {
		return nil, digitalhuman.ErrInvalidRequest
	}
	return &digitalhuman.TaskSchedulePreview{Schedule: schedule, NextRunAt: next}, nil
}

func (s *Service) ListTasks(ctx context.Context, access digitalhuman.TaskAccessContext, query digitalhuman.TaskListQuery) (*digitalhuman.TaskListResult, error) {
	if err := validateBoundary(access); err != nil {
		return nil, err
	}
	page, pageSize := normalizePage(query.Page, query.PageSize)
	base := s.taskQuery(ctx, access)
	if len(query.Statuses) > 0 && query.IncludeUnreviewed {
		base = base.Where(`(work_status IN ? OR EXISTS (SELECT 1 FROM digital_human_task_reports AS reports
			WHERE reports.task_id = automation_tasks.id AND reports.final AND reports.review_state = ?))`, stringValues(query.Statuses), string(digitalhuman.TaskReportNew))
	} else if len(query.Statuses) > 0 {
		base = base.Where("work_status IN ?", stringValues(query.Statuses))
	}
	if len(query.Priorities) > 0 {
		base = base.Where("priority IN ?", stringValues(query.Priorities))
	}
	if query.ScheduleType != "" {
		base = base.Where("schedule_type = ?", query.ScheduleType)
	}
	if source := strings.TrimSpace(query.Source); source != "" {
		base = base.Where("source_ref = ?", source)
	}
	if reference := strings.TrimSpace(query.ClientReference); reference != "" {
		base = base.Where("client_reference = ?", reference)
	}
	if showsRequesters(access) {
		var err error
		if base, err = applyRequesterFilter(base, query); err != nil {
			return nil, err
		}
	}
	if keyword := strings.TrimSpace(query.Query); keyword != "" {
		like := "%" + keyword + "%"
		base = base.Where("LOWER(name) LIKE LOWER(?) OR LOWER(goal) LIKE LOWER(?)", like, like)
	}
	if query.UpdatedAfter != nil {
		base = base.Where("updated_at >= ?", query.UpdatedAfter.UTC())
	}
	if query.UpdatedBefore != nil {
		base = base.Where("updated_at <= ?", query.UpdatedBefore.UTC())
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("count digital human tasks: %w", err)
	}
	var rows []*automationmodel.AutomationTask
	if err := base.Order("updated_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list digital human tasks: %w", err)
	}
	counts := make(map[digitalhuman.TaskWorkStatus]int64)
	type countRow struct {
		Status string
		Count  int64
	}
	var countRows []countRow
	if err := s.taskQuery(ctx, access).Select("work_status AS status, COUNT(*) AS count").Group("work_status").Scan(&countRows).Error; err != nil {
		return nil, fmt.Errorf("count digital human tasks by status: %w", err)
	}
	for _, row := range countRows {
		counts[digitalhuman.TaskWorkStatus(row.Status)] = row.Count
	}
	successfulRunCount, err := s.countSuccessfulRuns(ctx, access, query)
	if err != nil {
		return nil, err
	}
	taskIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		taskIDs = append(taskIDs, row.ID)
	}
	reports, err := s.latestReports(ctx, taskIDs)
	if err != nil {
		return nil, err
	}
	items := make([]*digitalhuman.Task, 0, len(rows))
	for _, row := range rows {
		item, err := publicTaskForAccess(row, access)
		if err != nil {
			return nil, err
		}
		item.LatestReport = reports[row.ID]
		items = append(items, item)
	}
	if err := s.attachRequesters(ctx, access, rows, items); err != nil {
		return nil, err
	}
	return &digitalhuman.TaskListResult{Items: items, Counts: counts, SuccessfulRunCount: successfulRunCount, Total: total, Page: page, PageSize: pageSize, HasMore: int64(page*pageSize) < total}, nil
}

func (s *Service) countSuccessfulRuns(ctx context.Context, access digitalhuman.TaskAccessContext, query digitalhuman.TaskListQuery) (int64, error) {
	if query.RunFinishedAfter == nil && query.RunFinishedBefore == nil {
		return 0, nil
	}
	if query.RunFinishedAfter != nil && query.RunFinishedBefore != nil && !query.RunFinishedBefore.After(*query.RunFinishedAfter) {
		return 0, digitalhuman.ErrInvalidRequest
	}
	taskIDs := s.taskQuery(ctx, access).Select("id")
	runs := s.db.WithContext(ctx).Model(&automationmodel.AutomationTaskRun{}).
		Where("task_id IN (?) AND status = ?", taskIDs, automationmodel.AutomationTaskRunStatusSucceeded)
	if query.RunFinishedAfter != nil {
		runs = runs.Where("finished_at >= ?", query.RunFinishedAfter.UTC())
	}
	if query.RunFinishedBefore != nil {
		runs = runs.Where("finished_at < ?", query.RunFinishedBefore.UTC())
	}
	var count int64
	if err := runs.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count successful digital human task runs: %w", err)
	}
	return count, nil
}

func (s *Service) CreateTask(ctx context.Context, access digitalhuman.TaskAccessContext, req digitalhuman.CreateTaskRequest, mutation digitalhuman.TaskMutationContext) (*digitalhuman.Task, error) {
	if err := validateBoundary(access); err != nil {
		return nil, err
	}
	if access.CurrentTaskID != nil {
		return nil, digitalhuman.ErrTaskPolicyRejected
	}
	if err := validateCreate(req); err != nil {
		return nil, err
	}
	if mutation.OperationID == uuid.Nil {
		mutation.OperationID = uuid.New()
	}
	if existing, err := s.taskByOperation(ctx, access, mutation.OperationID); err == nil {
		return publicTask(existing)
	}
	scheduleType, timezone, config, nextRunAt, err := compileSchedule(req.Schedule)
	if err != nil {
		return nil, digitalhuman.ErrInvalidRequest
	}
	priority := automationmodel.AutomationTaskPriority(req.Priority)
	if priority == "" {
		priority = automationmodel.AutomationTaskPriorityNormal
	}
	workStatus := automationmodel.AutomationWorkStatusPending
	lifecycle := automationmodel.AutomationTaskStatusActive
	var proposalExpiresAt *time.Time
	if mutation.Proposed {
		workStatus, lifecycle, nextRunAt = automationmodel.AutomationWorkStatusProposed, automationmodel.AutomationTaskStatusDraft, nil
		expires := s.now().Add(proposalTTL)
		proposalExpiresAt = &expires
	}
	delivery, err := deliveryConfig(req.DeliveryMode, mutation)
	if err != nil {
		return nil, err
	}
	goal := strings.TrimSpace(req.Goal)
	subjectID, sourceOperationID := access.DigitalHumanID.String(), mutation.OperationID.String()
	sourceRef := strings.TrimSpace(mutation.Source)
	if sourceRef == "" {
		sourceRef = "console"
	}
	createdBy := accessActorID(access).String()
	ownerKind, ownerSubjectKind, ownerSubjectID, ownerConversationID := storedOwner(access.Owner)
	originSurface := strings.TrimSpace(access.Surface)
	visibility := string(digitalhuman.DefaultTaskVisibility(access.Owner.Kind))
	if len(req.NotifyChannelIDs) > 0 {
		ids, err := normalizeNotifyChannels(req.NotifyChannelIDs)
		if err != nil {
			return nil, err
		}
		if access.Owner.Kind != digitalhuman.TaskOwnerDigitalHuman || access.Surface != digitalhuman.SurfaceConsole ||
			visibility != string(digitalhuman.TaskVisibilityWorkspace) {
			return nil, digitalhuman.ErrInvalidRequest
		}
		if err := s.checkNotifyChannels(ctx, access.OrganizationID.String(), access.WorkspaceID.String(), ids); err != nil {
			return nil, err
		}
		delivery = withNotifyChannels(delivery, ids)
	}
	task := &automationmodel.AutomationTask{
		OrganizationID: access.OrganizationID.String(), WorkspaceID: access.WorkspaceID.String(), Name: strings.TrimSpace(req.Name),
		TaskKind: automationmodel.AutomationTaskKindDigitalHuman, SubjectType: stringPointer(subjectTypeDigitalHuman), SubjectID: &subjectID,
		OwnerKind: &ownerKind, OwnerSubjectKind: ownerSubjectKind, OwnerSubjectID: ownerSubjectID, OwnerConversationID: ownerConversationID, OriginSurface: &originSurface,
		Visibility: &visibility, WorkStatus: &workStatus, Priority: &priority, Goal: &goal, SuccessCriteria: cleanStrings(req.SuccessCriteria),
		ExecutionTimeoutSeconds: defaultInt(req.ExecutionTimeoutSeconds, defaultTimeoutSeconds), MaxAttempts: clampAttempts(req.MaxAttempts), Revision: 1,
		DeliveryConfig: delivery, SourceOperationID: &sourceOperationID, ProposalExpiresAt: proposalExpiresAt,
		Status: lifecycle, TriggerType: automationmodel.AutomationTriggerTypeSchedule, ScheduleType: scheduleType, Timezone: timezone,
		ScheduleConfig: config, NextRunAt: nextRunAt, SourceType: sourceType(mutation), SourceRef: &sourceRef,
		SourceSnapshot: sourceSnapshot(mutation), CreatedBy: createdBy, UpdatedBy: createdBy,
		ClientReference: clientReference(req.ClientReference),
	}
	action := &automationmodel.AutomationTaskAction{ActionType: automationmodel.AutomationActionTypeRunDigitalHuman, ActionOrder: 1, Enabled: true, Config: map[string]any{}}
	activity := newActivity(task.ID, mutation, "created", "Task created", map[string]any{"work_status": workStatus})
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureStoredOwner(ctx, tx, access.Owner); err != nil {
			return err
		}
		if err := tx.Create(task).Error; err != nil {
			return err
		}
		action.TaskID, activity.TaskID = task.ID, task.ID
		if err := tx.Create(action).Error; err != nil {
			return err
		}
		return tx.Create(activity).Error
	})
	if err != nil {
		if existing, loadErr := s.taskByOperation(ctx, access, mutation.OperationID); loadErr == nil {
			return publicTask(existing)
		}
		return nil, fmt.Errorf("create digital human task: %w", err)
	}
	return publicTask(task)
}

func (s *Service) GetTask(ctx context.Context, access digitalhuman.TaskAccessContext, taskID uuid.UUID) (*digitalhuman.Task, error) {
	task, err := s.loadTask(ctx, s.db, access, taskID, false)
	if err != nil {
		return nil, err
	}
	result, err := publicTaskForAccess(task, access)
	if err != nil {
		return nil, err
	}
	reports, err := s.latestReports(ctx, []string{task.ID})
	if err != nil {
		return nil, err
	}
	result.LatestReport = reports[task.ID]
	if err := s.attachRequesters(ctx, access, []*automationmodel.AutomationTask{task}, []*digitalhuman.Task{result}); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) UpdateTask(ctx context.Context, access digitalhuman.TaskAccessContext, taskID uuid.UUID, req digitalhuman.UpdateTaskRequest, mutation digitalhuman.TaskMutationContext) (*digitalhuman.Task, error) {
	if access.CurrentTaskID != nil {
		return nil, digitalhuman.ErrTaskPolicyRejected
	}
	if mutation.OperationID == uuid.Nil {
		mutation.OperationID = uuid.New()
	}
	if existing, err := s.taskByActivityOperation(ctx, access, mutation.OperationID); err == nil {
		if existing.ID != taskID.String() {
			return nil, digitalhuman.ErrTaskConflict
		}
		return publicTask(existing)
	}
	if req.ExpectedRevision <= 0 {
		return nil, digitalhuman.ErrInvalidRequest
	}
	var updated *automationmodel.AutomationTask
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		task, err := s.loadTask(ctx, tx, access, taskID, true)
		if err != nil {
			return err
		}
		if task.Revision != req.ExpectedRevision {
			return digitalhuman.ErrTaskConflict
		}
		changes := map[string]any{}
		currentChannels := taskNotifyChannels(task)
		if req.Visibility != nil {
			if !slices.Contains(digitalhuman.TaskVisibilityOptions(digitalhuman.TaskOwnerKind(pointerValue(task.OwnerKind))), *req.Visibility) {
				return digitalhuman.ErrInvalidRequest
			}
			if !slices.Contains(taskVisibilityChoices(task, access), *req.Visibility) {
				return digitalhuman.ErrTaskVisibilityForbidden
			}
			v := string(*req.Visibility)
			task.Visibility = &v
			changes["visibility"] = v
		}
		if req.Name != nil {
			v := strings.TrimSpace(*req.Name)
			if v == "" {
				return digitalhuman.ErrInvalidRequest
			}
			task.Name = v
			changes["name"] = v
		}
		if req.Goal != nil {
			v := strings.TrimSpace(*req.Goal)
			if v == "" {
				return digitalhuman.ErrInvalidRequest
			}
			task.Goal = &v
			changes["goal"] = v
		}
		if req.SuccessCriteria != nil {
			task.SuccessCriteria = cleanStrings(*req.SuccessCriteria)
			changes["success_criteria"] = task.SuccessCriteria
		}
		if req.Priority != nil {
			if !validPriority(*req.Priority) {
				return digitalhuman.ErrInvalidRequest
			}
			v := automationmodel.AutomationTaskPriority(*req.Priority)
			task.Priority = &v
			changes["priority"] = v
		}
		if req.ExecutionTimeoutSeconds != nil {
			if *req.ExecutionTimeoutSeconds < 30 || *req.ExecutionTimeoutSeconds > 1800 {
				return digitalhuman.ErrInvalidRequest
			}
			task.ExecutionTimeoutSeconds = *req.ExecutionTimeoutSeconds
			changes["execution_timeout_seconds"] = *req.ExecutionTimeoutSeconds
		}
		if req.MaxAttempts != nil {
			if *req.MaxAttempts < 1 || *req.MaxAttempts > 3 {
				return digitalhuman.ErrInvalidRequest
			}
			task.MaxAttempts = *req.MaxAttempts
			changes["max_attempts"] = *req.MaxAttempts
		}
		if req.ClientReference != nil {
			if !validClientReference(*req.ClientReference) {
				return digitalhuman.ErrInvalidRequest
			}
			task.ClientReference = clientReference(*req.ClientReference)
			changes["client_reference"] = pointerValue(task.ClientReference)
		}
		var requestedScheduleNext *time.Time
		if req.Schedule != nil {
			st, tz, cfg, next, err := compileSchedule(*req.Schedule)
			if err != nil {
				return digitalhuman.ErrInvalidRequest
			}
			task.ScheduleType, task.Timezone, task.ScheduleConfig = st, tz, cfg
			requestedScheduleNext = next
			if task.Status == automationmodel.AutomationTaskStatusActive {
				task.NextRunAt = next
			}
			changes["schedule"] = req.Schedule
		}
		if req.LifecycleStatus != nil {
			lifecycle := automationmodel.AutomationTaskStatus(strings.ToLower(strings.TrimSpace(*req.LifecycleStatus)))
			switch lifecycle {
			case automationmodel.AutomationTaskStatusActive:
				if task.Status == automationmodel.AutomationTaskStatusArchived || pointerValue(task.WorkStatus) == string(automationmodel.AutomationWorkStatusProposed) {
					return digitalhuman.ErrTaskPolicyRejected
				}
				// Reactivation is an explicit approved mutation. It is allowed for
				// completed/paused/canceled tasks and resets only the pending work
				// state; historical last-run fields remain intact.
				if pointerValue(task.WorkStatus) == string(automationmodel.AutomationWorkStatusCompleted) ||
					pointerValue(task.WorkStatus) == string(automationmodel.AutomationWorkStatusCanceled) ||
					pointerValue(task.WorkStatus) == string(automationmodel.AutomationWorkStatusBlocked) {
					workStatus := automationmodel.AutomationWorkStatusPending
					task.WorkStatus = &workStatus
				}
				task.Status = automationmodel.AutomationTaskStatusActive
				task.NextRunAt = requestedScheduleNext
				if req.Schedule == nil {
					next, err := automationdefinition.CompileNextRunAt(task.ScheduleType, task.Timezone, task.ScheduleConfig)
					if err != nil {
						return digitalhuman.ErrInvalidRequest
					}
					task.NextRunAt = next
				}
			case automationmodel.AutomationTaskStatusPaused:
				task.Status = automationmodel.AutomationTaskStatusPaused
				task.NextRunAt = nil
			case automationmodel.AutomationTaskStatusArchived:
				task.Status = automationmodel.AutomationTaskStatusArchived
				task.NextRunAt = nil
			default:
				return digitalhuman.ErrInvalidRequest
			}
			changes["lifecycle_status"] = lifecycle
		}
		if req.DeliveryMode != nil {
			cfg, err := deliveryConfig(*req.DeliveryMode, mutation)
			if err != nil {
				return err
			}
			if *req.DeliveryMode == digitalhuman.TaskDeliverySourceConversation && mutation.ReplyTo == nil {
				cfg = task.DeliveryConfig
				if deliveryMode(cfg) != digitalhuman.TaskDeliverySourceConversation {
					return digitalhuman.ErrTaskPolicyRejected
				}
			}
			task.DeliveryConfig, changes["delivery_mode"] = cfg, *req.DeliveryMode
		}
		channels := currentChannels
		if req.NotifyChannelIDs != nil {
			if channels, err = s.changeNotifyChannels(ctx, task, access, currentChannels, *req.NotifyChannelIDs); err != nil {
				return err
			}
		}
		// A task withdrawn from its workspace stops reporting to team channels.
		if storedTaskVisibility(task) != digitalhuman.TaskVisibilityWorkspace {
			channels = nil
		}
		if !slices.Equal(channels, currentChannels) {
			changes["notify_channel_ids"] = channels
		}
		task.DeliveryConfig = withNotifyChannels(task.DeliveryConfig, channels)
		task.Revision++
		task.UpdatedBy = accessActorID(access).String()
		values, err := taskUpdateValues(task, s.now())
		if err != nil {
			return err
		}
		result := tx.Model(&automationmodel.AutomationTask{}).
			Where("id = ? AND revision = ?", task.ID, req.ExpectedRevision).
			Updates(values)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return digitalhuman.ErrTaskConflict
		}
		if err := tx.Create(newActivity(task.ID, mutation, "updated", "Task updated", changes)).Error; err != nil {
			return err
		}
		updated = task
		return nil
	})
	if err != nil {
		return nil, normalizeError(err)
	}
	return publicTask(updated)
}

func (s *Service) ApplyTaskAction(ctx context.Context, access digitalhuman.TaskAccessContext, taskID uuid.UUID, action digitalhuman.TaskAction, req digitalhuman.ApplyTaskActionRequest, mutation digitalhuman.TaskMutationContext) (*digitalhuman.Task, error) {
	if access.CurrentTaskID != nil {
		return nil, digitalhuman.ErrTaskPolicyRejected
	}
	if mutation.OperationID == uuid.Nil {
		mutation.OperationID = uuid.New()
	}
	if existing, err := s.taskByActivityOperation(ctx, access, mutation.OperationID); err == nil {
		if existing.ID != taskID.String() {
			return nil, digitalhuman.ErrTaskConflict
		}
		return publicTask(existing)
	}
	if req.ExpectedRevision <= 0 {
		return nil, digitalhuman.ErrInvalidRequest
	}
	var result *automationmodel.AutomationTask
	var runID string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		task, err := s.loadTask(ctx, tx, access, taskID, true)
		if err != nil {
			return err
		}
		if task.Revision != req.ExpectedRevision {
			return digitalhuman.ErrTaskConflict
		}
		if err := applyTransition(task, action, s.now()); err != nil {
			return err
		}
		if action == digitalhuman.TaskActionConfirm {
			recordConfirmation(task, mutation, s.now())
		}
		if action == digitalhuman.TaskActionRunNow || action == digitalhuman.TaskActionRetry {
			run := &automationmodel.AutomationTaskRun{TaskID: task.ID, TriggerSource: automationmodel.AutomationTriggerSourceManualRun, ScheduledFor: s.now(), Status: automationmodel.AutomationTaskRunStatusQueued, RuntimeContext: map[string]any{"attempt": 1, "trigger_mode": string(action)}}
			if action == digitalhuman.TaskActionRetry {
				run.TriggerSource = automationmodel.AutomationTriggerSourceRetry
			}
			if err := tx.Create(run).Error; err != nil {
				return err
			}
			runID = run.ID
		}
		task.Revision++
		task.UpdatedBy = accessActorID(access).String()
		values, err := taskUpdateValues(task, s.now())
		if err != nil {
			return err
		}
		update := tx.Model(&automationmodel.AutomationTask{}).
			Where("id = ? AND revision = ?", task.ID, req.ExpectedRevision).
			Updates(values)
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return digitalhuman.ErrTaskConflict
		}
		if err := tx.Create(newActivity(task.ID, mutation, string(action), actionSummary(action), map[string]any{"work_status": task.WorkStatus, "status": task.Status})).Error; err != nil {
			return err
		}
		result = task
		return nil
	})
	if err != nil {
		return nil, normalizeError(err)
	}
	if runID != "" {
		if err := s.dispatcher.DispatchTaskRun(ctx, runID); err != nil {
			return nil, err
		}
	}
	if action == digitalhuman.TaskActionCancel {
		_ = s.cancelActiveRuns(ctx, access, taskID)
	}
	return publicTask(result)
}

func (s *Service) cancelActiveRuns(ctx context.Context, access digitalhuman.TaskAccessContext, taskID uuid.UUID) error {
	if s.runner == nil {
		return nil
	}
	runner := s.runner()
	if runner == nil {
		return nil
	}
	type active struct{ ExternalRunID string }
	var rows []active
	err := s.db.WithContext(ctx).Table("automation_action_runs ar").Select("ar.external_run_id").Joins("JOIN automation_task_runs tr ON tr.id = ar.task_run_id").Where("tr.task_id = ? AND ar.status = ? AND ar.external_run_type = ?", taskID, automationmodel.AutomationActionRunStatusRunning, subjectTypeDigitalHuman).Scan(&rows).Error
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.ExternalRunID != "" {
			if err := runner.CancelDigitalHumanRun(ctx, access.OrganizationID.String(), access.WorkspaceID.String(), access.DigitalHumanID.String(), row.ExternalRunID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) SetTaskWorkStatus(ctx context.Context, access digitalhuman.TaskAccessContext, taskID uuid.UUID, status digitalhuman.TaskWorkStatus, revision int64, mutation digitalhuman.TaskMutationContext) (*digitalhuman.Task, error) {
	if !validWorkStatus(status) {
		return nil, digitalhuman.ErrInvalidRequest
	}
	if access.CurrentTaskID != nil && status != digitalhuman.TaskWorkStatusWaiting && status != digitalhuman.TaskWorkStatusBlocked {
		return nil, digitalhuman.ErrTaskPolicyRejected
	}
	return s.setStatus(ctx, access, taskID, status, revision, mutation)
}

func (s *Service) setStatus(ctx context.Context, access digitalhuman.TaskAccessContext, taskID uuid.UUID, status digitalhuman.TaskWorkStatus, revision int64, mutation digitalhuman.TaskMutationContext) (*digitalhuman.Task, error) {
	var task *automationmodel.AutomationTask
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		loaded, err := s.loadTask(ctx, tx, access, taskID, true)
		if err != nil {
			return err
		}
		if loaded.Revision != revision {
			return digitalhuman.ErrTaskConflict
		}
		if loaded.ScheduleType == automationmodel.AutomationScheduleTypeCron &&
			loaded.Status == automationmodel.AutomationTaskStatusActive &&
			status == digitalhuman.TaskWorkStatusCompleted {
			return digitalhuman.ErrTaskPolicyRejected
		}
		value := automationmodel.AutomationWorkStatus(status)
		loaded.WorkStatus = &value
		loaded.Revision++
		loaded.UpdatedBy = accessActorID(access).String()
		values, err := taskUpdateValues(loaded, s.now())
		if err != nil {
			return err
		}
		update := tx.Model(&automationmodel.AutomationTask{}).
			Where("id = ? AND revision = ?", loaded.ID, revision).
			Updates(values)
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return digitalhuman.ErrTaskConflict
		}
		if err := tx.Create(newActivity(loaded.ID, mutation, "status_changed", "Task status changed", map[string]any{"work_status": status})).Error; err != nil {
			return err
		}
		task = loaded
		return nil
	})
	if err != nil {
		return nil, normalizeError(err)
	}
	return publicTask(task)
}

func (s *Service) ListTaskRuns(ctx context.Context, access digitalhuman.TaskAccessContext, taskID uuid.UUID, page, pageSize int) (*digitalhuman.TaskRunListResult, error) {
	task, err := s.loadTask(ctx, s.db, access, taskID, false)
	if err != nil {
		return nil, err
	}
	page, pageSize = normalizePage(page, pageSize)
	query := s.db.WithContext(ctx).Model(&automationmodel.AutomationTaskRun{}).Where("task_id = ?", taskID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []*automationmodel.AutomationTaskRun
	if err := query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]*digitalhuman.TaskRun, 0, len(rows))
	for _, row := range rows {
		item := &digitalhuman.TaskRun{ID: uuid.MustParse(row.ID), TaskID: uuid.MustParse(row.TaskID), TriggerSource: string(row.TriggerSource), Status: string(row.Status), ScheduledFor: row.ScheduledFor, StartedAt: row.StartedAt, FinishedAt: row.FinishedAt, CreatedAt: row.CreatedAt}
		if row.ErrorSummary != nil {
			item.ErrorSummary = safeResultSummary(*row.ErrorSummary)
		}
		if attempt, ok := numericValue(row.RuntimeContext["attempt"]); ok {
			item.Attempt = attempt
		} else {
			item.Attempt = 1
		}
		var actionRun automationmodel.AutomationActionRun
		if err := s.db.WithContext(ctx).Where("task_run_id = ? AND external_run_type = ?", row.ID, subjectTypeDigitalHuman).First(&actionRun).Error; err == nil && actionRun.ExternalRunID != nil {
			id, parseErr := uuid.Parse(*actionRun.ExternalRunID)
			if parseErr == nil {
				item.DigitalHumanRunID = &id
				var run digitalhuman.Run
				if runErr := s.db.WithContext(ctx).Where("id = ? AND organization_id = ? AND workspace_id = ? AND digital_human_id = ?", id, access.OrganizationID, access.WorkspaceID, access.DigitalHumanID).First(&run).Error; runErr == nil {
					item.ResultSummary = safeResultSummary(run.ResultSummary)
					if deliveryMode(task.DeliveryConfig) == digitalhuman.TaskDeliverySourceConversation {
						var delivery channelruntime.Delivery
						if deliveryErr := s.db.WithContext(ctx).
							Where("run_id = ? AND delivery_kind = ?", id, channelruntime.DeliveryKindFinal).
							Order("created_at DESC").First(&delivery).Error; deliveryErr == nil {
							item.ResultDeliveryStatus = string(delivery.Status)
							item.ResultDeliveryError = safeResultSummary(delivery.ErrorMessage)
						} else if run.Status == digitalhuman.RunStatusSucceeded {
							// Console, API, and WebApp deliveries are ChatRuntime
							// messages rather than channel_delivery rows. Project the
							// durable delivery activity into the same public status so
							// the task detail makes the result destination explicit.
							var activity automationmodel.AutomationTaskActivity
							activityErr := s.db.WithContext(ctx).
								Where("task_id = ? AND operation_id = ? AND action IN ?", row.TaskID, row.ID, []string{"result_delivered", "result_delivery_failed"}).
								Order("created_at DESC").First(&activity).Error
							if activityErr == nil && activity.Action == "result_delivered" {
								item.ResultDeliveryStatus = string(channelruntime.DeliverySucceeded)
							} else if activityErr == nil && activity.Action == "result_delivery_failed" {
								item.ResultDeliveryStatus = string(channelruntime.DeliveryFailed)
								item.ResultDeliveryError = "The source conversation delivery could not be created; the result remains in task details."
							} else if _, hasChannelReply := task.DeliveryConfig["reply_to"]; !hasChannelReply {
								item.ResultDeliveryStatus = string(channelruntime.DeliveryPending)
							} else {
								item.ResultDeliveryStatus = string(channelruntime.DeliveryFailed)
								item.ResultDeliveryError = "The source conversation delivery could not be created; the result remains in task details."
							}
						}
					}
				}
			}
		}
		items = append(items, item)
	}
	return &digitalhuman.TaskRunListResult{Items: items, Total: total, Page: page, PageSize: pageSize, HasMore: int64(page*pageSize) < total}, nil
}

const maxPublicTaskResultSummaryRunes = 8000

func safeResultSummary(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxPublicTaskResultSummaryRunes {
		return value
	}
	return string(runes[:maxPublicTaskResultSummaryRunes])
}

func (s *Service) ListTaskActivities(ctx context.Context, access digitalhuman.TaskAccessContext, taskID uuid.UUID, page, pageSize int) (*digitalhuman.TaskActivityListResult, error) {
	if _, err := s.loadTask(ctx, s.db, access, taskID, false); err != nil {
		return nil, err
	}
	page, pageSize = normalizePage(page, pageSize)
	query := s.db.WithContext(ctx).Model(&automationmodel.AutomationTaskActivity{}).Where("task_id = ?", taskID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []*automationmodel.AutomationTaskActivity
	if err := query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]*digitalhuman.TaskActivity, 0, len(rows))
	for _, row := range rows {
		items = append(items, &digitalhuman.TaskActivity{ID: uuid.MustParse(row.ID), ActorType: string(row.ActorType), ActorRef: row.ActorRef, Action: row.Action, Summary: row.Summary, Changes: row.Changes, CreatedAt: row.CreatedAt})
	}
	return &digitalhuman.TaskActivityListResult{Items: items, Total: total, Page: page, PageSize: pageSize, HasMore: int64(page*pageSize) < total}, nil
}

// maxTaskOccupancy bounds the hidden work summary handed to the model.
const maxTaskOccupancy = 20

// ListTaskOccupancy summarizes open, committed tasks the reader cannot see.
// It returns only scheduling fields so the DigitalHuman can reason about its
// workload without disclosing another requester's work (ADR 0005).
func (s *Service) ListTaskOccupancy(ctx context.Context, access digitalhuman.TaskAccessContext, limit int) ([]digitalhuman.TaskOccupancy, error) {
	if err := validateBoundary(access); err != nil {
		return nil, err
	}
	if access.CurrentTaskID != nil {
		return nil, nil
	}
	if access.Viewer.Mode != digitalhuman.TaskViewMember && access.Viewer.Mode != digitalhuman.TaskViewChannel {
		// External end users never learn about other requesters' work.
		return nil, nil
	}
	visible, args, restricted := visibleTaskPredicate(access)
	if !restricted {
		return nil, nil
	}
	if limit <= 0 || limit > maxTaskOccupancy {
		limit = maxTaskOccupancy
	}
	var rows []automationmodel.AutomationTask
	err := s.db.WithContext(ctx).Model(&automationmodel.AutomationTask{}).
		Select("next_run_at", "work_status", "status", "priority").
		Where("organization_id = ? AND workspace_id = ? AND task_kind = ? AND subject_type = ? AND subject_id = ? AND status <> ?", access.OrganizationID, access.WorkspaceID, automationmodel.AutomationTaskKindDigitalHuman, subjectTypeDigitalHuman, access.DigitalHumanID, automationmodel.AutomationTaskStatusArchived).
		Where("work_status NOT IN ?", []string{string(automationmodel.AutomationWorkStatusProposed), string(automationmodel.AutomationWorkStatusCompleted), string(automationmodel.AutomationWorkStatusCanceled)}).
		Where("NOT COALESCE(("+visible+"), FALSE)", args...).
		Order("next_run_at IS NULL, next_run_at ASC").Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list digital human task occupancy: %w", err)
	}
	items := make([]digitalhuman.TaskOccupancy, 0, len(rows))
	for _, row := range rows {
		items = append(items, digitalhuman.TaskOccupancy{
			NextRunAt: row.NextRunAt, WorkStatus: digitalhuman.TaskWorkStatus(pointerValue(row.WorkStatus)),
			LifecycleStatus: string(row.Status), Priority: digitalhuman.TaskPriority(pointerValue(row.Priority)),
		})
	}
	return items, nil
}

func (s *Service) taskQuery(ctx context.Context, access digitalhuman.TaskAccessContext) *gorm.DB {
	query := s.db.WithContext(ctx).Model(&automationmodel.AutomationTask{}).
		Where("organization_id = ? AND workspace_id = ? AND task_kind = ? AND subject_type = ? AND subject_id = ? AND status <> ?", access.OrganizationID, access.WorkspaceID, automationmodel.AutomationTaskKindDigitalHuman, subjectTypeDigitalHuman, access.DigitalHumanID, automationmodel.AutomationTaskStatusArchived)
	return applyReadScope(query, access)
}

func (s *Service) loadTask(ctx context.Context, db *gorm.DB, access digitalhuman.TaskAccessContext, taskID uuid.UUID, lock bool) (*automationmodel.AutomationTask, error) {
	if err := validateBoundary(access); err != nil || taskID == uuid.Nil {
		return nil, digitalhuman.ErrInvalidRequest
	}
	query := db.WithContext(ctx).Where("id = ? AND organization_id = ? AND workspace_id = ? AND task_kind = ? AND subject_type = ? AND subject_id = ?", taskID, access.OrganizationID, access.WorkspaceID, automationmodel.AutomationTaskKindDigitalHuman, subjectTypeDigitalHuman, access.DigitalHumanID)
	// Locked loads precede a change; unlocked loads only read.
	if lock {
		query = applyMutationScope(query, access).Clauses(clause.Locking{Strength: "UPDATE"})
	} else {
		query = applyReadScope(query, access)
	}
	var task automationmodel.AutomationTask
	if err := query.First(&task).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, digitalhuman.ErrTaskNotFound
	} else if err != nil {
		return nil, fmt.Errorf("load digital human task: %w", err)
	}
	return &task, nil
}

func (s *Service) taskByOperation(ctx context.Context, access digitalhuman.TaskAccessContext, operationID uuid.UUID) (*automationmodel.AutomationTask, error) {
	var task automationmodel.AutomationTask
	query := s.db.WithContext(ctx).Where("organization_id = ? AND workspace_id = ? AND task_kind = ? AND subject_id = ? AND source_operation_id = ?", access.OrganizationID, access.WorkspaceID, automationmodel.AutomationTaskKindDigitalHuman, access.DigitalHumanID, operationID)
	err := applyOwnerScope(query, access).First(&task).Error
	return &task, err
}

func (s *Service) taskByActivityOperation(ctx context.Context, access digitalhuman.TaskAccessContext, operationID uuid.UUID) (*automationmodel.AutomationTask, error) {
	if operationID == uuid.Nil {
		return nil, gorm.ErrRecordNotFound
	}
	var activity automationmodel.AutomationTaskActivity
	if err := s.db.WithContext(ctx).Where("operation_id = ?", operationID).First(&activity).Error; err != nil {
		return nil, err
	}
	taskID, err := uuid.Parse(activity.TaskID)
	if err != nil {
		return nil, gorm.ErrRecordNotFound
	}
	return s.loadTask(ctx, s.db, access, taskID, false)
}

func compileSchedule(schedule digitalhuman.TaskSchedule) (automationmodel.AutomationScheduleType, string, map[string]any, *time.Time, error) {
	typeValue := automationmodel.AutomationScheduleType(schedule.Type)
	if typeValue == "" {
		typeValue = automationmodel.AutomationScheduleTypeManual
	}
	timezone := strings.TrimSpace(schedule.Timezone)
	if timezone == "" {
		timezone = "UTC"
	}
	config := map[string]any{}
	switch typeValue {
	case automationmodel.AutomationScheduleTypeManual:
	case automationmodel.AutomationScheduleTypeOnce:
		if schedule.RunAt == nil {
			return "", "", nil, nil, digitalhuman.ErrInvalidRequest
		}
		config["run_at"] = schedule.RunAt.UTC().Format(time.RFC3339)
	case automationmodel.AutomationScheduleTypeCron:
		if strings.TrimSpace(schedule.Cron) == "" {
			return "", "", nil, nil, digitalhuman.ErrInvalidRequest
		}
		config["cron_expr"] = strings.TrimSpace(schedule.Cron)
	default:
		return "", "", nil, nil, digitalhuman.ErrInvalidRequest
	}
	next, err := automationdefinition.CompileNextRunAt(typeValue, timezone, config)
	return typeValue, timezone, config, next, err
}

// publicTaskForAccess projects a task read by access and reports whether that
// reader may change it. Visibility can show a task its reader cannot change.
func publicTaskForAccess(row *automationmodel.AutomationTask, access digitalhuman.TaskAccessContext) (*digitalhuman.Task, error) {
	task, err := publicTask(row)
	if err != nil {
		return nil, err
	}
	task.CanChange = access.CurrentTaskID != nil || taskCenterManager(access) || taskRequestedBy(row, access)
	task.VisibilityOptions = taskVisibilityChoices(row, access)
	task.NotifyChannelAccess = notifyChannelAccess(row, access)
	return task, nil
}

// taskVisibilityChoices lists the visibilities access may set on row. In the
// console a requester may share their task with the team or withdraw it. A
// task center manager may narrow anyone's task but never widen someone
// else's, which would publish that requester's work without them. External
// end users never share tasks with the team.
func taskVisibilityChoices(row *automationmodel.AutomationTask, access digitalhuman.TaskAccessContext) []digitalhuman.TaskVisibility {
	if access.CurrentTaskID != nil {
		return nil
	}
	requestedByReader := access.Surface == digitalhuman.SurfaceConsole && taskRequestedBy(row, access)
	if !requestedByReader && !taskCenterManager(access) {
		return nil
	}
	current := storedTaskVisibility(row)
	var choices []digitalhuman.TaskVisibility
	for _, option := range digitalhuman.TaskVisibilityOptions(digitalhuman.TaskOwnerKind(pointerValue(row.OwnerKind))) {
		if requestedByReader || option == current || !current.Widens(option) {
			choices = append(choices, option)
		}
	}
	return choices
}

// taskRequestedBy mirrors mutationOwnerPredicate for one loaded row.
func taskRequestedBy(row *automationmodel.AutomationTask, access digitalhuman.TaskAccessContext) bool {
	owner := access.Owner
	if pointerValue(row.OwnerKind) != string(owner.Kind) {
		return false
	}
	switch owner.Kind {
	case digitalhuman.TaskOwnerDigitalHuman:
		if row.OwnerSubjectID != nil || row.OwnerConversationID != nil {
			return false
		}
		return row.CreatedBy == accessActorID(access).String()
	case digitalhuman.TaskOwnerUserSubject:
		return pointerValue(row.OwnerSubjectKind) == string(owner.Subject.Kind) && pointerValue(row.OwnerSubjectID) == owner.Subject.ID.String()
	case digitalhuman.TaskOwnerConversation:
		return pointerValue(row.OwnerConversationID) == owner.ConversationID.String()
	default:
		return false
	}
}

func publicTask(row *automationmodel.AutomationTask) (*digitalhuman.Task, error) {
	id, err := uuid.Parse(row.ID)
	if err != nil {
		return nil, err
	}
	subjectID, err := uuid.Parse(pointerValue(row.SubjectID))
	if err != nil {
		return nil, err
	}
	ownerKind := digitalhuman.TaskOwnerKind(pointerValue(row.OwnerKind))
	if ownerKind != digitalhuman.TaskOwnerDigitalHuman && ownerKind != digitalhuman.TaskOwnerUserSubject && ownerKind != digitalhuman.TaskOwnerConversation {
		return nil, fmt.Errorf("invalid stored digital human task owner")
	}
	mode := deliveryMode(row.DeliveryConfig)
	result := &digitalhuman.Task{ID: id, DigitalHumanID: subjectID, OwnerKind: ownerKind, Visibility: storedTaskVisibility(row), ConfirmedAt: row.ConfirmedAt, Name: row.Name, Goal: pointerValue(row.Goal), SuccessCriteria: append([]string(nil), row.SuccessCriteria...), WorkStatus: digitalhuman.TaskWorkStatus(pointerValue(row.WorkStatus)), LifecycleStatus: string(row.Status), Priority: digitalhuman.TaskPriority(pointerValue(row.Priority)), Schedule: publicSchedule(row), DeliveryMode: mode, DeliveryTarget: deliveryTarget(row.DeliveryConfig), DeliveryFallback: deliveryFallback(row.DeliveryConfig), DeliveryReplyTo: replyAddress(row.DeliveryConfig), ExecutionTimeoutSeconds: row.ExecutionTimeoutSeconds, MaxAttempts: row.MaxAttempts, NextRunAt: row.NextRunAt, LastRunAt: row.LastRunAt, Revision: row.Revision, ProposalExpiresAt: row.ProposalExpiresAt, NotifyChannelIDs: taskNotifyChannels(row), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	if row.LastRunStatus != nil {
		result.LastRunStatus = *row.LastRunStatus
	}
	if row.SourceRef != nil {
		result.Source = *row.SourceRef
	}
	result.ClientReference = pointerValue(row.ClientReference)
	result.SourceConversationType = sourceConversationType(row)
	// Results of a change are always changeable by the caller that made it.
	result.CanChange = true
	return result, nil
}

func sourceConversationType(row *automationmodel.AutomationTask) string {
	if row == nil || !strings.HasPrefix(pointerValue(row.SourceRef), "channel:") {
		return ""
	}
	if value, ok := row.SourceSnapshot["conversation_type"].(string); ok {
		switch strings.TrimSpace(value) {
		case "direct", "group":
			return strings.TrimSpace(value)
		}
	}
	reply, ok := row.DeliveryConfig["reply_to"].(map[string]any)
	if !ok {
		return ""
	}
	if recipientID, _ := reply["recipient_id"].(string); strings.TrimSpace(recipientID) != "" {
		return "group"
	}
	return "direct"
}

func publicSchedule(row *automationmodel.AutomationTask) digitalhuman.TaskSchedule {
	result := digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleType(row.ScheduleType), Timezone: row.Timezone}
	if row.ScheduleType == automationmodel.AutomationScheduleTypeOnce {
		if raw, ok := row.ScheduleConfig["run_at"].(string); ok {
			if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
				result.RunAt = &parsed
			}
		}
	}
	if row.ScheduleType == automationmodel.AutomationScheduleTypeCron {
		result.Cron, _ = row.ScheduleConfig["cron_expr"].(string)
	}
	return result
}

func deliveryConfig(mode digitalhuman.TaskDeliveryMode, mutation digitalhuman.TaskMutationContext) (map[string]any, error) {
	if mode == "" {
		if mutation.ReplyTo != nil || validConversationID(mutation.SourceConversationID) {
			mode = digitalhuman.TaskDeliverySourceConversation
		} else {
			mode = digitalhuman.TaskDeliveryTaskOnly
		}
	}
	config := map[string]any{"mode": string(mode)}
	switch mode {
	case digitalhuman.TaskDeliveryTaskOnly:
		config["target"] = string(digitalhuman.TaskDeliveryTargetTaskCenter)
		return config, nil
	case digitalhuman.TaskDeliverySourceConversation:
		if mutation.ReplyTo != nil {
			encoded, err := json.Marshal(mutation.ReplyTo)
			if err != nil {
				return nil, err
			}
			var reply map[string]any
			if err := json.Unmarshal(encoded, &reply); err != nil {
				return nil, err
			}
			config["reply_to"] = reply
		} else if validConversationID(mutation.SourceConversationID) {
			// Console, WebApp, and API runs do not have a channel ReplyAddress.
			// Persist the server-captured ChatRuntime conversation separately so
			// the scheduler can append the completed result there.
			config["conversation_id"] = strings.TrimSpace(mutation.SourceConversationID)
		} else {
			return nil, digitalhuman.ErrTaskPolicyRejected
		}
		config["target"] = string(digitalhuman.TaskDeliveryTargetSourceConversation)
		config["fallback"] = string(digitalhuman.TaskDeliveryFallbackTaskCenter)
		return config, nil
	default:
		return nil, digitalhuman.ErrInvalidRequest
	}
}

func validConversationID(value string) bool {
	id, err := uuid.Parse(strings.TrimSpace(value))
	return err == nil && id != uuid.Nil
}

func deliveryMode(config map[string]any) digitalhuman.TaskDeliveryMode {
	if value, ok := config["mode"].(string); ok {
		return digitalhuman.TaskDeliveryMode(value)
	}
	return digitalhuman.TaskDeliveryTaskOnly
}

func replyAddress(config map[string]any) *invocation.ReplyAddress {
	if deliveryMode(config) != digitalhuman.TaskDeliverySourceConversation {
		return nil
	}
	raw, ok := config["reply_to"]
	if !ok {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var reply invocation.ReplyAddress
	if err := json.Unmarshal(encoded, &reply); err != nil || strings.TrimSpace(reply.Provider) == "" || strings.TrimSpace(reply.ConversationID) == "" {
		return nil
	}
	return &reply
}

func deliveryTarget(config map[string]any) digitalhuman.TaskDeliveryTarget {
	if value, ok := config["target"].(string); ok {
		switch digitalhuman.TaskDeliveryTarget(value) {
		case digitalhuman.TaskDeliveryTargetTaskCenter, digitalhuman.TaskDeliveryTargetSourceConversation:
			return digitalhuman.TaskDeliveryTarget(value)
		}
	}
	if deliveryMode(config) == digitalhuman.TaskDeliverySourceConversation {
		return digitalhuman.TaskDeliveryTargetSourceConversation
	}
	return digitalhuman.TaskDeliveryTargetTaskCenter
}

func deliveryFallback(config map[string]any) digitalhuman.TaskDeliveryFallback {
	if value, ok := config["fallback"].(string); ok && value == string(digitalhuman.TaskDeliveryFallbackTaskCenter) {
		return digitalhuman.TaskDeliveryFallbackTaskCenter
	}
	if deliveryMode(config) == digitalhuman.TaskDeliverySourceConversation {
		return digitalhuman.TaskDeliveryFallbackTaskCenter
	}
	return ""
}
func sourceSnapshot(m digitalhuman.TaskMutationContext) map[string]any {
	result := map[string]any{}
	if m.SourceConversationID != "" {
		result["conversation_id"] = m.SourceConversationID
	}
	if m.ReplyTo != nil {
		result["installation_id"] = m.ReplyTo.InstallationID
	}
	if m.SourceConversationType == "direct" || m.SourceConversationType == "group" {
		result["conversation_type"] = m.SourceConversationType
	}
	return result
}
func sourceType(m digitalhuman.TaskMutationContext) automationmodel.AutomationSourceType {
	if m.SourceRunID != nil {
		return automationmodel.AutomationSourceTypeLLMTool
	}
	return automationmodel.AutomationSourceTypeManual
}
func newActivity(taskID string, m digitalhuman.TaskMutationContext, action, summary string, changes map[string]any) *automationmodel.AutomationTaskActivity {
	actor := automationmodel.AutomationTaskActivityActor(m.ActorType)
	if actor == "" {
		actor = automationmodel.AutomationTaskActivityActorAccount
	}
	actorRef := strings.TrimSpace(m.ActorRef)
	if actorRef == "" {
		actorRef = "system"
	}
	operationID := m.OperationID
	if operationID == uuid.Nil {
		operationID = uuid.New()
	}
	result := &automationmodel.AutomationTaskActivity{TaskID: taskID, OperationID: operationID.String(), ActorType: actor, ActorRef: actorRef, Action: action, Summary: summary, Changes: changes}
	if m.SourceRunID != nil {
		value := m.SourceRunID.String()
		result.SourceRunID = &value
	}
	return result
}

func applyTransition(task *automationmodel.AutomationTask, action digitalhuman.TaskAction, now time.Time) error {
	current := automationmodel.AutomationWorkStatus(pointerValue(task.WorkStatus))
	switch action {
	case digitalhuman.TaskActionConfirm:
		if current != automationmodel.AutomationWorkStatusProposed {
			return digitalhuman.ErrTaskPolicyRejected
		}
		next, err := automationdefinition.CompileNextRunAt(task.ScheduleType, task.Timezone, task.ScheduleConfig)
		if err != nil {
			return err
		}
		value := automationmodel.AutomationWorkStatusPending
		task.WorkStatus, task.Status, task.NextRunAt, task.ProposalExpiresAt = &value, automationmodel.AutomationTaskStatusActive, next, nil
	case digitalhuman.TaskActionDecline, digitalhuman.TaskActionCancel:
		value := automationmodel.AutomationWorkStatusCanceled
		task.WorkStatus, task.Status, task.NextRunAt = &value, automationmodel.AutomationTaskStatusPaused, nil
	case digitalhuman.TaskActionPause:
		if task.Status == automationmodel.AutomationTaskStatusArchived {
			return digitalhuman.ErrTaskPolicyRejected
		}
		task.Status, task.NextRunAt = automationmodel.AutomationTaskStatusPaused, nil
	case digitalhuman.TaskActionResume:
		if current == automationmodel.AutomationWorkStatusCanceled || current == automationmodel.AutomationWorkStatusCompleted {
			return digitalhuman.ErrTaskPolicyRejected
		}
		next, err := automationdefinition.CompileNextRunAt(task.ScheduleType, task.Timezone, task.ScheduleConfig)
		if err != nil {
			return err
		}
		// A blocked task is paused by the scheduler after an execution
		// preflight/runtime failure. Resuming it must clear the recoverable
		// work-state as well as the lifecycle pause; otherwise the scheduler
		// keeps returning the task as blocked even though it has a new run time.
		if current == automationmodel.AutomationWorkStatusBlocked {
			value := automationmodel.AutomationWorkStatusPending
			task.WorkStatus = &value
		}
		task.Status, task.NextRunAt = automationmodel.AutomationTaskStatusActive, next
	case digitalhuman.TaskActionRunNow, digitalhuman.TaskActionRetry:
		if current == automationmodel.AutomationWorkStatusProposed || current == automationmodel.AutomationWorkStatusCanceled {
			return digitalhuman.ErrTaskPolicyRejected
		}
		value := automationmodel.AutomationWorkStatusInProgress
		task.WorkStatus, task.Status = &value, automationmodel.AutomationTaskStatusActive
	case digitalhuman.TaskActionArchive:
		task.Status, task.NextRunAt = automationmodel.AutomationTaskStatusArchived, nil
	default:
		return digitalhuman.ErrInvalidRequest
	}
	_ = now
	return nil
}

// recordConfirmation stores who approved a proposal. Activities keep the full
// history; the task row keeps the approval used as the task's source.
func recordConfirmation(task *automationmodel.AutomationTask, mutation digitalhuman.TaskMutationContext, now time.Time) {
	actorType, actorRef := strings.TrimSpace(mutation.ActorType), strings.TrimSpace(mutation.ActorRef)
	if actorType == "" || actorRef == "" {
		return
	}
	confirmedAt := now
	task.ConfirmedByType, task.ConfirmedByRef, task.ConfirmedAt = &actorType, &actorRef, &confirmedAt
}

// storedTaskVisibility resolves rows written before visibility existed.
func storedTaskVisibility(row *automationmodel.AutomationTask) digitalhuman.TaskVisibility {
	if visibility := digitalhuman.TaskVisibility(pointerValue(row.Visibility)); visibility.Valid() {
		return visibility
	}
	return digitalhuman.DefaultTaskVisibility(digitalhuman.TaskOwnerKind(pointerValue(row.OwnerKind)))
}

func validateBoundary(access digitalhuman.TaskAccessContext) error {
	if access.OrganizationID == uuid.Nil || access.WorkspaceID == uuid.Nil || access.DigitalHumanID == uuid.Nil ||
		access.Principal.Type == "" || strings.TrimSpace(access.Principal.ID) == "" || strings.TrimSpace(access.Surface) == "" {
		return digitalhuman.ErrInvalidRequest
	}
	if access.CurrentTaskID != nil {
		if *access.CurrentTaskID == uuid.Nil || access.Surface != digitalhuman.SurfaceTask || access.Principal.Type != "system" {
			return digitalhuman.ErrTaskPolicyRejected
		}
		return nil
	}
	if !access.Owner.Valid() {
		return digitalhuman.ErrInvalidRequest
	}
	return nil
}

func applyOwnerScope(query *gorm.DB, access digitalhuman.TaskAccessContext) *gorm.DB {
	if access.CurrentTaskID != nil {
		return query.Where("id = ?", *access.CurrentTaskID)
	}
	clause, args := ownerPredicate(access.Owner)
	return query.Where(clause, args...)
}

// ownerPredicate matches the requester namespace a task was created in.
func ownerPredicate(owner digitalhuman.TaskOwnerRef) (string, []any) {
	switch owner.Kind {
	case digitalhuman.TaskOwnerDigitalHuman:
		return "owner_kind = ? AND owner_subject_id IS NULL AND owner_conversation_id IS NULL", []any{automationmodel.AutomationTaskOwnerDigitalHuman}
	case digitalhuman.TaskOwnerUserSubject:
		return "owner_kind = ? AND owner_subject_kind = ? AND owner_subject_id = ? AND owner_conversation_id IS NULL", []any{automationmodel.AutomationTaskOwnerUserSubject, owner.Subject.Kind, owner.Subject.ID}
	case digitalhuman.TaskOwnerConversation:
		return "owner_kind = ? AND owner_subject_id IS NULL AND owner_conversation_id = ?", []any{automationmodel.AutomationTaskOwnerConversation, owner.ConversationID}
	default:
		return "1 = 0", nil
	}
}

// workspaceVisiblePredicate matches tasks every workspace member may read.
// Rows written before visibility existed fall back to their owner kind. It is
// NULL-safe so that its negation is too.
const workspaceVisiblePredicate = "(COALESCE(visibility, CASE WHEN owner_kind = 'digital_human' THEN 'workspace' ELSE '' END) = 'workspace')"

// visibleTaskPredicate expresses the read visibility rules of ADR 0005. ok is
// false when the reader sees every task of the DigitalHuman.
func visibleTaskPredicate(access digitalhuman.TaskAccessContext) (clause string, args []any, ok bool) {
	if access.CurrentTaskID != nil {
		clause, args = ownerPredicate(access.Owner)
		return clause, args, true
	}
	switch access.Viewer.Mode {
	case digitalhuman.TaskViewTaskCenter:
		if access.Viewer.Manager {
			return "", nil, false
		}
		// A shared task's requester is its creator, who keeps seeing it even
		// after a manager restricts it.
		requested, requestedArgs := mutationOwnerPredicate(access)
		return "(" + workspaceVisiblePredicate + " OR (" + requested + "))", requestedArgs, true
	case digitalhuman.TaskViewMember:
		owner, ownerArgs := ownerPredicate(access.Owner)
		return "((" + owner + ") OR " + workspaceVisiblePredicate + ")", ownerArgs, true
	default:
		// Channel and external readers keep exactly their requester namespace.
		clause, args = ownerPredicate(access.Owner)
		return clause, args, true
	}
}

// applyReadScope limits reads to the tasks the reader may see.
func applyReadScope(query *gorm.DB, access digitalhuman.TaskAccessContext) *gorm.DB {
	if access.CurrentTaskID != nil {
		return query.Where("id = ?", *access.CurrentTaskID)
	}
	clause, args, ok := visibleTaskPredicate(access)
	if !ok {
		return query
	}
	return query.Where(clause, args...)
}

// applyMutationScope limits changes to the requester namespace. Only task
// center managers may change tasks requested by someone else.
func applyMutationScope(query *gorm.DB, access digitalhuman.TaskAccessContext) *gorm.DB {
	if taskCenterManager(access) {
		return query
	}
	if access.CurrentTaskID != nil {
		return query.Where("id = ?", *access.CurrentTaskID)
	}
	clause, args := mutationOwnerPredicate(access)
	return query.Where(clause, args...)
}

// mutationOwnerPredicate matches the tasks this caller requested. A shared
// task's requester is the account that created it, so a non-manager in the
// task center cannot change another account's shared or restricted task.
func mutationOwnerPredicate(access digitalhuman.TaskAccessContext) (string, []any) {
	clause, args := ownerPredicate(access.Owner)
	if access.Owner.Kind == digitalhuman.TaskOwnerDigitalHuman {
		return clause + " AND created_by = ?", append(args, accessActorID(access).String())
	}
	return clause, args
}

func taskCenterManager(access digitalhuman.TaskAccessContext) bool {
	return access.CurrentTaskID == nil &&
		access.Viewer.Mode == digitalhuman.TaskViewTaskCenter && access.Viewer.Manager
}

func storedOwner(owner digitalhuman.TaskOwnerRef) (automationmodel.AutomationTaskOwnerKind, *string, *string, *string) {
	kind := automationmodel.AutomationTaskOwnerKind(owner.Kind)
	switch owner.Kind {
	case digitalhuman.TaskOwnerUserSubject:
		subjectKind, subjectID := string(owner.Subject.Kind), owner.Subject.ID.String()
		return kind, &subjectKind, &subjectID, nil
	case digitalhuman.TaskOwnerConversation:
		conversationID := owner.ConversationID.String()
		return kind, nil, nil, &conversationID
	default:
		return kind, nil, nil, nil
	}
}

func ensureStoredOwner(ctx context.Context, tx *gorm.DB, owner digitalhuman.TaskOwnerRef) error {
	if owner.Kind != digitalhuman.TaskOwnerUserSubject {
		return nil
	}
	registry := usersubject.NewService(tx)
	if owner.Subject.Kind == usersubject.Account {
		_, err := registry.Ensure(ctx, usersubject.EnsureRequest{Subject: owner.Subject})
		return err
	}
	_, err := registry.Require(ctx, owner.Subject)
	return err
}

func accessActorID(access digitalhuman.TaskAccessContext) uuid.UUID {
	if id, err := uuid.Parse(strings.TrimSpace(access.Principal.ID)); err == nil && id != uuid.Nil {
		return id
	}
	return access.DigitalHumanID
}
func validateCreate(req digitalhuman.CreateTaskRequest) error {
	invalidTimeout := req.ExecutionTimeoutSeconds != 0 && (req.ExecutionTimeoutSeconds < 30 || req.ExecutionTimeoutSeconds > 1800)
	invalidAttempts := req.MaxAttempts != 0 && (req.MaxAttempts < 1 || req.MaxAttempts > 3)
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Goal) == "" || (req.Priority != "" && !validPriority(req.Priority)) || invalidTimeout || invalidAttempts ||
		!validClientReference(req.ClientReference) {
		return digitalhuman.ErrInvalidRequest
	}
	return nil
}

// maxClientReferenceRunes bounds an integrator's task identifier.
const maxClientReferenceRunes = 255

// validClientReference accepts an empty value or printable text that fits the
// column; control characters would break logs and webhook consumers.
func validClientReference(value string) bool {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) > maxClientReferenceRunes {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func clientReference(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

// taskUpdateValues deliberately includes nullable scheduling fields. GORM's
// struct Updates omits zero values, which would otherwise leave a stale next
// run or proposal expiry behind when a task is paused, confirmed, or canceled.
func taskUpdateValues(task *automationmodel.AutomationTask, updatedAt time.Time) (map[string]any, error) {
	successCriteria, err := json.Marshal(task.SuccessCriteria)
	if err != nil {
		return nil, fmt.Errorf("encode task success criteria: %w", err)
	}
	deliveryConfig, err := json.Marshal(task.DeliveryConfig)
	if err != nil {
		return nil, fmt.Errorf("encode task delivery config: %w", err)
	}
	scheduleConfig, err := json.Marshal(task.ScheduleConfig)
	if err != nil {
		return nil, fmt.Errorf("encode task schedule config: %w", err)
	}
	return map[string]any{
		"name":                      task.Name,
		"goal":                      task.Goal,
		"success_criteria":          datatypes.JSON(successCriteria),
		"priority":                  task.Priority,
		"execution_timeout_seconds": task.ExecutionTimeoutSeconds,
		"max_attempts":              task.MaxAttempts,
		"client_reference":          task.ClientReference,
		"delivery_config":           datatypes.JSON(deliveryConfig),
		"status":                    task.Status,
		"work_status":               task.WorkStatus,
		"visibility":                task.Visibility,
		"confirmed_by_type":         task.ConfirmedByType,
		"confirmed_by_ref":          task.ConfirmedByRef,
		"confirmed_at":              task.ConfirmedAt,
		"schedule_type":             task.ScheduleType,
		"timezone":                  task.Timezone,
		"schedule_config":           datatypes.JSON(scheduleConfig),
		"next_run_at":               task.NextRunAt,
		"proposal_expires_at":       task.ProposalExpiresAt,
		"revision":                  task.Revision,
		"updated_by":                task.UpdatedBy,
		"updated_at":                updatedAt,
	}, nil
}
func validPriority(value digitalhuman.TaskPriority) bool {
	return value == digitalhuman.TaskPriorityLow || value == digitalhuman.TaskPriorityNormal || value == digitalhuman.TaskPriorityHigh || value == digitalhuman.TaskPriorityUrgent
}
func validWorkStatus(value digitalhuman.TaskWorkStatus) bool {
	switch value {
	case digitalhuman.TaskWorkStatusPending, digitalhuman.TaskWorkStatusInProgress, digitalhuman.TaskWorkStatusWaiting, digitalhuman.TaskWorkStatusBlocked, digitalhuman.TaskWorkStatusCompleted, digitalhuman.TaskWorkStatusCanceled:
		return true
	}
	return false
}
func actionSummary(action digitalhuman.TaskAction) string {
	return "Task " + strings.ReplaceAll(string(action), "_", " ")
}
func cleanStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}
func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return page, pageSize
}
func stringValues[T ~string](values []T) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, string(value))
	}
	return result
}
func defaultInt(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}
func clampAttempts(value int) int {
	if value == 0 {
		return 1
	}
	return value
}
func stringPointer(value string) *string { return &value }
func pointerValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
func numericValue(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case float64:
		return int(typed), true
	case json.Number:
		v, err := typed.Int64()
		return int(v), err == nil
	}
	return 0, false
}
func normalizeError(err error) error {
	if errors.Is(err, digitalhuman.ErrTaskConflict) || errors.Is(err, digitalhuman.ErrTaskNotFound) || errors.Is(err, digitalhuman.ErrTaskPolicyRejected) || errors.Is(err, digitalhuman.ErrInvalidRequest) {
		return err
	}
	return fmt.Errorf("digital human task operation: %w", err)
}
