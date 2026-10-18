package feishu

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	providertransport "github.com/zgiai/zgi/api/internal/capabilities/channelruntime/providertransport"
)

const (
	feishuMessageHistoryPermission = "im:message:readonly"
	feishuMemberNamesPermission    = "im:chat.members:read"
)

// TestConversationCapabilities performs bounded, read-only probes against a
// real conversation that has already reached the verified webhook boundary.
// This is intentionally separate from normal context preparation so a failed
// history permission does not prevent the member permission from being tested.
func (adapter *ChannelAdapter) TestConversationCapabilities(
	ctx context.Context,
	request channelruntime.ConversationCapabilityTestRequest,
) []channelruntime.ConversationCapabilityCheck {
	checks := []channelruntime.ConversationCapabilityCheck{
		newFeishuCapabilityCheck(channelruntime.ConversationCapabilityMessageHistory, feishuMessageHistoryPermission),
		newFeishuCapabilityCheck(channelruntime.ConversationCapabilityMemberNames, feishuMemberNamesPermission),
	}
	if adapter == nil || !adapter.directProviderAvailable() || request.OrganizationID == uuid.Nil ||
		request.WorkspaceID == uuid.Nil || request.ActorID == uuid.Nil || request.InstallationID == uuid.Nil ||
		request.ConnectionID == uuid.Nil || request.Subject.ID == uuid.Nil ||
		request.Subject.OrganizationID != request.OrganizationID || request.Subject.WorkspaceID != request.WorkspaceID ||
		strings.TrimSpace(string(request.Subject.Type)) == "" || strings.TrimSpace(request.ExternalConversationID) == "" ||
		strings.TrimSpace(request.CurrentMessageID) == "" || request.OccurredAt.IsZero() {
		return failFeishuCapabilityChecks(checks, "configuration_invalid")
	}

	connection, cleanup, err := adapter.resolveDirectConnection(ctx, request.OrganizationID, request.WorkspaceID, request.InstallationID)
	if err != nil {
		return failFeishuCapabilityChecks(checks, feishuCapabilityFailureCode(err))
	}
	defer cleanup()
	region, err := feishuRegion(connection)
	if err != nil {
		return failFeishuCapabilityChecks(checks, feishuCapabilityFailureCode(err))
	}
	token, _, err := adapter.provider.tenantAccessToken(ctx, connection, region)
	if err != nil {
		return failFeishuCapabilityChecks(checks, feishuCapabilityFailureCode(err))
	}

	chatID := strings.TrimSpace(request.ExternalConversationID)
	_, _, historyErr := adapter.provider.listMessages(ctx, region, token, feishuConnectionAppID(connection), map[string]interface{}{
		"chat_id":    chatID,
		"page_size":  1,
		"sort_type":  "newest_first",
		"start_time": request.OccurredAt.Add(-feishuContextLookback).Unix(),
		"end_time":   request.OccurredAt.Unix(),
	})
	checks[0] = completeFeishuCapabilityCheck(checks[0], historyErr)

	_, _, memberErr := adapter.provider.listChatMembers(ctx, region, token, map[string]interface{}{
		"chat_id":   chatID,
		"page_size": 1,
	})
	checks[1] = completeFeishuCapabilityCheck(checks[1], memberErr)
	return checks
}

func newFeishuCapabilityCheck(
	capability channelruntime.ConversationCapability,
	permission string,
) channelruntime.ConversationCapabilityCheck {
	return channelruntime.ConversationCapabilityCheck{
		Capability: capability,
		Status:     channelruntime.CapabilityCheckFailed,
		RequiredPermissions: []string{
			permission,
		},
	}
}

func completeFeishuCapabilityCheck(
	check channelruntime.ConversationCapabilityCheck,
	err error,
) channelruntime.ConversationCapabilityCheck {
	if err == nil {
		check.Status = channelruntime.CapabilityCheckPassed
		return check
	}
	check.FailureCode = feishuCapabilityFailureCode(err)
	return check
}

func failFeishuCapabilityChecks(
	checks []channelruntime.ConversationCapabilityCheck,
	failureCode string,
) []channelruntime.ConversationCapabilityCheck {
	for index := range checks {
		checks[index].FailureCode = failureCode
	}
	return checks
}

func feishuCapabilityFailureCode(err error) string {
	switch providertransport.ErrorReasonCode(err) {
	case "credentials_invalid":
		return "credentials_invalid"
	case "bot_not_in_chat":
		return "bot_not_in_chat"
	case "app_unavailable":
		return "app_unavailable"
	case "permission_missing", "permission_or_resource_denied":
		return "permission_missing"
	}
	switch providertransport.ErrorCode(err) {
	case providertransport.ErrorCodeAuthInvalid:
		return "credentials_invalid"
	case providertransport.ErrorCodeAccessDenied:
		return "permission_missing"
	case providertransport.ErrorCodeRateLimited:
		return "rate_limited"
	case providertransport.ErrorCodeTimeout, providertransport.ErrorCodeUpstream:
		return "temporarily_unavailable"
	case providertransport.ErrorCodeInvalidInput, providertransport.ErrorCodeConnectionInvalid:
		return "configuration_invalid"
	default:
		return "provider_rejected"
	}
}
