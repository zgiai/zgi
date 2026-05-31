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
	"mime"
	"net"
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
const inlineSkillArtifactMaxBytes = 32 * 1024
const maxSkillScriptArtifactCount = 10
const maxSkillScriptArtifactBytes = 2 * 1024 * 1024
const maxSkillScriptInputFileCount = 10
const maxSkillScriptInputFileBytes = 10 * 1024 * 1024
const defaultSkillDependencyProfile = "stdlib"

const (
	defaultSandboxConnectTimeout         = 5 * time.Second
	defaultSandboxCreateTimeout          = 10 * time.Second
	defaultSandboxUploadTimeout          = 30 * time.Second
	defaultSandboxDependencyBuildTimeout = 10 * time.Minute
	defaultSandboxCommandTimeoutPadding  = 15 * time.Second
	defaultSandboxArtifactTimeout        = 10 * time.Second
	defaultSandboxCleanupTimeout         = 5 * time.Second
	defaultSandboxIdempotentAttempts     = 3
	defaultSandboxRetryBaseDelay         = 50 * time.Millisecond
)

type SandboxScriptRunnerConfig struct {
	Endpoint               string
	APIKey                 string
	ConnectTimeout         time.Duration
	CreateTimeout          time.Duration
	UploadTimeout          time.Duration
	DependencyBuildTimeout time.Duration
	CommandTimeoutPadding  time.Duration
	ArtifactTimeout        time.Duration
	CleanupTimeout         time.Duration
	ArtifactPersister      SkillScriptArtifactPersister
	InputFileProvider      SkillScriptInputFileProvider
}

type SandboxScriptRunner struct {
	endpoint          string
	apiKey            string
	client            *http.Client
	timeouts          sandboxScriptRunnerTimeouts
	artifactPersister SkillScriptArtifactPersister
	inputFileProvider SkillScriptInputFileProvider
}

