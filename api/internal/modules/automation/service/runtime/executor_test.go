package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	automationmodel "github.com/zgiai/ginext/internal/modules/automation/model"
	automationrepo "github.com/zgiai/ginext/internal/modules/automation/repository"
	automationaction "github.com/zgiai/ginext/internal/modules/automation/service/action"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestExecuteTaskRunUpdatesTaskLastRun(t *testing.T) {
	db, err := newRuntimeTestDB()
	if err != nil {
		t.Fatal(err)
	}

	taskRepo := automationrepo.NewTaskRepository(db)
	actionRepo := automationrepo.NewActionRepository(db)
	runRepo := automationrepo.NewRunRepository(db)
	ctx := context.Background()

	task := &automationmodel.AutomationTask{
		OrganizationID: "org-1",
		WorkspaceID:    "workspace-1",
		Name:           "Runtime smoke test",
		Status:         automationmodel.AutomationTaskStatusCompleted,
		TriggerType:    automationmodel.AutomationTriggerTypeSchedule,
		ScheduleType:   automationmodel.AutomationScheduleTypeOnce,
		Timezone:       "UTC",
		ScheduleConfig: map[string]interface{}{"run_at": time.Now().Add(-time.Minute).Format(time.RFC3339)},
		SourceType:     automationmodel.AutomationSourceTypeManual,
		CreatedBy:      "account-1",
		UpdatedBy:      "account-1",
	}
	originalUpdatedAt := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	task.CreatedAt = originalUpdatedAt
	task.UpdatedAt = originalUpdatedAt
	if err := taskRepo.Create(ctx, db, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	run := &automationmodel.AutomationTaskRun{
		TaskID:        task.ID,
		TriggerSource: automationmodel.AutomationTriggerSourceScheduler,
		ScheduledFor:  time.Now().Add(-time.Minute),
		Status:        automationmodel.AutomationTaskRunStatusQueued,
	}
	if err := runRepo.CreateTaskRun(ctx, db, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	executor := NewExecutorWithActionExecutors(db, taskRepo, actionRepo, runRepo)
	if err := executor.ExecuteTaskRun(ctx, run.ID); err != nil {
		t.Fatalf("execute task run: %v", err)
	}

	updatedTask, err := taskRepo.GetByIDAnyScope(ctx, db, task.ID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if updatedTask.LastRunAt == nil {
		t.Fatal("expected task last_run_at to be updated")
	}
	if updatedTask.LastRunStatus == nil || *updatedTask.LastRunStatus != string(automationmodel.AutomationTaskRunStatusSucceeded) {
		t.Fatalf("unexpected last_run_status: %v", updatedTask.LastRunStatus)
	}
	if !updatedTask.UpdatedAt.Equal(originalUpdatedAt) {
		t.Fatalf("expected last_run update not to touch updated_at, got %s want %s", updatedTask.UpdatedAt, originalUpdatedAt)
	}
}

func TestExecuteTaskRunDoesNotRetryWhenLastRunUpdateFails(t *testing.T) {
	db, err := newRuntimeTestDB()
	if err != nil {
		t.Fatal(err)
	}

	baseTaskRepo := automationrepo.NewTaskRepository(db)
	taskRepo := &failingLastRunTaskRepository{
		TaskRepository: baseTaskRepo,
		err:            errors.New("last run write failed"),
	}
	actionRepo := automationrepo.NewActionRepository(db)
	runRepo := automationrepo.NewRunRepository(db)
	ctx := context.Background()

	task := newRuntimeTestTask(automationmodel.AutomationTaskStatusActive)
	if err := baseTaskRepo.Create(ctx, db, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	run := newRuntimeTestRun(task.ID, automationmodel.AutomationTriggerSourceScheduler)
	if err := runRepo.CreateTaskRun(ctx, db, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	executor := NewExecutorWithActionExecutors(db, taskRepo, actionRepo, runRepo)
	if err := executor.ExecuteTaskRun(ctx, run.ID); err != nil {
		t.Fatalf("expected last_run update failure to be non-retryable, got: %v", err)
	}

	updatedRun, err := runRepo.GetTaskRunByID(ctx, db, run.ID)
	if err != nil {
		t.Fatalf("reload run: %v", err)
	}
	if updatedRun.Status != automationmodel.AutomationTaskRunStatusSucceeded {
		t.Fatalf("expected run to stay succeeded, got %s", updatedRun.Status)
	}
}

func TestExecuteTaskRunCancelsScheduledRunWhenTaskPaused(t *testing.T) {
	db, err := newRuntimeTestDB()
	if err != nil {
		t.Fatal(err)
	}

	taskRepo := automationrepo.NewTaskRepository(db)
	actionRepo := automationrepo.NewActionRepository(db)
	runRepo := automationrepo.NewRunRepository(db)
	ctx := context.Background()

	task := newRuntimeTestTask(automationmodel.AutomationTaskStatusPaused)
	if err := taskRepo.Create(ctx, db, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	action := &automationmodel.AutomationTaskAction{
		TaskID:      task.ID,
		ActionType:  automationmodel.AutomationActionTypeSendNotification,
		ActionOrder: 1,
		Enabled:     true,
		Config:      map[string]interface{}{},
	}
	if err := actionRepo.BatchCreate(ctx, db, []*automationmodel.AutomationTaskAction{action}); err != nil {
		t.Fatalf("create action: %v", err)
	}
	run := newRuntimeTestRun(task.ID, automationmodel.AutomationTriggerSourceScheduler)
	if err := runRepo.CreateTaskRun(ctx, db, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	actionExecutor := &countingActionExecutor{}
	executor := NewExecutorWithActionExecutors(db, taskRepo, actionRepo, runRepo, actionExecutor)
	if err := executor.ExecuteTaskRun(ctx, run.ID); err != nil {
		t.Fatalf("execute task run: %v", err)
	}

	updatedRun, err := runRepo.GetTaskRunByID(ctx, db, run.ID)
	if err != nil {
		t.Fatalf("reload run: %v", err)
	}
	if updatedRun.Status != automationmodel.AutomationTaskRunStatusCancelled {
		t.Fatalf("expected cancelled run, got %s", updatedRun.Status)
	}
	if updatedRun.ErrorSummary == nil || *updatedRun.ErrorSummary == "" {
		t.Fatal("expected cancellation reason")
	}
	actionRuns, err := runRepo.ListActionRunsByTaskRunID(ctx, db, run.ID)
	if err != nil {
		t.Fatalf("list action runs: %v", err)
	}
	if len(actionRuns) != 0 {
		t.Fatalf("expected no action runs for cancelled task, got %d", len(actionRuns))
	}
	if actionExecutor.count != 0 {
		t.Fatalf("expected action executor not to run, got %d calls", actionExecutor.count)
	}

	updatedTask, err := taskRepo.GetByIDAnyScope(ctx, db, task.ID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if updatedTask.LastRunAt != nil || updatedTask.LastRunStatus != nil {
		t.Fatalf("cancelled queued run should not change last_run fields: %#v %#v", updatedTask.LastRunAt, updatedTask.LastRunStatus)
	}
}

func newRuntimeTestDB() (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(
		&automationmodel.AutomationTask{},
		&automationmodel.AutomationTaskAction{},
		&automationmodel.AutomationTaskRun{},
		&automationmodel.AutomationActionRun{},
	); err != nil {
		return nil, err
	}
	return db, nil
}

func newRuntimeTestTask(status automationmodel.AutomationTaskStatus) *automationmodel.AutomationTask {
	return &automationmodel.AutomationTask{
		OrganizationID: "org-1",
		WorkspaceID:    "workspace-1",
		Name:           "Runtime smoke test",
		Status:         status,
		TriggerType:    automationmodel.AutomationTriggerTypeSchedule,
		ScheduleType:   automationmodel.AutomationScheduleTypeOnce,
		Timezone:       "UTC",
		ScheduleConfig: map[string]interface{}{"run_at": time.Now().Add(-time.Minute).Format(time.RFC3339)},
		SourceType:     automationmodel.AutomationSourceTypeManual,
		CreatedBy:      "account-1",
		UpdatedBy:      "account-1",
	}
}

func newRuntimeTestRun(taskID string, triggerSource automationmodel.AutomationTriggerSource) *automationmodel.AutomationTaskRun {
	return &automationmodel.AutomationTaskRun{
		TaskID:        taskID,
		TriggerSource: triggerSource,
		ScheduledFor:  time.Now().Add(-time.Minute),
		Status:        automationmodel.AutomationTaskRunStatusQueued,
	}
}

type failingLastRunTaskRepository struct {
	automationrepo.TaskRepository
	err error
}

func (r *failingLastRunTaskRepository) UpdateLastRun(ctx context.Context, tx *gorm.DB, taskID string, finishedAt time.Time, status string) error {
	return r.err
}

type countingActionExecutor struct {
	count int
}

func (e *countingActionExecutor) ActionType() automationmodel.AutomationActionType {
	return automationmodel.AutomationActionTypeSendNotification
}

func (e *countingActionExecutor) ExecuteAction(ctx context.Context, req automationaction.ActionExecutionRequest) (*automationaction.ActionExecutionResult, error) {
	e.count++
	return &automationaction.ActionExecutionResult{}, nil
}
