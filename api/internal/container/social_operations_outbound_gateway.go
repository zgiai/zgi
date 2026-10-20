package container

import (
	"context"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"github.com/zgiai/zgi/api/internal/modules/connectors/runtimeauth"
	"github.com/zgiai/zgi/api/internal/modules/socialops"
)

func (g *socialOperationsGateway) SendOutbound(ctx context.Context, out socialops.Outbound, message socialops.OutboundMessage) (socialops.OutboundReceipt, error) {
	if err := g.validateOutbound(out, message); err != nil {
		return socialops.OutboundReceipt{}, &socialops.NotSubmittedError{Cause: err}
	}
	input, err := outboundInput(out, message)
	if err != nil {
		return socialops.OutboundReceipt{}, &socialops.NotSubmittedError{Cause: err}
	}
	value, err := g.invokeOutbound(ctx, out, input)
	if err != nil {
		return socialops.OutboundReceipt{}, err
	}
	receipt, err := normalizeOutboundReceipt(out, value)
	if err != nil {
		return receipt, err
	}
	return receipt, nil
}

func (g *socialOperationsGateway) validateOutbound(out socialops.Outbound, message socialops.OutboundMessage) error {
	r := g.restored.Run
	if out.ID == uuid.Nil || out.State != "dispatching" || out.Revision < 1 || out.ConfigurationRevision < 1 || out.ReviewedDigest == "" ||
		out.OrganizationID != r.OrganizationID || out.WorkspaceID != g.restored.WorkspaceID || out.OwnerID != g.restored.OwnerID || out.EmployeeID != r.DigitalHumanID ||
		out.ConnectionID == uuid.Nil || !core.Identifier(out.ConnectorID).Valid() || !core.Identifier(out.OperationID).Valid() ||
		!strings.HasPrefix(out.OperationID, out.ConnectorID+".") || strings.TrimSpace(message.Text) == "" {
		return socialGatewayError(core.ErrorCodeAccessDenied)
	}
	return nil
}

func outboundInput(out socialops.Outbound, message socialops.OutboundMessage) (map[string]any, error) {
	if out.Kind == "dm" && (out.ConnectorID == "x-social" || out.ConnectorID == "facebook-social") && out.OperationID == out.ConnectorID+".message.send" {
		return map[string]any{"account_id": out.AccountID, "conversation_id": out.ConversationID, "text": message.Text}, nil
	}
	if out.Kind != "handoff" || out.AccountID != "" || out.ConversationID != "" || out.Destination == "" {
		return nil, socialGatewayError(core.ErrorCodeInvalidInput)
	}
	switch {
	case out.OperationID == "dingtalk.messages.send_direct":
		return map[string]any{"user_ids": []string{out.Destination}, "text": message.Text, "format": "text"}, nil
	case out.OperationID == "dingtalk.messages.send_group":
		return map[string]any{"conversation_id": out.Destination, "text": message.Text, "format": "text"}, nil
	case out.OperationID == "feishu.message.send":
		typeName := "open_id"
		if strings.HasPrefix(out.Destination, "oc_") {
			typeName = "chat_id"
		}
		return map[string]any{"receive_id": out.Destination, "receive_id_type": typeName, "text": message.Text}, nil
	case strings.HasSuffix(out.OperationID, ".message.send"):
		if _, ok := catalog.MailDefinition(out.ConnectorID); ok {
			return map[string]any{"to": []string{out.Destination}, "subject": message.Subject, "text": message.Text}, nil
		}
		return nil, socialGatewayError(core.ErrorCodeInvalidInput)
	default:
		return nil, socialGatewayError(core.ErrorCodeInvalidInput)
	}
}

