package skills

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const defaultSkillScriptTimeoutSeconds = 30
const defaultSkillDependencyProfile = "stdlib"

const (
	defaultSandboxConnectTimeout        = 5 * time.Second
	defaultSandboxCreateTimeout         = 10 * time.Second
	defaultSandboxUploadTimeout         = 30 * time.Second
	defaultSandboxCommandTimeoutPadding = 15 * time.Second
	defaultSandboxArtifactTimeout       = 10 * time.Second
	defaultSandboxCleanupTimeout        = 5 * time.Second
)

type SandboxScriptRunnerConfig struct {
	Endpoint              string
	APIKey                string
	ConnectTimeout        time.Duration
	CreateTimeout         time.Duration
	UploadTimeout         time.Duration
	CommandTimeoutPadding time.Duration
	ArtifactTimeout       time.Duration
	CleanupTimeout        time.Duration
}

type SandboxScriptRunner struct {
	endpoint string
	apiKey   string
	client   *http.Client
	timeouts sandboxScriptRunnerTimeouts
}

type sandboxScriptRunnerTimeouts struct {
	Create         time.Duration
	Upload         time.Duration
	CommandPadding time.Duration
	Artifact       time.Duration
	Cleanup        time.Duration
}

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

func NewSandboxScriptRunner(config SandboxScriptRunnerConfig) *SandboxScriptRunner {
	endpoint := strings.TrimRight(strings.TrimSpace(config.Endpoint), "/")
	if endpoint == "" {
		return nil
	}
	connectTimeout := durationOrDefault(config.ConnectTimeout, defaultSandboxConnectTimeout)
	return &SandboxScriptRunner{
		endpoint: endpoint,
		apiKey:   strings.TrimSpace(config.APIKey),
		client: &http.Client{
			Transport: sandboxHTTPTransport(connectTimeout),
		},
		timeouts: sandboxScriptRunnerTimeouts{
			Create:         durationOrDefault(config.CreateTimeout, defaultSandboxCreateTimeout),
			Upload:         durationOrDefault(config.UploadTimeout, defaultSandboxUploadTimeout),
			CommandPadding: durationOrDefault(config.CommandTimeoutPadding, defaultSandboxCommandTimeoutPadding),
			Artifact:       durationOrDefault(config.ArtifactTimeout, defaultSandboxArtifactTimeout),
			Cleanup:        durationOrDefault(config.CleanupTimeout, defaultSandboxCleanupTimeout),
		},
	}
}

func (r *SandboxScriptRunner) Configured() bool {
	return r != nil && r.endpoint != "" && r.client != nil
}

