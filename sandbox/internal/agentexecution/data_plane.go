package agentexecution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
	"sync"
	"time"
)

const (
	defaultJobClaimLease   = 30 * time.Second
	defaultCancelPoll      = 100 * time.Millisecond
	defaultJobOutputBytes  = 4 * 1024 * 1024
	defaultFileTransferMax = 64 * 1024 * 1024
)

type SubmitJobRequest struct {
	Scope          Scope
	BindingID      string
	IdempotencyKey string
	Command        string
	Args           []string
	Environment    map[string]string
	WorkDir        string
	PTY            bool
	PTYColumns     uint16
	PTYRows        uint16
	Timeout        time.Duration
}

type DataPlaneOptions struct {
	Now                  func() time.Time
	NewID                func(string) (string, error)
	MaxFileTransferBytes int64
}

type DataPlaneService struct {
	bindings             BindingReader
	jobs                 JobRepository
	backends             map[string]ExecutionBackend
	now                  func() time.Time
	newID                func(string) (string, error)
	maxFileTransferBytes int64
}

func NewDataPlaneService(bindings BindingReader, jobs JobRepository, backends map[string]ExecutionBackend, options DataPlaneOptions) (*DataPlaneService, error) {
	if bindings == nil || jobs == nil {
		return nil, fmt.Errorf("%w: binding and job repositories are required", ErrInvalidRequest)
	}
	if options.Now == nil {
		options.Now = func() time.Time { return time.Now().UTC() }
	}
	if options.NewID == nil {
		options.NewID = randomID
	}
	if options.MaxFileTransferBytes <= 0 {
		options.MaxFileTransferBytes = defaultFileTransferMax
	}
	return &DataPlaneService{
		bindings: bindings, jobs: jobs, backends: copyExecutionBackends(backends),
		now: options.Now, newID: options.NewID, maxFileTransferBytes: options.MaxFileTransferBytes,
	}, nil
}

func (s *DataPlaneService) SubmitJob(ctx context.Context, request SubmitJobRequest) (Job, error) {
	request = normalizeSubmitJobRequest(request)
	if err := validateSubmitJobRequest(request); err != nil {
		return Job{}, err
	}
	if request.WorkDir != "" {
		request.WorkDir, _ = cleanWorkspacePath(request.WorkDir, true)
	}
	binding, err := s.authorizedBinding(ctx, request.Scope, request.BindingID)
	if err != nil {
		return Job{}, err
	}
	if binding.Status != BindingStatusActive {
		return Job{}, fmt.Errorf("%w: binding is %s", ErrInvalidTransition, binding.Status)
	}
	if _, ok := s.backends[binding.Backend]; !ok {
		return Job{}, fmt.Errorf("%w: %s", ErrBackendUnavailable, binding.Backend)
	}
	id, err := s.newID("arj")
	if err != nil {
		return Job{}, fmt.Errorf("create job id: %w", err)
	}
	now := s.now().UTC()
	job := Job{
		ID: id, BindingID: binding.ID, OrganizationID: binding.OrganizationID, SubjectID: binding.SubjectID,
		Backend: binding.Backend, BackendRef: binding.BackendRef, WorkspaceRef: binding.WorkspaceRef,
		IdempotencyKey: request.IdempotencyKey, RequestHash: jobRequestFingerprint(request),
		Status: JobStatusQueued, Generation: 1, Command: request.Command,
		Args: append([]string(nil), request.Args...), Environment: cloneEnvironment(request.Environment),
		WorkDir: request.WorkDir, PTY: request.PTY, PTYColumns: request.PTYColumns,
		PTYRows: request.PTYRows, Timeout: request.Timeout, CreatedAt: now, UpdatedAt: now,
	}
	job, created, err := s.jobs.CreateJob(ctx, job)
	if err != nil {
		return Job{}, err
	}
	if !created && job.RequestHash != jobRequestFingerprint(request) {
		return Job{}, ErrIdempotencyConflict
	}
	return job, nil
}

func (s *DataPlaneService) GetJob(ctx context.Context, scope Scope, jobID string) (Job, error) {
	job, err := s.jobs.Job(ctx, strings.TrimSpace(jobID))
	if err != nil {
		return Job{}, err
	}
	if !ownsJob(scope, job) {
		return Job{}, ErrNotFound
	}
	return job, nil
}

