package worker

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
)

type datasetProjectionCleanerStub struct {
	datasetIDs []uuid.UUID
	err        error
}

func (s *datasetProjectionCleanerStub) DeleteDatasetProjection(_ context.Context, datasetID uuid.UUID) error {
	s.datasetIDs = append(s.datasetIDs, datasetID)
	return s.err
}

func TestDatasetPurgeHandlerDeletesProjectionByDatasetID(t *testing.T) {
	cleaner := &datasetProjectionCleanerStub{}
	handler := &DatasetPurgeHandler{cleaner: cleaner}
	datasetID := uuid.New()
	event := &model.GraphOutboxEvent{
		DatasetID: datasetID,
		EventType: model.GraphOutboxEventDatasetPurge,
	}

	if err := handler.Process(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if len(cleaner.datasetIDs) != 1 || cleaner.datasetIDs[0] != datasetID {
		t.Fatalf("purged datasets = %v, want [%s]", cleaner.datasetIDs, datasetID)
	}
}

func TestDatasetPurgeHandlerRejectsOtherEventTypes(t *testing.T) {
	handler := &DatasetPurgeHandler{cleaner: &datasetProjectionCleanerStub{}}
	err := handler.Process(context.Background(), &model.GraphOutboxEvent{EventType: model.GraphOutboxEventRun})
	if err == nil {
		t.Fatal("expected unsupported event type error")
	}
}
