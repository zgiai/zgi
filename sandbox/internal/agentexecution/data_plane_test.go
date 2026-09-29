package agentexecution

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"
)

func TestDataPlaneSubmitsIdempotentTenantScopedJob(t *testing.T) {
	bindings := newMemoryRepository()
	bindings.bindings["binding-1"] = activeTestBinding()
	jobs := newMemoryJobRepository()
	backend := newFakeExecutionBackend()
	service := newTestDataPlane(t, bindings, jobs, backend)
	request := testSubmitJobRequest()

	first, err := service.SubmitJob(context.Background(), request)
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}
	second, err := service.SubmitJob(context.Background(), request)
	if err != nil {
		t.Fatalf("submit idempotent job: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("idempotent jobs differ: %q != %q", first.ID, second.ID)
	}
	request.Command = "other"
	if _, err := service.SubmitJob(context.Background(), request); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("idempotency reuse error = %v", err)
	}
	if _, err := service.GetJob(context.Background(), Scope{OrganizationID: "other", SubjectID: "user-1"}, first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant get error = %v", err)
	}
}

func TestWorkerStreamsAndCompletesJob(t *testing.T) {
	bindings := newMemoryRepository()
	bindings.bindings["binding-1"] = activeTestBinding()
	jobs := newMemoryJobRepository()
	backend := newFakeExecutionBackend()
	service := newTestDataPlane(t, bindings, jobs, backend)
	job, err := service.SubmitJob(context.Background(), testSubmitJobRequest())
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}
	worker, err := NewWorker(jobs, map[string]ExecutionBackend{"fake": backend}, WorkerOptions{
		OwnerID: "worker-1", CancelPollInterval: 5 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	processed, err := worker.RunOne(context.Background())
	if err != nil || !processed {
		t.Fatalf("run job: processed=%v error=%v", processed, err)
	}
	completed, err := service.GetJob(context.Background(), testSubmitJobRequest().Scope, job.ID)
	if err != nil {
		t.Fatalf("get completed job: %v", err)
	}
	if completed.Status != JobStatusSucceeded || completed.ExitCode == nil || *completed.ExitCode != 0 {
		t.Fatalf("completed job = %+v", completed)
	}
	chunks, err := service.JobOutput(context.Background(), testSubmitJobRequest().Scope, job.ID, -1, 10)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if len(chunks) != 2 || string(chunks[0].Data) != "hello\n" || chunks[1].Stream != OutputStreamStderr {
		t.Fatalf("output chunks = %+v", chunks)
	}
}

func TestWorkerCancelsRunningJob(t *testing.T) {
	bindings := newMemoryRepository()
	bindings.bindings["binding-1"] = activeTestBinding()
	jobs := newMemoryJobRepository()
	backend := newFakeExecutionBackend()
	backend.blockUntilCancel = true
	service := newTestDataPlane(t, bindings, jobs, backend)
	job, err := service.SubmitJob(context.Background(), testSubmitJobRequest())
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}
	worker, err := NewWorker(jobs, map[string]ExecutionBackend{"fake": backend}, WorkerOptions{
		OwnerID: "worker-1", CancelPollInterval: 5 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, runErr := worker.RunOne(context.Background())
		done <- runErr
	}()

	var running Job
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		running, err = service.GetJob(context.Background(), testSubmitJobRequest().Scope, job.ID)
		if err == nil && running.Status == JobStatusRunning {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if running.Status != JobStatusRunning {
		t.Fatalf("job did not start: %+v error=%v", running, err)
	}
	if _, err := service.CancelJob(context.Background(), testSubmitJobRequest().Scope, job.ID, running.Generation); err != nil {
		t.Fatalf("cancel job: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("worker completion: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not stop canceled job")
	}
	canceled, err := service.GetJob(context.Background(), testSubmitJobRequest().Scope, job.ID)
	if err != nil {
		t.Fatalf("get canceled job: %v", err)
	}
	if canceled.Status != JobStatusCanceled || backend.cancelCalls != 1 {
		t.Fatalf("canceled job = %+v, cancel calls=%d", canceled, backend.cancelCalls)
	}
}

func TestDataPlaneValidatesPathsAndTransfersFiles(t *testing.T) {
	bindings := newMemoryRepository()
	bindings.bindings["binding-1"] = activeTestBinding()
	jobs := newMemoryJobRepository()
	backend := newFakeExecutionBackend()
	service := newTestDataPlane(t, bindings, jobs, backend)
	scope := testSubmitJobRequest().Scope
	if _, err := service.UploadFile(context.Background(), scope, "binding-1", "../escape", bytes.NewReader(nil), 0); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("path escape error = %v", err)
	}
	content := []byte("artifact")
	info, err := service.UploadFile(context.Background(), scope, "binding-1", "results/out.txt", bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("upload file: %v", err)
	}
	reader, downloaded, err := service.DownloadFile(context.Background(), scope, "binding-1", info.Path)
	if err != nil {
		t.Fatalf("download file: %v", err)
	}
	defer reader.Close()
	value, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read download: %v", err)
	}
	if !bytes.Equal(value, content) || downloaded.Size != int64(len(content)) {
		t.Fatalf("download = %q info=%+v", value, downloaded)
	}
}

func newTestDataPlane(t *testing.T, bindings BindingReader, jobs JobRepository, backend ExecutionBackend) *DataPlaneService {
	t.Helper()
	sequence := 0
	service, err := NewDataPlaneService(bindings, jobs, map[string]ExecutionBackend{"fake": backend}, DataPlaneOptions{
		Now: func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) },
		NewID: func(prefix string) (string, error) {
			sequence++
			return fmt.Sprintf("%s-%d", prefix, sequence), nil
		},
	})
	if err != nil {
		t.Fatalf("new data plane: %v", err)
	}
	return service
}