func (s *DataPlaneService) CancelJob(ctx context.Context, scope Scope, jobID string, generation int64) (Job, error) {
	job, err := s.GetJob(ctx, scope, jobID)
	if err != nil {
		return Job{}, err
	}
	if isTerminalJob(job.Status) {
		return job, nil
	}
	if generation != job.Generation {
		return Job{}, ErrGenerationConflict
	}
	return s.jobs.RequestJobCancellation(ctx, job.ID, generation, s.now().UTC())
}

func (s *DataPlaneService) JobOutput(ctx context.Context, scope Scope, jobID string, afterSequence int64, limit int) ([]OutputChunk, error) {
	if _, err := s.GetJob(ctx, scope, jobID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	return s.jobs.JobOutput(ctx, strings.TrimSpace(jobID), afterSequence, limit)
}

func (s *DataPlaneService) Capabilities(ctx context.Context, scope Scope, bindingID string) (RuntimeCapabilities, error) {
	binding, err := s.authorizedBinding(ctx, scope, bindingID)
	if err != nil {
		return RuntimeCapabilities{}, err
	}
	backend, ok := s.backends[binding.Backend]
	if !ok {
		return RuntimeCapabilities{}, fmt.Errorf("%w: %s", ErrBackendUnavailable, binding.Backend)
	}
	return backend.Capabilities(ctx, allocationForBinding(binding))
}

func (s *DataPlaneService) UploadFile(ctx context.Context, scope Scope, bindingID string, filePath string, reader io.Reader, size int64) (FileInfo, error) {
	cleanPath, err := cleanWorkspacePath(filePath, false)
	if err != nil {
		return FileInfo{}, err
	}
	if reader == nil || size < 0 || size > s.maxFileTransferBytes {
		return FileInfo{}, fmt.Errorf("%w: upload size is invalid", ErrInvalidRequest)
	}
	binding, err := s.authorizedBinding(ctx, scope, bindingID)
	if err != nil {
		return FileInfo{}, err
	}
	backend, ok := s.backends[binding.Backend]
	if !ok {
		return FileInfo{}, fmt.Errorf("%w: %s", ErrBackendUnavailable, binding.Backend)
	}
	return backend.UploadFile(ctx, allocationForBinding(binding), cleanPath, io.LimitReader(reader, size), size)
}

func (s *DataPlaneService) DownloadFile(ctx context.Context, scope Scope, bindingID string, filePath string) (io.ReadCloser, FileInfo, error) {
	cleanPath, err := cleanWorkspacePath(filePath, false)
	if err != nil {
		return nil, FileInfo{}, err
	}
	binding, err := s.authorizedBinding(ctx, scope, bindingID)
	if err != nil {
		return nil, FileInfo{}, err
	}
	backend, ok := s.backends[binding.Backend]
	if !ok {
		return nil, FileInfo{}, fmt.Errorf("%w: %s", ErrBackendUnavailable, binding.Backend)
	}
	reader, info, err := backend.DownloadFile(ctx, allocationForBinding(binding), cleanPath)
	if err != nil {
		return nil, FileInfo{}, err
	}
	if info.Size < 0 || info.Size > s.maxFileTransferBytes {
		_ = reader.Close()
		return nil, FileInfo{}, ErrOutputLimit
	}
	return reader, info, nil
}

func (s *DataPlaneService) authorizedBinding(ctx context.Context, scope Scope, bindingID string) (Binding, error) {
	binding, err := s.bindings.Binding(ctx, strings.TrimSpace(bindingID))
	if err != nil {
		return Binding{}, err
	}
	if !owns(scope, binding) {
		return Binding{}, ErrNotFound
	}
	return binding, nil
}

type WorkerOptions struct {
	OwnerID            string
	Now                func() time.Time
	ClaimLease         time.Duration
	CancelPollInterval time.Duration
	MaxOutputBytes     int64
}

type Worker struct {
	jobs               JobRepository
	backends           map[string]ExecutionBackend
	ownerID            string
	now                func() time.Time
	claimLease         time.Duration
	cancelPollInterval time.Duration
	maxOutputBytes     int64
}

func NewWorker(jobs JobRepository, backends map[string]ExecutionBackend, options WorkerOptions) (*Worker, error) {
	if jobs == nil || strings.TrimSpace(options.OwnerID) == "" {
		return nil, fmt.Errorf("%w: job repository and worker owner are required", ErrInvalidRequest)
	}
	if options.Now == nil {
		options.Now = func() time.Time { return time.Now().UTC() }
	}
	if options.ClaimLease <= 0 {
		options.ClaimLease = defaultJobClaimLease
	}
	if options.CancelPollInterval <= 0 {
		options.CancelPollInterval = defaultCancelPoll
	}
	if options.MaxOutputBytes <= 0 {
		options.MaxOutputBytes = defaultJobOutputBytes
	}
	return &Worker{
		jobs: jobs, backends: copyExecutionBackends(backends), ownerID: strings.TrimSpace(options.OwnerID),
		now: options.Now, claimLease: options.ClaimLease,
		cancelPollInterval: options.CancelPollInterval, maxOutputBytes: options.MaxOutputBytes,
	}, nil
}

// RunOne claims and runs at most one job. It blocks until that job reaches a
// terminal state; callers own the worker loop and its shutdown context.
func (w *Worker) RunOne(ctx context.Context) (bool, error) {
	now := w.now().UTC()
	job, claimed, err := w.jobs.ClaimJob(ctx, w.ownerID, now, now.Add(w.claimLease))
	if err != nil || !claimed {
		return claimed, err
	}
	backend, ok := w.backends[job.Backend]
	if !ok {
		return true, w.finishJob(ctx, job, JobResult{ExitCode: -1, Error: fmt.Sprintf("%v: %s", ErrBackendUnavailable, job.Backend)})
	}

	jobCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var outputMu sync.Mutex
	var outputErr error
	sink := func(stream OutputStream, data []byte) error {
		if len(data) == 0 {
			return nil
		}
		_, err := w.jobs.AppendJobOutput(jobCtx, job.ID, stream, append([]byte(nil), data...), w.maxOutputBytes, w.now().UTC())
		if err != nil {
			outputMu.Lock()
			if outputErr == nil {
				outputErr = err
			}
			outputMu.Unlock()
			cancel()
		}
		return err
	}
	running, err := backend.StartJob(jobCtx, allocationForJob(job), JobSpec{
		JobID: job.ID, Command: job.Command, Args: append([]string(nil), job.Args...),
		Environment: cloneEnvironment(job.Environment), WorkDir: job.WorkDir,
		PTY: job.PTY, PTYColumns: job.PTYColumns, PTYRows: job.PTYRows, Timeout: job.Timeout,
	}, sink)
	if err != nil {
		return true, w.finishJob(context.WithoutCancel(ctx), job, JobResult{ExitCode: -1, Error: err.Error()})
	}

	waitResult := make(chan workerResult, 1)
	go func() {
		result, waitErr := running.Wait(jobCtx)
		waitResult <- workerResult{result: result, err: waitErr}
	}()
	ticker := time.NewTicker(w.cancelPollInterval)
	defer ticker.Stop()
	cancelSent := false
	nextRenewal := now.Add(w.claimLease / 2)
	for {
		select {
		case result := <-waitResult:
			outputMu.Lock()
			capturedOutputErr := outputErr
			outputMu.Unlock()
			if capturedOutputErr != nil {
				result.result.Error = capturedOutputErr.Error()
				result.result.ExitCode = -1
			}
			if result.err != nil && result.result.Error == "" {
				result.result.Error = result.err.Error()
			}
			return true, w.finishJob(context.WithoutCancel(ctx), job, result.result)
		case <-ticker.C:
			tickNow := w.now().UTC()
			if !tickNow.Before(nextRenewal) {
				if err := w.jobs.RenewJobClaim(jobCtx, job.ID, w.ownerID, tickNow, tickNow.Add(w.claimLease)); err != nil {
					_ = running.Cancel(context.WithoutCancel(ctx))
					return true, err
				}
				nextRenewal = tickNow.Add(w.claimLease / 2)
			}
			requested, err := w.jobs.JobCancellationRequested(jobCtx, job.ID)
			if err != nil {
				_ = running.Cancel(context.WithoutCancel(ctx))
				return true, err
			}
			if requested && !cancelSent {
				cancelSent = true
				_ = running.Cancel(jobCtx)
			}
		case <-ctx.Done():
			_ = running.Cancel(context.WithoutCancel(ctx))
			return true, ctx.Err()
		}
	}
}

func (w *Worker) finishJob(ctx context.Context, job Job, result JobResult) error {
	if result.Error != "" {
		result.Error = limitFailureReason(result.Error)
	}
	_, err := w.jobs.CompleteJob(ctx, job.ID, w.ownerID, result, w.now().UTC())
	return err
}

type workerResult struct {
	result JobResult
	err    error
}

func normalizeSubmitJobRequest(request SubmitJobRequest) SubmitJobRequest {
	request.Scope.OrganizationID = strings.TrimSpace(request.Scope.OrganizationID)
	request.Scope.SubjectID = strings.TrimSpace(request.Scope.SubjectID)
	request.BindingID = strings.TrimSpace(request.BindingID)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	request.Command = strings.TrimSpace(request.Command)
	request.WorkDir = strings.TrimSpace(request.WorkDir)
	request.Args = append([]string(nil), request.Args...)
	request.Environment = cloneEnvironment(request.Environment)
	return request
}

func validateSubmitJobRequest(request SubmitJobRequest) error {
	if request.Scope.OrganizationID == "" || request.Scope.SubjectID == "" || request.BindingID == "" || request.IdempotencyKey == "" || request.Command == "" {
		return fmt.Errorf("%w: scope, binding, idempotency key, and command are required", ErrInvalidRequest)
	}
	if strings.ContainsRune(request.Command, '\x00') {
		return fmt.Errorf("%w: command contains a null byte", ErrInvalidRequest)
	}
	for _, argument := range request.Args {
		if strings.ContainsRune(argument, '\x00') {
			return fmt.Errorf("%w: argument contains a null byte", ErrInvalidRequest)
		}
	}
	for key, value := range request.Environment {
		if strings.TrimSpace(key) == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("%w: environment contains an invalid entry", ErrInvalidRequest)
		}
	}
	if request.WorkDir != "" {
		clean, err := cleanWorkspacePath(request.WorkDir, true)
		if err != nil {
			return err
		}
		request.WorkDir = clean
	}
	if request.PTY && (request.PTYColumns == 0 || request.PTYRows == 0) {
		return fmt.Errorf("%w: PTY dimensions are required", ErrInvalidRequest)
	}
	if request.Timeout < 0 {
		return fmt.Errorf("%w: timeout cannot be negative", ErrInvalidRequest)
	}
	return nil
}

