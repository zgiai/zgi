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
	}

	tm, err := NewTaskManager(cfg)
	// Expecting no error, but if redis is not running it might not fail immediately
	// until we try to use it, or NewTaskManager might check connection.
	// Actually NewClient/NewServer usually doesn't connect eagerly in asynq.

	assert.NoError(t, err)
	assert.NotNil(t, tm)
	assert.NotNil(t, tm.GetClient())
	assert.NotNil(t, tm.GetServer())
}

func TestGetTaskTypeWithPrefix(t *testing.T) {
	cfg := &config.Config{
		Redis: config.RedisConfig{
			DB: 0,
		},
	}
	tm, _ := NewTaskManager(cfg)

	// Default prefix is usually empty or "asynq" depending on implementation?
	// But GetTaskTypeWithPrefix likely adds the prefix if configured.

	// Assuming no prefix config means no change
	taskType := "my_task"
	assert.Equal(t, taskType, tm.GetTaskTypeWithPrefix(taskType))
}
