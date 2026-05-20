package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/dto"
	"github.com/zgiai/ginext/internal/modules/dataset/model"
	"github.com/zgiai/ginext/internal/modules/dataset/repository"
)

// TaskContext holds both the context and its cancel function for a task
type TaskContext struct {
	Ctx    context.Context
	Cancel context.CancelFunc
}

// BatchHitTestingTaskManager manages batch hit testing tasks
type BatchHitTestingTaskManager struct {
	tasks        sync.Map // map[string]*BatchHitTestingTask
	taskRepo     repository.BatchHitTestingTaskRepository
	taskContexts sync.Map // map[string]*TaskContext
}

// BatchHitTestingTask represents a batch hit testing task
type BatchHitTestingTask struct {
	TaskID         string
	DatasetID      string
	AccountID      string
	OrganizationID string
	Status         string // pending, processing, completed, failed
	Progress       int
	Total          int
	Completed      int
	Failed         int
	CreatedAt      time.Time
	StartedAt      *time.Time
	FinishedAt     *time.Time
	Queries        []*QueryTask
}

// QueryTask represents a single query task
type QueryTask struct {
	Query      string
	Status     string // pending, processing, completed, failed
	Result     *dto.HitTestingResponse
	Error      *string
	StartedAt  *time.Time
	FinishedAt *time.Time
}

// NewBatchHitTestingTaskManager creates a new batch hit testing task manager
func NewBatchHitTestingTaskManager(taskRepo repository.BatchHitTestingTaskRepository) *BatchHitTestingTaskManager {
	return &BatchHitTestingTaskManager{
		tasks:    sync.Map{},
		taskRepo: taskRepo,
	}
}

// CreateTask creates a new batch hit testing task
func (m *BatchHitTestingTaskManager) CreateTask(datasetID, accountID, organizationID string, req *dto.AsyncBatchHitTestingRequest) string {
	taskID := uuid.New().String()

	// Create query tasks
	var queryTasks []*QueryTask
	for _, query := range req.Queries {
		queryTasks = append(queryTasks, &QueryTask{
			Query:  query,
			Status: "pending",
		})
	}

	task := &BatchHitTestingTask{
		TaskID:         taskID,
		DatasetID:      datasetID,
		AccountID:      accountID,
		OrganizationID: organizationID,
		Status:         "pending",
		Total:          len(req.Queries),
		CreatedAt:      time.Now(),
		Queries:        queryTasks,
	}

	// Store in memory for quick access
	m.tasks.Store(taskID, task)

	// TODO: Store in database
	// Convert to database model
	dbTask := &model.BatchHitTestingTask{
		TaskID:         taskID,
		DatasetID:      datasetID,
		AccountID:      accountID,
		OrganizationID: organizationID,
		Status:         "pending",
		Total:          len(req.Queries),
		CreatedAt:      time.Now(),
		Queries:        make(model.JSONQueryTasks, len(queryTasks)),
	}

	// Copy query tasks to database model
	for i, qt := range queryTasks {
		dbTask.Queries[i] = model.QueryTask{
			Query:  qt.Query,
			Status: qt.Status,
		}
	}

	// Save to database
	ctx := context.Background()
	// TODO: Handle error properly
	_ = m.taskRepo.Create(ctx, dbTask)

	return taskID
}

// GetTask retrieves a batch hit testing task by ID
func (m *BatchHitTestingTaskManager) GetTask(taskID string) (*BatchHitTestingTask, bool) {
	// First check in memory
	task, ok := m.tasks.Load(taskID)
	if ok {
		return task.(*BatchHitTestingTask), true
	}

	// If not in memory, load from database
	ctx := context.Background()
	dbTask, err := m.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		return nil, false
	}

	// Convert database model to memory model
	task = m.convertDBTaskToMemoryTask(dbTask)

	// Store in memory for future access
	m.tasks.Store(taskID, task)

	return task.(*BatchHitTestingTask), true
}

