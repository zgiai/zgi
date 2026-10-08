package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

// checkPlatformApprovalScope checks authoritative metadata before persisting a
// decision, and again immediately before the frozen business invocation. It
// never executes a Shell, restores a Job grant, or substitutes a publication.
func checkPlatformApprovalScope(ctx context.Context, prepared *PreparedChat, event map[string]interface{}, approve bool) error {
	frozen, found, err := toolGovernanceFrozenInvocationFromEvent(event)
	if err != nil {
		return err
	}
	external := prepared != nil && (prepared.Caller.Type == "digital_human" ||
		(prepared.Caller.Type == "agent" && prepared.Caller.Source != "console"))
	if !found || frozen.ExecutionOrigin == nil {
		if external {
			return fmt.Errorf("%w: platform approval origin is required", ErrPermissionDenied)
		}
		return nil // Existing console/non-platform approval contracts are unchanged.
	}
	if err := validateToolGovernanceFrozenInvocationForDecision(frozen, toolGovernanceCorrelationID(event), approve); err != nil {
		return err
	}
	o := frozen.ExecutionOrigin
	if err := o.Validate(); err != nil {
		return fmt.Errorf("%w: invalid platform approval origin", ErrPermissionDenied)
	}
	if prepared == nil || prepared.Conversation == nil || prepared.Message == nil || prepared.Caller.ID == nil ||
		prepared.Caller.Type != o.ApplicationType || prepared.Caller.ID.String() != o.ApplicationID || prepared.Caller.Source != o.Entry ||
		prepared.Scope.OrganizationID.String() != o.OrganizationID || prepared.Scope.AgentResourceWorkspaceID() == nil ||
		prepared.Scope.AgentResourceWorkspaceID().String() != o.WorkspaceID || prepared.Conversation.ID.String() != o.ConversationID ||
		prepared.Message.ID.String() != o.MessageID || prepared.Message.ConversationID != prepared.Conversation.ID ||
		prepared.Conversation.DeletedAt != nil || prepared.Message.DeletedAt != nil || prepared.Scope.RuntimeWorkspaceID != nil || prepared.Scope.RuntimeSubjectID != nil {
		return fmt.Errorf("%w: platform approval owner mismatch", ErrPermissionDenied)
	}
	subject := prepared.Scope.AccountID
	if o.ApplicationType == "agent" && o.Entry == runtimemodel.ConversationSourceExternalAPI {
		if prepared.Scope.ExternalUserID == nil || *prepared.Scope.ExternalUserID == uuid.Nil {
			return ErrPermissionDenied
		}
		subject = *prepared.Scope.ExternalUserID
	}
	if subject.String() != o.SubjectID {
		return ErrPermissionDenied
	}
	if !approve {
		return nil
	} // Users may reject after compute/CLI admission is withdrawn.
	if prepared.RunConfig.RuntimeToolFactory == nil {
		return ErrPermissionDenied
	}
	for _, tool := range prepared.RunConfig.RuntimeToolFactory(prepared) {
		if tool.Definition.Function.Name != "shell_exec" || !tool.PlatformInteractiveApproval || tool.BindPlatform == nil ||
			tool.PlatformOrigin == nil || *tool.PlatformOrigin != *o || tool.AuthorizePlatform == nil {
			continue
		}
		return tool.AuthorizePlatform(ctx)
	}
	return fmt.Errorf("%w: original platform publication is unavailable", ErrPermissionDenied)
}
