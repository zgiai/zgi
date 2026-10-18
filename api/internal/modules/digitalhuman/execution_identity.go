package digitalhuman

import (
	"github.com/google/uuid"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
)

// executionAccess freezes the data owner from a verified durable run. For
// interactive runs it never falls back to the actor; background runs use a
// distinct derived owner and keep the existing billing account separately.
type executionAccess struct {
	organization, workspace, application, publication uuid.UUID
	run, user, principal, webApp, conversation        uuid.UUID
	surface, entry, principalType                     string
	kind                                              usersubject.Kind
	background                                        bool
	billing                                           uuid.UUID
}

func executionAccessFor(run *Run, resolved *ResolvedExecution, event invocation.Event) (executionAccess, error) {
	if run != nil && (run.Surface == SurfaceChannel || run.Surface == SurfaceTask) {
		return backgroundExecutionAccessFor(run, resolved, event)
	}
	invalid := execution.ErrInvalidIdentity
	if run == nil || resolved == nil || run.ID == uuid.Nil || run.OrganizationID == uuid.Nil || run.WorkspaceID == uuid.Nil ||
		run.DigitalHumanID == uuid.Nil || run.DigitalHumanVersionID == uuid.Nil || run.UserSubjectID == nil || *run.UserSubjectID == uuid.Nil ||
		run.PrincipalID == nil || *run.PrincipalID == uuid.Nil || event.Target.Type != runtimeidentity.SubjectTypeDigitalHuman ||
		event.Target.ID != run.DigitalHumanID || event.Target.OrganizationID != run.OrganizationID || event.Target.WorkspaceID != run.WorkspaceID ||
		resolved.Snapshot.Subject != event.Target || resolved.Snapshot.DigitalHumanVersion != run.DigitalHumanVersionID ||
		resolved.Snapshot.Revision == "" || resolved.Snapshot.Revision != run.ExecutionSnapshotRevision || resolved.Spec.Subject != event.Target ||
		resolved.Spec.InvocationID != event.ID || event.Sender.ID != run.PrincipalID.String() || string(event.Sender.Type) != run.PrincipalType {
		return executionAccess{}, invalid
	}
	surface, ok := stringContextValue(event.Context, "surface")
	if !ok || surface != run.Surface {
		return executionAccess{}, invalid
	}
	a := executionAccess{organization: run.OrganizationID, workspace: run.WorkspaceID, application: run.DigitalHumanID, publication: run.DigitalHumanVersionID,
		run: run.ID, user: *run.UserSubjectID, principal: *run.PrincipalID, surface: surface, principalType: run.PrincipalType, kind: usersubject.Kind(run.UserSubjectKind)}
	if run.ConversationID != nil {
		if *run.ConversationID == uuid.Nil {
			return executionAccess{}, invalid
		}
		a.conversation = *run.ConversationID
	}
	switch surface {
	case SurfaceConsole:
		a.entry = runtimemodel.ConversationSourceConsole
		if event.Type != invocation.EventTypeManual || event.Origin.Kind != invocation.OriginManual {
			return executionAccess{}, invalid
		}
	case SurfaceWebApp:
		a.entry = runtimemodel.ConversationSourceWebApp
		if event.Type != invocation.EventTypeAPI || event.Origin.Kind != invocation.OriginAPI {
			return executionAccess{}, invalid
		}
		raw, ok := stringContextValue(event.Context, "web_app_id")
		id, err := uuid.Parse(raw)
		if !ok || err != nil || id == uuid.Nil || id.String() != raw {
			return executionAccess{}, invalid
		}
		a.webApp = id
	case SurfaceAPI:
		a.entry = runtimemodel.ConversationSourceExternalAPI
		if event.Type != invocation.EventTypeAPI || event.Origin.Kind != invocation.OriginAPI || event.Sender.Type != invocation.PrincipalService ||
			a.kind != usersubject.APIEndUser || run.ActorID != nil {
			return executionAccess{}, invalid
		}
	default:
		return executionAccess{}, invalid
	}
	if surface != SurfaceWebApp {
		if _, exists := event.Context["web_app_id"]; exists {
			return executionAccess{}, invalid
		}
	}
	if surface != SurfaceAPI && (a.kind != usersubject.Account || event.Sender.Type != invocation.PrincipalAccount ||
		a.principal != a.user || run.ActorID == nil || *run.ActorID != a.user) {
		return executionAccess{}, invalid
	}
	return a, nil
}

func (a executionAccess) scope() Scope {
	user := a.user
	scope := Scope{OrganizationID: a.organization, WorkspaceID: a.workspace, ActorID: a.user,
		PrincipalType: a.principalType, PrincipalID: a.principal, UserSubjectKind: string(a.kind), UserSubjectID: &user, Surface: a.surface}
	if a.webApp != uuid.Nil {
		webApp := a.webApp
		scope.WebAppID = &webApp
	}
	return scope
}

func (a executionAccess) resolve(prepared *chatruntime.PreparedChat) (execution.Scope, error) {
	if a.background {
		return a.resolveBackground(prepared)
	}
	if prepared == nil || prepared.Conversation == nil || prepared.Message == nil || prepared.Scope.AccountID != a.user ||
		prepared.Scope.OrganizationID != a.organization || prepared.Scope.WorkspaceID == nil || *prepared.Scope.WorkspaceID != a.workspace ||
		prepared.Scope.AgentResourceWorkspaceID() == nil || *prepared.Scope.AgentResourceWorkspaceID() != a.workspace ||
		(prepared.Scope.ResourceWorkspaceID != nil && *prepared.Scope.ResourceWorkspaceID == uuid.Nil) || prepared.Scope.ExternalUserID != nil ||
		prepared.Scope.RuntimeWorkspaceID != nil || prepared.Scope.RuntimeSubjectID != nil ||
		prepared.Caller.Type != runtimemodel.ConversationCallerDigitalHuman || prepared.Caller.ID == nil || *prepared.Caller.ID != a.application ||
		prepared.Caller.RunID == nil || *prepared.Caller.RunID != a.run || prepared.Caller.Source != a.entry ||
		prepared.Message.ID == uuid.Nil || prepared.Message.DeletedAt != nil || prepared.Message.ConversationID != prepared.Conversation.ID ||
		(a.conversation != uuid.Nil && prepared.Conversation.ID != a.conversation) {
		return execution.Scope{}, execution.ErrInvalidIdentity
	}
	access, ok := digitalHumanExecutionFileAccess(a.scope(), a.application, prepared.Conversation)
	if !ok || access.SubjectID != a.user.String() || prepared.Conversation.ExternalUserID != nil {
		return execution.Scope{}, execution.ErrInvalidIdentity
	}
	if a.webApp != uuid.Nil {
		if prepared.Caller.SourceWebAppID == nil || *prepared.Caller.SourceWebAppID != a.webApp {
			return execution.Scope{}, execution.ErrInvalidIdentity
		}
	} else if prepared.Caller.SourceWebAppID != nil || prepared.Conversation.SourceWebAppID != nil {
		return execution.Scope{}, execution.ErrInvalidIdentity
	}
	return execution.NewScope(execution.VerifiedSubject{OrganizationID: a.organization, WorkspaceID: a.workspace,
		ApplicationType: runtimemodel.ConversationCallerDigitalHuman, ApplicationID: a.application, Entry: a.entry,
		User: usersubject.Ref{Kind: a.kind, ID: a.user}, ConversationID: prepared.Conversation.ID, PublicationRef: a.publication.String()})
}
