package runtimeagent

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

type RemoteOptions struct {
	BaseURL      string
	Token        string
	Headers      http.Header
	HTTPClient   *http.Client
	PollInterval time.Duration
}

type RemoteBackend struct {
	baseURL      string
	token        string
	headers      http.Header
	client       *http.Client
	pollInterval time.Duration
}

// NewUnixRemote connects to a Runtime Agent through a host-visible Unix
// socket. Local isolation providers use this transport so no TCP listener has
// to be exposed from an execution environment.
func NewUnixRemote(socketPath string, token string, pollInterval time.Duration) (*RemoteBackend, error) {
	socketPath = strings.TrimSpace(socketPath)
	if socketPath == "" || !strings.HasPrefix(socketPath, "/") {
		return nil, fmt.Errorf("%w: runtime agent socket path must be absolute", agentexecution.ErrInvalidRequest)
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
	return NewRemote(RemoteOptions{
		BaseURL:      "http://runtime-agent.local",
		Token:        token,
		HTTPClient:   &http.Client{Transport: transport, Timeout: 30 * time.Second},
		PollInterval: pollInterval,
	})
}

var _ agentexecution.ExecutionBackend = (*RemoteBackend)(nil)
var _ agentexecution.WorkspaceArchiver = (*RemoteBackend)(nil)

func NewRemote(options RemoteOptions) (*RemoteBackend, error) {
	options.BaseURL = strings.TrimRight(strings.TrimSpace(options.BaseURL), "/")
	options.Token = strings.TrimSpace(options.Token)
	if options.BaseURL == "" || options.Token == "" {
		return nil, fmt.Errorf("%w: runtime agent URL and token are required", agentexecution.ErrInvalidRequest)
	}
	parsed, err := url.Parse(options.BaseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("%w: runtime agent URL is invalid", agentexecution.ErrInvalidRequest)
	}
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	headers, err := validatedRemoteHeaders(options.Headers)
	if err != nil {
		return nil, err
	}
	if options.PollInterval <= 0 {
		options.PollInterval = 50 * time.Millisecond
	}
	return &RemoteBackend{
		baseURL: options.BaseURL, token: options.Token, headers: headers,
		client: options.HTTPClient, pollInterval: options.PollInterval,
	}, nil
}

func (b *RemoteBackend) Capabilities(ctx context.Context, _ agentexecution.Allocation) (agentexecution.RuntimeCapabilities, error) {
	var capabilities agentexecution.RuntimeCapabilities
	err := b.doJSON(ctx, http.MethodGet, "/v1/capabilities", nil, &capabilities)
	return capabilities, err
}

func (b *RemoteBackend) StartJob(ctx context.Context, _ agentexecution.Allocation, spec agentexecution.JobSpec, sink func(agentexecution.OutputChunk) error) (agentexecution.RunningJob, error) {
	if sink == nil {
		return nil, fmt.Errorf("%w: output sink is required", agentexecution.ErrInvalidRequest)
	}
	request := runtimeJobRequestFrom(spec)
	var status runtimeJobStatus
	if err := b.doJSON(ctx, http.MethodPost, "/v1/jobs", request, &status); err != nil {
		return nil, err
	}
	return &remoteRunningJob{backend: b, jobID: spec.JobID, sink: sink, nextSequence: -1}, nil
}

func (b *RemoteBackend) UploadFile(ctx context.Context, _ agentexecution.Allocation, filePath string, reader io.Reader, size int64) (agentexecution.FileInfo, error) {
	if reader == nil || size < 0 {
		return agentexecution.FileInfo{}, agentexecution.ErrInvalidRequest
	}
	target := "/v1/files?" + url.Values{"path": []string{filePath}}.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, b.baseURL+target, reader)
	if err != nil {
		return agentexecution.FileInfo{}, err
	}
	b.applyHeaders(request)
	request.ContentLength = size
	response, err := b.client.Do(request)
	if err != nil {
		return agentexecution.FileInfo{}, err
	}
	defer response.Body.Close()
	var info agentexecution.FileInfo
	if err := decodeRuntimeResponse(response, &info); err != nil {
		return agentexecution.FileInfo{}, err
	}
	return info, nil
}

func (b *RemoteBackend) DownloadFile(ctx context.Context, _ agentexecution.Allocation, filePath string) (io.ReadCloser, agentexecution.FileInfo, error) {
	target := "/v1/files?" + url.Values{"path": []string{filePath}}.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, b.baseURL+target, nil)
	if err != nil {
		return nil, agentexecution.FileInfo{}, err
	}
	b.applyHeaders(request)
	response, err := b.client.Do(request)
	if err != nil {
		return nil, agentexecution.FileInfo{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer response.Body.Close()
		return nil, agentexecution.FileInfo{}, decodeRuntimeResponse(response, nil)
	}
	size, err := strconv.ParseInt(response.Header.Get("Content-Length"), 10, 64)
	if err != nil || size < 0 {
		_ = response.Body.Close()
		return nil, agentexecution.FileInfo{}, errors.New("runtime agent returned an invalid file size")
	}
	return response.Body, agentexecution.FileInfo{
		Path: filePath, Size: size, ContentType: response.Header.Get("Content-Type"),
	}, nil
}

