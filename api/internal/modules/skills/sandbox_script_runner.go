package skills

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	workflowfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	tool_file "github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const defaultSkillScriptTimeoutSeconds = 30
const defaultSkillScriptHTTPTimeout = 6 * time.Minute
const inlineSkillArtifactMaxBytes = 32 * 1024
const maxSkillScriptArtifactCount = 10
const maxSkillScriptArtifactBytes = 2 * 1024 * 1024

type SandboxScriptRunnerConfig struct {
	Endpoint          string
	APIKey            string
	ArtifactPersister SkillScriptArtifactPersister
}

type SandboxScriptRunner struct {
	endpoint          string
	apiKey            string
	client            *http.Client
	artifactPersister SkillScriptArtifactPersister
}

type SkillScriptArtifactPersistRequest struct {
	ExecContext ExecutionContext
	Path        string
	Name        string
	Size        int64
	ContentType string
	Data        []byte
}

type SkillScriptArtifactPersister interface {
	PersistSkillScriptArtifact(ctx context.Context, request SkillScriptArtifactPersistRequest) (map[string]interface{}, error)
}

func NewSandboxScriptRunner(config SandboxScriptRunnerConfig) *SandboxScriptRunner {
	endpoint := strings.TrimRight(strings.TrimSpace(config.Endpoint), "/")
	if endpoint == "" {
		return nil
	}
	return &SandboxScriptRunner{
		endpoint:          endpoint,
		apiKey:            strings.TrimSpace(config.APIKey),
		client:            &http.Client{Timeout: defaultSkillScriptHTTPTimeout},
		artifactPersister: config.ArtifactPersister,
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
	hasManifest, err := hasSkillManifest(doc.Metadata.RootPath)
	if err != nil {
		return nil, fmt.Errorf("skill %s manifest is invalid: %w", doc.Metadata.ID, err)
	}
	if !hasManifest {
		entrypoint := filepath.Join(doc.Metadata.RootPath, "scripts", "run.py")
		if info, err := os.Stat(entrypoint); err != nil || info.IsDir() {
			return nil, fmt.Errorf("skill %s script entrypoint scripts/run.py not found", doc.Metadata.ID)
		}
	}

	start := time.Now()
	trace := SkillTrace{
		Kind:      "tool_call",
		SkillID:   doc.Metadata.ID,
		ToolName:  SkillScriptToolRun,
		Status:    "success",
		Arguments: summarizeArguments(arguments),
	}

	sandboxID, err := r.createSandbox(ctx, execCtx)
	if err != nil {
		trace.Status = "error"
		trace.Error = err.Error()
		trace.DurationMS = time.Since(start).Milliseconds()
		return &ToolInvocationResult{Trace: trace}, err
	}
	defer func() { _ = r.deleteSandbox(context.WithoutCancel(ctx), sandboxID) }()

	archiveBase64, err := zipSkillDirectoryBase64(doc.Metadata.RootPath)
	if err != nil {
		trace.Status = "error"
		trace.Error = err.Error()
		trace.DurationMS = time.Since(start).Milliseconds()
		return &ToolInvocationResult{Trace: trace}, err
	}
	if err := r.uploadArchive(ctx, sandboxID, archiveBase64, hasManifest); err != nil {
		trace.Status = "error"
		trace.Error = err.Error()
		trace.DurationMS = time.Since(start).Milliseconds()
		return &ToolInvocationResult{Trace: trace}, err
	}

	stdin, err := json.Marshal(arguments)
	if err != nil {
		trace.Status = "error"
		trace.Error = err.Error()
		trace.DurationMS = time.Since(start).Milliseconds()
		return &ToolInvocationResult{Trace: trace}, fmt.Errorf("failed to encode skill script arguments: %w", err)
	}

	var command *sandboxCommandResult
	var artifacts []skillScriptArtifact
	var artifactErr error
	if hasManifest {
		runResult, err := r.runSkill(ctx, sandboxID, string(stdin))
		if err != nil {
			trace.Status = "error"
			trace.Error = err.Error()
			trace.DurationMS = time.Since(start).Milliseconds()
			return &ToolInvocationResult{Trace: trace}, err
		}
		command = &runResult.Command
		artifacts, artifactErr = r.artifactsFromManifests(ctx, sandboxID, runResult.ArtifactManifests)
	} else {
		timeout := docTimeoutSeconds(doc)
		if timeout <= 0 {
			timeout = defaultSkillScriptTimeoutSeconds
		}
		command, err = r.runCommand(ctx, sandboxID, string(stdin), timeout, execCtx)
		if err != nil {
			trace.Status = "error"
			trace.Error = err.Error()
			trace.DurationMS = time.Since(start).Milliseconds()
			return &ToolInvocationResult{Trace: trace}, err
		}
		artifacts, artifactErr = r.collectArtifacts(ctx, sandboxID)
	}

	r.prepareArtifacts(ctx, sandboxID, artifacts, execCtx)
	messages, content, err := skillScriptMessages(command, artifacts, artifactErr)
	trace.DurationMS = time.Since(start).Milliseconds()
	if command.ExitCode != 0 {
		err = fmt.Errorf("skill script exited with code %d: %s", command.ExitCode, strings.TrimSpace(command.Error))
	}
	if err != nil {
		trace.Status = "error"
		trace.Error = err.Error()
		return &ToolInvocationResult{Trace: trace, Messages: messages, ToolMessage: skillScriptToolMessage(callID, content)}, err
	}
	trace.Result = summarizeSkillScriptResult(command, messages, artifacts)
	return &ToolInvocationResult{
		Messages:    messages,
		Trace:       trace,
		ToolMessage: skillScriptToolMessage(callID, content),
	}, nil
}

func (r *SandboxScriptRunner) createSandbox(ctx context.Context, execCtx ExecutionContext) (string, error) {
	var response struct {
		ID string `json:"id"`
	}
	payload := map[string]interface{}{
		"runtime_profile":    "session",
		"ttl_seconds":        300,
		"network_enabled":    false,
		"network_policy":     "deny-by-default",
		"dependency_profile": "stdlib",
	}
	if organizationID := strings.TrimSpace(execCtx.OrganizationID); organizationID != "" {
		payload["organization_id"] = organizationID
	}
	if err := r.doJSON(ctx, http.MethodPost, "/v1/sandboxes", payload, &response); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.ID) == "" {
		return "", fmt.Errorf("sandbox create response did not include sandbox id")
	}
	return response.ID, nil
}

