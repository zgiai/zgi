package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultSkillScriptTimeoutSeconds = 30
const inlineSkillArtifactMaxBytes = 32 * 1024
const maxSkillScriptArtifactCount = 10
const maxSkillScriptArtifactBytes = 2 * 1024 * 1024
const maxSkillScriptInputFileCount = 10
const maxSkillScriptInputFileBytes = 10 * 1024 * 1024
const defaultSkillDependencyProfile = "stdlib"

const (
	defaultSandboxConnectTimeout              = 5 * time.Second
	defaultSandboxCreateTimeout               = 10 * time.Second
	defaultSandboxUploadTimeout               = 30 * time.Second
	defaultSandboxCommandTimeoutPadding       = 15 * time.Second
	defaultSandboxArtifactTimeout             = 10 * time.Second
	defaultSandboxCleanupTimeout              = 5 * time.Second
	defaultSandboxDependencyBuildTimeout      = 60 * time.Second
	defaultSandboxDependencyBuildPollInterval = time.Second
	defaultSandboxIdempotentAttempts          = 3
	defaultSandboxRetryBaseDelay              = 50 * time.Millisecond
)

type SandboxScriptRunnerConfig struct {
	Endpoint                    string
	APIKey                      string
	ConnectTimeout              time.Duration
	CreateTimeout               time.Duration
	UploadTimeout               time.Duration
	CommandTimeoutPadding       time.Duration
	ArtifactTimeout             time.Duration
	CleanupTimeout              time.Duration
	DependencyBuildTimeout      time.Duration
	DependencyBuildPollInterval time.Duration
	ArtifactPersister           SkillScriptArtifactPersister
	InputFileProvider           SkillScriptInputFileProvider
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
	Create              time.Duration
	Upload              time.Duration
	CommandPadding      time.Duration
	Artifact            time.Duration
	Cleanup             time.Duration
	DependencyBuild     time.Duration
	DependencyBuildPoll time.Duration
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
	GetSkillScriptInputFile(ctx context.Context, fileID string, maxBytes int64, execCtx ExecutionContext) (SkillScriptInputFile, error)
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
			Create:              durationOrDefault(config.CreateTimeout, defaultSandboxCreateTimeout),
			Upload:              durationOrDefault(config.UploadTimeout, defaultSandboxUploadTimeout),
			CommandPadding:      durationOrDefault(config.CommandTimeoutPadding, defaultSandboxCommandTimeoutPadding),
			Artifact:            durationOrDefault(config.ArtifactTimeout, defaultSandboxArtifactTimeout),
			Cleanup:             durationOrDefault(config.CleanupTimeout, defaultSandboxCleanupTimeout),
			DependencyBuild:     durationOrDefault(config.DependencyBuildTimeout, defaultSandboxDependencyBuildTimeout),
			DependencyBuildPoll: durationOrDefault(config.DependencyBuildPollInterval, defaultSandboxDependencyBuildPollInterval),
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
	dependencyProfile := manifest.DependencyProfile
	if dependencyProfile == defaultSkillDependencyProfile && skillPackageHasDependencyHints(doc.Metadata.RootPath) {
		resolvedProfile, err := r.prepareDependencyProfile(ctx, execCtx, archiveBase64, manifest.Language)
		if err != nil {
			recordSkillScriptError(&trace, start, err)
			return &ToolInvocationResult{Trace: trace}, err
		}
		dependencyProfile = resolvedProfile
		manifest.DependencyProfile = resolvedProfile
		manifestRaw, err = json.Marshal(manifest)
		if err != nil {
			recordSkillScriptError(&trace, start, err)
			return &ToolInvocationResult{Trace: trace}, err
		}
		archiveBase64, err = zipSkillDirectoryBase64(doc.Metadata.RootPath, manifestRaw)
		if err != nil {
			recordSkillScriptError(&trace, start, err)
			return &ToolInvocationResult{Trace: trace}, err
		}
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
