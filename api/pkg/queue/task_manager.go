package queue

import (
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// TaskManager manages asynq tasks
type TaskManager struct {
	client *asynq.Client
	server *asynq.Server
	config *config.Config
}

// NewTaskManager creates a new task manager
func NewTaskManager(cfg *config.Config) (*TaskManager, error) {
	client := NewAsynqClient(cfg)
	server := NewAsynqServer(cfg)

	return &TaskManager{
		client: client,
		server: server,
		config: cfg,
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

// StartServer starts the asynq server with given mux
func (tm *TaskManager) StartServer(mux *asynq.ServeMux) error {
	logger.Info("Starting asynq server")
	return tm.server.Run(mux)
}

// StopServer stops the asynq server
func (tm *TaskManager) StopServer() {
	logger.Info("Stopping asynq server")
	tm.server.Shutdown()
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
