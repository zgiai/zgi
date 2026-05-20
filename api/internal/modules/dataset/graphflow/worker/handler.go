package worker

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/queue"
	"github.com/zgiai/zgi/api/pkg/ratelimit"
	pkgredis "github.com/zgiai/zgi/api/pkg/redis"
)

// TaskHandlerRegistry is an interface for registering asynq task handlers
// This interface is used to avoid import cycles with the container package
type TaskHandlerRegistry interface {
	Register(taskType string, handler func(context.Context, *asynq.Task) error) bool
}

// RegisterGraphFlowHandlers registers all GraphFlow task handlers with the provided task handler registry
func RegisterGraphFlowHandlers(registry TaskHandlerRegistry, svc *graphflow.Service, taskManager *queue.TaskManager) {
	// Initialize KB Rate Limiter
	kbLimiter := ratelimit.NewKBLimiter(pkgredis.GetClient())

	// Get task types with environment prefix
	extractionType := getTaskTypeWithPrefix(taskManager, TypeGraphFlowExtraction)
	alignmentType := getTaskTypeWithPrefix(taskManager, TypeGraphFlowAlignment)
	syncType := getTaskTypeWithPrefix(taskManager, TypeGraphFlowSync)
	vectorSyncType := getTaskTypeWithPrefix(taskManager, TypeGraphFlowVectorSync)
	cleanupType := getTaskTypeWithPrefix(taskManager, TypeGraphFlowCleanup)

	// Register extraction handler (with in-task concurrency)
	if isNew := registry.Register(extractionType, NewExtractionHandler(svc, taskManager, kbLimiter)); isNew {
		logger.Info("Registered GraphFlow extraction handler", map[string]interface{}{
			"task_type": extractionType,
		})
	} else {
		logger.Warn("GraphFlow extraction handler was replaced", map[string]interface{}{
			"task_type": extractionType,
		})
	}

	// Register alignment handler (with KB-level locking)
	if isNew := registry.Register(alignmentType, NewAlignmentHandler(svc, taskManager, kbLimiter)); isNew {
		logger.Info("Registered GraphFlow alignment handler", map[string]interface{}{
			"task_type": alignmentType,
		})
	} else {
		logger.Warn("GraphFlow alignment handler was replaced", map[string]interface{}{
			"task_type": alignmentType,
		})
	}

	// Register sync handler (Neo4j)
	if isNew := registry.Register(syncType, NewSyncHandler(svc, taskManager, kbLimiter)); isNew {
		logger.Info("Registered GraphFlow sync handler", map[string]interface{}{
			"task_type": syncType,
		})
	} else {
		logger.Warn("GraphFlow sync handler was replaced", map[string]interface{}{
			"task_type": syncType,
		})
	}

	// Register vector sync handler (Entity embeddings)
	if isNew := registry.Register(vectorSyncType, NewVectorSyncHandler(svc, taskManager, kbLimiter)); isNew {
		logger.Info("Registered GraphFlow vector sync handler", map[string]interface{}{
			"task_type": vectorSyncType,
		})
	} else {
		logger.Warn("GraphFlow vector sync handler was replaced", map[string]interface{}{
			"task_type": vectorSyncType,
		})
	}

	// Register cleanup handler
	if isNew := registry.Register(cleanupType, NewCleanupHandler(svc, taskManager)); isNew {
		logger.Info("Registered GraphFlow cleanup handler", map[string]interface{}{
			"task_type": cleanupType,
		})
	} else {
		logger.Warn("GraphFlow cleanup handler was replaced", map[string]interface{}{
			"task_type": cleanupType,
		})
	}

	logger.Info("GraphFlow handlers registered", map[string]interface{}{
		"total_handlers": 5,
	})
}

// Helper to get task type with prefix
func getTaskTypeWithPrefix(taskManager *queue.TaskManager, taskType string) string {
	if taskManager != nil {
		return taskManager.GetTaskTypeWithPrefix(taskType)
	}
	return taskType
}

// CreateGraphFlowExtractionTask creates and returns an asynq task for GraphFlow extraction
func CreateGraphFlowExtractionTask(taskID string, taskManager *queue.TaskManager, opts ...int) (*asynq.Task, error) {
	return NewGraphFlowTask(TypeGraphFlowExtraction, taskID, taskManager, opts...)
}

// CreateGraphFlowCleanupTask creates and returns an asynq task for GraphFlow cleanup
func CreateGraphFlowCleanupTask(taskID, documentID, kbID string, taskManager *queue.TaskManager) (*asynq.Task, error) {
	return NewGraphFlowCleanupTask(taskID, documentID, kbID, taskManager)
}
