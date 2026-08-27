package digitalhumantask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
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

type channelTaskPolicy struct {
	InstallationID uuid.UUID `gorm:"type:uuid;primaryKey"`
	OrganizationID uuid.UUID `gorm:"type:uuid;not null"`
	WorkspaceID    uuid.UUID `gorm:"type:uuid;not null"`
	DigitalHumanID uuid.UUID `gorm:"type:uuid;not null"`
	Enabled        bool      `gorm:"not null"`
	Revision       int64     `gorm:"not null"`
	UpdatedBy      uuid.UUID `gorm:"type:uuid;not null"`
	CreatedAt      time.Time `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"not null"`
}

func (channelTaskPolicy) TableName() string { return "digital_human_channel_task_policies" }

// Service owns the DigitalHuman product projection over the Automation scheduler.
type Service struct {
	db         *gorm.DB
	dispatcher RunDispatcher
	now        func() time.Time
	runner     automationaction.DigitalHumanRunnerProvider
}

func NewService(db *gorm.DB, dispatcher RunDispatcher, runner automationaction.DigitalHumanRunnerProvider) *Service {
	if db == nil || dispatcher == nil {
		panic("digital human task service requires db and run dispatcher")
	}
	return &Service{db: db, dispatcher: dispatcher, runner: runner, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) ListTasks(ctx context.Context, scope digitalhuman.Scope, digitalHumanID uuid.UUID, query digitalhuman.TaskListQuery) (*digitalhuman.TaskListResult, error) {
	if err := validateBoundary(scope, digitalHumanID); err != nil {
		return nil, err
	}
	page, pageSize := normalizePage(query.Page, query.PageSize)
	base := s.taskQuery(ctx, scope, digitalHumanID, query.SourceConversationID)
	if len(query.Statuses) > 0 {
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
	if err := s.taskQuery(ctx, scope, digitalHumanID, query.SourceConversationID).Select("work_status AS status, COUNT(*) AS count").Group("work_status").Scan(&countRows).Error; err != nil {
		return nil, fmt.Errorf("count digital human tasks by status: %w", err)
	}
	for _, row := range countRows {
		counts[digitalhuman.TaskWorkStatus(row.Status)] = row.Count
	}
	successfulRunCount, err := s.countSuccessfulRuns(ctx, scope, digitalHumanID, query)
	if err != nil {
		return nil, err
	}
	items := make([]*digitalhuman.Task, 0, len(rows))
	for _, row := range rows {
		item, err := publicTask(row)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return &digitalhuman.TaskListResult{Items: items, Counts: counts, SuccessfulRunCount: successfulRunCount, Total: total, Page: page, PageSize: pageSize, HasMore: int64(page*pageSize) < total}, nil
}

func (s *Service) countSuccessfulRuns(ctx context.Context, scope digitalhuman.Scope, digitalHumanID uuid.UUID, query digitalhuman.TaskListQuery) (int64, error) {
	if query.RunFinishedAfter == nil && query.RunFinishedBefore == nil {
		return 0, nil
	}
	if query.RunFinishedAfter != nil && query.RunFinishedBefore != nil && !query.RunFinishedBefore.After(*query.RunFinishedAfter) {
		return 0, digitalhuman.ErrInvalidRequest
	}
	taskIDs := s.taskQuery(ctx, scope, digitalHumanID, query.SourceConversationID).Select("id")
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

func (s *Service) CreateTask(ctx context.Context, scope digitalhuman.Scope, digitalHumanID uuid.UUID, req digitalhuman.CreateTaskRequest, mutation digitalhuman.TaskMutationContext) (*digitalhuman.Task, error) {
	if err := validateBoundary(scope, digitalHumanID); err != nil {
		return nil, err
	}
	if err := validateCreate(req); err != nil {
		return nil, err
	}
	if mutation.OperationID == uuid.Nil {
		mutation.OperationID = uuid.New()
	}
	if mutation.SourceConversationID != "" {
		if existing, err := s.taskByOperation(ctx, scope, digitalHumanID, mutation.OperationID); err == nil {
			return publicTask(existing)
		}
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
	subjectID, sourceOperationID := digitalHumanID.String(), mutation.OperationID.String()
	sourceRef := strings.TrimSpace(mutation.Source)
	if sourceRef == "" {
		sourceRef = "console"
	}
	createdBy := scope.ActorID.String()
	task := &automationmodel.AutomationTask{
		OrganizationID: scope.OrganizationID.String(), WorkspaceID: scope.WorkspaceID.String(), Name: strings.TrimSpace(req.Name),
		TaskKind: automationmodel.AutomationTaskKindDigitalHuman, SubjectType: stringPointer(subjectTypeDigitalHuman), SubjectID: &subjectID,
		WorkStatus: &workStatus, Priority: &priority, Goal: &goal, SuccessCriteria: cleanStrings(req.SuccessCriteria),
		ExecutionTimeoutSeconds: defaultInt(req.ExecutionTimeoutSeconds, defaultTimeoutSeconds), MaxAttempts: clampAttempts(req.MaxAttempts), Revision: 1,
		DeliveryConfig: delivery, SourceOperationID: &sourceOperationID, ProposalExpiresAt: proposalExpiresAt,
		Status: lifecycle, TriggerType: automationmodel.AutomationTriggerTypeSchedule, ScheduleType: scheduleType, Timezone: timezone,
		ScheduleConfig: config, NextRunAt: nextRunAt, SourceType: sourceType(mutation), SourceRef: &sourceRef,
		SourceSnapshot: sourceSnapshot(mutation), CreatedBy: createdBy, UpdatedBy: createdBy,
	}
	action := &automationmodel.AutomationTaskAction{ActionType: automationmodel.AutomationActionTypeRunDigitalHuman, ActionOrder: 1, Enabled: true, Config: map[string]any{}}
	activity := newActivity(task.ID, mutation, "created", "Task created", map[string]any{"work_status": workStatus})
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
		if existing, loadErr := s.taskByOperation(ctx, scope, digitalHumanID, mutation.OperationID); loadErr == nil {
			return publicTask(existing)
		}
		return nil, fmt.Errorf("create digital human task: %w", err)
	}
	return publicTask(task)
}

func (s *Service) GetTask(ctx context.Context, scope digitalhuman.Scope, digitalHumanID, taskID uuid.UUID, sourceConversationID string) (*digitalhuman.Task, error) {
	task, err := s.loadTask(ctx, s.db, scope, digitalHumanID, taskID, sourceConversationID, false)
	if err != nil {
		return nil, err
	}
	return publicTask(task)
}

func (s *Service) UpdateTask(ctx context.Context, scope digitalhuman.Scope, digitalHumanID, taskID uuid.UUID, req digitalhuman.UpdateTaskRequest, mutation digitalhuman.TaskMutationContext) (*digitalhuman.Task, error) {
	if req.ExpectedRevision <= 0 {
		return nil, digitalhuman.ErrInvalidRequest
	}
	var updated *automationmodel.AutomationTask
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		task, err := s.loadTask(ctx, tx, scope, digitalHumanID, taskID, mutation.SourceConversationID, true)
		if err != nil {
			return err
		}
		if task.Revision != req.ExpectedRevision {
			return digitalhuman.ErrTaskConflict
		}
		changes := map[string]any{}
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
		if req.Schedule != nil {
			st, tz, cfg, next, err := compileSchedule(*req.Schedule)
			if err != nil {
				return digitalhuman.ErrInvalidRequest
			}
			task.ScheduleType, task.Timezone, task.ScheduleConfig = st, tz, cfg
			if task.Status == automationmodel.AutomationTaskStatusActive {
				task.NextRunAt = next
			}
			changes["schedule"] = req.Schedule
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
		task.Revision++
		task.UpdatedBy = scope.ActorID.String()
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

func (s *Service) ApplyTaskAction(ctx context.Context, scope digitalhuman.Scope, digitalHumanID, taskID uuid.UUID, action digitalhuman.TaskAction, req digitalhuman.ApplyTaskActionRequest, mutation digitalhuman.TaskMutationContext) (*digitalhuman.Task, error) {
	if req.ExpectedRevision <= 0 {
		return nil, digitalhuman.ErrInvalidRequest
	}
	var result *automationmodel.AutomationTask
	var runID string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		task, err := s.loadTask(ctx, tx, scope, digitalHumanID, taskID, mutation.SourceConversationID, true)
		if err != nil {
			return err
		}
		if task.Revision != req.ExpectedRevision {
			return digitalhuman.ErrTaskConflict
		}
		if err := applyTransition(task, action, s.now()); err != nil {
			return err
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
		task.UpdatedBy = scope.ActorID.String()
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
		_ = s.cancelActiveRuns(ctx, scope, digitalHumanID, taskID)
	}
	return publicTask(result)
}

func (s *Service) cancelActiveRuns(ctx context.Context, scope digitalhuman.Scope, digitalHumanID, taskID uuid.UUID) error {
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
			if err := runner.CancelDigitalHumanRun(ctx, scope.OrganizationID.String(), scope.WorkspaceID.String(), digitalHumanID.String(), row.ExternalRunID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) SetTaskWorkStatus(ctx context.Context, scope digitalhuman.Scope, digitalHumanID, taskID uuid.UUID, status digitalhuman.TaskWorkStatus, revision int64, mutation digitalhuman.TaskMutationContext) (*digitalhuman.Task, error) {
	if !validWorkStatus(status) {
		return nil, digitalhuman.ErrInvalidRequest
	}
	return s.setStatus(ctx, scope, digitalHumanID, taskID, status, revision, mutation)
}

func (s *Service) setStatus(ctx context.Context, scope digitalhuman.Scope, digitalHumanID, taskID uuid.UUID, status digitalhuman.TaskWorkStatus, revision int64, mutation digitalhuman.TaskMutationContext) (*digitalhuman.Task, error) {
	var task *automationmodel.AutomationTask
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		loaded, err := s.loadTask(ctx, tx, scope, digitalHumanID, taskID, mutation.SourceConversationID, true)
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
		loaded.UpdatedBy = scope.ActorID.String()
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

func (s *Service) ListTaskRuns(ctx context.Context, scope digitalhuman.Scope, digitalHumanID, taskID uuid.UUID, page, pageSize int, sourceConversationID string) (*digitalhuman.TaskRunListResult, error) {
	if _, err := s.loadTask(ctx, s.db, scope, digitalHumanID, taskID, sourceConversationID, false); err != nil {
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
			item.ErrorSummary = *row.ErrorSummary
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
			}
		}
		items = append(items, item)
	}
	return &digitalhuman.TaskRunListResult{Items: items, Total: total, Page: page, PageSize: pageSize, HasMore: int64(page*pageSize) < total}, nil
}

func (s *Service) ListTaskActivities(ctx context.Context, scope digitalhuman.Scope, digitalHumanID, taskID uuid.UUID, page, pageSize int, sourceConversationID string) (*digitalhuman.TaskActivityListResult, error) {
	if _, err := s.loadTask(ctx, s.db, scope, digitalHumanID, taskID, sourceConversationID, false); err != nil {
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

func (s *Service) GetChannelTaskPolicy(ctx context.Context, scope digitalhuman.Scope, digitalHumanID, installationID uuid.UUID) (*digitalhuman.ChannelTaskPolicy, error) {
	if err := s.validateInstallation(ctx, scope, digitalHumanID, installationID); err != nil {
		return nil, err
	}
	var row channelTaskPolicy
	err := s.db.WithContext(ctx).Where("installation_id = ? AND organization_id = ? AND workspace_id = ? AND digital_human_id = ?", installationID, scope.OrganizationID, scope.WorkspaceID, digitalHumanID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &digitalhuman.ChannelTaskPolicy{InstallationID: installationID, Enabled: false, Revision: 0}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load channel task policy: %w", err)
	}
	return publicPolicy(&row), nil
}

func (s *Service) UpdateChannelTaskPolicy(ctx context.Context, scope digitalhuman.Scope, digitalHumanID, installationID uuid.UUID, req digitalhuman.UpdateChannelTaskPolicyRequest) (*digitalhuman.ChannelTaskPolicy, error) {
	if err := s.validateInstallation(ctx, scope, digitalHumanID, installationID); err != nil {
		return nil, err
	}
	now := s.now()
	if req.ExpectedRevision == 0 {
		row := channelTaskPolicy{InstallationID: installationID, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, DigitalHumanID: digitalHumanID, Enabled: req.Enabled, Revision: 1, UpdatedBy: scope.ActorID, CreatedAt: now, UpdatedAt: now}
		if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return nil, err
		}
		var stored channelTaskPolicy
		if err := s.db.WithContext(ctx).Where("installation_id = ?", installationID).First(&stored).Error; err != nil {
			return nil, err
		}
		if stored.Revision != 1 || stored.Enabled != req.Enabled {
			return nil, digitalhuman.ErrTaskConflict
		}
		return publicPolicy(&stored), nil
	}
	updates := map[string]any{"enabled": req.Enabled, "revision": gorm.Expr("revision + 1"), "updated_by": scope.ActorID, "updated_at": now}
	result := s.db.WithContext(ctx).Model(&channelTaskPolicy{}).Where("installation_id = ? AND organization_id = ? AND workspace_id = ? AND digital_human_id = ? AND revision = ?", installationID, scope.OrganizationID, scope.WorkspaceID, digitalHumanID, req.ExpectedRevision).Updates(updates)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, digitalhuman.ErrTaskConflict
	}
	return s.GetChannelTaskPolicy(ctx, scope, digitalHumanID, installationID)
}

func (s *Service) ChannelTaskManagementEnabled(ctx context.Context, scope digitalhuman.Scope, digitalHumanID, installationID uuid.UUID) (bool, error) {
	policy, err := s.GetChannelTaskPolicy(ctx, scope, digitalHumanID, installationID)
	if err != nil {
		return false, err
	}
	return policy.Enabled, nil
}

func (s *Service) taskQuery(ctx context.Context, scope digitalhuman.Scope, digitalHumanID uuid.UUID, sourceConversationID string) *gorm.DB {
	query := s.db.WithContext(ctx).Model(&automationmodel.AutomationTask{}).
		Where("organization_id = ? AND workspace_id = ? AND task_kind = ? AND subject_type = ? AND subject_id = ? AND status <> ?", scope.OrganizationID, scope.WorkspaceID, automationmodel.AutomationTaskKindDigitalHuman, subjectTypeDigitalHuman, digitalHumanID, automationmodel.AutomationTaskStatusArchived)
	if sourceConversationID != "" {
		query = query.Where("source_snapshot ->> 'conversation_id' = ?", sourceConversationID)
	}
	return query
}

func (s *Service) loadTask(ctx context.Context, db *gorm.DB, scope digitalhuman.Scope, digitalHumanID, taskID uuid.UUID, sourceConversationID string, lock bool) (*automationmodel.AutomationTask, error) {
	if err := validateBoundary(scope, digitalHumanID); err != nil || taskID == uuid.Nil {
		return nil, digitalhuman.ErrInvalidRequest
	}
	query := db.WithContext(ctx).Where("id = ? AND organization_id = ? AND workspace_id = ? AND task_kind = ? AND subject_id = ?", taskID, scope.OrganizationID, scope.WorkspaceID, automationmodel.AutomationTaskKindDigitalHuman, digitalHumanID)
	if sourceConversationID != "" {
		query = query.Where("source_snapshot ->> 'conversation_id' = ?", sourceConversationID)
	}
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var task automationmodel.AutomationTask
	if err := query.First(&task).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, digitalhuman.ErrTaskNotFound
	} else if err != nil {
		return nil, fmt.Errorf("load digital human task: %w", err)
	}
	return &task, nil
}

func (s *Service) taskByOperation(ctx context.Context, scope digitalhuman.Scope, digitalHumanID, operationID uuid.UUID) (*automationmodel.AutomationTask, error) {
	var task automationmodel.AutomationTask
	err := s.db.WithContext(ctx).Where("organization_id = ? AND workspace_id = ? AND task_kind = ? AND subject_id = ? AND source_operation_id = ?", scope.OrganizationID, scope.WorkspaceID, automationmodel.AutomationTaskKindDigitalHuman, digitalHumanID, operationID).First(&task).Error
	return &task, err
}

func (s *Service) validateInstallation(ctx context.Context, scope digitalhuman.Scope, digitalHumanID, installationID uuid.UUID) error {
	if err := validateBoundary(scope, digitalHumanID); err != nil || installationID == uuid.Nil {
		return digitalhuman.ErrInvalidRequest
	}
	var count int64
	err := s.db.WithContext(ctx).Table("channel_installations").Where("id = ? AND organization_id = ? AND workspace_id = ? AND default_subject_type = ? AND default_subject_id = ?", installationID, scope.OrganizationID, scope.WorkspaceID, subjectTypeDigitalHuman, digitalHumanID).Count(&count).Error
	if err != nil {
		return fmt.Errorf("validate channel installation: %w", err)
	}
	if count != 1 {
		return digitalhuman.ErrTaskPolicyRejected
	}
	return nil
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

func publicTask(row *automationmodel.AutomationTask) (*digitalhuman.Task, error) {
	id, err := uuid.Parse(row.ID)
	if err != nil {
		return nil, err
	}
	subjectID, err := uuid.Parse(pointerValue(row.SubjectID))
	if err != nil {
		return nil, err
	}
	result := &digitalhuman.Task{ID: id, DigitalHumanID: subjectID, Name: row.Name, Goal: pointerValue(row.Goal), SuccessCriteria: append([]string(nil), row.SuccessCriteria...), WorkStatus: digitalhuman.TaskWorkStatus(pointerValue(row.WorkStatus)), LifecycleStatus: string(row.Status), Priority: digitalhuman.TaskPriority(pointerValue(row.Priority)), Schedule: publicSchedule(row), DeliveryMode: deliveryMode(row.DeliveryConfig), ExecutionTimeoutSeconds: row.ExecutionTimeoutSeconds, MaxAttempts: row.MaxAttempts, NextRunAt: row.NextRunAt, LastRunAt: row.LastRunAt, Revision: row.Revision, ProposalExpiresAt: row.ProposalExpiresAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	if row.LastRunStatus != nil {
		result.LastRunStatus = *row.LastRunStatus
	}
	if row.SourceRef != nil {
		result.Source = *row.SourceRef
	}
	result.SourceConversationType = sourceConversationType(row)
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
		if mutation.ReplyTo != nil {
			mode = digitalhuman.TaskDeliverySourceConversation
		} else {
			mode = digitalhuman.TaskDeliveryTaskOnly
		}
	}
	config := map[string]any{"mode": string(mode)}
	switch mode {
	case digitalhuman.TaskDeliveryTaskOnly:
		return config, nil
	case digitalhuman.TaskDeliverySourceConversation:
		if mutation.ReplyTo == nil {
			return nil, digitalhuman.ErrTaskPolicyRejected
		}
		encoded, err := json.Marshal(mutation.ReplyTo)
		if err != nil {
			return nil, err
		}
		var reply map[string]any
		if err := json.Unmarshal(encoded, &reply); err != nil {
			return nil, err
		}
		config["reply_to"] = reply
		return config, nil
	default:
		return nil, digitalhuman.ErrInvalidRequest
	}
}

func deliveryMode(config map[string]any) digitalhuman.TaskDeliveryMode {
	if value, ok := config["mode"].(string); ok {
		return digitalhuman.TaskDeliveryMode(value)
	}
	return digitalhuman.TaskDeliveryTaskOnly
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

func validateBoundary(scope digitalhuman.Scope, digitalHumanID uuid.UUID) error {
	if scope.OrganizationID == uuid.Nil || scope.WorkspaceID == uuid.Nil || scope.ActorID == uuid.Nil || digitalHumanID == uuid.Nil {
		return digitalhuman.ErrInvalidRequest
	}
	return nil
}
func validateCreate(req digitalhuman.CreateTaskRequest) error {
	invalidTimeout := req.ExecutionTimeoutSeconds != 0 && (req.ExecutionTimeoutSeconds < 30 || req.ExecutionTimeoutSeconds > 1800)
	invalidAttempts := req.MaxAttempts != 0 && (req.MaxAttempts < 1 || req.MaxAttempts > 3)
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Goal) == "" || (req.Priority != "" && !validPriority(req.Priority)) || invalidTimeout || invalidAttempts {
		return digitalhuman.ErrInvalidRequest
	}
	return nil
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
		"delivery_config":           datatypes.JSON(deliveryConfig),
		"status":                    task.Status,
		"work_status":               task.WorkStatus,
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
func publicPolicy(row *channelTaskPolicy) *digitalhuman.ChannelTaskPolicy {
	return &digitalhuman.ChannelTaskPolicy{InstallationID: row.InstallationID, Enabled: row.Enabled, Revision: row.Revision, UpdatedAt: row.UpdatedAt}
}
