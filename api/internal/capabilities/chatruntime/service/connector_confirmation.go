package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	streamEventConnectorConfirmationRequired = "connector_confirmation_required"
	streamEventConnectorConfirmationResolved = "connector_confirmation_resolved"
	connectorConfirmationEventsMetadataKey   = "connector_confirmation_events"
)

func (s *service) persistConnectorConfirmationPendingResult(
	ctx context.Context,
	prepared *PreparedChat,
	payload map[string]interface{},
	usage *adapter.Usage,
) (map[string]interface{}, error) {
	if prepared == nil || prepared.Message == nil || prepared.Conversation == nil {
		return nil, fmt.Errorf("%w: prepared Connector confirmation is required", ErrInvalidInput)
	}
	event, err := connectorConfirmationRequiredEvent(prepared, payload)
	if err != nil {
		return nil, err
	}
	metadata := appendConnectorConfirmationEventMetadata(prepared.Message.Metadata, event)
	metadata = preparedResultMetadataForPrepared(prepared, metadata, usage)
	prepared.Message.Metadata = metadata

	err = s.persistPendingMessageAndFinishConversationBestEffort(
		ctx,
		prepared,
		"Connector confirmation",
		func(repo repository.MessageRepository) error {
			return repo.UpdateWaitingApproval(ctx, prepared.Message.ID, metadata)
		},
		func(repo repository.ConversationRepository) error {
			return repo.FinishContinuationMessage(ctx, prepared.Conversation.ID, prepared.Message.ID)
		},
	)
	return metadata, err
}

func connectorConfirmationRequiredEvent(
	prepared *PreparedChat,
	payload map[string]interface{},
) (map[string]interface{}, error) {
	confirmation, ok := connectorRuntimeConfirmationFromAny(payload["confirmation"])
	if !ok {
		return nil, fmt.Errorf("%w: trusted Connector confirmation is missing", ErrInvalidInput)
	}
	receipt := strings.TrimSpace(string(confirmation.RouteReceipt))
	if receipt == "" || receipt != string(confirmation.RouteReceipt) || len(receipt) > 128 ||
		strings.ContainsAny(receipt, "\x00\r\n") ||
		receipt != strings.TrimSpace(stringFromAny(payload["route_receipt"])) {
		return nil, fmt.Errorf("%w: invalid Connector confirmation receipt", ErrInvalidInput)
	}
	return map[string]interface{}{
		"event_type":      streamEventConnectorConfirmationRequired,
		"route_receipt":   receipt,
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"state":           string(execution.ExecutionStateRequiresConfirmation),
		"status":          "waiting_approval",
		"blocking":        true,
		"confirmation":    confirmation,
		"created_at":      time.Now().Unix(),
	}, nil
}

func connectorRuntimeConfirmationFromAny(value interface{}) (execution.RuntimeConfirmation, bool) {
	switch typed := value.(type) {
	case execution.RuntimeConfirmation:
		return typed, true
	case *execution.RuntimeConfirmation:
		if typed != nil {
			return *typed, true
		}
	}
	return execution.RuntimeConfirmation{}, false
}

func appendConnectorConfirmationEventMetadata(
	source map[string]interface{},
	event map[string]interface{},
) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	events := connectorConfirmationEventsFromAny(metadata[connectorConfirmationEventsMetadataKey])
	eventType := strings.TrimSpace(stringFromAny(event["event_type"]))
	receipt := strings.TrimSpace(stringFromAny(event["route_receipt"]))
	for _, existing := range events {
		if strings.TrimSpace(stringFromAny(existing["event_type"])) == eventType &&
			strings.TrimSpace(stringFromAny(existing["route_receipt"])) == receipt {
			return metadata
		}
	}
	events = append(events, copyStringAnyMap(event))
	metadata[connectorConfirmationEventsMetadataKey] = connectorConfirmationEventsToAny(events)
	return metadata
}

func connectorConfirmationEventsFromAny(value interface{}) []map[string]interface{} {
	var raw []interface{}
	switch typed := value.(type) {
	case []interface{}:
		raw = typed
	case []map[string]interface{}:
		raw = make([]interface{}, 0, len(typed))
		for _, item := range typed {
			raw = append(raw, item)
		}
	}
	out := make([]map[string]interface{}, 0, len(raw))
	for _, item := range raw {
		if record := copyStringAnyMap(mapFromOperationContext(item)); len(record) > 0 {
			out = append(out, record)
		}
	}
	return out
}

func connectorConfirmationEventsToAny(events []map[string]interface{}) []interface{} {
	out := make([]interface{}, 0, len(events))
	for _, event := range events {
		out = append(out, copyStringAnyMap(event))
	}
	return out
}
