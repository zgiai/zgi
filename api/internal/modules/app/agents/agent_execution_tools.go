package agents

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/dto"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/apperror"
)

const (
	agentExecutionMaxCommandBytes = 8192
	agentExecutionMaxOutputBytes  = 32 << 10
	agentExecutionMaxTimeout      = 60 * time.Second
)

func (h *AgentsHandler) attachConsoleAgentExecutionTools(runConfig *runtimeservice.RunConfig, scope runtimeservice.Scope, agentID uuid.UUID, cfg dto.AgentConfigResponse) error {
	if runConfig == nil || cfg.ExecutionRuntime == nil || !cfg.ExecutionRuntime.Enabled {
		return nil
	}
	resolvedRuntime, err := h.executionApprovals.resolve(scope.OrganizationID.String(), cfg.ExecutionRuntime)
	if err != nil {
		return err
	}
	if h.executionGateway == nil {
		return apperror.New(AppCodeAgentExecutionRuntimeUnavailable, apperror.WithOperation("agent_execution.gateway_unavailable"))
	}
	profileID := resolvedRuntime.ProfileID
	profileApproval := h.executionApprovals.Profiles[profileID]
	backend := profileApproval.Backend
	gateway := h.executionGateway
	if resolvedRuntime.HomeSnapshotID != nil && strings.TrimSpace(*resolvedRuntime.HomeSnapshotID) != "" {
		gateway = agentExecutionSnapshotGateway{
			agentExecutionGateway: gateway,
			homeSnapshotID:        strings.TrimSpace(*resolvedRuntime.HomeSnapshotID),
		}
	}
	fileReader := h.fileService
	persister := h.executionArtifactPersister
	if persister == nil {
		persister = defaultAgentExecutionArtifactPersister{}
	}
	skillResolver := newAgentSkillPackageResolver(h.db, scope.OrganizationID, cfg.EnabledSkillIDs)
	runConfig.RuntimeToolFactory = func(prepared *runtimeservice.PreparedChat) []skillloop.RuntimeTool {
		if prepared == nil || prepared.Conversation == nil || prepared.Message == nil ||
			prepared.Caller.Source != runtimemodel.ConversationSourceConsole ||
			prepared.Caller.Type != runtimemodel.ConversationCallerAgent ||
			prepared.Caller.ID == nil || *prepared.Caller.ID != agentID ||
			prepared.Scope.OrganizationID != scope.OrganizationID ||
			prepared.Scope.AccountID != scope.AccountID {
			return nil
		}
		executionScope := agentExecutionScope{
			OrganizationID: scope.OrganizationID.String(), SubjectID: scope.AccountID.String(),
		}
		tools := []skillloop.RuntimeTool{
			newAgentExecutionShellToolWithNetworkPolicy(gateway, executionScope, agentID.String(), prepared.Conversation.ID.String(), prepared.Message.ID.String(), profileID, backend, resolvedRuntime.PolicyID, prepared.RuntimeAttachmentRefs(), fileReader, skillResolver),
			newAgentExecutionExportTool(gateway, persister, executionScope, agentID.String(), prepared.Conversation.ID.String(), prepared.Message.ID.String(), profileID, backend, resolvedRuntime.PolicyID),
		}
		shellHandler := tools[0].Handler
		tools[0].Handler = func(ctx context.Context, call adapter.ToolCall) skillloop.RuntimeToolResult {
			return projectAgentExecutionToolFailure(shellHandler(ctx, call), h.appErrorProjector)
		}
		if profileApproval.DependencyPreparation && !profileApproval.DependencyInstallation {
			tools = append(tools, newAgentExecutionDependencyPreparationTool(gateway, executionScope, agentID.String(), prepared.Conversation.ID.String(), prepared.Message.ID.String(), profileID, backend, resolvedRuntime.PolicyID, skillResolver))
		}
		return tools
	}
	return nil
}

