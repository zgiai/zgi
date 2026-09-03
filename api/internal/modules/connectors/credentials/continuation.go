package credentials

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	connectorcatalog "github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"io"
	"strings"
)

const (
	connectorContinuationPayloadVersion = 2
	connectorContinuationAADPurpose     = "connector-confirmation-continuation"
)

// ContinuationPayloadProtector binds the frozen input to the native
// confirmation purpose, organization, receipt and payload version.
type ContinuationPayloadProtector struct {
	cipher *Store
}

var _ connectorexecution.ContinuationPayloadProtector = (*ContinuationPayloadProtector)(nil)

func NewContinuationPayloadProtector(
	cipher *Store,
) (*ContinuationPayloadProtector, error) {
	if cipher == nil {
		return nil, connectorcore.NewError(connectorcore.ErrorCodeInternal, false, nil)
	}
	return &ContinuationPayloadProtector{cipher: cipher}, nil
}

func (protector *ContinuationPayloadProtector) Seal(
	ctx context.Context,
	organizationID uuid.UUID,
	receipt connectorexecution.RouteClaimReceipt,
	frozen connectorexecution.FrozenConfirmationRequest,
) (connectorexecution.ContinuationPayloadEnvelope, error) {
	if protector == nil || protector.cipher == nil || ctx == nil || ctx.Err() != nil ||
		organizationID == uuid.Nil || frozen.RouteReceipt != receipt ||
		frozen.Request.OrganizationID != organizationID || frozen.Request.Confirmation != nil {
		return "", connectorContinuationProtectorError()
	}
	aad, err := connectorContinuationCredentialAAD(organizationID, receipt)
	if err != nil {
		return "", connectorContinuationProtectorError()
	}
	payload, err := marshalConnectorContinuationPayload(frozen)
	if err != nil {
		return "", connectorContinuationProtectorError()
	}
	defer clear(payload)
	envelope, err := protector.cipher.Protect(aad, payload)
	if err != nil || strings.TrimSpace(envelope) == "" {
		return "", connectorContinuationProtectorError()
	}
	return connectorexecution.ContinuationPayloadEnvelope(envelope), nil
}

func (protector *ContinuationPayloadProtector) Open(
	ctx context.Context,
	organizationID uuid.UUID,
	receipt connectorexecution.RouteClaimReceipt,
	envelope connectorexecution.ContinuationPayloadEnvelope,
) (connectorexecution.FrozenConfirmationRequest, error) {
	if protector == nil || protector.cipher == nil || ctx == nil || ctx.Err() != nil ||
		organizationID == uuid.Nil || strings.TrimSpace(string(envelope)) == "" {
		return connectorexecution.FrozenConfirmationRequest{}, connectorContinuationProtectorError()
	}
	aad, err := connectorContinuationCredentialAAD(organizationID, receipt)
	if err != nil {
		return connectorexecution.FrozenConfirmationRequest{}, connectorContinuationProtectorError()
	}
	payload, err := protector.cipher.Open(aad, string(envelope))
	if err != nil {
		return connectorexecution.FrozenConfirmationRequest{}, connectorContinuationProtectorError()
	}
	defer clear(payload)
	frozen, err := unmarshalConnectorContinuationPayload(payload)
	if err != nil || frozen.RouteReceipt != receipt || frozen.Request.OrganizationID != organizationID ||
		frozen.Request.Confirmation != nil {
		return connectorexecution.FrozenConfirmationRequest{}, connectorContinuationProtectorError()
	}
	return frozen, nil
}

type connectorContinuationPayload struct {
	Version           int                                    `json:"version"`
	RouteReceipt      connectorexecution.RouteClaimReceipt   `json:"route_receipt"`
	Request           connectorContinuationExecutionRequest  `json:"request"`
	Origin            connectorexecution.ConfirmationOrigin  `json:"origin"`
	AcceptedOperation connectorContinuationAcceptedOperation `json:"accepted_operation"`
	Input             json.RawMessage                        `json:"input"`
}

type connectorContinuationAcceptedOperation struct {
	ConnectorID    string                      `json:"connector_id"`
	PluginID       string                      `json:"plugin_id"`
	PluginVersion  string                      `json:"plugin_version"`
	PluginDigest   string                      `json:"plugin_digest,omitempty"`
	ContractDigest string                      `json:"contract_digest"`
	Definition     connectorcatalog.Definition `json:"definition"`
	Operation      connectorcatalog.Operation  `json:"operation"`
}

