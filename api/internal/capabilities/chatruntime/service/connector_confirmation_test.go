package service

import (
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
)

func TestLegacyClientActionCannotResumeConnectorConfirmation(t *testing.T) {
	if !isConnectorRuntimeConfirmationClientAction(map[string]interface{}{
		"action_type": "connector_runtime_confirmation",
	}) {
		t.Fatal("Connector confirmation client-action marker was not rejected")
	}
	if isConnectorRuntimeConfirmationClientAction(map[string]interface{}{
		"action_type": "route_navigation",
	}) {
		t.Fatal("ordinary client action was misclassified as Connector confirmation")
	}
}

func TestConnectorConfirmationRequiredEventUsesIndependentSafeMetadata(t *testing.T) {
	conversationID := uuid.New()
	messageID := uuid.New()
	receipt := uuid.New().String()
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: conversationID},
		Message:      &runtimemodel.Message{ID: messageID, Metadata: map[string]interface{}{}},
	}
	confirmation := execution.RuntimeConfirmation{
		RouteReceipt:  execution.RouteClaimReceipt(receipt),
		ConnectorID:   core.Identifier("github"),
		ConnectorName: "GitHub",
		OperationID:   core.Identifier("issue.create"),
		ActionName:    "Create issue",
		Target:        core.TargetRef{Type: core.TargetTypeChat, ID: conversationID.String()},
		Consequence: execution.RuntimeConsequence{
			ExternalEffect: "create",
			Description:    "Creates an issue.",
		},
		Risk: execution.RiskHigh,
	}
	event, err := connectorConfirmationRequiredEvent(prepared, map[string]interface{}{
		"route_receipt":  receipt,
		"confirmation":   confirmation,
		"provider_value": "must-not-persist",
	})
	if err != nil {
		t.Fatalf("connectorConfirmationRequiredEvent() error = %v", err)
	}
	if event["event_type"] != streamEventConnectorConfirmationRequired ||
		event["route_receipt"] != receipt || event["status"] != "waiting_approval" ||
		event["conversation_id"] != conversationID.String() || event["message_id"] != messageID.String() {
		t.Fatalf("event = %#v", event)
	}
	for _, forbidden := range []string{
		"provider_value", "action_id", "action_type", "continuation_policy", "input", "credential",
	} {
		if _, ok := event[forbidden]; ok {
			t.Fatalf("event persisted forbidden field %q: %#v", forbidden, event)
		}
	}

	metadata := appendConnectorConfirmationEventMetadata(nil, event)
	metadata = appendConnectorConfirmationEventMetadata(metadata, event)
	events := connectorConfirmationEventsFromAny(metadata[connectorConfirmationEventsMetadataKey])
	if len(events) != 1 {
		t.Fatalf("required event count = %d, want 1: %#v", len(events), events)
	}

	_, err = connectorConfirmationRequiredEvent(prepared, map[string]interface{}{
		"route_receipt": uuid.NewString(),
		"confirmation":  confirmation,
	})
	if err == nil {
		t.Fatal("mismatched route receipt was accepted")
	}
}
