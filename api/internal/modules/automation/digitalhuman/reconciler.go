package digitalhumantask

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	digitalhuman "github.com/zgiai/zgi/api/internal/modules/digitalhuman"
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
	deliverer  digitalhuman.TaskResultDeliverer
	notifier   digitalhuman.TaskReportNotifier
	now        func() time.Time
}

func NewReconcileHandler(db *gorm.DB, dispatcher RunDispatcher, provider automationaction.DigitalHumanRunnerProvider) *ReconcileHandler {
	return &ReconcileHandler{db: db, dispatcher: dispatcher, provider: provider, now: func() time.Time { return time.Now().UTC() }}
}

// SetResultDeliverer is called once the DigitalHuman runtime is initialized.
// The scheduler handler itself is registered earlier during route setup.
func (h *ReconcileHandler) SetResultDeliverer(deliverer digitalhuman.TaskResultDeliverer) {
	if h != nil {
		h.deliverer = deliverer
	}
}

// SetReportNotifier lets the in-app inbox tell requesters about final reports.
func (h *ReconcileHandler) SetReportNotifier(notifier digitalhuman.TaskReportNotifier) {
	if h != nil {
		h.notifier = notifier
	}
}

func (h *ReconcileHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	if h == nil || h.db == nil || h.dispatcher == nil {
		return fmt.Errorf("digital human task reconciler is not configured")
	}
	if err := h.expireProposals(ctx); err != nil {
		return err
	}
	if err := closeExpiredReports(ctx, h.db, h.now()); err != nil {
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
	var delivery *digitalhuman.TaskResultDeliveryRequest
	var deliveryAlreadyRecorded bool
	var finalReport *taskReportRow
	var finalTask *automationmodel.AutomationTask
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
		report, err := finalizeTaskReport(tx, &task, &taskRun, pointerValue(actionRun.ExternalRunID), succeeded, willRetry, state.Result, state.ErrorMessage, now)
		if err != nil {
			return err
		}
		if report != nil && h.deliverer != nil {
			var delivered int64
			if err := tx.Model(&automationmodel.AutomationTaskActivity{}).
				Where("task_id = ? AND action = ? AND operation_id = ?", task.ID, "result_delivered", taskRunID).
				Count(&delivered).Error; err != nil {
				return err
			}
			deliveryAlreadyRecorded = delivered > 0
			if conversationID, ok := uuidFromTaskDeliveryConfig(task.DeliveryConfig); ok && !deliveryAlreadyRecorded {
				accountID := uuid.Nil
				subjectKind := ""
				if task.OwnerSubjectKind != nil {
					subjectKind = strings.TrimSpace(*task.OwnerSubjectKind)
				}
				// ChatRuntime stores both account and API-end-user
				// conversations under AccountID. The runtime decides whether
				// membership checks apply from UserSubjectKind.
				if task.OwnerSubjectID != nil {
					accountID, _ = uuid.Parse(strings.TrimSpace(*task.OwnerSubjectID))
				}
				delivery = &digitalhuman.TaskResultDeliveryRequest{
					OrganizationID: mustUUID(task.OrganizationID), WorkspaceID: mustUUID(task.WorkspaceID), AccountID: accountID, UserSubjectKind: subjectKind,
					DigitalHumanID: mustUUID(pointerValue(task.SubjectID)), TaskID: mustUUID(task.ID), TaskRunID: mustUUID(taskRunID),
					ConversationID: conversationID, Source: pointerValue(task.OriginSurface), TaskName: task.Name, Answer: state.Result,
					Report: reportDelivery(report),
				}
			}
		}
		finalReport, finalTask = report, &task
		activity := &automationmodel.AutomationTaskActivity{TaskID: task.ID, OperationID: uuid.NewString(), ActorType: automationmodel.AutomationTaskActivityActorSystem, ActorRef: "scheduler", Action: "run_finished", Summary: "Task run finished", Changes: map[string]any{"run_status": taskRun.Status, "digital_human_status": state.Status}}
		return tx.Create(activity).Error
	})
	if err != nil {
		return err
	}
	if delivery != nil && !deliveryAlreadyRecorded {
		if err := h.deliverer.DeliverTaskResult(ctx, *delivery); err != nil {
			// The task result remains queryable in the task center. Keep the
			// scheduler run successful and leave a safe retry signal in its
			// activity history instead of turning a completed task into failed.
			_ = h.db.WithContext(ctx).Create(&automationmodel.AutomationTaskActivity{
				TaskID: delivery.TaskID.String(), OperationID: delivery.TaskRunID.String(),
				ActorType: automationmodel.AutomationTaskActivityActorSystem, ActorRef: "scheduler",
				Action: "result_delivery_failed", Summary: "Task result could not be returned to the source conversation",
				Changes: map[string]any{"error": "conversation delivery unavailable"},
			}).Error
		} else {
			_ = h.db.WithContext(ctx).Create(&automationmodel.AutomationTaskActivity{
				TaskID: delivery.TaskID.String(), OperationID: delivery.TaskRunID.String(),
				ActorType: automationmodel.AutomationTaskActivityActorSystem, ActorRef: "scheduler",
				Action: "result_delivered", Summary: "Task result returned to the source conversation",
				Changes: map[string]any{"conversation_id": delivery.ConversationID.String()},
			}).Error
		}
	}
	if finalReport != nil && h.notifier != nil {
		if notice, ok := reportNotice(finalReport, finalTask); ok {
			// The report stays in the task's report history if the inbox is
			// unavailable; a missing notice is not worth failing the run.
			_ = h.notifier.NotifyTaskReport(ctx, notice)
		}
	}
	if retryRunID != "" {
		if err := h.dispatcher.DispatchTaskRun(ctx, retryRunID); err != nil {
			return fmt.Errorf("enqueue digital human task retry: %w", err)
		}
	}
	return nil
}

func uuidFromTaskDeliveryConfig(config map[string]interface{}) (uuid.UUID, bool) {
	if config == nil || config["mode"] != string(digitalhuman.TaskDeliverySourceConversation) {
		return uuid.Nil, false
	}
	value, ok := config["conversation_id"].(string)
	if !ok {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(strings.TrimSpace(value))
	return id, err == nil && id != uuid.Nil
}

func mustUUID(value string) uuid.UUID {
	id, _ := uuid.Parse(strings.TrimSpace(value))
	return id
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
