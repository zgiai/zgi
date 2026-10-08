package agents

import (
	"github.com/google/uuid"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"github.com/zgiai/zgi/api/internal/dto"
)

// Values, rather than request-owned pointers, freeze the authenticated owner and
// published version. Billing and execution subject remain separate.
type publishedExecutionAccess struct {
	organization, resourceWorkspace, dataWorkspace, account uuid.UUID
	application, webApp, user, publication                  uuid.UUID
	entry                                                   string
	kind                                                    usersubject.Kind
}

func publishedExecutionAccessFor(scope runtimeservice.Scope, caller runtimeservice.Caller, published *dto.AgentWebAppRuntimeConfigResponse, authenticated bool) (publishedExecutionAccess, error) {
	invalid := execution.ErrInvalidIdentity
	if published == nil || published.AgentType != "AGENT" || scope.OrganizationID == uuid.Nil || scope.AccountID == uuid.Nil ||
		scope.WorkspaceID == nil || *scope.WorkspaceID == uuid.Nil || scope.AgentResourceWorkspaceID() == nil || *scope.AgentResourceWorkspaceID() == uuid.Nil ||
		(scope.ResourceWorkspaceID != nil && *scope.ResourceWorkspaceID == uuid.Nil) || scope.RuntimeWorkspaceID != nil || scope.RuntimeSubjectID != nil || caller.Type != runtimemodel.ConversationCallerAgent || caller.ID == nil || *caller.ID == uuid.Nil {
		return publishedExecutionAccess{}, invalid
	}
	publication, err := uuid.Parse(published.VersionUUID)
	if err != nil || publication == uuid.Nil || publication.String() != published.VersionUUID ||
		published.AgentID != caller.ID.String() || published.OrganizationID != scope.OrganizationID.String() || published.WorkspaceID != scope.AgentResourceWorkspaceID().String() {
		return publishedExecutionAccess{}, invalid
	}
	access := publishedExecutionAccess{organization: scope.OrganizationID, resourceWorkspace: *scope.AgentResourceWorkspaceID(), dataWorkspace: *scope.WorkspaceID,
		account: scope.AccountID, application: *caller.ID, user: scope.AccountID, publication: publication, entry: caller.Source, kind: usersubject.Account}
	switch caller.Source {
	case runtimemodel.ConversationSourceWebApp:
		if !authenticated || scope.ExternalUserID != nil || caller.SourceWebAppID == nil || *caller.SourceWebAppID == uuid.Nil || published.WebAppID != caller.SourceWebAppID.String() {
			return publishedExecutionAccess{}, invalid
		}
		access.webApp = *caller.SourceWebAppID
	case runtimemodel.ConversationSourceExternalAPI:
		if scope.ExternalUserID == nil || *scope.ExternalUserID == uuid.Nil || caller.SourceWebAppID != nil {
			return publishedExecutionAccess{}, invalid
		}
		access.user, access.kind = *scope.ExternalUserID, usersubject.APIEndUser
	default:
		return publishedExecutionAccess{}, invalid
	}
	return access, nil
}

func (a publishedExecutionAccess) resolve(prepared *runtimeservice.PreparedChat) (execution.Scope, error) {
	if prepared == nil || prepared.Conversation == nil || prepared.Message == nil || prepared.Scope.WorkspaceID == nil ||
		prepared.Scope.AgentResourceWorkspaceID() == nil || prepared.Scope.OrganizationID != a.organization || prepared.Scope.AccountID != a.account ||
		*prepared.Scope.WorkspaceID != a.dataWorkspace || *prepared.Scope.AgentResourceWorkspaceID() != a.resourceWorkspace ||
		prepared.Caller.Type != runtimemodel.ConversationCallerAgent || prepared.Caller.ID == nil || *prepared.Caller.ID != a.application || prepared.Caller.Source != a.entry ||
		(prepared.Scope.ResourceWorkspaceID != nil && *prepared.Scope.ResourceWorkspaceID == uuid.Nil) || prepared.Scope.RuntimeWorkspaceID != nil || prepared.Scope.RuntimeSubjectID != nil {
		return execution.Scope{}, execution.ErrInvalidIdentity
	}
	access, ok := agentExecutionFileAccess(prepared.Scope, prepared.Caller, prepared.Conversation)
	if !ok || access.SubjectID != a.user.String() || access.SubjectKind != string(a.kind) || prepared.Conversation.WorkspaceID == nil ||
		*prepared.Conversation.WorkspaceID != a.dataWorkspace || prepared.Conversation.AccountID != a.account ||
		prepared.Message.ID == uuid.Nil || prepared.Message.DeletedAt != nil || prepared.Message.ConversationID != prepared.Conversation.ID {
		return execution.Scope{}, execution.ErrInvalidIdentity
	}
	if a.entry == runtimemodel.ConversationSourceWebApp && (prepared.Scope.ExternalUserID != nil || prepared.Caller.SourceWebAppID == nil || *prepared.Caller.SourceWebAppID != a.webApp) {
		return execution.Scope{}, execution.ErrInvalidIdentity
	}
	if a.entry == runtimemodel.ConversationSourceExternalAPI && prepared.Caller.SourceWebAppID != nil {
		return execution.Scope{}, execution.ErrInvalidIdentity
	}
	return execution.NewScope(execution.VerifiedSubject{OrganizationID: a.organization, WorkspaceID: a.resourceWorkspace, ApplicationType: runtimemodel.ConversationCallerAgent,
		ApplicationID: a.application, Entry: a.entry, User: usersubject.Ref{Kind: a.kind, ID: a.user}, ConversationID: prepared.Conversation.ID, PublicationRef: a.publication.String()})
}
