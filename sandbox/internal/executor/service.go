package executor

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"text/template/parse"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/lifecycle"
	"github.com/zgiai/zgi-sandbox/internal/observer"
	"github.com/zgiai/zgi-sandbox/internal/policy"
	"github.com/zgiai/zgi-sandbox/internal/runner"
	"github.com/zgiai/zgi-sandbox/internal/sandbox"
)

type Service struct {
	lifecycle *lifecycle.Manager
	runner    *runner.Service
	observer  *observer.Recorder
	policy    *policy.Service

	executionMu                    sync.Mutex
	executionChanged               chan struct{}
	activeExecutions               int
	activeExecutionsByProfile      map[string]int
	activeExecutionsByOrganization map[string]int
	queuedExecutionsByOrganization map[string]int
}

type CodeRequest struct {
	SandboxID            string          `json:"sandbox_id"`
	OrganizationID       string          `json:"organization_id,omitempty"`
	Language             string          `json:"language"`
	Code                 string          `json:"code"`
	Preload              string          `json:"preload"`
	InputJSON            json.RawMessage `json:"input_json,omitempty"`
	ExpectedOutputSchema json.RawMessage `json:"expected_output_schema,omitempty"`
	Profile              string          `json:"profile,omitempty"`
	TimeoutSeconds       int             `json:"timeout_seconds,omitempty"`
	TimeoutMS            int             `json:"timeout_ms,omitempty"`
	StdoutLimitKB        int             `json:"stdout_limit_kb,omitempty"`
	StderrLimitKB        int             `json:"stderr_limit_kb,omitempty"`
	StrictResultJSON     bool            `json:"strict_result_json,omitempty"`
	BindWorkspace        bool            `json:"bind_workspace,omitempty"`
	EnableNetwork        bool            `json:"enable_network"`
}

type CommandRequest struct {
	SandboxID      string            `json:"sandbox_id"`
	OrganizationID string            `json:"organization_id,omitempty"`
	Command        string            `json:"command"`
	Args           []string          `json:"args"`
	Stdin          string            `json:"stdin"`
	Env            map[string]string `json:"env"`
	Profile        string            `json:"profile"`
	TimeoutSeconds int               `json:"timeout_seconds"`
	TimeoutMS      int               `json:"timeout_ms"`
	StdoutLimitKB  int               `json:"stdout_limit_kb"`
	StderrLimitKB  int               `json:"stderr_limit_kb"`
	WorkingSubpath string            `json:"working_subpath"`
}

type TemplateRequest struct {
	Engine         string         `json:"engine"`
	Template       string         `json:"template"`
	Variables      map[string]any `json:"variables"`
	Profile        string         `json:"profile"`
	TimeoutMS      int            `json:"timeout_ms"`
	OutputLimitKB  int            `json:"output_limit_kb"`
	OrganizationID string         `json:"organization_id,omitempty"`
	WorkspaceID    string         `json:"workspace_id,omitempty"`
	AppID          string         `json:"app_id,omitempty"`
	WorkflowRunID  string         `json:"workflow_run_id,omitempty"`
	UserID         string         `json:"user_id,omitempty"`
}

type TemplateResult struct {
	ExecutionID string   `json:"execution_id"`
	Content     string   `json:"content"`
	DurationMS  int64    `json:"duration_ms"`
	Truncated   bool     `json:"truncated"`
	Warnings    []string `json:"warnings,omitempty"`
}

