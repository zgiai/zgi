package pause

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPostgresV2ConcurrentSequenceAndResumeClaim(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not configured")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open postgres admin connection: %v", err)
	}
	schema := "workflow_v2_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	defer admin.Exec("DROP SCHEMA " + schema + " CASCADE")

	scopedDSN, err := postgresDSNWithSearchPath(dsn, schema)
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.Open(scopedDSN), &gorm.Config{})
	if err != nil {
		t.Fatalf("open scoped postgres connection: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("open scoped postgres connection pool: %v", err)
	}
	// Model many concurrent workflow streams without trying to allocate one
	// database connection per stream. Production applies the same bounded-pool
	// backpressure before PostgreSQL reaches max_connections.
	sqlDB.SetMaxOpenConns(32)
	sqlDB.SetMaxIdleConns(8)
	if err := db.AutoMigrate(&pauseTestWorkflowRun{}, &RunPause{}, &RunPauseReason{}, &RunEvent{}, &RuntimeOutbox{}); err != nil {
		t.Fatalf("migrate scoped tables: %v", err)
	}
	if err := db.Exec("CREATE UNIQUE INDEX workflow_event_sequence_unique ON workflow_run_events (tenant_id, workflow_run_id, sequence)").Error; err != nil {
		t.Fatalf("create event sequence index: %v", err)
	}
	if err := db.Create(&pauseTestWorkflowRun{
		ID: "00000000-0000-0000-0000-000000000101", RuntimeProtocolVersion: 2,
		ExecutionGeneration: 1, Status: "running",
	}).Error; err != nil {
		t.Fatalf("create event run: %v", err)
	}

	service := NewService(db)
	const appendCount = 32
	var appendWG sync.WaitGroup
	appendErrors := make(chan error, appendCount)
	for index := 0; index < appendCount; index++ {
		appendWG.Add(1)
		go func(index int) {
			defer appendWG.Done()
			_, err := service.AppendEventPayload(context.Background(), AppendEventParams{
				TenantID:      "00000000-0000-0000-0000-000000000201",
				AppID:         "00000000-0000-0000-0000-000000000301",
				WorkflowRunID: "00000000-0000-0000-0000-000000000101",
				EventType:     EventNodeStarted, EventData: map[string]interface{}{"index": index},
			})
			appendErrors <- err
		}(index)
	}
	appendWG.Wait()
	close(appendErrors)
	for err := range appendErrors {
		if err != nil {
			t.Fatalf("concurrent event append: %v", err)
		}
	}
	events, err := service.ListEvents(context.Background(), "00000000-0000-0000-0000-000000000201", "00000000-0000-0000-0000-000000000101", 0, appendCount)
	if err != nil {
		t.Fatal(err)
	}
	if len(events.Events) != appendCount {
		t.Fatalf("event count = %d, want %d", len(events.Events), appendCount)
	}
	for index, event := range events.Events {
		if event.Sequence != index+1 {
			t.Fatalf("event sequence[%d] = %d, want %d", index, event.Sequence, index+1)
		}
	}

	const eventsPerBatch = 16
	type batchRun struct {
		runID       string
		executionID string
	}
	for _, concurrentRunCount := range []int{1, 50, 200} {
		t.Run(fmt.Sprintf("%d_parallel_runs", concurrentRunCount), func(t *testing.T) {
			batchRuns := make([]batchRun, concurrentRunCount)
			for index := range batchRuns {
				batchRuns[index] = batchRun{runID: uuid.NewString(), executionID: uuid.NewString()}
				if err := db.Create(&pauseTestWorkflowRun{
					ID: batchRuns[index].runID, RuntimeProtocolVersion: 2, ExecutionGeneration: 1,
					ActiveExecutionID: &batchRuns[index].executionID, Status: "running",
				}).Error; err != nil {
					t.Fatalf("create batch run %d: %v", index, err)
				}
			}
			startedAt := time.Now()
			batchErrors := make(chan error, concurrentRunCount)
			var batchWG sync.WaitGroup
			for _, run := range batchRuns {
				run := run
				batchWG.Add(1)
				go func() {
					defer batchWG.Done()
					drafts := make([]EventDraft, eventsPerBatch)
					for index := range drafts {
						drafts[index] = EventDraft{
							EventType: EventNodeStarted, IdempotencyKey: fmt.Sprintf("node:%s:%d", run.executionID, index),
							EventData: map[string]interface{}{"node_id": fmt.Sprintf("node-%d", index)},
						}
					}
					_, err := service.AppendEventBatch(context.Background(), AppendEventBatchRequest{
						TenantID: "00000000-0000-0000-0000-000000000201", AppID: "00000000-0000-0000-0000-000000000301",
						WorkflowRunID: run.runID, FlushReason: "postgres_concurrency_test",
						Fence: RuntimeFence{ExpectedExecutionID: run.executionID, ExpectedExecutionGeneration: 1}, Events: drafts,
					})
					batchErrors <- err
				}()
			}
			batchWG.Wait()
			close(batchErrors)
			for err := range batchErrors {
				if err != nil {
					t.Fatalf("parallel event batch: %v", err)
				}
			}
			t.Logf("persisted %d runs x %d events in %s", concurrentRunCount, eventsPerBatch, time.Since(startedAt))
			for _, run := range batchRuns {
				payload, err := service.ListEvents(context.Background(), "00000000-0000-0000-0000-000000000201", run.runID, 0, eventsPerBatch)
				if err != nil {
					t.Fatal(err)
				}
				if len(payload.Events) != eventsPerBatch || payload.Events[eventsPerBatch-1].Sequence != eventsPerBatch {
					t.Fatalf("run %s events=%d last=%d", run.runID, len(payload.Events), payload.Events[len(payload.Events)-1].Sequence)
				}
			}
		})
	}

	runID := "00000000-0000-0000-0000-000000000102"
	pauseID := "00000000-0000-0000-0000-000000000402"
	if err := db.Create(&pauseTestWorkflowRun{ID: runID, RuntimeProtocolVersion: 2, ExecutionGeneration: 1, Status: "paused"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&RunPause{
		ID: pauseID, TenantID: "00000000-0000-0000-0000-000000000201",
		AppID: "00000000-0000-0000-0000-000000000301", WorkflowRunID: runID,
		NodeID: "approval", Reason: ReasonTypeApprovalRequired, StateJSON: `{"version":"2"}`,
		Generation: 1, Status: RunPauseStatusResumeReady,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.PrepareResume(context.Background(), runID, pauseID, "form-1"); err != nil {
		t.Fatal(err)
	}

	var claimWG sync.WaitGroup
	type claimResult struct {
		claim *ExecutionClaim
		err   error
	}
	claimResults := make(chan claimResult, 2)
	for index := 0; index < 2; index++ {
		claimWG.Add(1)
		go func() {
			defer claimWG.Done()
			claim, err := service.ClaimResume(context.Background(), runID, pauseID, time.Minute)
			claimResults <- claimResult{claim: claim, err: err}
		}()
	}
	claimWG.Wait()
	close(claimResults)
	succeeded := 0
	conflicted := 0
	var firstClaim *ExecutionClaim
	for result := range claimResults {
		err := result.err
		switch {
		case err == nil:
			succeeded++
			firstClaim = result.claim
		case errors.Is(err, ErrResumeAlreadyRunning):
			conflicted++
		default:
			t.Fatalf("unexpected claim error: %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("claim results succeeded=%d conflicted=%d", succeeded, conflicted)
	}
	if firstClaim == nil {
		t.Fatal("successful claim is missing")
	}
	expiredAt := time.Now().Add(-time.Minute)
	if err := db.Model(&RunPause{}).Where("id = ?", pauseID).Update("lease_expires_at", expiredAt).Error; err != nil {
		t.Fatal(err)
	}
	takeover, err := service.ClaimResume(context.Background(), runID, pauseID, time.Minute)
	if err != nil {
		t.Fatalf("claim expired lease takeover: %v", err)
	}
	if takeover.Generation <= firstClaim.Generation || takeover.ExecutionID == firstClaim.ExecutionID {
		t.Fatalf("takeover claim = %#v, first = %#v", takeover, firstClaim)
	}
	if _, err := service.RenewExecutionLease(context.Background(), *firstClaim, time.Minute); !errors.Is(err, ErrExecutionOwnershipLost) {
		t.Fatalf("old execution renewal error = %v, want ownership lost", err)
	}
	if _, err := service.AppendEventPayload(context.Background(), AppendEventParams{
		TenantID: "00000000-0000-0000-0000-000000000201", AppID: "00000000-0000-0000-0000-000000000301", WorkflowRunID: runID,
		EventType: EventNodeFinished, ExecutionID: firstClaim.ExecutionID,
		ExpectedExecutionID: firstClaim.ExecutionID, ExpectedExecutionGeneration: firstClaim.Generation,
		IdempotencyKey: "node:stale-owner:finished", EventData: map[string]interface{}{"node_id": "stale-owner"},
	}); !errors.Is(err, ErrExecutionOwnershipLost) {
		t.Fatalf("old execution event append error = %v, want ownership lost", err)
	}
	if _, err := service.AppendEventPayload(context.Background(), AppendEventParams{
		TenantID: "00000000-0000-0000-0000-000000000201", AppID: "00000000-0000-0000-0000-000000000301", WorkflowRunID: runID,
		EventType: EventNodeStarted, ExecutionID: takeover.ExecutionID,
		ExpectedExecutionID: takeover.ExecutionID, ExpectedExecutionGeneration: takeover.Generation,
		IdempotencyKey: "node:takeover-owner:started", EventData: map[string]interface{}{"node_id": "takeover-owner"},
	}); err != nil {
		t.Fatalf("takeover execution event append: %v", err)
	}

	multiRunID := "00000000-0000-0000-0000-000000000103"
	multiPauseID := "00000000-0000-0000-0000-000000000403"
	if err := db.Create(&pauseTestWorkflowRun{ID: multiRunID, RuntimeProtocolVersion: 2, ExecutionGeneration: 1, Status: "paused"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&RunPause{
		ID: multiPauseID, TenantID: "00000000-0000-0000-0000-000000000201",
		AppID: "00000000-0000-0000-0000-000000000301", WorkflowRunID: multiRunID,
		NodeID: "approval-a", Reason: ReasonTypeApprovalRequired, StateJSON: `{"version":"2"}`,
		Generation: 1, Status: RunPauseStatusPaused,
	}).Error; err != nil {
		t.Fatal(err)
	}
	for index, formID := range []string{"form-a", "form-b"} {
		if err := db.Create(&RunPauseReason{
			ID: uuid.NewString(), PauseID: multiPauseID, Type: ReasonTypeApprovalRequired,
			NodeID: fmt.Sprintf("approval-%d", index), FormID: formID, Status: RunPauseReasonStatusPending,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	completeResults := make(chan error, 2)
	for _, formID := range []string{"form-a", "form-b"} {
		go func(formID string) {
			_, _, err := service.CompleteReasons(context.Background(), CompleteReasonsParams{
				WorkflowRunID: multiRunID, PauseID: multiPauseID, ReasonType: ReasonTypeApprovalRequired,
				FormID: formID, SubmissionEventID: uuid.NewString(), TriggerID: formID,
			})
			completeResults <- err
		}(formID)
	}
	for range 2 {
		if err := <-completeResults; err != nil {
			t.Fatalf("complete concurrent pause reason: %v", err)
		}
	}
	var outboxCount int64
	if err := db.Model(&RuntimeOutbox{}).Where("workflow_run_id = ?", multiRunID).Count(&outboxCount).Error; err != nil {
		t.Fatal(err)
	}
	if outboxCount != 1 {
		t.Fatalf("resume outbox count = %d, want 1", outboxCount)
	}
}

func postgresDSNWithSearchPath(dsn, schema string) (string, error) {
	if strings.Contains(dsn, "://") {
		parsed, err := url.Parse(dsn)
		if err != nil {
			return "", fmt.Errorf("parse TEST_POSTGRES_DSN: %w", err)
		}
		query := parsed.Query()
		query.Set("search_path", schema)
		parsed.RawQuery = query.Encode()
		return parsed.String(), nil
	}
	return strings.TrimSpace(dsn) + " search_path=" + schema, nil
}