// UpdateTaskStatus updates the status of a batch hit testing task
func (m *BatchHitTestingTaskManager) UpdateTaskStatus(taskID, status string) {
	task, ok := m.GetTask(taskID)
	if !ok {
		return
	}

	task.Status = status
	if status == "processing" {
		now := time.Now()
		task.StartedAt = &now
	}

	if status == "completed" || status == "failed" {
		now := time.Now()
		task.FinishedAt = &now
	}

	// Update in memory
	m.tasks.Store(taskID, task)

	// Update in database
	ctx := context.Background()
	// TODO: Handle error properly
	_ = m.taskRepo.UpdateTaskStatus(ctx, taskID, status, task.StartedAt, task.FinishedAt)
}

// UpdateQueryTaskStatus updates the status of a query task
func (m *BatchHitTestingTaskManager) UpdateQueryTaskStatus(taskID string, queryIndex int, status string, result *dto.HitTestingResponse, err *string) {
	task, ok := m.GetTask(taskID)
	if !ok {
		return
	}

	if queryIndex >= len(task.Queries) {
		return
	}

	queryTask := task.Queries[queryIndex]
	queryTask.Status = status

	now := time.Now()
	if status == "processing" {
		queryTask.StartedAt = &now
	}

	if status == "completed" || status == "failed" {
		queryTask.FinishedAt = &now
		queryTask.Result = result
		queryTask.Error = err
	}

	// Update counters
	if status == "completed" {
		task.Completed++
	} else if status == "failed" {
		task.Failed++
	}

	// Update progress
	task.Progress = task.Completed + task.Failed

	// Update overall task status
	if task.Progress >= task.Total {
		if task.Failed == 0 {
			task.Status = "completed"
		} else if task.Completed > 0 {
			task.Status = "completed" // Partially completed
		} else {
			task.Status = "failed"
		}
		now := time.Now()
		task.FinishedAt = &now
	}

	// Update in memory
	m.tasks.Store(taskID, task)

	// Update in database
	ctx := context.Background()
	dbQueryTask := &model.QueryTask{
		Query:      queryTask.Query,
		Status:     queryTask.Status,
		Result:     queryTask.Result,
		Error:      queryTask.Error,
		StartedAt:  queryTask.StartedAt,
		FinishedAt: queryTask.FinishedAt,
	}
	// TODO: Handle error properly
	_ = m.taskRepo.UpdateQueryTaskStatus(ctx, taskID, queryIndex, status, dbQueryTask)
}

// StartTask starts processing a batch hit testing task
func (m *BatchHitTestingTaskManager) StartTask(taskID string) {
	task, ok := m.GetTask(taskID)
	if !ok {
		return
	}

	task.Status = "processing"
	now := time.Now()
	task.StartedAt = &now
	m.tasks.Store(taskID, task)

	// Update in database
	ctx := context.Background()
	// TODO: Handle error properly
	_ = m.taskRepo.UpdateTaskStatus(ctx, taskID, "processing", &now, nil)
}

// StopTask stops a batch hit testing task
func (m *BatchHitTestingTaskManager) StopTask(taskID string) error {
	task, ok := m.GetTask(taskID)
	if !ok {
		return fmt.Errorf("task not found")
	}

	// Update task status to failed (stopped)
	task.Status = "failed"
	now := time.Now()
	task.FinishedAt = &now

	// Update all pending/processing queries to failed
	for i, queryTask := range task.Queries {
		if queryTask.Status == "pending" || queryTask.Status == "processing" {
			queryTask.Status = "failed"
			queryTask.FinishedAt = &now

			// Add error message for stopped queries
			errorMsg := "Task was stopped by user"
			queryTask.Error = &errorMsg

			// Update database for each query
			ctx := context.Background()
			dbQueryTask := &model.QueryTask{
				Query:      queryTask.Query,
				Status:     queryTask.Status,
				Result:     queryTask.Result,
				Error:      queryTask.Error,
				StartedAt:  queryTask.StartedAt,
				FinishedAt: queryTask.FinishedAt,
			}
			// TODO: Handle error properly
			_ = m.taskRepo.UpdateQueryTaskStatus(ctx, taskID, i, "failed", dbQueryTask)
		}
	}

	// Update task progress
	task.Failed = task.Total - task.Completed
	task.Progress = task.Total

	// Update in memory
	m.tasks.Store(taskID, task)

	// Cancel the task context if it exists
	m.CancelTask(taskID)

	// Clean up context
	m.taskContexts.Delete(taskID)

	// Update in database
	ctx := context.Background()
	// TODO: Handle error properly
	_ = m.taskRepo.UpdateTaskStatus(ctx, taskID, "failed", task.StartedAt, &now)

	return nil
}

