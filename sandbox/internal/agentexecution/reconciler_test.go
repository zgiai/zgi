package agentexecution

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestReconcilerRecoversExpiredClaimAndFinalizesBinding(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	repository := newMemoryRepository()
	repository.bindings["binding-1"] = Binding{
		ID: "binding-1", OrganizationID: "org-1", SubjectID: "user-1", Backend: "fake",
		BackendRef: "backend-1", Status: BindingStatusDeleting, Generation: 4, UpdatedAt: now,
	}
	expired := now.Add(-time.Second)
	repository.cleanups = append(repository.cleanups, CleanupTask{
		ID: "cleanup-1", ResourceType: "binding", ResourceID: "binding-1", Backend: "fake",
		BackendRef: "backend-1", WorkspaceRef: "workspace-1", Action: "destroy", Status: "running", Attempts: 1,
		LeaseOwner: "dead-worker", LeaseExpiresAt: &expired, NotBefore: now.Add(-time.Minute), CreatedAt: now.Add(-time.Minute),
	})
	backend := &fakeBackend{}
	reconciler, err := NewReconciler(repository, map[string]Backend{"fake": backend}, ReconcilerOptions{
		OwnerID: "worker-2", Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new reconciler: %v", err)
	}
	processed, err := reconciler.ReconcileOne(context.Background())
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if !processed || backend.destroyCalls != 1 {
		t.Fatalf("processed=%v destroy calls=%d", processed, backend.destroyCalls)
	}
	if backend.destroyed.WorkspaceRef != "workspace-1" {
		t.Fatalf("destroy allocation = %+v", backend.destroyed)
	}
	if repository.bindings["binding-1"].Status != BindingStatusDeleted {
		t.Fatalf("binding status = %q, want deleted", repository.bindings["binding-1"].Status)
	}
	if repository.cleanups[0].Status != "completed" || repository.cleanups[0].Attempts != 2 {
		t.Fatalf("cleanup = %+v", repository.cleanups[0])
	}
}

func TestReconcilerSchedulesRetryOnBackendFailure(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	repository := newMemoryRepository()
	repository.bindings["binding-1"] = Binding{ID: "binding-1", Backend: "fake", Status: BindingStatusDeleting, Generation: 2}
	repository.cleanups = append(repository.cleanups, CleanupTask{
		ID: "cleanup-1", ResourceType: "binding", ResourceID: "binding-1", Backend: "fake",
		BackendRef: "backend-1", WorkspaceRef: "workspace-1", Action: "destroy", Status: "pending", NotBefore: now,
	})
	reconciler, err := NewReconciler(repository, map[string]Backend{"fake": &fakeBackend{destroyErr: errors.New("backend unavailable")}}, ReconcilerOptions{
		OwnerID: "worker-1", Now: func() time.Time { return now }, RetryDelay: 20 * time.Second,
	})
	if err != nil {
		t.Fatalf("new reconciler: %v", err)
	}
	processed, err := reconciler.ReconcileOne(context.Background())
	if !processed || err == nil {
		t.Fatalf("processed=%v error=%v, want processed failure", processed, err)
	}
	if repository.cleanups[0].Status != "retrying" || !repository.cleanups[0].NotBefore.Equal(now.Add(20*time.Second)) {
		t.Fatalf("cleanup retry = %+v", repository.cleanups[0])
	}
}

func TestReconcilerCompletesOnlyProvenEmptyDeletedBindingTasks(t *testing.T) {
	for _, test := range []struct {
		name                             string
		status                           BindingStatus
		backend, bindingID, publishedRef string
		missing, wantComplete            bool
	}{
		{name: "deleted", status: BindingStatusDeleted, backend: "fake", wantComplete: true},
		{name: "active", status: BindingStatusActive, backend: "fake"},
		{name: "deleting", status: BindingStatusDeleting, backend: "fake"},
		{name: "missing", missing: true},
		{name: "wrong-backend", status: BindingStatusDeleted, backend: "other"},
		{name: "wrong-binding", status: BindingStatusDeleted, backend: "fake", bindingID: "other"},
		{name: "published-resource", status: BindingStatusDeleted, backend: "fake", publishedRef: "must-preserve"},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := newMemoryRepository()
			if !test.missing {
				repository.bindings["binding-1"] = Binding{ID: "binding-1", Backend: test.backend, Status: test.status, BackendRef: test.publishedRef}
			}
			repository.cleanups = []CleanupTask{{ID: "legacy-empty", ResourceType: "binding", ResourceID: "binding-1", BindingID: test.bindingID, Backend: "fake", Action: "destroy", Status: "retrying"}}
			repository.cleanups = append(repository.cleanups, CleanupTask{
				ID: "allocation-discovery", BindingID: "binding-1", ResourceType: "allocation_attempt",
				Action: "reconcile", Status: "pending", NotBefore: time.Now().Add(time.Hour),
			})
			backend := &fakeBackend{destroyErr: ErrInvalidRequest}
			reconciler, err := NewReconciler(repository, map[string]Backend{"fake": backend}, ReconcilerOptions{OwnerID: "recovery"})
			if err != nil {
				t.Fatal(err)
			}
			processed, err := reconciler.ReconcileOne(t.Context())
			completed := repository.cleanups[0].Status == "completed"
			if !processed || completed != test.wantComplete || (err == nil) != test.wantComplete || backend.destroyCalls != 0 {
				t.Fatalf("processed=%v completed=%v error=%v destroy calls=%d; want complete=%v", processed, completed, err, backend.destroyCalls, test.wantComplete)
			}
			if len(repository.cleanups) != 2 || repository.cleanups[1].Status != "pending" || repository.cleanups[1].Attempts != 0 {
				t.Fatal("empty-task recovery changed allocation discovery ownership")
			}
		})
	}
}

