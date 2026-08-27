package digitalhumantask

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type reconcilerRunner struct {
	state        *automationaction.DigitalHumanRunState
	cancelCalled bool
}

func (*reconcilerRunner) RunDigitalHuman(context.Context, automationaction.DigitalHumanRunRequest) (*automationaction.DigitalHumanRunResult, error) {
	return nil, nil
}
func (r *reconcilerRunner) GetDigitalHumanRun(context.Context, string, string, string, string) (*automationaction.DigitalHumanRunState, error) {
	return r.state, nil
}

func (r *reconcilerRunner) CancelDigitalHumanRun(context.Context, string, string, string, string) error {
	r.cancelCalled = true
	return nil
}

func TestReconcilerCompletesTaskOnlyAfterDigitalHumanTerminalState(t *testing.T) {
	db := newReconcilerDB(t)
	dispatcher := &recordingDispatcher{}
	runner := &reconcilerRunner{state: &automationaction.DigitalHumanRunState{Status: "succeeded", Result: "done"}}
	task, taskRun, actionRun := createPendingExternalRun(t, db, 1)
	handler := NewReconcileHandler(db, dispatcher, func() automationaction.AutomationDigitalHumanRunner { return runner })

	if err := handler.Handle(t.Context(), nil); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if err := db.First(actionRun, "id = ?", actionRun.ID).Error; err != nil || actionRun.Status != automationmodel.AutomationActionRunStatusSucceeded {
		t.Fatalf("action run = %#v, error = %v", actionRun, err)
	}
	if err := db.First(taskRun, "id = ?", taskRun.ID).Error; err != nil || taskRun.Status != automationmodel.AutomationTaskRunStatusSucceeded {
		t.Fatalf("task run = %#v, error = %v", taskRun, err)
	}
	if err := db.First(task, "id = ?", task.ID).Error; err != nil || task.WorkStatus == nil || *task.WorkStatus != automationmodel.AutomationWorkStatusCompleted || task.Status != automationmodel.AutomationTaskStatusCompleted {
		t.Fatalf("task = %#v, error = %v", task, err)
	}
	if len(dispatcher.runIDs) != 0 {
		t.Fatalf("unexpected retry dispatch = %v", dispatcher.runIDs)
	}
}

func TestReconcilerRestoresActiveRecurringTaskAfterSuccessfulOccurrence(t *testing.T) {
	db := newReconcilerDB(t)
	dispatcher := &recordingDispatcher{}
	runner := &reconcilerRunner{state: &automationaction.DigitalHumanRunState{Status: "succeeded", Result: "reminder sent"}}
	task, _, _ := createPendingExternalRun(t, db, 1)
	nextRunAt := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	completed := automationmodel.AutomationWorkStatusCompleted
	task.ScheduleType = automationmodel.AutomationScheduleTypeCron
	task.ScheduleConfig = map[string]any{"cron_expr": "0 8 * * *"}
	task.Timezone = "Asia/Shanghai"
	task.WorkStatus = &completed
	task.NextRunAt = &nextRunAt
	if err := db.Save(task).Error; err != nil {
		t.Fatalf("save recurring task: %v", err)
	}
	handler := NewReconcileHandler(db, dispatcher, func() automationaction.AutomationDigitalHumanRunner { return runner })

	if err := handler.Handle(t.Context(), nil); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if err := db.First(task, "id = ?", task.ID).Error; err != nil || task.WorkStatus == nil || *task.WorkStatus != automationmodel.AutomationWorkStatusPending || task.Status != automationmodel.AutomationTaskStatusActive || task.NextRunAt == nil || !task.NextRunAt.Equal(nextRunAt) {
		t.Fatalf("recurring task = %#v, error = %v", task, err)
	}
}

