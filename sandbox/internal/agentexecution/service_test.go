package agentexecution

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCreateBindingIsIdempotentAndTenantScoped(t *testing.T) {
	repository := newMemoryRepository()
	backend := &fakeBackend{}
	service := newTestService(t, repository, backend)
	request := testCreateRequest()

	first, err := service.CreateBinding(context.Background(), request)
	if err != nil {
		t.Fatalf("create first binding: %v", err)
	}
	second, err := service.CreateBinding(context.Background(), request)
	if err != nil {
		t.Fatalf("create idempotent binding: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("idempotent create returned different bindings: %q != %q", first.ID, second.ID)
	}
	if backend.createCalls != 1 {
		t.Fatalf("backend create calls = %d, want 1", backend.createCalls)
	}
	if _, err := service.GetBinding(context.Background(), Scope{OrganizationID: "other", SubjectID: "user-1"}, first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant lookup error = %v, want ErrNotFound", err)
	}
}

func TestCreateBindingRejectsIdempotencyKeyReuse(t *testing.T) {
	repository := newMemoryRepository()
	service := newTestService(t, repository, &fakeBackend{})
	request := testCreateRequest()
	if _, err := service.CreateBinding(context.Background(), request); err != nil {
		t.Fatalf("create binding: %v", err)
	}
	request.ProfileID = "profile-other"
	if _, err := service.CreateBinding(context.Background(), request); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("reuse error = %v, want ErrIdempotencyConflict", err)
	}
}

func TestCreateBindingQueuesCleanupWhenActivationWriteFails(t *testing.T) {
	repository := newMemoryRepository()
	repository.failUpdate = true
	service := newTestService(t, repository, &fakeBackend{})
	if _, err := service.CreateBinding(context.Background(), testCreateRequest()); err == nil {
		t.Fatal("create binding succeeded, want activation persistence failure")
	}
	if len(repository.cleanups) != 1 {
		t.Fatalf("cleanup tasks = %d, want 1", len(repository.cleanups))
	}
	if repository.cleanups[0].Action != "destroy" {
		t.Fatalf("cleanup action = %q, want destroy", repository.cleanups[0].Action)
	}
}

func TestCreateBindingQueuesCleanupWhenBackendPartiallyFails(t *testing.T) {
	repository := newMemoryRepository()
	backend := &fakeBackend{createErr: errors.New("injected partial allocation failure")}
	service := newTestService(t, repository, backend)
	if _, err := service.CreateBinding(context.Background(), testCreateRequest()); err == nil {
		t.Fatal("create binding succeeded, want backend failure")
	}
	if len(repository.cleanups) != 1 {
		t.Fatalf("cleanup tasks = %d, want 1", len(repository.cleanups))
	}
	if repository.cleanups[0].BackendRef == "" {
		t.Fatal("partial backend reference was not retained for cleanup")
	}
}

func TestSuspendBindingUsesGenerationFence(t *testing.T) {
	repository := newMemoryRepository()
	backend := &fakeBackend{}
	service := newTestService(t, repository, backend)
	binding, err := service.CreateBinding(context.Background(), testCreateRequest())
	if err != nil {
		t.Fatalf("create binding: %v", err)
	}
	if _, err := service.SuspendBinding(context.Background(), testCreateRequest().Scope, binding.ID, binding.Generation-1); !errors.Is(err, ErrGenerationConflict) {
		t.Fatalf("stale suspend error = %v, want ErrGenerationConflict", err)
	}
	suspended, err := service.SuspendBinding(context.Background(), testCreateRequest().Scope, binding.ID, binding.Generation)
	if err != nil {
		t.Fatalf("suspend binding: %v", err)
	}
	if suspended.Status != BindingStatusSuspended {
		t.Fatalf("status = %q, want suspended", suspended.Status)
	}
	if backend.suspendCalls != 1 {
		t.Fatalf("backend suspend calls = %d, want 1", backend.suspendCalls)
	}
}

func TestSuspendBindingResumesAfterControlPlaneCrash(t *testing.T) {
	repository := newMemoryRepository()
	backend := &fakeBackend{}
	service := newTestService(t, repository, backend)
	binding, err := service.CreateBinding(context.Background(), testCreateRequest())
	if err != nil {
		t.Fatalf("create binding: %v", err)
	}

	repository.mu.Lock()
	binding.Status = BindingStatusSuspending
	binding.Generation++
	repository.bindings[binding.ID] = binding
	repository.mu.Unlock()

	suspended, err := service.SuspendBinding(context.Background(), testCreateRequest().Scope, binding.ID, binding.Generation)
	if err != nil {
		t.Fatalf("resume suspend: %v", err)
	}
	if suspended.Status != BindingStatusSuspended || backend.suspendCalls != 1 {
		t.Fatalf("resume result = %+v, backend calls = %d", suspended, backend.suspendCalls)
	}
}

func TestDestroyBindingIsIdempotent(t *testing.T) {
	repository := newMemoryRepository()
	backend := &fakeBackend{}
	service := newTestService(t, repository, backend)
	binding, err := service.CreateBinding(context.Background(), testCreateRequest())
	if err != nil {
		t.Fatalf("create binding: %v", err)
	}
	deleted, err := service.DestroyBinding(context.Background(), testCreateRequest().Scope, binding.ID, binding.Generation)
	if err != nil {
		t.Fatalf("destroy binding: %v", err)
	}
	deletedAgain, err := service.DestroyBinding(context.Background(), testCreateRequest().Scope, binding.ID, 0)
	if err != nil {
		t.Fatalf("repeat destroy: %v", err)
	}
	if deletedAgain.Status != BindingStatusDeleted || deletedAgain.ID != deleted.ID {
		t.Fatalf("repeat destroy returned unexpected binding: %+v", deletedAgain)
	}
	if backend.destroyCalls != 1 {
		t.Fatalf("backend destroy calls = %d, want 1", backend.destroyCalls)
	}
}

