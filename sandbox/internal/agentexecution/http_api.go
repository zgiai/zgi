package agentexecution

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const agentRuntimePathPrefix = "/v1/agent-runtime/"

type ControlPlane interface {
	CreateBinding(context.Context, CreateBindingRequest) (Binding, error)
	GetBinding(context.Context, Scope, string) (Binding, error)
	SuspendBinding(context.Context, Scope, string, int64) (Binding, error)
	ResumeBinding(context.Context, Scope, string, int64) (Binding, error)
	DestroyBinding(context.Context, Scope, string, int64) (Binding, error)
}

type BindingIdempotencyLookup interface {
	GetBindingByIdempotency(context.Context, Scope, string) (Binding, error)
}

type DataPlane interface {
	SubmitJob(context.Context, SubmitJobRequest) (Job, error)
	GetJob(context.Context, Scope, string) (Job, error)
	CancelJob(context.Context, Scope, string, int64) (Job, error)
	JobOutput(context.Context, Scope, string, int64, int) ([]OutputChunk, error)
	Capabilities(context.Context, Scope, string) (RuntimeCapabilities, error)
	UploadFile(context.Context, Scope, string, string, io.Reader, int64) (FileInfo, error)
	DownloadFile(context.Context, Scope, string, string) (io.ReadCloser, FileInfo, error)
}

type HTTPAPIOptions struct {
	APIKey       string
	MaxBodyBytes int64
}

type HTTPAPI struct {
	control      ControlPlane
	data         DataPlane
	apiKey       string
	maxBodyBytes int64
}

func NewHTTPAPI(control ControlPlane, data DataPlane, options HTTPAPIOptions) (*HTTPAPI, error) {
	if control == nil || data == nil {
		return nil, fmt.Errorf("%w: control and data planes are required", ErrInvalidRequest)
	}
	options.APIKey = strings.TrimSpace(options.APIKey)
	if options.APIKey == "" {
		return nil, fmt.Errorf("%w: a dedicated Agent Runtime API key is required", ErrInvalidRequest)
	}
	if options.MaxBodyBytes <= 0 {
		options.MaxBodyBytes = 1 * 1024 * 1024
	}
	return &HTTPAPI{control: control, data: data, apiKey: options.APIKey, maxBodyBytes: options.MaxBodyBytes}, nil
}

func (a *HTTPAPI) Register(mux *http.ServeMux) error {
	if mux == nil {
		return fmt.Errorf("%w: HTTP mux is required", ErrInvalidRequest)
	}
	mux.Handle(agentRuntimePathPrefix, a)
	return nil
}

func (a *HTTPAPI) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	if !a.authorized(request) {
		writeAgentRuntimeError(w, http.StatusUnauthorized, "unauthorized", "Agent Runtime credentials are required")
		return
	}
	scope, ok := requestScope(request)
	if !ok {
		writeAgentRuntimeError(w, http.StatusBadRequest, "invalid_scope", "organization and subject headers are required")
		return
	}
	pathValue := strings.TrimPrefix(request.URL.Path, agentRuntimePathPrefix)
	segments := splitAgentRuntimePath(pathValue)
	if len(segments) == 0 {
		http.NotFound(w, request)
		return
	}
	switch segments[0] {
	case "bindings":
		a.handleBindings(w, request, scope, segments[1:])
	case "jobs":
		a.handleJobs(w, request, scope, segments[1:])
	default:
		http.NotFound(w, request)
	}
}

