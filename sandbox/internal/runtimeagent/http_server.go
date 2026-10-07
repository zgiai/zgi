package runtimeagent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

const runtimeAgentPathPrefix = "/v1/"

type HTTPServerOptions struct {
	Token                  string
	MaxRequestBytes        int64
	MaxBufferedOutputBytes int64
}

type HTTPServer struct {
	backend                agentexecution.ExecutionBackend
	token                  string
	maxRequestBytes        int64
	maxBufferedOutputBytes int64
	ctx                    context.Context
	cancel                 context.CancelFunc
	mu                     sync.Mutex
	jobs                   map[string]*httpServerJob
}

type httpServerJob struct {
	mu          sync.Mutex
	specHash    string
	running     agentexecution.RunningJob
	chunks      []agentexecution.OutputChunk
	outputBytes int64
	truncated   bool
	done        bool
	result      agentexecution.JobResult
}

func NewHTTPServer(backend agentexecution.ExecutionBackend, options HTTPServerOptions) (*HTTPServer, error) {
	if backend == nil {
		return nil, fmt.Errorf("%w: runtime backend is required", agentexecution.ErrInvalidRequest)
	}
	options.Token = strings.TrimSpace(options.Token)
	if options.Token == "" {
		return nil, fmt.Errorf("%w: runtime agent token is required", agentexecution.ErrInvalidRequest)
	}
	if options.MaxRequestBytes <= 0 {
		options.MaxRequestBytes = 1 * 1024 * 1024
	}
	if options.MaxBufferedOutputBytes <= 0 {
		options.MaxBufferedOutputBytes = 8 * 1024 * 1024
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &HTTPServer{
		backend: backend, token: options.Token, maxRequestBytes: options.MaxRequestBytes,
		maxBufferedOutputBytes: options.MaxBufferedOutputBytes,
		ctx:                    ctx, cancel: cancel, jobs: make(map[string]*httpServerJob),
	}, nil
}

func (s *HTTPServer) Close() error {
	s.cancel()
	s.mu.Lock()
	jobs := make([]agentexecution.RunningJob, 0, len(s.jobs))
	for _, job := range s.jobs {
		job.mu.Lock()
		if job.running != nil && !job.done {
			jobs = append(jobs, job.running)
		}
		job.mu.Unlock()
	}
	s.mu.Unlock()
	for _, job := range jobs {
		_ = job.Cancel(context.Background())
	}
	return nil
}

func (s *HTTPServer) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	if !s.authorized(request) {
		writeRuntimeAgentError(w, http.StatusUnauthorized, "unauthorized", "runtime agent credentials are required")
		return
	}
	segments := splitRuntimeAgentPath(strings.TrimPrefix(request.URL.Path, runtimeAgentPathPrefix))
	if len(segments) == 0 {
		http.NotFound(w, request)
		return
	}
	switch segments[0] {
	case "capabilities":
		s.handleCapabilities(w, request, segments[1:])
	case "jobs":
		s.handleJobs(w, request, segments[1:])
	case "files":
		s.handleFiles(w, request, segments[1:])
	case "workspace":
		s.handleWorkspace(w, request, segments[1:])
	default:
		http.NotFound(w, request)
	}
}

// ServePackageHTTP is bound only to the mode-0660 package socket inside a
// package-install runtime. It intentionally has no bearer-token endpoint and
// accepts only the narrow apt request validated by Backend.InstallSystemPackages.
func (s *HTTPServer) ServePackageHTTP(w http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost || request.URL.Path != "/v1/package-install" {
		writeRuntimeAgentMethodNotAllowed(w, http.MethodPost)
		return
	}
	installer, ok := s.backend.(interface {
		InstallSystemPackages(context.Context, []string) (PackageInstallResult, error)
	})
	if !ok {
		writeRuntimeAgentError(w, http.StatusNotImplemented, "backend_unavailable", "system package installation is unavailable")
		return
	}
	var body struct {
		Args []string `json:"args"`
	}
	if !s.decodeJSON(w, request, &body) {
		return
	}
	ctx, cancel := context.WithCancel(request.Context())
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	defer cancel()
	result, err := installer.InstallSystemPackages(ctx, body.Args)
	s.writeResult(w, http.StatusOK, result, err)
}

