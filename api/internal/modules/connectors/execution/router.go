package execution

import (
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"strings"
)

type Path string

const PathConnectorNative Path = "connector_native"

func (path Path) valid() bool { return path == PathConnectorNative }

type RouteClaimReceipt string
type PinnedExecution struct {
	freshClaim bool
	Snapshot   *InvocationSnapshot
	Request    ExecutionRequest
	Path       Path
	Receipt    RouteClaimReceipt
}

func ValidatePinnedExecution(pinned PinnedExecution, expected Path) error {
	if !expected.valid() || pinned.Path != expected || !validReceipt(pinned.Receipt) || validateExecutionRequest(pinned.Request) != nil {
		return connectorError(core.ErrorCodeInvalidInput, false)
	}
	return nil
}

type ExecutionRequest struct {
	ConversationID     string
	OrganizationID     uuid.UUID
	OwnerAccountID     uuid.UUID
	ActingPrincipalID  uuid.UUID
	BindingID          uuid.UUID
	ConnectionID       uuid.UUID
	ConnectorID        core.Identifier
	Target             core.TargetRef
	OperationID        core.Identifier
	LogicalOperationID string
	CorrelationID      string
	Interactive        bool
	Confirmation       *RuntimeConfirmationCommand
	// AcceptedOperation is the server-frozen catalog contract used only when
	// resuming a runtime confirmation. It is sealed with Input and can never be
	// supplied by the client or copied into route/audit/provider metadata.
	AcceptedOperation *catalog.PinnedOperation      `json:"-"`
	SessionGrants     []toolgovernance.SessionGrant `json:"-"`
	Preauthorized     bool                          `json:"-"`
	Input             any                           `json:"-"`
}

type RouteResolutionRequest struct {
	ConversationID     string
	OrganizationID     uuid.UUID
	OwnerAccountID     uuid.UUID
	ActingPrincipalID  uuid.UUID
	BindingID          uuid.UUID
	ConnectionID       uuid.UUID
	ConnectorID        core.Identifier
	Target             core.TargetRef
	OperationID        core.Identifier
	LogicalOperationID string
	CorrelationID      string
}

func routeResolutionRequest(request ExecutionRequest) RouteResolutionRequest {
	return RouteResolutionRequest{
		ConversationID: request.ConversationID,
		OrganizationID: request.OrganizationID, OwnerAccountID: request.OwnerAccountID,
		ActingPrincipalID: request.ActingPrincipalID, BindingID: request.BindingID,
		ConnectionID: request.ConnectionID, ConnectorID: request.ConnectorID,
		Target: request.Target, OperationID: request.OperationID,
		LogicalOperationID: request.LogicalOperationID, CorrelationID: request.CorrelationID,
	}
}

func validateExecutionRequest(request ExecutionRequest) error {
	if request.OrganizationID == uuid.Nil || request.OwnerAccountID == uuid.Nil ||
		request.ActingPrincipalID == uuid.Nil || request.BindingID == uuid.Nil ||
		request.ConnectionID == uuid.Nil || !request.ConnectorID.Valid() ||
		!request.OperationID.Valid() || !validOpaqueText(request.LogicalOperationID, 128) ||
		!validOpaqueText(request.CorrelationID, 128) || request.Target.Validate() != nil ||
		!validOpaqueText(request.Target.ID, 255) ||
		(request.Target.BindingInstanceID != "" && !validOpaqueText(request.Target.BindingInstanceID, 255)) ||
		(request.Confirmation != nil && (!request.Interactive || !request.Confirmation.valid())) ||
		(request.Confirmation == nil) != (request.AcceptedOperation == nil) ||
		(request.AcceptedOperation != nil && !validAcceptedOperationSnapshot(
			*request.AcceptedOperation, request.ConnectorID, request.OperationID,
		)) {
		return connectorError(core.ErrorCodeInvalidInput, false)
	}
	return nil
}

func validReceipt(receipt RouteClaimReceipt) bool {
	return validOpaqueText(string(receipt), 128)
}

func validOpaqueText(value string, maximum int) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= maximum &&
		!strings.ContainsAny(value, "\x00\r\n")
}

func validateRouteResolutionRequest(request RouteResolutionRequest) error {
	return validateExecutionRequest(ExecutionRequest{
		OrganizationID: request.OrganizationID, OwnerAccountID: request.OwnerAccountID,
		ActingPrincipalID: request.ActingPrincipalID, BindingID: request.BindingID,
		ConnectionID: request.ConnectionID, ConnectorID: request.ConnectorID,
		Target: request.Target, OperationID: request.OperationID,
		LogicalOperationID: request.LogicalOperationID, CorrelationID: request.CorrelationID,
	})
}