func (a *HTTPAPI) handleBindings(w http.ResponseWriter, request *http.Request, scope Scope, segments []string) {
	if len(segments) == 0 {
		if request.Method != http.MethodPost {
			writeMethodNotAllowed(w, http.MethodPost)
			return
		}
		var body createBindingBody
		if !a.decodeJSON(w, request, &body) {
			return
		}
		binding, err := a.control.CreateBinding(request.Context(), CreateBindingRequest{
			Scope: scope, AgentID: body.AgentID, ConversationID: body.ConversationID,
			ProfileID: body.ProfileID, NetworkPolicy: body.NetworkPolicy, HomeSnapshotID: body.HomeSnapshotID,
			Backend: body.Backend, IdempotencyKey: body.IdempotencyKey,
		})
		a.writeResult(w, http.StatusCreated, bindingResponseFrom(binding), err)
		return
	}
	if len(segments) == 1 && segments[0] == "by-idempotency" {
		if request.Method != http.MethodGet {
			writeMethodNotAllowed(w, http.MethodGet)
			return
		}
		lookup, ok := a.control.(BindingIdempotencyLookup)
		if !ok {
			a.writeResult(w, http.StatusOK, nil, ErrBackendUnavailable)
			return
		}
		binding, err := lookup.GetBindingByIdempotency(request.Context(), scope, request.URL.Query().Get("key"))
		a.writeResult(w, http.StatusOK, bindingResponseFrom(binding), err)
		return
	}
	bindingID := segments[0]
	if len(segments) == 1 {
		switch request.Method {
		case http.MethodGet:
			binding, err := a.control.GetBinding(request.Context(), scope, bindingID)
			a.writeResult(w, http.StatusOK, bindingResponseFrom(binding), err)
		case http.MethodDelete:
			generation, ok := parseRequiredInt64(w, request.URL.Query().Get("generation"), "generation")
			if !ok {
				return
			}
			binding, err := a.control.DestroyBinding(request.Context(), scope, bindingID, generation)
			a.writeResult(w, http.StatusOK, bindingResponseFrom(binding), err)
		default:
			writeMethodNotAllowed(w, http.MethodGet, http.MethodDelete)
		}
		return
	}
	if len(segments) != 2 {
		http.NotFound(w, request)
		return
	}
	switch segments[1] {
	case "suspend":
		if request.Method != http.MethodPost {
			writeMethodNotAllowed(w, http.MethodPost)
			return
		}
		var body generationBody
		if !a.decodeJSON(w, request, &body) {
			return
		}
		binding, err := a.control.SuspendBinding(request.Context(), scope, bindingID, body.Generation)
		a.writeResult(w, http.StatusOK, bindingResponseFrom(binding), err)
	case "resume":
		if request.Method != http.MethodPost {
			writeMethodNotAllowed(w, http.MethodPost)
			return
		}
		var body generationBody
		if !a.decodeJSON(w, request, &body) {
			return
		}
		binding, err := a.control.ResumeBinding(request.Context(), scope, bindingID, body.Generation)
		a.writeResult(w, http.StatusOK, bindingResponseFrom(binding), err)
	case "capabilities":
		if request.Method != http.MethodGet {
			writeMethodNotAllowed(w, http.MethodGet)
			return
		}
		capabilities, err := a.data.Capabilities(request.Context(), scope, bindingID)
		a.writeResult(w, http.StatusOK, capabilities, err)
	case "jobs":
		if request.Method != http.MethodPost {
			writeMethodNotAllowed(w, http.MethodPost)
			return
		}
		var body submitJobBody
		if !a.decodeJSON(w, request, &body) {
			return
		}
		job, err := a.data.SubmitJob(request.Context(), SubmitJobRequest{
			Scope: scope, BindingID: bindingID, IdempotencyKey: body.IdempotencyKey,
			Command: body.Command, Args: body.Args, Environment: body.Environment,
			WorkDir: body.WorkDir, PTY: body.PTY, PTYColumns: body.PTYColumns,
			PTYRows: body.PTYRows, Timeout: time.Duration(body.TimeoutMS) * time.Millisecond,
		})
		a.writeResult(w, http.StatusAccepted, jobResponseFrom(job), err)
	case "files":
		a.handleFiles(w, request, scope, bindingID)
	default:
		http.NotFound(w, request)
	}
}

func (a *HTTPAPI) handleJobs(w http.ResponseWriter, request *http.Request, scope Scope, segments []string) {
	if len(segments) == 0 || len(segments) > 2 {
		http.NotFound(w, request)
		return
	}
	jobID := segments[0]
	if len(segments) == 1 {
		if request.Method != http.MethodGet {
			writeMethodNotAllowed(w, http.MethodGet)
			return
		}
		job, err := a.data.GetJob(request.Context(), scope, jobID)
		a.writeResult(w, http.StatusOK, jobResponseFrom(job), err)
		return
	}
	switch segments[1] {
	case "cancel":
		if request.Method != http.MethodPost {
			writeMethodNotAllowed(w, http.MethodPost)
			return
		}
		var body generationBody
		if !a.decodeJSON(w, request, &body) {
			return
		}
		job, err := a.data.CancelJob(request.Context(), scope, jobID, body.Generation)
		a.writeResult(w, http.StatusOK, jobResponseFrom(job), err)
	case "output":
		if request.Method != http.MethodGet {
			writeMethodNotAllowed(w, http.MethodGet)
			return
		}
		after, ok := parseOptionalInt64(w, request.URL.Query().Get("after"), -1, "after")
		if !ok {
			return
		}
		limit, ok := parseOptionalInt(w, request.URL.Query().Get("limit"), 1000, "limit")
		if !ok {
			return
		}
		chunks, err := a.data.JobOutput(request.Context(), scope, jobID, after, limit)
		a.writeResult(w, http.StatusOK, outputResponseFrom(chunks), err)
	default:
		http.NotFound(w, request)
	}
}