func (r *SandboxScriptRunner) RunSkillScript(ctx context.Context, doc SkillDocument, arguments map[string]interface{}, execCtx ExecutionContext, callID string) (*ToolInvocationResult, error) {
	if !r.Configured() {
		return nil, fmt.Errorf("skill script runner is not configured")
	}
	if !doc.Metadata.HasScripts {
		return nil, fmt.Errorf("skill %s does not include scripts", doc.Metadata.ID)
	}
	entrypoint := filepath.Join(doc.Metadata.RootPath, "scripts", "run.py")
	if info, err := os.Stat(entrypoint); err != nil || info.IsDir() {
		return nil, fmt.Errorf("skill %s script entrypoint scripts/run.py not found", doc.Metadata.ID)
	}

	start := time.Now()
	trace := SkillTrace{
		Kind:      "tool_call",
		SkillID:   doc.Metadata.ID,
		ToolName:  SkillScriptToolRun,
		Status:    "success",
		Arguments: summarizeArguments(arguments),
	}

	manifest, err := prepareSkillScriptManifest(doc.Metadata.RootPath, docTimeoutSeconds(doc))
	if err != nil {
		trace.Status = "error"
		trace.Error = err.Error()
		trace.DurationMS = time.Since(start).Milliseconds()
		return &ToolInvocationResult{Trace: trace}, err
	}
	dependencyProfile := manifest.Manifest.DependencyProfile

	sandboxID, err := r.createSandbox(ctx, execCtx, dependencyProfile)
	if err != nil {
		recordSkillScriptError(&trace, start, err)
		return &ToolInvocationResult{Trace: trace}, err
	}
	defer func() { _ = r.deleteSandbox(context.WithoutCancel(ctx), sandboxID, execCtx) }()

	archiveBase64, err := zipSkillDirectoryBase64(doc.Metadata.RootPath, manifest.Raw)
	if err != nil {
		trace.Status = "error"
		trace.Error = err.Error()
		trace.DurationMS = time.Since(start).Milliseconds()
		return &ToolInvocationResult{Trace: trace}, err
	}
	if err := r.uploadArchive(ctx, sandboxID, archiveBase64, execCtx); err != nil {
		recordSkillScriptError(&trace, start, err)
		return &ToolInvocationResult{Trace: trace}, err
	}

	stdin, err := json.Marshal(arguments)
	if err != nil {
		trace.Status = "error"
		trace.Error = err.Error()
		trace.DurationMS = time.Since(start).Milliseconds()
		return &ToolInvocationResult{Trace: trace}, fmt.Errorf("failed to encode skill script arguments: %w", err)
	}
	timeout := skillScriptTimeoutSeconds(manifest.Manifest.TimeoutMS)
	command, err := r.runCommand(ctx, sandboxID, string(stdin), timeout, execCtx)
	if err != nil {
		recordSkillScriptError(&trace, start, err)
		return &ToolInvocationResult{Trace: trace}, err
	}

	artifacts, artifactErr := r.collectArtifacts(ctx, sandboxID, execCtx, manifest.Manifest)
	messages, content, err := skillScriptMessages(command, artifacts, artifactErr)
	trace.DurationMS = time.Since(start).Milliseconds()
	if command.ExitCode != 0 {
		err = fmt.Errorf("skill script exited with code %d: %s", command.ExitCode, strings.TrimSpace(command.Error))
	}
	if err != nil {
		recordSkillScriptError(&trace, start, err)
		return &ToolInvocationResult{Trace: trace, Messages: messages, ToolMessage: skillScriptToolMessage(callID, content)}, err
	}
	trace.Result = summarizeSkillScriptResult(command, messages)
	return &ToolInvocationResult{
		Messages:    messages,
		Trace:       trace,
		ToolMessage: skillScriptToolMessage(callID, content),
	}, nil
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

func (r *SandboxScriptRunner) uploadArchive(ctx context.Context, sandboxID string, archiveBase64 string, execCtx ExecutionContext) error {
	payload := map[string]interface{}{
		"sandbox_id":              sandboxID,
		"path":                    ".",
		"archive_base64":          archiveBase64,
		"format":                  "zip",
		"strip_root":              false,
		"validate_skill_manifest": true,
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

func (r *SandboxScriptRunner) deleteSandbox(ctx context.Context, sandboxID string, execCtx ExecutionContext) error {
	if strings.TrimSpace(sandboxID) == "" {
		return nil
	}
	path := withOrganizationQuery("/v1/sandboxes/"+url.PathEscape(sandboxID), execCtx)
	return r.doJSON(ctx, http.MethodDelete, path, nil, nil, r.timeouts.Cleanup)
}

func (r *SandboxScriptRunner) collectArtifacts(ctx context.Context, sandboxID string, execCtx ExecutionContext, manifest skillScriptManifest) ([]skillScriptArtifact, error) {
	var tree struct {
		Items []sandboxFileInfo `json:"items"`
	}
	path := withOrganizationQuery("/v1/files/tree?sandbox_id="+url.QueryEscape(sandboxID), execCtx)
	if err := r.doJSON(ctx, http.MethodGet, path, nil, &tree, r.timeouts.Artifact); err != nil {
		return nil, err
	}

	artifacts := make([]skillScriptArtifact, 0)
	for _, item := range tree.Items {
		if item.IsDirectory || !skillManifestAllowsArtifactPath(manifest, item.Path) {
			continue
		}
		if len(artifacts) >= manifest.MaxArtifactCount {
			break
		}
		artifact := skillScriptArtifact{
			Path: item.Path,
			Name: filepath.Base(item.Path),
			Size: item.Size,
		}
		if item.Size <= manifest.MaxArtifactBytes && item.Size <= 32*1024 {
			content, err := r.downloadArtifact(ctx, sandboxID, item.Path, execCtx)
			if err != nil {
				artifact.Error = err.Error()
			} else {
				artifact.Content = content.Content
				artifact.Encoding = content.Encoding
			}
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}

func (r *SandboxScriptRunner) downloadArtifact(ctx context.Context, sandboxID string, path string, execCtx ExecutionContext) (*sandboxFileContent, error) {
	var content sandboxFileContent
	endpoint := "/v1/files/download?sandbox_id=" + url.QueryEscape(sandboxID) + "&path=" + url.QueryEscape(path) + "&encoding=base64"
	endpoint = withOrganizationQuery(endpoint, execCtx)
	if err := r.doJSON(ctx, http.MethodGet, endpoint, nil, &content, r.timeouts.Artifact); err != nil {
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

type skillScriptArtifact struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Content  string `json:"content,omitempty"`
	Encoding string `json:"encoding,omitempty"`
	Error    string `json:"error,omitempty"`
}

func zipSkillDirectoryBase64(root string, manifestRaw []byte) (string, error) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("skill package contains symlink: %s", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if filepath.ToSlash(rel) == "skill.manifest.json" && len(manifestRaw) > 0 {
			return nil
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		header.Method = zip.Deflate
		fileWriter, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		return copyFileIntoZip(path, fileWriter)
	})
	if err == nil && len(manifestRaw) > 0 {
		err = addSkillManifestToZip(writer, manifestRaw)
	}
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buffer.Bytes()), nil
}

type skillScriptManifest struct {
	Entrypoint           string   `json:"entrypoint"`
	Language             string   `json:"language"`
	DependencyProfile    string   `json:"dependency_profile"`
	TimeoutMS            int      `json:"timeout_ms"`
	AllowedArtifactPaths []string `json:"allowed_artifact_paths"`
	MaxArtifactCount     int      `json:"max_artifact_count"`
	MaxArtifactBytes     int64    `json:"max_artifact_bytes"`
	ResultMode           string   `json:"result_mode"`
}

type preparedSkillScriptManifest struct {
	Manifest skillScriptManifest
	Raw      []byte
}

func prepareSkillScriptManifest(root string, fallbackTimeoutSeconds int) (preparedSkillScriptManifest, error) {
	manifest, err := loadSkillScriptManifest(root)
	if err != nil {
		return preparedSkillScriptManifest{}, err
	}
	if err := normalizeSkillScriptManifest(root, fallbackTimeoutSeconds, &manifest); err != nil {
		return preparedSkillScriptManifest{}, err
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		return preparedSkillScriptManifest{}, err
	}
	return preparedSkillScriptManifest{Manifest: manifest, Raw: raw}, nil
}

func loadSkillScriptManifest(root string) (skillScriptManifest, error) {
	manifestPath := filepath.Join(root, "skill.manifest.json")
	info, err := os.Lstat(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return skillScriptManifest{}, nil
		}
		return skillScriptManifest{}, fmt.Errorf("failed to inspect skill manifest: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return skillScriptManifest{}, fmt.Errorf("skill.manifest.json must not be a symlink")
	}
	if info.IsDir() {
		return skillScriptManifest{}, fmt.Errorf("skill.manifest.json must be a file")
	}
	if info.Size() > 64*1024 {
		return skillScriptManifest{}, fmt.Errorf("skill.manifest.json exceeds max size of 65536 bytes")
	}
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		return skillScriptManifest{}, fmt.Errorf("failed to read skill manifest: %w", err)
	}
	var manifest skillScriptManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return skillScriptManifest{}, fmt.Errorf("invalid skill.manifest.json: %w", err)
	}
	return manifest, nil
}

func normalizeSkillScriptManifest(root string, fallbackTimeoutSeconds int, manifest *skillScriptManifest) error {
	if manifest == nil {
		return fmt.Errorf("skill manifest is empty")
	}
	manifest.Entrypoint = filepath.ToSlash(strings.TrimSpace(manifest.Entrypoint))
	if manifest.Entrypoint == "" {
		manifest.Entrypoint = "scripts/run.py"
	}
	if manifest.Entrypoint != "scripts/run.py" {
		return fmt.Errorf("skill manifest entrypoint must be scripts/run.py for API run_script: %s", manifest.Entrypoint)
	}
	if unsafeSkillManifestPath(manifest.Entrypoint) {
		return fmt.Errorf("skill manifest entrypoint escapes package root: %s", manifest.Entrypoint)
	}
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(manifest.Entrypoint)))
	if err != nil || info.IsDir() {
		return fmt.Errorf("skill manifest entrypoint is missing from package: %s", manifest.Entrypoint)
	}

	manifest.Language = normalizeSkillScriptLanguage(manifest.Language)
	if manifest.Language == "" {
		manifest.Language = "python3"
	}
	if manifest.Language != "python3" {
		return fmt.Errorf("skill manifest language must be python3 for API run_script: %s", manifest.Language)
	}

	manifest.DependencyProfile = strings.TrimSpace(manifest.DependencyProfile)
	if manifest.DependencyProfile == "" {
		manifest.DependencyProfile = defaultSkillDependencyProfile
	}
	if manifest.TimeoutMS <= 0 {
		timeoutSeconds := fallbackTimeoutSeconds
		if timeoutSeconds <= 0 {
			timeoutSeconds = defaultSkillScriptTimeoutSeconds
		}
		manifest.TimeoutMS = timeoutSeconds * 1000
	}
	if manifest.TimeoutMS <= 0 || manifest.TimeoutMS > 300000 {
		return fmt.Errorf("skill manifest timeout_ms must be between 1 and 300000")
	}
	if len(manifest.AllowedArtifactPaths) == 0 {
		manifest.AllowedArtifactPaths = []string{"artifacts"}
	}
	for i, raw := range manifest.AllowedArtifactPaths {
		path := filepath.ToSlash(strings.TrimSpace(raw))
		if path == "" {
			return fmt.Errorf("skill manifest allowed_artifact_paths contains an empty path")
		}
		if unsafeSkillManifestPath(path) || (path != "artifacts" && !strings.HasPrefix(path, "artifacts/")) {
			return fmt.Errorf("skill manifest allowed artifact path must be under artifacts: %s", path)
		}
		manifest.AllowedArtifactPaths[i] = path
	}
	if manifest.MaxArtifactCount <= 0 {
		manifest.MaxArtifactCount = 10
	}
	if manifest.MaxArtifactCount > 10 {
		return fmt.Errorf("skill manifest max_artifact_count must be between 1 and 10")
	}
	if manifest.MaxArtifactBytes <= 0 {
		manifest.MaxArtifactBytes = 32 * 1024
	}
	if manifest.MaxArtifactBytes > 32*1024 {
		return fmt.Errorf("skill manifest max_artifact_bytes must be between 1 and 32768")
	}
	manifest.ResultMode = strings.TrimSpace(manifest.ResultMode)
	if manifest.ResultMode == "" {
		manifest.ResultMode = "mixed"
	}
	switch manifest.ResultMode {
	case "stdout_json", "stdout_text", "artifacts", "mixed":
		return nil
	default:
		return fmt.Errorf("skill manifest result_mode must be stdout_json, stdout_text, artifacts, or mixed")
	}
}