type sandboxScriptRunnerTimeouts struct {
	Create          time.Duration
	Upload          time.Duration
	DependencyBuild time.Duration
	CommandPadding  time.Duration
	Artifact        time.Duration
	Cleanup         time.Duration
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

type SkillScriptInputFile struct {
	FileID         string
	Filename       string
	Extension      string
	MimeType       string
	Size           int64
	Data           []byte
	OrganizationID string
	TenantID       string
	CreatedBy      string
	IsTemporary    bool
}

type SkillScriptInputFileProvider interface {
	GetSkillScriptInputFile(ctx context.Context, fileID string, execCtx ExecutionContext) (SkillScriptInputFile, error)
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
			Create:          durationOrDefault(config.CreateTimeout, defaultSandboxCreateTimeout),
			Upload:          durationOrDefault(config.UploadTimeout, defaultSandboxUploadTimeout),
			DependencyBuild: durationOrDefault(config.DependencyBuildTimeout, defaultSandboxDependencyBuildTimeout),
			CommandPadding:  durationOrDefault(config.CommandTimeoutPadding, defaultSandboxCommandTimeoutPadding),
			Artifact:        durationOrDefault(config.ArtifactTimeout, defaultSandboxArtifactTimeout),
			Cleanup:         durationOrDefault(config.CleanupTimeout, defaultSandboxCleanupTimeout),
		},
		artifactPersister: config.ArtifactPersister,
		inputFileProvider: config.InputFileProvider,
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

	manifest := defaultSkillScriptManifest(docTimeoutSeconds(doc))
	var manifestRaw []byte
	if hasManifest {
		preparedManifest, err := prepareSkillScriptManifest(doc.Metadata.RootPath, docTimeoutSeconds(doc))
		if err != nil {
			trace.Status = "error"
			trace.Error = err.Error()
			trace.DurationMS = time.Since(start).Milliseconds()
			return &ToolInvocationResult{Trace: trace}, err
		}
		manifest = preparedManifest.Manifest
		manifestRaw = preparedManifest.Raw
	}
	inputFiles, err := r.resolveInputFiles(ctx, arguments, execCtx, manifest)
	if err != nil {
		recordSkillScriptError(&trace, start, err)
		return &ToolInvocationResult{Trace: trace}, err
	}

	archiveBase64, err := zipSkillDirectoryBase64(doc.Metadata.RootPath, manifestRaw)
	if err != nil {
		trace.Status = "error"
		trace.Error = err.Error()
		trace.DurationMS = time.Since(start).Milliseconds()
		return &ToolInvocationResult{Trace: trace}, err
	}

	dependencyProfile, err := r.resolveDependencyProfile(ctx, execCtx, archiveBase64)
	if err != nil {
		recordSkillScriptError(&trace, start, err)
		return &ToolInvocationResult{Trace: trace}, err
	}
	if err := r.preflightDependencyProfile(ctx, execCtx, manifest.Language, dependencyProfile); err != nil {
		recordSkillScriptError(&trace, start, err)
		return &ToolInvocationResult{Trace: trace}, err
	}

	sandboxID, err := r.createSandbox(ctx, execCtx, dependencyProfile)
	if err != nil {
		recordSkillScriptError(&trace, start, err)
		return &ToolInvocationResult{Trace: trace}, err
	}
	defer func() { _ = r.deleteSandbox(context.WithoutCancel(ctx), sandboxID, execCtx) }()

	if err := r.uploadArchive(ctx, sandboxID, archiveBase64, execCtx, hasManifest); err != nil {
		recordSkillScriptError(&trace, start, err)
		return &ToolInvocationResult{Trace: trace}, err
	}

	if err := r.uploadInputFiles(ctx, sandboxID, inputFiles, execCtx); err != nil {
		recordSkillScriptError(&trace, start, err)
		return &ToolInvocationResult{Trace: trace}, err
	}

	stdinPayload, err := skillScriptStdinPayload(arguments, inputFiles)
	if err != nil {
		recordSkillScriptError(&trace, start, err)
		return &ToolInvocationResult{Trace: trace}, err
	}
	stdin, err := json.Marshal(stdinPayload)
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
		runResult, err := r.runSkill(ctx, sandboxID, string(stdin), execCtx, skillScriptTimeoutSeconds(manifest.TimeoutMS))
		if err != nil {
			recordSkillScriptError(&trace, start, err)
			return &ToolInvocationResult{Trace: trace}, err
		}
		command = &runResult.Command
		artifacts, artifactErr = r.artifactsFromManifests(runResult.ArtifactManifests, manifest)
	} else {
		timeout := skillScriptTimeoutSeconds(manifest.TimeoutMS)
		command, err = r.runCommand(ctx, sandboxID, string(stdin), timeout, execCtx)
		if err != nil {
			recordSkillScriptError(&trace, start, err)
			return &ToolInvocationResult{Trace: trace}, err
		}
		artifacts, artifactErr = r.collectArtifacts(ctx, sandboxID, execCtx, manifest)
	}

	r.prepareArtifacts(ctx, sandboxID, artifacts, execCtx, manifest.MaxArtifactBytes)
	messages, content, err := skillScriptMessages(command, artifacts, artifactErr)
	trace.DurationMS = time.Since(start).Milliseconds()
	if command.ExitCode != 0 {
		err = fmt.Errorf("skill script exited with code %d: %s", command.ExitCode, strings.TrimSpace(command.Error))
	}
	if err != nil {
		recordSkillScriptError(&trace, start, err)
		return &ToolInvocationResult{Trace: trace, Messages: messages, ToolMessage: skillScriptToolMessage(callID, content)}, err
	}
	trace.Result = summarizeSkillScriptResult(command, messages, artifacts)
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

func (r *SandboxScriptRunner) resolveDependencyProfile(ctx context.Context, execCtx ExecutionContext, archiveBase64 string) (string, error) {
	prepare, err := r.prepareDependencies(ctx, execCtx, archiveBase64)
	if err != nil {
		return "", err
	}
	switch strings.TrimSpace(prepare.Status) {
	case "", "ready":
		profile := strings.TrimSpace(prepare.ProfileName)
		if profile == "" {
			profile = defaultSkillDependencyProfile
		}
		return profile, nil
	case "build_required":
		build, err := r.queueDependencyBuild(ctx, execCtx, archiveBase64)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(build.Status) == "queued" {
			built, runErr := r.runDependencyBuild(ctx, build.Fingerprint)
			if runErr != nil {
				return "", fmt.Errorf("skill dependency build is not ready: status=%s next_action=%s fingerprint=%s profile=%s: %w", build.Status, build.NextAction, build.Fingerprint, build.ProfileName, runErr)
			}
			build = built
		}
		if strings.TrimSpace(build.Status) == "ready" {
			profile := strings.TrimSpace(build.ProfileName)
			if profile == "" {
				profile = defaultSkillDependencyProfile
			}
			return profile, nil
		}
		return "", fmt.Errorf("skill dependency build is not ready: status=%s next_action=%s fingerprint=%s profile=%s", build.Status, build.NextAction, build.Fingerprint, build.ProfileName)
	default:
		return "", fmt.Errorf("skill dependency preparation returned unsupported status: %s", prepare.Status)
	}
}

