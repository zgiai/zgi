package skills

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type SandboxRequestError struct {
	Method     string                 `json:"method"`
	Path       string                 `json:"path"`
	StatusCode int                    `json:"status_code"`
	Code       int                    `json:"code"`
	Message    string                 `json:"message"`
	Data       map[string]interface{} `json:"data,omitempty"`
}

func (e *SandboxRequestError) Error() string {
	if e == nil {
		return ""
	}
	message := strings.TrimSpace(e.Message)
	if message == "" {
		message = http.StatusText(e.StatusCode)
	}
	return fmt.Sprintf("sandbox request %s %s failed: %s", e.Method, e.Path, message)
}

func (r *SandboxScriptRunner) createSandbox(ctx context.Context, execCtx ExecutionContext, dependencyProfile string) (string, error) {
	var response struct {
		ID string `json:"id"`
	}
	if strings.TrimSpace(dependencyProfile) == "" {
		dependencyProfile = defaultSkillDependencyProfile
	}
	payload := map[string]interface{}{
		"runtime_profile":    "session",
		"ttl_seconds":        300,
		"network_enabled":    false,
		"network_policy":     "deny-by-default",
		"dependency_profile": dependencyProfile,
	}
	if organizationID := strings.TrimSpace(execCtx.OrganizationID); organizationID != "" {
		payload["organization_id"] = organizationID
	}
	if err := r.doJSON(ctx, http.MethodPost, "/v1/sandboxes", payload, &response, r.timeouts.Create); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.ID) == "" {
		return "", fmt.Errorf("sandbox create response did not include sandbox id")
	}
	return response.ID, nil
}

func (r *SandboxScriptRunner) uploadArchive(ctx context.Context, sandboxID string, archiveBase64 string, execCtx ExecutionContext, validateSkillManifest bool) error {
	payload := map[string]interface{}{
		"sandbox_id":              sandboxID,
		"path":                    ".",
		"archive_base64":          archiveBase64,
		"format":                  "zip",
		"strip_root":              false,
		"validate_skill_manifest": validateSkillManifest,
	}
	if organizationID := strings.TrimSpace(execCtx.OrganizationID); organizationID != "" {
		payload["organization_id"] = organizationID
	}
	return r.doJSON(ctx, http.MethodPost, "/v1/files/upload-archive", payload, nil, r.timeouts.Upload)
}

func (r *SandboxScriptRunner) runCommand(ctx context.Context, sandboxID string, stdin string, timeoutSeconds int, execCtx ExecutionContext) (*sandboxCommandResult, error) {
	var response sandboxCommandResult
	payload := map[string]interface{}{
		"sandbox_id":      sandboxID,
		"command":         "python3",
		"args":            []string{"scripts/run.py"},
		"stdin":           stdin,
		"env":             skillScriptEnv(execCtx),
		"profile":         "skill-python",
		"timeout_seconds": timeoutSeconds,
		"stdout_limit_kb": 1024,
		"stderr_limit_kb": 1024,
		"working_subpath": ".",
	}
	if organizationID := strings.TrimSpace(execCtx.OrganizationID); organizationID != "" {
		payload["organization_id"] = organizationID
	}
	err := r.doJSON(ctx, http.MethodPost, "/v1/exec/command", payload, &response, r.commandRequestTimeout(timeoutSeconds))
	if err != nil {
		return nil, err
	}
	return &response, nil
}

func (r *SandboxScriptRunner) runSkill(ctx context.Context, sandboxID string, stdin string, execCtx ExecutionContext, timeoutSeconds int) (*sandboxSkillRunResult, error) {
	var response sandboxSkillRunResult
	payload := map[string]interface{}{
		"sandbox_id": sandboxID,
		"path":       ".",
		"stdin":      stdin,
		"env":        skillScriptEnv(execCtx),
	}
	if organizationID := strings.TrimSpace(execCtx.OrganizationID); organizationID != "" {
		payload["organization_id"] = organizationID
	}
	err := r.doJSON(ctx, http.MethodPost, "/v1/exec/skill", payload, &response, r.commandRequestTimeout(timeoutSeconds))
	if err != nil {
		return nil, err
	}
	return &response, nil
}

