package v1

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zgiai/zgi/api/config"
	pconsole "github.com/zgiai/zgi/api/internal/infra/platform/console"
	"github.com/zgiai/zgi/api/internal/modules/llm"
	"github.com/zgiai/zgi/api/internal/modules/llm/credential/upstreamstate"
	authservice "github.com/zgiai/zgi/api/internal/modules/user/auth/service"
	pkgscheduler "github.com/zgiai/zgi/api/pkg/scheduler"
)

type failingRegistrationTaskRegistrar struct {
	err error
}

func (r failingRegistrationTaskRegistrar) RegisterTask(pkgscheduler.ScheduledTask, pkgscheduler.TaskHandler) error {
	return r.err
}

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

func TestMustRegisterRegistrationProvisioningOutboxTaskPanicsWithoutScheduler(t *testing.T) {
	task := &authservice.RegistrationProvisioningOutboxTask{}
	handler := authservice.NewRegistrationProvisioningOutboxHandler(nil, 25)

	require.Panics(t, func() {
		mustRegisterRegistrationProvisioningOutboxTask(nil, task, handler)
	})
}

func TestMustRegisterRegistrationProvisioningOutboxTaskPanicsOnRegistrationFailure(t *testing.T) {
	task := &authservice.RegistrationProvisioningOutboxTask{}
	handler := authservice.NewRegistrationProvisioningOutboxHandler(nil, 25)

	require.PanicsWithValue(t, "failed to register registration provisioning outbox task: scheduler unavailable", func() {
		mustRegisterRegistrationProvisioningOutboxTask(
			failingRegistrationTaskRegistrar{err: errors.New("scheduler unavailable")},
			task,
			handler,
		)
	})
}

func TestFailCloudLLMInitializationPanicsOnlyInCloudMode(t *testing.T) {
	initErr := errors.New("invalid encryption key")
	require.Panics(t, func() {
		failCloudLLMInitialization(pconsole.NewRemote("http://console.invalid", "test-key"), "LLM crypto service", initErr)
	})
	require.NotPanics(t, func() {
		failCloudLLMInitialization(pconsole.NewStandalone(), "LLM crypto service", initErr)
	})
}
