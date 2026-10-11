package agentexecution

import (
	"context"
	"errors"
	"testing"
)

type cancellationCheckingRepository struct{ *memoryRepository }

func (r cancellationCheckingRepository) EnqueueCleanup(ctx context.Context, task CleanupTask) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.memoryRepository.EnqueueCleanup(ctx, task)
}

func (r cancellationCheckingRepository) UpdateBinding(ctx context.Context, binding Binding, generation int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.memoryRepository.UpdateBinding(ctx, binding, generation)
}

type cancelCreationBackend struct {
	fakeBackend
	cancel context.CancelFunc
}

func (b *cancelCreationBackend) CreateBinding(ctx context.Context, spec CreateSpec) (Allocation, error) {
	allocation, _ := b.fakeBackend.CreateBinding(ctx, spec)
	b.cancel()
	return allocation, context.Canceled
}

func TestCanceledCreationStillJournalsCleanupAndFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	memory := newMemoryRepository()
	repository := cancellationCheckingRepository{memory}
	backend := &cancelCreationBackend{cancel: cancel}
	service := newTestService(t, repository, backend)
	binding, err := service.CreateBinding(ctx, testCreateRequest())
	if !errors.Is(err, context.Canceled) || binding.Status != BindingStatusFailed {
		t.Fatalf("create = %+v, %v", binding, err)
	}
	stored, err := memory.Binding(t.Context(), binding.ID)
	if err != nil || stored.Status != BindingStatusFailed || stored.Generation != 2 {
		t.Fatalf("stored binding = %+v, %v", stored, err)
	}
	if len(memory.cleanups) != 1 || memory.cleanups[0].BackendRef == "" || memory.cleanups[0].Action != "destroy" {
		t.Fatalf("cleanup journal = %+v", memory.cleanups)
	}
}
