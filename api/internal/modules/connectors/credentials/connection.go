package credentials

import (
	"context"
	"encoding/json"

	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
)

var _ connections.CredentialSealer = (*Store)(nil)
var _ execution.CredentialResolver = (*Store)(nil)
var _ connections.MaintenanceCredentialResolver = (*Store)(nil)

func (store *Store) ResolveConnectionCredentials(ctx context.Context, connection core.PersonalConnection) (map[string]string, error) {
	return store.ResolveCredentials(ctx, execution.CredentialResolutionRequest{
		OrganizationID: connection.OrganizationID, ConnectionID: connection.ID,
		ConnectorID: connection.ConnectorID, CredentialVersion: connection.CredentialVersion,
		EncryptedCredentials: connection.EncryptedCredentials,
	})
}

func (store *Store) Seal(request connections.CredentialSealRequest) (string, error) {
	if len(request.Values) == 0 {
		return "", core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	data, err := json.Marshal(request.Values)
	if err != nil {
		return "", core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	defer clear(data)
	envelope, err := store.Protect(Subject{Purpose: ConnectionKey, OrganizationID: request.OrganizationID, ID: request.ConnectionID, ConnectorID: request.ConnectorID, Version: request.CredentialVersion}, data)
	if err != nil {
		return "", core.NewError(core.ErrorCodeInternal, false, err)
	}
	return envelope, nil
}

func (store *Store) ResolveCredentials(ctx context.Context, request execution.CredentialResolutionRequest) (map[string]string, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, core.NewError(core.ErrorCodeConnectionNotReady, false, nil)
	}
	data, err := store.Open(Subject{Purpose: ConnectionKey, OrganizationID: request.OrganizationID, ID: request.ConnectionID, ConnectorID: request.ConnectorID.String(), Version: request.CredentialVersion}, request.EncryptedCredentials)
	if err != nil {
		return nil, core.NewError(core.ErrorCodeConnectionNotReady, false, err)
	}
	defer clear(data)
	var values map[string]string
	if json.Unmarshal(data, &values) != nil || len(values) == 0 {
		clear(values)
		return nil, core.NewError(core.ErrorCodeConnectionNotReady, false, nil)
	}
	return values, nil
}
