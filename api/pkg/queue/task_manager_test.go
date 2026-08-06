package queue

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zgiai/zgi/api/config"
)

func TestNewTaskManager(t *testing.T) {
	cfg := &config.Config{
		Redis: config.RedisConfig{
			Host:     "localhost",
			Port:     6379,
			Password: "",
			DB:       0,
		},
		TaskQueue: config.TaskQueueConfig{
			Concurrency:          8,
			GraphFlowConcurrency: 4,
		},
	}

	tm, err := NewTaskManager(cfg)
	// Expecting no error, but if redis is not running it might not fail immediately
	// until we try to use it, or NewTaskManager might check connection.
	// Actually NewClient/NewServer usually doesn't connect eagerly in asynq.

	assert.NoError(t, err)
	assert.NotNil(t, tm)
	assert.NotNil(t, tm.GetClient())
	assert.NotNil(t, tm.GetServer())
	assert.NotNil(t, tm.GetGraphFlowServer())
}

func TestWorkerQueuesAreIsolated(t *testing.T) {
	cfg := &config.Config{TaskQueue: config.TaskQueueConfig{
		Concurrency:          8,
		GraphFlowConcurrency: 4,
	}}

	mainConfig := mainWorkerConfig(cfg)
	assert.Equal(t, 8, mainConfig.Concurrency)
	assert.NotContains(t, mainConfig.Queues, "graphflow")
	assert.Contains(t, mainConfig.Queues, "default")

	graphConfig := graphFlowWorkerConfig(cfg)
	assert.Equal(t, 4, graphConfig.Concurrency)
	assert.Equal(t, map[string]int{"graphflow": 1}, graphConfig.Queues)

	cfg.TaskQueue.GraphFlowConcurrency = 99
	assert.Equal(t, 4, graphFlowWorkerConfig(cfg).Concurrency)
}

func TestGetTaskTypeWithPrefix(t *testing.T) {
	cfg := &config.Config{
		Redis: config.RedisConfig{
			DB: 0,
		},
		TaskQueue: config.TaskQueueConfig{
			Concurrency:          8,
			GraphFlowConcurrency: 4,
		},
	}
	tm, _ := NewTaskManager(cfg)

	// Default prefix is usually empty or "asynq" depending on implementation?
	// But GetTaskTypeWithPrefix likely adds the prefix if configured.

	// Assuming no prefix config means no change
	taskType := "my_task"
	assert.Equal(t, taskType, tm.GetTaskTypeWithPrefix(taskType))
}
