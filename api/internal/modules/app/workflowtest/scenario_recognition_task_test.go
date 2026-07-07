package workflowtest

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
)

func TestNewScenarioRecognitionTaskAsynqTaskPayloadAndType(t *testing.T) {
	task, err := NewScenarioRecognitionTaskAsynqTask("task-1", nil)

	require.NoError(t, err)
	require.Equal(t, WorkflowTestScenarioRecognitionTaskType, task.Type())
	require.JSONEq(t, `{"task_id":"task-1"}`, string(task.Payload()))
}

func TestScenarioRecognitionTaskAsynqOptions(t *testing.T) {
	opts := scenarioRecognitionTaskAsynqOptions()

	require.Len(t, opts, 3)
	requireTaskOption(t, opts, asynq.QueueOpt, "default")
	requireTaskOption(t, opts, asynq.MaxRetryOpt, 0)
	requireTaskOption(t, opts, asynq.TimeoutOpt, 10*time.Minute)
}

func TestNewScenarioRecognitionTaskHandlerReturnsSkipRetryForBadPayload(t *testing.T) {
	handler := NewScenarioRecognitionTaskHandler(NewService(nil), &fakeLLMClient{})

	err := handler(context.Background(), asynq.NewTask(WorkflowTestScenarioRecognitionTaskType, []byte(`{`)))
	require.Error(t, err)
	require.True(t, errors.Is(err, asynq.SkipRetry))

	err = handler(context.Background(), asynq.NewTask(WorkflowTestScenarioRecognitionTaskType, []byte(`{}`)))
	require.Error(t, err)
	require.True(t, errors.Is(err, asynq.SkipRetry))
}

func TestRepositoryRecoverStaleRunningScenarioRecognitionTasksMarksOldActiveTasksFailed(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	repo := NewRepository(db)
	ctx := context.Background()
	staleBefore := time.Date(2026, 5, 25, 18, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_scenario_recognition_tasks" SET "completed_at"=$1,"error"=$2,"status"=$3,"updated_at"=$4 WHERE status IN ($5,$6,$7) AND updated_at < $8`)).
		WithArgs(sqlmock.AnyArg(), "stale failure", GenerationTaskStatusFailed, sqlmock.AnyArg(), GenerationTaskStatusQueued, GenerationTaskStatusRunning, GenerationTaskStatusCanceling, staleBefore).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	recovered, err := repo.RecoverStaleRunningScenarioRecognitionTasks(ctx, staleBefore, "stale failure", time.Now())

	require.NoError(t, err)
	require.Equal(t, int64(2), recovered)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryRecoverStaleRunningScenarioRecognitionTasksForAgentOnlyMarksRouteAgentTasks(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	repo := NewRepository(db)
	ctx := context.Background()
	staleBefore := time.Date(2026, 5, 25, 18, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_scenario_recognition_tasks" SET "completed_at"=$1,"error"=$2,"status"=$3,"updated_at"=$4 WHERE (status IN ($5,$6,$7) AND updated_at < $8) AND agent_id = $9`)).
		WithArgs(sqlmock.AnyArg(), "stale failure", GenerationTaskStatusFailed, sqlmock.AnyArg(), GenerationTaskStatusQueued, GenerationTaskStatusRunning, GenerationTaskStatusCanceling, staleBefore, "agent-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	recovered, err := repo.RecoverStaleRunningScenarioRecognitionTasksForAgent(ctx, "agent-1", staleBefore, "stale failure", time.Now())

	require.NoError(t, err)
	require.Equal(t, int64(1), recovered)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryCancelScenarioRecognitionTaskCompletesQueuedTaskImmediately(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_scenario_recognition_tasks" SET "cancel_requested_at"=$1,"completed_at"=$2,"error"=$3,"status"=$4,"updated_at"=$5 WHERE agent_id = $6 AND id = $7 AND status = $8`)).
		WithArgs(now, now, "", GenerationTaskStatusCanceled, now, "agent-1", "task-queued", GenerationTaskStatusQueued).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	changed, err := repo.CancelScenarioRecognitionTask(ctx, "agent-1", "task-queued", now)

	require.NoError(t, err)
	require.True(t, changed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryCancelScenarioRecognitionTaskChangesRunningToCanceling(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 5, 28, 10, 5, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_scenario_recognition_tasks" SET "cancel_requested_at"=$1,"completed_at"=$2,"error"=$3,"status"=$4,"updated_at"=$5 WHERE agent_id = $6 AND id = $7 AND status = $8`)).
		WithArgs(now, now, "", GenerationTaskStatusCanceled, now, "agent-1", "task-running", GenerationTaskStatusQueued).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_scenario_recognition_tasks" SET "cancel_requested_at"=$1,"status"=$2,"updated_at"=$3 WHERE agent_id = $4 AND id = $5 AND status = $6`)).
		WithArgs(now, GenerationTaskStatusCanceling, now, "agent-1", "task-running", GenerationTaskStatusRunning).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	changed, err := repo.CancelScenarioRecognitionTask(ctx, "agent-1", "task-running", now)

	require.NoError(t, err)
	require.True(t, changed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCancelScenarioRecognitionTaskCancelsLocalWorkerContext(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	canceler := &fakeTaskCanceler{}
	service.SetTaskCanceler(canceler)
	ctx := context.Background()
	now := time.Date(2026, 5, 28, 10, 5, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_scenario_recognition_tasks" SET "cancel_requested_at"=$1,"completed_at"=$2,"error"=$3,"status"=$4,"updated_at"=$5 WHERE agent_id = $6 AND id = $7 AND status = $8`)).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "", GenerationTaskStatusCanceled, sqlmock.AnyArg(), "agent-1", "task-1", GenerationTaskStatusQueued).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_scenario_recognition_tasks" SET "cancel_requested_at"=$1,"status"=$2,"updated_at"=$3 WHERE agent_id = $4 AND id = $5 AND status = $6`)).
		WithArgs(sqlmock.AnyArg(), GenerationTaskStatusCanceling, sqlmock.AnyArg(), "agent-1", "task-1", GenerationTaskStatusRunning).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workflow_test_scenario_recognition_tasks" WHERE agent_id = $1 AND id = $2 ORDER BY "workflow_test_scenario_recognition_tasks"."id" LIMIT $3`)).
		WithArgs("agent-1", "task-1", 1).
		WillReturnRows(scenarioRecognitionTaskRows().AddRow(
			"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusCanceling,
			"prompt", "context", "workflow", "", "", 0, 0, "", &now, &now, nil, now, now,
		))

	task, err := service.CancelScenarioRecognitionTask(ctx, "agent-1", "task-1")

	require.NoError(t, err)
	require.NotNil(t, task)
	require.Equal(t, GenerationTaskStatusCanceling, task.Status)
	require.Equal(t, []string{"task-1"}, canceler.taskIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryListQueuedScenarioRecognitionTasks(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workflow_test_scenario_recognition_tasks" WHERE status = $1 ORDER BY created_at ASC LIMIT $2`)).
		WithArgs(GenerationTaskStatusQueued, localWorkerClaimLimit).
		WillReturnRows(scenarioRecognitionTaskRows().AddRow(
			"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusQueued,
			"prompt", "context", "workflow", "", "", 0, 0, "", nil, nil, nil, now, now,
		))

	tasks, err := repo.ListQueuedScenarioRecognitionTasks(ctx, localWorkerClaimLimit)

	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, "task-1", tasks[0].ID)
	require.NoError(t, mock.ExpectationsWereMet())
}
