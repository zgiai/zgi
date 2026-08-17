package music

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

func TestMusicTaskTimeoutsPreserveBillingSettlementOrder(t *testing.T) {
	if generationTaskTimeout != 9*time.Minute {
		t.Fatalf("generationTaskTimeout = %s, want %s", generationTaskTimeout, 9*time.Minute)
	}
	if generationRecoveryAge != 10*time.Minute {
		t.Fatalf("generationRecoveryAge = %s, want %s", generationRecoveryAge, 10*time.Minute)
	}
}

func TestTaskHandlerPassesValidatedTaskID(t *testing.T) {
	want := uuid.New()
	var got uuid.UUID
	handler := newTaskHandler(func(_ context.Context, id uuid.UUID) error {
		got = id
		return nil
	})
	task := asynq.NewTask(GenerationTaskType, []byte(`{"task_id":"`+want.String()+`"}`))

	if err := handler(t.Context(), task); err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if got != want {
		t.Fatalf("task id = %s, want %s", got, want)
	}
}

func TestTaskHandlerRejectsMalformedPayloadWithoutRetry(t *testing.T) {
	err := newTaskHandler(func(context.Context, uuid.UUID) error { return nil })(
		t.Context(),
		asynq.NewTask(GenerationTaskType, []byte(`{"task_id":`)),
	)
	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("handler() error = %v, want SkipRetry", err)
	}
}

func TestTaskHandlerRejectsTrailingPayloadWithoutRetry(t *testing.T) {
	id := uuid.NewString()
	err := newTaskHandler(func(context.Context, uuid.UUID) error { return nil })(
		t.Context(),
		asynq.NewTask(GenerationTaskType, []byte(`{"task_id":"`+id+`"}{}`)),
	)
	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("handler() error = %v, want SkipRetry", err)
	}
}

func TestRegisterTaskHandlersRejectsDuplicateRegistration(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("RegisterTaskHandlers() did not panic for duplicate registration")
		}
	}()
	RegisterTaskHandlers(
		&taskRegistryStub{registered: map[string]bool{"test:" + GenerationTaskType: true}},
		taskPrefixerStub{},
		NewWorker(newMemoryRepository(), &dispatcherStub{}, &musicGeneratorStub{}, &lyricsGeneratorStub{}, &deliveryCompensatorStub{}, &assetStoreStub{}),
	)
}

type taskPrefixerStub struct{}

func (taskPrefixerStub) GetTaskTypeWithPrefix(taskType string) string { return "test:" + taskType }

type taskRegistryStub struct {
	registered map[string]bool
}

func (s *taskRegistryStub) Register(taskType string, _ func(context.Context, *asynq.Task) error) bool {
	if s.registered[taskType] {
		return false
	}
	s.registered[taskType] = true
	return true
}