type agentRuntimeAttachmentReader interface {
	DownloadFile(context.Context, string) ([]byte, error)
}

func newAgentExecutionShellTool(gateway agentExecutionGateway, scope agentExecutionScope, agentID, conversationID, messageID, profileID, backend string, attachments []runtimeservice.RuntimeAttachmentRef, fileReader agentRuntimeAttachmentReader, skillResolvers ...agentSkillPackageResolver) skillloop.RuntimeTool {
	return newAgentExecutionShellToolWithNetworkPolicy(gateway, scope, agentID, conversationID, messageID, profileID, backend, "offline", attachments, fileReader, skillResolvers...)
}

func newAgentExecutionShellToolWithNetworkPolicy(gateway agentExecutionGateway, scope agentExecutionScope, agentID, conversationID, messageID, profileID, backend, networkPolicy string, attachments []runtimeservice.RuntimeAttachmentRef, fileReader agentRuntimeAttachmentReader, skillResolvers ...agentSkillPackageResolver) skillloop.RuntimeTool {
	var stageOnce sync.Once
	var inputPaths []string
	var stageErr error
	stage := func(ctx context.Context, bindingID string, capabilities agentExecutionCapabilities) ([]string, error) {
		stageOnce.Do(func() {
			inputPaths, stageErr = stageAgentExecutionAttachments(ctx, gateway, scope, bindingID, capabilities, attachments, fileReader)
		})
		return inputPaths, stageErr
	}
	return skillloop.RuntimeTool{
		SkillID: "agent_execution",
		Definition: adapter.Tool{Type: "function", Function: adapter.Function{
			Name:        "shell_exec",
			Description: agentExecutionShellDescription(networkPolicy),
			Parameters: map[string]any{
				"type": "object", "additionalProperties": false,
				"properties": map[string]any{
					"command":         map[string]any{"type": "string", "description": "Shell command to run in the workspace", "maxLength": agentExecutionMaxCommandBytes},
					"timeout_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 60},
					"skill_id":        map[string]any{"type": "string", "description": "Enabled custom Skill package to stage before this command"},
					"package_sha256":  map[string]any{"type": "string", "description": "Exact package_sha256 supplied in session_scripts for skill_id", "minLength": 64, "maxLength": 64},
				},
				"required": []string{"command"},
			},
		}},
		Handler: func(ctx context.Context, call adapter.ToolCall) skillloop.RuntimeToolResult {
			return executeAgentShellToolWithNetworkPolicy(ctx, gateway, scope, agentID, conversationID, messageID, profileID, backend, networkPolicy, call, stage, skillResolvers...)
		},
	}
}

func agentExecutionShellDescription(networkPolicy string) string {
	network := "The requested network mode is offline: network access and dependency installation are unavailable."
	switch networkPolicy {
	case agentExecutionPackageNetworkMode:
		network = "The requested network mode is package-install: pip, python -m pip, npm, and guarded apt-get may install missing packages from approved package sources only. Diagnose missing imports or commands, install the missing package, and retry; no ZGI manifest or dependency declaration is required. Do not call workspace_prepare_dependencies in this mode. General Internet access is unavailable."
	case agentExecutionDependencyPreparationModeControlledHTTP:
		network = "The requested network mode is controlled-http: only approved HTTP(S) destinations are accessible. Use workspace_prepare_dependencies only if that legacy tool is explicitly available."
	}
	return "Run a command in the approved, isolated Linux workspace for this console conversation. " + network +
		" The runtime_capabilities in the result report the actual environment; an existing conversation can retain its previous network mode. Current-turn uploaded files are staged under inputs/ before the command. To run an active custom Skill's packaged Python or Node script, pass skill_id and the package_sha256 supplied in session_scripts, then use the Skill's documented paths under $ZGI_SKILL_DIR (the complete package root, not necessarily scripts/). Run from that directory if local module imports require it. Reload or reactivate the Skill if its package version changes. The workspace may be released after inactivity."
}

