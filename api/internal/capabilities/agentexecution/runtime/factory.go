package executionruntime

import (
	"context"

	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeplatform"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/apperror"
	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

// FactoryOptions contains only server-resolved settings. ResolveScope must
// check the entry's verified caller and return an error rather than fall back
// to a credential owner. LegacyAgentID is exclusively for historical console
// bindings; all other consumers require an explicit versioned identity.
type FactoryOptions struct {
	Gateway   Gateway
	Persister ArtifactPersister
	// ResolvePersister binds protected persistence to the prepared message.
	// A nil result fails closed instead of selecting a legacy public persister.
	ResolvePersister func(*runtimeservice.PreparedChat, execution.Scope) ArtifactPersister
	Files            AttachmentReader
	// ResolveFiles is mandatory for identity-scoped entries. A raw file service
	// cannot substitute for a turn-scoped, live-authorized input reader.
	ResolveFiles          func(*runtimeservice.PreparedChat, execution.Scope) (*ScopedAttachmentReader, error)
	Skills                SkillPackageResolver
	ResolveScope          func(*runtimeservice.PreparedChat) (execution.Scope, error)
	Authorize             AuthorizeExecution
	AuthorizePlatform     AuthorizeExecution
	LegacyAgentID         string
	ProfileID             string
	Backend               string
	NetworkPolicy         string
	HomeSnapshotID        string
	DependencyPreparation bool
	PlatformCLI           bool
	InteractiveApproval   bool
	ErrorProjector        *apptransport.Projector
	Entries               execution.EntryAdmission
	// EntryReady is supplied by server wiring only after the entry's protected
	// files and approval continuation are implemented. Operator allowlists alone
	// cannot release unfinished identity-scoped entries. Nil fails closed.
	EntryReady func(applicationType, entry string) bool
}

// InteractiveEntryImplemented is a code-readiness check, not admission. Every
// entry still needs operator allowlists, opt-in, verified identity and a capable
// Worker; anonymous, background and Runtime Workspace entries remain excluded.
func InteractiveEntryImplemented(applicationType, entry string) bool {
	return (applicationType == "agent" || applicationType == "digital_human") &&
		(entry == "console" || entry == "webapp" || entry == "external-api")
}

func NewToolFactory(options FactoryOptions) func(*runtimeservice.PreparedChat) []skillloop.RuntimeTool {
	return func(prepared *runtimeservice.PreparedChat) []skillloop.RuntimeTool {
		if prepared == nil || prepared.Conversation == nil || prepared.Message == nil || options.Gateway == nil || options.ResolveScope == nil {
			return nil
		}
		scope, err := options.ResolveScope(prepared)
		if err != nil || !factoryScopeMatches(scope, options.LegacyAgentID, prepared) ||
			!options.Entries.Allows(scope.OrganizationID, prepared.Caller.Type, prepared.Caller.Source) {
			return nil
		}
		// Copy the identity so an adapter cannot change an existing invocation's
		// frozen publication or owner by mutating a shared pointer.
		if scope.Identity != nil {
			if options.Authorize == nil || !factoryEntryReady(options, scope) {
				return nil // New entries require live application authorization.
			}
			identity := *scope.Identity
			scope.Identity = &identity
		}
		attachments := prepared.RuntimeAttachmentRefs()
		var scopedFiles *ScopedAttachmentReader
		if options.ResolveFiles != nil {
			var err error
			scopedFiles, err = options.ResolveFiles(prepared, scope)
			if err != nil || !scopedFiles.matches(scope, attachments) {
				return nil
			}
		} else if scope.Identity != nil {
			return nil
		}
		// Keep this invocation's resolved reader local; never mutate captured
		// options across concurrent conversations.
		invocation := options
		if scopedFiles != nil {
			invocation.Files = scopedFiles
		}
		persister := options.Persister
		if options.ResolvePersister != nil {
			persister = options.ResolvePersister(prepared, scope)
			if persister == nil {
				return nil
			}
		}
		if persister == nil {
			if scope.Identity != nil {
				return nil // New entries require the protected artifact path (P2).
			}
			persister = defaultAgentExecutionArtifactPersister{}
		}
		agentID := options.LegacyAgentID
		if scope.Identity != nil {
			agentID = ""
			if scope.Identity.ApplicationType == "agent" {
				agentID = scope.Identity.ApplicationID
			}
		}
		gateway := options.Gateway
		if options.HomeSnapshotID != "" {
			gateway = agentExecutionSnapshotGateway{agentExecutionGateway: gateway, homeSnapshotID: options.HomeSnapshotID}
		}
		conversationID, messageID := prepared.Conversation.ID.String(), prepared.Message.ID.String()
		tools := []skillloop.RuntimeTool{
			newAgentExecutionShellToolWithNetworkPolicy(gateway, scope, agentID, conversationID, messageID, options.ProfileID, options.Backend, options.NetworkPolicy, attachments, invocation.Files, options.Skills),
			newAgentExecutionExportTool(gateway, persister, scope, agentID, conversationID, messageID, options.ProfileID, options.Backend, options.NetworkPolicy),
		}
		platform, supported := options.Gateway.(agentExecutionPlatformGateway)
		if options.PlatformCLI && supported && options.AuthorizePlatform != nil {
			tools[0] = withPlatformRelay(tools[0], invocation, gateway, platform, persister, prepared, scope, agentID)
		}
		if options.DependencyPreparation {
			tools = append(tools, newAgentExecutionDependencyPreparationTool(gateway, scope, agentID, conversationID, messageID, options.ProfileID, options.Backend, options.NetworkPolicy, options.Skills))
		}
		for index := range tools {
			handler := tools[index].Handler
			isShell := index == 0
			tools[index].Handler = func(ctx context.Context, call adapter.ToolCall) skillloop.RuntimeToolResult {
				if prepared.Conversation == nil || prepared.Message == nil || prepared.Message.ID.String() != messageID {
					return deniedFactoryTool(options.ErrorProjector)
				}
				current, err := options.ResolveScope(prepared)
				if err != nil || !sameFactoryScope(scope, current) || !factoryScopeMatches(current, options.LegacyAgentID, prepared) ||
					!options.Entries.Allows(current.OrganizationID, prepared.Caller.Type, prepared.Caller.Source) || !factoryEntryReady(options, current) {
					return deniedFactoryTool(options.ErrorProjector)
				}
				if options.Authorize != nil && options.Authorize(ctx) != nil {
					return deniedFactoryTool(options.ErrorProjector)
				}
				if isShell && scopedFiles != nil && scopedFiles.CheckAttachments(ctx) != nil {
					return deniedFactoryTool(options.ErrorProjector)
				}
				return projectAgentExecutionToolFailure(handler(ctx, call), options.ErrorProjector)
			}
		}
		return tools
	}
}

func factoryEntryReady(options FactoryOptions, scope execution.Scope) bool {
	return scope.Identity == nil || (options.EntryReady != nil && options.EntryReady(scope.Identity.ApplicationType, scope.Identity.Entry))
}

func factoryScopeMatches(scope execution.Scope, legacyAgentID string, prepared *runtimeservice.PreparedChat) bool {
	if prepared == nil || prepared.Conversation == nil || prepared.Message == nil || scope.OrganizationID == "" || scope.SubjectID == "" ||
		scope.OrganizationID != prepared.Scope.OrganizationID.String() || prepared.Caller.ID == nil {
		return false
	}
	if scope.Identity == nil {
		return legacyAgentID != "" && prepared.Caller.Type == runtimemodel.ConversationCallerAgent &&
			prepared.Caller.Source == runtimemodel.ConversationSourceConsole && prepared.Caller.ID.String() == legacyAgentID &&
			prepared.Scope.AccountID.String() == scope.SubjectID
	}
	return identityMatchesPrepared(scope, prepared)
}

func sameFactoryScope(expected, current execution.Scope) bool {
	if expected.OrganizationID != current.OrganizationID || expected.SubjectID != current.SubjectID {
		return false
	}
	if expected.Identity == nil || current.Identity == nil {
		return expected.Identity == current.Identity
	}
	return *expected.Identity == *current.Identity
}

func deniedFactoryTool(projector *apptransport.Projector) skillloop.RuntimeToolResult {
	return projectAgentExecutionToolFailure(skillloop.RuntimeToolResult{
		Status: "failed", Result: map[string]interface{}{"error_code": "execution_denied"},
		ModelResult: map[string]interface{}{"error_code": "execution_denied"},
		Error:       apperror.New(catalog.CodePermissionDenied, apperror.WithOperation("agent_execution.live_authorization")),
	}, projector)
}

func withPlatformRelay(tool skillloop.RuntimeTool, options FactoryOptions, gateway Gateway, platform agentExecutionPlatformGateway, persister ArtifactPersister, prepared *runtimeservice.PreparedChat, scope execution.Scope, agentID string) skillloop.RuntimeTool {
	tool.PlatformInteractiveApproval = options.InteractiveApproval
	tool.AuthorizePlatform = func(ctx context.Context) error {
		current, err := options.ResolveScope(prepared)
		if err != nil || !sameFactoryScope(scope, current) || !factoryScopeMatches(current, options.LegacyAgentID, prepared) ||
			!options.Entries.Allows(current.OrganizationID, prepared.Caller.Type, prepared.Caller.Source) || !factoryEntryReady(options, current) {
			return execution.ErrInvalidIdentity
		}
		if options.Authorize != nil {
			if err := options.Authorize(ctx); err != nil {
				return err
			}
		}
		return options.AuthorizePlatform(ctx)
	}
	if scope.Identity != nil {
		i := scope.Identity
		tool.PlatformOrigin = &toolgovernance.ExecutionOrigin{OrganizationID: scope.OrganizationID, WorkspaceID: i.WorkspaceID,
			ApplicationType: i.ApplicationType, ApplicationID: i.ApplicationID, Entry: i.Entry, SubjectKind: i.SubjectKind,
			SubjectID: scope.SubjectID, ConversationID: i.ConversationID, MessageID: prepared.Message.ID.String(), PublicationRef: i.PublicationRef}
	}
	var services skillloop.RuntimePlatformServices
	tool.BindPlatform = func(bound skillloop.RuntimePlatformServices) { services = bound }
	tool.Definition.Function.Description += " When runtime_capabilities.platform_protocol_version is 1, scripts may use zgi file read/export, zgi model call, and zgi tool call. Every request requires a stable --id. Tool names must be current exposed business functions; no Shell recursion or tool activation. Exit code 75 means approval is required: stop the script and await the existing approval interaction; never bypass it or retry a mutation with a new ID."
	tool.Handler = func(ctx context.Context, call adapter.ToolCall) skillloop.RuntimeToolResult {
		files := &agentPlatformFiles{gateway: gateway, transfers: platform, reader: options.Files, persister: persister,
			attachments: prepared.RuntimeAttachmentRefs(), skillResolver: options.Skills, skillAllowed: services.AuthorizeSkill}
		platformScope := runtimeplatform.Scope{OrganizationID: scope.OrganizationID, SubjectID: scope.SubjectID, AgentID: agentID,
			ConversationID: prepared.Conversation.ID.String(), RunID: prepared.Message.ID.String()}
		if scope.Identity != nil {
			platformScope.ExecutionIdentity = *scope.Identity
		}
		relay := &agentPlatformRelay{agentExecutionGateway: gateway, platform: platform, files: files,
			services: services, scope: platformScope, authorize: tool.AuthorizePlatform}
		defer relay.Close(ctx)
		shell := newAgentExecutionShellToolWithNetworkPolicy(relay, scope, agentID, prepared.Conversation.ID.String(), prepared.Message.ID.String(), options.ProfileID, options.Backend, options.NetworkPolicy, prepared.RuntimeAttachmentRefs(), options.Files, options.Skills)
		result := shell.Handler(ctx, call)
		if relay.session != nil && result.ModelResult != nil {
			if actual, ok := result.ModelResult["runtime_capabilities"].(map[string]interface{}); ok {
				actual["platform_protocol_version"] = 1
			}
		}
		result.Artifacts = append(result.Artifacts, files.Artifacts()...)
		result.PendingGovernance, result.PendingConnector = relay.pending.PendingGovernance, relay.pending.PendingConnector
		result.PendingApproval, result.PendingQuestion = relay.pending.PendingApproval, relay.pending.PendingQuestion
		result.PendingClientAction, result.PendingUserInput = relay.pending.PendingClientAction, relay.pending.PendingUserInput
		if relay.pending.NeedsContinuation() {
			result.Status, result.Error = "awaiting_approval", nil
			result.ModelResult = map[string]interface{}{"status": "awaiting_approval", "continuation": string(prepared.Caller.Source)}
		}
		return result
	}
	return tool
}