func (g *socialOperationsGateway) invokeOutbound(ctx context.Context, out socialops.Outbound, input map[string]any) (map[string]any, error) {
	fail := func(err error) (map[string]any, error) { return nil, &socialops.NotSubmittedError{Cause: err} }
	connector, operation := core.Identifier(out.ConnectorID), core.Identifier(out.OperationID)
	target := core.TargetRef{Type: core.TargetTypeDigitalHuman, ID: out.EmployeeID.String(), Scope: core.ScopePublished, VersionID: g.restored.Run.DigitalHumanVersionID.String()}
	snapshot, err := g.resolver.ResolveDigitalHumanCapabilities(ctx, runtimeauth.DigitalHumanCapabilityRequest{OrganizationID: out.OrganizationID, OwnerAccountID: out.OwnerID,
		DigitalHumanID: out.EmployeeID.String(), Scope: core.ScopePublished, VersionID: target.VersionID})
	if err != nil {
		return fail(err)
	}
	found := false
	for _, capability := range snapshot.Capabilities {
		if capability.ConnectionID != out.ConnectionID || capability.OwnerAccountID != out.OwnerID || capability.ConnectorID != connector || !capability.Target.SameIdentity(target) {
			continue
		}
		for _, candidate := range capability.Operations {
			if candidate.ID == out.OperationID {
				found = true
				break
			}
		}
	}
	if !found {
		return fail(socialGatewayError(core.ErrorCodeTargetNotReady))
	}
	connection, err := g.connections.GetConnection(ctx, out.OrganizationID, out.OwnerID, out.ConnectionID)
	if err != nil {
		return fail(err)
	}
	if connection.ID != out.ConnectionID || connection.OrganizationID != out.OrganizationID || connection.OwnerAccountID != out.OwnerID || connection.ConnectorID != connector {
		return fail(socialGatewayError(core.ErrorCodeAccessDenied))
	}
	// Use a stable operation attempt identity; revisions change only after a
	// proven-not-submitted reset.
	operationKey := uuid.NewSHA1(out.ID, []byte(out.OperationID))
	logical := "socialout:" + out.ID.String() + ":" + formatRevision(out.Revision) + ":" + operationKey.String()
	identity, err := g.resolver.ResolveConnectorIdentity(ctx, dispatch.IdentityResolutionRequest{ConversationID: g.restored.Run.ConversationID.String(), OrganizationID: out.OrganizationID,
		OwnerAccountID: out.OwnerID, ActingPrincipalID: out.OwnerID, ConnectorID: connector, Target: target, OperationID: operation,
		LogicalOperationID: logical, CorrelationID: logical, Interactive: false})
	if err != nil {
		return fail(err)
	}
	dispatchCtx := execution.WithBackgroundIdentity(ctx, g.restored.Run.ID, *g.restored.Run.MessageID)
	dispatchCtx = withSocialOutboundClaim(dispatchCtx, out, input)
	result, err := g.dispatcher.Dispatch(dispatchCtx, dispatch.Invocation{Identity: identity, Input: input})
	if result.Evidence != nil && result.Evidence.Disposition == execution.DispatchNotSubmitted {
		if err == nil {
			err = socialGatewayError(core.ErrorCodeConflict)
		}
		return nil, &socialops.NotSubmittedError{Cause: err}
	}
	if err != nil {
		return nil, err
	}
	if result.State != execution.ExecutionStateCompleted {
		return nil, socialGatewayError(core.ErrorCodeOutcomeUnknown)
	}
	value, ok := result.Value.(map[string]any)
	if !ok {
		return nil, socialGatewayError(core.ErrorCodeOutcomeUnknown)
	}
	return value, nil
}

func formatRevision(value int64) string { return strconv.FormatInt(value, 10) }

func normalizeOutboundReceipt(out socialops.Outbound, value map[string]any) (socialops.OutboundReceipt, error) {
	if out.Kind == "dm" {
		data, ok := value["data"].(map[string]any)
		if !ok || value["provider"] != out.ConnectorID || value["transport"] != "mcp" {
			return socialops.OutboundReceipt{}, socialGatewayError(core.ErrorCodeOutcomeUnknown)
		}
		if nested, ok := data["data"].(map[string]any); ok {
			data = nested
		}
		id := firstString(data, "message_id", "dm_event_id", "id")
		if id == "" {
			return socialops.OutboundReceipt{}, socialGatewayError(core.ErrorCodeOutcomeUnknown)
		}
		return socialops.OutboundReceipt{ID: id, Status: "accepted_by_platform"}, nil
	}
	if value["provider"] != out.ConnectorID || value["operation"] != out.OperationID {
		return socialops.OutboundReceipt{}, socialGatewayError(core.ErrorCodeOutcomeUnknown)
	}
	switch {
	case out.ConnectorID == "dingtalk":
		if firstString(value, "status") != "accepted" {
			return socialops.OutboundReceipt{}, socialGatewayError(core.ErrorCodeOutcomeUnknown)
		}
		id := firstString(value, "receipt")
		if id == "" {
			return socialops.OutboundReceipt{}, socialGatewayError(core.ErrorCodeOutcomeUnknown)
		}
		return socialops.OutboundReceipt{ID: id, Status: "accepted_by_platform"}, nil
	case out.ConnectorID == "feishu":
		data, _ := value["data"].(map[string]any)
		id := firstString(data, "message_id")
		if id == "" {
			return socialops.OutboundReceipt{}, socialGatewayError(core.ErrorCodeOutcomeUnknown)
		}
		return socialops.OutboundReceipt{ID: id, Status: "sent"}, nil
	default:
		if firstString(value, "status") != "accepted_by_server" {
			return socialops.OutboundReceipt{}, socialGatewayError(core.ErrorCodeOutcomeUnknown)
		}
		id := firstString(value, "message_id")
		if id == "" {
			return socialops.OutboundReceipt{}, socialGatewayError(core.ErrorCodeOutcomeUnknown)
		}
		return socialops.OutboundReceipt{ID: id, Status: "accepted_by_server"}, nil
	}
}

var _ socialops.OutboundGateway = (*socialOperationsGateway)(nil)