func TestConcurrentCreateAllocatesOnce(t *testing.T) {
	repository := newMemoryRepository()
	backend := &fakeBackend{}
	service := newTestService(t, repository, backend)
	request := testCreateRequest()

	const workers = 12
	var wait sync.WaitGroup
	ids := make(chan string, workers)
	errs := make(chan error, workers)
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			binding, err := service.CreateBinding(context.Background(), request)
			if err != nil {
				errs <- err
				return
			}
			ids <- binding.ID
		}()
	}
	wait.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent create: %v", err)
	}
	var firstID string
	for id := range ids {
		if firstID == "" {
			firstID = id
		}
		if id != firstID {
			t.Fatalf("concurrent create IDs differ: %q != %q", id, firstID)
		}
	}
	if backend.createCalls != 1 {
		t.Fatalf("backend create calls = %d, want 1", backend.createCalls)
	}
}

func newTestService(t *testing.T, repository Repository, backend Backend) *Service {
	t.Helper()
	var sequence atomic.Int64
	service, err := NewService(repository, map[string]Backend{"fake": backend}, ServiceOptions{
		Now: func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) },
		NewID: func(prefix string) (string, error) {
			return fmt.Sprintf("%s_%d", prefix, sequence.Add(1)), nil
		},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return service
}

func testCreateRequest() CreateBindingRequest {
	return CreateBindingRequest{
		Scope:          Scope{OrganizationID: "org-1", SubjectID: "user-1"},
		AgentID:        "agent-1",
		ConversationID: "conversation-1",
		ProfileID:      "profile-1",
		Backend:        "fake",
		IdempotencyKey: "request-1",
	}
}

type fakeBackend struct {
	mu           sync.Mutex
	createCalls  int
	suspendCalls int
	destroyCalls int
	createErr    error
	destroyErr   error
}

func (b *fakeBackend) CreateBinding(_ context.Context, spec CreateSpec) (Allocation, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.createCalls++
	return Allocation{BindingRef: "backend-" + spec.BindingID, WorkspaceRef: "workspace-" + spec.WorkspaceID}, b.createErr
}

func (b *fakeBackend) SuspendBinding(context.Context, Allocation) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.suspendCalls++
	return nil
}

func (b *fakeBackend) DestroyBinding(context.Context, Allocation) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.destroyCalls++
	return b.destroyErr
}

type memoryRepository struct {
	mu          sync.Mutex
	bindings    map[string]Binding
	idempotency map[string]string
	cleanups    []CleanupTask
	failUpdate  bool
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{bindings: map[string]Binding{}, idempotency: map[string]string{}}
}

func (r *memoryRepository) ReserveBinding(_ context.Context, binding Binding) (Binding, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := binding.OrganizationID + "\x00" + binding.SubjectID + "\x00" + binding.IdempotencyKey
	if id, ok := r.idempotency[key]; ok {
		return r.bindings[id], false, nil
	}
	r.bindings[binding.ID] = binding
	r.idempotency[key] = binding.ID
	return binding, true, nil
}

func (r *memoryRepository) Binding(_ context.Context, id string) (Binding, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	binding, ok := r.bindings[id]
	if !ok {
		return Binding{}, ErrNotFound
	}
	return binding, nil
}

func (r *memoryRepository) UpdateBinding(_ context.Context, binding Binding, expectedGeneration int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failUpdate {
		return errors.New("injected update failure")
	}
	current, ok := r.bindings[binding.ID]
	if !ok {
		return ErrNotFound
	}
	if current.Generation != expectedGeneration {
		return ErrGenerationConflict
	}
	r.bindings[binding.ID] = binding
	return nil
}

func (r *memoryRepository) EnqueueCleanup(_ context.Context, task CleanupTask) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanups = append(r.cleanups, task)
	return nil
}

func (r *memoryRepository) ClaimCleanup(_ context.Context, ownerID string, now time.Time, leaseExpiresAt time.Time) (CleanupTask, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for index := range r.cleanups {
		task := &r.cleanups[index]
		due := (task.Status == "pending" || task.Status == "retrying") && !task.NotBefore.After(now)
		expired := task.Status == "running" && task.LeaseExpiresAt != nil && !task.LeaseExpiresAt.After(now)
		if !due && !expired {
			continue
		}
		task.Status = "running"
		task.Attempts++
		task.LeaseOwner = ownerID
		task.LeaseExpiresAt = &leaseExpiresAt
		task.UpdatedAt = now
		return *task, true, nil
	}
	return CleanupTask{}, false, nil
}

func (r *memoryRepository) CompleteCleanup(_ context.Context, taskID string, ownerID string, succeeded bool, lastError string, notBefore time.Time, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for index := range r.cleanups {
		task := &r.cleanups[index]
		if task.ID != taskID || task.LeaseOwner != ownerID || task.Status != "running" || task.LeaseExpiresAt == nil || !task.LeaseExpiresAt.After(now) {
			continue
		}
		task.Status = "retrying"
		if succeeded {
			task.Status = "completed"
		}
		task.LastError = lastError
		task.NotBefore = notBefore
		task.LeaseOwner = ""
		task.LeaseExpiresAt = nil
		task.UpdatedAt = now
		return nil
	}
	return ErrCleanupClaimLost
}