func (s *HTTPServer) handleWorkspace(w http.ResponseWriter, request *http.Request, segments []string) {
	if len(segments) != 1 || segments[0] != "archive" {
		http.NotFound(w, request)
		return
	}
	archiver, ok := s.backend.(agentexecution.WorkspaceArchiver)
	if !ok {
		writeRuntimeAgentError(w, http.StatusNotImplemented, "backend_unavailable", "workspace archive is unavailable")
		return
	}
	switch request.Method {
	case http.MethodGet:
		archive, err := archiver.ExportWorkspace(request.Context(), agentexecution.Allocation{})
		if err != nil {
			s.writeResult(w, http.StatusOK, nil, err)
			return
		}
		defer archive.Close()
		w.Header().Set("Content-Type", "application/x-tar")
		w.WriteHeader(http.StatusOK)
		digest := sha256.New()
		if _, err := io.Copy(io.MultiWriter(w, digest), archive); err != nil {
			return
		}
		// The body footer survives intermediaries that discard HTTP trailers.
		// A truncated or corrupted archive never becomes a valid checkpoint.
		if _, err := io.WriteString(w, workspaceArchiveFooterMagic); err != nil {
			return
		}
		_, _ = w.Write(digest.Sum(nil))
	case http.MethodPut:
		const maximum = maxWorkspaceArchiveBytes + int64(maxWorkspaceEntries)*4096
		limited := http.MaxBytesReader(w, request.Body, maximum)
		defer limited.Close()
		s.writeResult(w, http.StatusOK, struct{}{}, archiver.ImportWorkspace(request.Context(), agentexecution.Allocation{}, limited))
	default:
		writeRuntimeAgentMethodNotAllowed(w, http.MethodGet, http.MethodPut)
	}
}

func (s *HTTPServer) handleCapabilities(w http.ResponseWriter, request *http.Request, segments []string) {
	if len(segments) != 0 {
		http.NotFound(w, request)
		return
	}
	if request.Method != http.MethodGet {
		writeRuntimeAgentMethodNotAllowed(w, http.MethodGet)
		return
	}
	capabilities, err := s.backend.Capabilities(request.Context(), agentexecution.Allocation{})
	s.writeResult(w, http.StatusOK, capabilities, err)
}

func (s *HTTPServer) handleJobs(w http.ResponseWriter, request *http.Request, segments []string) {
	if len(segments) == 0 {
		if request.Method != http.MethodPost {
			writeRuntimeAgentMethodNotAllowed(w, http.MethodPost)
			return
		}
		s.startJob(w, request)
		return
	}
	if len(segments) > 2 {
		http.NotFound(w, request)
		return
	}
	job := s.job(segments[0])
	if job == nil {
		writeRuntimeAgentError(w, http.StatusNotFound, "not_found", "job not found")
		return
	}
	if len(segments) == 1 {
		if request.Method != http.MethodGet {
			writeRuntimeAgentMethodNotAllowed(w, http.MethodGet)
			return
		}
		writeRuntimeAgentJSON(w, http.StatusOK, map[string]any{"data": job.snapshot()})
		return
	}
	switch segments[1] {
	case "output":
		s.jobOutput(w, request, job)
	case "cancel":
		s.cancelJob(w, request, job)
	case "resize":
		s.resizeJob(w, request, job)
	default:
		http.NotFound(w, request)
	}
}

