package worker

import (
	"encoding/json"

	"github.com/hibiken/asynq"
	"github.com/zgiai/ginext/pkg/queue"
)

// Task type constants for GraphFlow
const (
	TypeGraphFlowExtraction = "graphflow:extraction"
	TypeGraphFlowAlignment  = "graphflow:alignment"
	TypeGraphFlowSync       = "graphflow:sync"
	TypeGraphFlowVectorSync = "graphflow:vector_sync"
	TypeGraphFlowCleanup    = "graphflow:cleanup"
)

// Concurrency configuration
const (
	ExtractionConcurrency = 10 // Number of segments processed concurrently within a single extraction task
)

// GraphFlowTaskPayload is the common payload for all GraphFlow tasks
type GraphFlowTaskPayload struct {
	TaskID           string `json:"task_id"`
	ExpectedSegments int    `json:"expected_segments,omitempty"` // For race condition check
}

// GraphFlowCleanupPayload is the payload for cleanup tasks
type GraphFlowCleanupPayload struct {
	TaskID     string `json:"task_id,omitempty"`
	DocumentID string `json:"document_id"`
	KBID       string `json:"kb_id,omitempty"`
}

// NewGraphFlowTask creates a new asynq task for GraphFlow operations
func NewGraphFlowTask(taskType string, taskID string, taskManager *queue.TaskManager, opts ...int) (*asynq.Task, error) {
	expectedSegments := 0
	if len(opts) > 0 {
		expectedSegments = opts[0]
	}

	payload := GraphFlowTaskPayload{
		TaskID:           taskID,
		ExpectedSegments: expectedSegments,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	if taskManager != nil {
		taskType = taskManager.GetTaskTypeWithPrefix(taskType)
	}

	return asynq.NewTask(taskType, payloadBytes, asynq.TaskID(taskID), asynq.MaxRetry(3)), nil
}

// NewGraphFlowCleanupTask creates a new asynq task for GraphFlow cleanup operations
func NewGraphFlowCleanupTask(taskID, documentID, kbID string, taskManager *queue.TaskManager) (*asynq.Task, error) {
	payload := GraphFlowCleanupPayload{
		TaskID:     taskID,
		DocumentID: documentID,
		KBID:       kbID,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	taskType := TypeGraphFlowCleanup
	if taskManager != nil {
		taskType = taskManager.GetTaskTypeWithPrefix(taskType)
	}

	return asynq.NewTask(taskType, payloadBytes, asynq.MaxRetry(3)), nil
}
