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
	calls int
	err   error
}

func (s *outboxProcessorStub) Process(context.Context, *model.GraphOutboxEvent) error {
	s.calls++
	return s.err
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