func (s *HTTPServer) startJob(w http.ResponseWriter, request *http.Request) {
	var body runtimeJobRequest
	if !s.decodeJSON(w, request, &body) {
		return
	}
	spec := body.spec()
	hash, err := hashRuntimeJobRequest(body)
	if err != nil {
		s.writeResult(w, http.StatusAccepted, nil, err)
		return
	}
	s.mu.Lock()
	if existing := s.jobs[spec.JobID]; existing != nil {
		s.mu.Unlock()
		if existing.specHash != hash {
			s.writeResult(w, http.StatusAccepted, nil, agentexecution.ErrIdempotencyConflict)
			return
		}
		writeRuntimeAgentJSON(w, http.StatusAccepted, map[string]any{"data": existing.snapshot()})
		return
	}
	job := &httpServerJob{specHash: hash, chunks: make([]agentexecution.OutputChunk, 0)}
	s.jobs[spec.JobID] = job
	s.mu.Unlock()

	running, err := s.backend.StartJob(s.ctx, agentexecution.Allocation{}, spec, func(chunk agentexecution.OutputChunk) error {
		return job.appendOutput(chunk, s.maxBufferedOutputBytes)
	})
	if err != nil {
		s.mu.Lock()
		delete(s.jobs, spec.JobID)
		s.mu.Unlock()
		s.writeResult(w, http.StatusAccepted, nil, err)
		return
	}
	job.mu.Lock()
	job.running = running
	job.mu.Unlock()
	go job.wait(s.ctx)
	writeRuntimeAgentJSON(w, http.StatusAccepted, map[string]any{"data": job.snapshot()})
}

func (s *HTTPServer) jobOutput(w http.ResponseWriter, request *http.Request, job *httpServerJob) {
	if request.Method != http.MethodGet {
		writeRuntimeAgentMethodNotAllowed(w, http.MethodGet)
		return
	}
	after, err := strconv.ParseInt(defaultStringValue(request.URL.Query().Get("after"), "-1"), 10, 64)
	if err != nil {
		writeRuntimeAgentError(w, http.StatusBadRequest, "invalid_request", "after must be an integer")
		return
	}
	writeRuntimeAgentJSON(w, http.StatusOK, map[string]any{"data": job.outputAfter(after)})
}

func (s *HTTPServer) cancelJob(w http.ResponseWriter, request *http.Request, job *httpServerJob) {
	if request.Method != http.MethodPost {
		writeRuntimeAgentMethodNotAllowed(w, http.MethodPost)
		return
	}
	job.mu.Lock()
	running := job.running
	done := job.done
	job.mu.Unlock()
	if running != nil && !done {
		if err := running.Cancel(request.Context()); err != nil {
			s.writeResult(w, http.StatusOK, nil, err)
			return
		}
	}
	writeRuntimeAgentJSON(w, http.StatusOK, map[string]any{"data": job.snapshot()})
}

func (s *HTTPServer) resizeJob(w http.ResponseWriter, request *http.Request, job *httpServerJob) {
	if request.Method != http.MethodPost {
		writeRuntimeAgentMethodNotAllowed(w, http.MethodPost)
		return
	}
	var body runtimeResizeRequest
	if !s.decodeJSON(w, request, &body) {
		return
	}
	job.mu.Lock()
	running := job.running
	done := job.done
	job.mu.Unlock()
	if running == nil || done {
		s.writeResult(w, http.StatusOK, nil, agentexecution.ErrInvalidTransition)
		return
	}
	err := running.ResizePTY(request.Context(), body.Columns, body.Rows)
	s.writeResult(w, http.StatusOK, job.snapshot(), err)
}

