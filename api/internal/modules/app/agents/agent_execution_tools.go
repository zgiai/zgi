package agents

import (
	"context"
	"strings"

	"github.com/google/uuid"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	executionruntime "github.com/zgiai/zgi/api/internal/capabilities/agentexecution/runtime"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/pkg/apperror"
)

type agentExecutionGateway = executionruntime.Gateway
type agentExecutionArtifactPersister = executionruntime.ArtifactPersister
type defaultAgentExecutionArtifactPersister = executionruntime.DefaultArtifactPersister

func agentExecutionGatewayFromEnv() agentExecutionGateway { return executionruntime.GatewayFromEnv() }

// Entry adapters retain authentication and publication selection. Execution,
// file staging and platform relay are shared with other application types.
func (h *AgentsHandler) attachConsoleAgentExecutionTools(runConfig *runtimeservice.RunConfig, scope runtimeservice.Scope, agentID uuid.UUID, cfg dto.AgentConfigResponse) error {
	if runConfig == nil || cfg.ExecutionRuntime == nil || !cfg.ExecutionRuntime.Enabled {
		return nil
	}
	resolved, err := h.executionApprovals.resolve(scope.OrganizationID.String(), cfg.ExecutionRuntime)
	if err != nil {
		return err
	}
	if h.executionGateway == nil {
		return apperror.New(AppCodeAgentExecutionRuntimeUnavailable, apperror.WithOperation("agent_execution.gateway_unavailable"))
	}
	profile := h.executionApprovals.Profiles[resolved.ProfileID]
	entries, err := executionruntime.EntryAdmissionFromEnv()
	if err != nil || !entries.Allows(scope.OrganizationID.String(), "agent", "console") {
		return apperror.New(AppCodeAgentExecutionRuntimeUnavailable, apperror.WithOperation("agent_execution.entry_unapproved"))
	}
	options := executionruntime.FactoryOptions{
		Entries: entries,
		Gateway: h.executionGateway, Persister: h.executionArtifactPersister, Files: h.fileService,
		Skills:        executionruntime.NewSkillPackageResolver(h.db, scope.OrganizationID, cfg.EnabledSkillIDs),
		LegacyAgentID: agentID.String(), ProfileID: resolved.ProfileID, Backend: profile.Backend, NetworkPolicy: resolved.PolicyID,
		DependencyPreparation: profile.DependencyPreparation && !profile.DependencyInstallation,
		ErrorProjector:        h.appErrorProjector,
		ResolveScope: func(prepared *runtimeservice.PreparedChat) (execution.Scope, error) {
			if prepared == nil || prepared.Scope.OrganizationID != scope.OrganizationID || prepared.Scope.AccountID != scope.AccountID ||
				prepared.Caller.Source != runtimemodel.ConversationSourceConsole || prepared.Caller.Type != runtimemodel.ConversationCallerAgent ||
				prepared.Caller.ID == nil || *prepared.Caller.ID != agentID || prepared.Scope.RuntimeWorkspaceID != nil {
				return execution.Scope{}, execution.ErrInvalidIdentity
			}
			return execution.Scope{OrganizationID: scope.OrganizationID.String(), SubjectID: scope.AccountID.String()}, nil
		},
	}
	if h.fileService != nil {
		options.ResolveFiles = func(prepared *runtimeservice.PreparedChat, owner execution.Scope) (*executionruntime.ScopedAttachmentReader, error) {
			return executionruntime.NewScopedAttachmentReader(h.fileService, owner, prepared.RuntimeAttachmentRefs(), func(ctx context.Context, file *dto.UploadFile) error {
				return runtimeservice.AuthorizeAttachment(ctx, scope, h.organizationService, file)
			})
		}
	}
	if resolved.HomeSnapshotID != nil {
		options.HomeSnapshotID = strings.TrimSpace(*resolved.HomeSnapshotID)
	}
	if agentExecutionEnvBool("ZGI_AGENT_RUNTIME_PLATFORM_CLI_ENABLED", false) {
		if live, supported := h.appService.(interface {
			authorizeRuntimePlatform(context.Context, string, string, string) error
		}); supported {
			options.PlatformCLI, options.InteractiveApproval = true, true
			options.AuthorizePlatform = func(ctx context.Context) error {
				return live.authorizeRuntimePlatform(ctx, agentID.String(), scope.AccountID.String(), scope.OrganizationID.String())
			}
		}
	}
	runConfig.RuntimeToolFactory = executionruntime.NewToolFactory(options)
	return nil
}