type SkillRunRequest struct {
	SandboxID      string            `json:"sandbox_id"`
	OrganizationID string            `json:"organization_id,omitempty"`
	Path           string            `json:"path"`
	InputJSON      json.RawMessage   `json:"input_json,omitempty"`
	Stdin          string            `json:"stdin,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
}

type SkillRunResult struct {
	ExecutionID       string                 `json:"execution_id"`
	SandboxID         string                 `json:"sandbox_id"`
	Path              string                 `json:"path"`
	Manifest          SkillExecutionManifest `json:"manifest"`
	Command           runner.CommandResult   `json:"command"`
	ArtifactManifests []FileManifest         `json:"artifact_manifests,omitempty"`
	ResultJSON        any                    `json:"result_json,omitempty"`
}

type FileWriteRequest struct {
	SandboxID      string `json:"sandbox_id"`
	OrganizationID string `json:"organization_id,omitempty"`
	Path           string `json:"path"`
	Content        string `json:"content"`
	Encoding       string `json:"encoding"`
}

type ArchiveUploadRequest struct {
	SandboxID             string `json:"sandbox_id"`
	OrganizationID        string `json:"organization_id,omitempty"`
	Path                  string `json:"path"`
	ArchiveBase64         string `json:"archive_base64"`
	Format                string `json:"format"`
	StripRoot             bool   `json:"strip_root"`
	ValidateSkillManifest bool   `json:"validate_skill_manifest"`
}

type ArchiveUploadResult struct {
	SandboxID     string                  `json:"sandbox_id"`
	Path          string                  `json:"path"`
	Files         []FileInfo              `json:"files"`
	FileCount     int                     `json:"file_count"`
	TotalSize     int64                   `json:"total_size"`
	SkillManifest *SkillExecutionManifest `json:"skill_manifest,omitempty"`
}

type SkillExecutionManifest struct {
	Entrypoint           string   `json:"entrypoint"`
	Language             string   `json:"language"`
	DependencyProfile    string   `json:"dependency_profile"`
	TimeoutMS            int      `json:"timeout_ms"`
	AllowedArtifactPaths []string `json:"allowed_artifact_paths"`
	MaxArtifactCount     int      `json:"max_artifact_count"`
	MaxArtifactBytes     int64    `json:"max_artifact_bytes"`
	ResultMode           string   `json:"result_mode"`
}

type FileInfo struct {
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	Mode        string    `json:"mode"`
	ModifiedAt  time.Time `json:"modified_at"`
	IsDirectory bool      `json:"is_directory"`
	SandboxID   string    `json:"sandbox_id"`
}

type FileContent struct {
	SandboxID string `json:"sandbox_id"`
	Path      string `json:"path"`
	Content   string `json:"content"`
	Encoding  string `json:"encoding"`
}

type FileManifest struct {
	SandboxID string             `json:"sandbox_id"`
	Path      string             `json:"path"`
	Items     []FileManifestItem `json:"items"`
	FileCount int                `json:"file_count"`
	TotalSize int64              `json:"total_size"`
	Truncated bool               `json:"truncated"`
}

type FileManifestItem struct {
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	Encoding    string    `json:"encoding"`
	SHA256      string    `json:"sha256"`
	ContentType string    `json:"content_type"`
	ModifiedAt  time.Time `json:"modified_at"`
}

type FileManifestOptions struct {
	MaxFiles      int
	MaxTotalBytes int64
}

type DependencyInstallError struct {
	PackageManager string
	Action         string
	Command        string
}

func (e *DependencyInstallError) Error() string {
	return "runtime dependency installation is disabled for managed dependency profiles"
}

func (e *DependencyInstallError) ResponseDetails() map[string]any {
	details := map[string]any{
		"error_type": "policy_denied",
		"code":       "dependency_install_disabled",
	}
	if e == nil {
		return details
	}
	if e.PackageManager != "" {
		details["package_manager"] = e.PackageManager
	}
	if e.Action != "" {
		details["action"] = e.Action
	}
	if e.Command != "" {
		details["command"] = e.Command
	}
	return details
}

func NewService(manager *lifecycle.Manager, runnerService *runner.Service, recorder *observer.Recorder, policyService *policy.Service) *Service {
	return &Service{
		lifecycle:                      manager,
		runner:                         runnerService,
		observer:                       recorder,
		policy:                         policyService,
		executionChanged:               make(chan struct{}),
		activeExecutionsByProfile:      make(map[string]int),
		activeExecutionsByOrganization: make(map[string]int),
		queuedExecutionsByOrganization: make(map[string]int),
	}
}

func (s *Service) RunCode(ctx context.Context, req CodeRequest) (runner.Result, error) {
	executionID := newExecutionID()
	requestedProfile := defaultString(req.Profile, "code-short")
	baseMetadata := map[string]any{
		"execution_id": executionID,
		"language":     req.Language,
		"profile":      requestedProfile,
	}
	limits, err := s.policy.NormalizeCommandLimits(requestedProfile, req.TimeoutSeconds, req.TimeoutMS, req.StdoutLimitKB, req.StderrLimitKB)
	if err != nil {
		s.recordExecutionFailure(ctx, "exec.code.failed", req.SandboxID, "sandbox code execution failed", baseMetadata, err)
		return runner.Result{}, err
	}
	if len(req.InputJSON) > limits.MaxStdinBytes {
		err := fmt.Errorf("input_json exceeds max size of %d bytes", limits.MaxStdinBytes)
		s.recordExecutionFailure(ctx, "exec.code.failed", req.SandboxID, "sandbox code execution failed", baseMetadata, err)
		return runner.Result{}, err
	}
	outputSchema, err := parseExpectedOutputSchema(req.ExpectedOutputSchema)
	if err != nil {
		s.recordExecutionFailure(ctx, "exec.code.failed", req.SandboxID, "sandbox code execution failed", baseMetadata, err)
		return runner.Result{}, err
	}

	runReq := runner.Request{
		Language:      req.Language,
		Code:          req.Code,
		Preload:       req.Preload,
		Stdin:         string(req.InputJSON),
		EnableNetwork: req.EnableNetwork,
	}

	result, box, workspaceBound, err := s.runCodeWithScope(ctx, req, runReq, limits)
	if err != nil {
		addExecutionSandboxMetadata(baseMetadata, box)
		s.recordExecutionFailure(ctx, "exec.code.failed", req.SandboxID, "sandbox code execution failed", baseMetadata, err)
		return runner.Result{}, err
	}
	if workspaceBound {
		if err := s.enforceWorkspaceByteLimit(box); err != nil {
			addExecutionSandboxMetadata(baseMetadata, box)
			s.recordExecutionFailure(ctx, "exec.code.failed", req.SandboxID, "sandbox code execution failed", baseMetadata, err)
			return runner.Result{}, err
		}
		if err := s.enforceWorkspaceFileLimit(box); err != nil {
			addExecutionSandboxMetadata(baseMetadata, box)
			s.recordExecutionFailure(ctx, "exec.code.failed", req.SandboxID, "sandbox code execution failed", baseMetadata, err)
			return runner.Result{}, err
		}
	}
	if err := attachResultJSON(&result, req.StrictResultJSON, outputSchema, limits.MaxResultJSONBytes); err != nil {
		addExecutionSandboxMetadata(baseMetadata, box)
		s.recordExecutionFailure(ctx, "exec.code.failed", req.SandboxID, "sandbox code execution failed", baseMetadata, err)
		return runner.Result{}, err
	}
	result.ExecutionID = executionID

	metadata := map[string]any{
		"execution_id":    executionID,
		"language":        req.Language,
		"profile":         limits.Profile,
		"exit_code":       result.ExitCode,
		"duration_ms":     result.DurationMS,
		"truncated":       result.Truncated,
		"backend":         result.Backend,
		"runtime_backend": result.Backend,
		"status":          "success",
		"stateless":       limits.Stateless,
		"workspace_bound": workspaceBound,
	}
	addExecutionSandboxMetadata(metadata, box)
	s.observer.Record("exec.code", req.SandboxID, "sandbox code executed", observer.MetadataWithContext(ctx, metadata))
	return result, nil
}

func (s *Service) runCodeWithScope(ctx context.Context, req CodeRequest, runReq runner.Request, limits policy.CommandLimits) (runner.Result, *sandbox.Sandbox, bool, error) {
	if strings.TrimSpace(req.SandboxID) == "" {
		if req.EnableNetwork {
			return runner.Result{}, nil, false, errors.New("network access is disabled for stateless code execution")
		}
		result, err := s.runner.RunWithLimits(ctx, runReq, limits.Timeout, limits.StdoutLimitBytes, limits.StderrLimitBytes)
		return result, nil, false, err
	}

	box, err := s.lifecycle.GetActive(req.SandboxID)
	if err != nil {
		return runner.Result{}, nil, false, err
	}
	if limits.Stateless && !req.BindWorkspace && req.EnableNetwork {
		return runner.Result{}, box, false, errors.New("network access is disabled for stateless code execution")
	}
	if err := s.policy.ValidateCommandProfileNetwork(limits, req.EnableNetwork); err != nil {
		return runner.Result{}, box, false, err
	}
	if err := s.policy.ValidateCodeExecution(*box, req.EnableNetwork); err != nil {
		return runner.Result{}, box, false, err
	}
	runReq.DependencyProfile = box.DependencyProfile
	if err := s.enforceOrganizationExecutionRate(box); err != nil {
		return runner.Result{}, box, false, err
	}
	releaseExecution, err := s.acquireExecutionAdmission(ctx, box, limits.Profile)
	if err != nil {
		return runner.Result{}, box, false, err
	}
	defer releaseExecution()
	if limits.Stateless && !req.BindWorkspace {
		result, err := s.runner.RunWithLimits(ctx, runReq, limits.Timeout, limits.StdoutLimitBytes, limits.StderrLimitBytes)
		return result, box, false, err
	}

	result, err := s.runner.RunInDirWithLimits(ctx, runReq, box.RootPath, limits.Timeout, limits.StdoutLimitBytes, limits.StderrLimitBytes)
	return result, box, true, err
}

func (s *Service) RunTemplate(ctx context.Context, req TemplateRequest) (TemplateResult, error) {
	executionID := newExecutionID()
	limits, err := s.policy.NormalizeTemplateLimits(req.Profile, req.Engine, req.TimeoutMS, req.OutputLimitKB)
	if err != nil {
		s.recordTemplateFailure(ctx, req, executionID, err)
		return TemplateResult{}, err
	}
	if len([]byte(req.Template)) > limits.MaxTemplateBytes {
		err := fmt.Errorf("template exceeds max size of %d bytes", limits.MaxTemplateBytes)
		s.recordTemplateFailure(ctx, req, executionID, err)
		return TemplateResult{}, err
	}
	if err := validateTemplateVariables(req.Variables, limits); err != nil {
		s.recordTemplateFailure(ctx, req, executionID, err)
		return TemplateResult{}, err
	}

	parsed, err := template.New("zgi-template").
		Option("missingkey=error").
		Funcs(templateFuncMap()).
		Parse(req.Template)
	if err != nil {
		s.recordTemplateFailure(ctx, req, executionID, err)
		return TemplateResult{}, err
	}
	if err := validateTemplateHelpers(parsed.Tree.Root); err != nil {
		s.recordTemplateFailure(ctx, req, executionID, err)
		return TemplateResult{}, err
	}

	started := time.Now()
	renderCtx, cancel := context.WithTimeout(ctx, limits.Timeout)
	defer cancel()

	resultCh := make(chan struct {
		content   string
		truncated bool
		err       error
	}, 1)
	go func() {
		var output strings.Builder
		writer := &limitedTemplateWriter{
			ctx:       renderCtx,
			writer:    &output,
			remaining: limits.OutputLimitBytes,
		}
		err := parsed.Execute(writer, req.Variables)
		if errors.Is(err, errTemplateOutputTruncated) {
			err = nil
		}
		resultCh <- struct {
			content   string
			truncated bool
			err       error
		}{
			content:   output.String(),
			truncated: writer.truncated,
			err:       err,
		}
	}()

	select {
	case rendered := <-resultCh:
		if rendered.err != nil {
			s.recordTemplateFailure(ctx, req, executionID, rendered.err)
			return TemplateResult{}, rendered.err
		}
		result := TemplateResult{
			ExecutionID: executionID,
			Content:     rendered.content,
			DurationMS:  time.Since(started).Milliseconds(),
			Truncated:   rendered.truncated,
		}
		if rendered.truncated {
			result.Warnings = append(result.Warnings, "output truncated")
		}
		metadata := map[string]any{
			"execution_id":    executionID,
			"engine":          limits.Engine,
			"profile":         limits.Profile,
			"duration_ms":     result.DurationMS,
			"truncated":       result.Truncated,
			"runtime_backend": s.policy.RuntimeBackend(),
			"status":          "success",
		}
		addTemplateOwnershipMetadata(metadata, req)
		s.observer.Record("exec.template", "", "template rendered", observer.MetadataWithContext(ctx, metadata))
		return result, nil
	case <-renderCtx.Done():
		err := fmt.Errorf("template execution timed out after %d ms", limits.Timeout.Milliseconds())
		s.recordTemplateFailure(ctx, req, executionID, err)
		return TemplateResult{}, err
	}
}

func (s *Service) RunCommand(ctx context.Context, req CommandRequest) (runner.CommandResult, error) {
	executionID := newExecutionID()
	baseMetadata := map[string]any{
		"execution_id": executionID,
		"command":      req.Command,
		"profile":      defaultString(req.Profile, "code-short"),
	}
	box, err := s.lifecycle.GetActive(req.SandboxID)
	if err != nil {
		s.recordExecutionFailure(ctx, "exec.command.failed", req.SandboxID, "sandbox command execution failed", baseMetadata, err)
		return runner.CommandResult{}, err
	}
	addExecutionSandboxMetadata(baseMetadata, box)

	workDir := box.RootPath
	if req.WorkingSubpath != "" {
		workDir, err = resolveExistingSandboxPath(box.RootPath, req.WorkingSubpath)
		if err != nil {
			s.recordExecutionFailure(ctx, "exec.command.failed", req.SandboxID, "sandbox command execution failed", baseMetadata, err)
			return runner.CommandResult{}, err
		}
	}

	limits, err := s.policy.NormalizeCommandLimits(req.Profile, req.TimeoutSeconds, req.TimeoutMS, req.StdoutLimitKB, req.StderrLimitKB)
	if err != nil {
		s.recordExecutionFailure(ctx, "exec.command.failed", req.SandboxID, "sandbox command execution failed", baseMetadata, err)
		return runner.CommandResult{}, err
	}
	baseMetadata["profile"] = limits.Profile
	if err := rejectRuntimeDependencyInstall(req.Command, req.Args); err != nil {
		s.recordExecutionFailure(ctx, "exec.command.failed", req.SandboxID, "sandbox command execution failed", baseMetadata, err)
		return runner.CommandResult{}, err
	}
	if len(req.Stdin) > limits.MaxStdinBytes {
		err := fmt.Errorf("stdin exceeds max size of %d bytes", limits.MaxStdinBytes)
		s.recordExecutionFailure(ctx, "exec.command.failed", req.SandboxID, "sandbox command execution failed", baseMetadata, err)
		return runner.CommandResult{}, err
	}
	env, err := normalizeCommandEnv(req.Env)
	if err != nil {
		s.recordExecutionFailure(ctx, "exec.command.failed", req.SandboxID, "sandbox command execution failed", baseMetadata, err)
		return runner.CommandResult{}, err
	}
	if err := s.enforceOrganizationExecutionRate(box); err != nil {
		s.recordExecutionFailure(ctx, "exec.command.failed", req.SandboxID, "sandbox command execution failed", baseMetadata, err)
		return runner.CommandResult{}, err
	}
	releaseExecution, err := s.acquireExecutionAdmission(ctx, box, limits.Profile)
	if err != nil {
		s.recordExecutionFailure(ctx, "exec.command.failed", req.SandboxID, "sandbox command execution failed", baseMetadata, err)
		return runner.CommandResult{}, err
	}
	defer releaseExecution()

	result, err := s.runner.ExecuteCommandSpec(ctx, runner.CommandSpec{
		WorkDir:           workDir,
		Command:           req.Command,
		Args:              req.Args,
		Stdin:             req.Stdin,
		Env:               env,
		DependencyProfile: box.DependencyProfile,
		Timeout:           limits.Timeout,
		StdoutLimit:       limits.StdoutLimitBytes,
		StderrLimit:       limits.StderrLimitBytes,
		AllowShellForm:    true,
	})
	if err != nil {
		s.recordExecutionFailure(ctx, "exec.command.failed", req.SandboxID, "sandbox command execution failed", baseMetadata, err)
		return runner.CommandResult{}, err
	}
	if err := s.enforceWorkspaceByteLimit(box); err != nil {
		s.recordExecutionFailure(ctx, "exec.command.failed", req.SandboxID, "sandbox command execution failed", baseMetadata, err)
		return runner.CommandResult{}, err
	}
	if err := s.enforceWorkspaceFileLimit(box); err != nil {
		s.recordExecutionFailure(ctx, "exec.command.failed", req.SandboxID, "sandbox command execution failed", baseMetadata, err)
		return runner.CommandResult{}, err
	}
	result.ExecutionID = executionID

	metadata := map[string]any{
		"execution_id":    executionID,
		"command":         req.Command,
		"profile":         limits.Profile,
		"exit_code":       result.ExitCode,
		"duration_ms":     result.DurationMS,
		"truncated":       result.Truncated,
		"backend":         result.Backend,
		"runtime_backend": result.Backend,
		"status":          "success",
	}
	addExecutionSandboxMetadata(metadata, box)
	s.observer.Record("exec.command", req.SandboxID, "sandbox command executed", observer.MetadataWithContext(ctx, metadata))
	return result, nil
}

func (s *Service) RunSkill(ctx context.Context, req SkillRunRequest) (SkillRunResult, error) {
	executionID := newExecutionID()
	if strings.TrimSpace(req.SandboxID) == "" {
		return SkillRunResult{}, errors.New("sandbox_id is required")
	}
	if strings.TrimSpace(req.Path) == "" {
		req.Path = "."
	}

	box, err := s.lifecycle.GetActive(req.SandboxID)
	if err != nil {
		s.recordExecutionFailure(ctx, "exec.skill.failed", req.SandboxID, "skill execution failed", map[string]any{"execution_id": executionID, "path": req.Path}, err)
		return SkillRunResult{}, err
	}
	baseMetadata := map[string]any{"execution_id": executionID, "path": req.Path}
	addExecutionSandboxMetadata(baseMetadata, box)

	packageRoot, err := resolveExistingSandboxPath(box.RootPath, req.Path)
	if err != nil {
		s.recordExecutionFailure(ctx, "exec.skill.failed", req.SandboxID, "skill execution failed", baseMetadata, err)
		return SkillRunResult{}, err
	}
	info, err := os.Stat(packageRoot)
	if err != nil {
		s.recordExecutionFailure(ctx, "exec.skill.failed", req.SandboxID, "skill execution failed", baseMetadata, err)
		return SkillRunResult{}, err
	}
	if !info.IsDir() {
		err := fmt.Errorf("skill package path is not a directory: %s", req.Path)
		s.recordExecutionFailure(ctx, "exec.skill.failed", req.SandboxID, "skill execution failed", baseMetadata, err)
		return SkillRunResult{}, err
	}

	manifest, err := loadSkillExecutionManifest(packageRoot, s.policy, box.DependencyProfile)
	if err != nil {
		s.recordExecutionFailure(ctx, "exec.skill.failed", req.SandboxID, "skill execution failed", baseMetadata, err)
		return SkillRunResult{}, err
	}

	stdin := req.Stdin
	if len(req.InputJSON) > 0 {
		if !json.Valid(req.InputJSON) {
			err := errors.New("input_json must contain valid JSON")
			s.recordExecutionFailure(ctx, "exec.skill.failed", req.SandboxID, "skill execution failed", baseMetadata, err)
			return SkillRunResult{}, err
		}
		stdin = string(req.InputJSON)
	}
	env, err := normalizeCommandEnv(req.Env)
	if err != nil {
		s.recordExecutionFailure(ctx, "exec.skill.failed", req.SandboxID, "skill execution failed", baseMetadata, err)
		return SkillRunResult{}, err
	}

	command, args := skillCommand(manifest)
	profile := skillCommandProfile(manifest)
	if err := s.enforceOrganizationExecutionRate(box); err != nil {
		s.recordExecutionFailure(ctx, "exec.skill.failed", req.SandboxID, "skill execution failed", baseMetadata, err)
		return SkillRunResult{}, err
	}
	releaseExecution, err := s.acquireExecutionAdmission(ctx, box, profile)
	if err != nil {
		s.recordExecutionFailure(ctx, "exec.skill.failed", req.SandboxID, "skill execution failed", baseMetadata, err)
		return SkillRunResult{}, err
	}
	defer releaseExecution()
	result, err := s.runner.ExecuteCommandSpec(ctx, runner.CommandSpec{
		WorkDir:           packageRoot,
		Command:           command,
		Args:              args,
		Stdin:             stdin,
		Env:               env,
		DependencyProfile: box.DependencyProfile,
		Timeout:           time.Duration(manifest.TimeoutMS) * time.Millisecond,
		StdoutLimit:       64 * 1024,
		StderrLimit:       64 * 1024,
		AllowShellForm:    false,
	})
	if err != nil {
		s.recordExecutionFailure(ctx, "exec.skill.failed", req.SandboxID, "skill execution failed", baseMetadata, err)
		return SkillRunResult{}, err
	}
	if err := s.enforceWorkspaceByteLimit(box); err != nil {
		s.recordExecutionFailure(ctx, "exec.skill.failed", req.SandboxID, "skill execution failed", baseMetadata, err)
		return SkillRunResult{}, err
	}
	if err := s.enforceWorkspaceFileLimit(box); err != nil {
		s.recordExecutionFailure(ctx, "exec.skill.failed", req.SandboxID, "skill execution failed", baseMetadata, err)
		return SkillRunResult{}, err
	}
	result.ExecutionID = executionID

	artifactManifests, err := s.skillArtifactManifests(req.SandboxID, req.Path, manifest)
	if err != nil {
		s.recordExecutionFailure(ctx, "exec.skill.failed", req.SandboxID, "skill execution failed", baseMetadata, err)
		return SkillRunResult{}, err
	}

	runResult := SkillRunResult{
		ExecutionID:       executionID,
		SandboxID:         req.SandboxID,
		Path:              req.Path,
		Manifest:          manifest,
		Command:           result,
		ArtifactManifests: artifactManifests,
	}
	if manifest.ResultMode == "stdout_json" || manifest.ResultMode == "mixed" {
		var decoded any
		if strings.TrimSpace(result.Stdout) != "" && json.Unmarshal([]byte(strings.TrimSpace(result.Stdout)), &decoded) == nil {
			runResult.ResultJSON = decoded
		}
	}

	metadata := map[string]any{
		"execution_id":    executionID,
		"path":            req.Path,
		"entrypoint":      manifest.Entrypoint,
		"language":        manifest.Language,
		"profile":         profile,
		"result_mode":     manifest.ResultMode,
		"exit_code":       result.ExitCode,
		"duration_ms":     result.DurationMS,
		"truncated":       result.Truncated,
		"backend":         result.Backend,
		"runtime_backend": result.Backend,
		"artifact_paths":  len(artifactManifests),
		"status":          "success",
	}
	addExecutionSandboxMetadata(metadata, box)
	s.observer.Record("exec.skill", req.SandboxID, "skill executed", observer.MetadataWithContext(ctx, metadata))
	return runResult, nil
}

func (s *Service) enforceOrganizationExecutionRate(box *sandbox.Sandbox) error {
	limit := s.policy.MaxExecutionsPerMinutePerOrganization()
	if limit <= 0 || box == nil || strings.TrimSpace(box.OrganizationID) == "" {
		return nil
	}

	windowStart := time.Now().UTC().Add(-time.Minute)
	events := s.observer.Query(observer.Query{
		OrganizationID: box.OrganizationID,
		TypePrefix:     "exec.",
		After:          windowStart,
		Limit:          limit + 1,
	})
	if len(events) < limit {
		return nil
	}
	return &policy.LimitError{
		Code:    "organization_execution_rate_limit_exceeded",
		Limit:   "max_executions_per_minute_per_organization",
		Maximum: limit,
		Actual:  len(events) + 1,
		Details: map[string]any{
			"organization_id":   box.OrganizationID,
			"window_seconds":    60,
			"recent_executions": len(events),
		},
	}
}

func (s *Service) acquireExecutionAdmission(ctx context.Context, box *sandbox.Sandbox, profile string) (func(), error) {
	profile = strings.TrimSpace(profile)
	serviceLimit := s.policy.MaxConcurrentExecutions()
	profileLimit := s.policy.MaxConcurrentExecutionsPerProfile()
	organizationLimit := s.policy.MaxConcurrentExecutionsPerOrganization()
	if serviceLimit <= 0 && profileLimit <= 0 && (organizationLimit <= 0 || box == nil || strings.TrimSpace(box.OrganizationID) == "") {
		return func() {}, nil
	}

	organizationID := ""
	if box != nil {
		organizationID = strings.TrimSpace(box.OrganizationID)
	}
	queueLimit := s.policy.MaxQueuedExecutionsPerOrganization()
	queueTimeout := time.Duration(s.policy.QueueTimeoutMS()) * time.Millisecond
	if queueTimeout <= 0 {
		queueTimeout = 5 * time.Second
	}
	timer := time.NewTimer(queueTimeout)
	defer timer.Stop()

	queued := false
	defer func() {
		if queued {
			s.executionMu.Lock()
			s.releaseQueuedOrganizationExecutionLocked(organizationID)
			s.notifyExecutionChangedLocked()
			s.executionMu.Unlock()
		}
	}()

	for {
		s.executionMu.Lock()
		if serviceLimit > 0 && s.activeExecutions >= serviceLimit {
			if queued {
				s.releaseQueuedOrganizationExecutionLocked(organizationID)
				queued = false
				s.notifyExecutionChangedLocked()
			}
			activeService := s.activeExecutions
			s.executionMu.Unlock()
			return nil, &policy.LimitError{
				Code:    "service_concurrent_execution_limit_exceeded",
				Limit:   "max_concurrent_executions",
				Maximum: serviceLimit,
				Actual:  activeService + 1,
				Details: map[string]any{
					"active_executions": activeService,
				},
			}
		}

		activeProfile := s.activeExecutionsByProfile[profile]
		if profileLimit > 0 && activeProfile >= profileLimit {
			if queued {
				s.releaseQueuedOrganizationExecutionLocked(organizationID)
				queued = false
				s.notifyExecutionChangedLocked()
			}
			s.executionMu.Unlock()
			return nil, &policy.LimitError{
				Code:    "profile_concurrent_execution_limit_exceeded",
				Limit:   "max_concurrent_executions_per_profile",
				Maximum: profileLimit,
				Actual:  activeProfile + 1,
				Details: map[string]any{
					"profile":           profile,
					"active_executions": activeProfile,
				},
			}
		}

		activeOrganization := s.activeExecutionsByOrganization[organizationID]
		currentQueued := s.queuedExecutionsByOrganization[organizationID]
		organizationLimited := organizationLimit > 0 && organizationID != ""
		if !organizationLimited || (activeOrganization < organizationLimit && (queued || currentQueued == 0)) {
			if serviceLimit > 0 {
				s.activeExecutions++
			}
			if profileLimit > 0 {
				s.activeExecutionsByProfile[profile] = activeProfile + 1
			}
			if organizationLimited {
				s.activeExecutionsByOrganization[organizationID] = activeOrganization + 1
			}
			if queued {
				s.releaseQueuedOrganizationExecutionLocked(organizationID)
				queued = false
				s.notifyExecutionChangedLocked()
			}
			s.executionMu.Unlock()
			return s.releaseExecutionAdmission(profile, organizationID, serviceLimit > 0), nil
		}

		if queueLimit <= 0 {
			s.executionMu.Unlock()
			return nil, &policy.LimitError{
				Code:    "organization_concurrent_execution_limit_exceeded",
				Limit:   "max_concurrent_executions_per_organization",
				Maximum: organizationLimit,
				Actual:  activeOrganization + 1,
				Details: map[string]any{
					"organization_id":   organizationID,
					"active_executions": activeOrganization,
				},
			}
		}

		if !queued {
			if currentQueued >= queueLimit {
				s.executionMu.Unlock()
				return nil, &policy.LimitError{
					Code:    "organization_queued_execution_limit_exceeded",
					Limit:   "max_queued_executions_per_organization",
					Maximum: queueLimit,
					Actual:  currentQueued + 1,
					Details: map[string]any{
						"organization_id":   organizationID,
						"queued_executions": currentQueued,
					},
				}
			}
			s.queuedExecutionsByOrganization[organizationID] = currentQueued + 1
			queued = true
		}

		changed := s.executionChanged
		queuedCount := s.queuedExecutionsByOrganization[organizationID]
		s.executionMu.Unlock()

		select {
		case <-changed:
			continue
		case <-timer.C:
			return nil, &policy.LimitError{
				Code:    "organization_execution_queue_timeout",
				Limit:   "queue_timeout_ms",
				Maximum: s.policy.QueueTimeoutMS(),
				Actual:  s.policy.QueueTimeoutMS(),
				Details: map[string]any{
					"organization_id":   organizationID,
					"queued_executions": queuedCount,
				},
			}
		case <-ctx.Done():
			return nil, &runner.CancellationError{Phase: "queue"}
		}
	}
}

func (s *Service) releaseExecutionAdmission(profile string, organizationID string, releaseService bool) func() {
	released := false
	return func() {
		s.executionMu.Lock()
		defer s.executionMu.Unlock()
		if released {
			return
		}
		released = true
		if releaseService && s.activeExecutions > 0 {
			s.activeExecutions--
		}
		if profile != "" {
			nextProfile := s.activeExecutionsByProfile[profile] - 1
			if nextProfile <= 0 {
				delete(s.activeExecutionsByProfile, profile)
			} else {
				s.activeExecutionsByProfile[profile] = nextProfile
			}
		}
		if organizationID != "" {
			nextOrganization := s.activeExecutionsByOrganization[organizationID] - 1
			if nextOrganization <= 0 {
				delete(s.activeExecutionsByOrganization, organizationID)
			} else {
				s.activeExecutionsByOrganization[organizationID] = nextOrganization
			}
		}
		s.notifyExecutionChangedLocked()
	}
}

func (s *Service) releaseQueuedOrganizationExecutionLocked(organizationID string) {
	next := s.queuedExecutionsByOrganization[organizationID] - 1
	if next <= 0 {
		delete(s.queuedExecutionsByOrganization, organizationID)
		return
	}
	s.queuedExecutionsByOrganization[organizationID] = next
}

func (s *Service) notifyExecutionChangedLocked() {
	close(s.executionChanged)
	s.executionChanged = make(chan struct{})
}

func (s *Service) enforceWorkspaceByteLimit(box *sandbox.Sandbox) error {
	if box == nil {
		return nil
	}
	size, err := workspaceByteSize(box.RootPath)
	if err != nil {
		return err
	}
	if err := s.enforceWorkspaceByteLimitForSize(box, size); err != nil {
		return err
	}
	return s.enforceOrganizationWorkspaceByteLimitForSize(box, size)
}

func (s *Service) enforceWorkspaceByteLimitForWrite(box *sandbox.Sandbox, target string, newSize int64) error {
	if box == nil {
		return nil
	}
	currentSize, err := workspaceByteSize(box.RootPath)
	if err != nil {
		return err
	}
	existingSize, err := existingRegularFileSize(target)
	if err != nil {
		return err
	}
	projectedSize := currentSize - existingSize + newSize
	if err := s.enforceWorkspaceByteLimitForSize(box, projectedSize); err != nil {
		return err
	}
	return s.enforceOrganizationWorkspaceByteLimitForSize(box, projectedSize)
}

func (s *Service) enforceWorkspaceByteLimitForSize(box *sandbox.Sandbox, actualSize int64) error {
	limit := s.policy.MaxWorkspaceBytes()
	if limit <= 0 || box == nil || actualSize <= limit {
		return nil
	}
	return &policy.LimitError{
		Code:    "workspace_byte_limit_exceeded",
		Limit:   "max_workspace_bytes",
		Maximum: limitMaximumInt(limit),
		Actual:  limitMaximumInt(actualSize),
		Details: map[string]any{
			"workspace_bytes": actualSize,
		},
	}
}

func (s *Service) enforceOrganizationWorkspaceByteLimitForSize(box *sandbox.Sandbox, projectedBoxSize int64) error {
	limit := s.policy.MaxWorkspaceBytesPerOrganization()
	if limit <= 0 || box == nil || box.OrganizationID == "" {
		return nil
	}
	actualSize, err := s.organizationWorkspaceByteSize(box, projectedBoxSize)
	if err != nil {
		return err
	}
	if actualSize <= limit {
		return nil
	}
	return &policy.LimitError{
		Code:    "organization_workspace_byte_limit_exceeded",
		Limit:   "max_workspace_bytes_per_organization",
		Maximum: limitMaximumInt(limit),
		Actual:  limitMaximumInt(actualSize),
		Details: map[string]any{
			"organization_id":              box.OrganizationID,
			"organization_workspace_bytes": actualSize,
		},
	}
}

func (s *Service) organizationWorkspaceByteSize(currentBox *sandbox.Sandbox, projectedCurrentBoxSize int64) (int64, error) {
	var total int64
	for _, item := range s.lifecycle.List() {
		if item.OrganizationID != currentBox.OrganizationID {
			continue
		}
		if item.ID == currentBox.ID {
			total += projectedCurrentBoxSize
			continue
		}
		size, err := workspaceByteSize(item.RootPath)
		if err != nil {
			return 0, err
		}
		total += size
	}
	return total, nil
}

func (s *Service) enforceWorkspaceFileLimit(box *sandbox.Sandbox) error {
	if box == nil {
		return nil
	}
	count, err := workspaceFileCount(box.RootPath)
	if err != nil {
		return err
	}
	return s.enforceWorkspaceFileLimitForCount(box, count)
}

func (s *Service) enforceWorkspaceFileLimitForWrite(box *sandbox.Sandbox, target string) error {
	if box == nil || s.policy.MaxWorkspaceFiles() <= 0 {
		return nil
	}
	currentCount, err := workspaceFileCount(box.RootPath)
	if err != nil {
		return err
	}
	exists, err := regularFileExists(target)
	if err != nil {
		return err
	}
	if !exists {
		currentCount++
	}
	return s.enforceWorkspaceFileLimitForCount(box, currentCount)
}

func (s *Service) enforceWorkspaceFileLimitForCount(box *sandbox.Sandbox, actualCount int) error {
	limit := s.policy.MaxWorkspaceFiles()
	if limit <= 0 || box == nil || actualCount <= limit {
		return nil
	}
	return &policy.LimitError{
		Code:    "workspace_file_count_limit_exceeded",
		Limit:   "max_workspace_files",
		Maximum: limit,
		Actual:  actualCount,
		Details: map[string]any{
			"workspace_files": actualCount,
		},
	}
}

func (s *Service) recordExecutionFailure(ctx context.Context, eventType string, sandboxID string, message string, metadata map[string]any, err error) {
	eventMetadata := map[string]any{
		"status":     "failure",
		"error_type": classifyExecutionError(err),
	}
	var detailsErr interface {
		ResponseDetails() map[string]any
	}
	if errors.As(err, &detailsErr) {
		for key, value := range detailsErr.ResponseDetails() {
			eventMetadata[key] = value
		}
	}
	for key, value := range metadata {
		eventMetadata[key] = value
	}
	addRuntimeBackendMetadata(eventMetadata, s.policy.RuntimeBackend())
	s.observer.Record(eventType, sandboxID, message, observer.MetadataWithContext(ctx, eventMetadata))
}

func classifyExecutionError(err error) string {
	var cancelErr *runner.CancellationError
	var queueErr *runner.QueueTimeoutError
	var limitErr *policy.LimitError
	var dependencyInstallErr *DependencyInstallError
	switch {
	case errors.As(err, &cancelErr):
		return "execution_canceled"
	case errors.As(err, &queueErr), errors.As(err, &limitErr):
		return "limit_exceeded"
	case errors.As(err, &dependencyInstallErr):
		return "policy_denied"
	case strings.Contains(err.Error(), "timed out"):
		return "limit_exceeded"
	case strings.Contains(err.Error(), "network access"):
		return "network_policy_rejected"
	case strings.Contains(err.Error(), "not found"):
		return "not_found"
	case strings.Contains(err.Error(), "unsupported"), strings.Contains(err.Error(), "unknown"), strings.Contains(err.Error(), "invalid"), strings.Contains(err.Error(), "exceeds"), strings.Contains(err.Error(), "dangerous"), strings.Contains(err.Error(), "not allowed"):
		return "validation_error"
	default:
		return "execution_error"
	}
}

func (s *Service) recordTemplateFailure(ctx context.Context, req TemplateRequest, executionID string, err error) {
	metadata := map[string]any{
		"execution_id":    executionID,
		"engine":          defaultString(req.Engine, "go-text"),
		"profile":         defaultString(req.Profile, "template-short"),
		"runtime_backend": s.policy.RuntimeBackend(),
		"status":          "failure",
		"error_type":      classifyExecutionError(err),
	}
	addTemplateOwnershipMetadata(metadata, req)
	s.observer.Record("exec.template.failed", "", "template render failed", observer.MetadataWithContext(ctx, metadata))
}

func addTemplateOwnershipMetadata(metadata map[string]any, req TemplateRequest) {
	if req.OrganizationID != "" {
		metadata["organization_id"] = req.OrganizationID
	}
	if req.WorkspaceID != "" {
		metadata["workspace_id"] = req.WorkspaceID
	}
	if req.AppID != "" {
		metadata["app_id"] = req.AppID
	}
	if req.WorkflowRunID != "" {
		metadata["workflow_run_id"] = req.WorkflowRunID
	}
	if req.UserID != "" {
		metadata["user_id"] = req.UserID
	}
}

var errTemplateOutputTruncated = errors.New("template output truncated")

type limitedTemplateWriter struct {
	ctx       context.Context
	writer    *strings.Builder
	remaining int
	truncated bool
}

func (w *limitedTemplateWriter) Write(data []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	if w.remaining <= 0 {
		w.truncated = true
		return 0, errTemplateOutputTruncated
	}
	if len(data) > w.remaining {
		n, _ := w.writer.Write(data[:w.remaining])
		w.remaining = 0
		w.truncated = true
		return n, errTemplateOutputTruncated
	}
	n, err := w.writer.Write(data)
	w.remaining -= n
	return n, err
}

func templateFuncMap() template.FuncMap {
	return template.FuncMap{
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"title": func(value string) string {
			words := strings.Fields(value)
			for i, word := range words {
				if word == "" {
					continue
				}
				words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
			}
			return strings.Join(words, " ")
		},
		"default": func(fallback string, value any) any {
			switch typed := value.(type) {
			case nil:
				return fallback
			case string:
				if typed == "" {
					return fallback
				}
			}
			return value
		},
	}
}

func validateTemplateHelpers(node parse.Node) error {
	switch typed := node.(type) {
	case nil:
		return nil
	case *parse.ListNode:
		for _, item := range typed.Nodes {
			if err := validateTemplateHelpers(item); err != nil {
				return err
			}
		}
	case *parse.ActionNode:
		return validateTemplateHelpers(typed.Pipe)
	case *parse.IfNode:
		if err := validateTemplateHelpers(typed.Pipe); err != nil {
			return err
		}
		if err := validateTemplateHelpers(typed.List); err != nil {
			return err
		}
		return validateTemplateHelpers(typed.ElseList)
	case *parse.RangeNode:
		if err := validateTemplateHelpers(typed.Pipe); err != nil {
			return err
		}
		if err := validateTemplateHelpers(typed.List); err != nil {
			return err
		}
		return validateTemplateHelpers(typed.ElseList)
	case *parse.WithNode:
		if err := validateTemplateHelpers(typed.Pipe); err != nil {
			return err
		}
		if err := validateTemplateHelpers(typed.List); err != nil {
			return err
		}
		return validateTemplateHelpers(typed.ElseList)
	case *parse.PipeNode:
		for _, command := range typed.Cmds {
			if err := validateTemplateHelpers(command); err != nil {
				return err
			}
		}
	case *parse.CommandNode:
		for _, arg := range typed.Args {
			if err := validateTemplateHelpers(arg); err != nil {
				return err
			}
		}
	case *parse.IdentifierNode:
		if !templateHelperAllowed(typed.Ident) {
			return fmt.Errorf("template helper is not allowed: %s", typed.Ident)
		}
	case *parse.TemplateNode:
		return errors.New("nested template execution is not allowed")
	}
	return nil
}

func templateHelperAllowed(name string) bool {
	switch name {
	case "upper", "lower", "title", "default":
		return true
	default:
		return false
	}
}

func validateTemplateVariables(value any, limits policy.TemplateLimits) error {
	count := 0
	return validateTemplateValue(value, 0, limits, &count)
}

func validateTemplateValue(value any, depth int, limits policy.TemplateLimits, count *int) error {
	if depth > limits.MaxVariableDepth {
		return fmt.Errorf("template variables exceed max depth of %d", limits.MaxVariableDepth)
	}
	switch typed := value.(type) {
	case map[string]any:
		*count += len(typed)
		if *count > limits.MaxVariableCount {
			return fmt.Errorf("template variables exceed max count of %d", limits.MaxVariableCount)
		}
		for _, item := range typed {
			if err := validateTemplateValue(item, depth+1, limits, count); err != nil {
				return err
			}
		}
	case []any:
		*count += len(typed)
		if *count > limits.MaxVariableCount {
			return fmt.Errorf("template variables exceed max count of %d", limits.MaxVariableCount)
		}
		for _, item := range typed {
			if err := validateTemplateValue(item, depth+1, limits, count); err != nil {
				return err
			}
		}
	case string:
		if len([]byte(typed)) > limits.MaxVariableStringBytes {
			return fmt.Errorf("template variable string exceeds max size of %d bytes", limits.MaxVariableStringBytes)
		}
	}
	return nil
}

func addOwnershipMetadata(metadata map[string]any, box *sandbox.Sandbox) {
	if box == nil {
		return
	}
	if box.OrganizationID != "" {
		metadata["organization_id"] = box.OrganizationID
	}
	if box.WorkspaceID != "" {
		metadata["workspace_id"] = box.WorkspaceID
	}
	if box.AppID != "" {
		metadata["app_id"] = box.AppID
	}
	if box.WorkflowRunID != "" {
		metadata["workflow_run_id"] = box.WorkflowRunID
	}
	if box.UserID != "" {
		metadata["user_id"] = box.UserID
	}
}

func addExecutionSandboxMetadata(metadata map[string]any, box *sandbox.Sandbox) {
	addOwnershipMetadata(metadata, box)
	if box == nil {
		return
	}
	if box.DependencyProfile != "" {
		metadata["dependency_profile"] = box.DependencyProfile
	}
	if box.DependencyProfileVersion != "" {
		metadata["dependency_profile_version"] = box.DependencyProfileVersion
	}
}

func addRuntimeBackendMetadata(metadata map[string]any, runtimeBackend string) {
	if strings.TrimSpace(runtimeBackend) != "" {
		metadata["runtime_backend"] = runtimeBackend
	}
}

func (s *Service) UploadFile(req FileWriteRequest) (*FileInfo, error) {
	box, err := s.lifecycle.GetActive(req.SandboxID)
	if err != nil {
		return nil, err
	}

	target, err := resolveWritableSandboxPath(box.RootPath, req.Path)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, err
	}

	content, err := decodeContent(req.Content, req.Encoding)
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > s.policy.MaxFileSizeBytes() {
		return nil, fmt.Errorf("file exceeds max size of %d bytes", s.policy.MaxFileSizeBytes())
	}
	if err := s.enforceWorkspaceByteLimitForWrite(box, target, int64(len(content))); err != nil {
		return nil, err
	}
	if err := s.enforceWorkspaceFileLimitForWrite(box, target); err != nil {
		return nil, err
	}
	if err := os.WriteFile(target, content, 0o644); err != nil {
		return nil, err
	}

	info, err := s.StatFile(req.SandboxID, req.Path)
	if err == nil {
		metadata := map[string]any{"path": req.Path, "size": info.Size}
		addRuntimeBackendMetadata(metadata, s.policy.RuntimeBackend())
		addOwnershipMetadata(metadata, box)
		s.observer.Record("files.upload", req.SandboxID, "file uploaded", metadata)
	}
	return info, err
}

func (s *Service) UploadArchive(req ArchiveUploadRequest) (*ArchiveUploadResult, error) {
	if strings.TrimSpace(req.SandboxID) == "" {
		return nil, errors.New("sandbox_id is required")
	}
	if strings.TrimSpace(req.Path) == "" {
		req.Path = "."
	}
	if strings.TrimSpace(req.ArchiveBase64) == "" {
		return nil, errors.New("archive_base64 is required")
	}
	if !strings.EqualFold(strings.TrimSpace(req.Format), "zip") {
		return nil, errors.New("unsupported archive format")
	}

	box, err := s.lifecycle.GetActive(req.SandboxID)
	if err != nil {
		return nil, err
	}
	if _, err := resolveWritableSandboxPath(box.RootPath, req.Path); err != nil {
		return nil, err
	}

	archiveBytes, err := base64.StdEncoding.DecodeString(req.ArchiveBase64)
	if err != nil {
		return nil, fmt.Errorf("invalid archive_base64: %w", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)))
	if err != nil {
		return nil, fmt.Errorf("invalid zip archive: %w", err)
	}

	entries, err := normalizeArchiveEntries(reader.File, req.StripRoot)
	if err != nil {
		return nil, err
	}
	var skillManifest *SkillExecutionManifest
	if req.ValidateSkillManifest {
		skillManifest, err = validateSkillExecutionManifest(entries, s.policy, box.DependencyProfile)
		if err != nil {
			return nil, err
		}
	}

	limit := archiveLimits{
		maxFiles:     256,
		maxFileSize:  s.policy.MaxFileSizeBytes(),
		maxTotalSize: s.policy.MaxFileSizeBytes() * 256,
	}
	totalSize, err := validateArchiveEntriesWithinLimits(entries, limit)
	if err != nil {
		return nil, err
	}
	if projectedSize, err := projectedWorkspaceSizeForArchive(box.RootPath, req.Path, entries); err != nil {
		return nil, err
	} else if err := s.enforceWorkspaceByteLimitForSize(box, projectedSize); err != nil {
		return nil, err
	} else if err := s.enforceOrganizationWorkspaceByteLimitForSize(box, projectedSize); err != nil {
		return nil, err
	}
	if projectedCount, err := projectedWorkspaceFileCountForArchive(box.RootPath, req.Path, entries); err != nil {
		return nil, err
	} else if err := s.enforceWorkspaceFileLimitForCount(box, projectedCount); err != nil {
		return nil, err
	}

	written := make([]fileSnapshot, 0, len(entries))
	files := make([]FileInfo, 0, len(entries))
	for _, entry := range entries {
		relativePath := filepath.ToSlash(filepath.Join(req.Path, entry.name))
		target, err := resolveWritableSandboxPath(box.RootPath, relativePath)
		if err != nil {
			rollbackWrittenFiles(written)
			return nil, err
		}
		snapshot, err := snapshotExistingFile(target)
		if err != nil {
			rollbackWrittenFiles(written)
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			rollbackWrittenFiles(written)
			return nil, err
		}

		content, err := readZipFile(entry.file, limit.maxFileSize)
		if err != nil {
			rollbackWrittenFiles(written)
			return nil, err
		}
		if err := os.WriteFile(target, content, 0o644); err != nil {
			rollbackWrittenFiles(written)
			return nil, err
		}
		written = append(written, snapshot)

		info, err := s.StatFile(req.SandboxID, relativePath)
		if err != nil {
			rollbackWrittenFiles(written)
			return nil, err
		}
		files = append(files, *info)
	}

	metadata := map[string]any{
		"path":       req.Path,
		"file_count": len(files),
		"total_size": totalSize,
	}
	addRuntimeBackendMetadata(metadata, s.policy.RuntimeBackend())
	addOwnershipMetadata(metadata, box)
	s.observer.Record("files.upload_archive", req.SandboxID, "archive uploaded", metadata)
	return &ArchiveUploadResult{
		SandboxID:     req.SandboxID,
		Path:          req.Path,
		Files:         files,
		FileCount:     len(files),
		TotalSize:     totalSize,
		SkillManifest: skillManifest,
	}, nil
}

func (s *Service) DownloadFile(sandboxID string, relativePath string, encoding string) (*FileContent, error) {
	box, err := s.lifecycle.GetActive(sandboxID)
	if err != nil {
		return nil, err
	}

	target, err := resolveExistingSandboxPath(box.RootPath, relativePath)
	if err != nil {
		return nil, err
	}

	content, err := os.ReadFile(target)
	if err != nil {
		return nil, err
	}

	normalized := normalizeEncoding(encoding)
	if normalized == "base64" {
		content = []byte(base64.StdEncoding.EncodeToString(content))
	}

	metadata := map[string]any{"path": relativePath}
	addRuntimeBackendMetadata(metadata, s.policy.RuntimeBackend())
	addOwnershipMetadata(metadata, box)
	s.observer.Record("files.download", sandboxID, "file downloaded", metadata)
	return &FileContent{
		SandboxID: sandboxID,
		Path:      relativePath,
		Content:   string(content),
		Encoding:  normalized,
	}, nil
}

func (s *Service) StatFile(sandboxID string, relativePath string) (*FileInfo, error) {
	box, err := s.lifecycle.GetActive(sandboxID)
	if err != nil {
		return nil, err
	}

	target, err := resolveExistingSandboxPath(box.RootPath, relativePath)
	if err != nil {
		return nil, err
	}

	stat, err := os.Stat(target)
	if err != nil {
		return nil, err
	}

	return &FileInfo{
		Path:        relativePath,
		Size:        stat.Size(),
		Mode:        stat.Mode().String(),
		ModifiedAt:  stat.ModTime().UTC(),
		IsDirectory: stat.IsDir(),
		SandboxID:   sandboxID,
	}, nil
}

func (s *Service) DeleteFile(sandboxID string, relativePath string) error {
	box, err := s.lifecycle.GetActive(sandboxID)
	if err != nil {
		return err
	}
	if filepath.Clean("/"+relativePath) == "/" {
		return errors.New("cannot delete sandbox root")
	}

	target, err := resolveExistingSandboxPath(box.RootPath, relativePath)
	if err != nil {
		return err
	}

	if err := os.RemoveAll(target); err != nil {
		return err
	}

	metadata := map[string]any{"path": relativePath}
	addRuntimeBackendMetadata(metadata, s.policy.RuntimeBackend())
	addOwnershipMetadata(metadata, box)
	s.observer.Record("files.delete", sandboxID, "file deleted", metadata)
	return nil
}

func (s *Service) ListFiles(sandboxID string) ([]FileInfo, error) {
	box, err := s.lifecycle.GetActive(sandboxID)
	if err != nil {
		return nil, err
	}

	entries, err := ListDirectory(box.RootPath)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		entries[i].SandboxID = sandboxID
	}
	return entries, nil
}

func (s *Service) BuildFileManifest(sandboxID string, relativePath string) (*FileManifest, error) {
	return s.BuildFileManifestWithOptions(sandboxID, relativePath, FileManifestOptions{})
}

func (s *Service) BuildFileManifestWithOptions(sandboxID string, relativePath string, options FileManifestOptions) (*FileManifest, error) {
	box, err := s.lifecycle.GetActive(sandboxID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(relativePath) == "" {
		relativePath = "artifacts"
	}

	target, err := resolveExistingSandboxPath(box.RootPath, relativePath)
	if err != nil {
		return nil, err
	}

	maxFiles, maxTotalBytes := s.normalizeManifestLimits(options)
	items := make([]FileManifestItem, 0)
	var totalSize int64
	err = filepath.WalkDir(target, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == target || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("manifest path contains symlink: %s", path)
		}
		if len(items)+1 > maxFiles {
			return &policy.LimitError{
				Code:    "artifact_manifest_file_count_exceeded",
				Limit:   "max_artifact_manifest_files",
				Maximum: maxFiles,
				Actual:  len(items) + 1,
				Details: map[string]any{"path": relativePath},
			}
		}
		if totalSize+info.Size() > maxTotalBytes {
			return &policy.LimitError{
				Code:    "artifact_manifest_total_bytes_exceeded",
				Limit:   "max_artifact_manifest_total_bytes",
				Maximum: limitMaximumInt(maxTotalBytes),
				Actual:  limitMaximumInt(totalSize + info.Size()),
				Details: map[string]any{"path": relativePath},
			}
		}

		item, err := fileManifestItem(box.RootPath, path, info)
		if err != nil {
			return err
		}
		items = append(items, item)
		totalSize += item.Size
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := s.enforceOrganizationArtifactByteLimit(box, target, totalSize); err != nil {
		return nil, err
	}

	metadata := map[string]any{
		"path":       relativePath,
		"file_count": len(items),
		"total_size": totalSize,
		"truncated":  false,
	}
	addRuntimeBackendMetadata(metadata, s.policy.RuntimeBackend())
	addOwnershipMetadata(metadata, box)
	s.observer.Record("files.manifest", sandboxID, "file manifest generated", metadata)
	return &FileManifest{
		SandboxID: sandboxID,
		Path:      relativePath,
		Items:     items,
		FileCount: len(items),
		TotalSize: totalSize,
		Truncated: false,
	}, nil
}

func (s *Service) enforceOrganizationArtifactByteLimit(box *sandbox.Sandbox, manifestTarget string, manifestTotalSize int64) error {
	limit := s.policy.MaxArtifactBytesPerOrganization()
	if limit <= 0 || box == nil || box.OrganizationID == "" {
		return nil
	}
	currentArtifactBytes, err := artifactByteSize(box.RootPath)
	if err != nil {
		return err
	}
	currentManifestBytes, err := directoryByteSize(manifestTarget)
	if err != nil {
		return err
	}
	actualSize := currentArtifactBytes - currentManifestBytes + manifestTotalSize
	for _, item := range s.lifecycle.List() {
		if item.ID == box.ID || item.OrganizationID != box.OrganizationID {
			continue
		}
		size, err := artifactByteSize(item.RootPath)
		if err != nil {
			return err
		}
		actualSize += size
	}
	if actualSize <= limit {
		return nil
	}
	return &policy.LimitError{
		Code:    "organization_artifact_byte_limit_exceeded",
		Limit:   "max_artifact_bytes_per_organization",
		Maximum: limitMaximumInt(limit),
		Actual:  limitMaximumInt(actualSize),
		Details: map[string]any{
			"organization_id":             box.OrganizationID,
			"organization_artifact_bytes": actualSize,
		},
	}
}

func (s *Service) normalizeManifestLimits(options FileManifestOptions) (int, int64) {
	effectiveLimits := s.policy.EffectiveLimits()
	maxFiles := effectiveLimits.MaxArtifactManifestFiles
	if maxFiles <= 0 {
		maxFiles = 100
	}
	if options.MaxFiles > 0 && options.MaxFiles < maxFiles {
		maxFiles = options.MaxFiles
	}

	maxTotalBytes := effectiveLimits.MaxArtifactManifestTotalBytes
	if maxTotalBytes <= 0 {
		maxTotalBytes = 64 * 1024 * 1024
	}
	if options.MaxTotalBytes > 0 && options.MaxTotalBytes < maxTotalBytes {
		maxTotalBytes = options.MaxTotalBytes
	}
	return maxFiles, maxTotalBytes
}

func limitMaximumInt(value int64) int {
	if value > int64(^uint(0)>>1) {
		return int(^uint(0) >> 1)
	}
	return int(value)
}

func workspaceByteSize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("workspace contains symlink: %s", path)
		}
		total += info.Size()
		return nil
	})
	return total, err
}

func artifactByteSize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("artifact path contains symlink: %s", path)
		}
		if pathHasSegment(root, path, "artifacts") {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

func directoryByteSize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("directory contains symlink: %s", path)
		}
		total += info.Size()
		return nil
	})
	return total, err
}

func pathHasSegment(root string, target string, segment string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if part == segment {
			return true
		}
	}
	return false
}

func workspaceFileCount(root string) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("workspace contains symlink: %s", path)
		}
		count++
		return nil
	})
	return count, err
}

func existingRegularFileSize(path string) (int64, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return 0, fmt.Errorf("target path is a symlink: %s", path)
	}
	if info.IsDir() {
		return 0, fmt.Errorf("target path is a directory: %s", path)
	}
	return info.Size(), nil
}

func regularFileExists(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("target path is a symlink: %s", path)
	}
	if info.IsDir() {
		return false, fmt.Errorf("target path is a directory: %s", path)
	}
	return true, nil
}

func projectedWorkspaceSizeForArchive(root string, destination string, entries []archiveEntry) (int64, error) {
	currentSize, err := workspaceByteSize(root)
	if err != nil {
		return 0, err
	}

	replacedSizes := make(map[string]int64)
	incomingSizes := make(map[string]int64)
	for _, entry := range entries {
		relativePath := filepath.ToSlash(filepath.Join(destination, entry.name))
		target, err := resolveWritableSandboxPath(root, relativePath)
		if err != nil {
			return 0, err
		}
		if _, seen := replacedSizes[target]; !seen {
			existingSize, err := existingRegularFileSize(target)
			if err != nil {
				return 0, err
			}
			replacedSizes[target] = existingSize
		}
		if entry.file.UncompressedSize64 > uint64(1<<63-1) {
			return 0, fmt.Errorf("file %s exceeds supported size", entry.name)
		}
		incomingSizes[target] = int64(entry.file.UncompressedSize64)
	}

	projectedSize := currentSize
	for target, existingSize := range replacedSizes {
		projectedSize -= existingSize
		projectedSize += incomingSizes[target]
	}
	return projectedSize, nil
}

func projectedWorkspaceFileCountForArchive(root string, destination string, entries []archiveEntry) (int, error) {
	currentCount, err := workspaceFileCount(root)
	if err != nil {
		return 0, err
	}

	targets := make(map[string]bool)
	for _, entry := range entries {
		relativePath := filepath.ToSlash(filepath.Join(destination, entry.name))
		target, err := resolveWritableSandboxPath(root, relativePath)
		if err != nil {
			return 0, err
		}
		if _, seen := targets[target]; seen {
			continue
		}
		exists, err := regularFileExists(target)
		if err != nil {
			return 0, err
		}
		if !exists {
			currentCount++
		}
		targets[target] = true
	}
	return currentCount, nil
}

func validateArchiveEntriesWithinLimits(entries []archiveEntry, limit archiveLimits) (int64, error) {
	var totalSize int64
	for index, entry := range entries {
		if index >= limit.maxFiles {
			return 0, fmt.Errorf("archive exceeds max file count of %d", limit.maxFiles)
		}
		if entry.file.FileInfo().Mode()&os.ModeSymlink != 0 {
			return 0, fmt.Errorf("archive contains symlink: %s", entry.name)
		}
		if entry.file.UncompressedSize64 > uint64(limit.maxFileSize) {
			return 0, fmt.Errorf("file %s exceeds max size of %d bytes", entry.name, limit.maxFileSize)
		}
		totalSize += int64(entry.file.UncompressedSize64)
		if totalSize > limit.maxTotalSize {
			return 0, fmt.Errorf("archive exceeds max total size of %d bytes", limit.maxTotalSize)
		}
	}
	return totalSize, nil
}

func resolveSandboxPath(root string, relativePath string) (string, error) {
	if strings.TrimSpace(relativePath) == "" {
		return "", errors.New("path is required")
	}

	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	clean := filepath.Clean("/" + relativePath)
	target := filepath.Join(root, clean)
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes sandbox root")
	}
	return target, nil
}

func resolveExistingSandboxPath(root string, relativePath string) (string, error) {
	target, err := resolveSandboxPath(root, relativePath)
	if err != nil {
		return "", err
	}
	if err := rejectSymlinkPath(root, target, false); err != nil {
		return "", err
	}
	return target, nil
}

func resolveWritableSandboxPath(root string, relativePath string) (string, error) {
	target, err := resolveSandboxPath(root, relativePath)
	if err != nil {
		return "", err
	}
	if err := rejectSymlinkPath(root, target, true); err != nil {
		return "", err
	}
	return target, nil
}

func rejectSymlinkPath(root string, target string, allowMissingTail bool) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return err
	}

	rootInfo, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("sandbox root is a symlink")
	}

	rel, err := filepath.Rel(root, target)
	if err != nil {
		return err
	}
	if rel == "." {
		return nil
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("path escapes sandbox root")
	}

	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) && allowMissingTail {
				return nil
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path contains symlink: %s", current)
		}
	}
	return nil
}

func decodeContent(content string, encoding string) ([]byte, error) {
	switch normalizeEncoding(encoding) {
	case "base64":
		return base64.StdEncoding.DecodeString(content)
	default:
		return []byte(content), nil
	}
}

func normalizeEncoding(encoding string) string {
	if strings.EqualFold(strings.TrimSpace(encoding), "base64") {
		return "base64"
	}
	return "utf-8"
}

func (s *Service) skillArtifactManifests(sandboxID string, packagePath string, manifest SkillExecutionManifest) ([]FileManifest, error) {
	manifests := make([]FileManifest, 0, len(manifest.AllowedArtifactPaths))
	for _, artifactPath := range manifest.AllowedArtifactPaths {
		relativePath := filepath.ToSlash(filepath.Join(packagePath, artifactPath))
		if _, err := s.StatFile(sandboxID, relativePath); err != nil {
			if os.IsNotExist(err) || strings.Contains(err.Error(), "no such file") {
				continue
			}
			return nil, err
		}
		artifactManifest, err := s.BuildFileManifestWithOptions(sandboxID, relativePath, FileManifestOptions{
			MaxFiles:      manifest.MaxArtifactCount,
			MaxTotalBytes: manifest.MaxArtifactBytes,
		})
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, *artifactManifest)
	}
	return manifests, nil
}

func skillCommand(manifest SkillExecutionManifest) (string, []string) {
	switch manifest.Language {
	case "nodejs":
		return "node", []string{manifest.Entrypoint}
	default:
		return "python3", []string{manifest.Entrypoint}
	}
}

func skillCommandProfile(manifest SkillExecutionManifest) string {
	switch manifest.Language {
	case "nodejs":
		return "skill-node"
	default:
		return "skill-python"
	}
}

func attachResultJSON(result *runner.Result, strict bool, schema *outputSchema, maxBytes int) error {
	if result == nil || result.ExitCode != 0 {
		return nil
	}
	raw := strings.TrimSpace(result.Stdout)
	if raw == "" {
		if strict || schema != nil {
			return errors.New("strict_result_json or expected_output_schema requires stdout to contain JSON")
		}
		return nil
	}
	if maxBytes > 0 && len([]byte(raw)) > maxBytes {
		if strict || schema != nil {
			return fmt.Errorf("result_json exceeds max size of %d bytes", maxBytes)
		}
		result.Warnings = append(result.Warnings, fmt.Sprintf("result_json omitted because output exceeded max size of %d bytes", maxBytes))
		return nil
	}

	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		if strict || schema != nil {
			return fmt.Errorf("strict_result_json or expected_output_schema failed to parse stdout JSON: %w", err)
		}
		return nil
	}
	if schema != nil {
		if err := schema.validate(decoded, "$"); err != nil {
			return fmt.Errorf("expected_output_schema validation failed: %w", err)
		}
	}
	result.ResultJSON = decoded
	return nil
}

type outputSchema struct {
	Type                 string                  `json:"type"`
	Required             []string                `json:"required"`
	Properties           map[string]outputSchema `json:"properties"`
	Items                *outputSchema           `json:"items"`
	AdditionalProperties *bool                   `json:"additional_properties"`
}

func parseExpectedOutputSchema(raw json.RawMessage) (*outputSchema, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > 16*1024 {
		return nil, errors.New("expected_output_schema exceeds max size of 16384 bytes")
	}
	var schema outputSchema
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, fmt.Errorf("expected_output_schema must contain valid JSON: %w", err)
	}
	if err := schema.validateDefinition("$"); err != nil {
		return nil, fmt.Errorf("invalid expected_output_schema: %w", err)
	}
	return &schema, nil
}

func (s outputSchema) validateDefinition(path string) error {
	if s.Type == "" {
		return fmt.Errorf("%s.type is required", path)
	}
	if !isSupportedOutputSchemaType(s.Type) {
		return fmt.Errorf("%s.type is unsupported: %s", path, s.Type)
	}
	if len(s.Required) > 128 {
		return fmt.Errorf("%s.required exceeds max field count of 128", path)
	}
	seenRequired := map[string]struct{}{}
	for _, name := range s.Required {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("%s.required contains an empty field", path)
		}
		if _, exists := seenRequired[name]; exists {
			return fmt.Errorf("%s.required contains duplicate field: %s", path, name)
		}
		seenRequired[name] = struct{}{}
	}
	if len(s.Properties) > 128 {
		return fmt.Errorf("%s.properties exceeds max field count of 128", path)
	}
	for name, child := range s.Properties {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("%s.properties contains an empty field", path)
		}
		if err := child.validateDefinition(path + ".properties." + name); err != nil {
			return err
		}
	}
	if s.Items != nil {
		if err := s.Items.validateDefinition(path + ".items"); err != nil {
			return err
		}
	}
	return nil
}

func isSupportedOutputSchemaType(value string) bool {
	switch value {
	case "object", "array", "string", "number", "integer", "boolean", "null":
		return true
	default:
		return false
	}
}

func (s outputSchema) validate(value any, path string) error {
	if err := validateOutputSchemaType(value, s.Type, path); err != nil {
		return err
	}
	switch s.Type {
	case "object":
		object, _ := value.(map[string]any)
		for _, name := range s.Required {
			if _, ok := object[name]; !ok {
				return fmt.Errorf("%s.%s is required", path, name)
			}
		}
		for name, child := range s.Properties {
			if childValue, ok := object[name]; ok {
				if err := child.validate(childValue, path+"."+name); err != nil {
					return err
				}
			}
		}
		if s.AdditionalProperties != nil && !*s.AdditionalProperties {
			for name := range object {
				if _, ok := s.Properties[name]; !ok {
					return fmt.Errorf("%s.%s is not allowed", path, name)
				}
			}
		}
	case "array":
		if s.Items != nil {
			items, _ := value.([]any)
			for index, item := range items {
				if err := s.Items.validate(item, fmt.Sprintf("%s[%d]", path, index)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateOutputSchemaType(value any, expected string, path string) error {
	switch expected {
	case "object":
		if _, ok := value.(map[string]any); !ok {
			return fmt.Errorf("%s must be object", path)
		}
	case "array":
		if _, ok := value.([]any); !ok {
			return fmt.Errorf("%s must be array", path)
		}
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%s must be string", path)
		}
	case "number":
		if _, ok := value.(float64); !ok {
			return fmt.Errorf("%s must be number", path)
		}
	case "integer":
		number, ok := value.(float64)
		if !ok || number != float64(int64(number)) {
			return fmt.Errorf("%s must be integer", path)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s must be boolean", path)
		}
	case "null":
		if value != nil {
			return fmt.Errorf("%s must be null", path)
		}
	}
	return nil
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func newExecutionID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "exec_" + time.Now().UTC().Format("20060102150405")
	}
	return "exec_" + hex.EncodeToString(buf[:])
}

func ListDirectory(root string) ([]FileInfo, error) {
	entries := make([]FileInfo, 0)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return statErr
		}
		entries = append(entries, FileInfo{
			Path:        strings.TrimPrefix(path, root+"/"),
			Size:        info.Size(),
			Mode:        info.Mode().String(),
			ModifiedAt:  info.ModTime().UTC(),
			IsDirectory: d.IsDir(),
		})
		return nil
	})
	return entries, err
}

func fileManifestItem(root string, path string, info fs.FileInfo) (FileManifestItem, error) {
	file, err := os.Open(path)
	if err != nil {
		return FileManifestItem{}, err
	}
	defer file.Close()

	contentType, sum, err := detectContentTypeAndHash(file, sha256.New())
	if err != nil {
		return FileManifestItem{}, err
	}
	rel := strings.TrimPrefix(path, root+string(filepath.Separator))
	return FileManifestItem{
		Path:        filepath.ToSlash(rel),
		Size:        info.Size(),
		Encoding:    "reference",
		SHA256:      hex.EncodeToString(sum),
		ContentType: contentType,
		ModifiedAt:  info.ModTime().UTC(),
	}, nil
}

func detectContentTypeAndHash(reader io.Reader, hasher hash.Hash) (string, []byte, error) {
	header := make([]byte, 512)
	n, err := io.ReadFull(reader, header)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", nil, err
	}
	header = header[:n]
	contentType := "application/octet-stream"
	if len(header) > 0 {
		contentType = http.DetectContentType(header)
	}
	if _, err := hasher.Write(header); err != nil {
		return "", nil, err
	}
	if _, err := io.Copy(hasher, reader); err != nil {
		return "", nil, err
	}
	return contentType, hasher.Sum(nil), nil
}

func normalizeCommandEnv(env map[string]string) (map[string]string, error) {
	if len(env) == 0 {
		return nil, nil
	}
	if len(env) > 32 {
		return nil, errors.New("env exceeds max entry count of 32")
	}

	normalized := make(map[string]string, len(env))
	for key, value := range env {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, errors.New("env key is required")
		}
		if len(key) > 64 {
			return nil, fmt.Errorf("env key exceeds max length: %s", key)
		}
		if len(value) > 4096 {
			return nil, fmt.Errorf("env value for %s exceeds max length of 4096 bytes", key)
		}
		if !validEnvKey(key) {
			return nil, fmt.Errorf("invalid env key: %s", key)
		}
		if dangerousEnvKey(key) {
			return nil, fmt.Errorf("env key is not allowed: %s", key)
		}
		normalized[key] = value
	}
	return normalized, nil
}

func rejectRuntimeDependencyInstall(command string, args []string) error {
	if len(args) > 0 {
		tokens := commandTokens(command, args)
		if len(tokens) > 0 && isShellCommand(tokens[0]) {
			if match := shellArgDependencyInstallMatch(command, args); match != nil {
				match.Command = strings.TrimSpace(command)
				return match
			}
			return nil
		}
		tokens = trimShellCommandPrefix(tokens)
		if match := dependencyInstallHeadMatch(tokens); match != nil {
			match.Command = strings.TrimSpace(command)
			return match
		}
		return nil
	}

	if match := shellDependencyInstallMatch(command); match != nil {
		match.Command = strings.TrimSpace(command)
		return match
	}
	return nil
}

func shellArgDependencyInstallMatch(command string, args []string) *DependencyInstallError {
	for i, arg := range args {
		if cleanCommandToken(arg) == "-c" && i+1 < len(args) {
			return shellDependencyInstallMatch(args[i+1])
		}
	}
	return nil
}

func commandTokens(command string, args []string) []string {
	if len(args) > 0 {
		tokens := []string{cleanCommandToken(command)}
		for _, arg := range args {
			tokens = append(tokens, cleanCommandToken(arg))
		}
		return compactTokens(tokens)
	}

	replacer := strings.NewReplacer(
		"&&", " ",
		"||", " ",
		";", " ",
		"|", " ",
		"\n", " ",
		"\t", " ",
	)
	parts := strings.Fields(replacer.Replace(command))
	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		tokens = append(tokens, cleanCommandToken(part))
	}
	return compactTokens(tokens)
}

func shellDependencyInstallMatch(command string) *DependencyInstallError {
	for _, segment := range shellCommandSegments(command) {
		tokens := commandTokens(segment, nil)
		tokens = trimShellCommandPrefix(tokens)
		if match := dependencyInstallHeadMatch(tokens); match != nil {
			return match
		}
	}
	return nil
}

func shellCommandSegments(command string) []string {
	replacer := strings.NewReplacer(
		"&&", "\n",
		"||", "\n",
		";", "\n",
		"|", "\n",
	)
	parts := strings.Split(replacer.Replace(command), "\n")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			segments = append(segments, part)
		}
	}
	return segments
}

func trimShellCommandPrefix(tokens []string) []string {
	for len(tokens) > 0 {
		switch {
		case tokens[0] == "env" || tokens[0] == "sudo" || tokens[0] == "command":
			tokens = tokens[1:]
		case strings.Contains(tokens[0], "="):
			tokens = tokens[1:]
		default:
			return tokens
		}
	}
	return tokens
}

func compactTokens(tokens []string) []string {
	compacted := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if token != "" {
			compacted = append(compacted, token)
		}
	}
	return compacted
}

func cleanCommandToken(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	value = strings.TrimRight(value, ",")
	return strings.ToLower(filepath.Base(value))
}

func dependencyInstallMatch(tokens []string) *DependencyInstallError {
	for i := 0; i < len(tokens); i++ {
		if match := dependencyInstallMatchAt(tokens, i); match != nil {
			return match
		}
	}
	return nil
}

func dependencyInstallHeadMatch(tokens []string) *DependencyInstallError {
	return dependencyInstallMatchAt(tokens, 0)
}

func dependencyInstallMatchAt(tokens []string, index int) *DependencyInstallError {
	if index >= len(tokens) {
		return nil
	}
	token := tokens[index]
	switch {
	case isPipCommand(token) && nextTokenIs(tokens, index, "install"):
		return dependencyInstallError(token, tokens[index+1])
	case isPythonCommand(token) && nextTokenIs(tokens, index, "-m") && nextTokenIsPip(tokens, index+1) && nextTokenIs(tokens, index+2, "install"):
		return dependencyInstallError(tokens[index+2], tokens[index+3])
	case token == "uv" && nextTokenIs(tokens, index, "pip") && nextTokenIsAny(tokens, index+1, "install", "sync"):
		return dependencyInstallError("uv pip", tokens[index+2])
	case token == "npm" && nextTokenIsAny(tokens, index, "install", "i", "ci", "add"):
		return dependencyInstallError(token, tokens[index+1])
	case token == "pnpm" && nextTokenIsAny(tokens, index, "install", "i", "add"):
		return dependencyInstallError(token, tokens[index+1])
	case token == "yarn" && nextTokenIsAny(tokens, index, "install", "add"):
		return dependencyInstallError(token, tokens[index+1])
	case token == "bun" && nextTokenIsAny(tokens, index, "install", "add"):
		return dependencyInstallError(token, tokens[index+1])
	case token == "poetry" && nextTokenIsAny(tokens, index, "install", "add"):
		return dependencyInstallError(token, tokens[index+1])
	default:
		return nil
	}
}

func dependencyInstallError(manager string, action string) *DependencyInstallError {
	return &DependencyInstallError{PackageManager: manager, Action: action}
}

func isPipCommand(value string) bool {
	return value == "pip" || value == "pip3" || value == "pipx"
}

func isPythonCommand(value string) bool {
	return value == "python" || value == "python3" || value == "py"
}

func isShellCommand(value string) bool {
	return value == "sh" || value == "bash" || value == "dash" || value == "zsh"
}

func nextTokenIs(tokens []string, index int, value string) bool {
	next := index + 1
	return next < len(tokens) && tokens[next] == value
}

func nextTokenIsPip(tokens []string, index int) bool {
	next := index + 1
	return next < len(tokens) && isPipCommand(tokens[next])
}

func nextTokenIsAny(tokens []string, index int, values ...string) bool {
	next := index + 1
	if next >= len(tokens) {
		return false
	}
	for _, value := range values {
		if tokens[next] == value {
			return true
		}
	}
	return false
}

func validEnvKey(key string) bool {
	for i, char := range key {
		if char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || char == '_' {
			continue
		}
		if i > 0 && char >= '0' && char <= '9' {
			continue
		}
		return false
	}
	return true
}

func dangerousEnvKey(key string) bool {
	upper := strings.ToUpper(key)
	switch upper {
	case "PATH", "HOME", "IFS", "SHELLOPTS", "BASH_ENV", "ENV", "PYTHONPATH", "NODE_OPTIONS":
		return true
	default:
		return strings.HasPrefix(upper, "LD_") || strings.HasPrefix(upper, "DYLD_")
	}
}

type archiveLimits struct {
	maxFiles     int
	maxFileSize  int64
	maxTotalSize int64
}

type archiveEntry struct {
	name string
	file *zip.File
}

func normalizeArchiveEntries(files []*zip.File, stripRoot bool) ([]archiveEntry, error) {
	root := commonArchiveRoot(files)
	entries := make([]archiveEntry, 0, len(files))
	for _, file := range files {
		name := strings.TrimSpace(filepath.ToSlash(file.Name))
		if name == "" {
			continue
		}
		if strings.HasPrefix(name, "/") || strings.Contains(name, "\x00") {
			return nil, fmt.Errorf("invalid archive path: %s", file.Name)
		}
		if stripRoot && root != "" {
			name = strings.TrimPrefix(name, root+"/")
		}
		name = strings.TrimPrefix(name, "./")
		if name == "" || strings.HasSuffix(name, "/") || file.FileInfo().IsDir() {
			continue
		}
		clean := filepath.ToSlash(filepath.Clean(name))
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
			return nil, fmt.Errorf("archive path escapes target root: %s", file.Name)
		}
		entries = append(entries, archiveEntry{name: clean, file: file})
	}
	return entries, nil
}

func validateSkillExecutionManifest(entries []archiveEntry, policyService *policy.Service, sandboxDependencyProfile string) (*SkillExecutionManifest, error) {
	names := make(map[string]bool, len(entries))
	for _, entry := range entries {
		names[filepath.ToSlash(entry.name)] = true
	}

	var manifestEntry *zip.File
	for _, entry := range entries {
		if filepath.ToSlash(entry.name) == "skill.manifest.json" {
			manifestEntry = entry.file
			break
		}
	}
	if manifestEntry == nil {
		return nil, errors.New("skill.manifest.json is required when validate_skill_manifest is true")
	}
	if manifestEntry.UncompressedSize64 > 64*1024 {
		return nil, errors.New("skill.manifest.json exceeds max size of 65536 bytes")
	}
	content, err := readZipFile(manifestEntry, 64*1024)
	if err != nil {
		return nil, err
	}

	return parseSkillExecutionManifest(content, names, policyService, sandboxDependencyProfile)
}

func loadSkillExecutionManifest(packageRoot string, policyService *policy.Service, sandboxDependencyProfile string) (SkillExecutionManifest, error) {
	manifestPath := filepath.Join(packageRoot, "skill.manifest.json")
	info, err := os.Lstat(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return SkillExecutionManifest{}, errors.New("skill.manifest.json is required")
		}
		return SkillExecutionManifest{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return SkillExecutionManifest{}, errors.New("skill.manifest.json must not be a symlink")
	}
	if info.IsDir() {
		return SkillExecutionManifest{}, errors.New("skill.manifest.json must be a file")
	}
	if info.Size() > 64*1024 {
		return SkillExecutionManifest{}, errors.New("skill.manifest.json exceeds max size of 65536 bytes")
	}
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		return SkillExecutionManifest{}, err
	}

	names := make(map[string]bool)
	err = filepath.WalkDir(packageRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == packageRoot || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("skill package contains symlink: %s", path)
		}
		relative, err := filepath.Rel(packageRoot, path)
		if err != nil {
			return err
		}
		names[filepath.ToSlash(relative)] = true
		return nil
	})
	if err != nil {
		return SkillExecutionManifest{}, err
	}

	manifest, err := parseSkillExecutionManifest(content, names, policyService, sandboxDependencyProfile)
	if err != nil {
		return SkillExecutionManifest{}, err
	}
	return *manifest, nil
}

func parseSkillExecutionManifest(content []byte, names map[string]bool, policyService *policy.Service, sandboxDependencyProfile string) (*SkillExecutionManifest, error) {
	var manifest SkillExecutionManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return nil, fmt.Errorf("invalid skill.manifest.json: %w", err)
	}
	if err := validateSkillManifestFields(&manifest, names, policyService, sandboxDependencyProfile); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func validateSkillManifestFields(manifest *SkillExecutionManifest, names map[string]bool, policyService *policy.Service, sandboxDependencyProfile string) error {
	manifest.Entrypoint = filepath.ToSlash(strings.TrimSpace(manifest.Entrypoint))
	if manifest.Entrypoint == "" {
		return errors.New("skill manifest entrypoint is required")
	}
	if unsafeArchivePath(manifest.Entrypoint) {
		return fmt.Errorf("skill manifest entrypoint escapes package root: %s", manifest.Entrypoint)
	}
	if !strings.HasPrefix(manifest.Entrypoint, "scripts/") {
		return fmt.Errorf("skill manifest entrypoint must be under scripts: %s", manifest.Entrypoint)
	}
	if !names[manifest.Entrypoint] {
		return fmt.Errorf("skill manifest entrypoint is missing from package: %s", manifest.Entrypoint)
	}

	manifest.Language = normalizeSkillLanguage(manifest.Language)
	if manifest.Language == "" {
		return errors.New("skill manifest language must be python3 or nodejs")
	}
	switch manifest.Language {
	case "python3":
		if filepath.Ext(manifest.Entrypoint) != ".py" {
			return fmt.Errorf("skill manifest python3 entrypoint must use .py: %s", manifest.Entrypoint)
		}
	case "nodejs":
		if filepath.Ext(manifest.Entrypoint) != ".js" {
			return fmt.Errorf("skill manifest nodejs entrypoint must use .js: %s", manifest.Entrypoint)
		}
	}
	if policyService == nil {
		return errors.New("dependency profile policy is not configured")
	}
	if strings.TrimSpace(manifest.DependencyProfile) == "" {
		manifest.DependencyProfile = strings.TrimSpace(sandboxDependencyProfile)
	}
	dependency, err := policyService.ValidateDependencyProfileForLanguage(manifest.DependencyProfile, manifest.Language)
	if err != nil {
		return err
	}
	manifest.DependencyProfile = dependency.Name
	if selected := strings.TrimSpace(sandboxDependencyProfile); selected != "" && selected != manifest.DependencyProfile {
		return fmt.Errorf("skill manifest dependency_profile %s does not match sandbox dependency_profile: %s", manifest.DependencyProfile, selected)
	}
	if manifest.TimeoutMS <= 0 || manifest.TimeoutMS > 300000 {
		return errors.New("skill manifest timeout_ms must be between 1 and 300000")
	}
	maxArtifactFiles := policyService.MaxArtifactManifestFiles()
	if maxArtifactFiles <= 0 {
		maxArtifactFiles = 100
	}
	if manifest.MaxArtifactCount <= 0 || manifest.MaxArtifactCount > maxArtifactFiles {
		return fmt.Errorf("skill manifest max_artifact_count must be between 1 and %d", maxArtifactFiles)
	}
	maxArtifactBytes := policyService.MaxArtifactManifestBytes()
	if manifest.MaxArtifactBytes <= 0 || manifest.MaxArtifactBytes > maxArtifactBytes {
		return fmt.Errorf("skill manifest max_artifact_bytes must be between 1 and %d", maxArtifactBytes)
	}

	if len(manifest.AllowedArtifactPaths) == 0 {
		return errors.New("skill manifest allowed_artifact_paths is required")
	}
	for i, rawPath := range manifest.AllowedArtifactPaths {
		path := filepath.ToSlash(strings.TrimSpace(rawPath))
		if path == "" {
			return errors.New("skill manifest allowed_artifact_paths contains an empty path")
		}
		if unsafeArchivePath(path) {
			return fmt.Errorf("skill manifest allowed artifact path escapes package root: %s", path)
		}
		if path != "artifacts" && !strings.HasPrefix(path, "artifacts/") {
			return fmt.Errorf("skill manifest allowed artifact path must be under artifacts: %s", path)
		}
		manifest.AllowedArtifactPaths[i] = path
	}

	manifest.ResultMode = strings.TrimSpace(manifest.ResultMode)
	switch manifest.ResultMode {
	case "stdout_json", "stdout_text", "artifacts", "mixed":
		return nil
	default:
		return errors.New("skill manifest result_mode must be stdout_json, stdout_text, artifacts, or mixed")
	}
}

func normalizeSkillLanguage(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "python", "python3":
		return "python3"
	case "node", "nodejs", "javascript":
		return "nodejs"
	default:
		return ""
	}
}

func unsafeArchivePath(path string) bool {
	path = filepath.ToSlash(strings.TrimSpace(path))
	if path == "" || strings.HasPrefix(path, "/") || filepath.IsAbs(path) {
		return true
	}
	for _, part := range strings.Split(path, "/") {
		if part == ".." {
			return true
		}
	}
	return filepath.Clean(path) == "."
}

func commonArchiveRoot(files []*zip.File) string {
	root := ""
	for _, file := range files {
		name := strings.Trim(filepath.ToSlash(file.Name), "/")
		if name == "" {
			continue
		}
		if file.FileInfo().IsDir() && !strings.Contains(name, "/") {
			continue
		}
		parts := strings.Split(name, "/")
		if len(parts) < 2 {
			return ""
		}
		if root == "" {
			root = parts[0]
			continue
		}
		if root != parts[0] {
			return ""
		}
	}
	return root
}

func readZipFile(file *zip.File, maxSize int64) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	content := make([]byte, 0)
	buffer := make([]byte, 32*1024)
	var total int64
	for {
		n, readErr := reader.Read(buffer)
		if n > 0 {
			total += int64(n)
			if total > maxSize {
				return nil, fmt.Errorf("file %s exceeds max size of %d bytes", file.Name, maxSize)
			}
			content = append(content, buffer[:n]...)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	return content, nil
}

type fileSnapshot struct {
	path    string
	exists  bool
	content []byte
	mode    os.FileMode
}

func snapshotExistingFile(path string) (fileSnapshot, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fileSnapshot{path: path}, nil
		}
		return fileSnapshot{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fileSnapshot{}, fmt.Errorf("target path is a symlink: %s", path)
	}
	if info.IsDir() {
		return fileSnapshot{}, fmt.Errorf("target path is a directory: %s", path)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return fileSnapshot{}, err
	}
	return fileSnapshot{
		path:    path,
		exists:  true,
		content: content,
		mode:    info.Mode().Perm(),
	}, nil
}

func rollbackWrittenFiles(files []fileSnapshot) {
	for i := len(files) - 1; i >= 0; i-- {
		file := files[i]
		if file.exists {
			_ = os.WriteFile(file.path, file.content, file.mode)
			continue
		}
		_ = os.Remove(file.path)
	}
}
