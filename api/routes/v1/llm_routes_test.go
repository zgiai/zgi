package v1

import (
	"testing"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/llm"
	"github.com/zgiai/zgi/api/internal/modules/llm/credential/upstreamstate"
	pkgscheduler "github.com/zgiai/zgi/api/pkg/scheduler"
)

func TestRegisterLLMUpstreamPollingIsAlwaysOn(t *testing.T) {
	scheduler, err := pkgscheduler.NewScheduler(&config.Config{
		Redis:     config.RedisConfig{Host: "127.0.0.1", Port: 6379},
		TaskQueue: config.TaskQueueConfig{Concurrency: 1},
	})
	if err != nil {
		t.Fatalf("NewScheduler() error = %v", err)
	}

	registerLLMUpstreamPolling(scheduler, &llm.LLMModule{UpstreamStateSvc: &upstreamstate.Service{}})

	tasks := scheduler.GetRegisteredTasks()
	if len(tasks) != 1 {
		t.Fatalf("registered tasks = %d, want 1", len(tasks))
	}
	if got, want := tasks[0].TaskType(), upstreamstate.NewPollingTask().TaskType(); got != want {
		t.Fatalf("task type = %q, want %q", got, want)
	}
}