func (s *HTTPServer) handleFiles(w http.ResponseWriter, request *http.Request, segments []string) {
	if len(segments) != 0 {
		http.NotFound(w, request)
		return
	}
	filePath := request.URL.Query().Get("path")
	switch request.Method {
	case http.MethodPut:
		if request.ContentLength < 0 {
			writeRuntimeAgentError(w, http.StatusLengthRequired, "content_length_required", "Content-Length is required")
			return
		}
		info, err := s.backend.UploadFile(request.Context(), agentexecution.Allocation{}, filePath, request.Body, request.ContentLength)
		s.writeResult(w, http.StatusCreated, info, err)
	case http.MethodGet:
		reader, info, err := s.backend.DownloadFile(request.Context(), agentexecution.Allocation{}, filePath)
		if err != nil {
			s.writeResult(w, http.StatusOK, nil, err)
			return
		}
		defer reader.Close()
		w.Header().Set("Content-Type", defaultRuntimeContentType(info.ContentType))
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, io.LimitReader(reader, info.Size))
	default:
		writeRuntimeAgentMethodNotAllowed(w, http.MethodGet, http.MethodPut)
	}
}

func (s *HTTPServer) decodeJSON(w http.ResponseWriter, request *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, request.Body, s.maxRequestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeRuntimeAgentError(w, http.StatusBadRequest, "invalid_json", "request body is invalid")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeRuntimeAgentError(w, http.StatusBadRequest, "invalid_json", "request body must contain one JSON object")
		return false
	}
	return true
}

func (s *HTTPServer) authorized(request *http.Request) bool {
	value := strings.TrimSpace(request.Header.Get("Authorization"))
	if !strings.HasPrefix(value, "Bearer ") {
		return false
	}
	provided := strings.TrimSpace(strings.TrimPrefix(value, "Bearer "))
	if len(provided) != len(s.token) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(s.token)) == 1
}

func (s *HTTPServer) job(id string) *httpServerJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.jobs[id]
}

func (s *HTTPServer) writeResult(w http.ResponseWriter, status int, value any, err error) {
	if err == nil {
		writeRuntimeAgentJSON(w, status, map[string]any{"data": value})
		return
	}
	httpStatus, code, message := runtimeAgentErrorResponse(err)
	writeRuntimeAgentError(w, httpStatus, code, message)
}

func (j *httpServerJob) appendOutput(chunk agentexecution.OutputChunk, maximum int64) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, existing := range j.chunks {
		if existing.Sequence == chunk.Sequence {
			if existing.Stream != chunk.Stream || !bytes.Equal(existing.Data, chunk.Data) {
				return agentexecution.ErrIdempotencyConflict
			}
			return nil
		}
	}
	if j.outputBytes+int64(len(chunk.Data)) > maximum {
		j.truncated = true
		return agentexecution.ErrOutputLimit
	}
	chunk.Data = append([]byte(nil), chunk.Data...)
	j.chunks = append(j.chunks, chunk)
	j.outputBytes += int64(len(chunk.Data))
	return nil
}

func (j *httpServerJob) wait(ctx context.Context) {
	j.mu.Lock()
	running := j.running
	j.mu.Unlock()
	result, err := running.Wait(ctx)
	if err != nil && result.Error == "" {
		result.Error = err.Error()
	}
	j.mu.Lock()
	j.result = result
	j.done = true
	j.mu.Unlock()
}

func (j *httpServerJob) snapshot() runtimeJobStatus {
	j.mu.Lock()
	defer j.mu.Unlock()
	status := runtimeJobStatus{Done: j.done, Truncated: j.truncated, NextSequence: int64(len(j.chunks))}
	if j.done {
		result := j.result
		status.Result = &result
	}
	return status
}

func (j *httpServerJob) outputAfter(after int64) []agentexecution.OutputChunk {
	j.mu.Lock()
	defer j.mu.Unlock()
	result := make([]agentexecution.OutputChunk, 0)
	for _, chunk := range j.chunks {
		if chunk.Sequence <= after {
			continue
		}
		copyChunk := chunk
		copyChunk.Data = append([]byte(nil), chunk.Data...)
		result = append(result, copyChunk)
	}
	sort.Slice(result, func(left int, right int) bool {
		return result[left].Sequence < result[right].Sequence
	})
	return result
}