func TestReconcilerCompletesClaimAfterBindingWasAlreadyFinalized(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	repository := newMemoryRepository()
	repository.bindings["binding-1"] = Binding{
		ID: "binding-1", Backend: "fake", Status: BindingStatusDeleted, Generation: 5,
	}
	expired := now.Add(-time.Second)
	repository.cleanups = append(repository.cleanups, CleanupTask{
		ID: "cleanup-1", ResourceType: "binding", ResourceID: "binding-1", Backend: "fake",
		BackendRef: "backend-1", WorkspaceRef: "workspace-1", Action: "destroy", Status: "running", Attempts: 1,
		LeaseOwner: "crashed-worker", LeaseExpiresAt: &expired, NotBefore: now.Add(-time.Minute),
	})
	reconciler, err := NewReconciler(repository, map[string]Backend{"fake": &fakeBackend{}}, ReconcilerOptions{
		OwnerID: "worker-2", Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new reconciler: %v", err)
	}
	processed, err := reconciler.ReconcileOne(context.Background())
	if err != nil || !processed {
		t.Fatalf("reconcile finalized binding: processed=%v error=%v", processed, err)
	}
	if repository.cleanups[0].Status != "completed" {
		t.Fatalf("cleanup status = %q, want completed", repository.cleanups[0].Status)
	}
}

func TestReconcilerDeletesSupersededCheckpoint(t *testing.T) {
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	repository := newMemoryRepository()
	store := &memoryCheckpointStore{data: map[string][]byte{"old": []byte("previous"), "new": []byte("current")}}
	repository.cleanups = append(repository.cleanups, CleanupTask{
		ID: "cleanup-checkpoint", ResourceType: "checkpoint", ResourceID: "old",
		Action: "delete", Status: "pending", NotBefore: now,
	})
	reconciler, err := NewReconciler(repository, map[string]Backend{"fake": &fakeBackend{}}, ReconcilerOptions{
		OwnerID: "checkpoint-worker", Now: func() time.Time { return now }, Checkpoints: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	if processed, err := reconciler.ReconcileOne(context.Background()); err != nil || !processed {
		t.Fatalf("checkpoint cleanup = %v, %v", processed, err)
	}
	if _, ok := store.data["old"]; ok || string(store.data["new"]) != "current" || repository.cleanups[0].Status != "completed" {
		t.Fatalf("checkpoint data = %#v; cleanup = %+v", store.data, repository.cleanups[0])
	}
}

func TestReconcilerDoesNotMarkBindingDeletedBeforeCheckpointRemoval(t *testing.T) {
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	repository := newMemoryRepository()
	repository.bindings["binding-1"] = Binding{
		ID: "binding-1", Backend: "fake", Status: BindingStatusDeleting,
		CheckpointKey: "retained", Generation: 3,
	}
	repository.cleanups = append(repository.cleanups, CleanupTask{
		ID: "cleanup-binding", ResourceType: "binding", ResourceID: "binding-1", Backend: "fake",
		BackendRef: "backend-1", Action: "destroy", Status: "pending", NotBefore: now,
	})
	reconciler, err := NewReconciler(repository, map[string]Backend{"fake": &fakeBackend{}}, ReconcilerOptions{
		OwnerID: "cleanup-worker", Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.ReconcileOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := repository.bindings["binding-1"].Status; got != BindingStatusDeleting {
		t.Fatalf("binding prematurely marked %s", got)
	}
}

func TestLeaseManagerFencesOldOwnerAfterExpiry(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	repository := newMemoryLeaseRepository()
	sequence := 0
	manager, err := NewLeaseManager(repository, LeaseManagerOptions{
		Now: func() time.Time { return now },
		NewID: func(prefix string) (string, error) {
			sequence++
			return fmt.Sprintf("%s-%d", prefix, sequence), nil
		},
		TTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("new lease manager: %v", err)
	}
	first, err := manager.Acquire(context.Background(), "binding-1", "worker-1")
	if err != nil {
		t.Fatalf("acquire first lease: %v", err)
	}
	if _, err := manager.Acquire(context.Background(), "binding-1", "worker-2"); !errors.Is(err, ErrLeaseHeld) {
		t.Fatalf("competing acquire error = %v, want ErrLeaseHeld", err)
	}
	now = now.Add(2 * time.Minute)
	second, err := manager.Acquire(context.Background(), "binding-1", "worker-2")
	if err != nil {
		t.Fatalf("take over expired lease: %v", err)
	}
	if second.Generation <= first.Generation || second.OwnerID != "worker-2" {
		t.Fatalf("takeover lease = %+v, first = %+v", second, first)
	}
	if _, err := manager.Renew(context.Background(), first); !errors.Is(err, ErrGenerationConflict) {
		t.Fatalf("old owner renew error = %v, want generation conflict", err)
	}
}

type memoryLeaseRepository struct {
	mu        sync.Mutex
	byID      map[string]AdmissionLease
	byBinding map[string]string
}

func newMemoryLeaseRepository() *memoryLeaseRepository {
	return &memoryLeaseRepository{byID: map[string]AdmissionLease{}, byBinding: map[string]string{}}
}

func (r *memoryLeaseRepository) AcquireLease(_ context.Context, requested AdmissionLease, now time.Time) (AdmissionLease, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if id, ok := r.byBinding[requested.BindingID]; ok {
		current := r.byID[id]
		if current.OwnerID != requested.OwnerID && current.ExpiresAt.After(now) {
			return AdmissionLease{}, ErrLeaseHeld
		}
		current.OwnerID = requested.OwnerID
		current.Generation++
		current.ExpiresAt = requested.ExpiresAt
		current.UpdatedAt = requested.UpdatedAt
		r.byID[id] = current
		return current, nil
	}
	r.byID[requested.ID] = requested
	r.byBinding[requested.BindingID] = requested.ID
	return requested, nil
}

func (r *memoryLeaseRepository) RenewLease(_ context.Context, id string, ownerID string, generation int64, now time.Time, expiresAt time.Time) (AdmissionLease, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.byID[id]
	if !ok || current.OwnerID != ownerID || current.Generation != generation || !current.ExpiresAt.After(now) {
		return AdmissionLease{}, ErrGenerationConflict
	}
	current.Generation++
	current.ExpiresAt = expiresAt
	current.UpdatedAt = now
	r.byID[id] = current
	return current, nil
}

func (r *memoryLeaseRepository) ReleaseLease(_ context.Context, id string, ownerID string, generation int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.byID[id]
	if !ok || current.OwnerID != ownerID || current.Generation != generation {
		return ErrGenerationConflict
	}
	delete(r.byID, id)
	delete(r.byBinding, current.BindingID)
	return nil
}