func (r *SandboxScriptRunner) prepareDependencies(ctx context.Context, execCtx ExecutionContext, archiveBase64 string) (*sandboxDependencyBuildResponse, error) {
	var response sandboxDependencyBuildResponse
	payload := dependencyPreparePayload(execCtx, archiveBase64)
	if err := r.doJSON(ctx, http.MethodPost, "/v1/sandbox/dependencies/prepare", payload, &response, r.timeouts.Upload); err != nil {
		return nil, fmt.Errorf("skill dependency preparation failed: %w", err)
	}
	return &response, nil
}

func (r *SandboxScriptRunner) queueDependencyBuild(ctx context.Context, execCtx ExecutionContext, archiveBase64 string) (*sandboxDependencyBuildResponse, error) {
	var response sandboxDependencyBuildResponse
	payload := dependencyPreparePayload(execCtx, archiveBase64)
	if err := r.doJSON(ctx, http.MethodPost, "/v1/sandbox/dependencies/builds", payload, &response, r.timeouts.Upload); err != nil {
		return nil, fmt.Errorf("skill dependency build request failed: %w", err)
	}
	return &response, nil
}

func (r *SandboxScriptRunner) runDependencyBuild(ctx context.Context, fingerprint string) (*sandboxDependencyBuildResponse, error) {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return nil, fmt.Errorf("dependency build fingerprint is required")
	}
	var response sandboxDependencyBuildResponse
	endpoint := "/v1/sandbox/dependencies/builds/" + url.PathEscape(fingerprint) + "/run"
	if err := r.doJSON(ctx, http.MethodPost, endpoint, nil, &response, r.timeouts.DependencyBuild); err != nil {
		return nil, fmt.Errorf("skill dependency build run failed: %w", err)
	}
	return &response, nil
}

func dependencyPreparePayload(execCtx ExecutionContext, archiveBase64 string) map[string]interface{} {
	payload := map[string]interface{}{
		"archive_base64": archiveBase64,
		"format":         "zip",
		"strip_root":     false,
		"base_runtime":   "linux-secure",
	}
	if organizationID := strings.TrimSpace(execCtx.OrganizationID); organizationID != "" {
		payload["organization_id"] = organizationID
	}
	return payload
}

