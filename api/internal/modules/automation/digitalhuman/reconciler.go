package digitalhumantask

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const TaskTypeReconcileDigitalHumanRuns = "automation:digital-human:reconcile"

type ReconcileTask struct{}

func (*ReconcileTask) TaskType() string        { return TaskTypeReconcileDigitalHumanRuns }
func (*ReconcileTask) CronSpec() string        { return "" }
func (*ReconcileTask) Interval() time.Duration { return 15 * time.Second }
func (*ReconcileTask) Payload() []byte         { return nil }
func (*ReconcileTask) Options() []asynq.Option {
	return []asynq.Option{asynq.Queue("scheduler"), asynq.MaxRetry(1), asynq.Timeout(30 * time.Second), asynq.Unique(10 * time.Second)}
}

type ReconcileHandler struct {
	db         *gorm.DB
	dispatcher RunDispatcher
	provider   automationaction.DigitalHumanRunnerProvider
	now        func() time.Time
}

func NewReconcileHandler(db *gorm.DB, dispatcher RunDispatcher, provider automationaction.DigitalHumanRunnerProvider) *ReconcileHandler {
	return &ReconcileHandler{db: db, dispatcher: dispatcher, provider: provider, now: func() time.Time { return time.Now().UTC() }}
}

func (h *ReconcileHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	if h == nil || h.db == nil || h.dispatcher == nil {
		return fmt.Errorf("digital human task reconciler is not configured")
	}
	if err := h.expireProposals(ctx); err != nil {
		return err
	}
	if h.provider == nil {
		return nil
	}
	runner := h.provider()
	if runner == nil {
		return nil
	}
	type pendingRun struct {
		ActionRunID             string
		TaskRunID               string
		TaskID                  string
		ExternalRunID           string
		OrganizationID          string
		WorkspaceID             string
		DigitalHumanID          string
		ExecutionTimeoutSeconds int
		ActionStartedAt         *time.Time
	}
	var rows []pendingRun
	err := h.db.WithContext(ctx).Table("automation_action_runs ar").
		Select("ar.id action_run_id, ar.task_run_id, tr.task_id, ar.external_run_id, ar.started_at action_started_at, t.organization_id, t.workspace_id, t.subject_id digital_human_id, t.execution_timeout_seconds").
		Joins("JOIN automation_task_runs tr ON tr.id = ar.task_run_id").Joins("JOIN automation_tasks t ON t.id = tr.task_id").
		Where("ar.action_type = ? AND ar.status = ? AND ar.external_run_type = ? AND ar.external_run_id IS NOT NULL", automationmodel.AutomationActionTypeRunDigitalHuman, automationmodel.AutomationActionRunStatusRunning, subjectTypeDigitalHuman).
		Order("ar.updated_at ASC").Limit(100).Scan(&rows).Error
	if err != nil {
		return fmt.Errorf("list pending digital human automation runs: %w", err)
	}
	for _, row := range rows {
		state, getErr := runner.GetDigitalHumanRun(ctx, row.OrganizationID, row.WorkspaceID, row.DigitalHumanID, row.ExternalRunID)
		if getErr != nil {
			logger.WarnContext(ctx, "digital human task run status unavailable", getErr)
			continue
		}
		if state != nil && terminalDigitalHumanStatus(state.Status) {
			if err := h.finish(ctx, row.ActionRunID, row.TaskRunID, row.TaskID, state); err != nil {
				return err
			}
			continue
		}
		if !digitalHumanRunTimedOut(row.ActionStartedAt, row.ExecutionTimeoutSeconds, h.now()) {
			continue
		}
		if err := runner.CancelDigitalHumanRun(ctx, row.OrganizationID, row.WorkspaceID, row.DigitalHumanID, row.ExternalRunID); err != nil {
			logger.WarnContext(ctx, "timed out digital human task run could not be canceled", err)
		}
		if err := h.finish(ctx, row.ActionRunID, row.TaskRunID, row.TaskID, &automationaction.DigitalHumanRunState{
			Status: "failed", ErrorMessage: "Digital human task execution timed out.",
		}); err != nil {
			return err
		}
	}
	return nil
}