func activeTestBinding() Binding {
	return Binding{
		ID: "binding-1", OrganizationID: "org-1", SubjectID: "user-1", Backend: "fake",
		BackendRef: "backend-1", WorkspaceRef: "workspace-1", Status: BindingStatusActive, Generation: 2,
	}
}

func testSubmitJobRequest() SubmitJobRequest {
	return SubmitJobRequest{
		Scope: Scope{OrganizationID: "org-1", SubjectID: "user-1"}, BindingID: "binding-1",
		IdempotencyKey: "job-request-1", Command: "python3", Args: []string{"-c", "print('hello')"},
		Environment: map[string]string{"LANG": "C.UTF-8"}, WorkDir: "workspace", Timeout: time.Minute,
	}
}

type fakeExecutionBackend struct {
	mu               sync.Mutex
	blockUntilCancel bool
	cancelCalls      int
	files            map[string][]byte
}

func newFakeExecutionBackend() *fakeExecutionBackend {
	return &fakeExecutionBackend{files: map[string][]byte{}}
}

func (b *fakeExecutionBackend) Capabilities(context.Context, Allocation) (RuntimeCapabilities, error) {
	return RuntimeCapabilities{Shell: true, PTY: true, FileTransfer: true, NetworkMode: "disabled", MaxUploadBytes: defaultFileTransferMax, MaxDownloadBytes: defaultFileTransferMax}, nil
}

func (b *fakeExecutionBackend) StartJob(_ context.Context, _ Allocation, _ JobSpec, sink func(OutputStream, []byte) error) (RunningJob, error) {
	if err := sink(OutputStreamStdout, []byte("hello\n")); err != nil {
		return nil, err
	}
	if err := sink(OutputStreamStderr, []byte("diagnostic\n")); err != nil {
		return nil, err
	}
	running := &fakeRunningJob{done: make(chan struct{}), backend: b}
	if !b.blockUntilCancel {
		close(running.done)
	}
	return running, nil
}

func (b *fakeExecutionBackend) UploadFile(_ context.Context, _ Allocation, filePath string, reader io.Reader, size int64) (FileInfo, error) {
	value, err := io.ReadAll(reader)
	if err != nil {
		return FileInfo{}, err
	}
	if int64(len(value)) != size {
		return FileInfo{}, errors.New("size mismatch")
	}
	b.mu.Lock()
	b.files[filePath] = append([]byte(nil), value...)
	b.mu.Unlock()
	return FileInfo{Path: filePath, Size: size, ContentType: "application/octet-stream"}, nil
}

func (b *fakeExecutionBackend) DownloadFile(_ context.Context, _ Allocation, filePath string) (io.ReadCloser, FileInfo, error) {
	b.mu.Lock()
	value, ok := b.files[filePath]
	b.mu.Unlock()
	if !ok {
		return nil, FileInfo{}, ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(value)), FileInfo{Path: filePath, Size: int64(len(value))}, nil
}

type fakeRunningJob struct {
	done    chan struct{}
	once    sync.Once
	backend *fakeExecutionBackend
}

func (j *fakeRunningJob) Wait(ctx context.Context) (JobResult, error) {
	select {
	case <-j.done:
		j.backend.mu.Lock()
		canceled := j.backend.cancelCalls > 0
		j.backend.mu.Unlock()
		return JobResult{ExitCode: 0, Canceled: canceled}, nil
	case <-ctx.Done():
		return JobResult{ExitCode: -1, Canceled: true}, ctx.Err()
	}
}

func (j *fakeRunningJob) Cancel(context.Context) error {
	j.backend.mu.Lock()
	j.backend.cancelCalls++
	j.backend.mu.Unlock()
	j.once.Do(func() { close(j.done) })
	return nil
}

func (j *fakeRunningJob) ResizePTY(context.Context, uint16, uint16) error { return nil }

type memoryJobRepository struct {
	mu          sync.Mutex
	jobs        map[string]Job
	idempotency map[string]string
	chunks      map[string][]OutputChunk
}

func newMemoryJobRepository() *memoryJobRepository {
	return &memoryJobRepository{jobs: map[string]Job{}, idempotency: map[string]string{}, chunks: map[string][]OutputChunk{}}
}