// CreateTaskContext creates a context for a task that can be cancelled
func (m *BatchHitTestingTaskManager) CreateTaskContext(taskID string) context.Context {
	// Check if context already exists
	if taskCtx, ok := m.taskContexts.Load(taskID); ok {
		return taskCtx.(*TaskContext).Ctx
	}

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Store the context and cancel function together
	taskCtx := &TaskContext{
		Ctx:    ctx,
		Cancel: cancel,
	}
	m.taskContexts.Store(taskID, taskCtx)

	return ctx
}

// IsTaskCancelled checks if a task has been cancelled
func (m *BatchHitTestingTaskManager) IsTaskCancelled(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// CancelTask cancels a task
func (m *BatchHitTestingTaskManager) CancelTask(taskID string) {
	// Get the task context
	if taskCtx, ok := m.taskContexts.Load(taskID); ok {
		taskCtx.(*TaskContext).Cancel()
	}
}

// ToDTO converts a BatchHitTestingTask to DTO
func (t *BatchHitTestingTask) ToDTO() *dto.BatchHitTestingTaskStatus {
	var startedAt *int64
	if t.StartedAt != nil {
		unix := t.StartedAt.Unix()
		startedAt = &unix
	}

	var finishedAt *int64
	if t.FinishedAt != nil {
		unix := t.FinishedAt.Unix()
		finishedAt = &unix
	}

	dtoTask := &dto.BatchHitTestingTaskStatus{
		TaskID:     t.TaskID,
		Status:     t.Status,
		Progress:   t.Progress,
		Total:      t.Total,
		Completed:  t.Completed,
		Failed:     t.Failed,
		CreatedAt:  t.CreatedAt.Unix(),
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
	}

	// Add query results for all states, not just completed or failed
	// This allows viewing individual query progress even while task is processing
	var results []dto.QueryResult
	for _, queryTask := range t.Queries {
		var qStartedAt *int64
		if queryTask.StartedAt != nil {
			unix := queryTask.StartedAt.Unix()
			qStartedAt = &unix
		}

		var qFinishedAt *int64
		if queryTask.FinishedAt != nil {
			unix := queryTask.FinishedAt.Unix()
			qFinishedAt = &unix
		}

		result := dto.QueryResult{
			Query:      queryTask.Query,
			Status:     queryTask.Status,
			Result:     queryTask.Result,
			Error:      queryTask.Error,
			StartedAt:  qStartedAt,
			FinishedAt: qFinishedAt,
		}
		results = append(results, result)
	}
	dtoTask.Results = results

	return dtoTask
}

// GetTaskContext returns the context for a task
func (m *BatchHitTestingTaskManager) GetTaskContext(taskID string) context.Context {
	// Create a new context if it doesn't exist
	return m.CreateTaskContext(taskID)
}

// UpdateProgress updates the progress of a batch hit testing task
func (m *BatchHitTestingTaskManager) UpdateProgress(taskID string) {
	task, ok := m.GetTask(taskID)
	if !ok {
		return
	}

	// Calculate progress as percentage
	if task.Total > 0 {
		task.Progress = (task.Completed + task.Failed) * 100 / task.Total
	}

	// Update in memory
	m.tasks.Store(taskID, task)

	// Update in database
	ctx := context.Background()
	// TODO: Handle error properly
	_ = m.taskRepo.UpdateTaskStatus(ctx, taskID, task.Status, task.StartedAt, task.FinishedAt)
}

// GetTaskReport generates a report for a completed batch hit testing task
func (m *BatchHitTestingTaskManager) GetTaskReport(taskID string) (*dto.BatchHitTestingTaskReport, error) {
	task, ok := m.GetTask(taskID)
	if !ok {
		return nil, fmt.Errorf("task not found")
	}

	// Check if task is completed or failed
	if task.Status != "completed" && task.Status != "failed" {
		switch task.Status {
		case "pending":
			return nil, fmt.Errorf("task is pending, please start the task first")
		case "processing":
			return nil, fmt.Errorf("task is still processing, please wait for completion")
		default:
			return nil, fmt.Errorf("task is in an invalid state: %s", task.Status)
		}
	}

	report := &dto.BatchHitTestingTaskReport{
		TaskID:       task.TaskID,
		TotalQueries: task.Total,
	}

	// Initialize metrics counters
	var totalSuccessRate float64
	var totalResponseTime float64
	var pregeneratedMatches int
	var completedQueries int

	for _, queryTask := range task.Queries {
		// Only consider completed queries for metrics calculation
		if queryTask.Status == "completed" && queryTask.Result != nil {
			completedQueries++

			// Count successful retrievals (queries with at least one record)
			if len(queryTask.Result.Records) > 0 {
				totalSuccessRate += 1.0
			}

			// Sum response times
			totalResponseTime += queryTask.Result.ElapsedTime

			// Count queries with pregenerated question matches
			for _, record := range queryTask.Result.Records {
				if record.MatchType == dto.MatchTypeQuestion {
					pregeneratedMatches++
					break // Count only once per query
				}
			}
		}
	}

	// Calculate retrieval success rate
	if completedQueries > 0 {
		report.RetrievalSuccessRate = totalSuccessRate / float64(completedQueries)
	}

	// Calculate average response time
	if completedQueries > 0 {
		report.AverageResponseTime = totalResponseTime / float64(completedQueries)
	}

	// Calculate question match rate
	if completedQueries > 0 {
		report.QuestionMatchRate = float64(pregeneratedMatches) / float64(completedQueries)
	}

	return report, nil
}

// convertDBTaskToMemoryTask converts a database task model to memory task model
func (m *BatchHitTestingTaskManager) convertDBTaskToMemoryTask(dbTask *model.BatchHitTestingTask) *BatchHitTestingTask {
	task := &BatchHitTestingTask{
		TaskID:         dbTask.TaskID,
		DatasetID:      dbTask.DatasetID,
		AccountID:      dbTask.AccountID,
		OrganizationID: dbTask.OrganizationID,
		Status:         dbTask.Status,
		Progress:       dbTask.Progress,
		Total:          dbTask.Total,
		Completed:      dbTask.Completed,
		Failed:         dbTask.Failed,
		CreatedAt:      dbTask.CreatedAt,
		Queries:        make([]*QueryTask, len(dbTask.Queries)),
	}

	// Convert timestamps
	if !dbTask.StartedAt.IsZero() {
		task.StartedAt = &dbTask.StartedAt
	}

	if !dbTask.FinishedAt.IsZero() {
		task.FinishedAt = &dbTask.FinishedAt
	}

	// Convert query tasks
	for i, dbQueryTask := range dbTask.Queries {
		queryTask := &QueryTask{
			Query:  dbQueryTask.Query,
			Status: dbQueryTask.Status,
			Result: dbQueryTask.Result,
			Error:  dbQueryTask.Error,
		}

		if dbQueryTask.StartedAt != nil && !dbQueryTask.StartedAt.IsZero() {
			queryTask.StartedAt = dbQueryTask.StartedAt
		}

		if dbQueryTask.FinishedAt != nil && !dbQueryTask.FinishedAt.IsZero() {
			queryTask.FinishedAt = dbQueryTask.FinishedAt
		}

		task.Queries[i] = queryTask
	}

	return task
}
