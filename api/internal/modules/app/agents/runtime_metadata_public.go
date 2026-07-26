package agents

import (
	"strings"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
)

func publicAgentRuntimeMessageMetadata(message *runtimemodel.Message, caller runtimeservice.Caller) map[string]interface{} {
	if message == nil || message.Metadata == nil {
		return map[string]interface{}{}
	}
	if agentRuntimeMessageNeedsApprovalToken(message, caller) {
		return message.Metadata
	}
	redacted, ok := redactAgentRuntimeApprovalCredentials(message.Metadata, false).(map[string]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	return redacted
}

func agentRuntimeMessageNeedsApprovalToken(message *runtimemodel.Message, caller runtimeservice.Caller) bool {
	if message == nil || message.Status != runtimemodel.MessageStatusWaitingApproval {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(caller.Source)) {
	case runtimemodel.ConversationSourceConsole:
		return true
	case runtimemodel.ConversationSourceWebApp, runtimemodel.ConversationSourceExternalAPI:
		continuation, _ := message.Metadata["agent_workflow_continuation"].(map[string]interface{})
		allowed, _ := continuation["ui_approval_allowed"].(bool)
		return allowed
	default:
		return false
	}
}

func redactAgentRuntimeApprovalCredentials(value interface{}, inApprovalForm bool) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		redacted := make(map[string]interface{}, len(typed))
		for key, child := range typed {
			normalizedKey := strings.ToLower(strings.TrimSpace(key))
			if normalizedKey == "approval_token" || normalizedKey == "__approval_token" {
				continue
			}
			childIsApprovalForm := inApprovalForm || normalizedKey == "approval_form" || normalizedKey == "__approval_form"
			if childIsApprovalForm && normalizedKey == "token" {
				continue
			}
			redacted[key] = redactAgentRuntimeApprovalCredentials(child, childIsApprovalForm)
		}
		return redacted
	case []interface{}:
		redacted := make([]interface{}, len(typed))
		for index, child := range typed {
			redacted[index] = redactAgentRuntimeApprovalCredentials(child, inApprovalForm)
		}
		return redacted
	default:
		return value
	}
}