func (a *HTTPAPI) handleFiles(w http.ResponseWriter, request *http.Request, scope Scope, bindingID string) {
	filePath := request.URL.Query().Get("path")
	switch request.Method {
	case http.MethodPut:
		if request.ContentLength < 0 {
			writeAgentRuntimeError(w, http.StatusLengthRequired, "content_length_required", "Content-Length is required")
			return
		}
		if request.ContentLength > defaultFileTransferMax {
			writeAgentRuntimeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "file exceeds the transfer limit")
			return
		}
		info, err := a.data.UploadFile(request.Context(), scope, bindingID, filePath, request.Body, request.ContentLength)
		a.writeResult(w, http.StatusCreated, info, err)
	case http.MethodGet:
		reader, info, err := a.data.DownloadFile(request.Context(), scope, bindingID, filePath)
		if err != nil {
			a.writeResult(w, http.StatusOK, nil, err)
			return
		}
		defer reader.Close()
		w.Header().Set("Content-Type", defaultContentType(info.ContentType))
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, io.LimitReader(reader, info.Size))
	default:
		writeMethodNotAllowed(w, http.MethodGet, http.MethodPut)
	}
}

func (a *HTTPAPI) decodeJSON(w http.ResponseWriter, request *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, request.Body, a.maxBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeAgentRuntimeError(w, http.StatusBadRequest, "invalid_json", "request body is invalid")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeAgentRuntimeError(w, http.StatusBadRequest, "invalid_json", "request body must contain one JSON object")
		return false
	}
	return true
}

func (a *HTTPAPI) authorized(request *http.Request) bool {
	value := strings.TrimSpace(request.Header.Get("Authorization"))
	if !strings.HasPrefix(value, "Bearer ") {
		return false
	}
	provided := strings.TrimSpace(strings.TrimPrefix(value, "Bearer "))
	if len(provided) != len(a.apiKey) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(a.apiKey)) == 1
}

func (a *HTTPAPI) writeResult(w http.ResponseWriter, status int, result any, err error) {
	if err == nil {
		writeAgentRuntimeJSON(w, status, map[string]any{"data": result})
		return
	}
	httpStatus, code, message := agentRuntimeErrorResponse(err)
	writeAgentRuntimeError(w, httpStatus, code, message)
}

func requestScope(request *http.Request) (Scope, bool) {
	scope := Scope{
		OrganizationID: strings.TrimSpace(request.Header.Get("X-ZGI-Organization-ID")),
		SubjectID:      strings.TrimSpace(request.Header.Get("X-ZGI-Subject-ID")),
	}
	return scope, scope.OrganizationID != "" && scope.SubjectID != ""
}

