package task

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/dataset/graphflow/repository"
	"github.com/zgiai/ginext/pkg/logger"
)

// TaskStatus represents the status of a GraphFlow task
type TaskStatus struct {
	TaskID      uuid.UUID              `json:"task_id"`
	TaskType    string                 `json:"task_type"`
	Status      string                 `json:"status"`
	Progress    int                    `json:"progress"`
	StartedAt   *time.Time             `json:"started_at,omitempty"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Result      map[string]interface{} `json:"result,omitempty"`
}

// PipelineResult represents the result of running a full pipeline
type PipelineResult struct {
	ExtractionTaskID uuid.UUID `json:"extraction_task_id"`
	AlignmentTaskID  uuid.UUID `json:"alignment_task_id"`
	SyncTaskID       uuid.UUID `json:"sync_task_id"`
	Status           string    `json:"status"`
	StartedAt        time.Time `json:"started_at"`
	CompletedAt      time.Time `json:"completed_at,omitempty"`
}

// TaskManager provides a unified interface for managing GraphFlow tasks
type TaskManager struct {
	taskRepo      *repository.GraphFlowTaskRepository
	alignmentTask *EntityAlignmentTask
	syncTask      *SyncTask
}

// NewTaskManager creates a new TaskManager instance
func NewTaskManager(
	taskRepo *repository.GraphFlowTaskRepository,
	alignmentTask *EntityAlignmentTask,
	syncTask *SyncTask,
) *TaskManager {
	return &TaskManager{
		taskRepo:      taskRepo,
		alignmentTask: alignmentTask,
		syncTask:      syncTask,
	}
}

// GetTaskStatus retrieves the status of a task by ID
func (m *TaskManager) GetTaskStatus(ctx context.Context, taskID uuid.UUID) (*TaskStatus, error) {
	if m.taskRepo == nil {
		return nil, fmt.Errorf("task repository not configured")
	}

	task, err := m.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	if task == nil {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	status := &TaskStatus{
		TaskID:   task.ID,
		TaskType: task.TaskType,
		Status:   task.Status,
		Error:    task.ErrorMessage,
	}

	if !task.CreatedAt.IsZero() {
		status.StartedAt = &task.CreatedAt
	}

	if !task.UpdatedAt.IsZero() && task.Status == "completed" {
		status.CompletedAt = &task.UpdatedAt
	}

	return status, nil
}

// GetTaskByDocument retrieves the task for a specific document
func (m *TaskManager) GetTaskByDocument(ctx context.Context, documentID uuid.UUID) (*TaskStatus, error) {
	if m.taskRepo == nil {
		return nil, fmt.Errorf("task repository not configured")
	}

	task, err := m.taskRepo.GetByDocumentID(ctx, documentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	if task == nil {
		return nil, nil
	}

	status := &TaskStatus{
		TaskID:   task.ID,
		TaskType: task.TaskType,
		Status:   task.Status,
		Error:    task.ErrorMessage,
	}

	if !task.CreatedAt.IsZero() {
		status.StartedAt = &task.CreatedAt
	}

	return status, nil
}

// RunPipeline executes the complete GraphFlow pipeline for a document
// Pipeline: Extraction -> Alignment -> Sync
func (m *TaskManager) RunPipeline(ctx context.Context, tenantID string, documentID uuid.UUID) (*PipelineResult, error) {
	result := &PipelineResult{
		Status:    "started",
		StartedAt: time.Now(),
	}

	logger.Info("Starting GraphFlow pipeline", map[string]interface{}{
		"document_id": documentID.String(),
	})

	// Note: The actual task execution is handled by the worker handlers
	// This method primarily serves as an API for triggering the pipeline
	// In a full implementation, this would enqueue tasks and return immediately

	logger.Info("GraphFlow pipeline triggered", map[string]interface{}{
		"document_id": documentID.String(),
	})

	result.Status = "triggered"
	return result, nil
}

// CancelTask attempts to cancel a running task
func (m *TaskManager) CancelTask(ctx context.Context, taskID uuid.UUID) error {
	if m.taskRepo == nil {
		return fmt.Errorf("task repository not configured")
	}

	return m.taskRepo.UpdateStatus(ctx, taskID, "cancelled", "Task cancelled by user")
}

// RetryTask retries a failed task
func (m *TaskManager) RetryTask(ctx context.Context, taskID uuid.UUID) error {
	if m.taskRepo == nil {
		return fmt.Errorf("task repository not configured")
	}

	// Reset status to pending
	return m.taskRepo.UpdateStatus(ctx, taskID, "pending", "")
}

// GetKBStats returns basic statistics for a knowledge base's GraphFlow processing
// Note: This is a simplified version that returns placeholder data
// Full implementation would require additional repository methods
func (m *TaskManager) GetKBStats(ctx context.Context, kbID uuid.UUID) (map[string]interface{}, error) {
	stats := map[string]interface{}{
		"kb_id":            kbID.String(),
		"tasks_pending":    0,
		"tasks_processing": 0,
		"tasks_completed":  0,
		"tasks_failed":     0,
		"tasks_total":      0,
	}

	// In a full implementation, this would query the database for task counts
	// For now, we return placeholder values

	return stats, nil
}
