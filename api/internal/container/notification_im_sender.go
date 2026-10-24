package container

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"github.com/zgiai/zgi/api/internal/modules/inbox"
)

// imDirectMessage is how a Connector sends one person a direct message.
type imDirectMessage struct {
	operation     string
	recipientKind string
	input         func(recipient, text string) map[string]any
}

// imDirectMessages are the Connectors a personal IM channel may use (ADR
// 0008 stage 4). Connectors that only reply inside a conversation are left
// out.
var imDirectMessages = map[string]imDirectMessage{
	"feishu": {operation: "feishu.message.send", recipientKind: "open_id", input: func(recipient, text string) map[string]any {
		return map[string]any{"receive_id": recipient, "receive_id_type": "open_id", "text": text}
	}},
	"dingtalk": {operation: "dingtalk.messages.send_direct", recipientKind: "user_id", input: func(recipient, text string) map[string]any {
		return map[string]any{"user_ids": []any{recipient}, "text": text, "format": "text"}
	}},
}

// notificationIMSender sends notification direct messages with an account's
// own IM Connection. The account set the channel up for this, so it runs
// outside model runs and their confirmations; it only ever reaches the
// recipient verified for the channel.
type notificationIMSender struct{ container *ServiceContainer }

// GetNotificationIMSender returns the sender personal IM channels use.
func (c *ServiceContainer) GetNotificationIMSender() inbox.IMSender {
	return notificationIMSender{container: c}
}

func (s notificationIMSender) Connections(ctx context.Context, organizationID, ownerID uuid.UUID) ([]inbox.IMConnection, error) {
	result := []inbox.IMConnection{}
	listed, err := s.container.GetConnectorConnectionRepository().ListConnections(ctx, organizationID, ownerID,
		connections.ConnectionListFilter{States: []core.Readiness{core.ReadinessReady}})
	if err != nil {
		return nil, err
	}
	for _, connection := range listed {
		message, ok := imDirectMessages[connection.ConnectorID.String()]
		if !ok || !slices.Contains(connection.UsableOperationIDs, core.Identifier(message.operation)) {
			continue
		}
		result = append(result, inbox.IMConnection{ID: connection.ID, ConnectorID: connection.ConnectorID.String(), Name: connection.Name, RecipientKind: message.recipientKind})
	}
	return result, nil
}

func (s notificationIMSender) Send(ctx context.Context, organizationID, ownerID, connectionID uuid.UUID, recipient, text, idempotencyKey string) error {
	connection, values, err := s.container.GetConnectorPersonalConnectionResolver().ResolvePersonalConnectionCredentials(ctx, organizationID, ownerID, connectionID)
	if err != nil {
		return err
	}
	defer core.DestroySecretValues(values)
	message, ok := imDirectMessages[connection.ConnectorID.String()]
	if !ok || !slices.Contains(connection.UsableOperationIDs, core.Identifier(message.operation)) {
		return core.NewError(core.ErrorCodeConnectionNotReady, false, nil)
	}
	logical := strings.TrimSpace(idempotencyKey)
	if len(logical) > 128 {
		logical = logical[:128]
	}
	_, err = s.container.GetConnectorCandidateVerifier().Invoke(ctx, execution.ProviderInvocation{
		OrganizationID: organizationID, OwnerAccountID: ownerID, ActingPrincipalID: ownerID, ConnectionID: connection.ID,
		ConnectorID: connection.ConnectorID, ProviderID: connection.ProviderID, DriverID: connection.DriverID, AuthMethodID: connection.AuthMethodID,
		CredentialVersion: connection.CredentialVersion, Target: core.TargetRef{Type: core.TargetTypeChat, ID: "notification"},
		OperationID: core.Identifier(message.operation), LogicalOperationID: logical, Credentials: values,
		Input: message.input(recipient, text),
	})
	return err
}

var _ inbox.IMSender = notificationIMSender{}