func splitAgentRuntimePath(value string) []string {
	parts := strings.Split(strings.Trim(value, "/"), "/")
	result := make([]string, 0, len(parts))
	for _, item := range parts {
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func agentRuntimeErrorResponse(err error) (int, string, string) {
	switch {
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound, "not_found", "resource not found"
	case errors.Is(err, ErrInvalidRequest), errors.Is(err, ErrInvalidPath):
		return http.StatusBadRequest, "invalid_request", "request is invalid"
	case errors.Is(err, ErrIdempotencyConflict):
		return http.StatusConflict, "idempotency_conflict", "idempotency key was reused with a different request"
	case errors.Is(err, ErrGenerationConflict), errors.Is(err, ErrInvalidTransition):
		return http.StatusConflict, "state_conflict", "resource state changed; refresh and retry"
	case errors.Is(err, ErrOutputLimit):
		return http.StatusRequestEntityTooLarge, "limit_exceeded", "configured output or file limit was exceeded"
	case errors.Is(err, ErrBackendUnavailable):
		return http.StatusServiceUnavailable, "backend_unavailable", "execution backend is unavailable"
	default:
		return http.StatusInternalServerError, "internal_error", "Agent Runtime operation failed"
	}
}

func writeAgentRuntimeError(w http.ResponseWriter, status int, code string, message string) {
	writeAgentRuntimeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func writeAgentRuntimeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeMethodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	writeAgentRuntimeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method is not allowed")
}

func parseRequiredInt64(w http.ResponseWriter, value string, field string) (int64, bool) {
	if strings.TrimSpace(value) == "" {
		writeAgentRuntimeError(w, http.StatusBadRequest, "invalid_request", field+" is required")
		return 0, false
	}
	return parseOptionalInt64(w, value, 0, field)
}

func parseOptionalInt64(w http.ResponseWriter, value string, fallback int64, field string) (int64, bool) {
	if strings.TrimSpace(value) == "" {
		return fallback, true
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		writeAgentRuntimeError(w, http.StatusBadRequest, "invalid_request", field+" must be an integer")
		return 0, false
	}
	return parsed, true
}

func parseOptionalInt(w http.ResponseWriter, value string, fallback int, field string) (int, bool) {
	parsed, ok := parseOptionalInt64(w, value, int64(fallback), field)
	return int(parsed), ok
}

func defaultContentType(value string) string {
	if strings.TrimSpace(value) == "" {
		return "application/octet-stream"
	}
	return value
}

type createBindingBody struct {
	AgentID        string `json:"agent_id"`
	ConversationID string `json:"conversation_id"`
	ProfileID      string `json:"profile_id"`
	NetworkPolicy  string `json:"network_policy,omitempty"`
	HomeSnapshotID string `json:"home_snapshot_id,omitempty"`
	Backend        string `json:"backend"`
	IdempotencyKey string `json:"idempotency_key"`
}

type generationBody struct {
	Generation int64 `json:"generation"`
}

type submitJobBody struct {
	IdempotencyKey string            `json:"idempotency_key"`
	Command        string            `json:"command"`
	Args           []string          `json:"args,omitempty"`
	Environment    map[string]string `json:"environment,omitempty"`
	WorkDir        string            `json:"work_dir,omitempty"`
	PTY            bool              `json:"pty"`
	PTYColumns     uint16            `json:"pty_columns,omitempty"`
	PTYRows        uint16            `json:"pty_rows,omitempty"`
	TimeoutMS      int64             `json:"timeout_ms,omitempty"`
}

type bindingResponse struct {
	ID                 string        `json:"id"`
	AgentID            string        `json:"agent_id"`
	ConversationID     string        `json:"conversation_id"`
	ProfileID          string        `json:"profile_id"`
	NetworkPolicy      string        `json:"network_policy"`
	NetworkMode        string        `json:"network_mode"`
	HomeSnapshotID     string        `json:"home_snapshot_id,omitempty"`
	WorkspaceID        string        `json:"workspace_id"`
	Backend            string        `json:"backend"`
	Status             BindingStatus `json:"status"`
	Generation         int64         `json:"generation"`
	CreatedAt          time.Time     `json:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at"`
	ExpiresAt          time.Time     `json:"expires_at"`
	RetentionExpiresAt time.Time     `json:"retention_expires_at"`
}

func bindingResponseFrom(binding Binding) bindingResponse {
	return bindingResponse{
		ID: binding.ID, AgentID: binding.AgentID, ConversationID: binding.ConversationID,
		ProfileID: binding.ProfileID, NetworkPolicy: binding.NetworkPolicy, NetworkMode: binding.NetworkMode,
		HomeSnapshotID: binding.HomeSnapshotID, WorkspaceID: binding.WorkspaceID, Backend: binding.Backend,
		Status: binding.Status, Generation: binding.Generation, CreatedAt: binding.CreatedAt,
		UpdatedAt: binding.UpdatedAt, ExpiresAt: binding.ExpiresAt,
		RetentionExpiresAt: binding.RetentionExpiresAt,
	}
}

type jobResponse struct {
	ID              string     `json:"id"`
	BindingID       string     `json:"binding_id"`
	Status          JobStatus  `json:"status"`
	Generation      int64      `json:"generation"`
	ExitCode        *int       `json:"exit_code,omitempty"`
	FailureReason   string     `json:"failure_reason,omitempty"`
	CancelRequested bool       `json:"cancel_requested"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
}

func jobResponseFrom(job Job) jobResponse {
	return jobResponse{
		ID: job.ID, BindingID: job.BindingID, Status: job.Status, Generation: job.Generation,
		ExitCode: job.ExitCode, FailureReason: job.FailureReason, CancelRequested: job.CancelRequested,
		CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt, StartedAt: job.StartedAt, FinishedAt: job.FinishedAt,
	}
}

type outputChunkResponse struct {
	Sequence  int64        `json:"sequence"`
	Stream    OutputStream `json:"stream"`
	Data      []byte       `json:"data"`
	CreatedAt time.Time    `json:"created_at"`
}

func outputResponseFrom(chunks []OutputChunk) []outputChunkResponse {
	result := make([]outputChunkResponse, 0, len(chunks))
	for _, chunk := range chunks {
		result = append(result, outputChunkResponse{
			Sequence: chunk.Sequence, Stream: chunk.Stream,
			Data: append([]byte(nil), chunk.Data...), CreatedAt: chunk.CreatedAt,
		})
	}
	return result
}