func executeAgentShellTool(ctx context.Context, gateway agentExecutionGateway, scope agentExecutionScope, agentID, conversationID, messageID, profileID, backend string, call adapter.ToolCall, stage func(context.Context, string, agentExecutionCapabilities) ([]string, error), skillResolvers ...agentSkillPackageResolver) skillloop.RuntimeToolResult {
	return executeAgentShellToolWithNetworkPolicy(ctx, gateway, scope, agentID, conversationID, messageID, profileID, backend, "offline", call, stage, skillResolvers...)
}

func executeAgentShellToolWithNetworkPolicy(ctx context.Context, gateway agentExecutionGateway, scope agentExecutionScope, agentID, conversationID, messageID, profileID, backend, networkPolicy string, call adapter.ToolCall, stage func(context.Context, string, agentExecutionCapabilities) ([]string, error), skillResolvers ...agentSkillPackageResolver) skillloop.RuntimeToolResult {
	failed := func(code string, err error) skillloop.RuntimeToolResult {
		return skillloop.RuntimeToolResult{
			Status: "failed", Result: map[string]interface{}{"error_code": code},
			ModelResult: map[string]interface{}{"error_code": code}, Error: err,
		}
	}
	var args struct {
		Command        string `json:"command"`
		TimeoutSeconds int    `json:"timeout_seconds"`
		SkillID        string `json:"skill_id"`
		PackageSHA256  string `json:"package_sha256"`
	}
	decoder := json.NewDecoder(strings.NewReader(call.Function.Arguments))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&args); err != nil {
		return failed("invalid_arguments", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return failed("invalid_arguments", fmt.Errorf("unexpected trailing shell arguments"))
	}
	if strings.TrimSpace(args.Command) == "" || len(args.Command) > agentExecutionMaxCommandBytes || strings.ContainsRune(args.Command, '\x00') {
		return failed("invalid_command", fmt.Errorf("shell command is empty or exceeds its limit"))
	}
	if args.TimeoutSeconds == 0 {
		args.TimeoutSeconds = 30
	}
	if args.TimeoutSeconds < 1 || args.TimeoutSeconds > 60 {
		return failed("invalid_timeout", fmt.Errorf("shell timeout is out of range"))
	}
	if strings.TrimSpace(args.SkillID) == "" {
		if args.PackageSHA256 != "" {
			return failed("invalid_arguments", fmt.Errorf("package version requires a Skill ID"))
		}
	} else if len(args.PackageSHA256) != 64 || strings.ToLower(args.PackageSHA256) != args.PackageSHA256 {
		return failed("skill_version_required", fmt.Errorf("pass the active Skill's session_scripts package_sha256 before running its script"))
	} else if _, err := hex.DecodeString(args.PackageSHA256); err != nil {
		return failed("skill_version_required", fmt.Errorf("pass the active Skill's session_scripts package_sha256 before running its script"))
	}
	timeout := time.Duration(args.TimeoutSeconds) * time.Second
	operationCtx, cancel := context.WithTimeout(ctx, timeout+120*time.Second)
	defer cancel()
	binding, err := readyAgentExecutionBinding(operationCtx, gateway, scope, agentID, conversationID, profileID, backend, networkPolicy)
	if err != nil {
		return failed("binding_failed", err)
	}
	capabilities, err := gateway.Capabilities(operationCtx, scope, binding.ID)
	if err != nil {
		return failed("capability_check_failed", err)
	}
	if !capabilities.Shell || !agentExecutionShellNetworkAllowed(capabilities) {
		return failed("capability_denied", fmt.Errorf("agent execution profile does not provide an approved shell network policy"))
	}
	var inputPaths []string
	if stage != nil {
		inputPaths, err = stage(operationCtx, binding.ID, capabilities)
		if err != nil {
			return failed("file_staging_failed", err)
		}
	}
	var skillPath string
	if strings.TrimSpace(args.SkillID) != "" {
		if len(skillResolvers) == 0 || skillResolvers[0] == nil {
			return failed("skill_package_unavailable", fmt.Errorf("Skill package execution is unavailable for this caller"))
		}
		pack, err := skillResolvers[0](operationCtx, args.SkillID)
		if err != nil {
			return failed("skill_package_unavailable", err)
		}
		if pack.ExpectedDigest != args.PackageSHA256 {
			return failed("skill_version_changed", fmt.Errorf("skill package changed since activation; start a new turn to load the updated Skill before running its script"))
		}
		skillPath, err = stageAgentSkillPackage(operationCtx, gateway, scope, binding, capabilities, pack)
		if err != nil {
			return failed("skill_package_staging_failed", err)
		}
	}
	callID := strings.TrimSpace(call.ID)
	if callID == "" {
		callID = uuid.NewString()
	}
	command := args.Command
	if skillPath != "" {
		command = "ZGI_SKILL_DIR='/workspace/" + skillPath + "'; export ZGI_SKILL_DIR; " + command
	}
	// Login shells source /etc/profile and overwrite the injected session PATH
	// and HOME, sending pip back to the base Profile instead of its retained venv.
	job, err := gateway.SubmitJob(operationCtx, scope, binding.ID, "tool:"+messageID+":"+callID, "/bin/sh", []string{"-c", command}, timeout)
	if err != nil {
		return failed("job_submission_failed", err)
	}
	for job.Status == "queued" || job.Status == "running" {
		select {
		case <-operationCtx.Done():
			return failed("job_interrupted", cancelInterruptedAgentExecutionJob(operationCtx, gateway, scope, job))
		case <-time.After(350 * time.Millisecond):
		}
		nextJob, err := gateway.GetJob(operationCtx, scope, job.ID)
		if err != nil {
			if operationCtx.Err() != nil {
				return failed("job_interrupted", cancelInterruptedAgentExecutionJob(operationCtx, gateway, scope, job))
			}
			return failed("job_status_failed", err)
		}
		job = nextJob
	}
	stdout, stderr, truncated, err := readAgentExecutionOutput(operationCtx, gateway, scope, job.ID)
	if err != nil {
		return failed("job_output_failed", err)
	}
	result := map[string]interface{}{"job_id": job.ID, "status": job.Status, "exit_code": job.ExitCode, "truncated": truncated, "input_count": len(inputPaths), "workspace_recreated": binding.Recreated}
	if skillPath != "" {
		result["skill_workspace_path"] = skillPath
	}
	toolResult := skillloop.RuntimeToolResult{
		Status: "success", Arguments: map[string]interface{}{"command_bytes": len(args.Command)}, Result: result,
		ModelResult: map[string]interface{}{"job_id": job.ID, "status": job.Status, "exit_code": job.ExitCode, "stdout": stdout, "stderr": stderr, "truncated": truncated, "input_files": inputPaths, "workspace_recreated": binding.Recreated, "skill_workspace_path": skillPath,
			"runtime_capabilities": map[string]interface{}{
				"network_mode": capabilities.NetworkMode, "dependency_installation": capabilities.DependencyInstallation,
				"package_network_mode": capabilities.PackageNetworkMode,
			},
		},
	}
	if job.Status == "failed" && (job.FailureCode != "" || job.ExitCode == nil || *job.ExitCode < 0) {
		code, appCode := agentExecutionJobFailure(job.FailureCode)
		toolResult.Status = "failed"
		toolResult.Error = apperror.New(appCode, apperror.WithOperation("agent_execution.shell_job"))
		toolResult.Result["error_code"] = code
		toolResult.ModelResult["error_code"] = code
	}
	return toolResult
}

