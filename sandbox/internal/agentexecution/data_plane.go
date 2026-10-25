package agentexecution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	defaultFileUploadMax   = 100 * 1024 * 1024
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
	MaxJobTimeout        time.Duration
	Now                  func() time.Time
	NewID                func(string) (string, error)
	MaxFileTransferBytes int64
	MaxFileUploadBytes   int64
	IdleTTL              time.Duration
}

type DataPlaneService struct {
	maxJobTimeout        time.Duration
	bindings             BindingReader
	jobs                 JobRepository
	backends             map[string]ExecutionBackend
	now                  func() time.Time
	newID                func(string) (string, error)
	maxFileTransferBytes int64
	maxFileUploadBytes   int64
	idleTTL              time.Duration
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
	if options.MaxFileUploadBytes <= 0 {
		options.MaxFileUploadBytes = options.MaxFileTransferBytes
		if options.MaxFileUploadBytes <= 0 {
			options.MaxFileUploadBytes = defaultFileUploadMax
		}
	}
	if options.MaxFileTransferBytes <= 0 {
		options.MaxFileTransferBytes = defaultFileTransferMax
	}
	if options.IdleTTL <= 0 {
		options.IdleTTL = defaultIdleTTL
	}
	if options.MaxJobTimeout < 0 {
		return nil, fmt.Errorf("%w: invalid maximum Job timeout", ErrInvalidRequest)
	}
	return &DataPlaneService{
		maxJobTimeout: options.MaxJobTimeout,
		bindings:      bindings, jobs: jobs, backends: copyExecutionBackends(backends),
		now: options.Now, newID: options.NewID, maxFileTransferBytes: options.MaxFileTransferBytes,
		maxFileUploadBytes: options.MaxFileUploadBytes,
		idleTTL:            options.IdleTTL,
	}, nil
}

