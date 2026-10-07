package agentexecution

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAllocationAttemptDoesNotDestroyQuarantinedDisk(t *testing.T) {
	repository := newMemoryRepository()
	backend := &discoverableBackend{}
	s := newTestService(t, repository, backend)
	binding, err := s.CreateBinding(t.Context(), testCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.QuarantineJob(t.Context(), Job{BindingID: binding.ID, BindingGeneration: binding.Generation, BackendRef: binding.BackendRef}); err != nil {
		t.Fatal(err)
	}
	backend.allocations = []Allocation{{BindingRef: binding.BackendRef, WorkspaceRef: binding.WorkspaceRef}, {BindingRef: "unrelated-orphan"}}
	now := repository.cleanups[0].NotBefore.Add(time.Second)
	reconciler, err := NewReconciler(repository, map[string]Backend{"fake": backend}, ReconcilerOptions{OwnerID: "recovery", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if processed, err := reconciler.ReconcileOne(t.Context()); err != nil || !processed {
		t.Fatalf("allocation audit=%v err=%v", processed, err)
	}
	if backend.destroyCalls != 1 || backend.destroyed.BindingRef != "unrelated-orphan" {
		t.Fatalf("quarantined storage deleted: %+v calls=%d", backend.destroyed, backend.destroyCalls)
	}
}

type retryablePauseBackend struct {
	fakeBackend
	pauseErr error
}

type unsafeExecutionBackend struct{ *fakeExecutionBackend }

func (b *unsafeExecutionBackend) StartJob(context.Context, Allocation, JobSpec, func(OutputChunk) error) (RunningJob, error) {
	return unsafeRunningJob{}, nil
}

type unsafeRunningJob struct{}

func (unsafeRunningJob) Wait(context.Context) (JobResult, error) {
	return JobResult{ExitCode: -1, TerminationFailed: true, Error: "termination denied"}, errors.New("termination denied")
}

func (unsafeRunningJob) Cancel(context.Context) error                    { return nil }
func (unsafeRunningJob) ResizePTY(context.Context, uint16, uint16) error { return nil }

func TestWorkerQuarantinesBeforeCompletingUnsafeJob(t *testing.T) {
	bindings := newMemoryRepository()
	bindings.bindings["binding-1"] = activeTestBinding()
	jobs := newMemoryJobRepository()
	backend := &unsafeExecutionBackend{newFakeExecutionBackend()}
	data := newTestDataPlane(t, bindings, jobs, backend)
	job, err := data.SubmitJob(t.Context(), testSubmitJobRequest())
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	worker, err := NewWorker(jobs, map[string]ExecutionBackend{"fake": backend}, WorkerOptions{
		OwnerID: "worker-unsafe",
		QuarantineJob: func(ctx context.Context, claimed Job) error {
			current, err := jobs.Job(ctx, claimed.ID)
			if err != nil || current.Status != JobStatusRunning || current.ClaimOwner != claimed.ClaimOwner {
				t.Fatalf("unsafe job completed before fencing compute: %+v error=%v", current, err)
			}
			calls++
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if processed, err := worker.RunOne(t.Context()); err != nil || !processed {
		t.Fatalf("run unsafe job=%v error=%v", processed, err)
	}
	current, err := jobs.Job(t.Context(), job.ID)
	if err != nil || calls != 1 || current.Status != JobStatusFailed || current.FailureReason != "termination denied" {
		t.Fatalf("unsafe job=%+v error=%v quarantine=%d", current, err, calls)
	}
}

func (b *retryablePauseBackend) SuspendBinding(ctx context.Context, allocation Allocation) error {
	_ = b.fakeBackend.SuspendBinding(ctx, allocation)
	return b.pauseErr
}

func TestFailedQuarantinePauseRetriesWithoutDeletingRetainedStorage(t *testing.T) {
	repository := newMemoryRepository()
	backend := &retryablePauseBackend{pauseErr: errors.New("provider unavailable")}
	s, err := NewService(repository, map[string]Backend{"fake": backend}, ServiceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := s.CreateBinding(t.Context(), testCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	job := Job{BindingID: binding.ID, BindingGeneration: binding.Generation, BackendRef: binding.BackendRef}
	if err := s.QuarantineJob(t.Context(), job); err == nil {
		t.Fatal("pause failure was hidden")
	}
	var task CleanupTask
	for _, queued := range repository.cleanups {
		if queued.Action == "quarantine" {
			task = queued
		}
	}
	if task.ID == "" || task.WorkspaceRef != binding.WorkspaceRef || task.BindingGeneration != binding.Generation+1 {
		t.Fatalf("pause retry was not retained: %+v", task)
	}
	backend.pauseErr = nil
	reconciler, err := NewReconciler(repository, map[string]Backend{"fake": backend}, ReconcilerOptions{OwnerID: "recovery-worker"})
	if err != nil {
		t.Fatal(err)
	}
	if processed, err := reconciler.ReconcileOne(t.Context()); err != nil || !processed {
		t.Fatalf("retry processed=%v err=%v", processed, err)
	}
	current, err := repository.Binding(t.Context(), binding.ID)
	if err != nil || current.Status != BindingStatusFailed || backend.destroyCalls != 0 || backend.suspendCalls != 2 {
		t.Fatalf("retry binding=%+v err=%v pause=%d destroy=%d", current, err, backend.suspendCalls, backend.destroyCalls)
	}
}

func TestQuarantineFencesExecutionWithoutDestroyingFilesOrComputeDisk(t *testing.T) {
	repository := newMemoryRepository()
	backend := &fakeBackend{}
	s := newTestService(t, repository, backend)
	binding, err := s.CreateBinding(t.Context(), testCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	job := Job{BindingID: binding.ID, BindingGeneration: binding.Generation, BackendRef: binding.BackendRef}
	stale := job
	stale.BindingGeneration--
	if err := s.QuarantineJob(t.Context(), stale); !errors.Is(err, ErrGenerationConflict) {
		t.Fatalf("stale quarantine = %v", err)
	}
	if err := s.QuarantineJob(t.Context(), job); err != nil {
		t.Fatal(err)
	}
	current, err := repository.Binding(t.Context(), binding.ID)
	if err != nil || current.Status != BindingStatusFailed || current.Generation != binding.Generation+1 || backend.suspendCalls != 1 || backend.destroyCalls != 0 {
		t.Fatalf("quarantine = %+v err=%v pause=%d destroy=%d", current, err, backend.suspendCalls, backend.destroyCalls)
	}
}
