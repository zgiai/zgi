package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	datalibraryservice "github.com/zgiai/zgi/api/internal/modules/datalibrary/service"
	"github.com/zgiai/zgi/api/pkg/queue"
)

const TypeDataLibraryFileCandidateEmbedding = "data_library:file_candidate_embedding"

type FileCandidateEmbeddingPayload struct {
	OrganizationID      string  `json:"organization_id"`
	WorkspaceID         *string `json:"workspace_id,omitempty"`
	DatasetID           string  `json:"dataset_id"`
	AssetID             string  `json:"asset_id"`
	RequestedBy         string  `json:"requested_by,omitempty"`
	ProcessingRequestID string  `json:"processing_request_id,omitempty"`
}

type fileCandidateEmbeddingRunner interface {
	Run(ctx context.Context, req datalibraryservice.KnowledgeBaseFileCandidateEmbeddingRequest) error
}

func (d *FileProcessTaskDispatcher) EnqueueFileCandidateEmbedding(ctx context.Context, req datalibraryservice.KnowledgeBaseFileCandidateEmbeddingRequest) error {
	if d == nil || d.taskManager == nil {
		return nil
	}
	task, err := NewFileCandidateEmbeddingTask(req, d.taskManager)
	if err != nil {
		return err
	}
	_, err = d.taskManager.EnqueueTask(task, asynq.Queue("chunking"))
	return err
}

func NewFileCandidateEmbeddingTask(req datalibraryservice.KnowledgeBaseFileCandidateEmbeddingRequest, taskManager *queue.TaskManager) (*asynq.Task, error) {
	if req.OrganizationID == "" {
		return nil, fmt.Errorf("organization_id is required")
	}
	if req.DatasetID == "" {
		return nil, fmt.Errorf("dataset_id is required")
	}
	if req.AssetID == uuid.Nil {
		return nil, fmt.Errorf("asset_id is required")
	}
	payload := FileCandidateEmbeddingPayload{
		OrganizationID: req.OrganizationID,
		WorkspaceID:    req.WorkspaceID,
		DatasetID:      req.DatasetID,
		AssetID:        req.AssetID.String(),
		RequestedBy:    req.RequestedBy,
	}
	if req.ProcessingRequestID != uuid.Nil {
		payload.ProcessingRequestID = req.ProcessingRequestID.String()
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	taskType := TypeDataLibraryFileCandidateEmbedding
	if taskManager != nil {
		taskType = taskManager.GetTaskTypeWithPrefix(taskType)
	}
	return asynq.NewTask(taskType, data, asynq.Queue("chunking"), asynq.MaxRetry(0), asynq.Timeout(60*time.Minute)), nil
}

func RegisterFileCandidateEmbeddingTaskHandler(registry TaskHandlerRegistry, runner fileCandidateEmbeddingRunner, taskManager *queue.TaskManager) {
	if registry == nil || runner == nil {
		return
	}
	taskType := TypeDataLibraryFileCandidateEmbedding
	if taskManager != nil {
		taskType = taskManager.GetTaskTypeWithPrefix(taskType)
	}
	registry.Register(taskType, NewFileCandidateEmbeddingTaskHandler(runner))
}

func NewFileCandidateEmbeddingTaskHandler(runner fileCandidateEmbeddingRunner) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, task *asynq.Task) error {
		if runner == nil {
			return fmt.Errorf("data library file candidate embedding runner is not configured: %w", asynq.SkipRetry)
		}
		var payload FileCandidateEmbeddingPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return fmt.Errorf("unmarshal data library file candidate embedding task payload: %v: %w", err, asynq.SkipRetry)
		}
		assetID, err := uuid.Parse(payload.AssetID)
		if err != nil || assetID == uuid.Nil {
			return fmt.Errorf("invalid asset_id %q: %w", payload.AssetID, asynq.SkipRetry)
		}
		if payload.OrganizationID == "" || payload.DatasetID == "" {
			return fmt.Errorf("missing file candidate embedding payload scope: %w", asynq.SkipRetry)
		}
		var processingRequestID uuid.UUID
		if payload.ProcessingRequestID != "" {
			processingRequestID, err = uuid.Parse(payload.ProcessingRequestID)
			if err != nil || processingRequestID == uuid.Nil {
				return fmt.Errorf("invalid processing_request_id %q: %w", payload.ProcessingRequestID, asynq.SkipRetry)
			}
		}
		return runner.Run(ctx, datalibraryservice.KnowledgeBaseFileCandidateEmbeddingRequest{
			OrganizationID:      payload.OrganizationID,
			WorkspaceID:         payload.WorkspaceID,
			DatasetID:           payload.DatasetID,
			AssetID:             assetID,
			RequestedBy:         payload.RequestedBy,
			ProcessingRequestID: processingRequestID,
		})
	}
}
