package credentials

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
)

// PersonalConnectionReader is the internal, full-record read required before
// opening a Connector credential envelope. Public connection services erase
// EncryptedCredentials and must not be used for this operation.
type PersonalConnectionReader interface {
	GetConnection(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (core.PersonalConnection, error)
}

// PersonalConnectionResolver combines the owner-scoped repository lookup and
// the Connector keyring. It intentionally returns the full connection only to
// another server-side security boundary; callers must clear Values after use.
type PersonalConnectionResolver struct {
	reader PersonalConnectionReader
	store  connections.MaintenanceCredentialResolver
}

func NewPersonalConnectionResolver(reader PersonalConnectionReader, store connections.MaintenanceCredentialResolver) *PersonalConnectionResolver {
	if reader == nil || store == nil {
		panic("personal connection resolver requires reader and credential store")
	}
	return &PersonalConnectionResolver{reader: reader, store: store}
}

// ResolvePersonalConnectionCredentials reads the full, owner-scoped record and
// fails closed unless it is ready, current, live, and still carries an
// encrypted envelope. It never logs the record or the decrypted values; the
// returned map is caller-owned and must be cleared after use.
func (resolver *PersonalConnectionResolver) ResolvePersonalConnectionCredentials(
	ctx context.Context,
	organizationID, ownerAccountID, connectionID uuid.UUID,
) (core.PersonalConnection, map[string]string, error) {
	if resolver == nil || resolver.reader == nil || resolver.store == nil ||
		organizationID == uuid.Nil || ownerAccountID == uuid.Nil || connectionID == uuid.Nil {
		return core.PersonalConnection{}, nil, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	connection, err := resolver.reader.GetConnection(ctx, organizationID, ownerAccountID, connectionID)
	if err != nil {
		return core.PersonalConnection{}, nil, err
	}
	if connection.ID != connectionID || connection.OrganizationID != organizationID ||
		connection.OwnerAccountID != ownerAccountID || connection.DeletedAt != nil ||
		connection.State != core.ReadinessReady || connection.Revision < 1 || connection.CredentialVersion < 1 ||
		connection.EncryptedCredentials == "" {
		return core.PersonalConnection{}, nil, core.NewError(core.ErrorCodeConnectionNotReady, false, nil)
	}
	values, err := resolver.store.ResolveConnectionCredentials(ctx, connection)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return core.PersonalConnection{}, nil, err
		}
		return core.PersonalConnection{}, nil, core.NewError(core.ErrorCodeConnectionNotReady, false, nil)
	}
	return connection, values, nil
}

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