func (r *SandboxScriptRunner) preflightDependencyProfile(ctx context.Context, execCtx ExecutionContext, language string, dependencyProfile string) error {
	profile := strings.TrimSpace(dependencyProfile)
	if profile == "" {
		profile = defaultSkillDependencyProfile
	}
	language = normalizeSkillScriptLanguage(language)
	if language == "" {
		language = "python3"
	}

	var catalog sandboxDependencyCatalog
	endpoint := "/v1/sandbox/dependencies?language=" + url.QueryEscape(language)
	endpoint = withOrganizationQuery(endpoint, execCtx)
	if err := r.doIdempotentJSON(ctx, http.MethodGet, endpoint, nil, &catalog, r.timeouts.Create); err != nil {
		return fmt.Errorf("skill dependency profile preflight failed: %w", err)
	}

	for _, item := range catalog.Profiles {
		if item.Name != profile {
			continue
		}
		if !item.Enabled || item.Status != "ready" {
			return fmt.Errorf("skill dependency profile preflight failed: dependency profile is not ready: %s", profile)
		}
		return nil
	}
	return fmt.Errorf("skill dependency profile preflight failed: unsupported dependency profile for %s: %s", language, profile)
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

type preparedSkillInputFile struct {
	Name     string
	Path     string
	FileID   string
	Filename string
	MimeType string
	Size     int64
	Data     []byte
	Multiple bool
}

func (r *SandboxScriptRunner) resolveInputFiles(ctx context.Context, arguments map[string]interface{}, execCtx ExecutionContext, manifest skillScriptManifest) ([]preparedSkillInputFile, error) {
	if len(manifest.InputFiles) == 0 {
		return nil, nil
	}
	if r.inputFileProvider == nil {
		return nil, fmt.Errorf("skill script input file provider is not configured")
	}
	prepared := make([]preparedSkillInputFile, 0, len(manifest.InputFiles))
	for _, spec := range manifest.InputFiles {
		fileIDs, ok := skillScriptInputFileIDs(arguments, spec.Argument)
		if !ok || len(fileIDs) == 0 {
			if spec.Required {
				return nil, fmt.Errorf("skill input file %s requires argument %s", spec.Name, spec.Argument)
			}
			continue
		}
		if !spec.Multiple && len(fileIDs) > 1 {
			return nil, fmt.Errorf("skill input file %s accepts one file, got %d", spec.Name, len(fileIDs))
		}
		if spec.Multiple && spec.MaxCount > 0 && len(fileIDs) > spec.MaxCount {
			return nil, fmt.Errorf("skill input file %s accepts at most %d files", spec.Name, spec.MaxCount)
		}
		for _, fileID := range fileIDs {
			inputFile, err := r.inputFileProvider.GetSkillScriptInputFile(ctx, fileID, execCtx)
			if err != nil {
				return nil, fmt.Errorf("failed to load skill input file %s: %w", spec.Name, err)
			}
			normalized, err := prepareSkillInputFile(spec, inputFile)
			if err != nil {
				return nil, err
			}
			prepared = append(prepared, normalized)
			if len(prepared) > maxSkillScriptInputFileCount {
				return nil, fmt.Errorf("skill script accepts at most %d input files", maxSkillScriptInputFileCount)
			}
		}
	}
	return prepared, nil
}

func (r *SandboxScriptRunner) uploadInputFiles(ctx context.Context, sandboxID string, files []preparedSkillInputFile, execCtx ExecutionContext) error {
	if len(files) == 0 {
		return nil
	}
	archiveBase64, err := zipSkillInputFilesBase64(files)
	if err != nil {
		return err
	}
	if err := r.uploadArchive(ctx, sandboxID, archiveBase64, execCtx, false); err != nil {
		return fmt.Errorf("failed to upload skill input files: %w", err)
	}
	return nil
}

func skillScriptInputFileIDs(arguments map[string]interface{}, argument string) ([]string, bool) {
	if arguments == nil {
		return nil, false
	}
	value, ok := arguments[argument]
	if !ok || value == nil {
		return nil, false
	}
	ids := []string{}
	appendID := func(raw interface{}) {
		fileID := ""
		switch typed := raw.(type) {
		case string:
			fileID = strings.TrimSpace(typed)
		default:
			fileID = strings.TrimSpace(fmt.Sprint(typed))
		}
		if fileID != "" {
			ids = append(ids, fileID)
		}
	}
	switch typed := value.(type) {
	case string:
		appendID(typed)
	case []string:
		for _, item := range typed {
			appendID(item)
		}
	case []interface{}:
		for _, item := range typed {
			appendID(item)
		}
	default:
		appendID(typed)
	}
	if len(ids) == 0 {
		return nil, false
	}
	return ids, true
}

func prepareSkillInputFile(spec skillScriptInputFileSpec, input SkillScriptInputFile) (preparedSkillInputFile, error) {
	dataSize := int64(len(input.Data))
	size := input.Size
	if size <= 0 {
		size = dataSize
	}
	if size > spec.MaxBytes || dataSize > spec.MaxBytes {
		return preparedSkillInputFile{}, fmt.Errorf("skill input file %s exceeds max_bytes %d", spec.Name, spec.MaxBytes)
	}
	filename := safeSkillInputFilename(input.Filename, input.FileID, input.Extension)
	extension := strings.ToLower(path.Ext(filename))
	if len(spec.Extensions) > 0 && !stringInList(extension, spec.Extensions) {
		return preparedSkillInputFile{}, fmt.Errorf("skill input file %s extension %s is not allowed", spec.Name, extension)
	}
	mimeType := strings.ToLower(strings.TrimSpace(strings.Split(input.MimeType, ";")[0]))
	if mimeType == "" {
		mimeType = skillArtifactMimeType(filename, "", input.Data)
	}
	if len(spec.MimeTypes) > 0 && !stringInList(mimeType, spec.MimeTypes) {
		return preparedSkillInputFile{}, fmt.Errorf("skill input file %s mime type %s is not allowed", spec.Name, mimeType)
	}
	fileID := strings.TrimSpace(input.FileID)
	inputPath := "inputs/" + spec.Name + "/" + filename
	if spec.Multiple {
		inputPath = "inputs/" + spec.Name + "/" + safeSkillInputPathSegment(fileID) + "/" + filename
	}
	return preparedSkillInputFile{
		Name:     spec.Name,
		Path:     inputPath,
		FileID:   fileID,
		Filename: filename,
		MimeType: mimeType,
		Size:     dataSize,
		Data:     input.Data,
		Multiple: spec.Multiple,
	}, nil
}

func safeSkillInputPathSegment(value string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			builder.WriteRune(r)
		}
	}
	out := builder.String()
	if out == "" || out == "." || out == ".." {
		return "input"
	}
	return out
}

