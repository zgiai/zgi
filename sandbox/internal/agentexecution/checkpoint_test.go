package agentexecution

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
)

func TestOperatorRecoveryRebuildsWithoutContactingSource(t *testing.T) {
	repository := newMemoryRepository()
	backend := &checkpointBackend{content: []byte("retained dependency and file bytes")}
	store := &memoryCheckpointStore{data: make(map[string][]byte)}
	control, err := NewService(repository, map[string]Backend{"fake": backend}, ServiceOptions{Checkpoints: store, Jobs: &testActiveJobs{}})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := control.CreateBinding(t.Context(), testCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	binding, err = control.SuspendBinding(t.Context(), testCreateRequest().Scope, binding.ID, binding.Generation)
	if err != nil {
		t.Fatal(err)
	}
	// Logical restore deliberately drops both source references.
	binding.BackendRef, binding.WorkspaceRef = "", ""
	repository.bindings[binding.ID] = binding
	restored, err := control.ResumeBinding(t.Context(), testCreateRequest().Scope, binding.ID, binding.Generation)
	if err != nil || restored.Status != BindingStatusActive || backend.resumeCalls != 0 || backend.createCalls != 2 || !bytes.Equal(backend.restored, backend.content) {
		t.Fatalf("restored=%+v error=%v resume=%d create=%d", restored, err, backend.resumeCalls, backend.createCalls)
	}
}

func TestCheckpointSurvivesControlRestartAndLostCompute(t *testing.T) {
	repository := newMemoryRepository()
	backend := &checkpointBackend{content: []byte("two-csv-result")}
	store := &memoryCheckpointStore{data: make(map[string][]byte)}
	jobs := &testActiveJobs{}
	newControl := func() *Service {
		service, err := NewService(repository, map[string]Backend{"fake": backend}, ServiceOptions{
			Checkpoints: store, Jobs: jobs,
		})
		if err != nil {
			t.Fatal(err)
		}
		return service
	}
	control := newControl()
	binding, err := control.CreateBinding(context.Background(), testCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	suspended, err := control.SuspendBinding(context.Background(), testCreateRequest().Scope, binding.ID, binding.Generation)
	if err != nil || suspended.CheckpointKey == "" {
		t.Fatalf("suspend = %+v, %v", suspended, err)
	}
	if !bytes.Equal(store.data[suspended.CheckpointKey], backend.content) {
		t.Fatal("checkpoint was not published")
	}
	backend.lost = true
	control = newControl() // control-plane process restarted; repository and store remain.
	resumed, err := control.ResumeBinding(context.Background(), testCreateRequest().Scope, suspended.ID, suspended.Generation)
	if err != nil || resumed.Status != BindingStatusActive {
		t.Fatalf("resume = %+v, %v", resumed, err)
	}
	if backend.createCalls != 2 || !bytes.Equal(backend.restored, backend.content) {
		t.Fatalf("rebuild calls = %d, restored = %q", backend.createCalls, backend.restored)
	}
	deleted, err := control.DestroyBinding(context.Background(), testCreateRequest().Scope, resumed.ID, resumed.Generation)
	if err != nil || deleted.Status != BindingStatusDeleted || len(store.data) != 0 {
		t.Fatalf("destroy = %+v, %v, retained snapshots = %d", deleted, err, len(store.data))
	}
}

func TestCheckpointWaitsForActiveJobsAndRetriesSuspension(t *testing.T) {
	repository := newMemoryRepository()
	backend := &checkpointBackend{content: []byte("result")}
	store := &memoryCheckpointStore{data: make(map[string][]byte)}
	jobs := &testActiveJobs{active: true}
	control, err := NewService(repository, map[string]Backend{"fake": backend}, ServiceOptions{Checkpoints: store, Jobs: jobs})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := control.CreateBinding(context.Background(), testCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := control.SuspendBinding(context.Background(), testCreateRequest().Scope, binding.ID, binding.Generation); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("busy suspension error = %v", err)
	}
	current, err := control.GetBinding(context.Background(), testCreateRequest().Scope, binding.ID)
	if err != nil || current.Status != BindingStatusSuspending || backend.suspendCalls != 0 {
		t.Fatalf("interrupted suspension = %+v, %v", current, err)
	}
	jobs.active = false
	suspended, err := control.SuspendBinding(context.Background(), testCreateRequest().Scope, binding.ID, current.Generation)
	if err != nil || suspended.Status != BindingStatusSuspended || backend.suspendCalls != 1 {
		t.Fatalf("retried suspension = %+v, %v", suspended, err)
	}
}

func TestCheckpointRetryUsesPublishedPendingKey(t *testing.T) {
	repository := newMemoryRepository()
	backend := &checkpointBackend{content: []byte("unchanged"), suspendFailure: true}
	store := &memoryCheckpointStore{data: make(map[string][]byte)}
	control, err := NewService(repository, map[string]Backend{"fake": backend}, ServiceOptions{Checkpoints: store, Jobs: &testActiveJobs{}})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := control.CreateBinding(context.Background(), testCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := control.SuspendBinding(context.Background(), testCreateRequest().Scope, binding.ID, binding.Generation); err == nil {
		t.Fatal("backend suspension unexpectedly succeeded")
	}
	current, err := control.GetBinding(context.Background(), testCreateRequest().Scope, binding.ID)
	if err != nil || current.PendingCheckpointKey == "" || len(store.data) != 1 {
		t.Fatalf("pending checkpoint = %+v, %v", current, err)
	}
	suspended, err := control.SuspendBinding(context.Background(), testCreateRequest().Scope, binding.ID, current.Generation)
	if err != nil || suspended.CheckpointKey != current.PendingCheckpointKey || len(store.data) != 1 {
		t.Fatalf("retried checkpoint = %+v, %v", suspended, err)
	}
}

func TestCheckpointRestoreFailureDestroysReplacement(t *testing.T) {
	repository := newMemoryRepository()
	backend := &checkpointBackend{content: []byte("result")}
	store := &memoryCheckpointStore{data: make(map[string][]byte)}
	control, err := NewService(repository, map[string]Backend{"fake": backend}, ServiceOptions{Checkpoints: store, Jobs: &testActiveJobs{}})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := control.CreateBinding(context.Background(), testCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	suspended, err := control.SuspendBinding(context.Background(), testCreateRequest().Scope, binding.ID, binding.Generation)
	if err != nil {
		t.Fatal(err)
	}
	backend.lost = true
	backend.importErr = errors.New("injected archive failure")
	if _, err := control.ResumeBinding(context.Background(), testCreateRequest().Scope, binding.ID, suspended.Generation); err == nil {
		t.Fatal("failed restore was accepted")
	}
	if backend.destroyCalls != 1 {
		t.Fatalf("replacement cleanup calls = %d", backend.destroyCalls)
	}
	current, err := control.GetBinding(context.Background(), testCreateRequest().Scope, binding.ID)
	if err != nil || current.Status != BindingStatusResuming {
		t.Fatalf("failed restore state = %+v, %v", current, err)
	}
}

type checkpointBackend struct {
	fakeBackend
	content        []byte
	restored       []byte
	lost           bool
	importErr      error
	suspendFailure bool
}

func (b *checkpointBackend) SuspendBinding(ctx context.Context, allocation Allocation) error {
	if b.suspendFailure {
		b.suspendFailure = false
		return errors.New("injected suspend failure")
	}
	return b.fakeBackend.SuspendBinding(ctx, allocation)
}

func (b *checkpointBackend) ResumeBinding(ctx context.Context, spec CreateSpec, allocation Allocation) (Allocation, error) {
	if b.lost {
		return Allocation{}, ErrNotFound
	}
	return b.fakeBackend.ResumeBinding(ctx, spec, allocation)
}

func (b *checkpointBackend) ExportWorkspace(context.Context, Allocation) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(b.content)), nil
}

func (b *checkpointBackend) ImportWorkspace(_ context.Context, _ Allocation, archive io.Reader) error {
	if b.importErr != nil {
		return b.importErr
	}
	var err error
	b.restored, err = io.ReadAll(archive)
	return err
}

type memoryCheckpointStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (s *memoryCheckpointStore) Save(_ context.Context, key string, source io.Reader) error {
	value, err := io.ReadAll(source)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.data[key] = value
	s.mu.Unlock()
	return nil
}

func (s *memoryCheckpointStore) Open(_ context.Context, key string) (io.ReadCloser, error) {
	s.mu.Lock()
	value, ok := s.data[key]
	s.mu.Unlock()
	if !ok {
		return nil, ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(value)), nil
}

func (s *memoryCheckpointStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	delete(s.data, key)
	s.mu.Unlock()
	return nil
}

type testActiveJobs struct{ active bool }

func (j *testActiveJobs) HasActiveJobs(context.Context, string) (bool, error) { return j.active, nil }