func agentExecutionShellNetworkAllowed(capabilities agentExecutionCapabilities) bool {
	if capabilities.NetworkMode == "disabled" {
		return true
	}
	if capabilities.NetworkMode == "package-install" {
		return capabilities.DependencyInstallation && capabilities.PackageNetworkMode == "package-install" &&
			(capabilities.PackageNetworkEnforced || strings.TrimSpace(capabilities.EgressProxyAddress) != "")
	}
	return capabilities.NetworkMode == "controlled-http" && strings.TrimSpace(capabilities.EgressProxyAddress) != ""
}

func cancelInterruptedAgentExecutionJob(ctx context.Context, gateway agentExecutionGateway, scope agentExecutionScope, job agentExecutionJob) error {
	cancelCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := gateway.CancelJob(cancelCtx, scope, job.ID, job.Generation); err != nil {
		return errors.Join(ctx.Err(), fmt.Errorf("cancel interrupted agent execution job: %w", err))
	}
	return ctx.Err()
}

func stageAgentExecutionAttachments(ctx context.Context, gateway agentExecutionGateway, scope agentExecutionScope, bindingID string, capabilities agentExecutionCapabilities, attachments []runtimeservice.RuntimeAttachmentRef, reader agentRuntimeAttachmentReader) ([]string, error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	if !capabilities.FileTransfer || capabilities.MaxUploadBytes <= 0 || reader == nil {
		return nil, fmt.Errorf("agent execution file transfer is unavailable")
	}
	paths := make([]string, 0, len(attachments))
	var total int64
	for _, attachment := range attachments {
		fileID, err := uuid.Parse(attachment.ID)
		if err != nil || attachment.Size < 0 || attachment.Size > capabilities.MaxUploadBytes || attachment.Size > 64<<20 {
			return nil, fmt.Errorf("agent execution attachment is invalid or exceeds the transfer limit")
		}
		total += attachment.Size
		if total > 64<<20 {
			return nil, fmt.Errorf("agent execution attachments exceed the per-turn transfer limit")
		}
		data, err := reader.DownloadFile(ctx, attachment.ID)
		if err != nil {
			return nil, fmt.Errorf("download authorized attachment: %w", err)
		}
		if int64(len(data)) != attachment.Size || int64(len(data)) > capabilities.MaxUploadBytes {
			return nil, fmt.Errorf("authorized attachment size changed")
		}
		ext := strings.ToLower(path.Ext(attachment.Name))
		if len(ext) > 12 || strings.Trim(ext, ".abcdefghijklmnopqrstuvwxyz0123456789") != "" {
			ext = ""
		}
		filePath := "inputs/" + fileID.String() + ext
		if err := gateway.UploadFile(ctx, scope, bindingID, filePath, data); err != nil {
			return nil, fmt.Errorf("stage authorized attachment: %w", err)
		}
		paths = append(paths, filePath)
	}
	return paths, nil
}