func TestReconcilerRetriesFailureWithoutCompletingTask(t *testing.T) {
	db := newReconcilerDB(t)
	dispatcher := &recordingDispatcher{}
	runner := &reconcilerRunner{state: &automationaction.DigitalHumanRunState{Status: "failed", ErrorMessage: "temporary"}}
	task, _, _ := createPendingExternalRun(t, db, 2)
	handler := NewReconcileHandler(db, dispatcher, func() automationaction.AutomationDigitalHumanRunner { return runner })

	if err := handler.Handle(t.Context(), nil); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(dispatcher.runIDs) != 1 {
		t.Fatalf("retry dispatches = %v, want one", dispatcher.runIDs)
	}
	var retry automationmodel.AutomationTaskRun
	if err := db.First(&retry, "id = ?", dispatcher.runIDs[0]).Error; err != nil || retry.Status != automationmodel.AutomationTaskRunStatusQueued {
		t.Fatalf("retry run = %#v, error = %v", retry, err)
	}
	if err := db.First(task, "id = ?", task.ID).Error; err != nil || task.WorkStatus == nil || *task.WorkStatus != automationmodel.AutomationWorkStatusInProgress {
		t.Fatalf("task after retry = %#v, error = %v", task, err)
	}
}

func TestReconcilerTimesOutAndBlocksExhaustedTask(t *testing.T) {
	db := newReconcilerDB(t)
	dispatcher := &recordingDispatcher{}
	runner := &reconcilerRunner{state: &automationaction.DigitalHumanRunState{Status: "running"}}
	task, taskRun, actionRun := createPendingExternalRun(t, db, 1)
	startedAt := time.Now().UTC().Add(-2 * time.Minute)
	task.ExecutionTimeoutSeconds = 30
	actionRun.StartedAt = &startedAt
	if err := db.Save(task).Error; err != nil {
		t.Fatalf("save timeout: %v", err)
	}
	if err := db.Save(actionRun).Error; err != nil {
		t.Fatalf("save started_at: %v", err)
	}
	handler := NewReconcileHandler(db, dispatcher, func() automationaction.AutomationDigitalHumanRunner { return runner })

	if err := handler.Handle(t.Context(), nil); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !runner.cancelCalled {
		t.Fatal("timed out DigitalHuman run was not canceled")
	}
	if err := db.First(actionRun, "id = ?", actionRun.ID).Error; err != nil || actionRun.Status != automationmodel.AutomationActionRunStatusFailed {
		t.Fatalf("action run = %#v, error = %v", actionRun, err)
	}
	if err := db.First(taskRun, "id = ?", taskRun.ID).Error; err != nil || taskRun.Status != automationmodel.AutomationTaskRunStatusFailed {
		t.Fatalf("task run = %#v, error = %v", taskRun, err)
	}
	if err := db.First(task, "id = ?", task.ID).Error; err != nil || task.WorkStatus == nil || *task.WorkStatus != automationmodel.AutomationWorkStatusBlocked {
		t.Fatalf("task = %#v, error = %v", task, err)
	}
}

func TestReconcilerExpiresProposalWithActivity(t *testing.T) {
	db := newReconcilerDB(t)
	now := time.Now().UTC()
	subjectID, goal := uuid.NewString(), "Confirm this work"
	workStatus, priority := automationmodel.AutomationWorkStatusProposed, automationmodel.AutomationTaskPriorityNormal
	expiresAt := now.Add(-time.Minute)
	task := &automationmodel.AutomationTask{
		OrganizationID: uuid.NewString(), WorkspaceID: uuid.NewString(), Name: "Proposal",
		TaskKind: automationmodel.AutomationTaskKindDigitalHuman, SubjectType: stringPointer(subjectTypeDigitalHuman), SubjectID: &subjectID,
		WorkStatus: &workStatus, Priority: &priority, Goal: &goal, MaxAttempts: 1, ExecutionTimeoutSeconds: 300, Revision: 1,
		Status: automationmodel.AutomationTaskStatusDraft, TriggerType: automationmodel.AutomationTriggerTypeSchedule,
		ScheduleType: automationmodel.AutomationScheduleTypeManual, Timezone: "UTC", ScheduleConfig: map[string]any{},
		SourceType: automationmodel.AutomationSourceTypeLLMTool, CreatedBy: uuid.NewString(), UpdatedBy: uuid.NewString(), ProposalExpiresAt: &expiresAt,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create proposal: %v", err)
	}
	handler := NewReconcileHandler(db, &recordingDispatcher{}, nil)
	handler.now = func() time.Time { return now }
	if err := handler.Handle(t.Context(), nil); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	var expired automationmodel.AutomationTask
	if err := db.First(&expired, "id = ?", task.ID).Error; err != nil || expired.WorkStatus == nil || *expired.WorkStatus != automationmodel.AutomationWorkStatusCanceled || expired.ProposalExpiresAt != nil {
		t.Fatalf("expired task = %#v, error = %v", &expired, err)
	}
	var activity automationmodel.AutomationTaskActivity
	if err := db.Where("task_id = ? AND action = ?", task.ID, "proposal_expired").First(&activity).Error; err != nil {
		t.Fatalf("load expiration activity: %v", err)
	}
}

func newReconcilerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:reconciler-"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&automationmodel.AutomationTask{}, &automationmodel.AutomationTaskRun{}, &automationmodel.AutomationTaskAction{}, &automationmodel.AutomationActionRun{}, &automationmodel.AutomationTaskActivity{}); err != nil {
		t.Fatalf("migrate reconciler: %v", err)
	}
	return db
}

func createPendingExternalRun(t *testing.T, db *gorm.DB, maxAttempts int) (*automationmodel.AutomationTask, *automationmodel.AutomationTaskRun, *automationmodel.AutomationActionRun) {
	t.Helper()
	subjectID := uuid.NewString()
	workStatus, priority := automationmodel.AutomationWorkStatusInProgress, automationmodel.AutomationTaskPriorityNormal
	goal := "finish the task"
	task := &automationmodel.AutomationTask{
		OrganizationID: uuid.NewString(), WorkspaceID: uuid.NewString(), Name: "Task",
		TaskKind: automationmodel.AutomationTaskKindDigitalHuman, SubjectType: stringPointer(subjectTypeDigitalHuman), SubjectID: &subjectID,
		WorkStatus: &workStatus, Priority: &priority, Goal: &goal, MaxAttempts: maxAttempts, ExecutionTimeoutSeconds: 300, Revision: 1,
		Status: automationmodel.AutomationTaskStatusActive, TriggerType: automationmodel.AutomationTriggerTypeSchedule,
		ScheduleType: automationmodel.AutomationScheduleTypeManual, Timezone: "UTC", ScheduleConfig: map[string]any{},
		SourceType: automationmodel.AutomationSourceTypeManual, CreatedBy: uuid.NewString(), UpdatedBy: uuid.NewString(),
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}
	taskRun := &automationmodel.AutomationTaskRun{TaskID: task.ID, TriggerSource: automationmodel.AutomationTriggerSourceManualRun, ScheduledFor: time.Now(), Status: automationmodel.AutomationTaskRunStatusRunning, RuntimeContext: map[string]any{"attempt": 1}}
	if err := db.Create(taskRun).Error; err != nil {
		t.Fatalf("create task run: %v", err)
	}
	action := &automationmodel.AutomationTaskAction{TaskID: task.ID, ActionType: automationmodel.AutomationActionTypeRunDigitalHuman, ActionOrder: 1, Enabled: true, Config: map[string]any{}}
	if err := db.Create(action).Error; err != nil {
		t.Fatalf("create action: %v", err)
	}
	externalType, externalID := subjectTypeDigitalHuman, uuid.NewString()
	startedAt := time.Now()
	actionRun := &automationmodel.AutomationActionRun{TaskRunID: taskRun.ID, TaskActionID: action.ID, ActionType: automationmodel.AutomationActionTypeRunDigitalHuman, Status: automationmodel.AutomationActionRunStatusRunning, ExternalRunType: &externalType, ExternalRunID: &externalID, StartedAt: &startedAt}
	if err := db.Create(actionRun).Error; err != nil {
		t.Fatalf("create action run: %v", err)
	}
	return task, taskRun, actionRun
}