func (r *SandboxScriptRunner) deleteSandbox(ctx context.Context, sandboxID string, execCtx ExecutionContext) error {
	if strings.TrimSpace(sandboxID) == "" {
		return nil
	}
	path := withOrganizationQuery("/v1/sandboxes/"+url.PathEscape(sandboxID), execCtx)
	return r.doIdempotentJSON(ctx, http.MethodDelete, path, nil, nil, r.timeouts.Cleanup)
}

func (r *SandboxScriptRunner) downloadArtifact(ctx context.Context, sandboxID string, path string, execCtx ExecutionContext) (*sandboxFileContent, error) {
	var content sandboxFileContent
	endpoint := "/v1/files/download?sandbox_id=" + url.QueryEscape(sandboxID) + "&path=" + url.QueryEscape(path) + "&encoding=base64"
	endpoint = withOrganizationQuery(endpoint, execCtx)
	if err := r.doIdempotentJSON(ctx, http.MethodGet, endpoint, nil, &content, r.timeouts.Artifact); err != nil {
		return nil, err
	}
	return &content, nil
}

func (r *SandboxScriptRunner) commandRequestTimeout(timeoutSeconds int) time.Duration {
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultSkillScriptTimeoutSeconds * time.Second
	}
	return timeout + r.timeouts.CommandPadding
}

func withOrganizationQuery(path string, execCtx ExecutionContext) string {
	organizationID := strings.TrimSpace(execCtx.OrganizationID)
	if organizationID == "" {
		return path
	}
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + "organization_id=" + url.QueryEscape(organizationID)
}

func (r *SandboxScriptRunner) doJSON(ctx context.Context, method string, path string, payload interface{}, out interface{}, timeout time.Duration) error {
	return r.doJSONOnce(ctx, method, path, payload, out, timeout)
}

func (r *SandboxScriptRunner) doIdempotentJSON(ctx context.Context, method string, path string, payload interface{}, out interface{}, timeout time.Duration) error {
	var lastErr error
	for attempt := 1; attempt <= defaultSandboxIdempotentAttempts; attempt++ {
		lastErr = r.doJSONOnce(ctx, method, path, payload, out, timeout)
		if lastErr == nil || !retryableSandboxError(ctx, lastErr) || attempt == defaultSandboxIdempotentAttempts {
			return lastErr
		}
		if err := sleepSandboxRetry(ctx, attempt); err != nil {
			return err
		}
	}
	return lastErr
}

func (r *SandboxScriptRunner) doJSONOnce(ctx context.Context, method string, path string, payload interface{}, out interface{}, timeout time.Duration) error {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, method, r.endpoint+path, body)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if r.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+r.apiKey)
		req.Header.Set("X-API-Key", r.apiKey)
	}
	res, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4*1024*1024))
	if err != nil {
		return err
	}
	var envelope struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("failed to parse sandbox response: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 || envelope.Code != 0 {
		message := strings.TrimSpace(envelope.Message)
		if message == "" {
			message = res.Status
		}
		return &SandboxRequestError{
			Method:     method,
			Path:       path,
			StatusCode: res.StatusCode,
			Code:       envelope.Code,
			Message:    message,
			Data:       sandboxErrorData(envelope.Data),
		}
	}
	if out != nil && len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return fmt.Errorf("failed to parse sandbox data: %w", err)
		}
	}
	return nil
}

