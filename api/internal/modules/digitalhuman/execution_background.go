package digitalhuman

import (
	"github.com/google/uuid"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
)

const backgroundExecutionEntryKey = "linux_execution_entry"
const backgroundTaskEntry = runtimemodel.ConversationSourceTask

// Background execution has its own environment owner. The legacy ChatRuntime
// account remains a billing/persistence principal, never a substitute end user.
func backgroundExecutionAccessFor(run *Run, resolved *ResolvedExecution, event invocation.Event) (executionAccess, error) {
	invalid := execution.ErrInvalidIdentity
	if run == nil || resolved == nil || run.ID == uuid.Nil || run.OrganizationID == uuid.Nil || run.WorkspaceID == uuid.Nil ||
		run.DigitalHumanID == uuid.Nil || run.DigitalHumanVersionID == uuid.Nil || run.ConversationID == nil || *run.ConversationID == uuid.Nil ||
		event.Target.Type != runtimeidentity.SubjectTypeDigitalHuman || event.Target.ID != run.DigitalHumanID ||
		event.Target.OrganizationID != run.OrganizationID || event.Target.WorkspaceID != run.WorkspaceID ||
		resolved.Snapshot.Subject != event.Target || resolved.Spec.Subject != event.Target || resolved.Spec.InvocationID != event.ID ||
		resolved.Snapshot.DigitalHumanVersion != run.DigitalHumanVersionID || resolved.Snapshot.Revision == "" || resolved.Snapshot.Revision != run.ExecutionSnapshotRevision {
		return executionAccess{}, invalid
	}
	entry := run.Surface
	switch entry {
	case SurfaceChannel:
		installation, err := uuid.Parse(event.Origin.InstallationID)
		binding, bindingOK := stringContextValue(event.Context, "channel_binding_id")
		bindingID, bindingErr := uuid.Parse(binding)
		conversation, _ := stringContextValue(event.Context, "external_conversation_id")
		if event.Origin.Kind != invocation.OriginChannel || (event.Type != invocation.EventTypeChannelMessage && event.Type != invocation.EventTypeChannelMention) || event.Sender.Type != invocation.PrincipalExternalUser || event.Sender.ID == "" ||
			event.Origin.Provider == "" || err != nil || installation == uuid.Nil || !bindingOK || bindingErr != nil || bindingID == uuid.Nil ||
			event.ReplyTo == nil || event.ReplyTo.Provider != event.Origin.Provider || event.ReplyTo.InstallationID != event.Origin.InstallationID || event.ReplyTo.ConversationID == "" || event.ReplyTo.ConversationID != conversation {
			return executionAccess{}, invalid
		}
	case SurfaceTask:
		entry = backgroundTaskEntry
		marker, _ := stringContextValue(event.Context, backgroundExecutionEntryKey)
		if marker != backgroundTaskEntry || event.Type != invocation.EventTypeSchedule || event.Origin.Kind != invocation.OriginSchedule ||
			event.Sender.Type != invocation.PrincipalSystem || event.Sender.ID != "digital-human-task-scheduler" {
			return executionAccess{}, invalid
		}
		for _, key := range []string{"task_id", "task_run_id", "action_run_id"} {
			raw, ok := stringContextValue(event.Context, key)
			id, err := uuid.Parse(raw)
			if !ok || err != nil || id == uuid.Nil || id.String() != raw {
				return executionAccess{}, invalid
			}
		}
		action, _ := stringContextValue(event.Context, "action_run_id")
		if event.ID != action {
			return executionAccess{}, invalid
		}
	default:
		return executionAccess{}, invalid
	}
	if _, exists := event.Context["web_app_id"]; exists || run.PrincipalType != string(event.Sender.Type) {
		return executionAccess{}, invalid
	}
	subject := execution.BackgroundSubject(run.OrganizationID, run.WorkspaceID, run.DigitalHumanID, *run.ConversationID, entry)
	billing := runtimeScope(run).AccountID
	if billing == uuid.Nil || billing == subject.ID {
		return executionAccess{}, invalid
	}
	return executionAccess{organization: run.OrganizationID, workspace: run.WorkspaceID, application: run.DigitalHumanID,
		publication: run.DigitalHumanVersionID, run: run.ID, conversation: *run.ConversationID, surface: run.Surface, entry: entry,
		user: subject.ID, kind: subject.Kind, billing: billing, background: true}, nil
}

func (a executionAccess) resolveBackground(prepared *chatruntime.PreparedChat) (execution.Scope, error) {
	if prepared == nil || prepared.Conversation == nil || prepared.Message == nil {
		return execution.Scope{}, execution.ErrInvalidIdentity
	}
	c, m, caller, scope := prepared.Conversation, prepared.Message, prepared.Caller, prepared.Scope
	if scope.OrganizationID != a.organization || scope.AccountID != a.billing || scope.WorkspaceID == nil || *scope.WorkspaceID != a.workspace ||
		scope.AgentResourceWorkspaceID() == nil || *scope.AgentResourceWorkspaceID() != a.workspace || scope.ExternalUserID != nil ||
		scope.RuntimeWorkspaceID != nil || scope.RuntimeSubjectID != nil ||
		caller.Type != runtimemodel.ConversationCallerDigitalHuman || caller.ID == nil || *caller.ID != a.application ||
		caller.RunID == nil || *caller.RunID != a.run || caller.Source != a.entry || caller.SourceWebAppID != nil ||
		c.ID != a.conversation || c.OrganizationID != a.organization || c.AccountID != a.billing || c.WorkspaceID == nil || *c.WorkspaceID != a.workspace ||
		c.CallerType != caller.Type || c.CallerID == nil || *c.CallerID != a.application || c.Source != a.entry || c.SourceWebAppID != nil ||
		c.ExternalUserID != nil || c.DeletedAt != nil || m.ID == uuid.Nil || m.DeletedAt != nil || m.ConversationID != c.ID {
		return execution.Scope{}, execution.ErrInvalidIdentity
	}
	return execution.NewScope(execution.VerifiedSubject{OrganizationID: a.organization, WorkspaceID: a.workspace,
		ApplicationType: runtimemodel.ConversationCallerDigitalHuman, ApplicationID: a.application, Entry: a.entry,
		User:           execution.BackgroundSubject(a.organization, a.workspace, a.application, a.conversation, a.entry),
		ConversationID: a.conversation, PublicationRef: a.publication.String()})
}
