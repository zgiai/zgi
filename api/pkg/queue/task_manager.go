package queue

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// TaskManager manages asynq tasks
type TaskManager struct {
	client          *asynq.Client
	server          *asynq.Server
	graphFlowServer *asynq.Server
	config          *config.Config
	stopping        atomic.Bool
	stopOnce        sync.Once
}

// NewTaskManager creates a new task manager
func NewTaskManager(cfg *config.Config) (*TaskManager, error) {
	client := NewAsynqClient(cfg)
	server := NewAsynqServer(cfg)
	graphFlowServer := NewGraphFlowAsynqServer(cfg)

	return &TaskManager{
		client:          client,
		server:          server,
		graphFlowServer: graphFlowServer,
		config:          cfg,
	}, nil
}

// EnqueueTask enqueues a task with given type and payload
func (tm *TaskManager) EnqueueTask(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	if tm.config != nil && tm.config.TaskQueue.Retention > 0 {
		opts = append(opts, asynq.Retention(tm.config.TaskQueue.Retention))
	}

	info, err := tm.client.Enqueue(task, opts...)
	if err != nil {
		logger.Critical("failed to enqueue task", "task_type", task.Type(), err)
		return nil, fmt.Errorf("failed to enqueue task: %w", err)
	}

	logger.Info("Task enqueued", map[string]interface{}{
		"task_id":   info.ID,
		"task_type": info.Type,
		"queue":     info.Queue,
	})

	return info, nil
}

// GetClient returns the asynq client
func (tm *TaskManager) GetClient() *asynq.Client {
	return tm.client
}

// GetServer returns the asynq server
func (tm *TaskManager) GetServer() *asynq.Server {
	return tm.server
}

// GetGraphFlowServer returns the dedicated GraphFlow worker server.
func (tm *TaskManager) GetGraphFlowServer() *asynq.Server {
	return tm.graphFlowServer
}

// StartServer starts the asynq server with given mux
func (tm *TaskManager) StartServer(mux *asynq.ServeMux) error {
	tm.stopping.Store(false)
	tm.stopOnce = sync.Once{}
	logger.Info("Starting asynq worker servers", map[string]interface{}{
		"main_concurrency":      tm.config.TaskQueue.Concurrency,
		"graphflow_concurrency": graphFlowWorkerConcurrency(tm.config),
	})
	type workerResult struct {
		name string
		err  error
	}
	resultCh := make(chan workerResult, 2)
	go func() {
		resultCh <- workerResult{name: "main", err: tm.server.Run(mux)}
	}()
	go func() {
		resultCh <- workerResult{name: "graphflow", err: tm.graphFlowServer.Run(mux)}
	}()

	first := <-resultCh
	intentionalStop := tm.stopping.Load()
	// The two servers form one worker subsystem. If either exits, stop its
	// sibling as well instead of leaving the API with a silently missing queue.
	tm.StopServer()
	second := <-resultCh
	if intentionalStop {
		return nil
	}
	return errors.Join(
		workerExitError(first.name, first.err),
		workerExitError(second.name, second.err),
	)
}

func workerExitError(name string, err error) error {
	if err == nil {
		return fmt.Errorf("%s task worker stopped unexpectedly", name)
	}
	return fmt.Errorf("%s task worker stopped: %w", name, err)
}

// StopServer stops the asynq server
func (tm *TaskManager) StopServer() {
	tm.stopping.Store(true)
	tm.stopOnce.Do(func() {
		logger.Info("Stopping asynq worker servers")
		tm.server.Shutdown()
		tm.graphFlowServer.Shutdown()
	})
}

// Close closes the task manager connections
func (tm *TaskManager) Close() error {
	if tm.client != nil {
		if err := tm.client.Close(); err != nil {
			return fmt.Errorf("failed to close asynq client: %w", err)
		}
	}
	return nil
}

// getTaskTypeWithPrefix returns the task type with environment prefix
func (tm *TaskManager) getTaskTypeWithPrefix(taskType string) string {
	if tm.config.TaskQueue.EnvPrefix != "" {
		return fmt.Sprintf("%s:%s", tm.config.TaskQueue.EnvPrefix, taskType)
	}
	return taskType
}

// GetTaskTypeWithPrefix returns the task type with environment prefix (public method)
func (tm *TaskManager) GetTaskTypeWithPrefix(taskType string) string {
	return tm.getTaskTypeWithPrefix(taskType)
}
