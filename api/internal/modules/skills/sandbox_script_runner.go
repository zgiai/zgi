package skills

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
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

type SandboxScriptRunnerConfig struct {
	Endpoint string
	APIKey   string
}

type SandboxScriptRunner struct {
	endpoint string
	apiKey   string
	client   *http.Client
}

func NewSandboxScriptRunner(config SandboxScriptRunnerConfig) *SandboxScriptRunner {
	endpoint := strings.TrimRight(strings.TrimSpace(config.Endpoint), "/")
	if endpoint == "" {
		return nil
	}
	return &SandboxScriptRunner{
		endpoint: endpoint,
		apiKey:   strings.TrimSpace(config.APIKey),
		client:   &http.Client{Timeout: 60 * time.Second},
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

	dependencyProfile, err := skillDependencyProfile(doc.Metadata.RootPath)
	if err != nil {
		trace.Status = "error"
		trace.Error = err.Error()
		trace.DurationMS = time.Since(start).Milliseconds()
		return &ToolInvocationResult{Trace: trace}, err
	}

	sandboxID, err := r.createSandbox(ctx, execCtx, dependencyProfile)
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
	if err := r.uploadArchive(ctx, sandboxID, archiveBase64); err != nil {
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
	timeout := docTimeoutSeconds(doc)
	if timeout <= 0 {
		timeout = defaultSkillScriptTimeoutSeconds
	}
	command, err := r.runCommand(ctx, sandboxID, string(stdin), timeout, execCtx)
	if err != nil {
		trace.Status = "error"
		trace.Error = err.Error()
		trace.DurationMS = time.Since(start).Milliseconds()
		return &ToolInvocationResult{Trace: trace}, err
	}

	artifacts, artifactErr := r.collectArtifacts(ctx, sandboxID)
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
	if err := r.doJSON(ctx, http.MethodPost, "/v1/sandboxes", payload, &response); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.ID) == "" {
		return "", fmt.Errorf("sandbox create response did not include sandbox id")
	}
	return response.ID, nil
}

func (r *SandboxScriptRunner) uploadArchive(ctx context.Context, sandboxID string, archiveBase64 string) error {
	return r.doJSON(ctx, http.MethodPost, "/v1/files/upload-archive", map[string]interface{}{
		"sandbox_id":     sandboxID,
		"path":           ".",
		"archive_base64": archiveBase64,
		"format":         "zip",
		"strip_root":     false,
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
		if len(artifacts) >= 10 {
			break
		}
		artifact := skillScriptArtifact{
			Path: item.Path,
			Name: filepath.Base(item.Path),
			Size: item.Size,
		}
		if item.Size <= 32*1024 {
			content, err := r.downloadArtifact(ctx, sandboxID, item.Path)
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

type skillScriptManifest struct {
	DependencyProfile string `json:"dependency_profile"`
}

func skillDependencyProfile(root string) (string, error) {
	manifestPath := filepath.Join(root, "skill.manifest.json")
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultSkillDependencyProfile, nil
		}
		return "", fmt.Errorf("failed to read skill manifest: %w", err)
	}
	var manifest skillScriptManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return "", fmt.Errorf("invalid skill.manifest.json: %w", err)
	}
	profile := strings.TrimSpace(manifest.DependencyProfile)
	if profile == "" {
		return defaultSkillDependencyProfile, nil
	}
	return profile, nil
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
