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

// A package may declare mutually exclusive input roles. Keep their number
// bounded independently of per-file byte limits.
const maxSkillScriptInputFileCount = 16
const maxSkillScriptInputFileBytes = 10 * 1024 * 1024
const maxManagedSkillScriptArtifactCount = 20
const maxManagedSkillScriptArtifactBytes = 256 * 1024 * 1024
const maxManagedSkillScriptInputFileBytes = 256 * 1024 * 1024
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
	RuntimeGrantsPath string
	Endpoint          string
	APIKey            string
	// FallbackDependencyProfile is used when the sandbox is configured for
	// operator-managed dependency profiles and does not permit builds during
	// a skill request. It should point at a prebuilt, verified profile such as
	// scientific-data; an empty value preserves the dynamic-build behavior.
	FallbackDependencyProfile string
	// DependencyScanDisabled routes default-profile skills directly to the
	// configured prebuilt profile without inspecting or building dependencies.
	DependencyScanDisabled      bool
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
	runtimeGrants             []ScriptRuntimeGrant
	runtimeGrantsError        error
	endpoint                  string
	apiKey                    string
	fallbackDependencyProfile string
	dependencyScanDisabled    bool
	client                    *http.Client
	timeouts                  sandboxScriptRunnerTimeouts
	artifactPersister         SkillScriptArtifactPersister
	inputFileProvider         SkillScriptInputFileProvider
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
	grants, grantsErr := loadScriptRuntimeGrants(config.RuntimeGrantsPath)
	connectTimeout := durationOrDefault(config.ConnectTimeout, defaultSandboxConnectTimeout)
	return &SandboxScriptRunner{
		runtimeGrants:             grants,
		runtimeGrantsError:        grantsErr,
		endpoint:                  endpoint,
		apiKey:                    strings.TrimSpace(config.APIKey),
		fallbackDependencyProfile: strings.TrimSpace(config.FallbackDependencyProfile),
		dependencyScanDisabled:    config.DependencyScanDisabled,
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

func (r *SandboxScriptRunner) RunSkillScript(ctx context.Context, doc SkillDocument, arguments map[string]interface{}, execCtx ExecutionContext, callID string) (invocation *ToolInvocationResult, invocationErr error) {
	if !r.Configured() {
		return nil, fmt.Errorf("skill script runner is not configured")
	}
	if !doc.Metadata.HasScripts {
		return nil, fmt.Errorf("skill %s does not include scripts", doc.Metadata.ID)
	}
	snapshot, err := snapshotSkillPackage(doc.Metadata.RootPath)
	if err != nil {
		return nil, fmt.Errorf("snapshot skill package: %w", err)
	}
	defer os.RemoveAll(snapshot)
	doc.Metadata.RootPath = snapshot
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

	identity := map[string]interface{}{}
	defer func() {
		if invocation == nil && invocationErr != nil {
			recordSkillScriptError(&trace, start, invocationErr)
			invocation = &ToolInvocationResult{Trace: trace}
		}
		if invocation != nil {
			if invocation.Trace.Result == nil {
				invocation.Trace.Result = map[string]interface{}{}
			}
			for key, value := range identity {
				invocation.Trace.Result[key] = value
			}
		}
	}()
	grant, packageHash, err := r.runtimeGrant(doc.Metadata.RootPath, execCtx.OrganizationID)
	if err != nil {
		return nil, err
	}
	identity["package_sha256"] = packageHash
	if grant != nil {
		identity["dependency_profile"] = grant.DependencyProfile
		identity["dependency_profile_version"] = grant.ProfileVersion
		identity["dependency_artifact_checksum"] = grant.ArtifactChecksum
	}
	manifest := defaultSkillScriptManifest(docTimeoutSeconds(doc))
	var manifestRaw []byte
	if hasManifest {
		preparedManifest, err := prepareSkillScriptManifest(
			doc.Metadata.RootPath,
			docTimeoutSeconds(doc),
			grant != nil,
		)
		if err != nil {
			trace.Status = "error"
			trace.Error = err.Error()
			trace.DurationMS = time.Since(start).Milliseconds()
			return &ToolInvocationResult{Trace: trace}, err
		}
		manifest = preparedManifest.Manifest
		manifestRaw = preparedManifest.Raw
	}
	if grant != nil {
		if manifest.DependencyProfile != grant.DependencyProfile || manifest.MaxArtifactCount > grant.MaxArtifactCount || manifest.MaxArtifactBytes > grant.MaxArtifactBytes {
			return nil, fmt.Errorf("skill runtime request exceeds organization grant")
		}
		for _, input := range manifest.InputFiles {
			if input.MaxBytes > grant.MaxInputFileBytes {
				return nil, fmt.Errorf("skill input quota exceeds organization grant")
			}
		}
	}
	if manifest.ModelCalls > 0 && (execCtx.ScriptModel == nil || execCtx.ScriptModel.Complete == nil || execCtx.OrganizationID == "" || execCtx.UserID == "") {
		return nil, fmt.Errorf("skill model channel is unavailable for this execution")
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
	resolvedProfile := ""
	if dependencyProfile == defaultSkillDependencyProfile && r.dependencyScanDisabled {
		resolvedProfile, err = r.configuredPrebuiltDependencyProfile()
		if err != nil {
			recordSkillScriptError(&trace, start, err)
			return &ToolInvocationResult{Trace: trace}, err
		}
	} else if dependencyProfile == defaultSkillDependencyProfile && skillPackageHasDependencyHints(doc.Metadata.RootPath) {
		var usedFallback bool
		resolvedProfile, usedFallback = r.fallbackDependencyProfileForRuntime(ctx, execCtx, manifest.Language)
		if !usedFallback {
			resolvedProfile, err = r.prepareDependencyProfile(ctx, execCtx, archiveBase64, manifest.Language)
			if err != nil {
				recordSkillScriptError(&trace, start, err)
				return &ToolInvocationResult{Trace: trace}, err
			}
		}
	}
	if resolvedProfile != "" {
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
	if grant != nil {
		var actual struct {
			Version  string `json:"dependency_profile_version"`
			Checksum string `json:"dependency_artifact_checksum"`
		}
		if err := r.doJSON(ctx, http.MethodGet, withOrganizationQuery("/v1/sandboxes/"+sandboxID, execCtx), nil, &actual, r.timeouts.Create); err != nil {
			return nil, err
		}
		if actual.Version != grant.ProfileVersion || actual.Checksum != grant.ArtifactChecksum {
			return nil, fmt.Errorf("skill runtime version or artifact checksum does not match organization grant")
		}
	}

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
		runResult, err := r.runSkillWithModel(ctx, sandboxID, string(stdin), execCtx, manifest)
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
	trace.Result = summarizeSkillScriptResult(command, messages, artifacts)
	trace.Result["package_sha256"] = packageHash
	if grant != nil {
		trace.Result["dependency_profile"] = grant.DependencyProfile
		trace.Result["dependency_profile_version"] = grant.ProfileVersion
		trace.Result["dependency_artifact_checksum"] = grant.ArtifactChecksum
	}
	if command.ExitCode != 0 {
		err = fmt.Errorf("skill script exited with code %d: %s", command.ExitCode, strings.TrimSpace(command.Error))
	}
	if err != nil {
		recordSkillScriptError(&trace, start, err)
		return &ToolInvocationResult{Trace: trace, Messages: messages, ToolMessage: skillScriptToolMessage(callID, content)}, err
	}
	return &ToolInvocationResult{
		Messages:    messages,
		Trace:       trace,
		ToolMessage: skillScriptToolMessage(callID, content),
	}, nil
}

func skillScriptEnv(execCtx ExecutionContext) map[string]string {
	env := map[string]string{}
	if execCtx.scriptModelPath != "" && execCtx.ScriptModel != nil {
		env["ZGI_MODEL_CHANNEL"] = execCtx.scriptModelPath
		env["MODEL_NAME"] = execCtx.ScriptModel.Model
	}
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
