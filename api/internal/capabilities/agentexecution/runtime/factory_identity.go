package executionruntime

import (
	"github.com/google/uuid"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
)

// identityMatchesPrepared is defense in depth after the entry authenticates its
// user. A plausible caller alone cannot authorize a different persisted
// conversation/message. Resource workspace and billing ownership are deliberately
// distinct from the execution user's identity for external Agent requests.
func identityMatchesPrepared(scope execution.Scope, prepared *runtimeservice.PreparedChat) bool {
	identity := scope.Identity
	chatScope, caller := prepared.Scope, prepared.Caller
	conversation, message := prepared.Conversation, prepared.Message
	resourceWorkspace := chatScope.AgentResourceWorkspaceID()
	if identity.Validate() != nil || chatScope.OrganizationID == uuid.Nil || chatScope.AccountID == uuid.Nil ||
		chatScope.RuntimeWorkspaceID != nil || chatScope.RuntimeSubjectID != nil {
		return false
	}
	if resourceWorkspace == nil || *resourceWorkspace == uuid.Nil || identity.WorkspaceID != resourceWorkspace.String() ||
		chatScope.WorkspaceID == nil || *chatScope.WorkspaceID == uuid.Nil {
		return false
	}
	if caller.ID == nil || *caller.ID == uuid.Nil || identity.ApplicationType != caller.Type ||
		identity.ApplicationID != caller.ID.String() || identity.Entry != caller.Source {
		return false
	}
	if conversation.ID == uuid.Nil || identity.ConversationID != conversation.ID.String() || conversation.DeletedAt != nil ||
		conversation.OrganizationID != chatScope.OrganizationID || conversation.WorkspaceID == nil ||
		*conversation.WorkspaceID != *chatScope.WorkspaceID || conversation.AccountID != chatScope.AccountID {
		return false
	}
	if conversation.CallerType != caller.Type || conversation.CallerID == nil || *conversation.CallerID != *caller.ID ||
		conversation.Source != caller.Source || message.ID == uuid.Nil || message.DeletedAt != nil || message.ConversationID != conversation.ID {
		return false
	}
	if caller.Source == runtimemodel.ConversationSourceWebApp {
		if caller.SourceWebAppID == nil || *caller.SourceWebAppID == uuid.Nil || conversation.SourceWebAppID == nil || *conversation.SourceWebAppID != *caller.SourceWebAppID {
			return false
		}
	} else if caller.SourceWebAppID != nil || conversation.SourceWebAppID != nil {
		return false
	}
	if caller.Type == runtimemodel.ConversationCallerAgent && caller.Source == runtimemodel.ConversationSourceExternalAPI {
		// AccountID remains the existing billing/persistence owner. Only the
		// separately resolved external user owns this execution environment.
		return chatScope.ExternalUserID != nil && *chatScope.ExternalUserID != uuid.Nil &&
			scope.SubjectID == chatScope.ExternalUserID.String() && conversation.ExternalUserID != nil && *conversation.ExternalUserID == *chatScope.ExternalUserID
	}
	if identity.Entry == "channel" || identity.Entry == "task" {
		if caller.Type != runtimemodel.ConversationCallerDigitalHuman || caller.RunID == nil || *caller.RunID == uuid.Nil ||
			chatScope.ExternalUserID != nil || conversation.ExternalUserID != nil {
			return false
		}
		subject := execution.BackgroundSubject(chatScope.OrganizationID, *resourceWorkspace, *caller.ID, conversation.ID, identity.Entry)
		return scope.SubjectID == subject.ID.String() && identity.SubjectKind == string(subject.Kind)
	}
	// DigitalHuman's durable runtime explicitly projects its verified UserSubject
	// into AccountID. Its interactive adapter must reject anonymous events;
	// the factory never substitutes its principal or creator here.
	return scope.SubjectID == chatScope.AccountID.String() && chatScope.ExternalUserID == nil && conversation.ExternalUserID == nil
}
