package skillloop

import (
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestConnectorConfirmationRequiredPayloadUsesIndependentBlockingContract(t *testing.T) {
	conversationID := uuid.NewString()
	messageID := uuid.NewString()
	receipt := uuid.NewString()
	prepared := NewPreparedChat(conversationID, messageID, "provider", "auto", nil)
	pending := &skills.ConnectorConfirmationPending{Confirmation: execution.RuntimeConfirmation{
		RouteReceipt:  execution.RouteClaimReceipt(receipt),
		ConnectorID:   core.Identifier("github"),
		ConnectorName: "GitHub",
		OperationID:   core.Identifier("issue.create"),
		ActionName:    "Create issue",
		Target:        core.TargetRef{Type: core.TargetTypeChat, ID: conversationID},
		Consequence: execution.RuntimeConsequence{
			ExternalEffect: "create",
			Description:    "Creates an issue on GitHub.",
		},
		Risk: execution.RiskHigh,
	}}

	payload := connectorConfirmationRequiredPayload(prepared, pending)
	if payload["event_type"] != EventConnectorConfirmation ||
		payload["status"] != "waiting_approval" ||
		payload["blocking"] != true ||
		payload["route_receipt"] != receipt {
		t.Fatalf("payload = %#v", payload)
	}
	for _, forbidden := range []string{"action_id", "action_type", "continuation_policy", "waiting_client_action"} {
		if _, exists := payload[forbidden]; exists {
			t.Fatalf("payload contains legacy field %q: %#v", forbidden, payload)
		}
	}
	if payload["conversation_id"] != conversationID || payload["message_id"] != messageID {
		t.Fatalf("payload host identity = %#v", payload)
	}
	result, ok := payload["result"].(map[string]interface{})
	if !ok || result["state"] != string(execution.ExecutionStateRequiresConfirmation) {
		t.Fatalf("payload result = %#v", payload["result"])
	}

	trace := skills.SkillTrace{SkillID: "github", ToolName: "create_issue"}
	payload["skill_id"] = trace.SkillID
	payload["tool_name"] = trace.ToolName
	step := connectorConfirmationPendingStep(trace, adapter.Message{}, result, payload)
	if step.pendingConnector == nil || step.pendingConnector["route_receipt"] != receipt ||
		step.pendingConnector["skill_id"] != trace.SkillID ||
		step.pendingConnector["tool_name"] != trace.ToolName ||
		step.toolResult["state"] != string(execution.ExecutionStateRequiresConfirmation) ||
		!step.usedSkill || !step.usedTool {
		t.Fatalf("projected step = %#v", step)
	}
	if step.pendingClientAction != nil {
		t.Fatalf("Connector confirmation reused client action: %#v", step.pendingClientAction)
	}
}