func (b *RemoteBackend) ExportWorkspace(ctx context.Context, _ agentexecution.Allocation) (io.ReadCloser, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, b.baseURL+"/v1/workspace/archive", nil)
	if err != nil {
		cancel()
		return nil, err
	}
	b.applyHeaders(request)
	client := *b.client
	client.Timeout = 0
	response, err := client.Do(request)
	if err != nil {
		cancel()
		return nil, err
	}
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "application/x-tar" {
		defer response.Body.Close()
		cancel()
		return nil, decodeRuntimeResponse(response, nil)
	}
	return &archiveResponseReader{
		response: response, cancel: cancel,
		reader: bufio.NewReader(response.Body), digest: sha256.New(),
	}, nil
}

func (b *RemoteBackend) ImportWorkspace(ctx context.Context, _ agentexecution.Allocation, archive io.Reader) error {
	if archive == nil {
		return agentexecution.ErrInvalidRequest
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	// The caller owns archive. net/http closes a request body after transport,
	// so wrap it to keep the caller's integrity-checked reader available for
	// its explicit close and error handling.
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, b.baseURL+"/v1/workspace/archive", io.NopCloser(archive))
	if err != nil {
		return err
	}
	b.applyHeaders(request)
	request.Header.Set("Content-Type", "application/x-tar")
	client := *b.client
	client.Timeout = 0
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return decodeRuntimeResponse(response, nil)
}

type archiveResponseReader struct {
	response *http.Response
	cancel   context.CancelFunc
	reader   *bufio.Reader
	digest   hash.Hash
	done     bool
}

const (
	workspaceArchiveFooterMagic = "ZGI-WORKSPACE-ARCHIVE-V1\n"
	workspaceArchiveFooterSize  = len(workspaceArchiveFooterMagic) + sha256.Size
)

func (r *archiveResponseReader) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	if r.done {
		return 0, io.EOF
	}
	_, err := r.reader.Peek(workspaceArchiveFooterSize + 1)
	if err == nil {
		available := r.reader.Buffered() - workspaceArchiveFooterSize
		n, readErr := r.reader.Read(buffer[:min(len(buffer), available)])
		_, _ = r.digest.Write(buffer[:n])
		return n, readErr
	}
	if !errors.Is(err, io.EOF) {
		return 0, err
	}
	r.done = true
	if r.reader.Buffered() != workspaceArchiveFooterSize {
		return 0, fmt.Errorf("%w: workspace archive stream did not complete", agentexecution.ErrBackendUnavailable)
	}
	var footer [workspaceArchiveFooterSize]byte
	if _, err := io.ReadFull(r.reader, footer[:]); err != nil {
		return 0, err
	}
	if !bytes.Equal(footer[:len(workspaceArchiveFooterMagic)], []byte(workspaceArchiveFooterMagic)) ||
		!bytes.Equal(footer[len(workspaceArchiveFooterMagic):], r.digest.Sum(nil)) {
		return 0, fmt.Errorf("%w: workspace archive checksum mismatch", agentexecution.ErrBackendUnavailable)
	}
	return 0, io.EOF
}

func (r *archiveResponseReader) Close() error {
	r.cancel()
	return r.response.Body.Close()
}

func (b *RemoteBackend) doJSON(ctx context.Context, method string, requestPath string, body any, target any) error {
	var reader io.Reader
	if body != nil {
		value, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(value)
	}
	request, err := http.NewRequestWithContext(ctx, method, b.baseURL+requestPath, reader)
	if err != nil {
		return err
	}
	b.applyHeaders(request)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := b.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return decodeRuntimeResponse(response, target)
}

func (b *RemoteBackend) applyHeaders(request *http.Request) {
	for key, values := range b.headers {
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}
	request.Header.Set("Authorization", "Bearer "+b.token)
}

func validatedRemoteHeaders(input http.Header) (http.Header, error) {
	result := make(http.Header, len(input))
	for key, values := range input {
		canonical := http.CanonicalHeaderKey(strings.TrimSpace(key))
		switch canonical {
		case "", "Authorization", "Host", "Content-Length", "Transfer-Encoding", "Connection":
			return nil, fmt.Errorf("%w: runtime agent proxy header %q is not allowed", agentexecution.ErrInvalidRequest, key)
		}
		for _, value := range values {
			if strings.ContainsAny(value, "\r\n") {
				return nil, fmt.Errorf("%w: runtime agent proxy header contains a newline", agentexecution.ErrInvalidRequest)
			}
			result.Add(canonical, value)
		}
	}
	return result, nil
}