type connectorContinuationExecutionRequest struct {
	OrganizationID     uuid.UUID                `json:"organization_id"`
	OwnerAccountID     uuid.UUID                `json:"owner_account_id"`
	ActingPrincipalID  uuid.UUID                `json:"acting_principal_id"`
	BindingID          uuid.UUID                `json:"binding_id"`
	ConnectionID       uuid.UUID                `json:"connection_id"`
	ConnectorID        connectorcore.Identifier `json:"connector_id"`
	Target             connectorcore.TargetRef  `json:"target"`
	OperationID        connectorcore.Identifier `json:"operation_id"`
	LogicalOperationID string                   `json:"logical_operation_id"`
	CorrelationID      string                   `json:"correlation_id"`
	Interactive        bool                     `json:"interactive"`
}

func marshalConnectorContinuationPayload(
	frozen connectorexecution.FrozenConfirmationRequest,
) ([]byte, error) {
	input, err := json.Marshal(frozen.Request.Input)
	if err != nil {
		return nil, connectorContinuationProtectorError()
	}
	defer clear(input)
	request := frozen.Request
	accepted := frozen.AcceptedOperation
	return json.Marshal(connectorContinuationPayload{
		Version: connectorContinuationPayloadVersion, RouteReceipt: frozen.RouteReceipt,
		Request: connectorContinuationExecutionRequest{
			OrganizationID: request.OrganizationID, OwnerAccountID: request.OwnerAccountID,
			ActingPrincipalID: request.ActingPrincipalID, BindingID: request.BindingID,
			ConnectionID: request.ConnectionID, ConnectorID: request.ConnectorID,
			Target: request.Target, OperationID: request.OperationID,
			LogicalOperationID: request.LogicalOperationID, CorrelationID: request.CorrelationID,
			Interactive: request.Interactive,
		},
		Origin: frozen.Origin,
		AcceptedOperation: connectorContinuationAcceptedOperation{
			ConnectorID: accepted.ConnectorID, PluginID: accepted.PluginID,
			PluginVersion: accepted.PluginVersion, PluginDigest: accepted.PluginDigest,
			ContractDigest: accepted.ContractDigest, Definition: accepted.Definition,
			Operation: accepted.Operation,
		},
		Input: input,
	})
}

func unmarshalConnectorContinuationPayload(
	payload []byte,
) (connectorexecution.FrozenConfirmationRequest, error) {
	var wire connectorContinuationPayload
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return connectorexecution.FrozenConfirmationRequest{}, connectorContinuationProtectorError()
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF ||
		wire.Version != connectorContinuationPayloadVersion || len(wire.Input) == 0 {
		return connectorexecution.FrozenConfirmationRequest{}, connectorContinuationProtectorError()
	}
	defer clear(wire.Input)
	var input any
	inputDecoder := json.NewDecoder(bytes.NewReader(wire.Input))
	if err := inputDecoder.Decode(&input); err != nil {
		return connectorexecution.FrozenConfirmationRequest{}, connectorContinuationProtectorError()
	}
	if err := inputDecoder.Decode(&struct{}{}); err != io.EOF {
		return connectorexecution.FrozenConfirmationRequest{}, connectorContinuationProtectorError()
	}
	request := wire.Request
	accepted := wire.AcceptedOperation
	accepted.Definition.PluginDigest = accepted.PluginDigest
	return connectorexecution.FrozenConfirmationRequest{
		RouteReceipt: wire.RouteReceipt,
		Request: connectorexecution.ExecutionRequest{
			OrganizationID: request.OrganizationID, OwnerAccountID: request.OwnerAccountID,
			ActingPrincipalID: request.ActingPrincipalID, BindingID: request.BindingID,
			ConnectionID: request.ConnectionID, ConnectorID: request.ConnectorID,
			Target: request.Target, OperationID: request.OperationID,
			LogicalOperationID: request.LogicalOperationID, CorrelationID: request.CorrelationID,
			Interactive: request.Interactive, Input: input,
		},
		Origin: wire.Origin,
		AcceptedOperation: connectorcatalog.PinnedOperation{
			ConnectorID: accepted.ConnectorID, PluginID: accepted.PluginID,
			PluginVersion: accepted.PluginVersion, PluginDigest: accepted.PluginDigest,
			ContractDigest: accepted.ContractDigest, Definition: accepted.Definition,
			Operation: accepted.Operation,
		},
	}, nil
}

func connectorContinuationCredentialAAD(organizationID uuid.UUID, receipt connectorexecution.RouteClaimReceipt) (Subject, error) {
	id, err := uuid.Parse(string(receipt))
	if err != nil || id == uuid.Nil || id.String() != string(receipt) || organizationID == uuid.Nil {
		return Subject{}, connectorContinuationProtectorError()
	}
	return Subject{Purpose: ConfirmationContinuation, OrganizationID: organizationID, ID: id, ConnectorID: connectorContinuationAADPurpose, Version: connectorContinuationPayloadVersion}, nil
}

func connectorContinuationProtectorError() error {
	return connectorcore.NewError(connectorcore.ErrorCodeInternal, false, nil)
}