func (s *DataPlaneService) SubmitJob(ctx context.Context, request SubmitJobRequest) (Job, error) {
	request = normalizeSubmitJobRequest(request)
	if err := validateSubmitJobRequest(request); err != nil {
		return Job{}, err
	}
	if s.maxJobTimeout > 0 {
		if request.Timeout == 0 {
			request.Timeout = s.maxJobTimeout
		}
		if request.Timeout > s.maxJobTimeout {
			return Job{}, fmt.Errorf("%w: Job timeout exceeds policy", ErrInvalidRequest)
		}
	}
	if request.WorkDir != "" {
		request.WorkDir, _ = cleanWorkspacePath(request.WorkDir, true)
	}
	binding, err := s.activeBinding(ctx, request.Scope, request.BindingID, true)
	if err != nil {
		return Job{}, err
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
		ID: id, BindingID: binding.ID, BindingGeneration: binding.Generation,
		OrganizationID: binding.OrganizationID, SubjectID: binding.SubjectID,
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
	if _, err := s.authorizedBinding(ctx, scope, job.BindingID); err != nil {
		return Job{}, err
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
	binding, err := s.activeBinding(ctx, scope, bindingID, false)
	if err != nil {
		return RuntimeCapabilities{}, err
	}
	backend, ok := s.backends[binding.Backend]
	if !ok {
		return RuntimeCapabilities{}, fmt.Errorf("%w: %s", ErrBackendUnavailable, binding.Backend)
	}
	capabilities, err := backend.Capabilities(ctx, allocationForBinding(binding))
	if err != nil {
		return RuntimeCapabilities{}, err
	}
	capabilities.MaxUploadBytes = min(capabilities.MaxUploadBytes, s.maxFileUploadBytes)
	capabilities.MaxDownloadBytes = min(capabilities.MaxDownloadBytes, s.maxFileTransferBytes)
	return capabilities, nil
}

func (s *DataPlaneService) UploadFile(ctx context.Context, scope Scope, bindingID string, filePath string, reader io.Reader, size int64) (FileInfo, error) {
	cleanPath, err := cleanWorkspacePath(filePath, false)
	if err != nil {
		return FileInfo{}, err
	}
	if reader == nil || size < 0 || size > s.maxFileUploadBytes {
		return FileInfo{}, fmt.Errorf("%w: upload size is invalid", ErrInvalidRequest)
	}
	binding, err := s.activeBinding(ctx, scope, bindingID, true)
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
	binding, err := s.activeBinding(ctx, scope, bindingID, true)
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

func (s *DataPlaneService) activeBinding(ctx context.Context, scope Scope, bindingID string, touch bool) (Binding, error) {
	binding, err := s.authorizedBinding(ctx, scope, bindingID)
	if err != nil {
		return Binding{}, err
	}
	if binding.Status != BindingStatusActive {
		return Binding{}, fmt.Errorf("%w: binding is %s", ErrInvalidTransition, binding.Status)
	}
	if !touch {
		return binding, nil
	}
	now := s.now().UTC()
	touched, err := s.bindings.TouchBinding(ctx, binding.ID, binding.Generation, now, now.Add(s.idleTTL), now.Add(defaultRetentionTTL))
	if err != nil {
		return Binding{}, err
	}
	return touched, nil
}

type WorkerOptions struct {
	OwnerID            string
	Now                func() time.Time
	ClaimLease         time.Duration
	CancelPollInterval time.Duration
	MaxOutputBytes     int64
	QuarantineJob      func(context.Context, Job) error
	// StartUnknownTimeout bounds how long a start whose outcome is unknown is
	// retried before the job is fenced and failed.
	StartUnknownTimeout time.Duration
}

type Worker struct {
	jobs                JobRepository
	backends            map[string]ExecutionBackend
	ownerID             string
	now                 func() time.Time
	claimLease          time.Duration
	cancelPollInterval  time.Duration
	maxOutputBytes      int64
	quarantineJob       func(context.Context, Job) error
	startUnknownTimeout time.Duration
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
	if options.StartUnknownTimeout <= 0 {
		options.StartUnknownTimeout = 2 * options.ClaimLease
	}
	return &Worker{
		jobs: jobs, backends: copyExecutionBackends(backends), ownerID: strings.TrimSpace(options.OwnerID),
		now: options.Now, claimLease: options.ClaimLease,
		cancelPollInterval: options.CancelPollInterval, maxOutputBytes: options.MaxOutputBytes,
		quarantineJob: options.QuarantineJob, startUnknownTimeout: options.StartUnknownTimeout,
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
	sink := func(chunk OutputChunk) error {
		if len(chunk.Data) == 0 {
			return nil
		}
		chunk.JobID = job.ID
		chunk.Data = append([]byte(nil), chunk.Data...)
		_, err := w.jobs.AppendJobOutput(jobCtx, job.ID, chunk, w.maxOutputBytes, w.now().UTC())
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
	start, err := w.startJob(ctx, jobCtx, job, backend, JobSpec{
		JobID: job.ID, Command: job.Command, Args: append([]string(nil), job.Args...),
		Environment: cloneEnvironment(job.Environment), WorkDir: job.WorkDir,
		PTY: job.PTY, PTYColumns: job.PTYColumns, PTYRows: job.PTYRows, Timeout: job.Timeout,
	}, sink, now.Add(w.claimLease/2))
	if err != nil {
		// The claim is kept for the next worker, whose start attaches to the job
		// if it is running.
		return true, err
	}
	if start.rejected != nil {
		return true, w.finishJob(context.WithoutCancel(ctx), job, JobResult{ExitCode: -1, Error: start.rejected.Error()})
	}
	if start.unresolved != nil {
		// The job may be running where it can no longer be reached or stopped:
		// fence its compute before the job settles.
		return true, w.settleJob(ctx, job, JobResult{
			ExitCode: -1, Error: start.unresolved.Error(), TerminationFailed: true,
		})
	}
	running := start.running

	waitResult := make(chan workerResult, 1)
	go func() {
		result, waitErr := running.Wait(jobCtx)
		waitResult <- workerResult{result: result, err: waitErr}
	}()
	ticker := time.NewTicker(w.cancelPollInterval)
	defer ticker.Stop()
	cancelSent := false
	nextRenewal := start.nextRenewal
	if start.cancelRequested {
		// Cancellation was requested while the start was unresolved.
		cancelSent = true
		_ = running.Cancel(jobCtx)
	}
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
			return true, w.settleJob(ctx, job, result.result)
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

// jobStart is how a job's start resolved: attached to its running job,
// rejected by the backend, or still unknown when the start window closed.
type jobStart struct {
	running         RunningJob
	rejected        error
	unresolved      error
	cancelRequested bool
	nextRenewal     time.Time
}

// startJob starts the job on its backend. A start whose outcome is unknown
// (the request may have reached the backend, but no answer came back) is not a
// failure: the job may be running. The backend start is idempotent by job ID,
// so it is retried to attach to the job while the claim is kept, and a
// cancellation requested meanwhile is reported for the caller to send once
// attached. An error is returned only when the claim must be left as is.
func (w *Worker) startJob(ctx context.Context, jobCtx context.Context, job Job, backend ExecutionBackend, spec JobSpec, sink func(OutputChunk) error, nextRenewal time.Time) (jobStart, error) {
	start := jobStart{nextRenewal: nextRenewal}
	deadline := w.now().UTC().Add(w.startUnknownTimeout)
	delay := w.cancelPollInterval
	for {
		running, err := backend.StartJob(jobCtx, allocationForJob(job), spec, sink)
		switch {
		case err == nil:
			start.running = running
			return start, nil
		case !errors.Is(err, ErrJobStartUnknown):
			start.rejected = err
			return start, nil
		case !w.now().UTC().Before(deadline):
			start.unresolved = err
			return start, nil
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return jobStart{}, ctx.Err()
		case <-timer.C:
		}
		delay = min(delay*2, max(w.claimLease/4, w.cancelPollInterval))
		now := w.now().UTC()
		if !now.Before(start.nextRenewal) {
			if err := w.jobs.RenewJobClaim(jobCtx, job.ID, w.ownerID, now, now.Add(w.claimLease)); err != nil {
				return jobStart{}, err
			}
			start.nextRenewal = now.Add(w.claimLease / 2)
		}
		if !start.cancelRequested {
			requested, err := w.jobs.JobCancellationRequested(jobCtx, job.ID)
			if err != nil {
				return jobStart{}, err
			}
			start.cancelRequested = requested
		}
	}
}

// settleJob records a job's result, quarantining its compute first when the
// job could not be confirmed stopped.
func (w *Worker) settleJob(ctx context.Context, job Job, result JobResult) error {
	var quarantineErr error
	if result.TerminationFailed && w.quarantineJob != nil {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		quarantineErr = w.quarantineJob(cleanupCtx, job)
		cleanupCancel()
	}
	finishErr := w.finishJob(context.WithoutCancel(ctx), job, result)
	return errors.Join(finishErr, quarantineErr)
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
		if strings.HasPrefix(strings.ToUpper(key), "ZGI_PLATFORM_") {
			return fmt.Errorf("%w: platform grants cannot be persisted in job environment", ErrInvalidRequest)
		}
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