type remoteRunningJob struct {
	backend      *RemoteBackend
	jobID        string
	sink         func(agentexecution.OutputChunk) error
	nextSequence int64
}

func (j *remoteRunningJob) Wait(ctx context.Context) (agentexecution.JobResult, error) {
	ticker := time.NewTicker(j.backend.pollInterval)
	defer ticker.Stop()
	for {
		if err := j.drainOutput(ctx); err != nil {
			return j.stopAfterFailure(ctx, err)
		}
		status, err := j.status(ctx)
		if err != nil {
			return j.stopAfterFailure(ctx, err)
		}
		if status.Truncated {
			return j.stopAfterFailure(ctx, agentexecution.ErrOutputLimit)
		}
		if status.Done {
			if err := j.drainOutput(ctx); err != nil {
				return agentexecution.JobResult{ExitCode: -1, Error: err.Error()}, err
			}
			if status.Result == nil {
				return agentexecution.JobResult{}, errors.New("runtime agent completed a job without a result")
			}
			return *status.Result, nil
		}
		select {
		case <-ctx.Done():
			return j.stopAfterFailure(ctx, ctx.Err())
		case <-ticker.C:
		}
	}
}

// A transport failure is not proof that compute has stopped. Failed cancellation
// is surfaced to the Worker so it can quarantine the provider allocation.
func (j *remoteRunningJob) stopAfterFailure(ctx context.Context, cause error) (agentexecution.JobResult, error) {
	cancelCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	stopErr := j.Cancel(cancelCtx)
	err := errors.Join(cause, stopErr)
	return agentexecution.JobResult{
		ExitCode: -1, Error: err.Error(), Canceled: stopErr == nil && errors.Is(cause, context.Canceled),
		TerminationFailed: stopErr != nil,
	}, err
}

func (j *remoteRunningJob) Cancel(ctx context.Context) error {
	var status runtimeJobStatus
	return j.backend.doJSON(ctx, http.MethodPost, "/v1/jobs/"+url.PathEscape(j.jobID)+"/cancel", struct{}{}, &status)
}

func (j *remoteRunningJob) ResizePTY(ctx context.Context, columns uint16, rows uint16) error {
	var status runtimeJobStatus
	return j.backend.doJSON(ctx, http.MethodPost, "/v1/jobs/"+url.PathEscape(j.jobID)+"/resize", runtimeResizeRequest{Columns: columns, Rows: rows}, &status)
}

func (j *remoteRunningJob) status(ctx context.Context) (runtimeJobStatus, error) {
	var status runtimeJobStatus
	err := j.backend.doJSON(ctx, http.MethodGet, "/v1/jobs/"+url.PathEscape(j.jobID), nil, &status)
	return status, err
}

func (j *remoteRunningJob) drainOutput(ctx context.Context) error {
	var chunks []agentexecution.OutputChunk
	requestPath := "/v1/jobs/" + url.PathEscape(j.jobID) + "/output?" + url.Values{"after": []string{strconv.FormatInt(j.nextSequence, 10)}}.Encode()
	if err := j.backend.doJSON(ctx, http.MethodGet, requestPath, nil, &chunks); err != nil {
		return err
	}
	for _, chunk := range chunks {
		if chunk.Sequence <= j.nextSequence {
			continue
		}
		if err := j.sink(chunk); err != nil {
			return err
		}
		j.nextSequence = chunk.Sequence
	}
	return nil
}

func decodeRuntimeResponse(response *http.Response, target any) error {
	var envelope struct {
		Data  json.RawMessage    `json:"data"`
		Error *runtimeAgentError `json:"error"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 16*1024*1024))
	if err := decoder.Decode(&envelope); err != nil {
		return fmt.Errorf("decode runtime agent response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || envelope.Error != nil {
		if envelope.Error == nil {
			return fmt.Errorf("runtime agent returned HTTP %d", response.StatusCode)
		}
		return runtimeAgentClientError(envelope.Error.Code)
	}
	if target == nil || len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, target); err != nil {
		return fmt.Errorf("decode runtime agent data: %w", err)
	}
	return nil
}

func runtimeAgentClientError(code string) error {
	switch code {
	case "not_found":
		return agentexecution.ErrNotFound
	case "invalid_request":
		return agentexecution.ErrInvalidRequest
	case "idempotency_conflict":
		return agentexecution.ErrIdempotencyConflict
	case "state_conflict":
		return agentexecution.ErrInvalidTransition
	case "limit_exceeded":
		return agentexecution.ErrOutputLimit
	case "backend_unavailable":
		return agentexecution.ErrBackendUnavailable
	case "unauthorized":
		return errors.New("runtime agent authentication failed")
	default:
		return errors.New("runtime agent request failed")
	}
}
