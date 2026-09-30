package agentexecution

import (
	"context"
	"testing"
	"time"
)

type discoverableBackend struct {
	fakeBackend
	attemptID   string
	allocations []Allocation
}

func (b *discoverableBackend) CreateBinding(ctx context.Context, spec CreateSpec) (Allocation, error) {
	b.attemptID = spec.AllocationAttemptID
	return b.fakeBackend.CreateBinding(ctx, spec)
}

func (b *discoverableBackend) FindAllocations(_ context.Context, attemptID string) ([]Allocation, error) {
	if attemptID != b.attemptID {
		return nil, ErrInvalidRequest
	}
	return b.allocations, nil
}

func TestAllocationAttemptIsDurableBeforeProviderCreation(t *testing.T) {
	repository := newMemoryRepository()
	backend := &discoverableBackend{}
	service := newTestService(t, repository, backend)
	binding, err := service.CreateBinding(context.Background(), testCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	if backend.attemptID == "" || len(repository.cleanups) != 1 || repository.cleanups[0].ResourceID != backend.attemptID || repository.cleanups[0].BindingID != binding.ID {
		t.Fatalf("provider attempt = %q, journal = %+v", backend.attemptID, repository.cleanups)
	}
	backend.allocations = []Allocation{{BindingRef: binding.BackendRef}, {BindingRef: "orphan-sandbox"}}
	now := repository.cleanups[0].NotBefore
	reconciler, err := NewReconciler(repository, map[string]Backend{"fake": backend}, ReconcilerOptions{
		OwnerID: "recovery-worker", Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if processed, err := reconciler.ReconcileOne(context.Background()); err != nil || !processed {
		t.Fatalf("reconcile = %v, %v", processed, err)
	}
	if backend.destroyCalls != 1 || backend.destroyed.BindingRef != "orphan-sandbox" || repository.cleanups[0].Status != "completed" {
		t.Fatalf("destroyed = %+v, calls=%d, task=%+v", backend.destroyed, backend.destroyCalls, repository.cleanups[0])
	}
}

func TestAllocationAttemptReapsCreationLostBeforeDatabaseWriteback(t *testing.T) {
	repository := newMemoryRepository()
	backend := &discoverableBackend{}
	service := newTestService(t, repository, backend)
	binding, err := service.CreateBinding(context.Background(), testCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	backend.allocations = []Allocation{{BindingRef: "unrecorded-sandbox"}}
	repository.mu.Lock()
	binding.Status = BindingStatusResuming
	binding.Generation++
	repository.bindings[binding.ID] = binding
	repository.mu.Unlock()
	now := repository.cleanups[0].NotBefore
	reconciler, err := NewReconciler(repository, map[string]Backend{"fake": backend}, ReconcilerOptions{
		OwnerID: "recovery-worker", Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.ReconcileOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if backend.destroyCalls != 1 || backend.destroyed.BindingRef != "unrecorded-sandbox" {
		t.Fatalf("orphan cleanup = %+v, calls=%d", backend.destroyed, backend.destroyCalls)
	}
}

func TestAllocationAttemptRecoversInterruptedBindingState(t *testing.T) {
	for _, status := range []BindingStatus{BindingStatusAllocating, BindingStatusResuming} {
		t.Run(string(status), func(t *testing.T) {
			repository := newMemoryRepository()
			backend := &discoverableBackend{}
			service := newTestService(t, repository, backend)
			binding, err := service.CreateBinding(context.Background(), testCreateRequest())
			if err != nil {
				t.Fatal(err)
			}
			task := repository.cleanups[0]
			if task.BindingGeneration != 1 {
				t.Fatalf("attempt generation = %d", task.BindingGeneration)
			}
			// Simulate a Worker crash after a provider allocation but before the
			// generation-fenced Binding update. A newer attempt must not be
			// confused with this stale allocation journal entry.
			repository.mu.Lock()
			binding.Status = status
			binding.Generation = task.BindingGeneration
			binding.CheckpointKey = "retained-checkpoint"
			repository.bindings[binding.ID] = binding
			repository.mu.Unlock()
			backend.allocations = []Allocation{{BindingRef: "orphan-sandbox"}}
			now := task.NotBefore
			reconciler, err := NewReconciler(repository, map[string]Backend{"fake": backend}, ReconcilerOptions{
				OwnerID: "recovery-worker", Now: func() time.Time { return now },
			})
			if err != nil {
				t.Fatal(err)
			}
			if processed, err := reconciler.ReconcileOne(context.Background()); err != nil || !processed {
				t.Fatalf("reconcile = %t, %v", processed, err)
			}
			got, err := repository.Binding(context.Background(), binding.ID)
			if err != nil {
				t.Fatal(err)
			}
			wantStatus := BindingStatusFailed
			if status == BindingStatusResuming {
				wantStatus = BindingStatusSuspended
			}
			if got.Status != wantStatus || got.Generation != task.BindingGeneration+1 || got.CheckpointKey != "retained-checkpoint" || backend.destroyCalls != 1 {
				t.Fatalf("recovered binding = %+v, destroy calls=%d", got, backend.destroyCalls)
			}
		})
	}
}