func cleanWorkspacePath(value string, allowDot bool) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || strings.ContainsRune(value, '\x00') || strings.HasPrefix(value, "/") || (len(value) >= 2 && value[1] == ':') {
		return "", ErrInvalidPath
	}
	clean := path.Clean(value)
	if clean == ".." || strings.HasPrefix(clean, "../") || (!allowDot && clean == ".") {
		return "", ErrInvalidPath
	}
	return clean, nil
}

func jobRequestFingerprint(request SubmitJobRequest) string {
	payload, _ := json.Marshal(struct {
		BindingID   string            `json:"binding_id"`
		Command     string            `json:"command"`
		Args        []string          `json:"args"`
		Environment map[string]string `json:"environment"`
		WorkDir     string            `json:"work_dir"`
		PTY         bool              `json:"pty"`
		Columns     uint16            `json:"columns"`
		Rows        uint16            `json:"rows"`
		TimeoutNS   int64             `json:"timeout_ns"`
	}{request.BindingID, request.Command, request.Args, request.Environment, request.WorkDir, request.PTY, request.PTYColumns, request.PTYRows, int64(request.Timeout)})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func ownsJob(scope Scope, job Job) bool {
	return strings.TrimSpace(scope.OrganizationID) != "" && strings.TrimSpace(scope.SubjectID) != "" &&
		strings.TrimSpace(scope.OrganizationID) == job.OrganizationID && strings.TrimSpace(scope.SubjectID) == job.SubjectID
}

func isTerminalJob(status JobStatus) bool {
	return status == JobStatusSucceeded || status == JobStatusFailed || status == JobStatusCanceled
}

func allocationForBinding(binding Binding) Allocation {
	return Allocation{BindingRef: binding.BackendRef, WorkspaceRef: binding.WorkspaceRef}
}

func allocationForJob(job Job) Allocation {
	return Allocation{BindingRef: job.BackendRef, WorkspaceRef: job.WorkspaceRef}
}

func cloneEnvironment(value map[string]string) map[string]string {
	if value == nil {
		return map[string]string{}
	}
	result := make(map[string]string, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}

func copyExecutionBackends(backends map[string]ExecutionBackend) map[string]ExecutionBackend {
	result := make(map[string]ExecutionBackend, len(backends))
	for name, backend := range backends {
		if strings.TrimSpace(name) != "" && backend != nil {
			result[strings.TrimSpace(name)] = backend
		}
	}
	return result
}
