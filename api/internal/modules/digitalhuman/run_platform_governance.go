package digitalhuman

import (
	"errors"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"gorm.io/gorm"
)

const platformCLIApprovalOrigin = "platform_cli"

// Platform CLI approvals are message-owned, not Connector route receipts. Only
// the persisted, sealed invocation may put their host run into a waiting state.
func runHasPlatformGovernanceMessage(tx *gorm.DB, run *Run, status string) (bool, error) {
	if run.MessageID == nil || run.ConversationID == nil {
		return false, nil
	}
	var message runtimemodel.Message
	if err := tx.Where("id = ? AND conversation_id = ? AND status = ? AND deleted_at IS NULL",
		*run.MessageID, *run.ConversationID, status).Take(&message).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return platformGovernanceMessageMatchesRun(run, &message), nil
}

func platformGovernanceMessageMatchesRun(run *Run, message *runtimemodel.Message) bool {
	if run == nil || message == nil || run.MessageID == nil || run.ConversationID == nil || run.UserSubjectID == nil ||
		message.ID != *run.MessageID || message.ConversationID != *run.ConversationID || message.DeletedAt != nil {
		return false
	}
	continuation, _ := message.Metadata["tool_governance_continuation"].(map[string]interface{})
	correlation, _ := continuation["correlation_id"].(string)
	if correlation == "" {
		return false
	}
	entry := map[string]string{SurfaceConsole: "console", SurfaceWebApp: "webapp", SurfaceAPI: "external-api"}[run.Surface]
	if entry == "" {
		return false
	}
	events, _ := message.Metadata["tool_governance_decisions"].([]interface{})
	for _, raw := range events {
		event, _ := raw.(map[string]interface{})
		if event["correlation_id"] != correlation {
			continue
		}
		approval, _ := event["approval_event"].(map[string]interface{})
		frozen, found, err := toolgovernance.FrozenInvocationFromAny(approval["frozen_invocation"])
		if err != nil || !found || frozen.CorrelationID != correlation || !toolgovernance.FrozenInvocationHashMatches(frozen) || frozen.ExecutionOrigin == nil {
			return false
		}
		origin := frozen.ExecutionOrigin
		return origin.Validate() == nil && origin.ApplicationType == "digital_human" &&
			origin.OrganizationID == run.OrganizationID.String() && origin.WorkspaceID == run.WorkspaceID.String() &&
			origin.ApplicationID == run.DigitalHumanID.String() && origin.PublicationRef == run.DigitalHumanVersionID.String() &&
			origin.Entry == entry && origin.SubjectKind == run.UserSubjectKind && origin.SubjectID == run.UserSubjectID.String() &&
			origin.ConversationID == message.ConversationID.String() && origin.MessageID == message.ID.String()
	}
	return false
}
