package queue

import (
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/config"
)

// NewAsynqClient creates a new asynq client instance
// Client is used to enqueue tasks
func NewAsynqClient(cfg *config.Config) *asynq.Client {
	// Create redis connection option
	redisOpt := asynq.RedisClientOpt{
		Addr:     fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port),
		Password: cfg.Redis.Password,
		DB:       cfg.TaskQueue.RedisDB,
	}

	client := asynq.NewClient(redisOpt)
	return client
}

// NewAsynqServer creates a new asynq server instance
// Server is used to process tasks
func NewAsynqServer(cfg *config.Config) *asynq.Server {
	// Create redis connection option
	redisOpt := asynq.RedisClientOpt{
		Addr:     fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port),
		Password: cfg.Redis.Password,
		DB:       cfg.TaskQueue.RedisDB,
	}

	// Create server configuration
	server := asynq.NewServer(
		redisOpt,
		asynq.Config{
			Concurrency: cfg.TaskQueue.Concurrency,
			Queues: map[string]int{
				"chunking":  10,
				"graphflow": 8,
				"critical":  6,
				"default":   3,
				"low":       1,
			},
		},
	)

	return server
}
