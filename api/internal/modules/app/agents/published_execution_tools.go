package agents

import (
	"context"
	"os"

	"github.com/google/uuid"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	executionruntime "github.com/zgiai/zgi/api/internal/capabilities/agentexecution/runtime"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	"github.com/zgiai/zgi/api/pkg/apperror"
	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

// Published entries never inherit console tools or creator authorization. The
// readiness callback confirms the entry's protected-file and approval wiring;
// operator admission and live resource authorization remain separate gates.
func (h *AgentsHandler) attachPublishedAgentExecutionTools(ctx context.Context, config *runtimeservice.RunConfig, scope runtimeservice.Scope, caller runtimeservice.Caller, published *dto.AgentWebAppRuntimeConfigResponse, authenticated bool, keyID uuid.UUID) error {
	if !h.executionApprovals.Enabled || config == nil || published == nil || published.Config.ExecutionRuntime == nil || !published.Config.ExecutionRuntime.Enabled ||
		h.executionEntryReady == nil || !h.executionEntryReady(runtimemodel.ConversationCallerAgent, caller.Source) {
		return nil
	}
	// Anonymous chat and Runtime Workspace chat retain their existing behavior,
	// but neither can acquire a Linux environment through this adapter.
	if scope.RuntimeWorkspaceID != nil || scope.RuntimeSubjectID != nil || (caller.Source == runtimemodel.ConversationSourceWebApp && !authenticated) {
		return nil
	}
	access, err := publishedExecutionAccessFor(scope, caller, published, authenticated)
	if err != nil {
		return publishedExecutionDenied(err)
	}
	resolved, err := h.executionApprovals.resolve(access.organization.String(), published.Config.ExecutionRuntime)
	if err != nil {
		return err
	}
	entries, err := execution.ParseEntryAdmission(os.Getenv("ZGI_AGENT_RUNTIME_ENTRY_ALLOWLIST_JSON"))
	if err != nil || !entries.Allows(access.organization.String(), "agent", access.entry) || h.executionGateway == nil || h.fileService == nil || tool_file.GlobalToolFileManager == nil {
		return apperror.New(AppCodeAgentExecutionRuntimeUnavailable, apperror.WithOperation("agent_execution.published_entry_unavailable"))
	}
	authorize := func(ctx context.Context) error { return h.authorizePublishedExecution(ctx, access, keyID) }
	if err := authorize(ctx); err != nil {
		return publishedExecutionDenied(err)
	}
	profile := h.executionApprovals.Profiles[resolved.ProfileID]
	options := executionruntime.FactoryOptions{
		Gateway: h.executionGateway, Entries: entries, EntryReady: h.executionEntryReady,
		ProfileID: resolved.ProfileID, Backend: profile.Backend, NetworkPolicy: resolved.PolicyID,
		DependencyPreparation: profile.DependencyPreparation && !profile.DependencyInstallation,
		Skills:                executionruntime.NewSkillPackageResolver(h.db, access.organization, append([]string(nil), published.Config.EnabledSkillIDs...)),
		ResolveScope:          access.resolve, ResolveFiles: h.resolveAgentExecutionInputs, Authorize: authorize, ErrorProjector: h.appErrorProjector,
		ResolvePersister: func(prepared *runtimeservice.PreparedChat, owner execution.Scope) executionruntime.ArtifactPersister {
			persister, err := executionruntime.NewProtectedArtifactPersister(tool_file.GlobalToolFileManager, owner, prepared.Message.ID.String())
			if err != nil {
				return nil
			}
			return persister
		},
	}
	if resolved.HomeSnapshotID != nil {
		options.HomeSnapshotID = *resolved.HomeSnapshotID
	}
	if agentExecutionEnvBool("ZGI_AGENT_RUNTIME_PLATFORM_CLI_ENABLED", false) {
		options.PlatformCLI, options.InteractiveApproval = true, true
		options.AuthorizePlatform = authorize
	}
	config.RuntimeToolFactory = executionruntime.NewToolFactory(options)
	return nil
}

func publishedExecutionDenied(cause error) error {
	return apperror.Wrap(cause, catalog.CodePermissionDenied, apperror.WithOperation("agent_execution.published_authorization"))
}

func (h *AgentsHandler) authorizePublishedExecution(ctx context.Context, access publishedExecutionAccess, keyID uuid.UUID) error {
	if h.appService == nil {
		return execution.ErrInvalidIdentity
	}
	var current *dto.AgentWebAppRuntimeConfigResponse
	var err error
	switch access.entry {
	case runtimemodel.ConversationSourceWebApp:
		capability, capabilityErr := h.appService.GetWebAppRuntimeCapability(ctx, access.webApp.String(), access.user.String(), true)
		if capabilityErr != nil {
			return capabilityErr
		}
		if capability == nil || !capability.Allowed {
			return execution.ErrInvalidIdentity
		}
		current, err = h.appService.GetPublishedAgentWebAppConfig(ctx, access.webApp.String())
	case runtimemodel.ConversationSourceExternalAPI:
		live, ok := h.appService.(interface {
			authorizePublishedExecutionAPI(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error
		})
		if !ok || keyID == uuid.Nil {
			return execution.ErrInvalidIdentity
		}
		if err := live.authorizePublishedExecutionAPI(ctx, access.application, keyID, access.dataWorkspace); err != nil {
			return err
		}
		current, err = h.appService.GetPublishedAgentRuntimeConfig(ctx, access.application.String())
	default:
		return execution.ErrInvalidIdentity
	}
	if err != nil {
		return err
	}
	// Changing publication requires a fresh turn, never a silent substitution in
	// an existing invocation. Only the published opt-in is relevant, not a draft.
	if current == nil || current.AgentType != "AGENT" || current.AgentID != access.application.String() ||
		current.OrganizationID != access.organization.String() || current.WorkspaceID != access.resourceWorkspace.String() ||
		current.VersionUUID != access.publication.String() || (access.entry == runtimemodel.ConversationSourceWebApp && current.WebAppID != access.webApp.String()) ||
		current.Config.ExecutionRuntime == nil || !current.Config.ExecutionRuntime.Enabled {
		return execution.ErrInvalidIdentity
	}
	return h.executionApprovals.validate(access.organization.String(), current.Config.ExecutionRuntime)
}
