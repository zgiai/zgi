package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/pkg/queue"
)

const TypeDataLibraryDatasetRefSync = "dataset_ref:sync"

type DatasetRefSyncPayload struct {
	RefID        string `json:"ref_id"`
	AssetID      string `json:"asset_id"`
	DatasetID    string `json:"dataset_id"`
	GenerationNo int64  `json:"generation_no"`
	SyncRunID    string `json:"sync_run_id"`
}

func (d *FileProcessTaskDispatcher) EnqueueDatasetRefSync(ctx context.Context, refID uuid.UUID, assetID uuid.UUID, datasetID string, generationNo int64, syncRunID uuid.UUID) error {
	if d == nil || d.taskManager == nil {
		return nil
	}
	task, err := NewDatasetRefSyncTask(refID, assetID, datasetID, generationNo, syncRunID, d.taskManager)
	if err != nil {
		return err
	}
	_, err = d.taskManager.EnqueueTask(task, asynq.Queue("chunking"))
	return err
}

func NewDatasetRefSyncTask(refID uuid.UUID, assetID uuid.UUID, datasetID string, generationNo int64, syncRunID uuid.UUID, taskManager *queue.TaskManager) (*asynq.Task, error) {
	if refID == uuid.Nil {
		return nil, fmt.Errorf("ref_id is required")
	}
	if assetID == uuid.Nil {
		return nil, fmt.Errorf("asset_id is required")
	}
	if datasetID == "" {
		return nil, fmt.Errorf("dataset_id is required")
	}
	if generationNo <= 0 {
		return nil, fmt.Errorf("generation_no is required")
	}
	if syncRunID == uuid.Nil {
		return nil, fmt.Errorf("sync_run_id is required")
	}
	payload, err := json.Marshal(DatasetRefSyncPayload{
		RefID:        refID.String(),
		AssetID:      assetID.String(),
		DatasetID:    datasetID,
		GenerationNo: generationNo,
		SyncRunID:    syncRunID.String(),
	})
	if err != nil {
		return nil, err
	}
	taskType := TypeDataLibraryDatasetRefSync
	if taskManager != nil {
		taskType = taskManager.GetTaskTypeWithPrefix(taskType)
	}
	return asynq.NewTask(taskType, payload, asynq.Queue("chunking"), asynq.MaxRetry(0), asynq.Timeout(60*time.Minute)), nil
}

func RegisterDatasetRefSyncTaskHandler(registry TaskHandlerRegistry, runner *DatasetRefSyncRunner, taskManager *queue.TaskManager) {
	if registry == nil || runner == nil {
		return
	}
	taskType := TypeDataLibraryDatasetRefSync
	if taskManager != nil {
		taskType = taskManager.GetTaskTypeWithPrefix(taskType)
	}
	registry.Register(taskType, NewDatasetRefSyncTaskHandler(runner))
}

func NewDatasetRefSyncTaskHandler(runner *DatasetRefSyncRunner) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, task *asynq.Task) error {
		if runner == nil {
			return fmt.Errorf("dataset ref sync runner is not configured: %w", asynq.SkipRetry)
		}
		var payload DatasetRefSyncPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return fmt.Errorf("unmarshal dataset ref sync task payload: %v: %w", err, asynq.SkipRetry)
		}
		return runner.Run(ctx, payload)
	}
}
