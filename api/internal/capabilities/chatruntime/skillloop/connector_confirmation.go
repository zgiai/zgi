package skillloop

import (
	"time"

	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func connectorConfirmationRequiredPayload(
	prepared *PreparedChat,
	pending *skills.ConnectorConfirmationPending,
) map[string]interface{} {
	state := string(execution.ExecutionStateRequiresConfirmation)
	receipt := ""
	if pending != nil {
		receipt = string(pending.Confirmation.RouteReceipt)
	}
	payload := map[string]interface{}{
		"state":      state,
		"event_type": EventConnectorConfirmation,
		"status":     "waiting_approval",
		"blocking":   true,
		"created_at": time.Now().Unix(),
	}
	if prepared != nil {
		if prepared.Conversation != nil {
			payload["conversation_id"] = prepared.Conversation.ID.String()
		}
		if prepared.Message != nil {
			payload["message_id"] = prepared.Message.ID.String()
		}
	}
	if pending == nil {
		return payload
	}
	confirmation := pending.Confirmation
	receipt = string(confirmation.RouteReceipt)
	payload["route_receipt"] = receipt
	payload["confirmation"] = confirmation
	payload["result"] = map[string]interface{}{
		"state":        state,
		"confirmation": confirmation,
	}
	return payload
}

func connectorConfirmationPendingStep(
	trace skills.SkillTrace,
	toolMessage adapter.Message,
	toolResult map[string]interface{},
	payload map[string]interface{},
) skillStepResult {
	result := successfulSkillStep(trace, toolMessage, true, true)
	result.toolResult = toolResult
	result.pendingConnector = payload
	return result
}
