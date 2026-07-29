package worker

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
)

type datasetProjectionCleaner interface {
	DeleteDatasetProjection(context.Context, uuid.UUID) error
}

type DatasetPurgeHandler struct {
	cleaner datasetProjectionCleaner
}

func NewDatasetPurgeHandler(service *graphflow.Service) *DatasetPurgeHandler {
	return &DatasetPurgeHandler{cleaner: service}
}

func (h *DatasetPurgeHandler) Process(ctx context.Context, event *model.GraphOutboxEvent) error {
	if h == nil || h.cleaner == nil || event == nil {
		return fmt.Errorf("dataset graph purge handler is not configured")
	}
	if event.EventType != model.GraphOutboxEventDatasetPurge {
		return fmt.Errorf("unsupported dataset graph purge event type")
	}
	return h.cleaner.DeleteDatasetProjection(ctx, event.DatasetID)
}