func retryableSandboxError(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	var sandboxErr *SandboxRequestError
	if errors.As(err, &sandboxErr) {
		switch sandboxErr.StatusCode {
		case http.StatusRequestTimeout, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		default:
			return false
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}

func sleepSandboxRetry(ctx context.Context, attempt int) error {
	delay := defaultSandboxRetryBaseDelay * time.Duration(1<<(attempt-1))
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func recordSkillScriptError(trace *SkillTrace, start time.Time, err error) {
	if trace == nil || err == nil {
		return
	}
	trace.Status = "error"
	trace.Error = err.Error()
	trace.DurationMS = time.Since(start).Milliseconds()
	if result := sandboxErrorTraceResult(err); len(result) > 0 {
		trace.Result = result
	}
}

func sandboxErrorTraceResult(err error) map[string]interface{} {
	var sandboxErr *SandboxRequestError
	if !errors.As(err, &sandboxErr) || sandboxErr == nil {
		return nil
	}
	result := map[string]interface{}{
		"sandbox_error": map[string]interface{}{
			"method":      sandboxErr.Method,
			"path":        sandboxErr.Path,
			"status_code": sandboxErr.StatusCode,
			"code":        sandboxErr.Code,
			"message":     sandboxErr.Message,
		},
	}
	if len(sandboxErr.Data) > 0 {
		result["sandbox_error"].(map[string]interface{})["data"] = sandboxErr.Data
	}
	return result
}

func sandboxErrorData(raw json.RawMessage) map[string]interface{} {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil || len(data) == 0 {
		return nil
	}
	return data
}

func durationOrDefault(value time.Duration, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}

func sandboxHTTPTransport(connectTimeout time.Duration) http.RoundTripper {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{
		Timeout:   connectTimeout,
		KeepAlive: 30 * time.Second,
	}).DialContext
	transport.TLSHandshakeTimeout = durationOrDefault(connectTimeout, defaultSandboxConnectTimeout)
	return transport
}

type sandboxCommandResult struct {
	Stdout     string   `json:"stdout"`
	Error      string   `json:"error"`
	ExitCode   int      `json:"exit_code"`
	DurationMS int64    `json:"duration_ms"`
	Truncated  bool     `json:"truncated"`
	Command    string   `json:"command"`
	Args       []string `json:"args"`
	Backend    string   `json:"backend"`
}

type sandboxSkillRunResult struct {
	ExecutionID       string                `json:"execution_id"`
	SandboxID         string                `json:"sandbox_id"`
	Path              string                `json:"path"`
	Manifest          sandboxSkillManifest  `json:"manifest"`
	Command           sandboxCommandResult  `json:"command"`
	ArtifactManifests []sandboxFileManifest `json:"artifact_manifests"`
	ResultJSON        interface{}           `json:"result_json"`
}

type sandboxDependencyCatalog struct {
	Language string                     `json:"language"`
	Profiles []sandboxDependencyProfile `json:"profiles"`
}

type sandboxDependencyProfile struct {
	Name      string   `json:"name"`
	Version   string   `json:"version"`
	Status    string   `json:"status"`
	Enabled   bool     `json:"enabled"`
	Languages []string `json:"languages"`
}

type sandboxDependencyBuild struct {
	BuildID          string `json:"build_id"`
	Fingerprint      string `json:"fingerprint"`
	Status           string `json:"status"`
	ProfileName      string `json:"profile_name"`
	ArtifactChecksum string `json:"artifact_checksum,omitempty"`
	NextAction       string `json:"next_action,omitempty"`
	Error            string `json:"error,omitempty"`
}

type sandboxSkillManifest struct {
	Entrypoint           string   `json:"entrypoint"`
	Language             string   `json:"language"`
	TimeoutMS            int      `json:"timeout_ms"`
	AllowedArtifactPaths []string `json:"allowed_artifact_paths"`
	MaxArtifactCount     int      `json:"max_artifact_count"`
	MaxArtifactBytes     int64    `json:"max_artifact_bytes"`
	ResultMode           string   `json:"result_mode"`
}

type sandboxFileManifest struct {
	SandboxID string                    `json:"sandbox_id"`
	Path      string                    `json:"path"`
	Items     []sandboxFileManifestItem `json:"items"`
	FileCount int                       `json:"file_count"`
	TotalSize int64                     `json:"total_size"`
	Truncated bool                      `json:"truncated"`
}

type sandboxFileManifestItem struct {
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	Encoding    string `json:"encoding"`
	SHA256      string `json:"sha256"`
	ContentType string `json:"content_type"`
}

type sandboxFileInfo struct {
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	IsDirectory bool   `json:"is_directory"`
}

type sandboxFileContent struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}
