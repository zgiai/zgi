package workflowtest

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

type blockingRunner struct{}

func (blockingRunner) RunCase(ctx context.Context, req RunCaseRequest) (*RunCaseResult, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

type passingJudge struct{}

func (passingJudge) JudgeCase(ctx context.Context, req JudgeRequest) (*JudgeResult, error) {
	return &JudgeResult{Status: BatchItemStatusPassed, Reason: "ok", Confidence: 1}, nil
}

type noopSummarizer struct{}

func (noopSummarizer) SummarizeBatch(ctx context.Context, req SummaryRequest) (*SummaryResult, error) {
	return &SummaryResult{Summary: "summary"}, nil
}

func TestStartBatchDoesNotMarkAllItemsRunning(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	now := testNow()

	expectGetBatch(mock, "agent-1", "batch-1", batchRows().
		AddRow("batch-1", "agent-1", "Batch", BatchStatusQueued, 2, 0, 0, 0, "judge", "", "", "draft", nil, "current_draft", "", now, now))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_batches" SET "status"=$1,"updated_at"=$2 WHERE agent_id = $3 AND id = $4 AND status = $5`)).
		WithArgs(BatchStatusRunning, sqlmock.AnyArg(), "agent-1", "batch-1", BatchStatusQueued).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	expectGetBatch(mock, "agent-1", "batch-1", batchRows().
		AddRow("batch-1", "agent-1", "Batch", BatchStatusRunning, 2, 0, 0, 0, "judge", "", "", "draft", nil, "current_draft", "", now, now))

	batch, err := service.StartBatch(context.Background(), "agent-1", "batch-1")

	require.NoError(t, err)
	require.Equal(t, BatchStatusRunning, batch.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestExecuteBatchFailsTimedOutItem(t *testing.T) {
	previousTimeout := batchItemExecutionTimeout
	batchItemExecutionTimeout = 20 * time.Millisecond
	t.Cleanup(func() { batchItemExecutionTimeout = previousTimeout })

	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	now := testNow()
	turns := `[{"role":"user","content":"question"}]`

	expectGetBatch(mock, "agent-1", "batch-1", batchRows().
		AddRow("batch-1", "agent-1", "Batch", BatchStatusRunning, 1, 0, 0, 0, "judge", "", "", "draft", nil, "current_draft", "", now, now))
	expectListBatchItems(mock, "agent-1", "batch-1", batchItemRows().
		AddRow("item-1", "agent-1", "batch-1", "case-1", `{"id":"case-1","content":"question","expected_result":"answer","question_type":"core","turns":`+turns+`}`, string(BatchItemStatusPending), "", `{}`, "", "", "", 0, now, now))
	expectGetBatch(mock, "agent-1", "batch-1", batchRows().
		AddRow("batch-1", "agent-1", "Batch", BatchStatusRunning, 1, 0, 0, 0, "judge", "", "", "draft", nil, "current_draft", "", now, now))
	expectUpdateBatchItemStatus(mock, "agent-1", "item-1", string(BatchItemStatusPending), string(BatchItemStatusRunning))
	expectUpdateBatchItemResult(mock, "agent-1", "item-1", string(BatchItemStatusFailed), "测试问题执行超时")
	expectTouchBatch(mock, "agent-1", "batch-1")
	expectUpdateBatchSummary(mock, "agent-1", "batch-1", BatchStatusCompleted, 0, 1, 0, "summary")
	expectGetBatch(mock, "agent-1", "batch-1", batchRows().
		AddRow("batch-1", "agent-1", "Batch", BatchStatusCompleted, 1, 0, 1, 0, "judge", "", "", "draft", nil, "current_draft", "summary", now, now))

	batch, err := service.ExecuteStartedBatchWithRunnerJudgeAndSummarizer(context.Background(), "agent-1", "batch-1", blockingRunner{}, passingJudge{}, noopSummarizer{})

	require.NoError(t, err)
	require.Equal(t, BatchStatusCompleted, batch.Status)
	require.Equal(t, 1, batch.FailedCount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRecoverStaleRunningBatchesStopsBatchAndFailsUnfinishedItems(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	now := testNow()
	staleBefore := now.Add(-1 * time.Hour)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "id" FROM "workflow_test_batches" WHERE agent_id = $1 AND status = $2 AND updated_at < $3`)).
		WithArgs("agent-1", BatchStatusRunning, staleBefore).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("batch-1"))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_batches" SET "status"=$1,"summary"=$2,"updated_at"=$3 WHERE agent_id = $4 AND id IN ($5)`)).
		WithArgs(BatchStatusStopped, batchStaleFailureMessage, sqlmock.AnyArg(), "agent-1", "batch-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_batch_items" SET "error"=$1,"status"=$2,"updated_at"=$3 WHERE agent_id = $4 AND batch_id IN ($5) AND status IN ($6,$7)`)).
		WithArgs(batchStaleFailureMessage, string(BatchItemStatusFailed), sqlmock.AnyArg(), "agent-1", "batch-1", string(BatchItemStatusPending), string(BatchItemStatusRunning)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	recovered, err := service.RecoverStaleRunningBatches(context.Background(), "agent-1", staleBefore)

	require.NoError(t, err)
	require.Equal(t, int64(1), recovered)
	require.NoError(t, mock.ExpectationsWereMet())
}

func batchRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id",
		"agent_id",
		"name",
		"status",
		"case_count",
		"passed_count",
		"failed_count",
		"review_count",
		"judge_prompt_snapshot",
		"judge_model_provider_snapshot",
		"judge_model_name_snapshot",
		"workflow_version_mode",
		"workflow_version_uuid",
		"workflow_version_label",
		"summary",
		"created_at",
		"updated_at",
	})
}

func batchItemRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id",
		"agent_id",
		"batch_id",
		"case_id",
		"case_snapshot",
		"status",
		"workflow_run_id",
		"outputs",
		"error",
		"judge_reason",
		"judge_suggestion",
		"judge_confidence",
		"created_at",
		"updated_at",
	})
}

func expectGetBatch(mock sqlmock.Sqlmock, agentID string, batchID string, rows *sqlmock.Rows) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workflow_test_batches" WHERE agent_id = $1 AND id = $2 ORDER BY "workflow_test_batches"."id" LIMIT $3`)).
		WithArgs(agentID, batchID, 1).
		WillReturnRows(rows)
}

func expectListBatchItems(mock sqlmock.Sqlmock, agentID string, batchID string, rows *sqlmock.Rows) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workflow_test_batch_items" WHERE agent_id = $1 AND batch_id = $2 ORDER BY created_at ASC`)).
		WithArgs(agentID, batchID).
		WillReturnRows(rows)
}

func expectUpdateBatchItemStatus(mock sqlmock.Sqlmock, agentID string, itemID string, currentStatus string, nextStatus string) {
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_batch_items" SET "status"=$1,"updated_at"=$2 WHERE agent_id = $3 AND id = $4 AND status = $5`)).
		WithArgs(nextStatus, sqlmock.AnyArg(), agentID, itemID, currentStatus).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func expectUpdateBatchItemResult(mock sqlmock.Sqlmock, agentID string, itemID string, status string, errorMessage string) {
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_batch_items" SET "error"=$1,"judge_confidence"=$2,"judge_reason"=$3,"judge_suggestion"=$4,"outputs"=$5,"status"=$6,"updated_at"=$7,"workflow_run_id"=$8 WHERE agent_id = $9 AND id = $10 AND status = $11`)).
		WithArgs(errorMessage, float64(0), "", "", "{}", status, sqlmock.AnyArg(), "", agentID, itemID, string(BatchItemStatusRunning)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func expectTouchBatch(mock sqlmock.Sqlmock, agentID string, batchID string) {
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_batches" SET "updated_at"=$1 WHERE agent_id = $2 AND id = $3`)).
		WithArgs(sqlmock.AnyArg(), agentID, batchID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func expectUpdateBatchSummary(mock sqlmock.Sqlmock, agentID string, batchID string, status string, passed, failed, review int, summary string) {
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_batches" SET "failed_count"=$1,"passed_count"=$2,"review_count"=$3,"status"=$4,"summary"=$5,"updated_at"=$6 WHERE agent_id = $7 AND id = $8`)).
		WithArgs(failed, passed, review, status, summary, sqlmock.AnyArg(), agentID, batchID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}