func safeSkillInputFilename(filename string, fileID string, extension string) string {
	name := filepath.Base(filepath.ToSlash(strings.TrimSpace(filename)))
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.Trim(name, " ./")
	if name == "" || name == "." {
		ext := strings.TrimSpace(extension)
		if ext != "" && !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		name = strings.TrimSpace(fileID)
		if name == "" {
			name = "input"
		}
		name += ext
	}
	if name == "" || name == "." || name == ".." {
		return "input.bin"
	}
	return name
}

func zipSkillInputFilesBase64(files []preparedSkillInputFile) (string, error) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, file := range files {
		if unsafeSkillManifestPath(file.Path) || !strings.HasPrefix(file.Path, "inputs/") {
			_ = writer.Close()
			return "", fmt.Errorf("skill input file path is invalid: %s", file.Path)
		}
		header := &zip.FileHeader{
			Name:   filepath.ToSlash(file.Path),
			Method: zip.Deflate,
		}
		header.SetMode(0o644)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			_ = writer.Close()
			return "", err
		}
		if _, err := entry.Write(file.Data); err != nil {
			_ = writer.Close()
			return "", err
		}
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buffer.Bytes()), nil
}

func skillScriptStdinPayload(arguments map[string]interface{}, inputFiles []preparedSkillInputFile) (map[string]interface{}, error) {
	if len(inputFiles) == 0 {
		return arguments, nil
	}
	if _, exists := arguments["input_files"]; exists {
		return nil, fmt.Errorf("skill script argument input_files is reserved")
	}
	payload := make(map[string]interface{}, len(arguments)+1)
	for key, value := range arguments {
		payload[key] = value
	}
	files := make(map[string]interface{}, len(inputFiles))
	for _, inputFile := range inputFiles {
		item := map[string]interface{}{
			"path":      inputFile.Path,
			"file_id":   inputFile.FileID,
			"filename":  inputFile.Filename,
			"mime_type": inputFile.MimeType,
			"size":      inputFile.Size,
		}
		if inputFile.Multiple {
			current, _ := files[inputFile.Name].([]map[string]interface{})
			files[inputFile.Name] = append(current, item)
			continue
		}
		files[inputFile.Name] = item
	}
	payload["input_files"] = files
	return payload, nil
}

func stringInList(value string, allowed []string) bool {
	for _, item := range allowed {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(item)) {
			return true
		}
	}
	return false
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

func (r *SandboxScriptRunner) collectArtifacts(ctx context.Context, sandboxID string, execCtx ExecutionContext, manifest skillScriptManifest) ([]skillScriptArtifact, error) {
	var tree struct {
		Items []sandboxFileInfo `json:"items"`
	}
	path := withOrganizationQuery("/v1/files/tree?sandbox_id="+url.QueryEscape(sandboxID), execCtx)
	if err := r.doIdempotentJSON(ctx, http.MethodGet, path, nil, &tree, r.timeouts.Artifact); err != nil {
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
		artifacts = append(artifacts, artifactFromFileInfo(item.Path, item.Size, ""))
	}
	return artifacts, nil
}