type runtimeJobRequest struct {
	JobID       string            `json:"job_id"`
	Command     string            `json:"command"`
	Args        []string          `json:"args,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	WorkDir     string            `json:"work_dir,omitempty"`
	PTY         bool              `json:"pty"`
	PTYColumns  uint16            `json:"pty_columns,omitempty"`
	PTYRows     uint16            `json:"pty_rows,omitempty"`
	TimeoutMS   int64             `json:"timeout_ms,omitempty"`
}

func runtimeJobRequestFrom(spec agentexecution.JobSpec) runtimeJobRequest {
	return runtimeJobRequest{
		JobID: spec.JobID, Command: spec.Command, Args: spec.Args, Environment: spec.Environment,
		WorkDir: spec.WorkDir, PTY: spec.PTY, PTYColumns: spec.PTYColumns,
		PTYRows: spec.PTYRows, TimeoutMS: spec.Timeout.Milliseconds(),
	}
}

func (r runtimeJobRequest) spec() agentexecution.JobSpec {
	return agentexecution.JobSpec{
		JobID: r.JobID, Command: r.Command, Args: r.Args, Environment: r.Environment,
		WorkDir: r.WorkDir, PTY: r.PTY, PTYColumns: r.PTYColumns,
		PTYRows: r.PTYRows, Timeout: time.Duration(r.TimeoutMS) * time.Millisecond,
	}
}

type runtimeResizeRequest struct {
	Columns uint16 `json:"columns"`
	Rows    uint16 `json:"rows"`
}

type runtimeJobStatus struct {
	Done         bool                      `json:"done"`
	Truncated    bool                      `json:"truncated"`
	NextSequence int64                     `json:"next_sequence"`
	Result       *agentexecution.JobResult `json:"result,omitempty"`
}

func hashRuntimeJobRequest(request runtimeJobRequest) (string, error) {
	payload, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	value := sha256.Sum256(payload)
	return hex.EncodeToString(value[:]), nil
}

func runtimeAgentErrorResponse(err error) (int, string, string) {
	var recovery *packageRecoveryError
	if errors.As(err, &recovery) {
		return http.StatusServiceUnavailable, "package_recovery_failed", packageRecoveryMessage
	}
	switch {
	case errors.Is(err, agentexecution.ErrNotFound):
		return http.StatusNotFound, "not_found", "resource not found"
	case errors.Is(err, agentexecution.ErrInvalidRequest), errors.Is(err, agentexecution.ErrInvalidPath):
		return http.StatusBadRequest, "invalid_request", "request is invalid"
	case errors.Is(err, agentexecution.ErrIdempotencyConflict):
		return http.StatusConflict, "idempotency_conflict", "job id was reused with a different request"
	case errors.Is(err, agentexecution.ErrInvalidTransition):
		return http.StatusConflict, "state_conflict", "job state does not allow the operation"
	case errors.Is(err, agentexecution.ErrOutputLimit):
		return http.StatusRequestEntityTooLarge, "limit_exceeded", "configured output or file limit was exceeded"
	case errors.Is(err, agentexecution.ErrBackendUnavailable):
		return http.StatusServiceUnavailable, "backend_unavailable", "runtime backend is unavailable"
	default:
		return http.StatusInternalServerError, "internal_error", "runtime agent operation failed"
	}
}

func writeRuntimeAgentError(w http.ResponseWriter, status int, code string, message string) {
	writeRuntimeAgentJSON(w, status, map[string]any{"error": runtimeAgentError{Code: code, Message: message}})
}

func writeRuntimeAgentJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeRuntimeAgentMethodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	writeRuntimeAgentError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method is not allowed")
}

func splitRuntimeAgentPath(value string) []string {
	parts := strings.Split(strings.Trim(value, "/"), "/")
	result := make([]string, 0, len(parts))
	for _, item := range parts {
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func defaultStringValue(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func defaultRuntimeContentType(value string) string {
	if strings.TrimSpace(value) == "" {
		return "application/octet-stream"
	}
	return value
}

type runtimeAgentError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
