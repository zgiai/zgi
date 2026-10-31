package queue

import (
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/config"
)

const maxGraphFlowWorkerConcurrency = 4

// NewAsynqClient creates a new asynq client instance
// Client is used to enqueue tasks
func NewAsynqClient(cfg *config.Config) *asynq.Client {
	client := asynq.NewClient(taskQueueRedisOpt(cfg))
	return client
}

// NewAsynqServer creates a new asynq server instance
// Server is used to process tasks
func NewAsynqServer(cfg *config.Config) *asynq.Server {
	return asynq.NewServer(taskQueueRedisOpt(cfg), mainWorkerConfig(cfg))
}

// NewGraphFlowAsynqServer creates the dedicated server that exclusively
// consumes the graphflow queue. Its concurrency is capped during config load.
func NewGraphFlowAsynqServer(cfg *config.Config) *asynq.Server {
	return asynq.NewServer(taskQueueRedisOpt(cfg), graphFlowWorkerConfig(cfg))
}

func taskQueueRedisOpt(cfg *config.Config) asynq.RedisClientOpt {
	return asynq.RedisClientOpt{
		Addr:     fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port),
		Password: cfg.Redis.Password,
		DB:       cfg.TaskQueue.RedisDB,
	}
}

// validateWorkerConcurrency rejects a concurrency Asynq would silently
// replace with the CPU count. Config load already enforces it; this keeps a
// hand-built config from starting workers the operator did not ask for.
func validateWorkerConcurrency(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("task queue config is required")
	}
	if cfg.TaskQueue.Concurrency < 1 {
		return fmt.Errorf("task queue concurrency must be at least 1, got %d", cfg.TaskQueue.Concurrency)
	}
	return nil
}

func mainWorkerConfig(cfg *config.Config) asynq.Config {
	return asynq.Config{
		Concurrency: cfg.TaskQueue.Concurrency,
		Queues: map[string]int{
			"chunking": 10,
			"critical": 6,
			"default":  3,
			"low":      1,
		},
		ErrorHandler: NewTaskFailureHandler("main"),
	}
}

func graphFlowWorkerConfig(cfg *config.Config) asynq.Config {
	concurrency := graphFlowWorkerConcurrency(cfg)
	return asynq.Config{
		Concurrency: concurrency,
		Queues: map[string]int{
			"graphflow": 1,
		},
		ErrorHandler: NewTaskFailureHandler("graphflow"),
	}
}

func graphFlowWorkerConcurrency(cfg *config.Config) int {
	concurrency := cfg.TaskQueue.GraphFlowConcurrency
	if concurrency < 1 || concurrency > maxGraphFlowWorkerConcurrency {
		concurrency = maxGraphFlowWorkerConcurrency
	}
	return concurrency
}