func (r *SandboxScriptRunner) artifactsFromManifests(manifests []sandboxFileManifest, manifest skillScriptManifest) ([]skillScriptArtifact, error) {
	artifacts := make([]skillScriptArtifact, 0)
	for _, fileManifest := range manifests {
		for _, item := range fileManifest.Items {
			if !skillManifestAllowsArtifactPath(manifest, item.Path) {
				continue
			}
			if len(artifacts) >= manifest.MaxArtifactCount {
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

func (r *SandboxScriptRunner) prepareArtifacts(ctx context.Context, sandboxID string, artifacts []skillScriptArtifact, execCtx ExecutionContext, maxArtifactBytes int64) {
	if maxArtifactBytes <= 0 || maxArtifactBytes > maxSkillScriptArtifactBytes {
		maxArtifactBytes = maxSkillScriptArtifactBytes
	}
	for index := range artifacts {
		r.prepareArtifact(ctx, sandboxID, &artifacts[index], execCtx, maxArtifactBytes)
	}
}

func (r *SandboxScriptRunner) prepareArtifact(ctx context.Context, sandboxID string, artifact *skillScriptArtifact, execCtx ExecutionContext, maxArtifactBytes int64) {
	if artifact == nil {
		return
	}
	if artifact.Size > maxArtifactBytes {
		artifact.Persisted = false
		artifact.Reason = "size_limit_exceeded"
		return
	}
	content, err := r.downloadArtifact(ctx, sandboxID, artifact.Path, execCtx)
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
	if int64(len(data)) > maxArtifactBytes {
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

type sandboxDependencyBuildResponse struct {
	BuildID          string `json:"build_id"`
	Fingerprint      string `json:"fingerprint"`
	Status           string `json:"status"`
	NextAction       string `json:"next_action"`
	ProfileName      string `json:"profile_name"`
	ArtifactChecksum string `json:"artifact_checksum"`
	PackageCount     int    `json:"package_count"`
	Error            string `json:"error"`
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
	Entrypoint           string                     `json:"entrypoint"`
	Language             string                     `json:"language"`
	DependencyProfile    string                     `json:"dependency_profile,omitempty"`
	TimeoutMS            int                        `json:"timeout_ms"`
	AllowedArtifactPaths []string                   `json:"allowed_artifact_paths"`
	MaxArtifactCount     int                        `json:"max_artifact_count"`
	MaxArtifactBytes     int64                      `json:"max_artifact_bytes"`
	ResultMode           string                     `json:"result_mode"`
	InputFiles           []skillScriptInputFileSpec `json:"input_files,omitempty"`
}

type skillScriptInputFileSpec struct {
	Name       string   `json:"name"`
	Argument   string   `json:"argument"`
	Required   bool     `json:"required"`
	Multiple   bool     `json:"multiple,omitempty"`
	MaxCount   int      `json:"max_count,omitempty"`
	Extensions []string `json:"extensions,omitempty"`
	MimeTypes  []string `json:"mime_types,omitempty"`
	MaxBytes   int64    `json:"max_bytes,omitempty"`
}

type preparedSkillScriptManifest struct {
	Manifest skillScriptManifest
	Raw      []byte
}

func defaultSkillScriptManifest(fallbackTimeoutSeconds int) skillScriptManifest {
	timeoutSeconds := fallbackTimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultSkillScriptTimeoutSeconds
	}
	return skillScriptManifest{
		Entrypoint:           "scripts/run.py",
		Language:             "python3",
		TimeoutMS:            timeoutSeconds * 1000,
		AllowedArtifactPaths: []string{"artifacts"},
		MaxArtifactCount:     maxSkillScriptArtifactCount,
		MaxArtifactBytes:     maxSkillScriptArtifactBytes,
		ResultMode:           "mixed",
	}
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
	if unsafeSkillManifestPath(manifest.Entrypoint) {
		return fmt.Errorf("skill manifest entrypoint escapes package root: %s", manifest.Entrypoint)
	}
	if manifest.Entrypoint == "scripts" || !strings.HasPrefix(manifest.Entrypoint, "scripts/") {
		return fmt.Errorf("skill manifest entrypoint must be under scripts/: %s", manifest.Entrypoint)
	}
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(manifest.Entrypoint)))
	if err != nil || info.IsDir() {
		return fmt.Errorf("skill manifest entrypoint is missing from package: %s", manifest.Entrypoint)
	}

	manifest.Language = normalizeSkillScriptLanguage(manifest.Language)
	if manifest.Language == "" {
		manifest.Language = "python3"
	}
	if manifest.Language != "python3" && manifest.Language != "nodejs" {
		return fmt.Errorf("skill manifest language must be python3 or nodejs for API run_script: %s", manifest.Language)
	}

	// Dependency profiles are selected by the platform from prepared dependency
	// requests and verified artifacts. Skill packages may declare dependencies,
	// but they must not choose the runtime profile directly.
	manifest.DependencyProfile = ""
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
		manifest.MaxArtifactBytes = maxSkillScriptArtifactBytes
	}
	if manifest.MaxArtifactBytes > maxSkillScriptArtifactBytes {
		return fmt.Errorf("skill manifest max_artifact_bytes must be between 1 and %d", maxSkillScriptArtifactBytes)
	}
	manifest.ResultMode = strings.TrimSpace(manifest.ResultMode)
	if manifest.ResultMode == "" {
		manifest.ResultMode = "mixed"
	}
	switch manifest.ResultMode {
	case "stdout_json", "stdout_text", "artifacts", "mixed":
	default:
		return fmt.Errorf("skill manifest result_mode must be stdout_json, stdout_text, artifacts, or mixed")
	}
	if len(manifest.InputFiles) > maxSkillScriptInputFileCount {
		return fmt.Errorf("skill manifest input_files must contain at most %d files", maxSkillScriptInputFileCount)
	}
	for index := range manifest.InputFiles {
		if err := normalizeSkillScriptInputFileSpec(&manifest.InputFiles[index]); err != nil {
			return err
		}
	}
	return nil
}

func normalizeSkillScriptInputFileSpec(spec *skillScriptInputFileSpec) error {
	if spec == nil {
		return fmt.Errorf("skill manifest input_files contains an empty item")
	}
	spec.Name = strings.TrimSpace(spec.Name)
	if !safeSkillInputSegment(spec.Name) {
		return fmt.Errorf("skill manifest input file name must be a safe path segment: %s", spec.Name)
	}
	spec.Argument = strings.TrimSpace(spec.Argument)
	if spec.Argument == "" {
		return fmt.Errorf("skill manifest input file %s argument is required", spec.Name)
	}
	if strings.ContainsAny(spec.Argument, "/\\") || spec.Argument == "." || spec.Argument == ".." {
		return fmt.Errorf("skill manifest input file %s argument is invalid: %s", spec.Name, spec.Argument)
	}
	for i, extension := range spec.Extensions {
		normalized := strings.ToLower(strings.TrimSpace(extension))
		if normalized == "" {
			return fmt.Errorf("skill manifest input file %s extension is empty", spec.Name)
		}
		if !strings.HasPrefix(normalized, ".") {
			normalized = "." + normalized
		}
		if strings.ContainsAny(normalized, `/\`) || normalized == "." || strings.Contains(normalized, "..") {
			return fmt.Errorf("skill manifest input file %s extension is invalid: %s", spec.Name, extension)
		}
		spec.Extensions[i] = normalized
	}
	for i, mimeType := range spec.MimeTypes {
		normalized := strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
		if normalized == "" || strings.ContainsAny(normalized, " \t\r\n") {
			return fmt.Errorf("skill manifest input file %s mime type is invalid: %s", spec.Name, mimeType)
		}
		spec.MimeTypes[i] = normalized
	}
	if spec.MaxBytes <= 0 {
		spec.MaxBytes = maxSkillScriptInputFileBytes
	}
	if spec.MaxBytes > maxSkillScriptInputFileBytes {
		return fmt.Errorf("skill manifest input file %s max_bytes must be between 1 and %d", spec.Name, maxSkillScriptInputFileBytes)
	}
	if spec.Multiple {
		if spec.MaxCount <= 0 {
			spec.MaxCount = maxSkillScriptInputFileCount
		}
		if spec.MaxCount > maxSkillScriptInputFileCount {
			return fmt.Errorf("skill manifest input file %s max_count must be between 1 and %d", spec.Name, maxSkillScriptInputFileCount)
		}
	} else if spec.MaxCount < 0 {
		return fmt.Errorf("skill manifest input file %s max_count must not be negative", spec.Name)
	}
	return nil
}

func safeSkillInputSegment(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	if strings.ContainsAny(value, `/\`) {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
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
	case "node", "nodejs", "javascript":
		return "nodejs"
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
	case "text/plain", "application/octet-stream", "application/zip", "application/x-zip-compressed":
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
	case "doc":
		return "application/msword"
	case "docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case "xls":
		return "application/vnd.ms-excel"
	case "xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case "ppt":
		return "application/vnd.ms-powerpoint"
	case "pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
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