func readAgentExecutionOutput(ctx context.Context, gateway agentExecutionGateway, scope agentExecutionScope, jobID string) (string, string, bool, error) {
	var stdout, stderr strings.Builder
	after := int64(-1)
	truncated := false
	for {
		chunks, err := gateway.JobOutput(ctx, scope, jobID, after, 32)
		if err != nil {
			return "", "", false, err
		}
		if len(chunks) == 0 {
			break
		}
		for _, chunk := range chunks {
			if chunk.Sequence <= after {
				return "", "", false, fmt.Errorf("agent execution output sequence did not advance")
			}
			after = chunk.Sequence
			remaining := agentExecutionMaxOutputBytes - stdout.Len() - stderr.Len()
			if remaining <= 0 {
				truncated = true
				break
			}
			data := chunk.Data
			if len(data) > remaining {
				data = data[:remaining]
				truncated = true
			}
			if chunk.Stream == "stderr" {
				_, _ = stderr.Write(data)
			} else if chunk.Stream == "stdout" {
				_, _ = stdout.Write(data)
			}
		}
		if truncated || len(chunks) < 32 {
			break
		}
	}
	return strings.ToValidUTF8(stdout.String(), "�"), strings.ToValidUTF8(stderr.String(), "�"), truncated, nil
}