func copyFileIntoZip(path string, writer io.Writer) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(writer, file)
	return err
}

func addSkillManifestToZip(writer *zip.Writer, manifestRaw []byte) error {
	header := &zip.FileHeader{
		Name:   "skill.manifest.json",
		Method: zip.Deflate,
	}
	header.SetMode(0o644)
	fileWriter, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = fileWriter.Write(manifestRaw)
	return err
}

func normalizeSkillScriptLanguage(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return ""
	case "python", "python3":
		return "python3"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func unsafeSkillManifestPath(value string) bool {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(value)))
	return clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/")
}

func skillScriptTimeoutSeconds(timeoutMS int) int {
	if timeoutMS <= 0 {
		return defaultSkillScriptTimeoutSeconds
	}
	return (timeoutMS + 999) / 1000
}

func skillManifestAllowsArtifactPath(manifest skillScriptManifest, value string) bool {
	path := filepath.ToSlash(strings.TrimSpace(value))
	if path == "" {
		return false
	}
	for _, allowed := range manifest.AllowedArtifactPaths {
		allowed = strings.TrimSuffix(filepath.ToSlash(strings.TrimSpace(allowed)), "/")
		if allowed == "" {
			continue
		}
		if path == allowed || strings.HasPrefix(path, allowed+"/") {
			return true
		}
	}
	return false
}