func (h *ReconcileHandler) expireProposals(ctx context.Context) error {
	now := h.now()
	value := automationmodel.AutomationWorkStatusCanceled
	return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tasks []automationmodel.AutomationTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("task_kind = ? AND work_status = ? AND proposal_expires_at IS NOT NULL AND proposal_expires_at <= ?", automationmodel.AutomationTaskKindDigitalHuman, automationmodel.AutomationWorkStatusProposed, now).
			Limit(100).Find(&tasks).Error; err != nil {
			return err
		}
		for index := range tasks {
			task := &tasks[index]
			update := tx.Model(&automationmodel.AutomationTask{}).
				Where("id = ? AND work_status = ?", task.ID, automationmodel.AutomationWorkStatusProposed).
				Updates(map[string]any{"work_status": value, "status": automationmodel.AutomationTaskStatusPaused, "next_run_at": nil, "proposal_expires_at": nil, "revision": gorm.Expr("revision + 1"), "updated_at": now})
			if update.Error != nil {
				return update.Error
			}
			if update.RowsAffected == 0 {
				continue
			}
			activity := &automationmodel.AutomationTaskActivity{
				TaskID: task.ID, OperationID: uuid.NewString(), ActorType: automationmodel.AutomationTaskActivityActorSystem,
				ActorRef: "scheduler", Action: "proposal_expired", Summary: "Task proposal expired",
				Changes: map[string]any{"work_status": value},
			}
			if err := tx.Create(activity).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (h *ReconcileHandler) finish(ctx context.Context, actionRunID, taskRunID, taskID string, state *automationaction.DigitalHumanRunState) error {
	now := h.now()
	retryRunID := ""
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var actionRun automationmodel.AutomationActionRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", actionRunID).First(&actionRun).Error; err != nil {
			return err
		}
		if actionRun.Status != automationmodel.AutomationActionRunStatusRunning {
			return nil
		}
		var taskRun automationmodel.AutomationTaskRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", taskRunID).First(&taskRun).Error; err != nil {
			return err
		}
		var task automationmodel.AutomationTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", taskID).First(&task).Error; err != nil {
			return err
		}
		succeeded := state.Status == "succeeded"
		actionRun.FinishedAt = &now
		if succeeded {
			actionRun.Status = automationmodel.AutomationActionRunStatusSucceeded
			actionRun.ResponsePayload = map[string]any{"result": state.Result, "digital_human_run_id": actionRun.ExternalRunID}
		} else {
			actionRun.Status = automationmodel.AutomationActionRunStatusFailed
			message := state.ErrorMessage
			if message == "" {
				message = "digital human run " + state.Status
			}
			actionRun.ErrorMessage = &message
		}
		if err := tx.Save(&actionRun).Error; err != nil {
			return err
		}
		taskRun.FinishedAt = &now
		if succeeded {
			taskRun.Status, taskRun.ErrorSummary = automationmodel.AutomationTaskRunStatusSucceeded, nil
		} else {
			taskRun.Status = automationmodel.AutomationTaskRunStatusFailed
			message := state.ErrorMessage
			taskRun.ErrorSummary = &message
		}
		if err := tx.Save(&taskRun).Error; err != nil {
			return err
		}
		updates := map[string]any{"last_run_at": now, "last_run_status": taskRun.Status, "updated_at": now}
		attempt, ok := numericValue(taskRun.RuntimeContext["attempt"])
		if !ok || attempt < 1 {
			attempt = 1
		}
		willRetry := !succeeded && attempt < task.MaxAttempts
		if willRetry {
			retry := &automationmodel.AutomationTaskRun{TaskID: task.ID, TriggerSource: automationmodel.AutomationTriggerSourceRetry, ScheduledFor: now, Status: automationmodel.AutomationTaskRunStatusQueued, RuntimeContext: map[string]any{"attempt": attempt + 1, "trigger_mode": "automatic_retry"}}
			if err := tx.Create(retry).Error; err != nil {
				return err
			}
			retryRunID = retry.ID
		}
		workStatus := automationmodel.AutomationWorkStatus("")
		if task.WorkStatus != nil {
			workStatus = *task.WorkStatus
		}
		if succeeded && task.ScheduleType == automationmodel.AutomationScheduleTypeCron &&
			task.Status == automationmodel.AutomationTaskStatusActive &&
			(workStatus == automationmodel.AutomationWorkStatusInProgress || workStatus == automationmodel.AutomationWorkStatusCompleted) {
			updates["work_status"] = automationmodel.AutomationWorkStatusPending
			updates["revision"] = gorm.Expr("revision + 1")
		} else if workStatus == automationmodel.AutomationWorkStatusInProgress {
			if succeeded {
				updates["work_status"] = automationmodel.AutomationWorkStatusCompleted
				updates["status"] = automationmodel.AutomationTaskStatusCompleted
				updates["next_run_at"] = nil
			} else if willRetry {
				updates["work_status"] = automationmodel.AutomationWorkStatusInProgress
				updates["status"] = automationmodel.AutomationTaskStatusActive
			} else {
				updates["work_status"] = automationmodel.AutomationWorkStatusBlocked
				updates["status"] = automationmodel.AutomationTaskStatusPaused
				updates["next_run_at"] = nil
			}
			updates["revision"] = gorm.Expr("revision + 1")
		}
		if err := tx.Model(&automationmodel.AutomationTask{}).Where("id = ?", task.ID).Updates(updates).Error; err != nil {
			return err
		}
		activity := &automationmodel.AutomationTaskActivity{TaskID: task.ID, OperationID: uuid.NewString(), ActorType: automationmodel.AutomationTaskActivityActorSystem, ActorRef: "scheduler", Action: "run_finished", Summary: "Task run finished", Changes: map[string]any{"run_status": taskRun.Status, "digital_human_status": state.Status}}
		return tx.Create(activity).Error
	})
	if err != nil {
		return err
	}
	if retryRunID != "" {
		if err := h.dispatcher.DispatchTaskRun(ctx, retryRunID); err != nil {
			return fmt.Errorf("enqueue digital human task retry: %w", err)
		}
	}
	return nil
}

func terminalDigitalHumanStatus(status string) bool {
	switch status {
	case "succeeded", "failed", "canceled", "unknown":
		return true
	}
	return false
}

func digitalHumanRunTimedOut(startedAt *time.Time, timeoutSeconds int, now time.Time) bool {
	if startedAt == nil || startedAt.IsZero() {
		return false
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultTimeoutSeconds
	}
	return !now.Before(startedAt.Add(time.Duration(timeoutSeconds) * time.Second))
}