func (r *memoryJobRepository) CreateJob(_ context.Context, job Job) (Job, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := job.BindingID + "\x00" + job.IdempotencyKey
	if id, ok := r.idempotency[key]; ok {
		return cloneJob(r.jobs[id]), false, nil
	}
	r.jobs[job.ID] = cloneJob(job)
	r.idempotency[key] = job.ID
	return cloneJob(job), true, nil
}

func (r *memoryJobRepository) Job(_ context.Context, id string) (Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok {
		return Job{}, ErrNotFound
	}
	return cloneJob(job), nil
}

func (r *memoryJobRepository) ClaimJob(_ context.Context, owner string, now time.Time, expiresAt time.Time) (Job, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, job := range r.jobs {
		claimable := job.Status == JobStatusQueued || (job.Status == JobStatusRunning && job.ClaimExpiresAt != nil && !job.ClaimExpiresAt.After(now))
		if !claimable || job.CancelRequested {
			continue
		}
		job.Status = JobStatusRunning
		job.Generation++
		job.ClaimOwner = owner
		job.ClaimExpiresAt = timePointer(expiresAt)
		job.Attempts++
		job.UpdatedAt = now
		if job.StartedAt == nil {
			job.StartedAt = timePointer(now)
		}
		r.jobs[id] = job
		return cloneJob(job), true, nil
	}
	return Job{}, false, nil
}

func (r *memoryJobRepository) RenewJobClaim(_ context.Context, id string, owner string, now time.Time, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok || job.Status != JobStatusRunning || job.ClaimOwner != owner || job.ClaimExpiresAt == nil || !job.ClaimExpiresAt.After(now) {
		return ErrJobClaimLost
	}
	job.ClaimExpiresAt = timePointer(expiresAt)
	job.UpdatedAt = now
	r.jobs[id] = job
	return nil
}

func (r *memoryJobRepository) AppendJobOutput(_ context.Context, id string, stream OutputStream, data []byte, maximum int64, now time.Time) (OutputChunk, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok {
		return OutputChunk{}, ErrNotFound
	}
	if job.StdoutBytes+job.StderrBytes+int64(len(data)) > maximum {
		return OutputChunk{}, ErrOutputLimit
	}
	if stream == OutputStreamStdout {
		job.StdoutBytes += int64(len(data))
	} else if stream == OutputStreamStderr {
		job.StderrBytes += int64(len(data))
	} else {
		return OutputChunk{}, ErrInvalidRequest
	}
	chunk := OutputChunk{JobID: id, Sequence: int64(len(r.chunks[id])), Stream: stream, Data: append([]byte(nil), data...), CreatedAt: now}
	r.jobs[id] = job
	r.chunks[id] = append(r.chunks[id], chunk)
	return chunk, nil
}

func (r *memoryJobRepository) JobCancellationRequested(_ context.Context, id string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok {
		return false, ErrNotFound
	}
	return job.CancelRequested, nil
}

func (r *memoryJobRepository) CompleteJob(_ context.Context, id string, owner string, result JobResult, now time.Time) (Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok || job.Status != JobStatusRunning || job.ClaimOwner != owner || job.ClaimExpiresAt == nil || !job.ClaimExpiresAt.After(now) {
		return Job{}, ErrJobClaimLost
	}
	status := JobStatusSucceeded
	if result.Canceled || job.CancelRequested {
		status = JobStatusCanceled
	} else if result.Error != "" || result.ExitCode != 0 {
		status = JobStatusFailed
	}
	job.Status = status
	job.Generation++
	job.ExitCode = intPointer(result.ExitCode)
	job.FailureReason = result.Error
	job.ClaimOwner = ""
	job.ClaimExpiresAt = nil
	job.UpdatedAt = now
	job.FinishedAt = timePointer(now)
	r.jobs[id] = job
	return cloneJob(job), nil
}

func (r *memoryJobRepository) RequestJobCancellation(_ context.Context, id string, generation int64, now time.Time) (Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok {
		return Job{}, ErrNotFound
	}
	if job.Generation != generation {
		return Job{}, ErrGenerationConflict
	}
	job.CancelRequested = true
	job.Generation++
	job.UpdatedAt = now
	if job.Status == JobStatusQueued {
		job.Status = JobStatusCanceled
		job.FinishedAt = timePointer(now)
	}
	r.jobs[id] = job
	return cloneJob(job), nil
}

func (r *memoryJobRepository) JobOutput(_ context.Context, id string, after int64, limit int) ([]OutputChunk, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.jobs[id]; !ok {
		return nil, ErrNotFound
	}
	result := make([]OutputChunk, 0, limit)
	for _, chunk := range r.chunks[id] {
		if chunk.Sequence <= after {
			continue
		}
		copyChunk := chunk
		copyChunk.Data = append([]byte(nil), chunk.Data...)
		result = append(result, copyChunk)
		if len(result) == limit {
			break
		}
	}
	return result, nil
}

func cloneJob(job Job) Job {
	job.Args = append([]string(nil), job.Args...)
	job.Environment = cloneEnvironment(job.Environment)
	return job
}

func timePointer(value time.Time) *time.Time { return &value }
func intPointer(value int) *int              { return &value }
