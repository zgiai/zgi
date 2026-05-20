package container_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/container"
	"github.com/zgiai/zgi/api/pkg/logger"
)

func TestTaskHandlerRegistrarAddsTaskTypeToContextLogs(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "app.log")
	l, err := logger.New(&config.Config{
		Log: config.LogConfig{
			Level:      "debug",
			Filename:   logPath,
			MaxSize:    10,
			MaxAge:     15,
			MaxBackups: 3,
			Compress:   false,
		},
	})
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	logger.SetLogger(l)
	t.Cleanup(logger.Sync)

	registrar := container.NewTaskHandlerRegistrar()
	registrar.Register("graphflow:extract", func(ctx context.Context, task *asynq.Task) error {
		logger.ErrorContext(ctx, "async task failed", errors.New("boom"))
		return nil
	})

	mux := asynq.NewServeMux()
	registrar.RegisterAll(mux)

	if err := mux.ProcessTask(context.Background(), asynq.NewTask("graphflow:extract", nil)); err != nil {
		t.Fatalf("mux.ProcessTask() error = %v", err)
	}
	logger.Sync()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", logPath, err)
	}

	text := string(content)
	if !strings.Contains(text, `"msg":"async task failed"`) {
		t.Fatalf("async task log not found\n%s", text)
	}
	if !strings.Contains(text, `"task_type":"graphflow:extract"`) {
		t.Fatalf("task_type not found in async task log\n%s", text)
	}
}
