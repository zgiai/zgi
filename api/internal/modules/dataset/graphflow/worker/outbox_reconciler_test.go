package worker

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type outboxProcessorStub struct {
	calls                int
	terminalFailureCalls int
	err                  error
}

func (s *outboxProcessorStub) Process(context.Context, *model.GraphOutboxEvent) error {
	s.calls++
	return s.err
}

func (s *outboxProcessorStub) HandleTerminalFailure(context.Context, *model.GraphOutboxEvent, error) error {
	s.terminalFailureCalls++
	return nil
}

func TestOutboxReconcilerRecoversStaleLeaseAndConfirmsDuplicateDelivery(t *testing.T) {
	dsn := fmt.Sprintf("file:outbox-reconciler-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.GraphOutboxEvent{}); err != nil {
		t.Fatal(err)
	}
	claimedAt := time.Now().UTC().Add(-time.Hour)
	event := &model.GraphOutboxEvent{
		OrganizationID: uuid.New(),
		DatasetID:      uuid.New(),
		EventType:      model.GraphOutboxEventVisibility,
		AggregateKey:   "visibility:1",
		Status:         model.GraphOutboxStatusProcessing,
		ClaimedAt:      &claimedAt,
	}
	if err := db.Create(event).Error; err != nil {
		t.Fatal(err)
	}
	processor := &outboxProcessorStub{}
	reconciler := NewOutboxReconciler(db, nil, processor, nil)
	if err := reconciler.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if processor.calls != 1 {
		t.Fatalf("processor calls = %d, want 1", processor.calls)
	}
	var persisted model.GraphOutboxEvent
	if err := db.First(&persisted, "id = ?", event.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Status != model.GraphOutboxStatusConfirmed {
		t.Fatalf("event status = %q, want confirmed", persisted.Status)
	}
	if err := reconciler.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if processor.calls != 1 {
		t.Fatalf("confirmed event was delivered again: %d calls", processor.calls)
	}
}

func TestOutboxReconcilerPreservesFailureAndStopsAfterMaximumAttempts(t *testing.T) {
	db := openOutboxReconcilerTestDB(t)
	event := &model.GraphOutboxEvent{
		OrganizationID: uuid.New(),
		DatasetID:      uuid.New(),
		EventType:      model.GraphOutboxEventRun,
		AggregateKey:   "run:permanent-failure",
		Status:         model.GraphOutboxStatusPending,
		AttemptCount:   maxOutboxAttempts - 1,
	}
	if err := db.Create(event).Error; err != nil {
		t.Fatal(err)
	}
	processorErr := fmt.Errorf("document task insert failed")
	processor := &outboxProcessorStub{err: processorErr}
	reconciler := NewOutboxReconciler(db, processor, nil, nil)

	if err := reconciler.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	var persisted model.GraphOutboxEvent
	if err := db.First(&persisted, "id = ?", event.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Status != model.GraphOutboxStatusFailed {
		t.Fatalf("event status = %q, want failed", persisted.Status)
	}
	if persisted.ErrorMessage == nil || *persisted.ErrorMessage != processorErr.Error() {
		t.Fatalf("event error = %v, want %q", persisted.ErrorMessage, processorErr.Error())
	}
	if processor.terminalFailureCalls != 1 {
		t.Fatalf("terminal failure calls = %d, want 1", processor.terminalFailureCalls)
	}
	if err := reconciler.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if processor.calls != 1 {
		t.Fatalf("failed event was delivered again: %d calls", processor.calls)
	}
}

func TestOutboxReconcilerRetriesWithOriginalErrorBeforeMaximumAttempts(t *testing.T) {
	db := openOutboxReconcilerTestDB(t)
	event := &model.GraphOutboxEvent{
		OrganizationID: uuid.New(),
		DatasetID:      uuid.New(),
		EventType:      model.GraphOutboxEventRun,
		AggregateKey:   "run:retryable-failure",
		Status:         model.GraphOutboxStatusPending,
	}
	if err := db.Create(event).Error; err != nil {
		t.Fatal(err)
	}
	processorErr := fmt.Errorf("temporary queue failure")
	reconciler := NewOutboxReconciler(db, &outboxProcessorStub{err: processorErr}, nil, nil)

	if err := reconciler.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	var persisted model.GraphOutboxEvent
	if err := db.First(&persisted, "id = ?", event.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Status != model.GraphOutboxStatusPending {
		t.Fatalf("event status = %q, want pending", persisted.Status)
	}
	if persisted.ErrorMessage == nil || *persisted.ErrorMessage != processorErr.Error() {
		t.Fatalf("event error = %v, want %q", persisted.ErrorMessage, processorErr.Error())
	}
}

func TestOutboxReconcilerDispatchesDatasetPurge(t *testing.T) {
	db := openOutboxReconcilerTestDB(t)
	event := &model.GraphOutboxEvent{
		OrganizationID: uuid.New(),
		DatasetID:      uuid.New(),
		EventType:      model.GraphOutboxEventDatasetPurge,
		AggregateKey:   "dataset-purge:test",
		Status:         model.GraphOutboxStatusPending,
	}
	if err := db.Create(event).Error; err != nil {
		t.Fatal(err)
	}
	purge := &outboxProcessorStub{}
	if err := NewOutboxReconciler(db, nil, nil, purge).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if purge.calls != 1 {
		t.Fatalf("dataset purge calls = %d, want 1", purge.calls)
	}
}

func TestOutboxReconcilerKeepsDatasetPurgeRetryable(t *testing.T) {
	db := openOutboxReconcilerTestDB(t)
	event := &model.GraphOutboxEvent{
		OrganizationID: uuid.New(),
		DatasetID:      uuid.New(),
		EventType:      model.GraphOutboxEventDatasetPurge,
		AggregateKey:   "dataset-purge:retry",
		Status:         model.GraphOutboxStatusPending,
		AttemptCount:   maxOutboxAttempts - 1,
	}
	if err := db.Create(event).Error; err != nil {
		t.Fatal(err)
	}
	reconciler := NewOutboxReconciler(db, nil, nil, &outboxProcessorStub{err: fmt.Errorf("neo4j unavailable")})
	now := time.Now().UTC()
	reconciler.now = func() time.Time { return now }
	if err := reconciler.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	var persisted model.GraphOutboxEvent
	if err := db.First(&persisted, "id = ?", event.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Status != model.GraphOutboxStatusPending {
		t.Fatalf("dataset purge status = %q, want pending", persisted.Status)
	}
	if persisted.AvailableAt.Before(now.Add(datasetPurgeRetryDelay)) {
		t.Fatalf("dataset purge retry scheduled at %s", persisted.AvailableAt)
	}
}

func openOutboxReconcilerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:outbox-reconciler-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.GraphOutboxEvent{}); err != nil {
		t.Fatal(err)
	}
	return db
}
