package agentexecution

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// unknownStartBackend reports the start outcome unknown for the first starts,
// as when the runtime agent's answer is lost, then attaches to the job.
type unknownStartBackend struct {
	*fakeExecutionBackend
	unknownStarts int
	startErr      error
	onUnknown     func()
	mu            sync.Mutex
	starts        int
}

func (b *unknownStartBackend) StartJob(ctx context.Context, allocation Allocation, spec JobSpec, sink func(OutputChunk) error) (RunningJob, error) {
	b.mu.Lock()
	b.starts++
	attempt := b.starts
	b.mu.Unlock()
	if b.startErr != nil {
		return nil, b.startErr
	}
	if b.unknownStarts < 0 || attempt <= b.unknownStarts {
		if b.onUnknown != nil {
			b.onUnknown()
		}
		return nil, errors.Join(ErrJobStartUnknown, errors.New("EOF"))
	}
	return b.fakeExecutionBackend.StartJob(ctx, allocation, spec, sink)
}

func (b *unknownStartBackend) startCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.starts
}

type unknownStartFixture struct {
	service *DataPlaneService
	jobs    *memoryJobRepository
	backend *unknownStartBackend
	job     Job
}

func newUnknownStartFixture(t *testing.T, backend *unknownStartBackend) unknownStartFixture {
	t.Helper()
	bindings := newMemoryRepository()
	bindings.bindings["binding-1"] = activeTestBinding()
	jobs := newMemoryJobRepository()
	service := newTestDataPlane(t, bindings, jobs, backend)
	job, err := service.SubmitJob(context.Background(), testSubmitJobRequest())
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}
	return unknownStartFixture{service: service, jobs: jobs, backend: backend, job: job}
}

func (f unknownStartFixture) worker(t *testing.T, options WorkerOptions) *Worker {
	t.Helper()
	options.OwnerID, options.CancelPollInterval = "worker-1", time.Millisecond
	worker, err := NewWorker(f.jobs, map[string]ExecutionBackend{"fake": f.backend}, options)
	if err != nil {
		t.Fatal(err)
	}
	return worker
}

func (f unknownStartFixture) current(t *testing.T) Job {
	t.Helper()
	job, err := f.service.GetJob(context.Background(), testSubmitJobRequest().Scope, f.job.ID)
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func TestWorkerAttachesToAJobWhoseStartAnswerWasLost(t *testing.T) {
	f := newUnknownStartFixture(t, &unknownStartBackend{fakeExecutionBackend: newFakeExecutionBackend(), unknownStarts: 2})
	processed, err := f.worker(t, WorkerOptions{}).RunOne(context.Background())
	if err != nil || !processed {
		t.Fatalf("run job: processed=%v error=%v", processed, err)
	}
	if job := f.current(t); job.Status != JobStatusSucceeded || f.backend.startCount() != 3 {
		t.Fatalf("job = %+v after %d starts; want it followed to success instead of failed on the lost answer", job, f.backend.startCount())
	}
}

func TestWorkerCancelsAJobCanceledWhileItsStartWasUnknown(t *testing.T) {
	backend := &unknownStartBackend{fakeExecutionBackend: newFakeExecutionBackend(), unknownStarts: 2}
	backend.blockUntilCancel = true
	f := newUnknownStartFixture(t, backend)
	var once sync.Once
	backend.onUnknown = func() {
		once.Do(func() {
			// The job is claimed and running by now; the user cancels it before
			// the worker could attach.
			running := f.current(t)
			if _, err := f.service.CancelJob(context.Background(), testSubmitJobRequest().Scope, running.ID, running.Generation); err != nil {
				t.Errorf("cancel job: %v", err)
			}
		})
	}
	done := make(chan error, 1)
	go func() {
		_, err := f.worker(t, WorkerOptions{}).RunOne(context.Background())
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("worker: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker never stopped the job canceled during its start")
	}
	backend.mu.Lock()
	cancelCalls := backend.cancelCalls
	backend.mu.Unlock()
	if job := f.current(t); job.Status != JobStatusCanceled || cancelCalls != 1 {
		t.Fatalf("job = %+v, cancel calls %d; want the attached job stopped before it settled", job, cancelCalls)
	}
}

func TestWorkerFencesAJobWhoseStartStaysUnknown(t *testing.T) {
	f := newUnknownStartFixture(t, &unknownStartBackend{fakeExecutionBackend: newFakeExecutionBackend(), unknownStarts: -1})
	var quarantined atomic.Int32
	worker := f.worker(t, WorkerOptions{
		StartUnknownTimeout: 20 * time.Millisecond,
		QuarantineJob: func(context.Context, Job) error {
			quarantined.Add(1)
			return nil
		},
	})
	if _, err := worker.RunOne(context.Background()); err != nil {
		t.Fatalf("run job: %v", err)
	}
	if job := f.current(t); job.Status != JobStatusFailed || quarantined.Load() != 1 || f.backend.startCount() < 2 {
		t.Fatalf("job = %+v, quarantined %d, starts %d; want the unreachable job fenced and failed after retrying", job, quarantined.Load(), f.backend.startCount())
	}
}

func TestWorkerKeepsTheClaimWhenStoppedDuringAnUnknownStart(t *testing.T) {
	f := newUnknownStartFixture(t, &unknownStartBackend{fakeExecutionBackend: newFakeExecutionBackend(), unknownStarts: -1})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := f.worker(t, WorkerOptions{}).RunOne(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("run job err = %v, want the worker stop reported", err)
	}
	if job := f.current(t); job.Status != JobStatusRunning {
		t.Fatalf("job = %+v; want it left running for the next claim to attach", job)
	}
}

func TestWorkerFailsAJobItsBackendRejected(t *testing.T) {
	f := newUnknownStartFixture(t, &unknownStartBackend{fakeExecutionBackend: newFakeExecutionBackend(), startErr: ErrIdempotencyConflict})
	if _, err := f.worker(t, WorkerOptions{}).RunOne(context.Background()); err != nil {
		t.Fatalf("run job: %v", err)
	}
	if job := f.current(t); job.Status != JobStatusFailed || f.backend.startCount() != 1 {
		t.Fatalf("job = %+v after %d starts; want a rejected start failed at once", job, f.backend.startCount())
	}
}