func (r *SandboxScriptRunner) uploadArchive(ctx context.Context, sandboxID string, archiveBase64 string, validateSkillManifest bool) error {
	return r.doJSON(ctx, http.MethodPost, "/v1/files/upload-archive", map[string]interface{}{
		"sandbox_id":              sandboxID,
		"path":                    ".",
		"archive_base64":          archiveBase64,
		"format":                  "zip",
		"strip_root":              false,
		"validate_skill_manifest": validateSkillManifest,
	}, nil)
}

func (r *SandboxScriptRunner) runCommand(ctx context.Context, sandboxID string, stdin string, timeoutSeconds int, execCtx ExecutionContext) (*sandboxCommandResult, error) {
	var response sandboxCommandResult
	err := r.doJSON(ctx, http.MethodPost, "/v1/exec/command", map[string]interface{}{
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
	}, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

func (r *SandboxScriptRunner) runSkill(ctx context.Context, sandboxID string, stdin string) (*sandboxSkillRunResult, error) {
	var response sandboxSkillRunResult
	err := r.doJSON(ctx, http.MethodPost, "/v1/exec/skill", map[string]interface{}{
		"sandbox_id": sandboxID,
		"path":       ".",
		"stdin":      stdin,
	}, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

func (r *SandboxScriptRunner) deleteSandbox(ctx context.Context, sandboxID string) error {
	if strings.TrimSpace(sandboxID) == "" {
		return nil
	}
	return r.doJSON(ctx, http.MethodDelete, "/v1/sandboxes/"+sandboxID, nil, nil)
}

func (r *SandboxScriptRunner) collectArtifacts(ctx context.Context, sandboxID string) ([]skillScriptArtifact, error) {
	var tree struct {
		Items []sandboxFileInfo `json:"items"`
	}
	path := "/v1/files/tree?sandbox_id=" + url.QueryEscape(sandboxID)
	if err := r.doJSON(ctx, http.MethodGet, path, nil, &tree); err != nil {
		return nil, err
	}

	artifacts := make([]skillScriptArtifact, 0)
	for _, item := range tree.Items {
		if item.IsDirectory || !strings.HasPrefix(filepath.ToSlash(item.Path), "artifacts/") {
			continue
		}
		if len(artifacts) >= maxSkillScriptArtifactCount {
			break
		}
		artifacts = append(artifacts, artifactFromFileInfo(item.Path, item.Size, ""))
	}
	return artifacts, nil
}

func (r *SandboxScriptRunner) artifactsFromManifests(ctx context.Context, sandboxID string, manifests []sandboxFileManifest) ([]skillScriptArtifact, error) {
	artifacts := make([]skillScriptArtifact, 0)
	for _, manifest := range manifests {
		for _, item := range manifest.Items {
			if len(artifacts) >= maxSkillScriptArtifactCount {
				return artifacts, nil
			}
			artifacts = append(artifacts, artifactFromFileInfo(item.Path, item.Size, item.ContentType))
		}
	}
	return artifacts, nil
}

func artifactFromFileInfo(artifactPath string, size int64, contentType string) skillScriptArtifact {
	return skillScriptArtifact{
		Path:        artifactPath,
		Name:        path.Base(filepath.ToSlash(artifactPath)),
		Size:        size,
		ContentType: strings.TrimSpace(contentType),
	}
}

func (r *SandboxScriptRunner) prepareArtifacts(ctx context.Context, sandboxID string, artifacts []skillScriptArtifact, execCtx ExecutionContext) {
	for index := range artifacts {
		r.prepareArtifact(ctx, sandboxID, &artifacts[index], execCtx)
	}
}

func (r *SandboxScriptRunner) prepareArtifact(ctx context.Context, sandboxID string, artifact *skillScriptArtifact, execCtx ExecutionContext) {
	if artifact == nil {
		return
	}
	if artifact.Size > maxSkillScriptArtifactBytes {
		artifact.Persisted = false
		artifact.Reason = "size_limit_exceeded"
		return
	}
	content, err := r.downloadArtifact(ctx, sandboxID, artifact.Path)
	if err != nil {
		artifact.Error = err.Error()
		artifact.Persisted = false
		artifact.Reason = "download_failed"
		return
	}
	if content.Encoding != "" && !strings.EqualFold(content.Encoding, "base64") {
		artifact.Error = "unsupported artifact encoding: " + content.Encoding
		artifact.Persisted = false
		artifact.Reason = "unsupported_encoding"
		return
	}
	data, err := base64.StdEncoding.DecodeString(content.Content)
	if err != nil {
		artifact.Error = err.Error()
		artifact.Persisted = false
		artifact.Reason = "decode_failed"
		return
	}
	if int64(len(data)) > maxSkillScriptArtifactBytes {
		artifact.Persisted = false
		artifact.Reason = "size_limit_exceeded"
		return
	}
	artifact.Size = int64(len(data))
	artifact.ContentType = skillArtifactMimeType(artifact.Name, artifact.ContentType, data)
	if artifact.Size <= inlineSkillArtifactMaxBytes {
		artifact.Content = content.Content
		artifact.Encoding = "base64"
	}
	persister := r.artifactPersister
	if persister == nil {
		persister = defaultSkillScriptArtifactPersister{}
	}
	fileMeta, err := persister.PersistSkillScriptArtifact(ctx, SkillScriptArtifactPersistRequest{
		ExecContext: execCtx,
		Path:        artifact.Path,
		Name:        artifact.Name,
		Size:        artifact.Size,
		ContentType: artifact.ContentType,
		Data:        data,
	})
	if err != nil {
		artifact.Error = err.Error()
		artifact.Persisted = false
		artifact.Reason = "persist_failed"
		return
	}
	artifact.Persisted = true
	artifact.File = fileMeta
}

func (r *SandboxScriptRunner) downloadArtifact(ctx context.Context, sandboxID string, path string) (*sandboxFileContent, error) {
	var content sandboxFileContent
	endpoint := "/v1/files/download?sandbox_id=" + url.QueryEscape(sandboxID) + "&path=" + url.QueryEscape(path) + "&encoding=base64"
	if err := r.doJSON(ctx, http.MethodGet, endpoint, nil, &content); err != nil {
		return nil, err
	}
	return &content, nil
}

func (r *SandboxScriptRunner) doJSON(ctx context.Context, method string, path string, payload interface{}, out interface{}) error {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
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
		return fmt.Errorf("sandbox request %s %s failed: %s", method, path, message)
	}
	if out != nil && len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return fmt.Errorf("failed to parse sandbox data: %w", err)
		}
	}
	return nil
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

type skillScriptArtifact struct {
	Path        string                 `json:"path"`
	Name        string                 `json:"name"`
	Size        int64                  `json:"size"`
	ContentType string                 `json:"content_type,omitempty"`
	Content     string                 `json:"content,omitempty"`
	Encoding    string                 `json:"encoding,omitempty"`
	Persisted   bool                   `json:"persisted"`
	Reason      string                 `json:"reason,omitempty"`
	File        map[string]interface{} `json:"file,omitempty"`
	Error       string                 `json:"error,omitempty"`
}

func hasSkillManifest(root string) (bool, error) {
	info, err := os.Lstat(filepath.Join(root, "skill.manifest.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if info.IsDir() {
		return false, fmt.Errorf("skill.manifest.json is a directory")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("skill.manifest.json is a symlink")
	}
	return true, nil
}

func zipSkillDirectoryBase64(root string) (string, error) {
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
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buffer.Bytes()), nil
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

type defaultSkillScriptArtifactPersister struct{}

func (defaultSkillScriptArtifactPersister) PersistSkillScriptArtifact(ctx context.Context, request SkillScriptArtifactPersistRequest) (map[string]interface{}, error) {
	organizationID := strings.TrimSpace(request.ExecContext.OrganizationID)
	userID := strings.TrimSpace(request.ExecContext.UserID)
	if organizationID == "" {
		return nil, fmt.Errorf("organization id is required to persist skill artifact")
	}
	if userID == "" {
		return nil, fmt.Errorf("user id is required to persist skill artifact")
	}
	if len(request.Data) == 0 {
		return nil, fmt.Errorf("skill artifact is empty")
	}

	filename := skillArtifactFilename(request.Name, request.Path)
	mimeType := skillArtifactMimeType(filename, request.ContentType, request.Data)
	extension := normalizedSkillArtifactExtension(filename, mimeType)
	conversationID := strings.TrimSpace(request.ExecContext.ConversationID)
	var conversationIDPtr *string
	if conversationID != "" {
		conversationIDPtr = &conversationID
	}

	toolFile, err := tool_file.CreateFileByRawGlobal(ctx, tool_file.CreateFileByRawParams{
		UserID:         userID,
		TenantID:       organizationID,
		ConversationID: conversationIDPtr,
		FileData:       request.Data,
		MimeType:       mimeType,
		Filename:       &filename,
		Lifecycle:      tool_file.ToolFileLifecyclePersistent,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create skill artifact file: %w", err)
	}
	if extension == "" {
		extension = normalizedSkillArtifactExtension(toolFile.Name, toolFile.MimeType)
	}
	url, err := tool_file.SignToolFileGlobal(toolFile.ID, extension)
	if err != nil {
		return nil, fmt.Errorf("failed to sign skill artifact file: %w", err)
	}
	downloadURL := appendSkillArtifactDownloadQuery(url)
	fileType := workflowfile.InferFileType(extension, mimeType)
	fileObj := workflowfile.NewFile(
		organizationID,
		fileType,
		workflowfile.FileTransferMethodToolFile,
		workflowfile.WithID(toolFile.ID),
		workflowfile.WithRelatedID(toolFile.ID),
		workflowfile.WithFilename(toolFile.Name),
		workflowfile.WithExtension(extension),
		workflowfile.WithMimeType(mimeType),
		workflowfile.WithSize(int(toolFile.Size)),
		workflowfile.WithURL(url),
	)
	fileMeta := fileObj.ToDict()
	fileMeta["url"] = url
	fileMeta["download_url"] = downloadURL
	return fileMeta, nil
}

func skillArtifactFilename(name string, artifactPath string) string {
	filename := strings.TrimSpace(name)
	if filename == "" || filename == "." || filename == "/" {
		filename = path.Base(filepath.ToSlash(artifactPath))
	}
	filename = strings.TrimSpace(strings.ReplaceAll(filename, "\\", "_"))
	filename = strings.Trim(filename, "/")
	if filename == "" || filename == "." {
		return "artifact.bin"
	}
	return filename
}

func skillArtifactMimeType(filename string, contentType string, data []byte) string {
	mimeType := strings.TrimSpace(strings.Split(contentType, ";")[0])
	extensionMimeType := ""
	if extension := path.Ext(filename); extension != "" {
		if byExtension := skillArtifactMimeTypeByExtension(extension); byExtension != "" {
			extensionMimeType = byExtension
		} else if byExtension := mime.TypeByExtension(extension); byExtension != "" {
			extensionMimeType = strings.Split(byExtension, ";")[0]
		}
	}
	if mimeType != "" {
		if extensionMimeType != "" && isGenericSkillArtifactMimeType(mimeType) {
			return extensionMimeType
		}
		return mimeType
	}
	if extensionMimeType != "" {
		return extensionMimeType
	}
	if len(data) > 0 {
		if detected := http.DetectContentType(data); detected != "" {
			return detected
		}
	}
	return "application/octet-stream"
}

func isGenericSkillArtifactMimeType(mimeType string) bool {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "text/plain", "application/octet-stream":
		return true
	default:
		return false
	}
}

func skillArtifactMimeTypeByExtension(extension string) string {
	switch strings.ToLower(strings.TrimPrefix(strings.TrimSpace(extension), ".")) {
	case "json":
		return "application/json"
	case "html", "htm":
		return "text/html"
	case "csv":
		return "text/csv"
	case "txt":
		return "text/plain"
	case "md", "markdown":
		return "text/markdown"
	case "pdf":
		return "application/pdf"
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	case "svg":
		return "image/svg+xml"
	case "zip":
		return "application/zip"
	default:
		return ""
	}
}

func normalizedSkillArtifactExtension(filename string, mimeType string) string {
	if extension := path.Ext(filename); extension != "" {
		return extension
	}
	extensions, err := mime.ExtensionsByType(mimeType)
	if err == nil && len(extensions) > 0 {
		return extensions[0]
	}
	return ".bin"
}

func appendSkillArtifactDownloadQuery(rawURL string) string {
	if strings.Contains(rawURL, "?") {
		return rawURL + "&download=1"
	}
	return rawURL + "?download=1"
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
				"path":      artifact.Path,
				"name":      artifact.Name,
				"size":      artifact.Size,
				"persisted": artifact.Persisted,
			}
			if artifact.ContentType != "" {
				item["content_type"] = artifact.ContentType
			}
			if artifact.Encoding != "" {
				item["encoding"] = artifact.Encoding
			}
			if artifact.Content != "" {
				item["content"] = artifact.Content
			}
			if artifact.Reason != "" {
				item["reason"] = artifact.Reason
			}
			if artifact.File != nil {
				messages = append(messages, tools.ToolInvokeMessage{
					Type: tools.ToolInvokeMessageTypeFile,
					Text: stringFromMap(artifact.File, "download_url"),
					Meta: map[string]interface{}{
						"file": artifact.File,
					},
				})
				item["file_id"] = stringFromMap(artifact.File, "id")
				item["download_url"] = stringFromMap(artifact.File, "download_url")
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

func stringFromMap(values map[string]interface{}, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
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

func summarizeSkillScriptResult(command *sandboxCommandResult, messages []tools.ToolInvokeMessage, artifacts []skillScriptArtifact) map[string]interface{} {
	if command == nil {
		return nil
	}
	persisted := 0
	skipped := 0
	for _, artifact := range artifacts {
		if artifact.Persisted {
			persisted++
		} else {
			skipped++
		}
	}
	return map[string]interface{}{
		"exit_code":       command.ExitCode,
		"duration_ms":     command.DurationMS,
		"truncated":       command.Truncated,
		"messages":        len(messages),
		"artifact_count":  len(artifacts),
		"persisted_count": persisted,
		"skipped_count":   skipped,
	}
}