func skillScriptEnv(execCtx ExecutionContext) map[string]string {
	env := map[string]string{}
	if value := strings.TrimSpace(execCtx.OrganizationID); value != "" {
		env["ZGI_ORGANIZATION_ID"] = value
	}
	if value := strings.TrimSpace(execCtx.UserID); value != "" {
		env["ZGI_USER_ID"] = value
	}
	if value := strings.TrimSpace(execCtx.ConversationID); value != "" {
		env["ZGI_CONVERSATION_ID"] = value
	}
	if value := strings.TrimSpace(execCtx.MessageID); value != "" {
		env["ZGI_MESSAGE_ID"] = value
	}
	return env
}

func skillScriptMessages(command *sandboxCommandResult, artifacts []skillScriptArtifact, artifactErr error) ([]tools.ToolInvokeMessage, string, error) {
	if command == nil {
		return nil, "", fmt.Errorf("sandbox command result is empty")
	}
	stdout := strings.TrimSpace(command.Stdout)
	messages := []tools.ToolInvokeMessage{}
	if stdout != "" {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(stdout), &data); err == nil {
			messages = append(messages, tools.ToolInvokeMessage{Type: tools.ToolInvokeMessageTypeJSON, Data: data})
		} else {
			messages = append(messages, tools.ToolInvokeMessage{Type: tools.ToolInvokeMessageTypeText, Text: command.Stdout})
		}
	}
	if strings.TrimSpace(command.Error) != "" {
		messages = append(messages, tools.ToolInvokeMessage{
			Type: tools.ToolInvokeMessageTypeLog,
			Text: command.Error,
			Meta: map[string]interface{}{"stream": "stderr"},
		})
	}
	if command.Truncated {
		messages = append(messages, tools.ToolInvokeMessage{
			Type: tools.ToolInvokeMessageTypeLog,
			Text: "skill script output was truncated",
		})
	}
	if len(artifacts) > 0 {
		items := make([]map[string]interface{}, 0, len(artifacts))
		for _, artifact := range artifacts {
			item := map[string]interface{}{
				"path": artifact.Path,
				"name": artifact.Name,
				"size": artifact.Size,
			}
			if artifact.Encoding != "" {
				item["encoding"] = artifact.Encoding
			}
			if artifact.Content != "" {
				item["content"] = artifact.Content
			}
			if artifact.Error != "" {
				item["error"] = artifact.Error
			}
			items = append(items, item)
		}
		messages = append(messages, tools.ToolInvokeMessage{
			Type: tools.ToolInvokeMessageTypeJSON,
			Data: map[string]interface{}{"artifacts": items},
		})
	}
	if artifactErr != nil {
		messages = append(messages, tools.ToolInvokeMessage{
			Type: tools.ToolInvokeMessageTypeLog,
			Text: "failed to collect skill script artifacts: " + artifactErr.Error(),
		})
	}
	contentBytes, err := json.Marshal(messages)
	if err != nil {
		return messages, "", err
	}
	return messages, string(contentBytes), nil
}

func skillScriptToolMessage(callID string, content string) llmadapter.Message {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		callID = "call_" + SkillScriptToolRun
	}
	return llmadapter.Message{
		Role:       "tool",
		ToolCallID: callID,
		Content:    content,
	}
}

func summarizeSkillScriptResult(command *sandboxCommandResult, messages []tools.ToolInvokeMessage) map[string]interface{} {
	if command == nil {
		return nil
	}
	return map[string]interface{}{
		"exit_code":   command.ExitCode,
		"duration_ms": command.DurationMS,
		"truncated":   command.Truncated,
		"messages":    len(messages),
	}
}
